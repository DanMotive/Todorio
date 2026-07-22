// Package setup: first-run configuration for `todorio setup` (interactive and non-interactive).
package setup

import (
	"bufio"
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/DanMotive/Todorio/internal/auth"
	"github.com/DanMotive/Todorio/internal/config"
	"github.com/DanMotive/Todorio/internal/db"
	"github.com/DanMotive/Todorio/internal/term"
)

const passwordAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnpqrstuvwxyz23456789!@#$%^&*-_=+"

// GeneratePassword returns a cryptographically strong 16-character temporary password.
func GeneratePassword() (string, error) {
	var b strings.Builder
	for i := 0; i < 16; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(passwordAlphabet))))
		if err != nil {
			return "", err
		}
		b.WriteByte(passwordAlphabet[n.Int64()])
	}
	return b.String(), nil
}

func ask(r *bufio.Reader, prompt, def string) string {
	if def != "" {
		fmt.Printf("%s [%s]: ", prompt, def)
	} else {
		fmt.Printf("%s: ", prompt)
	}
	line, _ := r.ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "" {
		return def
	}
	return line
}

func askYN(r *bufio.Reader, prompt string, def bool) bool {
	d := "y/N"
	if def {
		d = "Y/n"
	}
	ans := strings.ToLower(ask(r, prompt+" ("+d+")", ""))
	if ans == "" {
		return def
	}
	return ans == "y" || ans == "yes"
}

// issueLetsEncryptIPCertVerbose obtains a Let's Encrypt IP certificate and prints the
// same [INF]-style progress lines as 3x-ui's IP certificate flow. Returns ok=false
// (with a warning already printed) if the IP is empty or issuance fails, so callers
// can fall back to a self-signed certificate.
func issueLetsEncryptIPCertVerbose(ip, ipv6 string, acmePort int) (certFile, keyFile string, ok bool) {
	if strings.TrimSpace(ip) == "" {
		fmt.Println(term.Yellow("WARN"), "Could not detect the server's IP automatically and none was provided.")
		return "", "", false
	}
	fmt.Println("[INF] Starting automatic SSL certificate generation for server IP...")
	fmt.Println("[INF] Using Let's Encrypt shortlived profile (~6 days validity, auto-renews)")
	fmt.Println("[INF] Server IP detected:", ip)
	fmt.Printf("[INF] Using port %d to issue certificate for IP: %s\n", acmePort, ip)
	certFile, keyFile, shortlived, err := IssueLetsEncryptIPCert("/etc/todorio/ssl", ip, ipv6, acmePort)
	if err != nil {
		fmt.Println(term.Yellow("WARN"), "Failed to obtain Let's Encrypt certificate:", err)
		fmt.Println("  Falling back to a self-signed certificate.")
		return "", "", false
	}
	fmt.Println("[INF] Certificate issued successfully for IP:", ip)
	if shortlived {
		fmt.Println(term.Cyan("Certificate:"), certFile, "(~6 days validity, auto-renews via the acme.sh cron job)")
	} else {
		fmt.Println(term.Cyan("Certificate:"), certFile, "(~90 days validity, standard profile, auto-renews via the acme.sh cron job)")
	}
	return certFile, keyFile, true
}

func splitHosts(hostsStr string) []string {
	hosts := []string{}
	for _, h := range strings.Split(hostsStr, ",") {
		if h = strings.TrimSpace(h); h != "" {
			hosts = append(hosts, h)
		}
	}
	return hosts
}

// openFirewallPort opens the given TCP port with ufw, if ufw is installed and active.
// Does nothing (silently) if ufw is absent — most VPS images ship without it enabled.
func openFirewallPort(port int) {
	ufwPath, err := exec.LookPath("ufw")
	if err != nil {
		return
	}
	statusOut, err := exec.Command(ufwPath, "status").Output()
	if err != nil || !strings.Contains(string(statusOut), "Status: active") {
		return
	}
	if err := exec.Command(ufwPath, "allow", fmt.Sprintf("%d/tcp", port)).Run(); err != nil {
		fmt.Println(term.Yellow("WARN"), "Could not open port", port, "in ufw:", err)
		return
	}
	fmt.Println(term.Green("ufw:"), fmt.Sprintf("opened port %d/tcp", port))
}

