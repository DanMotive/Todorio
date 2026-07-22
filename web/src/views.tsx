// Todorio screens: login, "My tasks", spaces, tasks, notifications, admin panel.
import { useEffect, useState } from "react"
import { api, REACTIONS, type List, type Me, type Pulse, type Space, type Task } from "./api"
import { AttachmentsBlock, StatsCard } from "./extras"
import { tr, setLocale, getLocale, SUPPORTED } from "./i18n"

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
  return new Date(due).toLocaleDateString(undefined, { day: "numeric", month: "short" })
}

// ---------- login / registration ----------

export function AuthPage({ siteName, onLogin }: { siteName: string; onLogin: (me: Me) => void }) {
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
          {SUPPORTED.map((l) => <option key={l} value={l}>{l}</option>)}
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
        <div style={{ fontSize: 40 }}>⏳</div>
        <h2>{tr("pending.title")}</h2>
        <p className="muted">{tr("pending.text")}</p>
        <button className="btn" onClick={onLogout}>{tr("nav.logout")}</button>
      </div>
    </div>
  )
}

// ---------- tasks ----------

function TaskRow({ task, onToggle, onOpen }: { task: Task; onToggle: (t: Task) => void; onOpen: (t: Task) => void }) {
  const done = !!task.completed_at
  return (
    <div className={"task-row" + (done ? " done" : "")} onClick={() => onOpen(task)}>
      <input type="checkbox" checked={done} onClick={(e) => e.stopPropagation()} onChange={() => onToggle(task)} />
      <span className="task-title">{task.title}</span>
      {task.subtasks_total > 0 && (
        <span className="muted">{task.subtasks_done}/{task.subtasks_total}</span>
      )}
      {task.due_at && <span className={"due " + dueClass(task.due_at)}>{dueLabel(task.due_at)}</span>}
    </div>
  )
}

