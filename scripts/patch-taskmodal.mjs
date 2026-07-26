#!/usr/bin/env node
// One-shot codemod for web/src/views.tsx — delete this file once it has been run.
//
// Why a script instead of a normal commit: views.tsx is ~100 KB in a single file, and the tooling
// used to author this change can only replace a whole file, never a region. Re-emitting 100 KB
// from memory to change ~20 lines risks committing a silently truncated file, which would break
// the build and lose working code. An anchor-based codemod carries the same intent in 200 lines
// and is safer in one specific way that matters: it refuses to run unless every anchor matches
// exactly once, so it either applies the whole change or leaves the file byte-identical.
//
// What it adds — two endpoints that existed on the server and had no UI:
//
//   1. Assignee on the task form, via the AssigneePicker already written for the members screen
//      (GET /api/lists/{id}/assignable + PATCH /api/tasks/{id}).
//   2. Deleting your own comment (DELETE /api/comments/{id}). Editing was wired up; deleting
//      was not, so a comment posted by mistake could only be edited into something harmless.
//
// Usage:
//   node scripts/patch-taskmodal.mjs        # apply
//   node scripts/patch-taskmodal.mjs --check # report only, write nothing

import { readFile, writeFile } from "node:fs/promises"

const FILE = "web/src/views.tsx"
const checkOnly = process.argv.includes("--check")

// L joins lines so each anchor can be written with its real indentation and no escaping games.
const L = (...lines) => lines.join("\n")

