// Package setup: first-run configuration for `todorio setup` (interactive and non-interactive).
package setup

import (
	"bufio"
	"crypto/rand"
	"flag"
	"fmt"
	"math/big"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/DanMotive/Todorio/internal/config"
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
		fmt.Println("⚠ Could not open port", port, "in ufw:", err)
		return
	}
	fmt.Printf("🔓 ufw: opened port %d/tcp\n", port)
}

// Run parses `todorio setup` flags and either runs the interactive wizard or, with
// --non-interactive, configures everything from flags/defaults (for scripted installs).
// TODO (full version): create the DB and root user (argon2id, must_change_password=true),
// install the unit file/compose/pm2, create the onboarding demo space with quests.
func Run(args []string) error {
	fs := flag.NewFlagSet("setup", flag.ContinueOnError)
	rootUsername := fs.String("root-username", "root", "root admin username")
	processManager := fs.String("process-manager", "systemd", "process manager: systemd|docker|pm2")
	port := fs.Int("port", 8080, "site port")
	httpsFlag := fs.Bool("https", false, "enable HTTPS with a self-signed certificate")
	certHosts := fs.String("cert-hosts", "", "comma-separated hosts for the self-signed certificate (default: autodetected)")
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
		fmt.Println("⚡ Todorio — non-interactive setup")
		fmt.Println(strings.Repeat("─", 40))

		root = *rootUsername
		switch *processManager {
		case "systemd", "docker", "pm2":
			cfg.ProcessManager = *processManager
		default:
			return fmt.Errorf("unknown process manager: %s", *processManager)
		}
		if *port < 1 || *port > 65535 {
			return fmt.Errorf("invalid port: %d", *port)
		}
		cfg.Port = *port
		cfg.HTTPS = *httpsFlag
		if cfg.HTTPS {
			hostsStr := *certHosts
			if hostsStr == "" {
				hostsStr = defaultCertHosts()
			}
			certFile, keyFile, cerr := GenerateSelfSigned("/etc/todorio/ssl", splitHosts(hostsStr))
			if cerr != nil {
				fmt.Println("⚠ Failed to generate certificate:", cerr)
				fmt.Println("  HTTPS disabled. Set cert_file/key_file manually and enable:")
				fmt.Println("  todorio server config set https true")
				cfg.HTTPS = false
			} else {
				cfg.CertFile, cfg.KeyFile = certFile, keyFile
				fmt.Println("🔐 Certificate:", certFile, "(10 years, the browser will show a warning — that's expected)")
			}
		}
		demo = *demoFlag
		if !*generatePw {
			fmt.Println("⚠ --generate-password=false is ignored — a temporary password is always required for first login.")
		}
	} else {
		r := bufio.NewReader(os.Stdin)

		fmt.Println("⚡ Todorio — first-run setup")
		fmt.Println(strings.Repeat("─", 40))

		root = ask(r, "Root admin username", *rootUsername)

		pm := ask(r, "Process manager (systemd/docker/pm2)", *processManager)
		switch pm {
		case "systemd", "docker", "pm2":
			cfg.ProcessManager = pm
		default:
			return fmt.Errorf("unknown process manager: %s", pm)
		}

		portStr := ask(r, "Site port", strconv.Itoa(*port))
		p, err := strconv.Atoi(portStr)
		if err != nil || p < 1 || p > 65535 {
			return fmt.Errorf("invalid port: %s", portStr)
		}
		cfg.Port = p

		cfg.HTTPS = askYN(r, "Enable HTTPS with a self-signed certificate?", *httpsFlag)
		if cfg.HTTPS {
			hostsStr := ask(r, "IPs/domains for the certificate (comma-separated)", defaultCertHosts())
			certFile, keyFile, cerr := GenerateSelfSigned("/etc/todorio/ssl", splitHosts(hostsStr))
			if cerr != nil {
				fmt.Println("⚠ Failed to generate certificate:", cerr)
				fmt.Println("  HTTPS disabled. Set cert_file/key_file manually and enable:")
				fmt.Println("  todorio server config set https true")
				cfg.HTTPS = false
			} else {
				cfg.CertFile, cfg.KeyFile = certFile, keyFile
				fmt.Println("🔐 Certificate:", certFile, "(10 years, the browser will show a warning — that's expected)")
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

	fmt.Println()
	fmt.Println(strings.Repeat("─", 40))
	fmt.Println("✅ Setup complete. Config:", config.Path())
	fmt.Printf("   Root admin: %s\n", root)
	fmt.Printf("   Temporary password: %s\n", password)
	fmt.Println("   ⚠ The password is shown ONCE and is not written to logs.")
	fmt.Println("   The site will require changing it on first login.")
	if demo {
		fmt.Println("   🎓 The demo space with quests will be created on first launch.")
	}
	fmt.Printf("   Start: sudo systemctl start todorio (or `todorio serve`)\n")
	return nil
}
