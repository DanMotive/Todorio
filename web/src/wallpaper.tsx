// Desktop wallpapers behind the app (see wallpaper.css for how the layers are painted).
//
// Deliberately self-contained: the picker reads and writes localStorage itself and applies the
// result immediately, so adding it to a page costs one import and one tag. Theme colour and
// rich/lite are threaded down from App.tsx instead, which is why changing either of those
// touches three files — there was no reason to repeat that here.
//
// Two kinds of state, on purpose:
//
//   • *Which* wallpaper is selected, and its dim/blur, live in localStorage — per device. A
//     picture chosen for a 27" monitor is rarely the one you want on a phone.
//   • An *uploaded* picture lives on the server (users.wallpaper_path, see
//     internal/api/wallpaper.go). A file has to be stored somewhere, and re-uploading it on
//     every device would be worse than syncing it.
//
// So a user who uploads a photo on a laptop finds it offered on their phone, but has to pick it
// there — which is the behaviour you want when the two screens are shaped nothing alike.

import { useEffect, useRef, useState } from "react"
import { trOr } from "./i18n"

export type Wallpaper = {
  id: string
  name: string
  /** Any CSS background-image value. Used by the built-ins. */
  css?: string
  /** Path to an image served by the frontend, e.g. /wallpapers/mountains.jpg */
  url?: string
  /** Optional per-wallpaper starting dim, for pictures that are unusually bright or dark. */
  dim?: number
}

export type WallpaperState = { id: string; dim: number; blur: number }

const KEY = "todorio.wallpaper"
const NONE = "none"
const CUSTOM = "custom"
const CUSTOM_URL = "/api/me/wallpaper"
const DEFAULT: WallpaperState = { id: NONE, dim: 0.55, blur: 0 }

// Bumped after a successful upload. The endpoint answers with `private, max-age=3600`, which is
// what we want for a picture requested on every page load — but it also means that without a
// changing query string, replacing a wallpaper would keep painting the old one until the cache
// expired, and the upload would look like it had failed.
let customVersion = 0

function customWallpaper(): Wallpaper {
  return {
    id: CUSTOM,
    name: trOr("wallpaper.custom", "\u0421\u0432\u043e\u044f \u043a\u0430\u0440\u0442\u0438\u043d\u043a\u0430"),
    url: customVersion ? `${CUSTOM_URL}?v=${customVersion}` : CUSTOM_URL,
  }
}

/** Whether this account has an uploaded wallpaper. 404 from the endpoint means it has none. */
async function hasCustomWallpaper(): Promise<boolean> {
  try {
    // HEAD, not GET: this only needs the status, and the picture can be a few megabytes.
    // Go's ServeMux answers HEAD from the registered GET route.
    const r = await fetch(CUSTOM_URL, { method: "HEAD" })
    return r.ok
  } catch {
    return false
  }
}

// Built-ins are gradients, not files. They weigh nothing, never pixelate, and the first two
// are built from var(--accent), so they re-tint themselves when the colour theme changes.
export const BUILTIN: Wallpaper[] = [
  {
    id: "accent-glow",
    name: "Accent glow",
    css:
      "radial-gradient(120% 90% at 12% 8%, color-mix(in srgb, var(--accent) 60%, transparent) 0%, transparent 62%), " +
      "radial-gradient(90% 70% at 88% 18%, color-mix(in srgb, var(--accent) 28%, transparent) 0%, transparent 58%), " +
      "linear-gradient(165deg, #0F1420 0%, #1A2338 100%)",
  },
  {
    id: "aurora",
    name: "Aurora",
    css:
      "radial-gradient(100% 80% at 20% 100%, color-mix(in srgb, var(--accent) 45%, transparent) 0%, transparent 55%), " +
      "radial-gradient(80% 60% at 80% 0%, #7C3AED 0%, transparent 55%), " +
      "radial-gradient(70% 70% at 50% 50%, #22B8CF 0%, transparent 60%), " +
      "linear-gradient(180deg, #0B0F1A 0%, #131A2B 100%)",
    dim: 0.62,
  },
  {
    id: "dusk",
    name: "Dusk",
    css: "linear-gradient(200deg, #1B1035 0%, #172038 45%, #0F1420 100%)",
    dim: 0.35,
  },
  {
    id: "grid",
    name: "Grid",
    css:
      "repeating-linear-gradient(0deg, rgba(255,255,255,0.04) 0 1px, transparent 1px 48px), " +
      "repeating-linear-gradient(90deg, rgba(255,255,255,0.04) 0 1px, transparent 1px 48px), " +
      "linear-gradient(160deg, #111726 0%, #0F1420 100%)",
    dim: 0.2,
  },
  {
    id: "depth",
    name: "Depth",
    css:
      "radial-gradient(60% 60% at 50% 0%, #1D2740 0%, transparent 70%), " +
      "radial-gradient(80% 50% at 50% 100%, #131B2E 0%, transparent 65%), " +
      "#0B0F1A",
    dim: 0.25,
  },
]

