import { useEffect, useState } from "react"
import { createPortal } from "react-dom"
import { api, REACTIONS, DEFAULT_STATUSES, type Me, type Task, type Workflow, type ActiveFocus } from "./api"
import { AttachmentsBlock, ModalShell, useConfirm, FocusWidget, FocusPresence, type FieldDef } from "./extras"
import { tr, setLocale, getLocale, getFormattingLocale, SUPPORTED } from "./i18n"
import { AssigneePicker } from "./members"
import { IconClock, IconUser, IconLock, IconX, IconCalendar, IconRefresh } from "./icons"
import { endOfDayISO, formatSystemComment } from "./taskui"

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

export function TaskContextMenu({ task, x, y, statuses, meId, onClose, onPatch, onOpenFull }: {
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

export function TaskHistorySection({ taskId, onRestored }: { taskId: number; onRestored: () => void }) {
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

  const [status, setStatus] = useState(task.status || "open")
  const [priority, setPriority] = useState(task.priority || "normal")
  const [progress, setProgress] = useState<number | null>(
    typeof task.progress === "number" ? task.progress : null)
  const [weight, setWeight] = useState(task.weight ?? 1)
  const [assigneeId, setAssigneeId] = useState<number | null>(task.assignee_id)
  const dateInputValue = (v: string | null) => {
    if (!v) return ""
    const d = new Date(v)
    const m = String(d.getMonth() + 1).padStart(2, "0")
    const day = String(d.getDate()).padStart(2, "0")
    return `${d.getFullYear()}-${m}-${day}`
  }
  const [startAt, setStartAt] = useState(() => dateInputValue(task.start_at))
  const [dueAt, setDueAt] = useState(() => dateInputValue(task.due_at))
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
  const [subtasks, setSubtasks] = useState<Task[]>([])
  const [newSubtaskTitle, setNewSubtaskTitle] = useState("")

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
    api.get(`/api/spaces/${spaceId}/fields`).then((r) => setFieldSchema(r.fields || [])).catch(() => {})
  }, [spaceId])

  const loadSubtasks = () =>
    api.get(`/api/lists/${task.list_id}/tasks`)
      .then((r) => setSubtasks((r.tasks || []).filter((s: Task) => s.parent_id === task.id)))
      .catch(() => {})
  useEffect(() => { loadSubtasks() }, [task.id, task.list_id])

  async function addSubtask(e: React.FormEvent) {
    e.preventDefault()
    const title = newSubtaskTitle.trim()
    if (!title) return
    await api.post(`/api/lists/${task.list_id}/tasks`, { title, parent_id: task.id }).catch(() => {})
    setNewSubtaskTitle("")
    loadSubtasks()
    onChanged()
  }

  async function toggleSubtask(sub: Task) {
    const nextStatus = sub.status === "done" ? "open" : "done"
    await api.patch(`/api/tasks/${sub.id}`, { status: nextStatus }).catch(() => {})
    loadSubtasks()
    onChanged()
  }

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

        <div style={{ marginBottom: 16 }}>
          <div className="section-title" style={{ fontSize: 13, marginBottom: 6 }}>{tr("task.subtasks")}</div>
          {subtasks.length === 0 && <p className="muted" style={{ fontSize: 13 }}>{tr("task.subtasks_empty")}</p>}
          {subtasks.map((s) => (
            <label key={s.id} className="row" style={{ gap: 8, marginBottom: 4, fontSize: 13 }}>
              <input type="checkbox" checked={s.status === "done"} onChange={() => toggleSubtask(s)} />
              <span style={{ textDecoration: s.status === "done" ? "line-through" : undefined }}>{s.title}</span>
            </label>
          ))}
          <form className="row" style={{ marginTop: 6 }} onSubmit={addSubtask}>
            <input className="input grow" placeholder={tr("task.add_subtask_placeholder")} value={newSubtaskTitle}
              onChange={(e) => setNewSubtaskTitle(e.target.value)} />
            <button className="btn secondary" type="submit">{tr("common.create")}</button>
          </form>
        </div>

        <TaskHistorySection taskId={task.id} onRestored={onChanged} />

        <FocusWidget taskId={task.id} />
        <FocusPresence active={activeFocus} meId={me.id} />

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
