// Per-user settings (spec sections 7, 8, 9, 18, 20) — distinct from the root-only server
// settings panel in extras.tsx. Everything here is scoped to the logged-in user's own account:
// profile (name/avatar/language), appearance, notification preferences, deadline reminders, and
// a plain password-change form.
import { useEffect, useRef, useState } from "react"
import { api, type Me, type Profile, type NotifyPrefs } from "./api"
import { tr, setLocale, SUPPORTED } from "./i18n"
import { IconCamera, IconTrash, IconGlobe, IconBell, IconClock, IconShield, IconCheckCircle, IconAlertCircle, IconSliders, IconDownload } from "./icons"
import { TotpCard } from "./extras"
import { PersonalBotCard } from "./functional"
import { WallpaperPicker } from "./wallpaper"

const IT_STYLE_LOCALES = ["ru-RU", "en-US"]
const NOTIFY_TYPES = ["comment", "reaction", "task_assigned", "due_changed", "status_changed", "overdue"]
const THEME_COLORS = ["red", "blue", "green", "yellow", "gray"] as const
type ThemeState = { color: string; visual: string }

function hashHue(name: string): number {
  let h = 0
  for (let i = 0; i < name.length; i++) h = name.charCodeAt(i) + ((h << 5) - h)
  return Math.abs(h) % 360
}

// "Install app" (PWA) lives here in Settings rather than the sidebar.
//
// The browser fires `beforeinstallprompt` once, early — usually before this component ever
// mounts — so listening only from here would miss it and the button would never appear.
// main.tsx captures the event as soon as the page loads and stashes it on window; this
// component reads that stash on mount and also subscribes, whichever happens first.
//
// The row renders nothing at all when installation isn't offered: already installed, an
// unsupported browser, or plain HTTP (the spec requires HTTPS, self-signed is fine). A dead
// button the user can't act on is worse than no button.
function InstallAppRow() {
  const [evt, setEvt] = useState<any>(() => (window as any).todorioInstallEvent || null)
  const [installed, setInstalled] = useState(false)

  useEffect(() => {
    const onPrompt = (e: Event) => { e.preventDefault(); setEvt(e) }
    const onInstalled = () => { setInstalled(true); setEvt(null); (window as any).todorioInstallEvent = null }
    window.addEventListener("beforeinstallprompt", onPrompt)
    window.addEventListener("appinstalled", onInstalled)
    return () => {
      window.removeEventListener("beforeinstallprompt", onPrompt)
      window.removeEventListener("appinstalled", onInstalled)
    }
  }, [])

  // Already running as an installed app — nothing to offer.
  const standalone = typeof window.matchMedia === "function" &&
    window.matchMedia("(display-mode: standalone)").matches
  if (installed || standalone || !evt) return null

  return (
    <div style={{ marginBottom: 16 }}>
      <button className="btn secondary row" style={{ gap: 6 }} onClick={async () => {
        evt.prompt()
        await evt.userChoice.catch(() => {})
        setEvt(null)
        ;(window as any).todorioInstallEvent = null
      }}>
        <IconDownload size={14} /> {tr("pwa.install")}
      </button>
      <div className="muted" style={{ fontSize: 12, marginTop: 6 }}>{tr("pwa.install_hint")}</div>
    </div>
  )
}

// Reusable avatar: shows the uploaded photo if one exists, otherwise a stable-colored initial.
// userId is optional (e.g. for a not-yet-saved context) — without it, always shows initials.
export function Avatar({ userId, name, size = 32, refreshKey }: {
  userId?: number | null; name: string; size?: number; refreshKey?: number
}) {
  const [broken, setBroken] = useState(false)
  useEffect(() => { setBroken(false) }, [userId, refreshKey])
  const initials = (name || "?").trim().slice(0, 1).toUpperCase()
  const hue = hashHue(name || "?")
  if (!userId || broken) {
    return (
      <div style={{
        width: size, height: size, minWidth: size, borderRadius: "50%", display: "flex",
        alignItems: "center", justifyContent: "center", background: `hsl(${hue} 45% 32%)`,
        color: "#fff", fontSize: Math.round(size * 0.42), fontWeight: 600, flexShrink: 0,
      }}>{initials}</div>
    )
  }
  return (
    <img src={`/api/users/${userId}/avatar?v=${refreshKey ?? 0}`} width={size} height={size}
      style={{ borderRadius: "50%", objectFit: "cover", flexShrink: 0 }}
      onError={() => setBroken(true)} alt={name} />
  )
}