function clamp(n: unknown, min: number, max: number, fallback: number): number {
  const v = typeof n === "number" ? n : Number(n)
  if (!Number.isFinite(v)) return fallback
  return Math.min(max, Math.max(min, v))
}

export function loadWallpaper(): WallpaperState {
  try {
    const raw = JSON.parse(localStorage.getItem(KEY) || "null")
    if (!raw || typeof raw.id !== "string") return DEFAULT
    return {
      id: raw.id,
      dim: clamp(raw.dim, 0, 1, DEFAULT.dim),
      blur: clamp(raw.blur, 0, 40, DEFAULT.blur),
    }
  } catch {
    return DEFAULT
  }
}

function saveWallpaper(state: WallpaperState) {
  try {
    localStorage.setItem(KEY, JSON.stringify(state))
  } catch {
    /* private mode / quota — the wallpaper still applies for this session */
  }
}

/** Turns the CSS background-image value for a wallpaper, or "" when it has neither source. */
function imageValue(wp: Wallpaper): string {
  if (wp.css) return wp.css
  // encodeURI keeps a stray quote or paren in a filename from breaking out of url(…).
  return wp.url ? `url("${encodeURI(wp.url)}")` : ""
}

export function applyWallpaper(state: WallpaperState, list: Wallpaper[]) {
  const el = document.documentElement
  const wp = state.id === NONE ? undefined : list.find((w) => w.id === state.id)
  const image = wp ? imageValue(wp) : ""
  // An unknown id (manifest edited, file removed) falls back to no wallpaper rather than to a
  // broken url() that would paint an empty layer over the theme background.
  if (!image) {
    el.removeAttribute("data-wallpaper")
    el.style.removeProperty("--wp-image")
    return
  }
  el.dataset.wallpaper = "on"
  el.style.setProperty("--wp-image", image)
  el.style.setProperty("--wp-dim", String(state.dim))
  el.style.setProperty("--wp-blur", `${state.blur}px`)
}

// System wallpapers are plain files under web/public/wallpapers plus a wallpapers.json listing
// them. That folder is bundled into the binary with the rest of the frontend, so adding a
// picture is a matter of dropping the file in and naming it in the manifest.
type ManifestEntry = { id?: string; name?: string; file?: string; dim?: number }
let catalogCache: Wallpaper[] | null = null

export async function loadCatalog(): Promise<Wallpaper[]> {
  if (catalogCache) return catalogCache
  let system: Wallpaper[] = []
  try {
    const r = await fetch("/wallpapers/wallpapers.json", { cache: "no-cache" })
    if (r.ok) {
      const data = await r.json()
      if (Array.isArray(data)) {
        system = data
          .filter((e: ManifestEntry) => e && typeof e.file === "string" && e.file !== "")
          .map((e: ManifestEntry) => ({
            id: e.id || `sys:${e.file}`,
            name: e.name || (e.file as string),
            url: `/wallpapers/${e.file}`,
            dim: typeof e.dim === "number" ? e.dim : undefined,
          }))
      }
    }
  } catch {
    /* no manifest, or malformed — the built-ins are still a complete offering */
  }
  catalogCache = [...BUILTIN, ...system]
  return catalogCache
}

/**
 * Applies the stored wallpaper at startup. Called from main.tsx rather than App.tsx so it runs
 * before React mounts and before the login screen renders — a wallpaper that fades in a moment
 * after the rest of the page looks like a bug.
 *
 * Built-ins and the uploaded picture need no network round trip to *identify*, so the common
 * cases apply synchronously; only a stored system wallpaper waits for the manifest.
 *
 * No check that the uploaded file still exists: verifying would cost a request before every
 * paint, and if the picture was deleted from another device the layer simply renders nothing
 * over the theme background — which looks exactly like having no wallpaper.
 */
