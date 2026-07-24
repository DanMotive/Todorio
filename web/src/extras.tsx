// Additional Todorio blocks: announcements, digest, statistics, attachments, TOTP, notes,
// activity feed, focus timer, search, server settings.
import { useEffect, useRef, useState } from "react"
import { api, type Me, type Note, type ActivityEvent, type SearchResult, type SettingDef, type ActiveFocus } from "./api"
import { tr, trFormal, getFormattingLocale } from "./i18n"
import {
  IconAlertTriangle, IconAlertCircle, IconInfo, IconPin, IconMessage, IconCheckCircle,
  IconBarChart, IconAward, IconPaperclip, IconLock, IconUserPlus, IconCircle, IconFileText,
  IconSliders, IconPlay, IconPause, IconClock, IconArchive, IconRefresh, IconTrash, IconList,
  IconX,
} from "./icons"

// ---------- root announcements ----------

type Announcement = {
  id: number
  space_id: number | null
  level: "normal" | "important" | "emergency"
  body: string
  requires_ack: boolean
}

const LEVEL_STYLE: Record<string, React.CSSProperties> = {
  normal: { borderLeft: "4px solid var(--accent)" },
  important: { borderLeft: "4px solid #F5B301" },
  emergency: { borderLeft: "4px solid #E5484D", background: "rgba(229,72,77,.08)" },
}

export function AnnouncementsBanner() {
  const [items, setItems] = useState<Announcement[]>([])
  useEffect(() => {
    api.get("/api/announcements/active").then((r) => setItems(r.announcements)).catch(() => {})
  }, [])
  if (!items.length) return null
  const ack = async (id: number) => {
    try { await api.post(`/api/announcements/${id}/ack`) } catch { /* ignore */ }
    setItems((xs) => xs.filter((x) => x.id !== id))
  }
  return (
    <div style={{ marginBottom: 12 }}>
      {items.map((a) => (
        <div key={a.id} className="card" style={{ ...LEVEL_STYLE[a.level], padding: 12, marginBottom: 8, display: "flex", gap: 12, alignItems: "center" }}>
          <span style={{ flexShrink: 0 }}>
            {a.level === "emergency" ? <IconAlertTriangle /> : a.level === "important" ? <IconAlertCircle /> : <IconInfo />}
          </span>
          <div style={{ flex: 1 }}>{a.body}</div>
          <button className="nav-btn" onClick={() => ack(a.id)}>
            {a.requires_ack ? tr("announce.ack") : tr("announce.hide")}
          </button>
        </div>
      ))}
    </div>
  )
}

// ---------- "while you were away" digest ----------

type Digest = {
  show: boolean
  since?: string
  summary?: { assigned_to_me: number; new_comments: number; done_nearby: number; announcements: number }
}

export function DigestModal() {
  const [d, setD] = useState<Digest | null>(null)
  useEffect(() => {
    api.get("/api/digest").then(setD).catch(() => {})
  }, [])
  if (!d?.show || !d.summary) return null
  const close = () => {
    api.post("/api/digest/dismiss").catch(() => {})
    setD(null)
  }
  const rows: Array<[React.ReactNode, string, number]> = [
    [<IconPin size={14} />, tr("digest.assigned"), d.summary.assigned_to_me],
    [<IconMessage size={14} />, tr("digest.comments"), d.summary.new_comments],
    [<IconCheckCircle size={14} />, tr("digest.done"), d.summary.done_nearby],
    [<IconInfo size={14} />, tr("digest.announcements"), d.summary.announcements],
  ]
  return (
    <div className="card" style={{ padding: 16, marginBottom: 12, borderLeft: "4px solid var(--accent)" }}>
      <div className="row" style={{ justifyContent: "space-between" }}>
        <b>{tr("digest.title")} ({d.since ? new Date(d.since).toLocaleString(getFormattingLocale()) : ""})</b>
        <button className="nav-btn" onClick={close}>{tr("digest.ok")}</button>
      </div>
      <div style={{ marginTop: 8, display: "flex", gap: 16, flexWrap: "wrap" }}>
        {rows.filter(([, , n]) => n > 0).map(([icon, label, n]) => (
          <span key={label} className="row" style={{ gap: 5 }}>{icon} {label}: <b>{n}</b></span>
        ))}
      </div>
    </div>
  )
}

// ---------- space statistics ----------

type StatsMember = { id: number; username: string; name: string; done: number; done_weight: number; overdue: number }
type Stats = {
  enabled?: boolean
  period?: string
  members?: StatsMember[]
  caption?: { part1: string; part2: string; category: string }
  best?: StatsMember
}