// Forced password change (spec section 4: "on first login, the temporary password must be
// changed"). Rendered instead of the whole app while me.must_change_password is true.
export function ForcedPasswordChange({ me, onDone }: { me: Me; onDone: () => void }) {
  const [oldPassword, setOldPassword] = useState("")
  const [newPassword, setNewPassword] = useState("")
  const [confirm, setConfirm] = useState("")
  const [error, setError] = useState("")
  const [busy, setBusy] = useState(false)

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    setError("")
    if (newPassword.length < 8) { setError(tr("profile.password_too_short")); return }
    if (newPassword !== confirm) { setError(tr("profile.password_mismatch")); return }
    setBusy(true)
    try {
      await api.post("/api/me/password", { old_password: oldPassword, new_password: newPassword })
      onDone()
    } catch (err) {
      setError((err as Error).message)
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="center-page">
      <form className="card auth-card" onSubmit={submit}>
        <div className="row" style={{ gap: 8 }}><IconShield size={22} /><h2 style={{ margin: 0 }}>{tr("forcepw.title")}</h2></div>
        <p className="muted">{tr("forcepw.text").replace("{username}", me.username)}</p>
        <input className="input" type="password" placeholder={tr("forcepw.current")} value={oldPassword}
          onChange={(e) => setOldPassword(e.target.value)} autoFocus />
        <input className="input" type="password" placeholder={tr("profile.new_password")} value={newPassword}
          onChange={(e) => setNewPassword(e.target.value)} />
        <input className="input" type="password" placeholder={tr("profile.confirm_password")} value={confirm}
          onChange={(e) => setConfirm(e.target.value)} />
        <div className="error-text">{error}</div>
        <button className="btn" type="submit" disabled={busy}>{tr("forcepw.button")}</button>
      </form>
    </div>
  )
}

// Browser system notifications (spec section 12): a toggle in the profile that requests
// Notification permission. Only offered when it can actually work — over HTTPS (or localhost,
// where browsers also allow it for development) and when the API exists at all. See App.tsx's
// notifyBrowser for why this is the Notification API from an open tab, not full Web Push.
function BrowserNotificationRow() {
  const [supported] = useState(() => typeof window !== "undefined" && "Notification" in window)
  const [secure] = useState(() =>
    location.protocol === "https:" || location.hostname === "localhost" || location.hostname === "127.0.0.1")
  const [perm, setPerm] = useState(() => (supported ? Notification.permission : "denied"))

  if (!supported || !secure) return null

  return (
    <label className="row" style={{ gap: 8, marginBottom: 10 }}>
      <input type="checkbox" checked={perm === "granted"}
        onChange={async (e) => {
          if (!e.target.checked) {
            // There is no API to revoke permission from the page; explain instead of pretending.
            alert(tr("profile.browser_notif_revoke_hint"))
            return
          }
          const result = await Notification.requestPermission()
          setPerm(result)
        }} />
      {tr("profile.browser_notif")}
      {perm === "denied" && <span className="muted" style={{ fontSize: 12 }}>{tr("profile.browser_notif_blocked")}</span>}
    </label>
  )
}