export function initWallpaper() {
  const state = loadWallpaper()
  if (state.id === NONE) return
  if (state.id === CUSTOM) {
    applyWallpaper(state, [customWallpaper()])
    return
  }
  if (BUILTIN.some((w) => w.id === state.id)) {
    applyWallpaper(state, BUILTIN)
    return
  }
  loadCatalog().then((list) => applyWallpaper(state, list)).catch(() => {})
}

function wallpaperName(wp: Wallpaper): string {
  switch (wp.id) {
    case "accent-glow": return trOr("wallpaper.builtin.accent_glow", "Accent glow")
    case "aurora": return trOr("wallpaper.builtin.aurora", "Aurora")
    case "dusk": return trOr("wallpaper.builtin.dusk", "Dusk")
    case "grid": return trOr("wallpaper.builtin.grid", "Grid")
    case "depth": return trOr("wallpaper.builtin.depth", "Depth")
    case "roadtothemoon": return trOr("wallpaper.system.road_to_the_moon", "Road to the Moon")
    default: return wp.name
  }
}

function Swatch({ wp, selected, onPick }: { wp: Wallpaper | null; selected: boolean; onPick: () => void }) {
  const label = wp ? wallpaperName(wp) : trOr("wallpaper.none", "\u0411\u0435\u0437 \u043e\u0431\u043e\u0435\u0432")
  return (
    <button
      onClick={onPick}
      title={label}
      aria-pressed={selected}
      style={{
        width: 84,
        height: 52,
        padding: 0,
        cursor: "pointer",
        borderRadius: 8,
        border: selected ? "2px solid var(--accent)" : "1px solid var(--border)",
        backgroundImage: wp ? imageValue(wp) : "none",
        backgroundColor: wp ? undefined : "var(--bg)",
        backgroundSize: "cover",
        backgroundPosition: "center",
        position: "relative",
        overflow: "hidden",
      }}
    >
      {!wp && (
        <span className="muted" style={{ fontSize: 11 }}>{label}</span>
      )}
    </button>
  )
}