export function StatsCard({ spaceId }: { spaceId: number }) {
  const [stats, setStats] = useState<Stats | null>(null)
  const [period, setPeriod] = useState<"week" | "month">("week")
  useEffect(() => {
    api.get(`/api/spaces/${spaceId}/stats?period=${period}`).then(setStats).catch(() => {})
  }, [spaceId, period])
  if (!stats || stats.enabled === false || !stats.members?.length) return null
  const members = stats.members
  const max = Math.max(...members.map((m) => m.done_weight), 1)
  return (
    <div className="card" style={{ padding: 14, marginBottom: 12 }}>
      <div className="row" style={{ justifyContent: "space-between" }}>
        <b className="row" style={{ gap: 5 }}><IconBarChart size={15} /> {tr("stats.title")}</b>
        <span>
          <button className={"nav-btn" + (period === "week" ? " active" : "")} onClick={() => setPeriod("week")}>{tr("stats.week")}</button>
          <button className={"nav-btn" + (period === "month" ? " active" : "")} onClick={() => setPeriod("month")}>{tr("stats.month")}</button>
        </span>
      </div>
      {(stats.caption?.part1 || stats.caption?.part2) && (
        <div className="muted" style={{ margin: "6px 0 10px" }}>
          {stats.caption?.part1} {stats.caption?.part2}
        </div>
      )}
      {stats.best && (
        <div className="row" style={{ marginBottom: 10, gap: 5 }}>
          <IconAward size={15} /> {tr("stats.best")}: <b>@{stats.best.username}</b> · <IconCheckCircle size={13} /> {stats.best.done}
        </div>
      )}
      {members.map((m) => (
        <div key={m.id} style={{ marginBottom: 6 }}>
          <div className="row" style={{ justifyContent: "space-between", fontSize: 13 }}>
            <span>@{m.username}</span>
            <span className="muted row" style={{ gap: 4, display: "inline-flex" }}>
              <IconCheckCircle size={13} /> {m.done}
              {m.overdue > 0 ? <span className="row" style={{ gap: 4 }}>· <IconAlertCircle size={13} /> {m.overdue}</span> : null}
            </span>
          </div>
          <div style={{ height: 6, borderRadius: 3, background: "rgba(128,128,128,.15)" }}>
            <div style={{ height: 6, borderRadius: 3, width: `${Math.round((m.done_weight / max) * 100)}%`, background: "var(--accent)" }} />
          </div>
        </div>
      ))}
    </div>
  )
}

// ---------- image attachments ----------

type Attachment = { id: number; mime_type: string; size_bytes: number }

export function AttachmentsBlock({ taskId }: { taskId: number }) {
  const [items, setItems] = useState<Attachment[]>([])
  const [busy, setBusy] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)
  const load = () =>
    api.get(`/api/tasks/${taskId}/attachments`).then((r) => setItems(r.attachments)).catch(() => {})
  useEffect(() => { load() }, [taskId])

  const upload = async (f: File) => {
    setBusy(true)
    try {
      const fd = new FormData()
      fd.append("file", f)
      const r = await fetch(`/api/tasks/${taskId}/attachments`, { method: "POST", body: fd, credentials: "same-origin" })
      if (!r.ok) {
        const e = await r.json().catch(() => null)
        alert(e?.error ?? tr("attach.failed"))
      }
      await load()
    } finally {
      setBusy(false)
      if (fileRef.current) fileRef.current.value = ""
    }
  }

  return (
    <div style={{ margin: "8px 0 12px" }}>
      <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
        {items.map((a) => (
          <a key={a.id} href={`/api/attachments/${a.id}`} target="_blank" rel="noreferrer">
            <img src={`/api/attachments/${a.id}`} alt={`#${a.id}`}
              style={{ width: 84, height: 84, objectFit: "cover", borderRadius: 8 }} />
          </a>
        ))}
      </div>
      <label className="nav-btn row" style={{ display: "inline-flex", gap: 5, marginTop: 6, cursor: "pointer" }}>
        {busy ? tr("attach.uploading") : <><IconPaperclip size={14} /> {tr("attach.add")}</>}
        <input ref={fileRef} type="file" accept="image/*" style={{ display: "none" }}
          onChange={(e) => e.target.files?.[0] && upload(e.target.files[0])} />
      </label>
    </div>
  )
}

// ---------- TOTP (two-factor auth — available to every account, spec calls it out as
// "especially important for root", not root/admin-exclusive) ----------

export function TotpCard() {
  const [setup, setSetup] = useState<{ secret: string; otpauth: string } | null>(null)
  const [code, setCode] = useState("")
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)

  const start = async () => {
    try { setSetup(await api.post("/api/me/totp/setup")); setMsg(null) } catch (e: any) { setMsg({ ok: false, text: e.message }) }
  }
  const enable = async () => {
    try {
      await api.post("/api/me/totp/enable", { code })
      setSetup(null); setCode(""); setMsg({ ok: true, text: tr("totp.enabled") })
    } catch (e: any) { setMsg({ ok: false, text: e.message }) }
  }
  const disable = async () => {
    const c = prompt(tr("totp.disable_prompt"))
    if (!c) return
    try { await api.post("/api/me/totp/disable", { code: c }); setMsg({ ok: true, text: tr("totp.disabled") }) } catch (e: any) { setMsg({ ok: false, text: e.message }) }
  }

  return (
    <div className="card" style={{ padding: 14, marginTop: 12 }}>
      <b className="row" style={{ gap: 5 }}><IconLock size={15} /> {tr("totp.title")}</b>
      <div className="muted" style={{ margin: "6px 0" }}>
        {tr("totp.desc")}
      </div>
      {!setup ? (
        <div className="row" style={{ gap: 8 }}>
          <button className="nav-btn" onClick={start}>{tr("totp.setup")}</button>
          <button className="nav-btn" onClick={disable}>{tr("totp.disable")}</button>
        </div>
      ) : (
        <div>
          <div style={{ margin: "6px 0" }}>
            {tr("totp.step1")} <code>{setup.secret}</code>
            <div className="muted" style={{ wordBreak: "break-all" }}>{tr("totp.or_link")} {setup.otpauth}</div>
          </div>
          <div className="row" style={{ gap: 8 }}>
            2. <input value={code} onChange={(e) => setCode(e.target.value)} placeholder={tr("totp.code")} maxLength={6} />
            <button className="nav-btn" onClick={enable}>{tr("totp.confirm")}</button>
          </div>
        </div>
      )}
      {msg && (
        <div className="row" style={{ marginTop: 6, gap: 5, color: msg.ok ? "var(--pulse-ok)" : "var(--due-overdue)" }}>
          {msg.ok ? <IconCheckCircle size={14} /> : <IconAlertCircle size={14} />} {msg.text}
        </div>
      )}
    </div>
  )
}

