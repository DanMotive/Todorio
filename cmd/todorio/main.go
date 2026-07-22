// Todorio — your private workspace for tasks and teams.
// https://github.com/DanMotive/Todorio · Apache 2.0 · Developed by Vlad
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/DanMotive/Todorio/internal/config"
	"github.com/DanMotive/Todorio/internal/ops"
	"github.com/DanMotive/Todorio/internal/server"
	"github.com/DanMotive/Todorio/internal/setup"
	"github.com/DanMotive/Todorio/internal/term"
)

var version = "1.0.0-dev"

const banner = `████████╗ ██████╗ ██████╗  ██████╗ ██████╗ ██╗ ██████╗
╚══██╔══╝██╔═══██╗██╔══██╗██╔═══██╗██╔══██╗██║██╔═══██╗
   ██║   ██║   ██║██║  ██║██║   ██║██████╔╝██║██║   ██║
   ██║   ██║   ██║██║  ██║██║   ██║██╔══██╗██║██║   ██║
   ██║   ╚██████╔╝██████╔╝╚██████╔╝██║  ██║██║╚██████╔╝
   ╚═╝    ╚═════╝ ╚═════╝  ╚═════╝ ╚═╝  ╚═╝╚═╝ ╚═════╝`

// cmdLine prints a top-level command, its name colorized and aligned with its
// description. Pass an empty desc for a bare heading-style entry.
func cmdLine(name, desc string) {
	if desc == "" {
		fmt.Println("  " + term.Cyan(name))
		return
	}
	fmt.Println("  "+term.Cyan(fmt.Sprintf("%-34s", name)), desc)
}

// subLine prints an indented flag/sub-option line under a command.
func subLine(name, desc string) {
	if desc == "" {
		fmt.Println("      " + name)
		return
	}
	fmt.Println("      "+fmt.Sprintf("%-32s", name), desc)
}

func usage() {
	fmt.Println(term.Cyan(banner))
	fmt.Println()
	fmt.Println(term.Bold("Todorio "+version) + " — your private workspace for tasks and teams")
	fmt.Println(strings.Repeat("─", 62))
	fmt.Println()

	fmt.Println(term.Bold("Setup & running"))
	cmdLine("todorio setup [flags]", "First-run setup (interactive by default)")
	subLine("--non-interactive", "Skip all prompts, use flags/defaults (scripted installs)")
	subLine("--root-username <name>", "Root admin username (default: root)")
	subLine("--process-manager <systemd|docker|pm2>", "")
	subLine("--port <port>", "")
	subLine("--https", "Enable HTTPS")
	subLine("--cert-mode <self-signed|letsencrypt-ip|custom>", "")
	subLine("--cert-hosts <ip,domain,...>", "Hosts for a self-signed certificate")
	subLine("--acme-ip <ip>", "IP for a Let's Encrypt IP certificate")
	subLine("--acme-ipv6 <ip>", "Optional IPv6 for a Let's Encrypt IP certificate")
	subLine("--acme-port <port>", "Port for the ACME HTTP challenge (default: 80)")
	subLine("--cert-file <path>", "Your own certificate file (with --cert-mode=custom)")
	subLine("--key-file <path>", "Your own private key file (with --cert-mode=custom)")
	subLine("--demo", "Create the onboarding demo space (default: true)")
	fmt.Println()
	cmdLine("todorio serve [--dev]", "Run the server")
	fmt.Println()

	fmt.Println(term.Bold("Maintenance"))
	cmdLine("todorio status", "Diagnostics (service, DB, disk, SSL, backups) + server URL")
	cmdLine("todorio resetroot [flags]", "Reset the root admin's username and password")
	subLine("--username <name>", "New root admin username (default: keep current)")
	subLine("--yes", "Skip the confirmation prompt")
	cmdLine("todorio backup create", "Create a backup")
	cmdLine("todorio update", "Update to the latest release")
	cmdLine("todorio uninstall [flags]", "Remove Todorio from this machine")
	subLine("--purge", "Also remove application data (uploads, backups) and the database")
	subLine("--saveconfig", "Keep the config (/etc/todorio) instead of removing it by default")
	subLine("--yes", "Skip the confirmation prompt")
	fmt.Println()

	fmt.Println(term.Bold("Server configuration"))
	cmdLine("todorio server config set K V", "Settings (default_locale, detect_browser_locale, ...)")
	cmdLine("todorio server policy set K V", "Policies (registration.mode, users.can_create_spaces, ...)")
	cmdLine("todorio server limits set K V", "Limits (uploads.max_file_size_mb, ...)")
	cmdLine("todorio server branding set K V", "Branding (site_name, browser_title, developer_name, ...)")
	cmdLine("todorio server locales enable L", "Enable a locale (e.g. tr-TR)")
	fmt.Println()
	cmdLine("todorio version", "")

	fmt.Println()
	fmt.Println(strings.Repeat("─", 62))
	fmt.Println(term.Bold("Todorio") + " — made by DanMotive · https://github.com/DanMotive/Todorio")
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
	case "status", "doctor": // "doctor" kept as a hidden alias for backward compatibility
		cfg, _ := config.Load() // status also works without a config — it will show what's missing
		if err := ops.Status(cfg, version); err != nil {
			fail(err)
		}
	case "resetroot":
		cfg, err := config.Load()
		if err != nil {
			fail(fmt.Errorf("config not found — run `todorio setup` first: %w", err))
		}
		var newUsername string
		var yes bool
		args := os.Args[2:]
		for i := 0; i < len(args); i++ {
			switch args[i] {
			case "--username":
				if i+1 >= len(args) {
					fail(fmt.Errorf("--username requires a value"))
				}
				i++
				newUsername = args[i]
			case "--yes", "-y":
				yes = true
			default:
				fail(fmt.Errorf("unknown flag for resetroot: %s", args[i]))
			}
		}
		if err := ops.ResetRoot(cfg, newUsername, yes); err != nil {
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
	case "uninstall":
		cfg, _ := config.Load() // uninstall must still work if the config is already partly gone
		var purge, saveConfig, yes bool
		for _, arg := range os.Args[2:] {
			switch arg {
			case "--purge":
				purge = true
			case "--saveconfig":
				saveConfig = true
			case "--yes", "-y":
				yes = true
			default:
				fail(fmt.Errorf("unknown flag for uninstall: %s", arg))
			}
		}
		if err := ops.Uninstall(cfg, purge, saveConfig, yes); err != nil {
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
	fmt.Fprintln(os.Stderr, term.Red("error:"), err)
	os.Exit(1)
}
