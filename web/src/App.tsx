import { useEffect, useRef, useState } from "react"
import { api, type Me } from "./api"
import "./theme.css"
import "./ui.css"
import { AdminPage, AuthPage, MyTasksPage, NotificationsPage, PendingPage, SpacesPage } from "./views"
import { AnnouncementsBanner, DigestModal, TotpCard, InvitesCard } from "./extras"
import { detectLocale, setLocale, tr } from "./i18n"

type Bootstrap = {
  site_name: string
  browser_title: string
  developer_name: string
  default_locale: string
  theme: { color: string; scheme: string; visual: string }
}

const COLORS = ["red", "blue", "green", "yellow", "gray"] as const

function applyTheme(color: string, scheme: string, visual: string) {
  const el = document.documentElement
  el.dataset.color = color
  el.dataset.scheme = scheme
  el.dataset.visual = visual
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
  const [loaded, setLoaded] = useState(false)
  const [view, setView] = useState<"my" | "spaces" | "notifications" | "admin">("my")
  const [unread, setUnread] = useState(0)
  const [soundOn, setSoundOn] = useState(localStorage.getItem("todorio.sound") === "1")
  const [installEvt, setInstallEvt] = useState<any>(null)
  const esRef = useRef<EventSource | null>(null)

  // theme: server default <- personal override (localStorage + profile)
  const savedTheme = JSON.parse(localStorage.getItem("todorio.theme") || "null")
  const [theme, setTheme] = useState<{ color: string; scheme: string; visual: string }>(
    savedTheme || { color: "blue", scheme: "dark", visual: "rich" },
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
      .then((r) => { setMe(r.user); setUnread(r.unread_notifications) })
      .catch(() => {})
      .finally(() => setLoaded(true))
    const onInstall = (e: Event) => { e.preventDefault(); setInstallEvt(e) }
    window.addEventListener("beforeinstallprompt", onInstall)
    return () => window.removeEventListener("beforeinstallprompt", onInstall)
  }, [])

  useEffect(() => { applyTheme(theme.color, theme.scheme, theme.visual) }, [theme])

  // SSE — live notifications after login
  useEffect(() => {
    if (!me || me.status !== "active") return
    const es = new EventSource("/api/events")
    es.addEventListener("notification", () => {
      setUnread((n) => n + 1)
      if (localStorage.getItem("todorio.sound") === "1") beep()
    })
    esRef.current = es
    return () => es.close()
  }, [me?.id, me?.status])

  function updateTheme(patch: Partial<typeof theme>) {
    const next = { ...theme, ...patch }
    setTheme(next)
    localStorage.setItem("todorio.theme", JSON.stringify(next))
    if (me) {
      api.patch("/api/me", {
        theme_color: next.color, theme_scheme: next.scheme, theme_visual: next.visual,
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

  if (!me) return <AuthPage siteName={siteName} onLogin={(u) => { setMe(u); setView("my") }} />
  if (me.status !== "active") return <PendingPage onLogout={logout} />

  return (
    <div className="app-layout">
      <aside className="sidebar">
        <div className="sidebar-header">
          <img src="/icons/logo.svg" alt="" />
          <b>{siteName}</b>
        </div>
        
        <div className="sidebar-nav">
          <button className={"sidebar-btn" + (view === "my" ? " active" : "")} onClick={() => setView("my")}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M9 11l3 3L22 4"/><path d="M21 12v7a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h11"/></svg>
            {tr("nav.my")}
          </button>
          
          <button className={"sidebar-btn" + (view === "spaces" ? " active" : "")} onClick={() => setView("spaces")}>
            <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><rect x="3" y="3" width="7" height="7"/><rect x="14" y="3" width="7" height="7"/><rect x="14" y="14" width="7" height="7"/><rect x="3" y="14" width="7" height="7"/></svg>
            {tr("nav.spaces")}
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
        </div>

        <div className="sidebar-footer">
          <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", marginBottom: 8, fontSize: 13 }}>
            <span className="muted">@{me.username}</span>
            <select className="input" style={{ width: "auto", padding: "4px 6px", fontSize: 12 }} value={theme.color}
              onChange={(e) => updateTheme({ color: e.target.value })}>
              {COLORS.map((c) => <option key={c} value={c}>{c}</option>)}
            </select>
          </div>

          <div className="sidebar-controls">
            <button className="ctrl-btn" title={tr("nav.theme")}
              onClick={() => updateTheme({ scheme: theme.scheme === "dark" ? "light" : "dark" })}>
              {theme.scheme === "dark" ? (
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M21 12.79A9 9 0 1 1 11.21 3 7 7 0 0 0 21 12.79z"/></svg>
              ) : (
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><circle cx="12" cy="12" r="5"/><path d="M12 1v2m0 18v2M4.22 4.22l1.42 1.42m12.72 12.72l1.42 1.42M1 12h2m18 0h2M4.22 19.78l1.42-1.42M18.36 5.64l1.42-1.42"/></svg>
              )}
            </button>

            <button className="ctrl-btn" title={tr("nav.visual")}
              onClick={() => updateTheme({ visual: theme.visual === "rich" ? "lite" : "rich" })}>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><polygon points="12 2 15.09 8.26 22 9.27 17 14.14 18.18 21.02 12 17.77 5.82 21.02 7 14.14 2 9.27 8.91 8.26 12 2"/></svg>
            </button>

            <button className="ctrl-btn" title={tr("nav.sound")} onClick={() => {
              const next = !soundOn
              setSoundOn(next)
              localStorage.setItem("todorio.sound", next ? "1" : "0")
            }}>
              {soundOn ? (
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><polygon points="11 5 6 9H2v6h4l5 4V5z"/><path d="M15.54 8.46a5 5 0 0 1 0 7.07"/></svg>
              ) : (
                <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><polygon points="11 5 6 9H2v6h4l5 4V5z"/><line x1="23" y1="9" x2="17" y2="15"/><line x1="17" y1="9" x2="23" y2="15"/></svg>
              )}
            </button>

            <button className="ctrl-btn" title={tr("nav.logout")} onClick={logout}>
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2"><path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4"/><polyline points="16 17 21 12 16 7"/><line x1="21" y1="12" x2="9" y2="12"/></svg>
            </button>
          </div>
        </div>
      </aside>

      <main className="main-content">
        <AnnouncementsBanner />
        <DigestModal />
        {view === "my" && <MyTasksPage me={me} />}
        {view === "spaces" && <SpacesPage me={me} />}
        {view === "notifications" && <NotificationsPage onRead={() => setUnread(0)} />}
        {view === "admin" && <><AdminPage me={me} /><TotpCard me={me} /><InvitesCard me={me} /></>}

        <footer className="muted" style={{ marginTop: 60, textAlign: "center", fontSize: 13 }}>
          {siteName} · {tr("footer.developed_by")} {boot?.developer_name || "Vlad"}
        </footer>
      </main>
    </div>
  )
}
