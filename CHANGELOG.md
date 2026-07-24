# Changelog

## Unreleased — Gantt drag/resize, critical path, onboarding progress, attachment delete, browser notifications

No new migrations in this batch (the Timeline endpoint gained one extra `LEFT JOIN`-sourced field,
no schema change). `todorio testsql` now covers 56 queries against a real PostgreSQL 16.2.

### Added — drag-and-drop rescheduling on the Timeline / Gantt (spec section 12)

The chart was read-only: seeing a bar told you the schedule, changing it meant opening the task.
A bar's body can now be dragged to move it, and either edge dragged to resize it, both via native
Pointer Events (mouse and touch, single code path) with the pixel delta rounded to whole days at
the chart's current zoom level.

- **Moving** a bar (drag its body) is only offered when both ends are already real dates. A bar
  with only a deadline (or only a start) renders "implied" — hatched, per the existing convention
  — specifically so a guessed range never looks like one the user set. Letting a body-drag on such
  a bar silently invent the missing date would do exactly that, so the body is inert there; only
  the edges are live.
- **Resizing** either edge is always offered (regardless of implied state) and always writes
  exactly the one date that edge represents — dragging the blank edge of an implied bar is an
  unambiguous "set this date" action, unlike grabbing the middle.
- A live preview (day-accurate, no network calls mid-drag) follows the pointer; the PATCH fires
  once on release, and a plain click that never moved the pointer still opens the task — the two
  are told apart by whether the drag actually crossed a day boundary, not by a timer.
- Handles are hover-revealed (same treatment as the attachment remove button below) rather than
  permanently visible clutter, and only rendered for bars the caller can actually edit — the
  Timeline endpoint now reports `can_edit` per task, mirroring `listPermission(...) >= editor`.
  `handleUpdateTask` re-checks that permission itself regardless, so this is a UI affordance, not
  the security boundary.
- Verified end-to-end in a real headless browser against the live server: dragging a due-only
  bar's start handle sets exactly the expected `start_at` and leaves `due_at` untouched; moving a
  fully-scheduled bar shifts both ends by the same delta; a plain click still opens the task modal.

### Added — critical path highlighting on the Timeline (spec section 12)

An opt-in "Critical path" checkbox runs a textbook CPM forward/backward pass (earliest/latest
start and finish, then zero-slack) over whatever's currently loaded in the chart, and outlines the
zero-slack bars and the dependency arrows between them.

`blocked_by` is a free-form array with no DB constraint against cycles — `ops.go`'s own integrity
check exists for exactly that reason. The graph is topologically sorted with Kahn's algorithm
first; anything left over once the queue drains is part of a cycle and is excluded from the CPM
math entirely (with a small note shown to the user) rather than risking an infinite recursion or a
silently wrong slack value.

### Fixed — Timeline bars didn't open the task modal

`TimelineView` accepted an `onOpenTask` callback but `SpaceView` never passed one, so clicking a
bar did nothing at all — a pre-existing gap, not a regression from the drag work above. `SpaceView`
now fetches the full task on click and renders it through the same `TaskModal` every other view
uses, refreshing the space's lists/pulse on change like `ListView` already does.

### Added — browser system notifications (spec section 12: "push-уведомления браузера")

A lightweight implementation on purpose: the spec's own wording ("работает в открытой вкладке")
describes the `Notification` API firing from an open tab, not full Web Push — which would require
routing through an external push relay (FCM/Autopush), contradicting the project's no-external-
services stance. A toggle in profile settings requests permission and reflects the current state;
it's hidden entirely over plain HTTP or in a browser without the API, and unchecking it explains
(rather than pretends to do) that revoking a granted permission is the browser's own setting, not
this page's.

### Added — onboarding quest progress bar (spec section 12)

The guided quest list (and the "add it to a new list on approval" step) already existed with no
visible progress. `GET /api/onboarding/progress` reports done/total for the caller's own quest
list; a dismissible bar above "My Tasks" shows it and disappears once every quest is done or the
user dismisses it (remembered per-browser, not server-side — this is a one-time nudge, not a
setting worth a database column).

### Added — delete an attachment (spec section 7)

