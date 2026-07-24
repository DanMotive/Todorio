// Todorio screens: login, "My tasks", spaces, tasks, notifications, admin panel.
import { useEffect, useState } from "react"
import {
  api, REACTIONS, DEFAULT_STATUSES,
  type List, type Me, type Pulse, type Space, type Task, type Workflow,
} from "./api"
import { AttachmentsBlock, StatsCard, FocusWidget, NotesPanel, ActivityPanel, ArchivePanel, ArchivedSpacesPanel, FieldsPanel, type FieldDef } from "./extras"
import { tr, trFormal, setLocale, getLocale, getFormattingLocale, SUPPORTED } from "./i18n"
import {
  IconStar, IconRefresh, IconLock, IconX, IconUser, IconPause, IconSlash, IconClock,
  IconGrid, IconArrowLeft, IconList, IconFileText, IconActivity, IconMenu, IconColumns,
  IconTable, IconCheckCircle, IconMessage, IconPin, IconAlertCircle, IconArchive, IconCalendar,
  IconSliders,
} from "./icons"

// ---------- helpers ----------

function dueClass(due: string | null): string {
  if (!due) return ""
  const d = new Date(due).getTime() - Date.now()
  if (d < 0) return "overdue"
  if (d < 24 * 3600e3) return "today"
  if (d < 3 * 24 * 3600e3) return "soon"
  return "later"
}

function dueLabel(due: string | null): string {
  if (!due) return ""
  return new Date(due).toLocaleDateString(getFormattingLocale(), { day: "numeric", month: "short" })
}

// ---------- login / registration ----------

export function AuthPage({ siteName, locales, onLogin }: { siteName: string; locales?: string[]; onLogin: (me: Me) => void }) {
  const langOptions = locales && locales.length > 0 ? locales : SUPPORTED
  const [mode, setMode] = useState<"login" | "register">("login")
  const [username, setUsername] = useState("")
  const [password, setPassword] = useState("")
  const [error, setError] = useState("")
  const [info, setInfo] = useState("")
  const [totp, setTotp] = useState("")
  const [invite, setInvite] = useState("")
  const [locale, setLocaleState] = useState(getLocale())

  function changeLocale(l: string) {
    setLocale(l)
    setLocaleState(l)
  }

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setError("")
    setInfo("")
    try {
      if (mode === "login") {
        const me = await api.post("/api/login", { username, password, totp_code: totp })
        onLogin(me as Me)
      } else {
        const res = await api.post("/api/register", { username, password, invite_code: invite })
        if (res.status === "pending") {
          setInfo(tr("auth.request_sent"))
          setMode("login")
        } else {
          const me = await api.post("/api/login", { username, password })
          onLogin(me as Me)
        }
      }
    } catch (err) {
      setError((err as Error).message)
    }
  }

  return (
    <div className="center-page"> 
      <form className="card auth-card" onSubmit={submit}>
        <select className="input lang-select" value={locale} aria-label={tr("auth.language")}
          onChange={(e) => changeLocale(e.target.value)}>
          {langOptions.map((l) => <option key={l} value={l}>{l}</option>)}
        </select>
        <div className="row" style={{ marginTop: 22 }}>
          <img src="/icons/logo.svg" alt="" width={40} height={40} />
          <h1 style={{ margin: 0, fontSize: 22 }}>{siteName}</h1>
        </div>
        <input className="input" placeholder={tr("auth.username")} value={username}
          onChange={(e) => setUsername(e.target.value)} autoFocus />
        <input className="input" type="password" placeholder={tr("auth.password")} value={password}
          onChange={(e) => setPassword(e.target.value)} />
        {mode === "login" && (
          <input className="input" placeholder={tr("auth.totp")} value={totp}
            onChange={(e) => setTotp(e.target.value)} />
        )}
        {mode === "register" && (
          <input className="input" placeholder={tr("auth.invite")} value={invite}
            onChange={(e) => setInvite(e.target.value)} />
        )}
        <div className="error-text">{error || info}</div>
        <button className="btn" type="submit">
          {mode === "login" ? tr("auth.sign_in") : tr("auth.sign_up")}
        </button>
        <button className="nav-btn" type="button" onClick={() => setMode(mode === "login" ? "register" : "login")}>
          {mode === "login" ? tr("auth.no_account") : tr("auth.have_account")}
        </button>
      </form>
    </div>
  )
}

export function PendingPage({ onLogout }: { onLogout: () => void }) {
  return (
    <div className="center-page">
      <div className="card auth-card" style={{ textAlign: "center" }}>
        <div style={{ display: "flex", justifyContent: "center", color: "var(--text-muted)" }}><IconClock size={40} /></div>
        <h2>{tr("pending.title")}</h2>
        <p className="muted">{tr("pending.text")}</p>
        <button className="btn" onClick={onLogout}>{tr("nav.logout")}</button>
      </div>
    </div>
  )
}

// ---------- tasks ----------

function TaskRow({ task, onToggle, onOpen, favorite, onToggleFavorite }: {
  task: Task; onToggle: (t: Task) => void; onOpen: (t: Task) => void
  favorite?: boolean; onToggleFavorite?: (t: Task) => void
}) {
  const done = !!task.completed_at
  return (
    <div className={"task-row" + (done ? " done" : "")} onClick={() => onOpen(task)}>
      <input type="checkbox" checked={done} onClick={(e) => e.stopPropagation()} onChange={() => onToggle(task)} />
      <span className="task-title">{task.title}</span>
      {task.subtasks_total > 0 && (
        <span className="muted">{task.subtasks_done}/{task.subtasks_total}</span>
      )}
      {task.due_at && <span className={"due " + dueClass(task.due_at)}>{dueLabel(task.due_at)}</span>}
      {onToggleFavorite && (
        <button className="nav-btn" style={{ padding: "2px 6px", color: favorite ? "var(--due-soon)" : undefined }} title={tr("favorites.toggle")}
          onClick={(e) => { e.stopPropagation(); onToggleFavorite(task) }}>
          <IconStar size={14} filled={favorite} />
        </button>
      )}
    </div>
  )
}

