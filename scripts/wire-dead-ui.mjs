#!/usr/bin/env node
// Wires two finished-but-unreachable components into views.tsx.
//
//   * WorkflowEditor (web/src/workflow.tsx) had no importer at all. Custom per-space statuses
//     could be read by every selector in the app but never written, because the only screen
//     that writes them was not mounted anywhere.
//   * AssigneePicker (web/src/members.tsx) is exported with a comment saying it is for the task
//     modal, and the task modal never used it. The result was that a task could only be assigned
//     to the signed-in user ("assign to me" in the context menu and the bulk bar) and to nobody
//     else -- in a product whose point is working together.
//
// This is a codemod rather than a plain edit because views.tsx is ~100 KB and the tooling that
// produced this commit cannot rewrite a file that large in one piece. Same shape and same reason
// as scripts/split-views-taskui.mjs.
//
// Every edit carries a `done` marker, so running this twice is a no-op instead of a double
// insertion, and an anchor that no longer matches is a hard error rather than a silent skip:
// a codemod that quietly does nothing is worse than one that fails.
//
// Usage:
//   node scripts/wire-dead-ui.mjs           apply
//   node scripts/wire-dead-ui.mjs --check   verify the anchors still match, write nothing

import { readFileSync, writeFileSync } from "node:fs"
import { dirname, resolve } from "node:path"
import { fileURLToPath } from "node:url"

const here = dirname(fileURLToPath(import.meta.url))
const VIEWS = resolve(here, "..", "web", "src", "views.tsx")
const check = process.argv.includes("--check")

const TAB_FIELDS =
  '<button className={"nav-btn row" + (tab === "fields" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("fields")}><IconSliders size={14} /> {tr("fields.title")}</button>'
const RENDER_FIELDS =
  '{tab === "fields" && <div className="card"><FieldsPanel spaceId={space.id} isOwner={space.my_role === "owner"} /></div>}'
const WEIGHT_FIELD =
  '          <div>\n            <label className="muted" style={{ display: "block", fontSize: 12, marginBottom: 4 }}>{tr("task.weight")}</label>'

const ASSIGNEE_FIELD = [
  '          {/* Assignee. The picker asks the server who may hold this task',
  '              (GET /api/lists/{id}/assignable), so the options can never contain someone whose',
  '              write would be rejected. Clearing it sends clear_assignee, the same flag the row',
  '              context menu already uses -- assignee_id: null would be read as "absent". */}',
  '          <div>',
  '            <label className="muted row" style={{ fontSize: 12, marginBottom: 4, gap: 4 }}>',
  '              <IconUser size={12} /> {tr("task.assignee")}',
  '            </label>',
  '            <AssigneePicker listId={task.list_id} value={assigneeId}',
  '              onChange={(v) => {',
  '                setAssigneeId(v)',
  '                updateTask(v === null ? { clear_assignee: true } : { assignee_id: v })',
  '              }} />',
  '          </div>',
].join("\n")

const EDITS = [
  {
    name: "import WorkflowEditor and AssigneePicker",
    done: 'from "./workflow"',
    find: 'import { TimelineView } from "./timeline"\n',
    replace:
      'import { TimelineView } from "./timeline"\n' +
      'import { WorkflowEditor } from "./workflow"\n' +
      'import { AssigneePicker } from "./members"\n',
  },
  {
    name: "add the workflow tab to the SpaceView tab union",
    done: '| "workflow">("lists")',
    find: '"activity" | "archive" | "fields">("lists")',
    replace: '"activity" | "archive" | "fields" | "workflow">("lists")',
  },
  {
    name: "add the workflow tab button",
    done: 'setTab("workflow")',
    find: TAB_FIELDS,
    replace:
      TAB_FIELDS +
      '\n        <button className={"nav-btn row" + (tab === "workflow" ? " active" : "")} style={{ gap: 5, display: "inline-flex" }} onClick={() => setTab("workflow")}><IconColumns size={14} /> {tr("workflow.title")}</button>',
  },
  {
    name: "render the workflow editor",
    done: "<WorkflowEditor",
    find: RENDER_FIELDS,
    replace:
      RENDER_FIELDS +
      '\n      {tab === "workflow" && <div className="card"><WorkflowEditor spaceId={space.id} isOwner={space.my_role === "owner"} /></div>}',
  },
  {
    name: "track the assignee in TaskModal state",
    done: "const [assigneeId, setAssigneeId]",
    find: "  const [weight, setWeight] = useState(task.weight ?? 1)\n",
    replace:
      "  const [weight, setWeight] = useState(task.weight ?? 1)\n" +
      "  const [assigneeId, setAssigneeId] = useState<number | null>(task.assignee_id)\n",
  },
  {
    name: "render the assignee picker in TaskModal",
    done: "<AssigneePicker",
    find: WEIGHT_FIELD,
    replace: ASSIGNEE_FIELD + "\n\n" + WEIGHT_FIELD,
  },
]

let src = readFileSync(VIEWS, "utf8")
const applied = []
const skipped = []

for (const edit of EDITS) {
  if (src.includes(edit.done)) {
    skipped.push(edit.name)
    continue
  }
  const count = src.split(edit.find).length - 1
  if (count !== 1) {
    console.error(
      `refusing to run, the anchor for "${edit.name}" matched ${count} times (expected 1).\n` +
        "views.tsx changed since this codemod was written -- update the anchor.",
    )
    process.exit(1)
  }
  src = src.replace(edit.find, edit.replace)
  applied.push(edit.name)
}

if (check) {
  console.log(
    applied.length === 0
      ? `wire-dead-ui: already applied (${skipped.length} edit(s) in place)`
      : `wire-dead-ui: ${applied.length} edit(s) would be applied, anchors OK`,
  )
  process.exit(0)
}

if (applied.length === 0) {
  console.log("wire-dead-ui: nothing to do, all edits already in place")
  process.exit(0)
}

writeFileSync(VIEWS, src)
console.log(`wire-dead-ui: applied ${applied.length} edit(s):`)
for (const name of applied) console.log(`  - ${name}`)
if (skipped.length > 0) console.log(`  (${skipped.length} already in place)`)
