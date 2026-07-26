#!/usr/bin/env node
// One-shot codemod — delete this file once it has been run.
//
// Mounts WorkflowEditor (web/src/workflow.tsx, committed normally in this PR) as a tab in
// SpaceView. Only this mounting step needs a codemod: views.tsx is ~100 KB in one file and the
// tooling used to author it can only replace a whole file, so re-emitting it to add four lines
// risks committing a truncated file. Every anchor must match exactly once or nothing is written.
//
// Usage:
//   node scripts/patch-workflow-tab.mjs --check
//   node scripts/patch-workflow-tab.mjs

import { readFile, writeFile } from "node:fs/promises"

const FILE = "web/src/views.tsx"
const checkOnly = process.argv.includes("--check")
const L = (...lines) => lines.join("\n")

const edits = [
  {
    name: "import WorkflowEditor",
    find: 'import { WorkloadPanel, ImportCard } from "./functional"',
    replace: L(
      'import { WorkloadPanel, ImportCard } from "./functional"',
      'import { WorkflowEditor } from "./workflow"',
    ),
  },
  {
    name: "SpaceView: add the workflow tab to the tab union",
    find: '  const [tab, setTab] = useState<"lists" | "timeline" | "workload" | "notes" | "activity" | "archive" | "fields">("lists")',
    replace: '  const [tab, setTab] = useState<"lists" | "timeline" | "workload" | "notes" | "activity" | "archive" | "fields" | "workflow">("lists")',
  },
  {
    // Placed next to "fields": both are space-level configuration a viewer can look at and only an
    // owner can change, unlike the tabs to their left which are ways of reading the work itself.
    name: "SpaceView: workflow tab button",
    find: '        <button className={"nav-btn row" + (tab === "fields" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("fields")}><IconSliders size={14} /> {tr("fields.title")}</button>',
    replace: L(
      '        <button className={"nav-btn row" + (tab === "fields" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("fields")}><IconSliders size={14} /> {tr("fields.title")}</button>',
      '        <button className={"nav-btn row" + (tab === "workflow" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("workflow")}><IconColumns size={14} /> {tr("workflow.title") || "Статусы"}</button>',
    ),
  },
  {
    name: "SpaceView: render the workflow tab",
    find: '      {tab === "fields" && <div className="card"><FieldsPanel spaceId={space.id} isOwner={space.my_role === "owner"} /></div>}',
    replace: L(
      '      {tab === "fields" && <div className="card"><FieldsPanel spaceId={space.id} isOwner={space.my_role === "owner"} /></div>}',
      '      {tab === "workflow" && <div className="card"><WorkflowEditor spaceId={space.id} isOwner={space.my_role === "owner" || me.role === "root" || me.role === "admin"} /></div>}',
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
  // Function replacer, so $ sequences in the replacement are inserted literally.
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
console.log("Next: npm run build --prefix web, review git diff, commit, delete this script.")
