#!/usr/bin/env node
// One-shot codemod — delete this file once it has been run.
//
// Why a script instead of a normal commit: web/src/views.tsx is ~100 KB in one file and the tooling
// used to author this change can only replace a whole file, never a region. Re-emitting 100 KB from
// memory to change ~40 lines risks committing a silently truncated file. This codemod refuses to run
// unless every anchor matches exactly once, across every target file, before anything is written —
// so it either applies completely or leaves the tree byte-identical.
//
// What it fixes: a list could be created but never renamed or removed. PATCH /api/lists/{id} and
// DELETE /api/lists/{id} were both implemented, permission-checked, and called from nowhere.
//
// It also aligns one backend detail — see the spaces_lists.go target at the bottom. To skip that
// part, delete that one object from `targets`; the frontend edits stand on their own.
//
// Usage:
//   node scripts/patch-list-manage.mjs --check   # verify anchors, write nothing
//   node scripts/patch-list-manage.mjs           # apply

import { readFile, writeFile } from "node:fs/promises"

const checkOnly = process.argv.includes("--check")

// L joins lines so every anchor can be written with its real indentation and no escaping games.
const L = (...lines) => lines.join("\n")

const targets = [
  {
    file: "web/src/views.tsx",
    edits: [
      {
        name: "ListView: rename/archive state and handlers",
        find: L(
          '  const [menu, setMenu] = useState<{ task: Task; x: number; y: number } | null>(null)',
          '  const { confirm, confirmElement } = useConfirm()',
        ),
        replace: L(
          '  const [menu, setMenu] = useState<{ task: Task; x: number; y: number } | null>(null)',
          '  const { confirm, confirmElement } = useConfirm()',
          '',
          '  // Renaming and archiving a list. Both endpoints existed on the server from the start with',
          '  // no caller here, so a list could be created and then never corrected or removed.',
          '  const [renaming, setRenaming] = useState(false)',
          '  // The displayed name lives here rather than in the `list` prop: the prop is a snapshot owned',
          '  // by SpaceView, and mutating it would be a lie the parent never hears about. SpaceView',
          '  // reloads its lists on back-navigation, so the two converge on exit.',
          '  const [listName, setListName] = useState(list.name)',
          '  const [listError, setListError] = useState("")',
          '  // my_permission is what GET /api/spaces/{id}/lists sends for each list. Read through a cast',
          '  // so this compiles whether or not it has been declared on the List type yet.',
          '  const canManageList = (list as { my_permission?: string }).my_permission === "owner"',
          '    || me.role === "root" || me.role === "admin"',
          '',
          '  async function saveListName() {',
          '    const next = listName.trim()',
          '    // An empty name would be accepted by COALESCE and leave a nameless list, and renaming to',
          '    // the same string is not worth a request.',
          '    if (!next || next === list.name) { setListName(list.name); setRenaming(false); return }',
          '    setListError("")',
          '    try {',
          '      await api.patch(`/api/lists/${list.id}`, { name: next })',
          '      setRenaming(false)',
          '    } catch (err) {',
          '      setListError((err as Error).message)',
          '    }',
          '  }',
          '',
          '  function archiveList() {',
          '    confirm({',
          '      title: (tr("lists.archive_confirm") || "Архивировать список «{name}»?").replace("{name}", listName),',
          '      // Spelled out because it is not obvious and it is not reversible per-task: the server',
          '      // archives every task in the list too, and restoring the list brings all of them back.',
          '      body: tr("confirm.archive_body") + " " +',
          '        (tr("lists.archive_cascade_hint") || "Задачи списка тоже уйдут в архив и вернутся вместе с ним."),',
          '      confirmLabel: tr("task.archive"),',
          '      danger: true,',
          '      action: async () => {',
          '        try {',
          '          await api.del(`/api/lists/${list.id}`)',
          '          // The list is gone from this space now, so staying on its screen would show a view',
          '          // of something that no longer exists.',
          '          onBack()',
          '        } catch (err) {',
          '          setListError((err as Error).message)',
          '        }',
          '      },',
          '    })',
          '  }',
        ),
      },
      {
        name: "ListView: editable title + archive button in the header",
        find: '        <h2 style={{ margin: 0 }} className="grow">{list.name}</h2>',
        replace: L(
          '        {renaming ? (',
          '          <form className="row grow" style={{ gap: 4 }} onSubmit={(e) => { e.preventDefault(); saveListName() }}>',
          '            <input className="input grow" value={listName} autoFocus',
          '              onChange={(e) => setListName(e.target.value)}',
          '              onKeyDown={(e) => { if (e.key === "Escape") { setListName(list.name); setRenaming(false) } }} />',
          '            <button className="btn" type="submit">{tr("common.save")}</button>',
          '            <button className="nav-btn" type="button"',
          '              onClick={() => { setListName(list.name); setRenaming(false) }}>',
          '              {tr("common.cancel") || tr("task.comment_cancel")}',
          '            </button>',
          '          </form>',
          '        ) : (',
          '          // Double-click to rename mirrors how the rest of the app treats titles, but it is not',
          '          // the only way in — an invisible affordance is not a feature, hence the button below.',
          '          <h2 style={{ margin: 0 }} className="grow"',
          '            onDoubleClick={() => { if (canManageList) setRenaming(true) }}>{listName}</h2>',
          '        )}',
          '        {canManageList && !renaming && (',
          '          <div className="row" style={{ gap: 4 }}>',
          '            <button className="nav-btn" onClick={() => setRenaming(true)}>',
          '              {tr("lists.rename") || "Переименовать"}',
          '            </button>',
          '            <button className="nav-btn" style={{ color: "var(--due-overdue)" }}',
          '              title={tr("task.archive")} aria-label={tr("task.archive")} onClick={archiveList}>',
          '              <IconArchive size={14} />',
          '            </button>',
          '          </div>',
          '        )}',
        ),
      },
      {
        name: "ListView: surface rename/archive errors",
        find: '      {loadError && <p className="error-text">{tr("task.load_error")}: {loadError}</p>}',
        replace: L(
          '      {loadError && <p className="error-text">{tr("task.load_error")}: {loadError}</p>}',
          '      {listError && <p className="error-text">{listError}</p>}',
        ),
      },
    ],
  },
  {
    // Consistency fix, safe to drop by deleting this whole object.
    //
    // PATCH /api/spaces/{id} shallow-merges settings (`settings || $3`) specifically so that the
    // Pulse form cannot wipe the space's workflow. PATCH /api/lists/{id} replaces the object
    // wholesale (`COALESCE($3, settings)`). A list's settings has one writer today, so this is not
    // a live bug — it is the same bug already fixed once, sitting armed for the second writer.
    file: "internal/api/spaces_lists.go",
    edits: [
      {
        name: "handleUpdateList: shallow-merge settings like handleUpdateSpace",
        find: L(
          "\t_, err = a.DB.Pool.Exec(r.Context(), `",
          "\t\tUPDATE lists SET name=COALESCE($2,name), settings=COALESCE($3,settings), position=COALESCE($4,position)",
          "\t\tWHERE id=$1`, id, in.Name, in.Settings, in.Position)",
        ),
        replace: L(
          "\t// settings is shallow-merged (jsonb ||) rather than replaced, for the same reason as",
          "\t// handleUpdateSpace: each section of the object is edited by its own bit of UI, and a",
          "\t// whole-object write means whichever form saves last silently drops the others.",
          "\t_, err = a.DB.Pool.Exec(r.Context(), `",
          "\t\tUPDATE lists SET name=COALESCE($2,name),",
          "\t\t\tsettings = settings || COALESCE($3, '{}'::jsonb),",
          "\t\t\tposition=COALESCE($4,position)",
          "\t\tWHERE id=$1`, id, in.Name, in.Settings, in.Position)",
        ),
      },
    ],
  },
]

