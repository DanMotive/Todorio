import { useEffect, useState } from "react"
import { api, type List, type Me, type Space } from "./api"
import { useConfirm, ImportCard, ArchivedSpacesPanel, StatsCard, WorkloadPanel, NotesPanel, ActivityPanel, ArchivePanel, FieldsPanel } from "./extras"
import { tr } from "./i18n"
import { TimelineView } from "./timeline"
import { WorkflowEditor } from "./workflow"
import { IconGrid, IconArchive, IconEdit, IconCopy, IconArrowLeft, IconList, IconFileText, IconActivity, IconColumns, IconLock, IconSliders, IconBarChart } from "./icons"
import { ListView, PulseCard } from "./viewsPageC"

export { AuthPage, PendingPage, TaskModal, TaskContextMenu, TaskHistorySection } from "./viewsPageA"
export { MyTasksPage } from "./viewsPageB"
export { KanbanBoard, TableView, CalendarView, FiltersBar, ListView, NotificationsPage, AdminPage } from "./viewsPageC"

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
      <ImportCard onImported={load} />
      <button className="nav-btn row" style={{ gap: 5, marginTop: 14 }} onClick={() => setShowArchived((v) => !v)}>
        <IconArchive size={13} /> {tr("archive.show_archived_spaces")}
      </button>
      {showArchived && <div style={{ marginTop: 8 }}><ArchivedSpacesPanel me={me} /></div>}
    </div>
  )
}

function SpaceView({ me, space, onBack }: { me: Me; space: Space; onBack: () => void }) {
  const [lists, setLists] = useState<List[]>([])
  const [pulse, setPulse] = useState<any | null>(null)
  const [currentList, setCurrentList] = useState<List | null>(null)
  const [name, setName] = useState("")
  const [tab, setTab] = useState<"lists" | "timeline" | "workload" | "notes" | "activity" | "archive" | "fields" | "workflow">("lists")
  const [templates, setTemplates] = useState<Array<{ id: number; name: string }>>([])
  const [progressMode, setProgressMode] = useState<"count" | "weight">(
    () => (localStorage.getItem("todorio.progress_mode") === "weight" ? "weight" : "count"))
  const [open, setOpen] = useState<any | null>(null)
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
      {open && <TaskModalRef task={open} me={me} spaceId={space.id} onClose={() => setOpen(null)} onChanged={load} />}
    </div>
  )
}

import { TaskModal as TaskModalRef } from "./viewsPageA"