export function TaskModal({ task, me, spaceId, onClose, onChanged }: {
  task: Task; me: Me; spaceId?: number; onClose: () => void; onChanged: () => void
}) {
  const [comments, setComments] = useState<any[]>([])
  const [body, setBody] = useState("")
  const [error, setError] = useState("")
  const [editingId, setEditingId] = useState<number | null>(null)
  const [editBody, setEditBody] = useState("")
  const [statuses, setStatuses] = useState<string[]>(DEFAULT_STATUSES)

  // Editable task states
  const [status, setStatus] = useState(task.status || "open")
  const [priority, setPriority] = useState(task.priority || "normal")
  const [freq, setFreq] = useState(task.recurrence?.freq || "none")
  const [blockedByInput, setBlockedByInput] = useState("")
  const [blockedBy, setBlockedBy] = useState<number[]>(task.blocked_by || [])
  const [customFields, setCustomFields] = useState<Record<string, string>>(task.custom_fields || {})
  const [newKey, setNewKey] = useState("")
  const [newValue, setNewValue] = useState("")
  const [fieldSchema, setFieldSchema] = useState<FieldDef[]>([])

  const load = () => api.get(`/api/tasks/${task.id}/comments`).then((r) => setComments(r.comments)).catch(() => {})
  useEffect(() => { load() }, [task.id])
  useEffect(() => {
    if (!spaceId) { setFieldSchema([]); return }
    api.get(`/api/spaces/${spaceId}/workflow`).then((r: Workflow) => setStatuses(r.statuses)).catch(() => {})
    // Typed field definitions the space owner configured (spec section 13) — when present, these
    // drive proper controls below instead of the plain freeform key/value editor. Falls back to
    // freeform when spaceId is unknown (e.g. opened from "My tasks", which spans many spaces).
    api.get(`/api/spaces/${spaceId}/fields`).then((r) => setFieldSchema(r.fields || [])).catch(() => {})
  }, [spaceId])

  async function setCustomField(k: string, v: string) {
    const next = { ...customFields, [k]: v }
    setCustomFields(next)
    await updateTask({ custom_fields: next })
  }

  function renderFieldControl(f: FieldDef) {
    const val = customFields[f.key] ?? ""
    switch (f.type) {
      case "checkbox":
        return <input type="checkbox" checked={val === "true"} onChange={(e) => setCustomField(f.key, e.target.checked ? "true" : "false")} />
      case "number":
        return <input className="input" type="number" value={val} onChange={(e) => setCustomField(f.key, e.target.value)} />
      case "date":
        return <input className="input" type="date" value={val} onChange={(e) => setCustomField(f.key, e.target.value)} />
      case "link":
        return <input className="input" type="url" placeholder="https://" value={val} onChange={(e) => setCustomField(f.key, e.target.value)} />
      case "rating":
        return (
          <select className="input" style={{ width: "auto" }} value={val} onChange={(e) => setCustomField(f.key, e.target.value)}>
            <option value="">—</option>
            {[1, 2, 3, 4, 5].map((n) => <option key={n} value={n}>{n}</option>)}
          </select>
        )
      case "select":
        return (
          <select className="input" style={{ width: "auto" }} value={val} onChange={(e) => setCustomField(f.key, e.target.value)}>
            <option value="">—</option>
            {(f.options || []).map((o) => <option key={o} value={o}>{o}</option>)}
          </select>
        )
      case "multiselect": {
        const selected = val ? val.split(",") : []
        return (
          <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
            {(f.options || []).map((o) => (
              <label key={o} className="row" style={{ gap: 3, fontSize: 13 }}>
                <input type="checkbox" checked={selected.includes(o)} onChange={(e) => {
                  const nextSel = e.target.checked ? [...selected, o] : selected.filter((s) => s !== o)
                  setCustomField(f.key, nextSel.join(","))
                }} /> {o}
              </label>
            ))}
          </div>
        )
      }
      case "user":
        return <input className="input" placeholder={tr("fields.user_placeholder")} value={val} onChange={(e) => setCustomField(f.key, e.target.value)} />
      default:
        return <input className="input" value={val} onChange={(e) => setCustomField(f.key, e.target.value)} />
    }
  }

  async function updateTask(patch: any) {
    try {
      await api.patch(`/api/tasks/${task.id}`, patch)
      onChanged()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  async function handleStatusChange(s: string) {
    setStatus(s)
    await updateTask({ status: s })
  }

  async function handlePriorityChange(p: string) {
    setPriority(p)
    await updateTask({ priority: p })
  }

  async function handleFreqChange(f: string) {
    setFreq(f)
    if (f === "none") {
      await updateTask({ clear_recurrence: true })
    } else {
      await updateTask({ recurrence: { freq: f, interval: 1 } })
    }
  }

  async function addBlockedBy() {
    const id = parseInt(blockedByInput.trim(), 10)
    if (!id || isNaN(id) || blockedBy.includes(id)) return
    const next = [...blockedBy, id]
    setBlockedBy(next)
    setBlockedByInput("")
    await updateTask({ blocked_by: next })
  }

  async function removeBlockedBy(id: number) {
    const next = blockedBy.filter((b) => b !== id)
    setBlockedBy(next)
    await updateTask({ blocked_by: next })
  }

  async function addCustomField() {
    if (!newKey.trim()) return
    const next = { ...customFields, [newKey.trim()]: newValue.trim() }
    setCustomFields(next)
    setNewKey("")
    setNewValue("")
    await updateTask({ custom_fields: next })
  }

  async function removeCustomField(key: string) {
    const next = { ...customFields }
    delete next[key]
    setCustomFields(next)
    await updateTask({ custom_fields: next })
  }

  async function send(e: React.FormEvent) {
    e.preventDefault()
    if (!body.trim()) return
    try {
      await api.post(`/api/tasks/${task.id}/comments`, { body })
      setBody("")
      load()
    } catch (err) { setError((err as Error).message) }
  }

  function startEdit(c: any) { setEditingId(c.id); setEditBody(c.body) }
  function cancelEdit() { setEditingId(null); setEditBody("") }
  async function saveEdit(id: number) {
    if (!editBody.trim()) return
    try {
      await api.patch(`/api/comments/${id}`, { body: editBody })
      setEditingId(null); setEditBody("")
      load()
    } catch (err) { setError((err as Error).message) }
  }

  async function react(commentId: number, emoji: string) {
    await api.post("/api/reactions", { target_type: "comment", target_id: commentId, emoji }).catch(() => {})
    load()
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()} style={{ maxWidth: 640 }}>
        <div className="row">
          <h2 className="grow" style={{ margin: 0, fontSize: 20 }}>{task.title}</h2>
          <button className="nav-btn" onClick={onClose}>
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>

        {task.description && <p style={{ color: "var(--text-muted)", margin: "8px 0 16px" }}>{task.description}</p>}

        {/* Task Properties grid */}
        <div className="card" style={{ padding: 12, marginBottom: 16, display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
          <div>
            <label className="muted" style={{ display: "block", fontSize: 12, marginBottom: 4 }}>{tr("task.status")}</label>
            <select className="input" value={status} onChange={(e) => handleStatusChange(e.target.value)}>
              {statuses.map((s) => (
                <option key={s} value={s}>{DEFAULT_STATUSES.includes(s) ? tr("task.status." + s) : s}</option>
              ))}
            </select>
          </div>

          <div>
            <label className="muted" style={{ display: "block", fontSize: 12, marginBottom: 4 }}>{tr("task.priority")}</label>
            <select className="input" value={priority} onChange={(e) => handlePriorityChange(e.target.value)}>
              <option value="low">{tr("task.priority.low")}</option>
              <option value="normal">{tr("task.priority.normal")}</option>
              <option value="high">{tr("task.priority.high")}</option>
              <option value="urgent">{tr("task.priority.urgent")}</option>
            </select>
          </div>

          <div>
            <label className="muted row" style={{ fontSize: 12, marginBottom: 4, gap: 4 }}>
              <IconRefresh size={12} /> {tr("task.recurrence")}
            </label>
            <select className="input" value={freq} onChange={(e) => handleFreqChange(e.target.value)}>
              <option value="none">{tr("task.recurrence.none")}</option>
              <option value="daily">{tr("task.recurrence.daily")}</option>
              <option value="weekly">{tr("task.recurrence.weekly")}</option>
              <option value="monthly">{tr("task.recurrence.monthly")}</option>
            </select>
          </div>

          <div>
            <label className="muted row" style={{ fontSize: 12, marginBottom: 4, gap: 4 }}>
              <IconLock size={12} /> {tr("task.blocked_by")}
            </label>
            <div className="row">
              <input className="input grow" placeholder={tr("task.blocked_by_placeholder")} value={blockedByInput} onChange={(e) => setBlockedByInput(e.target.value)} />
              <button className="btn secondary" style={{ padding: "6px 10px" }} onClick={addBlockedBy}>+</button>
            </div>
            {blockedBy.length > 0 && (
              <div className="row" style={{ marginTop: 6, flexWrap: "wrap", gap: 4 }}>
                {blockedBy.map((id) => (
                  <span key={id} className="badge row" style={{ cursor: "pointer", gap: 3, display: "inline-flex" }} onClick={() => removeBlockedBy(id)} title={tr("common.click_to_remove")}>
                    #{id} <IconX size={10} />
                  </span>
                ))}
              </div>
            )}
          </div>
        </div>

        {/* Custom Fields section: typed controls for the space's defined schema (if any), plus a
            freeform key/value editor underneath for anything outside that schema. */}
        <div style={{ marginBottom: 16 }}>
          <div className="section-title" style={{ fontSize: 13, marginBottom: 6 }}>{tr("task.custom_fields")}</div>
          {fieldSchema.map((f) => (
            <div key={f.key} className="row" style={{ marginBottom: 6, fontSize: 13, gap: 8 }}>
              <label style={{ minWidth: 100 }}>{f.label}</label>
              {renderFieldControl(f)}
            </div>
          ))}
          {Object.entries(customFields).filter(([k]) => !fieldSchema.some((f) => f.key === k)).map(([k, v]) => (
            <div key={k} className="row" style={{ marginBottom: 4, fontSize: 13 }}>
              <b>{k}:</b> <span>{v}</span>
              <button className="nav-btn" style={{ padding: "2px 6px" }} onClick={() => removeCustomField(k)}><IconX size={11} /></button>
            </div>
          ))}
          <div className="row" style={{ marginTop: 6 }}>
            <input className="input" style={{ width: 120 }} placeholder={tr("task.field_key")} value={newKey} onChange={(e) => setNewKey(e.target.value)} />
            <input className="input grow" placeholder={tr("task.field_value")} value={newValue} onChange={(e) => setNewValue(e.target.value)} />
            <button className="btn secondary" onClick={addCustomField}>+ {tr("task.add_field")}</button>
          </div>
        </div>

        <FocusWidget taskId={task.id} />

        <div className="section-title">{tr("task.comments")}</div>
        <AttachmentsBlock taskId={task.id} />
        {comments.map((c) => (
          <div key={c.id} className="card" style={{ marginBottom: 8, padding: 12 }}>
            <div className="row">
              <b>@{c.author}</b>
              <span className="muted">
                {new Date(c.created_at).toLocaleString(getFormattingLocale())}
                {c.edited_at ? ` · ${tr("task.comment_edited")}` : ""}
              </span>
              {c.author_id === me.id && editingId !== c.id && (
                <button className="nav-btn" style={{ marginLeft: "auto", fontSize: 12 }} onClick={() => startEdit(c)}>
                  {tr("task.comment_edit")}
                </button>
              )}
            </div>
            {editingId === c.id ? (
              <div>
                <textarea className="input" style={{ width: "100%", minHeight: 60, marginTop: 4 }}
                  value={editBody} onChange={(e) => setEditBody(e.target.value)} />
                <div className="row" style={{ marginTop: 6, gap: 6 }}>
                  <button className="btn" onClick={() => saveEdit(c.id)}>{tr("task.comment_save")}</button>
                  <button className="nav-btn" onClick={cancelEdit}>{tr("task.comment_cancel")}</button>
                </div>
              </div>
            ) : (
              <div>{c.body}</div>
            )}
            <div className="row" style={{ marginTop: 6, flexWrap: "wrap" }}>
              {REACTIONS.map((emoji) => {
                const rx = (c.reactions as any[]).filter((r) => r.emoji === emoji)
                const mine = rx.some((r) => r.user_id === me.id)
                if (rx.length === 0 && !mine) {
                  return (
                    <button key={emoji} className="reaction" style={{ opacity: 0.4 }}
                      onClick={() => react(c.id, emoji)}>{emoji}</button>
                  )
                }
                return (
                  <button key={emoji} className={"reaction" + (mine ? " mine" : "")}
                    onClick={() => react(c.id, emoji)}>{emoji} {rx.length}</button>
                )
              })}
            </div>
          </div>
        ))}
        <form className="row" onSubmit={send}>
          <input className="input grow" placeholder={tr("task.comment_placeholder")} value={body}
            onChange={(e) => setBody(e.target.value)} />
          <button className="btn" type="submit">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/></svg>
          </button>
        </form>
        <div className="error-text">{error}</div>

        <div className="row" style={{ marginTop: 16, justifyContent: "space-between" }}>
          <button className="nav-btn" style={{ color: "var(--due-overdue)" }} onClick={async () => { await api.del(`/api/tasks/${task.id}`); onChanged(); onClose() }}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ marginRight: 6 }}><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
            {tr("task.archive")}
          </button>
        </div>
      </div>
    </div>
  )
}

// ---------- "My tasks" ----------

function MyWeekView({ tasks, favorites, onOpen, onToggle, onToggleFavorite }: {
  tasks: Task[]; favorites: Set<number>; onOpen: (t: Task) => void; onToggle: (t: Task) => void
  onToggleFavorite: (t: Task) => void
}) {
  const now = new Date()
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate())
  const days: { date: Date; tasks: Task[] }[] = []
  for (let i = 0; i < 7; i++) {
    const d = new Date(startOfToday)
    d.setDate(d.getDate() + i)
    days.push({ date: d, tasks: [] })
  }
  const overdue: Task[] = []
  const later: Task[] = []
  for (const t of tasks) {
    if (!t.due_at) { later.push(t); continue }
    const d = new Date(t.due_at)
    const dayStart = new Date(d.getFullYear(), d.getMonth(), d.getDate())
    const diff = Math.round((dayStart.getTime() - startOfToday.getTime()) / 86400000)
    if (diff < 0) overdue.push(t)
    else if (diff < 7) days[diff].tasks.push(t)
    else later.push(t)
  }
  const total = overdue.length + later.length + days.reduce((n, d) => n + d.tasks.length, 0)
  const Section = ({ label, list }: { label: React.ReactNode; list: Task[] }) => list.length === 0 ? null : (
    <div style={{ marginBottom: 14 }}>
      <div className="section-title row" style={{ margin: "0 0 6px", fontSize: 13, gap: 5 }}>{label}</div>
      {list.map((t) => (
        <TaskRow key={t.id} task={t} onToggle={onToggle} onOpen={onOpen}
          favorite={favorites.has(t.id)} onToggleFavorite={onToggleFavorite} />
      ))}
    </div>
  )
  return (
    <div>
      <Section label={<><IconAlertCircle size={13} style={{ color: "var(--due-overdue)" }} /> {tr("my_week.overdue")}</>} list={overdue} />
      {days.map((d, i) => (
        <Section key={i} list={d.tasks}
          label={d.date.toLocaleDateString(getFormattingLocale(), { weekday: "long", day: "numeric", month: "short" })} />
      ))}
      <Section label={tr("my_week.later")} list={later} />
      {total === 0 && <p className="muted">{tr("my.empty")}</p>}
    </div>
  )
}

