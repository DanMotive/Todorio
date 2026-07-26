#!/usr/bin/env node
// Fills in UI strings that the code asks for but the locale files do not have yet.
//
// Why this exists: t() in web/src/i18n.ts ends with `return key`, so a key that is missing
// everywhere is printed to the user verbatim -- "nav.members", "focus.stats_title". en-US is the
// last link in the fallback chain, so a key missing there is missing for all 13 languages.
//
// Usage:
//   node scripts/i18n-add-keys.mjs           apply (writes the locale files)
//   node scripts/i18n-add-keys.mjs --check    report what is missing, write nothing (CI)
//
// Translations live in scripts/i18n-keys/*.json, one file per screen or feature:
//
//   { "overwrite": false, "keys": { "nav.members": { "en-US": "Members", "ru-RU": "..." } } }
//
// Keeping them as data rather than inline in the script is deliberate: adding a screen later is
// a new small file, and a translation fix is a one-line diff a non-programmer can make.

import { readdirSync, readFileSync, writeFileSync } from "node:fs"
import { dirname, join, resolve } from "node:path"
import { fileURLToPath } from "node:url"

const here = dirname(fileURLToPath(import.meta.url))
const localesDir = resolve(here, "..", "web", "src", "locales")
const keysDir = join(here, "i18n-keys")

// ru-RU-it / en-US-it are slang overlays layered on top of a base locale, not languages of their
// own: they hold only the phrases whose tone differs, and anything absent falls through to the
// base. Adding neutral strings to them would freeze a tone they are supposed to override.
const OVERLAYS = new Set(["ru-RU-it", "en-US-it"])

const checkOnly = process.argv.includes("--check")

// Serialise exactly the way the locale files are already formatted: keys sorted, two-space
// indent, one key per line, trailing newline. Anything else would turn every run into a diff
// against the whole file and make review impossible.
function serialize(table) {
	const body = Object.keys(table)
		.sort()
		.map((key) => `  ${JSON.stringify(key)}: ${JSON.stringify(table[key])}`)
		.join(",\n")
	return `{\n${body}\n}\n`
}

const localeFiles = readdirSync(localesDir)
	.filter((f) => f.endsWith(".json"))
	.map((f) => f.slice(0, -5))
const locales = localeFiles.filter((l) => !OVERLAYS.has(l)).sort()

if (locales.length === 0) {
	console.error(`no locale files found in ${localesDir}`)
	process.exit(1)
}

const groups = readdirSync(keysDir)
	.filter((f) => f.endsWith(".json"))
	.sort()
	.map((f) => ({ name: f, ...JSON.parse(readFileSync(join(keysDir, f), "utf8")) }))

// Validate every group against every locale *before* writing anything. A half-applied run would
// leave some languages with the key and some without, which is the state this script exists to
// get out of -- and the missing ones would silently fall back to English, so nobody would notice
// until a user did.
const problems = []
for (const group of groups) {
	if (!group.keys || typeof group.keys !== "object") {
		problems.push(`${group.name}: no "keys" object`)
		continue
	}
	for (const [key, byLocale] of Object.entries(group.keys)) {
		for (const locale of locales) {
			const value = byLocale[locale]
			if (typeof value !== "string" || value.trim() === "") {
				problems.push(`${group.name}: ${key} has no ${locale} translation`)
			}
		}
		for (const locale of Object.keys(byLocale)) {
			if (!locales.includes(locale)) {
				problems.push(`${group.name}: ${key} has an unknown locale ${locale}`)
			}
		}
	}
}
if (problems.length > 0) {
	console.error("refusing to run, the key tables are incomplete:")
	for (const p of problems) console.error(`  ${p}`)
	process.exit(1)
}

let written = 0
const missing = []

for (const locale of locales) {
	const path = join(localesDir, `${locale}.json`)
	const table = JSON.parse(readFileSync(path, "utf8"))
	const before = serialize(table)

	for (const group of groups) {
		const overwrite = group.overwrite === true
		for (const [key, byLocale] of Object.entries(group.keys)) {
			const next = byLocale[locale]
			if (!overwrite && Object.prototype.hasOwnProperty.call(table, key)) continue
			if (table[key] === next) continue
			missing.push(`${locale}: ${key}`)
			table[key] = next
		}
	}

	const after = serialize(table)
	if (after === before) continue
	if (!checkOnly) {
		writeFileSync(path, after, "utf8")
		written++
	}
}

if (checkOnly) {
	if (missing.length === 0) {
		console.log(`i18n keys OK (${locales.length} locales, ${groups.length} groups)`)
		process.exit(0)
	}
	console.error(`${missing.length} locale entries are missing or outdated:`)
	for (const m of missing) console.error(`  ${m}`)
	console.error("run: node scripts/i18n-add-keys.mjs")
	process.exit(1)
}

console.log(`updated ${written} locale file(s), ${missing.length} entries added`)