// ---------- invites (admin panel) ----------

type Invite = {
  id: number
  code: string
  role: string
  max_uses: number
  used_count: number
  expires_at: string | null
  created_by: string
}

export function InvitesCard({ me }: { me: Me }) {
  const [invites, setInvites] = useState<Invite[]>([])
  const [lastCode, setLastCode] = useState("")

  const load = () => {
    api.get("/api/invites").then((d: any) => setInvites(d.invites)).catch(() => {})
  }
  useEffect(load, [])

  if (me.role !== "root" && me.role !== "admin") return null

  const create = async () => {
    try {
      const d: any = await api.post("/api/invites", { max_uses: 1, expires_days: 7 })
      setLastCode(d.code)
      load()
    } catch (e) {
      alert(String((e as Error).message || e))
    }
  }
  const remove = async (id: number) => {
    try {
      await api.del(`/api/invites/${id}`)
      load()
    } catch {
      /* ignore */
    }
  }

  return (
    <div className="card" style={{ marginTop: 12 }}>
      <h3 className="row" style={{ marginTop: 0, gap: 6 }}><IconUserPlus size={17} /> {trFormal("invites.title")}</h3>
      <p style={{ opacity: 0.7, fontSize: 13 }}>
        {trFormal("invites.hint")} <code>todorio server policy set users.can_invite true</code>
      </p>
      <button className="btn" onClick={create}>{trFormal("invites.create")}</button>
      {lastCode && (
        <p>
          {trFormal("invites.new_code")} <code>{lastCode}</code>
        </p>
      )}
      {invites.map((i) => (
        <div key={i.id} className="row" style={{ justifyContent: "space-between", padding: "4px 0" }}>
          <span>
            <code>{i.code}</code> · {i.role} · {i.used_count}/{i.max_uses} · {trFormal("invites.by")} {i.created_by}
          </span>
          <button className="nav-btn" onClick={() => remove(i.id)}>{trFormal("invites.delete")}</button>
        </div>
      ))}
    </div>
  )
}

// ---------- public read-only list page (/s/{token}) ----------

export function PublicListPage({ token }: { token: string }) {
  const [data, setData] = useState<any>(null)
  const [error, setError] = useState("")

  useEffect(() => {
    fetch(`/api/public/${token}`)
      .then(async (r) => {
        const d = await r.json()
        if (!r.ok) throw new Error(d.error || tr("public.invalid"))
        return d
      })
      .then(setData)
      .catch((e) => setError(String((e as Error).message || e)))
  }, [token])

  return (
    <div className="center-page" style={{ alignItems: "flex-start", paddingTop: 40 }}>
      <div className="card" style={{ width: "min(680px, 92vw)", margin: "0 auto" }}>
        <div className="row">
          <img src="/icons/logo.svg" alt="" width={28} height={28} />
          <h2 style={{ margin: 0 }}>{data ? data.list.name : "Todorio"}</h2>
        </div>
        <p style={{ opacity: 0.6, fontSize: 13 }}>{tr("public.readonly")}</p>
        {error && <p>{error}</p>}
        {data &&
          data.tasks.map((t: any) => (
            <div
              key={t.id}
              className="row"
              style={{ padding: "6px 0", borderBottom: "1px solid rgba(255,255,255,0.06)" }}
            >
              <span style={{ color: t.completed_at ? "var(--pulse-ok)" : "var(--text-muted)" }}>
                {t.completed_at ? <IconCheckCircle size={15} /> : <IconCircle size={15} />}
              </span>
              <span style={{ textDecoration: t.completed_at ? "line-through" : "none" }}>{t.title}</span>
              <span style={{ marginLeft: "auto", opacity: 0.6, fontSize: 12 }}>
                {t.due_at ? new Date(t.due_at).toLocaleDateString(getFormattingLocale()) : ""}
              </span>
            </div>
          ))}
        {data && data.tasks.length === 0 && <p>{tr("public.empty")}</p>}
      </div>
    </div>
  )
}

// ---------- notes (Markdown pages inside a space) ----------

function NoteModal({ note, onClose, onChanged }: { note: Note; onClose: () => void; onChanged: () => void }) {
  const [title, setTitle] = useState(note.title)
  const [body, setBody] = useState(note.body || "")
  const [saved, setSaved] = useState(true)

  async function save() {
    await api.patch(`/api/notes/${note.id}`, { title, body }).catch(() => {})
    setSaved(true)
    onChanged()
  }
  async function remove() {
    await api.del(`/api/notes/${note.id}`).catch(() => {})
    onChanged()
    onClose()
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()} style={{ maxWidth: 640 }}>
        <input className="input" style={{ fontSize: 18, fontWeight: 600, marginBottom: 10 }}
          value={title} onChange={(e) => { setTitle(e.target.value); setSaved(false) }} />
        <textarea className="input" style={{ minHeight: 260, fontFamily: "inherit", resize: "vertical" }}
          value={body} onChange={(e) => { setBody(e.target.value); setSaved(false) }} />
        <div className="row" style={{ marginTop: 12, justifyContent: "space-between" }}>
          <button className="nav-btn" style={{ color: "var(--due-overdue)" }} onClick={remove}>{tr("task.archive")}</button>
          <div className="row">
            {!saved && <span className="muted">{tr("notes.unsaved")}</span>}
            <button className="btn" onClick={save}>{tr("notes.save")}</button>
            <button className="nav-btn" onClick={onClose}>{tr("common.back")}</button>
          </div>
        </div>
      </div>
    </div>
  )
}

