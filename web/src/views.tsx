import { useEffect, useState } from "react"
import { createPortal } from "react-dom"
import { api, REACTIONS, DEFAULT_STATUSES, type List, type Me, type Pulse, type Space, type Task, type Workflow, type ActiveFocus } from "./api"
import { AttachmentsBlock, ModalShell, StatsCard, useConfirm, FocusWidget, FocusPresence, NotesPanel, ActivityPanel, ArchivePanel, ArchivedSpacesPanel, FieldsPanel, type FieldDef } from "./extras"
import { tr, trFormal, setLocale, getLocale, getFormattingLocale, SUPPORTED } from "./i18n"
import { TimelineView } from "./timeline"
import { WorkflowEditor } from "./workflow"
import { AssigneePicker } from "./members"
import { WorkloadPanel, ImportCard } from "./functional"
import { IconStar, IconRefresh, IconLock, IconX, IconUser, IconPause, IconSlash, IconClock, IconGrid, IconArrowLeft, IconList, IconFileText, IconActivity, IconMenu, IconColumns, IconTable, IconCheckCircle, IconMessage, IconPin, IconAlertCircle, IconArchive, IconCalendar, IconSliders, IconBarChart, IconEdit, IconCopy } from "./icons"
import { endOfDayISO, dueClass, dueLabel, formatSystemComment, StatusChip, TaskRow } from "./taskui"

// ---------- helpers ----------




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

// ---------- inline focus button ----------


// ---------- status chip ----------




// ---------- right-click menu on a task row ----------

// Quick edits without opening the modal. This exists because changing one field meant opening the
// full task view, which is slow and — before the timer was made global — used to knock the focus
// session off screen.
//
// Rendered through a portal so it can't be clipped by a scrolling ancestor, and positioned from
// the click point but clamped to the viewport so a right-click near the bottom edge stays visible.
function TaskContextMenu({ task, x, y, statuses, meId, onClose, onPatch, onOpenFull }: {
  task: Task
  x: number; y: number
  statuses: string[]
  meId: number
  onClose: () => void
  onPatch: (patch: Record<string, unknown>) => void
  onOpenFull: () => void
}) {
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") onClose() }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [onClose])

  const W = 210, H = 330
  const left = Math.min(x, Math.max(8, window.innerWidth - W - 8))
  const top = Math.min(y, Math.max(8, window.innerHeight - H - 8))

  return createPortal(
    // A full-screen transparent layer catches the click that dismisses the menu, so no document
    // listener has to be juggled against the opening click itself.
    <div className="ctx-backdrop" onMouseDown={onClose} onContextMenu={(e) => { e.preventDefault(); onClose() }}>
      <div className="ctx-menu" style={{ left, top, width: W }} onMouseDown={(e) => e.stopPropagation()}>
        <div className="ctx-title" title={task.title}>{task.title}</div>

        <div className="ctx-group">{tr("task.status")}</div>
        {statuses.map((st) => (
          <button key={st} className={"ctx-item" + (task.status === st ? " active" : "")}
            onClick={() => onPatch({ status: st })}>
            {DEFAULT_STATUSES.includes(st) ? tr("task.status." + st) : st}
          </button>
        ))}

        <div className="ctx-group">{tr("task.priority")}</div>
        {["low", "normal", "high", "urgent"].map((p) => (
          <button key={p} className={"ctx-item" + (task.priority === p ? " active" : "")}
            onClick={() => onPatch({ priority: p })}>
            {tr("task.priority." + p)}
          </button>
        ))}

        <div className="ctx-group">{tr("task.due_at")}</div>
        <button className="ctx-item" onClick={() => onPatch({ due_at: endOfDayISO(0) })}>{tr("ctx.due_today")}</button>
        <button className="ctx-item" onClick={() => onPatch({ due_at: endOfDayISO(1) })}>{tr("ctx.due_tomorrow")}</button>
        <button className="ctx-item" onClick={() => onPatch({ due_at: endOfDayISO(7) })}>{tr("ctx.due_week")}</button>
        <button className="ctx-item" onClick={() => onPatch({ clear_due_at: true })}>{tr("bulk.clear_due")}</button>

        <div className="ctx-sep" />
        {task.assignee_id === meId
          ? <button className="ctx-item" onClick={() => onPatch({ clear_assignee: true })}>{tr("ctx.unassign")}</button>
          : <button className="ctx-item" onClick={() => onPatch({ assignee_id: meId })}>{tr("bulk.assign_me")}</button>}
        <button className="ctx-item" onClick={onOpenFull}>{tr("ctx.open_full")}</button>
      </div>
    </div>,
    document.body,
  )
}


// ---------- task version history (spec section 11 — task_versions was write-only until now) ----------

