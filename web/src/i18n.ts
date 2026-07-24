// Todorio localization.
// Locales use language-COUNTRY format + IT-slang styles (ru-RU-it, en-US-it) as overlay packs on top of the base locale.
// Selection order: profile -> Accept-Language/navigator -> server default -> en-US.

import ruRU from "./locales/ru-RU.json"
import enUS from "./locales/en-US.json"
import ukUA from "./locales/uk-UA.json"
import beBY from "./locales/be-BY.json"
import kkKZ from "./locales/kk-KZ.json"
import esES from "./locales/es-ES.json"
import ptBR from "./locales/pt-BR.json"
import trTR from "./locales/tr-TR.json"
import zhCN from "./locales/zh-CN.json"
import hiIN from "./locales/hi-IN.json"
import bnBD from "./locales/bn-BD.json"
import jaJP from "./locales/ja-JP.json"
import koKR from "./locales/ko-KR.json"
import ruRUit from "./locales/ru-RU-it.json"
import enUSit from "./locales/en-US-it.json"

export const SUPPORTED = [
  "en-US", "ru-RU", "uk-UA", "be-BY", "kk-KZ",
  "es-ES", "pt-BR", "tr-TR",
  "zh-CN", "hi-IN", "bn-BD", "ja-JP", "ko-KR",
] as const

const bundles: Record<string, Record<string, string>> = {
  "ru-RU": ruRU,
  "en-US": enUS,
  "uk-UA": ukUA,
  "be-BY": beBY,
  "kk-KZ": kkKZ,
  "es-ES": esES,
  "pt-BR": ptBR,
  "tr-TR": trTR,
  "zh-CN": zhCN,
  "hi-IN": hiIN,
  "bn-BD": bnBD,
  "ja-JP": jaJP,
  "ko-KR": koKR,
  // IT-slang styles are partial packs layered on top of the base locale (see t()).
  "ru-RU-it": ruRUit,
  "en-US-it": enUSit,
}

export function detectLocale(profileLocale?: string): string {
  if (profileLocale) return profileLocale
  for (const lang of navigator.languages ?? [navigator.language]) {
    const exact = SUPPORTED.find((l) => l.toLowerCase() === lang.toLowerCase())
    if (exact) return exact
    const base = lang.split("-")[0]
    const regional = SUPPORTED.find((l) => l.startsWith(base + "-"))
    if (regional) return regional
  }
  return "en-US"
}

export function t(locale: string, key: string): string {
  // IT-slang style: look in the style pack first, then the base locale, then en-US.
  const chain = locale.endsWith("-it")
    ? [locale, locale.slice(0, -3), "en-US"]
    : [locale, "en-US"]
  for (const l of chain) {
    const v = bundles[l]?.[key]
    if (v) return v
  }
  return key
}

// Current locale (module state) and a short tr() helper.
let current = detectLocale()
export function setLocale(l: string) { current = l }
export function getLocale(): string { return current }
export function tr(key: string): string { return t(current, key) }

// getFormattingLocale returns a locale string that's safe to pass to the browser's Intl API
// (toLocaleDateString, toLocaleString, ...). getLocale() can return an IT-slang overlay id like
// "ru-RU-it" — that's our own convention for picking a translation-string pack, not a real BCP-47
// tag, and Intl throws `RangeError: Incorrect locale information provided` if you pass it
// directly (verified: both "ru-RU-it" and "en-US-it" throw). Date/number formatting should follow
// the base language's real regional convention regardless of slang style anyway, so this just
// strips the "-it" suffix. Always use this (never getLocale() directly) when calling
// toLocaleDateString/toLocaleString/Intl.*.
export function getFormattingLocale(): string {
  return current.endsWith("-it") ? current.slice(0, -3) : current
}

// trFormal renders `key` in the user's current base language but always skips the IT-slang
// overlay, even if they have it selected — spec section 9: the admin panel should stay in the
// "normal style to avoid ambiguity", since slangy phrasing ("Задеплоено", "Хотфикс", ...) could
// read as unclear or unintentionally ironic in an administrative context. Use this instead of
// tr() inside admin-only screens (AdminPage, InvitesCard, ServerSettingsCard).
export function trFormal(key: string): string {
  const base = current.endsWith("-it") ? current.slice(0, -3) : current
  return t(base, key)
}
