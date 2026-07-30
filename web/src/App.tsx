import { useEffect, useRef, useState } from "react"
import { api, DEVELOPER_NAME, type Me, type Profile, type Task, type Note } from "./api"
import "./theme.css"
import "./ui.css"
import { AuthPage, MyTasksPage, NotificationsPage, PendingPage, SpacesPage, TaskModal } from "./views"
import { AboutPage, AnnouncementsBanner, GlobalFocusTimer, InboxPage, ModalShell, DigestModal, InvitesCard, SearchPage, NoteModal, ServerSettingsCard, TemplatesAdminCard, AnnouncementsAdminCard } from "./extras"
import { MembersPage } from "./members"
import { AuditLogCard, FocusStatsCard } from "./insights"
import { parseRoute, pushRoute, replaceRoute, routeView, type AppRoute, type MainView } from "./router"
import { Avatar, SettingsPage, ForcedPasswordChange } from "./settings"
import { detectLocale, setLocale, tr, trOr } from "./i18n"
import { IconInbox, IconKeyboard, IconMenu, IconSliders } from "./icons"

type Bootstrap = {
  site_name: string
  browser_title: string
  developer_url?: string
  footer_text?: string
  show_product_name?: boolean
  about_text?: string
  logo_path?: string
  source_url?: string
  donate_url?: string
  version?: string
  default_locale: string
  locales_enabled?: string[]
  theme: { color: string; visual: string }
}

const COLORS = ["red", "blue", "green", "yellow", "gray"] as const

function applyTheme(color: string, visual: string) {
  const el = document.documentElement
  el.dataset.color = color
  el.dataset.visual = visual
}

// System browser notifications (spec section 12): "работает в открытой вкладке ... системные
// push-уведомления браузера — только при HTTPS". Read literally, this is the Notification API
// fired from an already-open tab, NOT full Web Push.
//
// That distinction is deliberate, not a shortcut: real Web Push delivers through the browser
// vendor's own relay (Google's FCM for Chrome, Mozilla's autopush, ...) even when the tab is
// closed — which means routing every notification through a third party. That directly
// contradicts the product's first stated principle ("приватность: все данные на своём сервере,
// без внешних сервисов"). The spec's own wording ("in an open tab") matches the lighter
// mechanism, so that's what this is: zero external services, works the moment HTTPS + permission
// are granted, and stops the moment the tab is closed — which is an honest trade-off to state,
// not a hidden one.
function notifyBrowser(raw: string, onOpenTask?: (taskId: number) => void) {
  if (typeof Notification === "undefined") return
  if (Notification.permission !== "granted") return
  // Only when the tab genuinely isn't the one the user is looking at — a notification while
  // they're already staring at the bell icon would be redundant.
  if (document.visibilityState === "visible") return
  let kind = "", payload: any = {}
  try { const d = JSON.parse(raw); kind = d.kind; payload = d.payload || {} } catch { return }
  const title = trOr("profile.type." + kind, tr("nav.notifications"))
  const body = payload.title || payload.task_title || (payload.by ? "@" + payload.by : "")
  try {
    const n = new Notification(title, { body, tag: "todorio-" + kind, icon: "/icons/icon-192.png" })
    n.onclick = () => {
      window.focus()
      const taskId = Number(payload.task_id)
      if (Number.isSafeInteger(taskId) && taskId > 0) onOpenTask?.(taskId)
      n.close()
    }
  } catch { /* some browsers reject Notification() outside a user gesture context; ignore */ }
}

function beep() {
  try {
    const ctx = new AudioContext()
    const osc = ctx.createOscillator()
    const gain = ctx.createGain()
    osc.connect(gain)
    gain.connect(ctx.destination)
    osc.frequency.value = 880
    gain.gain.setValueAtTime(0.08, ctx.currentTime)
    gain.gain.exponentialRampToValueAtTime(0.0001, ctx.currentTime + 0.25)
    osc.start()
    osc.stop(ctx.currentTime + 0.25)
  } catch { /* sound unavailable */ }
}