function TaskHistorySection({ taskId, onRestored }: { taskId: number; onRestored: () => void }) {
  const [show, setShow] = useState(false)
  const [versions, setVersions] = useState<any[]>([])
  const load = () => api.get(`/api/tasks/${taskId}/versions`).then((r) => setVersions(r.versions)).catch(() => {})
  useEffect(() => { if (show) load() }, [show])

  async function restore(versionId: number) {
    if (!window.confirm(tr("task.history_confirm_restore"))) return
    await api.post(`/api/tasks/${taskId}/versions/${versionId}/restore`).catch(() => {})
    onRestored()
    load()
  }

  return (
    <div style={{ marginBottom: 16 }}>
      <button className="nav-btn row" style={{ gap: 5, display: "inline-flex" }} onClick={() => setShow((v) => !v)}>
        <IconClock size={13} /> {tr("task.history")}
      </button>
      {show && (
        <div style={{ marginTop: 8 }}>
          {versions.length === 0 && <p className="muted" style={{ fontSize: 12 }}>{tr("task.history_empty")}</p>}
          {versions.map((v) => (
            <div key={v.id} className="row" style={{ fontSize: 12, marginBottom: 4, gap: 6 }}>
              <span className="muted">{new Date(v.changed_at).toLocaleString(getFormattingLocale())} · @{v.editor}</span>
              <button className="nav-btn" style={{ padding: "1px 6px", fontSize: 11 }} onClick={() => restore(v.id)}>{tr("task.history_restore")}</button>
            </div>
          ))}
        </div>
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
  // null = no manual override (progress derives from subtasks, or is simply unset)
  const [progress, setProgress] = useState<number | null>(
    typeof task.progress === "number" ? task.progress : null)
  const [weight, setWeight] = useState(task.weight ?? 1)
  const [assigneeId, setAssigneeId] = useState<number | null>(task.assignee_id)
  // <input type="date"> wants YYYY-MM-DD in *local* time; slicing an ISO string would shift
  // the day for anyone not on UTC.
  const dateInputValue = (v: string | null) => {
    if (!v) return ""
    const d = new Date(v)
    const m = String(d.getMonth() + 1).padStart(2, "0")
    const day = String(d.getDate()).padStart(2, "0")
    return `${d.getFullYear()}-${m}-${day}`
  }
  const [startAt, setStartAt] = useState(() => dateInputValue(task.start_at))
  const [dueAt, setDueAt] = useState(() => dateInputValue(task.due_at))
  // Watchers, review workflow and threaded replies (spec sections 5 and 7).
  const [watching, setWatching] = useState(false)
  const [watchers, setWatchers] = useState<Array<{ id: number; username: string }>>([])
  const [reviewState, setReviewState] = useState(task.review_state)
  const [reviewNote, setReviewNote] = useState(task.review_note || "")
  const [returnNote, setReturnNote] = useState("")
  const [replyTo, setReplyTo] = useState<{ id: number; author: string } | null>(null)
  const { confirm, confirmElement } = useConfirm()

  const loadWatchers = () =>
    api.get(`/api/tasks/${task.id}/watchers`).then((r) => {
      setWatchers(r.watchers || [])
      setWatching(!!r.watching)
    }).catch(() => {})
  useEffect(() => { loadWatchers() }, [task.id])

  async function toggleWatch() {
    if (watching) await api.del(`/api/tasks/${task.id}/watch`).catch(() => {})
    else await api.post(`/api/tasks/${task.id}/watch`).catch(() => {})
    loadWatchers()
  }

  async function submitForReview() {
    try {
      const r = await api.post(`/api/tasks/${task.id}/review/submit`)
      setReviewState(r.review_state)
      setStatus("review")
      onChanged()
    } catch (e) { setError((e as Error).message) }
  }

  async function decideReview(accept: boolean) {
    try {
      const r = await api.post(`/api/tasks/${task.id}/review/decide`,
        { accept, note: accept ? "" : returnNote })
      setReviewState(r.review_state)
      setStatus(r.status)
      setReviewNote(accept ? "" : returnNote)
      setReturnNote("")
      // Accepting completes the task, which closes its focus session server-side.
      if (accept) window.dispatchEvent(new CustomEvent("todorio:focus-changed"))
      onChanged()
    } catch (e) { setError((e as Error).message) }
  }
  const [freq, setFreq] = useState(task.recurrence?.freq || "none")
  const [blockedByInput, setBlockedByInput] = useState("")
  const [blockedBy, setBlockedBy] = useState<number[]>(task.blocked_by || [])
  const [customFields, setCustomFields] = useState<Record<string, string>>(task.custom_fields || {})
  const [newKey, setNewKey] = useState("")
  const [newValue, setNewValue] = useState("")
  const [fieldSchema, setFieldSchema] = useState<FieldDef[]>([])

  // Presence: the `task` prop is a snapshot from whenever the parent list last loaded it (the
  // modal doesn't otherwise refresh while open), so "who's working on this" and its elapsed time
  // would freeze at open-time forever without this. A light poll is enough for a presence caption
  // — nobody needs sub-second precision for "is Ivan still on this".
  const [activeFocus, setActiveFocus] = useState<ActiveFocus[]>(task.active_focus || [])
  useEffect(() => {
    setActiveFocus(task.active_focus || [])
    const refresh = () => api.get(`/api/tasks/${task.id}`).then((r) => setActiveFocus(r.task?.active_focus || [])).catch(() => {})
    const iv = setInterval(refresh, 20000)
    return () => clearInterval(iv)
  }, [task.id])

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
    // "done" closes any focus session on this task server-side — refresh the sidebar clock.
    if (s === "done") window.dispatchEvent(new CustomEvent("todorio:focus-changed"))
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
      await api.post(`/api/tasks/${task.id}/comments`,
        replyTo ? { body, parent_id: replyTo.id } : { body })
      setBody("")
      setReplyTo(null)
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
    <ModalShell onClose={onClose} maxWidth={640}>
        {confirmElement}
        <div className="row">
          <h2 className="grow" style={{ margin: 0, fontSize: 20 }}>{task.title}</h2>
          <button className="nav-btn" onClick={onClose}>
            <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><line x1="18" y1="6" x2="6" y2="18"/><line x1="6" y1="6" x2="18" y2="18"/></svg>
          </button>
        </div>

        {task.description && <p style={{ color: "var(--text-muted)", margin: "8px 0 16px" }}>{task.description}</p>}

        {/* Task Properties grid */}
        <div className="card task-props" style={{ padding: 12, marginBottom: 16, display: "grid", gridTemplateColumns: "1fr 1fr", gap: 12 }}>
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

          {/* Schedule: start feeds the Timeline bar's left edge, the deadline its right (spec
              sections 8 and 12). Date-only inputs — the product never asks for a time of day,
              and <input type="date"> is localised by the browser for free. */}
          <div>
            <label className="muted row" style={{ fontSize: 12, marginBottom: 4, gap: 4 }}>
              <IconCalendar size={12} /> {tr("task.start_at")}
            </label>
            <input className="input" type="date" value={startAt}
              onChange={(e) => {
                const v = e.target.value
                setStartAt(v)
                updateTask(v ? { start_at: new Date(v).toISOString() } : { clear_start_at: true })
              }} />
          </div>

          <div>
            <label className="muted row" style={{ fontSize: 12, marginBottom: 4, gap: 4 }}>
              <IconClock size={12} /> {tr("task.due_at")}
            </label>
            <input className="input" type="date" value={dueAt}
              onChange={(e) => {
                const v = e.target.value
                setDueAt(v)
                updateTask(v ? { due_at: new Date(v).toISOString() } : { clear_due_at: true })
              }} />
          </div>

          {/* Manual progress (spec section 6): only for tasks without subtasks — when a task has
              subtasks its progress is derived from them (done/total), so a manual override would
              contradict what the subtask list plainly shows. */}
          {task.subtasks_total === 0 ? (
            <div>
              <label className="muted" style={{ display: "block", fontSize: 12, marginBottom: 4 }}>
                {tr("task.progress")}: {progress ?? 0}%
              </label>
              <div className="row" style={{ gap: 8 }}>
                <input type="range" min={0} max={100} step={5} style={{ flex: 1 }}
                  value={progress ?? 0}
                  onChange={(e) => setProgress(Number(e.target.value))}
                  onMouseUp={(e) => updateTask({ progress: Number((e.target as HTMLInputElement).value) })}
                  onTouchEnd={(e) => updateTask({ progress: Number((e.target as HTMLInputElement).value) })}
                  onKeyUp={(e) => updateTask({ progress: Number((e.target as HTMLInputElement).value) })} />
                {progress !== null && (
                  <button className="nav-btn" title={tr("task.progress_clear")}
                    onClick={() => { setProgress(null); updateTask({ clear_progress: true }) }}>
                    <IconX size={12} />
                  </button>
                )}
              </div>
            </div>
          ) : (
            <div>
              <label className="muted" style={{ display: "block", fontSize: 12, marginBottom: 4 }}>
                {tr("task.progress")}
              </label>
              <progress className="progress" max={task.subtasks_total} value={task.subtasks_done} />
              <div className="muted" style={{ fontSize: 12, marginTop: 2 }}>
                {task.subtasks_done}/{task.subtasks_total} · {tr("task.progress_auto")}
              </div>
            </div>
          )}

          {/* Weight feeds weighted list progress and the rankings score, so that closing ten
              trivial tasks doesn't outrank finishing one hard one (spec sections 6 and 14). */}
          {/* Assignee. The picker asks the server who may hold this task
              (GET /api/lists/{id}/assignable), so the options can never contain someone whose
              write would be rejected. Clearing it sends clear_assignee, the same flag the row
              context menu already uses -- assignee_id: null would be read as "absent". */}
          <div>
            <label className="muted row" style={{ fontSize: 12, marginBottom: 4, gap: 4 }}>
              <IconUser size={12} /> {tr("task.assignee")}
            </label>
            <AssigneePicker listId={task.list_id} value={assigneeId}
              onChange={(v) => {
                setAssigneeId(v)
                updateTask(v === null ? { clear_assignee: true } : { assignee_id: v })
              }} />
          </div>

          <div>
            <label className="muted" style={{ display: "block", fontSize: 12, marginBottom: 4 }}>{tr("task.weight")}</label>
            <input className="input" type="number" min={1} max={100} value={weight}
              onChange={(e) => setWeight(Number(e.target.value))}
              onBlur={() => updateTask({ weight })} />
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

        <TaskHistorySection taskId={task.id} onRestored={onChanged} />

        <FocusWidget taskId={task.id} />
        <FocusPresence active={activeFocus} meId={me.id} />

        {/* Review workflow (spec section 5): the owner accepts or returns; a return must carry a
            reason so the author knows what to change. */}
        <div className="card" style={{ padding: 12, marginBottom: 16 }}>
          <div className="row" style={{ gap: 8, flexWrap: "wrap" }}>
            {reviewState === "pending" && (
              <span className="badge">{tr("task.review_pending")}</span>
            )}
            {reviewState === "accepted" && (
              <span className="badge" style={{ background: "var(--pulse-ok)" }}>{tr("task.review_accepted")}</span>
            )}
            {reviewState === "returned" && (
              <span className="badge" style={{ background: "var(--due-overdue)" }}>{tr("task.review_returned")}</span>
            )}
            {reviewState !== "pending" && (
              <button className="nav-btn" onClick={submitForReview}>{tr("task.review_submit")}</button>
            )}
            {reviewState === "pending" && (
              <>
                <button className="btn" style={{ padding: "4px 10px" }} onClick={() => decideReview(true)}>
                  {tr("task.review_accept")}
                </button>
                <input className="input" style={{ maxWidth: 220 }} placeholder={tr("task.review_note")}
                  value={returnNote} onChange={(e) => setReturnNote(e.target.value)} />
                <button className="nav-btn" onClick={() => decideReview(false)}>{tr("task.review_return")}</button>
              </>
            )}
            <button className={"nav-btn row" + (watching ? " active" : "")}
              style={{ gap: 5, display: "inline-flex", marginLeft: "auto" }} onClick={toggleWatch}>
              <IconUser size={13} /> {watching ? tr("task.unwatch") : tr("task.watch")}
              {watchers.length > 0 && <span className="muted">· {watchers.length}</span>}
            </button>
          </div>
          {reviewState === "returned" && reviewNote && (
            <div className="muted" style={{ fontSize: 13, marginTop: 6 }}>
              {tr("task.review_note")}: {reviewNote}
            </div>
          )}
          {watchers.length > 0 && (
            <div className="muted" style={{ fontSize: 12, marginTop: 6 }}>
              {tr("task.watchers")}: {watchers.map((wt) => "@" + wt.username).join(", ")}
            </div>
          )}
        </div>

        <div className="section-title">{tr("task.comments")}</div>
        <AttachmentsBlock taskId={task.id} />
        {comments.map((c) => c.is_system ? (
          <div key={c.id} className="row muted" style={{ gap: 6, fontSize: 12, padding: "4px 0" }}>
            <IconRefresh size={12} />
            <span>@{c.author} {formatSystemComment(c.body)} · {new Date(c.created_at).toLocaleString(getFormattingLocale())}</span>
          </div>
        ) : (
          <div key={c.id} className="card"
            style={{ marginBottom: 8, padding: 12, marginLeft: c.parent_id ? 24 : 0 }}>
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
            <AttachmentsBlock commentId={c.id} compact />
            <button className="nav-btn" style={{ fontSize: 12, marginTop: 4 }}
              onClick={() => setReplyTo({ id: c.id, author: c.author })}>
              {tr("comment.reply")}
            </button>
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
        {replyTo && (
          <div className="row muted" style={{ gap: 6, fontSize: 12, marginBottom: 4 }}>
            {tr("comment.replying_to")} @{replyTo.author}
            <button className="nav-btn" style={{ fontSize: 11 }} onClick={() => setReplyTo(null)}>
              {tr("comment.cancel_reply")}
            </button>
          </div>
        )}
        <form className="row" onSubmit={send}>
          <input className="input grow" placeholder={tr("task.comment_placeholder")} value={body}
            onChange={(e) => setBody(e.target.value)} />
          <button className="btn" type="submit">
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><line x1="22" y1="2" x2="11" y2="13"/><polygon points="22 2 15 22 11 13 2 9 22 2"/></svg>
          </button>
        </form>
        <div className="error-text">{error}</div>

        <div className="row" style={{ marginTop: 16, justifyContent: "space-between" }}>
          <button className="nav-btn" style={{ color: "var(--due-overdue)" }} onClick={() => confirm({
              title: tr("confirm.archive_task_title").replace("{title}", task.title),
              body: tr("confirm.archive_body"),
              confirmLabel: tr("task.archive"), danger: true,
              action: async () => {
                await api.del(`/api/tasks/${task.id}`)
                window.dispatchEvent(new CustomEvent("todorio:focus-changed"))
                onChanged(); onClose()
              },
            })}>
            <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" style={{ marginRight: 6 }}><polyline points="3 6 5 6 21 6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/></svg>
            {tr("task.archive")}
          </button>
        </div>
    </ModalShell>
  )
}

// ---------- "My tasks" ----------

function MyWeekView({ tasks, favorites, onOpen, onToggle, onToggleFavorite, meId }: {
  tasks: Task[]; favorites: Set<number>; onOpen: (t: Task) => void; onToggle: (t: Task) => void
  onToggleFavorite: (t: Task) => void; meId?: number
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
          favorite={favorites.has(t.id)} onToggleFavorite={onToggleFavorite} meId={meId} />
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

// ---------- personal stats (spec section 14 — distinct from the per-space leaderboard) ----------

function MyStatsPanel() {
  const [period, setPeriod] = useState<"week" | "month">("week")
  const [stats, setStats] = useState<any | null>(null)
  useEffect(() => { api.get(`/api/my/stats?period=${period}`).then(setStats).catch(() => {}) }, [period])

  return (
    <div>
      <div className="row" style={{ gap: 4, marginBottom: 12 }}>
        <button className={"nav-btn" + (period === "week" ? " active" : "")} onClick={() => setPeriod("week")}>{tr("stats.week")}</button>
        <button className={"nav-btn" + (period === "month" ? " active" : "")} onClick={() => setPeriod("month")}>{tr("stats.month")}</button>
      </div>
      {!stats ? <p className="muted">{tr("search.searching")}</p> : stats.enabled === false ? (
        <p className="muted">{tr("mystats.disabled")}</p>
      ) : (
        <div className="card" style={{ padding: 14, display: "grid", gridTemplateColumns: "1fr 1fr", gap: 16 }}>
          <div><b style={{ fontSize: 20 }}>{stats.done}</b><div className="muted" style={{ fontSize: 12 }}>{tr("mystats.done")}</div></div>
          <div><b style={{ fontSize: 20 }}>{stats.on_time_pct}%</b><div className="muted" style={{ fontSize: 12 }}>{tr("mystats.on_time")}</div></div>
          <div><b style={{ fontSize: 20 }}>{stats.overdue}</b><div className="muted" style={{ fontSize: 12 }}>{tr("mystats.overdue")}</div></div>
          <div><b style={{ fontSize: 20 }}>{stats.most_active_list || "—"}</b><div className="muted" style={{ fontSize: 12 }}>{tr("mystats.most_active_list")}</div></div>
          <div><b style={{ fontSize: 20 }}>{Math.round(stats.focus_seconds / 60)}</b><div className="muted" style={{ fontSize: 12 }}>{tr("mystats.focus_time")}</div></div>
        </div>
      )}
    </div>
  )
}

type MySubTab = "all" | "today" | "overdue" | "review" | "no_deadline" | "mentions"

// Onboarding quest progress (spec section 12: "с прогресс-баром освоения"). The quests and the
// "create them on approval" step already existed; this was the missing readout.
//
// Hides itself in three cases: the account has no quest list at all (quests were off, or this
// predates the feature), all quests are done, or the user dismissed it — dismissal is per
// browser via localStorage, matching how the hotkey and sound toggles already persist, since
// there is no server-side "onboarding UI state" to put it in.
function OnboardingProgressBar() {
  const [data, setData] = useState<{ enabled: boolean; done?: number; total?: number } | null>(null)
  const [dismissed, setDismissed] = useState(() => localStorage.getItem("todorio.onboarding_dismissed") === "1")

  useEffect(() => {
    api.get("/api/onboarding/progress").then(setData).catch(() => setData({ enabled: false }))
  }, [])

  if (!data?.enabled || dismissed || (data.total && data.done === data.total)) return null

  const pct = data.total ? Math.round((data.done! / data.total) * 100) : 0
  return (
    <div className="card onboarding-bar" style={{ marginBottom: 12 }}>
      <div className="row" style={{ justifyContent: "space-between", marginBottom: 6 }}>
        <b>{tr("onboarding.progress_title")}</b>
        <button className="nav-btn" style={{ fontSize: 12 }}
          onClick={() => { localStorage.setItem("todorio.onboarding_dismissed", "1"); setDismissed(true) }}>
          {tr("onboarding.dismiss")}
        </button>
      </div>
      <progress className="progress" max={data.total} value={data.done} style={{ width: "100%" }} />
      <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>
        {tr("onboarding.progress_hint").replace("{done}", String(data.done)).replace("{total}", String(data.total))}
      </div>
    </div>
  )
}

export function MyTasksPage({ me }: { me: Me }) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [open, setOpen] = useState<Task | null>(null)
  const [tab, setTab] = useState<"list" | "week" | "stats">("list")
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
    // Completing a task closes its focus session server-side; tell the sidebar timer to
    // re-read now instead of ticking on until its next poll.
    window.dispatchEvent(new CustomEvent("todorio:focus-changed"))
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
      <OnboardingProgressBar />
      <div className="row" style={{ marginBottom: 8 }}>
        <h2 className="grow" style={{ margin: 0 }}>{tr("my.title")}</h2>
        <div className="row" style={{ gap: 4 }}>
          <button className={"nav-btn" + (tab === "list" ? " active" : "")} onClick={() => setTab("list")}>{tr("view.list")}</button>
          <button className={"nav-btn" + (tab === "week" ? " active" : "")} onClick={() => setTab("week")}>{tr("view.my_week")}</button>
          <button className={"nav-btn" + (tab === "stats" ? " active" : "")} onClick={() => setTab("stats")}>{tr("mystats.title")}</button>
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
                  favorite={favorites.has(task.id)} onToggleFavorite={toggleFavorite} meId={me.id} />
              ))}
            </>
          )}
        </>
      )}
      {tab === "week" && (
        <MyWeekView tasks={tasks} favorites={favorites} onOpen={setOpen} onToggle={toggle} onToggleFavorite={toggleFavorite} meId={me.id} />
      )}
      {tab === "stats" && <MyStatsPanel />}
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
  const [renamingId, setRenamingId] = useState<number | null>(null)
  const [renameValue, setRenameValue] = useState("")
  const { confirm, confirmElement } = useConfirm()
  const load = () => api.get("/api/spaces").then((r) => setSpaces(r.spaces)).catch(() => {})
  useEffect(() => { load() }, [])

  if (current) return <SpaceView me={me} space={current} onBack={() => { setCurrent(null); load() }} />

  const canManage = (s: Space) => s.my_role === "owner" || me.role === "root" || me.role === "admin"

  async function renameSpace(id: number) {
    const val = renameValue.trim()
    setRenamingId(null)
    if (!val) return
    await api.patch(`/api/spaces/${id}`, { name: val }).catch(() => {})
    load()
  }

  async function duplicateSpace(id: number) {
    await api.post(`/api/spaces/${id}/duplicate`, {}).catch(() => {})
    load()
  }

  return (
    <div className="card">
      {confirmElement}
      <h2>{tr("spaces.title")}</h2>
      {spaces.length === 0 && <p className="muted">{tr("spaces.empty")}</p>}
      {spaces.map((s) => {
        const editing = renamingId === s.id
        return (
          <div key={s.id} className="task-row" onClick={() => !editing && setCurrent(s)}>
            {editing ? (
              <form className="row grow" style={{ gap: 6 }} onClick={(e) => e.stopPropagation()}
                onSubmit={(e) => { e.preventDefault(); renameSpace(s.id) }}>
                <input className="input grow" autoFocus value={renameValue} onChange={(e) => setRenameValue(e.target.value)} />
                <button className="btn" type="submit">{tr("common.save")}</button>
                <button className="nav-btn" type="button" onClick={() => setRenamingId(null)}>{tr("confirm.cancel")}</button>
              </form>
            ) : (
              <>
                <span className="task-title row" style={{ gap: 6 }}><IconGrid size={14} /> {s.name}</span>
                <span className="muted" style={{ fontSize: 12 }}>{s.my_role || tr("spaces.admin_access")}</span>
              </>
            )}
            {!editing && canManage(s) && (
              <>
                <button className="nav-btn" style={{ padding: "2px 6px" }} title={tr("action.rename")}
                  onClick={(e) => { e.stopPropagation(); setRenamingId(s.id); setRenameValue(s.name) }}>
                  <IconEdit size={14} />
                </button>
                <button className="nav-btn" style={{ padding: "2px 6px" }} title={tr("action.duplicate")}
                  onClick={(e) => { e.stopPropagation(); duplicateSpace(s.id) }}>
                  <IconCopy size={14} />
                </button>
                <button className="nav-btn" style={{ padding: "2px 6px", color: "var(--due-overdue)" }}
                  title={tr("task.archive")}
                  onClick={(e) => {
                    e.stopPropagation()
                    confirm({
                      title: tr("spaces.archive_confirm").replace("{name}", s.name),
                      body: tr("confirm.archive_body"),
                      confirmLabel: tr("task.archive"),
                      danger: true,
                      action: async () => {
                        await api.del(`/api/spaces/${s.id}`).catch(() => {})
                        load()
                      },
                    })
                  }}>
                  <IconArchive size={14} />
                </button>
              </>
            )}
            {!editing && <span className="muted" style={{ fontSize: 16, lineHeight: 1 }}>›</span>}
          </div>
        )
      })}
      <form className="row" style={{ marginTop: 12 }} onSubmit={async (e) => {
        e.preventDefault()
        if (!name.trim()) return
        await api.post("/api/spaces", { name }).catch(() => {})
        setName(""); load()
      }}>
        <input className="input grow" placeholder={tr("spaces.new_placeholder")} value={name} onChange={(e) => setName(e.target.value)} />
        <button className="btn" type="submit">{tr("common.create")}</button>
      </form>
      {/* Import lands in a brand new space, so it belongs next to "create a space". */}
      <ImportCard onImported={load} />
      <button className="nav-btn row" style={{ gap: 5, marginTop: 14 }} onClick={() => setShowArchived((v) => !v)}>
        <IconArchive size={13} /> {tr("archive.show_archived_spaces")}
      </button>
      {showArchived && <div style={{ marginTop: 8 }}><ArchivedSpacesPanel me={me} /></div>}
    </div>
  )
}

