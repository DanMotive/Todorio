// Task presentation primitives, split out of views.tsx.
//
// Everything here is a leaf: due-date formatting, the status colour table and
// chip, the single task row, and the focus start/stop button. They are imported
// by views.tsx (and by anything else that renders a task), and they import
// nothing from views.tsx — keep it that way, or the two files start requiring
// each other.

import { useEffect, useRef, useState } from "react"
import { api, DEFAULT_STATUSES, type Task } from "./api"
import { FocusWidget, FocusPresence } from "./extras"
import { tr, getFormattingLocale } from "./i18n"
import { IconStar, IconPause, IconPlay } from "./icons"

// endOfDayISO returns an ISO timestamp for 23:59 local time, N days from today. Built from local
// date parts rather than from a UTC offset, so "today" means the user's today.
export function endOfDayISO(daysFromNow: number): string {
  const d = new Date()
  d.setDate(d.getDate() + daysFromNow)
  d.setHours(23, 59, 0, 0)
  return d.toISOString()
}

export function dueClass(due: string | null): string {
  if (!due) return ""
  const d = new Date(due).getTime() - Date.now()
  if (d < 0) return "overdue"
  if (d < 24 * 3600e3) return "today"
  if (d < 3 * 24 * 3600e3) return "soon"
  return "later"
}

export function dueLabel(due: string | null): string {
  if (!due) return ""
  return new Date(due).toLocaleDateString(getFormattingLocale(), { day: "numeric", month: "short" })
}

// formatSystemComment renders an is_system comment's small structured JSON body (written by
// insertSystemComment on the backend) as localized text, rather than trusting a frozen sentence
// in whatever language the editor happened to be using when the change happened.
export function formatSystemComment(body: string): string {
  try {
    const data = JSON.parse(body)
    const statusLabel = (s: string) => (DEFAULT_STATUSES.includes(s) ? tr("task.status." + s) : s)
    switch (data.type) {
      case "status_changed":
        return tr("task.system.status_changed").replace("{from}", statusLabel(data.from)).replace("{to}", statusLabel(data.to))
      case "due_changed":
        return tr("task.system.due_changed")
      case "assignee_changed":
        return tr("task.system.assignee_changed")
      default:
        return ""
    }
  } catch {
    return ""
  }
}

// Colours for the four built-in statuses. A space can define its own workflow, so anything
// unknown gets one of the spare hues, chosen from the name: the same custom status then always
// looks the same everywhere without storing a colour anywhere.
export const STATUS_VARS: Record<string, string> = {
  open: "var(--st-open)",
  in_progress: "var(--st-progress)",
  review: "var(--st-review)",
  done: "var(--st-done)",
}

export const STATUS_ALT = ["var(--st-alt1)", "var(--st-alt2)", "var(--st-alt3)", "var(--st-alt4)"]

export function statusColor(s: string) {
  if (STATUS_VARS[s]) return STATUS_VARS[s]
  let h = 0
  for (let i = 0; i < s.length; i++) h = (h * 31 + s.charCodeAt(i)) >>> 0
  return STATUS_ALT[h % STATUS_ALT.length]
}

export const statusText = (s: string) => (DEFAULT_STATUSES.includes(s) ? tr("task.status." + s) : s)

// A coloured pill that also switches the status in one click, so a routine change no longer
// requires the right-click menu or the full task modal. Without `statuses`/`onPick` it stays a
// read-only label - that is how the kanban header and the table use it.
export function StatusChip({ status, statuses, onPick }: {
  status: string
  statuses?: string[]
  onPick?: (s: string) => void
}) {
  const [open, setOpen] = useState(false)
  const wrap = useRef<HTMLSpanElement>(null)
  useEffect(() => {
    if (!open) return
    const away = (e: MouseEvent) => {
      if (wrap.current && !wrap.current.contains(e.target as Node)) setOpen(false)
    }
    const esc = (e: KeyboardEvent) => { if (e.key === "Escape") setOpen(false) }
    document.addEventListener("mousedown", away)
    document.addEventListener("keydown", esc)
    return () => {
      document.removeEventListener("mousedown", away)
      document.removeEventListener("keydown", esc)
    }
  }, [open])
  const color = statusColor(status)
  const list = statuses || []
  const interactive = !!onPick && list.length > 0
  return (
    // The row underneath opens the task on click, so every click inside the chip stops here.
    <span className="status-wrap" ref={wrap} onClick={(e) => e.stopPropagation()}>
      <span className={"status-chip" + (interactive ? " clickable" : "")}
        style={{ color, borderColor: color, background: `color-mix(in srgb, ${color} 16%, transparent)` }}
        title={statusText(status)}
        role={interactive ? "button" : undefined}
        tabIndex={interactive ? 0 : undefined}
        onClick={interactive ? () => setOpen((v) => !v) : undefined}
        onKeyDown={interactive ? (e) => {
          if (e.key === "Enter" || e.key === " ") { e.preventDefault(); setOpen((v) => !v) }
        } : undefined}>
        {statusText(status)}
      </span>
      {open && interactive && (
        <span className="status-pop">
          {list.map((s) => (
            <button key={s} type="button" className={"status-opt" + (s === status ? " current" : "")}
              onClick={() => { setOpen(false); if (s !== status) onPick!(s) }}>
              <span className="status-dot" style={{ background: statusColor(s) }} />
              {statusText(s)}
            </button>
          ))}
        </span>
      )}
    </span>
  )
}

