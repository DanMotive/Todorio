import { useEffect, useState } from "react"
import { api, type List, type Me, type Pulse, type Space } from "./api"
import { useConfirm, ArchivedSpacesPanel, StatsCard, NotesPanel, ActivityPanel, ArchivePanel, FieldsPanel } from "./extras"
import { ImportCard, WorkloadPanel } from "./functional"
import { tr } from "./i18n"
import { TimelineView } from "./timeline"
import { DashboardPanel, dashboardTabLabel } from "./dashboard"
import { WorkflowEditor } from "./workflow"
import { IconGrid, IconArchive, IconEdit, IconCopy, IconArrowLeft, IconList, IconFileText, IconActivity, IconColumns, IconLock, IconSliders, IconBarChart } from "./icons"
import { ListView } from "./viewsPageC"
import { PulseCard } from "./viewsPageB"
import { pushRoute, type AppRoute } from "./router"

export { AuthPage, PendingPage, TaskModal, TaskContextMenu, TaskHistorySection } from "./viewsPageA"
export { MyTasksPage } from "./viewsPageB"
export { KanbanBoard, TableView, CalendarView, FiltersBar, ListView, NotificationsPage } from "./viewsPageC"

export function SpacesPage({ me, route, onOpenTask, onOpenNote }: {
  me: Me
  route?: AppRoute
  onOpenTask: (taskId: number) => void
  onOpenNote: (noteId: number) => void
}) {
  const [spaces, setSpaces] = useState<Space[]>([])
  const [current, setCurrent] = useState<Space | null>(null)
  const [name, setName] = useState("")
  const [showArchived, setShowArchived] = useState(false)
  const [error, setError] = useState("")
  const [renamingId, setRenamingId] = useState<number | null>(null)
  const [renameValue, setRenameValue] = useState("")
  const { confirm, confirmElement } = useConfirm()
  const load = () => {
    setError("")
    return api.get("/api/spaces").then((r) => {
      const next: Space[] = r.spaces
      setSpaces(next)
      const routeSpaceId = route?.kind === "space" || route?.kind === "list" ? route.spaceId : null
      setCurrent(routeSpaceId ? next.find((s) => s.id === routeSpaceId) || null : null)
      if (routeSpaceId && !next.some((s) => s.id === routeSpaceId)) setError(tr("search.empty"))
    }).catch((err) => setError((err as Error).message))
  }
  useEffect(() => { load() }, [route?.kind, route?.kind === "space" || route?.kind === "list" ? route.spaceId : 0])

  if (current) return <SpaceView me={me} space={current} route={route} onOpenTask={onOpenTask} onOpenNote={onOpenNote}
    onBack={() => { pushRoute({ kind: "view", view: "spaces" }); setCurrent(null); load() }} />

  const canManage = (s: Space) => s.my_role === "owner" || me.role === "root" || me.role === "admin"

  async function renameSpace(id: number) {
    const val = renameValue.trim()
    setRenamingId(null)
    if (!val) return
    setError("")
    try {
      await api.patch(`/api/spaces/${id}`, { name: val })
      load()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  async function duplicateSpace(id: number) {
    setError("")
    try {
      await api.post(`/api/spaces/${id}/duplicate`, {})
      load()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  return (
    <div className="card">
      {confirmElement}
      <h2>{tr("spaces.title")}</h2>
      {error && <p className="error-text">{error}</p>}
      {spaces.length === 0 && !error && <p className="muted">{tr("spaces.empty")}</p>}
      {spaces.map((s) => {
        const editing = renamingId === s.id
        return (
          <div key={s.id} className="task-row" onClick={() => {
            if (!editing) { setCurrent(s); pushRoute({ kind: "space", spaceId: s.id }) }
          }}>
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
                        await api.del(`/api/spaces/${s.id}`)
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
        setError("")
        try {
          await api.post("/api/spaces", { name })
          setName("")
          load()
        } catch (err) {
          setError((err as Error).message)
        }
      }}>
        <input className="input grow" placeholder={tr("spaces.new_placeholder")} value={name} onChange={(e) => setName(e.target.value)} />
        <button className="btn" type="submit">{tr("common.create")}</button>
      </form>
      <ImportCard onImported={load} />
      <button className="nav-btn row" style={{ gap: 5, marginTop: 14 }} onClick={() => setShowArchived((v) => !v)}>
        <IconArchive size={13} /> {tr("archive.show_archived_spaces")}
      </button>
      {showArchived && <div style={{ marginTop: 8 }}><ArchivedSpacesPanel me={me} /></div>}
    </div>
  )
}

function SpaceView({ me, space, route, onBack, onOpenTask, onOpenNote }: {
  me: Me
  space: Space
  route?: AppRoute
  onBack: () => void
  onOpenTask: (taskId: number) => void
  onOpenNote: (noteId: number) => void
}) {
  const [lists, setLists] = useState<List[]>([])
  const [pulse, setPulse] = useState<Pulse | null>(null)
  const [error, setError] = useState("")
  const [currentList, setCurrentList] = useState<List | null>(null)
  useEffect(() => {
    if (route?.kind !== "list") { setCurrentList(null); return }
    const next = lists.find((l) => l.id === route.listId) || null
    setCurrentList(next)
    if (lists.length > 0 && !next) setError(tr("search.empty"))
  }, [route?.kind, route?.kind === "list" ? route.listId : 0, lists])
  const [name, setName] = useState("")
  const [newIsPrivate, setNewIsPrivate] = useState(false)
  const [tab, setTab] = useState<"lists" | "dashboard" | "timeline" | "workload" | "notes" | "activity" | "archive" | "fields" | "workflow">("lists")
  const [templates, setTemplates] = useState<Array<{ id: number; name: string }>>([])
  const [progressMode, setProgressMode] = useState<"count" | "weight">(
    () => (localStorage.getItem("todorio.progress_mode") === "weight" ? "weight" : "count"))
  const [allSpaces, setAllSpaces] = useState<Space[]>([])
  const [renamingListId, setRenamingListId] = useState<number | null>(null)
  const [renameValue, setRenameValue] = useState("")
  const [duplicatingListId, setDuplicatingListId] = useState<number | null>(null)
  const [dupName, setDupName] = useState("")
  const [dupTargetSpace, setDupTargetSpace] = useState<number>(space.id)
  const [draggedListId, setDraggedListId] = useState<number | null>(null)
  const { confirm, confirmElement } = useConfirm()

  const load = () => {
    setError("")
    api.get(`/api/spaces/${space.id}/lists`).then((r) => setLists(r.lists)).catch((err) => setError((err as Error).message))
    api.get(`/api/spaces/${space.id}/pulse`).then(setPulse).catch(() => {})
  }
  useEffect(() => { load() }, [space.id])
  useEffect(() => { api.get("/api/templates").then((r) => setTemplates(r.templates)).catch(() => {}) }, [])
  useEffect(() => { api.get("/api/spaces").then((r) => setAllSpaces(r.spaces)).catch(() => {}) }, [])

  async function renameList(id: number) {
    const val = renameValue.trim()
    setRenamingListId(null)
    if (!val) return
    setError("")
    try {
      await api.patch(`/api/lists/${id}`, { name: val })
      load()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  function startDuplicate(l: List) {
    setDuplicatingListId(l.id)
    setDupName(l.name)
    setDupTargetSpace(space.id)
  }

  async function confirmDuplicate(id: number) {
    setError("")
    try {
      await api.post(`/api/lists/${id}/duplicate`, {
        space_id: dupTargetSpace,
        name: dupName.trim() || undefined,
      })
      setDuplicatingListId(null)
      load()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  // Drag-and-drop reordering of lists within the space. Position updates are sent for every
  // list in the new order (not just the two swapped), matching how the backend simply stores a
  // linear "position" column per list (see handleUpdateList) rather than a gap-based ordering
  // scheme. A PATCH the current user isn't the owner of will just 403 and is ignored — the same
  // best-effort pattern already used for bulk task edits elsewhere in this file.
  async function reorderLists(draggedId: number, targetId: number) {
    if (draggedId === targetId) return
    const draggedIdx = lists.findIndex((l) => l.id === draggedId)
    const targetIdx = lists.findIndex((l) => l.id === targetId)
    if (draggedIdx === -1 || targetIdx === -1) return
    const next = [...lists]
    const [moved] = next.splice(draggedIdx, 1)
    next.splice(targetIdx, 0, moved)
    setLists(next)
    setError("")
    try {
      await Promise.all(next.map((l, i) => api.patch(`/api/lists/${l.id}`, { position: i })))
    } catch (err) {
      setError((err as Error).message)
    }
    load()
  }

  if (currentList) return <ListView me={me} list={currentList} spaceId={space.id} onOpenTask={(task) => onOpenTask(task.id)} onBack={() => {
    pushRoute({ kind: "space", spaceId: space.id }); setCurrentList(null); load()
  }} />

  async function applyTemplate(templateId: number) {
    if (!templateId) return
    setError("")
    try {
      await api.post(`/api/templates/${templateId}/apply`, { space_id: space.id })
      load()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  return (
    <div>
      {confirmElement}
      <div className="row" style={{ marginBottom: 12 }}>
        <button className="nav-btn row" style={{ gap: 4, display: "inline-flex" }} onClick={onBack}><IconArrowLeft size={14} /> {tr("common.back")}</button>
        <h2 style={{ margin: 0 }}>{space.name}</h2>
      </div>

      {error && <p className="error-text">{error}</p>}
      <StatsCard spaceId={space.id} canEdit={space.my_role === "owner" || me.role === "root" || me.role === "admin"} />
      {pulse && pulse.enabled !== false && (
        <PulseCard pulse={pulse} spaceId={space.id} canEdit={space.my_role === "owner" || me.role === "root" || me.role === "admin"}
          onChanged={load} />
      )}

      <div className="row tab-strip" style={{ marginBottom: 8, gap: 4 }}>
        <button className={"nav-btn row" + (tab === "lists" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("lists")}><IconList size={14} /> {tr("lists.title")}</button>
        <button className={"nav-btn row" + (tab === "dashboard" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("dashboard")}><IconGrid size={14} /> {dashboardTabLabel()}</button>
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
            const byWeight = progressMode === "weight" && l.weight_total !== undefined
            const done = byWeight ? (l.weight_done ?? 0) : l.done_count
            const total = byWeight ? (l.weight_total ?? 0) : l.task_count
            const canManageList = l.my_permission === "owner"
            const editing = renamingListId === l.id
            const duplicating = duplicatingListId === l.id
            return (
              <div key={l.id} className="task-row"
                draggable={!editing && !duplicating}
                style={{
                  ...(duplicating ? { flexWrap: "wrap" as const } : {}),
                  opacity: draggedListId === l.id ? 0.4 : 1,
                  cursor: editing || duplicating ? undefined : "grab",
                }}
                onClick={() => {
                  if (!editing && !duplicating) {
                    setCurrentList(l)
                    pushRoute({ kind: "list", spaceId: space.id, listId: l.id })
                  }
                }}
                onDragStart={(e) => { e.stopPropagation(); setDraggedListId(l.id); e.dataTransfer.effectAllowed = "move" }}
                onDragOver={(e) => { if (draggedListId !== null) e.preventDefault() }}
                onDrop={(e) => {
                  e.preventDefault()
                  e.stopPropagation()
                  if (draggedListId !== null) reorderLists(draggedListId, l.id)
                  setDraggedListId(null)
                }}
                onDragEnd={() => setDraggedListId(null)}>
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
                            action: async () => { await api.del(`/api/lists/${l.id}`); load() },
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
          <form className="row" style={{ marginTop: 12, flexWrap: "wrap" }} onSubmit={async (e) => {
            e.preventDefault()
            if (!name.trim()) return
            try {
              await api.post(`/api/spaces/${space.id}/lists`, { name, is_private: newIsPrivate })
              setName(""); setNewIsPrivate(false); load()
            } catch (err) {
              setError((err as Error).message)
            }
          }}>
            <input className="input grow" placeholder={tr("lists.new_placeholder")} value={name} onChange={(e) => setName(e.target.value)} />
            <label className="row muted" style={{ gap: 4, fontSize: 12 }}>
              <input type="checkbox" checked={newIsPrivate} onChange={(e) => setNewIsPrivate(e.target.checked)} />
              {tr("lists.private_checkbox")}
            </label>
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
      {tab === "dashboard" && <div className="card"><DashboardPanel spaceId={space.id} onOpenTask={onOpenTask} /></div>}
      {tab === "timeline" && <div className="card"><TimelineView spaceId={space.id} onOpenTask={onOpenTask} /></div>}
      {tab === "workload" && <div className="card"><WorkloadPanel spaceId={space.id} /></div>}
      {tab === "notes" && <div className="card"><NotesPanel spaceId={space.id} onOpenNote={(note) => onOpenNote(note.id)} /></div>}
      {tab === "activity" && <div className="card"><ActivityPanel spaceId={space.id} /></div>}
      {tab === "archive" && <div className="card"><ArchivePanel me={me} spaceId={space.id} /></div>}
      {tab === "fields" && <div className="card"><FieldsPanel spaceId={space.id} isOwner={space.my_role === "owner"} /></div>}
      {tab === "workflow" && <div className="card"><WorkflowEditor spaceId={space.id} isOwner={space.my_role === "owner"} /></div>}
    </div>
  )
}
