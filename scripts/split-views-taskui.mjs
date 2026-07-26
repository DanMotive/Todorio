#!/usr/bin/env node
// Lift the task-row helpers out of web/src/views.tsx into web/src/taskui.tsx.
//
//   node scripts/split-views-taskui.mjs --check   # report the plan, write nothing
//   node scripts/split-views-taskui.mjs           # perform the move
//
// Why a codemod instead of an edit: views.tsx is ~100 KB, and rewriting a file
// that size by hand (or through a remote API that only accepts whole files)
// invites silent damage. A script that locates declarations, moves them
// verbatim, and refuses to proceed when anything looks unsafe is auditable in a
// way a 100 KB diff is not.
//
// What it will not do: guess. If a moved declaration references something that
// stays behind in views.tsx, the script stops and names it, because that case
// needs a human decision (move that too, or pass it in as a prop).
//
// The first slice is deliberately the leaves of the dependency graph. Do not add
// KanbanBoard / TableView / CalendarView / SpaceView / TaskModal to MOVE without
// thinking it through: they reference one another, so moving one of them alone
// produces an import cycle between views.tsx and taskui.tsx.

import { readFileSync, writeFileSync, existsSync, readdirSync } from "node:fs"
import { join, dirname } from "node:path"
import { fileURLToPath } from "node:url"

const ROOT = dirname(dirname(fileURLToPath(import.meta.url)))
const SRC = join(ROOT, "web", "src")
const VIEWS = join(SRC, "views.tsx")
const TARGET = join(SRC, "taskui.tsx")

const CHECK = process.argv.includes("--check")
const FORCE = process.argv.includes("--force")

// Order here is the order they will appear in taskui.tsx.
const MOVE = [
	"endOfDayISO",
	"dueClass",
	"dueLabel",
	"formatSystemComment",
	"STATUS_VARS",
	"STATUS_ALT",
	"statusColor",
	"statusText",
	"StatusChip",
	"FocusButton",
	"TaskRow",
]

const HEADER = `// Task presentation primitives, split out of views.tsx.
//
// Everything here is a leaf: due-date formatting, the status colour table and
// chip, the single task row, and the focus start/stop button. They are imported
// by views.tsx (and by anything else that renders a task), and they import
// nothing from views.tsx \u2014 keep it that way, or the two files start requiring
// each other.
`

function fail(msg, extra = []) {
	console.error(`\n\u2716 ${msg}`)
	for (const line of extra) console.error(`    ${line}`)
	console.error("\nNothing was written.")
	process.exit(1)
}

// --- scanning -------------------------------------------------------------