Attachments could be added but never removed short of deleting the whole task/comment. Each
thumbnail now has a hover-revealed remove button behind a confirmation dialog (reversible in the
sense that it's a deliberate two-step action, but the file itself is gone for good, so it isn't
wired through the type-to-confirm path — that's reserved for whole-task/list/space deletion).

## Unreleased — confirmations, bulk operations, right-click task menu

No new migrations in this batch — all frontend, on top of the schema from 0009/0010.

### Added — right-click menu on a task row

Changing one field used to mean opening the whole task modal. Right-clicking a row in the list
view now gives status, priority, a deadline (today / tomorrow / in a week / none), assign-or-
unassign, and "open task" — the case that prompted this, since the modal was both slow and (before
the timer became global) knocked the focus session off screen.

Rendered through a portal so a scrolling ancestor can't clip it, and clamped to the viewport so a
right-click near the bottom edge stays fully visible. The task's current status and priority are
highlighted, so the menu doubles as a read-out. Escape or any outside click dismisses it.

### Added — bulk operations

A selection checkbox on each row in the list view; selecting anything reveals an action bar for
status, priority, deadline (set or clear), assign-to-me, and archive. The bar only exists while
something is selected, so it never takes space it hasn't earned.

Changes are sent per task rather than through a bulk endpoint (there isn't one). That's deliberate:
a partial failure leaves the successful ones applied and reports how many didn't make it, instead
of silently rolling back work the user believes is done.

### Added — real confirmation dialogs (spec section 10)

`window.confirm` was doing this job in four places and missing entirely in several others. There
is now a proper dialog, and for genuinely irreversible actions it is **type-to-confirm**: the
button stays disabled until the exact name is typed.

- Type-to-confirm: permanently deleting a task, list, or space.
- Plain confirm: archiving a task, archiving a note, bulk archive, blocking a user, resetting
  someone's password — all previously one unguarded click.
- Type-to-confirm is reserved for the no-undo cases on purpose. Requiring it everywhere trains
  people to type past it without reading, which defeats the point.

## Unreleased — Markdown notes, watchers, review workflow, captions, portability

Second batch. Two new migrations (0009, 0010); 0001–0008 untouched. Every new SQL statement was
executed against a real PostgreSQL 16.2 before shipping — `todorio testsql` now covers 55 queries.

### Added — Markdown rendering for notes (spec section 12)

Notes were described as "Markdown pages" but the body was only ever shown in a textarea; the
markup was never rendered. There is now an Edit/Preview toggle, opening in preview when the note
already has content.

The renderer (`web/src/markdown.tsx`) is hand-written rather than a library, for a specific
reason: it emits **React elements, never HTML strings**, so there is no `dangerouslySetInnerHTML`
anywhere and note text cannot inject markup. Verified against a live attack payload in a real
browser — `<img onerror>` and `<script>` in a note produce zero elements and zero alerts, and
`[label](javascript:...)` is refused as a link while a normal https link still works. Unsupported
syntax degrades to plain text instead of vanishing.

Supported: headings, ordered/unordered lists, blockquotes, fenced and inline code, bold, italic,
strikethrough, links, horizontal rules, paragraphs.

### Added — watchers (spec section 5)

A task field the spec listed but that never existed. A watcher follows a task without owning it
and receives the same status/deadline/assignee/comment notifications. Watching grants no extra
access — you can only watch what you can already see, so it can't be used to subscribe into a
private list. Fan-out skips the actor and the assignee, so nobody is notified twice.

### Added — review workflow with accept/return (spec section 5)

"На проверке" was previously just a status string with no semantics. It now records a real
decision: who submitted, who decided, when, and why. Accepting completes the task (and stops any
focus timer on it); returning sends it back to in_progress with the reviewer's note. **A return
requires a reason** — sending work back with no explanation tells the author nothing.

### Added — threaded comment replies (spec section 7)

One level deep on purpose: replying to a reply attaches to the same thread root rather than
drifting indefinitely to the right. A parent is validated to belong to the same task, so a crafted
id can't graft a reply onto a conversation in a task the caller can't see.

### Added — dynamic stat captions for all 13 locales (spec section 14)

The caption engine worked but content covered only ru-RU (30 phrases) and en-US (10) across 4 of
7 categories, so 11 of 13 locales showed a blank caption. Migration 0010 adds **728 phrases** —
4 per part, per category, per locale — written idiomatically per language rather than translated
from English, as the spec requires. Tone is matched per category: nothing congratulatory appears
next to overdue work. No emoji.

### Added — export and import

A self-hosted product shouldn't be the thing that traps your data. `GET /api/spaces/{id}/export`
produces one readable JSON document (lists, tasks, subtasks, comments, notes, settings);
`POST /api/spaces/import` rebuilds it into a **new** space inside one transaction.

- Import always creates a new space rather than merging — a merge would have to guess whether a
  same-named list is the same list, and guessing wrong silently mangles real data.
- Assignees are stored as usernames and resolved against the destination server; an unknown user
  means the task arrives unassigned rather than pointing at a stranger.
- Attachment *metadata* is exported but not the bytes: base64-inlining images would turn a modest
  space into a hundred-megabyte file. The files themselves are covered by `todorio backup create`.

### Added — integrity checks in `todorio status`

Nine read-only checks for inconsistencies no single request would notice: orphaned list/space
members, tasks whose list is gone, subtasks with a missing parent, comments on deleted tasks,
stale `blocked_by` ids, **mutual dependency cycles** (previously creatable, and they render as
circular arrows on the Gantt), tasks starting after their deadline, focus sessions left open over
24 hours, and attachment rows whose file is missing from disk. Each reports a count and what to do
about it. Nothing is repaired automatically — the operator decides.

### Added — action rate limits (spec section 10)

`limits.actions.tasks_per_hour` and `limits.actions.uploads_per_hour`, both defaulting to 0
(unlimited), counted in hourly buckets per user. Guards against a broken script creating thousands
of rows. Fails open on a database hiccup: a guardrail shouldn't block legitimate work. Stale
buckets are pruned daily by the worker.

### Changed — notification bursts are collapsed

Editing a task's status, then its deadline, then its assignee within a minute produced three
separate bell entries. An unread notification of the same kind about the same task inside
`limits.notify.collapse_seconds` (default 120) is now refreshed in place. Only **unread** ones are
merged — rewriting something already seen would make the history unreliable.

### Added — confirmation before archiving a note

## Unreleased — focus timer stopped on task completion, About page content

### Fixed — the focus timer kept running after a task was completed

Nothing anywhere closed a task's focus session when the task was finished, so the sidebar clock
went on billing time against work that was already done.

Fixed server-side rather than in the UI, so it holds for every client and every path into
completion:

- `closeFocusForTask` ends every open session on a task — for **all** users, not just the
  caller: if two people were focused on it, both timers have to stop. Elapsed time already
  worked is preserved (`duration_seconds` is computed on close), so it still counts in stats.
- `closeFocusForTaskTree` covers a task **and its subtasks**, matching the archive cascade.
- Wired into completion (`status = done`) and archiving.
- The frontend fires a `todorio:focus-changed` event from all four completion paths (list
  checkbox, "My tasks" checkbox, the modal's status dropdown, the modal's archive button) so
  the sidebar clock disappears immediately instead of at its next 60-second poll.
- Two new `testsql` checks cover both queries — 34/34 now pass against real PostgreSQL.

### Changed — About page

- The version shown is whatever the build injects via `-ldflags` (unchanged): the release
  workflow passes the git tag, so tagging `v0.2.1` displays `v0.2.1` with no source edit.
- Added "Source code" and "Donate" rows, defaulting to
  `https://github.com/DanMotive/Todorio` and `https://boosty.to/danter1/about`.
- Both are root-editable branding settings (`branding.source_url`, `branding.donate_url`), so
  they can be changed or blanked — a blank value hides its row rather than showing a dead label.
- Only `http(s)` URLs are rendered as links. These fields are root-editable text, so a
  `javascript:` or `data:` URL typed into one must never become clickable.

## Unreleased — `todorio testsql`, focus timer fix

First of two batches. This one is about verification and a real bug, not new features.

### Added — `todorio testsql` (temporary diagnostic)

Every non-trivial SQL query in the product now runs against the real database on demand, inside
**one transaction that is always rolled back**. Writes are exercised so INSERT/UPDATE statements
are genuinely validated, but nothing is committed — safe to run against production.

This closes a risk that had been carried for several rounds: the development sandbox has no
PostgreSQL, so queries could only be checked by compiling the surrounding Go. `go build` cannot
catch a misspelled column or a broken JOIN — exactly the bug class that shipped once before
(`tasks.position` referenced by a query no migration created; every list read 500'd).

It verifies: 12 schema columns added by later migrations, the applied-migration list, and 32
queries covering tasks, weighted list progress, Pulse, Timeline, Inbox, stats/leaderboard,
attachments on both targets, templates with audience, focus sessions, profile reads/writes,
search, activity, archive, and settings upsert. Each check runs in its own savepoint, so one
failure doesn't abort the rest.

`internal/ops/testsql.go` keeps a copy of `taskSelect` (the original is unexported in another
package). A copy that silently drifts would be worse than no check at all, so a unit test
compares the two verbatim and fails the build if they diverge.

### Fixed — focus timer reset when leaving a task

`FocusWidget` lived inside the task modal and kept elapsed time in local component state.
Closing the modal unmounted it, so the timer appeared to reset even though the server session
was still open — the user reported this as "focus mode gets knocked off".

- New `GET /api/focus/current` returns the open session with a server-computed elapsed time, so
  the clock is correct across navigation, a reload, and even another device.
- New `GlobalFocusTimer` in the sidebar shows a ticking clock, the task name, and a stop button
  from anywhere in the app. `FocusWidget` is now only a start/stop control and holds no timing
  state of its own.
- The "Shortcuts" text button moved into the icon-button row next to rich/lite, sound, and
  logout.

## Unreleased — dark-only theme, Inbox with smart quick-add

Two changes on top of the previous pass: the light colour scheme was removed at the user's
request, and the last major unstarted spec feature — «Входящие» with quick-add parsing — landed.

### Removed — light colour scheme

Todorio is dark-only now. The `data-scheme` attribute is gone entirely; the base tokens moved
from `:root[data-scheme="dark"]` into plain `:root`, so the palette applies before any JS runs
and there's no flash of unstyled UI on a cold load.

- Removed the sidebar light/dark toggle, the scheme selector in Settings → Appearance, the
  `branding.default_scheme` server setting, `DefaultScheme` in config, the `theme.scheme` field
  in `/api/bootstrap`, and the four now-dead locale keys across all 13 locales.
- New migration `0008_drop_theme_scheme.sql` drops `users.theme_scheme`. This was confirmed
  rather than assumed — the column held real per-user preferences on the live database, so
  dropping it is irreversible. `IF EXISTS` keeps the migration idempotent.
- A theme cached in localStorage by an older build still carries a `scheme` key; it's stripped
  on read rather than being spread back into state and re-persisted.
- Also removed the light-mode `--due-*` variants, which existed only to hit WCAG contrast on a
  white card, and a stale hardcoded `data-scheme="dark"` in `index.html`.
- The rich/lite visual mode toggle is a separate feature and is untouched.

### Added — Inbox and smart quick-add (spec sections 5 and 12)

`GET /api/inbox` is a cross-space triage list, deliberately distinct from "My tasks": that view
is deadline-sorted, which pushes undated and unassigned work to the bottom or hides it. Every
item carries a `reason` (`review` / `mentioned` / `assigned` / `unassigned`) so the UI groups
them into short labelled sections instead of one undifferentiated pile.

The quick-add parser (`internal/api/quickadd.go`) recognises `#tag`, `!priority`, `@user`, and
dates — relative words (`today`/`сегодня`, `tomorrow`/`завтра`), weekday names, `dd.mm`,
`dd.mm.yyyy`, and `yyyy-mm-dd` — in both English and Russian.

It is deliberately conservative: a token is consumed **only** when it resolves to something
real. `@nobody` who isn't a visible member stays in the title as literal text and is reported
back in `unresolved`; `!bogus` is left alone because it isn't a priority. Silently eating part
of what someone typed would leave them with a title they never wrote and no way to know why.

- Parsing is opt-in per request (`"parse": true`) so a title that legitimately contains `#1`
  isn't rewritten unexpectedly. Explicit fields always beat parsed ones.
- Parsed `#tags` land in the `labels` multiselect custom field — the product has no separate
  label system by design.
- 10 unit tests cover the edge cases: `31.02` is rejected rather than rolling over to 03.03,
  `v1.2.3` isn't mistaken for a date, a weekday always resolves to the *next* one, deadlines
  land at 23:59 so a task isn't overdue all the day it's due, and plain text is untouched.
- Inbox gets a sidebar entry and the `i` hotkey; the quick-add syntax is advertised under the
  task field, since a parser nobody knows about gets no use.

## Unreleased — task-creation bugfix, Space Pulse completion, mobile layout, and the Timeline/Gantt view

Four rounds of work in one pass: a confirmed P0 bugfix, finishing the product's headline feature,
a real mobile layout, and the last unstarted spec view. No `.git` history was touched and no
existing migration was edited — only source files plus one new numbered migration.

### Fixed — "database error" when creating a task (P0)

The root cause was **not** a permissions problem, which is what two earlier rounds assumed.
`handleCreateTask` passed the decoded `*string` description straight into the insert. The ListView
quick-add form only sends `title` and `due_at`, so `description` decoded to `nil`, pgx bound SQL
`NULL`, and the column is `description TEXT NOT NULL` — a guaranteed failure for **any** user
creating a task from the main form, root included. `notes.go` already handled this correctly for its
own NOT NULL column; tasks now do the same.

- Added `dbFail()`: the real database error is written to the server log while the client still gets
  a deliberately vague `"database error"`. A blind 500 is undiagnosable, which is exactly why this
  bug survived two rounds of investigation. Applied across the handlers touched in this pass.
- Fixed `PATCH /api/spaces/{id}` replacing the whole `settings` object. Space settings hold several
  independent sections (workflow, fields, rankings, pulse) edited by different bits of UI, so the
  new Pulse settings form would have silently wiped a space's workflow. Now a shallow `jsonb ||`
  merge, matching how `notify_prefs` already worked.
- Fixed modals rendering behind the page footer: `.card` carries `backdrop-filter` in "rich" visual
  mode, which creates a stacking context that trapped the modal's `z-index`. Modals now render
  through a portal into `<body>` (new `ModalShell`), which also brought Escape-to-close and
  scroll locking. This was a pre-existing desktop bug, not a mobile-only one.
- Links had no styling at all and fell back to the browser default `#0000EE`, effectively unreadable
  on the dark surface (visible on the developer credit and About page). They now use the
  contrast-audited accent color.

### Added — Timeline / Gantt view (spec section 12)

The last spec view with no implementation. Built from CSS + one SVG overlay rather than a charting
dependency, keeping the single-binary bundle small.

- New migration `0007_task_start_at.sql`: `tasks.start_at` (nullable) plus a partial index for the
  date-window scan. A Gantt bar needs both ends and tasks only had `due_at`.
- `GET /api/spaces/{id}/timeline` with a date window, optional single-list filter, dependency links,
  and a count of unschedulable tasks. Ranges the server derives from a deadline alone are flagged
  `implied` and drawn hatched, so a guessed date never looks like one the user entered.
- Dependency arrows are returned only when both endpoints are inside the window — an arrow to an
  off-screen bar has nowhere to point.
- Day/week/month zoom, date ticks with gridlines, a today marker, and dotted edges on bars that
  extend past the visible window.
- Start and deadline date pickers in the task modal (it previously had no date editor at all), with
  server-side validation that a start can't fall after the deadline.

### Added — Space Pulse, completed (spec section 17)

- Per-space settings in `spaces.settings->pulse`: stall threshold, the green/yellow/red score
  bounds, and which signals to track. A disabled signal no longer drags the score down.
- The daily mini-standup ("what I did / am doing / is blocking me") and the "next best action"
  suggestion from the spec mockup, ranked unblock > assign > schedule > chase-overdue.
- Owner-facing settings form; root keeps the existing global `pulse.enabled` kill switch.

### Added — remaining spec gaps

- Manual progress slider for tasks without subtasks, plus a "by weight" list progress mode
  (spec section 6). `clear_progress` was needed because `COALESCE` can't distinguish a `null` from
  an omitted field.
- Attachments on comments, with the per-target limits from the spec (10/task, 5/comment). The
  file-serving handler was hardcoded to `target_type='task'` and is now generic — it resolves a
  comment attachment through its task for the same permission check.
- "About" page with version and developer credit, `branding.developer_url`, and a
  `show_product_name` footer toggle.
- Root logo upload replacing the hardcoded SVG. SVG is accepted (it's the right format for a logo)
  but served with a restrictive CSP and `nosniff` so uploaded markup can't execute in the site's
  origin.
- Leaderboard visibility: full table / top 3 / own place / owner only (spec section 14), filtered
  **server-side** — "owner only" has to mean the rows never reach anyone else's browser.
- Template audience: all users / specific roles / admins only (spec section 16), enforced when
  listing, when applying by id, and on the auto-apply path for newly approved users.

### Changed — no emoji in the interface

Per an explicit product rule, emoji were removed from every interface string: the `my.empty` key in
all 13 locales, four keys in each IT-slang overlay, the onboarding quest titles, and the generated
demo space/list names. The fixed reaction set is user content and stays. The typographic arrow in
`task.system.status_changed` is not an emoji and was kept.

- The PWA "Install app" button moved from the sidebar into Settings → Appearance. `beforeinstallprompt`
  is now captured in `main.tsx`, since the browser fires it once before the settings page mounts.

### Added — mobile layout (spec section 20)

- The sidebar becomes an off-canvas drawer under 860px with a hamburger top bar; a closed drawer is
  removed from the tab order via `inert` so it isn't keyboard-reachable.
- Reflowed the card headers, task-properties grid, and Pulse header for narrow screens; the space tab
  strip scrolls horizontally rather than wrapping mid-strip.

### Added — i18n verification script

`scripts/check_i18n.py` checks that all 13 base locales carry an identical key set, that no interface
string contains emoji, and that every `tr()`/`trFormal()` call in the TSX resolves to a real key
(including dynamic `tr("prefix." + var)` forms). It immediately caught a `common.save` key that was
referenced but never defined.

## Unreleased — client settings, spec audit, and a full feature-parity push

This pass covered three rounds of work: (1) a full client-facing settings page, (2) a systematic
audit of all 22 spec sections against the actual code, and (3) closing the highest-value gaps that
audit found — a data-loss risk in the archive flow, a switch to self-contained release binaries,
opening 2FA to every account, a Calendar view, and five more features chosen by priority (schema-
driven custom fields, "My tasks" sub-views, saved filters, admin forms for templates/announcements,
and a bugfix bundle). No `.git` history was touched — only source files, per usual for this project.

### Added — client settings page (spec sections 7, 8, 9, 18, 20)

- A full per-user settings page (distinct from the root-only server settings panel): profile/avatar,
  language + IT-slang toggle, appearance (color/scheme/visual mode), notification preferences
  (per-type on/off, sound, Do Not Disturb), deadline reminder preferences (7/3/1 days before, on the
  due day, daily while overdue), and a password-change form.
- Forced password change on first login (`must_change_password`), blocking the app until the user
  sets their own password.
- Avatar upload/remove (`internal/api/avatar.go`, new) with image sniffing and size limits.
- Fixed `notify_prefs` being parsed but silently discarded on save (same bug class as an earlier
  `permissions`-discard bug); fixed `/api/me` not returning the profile at all, so settings never
  synced across devices; added the `status_changed`/`due_changed` notifications the spec requires on
  task updates but that were never sent; rewrote the deadline worker to honor each user's own
  reminder preferences instead of one hardcoded global sweep.
- **Critical fix**: `tasks.position` was referenced throughout `tasks.go` but no migration ever
  created that column — `GET /api/lists/{id}/tasks` 500'd on every call, silently swallowed by the
  frontend. This explained reports of "tasks not appearing in lists." Fixed via an additive
  migration (`0005_task_position.sql`) since 0001–0004 were already applied on the live deployment.

### Added — self-contained single-binary distribution

- `assets.go` (new, module root) embeds `migrations/*.sql` and the built `web/dist` directly into
  the `todorio` binary via `go:embed`, matching the spec's "single Go binary — API, frontend, and
  migrations" intent. Migrations are always embedded (they're committed to the repo); the frontend
  embed is real whenever `npm run build` ran before `go build` (as the release pipeline does) and
  falls back to on-disk serving otherwise, so a plain backend-only `go build` still works.
- `.github/workflows/release.yml` (new): builds the frontend once, cross-compiles `linux/amd64` and
  `linux/arm64` binaries with it embedded, generates `checksums.txt`, and publishes all three to a
  GitHub Release on every `v*` tag.
- `scripts/install.sh` rewritten: downloads the matching release binary + verifies its SHA-256
  instead of cloning the repo and building from source on the target server. No Go/Node toolchain is
  installed on the server anymore. `todorio update` benefits too — since migrations are now baked
  into the binary it downloads, updates can no longer leave new migrations un-applied (previously,
  `update` only replaced the binary and never touched the separately-installed migrations directory).

### Added — archive safety net (spec section 11)

- Tasks, lists, and spaces can now be **restored** from the archive (`POST .../restore`), fully
  resetting the 30-day auto-cleanup countdown — previously the only way out of the archive was
  automatic permanent deletion.
- A **3-day warning** notification (`archive_expiring`) before an archived item is permanently
  deleted, sent to whoever archived it (tracked via a new `archived_by` column).
- **Permanent delete** (`DELETE .../permanent`), root-only, and only for items already archived —
  a genuine "gone forever" action distinct from archiving, with a confirmation dialog in the UI.
- A new "Archive" tab per space (restore/delete archived lists and tasks) and an "Archived spaces"
  panel from the Spaces page.

### Added — five more features, chosen by priority after the spec audit

- **Calendar view**: a month grid showing tasks on their due date, as a 4th view mode alongside
  List/Kanban/Table. (Timeline/Gantt was deliberately left out of this pass — it's a materially
  bigger feature, closer to a mini project-scheduling UI than the other views, and building it
  quickly would likely mean a version worth redoing rather than a version worth shipping.)
- **2FA opened to every account** — TOTP setup/enable/disable was gated behind admin/root even
  though the spec calls it out as "especially important for root," not root-exclusive. Moved the
  TOTP card from the admin panel into the personal settings page's Security section.
- **Typed custom fields wired into the task modal** (spec section 13): the space-level field-schema
  editor (`GET/PUT /api/spaces/{id}/fields`) already existed on the backend with zero UI; added a
  "Fields" tab per space to define fields (text/number/date/select/multiselect/checkbox/user/
  link/rating), and the task modal now renders a real typed control per defined field instead of a
  freeform key/value box (which still exists underneath for anything outside the schema).
- **Saved filters UI** (spec section 12): another backend that existed with no frontend
  (`internal/api/filters.go`). Added a filter bar to the list view — status/priority/overdue-only,
  named and savable, applied client-side across all four view modes.
- **"My tasks" sub-views** (spec section 12): split the flat task list into Today / Overdue / In
  review / No deadline / Mentions, matching the spec's named subsections. "Mentions" reuses the
  existing search endpoint (`?q=@username`) rather than a new backend query.
- **Admin forms for templates and announcements**: both had complete backends
  (`internal/api/templates.go`, `announcements.go`) and zero UI — root had to hit the API directly
  to use either. Added a template builder (name, target list name, per-task title/priority/due-in-
  days rows, auto-apply toggle) and an announcement composer (level, body, expiry, read-receipt
  requirement) to the admin panel.
- **Bugfix bundle**: `pulse.enabled` in root settings didn't actually disable the Pulse endpoint;
  fixed. Login lockout window was 10 minutes against a spec'd 15; fixed. Dates/times rendered with
  the browser's locale instead of the user's selected app language; fixed throughout. The admin
  panel could render in IT-slang style if the viewing admin had it selected, which the spec
  explicitly says should stay formal to avoid ambiguity; added `trFormal()` (i18n.ts) and applied it
  to `AdminPage`/`InvitesCard`/`ServerSettingsCard`. Comment editing (`comments.edited_at` existed in
  the schema, unused) is now wired end-to-end with an "(edited)" marker.

### Fixed — follow-up round (user-reported)

- **`toLocaleDateString`/`toLocaleString` crashed whenever IT-slang style was active.** The above
  "dates follow the app language" fix passed `getLocale()` straight into the browser's Intl API, but
  `getLocale()` can return `"ru-RU-it"`/`"en-US-it"` — our own convention for picking the slang
  translation pack, not a real BCP-47 tag — which throws `RangeError: Incorrect locale information
  provided` (reproduced directly in Node). This broke rendering on any page showing a date while
  IT-style was selected. Added `getFormattingLocale()` (i18n.ts), which strips the `-it` suffix
  before any Intl call; replaced all 10 call sites.
- **Task creation silently "failed"**: the new-task form ignored errors (`.catch(() => {})`) and
  unconditionally cleared its inputs regardless of whether the request actually succeeded, so a
  failed `POST` looked identical to "it worked" — no task appeared, no error, no clue why. Same
  failure shape as an earlier, real bug in this project (a missing DB column reported as "tasks
  aren't created"). `ListView` now surfaces the actual server error under the form (and under the
  list, if loading tasks fails) instead of swallowing it.
- **`todorio start` / `stop` / `restart`**: new commands wrapping `systemctl <action> todorio`, so
  day-to-day service control doesn't require remembering the unit name.
- **`install.sh` no longer re-prompts setup on an already-configured machine.** It used to run
  `todorio setup` unconditionally every time, including when re-run just to pick up a freshly built
  binary. It now skips straight to restarting the service if `/etc/todorio/config.json` already
  exists; setup only runs automatically on a genuinely fresh install, or explicitly via
  `sudo todorio setup`.

### Added — more spec sections closed out

- **Personal statistics** (spec section 14): `GET /api/my/stats` aggregates completed/on-time/
  overdue counts, most-active list, and focus time across *all* of a user's spaces (not just one),
  surfaced as a new "Stats" tab on the "My tasks" page. `/api/focus/stats` existed before with no
  UI; this supersedes it for that purpose.
- **Task version history** (spec section 11): `task_versions` has recorded a snapshot on every
  edit since early on, but nothing ever read it back. Added `GET /api/tasks/{id}/versions` and
  `POST .../versions/{id}/restore` (itself undoable — restoring snapshots the pre-restore state
  too), plus a "History" section in the task modal.
- **System records in the task feed** (spec section 7): status/deadline/assignee changes were
  previously only ever a notification to the assignee. `comments.is_system` existed but was never
  written; `handleUpdateTask` now also inserts a small structured system comment for each of these
  three change types, visible to everyone looking at the task (not just whoever's assigned), and
  rendered by the frontend with `tr()` so it's never frozen in one editor's language.
- **`0 = unlimited` actually means unlimited now** (spec section 10): every existing limit
  (attachment size, login attempts, comment length) previously treated an explicitly-configured `0`
  identically to "not configured" and silently fell back to the hardcoded default. Added a shared
  `intSetting()` helper that tells the two apart; `0` now truly disables each check.
- **Four more configurable limits** (spec section 10 examples): max lists per user, max tasks per
  list, max comments per task, max concurrent sessions per user (oldest session is evicted rather
  than rejecting the login outright). All default to `0` (unlimited) so upgrading never suddenly
  caps an existing instance.
- **Global feature toggles** (spec section 10): `policy.features.{comments,reactions,attachments,
  versions,stats}` — each independently switchable off instance-wide from the root settings panel.
  All default to `true` (opt-out, not opt-in).
- **Total storage quota** (spec section 10 example: "20 GB total"): a new
  `limits.uploads.max_total_storage_mb` setting (0 = unlimited, the default) caps the combined size
  of every task attachment and avatar under the uploads directory — once the whole folder would
  exceed it, further uploads are rejected with `507 Insufficient Storage` instead of silently
  filling the disk. Checked with a plain recursive directory walk rather than a DB-tracked running
  total, so it can't drift from what's actually on disk. `todorio status` now also reports current
  uploads usage in MB.

### Fixed — release pipeline build failure

- The first real GitHub Actions release run (triggered after writing the release guide in this
  pass) failed at `tsc -b`: `IconX`'s call site in the new saved-filters bar (`FiltersBar`) passed
  an `onClick` handler, but `IconProps` never declared one, so the type checker rejected it —
  exactly the class of error this sandboxed environment can't catch without a local `tsc`. Fixed
  at the source rather than at the call site: `IconProps` now accepts an optional `onClick` and
  `base()` forwards it to the underlying `<svg>`, so every icon in the set supports a click handler
  going forward, not just `IconX`. Confirmed by grep that this was the only call site affected.

### Added — task presence ("who's working on this right now")

- Starting a focus session on a task (the existing focus-mode timer, spec section on time
  tracking) now doubles as a live presence signal. Every task read (list, kanban, table, my tasks,
  my week, the task modal) carries an `active_focus` array of whoever currently has that task open
  in focus mode, and when they started — a single `json_agg` subquery on `taskSelect`, so every
  existing endpoint picked it up for free instead of needing a new one. Ending a session, or
  starting a new one on a different task, closes the old one automatically, exactly as before.
- Shown as a small colored dot + "*{name} is working on this · {time}*" caption: full caption on
  list rows/kanban cards/my-tasks/my-week, dot-only with names on hover in the compact table view,
  full caption again (auto-refreshed every 20s while open) in the task modal. Deliberately **not an
  emoji** — the dot reuses the same plain-colored-indicator language as the Space Pulse mood dot
  (see `pulse.go`'s own comment: avoiding the OS/browser emoji font is exactly why Pulse used a CSS
  dot instead of 🟢/🟡/🔴 — this is the same call, made consistently across the app).
- `focus.started` / `focus.stopped` are also broadcast on the existing SSE bus to everyone with
  access to the task's list, so the plumbing is ready for instant cross-tab updates later. The
  shipped frontend doesn't subscribe to them yet — it refreshes on normal reloads plus the task
  modal's 20s poll, which is enough freshness for a presence caption without restructuring the
  single app-wide `EventSource` connection. A natural, self-contained follow-up if instant
  cross-tab presence is ever wanted.

### Verification

- `go build ./...`, `go vet ./...`, `gofmt -l .`, and `go test ./...` (including
  `TestRoutesRegisterWithoutConflict`) all pass clean on every round in this pass, including the
  two rounds above.
- All 13 base locales carry identical key sets (verified by a script comparing key counts and
  diffing against every static and dynamic `tr()`/`trFormal()` call site in the frontend) — zero
  missing translations for any string introduced in this pass, including this round's 6 new
  `focus.presence.*` keys.
- The frontend still could not be fully `npm install`ed / built in this environment — the sandbox's
  network access is now (as of this round) requested for `registry.npmjs.org` specifically to close
  this gap, pending approval. Until it's granted, TypeScript/React changes are verified by
  brace/paren balance checks plus careful manual review, not by the compiler or a browser. Run
  `cd web && npm install && npm run build` and smoke-test before your next release — this is
  exactly the step that would have caught the `IconX` build failure above before it reached CI.

## Unreleased — stabilization pass

This pass focused on closing the gap between the technical spec (`Todorio — Техническое задание.md`)
and the actual code, and on fixing several features that were wired on one side (frontend or
database) but not the other. No `.git` history was touched — only source files.

### Fixed (previously broken in normal use)

- **Registration was broken for everyone.** The web sign-up form always sent an `invite_code`
  field, but the backend rejected any request with an unrecognized JSON field — every signup via
  the UI failed with 400, including the very first (root-bootstrap) registration.
- **Invite codes had no backend.** The `invites` database table and the admin "Invites" card
  existed, but there was no `/api/invites` route at all (404 on every use). Implemented
  `internal/api/invites.go` (list/create/delete) and wired invite-code redemption into
  registration, including the `invite_only` registration mode from the spec.
- **The task detail modal's recurrence and custom-fields controls 400'd on every use** — the
  backend's update handler didn't accept those fields (`DisallowUnknownFields` rejected them).
  Both are now read and written correctly, including on the task list/detail responses.
- **Task status vocabulary was inconsistent in three places**: the database default (`new`), the
  workflow engine's defaults (`open/in_progress/review/done`), and the task modal's hardcoded
  dropdown (`todo/in_progress/done`, English-only, untranslated). Unified on the workflow engine's
  vocabulary; the task modal now fetches the space's real (possibly custom) statuses.
- **Login had no rate limiting in practice.** The limiter (`internal/api/ratelimit.go`) was fully
  implemented but never called from `handleLogin`. Now wired in, keyed by IP + username.
- **The PWA install button never rendered** — the `beforeinstallprompt` event was captured into
  state but nothing in the UI used it. Added a working "Install app" button in the sidebar.
- `todorio server config|policy|limits|branding|locales set/enable/disable` was a stub that only
  printed `TODO: write to system_settings`. It now actually persists to the database — the same
  table a new web-based root settings panel (Admin → Server settings) reads and writes.
- Archiving a list no longer leaves its tasks live and reachable via `/api/my/tasks` /
  `GET /api/tasks/{id}` — it now cascades, matching how archiving a task cascades to its subtasks.
- Reacting to a task/comment now checks list permission (previously any logged-in user could react
  to content in lists they had no access to).
- Comment length limit now reads `limits.content.comment_max_len` from settings instead of a
  hardcoded `4000`.
- Fine-grained `permissions` passed on user approval were parsed and silently discarded; they're
  now persisted to `users.permissions`.
- Normalized `LICENSE` back to LF line endings (a local edit had converted it to CRLF).

### Added

- **Kanban board and Table views** for lists (in addition to the existing flat List view), plus a
  **"My Week"** view on "My Tasks" (grouped by day, with an overdue bucket). All three reuse
  existing backend endpoints — no new task-storage logic was needed for the frontend.
- **Notes UI** (space-scoped Markdown pages), **Favorites** (star toggle on tasks + a My Tasks
  view), **global search** (tasks/notes/comments), and an **activity feed** per space — the
  backends for all of these already existed but had no UI.
- **Focus mode timer** (start/stop, live elapsed time) inside the task modal.
- **Server settings panel** in the web admin UI (branding, registration/sharing/space-creation
  policy, upload/comment/login limits, per-locale enable/disable) — previously only the CLI could
  touch these, and the CLI itself didn't actually work (see above).
- **Keyboard shortcuts** (`m`/`s`/`/`/`n`, `?` for a help dialog, disableable), ignored while
  typing in a field, per spec section 12.
- A minimal template-apply picker in the space view (backend already supported templates).
- `go.sum` generated and committed to the working tree (`go.mod` already instructed `go mod
  tidy` after cloning — this pins the exact versions this pass was tested against).

### Known remaining gaps (not attempted or only partially done in this pass)

- **Timeline/Gantt view** (spec section 12) — not implemented; List/Kanban/Table/My Week cover the
  rest. A month **Calendar** view is also not implemented.
- **Typed custom fields** (spec section 13: text/number/date/select/multiselect/checkbox/user/
  link/rating, configured per space) — the backend schema (`fields.go`) and storage exist, but the
  task modal still uses a simpler free-text key/value editor rather than a per-space typed field
  builder tied to `GET/PUT /api/spaces/{id}/fields`.
  - Saved filters UI (backend exists in `filters.go`, no frontend yet).
- Docker Compose / PM2 process-manager choice at install time (spec section 3) — `scripts/install.sh`
  only sets up systemd; the README already reflects this as the current, intentionally-scoped
  behavior rather than a bug.
- Translations beyond `en-US`/`ru-RU` (11 locales + 2 IT-slang overlays) were extended by a
  bilingual pass for this update's new strings, not reviewed by native speakers — worth a native
  proofread pass before treating them as production-quality.
- The Go backend was compiled and vetted clean (`go build ./...`, `go vet ./...`) during this pass.
  The frontend could **not** be `npm install`ed / type-checked / built in this environment (no
  registry access), so the TypeScript/React changes were reviewed by hand but not compiler- or
  browser-verified. Run `cd web && npm install && npm run build` (and smoke-test in a browser)
  before treating the frontend as verified.
