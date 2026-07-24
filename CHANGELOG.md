# Changelog

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