export function TaskModal({ task, me, onClose, onChanged }: {
  task: Task; me: Me; onClose: () => void; onChanged: () => void
}) {
  const [comments, setComments] = useState<any[]>([])
  const [body, setBody] = useState("")
  const [error, setError] = useState("")

  const load = () => api.get(`/api/tasks/${task.id}/comments`).then((r) => setComments(r.comments)).catch(() => {})
  useEffect(() => { load() }, [task.id])

  async function send(e: React.FormEvent) {
    e.preventDefault()
    if (!body.trim()) return
    try {
      await api.post(`/api/tasks/${task.id}/comments`, { body })
      setBody("")
      load()
    } catch (err) { setError((err as Error).message) }
  }

  async function react(commentId: number, emoji: string) {
    await api.post("/api/reactions", { target_type: "comment", target_id: commentId, emoji }).catch(() => {})
    load()
  }

  return (
    <div className="modal-backdrop" onClick={onClose}>
      <div className="modal" onClick={(e) => e.stopPropagation()}>
        <div className="row">
          <h2 className="grow" style={{ margin: 0 }}>{task.title}</h2>
          <button className="nav-btn" onClick={onClose}>✕</button>
        </div>
        {task.description && <p>{task.description}</p>}
        <div className="row muted">
          {task.due_at && <span className={"due " + dueClass(task.due_at)}>{dueLabel(task.due_at)}</span>}
          <span>{tr("task.priority")}: {task.priority}</span>
          <span>{tr("task.status")}: {task.status}</span>
        </div>

        <div className="section-title">{tr("task.comments")}</div>
        <AttachmentsBlock taskId={task.id} />
        {comments.map((c) => (
          <div key={c.id} className="card" style={{ marginBottom: 8, padding: 12 }}>
            <div className="row">
              <b>@{c.author}</b>
              <span className="muted">{new Date(c.created_at).toLocaleString()}</span>
            </div>
            <div>{c.body}</div>
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
          <button className="btn" type="submit">↵</button>
        </form>
        <div className="error-text">{error}</div>

        <div className="row" style={{ marginTop: 12 }}>
          <button className="nav-btn" onClick={async () => { await api.del(`/api/tasks/${task.id}`); onChanged(); onClose() }}>
            🗑 {tr("task.archive")}
          </button>
        </div>
      </div>
    </div>
  )
}

// ---------- "My tasks" ----------

export function MyTasksPage({ me }: { me: Me }) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [open, setOpen] = useState<Task | null>(null)
  const load = () => api.get("/api/my/tasks").then((r) => setTasks(r.tasks)).catch(() => {})
  useEffect(() => { load() }, [])

  async function toggle(task: Task) {
    await api.patch(`/api/tasks/${task.id}`, { status: task.completed_at ? "open" : "done" }).catch(() => {})
    load()
  }

  return (
    <div className="card">
      <h2>{tr("my.title")}</h2>
      {tasks.length === 0 && <p className="muted">{tr("my.empty")}</p>}
      {tasks.map((task) => <TaskRow key={task.id} task={task} onToggle={toggle} onOpen={setOpen} />)}
      {open && <TaskModal task={open} me={me} onClose={() => setOpen(null)} onChanged={load} />}
    </div>
  )
}

// ---------- spaces ----------

export function SpacesPage({ me }: { me: Me }) {
  const [spaces, setSpaces] = useState<Space[]>([])
  const [current, setCurrent] = useState<Space | null>(null)
  const [name, setName] = useState("")
  const load = () => api.get("/api/spaces").then((r) => setSpaces(r.spaces)).catch(() => {})
  useEffect(() => { load() }, [])

  if (current) return <SpaceView me={me} space={current} onBack={() => { setCurrent(null); load() }} />

  return (
    <div className="card">
      <h2>{tr("spaces.title")}</h2>
      {spaces.map((s) => (
        <div key={s.id} className="task-row" onClick={() => setCurrent(s)}>
          <span className="task-title">🌌 {s.name}</span>
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
    </div>
  )
}

function SpaceView({ me, space, onBack }: { me: Me; space: Space; onBack: () => void }) {
  const [lists, setLists] = useState<List[]>([])
  const [pulse, setPulse] = useState<Pulse | null>(null)
  const [currentList, setCurrentList] = useState<List | null>(null)
  const [name, setName] = useState("")

  const load = () => {
    api.get(`/api/spaces/${space.id}/lists`).then((r) => setLists(r.lists)).catch(() => {})
    api.get(`/api/spaces/${space.id}/pulse`).then(setPulse).catch(() => {})
  }
  useEffect(() => { load() }, [space.id])

  if (currentList) return <ListView me={me} list={currentList} onBack={() => { setCurrentList(null); load() }} />

  return (
    <div>
      <div className="row" style={{ marginBottom: 12 }}>
        <button className="nav-btn" onClick={onBack}>← {tr("common.back")}</button>
        <h2 style={{ margin: 0 }}>{space.name}</h2>
      </div>

      <StatsCard spaceId={space.id} />
      {pulse && (
        <div className="card pulse-card" style={{ marginBottom: 12 }}>
          <div className="pulse-score">{pulse.mood} {pulse.score}</div>
          <div>
            <div><b>{tr("pulse.title")}</b> · {tr("pulse.open")}: {pulse.open}/{pulse.total}</div>
            <div style={{ marginTop: 4 }}>
              <span className="signal">⏰ {tr("pulse.overdue")}: {pulse.signals.overdue}</span>
              <span className="signal">👤 {tr("pulse.unassigned")}: {pulse.signals.unassigned}</span>
              <span className="signal">🧊 {tr("pulse.stale")}: {pulse.signals.stale}</span>
              <span className="signal">⛔ {tr("pulse.blocked")}: {pulse.signals.blocked}</span>
            </div>
          </div>
        </div>
      )}

      <div className="card">
        <h3>{tr("lists.title")}</h3>
        {lists.map((l) => (
          <div key={l.id} className="task-row" onClick={() => setCurrentList(l)}>
            <span className="task-title">{l.is_private ? "🔒" : "📋"} {l.name}</span>
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
      </div>
    </div>
  )
}

function ListView({ me, list, onBack }: { me: Me; list: List; onBack: () => void }) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [open, setOpen] = useState<Task | null>(null)
  const [title, setTitle] = useState("")
  const [due, setDue] = useState("")

  const load = () => api.get(`/api/lists/${list.id}/tasks`).then((r) => setTasks(r.tasks)).catch(() => {})
  useEffect(() => { load() }, [list.id])

  async function toggle(task: Task) {
    await api.patch(`/api/tasks/${task.id}`, { status: task.completed_at ? "open" : "done" }).catch(() => {})
    load()
  }

  const roots = tasks.filter((t) => !t.parent_id)

  return (
    <div className="card">
      <div className="row" style={{ marginBottom: 8 }}>
        <button className="nav-btn" onClick={onBack}>← {tr("common.back")}</button>
        <h2 style={{ margin: 0 }}>{list.name}</h2>
      </div>
      {roots.map((task) => (
        <div key={task.id}>
          <TaskRow task={task} onToggle={toggle} onOpen={setOpen} />
          {tasks.filter((s) => s.parent_id === task.id).map((sub) => (
            <div key={sub.id} style={{ marginLeft: 28 }}>
              <TaskRow task={sub} onToggle={toggle} onOpen={setOpen} />
            </div>
          ))}
        </div>
      ))}
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
      {open && <TaskModal task={open} me={me} onClose={() => setOpen(null)} onChanged={load} />}
    </div>
  )
}

// ---------- notifications ----------

const KIND_EMOJI: Record<string, string> = {
  approved: "✅", task_assigned: "📌", comment: "💬", reaction: "✨",
  overdue: "⏰", space_added: "🌌", list_shared: "📋",
}
const kindLabel = (kind: string) =>
  KIND_EMOJI[kind] ? `${KIND_EMOJI[kind]} ${tr("notif.kind." + kind)}` : kind

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
          <span className="task-title">
            {kindLabel(n.kind)}
            {n.payload?.title ? ` · «${n.payload.title}»` : ""}
            {n.payload?.task_title ? ` · «${n.payload.task_title}»` : ""}
            {n.payload?.by ? ` · ${tr("notif.by")} @${n.payload.by}` : ""}
            {n.payload?.emoji ? ` ${n.payload.emoji}` : ""}
          </span>
          <span className="muted">{new Date(n.created_at).toLocaleString()}</span>
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
      <h2>{tr("admin.users")}</h2>
      {tempPass && (
        <div className="card" style={{ borderColor: "var(--accent)", marginBottom: 12 }}>
          {tr("admin.temp_pass_for")} <b>@{tempPass.user}</b>: <code>{tempPass.pass}</code>
          <div className="muted">{tr("admin.shown_once")}</div>
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
                {tr("admin.approve")}
              </button>
              <button className="nav-btn" onClick={async () => { await api.post(`/api/admin/users/${u.id}/status`, { status: "rejected" }); load() }}>
                {tr("admin.reject")}
              </button>
            </>
          )}
          {u.status === "active" && u.role !== "root" && (
            <>
              <button className="nav-btn" onClick={async () => { await api.post(`/api/admin/users/${u.id}/status`, { status: "blocked" }); load() }}>
                {tr("admin.block")}
              </button>
              <button className="nav-btn" onClick={async () => {
                const r = await api.post(`/api/admin/users/${u.id}/reset-password`)
                setTempPass({ user: u.username, pass: r.temp_password })
              }}>
                {tr("admin.reset_password")}
              </button>
            </>
          )}
          {u.status === "blocked" && (
            <button className="btn" onClick={async () => { await api.post(`/api/admin/users/${u.id}/status`, { status: "active" }); load() }}>
              {tr("admin.unblock")}
            </button>
          )}
        </div>
      ))}
    </div>
  )
}