// Telegram notification delivery (spec follow-up: root supplies a bot token, each user links
// their own chat — see internal/telegram). Hidden entirely while root hasn't configured a bot,
// same convention as BrowserNotificationRow above: no dead controls for something that can't
// currently do anything.
//
// telegramEnabled/onToggleEnabled are lifted to the parent because that one flag lives in the
// same notify_prefs blob as sound/types/dnd and is saved through the same save() path — the
// connect/disconnect flow itself needs no such integration and manages its own state.
function TelegramLinkRow({ telegramEnabled, onToggleEnabled }: {
  telegramEnabled: boolean
  onToggleEnabled: (v: boolean) => void
}) {
  const [status, setStatus] = useState<{ enabled: boolean; linked: boolean } | null>(null)
  const [link, setLink] = useState<{ deep_link: string } | null>(null)
  const [busy, setBusy] = useState(false)

  const refresh = () => api.get("/api/telegram/status").then(setStatus).catch(() => setStatus({ enabled: false, linked: false }))
  useEffect(() => { refresh() }, [])

  // While a link is pending, poll every few seconds for the /start message to have landed —
  // gives a "it just connected itself" feel instead of making the user remember to come back
  // and click something. Stops on success, on unmount, or after ~2 minutes of nobody finishing.
  useEffect(() => {
    if (!link) return
    let tries = 0
    const id = window.setInterval(async () => {
      tries++
      const r = await api.get("/api/telegram/status").catch(() => null)
      if (r) setStatus(r)
      if (r?.linked || tries > 40) { window.clearInterval(id); setLink(null) }
    }, 3000)
    return () => window.clearInterval(id)
  }, [link])

  if (!status?.enabled) return null

  async function connect() {
    setBusy(true)
    try {
      setLink(await api.post("/api/telegram/link"))
    } catch { /* leave the button available to retry */ }
    setBusy(false)
  }
  async function disconnect() {
    await api.post("/api/telegram/unlink").catch(() => {})
    setLink(null)
    refresh()
  }

  if (status.linked) {
    return (
      <div style={{ marginBottom: 10 }}>
        <label className="row" style={{ gap: 8, marginBottom: 6 }}>
          <input type="checkbox" checked={telegramEnabled} onChange={(e) => onToggleEnabled(e.target.checked)} />
          {tr("profile.telegram_enabled_toggle")}
        </label>
        <div className="row" style={{ gap: 10 }}>
          <span className="row muted" style={{ gap: 5, fontSize: 13 }}>
            <IconCheckCircle size={13} /> {tr("profile.telegram_connected")}
          </span>
          <button className="nav-btn" onClick={disconnect}>{tr("profile.telegram_disconnect")}</button>
        </div>
      </div>
    )
  }

  if (link) {
    return (
      <div style={{ marginBottom: 10 }}>
        <a className="btn secondary row" style={{ gap: 6, display: "inline-flex", width: "fit-content", textDecoration: "none" }}
          href={link.deep_link} target="_blank" rel="noreferrer">
          {tr("profile.telegram_open_bot")}
        </a>
        <div className="muted" style={{ fontSize: 12, marginTop: 6 }}>{tr("profile.telegram_waiting")}</div>
      </div>
    )
  }

  return (
    <div style={{ marginBottom: 10 }}>
      <button className="nav-btn" onClick={connect} disabled={busy}>{tr("profile.telegram_connect")}</button>
      <div className="muted" style={{ fontSize: 12, marginTop: 6 }}>{tr("profile.telegram_connect_hint")}</div>
    </div>
  )
}

