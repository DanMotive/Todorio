// Additional Todorio blocks: announcements, digest, statistics, attachments, TOTP, notes,
// activity feed, focus timer, search, server settings.
import { useEffect, useRef, useState } from "react"
import { createPortal } from "react-dom"
import { api, DEVELOPER_NAME, type Me, type Note, type ActivityEvent, type SearchResult, type SettingDef, type ActiveFocus, type Inbox, type InboxItem } from "./api"
import { tr, trFormal, trOr, getFormattingLocale } from "./i18n"
import { renderMarkdown } from "./markdown"
import { NoteTasksBlock } from "./functional"
import {
  IconAlertTriangle, IconAlertCircle, IconInfo, IconPin, IconMessage, IconCheckCircle,
  IconBarChart, IconAward, IconPaperclip, IconLock, IconUserPlus, IconCircle, IconFileText,
  IconSliders, IconPlay, IconPause, IconClock, IconArchive, IconRefresh, IconTrash, IconList,
  IconX, IconArrowLeft, IconInbox,
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
  // Leaderboard visibility (spec section 14). members[] is already trimmed server-side to
  // match; my_rank/total_ranked are computed before trimming so "your place" stays truthful
  // even when the table itself is hidden.
  visibility?: "full" | "top3" | "own" | "owner_only"
  my_rank?: number
  total_ranked?: number
}

