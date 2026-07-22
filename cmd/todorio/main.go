// Todorio — your private workspace for tasks and teams.
// https://github.com/DanMotive/Todorio · Apache 2.0 · Developed by Vlad
package main

import (
	"fmt"
	"os"

	"github.com/DanMotive/Todorio/internal/config"
	"github.com/DanMotive/Todorio/internal/ops"
	"github.com/DanMotive/Todorio/internal/server"
	"github.com/DanMotive/Todorio/internal/setup"
)

var version = "1.0.0-dev"

func usage() {
	fmt.Println(`Todorio ` + version + `

Commands:
  todorio setup [flags]               First-run setup (interactive by default)
    --non-interactive                   Skip all prompts, use flags/defaults (for scripted installs)
    --root-username <name>              Root admin username (default: root)
    --process-manager <systemd|docker|pm2>
    --port <port>
    --https                             Enable HTTPS with a self-signed certificate
    --cert-hosts <ip,domain,...>
    --demo                              Create the onboarding demo space (default: true)
  todorio serve [--dev]               Run the server
  todorio doctor                      Diagnostics (service, DB, disk, SSL, backups)
  todorio backup create               Create a backup
  todorio update                      Update to the latest release
  todorio server config set K V       Settings (default_locale, detect_browser_locale, ...)
  todorio server policy set K V       Policies (registration.mode, users.can_create_spaces, ...)
  todorio server limits set K V       Limits (uploads.max_file_size_mb, ...)
  todorio server branding set K V     Branding (site_name, browser_title, developer_name, ...)
  todorio server locales enable L     Enable a locale (e.g. tr-TR)
  todorio version`)
}

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Println("todorio", version)
	case "setup":
		if err := setup.Run(os.Args[2:]); err != nil {
			fail(err)
		}
	case "serve":
		cfg, err := config.Load()
		if err != nil {
			fail(fmt.Errorf("config not found — run `todorio setup` first: %w", err))
		}
		if err := server.Run(cfg, version); err != nil {
			fail(err)
		}
	case "doctor":
		cfg, _ := config.Load() // doctor also works without a config — it will show what's missing
		if err := ops.Doctor(cfg, version); err != nil {
			fail(err)
		}
	case "backup":
		cfg, err := config.Load()
		if err != nil {
			fail(fmt.Errorf("config not found — run `todorio setup` first: %w", err))
		}
		if err := ops.Backup(cfg); err != nil {
			fail(err)
		}
	case "update":
		if err := ops.Update(version); err != nil {
			fail(err)
		}
	case "server":
		if err := config.RunCLI(os.Args[2:]); err != nil {
			fail(err)
		}
	default:
		usage()
		os.Exit(1)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "error:", err)
	os.Exit(1)
}
