# Changelog

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

### Verification

- `go build ./...`, `go vet ./...`, `gofmt -l .`, and `go test ./...` (including a new
  `TestRoutesRegisterWithoutConflict` guarding against `net/http` mux pattern collisions when adding
  nested routes) all pass clean on every round in this pass.
- All 13 base locales carry identical key sets (verified by a script comparing key counts and
  diffing against every static and dynamic `tr()`/`trFormal()` call site in the frontend) — zero
  missing translations for any string introduced in this pass.
- The frontend still could not be `npm install`ed / built in this environment (no registry access),
  so TypeScript/React changes were verified by brace/paren balance checks and careful manual review,
  not by the compiler or a browser. Run `cd web && npm install && npm run build` and smoke-test
  before your next release.

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