export function NotesPanel({ spaceId }: { spaceId: number }) {
  const [notes, setNotes] = useState<Note[]>([])
  const [open, setOpen] = useState<Note | null>(null)
  const [title, setTitle] = useState("")
  const load = () => api.get(`/api/spaces/${spaceId}/notes`).then((r) => setNotes(r.notes)).catch(() => {})
  useEffect(() => { load() }, [spaceId])

  async function create() {
    if (!title.trim()) return
    await api.post(`/api/spaces/${spaceId}/notes`, { title }).catch(() => {})
    setTitle("")
    load()
  }
  async function openNote(n: Note) {
    const r = await api.get(`/api/notes/${n.id}`).catch(() => null)
    if (r) setOpen(r.note)
  }

  return (
    <div>
      {notes.map((n) => (
        <div key={n.id} className="task-row" onClick={() => openNote(n)}>
          <span className="task-title row" style={{ gap: 6 }}><IconFileText size={14} /> {n.title}</span>
          <span className="muted">{new Date(n.updated_at).toLocaleDateString(getFormattingLocale())}</span>
        </div>
      ))}
      {notes.length === 0 && <p className="muted">{tr("notes.empty")}</p>}
      <form className="row" style={{ marginTop: 12 }} onSubmit={(e) => { e.preventDefault(); create() }}>
        <input className="input grow" placeholder={tr("notes.new_placeholder")} value={title} onChange={(e) => setTitle(e.target.value)} />
        <button className="btn" type="submit">{tr("common.create")}</button>
      </form>
      {open && <NoteModal note={open} onClose={() => setOpen(null)} onChanged={load} />}
    </div>
  )
}

// ---------- space activity feed ----------

export function ActivityPanel({ spaceId }: { spaceId: number }) {
  const [events, setEvents] = useState<ActivityEvent[]>([])
  useEffect(() => {
    api.get(`/api/spaces/${spaceId}/activity`).then((r) => {
      // the API concatenates three separately-sorted queries — re-sort client-side (documented API quirk)
      const sorted = [...(r.events as ActivityEvent[])].sort((a, b) => new Date(b.at).getTime() - new Date(a.at).getTime())
      setEvents(sorted)
    }).catch(() => {})
  }, [spaceId])
  const ICON: Record<string, React.ReactNode> = {
    task_created: <IconPin size={14} />, task_completed: <IconCheckCircle size={14} />, comment: <IconMessage size={14} />,
  }
  if (events.length === 0) return <p className="muted">{tr("activity.empty")}</p>
  return (
    <div>
      {events.map((e, i) => (
        <div key={i} className="task-row" style={{ cursor: "default" }}>
          <span className="task-title row" style={{ gap: 6 }}>
            {ICON[e.type] || <IconCircle size={14} />} {tr("activity." + e.type)} · «{e.title}» · @{e.by}
          </span>
          <span className="muted">{new Date(e.at).toLocaleString(getFormattingLocale())}</span>
        </div>
      ))}
    </div>
  )
}

// ---------- archive (spec section 11: restore + a 3-day warning before auto-cleanup) ----------

type ArchivedList = { id: number; name: string; archived_at: string; archived_by: number | null }
type ArchivedTask = { id: number; title: string; list_id: number; list_name: string; archived_at: string; archived_by: number | null }

// daysLeft computes how many days remain before the worker's cleanupArchive would permanently
// delete something archived at `archivedAt`, given the space/server's retention_days policy.
function daysLeft(archivedAt: string, retentionDays: string): number {
  const archived = new Date(archivedAt).getTime()
  const deadline = archived + Number(retentionDays) * 86400000
  return Math.max(0, Math.ceil((deadline - Date.now()) / 86400000))
}

export function ArchivePanel({ me, spaceId }: { me: Me; spaceId: number }) {
  const [lists, setLists] = useState<ArchivedList[]>([])
  const [tasks, setTasks] = useState<ArchivedTask[]>([])
  const [retentionDays, setRetentionDays] = useState("30")

  const load = () => api.get(`/api/spaces/${spaceId}/archive`).then((r) => {
    setLists(r.lists); setTasks(r.tasks); setRetentionDays(String(r.retention_days))
  }).catch(() => {})
  useEffect(() => { load() }, [spaceId])

  async function restoreList(id: number) { await api.post(`/api/lists/${id}/restore`).catch(() => {}); load() }
  async function restoreTask(id: number) { await api.post(`/api/tasks/${id}/restore`).catch(() => {}); load() }
  async function deleteListForever(id: number, name: string) {
    if (!window.confirm(tr("archive.confirm_delete").replace("{name}", name))) return
    await api.del(`/api/lists/${id}/permanent`).catch(() => {}); load()
  }
  async function deleteTaskForever(id: number, title: string) {
    if (!window.confirm(tr("archive.confirm_delete").replace("{name}", title))) return
    await api.del(`/api/tasks/${id}/permanent`).catch(() => {}); load()
  }

  if (lists.length === 0 && tasks.length === 0) {
    return <p className="muted row" style={{ gap: 6 }}><IconArchive size={14} /> {tr("archive.empty")}</p>
  }
  return (
    <div>
      <p className="muted" style={{ marginTop: 0, fontSize: 12 }}>{tr("archive.hint").replace("{days}", retentionDays)}</p>
      {lists.map((l) => (
        <div key={"list-" + l.id} className="task-row" style={{ cursor: "default" }}>
          <span className="task-title row" style={{ gap: 6 }}><IconList size={14} /> {l.name}</span>
          <span className="muted">{tr("archive.days_left").replace("{n}", String(daysLeft(l.archived_at, retentionDays)))}</span>
          <button className="nav-btn row" style={{ gap: 4 }} onClick={() => restoreList(l.id)}><IconRefresh size={13} /> {tr("archive.restore")}</button>
          {me.role === "root" && (
            <button className="nav-btn row" style={{ gap: 4, color: "var(--due-overdue)" }} onClick={() => deleteListForever(l.id, l.name)}>
              <IconTrash size={13} /> {tr("archive.delete_forever")}
            </button>
          )}
        </div>
      ))}
      {tasks.map((t) => (
        <div key={"task-" + t.id} className="task-row" style={{ cursor: "default" }}>
          <span className="task-title row" style={{ gap: 6 }}><IconPin size={14} /> {t.title} <span className="muted">· {t.list_name}</span></span>
          <span className="muted">{tr("archive.days_left").replace("{n}", String(daysLeft(t.archived_at, retentionDays)))}</span>
          <button className="nav-btn row" style={{ gap: 4 }} onClick={() => restoreTask(t.id)}><IconRefresh size={13} /> {tr("archive.restore")}</button>
          {me.role === "root" && (
            <button className="nav-btn row" style={{ gap: 4, color: "var(--due-overdue)" }} onClick={() => deleteTaskForever(t.id, t.title)}>
              <IconTrash size={13} /> {tr("archive.delete_forever")}
            </button>
          )}
        </div>
      ))}
    </div>
  )
}

