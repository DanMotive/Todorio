#!/usr/bin/env node
// One-shot codemod — delete this file once it has been run.
//
// Adds drag-to-reorder for the lists inside a space (SpaceView, the "lists" tab).
//
// Why this was worth doing: handleUpdateList has accepted a `position` since the beginning and
// handleListLists has always sorted by (l.position, l.id). Nothing ever wrote a position, so every
// space stayed ordered by creation date permanently. The server side was already finished.
//
// Why a script: views.tsx is ~100 KB in one file and the tooling used to author this can only
// replace a whole file, never a region. Every anchor must match exactly once or nothing is written.
//
// Usage:
//   node scripts/patch-list-reorder.mjs --check
//   node scripts/patch-list-reorder.mjs

import { readFile, writeFile } from "node:fs/promises"

const FILE = "web/src/views.tsx"
const checkOnly = process.argv.includes("--check")
const L = (...lines) => lines.join("\n")

const edits = [
  {
    // Anchored on the comment above it: `const [open, setOpen] = ...` alone appears in three
    // components in this file.
    name: "SpaceView: drag state and permission helper",
    find: L(
      "  // The Timeline tab only knows a task's id (it's plotting bars, not full task objects), so",
      '  // opening one from a bar click fetches it the same way openMentionedTask does elsewhere.',
      '  const [open, setOpen] = useState<Task | null>(null)',
    ),
    replace: L(
      "  // The Timeline tab only knows a task's id (it's plotting bars, not full task objects), so",
      '  // opening one from a bar click fetches it the same way openMentionedTask does elsewhere.',
      '  const [open, setOpen] = useState<Task | null>(null)',
      '',
      '  // Reordering lists. PATCH /api/lists/{id} has always accepted a position and',
      '  // handleListLists has always sorted by (position, id) — nothing ever wrote one, so every',
      '  // space was permanently ordered by creation date.',
      '  const [dragListId, setDragListId] = useState<number | null>(null)',
      '  const [reorderError, setReorderError] = useState("")',
      '  // The endpoint requires owner permission on each individual list, and being the owner of the',
      '  // space does not make you the owner of a list someone else created. So only lists you can',
      '  // actually move are draggable, instead of offering the gesture and answering with a 403.',
      '  // Read through a cast because nothing in the frontend consumed my_permission before this.',
      '  const canMoveList = (l: List) => (l as { my_permission?: string }).my_permission === "owner"',
      '    || me.role === "root" || me.role === "admin"',
    ),
  },
  {
    name: "SpaceView: reorderLists",
    find: '  async function openTaskById(id: number) {',
    replace: L(
      '  async function reorderLists(fromId: number, toId: number) {',
      '    if (fromId === toId) return',
      '    const order = [...lists]',
      '    const from = order.findIndex((l) => l.id === fromId)',
      '    const to = order.findIndex((l) => l.id === toId)',
      '    if (from < 0 || to < 0) return',
      '    const [moved] = order.splice(from, 1)',
      '    order.splice(to, 0, moved)',
      '    // Optimistic: a drag that snaps back while a request flies feels broken. load() at the end',
      '    // replaces this guess with the order the server actually stored.',
      '    setLists(order)',
      '    setReorderError("")',
      '    let failed = 0',
      '    for (let i = 0; i < order.length; i++) {',
      '      // Skip rows already sitting at their index: one drag usually shifts a couple of',
      '      // neighbours, and PATCHing every list in the space each time is pointless write load.',
      '      if ((order[i] as { position?: number }).position === i) continue',
      '      try {',
      '        await api.patch(`/api/lists/${order[i].id}`, { position: i })',
      '      } catch {',
      '        // Counted rather than thrown: a partial reorder still has to be reported, and',
      '        // stopping halfway would leave the remaining positions inconsistent with the UI.',
      '        failed++',
      '      }',
      '    }',
      '    if (failed > 0) {',
      '      setReorderError(tr("lists.reorder_partial")',
      '        || "Не удалось переставить часть списков — нужны права владельца списка.")',
      '    }',
      '    load()',
      '  }',
      '',
      '  async function openTaskById(id: number) {',
    ),
  },
  {
    name: "SpaceView: make list rows draggable drop targets",
    find: '              <div key={l.id} className="task-row" onClick={() => setCurrentList(l)}>',
    replace: L(
      '              <div key={l.id} className="task-row" onClick={() => setCurrentList(l)}',
      '                draggable={canMoveList(l)}',
      '                onDragStart={() => setDragListId(l.id)}',
      '                onDragEnd={() => setDragListId(null)}',
      '                // preventDefault is what marks this element as a valid drop target; without it',
      '                // the browser refuses the drop and no onDrop ever fires.',
      '                onDragOver={(e) => { if (dragListId !== null && dragListId !== l.id) e.preventDefault() }}',
      '                onDrop={(e) => {',
      '                  e.preventDefault()',
      '                  // Stop the row from also handling this as a click and navigating into the',
      '                  // list the drop landed on.',
      '                  e.stopPropagation()',
      '                  if (dragListId !== null) reorderLists(dragListId, l.id)',
      '                  setDragListId(null)',
      '                }}',
      '                style={{ opacity: dragListId === l.id ? 0.45 : 1, cursor: canMoveList(l) ? "grab" : undefined }}>',
    ),
  },
  {
    name: "SpaceView: reorder error and affordance hint",
    find: '          {lists.length === 0 && <p className="muted">{tr("spaces.lists_empty")}</p>}',
    replace: L(
      '          {lists.length === 0 && <p className="muted">{tr("spaces.lists_empty")}</p>}',
      '          {reorderError && <p className="error-text">{reorderError}</p>}',
      '          {/* Drag targets are invisible by nature, so the gesture is stated once — and only',
      '              when there is more than one list and at least one of them can actually move. */}',
      '          {lists.length > 1 && lists.some(canMoveList) && (',
      '            <p className="muted" style={{ fontSize: 12 }}>',
      '              {tr("lists.reorder_hint") || "Списки можно перетаскивать мышкой, чтобы поменять порядок."}',
      '            </p>',
      '          )}',
    ),
  },
]

const src = await readFile(FILE, "utf8")
let out = src
const problems = []

for (const edit of edits) {
  const hits = out.split(edit.find).length - 1
  if (hits !== 1) {
    problems.push(`${edit.name}: anchor matched ${hits} time(s), expected exactly 1`)
    continue
  }
  // Function replacer, so ${...} and $1 sequences in the replacement stay literal.
  out = out.replace(edit.find, () => edit.replace)
}

if (problems.length > 0) {
  console.error(`Refusing to modify ${FILE} — it is not the expected revision:`)
  for (const p of problems) console.error(`  ✗ ${p}`)
  console.error("No changes were written.")
  process.exit(1)
}

for (const edit of edits) console.log(`  ✓ ${edit.name}`)

if (checkOnly) {
  console.log("\nAll anchors matched. --check given, so nothing was written.")
  process.exit(0)
}

await writeFile(FILE, out)
console.log(`\nApplied ${edits.length} edits to ${FILE}.`)
console.log("Note: JSX comments inside an element's attribute list are not valid — the ones added")
console.log("here sit on their own lines between attributes, which is fine, but if the build")
console.log("complains, delete those two comment lines. Then: npm run build --prefix web.")