export function SettingsPage({ me, theme, onUpdateTheme, onProfileSaved }: {
  me: Me; theme: ThemeState; onUpdateTheme: (patch: Partial<ThemeState>) => void; onProfileSaved: (p: Profile) => void
}) {
  const [profile, setProfile] = useState<Profile | null>(null)
  const [displayName, setDisplayName] = useState("")
  const [locale, setLocaleField] = useState("en-US")
  const [itStyle, setItStyle] = useState(false)
  const [avatarKey, setAvatarKey] = useState(0)
  const [uploading, setUploading] = useState(false)
  const fileRef = useRef<HTMLInputElement>(null)

  const [oldPassword, setOldPassword] = useState("")
  const [newPassword, setNewPassword] = useState("")
  const [confirmPassword, setConfirmPassword] = useState("")
  const [pwMsg, setPwMsg] = useState<{ ok: boolean; text: string } | null>(null)

  const [msg, setMsg] = useState<{ ok: boolean; text: string } | null>(null)

  const load = () => api.get("/api/me").then((r) => {
    const p: Profile = r.profile
    setProfile(p)
    setDisplayName(p.display_name || "")
    const loc = p.locale || "en-US"
    setItStyle(loc.endsWith("-it"))
    setLocaleField(loc.endsWith("-it") ? loc.slice(0, -3) : loc)
  }).catch(() => {})
  useEffect(() => { load() }, [])

  if (!profile) return <div className="card">{tr("search.searching")}</div>

  const notifyPrefs: NotifyPrefs = profile.notify_prefs || {}

  async function save(patch: any, successText?: string) {
    try {
      await api.patch("/api/me", patch)
      setMsg({ ok: true, text: successText || tr("profile.saved") })
      const r = await api.get("/api/me")
      setProfile(r.profile)
      onProfileSaved(r.profile)
    } catch (e) {
      setMsg({ ok: false, text: (e as Error).message })
    }
  }

  function saveNotifyPrefs(patch: NotifyPrefs) {
    return save({ notify_prefs: patch })
  }

  async function uploadAvatar(f: File) {
    setUploading(true)
    try {
      const fd = new FormData()
      fd.append("file", f)
      const r = await fetch("/api/me/avatar", { method: "POST", body: fd, credentials: "same-origin" })
      if (!r.ok) {
        const e = await r.json().catch(() => null)
        setMsg({ ok: false, text: e?.error ?? tr("attach.failed") })
      } else {
        setAvatarKey((k) => k + 1)
        const r2 = await api.get("/api/me")
        setProfile(r2.profile)
        onProfileSaved(r2.profile)
      }
    } finally {
      setUploading(false)
      if (fileRef.current) fileRef.current.value = ""
    }
  }

  async function removeAvatar() {
    await api.del("/api/me/avatar").catch(() => {})
    setAvatarKey((k) => k + 1)
    const r = await api.get("/api/me")
    setProfile(r.profile)
    onProfileSaved(r.profile)
  }

  async function changePassword(e: React.FormEvent) {
    e.preventDefault()
    setPwMsg(null)
    if (newPassword.length < 8) { setPwMsg({ ok: false, text: tr("profile.password_too_short") }); return }
    if (newPassword !== confirmPassword) { setPwMsg({ ok: false, text: tr("profile.password_mismatch") }); return }
    try {
      await api.post("/api/me/password", { old_password: oldPassword, new_password: newPassword })
      setOldPassword(""); setNewPassword(""); setConfirmPassword("")
      setPwMsg({ ok: true, text: tr("profile.password_changed") })
    } catch (err) {
      setPwMsg({ ok: false, text: (err as Error).message })
    }
  }

  function toggleType(key: string, value: boolean) {
    const types = { ...(notifyPrefs.types || {}), [key]: value }
    saveNotifyPrefs({ types })
  }

  function toggleBeforeDay(day: number, checked: boolean) {
    const current = new Set(notifyPrefs.reminders?.before_days ?? [3, 1])
    if (checked) current.add(day); else current.delete(day)
    saveNotifyPrefs({ reminders: { ...notifyPrefs.reminders, before_days: [...current].sort((a, b) => b - a) } })
  }

  const dnd = notifyPrefs.dnd || { enabled: false, start: "22:00", end: "08:00" }
  const reminders = notifyPrefs.reminders || { before_days: [3, 1], on_due_day: true, daily_overdue: true }

  return (
    <div className="card">
      <h2>{tr("profile.title")}</h2>

      {/* ---------- profile ---------- */}
      <div className="section-title row" style={{ gap: 6 }}><IconCamera size={15} /> {tr("profile.section.profile")}</div>
      <div className="row" style={{ gap: 16, marginBottom: 14 }}>
        <Avatar userId={me.id} name={displayName || me.username} size={64} refreshKey={avatarKey} />
        <div>
          <div className="row" style={{ gap: 8 }}>
            <label className="nav-btn" style={{ cursor: "pointer" }}>
              {uploading ? tr("attach.uploading") : <span className="row" style={{ gap: 5 }}><IconCamera size={14} /> {tr("profile.avatar_upload")}</span>}
              <input ref={fileRef} type="file" accept="image/*" style={{ display: "none" }}
                onChange={(e) => e.target.files?.[0] && uploadAvatar(e.target.files[0])} />
            </label>
            {profile.avatar_path && (
              <button className="nav-btn row" style={{ gap: 5 }} onClick={removeAvatar}><IconTrash size={14} /> {tr("profile.avatar_remove")}</button>
            )}
          </div>
          <div className="muted" style={{ marginTop: 6, fontSize: 12 }}>{tr("profile.avatar_hint")}</div>
        </div>
      </div>
      <div className="row" style={{ gap: 8, marginBottom: 10 }}>
        <input className="input" style={{ maxWidth: 320 }} placeholder={tr("profile.display_name_placeholder")}
          value={displayName} onChange={(e) => setDisplayName(e.target.value)}
          onBlur={() => save({ display_name: displayName || null })} />
      </div>

      <div className="section-title row" style={{ gap: 6 }}><IconGlobe size={15} /> {tr("profile.language")}</div>
      <div className="row" style={{ gap: 10, marginBottom: 16, flexWrap: "wrap" }}>
        <select className="input" style={{ width: "auto" }} value={locale} onChange={(e) => {
          const next = e.target.value
          setLocaleField(next)
          const full = itStyle && IT_STYLE_LOCALES.includes(next) ? next + "-it" : next
          setLocale(full)
          save({ locale: full })
        }}>
          {SUPPORTED.map((l) => <option key={l} value={l}>{l}</option>)}
        </select>
        {IT_STYLE_LOCALES.includes(locale) && (
          <label className="row" style={{ gap: 6, fontSize: 13 }}>
            <input type="checkbox" checked={itStyle} onChange={(e) => {
              const checked = e.target.checked
              setItStyle(checked)
              const full = checked ? locale + "-it" : locale
              setLocale(full)
              save({ locale: full })
            }} />
            {tr("profile.it_style")}
          </label>
        )}
      </div>

      <div className="section-title row" style={{ gap: 6 }}><IconSliders size={15} /> {tr("profile.section.appearance")}</div>
      <div className="row" style={{ gap: 20, marginBottom: 16, flexWrap: "wrap" }}>
        <label className="row" style={{ gap: 6, fontSize: 13 }}>
          {tr("profile.color")}
          <select className="input" style={{ width: "auto" }} value={theme.color} onChange={(e) => onUpdateTheme({ color: e.target.value })}>
            {THEME_COLORS.map((c) => <option key={c} value={c}>{c}</option>)}
          </select>
        </label>
        <label className="row" style={{ gap: 6, fontSize: 13 }}>
          {tr("profile.visual")}
          <select className="input" style={{ width: "auto" }} value={theme.visual} onChange={(e) => onUpdateTheme({ visual: e.target.value })}>
            <option value="rich">{tr("profile.visual_rich")}</option>
            <option value="lite">{tr("profile.visual_lite")}</option>
          </select>
        </label>
      </div>
      {/* Wallpaper is a per-device choice and keeps its own state, so it needs no props. */}
      <WallpaperPicker />
      <InstallAppRow />

      {/* ---------- notifications ---------- */}
      <div className="section-title row" style={{ gap: 6 }}><IconBell size={15} /> {tr("profile.section.notifications")}</div>
      <label className="row" style={{ gap: 8, marginBottom: 10 }}>
        <input type="checkbox" checked={!!notifyPrefs.sound}
          onChange={(e) => saveNotifyPrefs({ sound: e.target.checked })} />
        {tr("nav.sound")}
      </label>
      <BrowserNotificationRow />
      <TelegramLinkRow telegramEnabled={notifyPrefs.telegram !== false}
        onToggleEnabled={(v) => saveNotifyPrefs({ telegram: v })} />
      {/* Always visible, unlike the row above: a personal bot is exactly what makes Telegram
          usable when the server has no bot of its own. */}
      <PersonalBotCard />
      <div style={{ marginBottom: 12 }}>
        <div className="muted" style={{ fontSize: 12, marginBottom: 6 }}>{tr("profile.notif_types")}</div>
        {NOTIFY_TYPES.map((k) => (
          <label key={k} className="row" style={{ gap: 8, marginBottom: 4, fontSize: 13 }}>
            <input type="checkbox" checked={notifyPrefs.types?.[k] !== false}
              onChange={(e) => toggleType(k, e.target.checked)} />
            {tr("profile.type." + k)}
          </label>
        ))}
      </div>
      <div style={{ marginBottom: 16 }}>
        <label className="row" style={{ gap: 8, marginBottom: 8 }}>
          <input type="checkbox" checked={!!dnd.enabled}
            onChange={(e) => saveNotifyPrefs({ dnd: { ...dnd, enabled: e.target.checked } })} />
          {tr("profile.dnd_title")}
        </label>
        {dnd.enabled && (
          <div className="row" style={{ gap: 10, marginLeft: 24 }}>
            <label className="row" style={{ gap: 6, fontSize: 13 }}>
              {tr("profile.dnd_from")}
              <input className="input" type="time" style={{ width: "auto" }} value={dnd.start}
                onChange={(e) => saveNotifyPrefs({ dnd: { ...dnd, start: e.target.value } })} />
            </label>
            <label className="row" style={{ gap: 6, fontSize: 13 }}>
              {tr("profile.dnd_to")}
              <input className="input" type="time" style={{ width: "auto" }} value={dnd.end}
                onChange={(e) => saveNotifyPrefs({ dnd: { ...dnd, end: e.target.value } })} />
            </label>
          </div>
        )}
      </div>

      {/* ---------- reminders ---------- */}
      <div className="section-title row" style={{ gap: 6 }}><IconClock size={15} /> {tr("profile.section.reminders")}</div>
      <div style={{ marginBottom: 16 }}>
        <div className="muted" style={{ fontSize: 12, marginBottom: 6 }}>{tr("profile.reminders_before")}</div>
        {[7, 3, 1].map((d) => (
          <label key={d} className="row" style={{ gap: 8, marginBottom: 4, fontSize: 13 }}>
            <input type="checkbox" checked={(reminders.before_days ?? [3, 1]).includes(d)}
              onChange={(e) => toggleBeforeDay(d, e.target.checked)} />
            {tr("profile.reminders_days_" + d)}
          </label>
        ))}
        <label className="row" style={{ gap: 8, marginBottom: 4, fontSize: 13 }}>
          <input type="checkbox" checked={reminders.on_due_day !== false}
            onChange={(e) => saveNotifyPrefs({ reminders: { ...reminders, on_due_day: e.target.checked } })} />
          {tr("profile.reminders_on_due")}
        </label>
        <label className="row" style={{ gap: 8, fontSize: 13 }}>
          <input type="checkbox" checked={reminders.daily_overdue !== false}
            onChange={(e) => saveNotifyPrefs({ reminders: { ...reminders, daily_overdue: e.target.checked } })} />
          {tr("profile.reminders_overdue")}
        </label>
      </div>

      {/* ---------- security ---------- */}
      <div className="section-title row" style={{ gap: 6 }}><IconShield size={15} /> {tr("profile.section.security")}</div>
      <form onSubmit={changePassword} style={{ maxWidth: 320, marginBottom: 8 }}>
        <input className="input" style={{ marginBottom: 8 }} type="password" placeholder={tr("profile.current_password")}
          value={oldPassword} onChange={(e) => setOldPassword(e.target.value)} />
        <input className="input" style={{ marginBottom: 8 }} type="password" placeholder={tr("profile.new_password")}
          value={newPassword} onChange={(e) => setNewPassword(e.target.value)} />
        <input className="input" style={{ marginBottom: 8 }} type="password" placeholder={tr("profile.confirm_password")}
          value={confirmPassword} onChange={(e) => setConfirmPassword(e.target.value)} />
        <button className="btn" type="submit">{tr("profile.password_change")}</button>
      </form>
      {pwMsg && (
        <div className="row" style={{ gap: 5, marginBottom: 12, color: pwMsg.ok ? "var(--pulse-ok)" : "var(--due-overdue)" }}>
          {pwMsg.ok ? <IconCheckCircle size={13} /> : <IconAlertCircle size={13} />} {pwMsg.text}
        </div>
      )}
      <TotpCard />

      {msg && (
        <div className="row" style={{ gap: 5, marginTop: 8, color: msg.ok ? "var(--pulse-ok)" : "var(--due-overdue)" }}>
          {msg.ok ? <IconCheckCircle size={13} /> : <IconAlertCircle size={13} />} {msg.text}
        </div>
      )}
    </div>
  )
}
