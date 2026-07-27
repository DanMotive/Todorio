<p align="center">
  <img src="web/public/icons/icon-192.png" alt="Todorio Logo" width="128"/>
</p>

<h1 align="center">Todorio</h1>

<p align="center">
  A private, self-hosted task manager for personal use and teams.
</p>

<p align="center">
  <a href="https://github.com/DanMotive/Todorio/releases"><img src="https://img.shields.io/github/v/release/DanMotive/Todorio?style=flat-square&color=blue" alt="Latest Release"></a>
  <a href="https://github.com/DanMotive/Todorio/releases"><img src="https://img.shields.io/github/downloads/DanMotive/Todorio/total?style=flat-square&color=success" alt="Total Downloads"></a>
  <a href="https://github.com/DanMotive/Todorio/stargazers"><img src="https://img.shields.io/github/stars/DanMotive/Todorio?style=flat-square&color=gold" alt="GitHub Stars"></a>
  <a href="https://github.com/DanMotive/Todorio/blob/main/LICENSE"><img src="https://img.shields.io/github/license/DanMotive/Todorio?style=flat-square&color=green" alt="License"></a>
  <a href="https://github.com/DanMotive/Todorio/actions"><img src="https://img.shields.io/github/actions/workflow/status/DanMotive/Todorio/release.yml?style=flat-square" alt="Build Status"></a>
  <img src="https://img.shields.io/badge/Go-1.22+-00ADD8?style=flat-square&logo=go&logoColor=white" alt="Go Version">
</p>

---

Todorio installs on a VPS with a single command and runs without any external SaaS, email, or public API. Its flagship feature is **"Space Pulse"**: a live summary of your project's health.

