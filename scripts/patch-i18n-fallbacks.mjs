#!/usr/bin/env node
// One-shot codemod — delete this file once it has been run.
//
// Fixes a mistake of mine that runs through every string added in Phases 1-4.
//
// i18n.ts t() ends with `return key`. So tr("some.key") on a miss returns the string
// "some.key" — which is truthy. Every place I wrote `tr("some.key") || "русский текст"`, and every
// local helper defined as `tr(key) || fallback`, therefore had a right-hand side that could never
// execute. The UI rendered the key name instead: nav.members, focus.stats_title, focus.total.
//
// Three fixes, in one script because they are worthless separately:
//   1. i18n.ts gains trOr(key, fallback) — the only place that can tell a miss from a hit, because
//      it is the only place that knows the sentinel is the key itself.
//   2. Every `tr("k") || "text"` and every local `tr(key) || fallback` helper is rewritten to use
//      it, with the import updated. Mechanical, so no string is retyped or lost.
//   3. Keys that never had a fallback are written into ru-RU.json and en-US.json.
//
// Why en-US.json matters more than the rest: t() walks locale -> en-US -> key. A key present in
// en-US stops being a raw key in all 13 locales at once. That is the difference between fixing this
// screen and fixing it for every language.
//
// Nothing is written unless every step validates first.
//
// Usage:
//   node scripts/patch-i18n-fallbacks.mjs --check
//   node scripts/patch-i18n-fallbacks.mjs

import { readdir, readFile, writeFile } from "node:fs/promises"
import { join } from "node:path"

const SRC = "web/src"
const LOCALES = join(SRC, "locales")
const I18N = join(SRC, "i18n.ts")
const checkOnly = process.argv.includes("--check")
const L = (...lines) => lines.join("\n")

// --- strings for the keys that never had an inline fallback ------------------------------------
// Only keys observed rendering as raw key names are listed. A locale entry that no code asks for is
// dead weight, so nothing is added speculatively — run scripts/sync-locale-keys.mjs --audit for the
// full picture.
const STRINGS = {
  "nav.members":         { ru: "Участники",                                  en: "Members" },
  "focus.stats_title":   { ru: "Статистика фокуса",                          en: "Focus stats" },
  "focus.week":          { ru: "Неделя",                                      en: "Week" },
  "focus.month":         { ru: "Месяц",                                      en: "Month" },
  "focus.total":         { ru: "Всего",                                       en: "Total" },
  "focus.sessions":      { ru: "Сессий",                                      en: "Sessions" },
  "focus.average":       { ru: "В среднем",                                  en: "Average" },
  // Used as a unit suffix after a number ("33 мин"), so it stays abbreviated in both languages.
  "focus.minutes_short": { ru: "мин",                                         en: "min" },
  "focus.stats_hint":    { ru: "Учитываются только завершённые сессии фокуса.", en: "Only completed focus sessions are counted." },
}

// --- 1. i18n.ts: add trOr ----------------------------------------------------------------------
const I18N_ANCHOR = 'export function tr(key: string): string { return t(current, key) }'
const I18N_REPLACEMENT = L(
  'export function tr(key: string): string { return t(current, key) }',
  '',
  '// trOr renders `key`, or `fallback` if no bundle in the lookup chain has it.',
  '//',
  '// Needed because t() signals a miss by returning the key itself, which is a truthy string. So',
  '// the obvious-looking `tr("some.key") || "text"` never falls back — it renders "some.key" to the',
  '// user. This is the only place that can tell a miss from a hit, since it is the only place that',
  '// knows what the sentinel is.',
  '//',
  '// Use it for a string that is shipping ahead of its locale entries; once the key exists in',
  '// ru-RU.json and en-US.json the fallback simply stops being reached, and switching to a plain',
  '// tr() is then a safe cleanup.',
  'export function trOr(key: string, fallback: string): string {',
  '  const value = t(current, key)',
  '  return value === key ? fallback : value',
  '}',
)

// --- 2. rewrite the dead pattern ---------------------------------------------------------------
// tr("some.key") || "text"  ->  trOr("some.key", "text")
const DEAD_CALL = /\btr\(\s*("[\w.]+")\s*\)\s*\|\|\s*("(?:[^"\\]|\\.)*")/g
// Local helpers, both shapes used in the codebase.
const DEAD_HELPERS = [
  {
    from: "const t = (key: string, fallback: string) => tr(key) || fallback",
    to: "const t = (key: string, fallback: string) => trOr(key, fallback)",
  },
  {
    from: "function t(key: string, fallback: string) { return tr(key) || fallback }",
    to: "function t(key: string, fallback: string) { return trOr(key, fallback) }",
  },
]