// ---------- archived spaces (top-level — a space's own archive isn't scoped to another space) ----------

type ArchivedSpace = { id: number; name: string; archived_at: string; archived_by: number | null }

export function ArchivedSpacesPanel({ me }: { me: Me }) {
  const [spaces, setSpaces] = useState<ArchivedSpace[]>([])
  const [retentionDays, setRetentionDays] = useState("30")

  const load = () => api.get("/api/archive/spaces").then((r) => {
    setSpaces(r.spaces); setRetentionDays(String(r.retention_days))
  }).catch(() => {})
  useEffect(() => { load() }, [])

  async function restore(id: number) { await api.post(`/api/spaces/${id}/restore`).catch(() => {}); load() }
  async function deleteForever(id: number, name: string) {
    if (!window.confirm(tr("archive.confirm_delete").replace("{name}", name))) return
    await api.del(`/api/spaces/${id}/permanent`).catch(() => {}); load()
  }

  if (spaces.length === 0) return <p className="muted row" style={{ gap: 6 }}><IconArchive size={14} /> {tr("archive.empty")}</p>
  return (
    <div>
      {spaces.map((s) => (
        <div key={s.id} className="task-row" style={{ cursor: "default" }}>
          <span className="task-title row" style={{ gap: 6 }}><IconArchive size={14} /> {s.name}</span>
          <span className="muted">{tr("archive.days_left").replace("{n}", String(daysLeft(s.archived_at, retentionDays)))}</span>
          <button className="nav-btn row" style={{ gap: 4 }} onClick={() => restore(s.id)}><IconRefresh size={13} /> {tr("archive.restore")}</button>
          {me.role === "root" && (
            <button className="nav-btn row" style={{ gap: 4, color: "var(--due-overdue)" }} onClick={() => deleteForever(s.id, s.name)}>
              <IconTrash size={13} /> {tr("archive.delete_forever")}
            </button>
          )}
        </div>
      ))}
    </div>
  )
}

// ---------- focus mode / time tracking ----------