const edits = [
  {
    name: "import AssigneePicker",
    find: 'import { tr, trFormal, setLocale, getLocale, getFormattingLocale, SUPPORTED } from "./i18n"',
    replace: L(
      'import { tr, trFormal, setLocale, getLocale, getFormattingLocale, SUPPORTED } from "./i18n"',
      '// Written for the members screen; the task form is the other place that needs it.',
      'import { AssigneePicker } from "./members"',
    ),
  },
  {
    // listId is optional on purpose: assignable users are per list, and "My tasks" / Timeline
    // open a task without knowing its list. There the field simply does not render, rather than
    // showing an empty or wrong set of people.
    name: "TaskModal signature",
    find: L(
      'export function TaskModal({ task, me, spaceId, onClose, onChanged }: {',
      '  task: Task; me: Me; spaceId?: number; onClose: () => void; onChanged: () => void',
      '}) {',
    ),
    replace: L(
      'export function TaskModal({ task, me, spaceId, listId, onClose, onChanged }: {',
      '  task: Task; me: Me; spaceId?: number; listId?: number; onClose: () => void; onChanged: () => void',
      '}) {',
    ),
  },
  {
    name: "assignee state",
    find: L(
      '  const [editBody, setEditBody] = useState("")',
      '  const [statuses, setStatuses] = useState<string[]>(DEFAULT_STATUSES)',
    ),
    replace: L(
      '  const [editBody, setEditBody] = useState("")',
      '  const [statuses, setStatuses] = useState<string[]>(DEFAULT_STATUSES)',
      '  const [assignee, setAssignee] = useState<number | null>(task.assignee_id ?? null)',
    ),
  },
  {
    name: "assignee field in the properties grid",
    find: L(
      '            <select className="input" value={priority} onChange={(e) => handlePriorityChange(e.target.value)}>',
      '              <option value="low">{tr("task.priority.low")}</option>',
      '              <option value="normal">{tr("task.priority.normal")}</option>',
      '              <option value="high">{tr("task.priority.high")}</option>',
      '              <option value="urgent">{tr("task.priority.urgent")}</option>',
      '            </select>',
      '          </div>',
    ),
    replace: L(
      '            <select className="input" value={priority} onChange={(e) => handlePriorityChange(e.target.value)}>',
      '              <option value="low">{tr("task.priority.low")}</option>',
      '              <option value="normal">{tr("task.priority.normal")}</option>',
      '              <option value="high">{tr("task.priority.high")}</option>',
      '              <option value="urgent">{tr("task.priority.urgent")}</option>',
      '            </select>',
      '          </div>',
      '',
      '          {/* Assignee. Only rendered when the list is known, because the candidate list',
      '              comes from that list\'s members. Clearing sends clear_assignee rather than a',
      '              null id, matching how the context menu already unassigns. */}',
      '          {listId !== undefined && (',
      '            <div>',
      '              <label className="muted row" style={{ fontSize: 12, marginBottom: 4, gap: 4 }}>',
      '                <IconUser size={12} /> {tr("task.assignee") || "Исполнитель"}',
      '              </label>',
      '              <AssigneePicker listId={listId} value={assignee} onChange={async (v) => {',
      '                setAssignee(v)',
      '                await updateTask(v === null ? { clear_assignee: true } : { assignee_id: v })',
      '              }} />',
      '            </div>',
      '          )}',
    ),
  },
  {
    name: "removeComment handler",
    find: '  async function react(commentId: number, emoji: string) {',
    replace: L(
      '  // Deletion is confirmed because it is not recoverable from the UI, and the error is shown',
      '  // rather than swallowed: a comment that silently stays after a failed delete looks like the',
      '  // button does nothing.',
      '  async function removeComment(id: number) {',
      '    confirm({',
      '      title: tr("task.comment_delete_confirm") || "Удалить комментарий?",',
      '      body: tr("confirm.archive_body"),',
      '      confirmLabel: tr("task.comment_delete") || "Удалить",',
      '      danger: true,',
      '      action: async () => {',
      '        try {',
      '          await api.del(`/api/comments/${id}`)',
      '          load()',
      '        } catch (err) {',
      '          setError((err as Error).message)',
      '        }',
      '      },',
      '    })',
      '  }',
      '',
      '  async function react(commentId: number, emoji: string) {',
    ),
  },
  {
    // Gated on the author only. The server may well also allow the list owner, but that was not
    // verified while writing this, and a button that 403s for some people is worse than a
    // missing one. Widen the condition once the handler's rule is confirmed.
    name: "delete button next to the edit button",
    find: L(
      '              {c.author_id === me.id && editingId !== c.id && (',
      '                <button className="nav-btn" style={{ marginLeft: "auto", fontSize: 12 }} onClick={() => startEdit(c)}>',
      '                  {tr("task.comment_edit")}',
      '                </button>',
      '              )}',
    ),
    replace: L(
      '              {c.author_id === me.id && editingId !== c.id && (',
      '                <button className="nav-btn" style={{ marginLeft: "auto", fontSize: 12 }} onClick={() => startEdit(c)}>',
      '                  {tr("task.comment_edit")}',
      '                </button>',
      '              )}',
      '              {c.author_id === me.id && editingId !== c.id && (',
      '                <button className="nav-btn" style={{ fontSize: 12, color: "var(--due-overdue)" }}',
      '                  onClick={() => removeComment(c.id)}>',
      '                  {tr("task.comment_delete") || "Удалить"}',
      '                </button>',
      '              )}',
    ),
  },
  {
    // Only the ListView call site knows the list. SpaceView's timeline modal and MyTasksPage
    // deliberately keep passing no listId.
    name: "pass listId from ListView",
    find: '      {open && <TaskModal task={open} me={me} spaceId={spaceId} onClose={() => setOpen(null)} onChanged={load} />}',
    replace: '      {open && <TaskModal task={open} me={me} spaceId={spaceId} listId={list.id} onClose={() => setOpen(null)} onChanged={load} />}',
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
  // A function replacer, so $ sequences inside the replacement (template literals such as
  // ${id}) are inserted literally instead of being read as capture-group references.
  out = out.replace(edit.find, () => edit.replace)
}

if (problems.length > 0) {
  console.error(`Refusing to modify ${FILE} — it is not the expected revision:`)
  for (const p of problems) console.error(`  ✗ ${p}`)
  console.error("No changes were written. Re-check the anchors against the current file.")
  process.exit(1)
}

for (const edit of edits) console.log(`  ✓ ${edit.name}`)

if (checkOnly) {
  console.log(`All ${edits.length} anchors matched. --check given, so ${FILE} was left untouched.`)
  process.exit(0)
}

await writeFile(FILE, out)
console.log(`\nApplied ${edits.length} edits to ${FILE}.`)
console.log("Next: npm run build, review git diff, commit, then delete this script.")
