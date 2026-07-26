#!/usr/bin/env node
// Fills web/src/locales/*.json from the inline fallbacks in the source.
//
// Unlike the patch-*.mjs codemods, this one is worth keeping: it is re-runnable and stays useful
// every time a feature ships strings ahead of its translations.
//
// The problem it solves: Phases 1-4 wrote every new string as `tr("some.key") || "русский текст"` (or
// `t("some.key", "русский текст")`), so the UI reads correctly in Russian immediately and switches to
// the locale file the moment the key exists. That leaves the keys to be transcribed into JSON —
// by hand, across 15 files, which is exactly the kind of copying that produces typos and
// keys that never match what the code asks for.
//
// So the pairs are harvested from the source rather than retyped. The fallback in the code IS the
// Russian string, which makes ru-RU and ru-RU-it correct by construction.
//
// What it deliberately does NOT do: put those Russian strings into ja-JP, zh-CN, be-BY and the
// rest. A locale file quietly filled with the wrong language is worse than a missing key, because a
// missing key is visible and a wrong one is not. For those locales it prints the table of what is
// missing and writes nothing.
//
// Usage:
//   node scripts/sync-locale-keys.mjs --check   # report only, write nothing
//   node scripts/sync-locale-keys.mjs           # write the ru-* files

import { readdir, readFile, writeFile } from "node:fs/promises"
import { join } from "node:path"

const SRC = "web/src"
const LOCALES = join(SRC, "locales")
const checkOnly = process.argv.includes("--check")

// Which locale files can be filled automatically: only the ones whose language matches the
// language the fallbacks are written in.
const AUTOFILL = /^ru-/

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

// Both shapes used in the codebase. Only double-quoted literals are matched on purpose: a
// template-literal fallback can contain an interpolation, which is not a translatable constant and
// must not be written into a locale file as if it were.
const PATTERNS = [
  /\btr\("([\w.]+)"\)\s*\|\|\s*"((?:[^"\\]|\\.)*)"/g,
  /\bt\("([\w.]+)",\s*"((?:[^"\\]|\\.)*)"\)/g,
]

const harvested = new Map() // key -> { text, files: Set }
const conflicts = []

for (const file of await sourceFiles(SRC)) {
  const src = await readFile(file, "utf8")
  for (const pattern of PATTERNS) {
    for (const m of src.matchAll(pattern)) {
      const key = m[1]
      // Undo the source-level escaping so the JSON holds the real characters.
      const text = m[2].replace(/\\"/g, '"').replace(/\\\\/g, "\\")
      const seen = harvested.get(key)
      if (seen && seen.text !== text) {
        // The same key with two different fallbacks means the code itself disagrees about what the
        // string says. Picking one silently would hide a real bug, so it is reported and skipped.
        conflicts.push(`${key}: "${seen.text}" (${[...seen.files].join(", ")}) vs "${text}" (${file})`)
        continue
      }
      if (seen) seen.files.add(file)
      else harvested.set(key, { text, files: new Set([file]) })
    }
  }
}

console.log(`Harvested ${harvested.size} key/fallback pairs from ${SRC}.`)
if (conflicts.length > 0) {
  console.error("\nConflicting fallbacks for the same key — fix these in the source first:")
  for (const c of conflicts) console.error(`  ✗ ${c}`)
  process.exit(1)
}
if (harvested.size === 0) {
  console.log("Nothing to do.")
  process.exit(0)
}

const localeFiles = (await readdir(LOCALES)).filter((f) => f.endsWith(".json")).sort()
const pending = new Map() // locale -> string[] of missing keys
let written = 0

for (const name of localeFiles) {
  const path = join(LOCALES, name)
  const raw = await readFile(path, "utf8")
  let data
  try {
    data = JSON.parse(raw)
  } catch (err) {
    console.error(`✗ ${name}: not valid JSON (${err.message})`)
    process.exit(1)
  }
  // This script only understands a flat map of strings. If a locale file is nested, stop rather
  // than flatten it into a shape the loader may not read.
  const flat = data && typeof data === "object" && !Array.isArray(data)
    && Object.values(data).every((v) => typeof v === "string")
  if (!flat) {
    console.error(`✗ ${name}: expected a flat object of strings; this script will not guess at a nested shape`)
    process.exit(1)
  }

  const missing = [...harvested.keys()].filter((k) => !(k in data))
  if (missing.length === 0) continue

  const locale = name.replace(/\.json$/, "")
  if (!AUTOFILL.test(locale)) {
    pending.set(locale, missing)
    continue
  }

  // Existing keys keep their position and their value; new ones are appended, so the diff is only
  // additions and nothing already translated is overwritten.
  for (const key of missing) data[key] = harvested.get(key).text

  // Indentation is detected rather than assumed, so re-serialising does not reformat the entire
  // file and bury the actual change in whitespace noise.
  const indentMatch = raw.match(/\n([ \t]+)"/)
  const indent = indentMatch ? indentMatch[1] : "  "
  const trailingNewline = raw.endsWith("\n") ? "\n" : ""
  const next = JSON.stringify(data, null, indent) + trailingNewline

  if (!checkOnly) await writeFile(path, next)
  written++
  console.log(`  ${checkOnly ? "would fill" : "filled"} ${name}: +${missing.length} key(s)`)
}

if (pending.size > 0) {
  console.log("\nNot touched — these locales need real translations, not the Russian fallback:")
  for (const [locale, missing] of pending) {
    console.log(`\n  ${locale} (${missing.length} missing)`)
    for (const key of missing) console.log(`    ${key} = ${harvested.get(key).text}`)
  }
}

if (written === 0 && pending.size === 0) console.log("All locale files already have every key.")
else if (checkOnly) console.log("\n--check given, so nothing was written.")