export function FocusWidget({ taskId }: { taskId?: number }) {
  const [running, setRunning] = useState(false)
  const [elapsed, setElapsed] = useState(0)
  const [startedAt, setStartedAt] = useState<number | null>(null)

  useEffect(() => {
    if (!running || startedAt == null) return
    const id = setInterval(() => setElapsed(Math.floor((Date.now() - startedAt) / 1000)), 1000)
    return () => clearInterval(id)
  }, [running, startedAt])

  async function start() {
    await api.post("/api/focus/start", taskId ? { task_id: taskId } : {}).catch(() => {})
    setStartedAt(Date.now())
    setElapsed(0)
    setRunning(true)
  }
  async function stop() {
    await api.post("/api/focus/stop").catch(() => {})
    setRunning(false)
  }
  function fmt(s: number) {
    const m = Math.floor(s / 60).toString().padStart(2, "0")
    const ss = (s % 60).toString().padStart(2, "0")
    return `${m}:${ss}`
  }

  return (
    <div className="row" style={{ margin: "10px 0", gap: 8 }}>
      <button className={"nav-btn row" + (running ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={running ? stop : start}>
        {running ? <IconPause size={13} /> : <IconPlay size={13} />} {running ? tr("focus.stop") : tr("focus.start")}
      </button>
      {running && <span className="muted row" style={{ gap: 4, display: "inline-flex" }}><IconClock size={13} /> {fmt(elapsed)}</span>}
    </div>
  )
}

// ---------- presence: "who's working on this right now" ----------
// A caption + timestamp for teammates actively focused on a task (spec ask), built on the same
// focus_sessions rows FocusWidget above already writes — an open session with a task_id attached
// is "presence" for anyone else looking at that task. Deliberately a plain colored dot, not an
// emoji: same reasoning as Space Pulse's mood indicator (icons.tsx's header comment) — this app
// draws its own status dots so they render identically for every teammate.

function timeAgo(iso: string): string {
  const mins = Math.floor((Date.now() - new Date(iso).getTime()) / 60000)
  if (mins < 1) return tr("focus.presence.now")
  if (mins < 60) return tr("focus.presence.m").replace("{n}", String(mins))
  return tr("focus.presence.h").replace("{n}", String(Math.floor(mins / 60)))
}

// meId lets the caption say "You" instead of echoing the viewer's own username back at them.
export function presenceLabel(active: ActiveFocus[] | undefined, meId?: number): string | null {
  if (!active || active.length === 0) return null
  const first = active[0]
  const name = meId != null && first.user_id === meId ? tr("focus.presence.you") : first.username
  const time = timeAgo(first.started_at)
  return active.length === 1
    ? tr("focus.presence.one").replace("{name}", name).replace("{time}", time)
    : tr("focus.presence.many").replace("{name}", name).replace("{n}", String(active.length - 1)).replace("{time}", time)
}

// Compact by default (dot + text) for cards/rows; pass showLabel={false} for a dot-only badge
// where space is tight (e.g. a table row) — the full caption still shows on hover via title.
export function FocusPresence({ active, meId, showLabel = true }: {
  active: ActiveFocus[] | undefined; meId?: number; showLabel?: boolean
}) {
  const label = presenceLabel(active, meId)
  if (!label) return null
  const names = (active || []).map((f) => f.username).join(", ")
  return (
    <span className="focus-caption" title={names}>
      <span className="focus-dot" />
      {showLabel && <span>{label}</span>}
    </span>
  )
}

// ---------- global search ----------

export function SearchPage() {
  const [q, setQ] = useState("")
  const [results, setResults] = useState<SearchResult[]>([])
  const [busy, setBusy] = useState(false)

  async function run(query: string) {
    setQ(query)
    if (query.trim().length < 2) {
      setResults([])
      return
    }
    setBusy(true)
    try {
      const r = await api.get(`/api/search?q=${encodeURIComponent(query)}`)
      setResults(r.results)
    } catch {
      setResults([])
    }
    setBusy(false)
  }

  return (
    <div className="card">
      <h2>{tr("search.title")}</h2>
      <input className="input" placeholder={tr("search.placeholder")} value={q} onChange={(e) => run(e.target.value)} autoFocus />
      {busy && <p className="muted">{tr("search.searching")}</p>}
      {!busy && q.trim().length >= 2 && results.length === 0 && <p className="muted">{tr("search.empty")}</p>}
      {results.map((r, i) => (
        <div key={i} className="task-row" style={{ cursor: "default" }}>
          {r.type === "task" && <span className="task-title row" style={{ gap: 6 }}><IconCheckCircle size={14} /> {r.title}</span>}
          {r.type === "note" && <span className="task-title row" style={{ gap: 6 }}><IconFileText size={14} /> {r.title}</span>}
          {r.type === "comment" && <span className="task-title row" style={{ gap: 6 }}><IconMessage size={14} /> «{r.task_title}» — {r.snippet}</span>}
        </div>
      ))}
    </div>
  )
}

// ---------- server settings (root only) ----------

export function ServerSettingsCard({ me }: { me: Me }) {
  const [settings, setSettings] = useState<SettingDef[]>([])
  const [allLocales, setAllLocales] = useState<string[]>([])
  const [enabledLocales, setEnabledLocales] = useState<string[]>([])
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)
  const load = () => api.get("/api/admin/settings").then((r) => {
    setSettings(r.settings)
    setAllLocales(r.all_locales)
    setEnabledLocales(r.locales_enabled)
  }).catch(() => {})
  useEffect(() => { load() }, [])

  if (me.role !== "root") return null

  async function save(key: string, value: string) {
    try {
      await api.post("/api/admin/settings", { key, value })
      setMsg({ ok: true, text: key })
      load()
    } catch (e) {
      setMsg({ ok: false, text: (e as Error).message })
    }
  }

  async function toggleLocale(locale: string, enabled: boolean) {
    await api.post("/api/admin/locales", { locale, enabled }).catch(() => {})
    load()
  }

  return (
    <div className="card" style={{ marginTop: 12 }}>
      <h3 className="row" style={{ marginTop: 0, gap: 6 }}><IconSliders size={17} /> {trFormal("settings.title")}</h3>
      <p className="muted">{trFormal("settings.hint")}</p>
      {settings.map((s) => (
        <div key={s.key} className="row" style={{ margin: "8px 0", flexWrap: "wrap" }}>
          <label style={{ width: 260 }}>{s.label}</label>
          {s.type === "select" && (
            <select className="input" style={{ width: "auto" }} defaultValue={s.value} onChange={(e) => save(s.key, e.target.value)}>
              {(s.options || []).map((o) => <option key={o} value={o}>{o}</option>)}
            </select>
          )}
          {s.type === "bool" && (
            <select className="input" style={{ width: "auto" }} defaultValue={s.value} onChange={(e) => save(s.key, e.target.value)}>
              <option value="true">{trFormal("common.on")}</option>
              <option value="false">{trFormal("common.off")}</option>
            </select>
          )}
          {(s.type === "text" || s.type === "number") && (
            <input className="input" style={{ width: 220 }} type={s.type === "number" ? "number" : "text"}
              defaultValue={s.value} onBlur={(e) => save(s.key, e.target.value)} />
          )}
        </div>
      ))}
      <div className="section-title" style={{ fontSize: 13 }}>{trFormal("settings.locales")}</div>
      <div className="row" style={{ flexWrap: "wrap", gap: 10 }}>
        {allLocales.map((l) => (
          <label key={l} className="row" style={{ gap: 4, fontSize: 13 }}>
            <input type="checkbox" checked={enabledLocales.includes(l)}
              onChange={(e) => toggleLocale(l, e.target.checked)} />
            {l}
          </label>
        ))}
      </div>
      {msg && (
        <div className="row" style={{ marginTop: 8, gap: 5, color: msg.ok ? "var(--pulse-ok)" : "var(--due-overdue)" }}>
          {msg.ok ? <IconCheckCircle size={13} /> : <IconAlertCircle size={13} />} {msg.text}
        </div>
      )}
    </div>
  )
}

// ---------- admin: template management (backend existed in templates.go, no UI until now) ----------

type TemplateTaskDraft = { title: string; description: string; priority: string; due_in_days: string }
const blankTemplateTask = (): TemplateTaskDraft => ({ title: "", description: "", priority: "normal", due_in_days: "" })

export function TemplatesAdminCard({ me }: { me: Me }) {
  const [templates, setTemplates] = useState<any[]>([])
  const [name, setName] = useState("")
  const [listName, setListName] = useState("")
  const [autoApply, setAutoApply] = useState(false)
  const [taskDrafts, setTaskDrafts] = useState<TemplateTaskDraft[]>([blankTemplateTask()])
  const [error, setError] = useState("")

  const load = () => api.get("/api/templates").then((r) => setTemplates(r.templates)).catch(() => {})
  useEffect(() => { load() }, [])
  if (me.role !== "root") return null

  function updateDraft(i: number, patch: Partial<TemplateTaskDraft>) {
    setTaskDrafts((prev) => prev.map((d, idx) => (idx === i ? { ...d, ...patch } : d)))
  }
  function addDraftRow() { setTaskDrafts((prev) => [...prev, blankTemplateTask()]) }
  function removeDraftRow(i: number) { setTaskDrafts((prev) => (prev.length > 1 ? prev.filter((_, idx) => idx !== i) : prev)) }

  async function create(e: React.FormEvent) {
    e.preventDefault()
    setError("")
    if (!name.trim() || !listName.trim()) { setError(trFormal("templates.name_required")); return }
    const tasks = taskDrafts.filter((d) => d.title.trim()).map((d) => ({
      title: d.title, description: d.description, priority: d.priority,
      due_in_days: d.due_in_days ? Number(d.due_in_days) : null,
    }))
    if (tasks.length === 0) { setError(trFormal("templates.task_required")); return }
    try {
      await api.post("/api/admin/templates", {
        name, auto_apply: autoApply,
        body: JSON.stringify({ list_name: listName, tasks }),
      })
      setName(""); setListName(""); setAutoApply(false); setTaskDrafts([blankTemplateTask()])
      load()
    } catch (err) { setError((err as Error).message) }
  }

  async function remove(id: number) {
    await api.del(`/api/admin/templates/${id}`).catch(() => {})
    load()
  }

  return (
    <div className="card" style={{ padding: 14, marginTop: 12 }}>
      <b className="row" style={{ gap: 5 }}><IconFileText size={15} /> {trFormal("templates.admin_title")}</b>
      {templates.map((t) => (
        <div key={t.id} className="task-row" style={{ cursor: "default" }}>
          <span className="task-title row" style={{ gap: 6 }}>
            {t.name} {t.auto_apply && <span className="muted">· {trFormal("templates.auto_apply_badge")}</span>}
          </span>
          <button className="nav-btn" onClick={() => remove(t.id)}>{trFormal("templates.delete")}</button>
        </div>
      ))}
      <form onSubmit={create} style={{ marginTop: 10 }}>
        <div className="row" style={{ gap: 8, marginBottom: 8, flexWrap: "wrap" }}>
          <input className="input" style={{ maxWidth: 220 }} placeholder={trFormal("templates.name_placeholder")}
            value={name} onChange={(e) => setName(e.target.value)} />
          <input className="input" style={{ maxWidth: 220 }} placeholder={trFormal("templates.list_name_placeholder")}
            value={listName} onChange={(e) => setListName(e.target.value)} />
          <label className="row" style={{ gap: 4, fontSize: 13 }}>
            <input type="checkbox" checked={autoApply} onChange={(e) => setAutoApply(e.target.checked)} /> {trFormal("templates.auto_apply")}
          </label>
        </div>
        {taskDrafts.map((d, i) => (
          <div key={i} className="row" style={{ gap: 6, marginBottom: 6, flexWrap: "wrap" }}>
            <input className="input grow" style={{ minWidth: 140 }} placeholder={trFormal("templates.task_title_placeholder")}
              value={d.title} onChange={(e) => updateDraft(i, { title: e.target.value })} />
            <select className="input" style={{ width: "auto" }} value={d.priority} onChange={(e) => updateDraft(i, { priority: e.target.value })}>
              <option value="low">{trFormal("task.priority.low")}</option>
              <option value="normal">{trFormal("task.priority.normal")}</option>
              <option value="high">{trFormal("task.priority.high")}</option>
              <option value="urgent">{trFormal("task.priority.urgent")}</option>
            </select>
            <input className="input" style={{ width: 130 }} type="number" min={0} placeholder={trFormal("templates.due_in_days_placeholder")}
              value={d.due_in_days} onChange={(e) => updateDraft(i, { due_in_days: e.target.value })} />
            <button type="button" className="nav-btn" style={{ padding: "2px 6px" }} onClick={() => removeDraftRow(i)}><IconX size={11} /></button>
          </div>
        ))}
        <button type="button" className="nav-btn row" style={{ gap: 4, marginBottom: 8, display: "inline-flex" }} onClick={addDraftRow}>
          + {trFormal("templates.add_task")}
        </button>
        {error && <div className="error-text">{error}</div>}
        <div><button className="btn" type="submit">{trFormal("templates.create")}</button></div>
      </form>
    </div>
  )
}

// ---------- admin: create announcements (backend existed in announcements.go, no UI until now) ----------

export function AnnouncementsAdminCard({ me }: { me: Me }) {
  const [level, setLevel] = useState("normal")
  const [body, setBody] = useState("")
  const [requiresAck, setRequiresAck] = useState(false)
  const [expiresDays, setExpiresDays] = useState("")
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)
  if (me.role !== "root") return null

  async function create(e: React.FormEvent) {
    e.preventDefault()
    if (!body.trim()) return
    try {
      await api.post("/api/announcements", {
        level, body, requires_ack: requiresAck,
        expires_days: expiresDays ? Number(expiresDays) : null,
      })
      setBody(""); setRequiresAck(false); setExpiresDays("")
      setMsg({ ok: true, text: trFormal("announce.created") })
    } catch (err) { setMsg({ ok: false, text: (err as Error).message }) }
  }

  return (
    <div className="card" style={{ padding: 14, marginTop: 12 }}>
      <b className="row" style={{ gap: 5 }}><IconAlertTriangle size={15} /> {trFormal("announce.admin_title")}</b>
      <form onSubmit={create} style={{ marginTop: 8 }}>
        <div className="row" style={{ gap: 8, marginBottom: 8, flexWrap: "wrap" }}>
          <select className="input" style={{ width: "auto" }} value={level} onChange={(e) => setLevel(e.target.value)}>
            <option value="normal">{trFormal("announce.level.normal")}</option>
            <option value="important">{trFormal("announce.level.important")}</option>
            <option value="emergency">{trFormal("announce.level.emergency")}</option>
          </select>
          <input className="input" style={{ width: 150 }} type="number" min={0} placeholder={trFormal("announce.expires_days_placeholder")}
            value={expiresDays} onChange={(e) => setExpiresDays(e.target.value)} />
          <label className="row" style={{ gap: 4, fontSize: 13 }}>
            <input type="checkbox" checked={requiresAck} onChange={(e) => setRequiresAck(e.target.checked)} /> {trFormal("announce.requires_ack")}
          </label>
        </div>
        <textarea className="input" style={{ width: "100%", minHeight: 70, marginBottom: 8 }}
          placeholder={trFormal("announce.body_placeholder")} value={body} onChange={(e) => setBody(e.target.value)} />
        {msg && (
          <div className="row" style={{ gap: 5, marginBottom: 8, color: msg.ok ? "var(--pulse-ok)" : "var(--due-overdue)" }}>
            {msg.ok ? <IconCheckCircle size={13} /> : <IconAlertCircle size={13} />} {msg.text}
          </div>
        )}
        <button className="btn" type="submit">{trFormal("announce.publish")}</button>
      </form>
    </div>
  )
}