// Starting a focus session used to require opening the task and using FocusWidget. This is the
// same action as an icon next to the title.
//
// Whether a session is running is taken from task.active_focus, which the list already loads, so
// no extra request per row. The click result is kept locally until the list reloads, and the
// usual custom event tells the sidebar clock to re-read right away.
export function FocusButton({ task, meId }: { task: Task; meId?: number }) {
  const mine = !!meId && (task.active_focus || []).some((f) => f.user_id === meId)
  const [running, setRunning] = useState(mine)
  const [busy, setBusy] = useState(false)
  useEffect(() => { setRunning(mine) }, [mine])

  async function toggle(e: React.MouseEvent) {
    e.stopPropagation() // the row itself opens the task
    setBusy(true)
    try {
      if (running) await api.post("/api/focus/stop")
      else await api.post("/api/focus/start", { task_id: task.id })
      // Only flip the icon after the server accepted the action. Previously a failed request
      // still changed the local UI, leaving it out of sync with the sidebar timer.
      setRunning(!running)
      window.dispatchEvent(new CustomEvent("todorio:focus-changed"))
    } finally {
      setBusy(false)
    }
  }

  const label = running ? tr("focus.stop") : tr("focus.start")
  return (
    <button className="nav-btn" style={{ padding: "2px 6px", color: running ? "var(--accent)" : undefined }}
      title={label} aria-label={label} disabled={busy} onClick={toggle}>
      {running ? <IconPause size={13} /> : <IconPlay size={13} />}
    </button>
  )
}

export function TaskRow({ task, onToggle, onOpen, favorite, onToggleFavorite, meId,
  selected, onSelect, onContext, statuses, onStatus }: {
  task: Task; onToggle: (t: Task) => void; onOpen: (t: Task) => void
  favorite?: boolean; onToggleFavorite?: (t: Task) => void; meId?: number
  // Bulk selection and the right-click menu are optional: only the list view wires them up, so
  // "My tasks" and the other views keep their simpler row.
  selected?: boolean
  onSelect?: (t: Task, additive: boolean) => void
  onContext?: (t: Task, x: number, y: number) => void
  // Same idea for the inline status switcher: views that don't know the space workflow show the
  // status as a plain coloured label.
  statuses?: string[]
  onStatus?: (t: Task, s: string) => void
}) {
  const done = !!task.completed_at
  return (
    <div className={"task-row" + (done ? " done" : "") + (selected ? " selected" : "")}
      onClick={() => onOpen(task)}
      onContextMenu={onContext ? (e) => { e.preventDefault(); onContext(task, e.clientX, e.clientY) } : undefined}>
      {onSelect && (
        <input type="checkbox" className="task-select" checked={!!selected} title={tr("bulk.select")}
          onClick={(e) => e.stopPropagation()}
          onChange={(e) => onSelect(task, (e.nativeEvent as MouseEvent).shiftKey)} />
      )}
      <input type="checkbox" checked={done} onClick={(e) => e.stopPropagation()} onChange={() => onToggle(task)} />
      <span className="task-title">{task.title}</span>
      <FocusButton task={task} meId={meId} />
      <StatusChip status={task.status} statuses={statuses}
        onPick={onStatus ? (s) => onStatus(task, s) : undefined} />
      {task.subtasks_total > 0 && (
        <span className="muted">{task.subtasks_done}/{task.subtasks_total}</span>
      )}
      <FocusPresence active={task.active_focus} meId={meId} />
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
