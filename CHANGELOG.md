# Changelog

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
