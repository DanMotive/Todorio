# Todorio

**Your private workspace for tasks and teams.**

Todorio is a private, self-hosted task manager for personal use and small teams. It installs on a VPS with a single command, and runs without any external SaaS, email, or public API. Its flagship feature is **"Space Pulse"**: a live summary of your project's health.

## Features (v1)

- Login/password authentication (no email required), TOTP for any account, manual registration or invite codes
- RBAC: root admin / admin / user / viewer, rate-limited login, registration modes (open/invite-only/closed)
- A full personal settings page: profile/avatar, language, appearance, notification and Do Not
  Disturb preferences, deadline reminders, password/2FA — separate from the root-only server panel
- Spaces → lists → tasks (subtasks, dependencies, recurrence, checklists), with a per-space schema
  for typed custom fields (text/number/date/select/multiselect/checkbox/user/link/rating)
- Six task views: List, Kanban board (drag & drop), Table, Calendar, Timeline/Gantt (drag to
  reschedule or resize, critical-path highlighting), and "My Week" — plus saved, named filters
  (status/priority/overdue) that apply across all of them
- "My tasks" split into Today / Overdue / In review / No deadline / Mentions
- Progress bars, configurable workflow statuses, notes (Markdown pages), favorites
- Comments (with editing), @mentions, reactions, image attachments
- In-app notifications, due-date badges, "Do Not Disturb" mode, "while you were away" digest
- Optional Telegram delivery: root pastes in their own bot token (from @BotFather), each user
  links their own chat from their profile — fully opt-in, and off by default like every other
  external integration this project doesn't require
- An archive with restore, a 3-day warning before permanent cleanup, and root-only permanent delete
- Focus mode / time tracking, with live presence ("Ivan is working on this · 12m", not an emoji —
  a plain colored dot like Space Pulse's) visible to teammates on every task view
- Global search across tasks/notes/comments, keyboard shortcuts (`?` for help)
- 13 locales (language-country format) + IT styles `ru-RU-it`, `en-US-it`
- 5 color themes (red/blue/green/yellow/gray), light + dark, "cozy"/"lite" density modes
- SSE realtime updates, working PWA install button (requires HTTPS)
- Statistics, dynamic labels, leaderboards, **Space Pulse**
- List templates and emergency announcements (with admin-panel forms for both), list-level public read-only share links
- Everything configurable from the root panel **and** the terminal (`todorio server ...`) — same settings, either way
- Ships as a single self-contained binary (frontend + migrations embedded) for easy installs and updates

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/DanMotive/Todorio/main/scripts/install.sh | sudo bash
```

That's the whole install — one command. `todorio` ships as a single self-contained binary (the frontend and SQL migrations are embedded into it at release time — see `assets.go`), so the script just downloads the right binary for your OS/architecture from the latest [GitHub Release](https://github.com/DanMotive/Todorio/releases), verifies its SHA-256 checksum, sets up PostgreSQL and a systemd service, and then runs `todorio setup` for you automatically and interactively (via `/dev/tty`, so prompts still work even though the script itself arrives through a pipe). No Go or Node.js toolchain is ever installed on your server. Setup asks for: the root admin's username, port, HTTPS (self-signed, Let's Encrypt for your server's IP, or your own certificate), and whether to create the demo onboarding space with quests (y/n) — then it generates a 16-character temporary password, creates the root admin account, and starts the service.

If the environment has no attachable terminal (no `/dev/tty` — rare, e.g. some automated pipelines), the script stops short and tells you to finish manually with `sudo todorio setup && sudo systemctl enable --now todorio`. You'd also run `sudo todorio setup` by hand if you ever want to redo setup later (new port, HTTPS, etc.); to just reset the root password use `sudo todorio resetroot` instead.

Re-running `install.sh` on a machine that's already set up (`/etc/todorio/config.json` exists) skips setup entirely and just restarts the service with whatever binary is currently installed — it's safe to run again, e.g. right after building a new binary yourself. Setup only ever runs automatically on a genuinely fresh install; the only other way to trigger it is the explicit `sudo todorio setup`.

`sudo todorio update` later fetches the newest release the same way (download + checksum verify) and replaces the running binary in place — since migrations are embedded, any new ones ship and apply automatically on the next start, no separate file sync needed. Day-to-day service control doesn't need bare `systemctl`: `sudo todorio start` / `stop` / `restart` wrap the same systemd unit.

To remove Todorio later, run `sudo todorio uninstall`. By default this removes the binary, service, and config; add `--saveconfig` to keep the config, or `--purge` to also delete application data and the database.

## Development

```bash
# Backend only (Go 1.22+) — serves the API; the UI falls back to ./web/dist on
# disk if present, otherwise start the Vite dev server separately (below).
go run ./cmd/todorio serve --dev

# Frontend (Node 20+)
cd web && npm install && npm run dev
```

To build a single release-style binary with the real frontend embedded (rather than the on-disk fallback), build the frontend first, then the Go binary — same order the release workflow uses:

```bash
cd web && npm install && npm run build && cd ..
go build -o todorio ./cmd/todorio
```

A plain `go build ./...` without building the frontend first still works — it just embeds a placeholder (see `web/dist/placeholder.txt`) and the server falls back to disk paths for the UI at runtime.

## Releasing

Pushing a tag matching `v*` (e.g. `git tag v1.2.3 && git push origin v1.2.3`) triggers `.github/workflows/release.yml`, which builds the frontend once, builds `linux/amd64` and `linux/arm64` binaries with it embedded, generates `checksums.txt`, and publishes all three as assets on a GitHub Release. `scripts/install.sh` and `todorio update` both consume exactly those assets.

## Project structure

```
assets.go        — embeds web/dist and migrations/ into the binary (see above)
cmd/todorio/     — CLI and entry point (setup, serve, status, backup, update, server config)
internal/        — config, server (HTTP+SSE), setup, ops (status/backup/update/uninstall)
migrations/      — PostgreSQL SQL migrations (embedded into the binary)
scripts/         — install.sh
web/             — React + Vite frontend, themes, locales, PWA (built to web/dist, embedded into the binary)
.github/workflows/ — release.yml (builds and publishes the binaries scripts/install.sh downloads)
```

## License

Apache 2.0 · Developed by **DanMotive**