// ensureRootUser connects to the database, applies migrations if needed, and creates
// the root admin account with the given username/password if no root account exists
// yet. If a root account already exists, its credentials are left untouched (use
// `todorio resetroot` to change them) and created is reported as false.
func ensureRootUser(cfg config.Config, username, password string) (created bool, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	database, err := db.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return false, fmt.Errorf("connecting to the database: %w", err)
	}
	defer database.Pool.Close()

	if err := database.Migrate(ctx, migrationsDir()); err != nil {
		return false, fmt.Errorf("running migrations: %w", err)
	}

	var existing int
	if err := database.Pool.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='root'`).Scan(&existing); err != nil {
		return false, fmt.Errorf("checking for an existing root admin: %w", err)
	}
	if existing > 0 {
		return false, nil
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		return false, fmt.Errorf("hashing the root admin password: %w", err)
	}
	if _, err := database.Pool.Exec(ctx,
		`INSERT INTO users(username, password_hash, role, status, must_change_password) VALUES($1,$2,'root','active',true)`,
		username, hash); err != nil {
		return false, fmt.Errorf("creating the root admin account: %w", err)
	}
	return true, nil
}

// migrationsDir mirrors internal/server's lookup: next to the binary in prod
// (/usr/share/todorio/migrations), ./migrations when run from the repo.
func migrationsDir() string {
	if _, err := os.Stat("/usr/share/todorio/migrations"); err == nil {
		return "/usr/share/todorio/migrations"
	}
	return "migrations"
}

// Run parses `todorio setup` flags and either runs the interactive wizard or, with
// --non-interactive, configures everything from flags/defaults (for scripted installs).
// The process is always managed by systemd (see scripts/install.sh).
// TODO (full version): create the onboarding demo space with quests.
func Run(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	rootUsername := fs.String("root-username", "root", "root admin username")
	port := fs.Int("port", 8080, "site port")
	httpsFlag := fs.Bool("https", false, "enable HTTPS")
	certMode := fs.String("cert-mode", "self-signed", "certificate type: self-signed|letsencrypt-ip|custom")
	certHosts := fs.String("cert-hosts", "", "comma-separated hosts for the self-signed certificate (default: autodetected)")
	acmeIP := fs.String("acme-ip", "", "server IP for the Let's Encrypt IP certificate (default: autodetected)")
	acmeIPv6 := fs.String("acme-ipv6", "", "optional IPv6 address to include in the Let's Encrypt IP certificate")
	acmePort := fs.Int("acme-port", 80, "port for the ACME HTTP-01 standalone listener (must be open to the internet)")
	certFileFlag := fs.String("cert-file", "", "your own certificate file (with --cert-mode=custom)")
	keyFileFlag := fs.String("key-file", "", "your own private key file (with --cert-mode=custom)")
	demoFlag := fs.Bool("demo", true, "create the onboarding demo space with quests")
	generatePw := fs.Bool("generate-password", true, "generate the root admin's temporary password")
	nonInteractive := fs.Bool("non-interactive", false, "skip all prompts and use flags/defaults (for scripted installs)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg := config.Defaults()
	var root string
	var demo bool

	if *nonInteractive {
		fmt.Println(term.Bold("Todorio — non-interactive setup"))
		fmt.Println(strings.Repeat("─", 40))

		root = *rootUsername
		if *port < 1 || *port > 65535 {
			return fmt.Errorf("invalid port: %d", *port)
		}
		cfg.Port = *port
		cfg.HTTPS = *httpsFlag
		if cfg.HTTPS {
			switch *certMode {
			case "letsencrypt-ip":
				ip := *acmeIP
				if ip == "" {
					ip = DetectPublicIP()
				}
				if certFile, keyFile, ok := issueLetsEncryptIPCertVerbose(ip, *acmeIPv6, *acmePort); ok {
					cfg.CertFile, cfg.KeyFile = certFile, keyFile
				} else if certFile, keyFile, cerr := GenerateSelfSigned("/etc/todorio/ssl", splitHosts(defaultCertHosts())); cerr == nil {
					cfg.CertFile, cfg.KeyFile = certFile, keyFile
					fmt.Println(term.Cyan("Fell back to a self-signed certificate:"), certFile)
				} else {
					fmt.Println(term.Yellow("WARN"), "Failed to generate a fallback certificate:", cerr)
					cfg.HTTPS = false
				}
			case "custom":
				certFile, keyFile, cerr := InstallCustomCert("/etc/todorio/ssl", *certFileFlag, *keyFileFlag)
				if cerr != nil {
					fmt.Println(term.Yellow("WARN"), "Failed to install your certificate:", cerr)
					fmt.Println("  HTTPS disabled. Re-run setup with --cert-file and --key-file pointing at valid files.")
					cfg.HTTPS = false
				} else {
					cfg.CertFile, cfg.KeyFile = certFile, keyFile
					fmt.Println(term.Cyan("Certificate:"), certFile, "(your own certificate)")
				}
			default:
				hostsStr := *certHosts
				if hostsStr == "" {
					hostsStr = defaultCertHosts()
				}
				certFile, keyFile, cerr := GenerateSelfSigned("/etc/todorio/ssl", splitHosts(hostsStr))
				if cerr != nil {
					fmt.Println(term.Yellow("WARN"), "Failed to generate certificate:", cerr)
					fmt.Println("  HTTPS disabled. Set cert_file/key_file manually and enable:")
					fmt.Println("  todorio server config set https true")
					cfg.HTTPS = false
				} else {
					cfg.CertFile, cfg.KeyFile = certFile, keyFile
					fmt.Println(term.Cyan("Certificate:"), certFile, "(10 years, the browser will show a warning — that's expected)")
				}
			}
		}
		demo = *demoFlag
		if !*generatePw {
			fmt.Println(term.Yellow("WARN"), "--generate-password=false is ignored — a temporary password is always required for first login.")
		}
	} else {
		r := bufio.NewReader(os.Stdin)

		fmt.Println(term.Bold("Todorio — first-run setup"))
		fmt.Println(strings.Repeat("─", 40))

		root = ask(r, "Root admin username", *rootUsername)

		portStr := ask(r, "Site port", strconv.Itoa(*port))
		p, err := strconv.Atoi(portStr)
		if err != nil || p < 1 || p > 65535 {
			return fmt.Errorf("invalid port: %s", portStr)
		}
		cfg.Port = p

		cfg.HTTPS = askYN(r, "Enable HTTPS?", *httpsFlag)
		if cfg.HTTPS {
			fmt.Println("Certificate type:")
			fmt.Println("  1) Self-signed (instant, browsers will show an \"untrusted\" warning)")
			fmt.Println("  2) Let's Encrypt SSL Certificate for IP Address (trusted, ~6-day cert, auto-renews, requires port 80 open to the internet)")
			fmt.Println("  3) Use your own certificate (existing cert + key files)")
			choice := ask(r, "Choose 1, 2 or 3", "1")

			if choice == "3" {
				certPath := ask(r, "Path to your certificate file (PEM)", *certFileFlag)
				keyPath := ask(r, "Path to your private key file (PEM)", *keyFileFlag)
				certFile, keyFile, cerr := InstallCustomCert("/etc/todorio/ssl", certPath, keyPath)
				if cerr != nil {
					fmt.Println(term.Yellow("WARN"), "Failed to install your certificate:", cerr)
					fmt.Println("  HTTPS disabled. Re-run setup once you have a valid certificate and key.")
					cfg.HTTPS = false
				} else {
					cfg.CertFile, cfg.KeyFile = certFile, keyFile
					fmt.Println(term.Cyan("Certificate:"), certFile, "(your own certificate)")
				}
			} else if choice == "2" {
				fmt.Println("This will obtain a certificate for your server's IP using the shortlived profile.")
				fmt.Println("Certificate valid for ~6 days, auto-renews via acme.sh cron job.")
				fmt.Println("Port 80 must be open and accessible from the internet.")
				if askYN(r, "Do you want to proceed?", true) {
					ip := ask(r, "Server IP", DetectPublicIP())
					ipv6 := ask(r, "Do you have an IPv6 address to include? (leave empty to skip)", "")
					portStr := ask(r, "Port to use for ACME HTTP-01 listener", "80")
					acmePortN, perr := strconv.Atoi(portStr)
					if perr != nil || acmePortN < 1 || acmePortN > 65535 {
						return fmt.Errorf("invalid ACME port: %s", portStr)
					}
					if certFile, keyFile, ok := issueLetsEncryptIPCertVerbose(ip, ipv6, acmePortN); ok {
						cfg.CertFile, cfg.KeyFile = certFile, keyFile
					} else if certFile, keyFile, cerr := GenerateSelfSigned("/etc/todorio/ssl", splitHosts(defaultCertHosts())); cerr == nil {
						cfg.CertFile, cfg.KeyFile = certFile, keyFile
						fmt.Println(term.Cyan("Fell back to a self-signed certificate:"), certFile)
					} else {
						fmt.Println(term.Yellow("WARN"), "Failed to generate a fallback certificate:", cerr)
						cfg.HTTPS = false
					}
				} else {
					cfg.HTTPS = false
				}
			} else {
				hostsStr := ask(r, "IPs/domains for the certificate (comma-separated)", defaultCertHosts())
				certFile, keyFile, cerr := GenerateSelfSigned("/etc/todorio/ssl", splitHosts(hostsStr))
				if cerr != nil {
					fmt.Println(term.Yellow("WARN"), "Failed to generate certificate:", cerr)
					fmt.Println("  HTTPS disabled. Set cert_file/key_file manually and enable:")
					fmt.Println("  todorio server config set https true")
					cfg.HTTPS = false
				} else {
					cfg.CertFile, cfg.KeyFile = certFile, keyFile
					fmt.Println(term.Cyan("Certificate:"), certFile, "(10 years, the browser will show a warning — that's expected)")
				}
			}
		}
		demo = askYN(r, "Create a demo onboarding space with quests?", *demoFlag)
	}

	password, err := GeneratePassword()
	if err != nil {
		return err
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	openFirewallPort(cfg.Port)

	rootCreated, rootErr := ensureRootUser(cfg, root, password)

	fmt.Println()
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println(term.Green("Setup complete."), "Config:", config.Path())
	switch {
	case rootErr != nil:
		fmt.Println("  ", term.Yellow("WARN"), "Could not create the root admin account:", rootErr)
		fmt.Println("   Login will not work until this is fixed. Check that PostgreSQL is running and")
		fmt.Println("   database_url in", config.Path(), "is correct, then run `sudo todorio setup` again.")
	case rootCreated:
		fmt.Printf("   Root admin: %s\n", root)
		fmt.Printf("   Temporary password: %s\n", password)
		fmt.Println("  ", term.Yellow("NOTE"), "The password is shown ONCE and is not written to logs.")
		fmt.Println("   The site will require changing it on first login.")
	default:
		fmt.Println("  ", term.Cyan("NOTE"), "A root admin account already exists — its credentials were not changed.")
		fmt.Println("   Run `sudo todorio resetroot` if you need to reset the root username/password.")
	}
	if demo {
		fmt.Println("   The demo space with quests will be created on first launch.")
	}

	scheme := "http"
	if cfg.HTTPS {
		scheme = "https"
	}
	host := DetectPublicIP()
	if host == "" {
		host = "localhost"
	}
	fmt.Printf("   Server: %s\n", term.Cyan(fmt.Sprintf("%s://%s:%d", scheme, host, cfg.Port)))

	// If todorio is already running under systemd, restart it now so the new config
	// (port/HTTPS/certificate) takes effect immediately. Without this, a process
	// started under the old config keeps running — e.g. an old HTTPS listener would
	// keep rejecting plain HTTP requests even after HTTPS is disabled.
	restartRunningService()

	fmt.Printf("   Start: sudo systemctl start todorio (or `todorio serve`)\n")
	return nil
}

// restartRunningService best-effort restarts an already-running todorio systemd
// service, so config changes from `todorio setup` take effect right away instead of
// requiring the user to remember to do it manually. It does nothing (silently) if
// systemctl isn't available or no todorio unit is installed yet — that's expected on
// a brand-new install (the unit is created by scripts/install.sh).
func restartRunningService() {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return
	}
	if _, err := os.Stat("/etc/systemd/system/todorio.service"); err != nil {
		return
	}
	if err := exec.Command("systemctl", "try-restart", "todorio").Run(); err == nil {
		fmt.Println("  ", term.Green("systemd:"), "restarted the todorio service to apply the new config")
	}
}