/** Wallpaper controls for the appearance section of Settings. */
export function WallpaperPicker() {
  const [state, setState] = useState<WallpaperState>(() => loadWallpaper())
  const [list, setList] = useState<Wallpaper[]>(BUILTIN)
  const [hasCustom, setHasCustom] = useState(false)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState("")
  const fileRef = useRef<HTMLInputElement | null>(null)

  useEffect(() => {
    let alive = true
    loadCatalog().then((c) => { if (alive) setList(c) }).catch(() => {})
    hasCustomWallpaper().then((yes) => { if (alive) setHasCustom(yes) })
    return () => { alive = false }
  }, [])

  // The uploaded picture goes first: it is the one the user chose deliberately.
  const fullList = hasCustom ? [customWallpaper(), ...list] : list

  function update(patch: Partial<WallpaperState>) {
    const next = { ...state, ...patch }
    setState(next)
    saveWallpaper(next)
    applyWallpaper(next, fullList)
  }

  function pick(wp: Wallpaper | null) {
    // Honour a wallpaper's own suggested dim when switching to it, but never overwrite a value
    // the user has just dragged for the wallpaper they are already on.
    const dim = wp && wp.id !== state.id && typeof wp.dim === "number" ? wp.dim : state.dim
    update({ id: wp ? wp.id : NONE, dim })
  }

  async function upload(file: File) {
    setBusy(true)
    setError("")
    try {
      const body = new FormData()
      body.append("file", file)
      const r = await fetch(CUSTOM_URL, { method: "POST", body })
      if (!r.ok) {
        // The server's message is the useful part here — too large, out of storage, or too many
        // uploads this hour are three different problems with three different fixes.
        const data = await r.json().catch(() => null)
        setError((data && data.error) || trOr("wallpaper.upload_failed", "\u041d\u0435 \u0443\u0434\u0430\u043b\u043e\u0441\u044c \u0437\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u044c"))
        return
      }
      customVersion = Date.now()
      setHasCustom(true)
      const next = { ...state, id: CUSTOM }
      setState(next)
      saveWallpaper(next)
      applyWallpaper(next, [customWallpaper(), ...list])
    } catch {
      setError(trOr("wallpaper.upload_failed", "\u041d\u0435 \u0443\u0434\u0430\u043b\u043e\u0441\u044c \u0437\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u044c"))
    } finally {
      setBusy(false)
    }
  }

  async function removeCustom() {
    setBusy(true)
    setError("")
    try {
      await fetch(CUSTOM_URL, { method: "DELETE" })
      setHasCustom(false)
      if (state.id === CUSTOM) {
        const next = { ...state, id: NONE }
        setState(next)
        saveWallpaper(next)
        applyWallpaper(next, list)
      }
    } catch {
      setError(trOr("wallpaper.upload_failed", "\u041d\u0435 \u0443\u0434\u0430\u043b\u043e\u0441\u044c \u0437\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u044c"))
    } finally {
      setBusy(false)
    }
  }

  const active = state.id !== NONE

  return (
    <div style={{ marginBottom: 16 }}>
      <div className="muted" style={{ fontSize: 12, marginBottom: 6 }}>
        {trOr("wallpaper.title", "\u041e\u0431\u043e\u0438")}
      </div>
      <div className="row" style={{ gap: 8, flexWrap: "wrap", marginBottom: 10 }}>
        <Swatch wp={null} selected={!active} onPick={() => pick(null)} />
        {fullList.map((wp) => (
          <Swatch key={wp.id} wp={wp} selected={state.id === wp.id} onPick={() => pick(wp)} />
        ))}
      </div>
      <div className="row" style={{ gap: 8, flexWrap: "wrap", marginBottom: 10 }}>
        <button onClick={() => fileRef.current?.click()} disabled={busy}>
          {busy
            ? trOr("wallpaper.uploading", "\u0417\u0430\u0433\u0440\u0443\u0437\u043a\u0430\u2026")
            : trOr("wallpaper.upload", "\u0417\u0430\u0433\u0440\u0443\u0437\u0438\u0442\u044c \u0441\u0432\u043e\u044e")}
        </button>
        {hasCustom && (
          <button onClick={removeCustom} disabled={busy}>
            {trOr("wallpaper.remove", "\u0423\u0434\u0430\u043b\u0438\u0442\u044c \u0441\u0432\u043e\u044e")}
          </button>
        )}
        <input
          ref={fileRef}
          type="file"
          accept="image/*"
          style={{ display: "none" }}
          onChange={(e) => {
            const f = e.target.files && e.target.files[0]
            // Cleared so that picking the same file again still fires a change event.
            e.target.value = ""
            if (f) upload(f)
          }}
        />
      </div>
      {error && (
        <div className="muted" style={{ fontSize: 12, marginBottom: 8, color: "var(--danger, #F87171)" }}>{error}</div>
      )}
      {active && (
        <div className="row" style={{ gap: 20, flexWrap: "wrap" }}>
          <label className="row" style={{ gap: 8, fontSize: 13 }}>
            {trOr("wallpaper.dim", "\u0417\u0430\u0442\u0435\u043c\u043d\u0435\u043d\u0438\u0435")}
            <input type="range" min={0} max={100} step={5} value={Math.round(state.dim * 100)}
              onChange={(e) => update({ dim: Number(e.target.value) / 100 })} />
          </label>
          <label className="row" style={{ gap: 8, fontSize: 13 }}>
            {trOr("wallpaper.blur", "\u0420\u0430\u0437\u043c\u044b\u0442\u0438\u0435")}
            <input type="range" min={0} max={24} step={2} value={state.blur}
              onChange={(e) => update({ blur: Number(e.target.value) })} />
          </label>
        </div>
      )}
      <div className="muted" style={{ fontSize: 12, marginTop: 8 }}>
        {trOr("wallpaper.hint", "\u0412\u044b\u0431\u043e\u0440 \u043e\u0431\u043e\u0435\u0432 \u0445\u0440\u0430\u043d\u0438\u0442\u0441\u044f \u0434\u043b\u044f \u044d\u0442\u043e\u0433\u043e \u0443\u0441\u0442\u0440\u043e\u0439\u0441\u0442\u0432\u0430. \u0421\u0432\u043e\u044f \u043a\u0430\u0440\u0442\u0438\u043d\u043a\u0430 \u0445\u0440\u0430\u043d\u0438\u0442\u0441\u044f \u0432 \u0430\u043a\u043a\u0430\u0443\u043d\u0442\u0435 \u0438 \u0432\u0438\u0434\u043d\u0430 \u043d\u0430 \u0432\u0441\u0435\u0445 \u0443\u0441\u0442\u0440\u043e\u0439\u0441\u0442\u0432\u0430\u0445.")}
      </div>
    </div>
  )
}
