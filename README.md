# Todorio

**Your private workspace for tasks and teams.**

Todorio is a private, self-hosted task manager for personal use and small teams. It installs on a VPS with a single command, and runs without any external SaaS, email, or public API. Its flagship feature is **"Space Pulse"**: a live summary of your project's health.

## Features (v1)

- Login/password authentication (no email required), TOTP, manual registration or invite codes
- RBAC: root admin / admin / user / viewer, rate-limited login, registration modes (open/invite-only/closed)
- Spaces → lists → tasks (subtasks, dependencies, recurrence, checklists, custom fields)
- Four task views: List, Kanban board (drag & drop), Table, and "My Week"
- Progress bars, configurable workflow statuses, notes (Markdown pages), favorites
- Comments, @mentions, reactions (👍 ✅ 🎉 🔥 👀 ❓ ❗ ❌ 😭 ⭐), image attachments
- In-app notifications, due-date badges, "Do Not Disturb" mode, "while you were away" digest
- Focus mode / time tracking, global search across tasks/notes/comments, keyboard shortcuts (`?` for help)
- 13 locales (language-country format) + IT styles `ru-RU-it`, `en-US-it`
- 5 color themes (red/blue/green/yellow/gray), light + dark, "cozy"/"lite" density modes
- SSE realtime updates, working PWA install button (requires HTTPS)
- Statistics, dynamic labels, leaderboards, **Space Pulse**
- List templates, emergency announcements, list-level public read-only share links
- Everything configurable from the root panel **and** the terminal (`todorio server ...`) — same settings, either way

## Installation

```bash
curl -fsSL https://raw.githubusercontent.com/DanMotive/Todorio/main/scripts/install.sh | sudo bash
```

That's the whole install — one command. The script builds the backend and frontend, sets up PostgreSQL and a systemd service, and then runs `todorio setup` for you automatically and interactively (via `/dev/tty`, so prompts still work even though the script itself arrives through a pipe). Setup asks for: the root admin's username, port, HTTPS (self-signed, Let's Encrypt for your server's IP, or your own certificate), and whether to create the demo onboarding space with quests (y/n) — then it generates a 16-character temporary password, creates the root admin account, and starts the service.

If the environment has no attachable terminal (no `/dev/tty` — rare, e.g. some automated pipelines), the script stops short and tells you to finish manually with `sudo todorio setup && sudo systemctl enable --now todorio`. You'd also run `sudo todorio setup` by hand if you ever want to redo setup later (new port, HTTPS, etc.); to just reset the root password use `sudo todorio resetroot` instead.

To remove Todorio later, run `sudo todorio uninstall`. By default this removes the binary, service, and config; add `--saveconfig` to keep the config, or `--purge` to also delete application data and the database.

## Development

```bash
# Backend (Go 1.22+)
go run ./cmd/todorio serve --dev

# Frontend (Node 20+)
cd web && npm install && npm run dev
```

## Project structure

```
cmd/todorio/     — CLI and entry point (setup, serve, doctor, backup, server config)
internal/        — config, server (HTTP+SSE), setup
migrations/      — PostgreSQL SQL migrations
scripts/         — install.sh
web/             — React + Vite frontend, themes, locales, PWA
```

## License

Apache 2.0 · Developed by **DanMotive**