## Table of Contents
- [Preview](#preview)
- [Features](#features-v100)
- [System Requirements](#system-requirements)
- [Installation](#installation)
- [CLI Quick Reference](#cli-quick-reference)
- [Development](#development)
- [Releasing](#releasing)
- [Project Structure](#project-structure)
- [Contributing](#contributing)
- [License](#license)

---

## Preview

<!-- Replace the placeholder paths below with your actual screenshot URLs -->
![Todorio Overview](web/public/icons/icon-192.png)

<details>
  <summary>📸 Click to view more screenshots & UI views</summary>
  <br/>

  ### Kanban Board
  <img width="1600" height="860" alt="image" src="https://github.com/user-attachments/assets/ad2b52da-fc77-4cc1-a014-972980342d29" />


  ### Configuration
  <img width="1600" height="860" alt="image" src="https://github.com/user-attachments/assets/5e1891eb-1ec2-4c84-898a-1f90c2491808" />


  ### Space Pulse
  <img width="1600" height="860" alt="image" src="https://github.com/user-attachments/assets/40d8800b-9e82-42c4-b7a1-c697a515996c" />

</details>

---

## Features (v1.2.0)

- **Authentication & Security:** Login/password authentication (no email required), TOTP for any account, manual registration or invite codes.
- **RBAC:** Root admin / admin / user / viewer, rate-limited login, registration modes (open/invite-only/closed).
- **Personal Settings:** Profile/avatar, language, appearance, notification and Do Not Disturb preferences, deadline reminders, password/2FA — separate from the root-only server panel.
- **Task Structure:** Spaces → lists → tasks (subtasks, dependencies, recurrence, checklists), with a per-space schema for typed custom fields (*text/number/date/select/multiselect/checkbox/user/link/rating*).
- **Six Task Views:** List, Kanban board (drag & drop), Table, Calendar, Timeline/Gantt (drag to reschedule or resize, critical-path highlighting), and "My Week" — plus saved, named filters (*status/priority/overdue*) that apply across all of them.
- **My Tasks Dashboard:** Split into Today / Overdue / In review / No deadline / Mentions.
- **Organization & Collaboration:** Progress bars, configurable workflow statuses, notes (Markdown pages), favorites, comments (with editing), @mentions, reactions (👍 ✅ 🎉 🔥 👀 ❓ ❗ ❌ 😭 ⭐), and image attachments.
- **Notifications & Digest:** In-app notifications, due-date badges, "Do Not Disturb" mode, "while you were away" digest.
- **Optional Telegram Delivery:** Root pastes in their own bot token (from `@BotFather`), each user links their own chat from their profile — fully opt-in, and off by default.
- **Archive & Cleanup:** Restore options, a 3-day warning before permanent cleanup, and root-only permanent delete.
- **Focus Mode & Time Tracking:** Live presence (*"Ivan is working on this · 12m"* with a plain colored dot like Space Pulse's) visible to teammates on every task view.
- **Global Search & Shortcuts:** Keyboard shortcuts (`?` for help), search across tasks/notes/comments.
- **Localization & Themes:** 13 locales (language-country format) + IT styles `ru-RU-it`, `en-US-it`. 5 color themes (red/blue/green/yellow/gray), light + dark, "cozy"/"lite" density modes.
- **Realtime & PWA:** SSE realtime updates, working PWA install button (requires HTTPS).
- **Analytics & Public Sharing:** Statistics, dynamic labels, leaderboards, **Space Pulse**, list templates, emergency announcements, list-level public read-only share links.
- **Single Binary:** Everything configurable from the root panel **and** the terminal (`todorio server ...`). Ships as a single self-contained binary (frontend + migrations embedded).

---

## System Requirements

These are runtime requirements for the server that hosts Todorio. You do **not** need Go, Node.js, or a build toolchain there — release binaries ship with the frontend and SQL migrations embedded. For building from source, see [Development](#development).

### Operating system

| | Supported | Notes |
| :--- | :--- | :--- |
| **One-command install** | Ubuntu 22.04+, Debian 12+ | `scripts/install.sh` uses `apt-get`/`dpkg`, so it is Debian-family only. |
| **Manual install** | Any modern Linux distribution (RHEL, CentOS, Fedora, Alpine, Arch, …) | The binary is statically linked (`CGO_ENABLED=0`) and has no glibc or system dependencies. You provide PostgreSQL, the systemd unit, and the database yourself. |
| **Not supported** | Windows, macOS, \*BSD | Only Linux binaries are published. |

Ubuntu 20.04 and Debian 11 are **not** recommended: their `apt` PostgreSQL packages are 12 and 13, below the 14 that Todorio requires (see below). Installing there gives you an unsupported database version.

### Architecture

`amd64` (x86_64) and `arm64` (AArch64) — the two targets built by `.github/workflows/release.yml`. 32-bit platforms (`armv7`, `i386`) are not supported and `install.sh` refuses to run on them.

### CPU, memory, and disk

- **CPU:** 1 vCPU.
- **RAM:** 512 MB with PostgreSQL on the same host. Configure swap if you run at 512 MB; 1 GB is more comfortable for a small team.
- **Disk:** ~1 GB covers the OS, the binary, and an empty database. **Attachments and backups grow on top of that**, in `/var/lib/todorio/uploads` and `/var/lib/todorio/backups` (written by `sudo todorio backup`). Uploads are **unlimited by default** — set a quota in the root panel or via `sudo todorio server limits set limits.uploads.max_total_storage_mb <MB>` before opening the instance to other people.

### Database

**PostgreSQL 14 or newer.** `install.sh` installs the distribution package if `psql` is missing or older than 14, then creates the role and database `todorio`.

> The role is created with the default password `todorio`. It is only reachable locally, but change it (and `/etc/todorio/config.json`) if the host is shared or the database port is exposed.

---

## Installation

Run the following command on your VPS (Ubuntu 22.04+ or Debian 12+):

```bash
curl -fsSL https://raw.githubusercontent.com/DanMotive/Todorio/main/scripts/install.sh | sudo bash
```

That's the whole install — one command. `todorio` ships as a single self-contained binary (the frontend and SQL migrations are embedded into it at release time — see `assets.go`), so the script:
1. Downloads the right binary for your OS/architecture from the latest [GitHub Release](https://github.com/DanMotive/Todorio/releases).
2. Verifies its SHA-256 checksum.
3. Sets up PostgreSQL and a `systemd` service.
4. Runs `todorio setup` automatically and interactively (via `/dev/tty`).

**Setup Prompts:**
The setup interactively asks for: the root admin's username, port, HTTPS choice (self-signed, Let's Encrypt for your server's IP, or custom certificate), and whether to create the demo onboarding space with quests. It then generates a 16-character temporary password, creates the root admin account, and starts the service.

> **Note (Headless/Automated Envs):**  
> If the environment has no attachable terminal (no `/dev/tty`), the script stops short. Finish setup manually using:  
> `sudo todorio setup && sudo systemctl enable --now todorio`

Re-running `install.sh` on an already configured system (`/etc/todorio/config.json` exists) skips setup entirely and just restarts the service with whatever binary is currently installed.

**On non-Debian distributions,** install the pieces yourself: create a PostgreSQL 14+ role and database named `todorio`, download `todorio_linux_<arch>` from the [latest release](https://github.com/DanMotive/Todorio/releases) and verify it against `checksums.txt`, install it to `/usr/local/bin/todorio`, create `/var/lib/todorio/{uploads,backups}` and `/etc/todorio`, then run `sudo todorio setup`. `scripts/install.sh` is short and readable — use it as the reference for the systemd unit.

---

## CLI Quick Reference

All day-to-day operations can be managed directly via the `todorio` command:

| Command | Description |
| :--- | :--- |
| `sudo todorio start` | Start the Todorio systemd service |
| `sudo todorio stop` | Stop the Todorio service |
| `sudo todorio restart` | Restart the Todorio service |
| `sudo todorio status` | Display service status and runtime diagnostics |
| `sudo todorio setup` | Interactively configure server settings (port, SSL, root admin) |
| `sudo todorio resetroot` | Reset the root administrator password |
| `sudo todorio update` | Fetch and update to the latest release binary with zero extra config |
| `sudo todorio backup` | Create a snapshot/backup of application data and database |
| `sudo todorio uninstall` | Remove Todorio binaries, service, and configs |

*Uninstall flags:*
- `sudo todorio uninstall --saveconfig` — removes binary & service, keeps config files.
- `sudo todorio uninstall --purge` — deletes configuration, application data, and the database.

---

## Development

### Prerequisites
- **Backend:** Go 1.22+
- **Frontend:** Node.js 20+
- **Database:** PostgreSQL 14+

These are needed only to build or hack on Todorio — not to run a published release.

### Local Dev Server

```bash
# Backend only — serves the API; UI falls back to ./web/dist if present
go run ./cmd/todorio serve --dev

# Frontend (Vite dev server)
cd web && npm install && npm run dev
```

### Production Build Test

To build a release-style binary with the frontend embedded:

```bash
# Build frontend asset bundle
cd web && npm install && npm run build && cd ..

# Build binary with embedded assets
go build -o todorio ./cmd/todorio
```

*Note:* Plain `go build ./...` without building the frontend first embeds a fallback placeholder (`web/dist/placeholder.txt`), causing the server to rely on on-disk web paths at runtime.

---

## Releasing

Pushing a tag matching `v*` (e.g. `git tag v1.2.3 && git push origin v1.2.3`) triggers `.github/workflows/release.yml`, which:
1. Builds the frontend once.
2. Compiles `linux/amd64` and `linux/arm64` Go binaries with frontend and migrations embedded.
3. Generates `checksums.txt`.
4. Publishes all assets to GitHub Releases.

`scripts/install.sh` and `todorio update` consume these release artifacts directly.

---

## Project Structure

```
assets.go          — Embeds web/dist/ and migrations/ into the binary
cmd/todorio/       — CLI and entry point (setup, serve, status, backup, update, server config)
internal/          — Core logic: config, HTTP+SSE server, setup, ops (status/backup/update/uninstall)
migrations/        — PostgreSQL SQL migrations (embedded)
scripts/           — Installation script (install.sh) and maintenance/CI helpers
web/               — React + Vite frontend, themes, locales, PWA
.github/workflows/ — Automated GitHub release pipelines
```

---

## Contributing

Contributions, bug reports, and feature requests are welcome!  
Feel free to open an issue or submit a pull request on [GitHub Issues](https://github.com/DanMotive/Todorio/issues).

---

## License

Distributed under the **Apache 2.0 License**. Developed by **DanMotive**.