// ---------- Space Pulse (spec section 17) ----------

// Signal rows are data-driven so a signal the owner disabled (absent from pulse.signals)
// simply doesn't render, rather than showing "undefined".
const PULSE_SIGNALS = [
  { key: "overdue", Icon: IconClock },
  { key: "unassigned", Icon: IconUser },
  { key: "stale", Icon: IconPause },
  { key: "blocked", Icon: IconSlash },
  { key: "no_deadline", Icon: IconCalendar },
] as const

function PulseCard({ pulse, spaceId, canEdit, onChanged }: {
  pulse: Pulse; spaceId: number; canEdit: boolean; onChanged: () => void
}) {
  const [editing, setEditing] = useState(false)
  const s = pulse.settings
  const standup = pulse.standup
  const standupEmpty = !standup ||
    (standup.did.length === 0 && standup.doing.length === 0 && standup.blocked.length === 0)

  return (
    <div className="card" style={{ marginBottom: 12 }}>
      <div className="pulse-card">
        <div className="pulse-visual">
          <span className={"pulse-dot pulse-dot--" + pulse.mood} title={tr("pulse.title")} />
          <span className="pulse-score-num">{pulse.score}</span>
        </div>
        <div className="grow">
          <div className="row" style={{ justifyContent: "space-between", gap: 8 }}>
            <div><b>{tr("pulse.title")}</b> · {tr("pulse.open")}: {pulse.open}/{pulse.total}</div>
            {canEdit && (
              <button className="nav-btn row" style={{ gap: 5, display: "inline-flex", flexShrink: 0 }}
                onClick={() => setEditing((v) => !v)}>
                <IconSliders size={13} /> {tr("pulse.settings")}
              </button>
            )}
          </div>
          <div style={{ marginTop: 4 }}>
            {PULSE_SIGNALS.map(({ key, Icon }) => {
              const n = pulse.signals[key]
              if (n === undefined) return null
              return (
                <span key={key} className="signal">
                  <Icon size={12} /> {tr("pulse." + key)}: {n}
                </span>
              )
            })}
          </div>
        </div>
      </div>

      {pulse.next_action && (
        <div className="pulse-next">
          <b>{tr("pulse.next_action")}</b>{" "}
          {tr("pulse.action." + pulse.next_action.kind).replace("{title}", pulse.next_action.title)}
        </div>
      )}

      {pulse.in_progress && pulse.in_progress.length > 0 && (
        <div style={{ marginTop: 10 }}>
          <div className="muted" style={{ fontSize: 12, marginBottom: 4 }}>{tr("pulse.in_progress")}</div>
          {pulse.in_progress.map((t) => (
            <div key={t.id} className="row" style={{ gap: 6, fontSize: 13, marginBottom: 2 }}>
              <span>{t.title}</span>
              {t.assignee && <span className="muted">· {t.assignee}</span>}
              {typeof t.progress === "number" && <span className="muted">· {t.progress}%</span>}
            </div>
          ))}
        </div>
      )}

      {standup && !standupEmpty && (
        <div style={{ marginTop: 10 }}>
          <div className="muted" style={{ fontSize: 12, marginBottom: 4 }}>{tr("pulse.standup")}</div>
          {([["did", standup.did], ["doing", standup.doing], ["blocked", standup.blocked]] as const).map(
            ([k, items]) => items.length > 0 && (
              <div key={k} style={{ fontSize: 13, marginBottom: 2 }}>
                <span className="muted">{tr("pulse.standup_" + k)}:</span>{" "}
                {items.map((t) => t.title).join(", ")}
              </div>
            ),
          )}
        </div>
      )}

      {editing && s && (
        <PulseSettingsForm spaceId={spaceId} settings={s} signals={pulse.signals}
          onSaved={() => { setEditing(false); onChanged() }} />
      )}
    </div>
  )
}