// ---------- custom field schema (spec section 13; backend existed in fields.go, TaskModal used a
// freeform key/value editor only — this defines the per-space typed schema that drives it) ----------

export type FieldDef = { key: string; label: string; type: string; options?: string[]; color?: string }
export const FIELD_TYPES = ["text", "number", "date", "select", "multiselect", "checkbox", "user", "link", "rating"]

export function FieldsPanel({ spaceId, isOwner }: { spaceId: number; isOwner: boolean }) {
  const [fields, setFields] = useState<FieldDef[]>([])
  const [key, setKey] = useState("")
  const [label, setLabel] = useState("")
  const [type, setType] = useState("text")
  const [optionsInput, setOptionsInput] = useState("")
  const [error, setError] = useState("")

  const load = () => api.get(`/api/spaces/${spaceId}/fields`).then((r) => setFields(r.fields || [])).catch(() => {})
  useEffect(() => { load() }, [spaceId])

  async function save(next: FieldDef[]) {
    setError("")
    try {
      await api.put(`/api/spaces/${spaceId}/fields`, { fields: next })
      setFields(next)
    } catch (err) { setError((err as Error).message) }
  }

  async function addField(e: React.FormEvent) {
    e.preventDefault()
    if (!key.trim() || !label.trim()) return
    if (fields.some((f) => f.key === key.trim())) { setError(tr("fields.key_taken")); return }
    const field: FieldDef = { key: key.trim(), label: label.trim(), type }
    if (type === "select" || type === "multiselect") {
      field.options = optionsInput.split(",").map((s) => s.trim()).filter(Boolean)
    }
    await save([...fields, field])
    setKey(""); setLabel(""); setType("text"); setOptionsInput("")
  }

  async function removeField(k: string) {
    await save(fields.filter((f) => f.key !== k))
  }

  return (
    <div>
      <p className="muted" style={{ marginTop: 0, fontSize: 12 }}>{tr("fields.hint")}</p>
      {fields.length === 0 && <p className="muted">{tr("fields.empty")}</p>}
      {fields.map((f) => (
        <div key={f.key} className="task-row" style={{ cursor: "default" }}>
          <span className="task-title row" style={{ gap: 6 }}>
            {f.label} <span className="muted">· {tr("fields.type." + f.type)}{f.options ? `: ${f.options.join(", ")}` : ""}</span>
          </span>
          {isOwner && <button className="nav-btn" onClick={() => removeField(f.key)}><IconTrash size={13} /></button>}
        </div>
      ))}
      {isOwner && (
        <form className="row" style={{ gap: 6, marginTop: 10, flexWrap: "wrap" }} onSubmit={addField}>
          <input className="input" style={{ maxWidth: 130 }} placeholder={tr("fields.key_placeholder")} value={key} onChange={(e) => setKey(e.target.value)} />
          <input className="input" style={{ maxWidth: 160 }} placeholder={tr("fields.label_placeholder")} value={label} onChange={(e) => setLabel(e.target.value)} />
          <select className="input" style={{ width: "auto" }} value={type} onChange={(e) => setType(e.target.value)}>
            {FIELD_TYPES.map((t) => <option key={t} value={t}>{tr("fields.type." + t)}</option>)}
          </select>
          {(type === "select" || type === "multiselect") && (
            <input className="input grow" style={{ minWidth: 160 }} placeholder={tr("fields.options_placeholder")}
              value={optionsInput} onChange={(e) => setOptionsInput(e.target.value)} />
          )}
          <button className="btn" type="submit">+ {tr("fields.add")}</button>
        </form>
      )}
      {error && <div className="error-text">{error}</div>}
    </div>
  )
}
