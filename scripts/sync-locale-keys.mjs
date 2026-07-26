#!/usr/bin/env node
// Locale key maintenance for web/src/locales/*.json.
//
// Keep this script in the repo — unlike the patch-*.mjs codemods it is re-runnable, and its --check
// mode is meant to be a CI gate.
//
// Three jobs:
//   1. Harvest `tr("key") || "русский текст"` / `t("key", "русский текст")` pairs from the source and
//      write the missing ones into ru-RU.json. The fallback in the code IS the Russian string, so
//      this needs no translation and no retyping.
//   2. Report what every other locale is missing, without inventing it.
//   3. Audit: coverage per locale, and keys in locale files that nothing in the source asks for.
//
// Why ru-RU only, and why *-it.json is left alone:
// ru-RU-it.json (~54 keys) and en-US-it.json are not locales, they are slang overlays on top of the
// base language — "Задеплоено" for task.done, "В /dev/null" for task.archive. A key present there
// overrides the base, so filling them with neutral text would both duplicate the base and squat on
// exactly the keys the overlay exists to override. They are listed as opportunities, never written.
//
// Usage:
//   node scripts/sync-locale-keys.mjs --audit   # coverage + orphans, writes nothing
//   node scripts/sync-locale-keys.mjs --check   # report; exits 1 if anything is missing (CI gate)
//   node scripts/sync-locale-keys.mjs           # fill ru-RU.json

import { readdir, readFile, writeFile } from "node:fs/promises"
import { join } from "node:path"

const SRC = "web/src"
const LOCALES = join(SRC, "locales")
const BASE = "ru-RU.json" // the language the inline fallbacks are written in
const checkOnly = process.argv.includes("--check")
const audit = process.argv.includes("--audit")

const isOverlay = (name) => /-it\.json$/.test(name)

async function sourceFiles(dir) {
  const out = []
  for (const entry of await readdir(dir, { withFileTypes: true })) {
    const path = join(dir, entry.name)
    if (entry.isDirectory()) {
      if (entry.name === "locales" || entry.name === "node_modules") continue
      out.push(...(await sourceFiles(path)))
    } else if (/\.tsx?$/.test(entry.name)) {
      out.push(path)
    }
  }
  return out
}