async function sourceFiles(dir) {
  const out = []
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name)
    if (entry.isDirectory()) {
      if (entry.name === "locales" || entry.name === "node_modules") continue
      out.push(...(await sourceFiles(path)))
    } else if (/\.tsx?$/.test(entry.name) && path !== I18N) {
      out.push(path)
    }
  }
  return out
}

const planned = []
const problems = []
const leftovers = []
let rewrites = 0

// i18n.ts first: without trOr nothing else compiles.
const i18nSrc = await readFile(I18N, "utf8")
if (i18nSrc.includes("export function trOr")) {
  console.log("i18n.ts already has trOr — leaving it alone.")
  planned.push({ path: I18N, out: i18nSrc, changed: false })
} else {
  const hits = i18nSrc.split(I18N_ANCHOR).length - 1
  if (hits !== 1) problems.push(`${I18N}: tr() anchor matched ${hits} time(s), expected exactly 1`)
  else planned.push({ path: I18N, out: i18nSrc.replace(I18N_ANCHOR, () => I18N_REPLACEMENT), changed: true })
}

for (const path of await sourceFiles(SRC)) {
  const src = await readFile(path, "utf8")
  let out = src
  let count = 0

  out = out.replace(DEAD_CALL, (_m, key, text) => { count++; return `trOr(${key}, ${text})` })
  for (const helper of DEAD_HELPERS) {
    if (out.includes(helper.from)) {
      out = out.split(helper.from).join(helper.to)
      count++
    }
  }
  if (count === 0) continue

  // The import has to gain trOr, or the file stops compiling. Two shapes are handled; anything
  // else is reported rather than guessed at.
  if (!/\btrOr\b/.test(src) && !/import\s*\{[^}]*\btrOr\b[^}]*\}\s*from\s*"\.\/i18n"/.test(out)) {
    const importMatch = out.match(/import\s*\{([^}]*)\}\s*from\s*"\.\/i18n"/)
    if (!importMatch) {
      problems.push(`${path}: rewrote ${count} call(s) but found no { ... } from "./i18n" import to extend`)
    } else {
      const names = importMatch[1]
      const extended = names.trimEnd().endsWith(",")
        ? `${names} trOr`
        : `${names.trimEnd()}, trOr`
      out = out.replace(importMatch[0], () => `import {${extended} } from "./i18n"`)
    }
  }

  // Anything still combining tr() with || is a shape this script does not understand — most likely
  // a template-literal fallback. Reported so it cannot stay silently broken.
  for (const [i, line] of out.split("\n").entries()) {
    if (/\btr\(/.test(line) && /\|\|/.test(line)) leftovers.push(`${path}:${i + 1}: ${line.trim()}`)
  }

  rewrites += count
  planned.push({ path, out, changed: true })
}

// --- 3. locale entries -------------------------------------------------------------------------
for (const [name, lang] of [["ru-RU.json", "ru"], ["en-US.json", "en"]]) {
  const path = join(LOCALES, name)
  const raw = await readFile(path, "utf8")
  let data
  try {
    data = JSON.parse(raw)
  } catch (err) {
    problems.push(`${path}: not valid JSON (${err.message})`)
    continue
  }
  const missing = Object.keys(STRINGS).filter((k) => !(k in data))
  if (missing.length === 0) {
    planned.push({ path, out: raw, changed: false })
    continue
  }
  // Existing entries are never overwritten — a translation already in the file beats anything
  // hardcoded here.
  for (const key of missing) data[key] = STRINGS[key][lang]
  const indentMatch = raw.match(/\n([ \t]+)"/)
  const indent = indentMatch ? indentMatch[1] : "  "
  const eol = raw.endsWith("\n") ? "\n" : ""
  planned.push({ path, out: JSON.stringify(data, null, indent) + eol, changed: true, added: missing.length })
}

if (problems.length > 0) {
  console.error("Refusing to modify anything:")
  for (const p of problems) console.error(`  ✗ ${p}`)
  console.error("\nNo files were written.")
  process.exit(1)
}

console.log(`Rewrote ${rewrites} dead fallback(s) across ${planned.filter((p) => p.changed).length} file(s):`)
for (const p of planned.filter((x) => x.changed)) {
  console.log(`  ✓ ${p.path}${p.added ? ` (+${p.added} key(s))` : ""}`)
}

if (leftovers.length > 0) {
  console.log("\nStill combining tr() with || — review these by hand:")
  for (const l of leftovers) console.log(`  ! ${l}`)
}

if (checkOnly) {
  console.log("\n--check given, so nothing was written.")
  process.exit(0)
}

for (const p of planned) {
  if (p.changed) await writeFile(p.path, p.out)
}

console.log("\nApplied. Next: npm run build --prefix web, then reload and confirm no raw keys remain.")
console.log("Then run: node scripts/sync-locale-keys.mjs --audit")
