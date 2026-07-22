// Additional Todorio blocks: announcements, digest, statistics, attachments, TOTP.
import { useEffect, useRef, useState } from "react"
import { api, type Me } from "./api"
import { tr } from "./i18n"

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
          <span>{a.level === "emergency" ? "🚨" : a.level === "important" ? "⚠️" : "📢"}</span>
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
  const rows: Array<[string, number]> = [
    ["📌 " + tr("digest.assigned"), d.summary.assigned_to_me],
    ["💬 " + tr("digest.comments"), d.summary.new_comments],
    ["✅ " + tr("digest.done"), d.summary.done_nearby],
    ["📢 " + tr("digest.announcements"), d.summary.announcements],
  ]
  return (
    <div className="card" style={{ padding: 16, marginBottom: 12, borderLeft: "4px solid var(--accent)" }}>
      <div className="row" style={{ justifyContent: "space-between" }}>
        <b>👋 {tr("digest.title")} ({d.since ? new Date(d.since).toLocaleString() : ""})</b>
        <button className="nav-btn" onClick={close}>{tr("digest.ok")}</button>
      </div>
      <div style={{ marginTop: 8, display: "flex", gap: 16, flexWrap: "wrap" }}>
        {rows.filter(([, n]) => n > 0).map(([label, n]) => (
          <span key={label}>{label}: <b>{n}</b></span>
        ))}
      </div>
    </div>
  )
}

// ---------- space statistics ----------

type StatsMember = { id: number; username: string; name: string; done: number; done_weight: number; overdue: number }
type Stats = {
  period: string
  members: StatsMember[]
  caption: { part1: string; part2: string; category: string }
  best?: StatsMember
}

export function StatsCard({ spaceId }: { spaceId: number }) {
  const [stats, setStats] = useState<Stats | null>(null)
  const [period, setPeriod] = useState<"week" | "month">("week")
  useEffect(() => {
    api.get(`/api/spaces/${spaceId}/stats?period=${period}`).then(setStats).catch(() => {})
  }, [spaceId, period])
  if (!stats || !stats.members.length) return null
  const max = Math.max(...stats.members.map((m) => m.done_weight), 1)
  return (
    <div className="card" style={{ padding: 14, marginBottom: 12 }}>
      <div className="row" style={{ justifyContent: "space-between" }}>
        <b>📊 {tr("stats.title")}</b>
        <span>
          <button className={"nav-btn" + (period === "week" ? " active" : "")} onClick={() => setPeriod("week")}>{tr("stats.week")}</button>
          <button className={"nav-btn" + (period === "month" ? " active" : "")} onClick={() => setPeriod("month")}>{tr("stats.month")}</button>
        </span>
      </div>
      {(stats.caption.part1 || stats.caption.part2) && (
        <div className="muted" style={{ margin: "6px 0 10px" }}>
          {stats.caption.part1} {stats.caption.part2}
        </div>
      )}
      {stats.best && (
        <div style={{ marginBottom: 10 }}>👑 {tr("stats.best")}: <b>@{stats.best.username}</b> · ✅ {stats.best.done}</div>
      )}
      {stats.members.map((m) => (
        <div key={m.id} style={{ marginBottom: 6 }}>
          <div className="row" style={{ justifyContent: "space-between", fontSize: 13 }}>
            <span>@{m.username}</span>
            <span className="muted">✅ {m.done}{m.overdue > 0 ? ` · ⏰ ${m.overdue}` : ""}</span>
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
      <label className="nav-btn" style={{ display: "inline-block", marginTop: 6, cursor: "pointer" }}>
        {busy ? tr("attach.uploading") : "📎 " + tr("attach.add")}
        <input ref={fileRef} type="file" accept="image/*" style={{ display: "none" }}
          onChange={(e) => e.target.files?.[0] && upload(e.target.files[0])} />
      </label>
    </div>
  )
}

// ---------- TOTP (two-factor auth for root/admins) ----------

export function TotpCard({ me }: { me: Me }) {
  const [setup, setSetup] = useState<{ secret: string; otpauth: string } | null>(null)
  const [code, setCode] = useState("")
  const [msg, setMsg] = useState("")
  if (me.role !== "root" && me.role !== "admin") return null

  const start = async () => {
    try { setSetup(await api.post("/api/me/totp/setup")); setMsg("") } catch (e: any) { setMsg(e.message) }
  }
  const enable = async () => {
    try {
      await api.post("/api/me/totp/enable", { code })
      setSetup(null); setCode(""); setMsg("✅ " + tr("totp.enabled"))
    } catch (e: any) { setMsg("❌ " + e.message) }
  }
  const disable = async () => {
    const c = prompt(tr("totp.disable_prompt"))
    if (!c) return
    try { await api.post("/api/me/totp/disable", { code: c }); setMsg(tr("totp.disabled")) } catch (e: any) { setMsg("❌ " + e.message) }
  }

  return (
    <div className="card" style={{ padding: 14, marginTop: 12 }}>
      <b>🔐 {tr("totp.title")}</b>
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
      {msg && <div style={{ marginTop: 6 }}>{msg}</div>}
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
      <h3 style={{ marginTop: 0 }}>🎟 {tr("invites.title")}</h3>
      <p style={{ opacity: 0.7, fontSize: 13 }}>
        {tr("invites.hint")} <code>todorio server policy set users.can_invite true</code>
      </p>
      <button className="btn" onClick={create}>{tr("invites.create")}</button>
      {lastCode && (
        <p>
          {tr("invites.new_code")} <code>{lastCode}</code>
        </p>
      )}
      {invites.map((i) => (
        <div key={i.id} className="row" style={{ justifyContent: "space-between", padding: "4px 0" }}>
          <span>
            <code>{i.code}</code> · {i.role} · {i.used_count}/{i.max_uses} · {tr("invites.by")} {i.created_by}
          </span>
          <button className="nav-btn" onClick={() => remove(i.id)}>{tr("invites.delete")}</button>
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
              <span>{t.completed_at ? "✅" : "⬜"}</span>
              <span style={{ textDecoration: t.completed_at ? "line-through" : "none" }}>{t.title}</span>
              <span style={{ marginLeft: "auto", opacity: 0.6, fontSize: 12 }}>
                {t.due_at ? new Date(t.due_at).toLocaleDateString() : ""}
              </span>
            </div>
          ))}
        {data && data.tasks.length === 0 && <p>{tr("public.empty")}</p>}
      </div>
    </div>
  )
}
