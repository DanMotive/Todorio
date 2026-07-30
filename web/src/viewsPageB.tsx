import { useEffect, useState } from "react"
import { api, DEFAULT_STATUSES, type Me, type Pulse, type Task } from "./api"
import { useConfirm } from "./extras"
import { tr, getFormattingLocale } from "./i18n"
import { IconAlertCircle, IconMessage, IconClock, IconUser, IconPause, IconSlash, IconCalendar, IconSliders } from "./icons"
import { dueClass, TaskRow } from "./taskui"

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

export function MyTasksPage({ me, onOpenTask }: { me: Me; onOpenTask: (task: Task) => void }) {
  const [tasks, setTasks] = useState<Task[]>([])
  const [tab, setTab] = useState<"list" | "week" | "stats">("list")
  const [subTab, setSubTab] = useState<MySubTab>("all")
  const [mentions, setMentions] = useState<any[]>([])
  const [favorites, setFavorites] = useState<Set<number>>(new Set())
  const [error, setError] = useState("")
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
    setError("")
    try {
      await api.patch(`/api/tasks/${task.id}`, { status: task.completed_at ? "open" : "done" })
      window.dispatchEvent(new CustomEvent("todorio:focus-changed"))
      load()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  async function toggleFavorite(task: Task) {
    setError("")
    try {
      await api.post("/api/favorites", { target_type: "task", target_id: task.id })
      loadFavorites()
    } catch (err) {
      setError((err as Error).message)
    }
  }

  async function openMentionedTask(taskID: number) {
    const r = await api.get(`/api/tasks/${taskID}`).catch(() => null)
    if (r?.task) onOpenTask(r.task)
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
      {error && <p className="error-text">{error}</p>}
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
                <TaskRow key={task.id} task={task} onToggle={toggle} onOpen={onOpenTask}
                  favorite={favorites.has(task.id)} onToggleFavorite={toggleFavorite} meId={me.id} />
              ))}
            </>
          )}
        </>
      )}
      {tab === "week" && (
        <MyWeekView tasks={tasks} favorites={favorites} onOpen={onOpenTask} onToggle={toggle} onToggleFavorite={toggleFavorite} meId={me.id} />
      )}
      {tab === "stats" && <MyStatsPanel />}
    </div>
  )
}

export const PULSE_SIGNALS = [
  { key: "overdue", Icon: IconClock },
  { key: "unassigned", Icon: IconUser },
  { key: "stale", Icon: IconPause },
  { key: "blocked", Icon: IconSlash },
  { key: "no_deadline", Icon: IconCalendar },
] as const

export function PulseCard({ pulse, spaceId, canEdit, onChanged }: {
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
  const [on, setOn] = useState<Record<string, boolean>>(() =>
    Object.fromEntries(PULSE_SIGNALS.map(({ key }) => [key, signals[key] !== undefined])))
  const [err, setErr] = useState("")

  async function save() {
    setErr("")
    try {
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
