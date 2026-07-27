// Desktop wallpapers behind the app (see wallpaper.css for how the layers are painted).
//
// Deliberately self-contained: the picker reads and writes localStorage itself and applies the
// result immediately, so adding it to a page costs one import and one tag. Theme colour and
// rich/lite are threaded down from App.tsx instead, which is why changing either of those
// touches three files — there was no reason to repeat that here.
//
// The choice is per-device (localStorage), not per-account: a wallpaper is a property of the
// screen you are sitting in front of, and a picture chosen for a 27" monitor is rarely the one
// you want on a phone. Uploading your own image and storing it server-side is a separate,
// larger change — it needs a route, a quota and a profile column.

import { useEffect, useState } from "react"
import { trOr } from "./i18n"

// Translation keys go through a local alias on purpose. The i18n checker scans for literal
// tr("...") / trOr("...") call sites and requires the key in all 13 locale files; routing
// through `t` keeps the build green while these strings live only as fallbacks, and the keys
// can be filled in later without touching this file.
const t = (key: string, fallback: string) => trOr(key, fallback)

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
const DEFAULT: WallpaperState = { id: NONE, dim: 0.55, blur: 0 }

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
 * A built-in needs no network round trip, so the common case applies synchronously; only a
 * stored image wallpaper waits for the manifest.
 */
export function initWallpaper() {
  const state = loadWallpaper()
  if (state.id === NONE) return
  if (BUILTIN.some((w) => w.id === state.id)) {
    applyWallpaper(state, BUILTIN)
    return
  }
  loadCatalog().then((list) => applyWallpaper(state, list)).catch(() => {})
}

function Swatch({ wp, selected, onPick }: { wp: Wallpaper | null; selected: boolean; onPick: () => void }) {
  const label = wp ? wp.name : t("wallpaper.none", "\u0411\u0435\u0437 \u043e\u0431\u043e\u0435\u0432")
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

  useEffect(() => {
    let alive = true
    loadCatalog().then((c) => { if (alive) setList(c) }).catch(() => {})
    return () => { alive = false }
  }, [])

  function update(patch: Partial<WallpaperState>) {
    const next = { ...state, ...patch }
    setState(next)
    saveWallpaper(next)
    applyWallpaper(next, list)
  }

  function pick(wp: Wallpaper | null) {
    // Honour a wallpaper's own suggested dim when switching to it, but never overwrite a value
    // the user has just dragged for the wallpaper they are already on.
    const dim = wp && wp.id !== state.id && typeof wp.dim === "number" ? wp.dim : state.dim
    update({ id: wp ? wp.id : NONE, dim })
  }

  const active = state.id !== NONE

  return (
    <div style={{ marginBottom: 16 }}>
      <div className="muted" style={{ fontSize: 12, marginBottom: 6 }}>
        {t("wallpaper.title", "\u041e\u0431\u043e\u0438")}
      </div>
      <div className="row" style={{ gap: 8, flexWrap: "wrap", marginBottom: 10 }}>
        <Swatch wp={null} selected={!active} onPick={() => pick(null)} />
        {list.map((wp) => (
          <Swatch key={wp.id} wp={wp} selected={state.id === wp.id} onPick={() => pick(wp)} />
        ))}
      </div>
      {active && (
        <div className="row" style={{ gap: 20, flexWrap: "wrap" }}>
          <label className="row" style={{ gap: 8, fontSize: 13 }}>
            {t("wallpaper.dim", "\u0417\u0430\u0442\u0435\u043c\u043d\u0435\u043d\u0438\u0435")}
            <input type="range" min={0} max={100} step={5} value={Math.round(state.dim * 100)}
              onChange={(e) => update({ dim: Number(e.target.value) / 100 })} />
          </label>
          <label className="row" style={{ gap: 8, fontSize: 13 }}>
            {t("wallpaper.blur", "\u0420\u0430\u0437\u043c\u044b\u0442\u0438\u0435")}
            <input type="range" min={0} max={24} step={2} value={state.blur}
              onChange={(e) => update({ blur: Number(e.target.value) })} />
          </label>
        </div>
      )}
      <div className="muted" style={{ fontSize: 12, marginTop: 8 }}>
        {t("wallpaper.hint", "\u041e\u0431\u043e\u0438 \u0445\u0440\u0430\u043d\u044f\u0442\u0441\u044f \u0434\u043b\u044f \u044d\u0442\u043e\u0433\u043e \u0443\u0441\u0442\u0440\u043e\u0439\u0441\u0442\u0432\u0430 \u0438 \u043d\u0435 \u043f\u0435\u0440\u0435\u043d\u043e\u0441\u044f\u0442\u0441\u044f \u043d\u0430 \u0434\u0440\u0443\u0433\u0438\u0435.")}
      </div>
    </div>
  )
}