function PulseSettingsForm({ spaceId, settings, signals, onSaved }: {
  spaceId: number
  settings: NonNullable<Pulse["settings"]>
  signals: Pulse["signals"]
  onSaved: () => void
}) {
  const [staleDays, setStaleDays] = useState(String(settings.stale_days))
  const [greenAt, setGreenAt] = useState(String(settings.green_at))
  const [yellowAt, setYellowAt] = useState(String(settings.yellow_at))
  const [standup, setStandup] = useState(settings.standup)
  // A signal missing from the response means the owner turned it off.
  const [on, setOn] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(PULSE_SIGNALS.map(({ key }) => [key, signals[key] !== undefined])))
  const [err, setErr] = useState("")

  async function save() {
    setErr("")
    try {
      // PATCH replaces the whole settings object, so send pulse nested under it.
      await api.patch(`/api/spaces/${spaceId}`, {
        settings: {
          pulse: {
            stale_days: Number(staleDays) || settings.stale_days,
            green_at: Number(greenAt),
            yellow_at: Number(yellowAt),
            standup,
            signals: on,
          },
        },
      })
      onSaved()
    } catch (e) {
      setErr((e as Error).message)
    }
  }

  return (
    <div style={{ marginTop: 12, borderTop: "1px solid var(--border)", paddingTop: 12 }}>
      <div className="row" style={{ gap: 12, flexWrap: "wrap", marginBottom: 10 }}>
        <label className="row" style={{ gap: 6, fontSize: 13 }}>
          {tr("pulse.stale_days")}
          <input className="input" type="number" min={1} max={365} style={{ width: 80 }}
            value={staleDays} onChange={(e) => setStaleDays(e.target.value)} />
        </label>
        <label className="row" style={{ gap: 6, fontSize: 13 }}>
          {tr("pulse.green_at")}
          <input className="input" type="number" min={0} max={100} style={{ width: 80 }}
            value={greenAt} onChange={(e) => setGreenAt(e.target.value)} />
        </label>
        <label className="row" style={{ gap: 6, fontSize: 13 }}>
          {tr("pulse.yellow_at")}
          <input className="input" type="number" min={0} max={100} style={{ width: 80 }}
            value={yellowAt} onChange={(e) => setYellowAt(e.target.value)} />
        </label>
      </div>
      <div className="muted" style={{ fontSize: 12, marginBottom: 4 }}>{tr("pulse.signals")}</div>
      <div className="row" style={{ gap: 14, flexWrap: "wrap", marginBottom: 10 }}>
        {PULSE_SIGNALS.map(({ key }) => (
          <label key={key} className="row" style={{ gap: 6, fontSize: 13 }}>
            <input type="checkbox" checked={!!on[key]}
              onChange={(e) => setOn((p) => ({ ...p, [key]: e.target.checked }))} />
            {tr("pulse." + key)}
          </label>
        ))}
      </div>
      <label className="row" style={{ gap: 6, fontSize: 13, marginBottom: 10 }}>
        <input type="checkbox" checked={standup} onChange={(e) => setStandup(e.target.checked)} />
        {tr("pulse.standup")}
      </label>
      {err && <div style={{ color: "var(--due-overdue)", fontSize: 13, marginBottom: 8 }}>{err}</div>}
      <button className="btn" onClick={save}>{tr("common.save")}</button>
    </div>
  )
}