type MySubTab = "all" | "today" | "overdue" | "review" | "no_deadline" | "mentions"

export function MyTasksPage({ me }: { me: Me }) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [open, setOpen] = useState<Task | null>(null)
  const [tab, setTab] = useState<"list" | "week">("list")
  const [subTab, setSubTab] = useState<MySubTab>("all")
  const [mentions, setMentions] = useState<any[]>([])
  const [favorites, setFavorites] = useState<Set<number>>(new Set())
  const load = () => api.get("/api/my/tasks").then((r) => setTasks(r.tasks)).catch(() => {})
  const loadFavorites = () => api.get("/api/favorites").then((r) => {
    setFavorites(new Set(
      (r.favorites as Array<{ target_type: string; target_id: number }>)
        .filter((f) => f.target_type === "task").map((f) => f.target_id),
    ))
  }).catch(() => {})
  useEffect(() => { load(); loadFavorites() }, [])
  useEffect(() => {
    if (subTab !== "mentions") return
    api.get(`/api/search?q=${encodeURIComponent("@" + me.username)}`)
      .then((r) => setMentions((r.results as any[]).filter((x) => x.type === "comment")))
      .catch(() => {})
  }, [subTab, me.username])

  async function toggle(task: Task) {
    await api.patch(`/api/tasks/${task.id}`, { status: task.completed_at ? "open" : "done" }).catch(() => {})
    load()
  }

  async function toggleFavorite(task: Task) {
    await api.post("/api/favorites", { target_type: "task", target_id: task.id }).catch(() => {})
    loadFavorites()
  }

  async function openMentionedTask(taskID: number) {
    const r = await api.get(`/api/tasks/${taskID}`).catch(() => null)
    if (r?.task) setOpen(r.task)
  }

  const subFiltered = tasks.filter((t) => {
    switch (subTab) {
      case "today": return !t.completed_at && dueClass(t.due_at) === "today"
      case "overdue": return !t.completed_at && dueClass(t.due_at) === "overdue"
      case "review": return !t.completed_at && t.status === "review"
      case "no_deadline": return !t.completed_at && !t.due_at
      default: return true
    }
  })

  return (
    <div className="card">
      <div className="row" style={{ marginBottom: 8 }}>
        <h2 className="grow" style={{ margin: 0 }}>{tr("my.title")}</h2>
        <div className="row" style={{ gap: 4 }}>
          <button className={"nav-btn" + (tab === "list" ? " active" : "")} onClick={() => setTab("list")}>{tr("view.list")}</button>
          <button className={"nav-btn" + (tab === "week" ? " active" : "")} onClick={() => setTab("week")}>{tr("view.my_week")}</button>
        </div>
      </div>
      {tab === "list" && (
        <>
          <div className="row" style={{ gap: 4, marginBottom: 10, flexWrap: "wrap" }}>
            {(["all", "today", "overdue", "review", "no_deadline", "mentions"] as MySubTab[]).map((s) => (
              <button key={s} className={"nav-btn" + (subTab === s ? " active" : "")} onClick={() => setSubTab(s)}>
                {tr("my.sub." + s)}
              </button>
            ))}
          </div>
          {subTab === "mentions" ? (
            <>
              {mentions.length === 0 && <p className="muted">{tr("my.empty")}</p>}
              {mentions.map((m) => (
                <div key={m.id} className="task-row" onClick={() => openMentionedTask(m.task_id)}>
                  <span className="task-title row" style={{ gap: 6 }}><IconMessage size={14} /> «{m.task_title}»</span>
                  <span className="muted" style={{ maxWidth: 320, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>{m.snippet}</span>
                </div>
              ))}
            </>
          ) : (
            <>
              {subFiltered.length === 0 && <p className="muted">{tr("my.empty")}</p>}
              {subFiltered.map((task) => (
                <TaskRow key={task.id} task={task} onToggle={toggle} onOpen={setOpen}
                  favorite={favorites.has(task.id)} onToggleFavorite={toggleFavorite} />
              ))}
            </>
          )}
        </>
      )}
      {tab === "week" && (
        <MyWeekView tasks={tasks} favorites={favorites} onOpen={setOpen} onToggle={toggle} onToggleFavorite={toggleFavorite} />
      )}
      {open && <TaskModal task={open} me={me} onClose={() => setOpen(null)} onChanged={load} />}
    </div>
  )
}

// ---------- spaces ----------

export function SpacesPage({ me }: { me: Me }) {
  const [spaces, setSpaces] = useState<Space[]>([])
  const [current, setCurrent] = useState<Space | null>(null)
  const [name, setName] = useState("")
  const [showArchived, setShowArchived] = useState(false)
  const load = () => api.get("/api/spaces").then((r) => setSpaces(r.spaces)).catch(() => {})
  useEffect(() => { load() }, [])

  if (current) return <SpaceView me={me} space={current} onBack={() => { setCurrent(null); load() }} />

  return (
    <div className="card">
      <h2>{tr("spaces.title")}</h2>
      {spaces.map((s) => (
        <div key={s.id} className="task-row" onClick={() => setCurrent(s)}>
          <span className="task-title row" style={{ gap: 6 }}><IconGrid size={14} /> {s.name}</span>
          <span className="muted">{s.my_role || tr("spaces.admin_access")}</span>
        </div>
      ))}
      <form className="row" style={{ marginTop: 12 }} onSubmit={async (e) => {
        e.preventDefault()
        if (!name.trim()) return
        await api.post("/api/spaces", { name }).catch(() => {})
        setName(""); load()
      }}>
        <input className="input grow" placeholder={tr("spaces.new_placeholder")} value={name} onChange={(e) => setName(e.target.value)} />
        <button className="btn" type="submit">{tr("common.create")}</button>
      </form>
      <button className="nav-btn row" style={{ gap: 5, marginTop: 14 }} onClick={() => setShowArchived((v) => !v)}>
        <IconArchive size={13} /> {tr("archive.show_archived_spaces")}
      </button>
      {showArchived && <div style={{ marginTop: 8 }}><ArchivedSpacesPanel me={me} /></div>}
    </div>
  )
}

function SpaceView({ me, space, onBack }: { me: Me; space: Space; onBack: () => void }) {
  const [lists, setLists] = useState<List[]>([])
  const [pulse, setPulse] = useState<Pulse | null>(null)
  const [currentList, setCurrentList] = useState<List | null>(null)
  const [name, setName] = useState("")
  const [tab, setTab] = useState<"lists" | "notes" | "activity" | "archive" | "fields">("lists")
  const [templates, setTemplates] = useState<Array<{ id: number; name: string }>>([])

  const load = () => {
    api.get(`/api/spaces/${space.id}/lists`).then((r) => setLists(r.lists)).catch(() => {})
    api.get(`/api/spaces/${space.id}/pulse`).then(setPulse).catch(() => {})
  }
  useEffect(() => { load() }, [space.id])
  useEffect(() => { api.get("/api/templates").then((r) => setTemplates(r.templates)).catch(() => {}) }, [])

  if (currentList) return <ListView me={me} list={currentList} spaceId={space.id} onBack={() => { setCurrentList(null); load() }} />

  async function applyTemplate(templateId: number) {
    if (!templateId) return
    await api.post(`/api/templates/${templateId}/apply`, { space_id: space.id }).catch(() => {})
    load()
  }

  return (
    <div>
      <div className="row" style={{ marginBottom: 12 }}>
        <button className="nav-btn row" style={{ gap: 4, display: "inline-flex" }} onClick={onBack}><IconArrowLeft size={14} /> {tr("common.back")}</button>
        <h2 style={{ margin: 0 }}>{space.name}</h2>
      </div>

      <StatsCard spaceId={space.id} />
      {pulse && pulse.enabled !== false && (
        <div className="card pulse-card" style={{ marginBottom: 12 }}>
          <div className="pulse-visual">
            <span className={"pulse-dot pulse-dot--" + pulse.mood} title={tr("pulse.title")} />
            <span className="pulse-score-num">{pulse.score}</span>
          </div>
          <div>
            <div><b>{tr("pulse.title")}</b> · {tr("pulse.open")}: {pulse.open}/{pulse.total}</div>
            <div style={{ marginTop: 4 }}>
              <span className="signal"><IconClock size={12} /> {tr("pulse.overdue")}: {pulse.signals.overdue}</span>
              <span className="signal"><IconUser size={12} /> {tr("pulse.unassigned")}: {pulse.signals.unassigned}</span>
              <span className="signal"><IconPause size={12} /> {tr("pulse.stale")}: {pulse.signals.stale}</span>
              <span className="signal"><IconSlash size={12} /> {tr("pulse.blocked")}: {pulse.signals.blocked}</span>
            </div>
          </div>
        </div>
      )}

      <div className="row" style={{ marginBottom: 8, gap: 4 }}>
        <button className={"nav-btn row" + (tab === "lists" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("lists")}><IconList size={14} /> {tr("lists.title")}</button>
        <button className={"nav-btn row" + (tab === "notes" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("notes")}><IconFileText size={14} /> {tr("notes.title")}</button>
        <button className={"nav-btn row" + (tab === "activity" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("activity")}><IconActivity size={14} /> {tr("activity.title")}</button>
        <button className={"nav-btn row" + (tab === "archive" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("archive")}><IconArchive size={14} /> {tr("archive.title")}</button>
        <button className={"nav-btn row" + (tab === "fields" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("fields")}><IconSliders size={14} /> {tr("fields.title")}</button>
      </div>

      {tab === "lists" && (
        <div className="card">
          {lists.map((l) => (
            <div key={l.id} className="task-row" onClick={() => setCurrentList(l)}>
              <span className="task-title row" style={{ gap: 6 }}>{l.is_private ? <IconLock size={14} /> : <IconList size={14} />} {l.name}</span>
              <span className="muted">{l.done_count}/{l.task_count}</span>
              <progress className="progress" max={l.task_count || 1} value={l.done_count} />
            </div>
          ))}
          <form className="row" style={{ marginTop: 12 }} onSubmit={async (e) => {
            e.preventDefault()
            if (!name.trim()) return
            await api.post(`/api/spaces/${space.id}/lists`, { name, is_private: false }).catch(() => {})
            setName(""); load()
          }}>
            <input className="input grow" placeholder={tr("lists.new_placeholder")} value={name} onChange={(e) => setName(e.target.value)} />
            <button className="btn" type="submit">{tr("common.create")}</button>
          </form>
          {templates.length > 0 && (
            <div className="row" style={{ marginTop: 8 }}>
              <select className="input" defaultValue="" onChange={(e) => applyTemplate(Number(e.target.value))}>
                <option value="" disabled>{tr("templates.apply")}</option>
                {templates.map((t) => <option key={t.id} value={t.id}>{t.name}</option>)}
              </select>
            </div>
          )}
        </div>
      )}
      {tab === "notes" && <div className="card"><NotesPanel spaceId={space.id} /></div>}
      {tab === "activity" && <div className="card"><ActivityPanel spaceId={space.id} /></div>}
      {tab === "archive" && <div className="card"><ArchivePanel me={me} spaceId={space.id} /></div>}
      {tab === "fields" && <div className="card"><FieldsPanel spaceId={space.id} isOwner={space.my_role === "owner"} /></div>}
    </div>
  )
}

// ---------- Kanban board (drag cards between the space's workflow statuses) ----------

function KanbanBoard({ tasks, statuses, onOpen, onDrop }: {
  tasks: Task[]; statuses: string[]; onOpen: (t: Task) => void; onDrop: (task: Task, status: string) => void
}) {
  return (
    <div className="kanban-board" style={{ gridTemplateColumns: `repeat(${statuses.length}, 1fr)` }}>
      {statuses.map((s) => (
        <div key={s} className="kanban-col"
          onDragOver={(e) => e.preventDefault()}
          onDrop={(e) => {
            e.preventDefault()
            const id = Number(e.dataTransfer.getData("text/plain"))
            const t = tasks.find((x) => x.id === id)
            if (t && t.status !== s) onDrop(t, s)
          }}>
          <div className="kanban-col-header">
            <span>{DEFAULT_STATUSES.includes(s) ? tr("task.status." + s) : s}</span>
            <span className="muted">{tasks.filter((t) => t.status === s).length}</span>
          </div>
          {tasks.filter((t) => t.status === s).map((t) => (
            <div key={t.id} className="kanban-card" draggable
              onDragStart={(e) => e.dataTransfer.setData("text/plain", String(t.id))}
              onClick={() => onOpen(t)}>
              <div>{t.title}</div>
              <div className="row" style={{ marginTop: 6, gap: 6, flexWrap: "wrap" }}>
                {t.priority && <span className="muted" style={{ fontSize: 11 }}>{tr("task.priority." + t.priority)}</span>}
                {t.due_at && <span className={"due " + dueClass(t.due_at)}>{dueLabel(t.due_at)}</span>}
                {t.subtasks_total > 0 && <span className="muted" style={{ fontSize: 11 }}>{t.subtasks_done}/{t.subtasks_total}</span>}
              </div>
            </div>
          ))}
        </div>
      ))}
    </div>
  )
}

// ---------- Table view ----------

function TableView({ tasks, onOpen, onToggle }: {
  tasks: Task[]; onOpen: (t: Task) => void; onToggle: (t: Task) => void
}) {
  return (
    <table style={{ width: "100%", borderCollapse: "collapse", fontSize: 13 }}>
      <thead>
        <tr style={{ textAlign: "left", borderBottom: "1px solid var(--border)" }}>
          <th style={{ padding: "8px 6px", width: 28 }}></th>
          <th style={{ padding: "8px 6px" }}>{tr("table.title")}</th>
          <th style={{ padding: "8px 6px" }}>{tr("task.status")}</th>
          <th style={{ padding: "8px 6px" }}>{tr("task.priority")}</th>
          <th style={{ padding: "8px 6px" }}>{tr("table.due")}</th>
        </tr>
      </thead>
      <tbody>
        {tasks.map((t) => (
          <tr key={t.id} style={{ borderBottom: "1px solid var(--border)", cursor: "pointer" }} onClick={() => onOpen(t)}>
            <td style={{ padding: "8px 6px" }} onClick={(e) => e.stopPropagation()}>
              <input type="checkbox" checked={!!t.completed_at} onChange={() => onToggle(t)} />
            </td>
            <td style={{ padding: "8px 6px", textDecoration: t.completed_at ? "line-through" : "none", opacity: t.completed_at ? 0.55 : 1 }}>{t.title}</td>
            <td style={{ padding: "8px 6px" }}>{DEFAULT_STATUSES.includes(t.status) ? tr("task.status." + t.status) : t.status}</td>
            <td style={{ padding: "8px 6px" }}>{tr("task.priority." + t.priority)}</td>
            <td style={{ padding: "8px 6px" }}>{t.due_at ? <span className={"due " + dueClass(t.due_at)}>{dueLabel(t.due_at)}</span> : ""}</td>
          </tr>
        ))}
        {tasks.length === 0 && <tr><td colSpan={5} className="muted" style={{ padding: "8px 6px" }}>{tr("my.empty")}</td></tr>}
      </tbody>
    </table>
  )
}

// ---------- Calendar (month grid; tasks placed on their due date, spec section 12) ----------

function CalendarView({ tasks, onOpen }: { tasks: Task[]; onOpen: (t: Task) => void }) {
  const [cursor, setCursor] = useState(() => { const d = new Date(); d.setDate(1); return d })

  const byDay = new Map<string, Task[]>()
  for (const t of tasks) {
    if (!t.due_at) continue
    const key = new Date(t.due_at).toDateString()
    if (!byDay.has(key)) byDay.set(key, [])
    byDay.get(key)!.push(t)
  }

  const year = cursor.getFullYear()
  const month = cursor.getMonth()
  const startOffset = (new Date(year, month, 1).getDay() + 6) % 7 // week starts Monday
  const daysInMonth = new Date(year, month + 1, 0).getDate()
  const cells: (Date | null)[] = []
  for (let i = 0; i < startOffset; i++) cells.push(null)
  for (let d = 1; d <= daysInMonth; d++) cells.push(new Date(year, month, d))
  while (cells.length % 7 !== 0) cells.push(null)

  const todayKey = new Date().toDateString()
  const weekdayLabels = Array.from({ length: 7 }, (_, i) =>
    new Date(2024, 0, 1 + i).toLocaleDateString(getFormattingLocale(), { weekday: "short" }))

  return (
    <div className="calendar-view">
      <div className="row" style={{ justifyContent: "space-between", marginBottom: 8 }}>
        <button className="nav-btn" onClick={() => setCursor(new Date(year, month - 1, 1))}>‹</button>
        <b>{cursor.toLocaleDateString(getFormattingLocale(), { month: "long", year: "numeric" })}</b>
        <button className="nav-btn" onClick={() => setCursor(new Date(year, month + 1, 1))}>›</button>
      </div>
      <div className="calendar-grid calendar-grid--head">
        {weekdayLabels.map((w, i) => <div key={i} className="calendar-weekday">{w}</div>)}
      </div>
      <div className="calendar-grid">
        {cells.map((d, i) => {
          if (!d) return <div key={i} className="calendar-cell calendar-cell--pad" />
          const dayTasks = byDay.get(d.toDateString()) || []
          const visible = dayTasks.slice(0, 3)
          return (
            <div key={i} className={"calendar-cell" + (d.toDateString() === todayKey ? " calendar-cell--today" : "")}>
              <div className="calendar-daynum">{d.getDate()}</div>
              {visible.map((t) => (
                <div key={t.id} className={"calendar-task " + dueClass(t.due_at)} title={t.title} onClick={() => onOpen(t)}>
                  {t.title}
                </div>
              ))}
              {dayTasks.length > visible.length && (
                <div className="muted" style={{ fontSize: 11 }}>+{dayTasks.length - visible.length} {tr("view.calendar_more")}</div>
              )}
            </div>
          )
        })}
      </div>
    </div>
  )
}

// ---------- saved filters (spec section 12; backend already existed in filters.go, unused until now) ----------

type FilterQuery = { status?: string; priority?: string; overdue?: boolean }
type SavedFilterT = { id: number; list_id: number | null; name: string; query: FilterQuery }

function matchesFilter(t: Task, q: FilterQuery): boolean {
  if (q.status && t.status !== q.status) return false
  if (q.priority && t.priority !== q.priority) return false
  if (q.overdue && !(t.due_at && !t.completed_at && new Date(t.due_at).getTime() < Date.now())) return false
  return true
}

function FiltersBar({ listId, statuses, onFilter }: {
  listId?: number | null; statuses: string[]; onFilter: (q: FilterQuery | null) => void
}) {
  const [filters, setFilters] = useState<SavedFilterT[]>([])
  const [activeId, setActiveId] = useState<number | null>(null)
  const [showForm, setShowForm] = useState(false)
  const [name, setName] = useState("")
  const [fStatus, setFStatus] = useState("")
  const [fPriority, setFPriority] = useState("")
  const [fOverdue, setFOverdue] = useState(false)

  const load = () => {
    const qs = listId ? `?list_id=${listId}` : ""
    api.get(`/api/filters${qs}`).then((r) => setFilters(r.filters)).catch(() => {})
  }
  useEffect(() => { load() }, [listId])

  function apply(f: SavedFilterT | null) {
    setActiveId(f?.id ?? null)
    onFilter(f?.query ?? null)
  }

  async function save(e: React.FormEvent) {
    e.preventDefault()
    if (!name.trim()) return
    const query: FilterQuery = {}
    if (fStatus) query.status = fStatus
    if (fPriority) query.priority = fPriority
    if (fOverdue) query.overdue = true
    await api.post("/api/filters", { name, list_id: listId ?? null, query }).catch(() => {})
    setName(""); setFStatus(""); setFPriority(""); setFOverdue(false); setShowForm(false)
    load()
  }

  async function remove(id: number) {
    await api.del(`/api/filters/${id}`).catch(() => {})
    if (activeId === id) apply(null)
    load()
  }

  return (
    <div className="row" style={{ gap: 6, flexWrap: "wrap", marginBottom: 10 }}>
      <button className={"nav-btn row" + (activeId === null ? " active" : "")} style={{ gap: 4, display: "inline-flex" }} onClick={() => apply(null)}>
        <IconSliders size={12} /> {tr("filters.all")}
      </button>
      {filters.map((f) => (
        <span key={f.id} className={"nav-btn row" + (activeId === f.id ? " active" : "")} style={{ gap: 4 }}>
          <span style={{ cursor: "pointer" }} onClick={() => apply(activeId === f.id ? null : f)}>{f.name}</span>
          <IconX size={11} style={{ cursor: "pointer", opacity: 0.6 }} onClick={() => remove(f.id)} />
        </span>
      ))}
      <button className="nav-btn row" style={{ gap: 4, display: "inline-flex" }} onClick={() => setShowForm((v) => !v)}>+ {tr("filters.new")}</button>
      {showForm && (
        <form className="row" style={{ gap: 6, width: "100%", marginTop: 4, flexWrap: "wrap" }} onSubmit={save}>
          <input className="input" style={{ maxWidth: 160 }} placeholder={tr("filters.name_placeholder")} value={name} onChange={(e) => setName(e.target.value)} />
          <select className="input" style={{ width: "auto" }} value={fStatus} onChange={(e) => setFStatus(e.target.value)}>
            <option value="">{tr("task.status")}</option>
            {statuses.map((s) => <option key={s} value={s}>{DEFAULT_STATUSES.includes(s) ? tr("task.status." + s) : s}</option>)}
          </select>
          <select className="input" style={{ width: "auto" }} value={fPriority} onChange={(e) => setFPriority(e.target.value)}>
            <option value="">{tr("task.priority")}</option>
            <option value="low">{tr("task.priority.low")}</option>
            <option value="normal">{tr("task.priority.normal")}</option>
            <option value="high">{tr("task.priority.high")}</option>
            <option value="urgent">{tr("task.priority.urgent")}</option>
          </select>
          <label className="row" style={{ gap: 4, fontSize: 13 }}>
            <input type="checkbox" checked={fOverdue} onChange={(e) => setFOverdue(e.target.checked)} /> {tr("my_week.overdue")}
          </label>
          <button className="btn" type="submit">{tr("common.create")}</button>
        </form>
      )}
    </div>
  )
}

function ListView({ me, list, spaceId, onBack }: { me: Me; list: List; spaceId: number; onBack: () => void }) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [open, setOpen] = useState<Task | null>(null)
  const [title, setTitle] = useState("")
  const [due, setDue] = useState("")
  const [viewMode, setViewMode] = useState<"list" | "kanban" | "table" | "calendar">("list")
  const [statuses, setStatuses] = useState<string[]>(DEFAULT_STATUSES)
  const [filterQuery, setFilterQuery] = useState<FilterQuery | null>(null)

  const load = () => api.get(`/api/lists/${list.id}/tasks`).then((r) => setTasks(r.tasks)).catch(() => {})
  useEffect(() => { load() }, [list.id])
  useEffect(() => {
    api.get(`/api/spaces/${spaceId}/workflow`).then((r: Workflow) => setStatuses(r.statuses)).catch(() => {})
  }, [spaceId])

  async function toggle(task: Task) {
    await api.patch(`/api/tasks/${task.id}`, { status: task.completed_at ? "open" : "done" }).catch(() => {})
    load()
  }

  async function moveToStatus(task: Task, status: string) {
    await api.patch(`/api/tasks/${task.id}`, { status }).catch(() => {})
    load()
  }

  const roots = tasks.filter((t) => !t.parent_id)
  const filteredRoots = filterQuery ? roots.filter((t) => matchesFilter(t, filterQuery)) : roots

  return (
    <div className="card">
      <div className="row" style={{ marginBottom: 8, flexWrap: "wrap" }}>
        <button className="nav-btn row" style={{ gap: 4, display: "inline-flex" }} onClick={onBack}><IconArrowLeft size={14} /> {tr("common.back")}</button>
        <h2 style={{ margin: 0 }} className="grow">{list.name}</h2>
        <div className="row" style={{ gap: 4 }}>
          <button className={"nav-btn row" + (viewMode === "list" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setViewMode("list")}><IconMenu size={14} /> {tr("view.list")}</button>
          <button className={"nav-btn row" + (viewMode === "kanban" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setViewMode("kanban")}><IconColumns size={14} /> {tr("view.kanban")}</button>
          <button className={"nav-btn row" + (viewMode === "table" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setViewMode("table")}><IconTable size={14} /> {tr("view.table")}</button>
          <button className={"nav-btn row" + (viewMode === "calendar" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setViewMode("calendar")}><IconCalendar size={14} /> {tr("view.calendar")}</button>
        </div>
      </div>

      <FiltersBar listId={list.id} statuses={statuses} onFilter={setFilterQuery} />

      {viewMode === "list" && filteredRoots.map((task) => (
        <div key={task.id}>
          <TaskRow task={task} onToggle={toggle} onOpen={setOpen} />
          {tasks.filter((s) => s.parent_id === task.id).map((sub) => (
            <div key={sub.id} style={{ marginLeft: 28 }}>
              <TaskRow task={sub} onToggle={toggle} onOpen={setOpen} />
            </div>
          ))}
        </div>
      ))}
      {viewMode === "kanban" && <KanbanBoard tasks={filteredRoots} statuses={statuses} onOpen={setOpen} onDrop={moveToStatus} />}
      {viewMode === "table" && <TableView tasks={filteredRoots} onOpen={setOpen} onToggle={toggle} />}
      {viewMode === "calendar" && <CalendarView tasks={filteredRoots} onOpen={setOpen} />}

      <form className="row" style={{ marginTop: 12 }} onSubmit={async (e) => {
        e.preventDefault()
        if (!title.trim()) return
        await api.post(`/api/lists/${list.id}/tasks`, {
          title, due_at: due ? new Date(due).toISOString() : null,
        }).catch(() => {})
        setTitle(""); setDue(""); load()
      }}>
        <input className="input grow" placeholder={tr("task.new_placeholder")} value={title} onChange={(e) => setTitle(e.target.value)} />
        <input className="input" style={{ width: 170 }} type="date" value={due} onChange={(e) => setDue(e.target.value)} />
        <button className="btn" type="submit">+</button>
      </form>
      {open && <TaskModal task={open} me={me} spaceId={spaceId} onClose={() => setOpen(null)} onChanged={load} />}
    </div>
  )
}

// ---------- notifications ----------

const KIND_ICON: Record<string, React.ReactNode> = {
  approved: <IconCheckCircle size={15} />, task_assigned: <IconPin size={15} />,
  comment: <IconMessage size={15} />, reaction: <IconStar size={15} />,
  overdue: <IconClock size={15} />, space_added: <IconGrid size={15} />, list_shared: <IconList size={15} />,
  status_changed: <IconRefresh size={15} />, due_changed: <IconClock size={15} />,
  due_soon: <IconClock size={15} />, due_today: <IconAlertCircle size={15} />,
  archive_expiring: <IconArchive size={15} />,
}

export function NotificationsPage({ onRead }: { onRead: () => void }) {
  const [items, setItems] = useState<any[]>([])
  const load = () => api.get("/api/notifications").then((r) => setItems(r.notifications)).catch(() => {})
  useEffect(() => { load() }, [])

  return (
    <div className="card">
      <div className="row">
        <h2 className="grow">{tr("notif.title")}</h2>
        <button className="nav-btn" onClick={async () => { await api.post("/api/notifications/read"); load(); onRead() }}>
          {tr("notif.read_all")}
        </button>
      </div>
      {items.length === 0 && <p className="muted">{tr("notif.empty")}</p>}
      {items.map((n) => (
        <div key={n.id} className="task-row" style={{ opacity: n.read_at ? 0.55 : 1 }}>
          <span className="task-title row" style={{ gap: 6 }}>
            {KIND_ICON[n.kind] || null}
            <span>
              {tr("notif.kind." + n.kind)}
              {n.kind === "due_soon" && n.payload?.days ? ` (${n.payload.days}d)` : ""}
              {n.kind === "archive_expiring" && n.payload?.days_left ? ` (${n.payload.days_left}d)` : ""}
              {n.payload?.title ? ` · «${n.payload.title}»` : ""}
              {n.payload?.task_title ? ` · «${n.payload.task_title}»` : ""}
              {n.payload?.by ? ` · ${tr("notif.by")} @${n.payload.by}` : ""}
              {n.payload?.emoji ? ` ${n.payload.emoji}` : ""}
            </span>
          </span>
          <span className="muted">{new Date(n.created_at).toLocaleString(getFormattingLocale())}</span>
        </div>
      ))}
    </div>
  )
}

// ---------- admin panel ----------

export function AdminPage({ me }: { me: Me }) {
  const [users, setUsers] = useState<any[]>([])
  const [tempPass, setTempPass] = useState<{ user: string; pass: string } | null>(null)
  const load = () => api.get("/api/admin/users").then((r) => setUsers(r.users)).catch(() => {})
  useEffect(() => { load() }, [])

  return (
    <div className="card">
      <h2>{trFormal("admin.users")}</h2>
      {tempPass && (
        <div className="card" style={{ borderColor: "var(--accent)", marginBottom: 12 }}>
          {trFormal("admin.temp_pass_for")} <b>@{tempPass.user}</b>: <code>{tempPass.pass}</code>
          <div className="muted">{trFormal("admin.shown_once")}</div>
        </div>
      )}
      {users.map((u) => (
        <div key={u.id} className="task-row" style={{ cursor: "default" }}>
          <span className="task-title">
            @{u.username} <span className="muted">· {u.role} · {u.status}</span>
          </span>
          {u.status === "pending" && (
            <>
              <button className="btn" onClick={async () => { await api.post(`/api/admin/users/${u.id}/approve`, { role: "user" }); load() }}>
                {trFormal("admin.approve")}
              </button>
              <button className="nav-btn" onClick={async () => { await api.post(`/api/admin/users/${u.id}/status`, { status: "rejected" }); load() }}>
                {trFormal("admin.reject")}
              </button>
            </>
          )}
          {u.status === "active" && u.role !== "root" && (
            <>
              <button className="nav-btn" onClick={async () => { await api.post(`/api/admin/users/${u.id}/status`, { status: "blocked" }); load() }}>
                {trFormal("admin.block")}
              </button>
              <button className="nav-btn" onClick={async () => {
                const r = await api.post(`/api/admin/users/${u.id}/reset-password`)
                setTempPass({ user: u.username, pass: r.temp_password })
              }}>
                {trFormal("admin.reset_password")}
              </button>
            </>
          )}
          {u.status === "blocked" && (
            <button className="btn" onClick={async () => { await api.post(`/api/admin/users/${u.id}/status`, { status: "active" }); load() }}>
              {trFormal("admin.unblock")}
            </button>
          )}
        </div>
      ))}
    </div>
  )
}