// Only double-quoted fallbacks are harvested. A template-literal fallback can contain an
// interpolation, which is not a translatable constant and must not land in a locale file as if it
// were one.
const FALLBACK_PATTERNS = [
  /\btr\("([\w.]+)"\)\s*\|\|\s*"((?:[^"\\]|\\.)*)"/g,
  /\bt\("([\w.]+)",\s*"((?:[^"\\]|\\.)*)"\)/g,
]
// Every literal key the code reads, with or without a fallback — used by the orphan report.
const USED_KEY_PATTERN = /\b(?:tr|trFormal|t)\(\s*"([\w.]+)"/g
// Keys built at runtime, e.g. tr("task.status." + s) or tr("notif.kind." + type). The individual
// keys are never spelled out in the source, so their prefixes are collected and any locale key
// under such a prefix is treated as used. Without this, the orphan report would accuse every
// status, priority and notification label of being dead.
const DYNAMIC_PREFIX_PATTERN = /\b(?:tr|trFormal|t)\(\s*"([\w.]+\.)"\s*\+/g

const harvested = new Map() // key -> { text, files:Set }
const usedKeys = new Set()
const dynamicPrefixes = new Set()
const conflicts = []

for (const file of await sourceFiles(SRC)) {
  const src = await readFile(file, "utf8")
  for (const m of src.matchAll(USED_KEY_PATTERN)) usedKeys.add(m[1])
  for (const m of src.matchAll(DYNAMIC_PREFIX_PATTERN)) dynamicPrefixes.add(m[1])
  for (const pattern of FALLBACK_PATTERNS) {
    for (const m of src.matchAll(pattern)) {
      const key = m[1]
      const text = m[2].replace(/\\"/g, '"').replace(/\\\\/g, "\\")
      const seen = harvested.get(key)
      if (seen && seen.text !== text) {
        // The same key with two different fallbacks means the code disagrees with itself about what
        // the string says. Choosing one silently would hide a real bug.
        conflicts.push(`${key}: "${seen.text}" (${[...seen.files].join(", ")}) vs "${text}" (${file})`)
        continue
      }
      if (seen) seen.files.add(file)
      else harvested.set(key, { text, files: new Set([file]) })
    }
  }
}

if (conflicts.length > 0) {
  console.error("Conflicting fallbacks for the same key — fix these in the source first:")
  for (const c of conflicts) console.error(`  ✗ ${c}`)
  process.exit(1)
}
console.log(`Harvested ${harvested.size} key/fallback pairs from ${SRC}.`)

async function readLocale(name) {
  const path = join(LOCALES, name)
  const raw = await readFile(path, "utf8")
  let data
  try {
    data = JSON.parse(raw)
  } catch (err) {
    console.error(`✗ ${name}: not valid JSON (${err.message})`)
    process.exit(1)
  }
  const flat = data && typeof data === "object" && !Array.isArray(data)
    && Object.values(data).every((v) => typeof v === "string")
  if (!flat) {
    console.error(`✗ ${name}: expected a flat object of strings; this script will not guess at a nested shape`)
    process.exit(1)
  }
  return { path, raw, data }
}

const names = (await readdir(LOCALES)).filter((f) => f.endsWith(".json")).sort()
if (!names.includes(BASE)) {
  console.error(`✗ ${BASE} is missing; it is the reference locale`)
  process.exit(1)
}

// --- 1. Fill the base locale ------------------------------------------------------------------
const base = await readLocale(BASE)
const missingInBase = [...harvested.keys()].filter((k) => !(k in base.data))

if (missingInBase.length === 0) {
  console.log(`${BASE}: already complete.`)
} else if (audit || checkOnly) {
  console.log(`${BASE}: ${missingInBase.length} key(s) missing (would be filled from the fallbacks).`)
} else {
  // Existing keys keep their position and value; new ones are appended, so the diff is additions
  // only and nothing already written is overwritten.
  for (const key of missingInBase) base.data[key] = harvested.get(key).text
  // Indentation is detected rather than assumed, so re-serialising does not reformat the file and
  // bury the change in whitespace noise.
  const indentMatch = base.raw.match(/\n([ \t]+)"/)
  const indent = indentMatch ? indentMatch[1] : "  "
  const eol = base.raw.endsWith("\n") ? "\n" : ""
  await writeFile(base.path, JSON.stringify(base.data, null, indent) + indent.slice(0, 0) + eol)
  console.log(`${BASE}: +${missingInBase.length} key(s) written.`)
}

// The reference set of keys the app can ask for: whatever the base locale defines, plus anything
// harvested that is not in it yet.
const reference = new Set([...Object.keys(base.data), ...harvested.keys()])

// --- 2. Report the other locales --------------------------------------------------------------
let incomplete = 0
const report = []

for (const name of names) {
  if (name === BASE) continue
  const { data } = await readLocale(name)
  const missing = [...reference].filter((k) => !(k in data))
  if (isOverlay(name)) {
    // An overlay is supposed to be partial — missing keys there are style opportunities, not gaps,
    // so they never count as failures.
    report.push({ name, overlay: true, have: Object.keys(data).length, missing: missing.length })
    continue
  }
  const coverage = Math.round(((reference.size - missing.length) / reference.size) * 100)
  if (missing.length > 0) incomplete++
  report.push({ name, overlay: false, missing: missing.length, coverage, keys: missing })
}

console.log("\nCoverage against the reference key set:")
for (const r of report) {
  if (r.overlay) console.log(`  ${r.name.padEnd(14)} overlay — ${r.have} override(s); not a gap`)
  else console.log(`  ${r.name.padEnd(14)} ${String(r.coverage).padStart(3)}%  missing ${r.missing}`)
}

if (!audit) {
  for (const r of report) {
    if (r.overlay || r.missing === 0) continue
    console.log(`\n  ${r.name} needs real translations for:`)
    for (const key of r.keys) {
      const text = harvested.get(key)?.text ?? base.data[key] ?? ""
      console.log(`    ${key} = ${text}`)
    }
  }
}

// --- 3. Orphans -------------------------------------------------------------------------------
if (audit) {
  const used = (key) => usedKeys.has(key) || [...dynamicPrefixes].some((p) => key.startsWith(p))
  const orphans = Object.keys(base.data).filter((k) => !used(k))
  console.log(`\nKeys in ${BASE} that no source file appears to ask for: ${orphans.length}`)
  // Reported, never deleted: a key can also be reached from a runtime-built name this scan cannot
  // see, and silently dropping a live string is much worse than carrying a dead one.
  for (const k of orphans) console.log(`    ${k}`)
  if (orphans.length > 0) console.log("  (review by hand — dynamic key construction can hide a real use)")
}

if (checkOnly) {
  if (missingInBase.length > 0 || incomplete > 0) {
    console.error(`\n✗ ${missingInBase.length} key(s) missing in ${BASE}, ${incomplete} locale(s) incomplete.`)
    process.exit(1)
  }
  console.log("\n✓ Every locale has every key.")
}