export default function App() {
  const [boot, setBoot] = useState<Bootstrap | null>(null)
  const [me, setMe] = useState<Me | null>(null)
  const [profile, setProfile] = useState<Profile | null>(null)
  const [loaded, setLoaded] = useState(false)
  const [route, setRoute] = useState<AppRoute>(() => parseRoute())
  const view = routeView(route)
  function setView(next: MainView) { pushRoute({ kind: "view", view: next }) }
  useEffect(() => {
    const onPop = () => setRoute(parseRoute())
    window.addEventListener("popstate", onPop)
    // Canonicalise the old root URL without adding a useless history entry.
    if (!window.location.pathname.startsWith("/app/")) replaceRoute(route)
    return () => window.removeEventListener("popstate", onPop)
  }, [])
  const [unread, setUnread] = useState(0)
  const [soundOn, setSoundOn] = useState(localStorage.getItem("todorio.sound") === "1")
  const soundOnRef = useRef(soundOn)
  useEffect(() => { soundOnRef.current = soundOn }, [soundOn])
  const [showHelp, setShowHelp] = useState(false)
  // Mobile drawer. Only meaningful under the 860px breakpoint; on desktop the sidebar is
  // always visible and this state is inert.
  const [navOpen, setNavOpen] = useState(false)
  // Whether the mobile layout is active, matching the CSS breakpoint. Needed because a closed
  // off-canvas drawer must be removed from the tab order, but the same element on desktop is
  // the permanent nav and must stay reachable.
  const [isNarrow, setIsNarrow] = useState(
    () => typeof window.matchMedia === "function" && window.matchMedia("(max-width: 860px)").matches)
  useEffect(() => {
    if (typeof window.matchMedia !== "function") return
    const mq = window.matchMedia("(max-width: 860px)")
    const onChange = () => { setIsNarrow(mq.matches); if (!mq.matches) setNavOpen(false) }
    mq.addEventListener("change", onChange)
    return () => mq.removeEventListener("change", onChange)
  }, [])
  const esRef = useRef<EventSource | null>(null)

  // Global search opens a task/note in-place regardless of which space/list it belongs to.
  // The search results only carry id/title-ish fields, so fetch the full record here (this is
  // the one place that already has `me` and imports both TaskModal and NoteModal) before
  // rendering the same modals every other page uses.
  const [searchTask, setSearchTask] = useState<Task | null>(null)
  const [searchNote, setSearchNote] = useState<Note | null>(null)
  const [routeError, setRouteError] = useState("")
  function openSearchTask(taskId: number) {
    const background = route.kind === "task" || route.kind === "note" ? route.background : route
    pushRoute({ kind: "task", taskId, background })
  }
  function openSearchNote(noteId: number) {
    const background = route.kind === "task" || route.kind === "note" ? route.background : route
    pushRoute({ kind: "note", noteId, background })
  }
  useEffect(() => {
    function onOpenRef(e: Event) {
      const detail = (e as CustomEvent).detail as { kind?: string; id?: number } | undefined
      if (!detail || !Number.isSafeInteger(detail.id) || (detail.id ?? 0) <= 0) return
      if (detail.kind === "task") openSearchTask(detail.id!)
      if (detail.kind === "note") openSearchNote(detail.id!)
    }
    window.addEventListener("todorio:open-ref", onOpenRef as EventListener)
    return () => window.removeEventListener("todorio:open-ref", onOpenRef as EventListener)
  }, [route])
  useEffect(() => {
    let alive = true
    setRouteError("")
    setSearchTask(null)
    setSearchNote(null)
    if (route.kind === "task") {
      api.get(`/api/tasks/${route.taskId}`).then((r) => {
        if (alive && r?.task) setSearchTask(r.task)
      }).catch((err) => { if (alive) setRouteError((err as Error).message) })
    } else if (route.kind === "note") {
      api.get(`/api/notes/${route.noteId}`).then((r) => {
        if (alive && r?.note) setSearchNote(r.note)
      }).catch((err) => { if (alive) setRouteError((err as Error).message) })
    }
    return () => { alive = false }
  }, [route.kind, route.kind === "task" ? route.taskId : 0, route.kind === "note" ? route.noteId : 0])
  function closeRouteModal() {
    if (route.kind !== "task" && route.kind !== "note") return
    if (route.background) {
      window.history.back()
      return
    }
    replaceRoute({ kind: "view", view: "my" })
  }

  // theme: server default <- personal override (localStorage + profile)
  // A theme cached by an older build still carries a `scheme` key. Strip it on read rather
  // than spreading it back into state and re-persisting a field the product no longer has.
  const savedTheme = (() => {
    try {
      const raw = JSON.parse(localStorage.getItem("todorio.theme") || "null")
      if (!raw || typeof raw !== "object") return null
      const colors = new Set(["red", "blue", "green", "yellow", "gray"])
      const visuals = new Set(["rich", "lite"])
      return {
        color: colors.has(raw.color) ? raw.color : "blue",
        visual: visuals.has(raw.visual) ? raw.visual : "rich",
      }
    } catch {
      localStorage.removeItem("todorio.theme")
      return null
    }
  })()
  const [theme, setTheme] = useState<{ color: string; visual: string }>(
    savedTheme || { color: "blue", visual: "rich" },
  )

  useEffect(() => {
    api.get("/api/bootstrap").then((b: Bootstrap) => {
      const nav = detectLocale()
      setLocale(nav !== "en-US" ? nav : (b.default_locale || "en-US"))
      setBoot(b)
      document.title = b.browser_title || b.site_name
      if (!savedTheme) setTheme(b.theme)
    }).catch(() => {})
    api.get("/api/me")
      .then((r) => {
        setMe(r.user)
        setUnread(r.unread_notifications)
        const p: Profile | undefined = r.profile
        setProfile(p ?? null)
        // The profile is the primary source for locale/theme (spec section 9: "1. язык из
        // профиля (главный)") — it overrides both the bootstrap default and any localStorage
        // cache from before login, so a user's own settings follow them to a new device.
        if (p?.locale) setLocale(p.locale)
        if (p?.theme_color || p?.theme_visual) {
          setTheme((t) => ({
            color: p.theme_color || t.color, visual: p.theme_visual || t.visual,
          }))
        }
        if (p?.notify_prefs && typeof p.notify_prefs.sound === "boolean") {
          setSoundOn(p.notify_prefs.sound)
          localStorage.setItem("todorio.sound", p.notify_prefs.sound ? "1" : "0")
        }
      })
      .catch(() => {})
      .finally(() => setLoaded(true))
  }, [])

  useEffect(() => { applyTheme(theme.color, theme.visual) }, [theme])

  // Navigating always dismisses the drawer — otherwise it would stay open over the page the
  // user just chose. Escape closes it too, matching the modal behaviour.
  useEffect(() => { setNavOpen(false) }, [view])
  // Opening the notifications page is what "reading" them means here — there's no separate
  // per-item read action surfaced from this list, so the sidebar badge clears on arrival.
  useEffect(() => { if (view === "notifications") setUnread(0) }, [view])
  useEffect(() => {
    if (!navOpen) return
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setNavOpen(false) }
    window.addEventListener("keydown", onKey)
    return () => window.removeEventListener("keydown", onKey)
  }, [navOpen])

  // SSE — live notifications after login
  useEffect(() => {
    if (!me || me.status !== "active") return
    const es = new EventSource("/api/events")
    es.addEventListener("notification", (e) => {
      if (routeView(parseRoute()) !== "notifications") setUnread((n) => n + 1)
      if (soundOnRef.current) beep()
      notifyBrowser(e.data, openSearchTask)
    })
    esRef.current = es
    return () => es.close()
  }, [me?.id, me?.status])

  // Hotkeys: single keys only, ignored while typing in a field or with a modifier held, and
  // disableable via localStorage (per spec: work outside inputs, no single-key destructive actions).
  useEffect(() => {
    if (!me || me.status !== "active") return
    function onKeyDown(e: KeyboardEvent) {
      if (localStorage.getItem("todorio.hotkeys") === "0") return
      if (e.ctrlKey || e.metaKey || e.altKey) return
      const t = e.target as HTMLElement
      if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA" || t.tagName === "SELECT" || t.isContentEditable)) return
      switch (e.key) {
        case "?": setShowHelp((v) => !v); break
        case "m": setView("my"); break
        case "i": setView("inbox"); break
        case "s": setView("spaces"); break
        // "u" for users: "m" is already My tasks and "p" reads as nothing in either locale.
        case "u": setView("members"); break
        case "/": setView("search"); break
        case "n": setView("notifications"); break
        case "Escape": setShowHelp(false); break
      }
    }
    window.addEventListener("keydown", onKeyDown)
    return () => window.removeEventListener("keydown", onKeyDown)
  }, [me?.status])

  function updateTheme(patch: Partial<typeof theme>) {
    const next = { ...theme, ...patch }
    setTheme(next)
    localStorage.setItem("todorio.theme", JSON.stringify(next))
    if (me) {
      api.patch("/api/me", {
        theme_color: next.color, theme_visual: next.visual,
      }).catch(() => {})
    }
  }

  async function logout() {
    await api.post("/api/logout").catch(() => {})
    esRef.current?.close()
    setMe(null)
  }

  if (!loaded) return null
  const siteName = boot?.site_name || "Todorio"

  if (!me) return <AuthPage siteName={siteName} locales={boot?.locales_enabled} onLogin={setMe} />
  if (me.status !== "active") return <PendingPage onLogout={logout} />
  if (me.must_change_password) {
    return <ForcedPasswordChange me={me} onDone={() => setMe((m) => (m ? { ...m, must_change_password: false } : m))} />
  }

  return (
    <div className="app-layout">
      {/* Mobile-only top bar: the hamburger is the sole way to reach the nav under 860px. */}
      <div className="mobile-topbar">
        <button className="ctrl-btn" aria-label={tr("nav.menu")} aria-expanded={navOpen}
          onClick={() => setNavOpen((v) => !v)}>
          <IconMenu size={20} />
        </button>
        <img src={boot?.logo_path ? "/api/logo" : "/icons/logo.svg"} alt=""
          onError={(e) => { (e.currentTarget as HTMLImageElement).src = "/icons/logo.svg" }} />
        <b>{siteName}</b>
        {unread > 0 && <span className="badge" style={{ marginLeft: "auto" }}>{unread}</span>}
      </div>

      {navOpen && <div className="sidebar-backdrop" onClick={() => setNavOpen(false)} />}

      <aside className={"sidebar" + (navOpen ? " open" : "")}
        // A closed drawer is off-screen: keep it out of the tab order and away from screen
        // readers. `inert` covers both in one attribute.
        {...(isNarrow && !navOpen ? { inert: "" as any } : {})}>
        <div className="sidebar-header">
          {/* A root-uploaded logo wins; if none is set /api/logo 404s and we fall back to
              the bundled SVG. Swapping src in onError keeps that fallback declarative. */}
          <img src={boot?.logo_path ? "/api/logo" : "/icons/logo.svg"} alt=""
            onError={(e) => { (e.currentTarget as HTMLImageElement).src = "/icons/logo.svg" }} />
          <b>{siteName}</b>
        </div>
        
        <div className="sidebar-nav">
          <button className={"sidebar-btn" + (view === "my" ? " active" : "")} onClick={() => setView("my")}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M9 11l3 3L22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/></svg>
            {tr("nav.my")}
          </button>
          
          <button className={"sidebar-btn" + (view === "inbox" ? " active" : "")} onClick={() => setView("inbox")}>
            <IconInbox size={18} />
            {tr("inbox.title")}
          </button>
          
          <button className={"sidebar-btn" + (view === "spaces" ? " active" : "")} onClick={() => setView("spaces")}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>
            {tr("nav.spaces")}
          </button>

          {/* Members lives next to Spaces because that's what it administers. Read-only accounts
              still see it: the roster answers "who can see this?", which is not a write. */}
          <button className={"sidebar-btn" + (view === "members" ? " active" : "")} onClick={() => setView("members")}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M17 21v-2a4 4 0 0 0-4-4H5a4 4 0 0 0-4 4v2"/><circle cx="9" cy="7" r="4"/><path d="M23 21v-2a4 4 0 0 0-3-3.87"/><path d="M16 3.13a4 4 0 0 1 0 7.75"/></svg>
            {tr("nav.members")}
          </button>
          
          <button className={"sidebar-btn" + (view === "search" ? " active" : "")} onClick={() => setView("search")}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
            {tr("nav.search")}
          </button>

          <button className={"sidebar-btn" + (view === "notifications" ? " active" : "")} onClick={() => setView("notifications")}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M18 8A6 6 0 0 0 6 8c0 7-3 9-3 9h18s-3-2-3-9"/><path d="M13.73 21a2 2 0 0 1-3.46 0"/></svg>
            {tr("nav.notifications")}
            {unread > 0 && <span className="badge" style={{ marginLeft: "auto" }}>{unread}</span>}
          </button>

          {me.role !== "user" && me.role !== "viewer" && (
            <button className={"sidebar-btn" + (view === "admin" ? " active" : "")} onClick={() => setView("admin")}>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M12 2l3.09 6.26L22 9.27l-5 4.87 1.18 6.88L12 17.77l-6.18 3.25L7 14.14 2 9.27l6.91-1.01L12 2z"/></svg>
              {tr("nav.admin")}
            </button>
          )}

          <button className={"sidebar-btn" + (view === "settings" ? " active" : "")} onClick={() => setView("settings")}>
            <IconSliders size={18} />
            {tr("nav.settings")}
          </button>
        </div>

        <div className="sidebar-footer">
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 8, fontSize: 13, gap: 8 }}>
            <button className="row" style={{ gap: 8, background: "none", border: "none", color: "inherit", cursor: "pointer", padding: 0, overflow: "hidden" }}
              onClick={() => setView("settings")} title={tr("nav.settings")}>
              <Avatar userId={me.id} name={profile?.display_name || me.username} size={26} />
              <span className="muted" style={{ overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                {profile?.display_name || `@${me.username}`}
              </span>
            </button>
            <select className="input" style={{ width: "auto", padding: "4px 6px", fontSize: 12, flexShrink: 0 }} value={theme.color}
              onChange={(e) => updateTheme({ color: e.target.value })}>
              {COLORS.map((c) => <option key={c} value={c}>{c}</option>)}
            </select>
          </div>

          <div className="sidebar-controls">
            <button className="ctrl-btn" title={tr("nav.visual")}
              onClick={() => updateTheme({ visual: theme.visual === "rich" ? "lite" : "rich" })}>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
            </button>

            <button className="ctrl-btn" title={tr("nav.sound")} onClick={() => {
              const next = !soundOn
              setSoundOn(next)
              localStorage.setItem("todorio.sound", next ? "1" : "0")
              api.patch("/api/me", { notify_prefs: { sound: next } }).catch(() => {})
            }}>
              {soundOn ? (
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M11 5L6 9H2v6h4l5 4V5z"/><path d="M15.54 8.46a5 5 0 0 1 0 7.07"/></svg>
              ) : (
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M11 5L6 9H2v6h4l5 4V5z"/><line x1="23" y1="9" x2="17" y2="15"/><line x1="17" y1="9" x2="23" y2="15"/></svg>
              )}
            </button>

            <button className="ctrl-btn" title={tr("help.shortcuts")} onClick={() => setShowHelp(true)}>
              <IconKeyboard size={16} />
            </button>

            <button className="ctrl-btn" title={tr("nav.logout")} onClick={logout}>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
            </button>
          </div>

          <GlobalFocusTimer />
        </div>
      </aside>

      <main className="main-content">
        <AnnouncementsBanner />
        <DigestModal />
        {view === "my" && <MyTasksPage me={me} onOpenTask={(task) => openSearchTask(task.id)} />}
        {/* Own focused time, under the task list: the sidebar timer could start and stop
            sessions but the totals were never shown anywhere. Hides itself when nothing has
            been tracked. */}
        {view === "my" && <FocusStatsCard />}
        {view === "inbox" && <InboxPage onOpenTask={(item) => openSearchTask(item.id)} />}
        {view === "spaces" && <SpacesPage me={me} route={route} onOpenTask={openSearchTask} onOpenNote={openSearchNote} />}
        {view === "members" && <MembersPage me={me} />}
        {view === "search" && <SearchPage onOpenTask={openSearchTask} onOpenNote={openSearchNote} />}
        {routeError && (route.kind === "task" || route.kind === "note") && (
          <ModalShell onClose={closeRouteModal} maxWidth={520}>
            <p className="error-text">{routeError}</p>
            <button className="nav-btn" onClick={closeRouteModal}>{tr("common.back")}</button>
          </ModalShell>
        )}
        {searchTask && me && (
          <TaskModal task={searchTask} me={me} onClose={closeRouteModal}
            onChanged={() => api.get(`/api/tasks/${searchTask.id}`).then((r) => { if (r?.task) setSearchTask(r.task) })} />
        )}
        {searchNote && (
          <NoteModal note={searchNote} spaceId={searchNote.space_id} onClose={closeRouteModal}
            onChanged={() => api.get(`/api/notes/${searchNote.id}`).then((r) => { if (r?.note) setSearchNote(r.note) })} />
        )}
        {view === "notifications" && <NotificationsPage onNavigateTask={openSearchTask} />}
        {view === "settings" && <SettingsPage me={me} theme={theme} onUpdateTheme={updateTheme} onProfileSaved={setProfile} />}
        {view === "about" && (
          <AboutPage siteName={siteName} version={boot?.version}
            developerUrl={boot?.developer_url} aboutText={boot?.about_text}
            sourceUrl={boot?.source_url} donateUrl={boot?.donate_url} onBack={() => setView("my")} />
        )}
        {view === "admin" && (
          <>
            <InvitesCard me={me} />
            <TemplatesAdminCard me={me} />
            <AnnouncementsAdminCard me={me} />
            <ServerSettingsCard me={me} />
            {/* Last: the audit trail explains the actions taken with the cards above it. */}
            <AuditLogCard />
          </>
        )}

        <footer className="muted" style={{ marginTop: 60, textAlign: "center", fontSize: 13 }}>
          {boot?.show_product_name !== false && <>{siteName} · </>}
          {tr("footer.developed_by")}{" "}
          {boot?.developer_url
            ? <a href={boot.developer_url} target="_blank" rel="noreferrer noopener">{DEVELOPER_NAME}</a>
            : DEVELOPER_NAME}
          {boot?.footer_text ? ` · ${boot.footer_text}` : ""}
          {" · "}
          <button className="linklike" onClick={() => setView("about")}>{tr("about.title")}</button>
        </footer>
      </main>

      {showHelp && (
        <ModalShell onClose={() => setShowHelp(false)} maxWidth={420}>
            <h3 className="row" style={{ marginTop: 0, gap: 6 }}><IconKeyboard size={17} /> {tr("help.shortcuts")}</h3>
            <div style={{ display: "grid", gridTemplateColumns: "auto 1fr", gap: "6px 14px", fontSize: 14 }}>
              <code>?</code><span>{tr("help.toggle")}</span>
              <code>m</code><span>{tr("nav.my")}</span>
              <code>i</code><span>{tr("inbox.title")}</span>
              <code>s</code><span>{tr("nav.spaces")}</span>
              <code>u</code><span>{tr("nav.members")}</span>
              <code>/</code><span>{tr("nav.search")}</span>
              <code>n</code><span>{tr("nav.notifications")}</span>
              <code>Esc</code><span>{tr("help.close")}</span>
            </div>
            <label className="row" style={{ marginTop: 14, gap: 6, fontSize: 13 }}>
              <input type="checkbox" defaultChecked={localStorage.getItem("todorio.hotkeys") !== "0"}
                onChange={(e) => localStorage.setItem("todorio.hotkeys", e.target.checked ? "1" : "0")} />
              {tr("help.enable_hotkeys")}
            </label>
            <button className="btn" style={{ marginTop: 14 }} onClick={() => setShowHelp(false)}>{tr("digest.ok")}</button>
        </ModalShell>
      )}
    </div>
  )
}
