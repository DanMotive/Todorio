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
    <div style={{ maxWidth: 860, margin: "0 auto", padding: "0 12px 40px" }}>
      <nav className="nav">
        <img src="/icons/logo.svg" alt="" width={28} height={28} />
        <b>{siteName}</b>
        <button className={"nav-btn" + (view === "my" ? " active" : "")} onClick={() => setView("my")}>{tr("nav.my")}</button>
        <button className={"nav-btn" + (view === "spaces" ? " active" : "")} onClick={() => setView("spaces")}>{tr("nav.spaces")}</button>
        <button className={"nav-btn" + (view === "notifications" ? " active" : "")} onClick={() => { setView("notifications") }}>
          🔔{unread > 0 && <span className="badge">{unread}</span>}
        </button>
        {me.role !== "user" && me.role !== "viewer" && (
          <button className={"nav-btn" + (view === "admin" ? " active" : "")} onClick={() => setView("admin")}>{tr("nav.admin")}</button>
        )}
        <span className="spacer" />
        <select className="input" style={{ width: "auto", padding: "6px 8px" }} value={theme.color}
          onChange={(e) => updateTheme({ color: e.target.value })}>
          {COLORS.map((c) => <option key={c} value={c}>{c}</option>)}
        </select>
        <button className="nav-btn" title={tr("nav.theme")}
          onClick={() => updateTheme({ scheme: theme.scheme === "dark" ? "light" : "dark" })}>
          {theme.scheme === "dark" ? "🌙" : "☀️"}
        </button>
        <button className="nav-btn" title={tr("nav.visual")}
          onClick={() => updateTheme({ visual: theme.visual === "rich" ? "lite" : "rich" })}>
          {theme.visual === "rich" ? "✨" : "🪶"}
        </button>
        <button className="nav-btn" title={tr("nav.sound")} onClick={() => {
          const next = !soundOn
          setSoundOn(next)
          localStorage.setItem("todorio.sound", next ? "1" : "0")
        }}>{soundOn ? "🔊" : "🔇"}</button>
        {installEvt && (
          <button className="nav-btn" title={tr("pwa.install")} onClick={async () => {
            installEvt.prompt()
            await installEvt.userChoice
            setInstallEvt(null)
          }}>⬇️</button>
        )}
        <button className="nav-btn" onClick={logout} title={"@" + me.username}>{tr("nav.logout")}</button>
      </nav>

      <main style={{ marginTop: 16 }}>
        <AnnouncementsBanner />
        <DigestModal />
        {view === "my" && <MyTasksPage me={me} />}
        {view === "spaces" && <SpacesPage me={me} />}
        {view === "notifications" && <NotificationsPage onRead={() => setUnread(0)} />}
        {view === "admin" && <><AdminPage me={me} /><TotpCard me={me} /><InvitesCard me={me} /></>}
      </main>

      <footer className="muted" style={{ marginTop: 40, textAlign: "center" }}>
        {siteName} · {tr("footer.developed_by")} {boot?.developer_name || "Vlad"}
      </footer>
    </div>
  )
}