// Pass 1: read everything and verify every anchor before a single byte is written.
const planned = []
const problems = []

for (const target of targets) {
  let src
  try {
    src = await readFile(target.file, "utf8")
  } catch {
    problems.push(`${target.file}: cannot be read — run this from the repository root`)
    continue
  }
  let out = src
  for (const edit of target.edits) {
    const hits = out.split(edit.find).length - 1
    if (hits !== 1) {
      problems.push(`${target.file} — ${edit.name}: anchor matched ${hits} time(s), expected exactly 1`)
      continue
    }
    // A function replacer, so $ sequences in the replacement (template literals such as ${list.id},
    // and the SQL placeholders $1..$4) are inserted literally instead of being read as
    // capture-group references.
    out = out.replace(edit.find, () => edit.replace)
  }
  planned.push({ file: target.file, out, changed: out !== src })
}

if (problems.length > 0) {
  console.error("Refusing to modify anything — the tree is not the expected revision:")
  for (const p of problems) console.error(`  ✗ ${p}`)
  console.error("\nNo files were written.")
  process.exit(1)
}

for (const target of targets) {
  for (const edit of target.edits) console.log(`  ✓ ${target.file} — ${edit.name}`)
}

if (checkOnly) {
  console.log("\nAll anchors matched. --check given, so nothing was written.")
  process.exit(0)
}

// Pass 2: write.
for (const p of planned) {
  if (p.changed) await writeFile(p.file, p.out)
}

console.log("\nApplied. Next: npm run build --prefix web, go build ./..., review git diff,")
console.log("commit, then delete this script.")