function SpaceView({ me, space, onBack }: { me: Me; space: Space; onBack: () => void }) {
  const [lists, setLists] = useState<List[]>([])
  const [pulse, setPulse] = useState<Pulse | null>(null)
  const [currentList, setCurrentList] = useState<List | null>(null)
  const [name, setName] = useState("")
  const [tab, setTab] = useState<"lists" | "timeline" | "workload" | "notes" | "activity" | "archive" | "fields" | "workflow">("lists")
  const [templates, setTemplates] = useState<Array<{ id: number; name: string }>>([])
  // Count vs. weight is a per-viewer display preference (spec section 6), so it lives in
  // localStorage rather than on the list — two people can read the same space differently.
  const [progressMode, setProgressMode] = useState<"count" | "weight">(
    () => (localStorage.getItem("todorio.progress_mode") === "weight" ? "weight" : "count"))
  // The Timeline tab only knows a task's id (it's plotting bars, not full task objects), so
  // opening one from a bar click fetches it the same way openMentionedTask does elsewhere.
  const [open, setOpen] = useState<Task | null>(null)
  // List row actions: rename (inline edit) and duplicate (optionally into another space, which
  // doubles as "copy list elsewhere" since the duplicate endpoint already accepts a target
  // space_id — no separate clipboard concept needed).
  const [allSpaces, setAllSpaces] = useState<Space[]>([])
  const [renamingListId, setRenamingListId] = useState<number | null>(null)
  const [renameValue, setRenameValue] = useState("")
  const [duplicatingListId, setDuplicatingListId] = useState<number | null>(null)
  const [dupName, setDupName] = useState("")
  const [dupTargetSpace, setDupTargetSpace] = useState<number>(space.id)
  const { confirm, confirmElement } = useConfirm()

  const load = () => {
    api.get(`/api/spaces/${space.id}/lists`).then((r) => setLists(r.lists)).catch(() => {})
    api.get(`/api/spaces/${space.id}/pulse`).then(setPulse).catch(() => {})
  }
  useEffect(() => { load() }, [space.id])
  useEffect(() => { api.get("/api/templates").then((r) => setTemplates(r.templates)).catch(() => {}) }, [])
  useEffect(() => { api.get("/api/spaces").then((r) => setAllSpaces(r.spaces)).catch(() => {}) }, [])

  async function openTaskById(id: number) {
    const r = await api.get(`/api/tasks/${id}`).catch(() => null)
    if (r?.task) setOpen(r.task)
  }

  async function renameList(id: number) {
    const val = renameValue.trim()
    setRenamingListId(null)
    if (!val) return
    await api.patch(`/api/lists/${id}`, { name: val }).catch(() => {})
    load()
  }

  function startDuplicate(l: List) {
    setDuplicatingListId(l.id)
    setDupName(l.name)
    setDupTargetSpace(space.id)
  }

  async function confirmDuplicate(id: number) {
    await api.post(`/api/lists/${id}/duplicate`, {
      space_id: dupTargetSpace,
      name: dupName.trim() || undefined,
    }).catch(() => {})
    setDuplicatingListId(null)
    load()
  }

  if (currentList) return <ListView me={me} list={currentList} spaceId={space.id} onBack={() => { setCurrentList(null); load() }} />

  async function applyTemplate(templateId: number) {
    if (!templateId) return
    await api.post(`/api/templates/${templateId}/apply`, { space_id: space.id }).catch(() => {})
    load()
  }

  return (
    <div>
      {confirmElement}
      <div className="row" style={{ marginBottom: 12 }}>
        <button className="nav-btn row" style={{ gap: 4, display: "inline-flex" }} onClick={onBack}><IconArrowLeft size={14} /> {tr("common.back")}</button>
        <h2 style={{ margin: 0 }}>{space.name}</h2>
      </div>

      <StatsCard spaceId={space.id} canEdit={space.my_role === "owner" || me.role === "root" || me.role === "admin"} />
      {pulse && pulse.enabled !== false && (
        <PulseCard pulse={pulse} spaceId={space.id} canEdit={space.my_role === "owner" || me.role === "root" || me.role === "admin"}
          onChanged={load} />
      )}

      <div className="row tab-strip" style={{ marginBottom: 8, gap: 4 }}>
        <button className={"nav-btn row" + (tab === "lists" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("lists")}><IconList size={14} /> {tr("lists.title")}</button>
        <button className={"nav-btn row" + (tab === "timeline" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("timeline")}><IconActivity size={14} /> {tr("view.timeline")}</button>
        <button className={"nav-btn row" + (tab === "workload" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("workload")}><IconBarChart size={14} /> {tr("workload.title")}</button>
        <button className={"nav-btn row" + (tab === "notes" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("notes")}><IconFileText size={14} /> {tr("notes.title")}</button>
        <button className={"nav-btn row" + (tab === "activity" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("activity")}><IconActivity size={14} /> {tr("activity.title")}</button>
        <button className={"nav-btn row" + (tab === "archive" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("archive")}><IconArchive size={14} /> {tr("archive.title")}</button>
        <button className={"nav-btn row" + (tab === "fields" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("fields")}><IconSliders size={14} /> {tr("fields.title")}</button>
        <button className={"nav-btn row" + (tab === "workflow" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("workflow")}><IconColumns size={14} /> {tr("workflow.title")}</button>
      </div>

      {tab === "lists" && (
        <div className="card">
          <div className="row" style={{ justifyContent: "flex-end", marginBottom: 6 }}>
            <label className="row muted" style={{ gap: 6, fontSize: 12 }}>
              {tr("lists.progress_mode")}
              <select className="input" style={{ width: "auto", padding: "2px 6px", fontSize: 12 }}
                value={progressMode} onChange={(e) => {
                  const m = e.target.value as "count" | "weight"
                  setProgressMode(m)
                  localStorage.setItem("todorio.progress_mode", m)
                }}>
                <option value="count">{tr("lists.progress_by_count")}</option>
                <option value="weight">{tr("lists.progress_by_weight")}</option>
              </select>
            </label>
          </div>
          {lists.length === 0 && <p className="muted">{tr("spaces.lists_empty")}</p>}
          {lists.map((l) => {
            // Fall back to plain counts when the server didn't send weighted totals.
            const byWeight = progressMode === "weight" && l.weight_total !== undefined
            const done = byWeight ? (l.weight_done ?? 0) : l.done_count
            const total = byWeight ? (l.weight_total ?? 0) : l.task_count
            const canManageList = l.my_permission === "owner"
            const editing = renamingListId === l.id
            const duplicating = duplicatingListId === l.id
            return (
              <div key={l.id} className="task-row" style={duplicating ? { flexWrap: "wrap" } : undefined}
                onClick={() => !editing && !duplicating && setCurrentList(l)}>
                {editing ? (
                  <form className="row grow" style={{ gap: 6 }} onClick={(e) => e.stopPropagation()}
                    onSubmit={(e) => { e.preventDefault(); renameList(l.id) }}>
                    <input className="input grow" autoFocus value={renameValue} onChange={(e) => setRenameValue(e.target.value)} />
                    <button className="btn" type="submit">{tr("common.save")}</button>
                    <button className="nav-btn" type="button" onClick={() => setRenamingListId(null)}>{tr("confirm.cancel")}</button>
                  </form>
                ) : (
                  <>
                    <span className="task-title row" style={{ gap: 6 }}>{l.is_private ? <IconLock size={14} /> : <IconList size={14} />} {l.name}</span>
                    <span className="muted">{done}/{total}</span>
                    <progress className="progress" max={total || 1} value={done} />
                  </>
                )}
                {!editing && (
                  <>
                    {canManageList && (
                      <button className="nav-btn" style={{ padding: "2px 6px" }} title={tr("action.rename")}
                        onClick={(e) => { e.stopPropagation(); setRenamingListId(l.id); setRenameValue(l.name) }}>
                        <IconEdit size={14} />
                      </button>
                    )}
                    <button className="nav-btn" style={{ padding: "2px 6px" }} title={tr("action.duplicate")}
                      onClick={(e) => { e.stopPropagation(); startDuplicate(l) }}>
                      <IconCopy size={14} />
                    </button>
                    {canManageList && (
                      <button className="nav-btn" style={{ padding: "2px 6px", color: "var(--due-overdue)" }} title={tr("task.archive")}
                        onClick={(e) => {
                          e.stopPropagation()
                          confirm({
                            title: tr("lists.archive_confirm").replace("{name}", l.name),
                            body: tr("confirm.archive_body"),
                            confirmLabel: tr("task.archive"), danger: true,
                            action: async () => { await api.del(`/api/lists/${l.id}`).catch(() => {}); load() },
                          })
                        }}>
                        <IconArchive size={14} />
                      </button>
                    )}
                    <span className="muted" style={{ fontSize: 16, lineHeight: 1 }}>›</span>
                  </>
                )}
                {duplicating && (
                  <div className="row" style={{ gap: 6, width: "100%", marginTop: 6 }} onClick={(e) => e.stopPropagation()}>
                    <input className="input grow" placeholder={tr("action.duplicate_name_placeholder")}
                      value={dupName} onChange={(e) => setDupName(e.target.value)} />
                    <select className="input" style={{ width: "auto" }} title={tr("action.duplicate_target_space")}
                      value={dupTargetSpace} onChange={(e) => setDupTargetSpace(Number(e.target.value))}>
                      {allSpaces.map((sp) => <option key={sp.id} value={sp.id}>{sp.name}</option>)}
                    </select>
                    <button className="btn" onClick={() => confirmDuplicate(l.id)}>{tr("action.duplicate_confirm")}</button>
                    <button className="nav-btn" onClick={() => setDuplicatingListId(null)}>{tr("confirm.cancel")}</button>
                  </div>
                )}
              </div>
            )
          })}
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
      {tab === "timeline" && <div className="card"><TimelineView spaceId={space.id} onOpenTask={openTaskById} /></div>}
      {tab === "workload" && <div className="card"><WorkloadPanel spaceId={space.id} /></div>}
      {tab === "notes" && <div className="card"><NotesPanel spaceId={space.id} /></div>}
      {tab === "activity" && <div className="card"><ActivityPanel spaceId={space.id} /></div>}
      {tab === "archive" && <div className="card"><ArchivePanel me={me} spaceId={space.id} /></div>}
      {tab === "fields" && <div className="card"><FieldsPanel spaceId={space.id} isOwner={space.my_role === "owner"} /></div>}
      {tab === "workflow" && <div className="card"><WorkflowEditor spaceId={space.id} isOwner={space.my_role === "owner"} /></div>}
      {open && <TaskModal task={open} me={me} spaceId={space.id} onClose={() => setOpen(null)} onChanged={load} />}
    </div>
  )
}

// ---------- Kanban board (drag cards between the space's workflow statuses) ----------

function KanbanBoard({ tasks, statuses, onOpen, onDrop, meId }: {
  tasks: Task[]; statuses: string[]; onOpen: (t: Task) => void; onDrop: (task: Task, status: string) => void; meId?: number
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
            <StatusChip status={s} />
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
                <FocusPresence active={t.active_focus} meId={meId} />
              </div>
            </div>
          ))}
        </div>
      ))}
    </div>
  )
}

// ---------- Table view ----------

function TableView({ tasks, onOpen, onToggle, meId }: {
  tasks: Task[]; onOpen: (t: Task) => void; onToggle: (t: Task) => void; meId?: number
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
            <td style={{ padding: "8px 6px", textDecoration: t.completed_at ? "line-through" : "none", opacity: t.completed_at ? 0.55 : 1 }}>
              <span className="row" style={{ gap: 6, display: "inline-flex" }}>{t.title} <FocusPresence active={t.active_focus} meId={meId} showLabel={false} /></span>
            </td>
            <td style={{ padding: "8px 6px" }}><StatusChip status={t.status} /></td>
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
  const [loadError, setLoadError] = useState("")
  const [createError, setCreateError] = useState("")
  // Bulk selection and the right-click menu exist because changing one field on one task used to
  // mean opening the modal — which also knocked the focus timer off screen.
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [menu, setMenu] = useState<{ task: Task; x: number; y: number } | null>(null)
  const { confirm, confirmElement } = useConfirm()

  function toggleSelect(t: Task) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(t.id)) next.delete(t.id)
      else next.add(t.id)
      return next
    })
  }

  // patchMany applies the same change to every selected task. Sent sequentially rather than in
  // one request: there is no bulk endpoint, and a partial failure this way still leaves the
  // successful ones applied instead of rolling everything back invisibly.
  async function patchMany(patch: Record<string, unknown>) {
    const ids = [...selected]
    let failed = 0
    for (const id of ids) {
      try { await api.patch(`/api/tasks/${id}`, patch) } catch { failed++ }
    }
    setSelected(new Set())
    if (failed > 0) setCreateError(tr("bulk.partial").replace("{n}", String(failed)))
    load()
  }

  async function archiveMany() {
    const ids = [...selected]
    for (const id of ids) {
      await api.del(`/api/tasks/${id}`).catch(() => {})
    }
    setSelected(new Set())
    window.dispatchEvent(new CustomEvent("todorio:focus-changed"))
    load()
  }

  // Both load() and the create-task submit below used to swallow errors entirely
  // (`.catch(() => {})`), so a failed request looked identical to "the list is just empty" or
  // "the task was created" — exactly the confusing silent-failure shape that made an earlier,
  // real bug (a missing DB column) look like "tasks aren't created" with zero diagnostic
  // information. Both now surface the actual error message instead of hiding it.
  const load = () => {
    setLoadError("")
    api.get(`/api/lists/${list.id}/tasks`).then((r) => setTasks(r.tasks)).catch((err) => setLoadError((err as Error).message))
  }
  useEffect(() => { load() }, [list.id])
  useEffect(() => {
    api.get(`/api/spaces/${spaceId}/workflow`).then((r: Workflow) => setStatuses(r.statuses)).catch(() => {})
  }, [spaceId])

  async function toggle(task: Task) {
    await api.patch(`/api/tasks/${task.id}`, { status: task.completed_at ? "open" : "done" }).catch(() => {})
    // Completing a task closes its focus session server-side; tell the sidebar timer to
    // re-read now instead of ticking on until its next poll.
    window.dispatchEvent(new CustomEvent("todorio:focus-changed"))
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
          <button className={"nav-btn row" + (viewMode === "calendar" ? " active" : "")} style