export function StatsCard({ spaceId, canEdit }: { spaceId: number; canEdit?: boolean }) {
  const [stats, setStats] = useState<Stats | null>(null)
  const [period, setPeriod] = useState<"week" | "month">("week")
  const [rev, setRev] = useState(0)
  useEffect(() => {
    api.get(`/api/spaces/${spaceId}/stats?period=${period}`).then(setStats).catch(() => {})
  }, [spaceId, period, rev])
  // An empty member list no longer hides the card outright: with visibility "owner_only" a
  // non-owner legitimately receives zero rows, and the caption plus their own rank are still
  // worth showing. The card only disappears when stats are switched off server-wide.
  if (!stats || stats.enabled === false) return null
  const members = stats.members ?? []
  const max = Math.max(...members.map((m) => m.done_weight), 1)
  // "Your place: 3 of 8" — the useful part of a leaderboard when the table itself is hidden.
  const rankLine = stats.my_rank
    ? tr("stats.my_rank").replace("{rank}", String(stats.my_rank)).replace("{total}", String(stats.total_ranked ?? 0))
    : ""
  return (
    <div className="card" style={{ padding: 14, marginBottom: 12 }}>
      <div className="row" style={{ justifyContent: "space-between" }}>
        <b className="row" style={{ gap: 5 }}><IconBarChart size={15} /> {tr("stats.title")}</b>
        <span className="row" style={{ gap: 4 }}>
          <button className={"nav-btn" + (period === "week" ? " active" : "")} onClick={() => setPeriod("week")}>{tr("stats.week")}</button>
          <button className={"nav-btn" + (period === "month" ? " active" : "")} onClick={() => setPeriod("month")}>{tr("stats.month")}</button>
          {canEdit && (
            <select className="input" style={{ width: "auto", padding: "2px 6px", fontSize: 12 }}
              value={stats.visibility ?? "full"}
              title={tr("stats.visibility")}
              onChange={async (e) => {
                await api.patch(`/api/spaces/${spaceId}`, { settings: { stats: { visibility: e.target.value } } }).catch(() => {})
                setRev((v) => v + 1)
              }}>
              <option value="full">{tr("stats.vis_full")}</option>
              <option value="top3">{tr("stats.vis_top3")}</option>
              <option value="own">{tr("stats.vis_own")}</option>
              <option value="owner_only">{tr("stats.vis_owner")}</option>
            </select>
          )}
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
      {rankLine && <div className="muted" style={{ fontSize: 12, marginBottom: 8 }}>{rankLine}</div>}
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

// Attachments hang off either a task or a comment (spec section 7). Both use the same
// endpoint shape — /api/{tasks|comments}/{id}/attachments — so one component serves both;
// `target` picks which. Rendering is compact for comments, where the images sit inline
// under the comment body rather than in a section of their own.
export function AttachmentsBlock({ taskId, commentId, compact }: {
  taskId?: number; commentId?: number; compact?: boolean
}) {
  const [items, setItems] = useState<Attachment[]>([])
  const [busy, setBusy] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)
  const { confirm, confirmElement } = useConfirm()
  const base = commentId !== undefined ? `/api/comments/${commentId}` : `/api/tasks/${taskId}`
  const load = () =>
    api.get(`${base}/attachments`).then((r) => setItems(r.attachments)).catch(() => {})
  useEffect(() => { load() }, [taskId, commentId])

  // Deleting an attachment had no UI at all — the DELETE endpoint existed and was simply
  // unreachable. It is a genuine loss (the image is gone, not archived), so it goes through the
  // same confirmation dialog as every other permanent action, plain rather than type-to-confirm:
  // one small image is a much lower-stakes loss than a whole task or space.
  function removeAttachment(id: number) {
    confirm({
      title: tr("attach.confirm_delete_title"),
      confirmLabel: tr("task.archive"),
      danger: true,
      action: async () => { await api.del(`/api/attachments/${id}`).catch(() => {}); load() },
    })
  }

  const upload = async (f: File) => {
    setBusy(true)
    try {
      const fd = new FormData()
      fd.append("file", f)
      const r = await fetch(`${base}/attachments`, { method: "POST", body: fd, credentials: "same-origin" })
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

  const thumb = compact ? 56 : 84
  return (
    <div style={{ margin: compact ? "4px 0" : "8px 0 12px" }}>
      {confirmElement}
      <div style={{ display: "flex", gap: compact ? 6 : 8, flexWrap: "wrap" }}>
        {items.map((a) => (
          <div key={a.id} className="attach-thumb" style={{ width: thumb, height: thumb }}>
            <a href={`/api/attachments/${a.id}`} target="_blank" rel="noreferrer">
              <img src={`/api/attachments/${a.id}`} alt={`#${a.id}`}
                style={{ width: thumb, height: thumb, objectFit: "cover", borderRadius: 8 }} />
            </a>
            <button className="attach-remove" title={tr("attach.remove")}
              onClick={() => removeAttachment(a.id)}>
              <IconX size={11} />
            </button>
          </div>
        ))}
      </div>
      <label className="nav-btn row"
        style={{ display: "inline-flex", gap: 5, marginTop: compact ? 4 : 6, cursor: "pointer", fontSize: compact ? 12 : undefined }}>
        {busy ? tr("attach.uploading") : <><IconPaperclip size={compact ? 12 : 14} /> {tr("attach.add")}</>}
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
  // Recovery codes come back from /enable exactly once and are never retrievable afterwards,
  // so they stay on screen until the user navigates away rather than behind a toast.
  const [recovery, setRecovery] = useState<string[] | null>(null)

  const start = async () => {
    try { setSetup(await api.post("/api/me/totp/setup")); setMsg(null); setRecovery(null) } catch (e: any) { setMsg({ ok: false, text: e.message }) }
  }
  const enable = async () => {
    try {
      const res: any = await api.post("/api/me/totp/enable", { code })
      setSetup(null); setCode(""); setMsg({ ok: true, text: tr("totp.enabled") })
      setRecovery(Array.isArray(res?.recovery_codes) ? res.recovery_codes : null)
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
      {recovery && (
        <div style={{ marginTop: 10 }}>
          <b>{tr("totp.recovery_title")}</b>
          <div className="muted" style={{ margin: "4px 0" }}>{tr("totp.recovery_desc")}</div>
          <div className="row" style={{ flexWrap: "wrap", gap: 6 }}>
            {recovery.map((c) => <code key={c}>{c}</code>)}
          </div>
          <button className="nav-btn" style={{ marginTop: 6 }}
            onClick={() => navigator.clipboard?.writeText(recovery.join("\n"))}>{tr("totp.recovery_copy")}</button>
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
  const [needsPassword, setNeedsPassword] = useState(false)
  const [password, setPassword] = useState("")
  const [busy, setBusy] = useState(false)

  async function load(passwordValue?: string) {
    setBusy(true)
    setError("")
    try {
      const options: RequestInit = passwordValue === undefined ? {} : {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ password: passwordValue }),
      }
      const r = await fetch(`/api/public/${token}`, options)
      const d = await r.json().catch(() => ({}))
      if (!r.ok) {
        if (r.status === 401) {
          setNeedsPassword(true)
          throw new Error(passwordValue === undefined
            ? trOr("public.password_required", "Для просмотра нужен пароль")
            : trOr("public.password_invalid", "Неверный пароль"))
        }
        if (r.status === 429) throw new Error(trOr("public.too_many_attempts", "Слишком много попыток — попробуйте позже"))
        throw new Error(d.error || tr("public.invalid"))
      }
      setData(d)
      setNeedsPassword(false)
      setPassword("")
    } catch (e) {
      setError(String((e as Error).message || e))
    } finally {
      setBusy(false)
    }
  }

  useEffect(() => { load() }, [token])

  return (
    <div className="center-page" style={{ alignItems: "flex-start", paddingTop: 40 }}>
      <div className="card" style={{ width: "min(680px, 92vw)", margin: "0 auto" }}>
        <div className="row">
          <img src="/icons/logo.svg" alt="" width={28} height={28} />
          <h2 style={{ margin: 0 }}>{data ? data.list.name : "Todorio"}</h2>
        </div>
        <p style={{ opacity: 0.6, fontSize: 13 }}>{tr("public.readonly")}</p>
        {error && <p style={{ color: "var(--due-overdue)" }}>{error}</p>}
        {needsPassword && !data && (
          <form className="row" style={{ gap: 8, alignItems: "center", flexWrap: "wrap" }}
            onSubmit={(e) => { e.preventDefault(); if (password) load(password) }}>
            <input className="input" type="password" value={password} disabled={busy}
              autoComplete="current-password" style={{ maxWidth: 300 }}
              placeholder={trOr("public.password_placeholder", "Пароль ссылки")}
              onChange={(e) => setPassword(e.target.value)} autoFocus />
            <button className="btn" type="submit" disabled={busy || !password}>
              {busy ? trOr("common.loading", "Загрузка…") : trOr("public.open", "Открыть")}
            </button>
          </form>
        )}
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

export function NoteModal({ note, spaceId, onClose, onChanged }: {
  note: Note; spaceId: number; onClose: () => void; onChanged: () => void
}) {
  const { confirm, confirmElement } = useConfirm()
  const [title, setTitle] = useState(note.title)
  const [body, setBody] = useState(note.body || "")
  const [saved, setSaved] = useState(true)
  const [saveError, setSaveError] = useState("")
  // Notes are Markdown pages (spec section 12), but until now the body was only ever shown in a
  // textarea — the markup was never rendered. Opens in preview when there's already content to
  // read, and in edit mode for an empty note where there is nothing to preview.
  const [preview, setPreview] = useState(!!(note.body && note.body.trim()))

  async function save() {
    setSaveError("")
    try {
      await api.patch(`/api/notes/${note.id}`, { title, body })
      setSaved(true)
      onChanged()
    } catch (e) {
      setSaved(false)
      setSaveError((e as Error).message)
    }
  }
  async function remove() {
    setSaveError("")
    try {
      await api.del(`/api/notes/${note.id}`)
      onChanged()
      onClose()
    } catch (e) {
      setSaveError((e as Error).message)
    }
  }

  return (
    <ModalShell onClose={onClose} maxWidth={640}>
        {confirmElement}
        <input className="input" style={{ fontSize: 18, fontWeight: 600, marginBottom: 10 }}
          value={title} onChange={(e) => { setTitle(e.target.value); setSaved(false) }} />
        <div className="row" style={{ gap: 4, marginBottom: 8 }}>
          <button className={"nav-btn" + (!preview ? " active" : "")} onClick={() => setPreview(false)}>
            {tr("notes.edit")}
          </button>
          <button className={"nav-btn" + (preview ? " active" : "")} onClick={() => setPreview(true)}>
            {tr("notes.preview")}
          </button>
          <span className="muted" style={{ marginLeft: "auto", fontSize: 12 }}>{tr("notes.md_hint")}</span>
        </div>
        {preview ? (
          <div className="md-view">
            {body.trim()
              ? renderMarkdown(body)
              : <p className="muted">{tr("notes.empty_body")}</p>}
          </div>
        ) : (
          <textarea className="input" style={{ minHeight: 260, fontFamily: "inherit", resize: "vertical" }}
            value={body} onChange={(e) => { setBody(e.target.value); setSaved(false) }} />
        )}
        <NoteTasksBlock note={note} spaceId={spaceId} />
        {saveError && <div style={{ color: "var(--due-overdue)", marginTop: 8 }}>{saveError}</div>}
        <div className="row" style={{ marginTop: 12, justifyContent: "space-between" }}>
          <button className="nav-btn" style={{ color: "var(--due-overdue)" }}
            onClick={() => confirm({
              title: tr("notes.confirm_archive").replace("{title}", title),
              body: tr("confirm.archive_body"),
              confirmLabel: tr("task.archive"), danger: true, action: remove,
            })}>{tr("task.archive")}</button>
          <div className="row">
            {!saved && <span className="muted">{tr("notes.unsaved")}</span>}
            <button className="btn" onClick={save}>{tr("notes.save")}</button>
            <button className="nav-btn" onClick={onClose}>{tr("common.back")}</button>
          </div>
        </div>
    </ModalShell>
  )
}

export function NotesPanel({ spaceId, onOpenNote }: { spaceId: number; onOpenNote: (note: Note) => void }) {
  const [notes, setNotes] = useState<Note[]>([])
  const [title, setTitle] = useState("")
  const [error, setError] = useState("")
  const load = () => api.get(`/api/spaces/${spaceId}/notes`).then((r) => setNotes(r.notes)).catch(() => {})
  useEffect(() => { load() }, [spaceId])

  async function create() {
    if (!title.trim()) return
    setError("")
    try {
      await api.post(`/api/spaces/${spaceId}/notes`, { title })
      setTitle("")
      load()
    } catch (e) {
      setError((e as Error).message)
    }
  }
  return (
    <div>
      {notes.map((n) => (
        <div key={n.id} className="task-row" onClick={() => onOpenNote(n)}>
          <span className="task-title row" style={{ gap: 6 }}><IconFileText size={14} /> {n.title}</span>
          <span className="muted">{new Date(n.updated_at).toLocaleDateString(getFormattingLocale())}</span>
        </div>
      ))}
      {notes.length === 0 && <p className="muted">{tr("notes.empty")}</p>}
      {error && <p style={{ color: "var(--due-overdue)" }}>{error}</p>}
      <form className="row" style={{ marginTop: 12 }} onSubmit={(e) => { e.preventDefault(); create() }}>
        <input className="input grow" placeholder={tr("notes.new_placeholder")} value={title} onChange={(e) => setTitle(e.target.value)} />
        <button className="btn" type="submit">{tr("common.create")}</button>
      </form>
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
  const [error, setError] = useState("")

  const { confirm, confirmElement } = useConfirm()
  const load = () => api.get(`/api/spaces/${spaceId}/archive`).then((r) => {
    setLists(r.lists); setTasks(r.tasks); setRetentionDays(String(r.retention_days))
  }).catch(() => {})
  useEffect(() => { load() }, [spaceId])

  async function restoreList(id: number) {
    setError("")
    try { await api.post(`/api/lists/${id}/restore`); load() } catch (err) { setError((err as Error).message) }
  }
  async function restoreTask(id: number) {
    setError("")
    try { await api.post(`/api/tasks/${id}/restore`); load() } catch (err) { setError((err as Error).message) }
  }
  // Permanent deletion has no undo, so it asks the user to type the name rather than accepting a
  // single click (spec section 10 requires confirmation for exactly these actions).
  function deleteListForever(id: number, name: string) {
    confirm({
      title: tr("confirm.delete_forever_title"), body: tr("confirm.delete_forever_body"),
      confirmLabel: tr("archive.delete_forever"), requireText: name, danger: true,
      action: async () => { await api.del(`/api/lists/${id}/permanent`); load() },
    })
  }
  function deleteTaskForever(id: number, title: string) {
    confirm({
      title: tr("confirm.delete_forever_title"), body: tr("confirm.delete_forever_body"),
      confirmLabel: tr("archive.delete_forever"), requireText: title, danger: true,
      action: async () => { await api.del(`/api/tasks/${id}/permanent`); load() },
    })
  }

  if (lists.length === 0 && tasks.length === 0) {
    return <p className="muted row" style={{ gap: 6 }}><IconArchive size={14} /> {tr("archive.empty")}</p>
  }
  return (
    <div>
      {confirmElement}
      {error && <p className="error-text">{error}</p>}
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
  const [error, setError] = useState("")
  const { confirm, confirmElement } = useConfirm()

  const load = () => api.get("/api/archive/spaces").then((r) => {
    setSpaces(r.spaces); setRetentionDays(String(r.retention_days))
  }).catch(() => {})
  useEffect(() => { load() }, [])

  async function restore(id: number) {
    setError("")
    try { await api.post(`/api/spaces/${id}/restore`); load() } catch (err) { setError((err as Error).message) }
  }
  function deleteForever(id: number, name: string) {
    confirm({
      title: tr("confirm.delete_forever_title"), body: tr("confirm.delete_space_body"),
      confirmLabel: tr("archive.delete_forever"), requireText: name, danger: true,
      action: async () => { await api.del(`/api/spaces/${id}/permanent`); load() },
    })
  }

  if (spaces.length === 0) return <p className="muted row" style={{ gap: 6 }}><IconArchive size={14} /> {tr("archive.empty")}</p>
  return (
    <div>
      {confirmElement}
      {error && <p className="error-text">{error}</p>}
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

// FocusWidget — the per-task start/stop control inside the task modal.
//
// The elapsed time is NOT kept here. The component unmounts whenever the modal closes, so any
// local timer appeared to reset even though the server session was still running — that was a
// real bug, not cosmetic. The single source of truth is the server (GET /api/focus/current);
// this widget only issues start/stop and then tells the app shell to refresh, which renders
// the always-visible ticking timer in the sidebar.
export function FocusWidget({ taskId }: { taskId?: number }) {
  const [running, setRunning] = useState(false)
  const [busy, setBusy] = useState(false)

  const sync = () =>
    api.get("/api/focus/current")
      .then((r) => setRunning(!!r.running && (taskId === undefined || r.task_id === taskId)))
      .catch(() => {})
  useEffect(() => { sync() }, [taskId])

  async function toggle() {
    setBusy(true)
    try {
      if (running) {
        await api.post("/api/focus/stop").catch(() => {})
      } else {
        await api.post("/api/focus/start", taskId ? { task_id: taskId } : {}).catch(() => {})
      }
      await sync()
      // Tell the global timer to re-read immediately instead of waiting for its poll.
      window.dispatchEvent(new CustomEvent("todorio:focus-changed"))
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="row" style={{ margin: "10px 0", gap: 8 }}>
      <button className={"nav-btn row" + (running ? " active" : "")}
        style={{ gap: 5, display: "inline-flex" }} disabled={busy} onClick={toggle}>
        {running ? <IconPause size={13} /> : <IconPlay size={13} />}
        {running ? tr("focus.stop") : tr("focus.start")}
      </button>
    </div>
  )
}

// GlobalFocusTimer — the always-visible ticking clock in the sidebar (user ask: "focus mode
// should keep running and ticking somewhere in the interface").
//
// Elapsed time is derived from the server's started_at, so it stays correct across navigation,
// a page reload, and a different device. It polls slowly as a safety net and re-reads instantly
// on the custom event fired by FocusWidget.
export function GlobalFocusTimer() {
  const [state, setState] = useState<{ running: boolean; startedAt?: number; title?: string | null }>({ running: false })
  const [, force] = useState(0)

  const load = () =>
    api.get("/api/focus/current").then((r) => {
      setState(r.running
        ? { running: true, startedAt: new Date(r.started_at).getTime(), title: r.task_title }
        : { running: false })
    }).catch(() => {})

  useEffect(() => {
    load()
    const onChanged = () => load()
    window.addEventListener("todorio:focus-changed", onChanged)
    // A slow poll catches a session started in another tab or on another device.
    const poll = setInterval(load, 60000)
    return () => {
      window.removeEventListener("todorio:focus-changed", onChanged)
      clearInterval(poll)
    }
  }, [])

  // Re-render once a second only while a session is actually open.
  useEffect(() => {
    if (!state.running) return
    const id = setInterval(() => force((n) => n + 1), 1000)
    return () => clearInterval(id)
  }, [state.running])

  if (!state.running || !state.startedAt) return null

  const secs = Math.max(0, Math.floor((Date.now() - state.startedAt) / 1000))
  const h = Math.floor(secs / 3600)
  const m = Math.floor((secs % 3600) / 60).toString().padStart(2, "0")
  const ss = (secs % 60).toString().padStart(2, "0")
  const clock = h > 0 ? `${h}:${m}:${ss}` : `${m}:${ss}`

  async function stop() {
    await api.post("/api/focus/stop").catch(() => {})
    await load()
    window.dispatchEvent(new CustomEvent("todorio:focus-changed"))
  }

  return (
    <div className="focus-bar" title={state.title || tr("focus.start")}>
      <span className="focus-dot" />
      <span className="focus-clock">{clock}</span>
      {state.title && <span className="focus-task">{state.title}</span>}
      <button className="ctrl-btn" style={{ marginLeft: "auto" }} title={tr("focus.stop")} onClick={stop}>
        <IconPause size={13} />
      </button>
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

export function SearchPage({ onOpenTask, onOpenNote }: {
  // Opening a task/note needs the full record (search only returns id/title-ish fields), so the
  // fetch-by-id + modal rendering lives one level up in App.tsx, which already owns `me` and can
  // import TaskModal (views.tsx) and NoteModal (this file) without creating a circular import
  // between extras.tsx and views.tsx.
  onOpenTask: (taskId: number) => void
  onOpenNote: (noteId: number) => void
}) {
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

  function openResult(r: SearchResult) {
    if (r.type === "task") onOpenTask(r.id)
    else if (r.type === "note") onOpenNote(r.id)
    // Comments aren't opened on their own — they're shown inside their parent task's modal.
    else if (r.type === "comment") onOpenTask(r.task_id)
  }

  return (
    <div className="card">
      <h2>{tr("search.title")}</h2>
      <input className="input" placeholder={tr("search.placeholder")} value={q} onChange={(e) => run(e.target.value)} autoFocus />
      {busy && <p className="muted">{tr("search.searching")}</p>}
      {!busy && q.trim().length >= 2 && results.length === 0 && <p className="muted">{tr("search.empty")}</p>}
      {results.map((r, i) => (
        <div key={i} className="task-row" style={{ cursor: "pointer" }} onClick={() => openResult(r)}>
          {r.type === "task" && <span className="task-title row" style={{ gap: 6 }}><IconCheckCircle size={14} /> {r.title}</span>}
          {r.type === "note" && <span className="task-title row" style={{ gap: 6 }}><IconFileText size={14} /> {r.title}</span>}
          {r.type === "comment" && <span className="task-title row" style={{ gap: 6 }}><IconMessage size={14} /> «{r.task_title}» — {r.snippet}</span>}
        </div>
      ))}
    </div>
  )
}

// ---------- server settings (root only) ----------

// Root replaces the bundled logo with an uploaded image (spec section 18). Kept separate from
// the generic settings rows above because it's a file upload, not a key/value pair — the path
// itself is written server-side by the upload handler.
function LogoSettingRow() {
  // Cache-buster: the logo URL is fixed (/api/logo), so after replacing the file the browser
  // would otherwise keep showing the cached previous image.
  const [rev, setRev] = useState(0)
  const [hasLogo, setHasLogo] = useState<boolean | null>(null)
  const [busy, setBusy] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    api.get("/api/bootstrap").then((b) => setHasLogo(!!b.logo_path)).catch(() => setHasLogo(false))
  }, [rev])

  async function upload(f: File) {
    setBusy(true)
    try {
      const fd = new FormData()
      fd.append("file", f)
      const r = await fetch("/api/admin/logo", { method: "POST", body: fd, credentials: "same-origin" })
      if (!r.ok) {
        const e = await r.json().catch(() => null)
        alert(e?.error ?? trFormal("attach.failed"))
      }
      setRev((v) => v + 1)
    } finally {
      setBusy(false)
      if (fileRef.current) fileRef.current.value = ""
    }
  }

  async function remove() {
    await api.del("/api/admin/logo").catch(() => {})
    setRev((v) => v + 1)
  }

  return (
    <div className="row" style={{ margin: "8px 0", flexWrap: "wrap" }}>
      <label style={{ width: 260 }}>{trFormal("branding.logo")}</label>
      <div className="row" style={{ gap: 8 }}>
        <img src={hasLogo ? `/api/logo?v=${rev}` : "/icons/logo.svg"} alt=""
          style={{ width: 32, height: 32, objectFit: "contain" }}
          onError={(e) => { (e.currentTarget as HTMLImageElement).src = "/icons/logo.svg" }} />
        <label className="nav-btn row" style={{ display: "inline-flex", gap: 5, cursor: "pointer" }}>
          {busy ? trFormal("attach.uploading") : <><IconPaperclip size={14} /> {trFormal("branding.logo_upload")}</>}
          <input ref={fileRef} type="file" accept="image/*,.svg" style={{ display: "none" }}
            onChange={(e) => e.target.files?.[0] && upload(e.target.files[0])} />
        </label>
        {hasLogo && (
          <button className="nav-btn row" style={{ gap: 5, display: "inline-flex" }} onClick={remove}>
            <IconTrash size={13} /> {trFormal("branding.logo_remove")}
          </button>
        )}
      </div>
    </div>
  )
}

export function ServerSettingsCard({ me }: { me: Me }) {
  const [settings, setSettings] = useState<SettingDef[]>([])
  const [allLocales, setAllLocales] = useState<string[]>([])
  const [enabledLocales, setEnabledLocales] = useState<string[]>([])
  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)
  // Forces every secret input to remount (and so drop back to blank) after any save — an
  // uncontrolled input's defaultValue only applies once, so without this a just-typed token
  // would keep showing in the field indefinitely instead of going back to a placeholder.
  const [saveNonce, setSaveNonce] = useState(0)
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
      setSaveNonce((n) => n + 1)
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
      <LogoSettingRow />
      {settings.map((s) => (
        <div key={s.key} className="row" style={{ margin: "8px 0", flexWrap: "wrap" }}>
          <label style={{ width: 260 }}>{trFormal("server_setting." + s.key)}</label>
          {s.type === "select" && (
            <select className="input" style={{ width: "auto" }} defaultValue={s.value} onChange={(e) => save(s.key, e.target.value)}>
              {(s.options || []).map((o) => <option key={o} value={o}>{trFormal("server_setting.option." + o)}</option>)}
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
          {s.type === "secret" && (
            <div className="row" style={{ gap: 6, flexWrap: "wrap" }}>
              <input key={s.key + "-" + saveNonce} className="input" style={{ width: 260 }} type="password"
                placeholder={s.is_set ? trFormal("settings.secret_configured") : trFormal("settings.secret_not_configured")}
                defaultValue=""
                onBlur={(e) => { if (e.target.value) save(s.key, e.target.value) }} />
              {s.is_set && (
                <button className="nav-btn" type="button" onClick={() => save(s.key, "")}>{trFormal("attach.remove")}</button>
              )}
            </div>
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
  // Audience (spec section 16): all active users / specific roles / admins only.
  const [audienceMode, setAudienceMode] = useState<"all" | "roles" | "admins">("all")
  const [audienceRoles, setAudienceRoles] = useState<string[]>(["user"])
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
        audience: audienceMode === "roles"
          ? { mode: "roles", roles: audienceRoles }
          : { mode: audienceMode },
      })
      setName(""); setListName(""); setAutoApply(false)
      setAudienceMode("all"); setAudienceRoles(["user"])
      setTaskDrafts([blankTemplateTask()])
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
            {t.audience?.mode && t.audience.mode !== "all" && (
              <span className="muted">· {trFormal("templates.audience_" + t.audience.mode)}</span>
            )}
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
          <label className="row" style={{ gap: 4, fontSize: 13 }}>
            {trFormal("templates.audience")}
            <select className="input" style={{ width: "auto" }} value={audienceMode}
              onChange={(e) => setAudienceMode(e.target.value as "all" | "roles" | "admins")}>
              <option value="all">{trFormal("templates.audience_all")}</option>
              <option value="roles">{trFormal("templates.audience_roles")}</option>
              <option value="admins">{trFormal("templates.audience_admins")}</option>
            </select>
          </label>
          {audienceMode === "roles" && (
            <div className="row" style={{ gap: 10, fontSize: 13 }}>
              {["admin", "user", "viewer"].map((rl) => (
                <label key={rl} className="row" style={{ gap: 4 }}>
                  <input type="checkbox" checked={audienceRoles.includes(rl)}
                    onChange={(e) => setAudienceRoles((prev) =>
                      e.target.checked ? [...prev, rl] : prev.filter((x) => x !== rl))} />
                  {trFormal("admin.role." + rl)}
                </label>
              ))}
            </div>
          )}
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

// ---------- About page (spec section 18: "также на странице «О сайте» (версия, разработчик)") ----------

export function AboutPage({ siteName, version, developerUrl, aboutText,
  sourceUrl, donateUrl, onBack }: {
  siteName: string
  version?: string
  developerUrl?: string
  aboutText?: string
  sourceUrl?: string
  donateUrl?: string
  onBack: () => void
}) {
  // Only http(s) links are rendered. A branding field is root-editable text, so a
  // "javascript:" or "data:" URL typed into it must never become a clickable link.
  const safe = (u?: string) => (u && /^https?:\/\//i.test(u) ? u : "")
  const src = safe(sourceUrl)
  const donate = safe(donateUrl)
  const devLink = safe(developerUrl)
  return (
    <div>
      <div className="row" style={{ marginBottom: 12 }}>
        <button className="nav-btn row" style={{ gap: 4, display: "inline-flex" }} onClick={onBack}>
          <IconArrowLeft size={14} /> {tr("common.back")}
        </button>
        <h2 style={{ margin: 0 }}>{tr("about.title")}</h2>
      </div>
      <div className="card" style={{ maxWidth: 520 }}>
        <div style={{ fontSize: 18, fontWeight: 600, marginBottom: 10 }}>{siteName}</div>
        {/* Rendered as plain text, never as HTML — the same escaping rule the spec sets for
            the site name applies to anything root types into branding. */}
        {aboutText && <p style={{ marginTop: 0, color: "var(--text-muted)" }}>{aboutText}</p>}
        <div className="row" style={{ gap: 8, fontSize: 14, marginBottom: 6 }}>
          <span className="muted" style={{ minWidth: 110 }}>{tr("about.version")}</span>
          <span>{version || "—"}</span>
        </div>
        <div className="row" style={{ gap: 8, fontSize: 14, marginBottom: 6 }}>
          <span className="muted" style={{ minWidth: 110 }}>{tr("about.developer")}</span>
          <span>
            {devLink
              ? <a href={devLink} target="_blank" rel="noreferrer noopener">{DEVELOPER_NAME}</a>
              : DEVELOPER_NAME}
          </span>
        </div>
        {src && (
          <div className="row" style={{ gap: 8, fontSize: 14, marginBottom: 6 }}>
            <span className="muted" style={{ minWidth: 110 }}>{tr("about.source")}</span>
            <a href={src} target="_blank" rel="noreferrer noopener">{src.replace(/^https?:\/\//, "")}</a>
          </div>
        )}
        {donate && (
          <div className="row" style={{ gap: 8, fontSize: 14 }}>
            <span className="muted" style={{ minWidth: 110 }}>{tr("about.donate")}</span>
            <a href={donate} target="_blank" rel="noreferrer noopener">{donate.replace(/^https?:\/\//, "")}</a>
          </div>
        )}
      </div>
    </div>
  )
}

// ---------- modal shell ----------

// Modals render through a portal into <body>.
//
// Without it, any ancestor with backdrop-filter (which .card has in the "rich" visual mode)
// establishes a stacking context that confines the modal's z-index to that subtree — so
// content later in the DOM, e.g. the page footer, paints on top of the dialog. Portalling
// sidesteps that instead of escalating z-index values.
//
// Escape closes, and the backdrop click is only honoured when it lands on the backdrop
// itself so a drag that ends outside the panel doesn't dismiss the user's work.
export function ModalShell({ onClose, children, maxWidth }: {
  onClose: () => void
  children: React.ReactNode
  maxWidth?: number
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose() }
    window.addEventListener("keydown", onKey)
    // The page behind a dialog must not scroll while it's open.
    const prev = document.body.style.overflow
    document.body.style.overflow = "hidden"
    return () => {
      window.removeEventListener("keydown", onKey)
      document.body.style.overflow = prev
    }
  }, [onClose])

  return createPortal(
    <div className="modal-backdrop" role="dialog" aria-modal="true"
      onMouseDown={(e) => { if (e.target === e.currentTarget) onClose() }}>
      <div className="modal" style={maxWidth ? { maxWidth } : undefined}>
        {children}
      </div>
    </div>,
    document.body,
  )
}

// ---------- Inbox (spec section 12) ----------

// Cross-space triage list. Grouped by *why* each item is here — an undifferentiated pile of
// "things needing attention" is far harder to act on than four short, labelled groups.
const INBOX_REASONS = ["review", "mentioned", "assigned", "unassigned"] as const

export function InboxPage({ onOpenTask }: { onOpenTask?: (item: InboxItem) => void }) {
  const [data, setData] = useState<Inbox | null>(null)
  const [error, setError] = useState("")

  const load = () => {
    setError("")
    api.get("/api/inbox").then(setData).catch((e) => setError((e as Error).message))
  }
  useEffect(() => { load() }, [])

  if (error) return <div className="card"><p className="error-text">{error}</p></div>
  if (!data) return <div className="card">{tr("search.searching")}</div>

  const total = data.items.length

  return (
    <div>
      <h2 className="row" style={{ marginTop: 0, gap: 8 }}>
        <IconInbox size={20} /> {tr("inbox.title")}
        {total > 0 && <span className="badge">{total}</span>}
      </h2>

      {total === 0 && <div className="card"><p className="muted">{tr("inbox.empty")}</p></div>}

      {INBOX_REASONS.map((reason) => {
        const group = data.items.filter((i) => i.reason === reason)
        if (group.length === 0) return null
        return (
          <div key={reason} className="card" style={{ marginBottom: 12 }}>
            <div className="section-title" style={{ fontSize: 13, marginBottom: 8 }}>
              {tr("inbox.reason." + reason)} · {group.length}
            </div>
            {group.map((it) => (
              <div key={it.id} className="task-row" style={{ cursor: onOpenTask ? "pointer" : "default" }}
                onClick={() => onOpenTask?.(it)}>
                <span className="task-title">{it.title}</span>
                <span className="muted" style={{ fontSize: 12 }}>
                  {it.space_name} · {it.list_name}
                </span>
                {it.due_at && (
                  <span className="muted" style={{ fontSize: 12 }}>
                    {new Date(it.due_at).toLocaleDateString(getFormattingLocale())}
                  </span>
                )}
              </div>
            ))}
          </div>
        )
      })}
    </div>
  )
}

// ---------- confirmation dialog (spec section 10) ----------

// A real dialog instead of window.confirm, for two reasons: the native prompt can't be styled or
// localised consistently, and for genuinely irreversible actions a single OK click is too cheap.
//
// `requireText` turns it into a type-to-confirm: the button stays disabled until the user types
// the exact name. Reserved for actions that destroy data with no undo (permanent delete, deleting
// a space) — using it everywhere would train people to copy-paste past it without reading.
export function ConfirmDialog({ title, body, confirmLabel, requireText, danger, onConfirm, onCancel }: {
  title: string
  body?: string
  confirmLabel: string
  requireText?: string
  danger?: boolean
  onConfirm: () => void
  onCancel: () => void
}) {
  const [typed, setTyped] = useState("")
  const ready = !requireText || typed.trim() === requireText.trim()

  return (
    <ModalShell onClose={onCancel} maxWidth={440}>
      <h3 className="row" style={{ marginTop: 0, gap: 6 }}>
        {danger && <IconAlertTriangle size={17} style={{ color: "var(--due-overdue)" }} />}
        {title}
      </h3>
      {body && <p className="muted" style={{ marginTop: 0 }}>{body}</p>}
      {requireText && (
        <>
          <p style={{ fontSize: 13, marginBottom: 6 }}>
            {tr("confirm.type_to_proceed").replace("{name}", requireText)}
          </p>
          <input className="input" value={typed} autoFocus
            onChange={(e) => setTyped(e.target.value)}
            onKeyDown={(e) => { if (e.key === "Enter" && ready) onConfirm() }} />
        </>
      )}
      <div className="row" style={{ marginTop: 14, justifyContent: "flex-end", gap: 8 }}>
        <button className="nav-btn" onClick={onCancel}>{tr("confirm.cancel")}</button>
        <button className="btn" disabled={!ready}
          style={danger ? { background: "var(--due-overdue)", opacity: ready ? 1 : 0.5 } : { opacity: ready ? 1 : 0.5 }}
          onClick={onConfirm}>
          {confirmLabel}
        </button>
      </div>
    </ModalShell>
  )
}

// useConfirm gives any component a single piece of state plus a render slot, so adding a guarded
// action doesn't mean adding three useStates each time.
export function useConfirm() {
  const [pending, setPending] = useState<{
    title: string; body?: string; confirmLabel: string
    requireText?: string; danger?: boolean; action: () => void | Promise<void>
  } | null>(null)

  const element = pending ? (
    <ConfirmDialog
      title={pending.title} body={pending.body} confirmLabel={pending.confirmLabel}
      requireText={pending.requireText} danger={pending.danger}
      onCancel={() => setPending(null)}
      onConfirm={async () => { const a = pending.action; setPending(null); await a() }}
    />
  ) : null

  return { confirm: setPending, confirmElement: element }
}