// True when a line ends mid-expression, so a newline at depth 0 does not end the
// statement. `const X: Foo =` followed by the value on the next line is the case
// that matters.
function endsOpen(line) {
	const code = line.replace(/\/\/.*$/, "").trimEnd()
	return /[=+\-*/%&|^<>?:,.([{]$/.test(code) || /\b(?:extends|typeof|keyof|as|in|of|return|new)$/.test(code)
}

// Walk forward from `start` to the end of the declaration, tracking bracket
// depth while skipping strings, template literals and comments.
function endOfDeclaration(src, start) {
	let i = start
	let depth = 0
	let lineStart = start
	let quote = null
	while (i < src.length) {
		const c = src[i]
		const n = src[i + 1]
		if (quote) {
			if (c === "\\") {
				i += 2
				continue
			}
			if (c === quote) quote = null
			if (c === "\n") lineStart = i + 1
			i++
			continue
		}
		if (c === "/" && n === "/") {
			const nl = src.indexOf("\n", i)
			i = nl === -1 ? src.length : nl
			continue
		}
		if (c === "/" && n === "*") {
			const end = src.indexOf("*/", i)
			i = end === -1 ? src.length : end + 2
			continue
		}
		if (c === '"' || c === "'" || c === "`") {
			quote = c
			i++
			continue
		}
		if (c === "{" || c === "(" || c === "[") {
			depth++
			i++
			continue
		}
		if (c === "}" || c === ")" || c === "]") {
			depth--
			i++
			if (depth <= 0) {
				// End of the statement only if nothing but `;` follows on this line.
				let j = i
				while (j < src.length && (src[j] === ";" || src[j] === " " || src[j] === "\t")) j++
				if (j >= src.length) return src.length
				if (src[j] === "\n") return j + 1
			}
			continue
		}
		if (c === "\n") {
			if (depth === 0 && !endsOpen(src.slice(lineStart, i))) return i + 1
			lineStart = i + 1
			i++
			continue
		}
		i++
	}
	return src.length
}

// Include the comment block sitting directly above a declaration: it explains the
// thing being moved, so it belongs with it.
function startWithComments(src, declStart) {
	const before = src.slice(0, declStart)
	const lines = before.split("\n")
	lines.pop() // the (empty) fragment before declStart
	let keep = 0
	for (let i = lines.length - 1; i >= 0; i--) {
		if (/^\s*\/\//.test(lines[i])) keep++
		else break
	}
	if (keep === 0) return declStart
	return before.length - lines.slice(lines.length - keep).join("\n").length - 1
}

function findDeclaration(src, name) {
	const re = new RegExp(
		`^(?:export\\s+)?(?:default\\s+)?(?:async\\s+)?(?:const|let|var|function|type|interface|class)\\s+${name}\\b`,
		"m",
	)
	const m = re.exec(src)
	if (!m) return null
	const declStart = m.index
	return { start: startWithComments(src, declStart), declStart, end: endOfDeclaration(src, declStart) }
}

// --- imports --------------------------------------------------------------

const IMPORT_RE = /^import\s+(?:type\s+)?(?:(?:[\w*{}\s,]+?)\s+from\s+)?["'][^"']+["'];?\n/gm

function parseImports(src) {
	const out = []
	for (const m of src.matchAll(IMPORT_RE)) {
		const text = m[0]
		const source = /["']([^"']+)["']/.exec(text)[1]
		const isType = /^import\s+type\b/.test(text)
		const named = []
		let other = []
		const braces = /{([\s\S]*?)}/.exec(text)
		if (braces) {
			for (const part of braces[1].split(",")) {
				const spec = part.trim()
				if (!spec) continue
				const as = /^(.+?)\s+as\s+(.+)$/.exec(spec)
				named.push({ spec, local: as ? as[2].trim() : spec })
			}
		}
		const head = text.replace(/^import\s+(?:type\s+)?/, "").split(/\s+from\s+/)[0] || ""
		for (const piece of head.split(",")) {
			const t = piece.trim()
			if (!t || t.startsWith("{") || t.includes("}")) continue
			const ns = /^\*\s+as\s+(\w+)$/.exec(t)
			other.push(ns ? ns[1] : t)
		}
		other = other.filter((x) => /^\w+$/.test(x))
		out.push({ text, source, isType, named, other, start: m.index, end: m.index + text.length })
	}
	return out
}

function usesName(body, name) {
	return new RegExp(`(?<![\\w$.])${name.replace(/[$]/g, "\\$")}(?![\\w$])`).test(body)
}

// Rebuild an import for a given body, dropping specifiers that body does not use.
// Returns null when nothing from it is used.
function narrowImport(imp, body) {
	if (imp.named.length === 0 && imp.other.length === 0) return null // side-effect import
	const other = imp.other.filter((n) => usesName(body, n))
	const named = imp.named.filter((n) => usesName(body, n.local))
	if (other.length === 0 && named.length === 0) return null
	const kw = imp.isType ? "import type " : "import "
	const parts = []
	if (other.length) parts.push(other.join(", "))
	if (named.length) parts.push(`{ ${named.map((n) => n.spec).join(", ")} }`)
	return `${kw}${parts.join(", ")} from "${imp.source}"\n`
}

// --- main -----------------------------------------------------------------

if (!existsSync(VIEWS)) fail(`not found: ${VIEWS}`)
if (existsSync(TARGET) && !FORCE && !CHECK) {
	fail(`${TARGET} already exists \u2014 this codemod has probably run already. Pass --force to overwrite.`)
}

let views = readFileSync(VIEWS, "utf8")
const imports = parseImports(views)
if (imports.length === 0) fail("no import statements found in views.tsx \u2014 refusing to guess where the body starts")
const importsEnd = imports[imports.length - 1].end

// 1. locate every declaration before touching anything
const found = []
const absent = []
for (const name of MOVE) {
	const loc = findDeclaration(views, name)
	if (!loc) absent.push(name)
	else found.push({ name, ...loc, text: views.slice(loc.start, loc.end) })
}
if (absent.length) {
	fail("could not find these declarations in views.tsx:", absent)
}
for (const f of found) {
	if (f.declStart < importsEnd) fail(`${f.name} was matched inside the import block \u2014 aborting rather than mangling imports`)
}

// Overlapping ranges mean the scanner ran past the end of a declaration.
const sorted = [...found].sort((a, b) => a.start - b.start)
for (let i = 1; i < sorted.length; i++) {
	if (sorted[i].start < sorted[i - 1].end) {
		fail(
			`extracted ranges overlap: ${sorted[i - 1].name} appears to swallow ${sorted[i].name}. ` +
				"The declaration scanner needs fixing before this can run.",
		)
	}
}

const movedText = found.map((f) => f.text).join("\n")

// 2. what stays behind in views.tsx
let remaining = views
for (const f of sorted.slice().reverse()) {
	remaining = remaining.slice(0, f.start) + remaining.slice(f.end)
}
const remainingBody = remaining.slice(importsEnd)

// 3. safety net: the moved code must not depend on anything left in views.tsx
const topLevel = new Set()
for (const m of remainingBody.matchAll(
	/^(?:export\s+)?(?:async\s+)?(?:const|let|var|function|type|interface|class)\s+(\w+)/gm,
)) {
	topLevel.add(m[1])
}
const dangling = [...topLevel].filter((n) => !MOVE.includes(n) && usesName(movedText, n))
if (dangling.length) {
	fail("the moved code still references declarations that stay in views.tsx:", [
		...dangling,
		"",
		"Either add them to MOVE (if they are leaves too) or pass them in as props.",
	])
}

// 4. build taskui.tsx
const targetImports = imports.map((imp) => narrowImport(imp, movedText)).filter(Boolean)
const body = found
	.map((f) => (/^\s*export\b/m.test(f.text.split("\n").find((l) => !/^\s*\/\//.test(l)) || "") ? f.text : f.text.replace(/^(\s*)(const|let|var|function|type|interface|class|async)\b/m, "$1export $2")))
	.join("\n")
const target = `${HEADER}\n${targetImports.join("")}\n${body.trimEnd()}\n`

// 5. rebuild views.tsx imports against what is left, then import the moved names back
const keptImports = imports.map((imp) => (imp.named.length === 0 && imp.other.length === 0 ? imp.text : narrowImport(imp, remainingBody))).filter(Boolean)
const backImports = MOVE.filter((n) => usesName(remainingBody, n))
if (backImports.length) {
	keptImports.push(`import { ${backImports.join(", ")} } from "./taskui"\n`)
}
const newViews = `${keptImports.join("")}${remainingBody.replace(/^\n+/, "\n")}`

// 6. other files may import the moved names from "./views"
const rewrites = []
for (const fn of readdirSync(SRC)) {
	if (!/\.(tsx|ts)$/.test(fn) || fn === "views.tsx" || fn === "taskui.tsx") continue
	const path = join(SRC, fn)
	const text = readFileSync(path, "utf8")
	let next = text
	for (const imp of parseImports(text)) {
		if (!/\.\/views$/.test(imp.source)) continue
		const moves = imp.named.filter((n) => MOVE.includes(n.local))
		if (moves.length === 0) continue
		const stays = imp.named.filter((n) => !MOVE.includes(n.local))
		const lines = []
		if (stays.length || imp.other.length) {
			const parts = []
			if (imp.other.length) parts.push(imp.other.join(", "))
			if (stays.length) parts.push(`{ ${stays.map((n) => n.spec).join(", ")} }`)
			lines.push(`import ${parts.join(", ")} from "${imp.source}"\n`)
		}
		lines.push(`import { ${moves.map((n) => n.spec).join(", ")} } from "./taskui"\n`)
		next = next.replace(imp.text, lines.join(""))
	}
	if (next !== text) rewrites.push({ path, fn, text: next, names: MOVE.filter((n) => usesName(text, n)) })
}

// --- report and write -----------------------------------------------------

const kb = (s) => `${(Buffer.byteLength(s) / 1024).toFixed(1)} KB`
console.log(`views.tsx: ${kb(views)} \u2192 ${kb(newViews)}`)
console.log(`taskui.tsx: ${kb(target)} (${found.length} declarations)`)
for (const f of found) console.log(`  moved ${f.name} (${f.end - f.start} bytes)`)
if (backImports.length) console.log(`views.tsx imports back: ${backImports.join(", ")}`)
for (const r of rewrites) console.log(`rewrote ./views import in ${r.fn}`)

if (CHECK) {
	console.log("\n--check: nothing written.")
	process.exit(0)
}

writeFileSync(TARGET, target)
writeFileSync(VIEWS, newViews)
for (const r of rewrites) writeFileSync(r.path, r.text)

console.log("\nDone. Now run:")
console.log("  cd web && npx tsc --noEmit && npm run build")
console.log("If tsc reports a missing import in either file, add it by hand \u2014 the narrowing")
console.log("above is textual and can drop something reached only through a type position.")
