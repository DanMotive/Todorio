import { useEffect, useState } from "react"
import { api, DEFAULT_STATUSES, type List, type Me, type Task, type Workflow } from "./api"
import { useConfirm } from "./extras"
import { tr, trFormal, getFormattingLocale } from "./i18n"
import { IconX, IconSliders, IconArrowLeft, IconMenu, IconColumns, IconTable, IconCalendar } from "./icons"
import { dueClass, dueLabel, StatusChip, TaskRow } from "./taskui"
import { FocusPresence } from "./extras"
import { TaskContextMenu, TaskModal } from "./viewsPageA"

export function KanbanBoard({ tasks, statuses, onOpen, onDrop, meId }: {
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

export function TableView({ tasks, onOpen, onToggle, meId }: {
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

export function CalendarView({ tasks, onOpen }: { tasks: Task[]; onOpen: (t: Task) => void }) {
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
  const startOffset = (new Date(year, month, 1).getDay() + 6) % 7
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

type FilterQuery = { status?: string; priority?: string; overdue?: boolean }
type SavedFilterT = { id: number; list_id: number | null; name: string; query: FilterQuery }

function matchesFilter(t: Task, q: FilterQuery): boolean {
  if (q.status && t.status !== q.status) return false
  if (q.priority && t.priority !== q.priority) return false
  if (q.overdue && !(t.due_at && !t.completed_at && new Date(t.due_at).getTime() < Date.now())) return false
  return true
}

export function FiltersBar({ listId, statuses, onFilter }: {
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

export function ListView({ me, list, spaceId, onBack }: { me: Me; list: List; spaceId: number; onBack: () => void }) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [open, setOpen] = useState<Task | null>(null)
  const [title, setTitle] = useState("")
  const [due, setDue] = useState("")
  const [viewMode, setViewMode] = useState<"list" | "kanban" | "table" | "calendar">("list")
  const [statuses, setStatuses] = useState<string[]>(DEFAULT_STATUSES)
  const [filterQuery, setFilterQuery] = useState<FilterQuery | null>(null)
  const [loadError, setLoadError] = useState("")
  const [createError, setCreateError] = useState("")
  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [menu, setMenu] = useState<{ task: Task; x: number; y: number } | null>(null)
  const [draggedTaskId, setDraggedTaskId] = useState<number | null>(null)
  const { confirm, confirmElement } = useConfirm()

  function toggleSelect(t: Task) {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(t.id)) next.delete(t.id)
      else next.add(t.id)
      return next
    })
  }

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
    window.dispatchEvent(new CustomEvent("todorio:focus-changed"))
    load()
  }

  async function moveToStatus(task: Task, status: string) {
    await api.patch(`/api/tasks/${task.id}`, { status }).catch(() => {})
    load()
  }

  // Drag-and-drop reordering of root-level tasks within this list (spec parity with the Kanban
  // board's status drag). Disabled while a filter is active, since filteredRoots would then be a
  // subset of roots and dropping at a visible index wouldn't map to the task's real position.
  // Subtasks keep their existing relative order under their parent; only the root tasks around
  // them are reshuffled.
  async function reorderTasks(draggedId: number, targetId: number) {
    if (draggedId === targetId) return
    const currentRoots = tasks.filter((t) => !t.parent_id)
    const draggedIdx = currentRoots.findIndex((t) => t.id === draggedId)
    const targetIdx = currentRoots.findIndex((t) => t.id === targetId)
    if (draggedIdx === -1 || targetIdx === -1) return
    const nextRoots = [...currentRoots]
    const [moved] = nextRoots.splice(draggedIdx, 1)
    nextRoots.splice(targetIdx, 0, moved)
    const reordered = nextRoots.flatMap((r) => [r, ...tasks.filter((s) => s.parent_id === r.id)])
    setTasks(reordered)
    await Promise.all(nextRoots.map((t, i) => api.patch(`/api/tasks/${t.id}`, { position: i }).catch(() => {})))
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

      {loadError && <p className="error-text">{tr("task.load_error")}: {loadError}</p>}
      {confirmElement}

      {selected.size > 0 && (
        <div className="bulk-bar">
          <b>{tr("bulk.selected").replace("{n}", String(selected.size))}</b>
          <select className="input" style={{ width: "auto" }} defaultValue=""
            onChange={(e) => { if (e.target.value) { patchMany({ status: e.target.value }); e.target.value = "" } }}>
            <option value="">{tr("task.status")}</option>
            {statuses.map((s) => (
              <option key={s} value={s}>{DEFAULT_STATUSES.includes(s) ? tr("task.status." + s) : s}</option>
            ))}
          </select>
          <select className="input" style={{ width: "auto" }} defaultValue=""
            onChange={(e) => { if (e.target.value) { patchMany({ priority: e.target.value }); e.target.value = "" } }}>
            <option value="">{tr("task.priority")}</option>
            <option value="low">{tr("task.priority.low")}</option>
            <option value="normal">{tr("task.priority.normal")}</option>
            <option value="high">{tr("task.priority.high")}</option>
            <option value="urgent">{tr("task.priority.urgent")}</option>
          </select>
          <input className="input" type="date" style={{ width: "auto" }}
            title={tr("task.due_at")}
            onChange={(e) => {
              if (e.target.value) patchMany({ due_at: new Date(e.target.value).toISOString() })
            }} />
          <button className="nav-btn" onClick={() => patchMany({ clear_due_at: true })}>
            {tr("bulk.clear_due")}
          </button>
          <button className="nav-btn" onClick={() => patchMany({ assignee_id: me.id })}>
            {tr("bulk.assign_me")}
          </button>
          <button className="nav-btn" style={{ color: "var(--due-overdue)" }}
            onClick={() => confirm({
              title: tr("bulk.confirm_archive_title").replace("{n}", String(selected.size)),
              body: tr("confirm.archive_body"),
              confirmLabel: tr("task.archive"), danger: true, action: archiveMany,
            })}>
            {tr("task.archive")}
          </button>
          <button className="nav-btn" style={{ marginLeft: "auto" }} onClick={() => setSelected(new Set())}>
            {tr("bulk.clear_selection")}
          </button>
        </div>
      )}

      {viewMode === "list" && filteredRoots.map((task) => (
        <div key={task.id}
          draggable={!filterQuery}
          style={{ opacity: draggedTaskId === task.id ? 0.4 : 1, cursor: filterQuery ? undefined : "grab" }}
          onDragStart={(e) => { setDraggedTaskId(task.id); e.dataTransfer.effectAllowed = "move" }}
          onDragOver={(e) => { if (draggedTaskId !== null) e.preventDefault() }}
          onDrop={(e) => {
            e.preventDefault()
            if (draggedTaskId !== null) reorderTasks(draggedTaskId, task.id)
            setDraggedTaskId(null)
          }}
          onDragEnd={() => setDraggedTaskId(null)}>
          <TaskRow task={task} onToggle={toggle} onOpen={setOpen} meId={me.id}
            selected={selected.has(task.id)} onSelect={toggleSelect}
            statuses={statuses} onStatus={moveToStatus}
            onContext={(t, x, y) => setMenu({ task: t, x, y })} />
          {tasks.filter((s) => s.parent_id === task.id).map((sub) => (
            <div key={sub.id} style={{ marginLeft: 28 }}>
              <TaskRow task={sub} onToggle={toggle} onOpen={setOpen} meId={me.id}
                selected={selected.has(sub.id)} onSelect={toggleSelect}
                statuses={statuses} onStatus={moveToStatus}
                onContext={(t, x, y) => setMenu({ task: t, x, y })} />
            </div>
          ))}
        </div>
      ))}
      {viewMode === "kanban" && <KanbanBoard tasks={filteredRoots} statuses={statuses} onOpen={setOpen} onDrop={moveToStatus} meId={me.id} />}
      {viewMode === "table" && <TableView tasks={filteredRoots} onOpen={setOpen} onToggle={toggle} meId={me.id} />}
      {viewMode === "calendar" && <CalendarView tasks={filteredRoots} onOpen={setOpen} />}

      <form className="row" style={{ marginTop: 12 }} onSubmit={async (e) => {
        e.preventDefault()
        if (!title.trim()) return
        setCreateError("")
        try {
          await api.post(`/api/lists/${list.id}/tasks`, {
            title, due_at: due ? new Date(due).toISOString() : null,
            parse: true,
          })
          setTitle(""); setDue("")
          load()
        } catch (err) {
          setCreateError((err as Error).message)
        }
      }}>
        <input className="input grow" placeholder={tr("task.new_placeholder")} value={title} onChange={(e) => setTitle(e.target.value)} />
        <input className="input" style={{ width: 170 }} type="date" value={due} onChange={(e) => setDue(e.target.value)} />
        <button className="btn" type="submit">+</button>
      </form>
      <div className="muted" style={{ fontSize: 12, marginTop: 4 }}>{tr("task.quickadd_hint")}</div>
      {createError && <p className="error-text">{createError}</p>}
      {menu && (
        <TaskContextMenu task={menu.task} x={menu.x} y={menu.y} statuses={statuses} meId={me.id}
          onClose={() => setMenu(null)}
          onPatch={async (patch) => {
            await api.patch(`/api/tasks/${menu.task.id}`, patch).catch((e) => setCreateError((e as Error).message))
            if (patch.status === "done") window.dispatchEvent(new CustomEvent("todorio:focus-changed"))
            setMenu(null)
            load()
          }}
          onOpenFull={() => { setOpen(menu.task); setMenu(null) }} />
      )}
      {open && <TaskModal task={open} me={me} spaceId={spaceId} onClose={() => setOpen(null)} onChanged={load} />}
    </div>
  )
}

const KIND_ICON: Record<string, string> = {
  mention: "💬", assignment: "📌", due_soon: "⏰", overdue: "⚠️", comment: "💬",
  review_submitted: "👀", review_returned: "↩️", digest: "📬",
}

type NotificationItem = {
  id: number
  kind: string
  text: string
  task_id?: number
  read_at: string | null
  created_at: string
  payload: Record<string, unknown>
}

export function NotificationsPage({ onNavigateTask }: { onNavigateTask: (taskId: number) => void }) {
  const [items, setItems] = useState<NotificationItem[]>([])
  const load = () => api.get("/api/notifications").then((r) => setItems(r.notifications)).catch(() => {})
  useEffect(() => { load() }, [])

  async function markRead(n: NotificationItem) {
    if (!n.read_at) await api.post("/api/notifications/read", { ids: [n.id] }).catch(() => {})
    if (n.task_id) onNavigateTask(n.task_id)
    load()
  }

  async function markAllRead() {
    await api.post("/api/notifications/read").catch(() => {})
    load()
  }

  return (
    <div className="card">
      <div className="row" style={{ marginBottom: 8 }}>
        <h2 className="grow" style={{ margin: 0 }}>{tr("notifications.title")}</h2>
        <button className="nav-btn" onClick={markAllRead}>{tr("notifications.mark_all_read")}</button>
      </div>
      {items.length === 0 && <p className="muted">{tr("notifications.empty")}</p>}
      {items.map((n) => (
        <div key={n.id} className="task-row" style={{ opacity: n.read_at ? 0.6 : 1, cursor: "pointer" }}
          onClick={() => markRead(n)}>
          <span style={{ marginRight: 8 }}>{KIND_ICON[n.kind] || "🔔"}</span>
          <span className="grow">{n.text}</span>
          <span className="muted">{new Date(n.created_at).toLocaleString(getFormattingLocale())}</span>
        </div>
      ))}
    </div>
  )
}

export function AdminPage({ me }: { me: Me }) {
  const [users, setUsers] = useState<any[]>([])
  const [invites, setInvites] = useState<any[]>([])
  const [settings, setSettings] = useState<any>(null)
  const [announcement, setAnnouncement] = useState("")
  const [tab, setTab] = useState<"users" | "invites" | "settings" | "announcements">("users")

  const load = () => {
    api.get("/api/admin/users").then((r) => setUsers(r.users)).catch(() => {})
    api.get("/api/admin/invites").then((r) => setInvites(r.invites)).catch(() => {})
    api.get("/api/admin/settings").then(setSettings).catch(() => {})
  }
  useEffect(() => { load() }, [])

  async function approve(id: number) {
    await api.post(`/api/admin/users/${id}/approve`).catch(() => {})
    load()
  }

  async function setRole(id: number, role: string) {
    await api.patch(`/api/admin/users/${id}`, { role }).catch(() => {})
    load()
  }

  async function createInvite() {
    await api.post("/api/admin/invites", {}).catch(() => {})
    load()
  }

  async function saveSettings() {
    await api.patch("/api/admin/settings", settings).catch(() => {})
    load()
  }

  async function postAnnouncement() {
    if (!announcement.trim()) return
    await api.post("/api/announcements", { text: announcement }).catch(() => {})
    setAnnouncement("")
  }

  return (
    <div className="card">
      <h2>{trFormal("admin.title")}</h2>
      <div className="row" style={{ gap: 4, marginBottom: 10 }}>
        <button className={"nav-btn" + (tab === "users" ? " active" : "")} onClick={() => setTab("users")}>{trFormal("admin.users")}</button>
        <button className={"nav-btn" + (tab === "invites" ? " active" : "")} onClick={() => setTab("invites")}>{trFormal("admin.invites")}</button>
        <button className={"nav-btn" + (tab === "settings" ? " active" : "")} onClick={() => setTab("settings")}>{trFormal("admin.settings")}</button>
        <button className={"nav-btn" + (tab === "announcements" ? " active" : "")} onClick={() => setTab("announcements")}>{trFormal("admin.announcements")}</button>
      </div>
      {tab === "users" && users.map((u) => (
        <div key={u.id} className="task-row">
          <span className="grow">@{u.username} {!u.approved && <span className="badge">{trFormal("admin.pending")}</span>}</span>
          {!u.approved && <button className="nav-btn" onClick={() => approve(u.id)}>{trFormal("admin.approve")}</button>}
          <select className="input" style={{ width: "auto" }} value={u.role} onChange={(e) => setRole(u.id, e.target.value)}>
            <option value="member">{trFormal("admin.role_member")}</option>
            <option value="admin">{trFormal("admin.role_admin")}</option>
            <option value="root">{trFormal("admin.role_root")}</option>
          </select>
        </div>
      ))}
      {tab === "invites" && (
        <div>
          <button className="btn" onClick={createInvite}>{trFormal("admin.create_invite")}</button>
          {invites.map((i) => (
            <div key={i.id} className="task-row">
              <code className="grow">{i.code}</code>
              <span className="muted">{i.used_by ? trFormal("admin.invite_used") : trFormal("admin.invite_unused")}</span>
            </div>
          ))}
        </div>
      )}
      {tab === "settings" && settings && (
        <div>
          <label className="row" style={{ gap: 6, marginBottom: 8 }}>
            <input type="checkbox" checked={settings.require_approval}
              onChange={(e) => setSettings({ ...settings, require_approval: e.target.checked })} />
            {trFormal("admin.require_approval")}
          </label>
          <button className="btn" onClick={saveSettings}>{tr("common.save")}</button>
        </div>
      )}
      {tab === "announcements" && (
        <div>
          <textarea className="input" style={{ width: "100%", minHeight: 80 }} value={announcement}
            onChange={(e) => setAnnouncement(e.target.value)} placeholder={trFormal("admin.announcement_placeholder")} />
          <button className="btn" style={{ marginTop: 8 }} onClick={postAnnouncement}>{trFormal("admin.post_announcement")}</button>
        </div>
      )}
    </div>
  )
}
