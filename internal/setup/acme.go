package setup

// Trusted Let's Encrypt certificates for a bare server IP address (RFC 8738 ACME IP
// identifiers), issued via acme.sh in standalone HTTP-01 mode — the same approach
// used by 3x-ui's "Let's Encrypt SSL Certificate for IP Address" flow.
//
// These certificates use Let's Encrypt's "shortlived" ACME profile (~6 days validity)
// and are renewed automatically by the cron job acme.sh installs for itself.

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/DanMotive/Todorio/internal/term"
)

// acmeShPath returns the path to the acme.sh script, installing it via the official
// installer if it isn't present yet.
func acmeShPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "/root"
	}
	p := filepath.Join(home, ".acme.sh", "acme.sh")
	if _, err := os.Stat(p); err == nil {
		return p, nil
	}
	fmt.Println(term.Cyan("Installing acme.sh..."))
	cmd := exec.Command("sh", "-c", "curl -fsSL https://get.acme.sh | sh -s email=todorio@localhost")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("failed to install acme.sh: %w", err)
	}
	if _, err := os.Stat(p); err != nil {
		return "", fmt.Errorf("acme.sh installer finished but %s was not found", p)
	}
	return p, nil
}

// DetectPublicIP returns the first non-loopback, non-link-local IPv4 address found on
// the machine's network interfaces. Used as the default subject for the IP certificate.
func DetectPublicIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok || ipNet.IP.IsLoopback() || ipNet.IP.IsLinkLocalUnicast() {
			continue
		}
		if ip4 := ipNet.IP.To4(); ip4 != nil {
			return ip4.String()
		}
	}
	return ""
}

// IssueLetsEncryptIPCert obtains a short-lived, trusted Let's Encrypt certificate for a
// bare server IP address using acme.sh in standalone mode on httpPort (port 80 by
// default must be open and reachable from the internet for HTTP-01 validation). The
// certificate is valid for ~6 days and auto-renews via the cron job acme.sh installs
// for itself. ipv6 may be empty to issue for the IPv4 address only.
func IssueLetsEncryptIPCert(dir, ip, ipv6 string, httpPort int) (certFile, keyFile string, err error) {
	if ip == "" {
		return "", "", fmt.Errorf("server IP is required")
	}
	acme, err := acmeShPath()
	if err != nil {
		return "", "", err
	}

	args := []string{
		"--issue", "--standalone",
		"-d", ip,
		"--httpport", fmt.Sprintf("%d", httpPort),
		"--server", "letsencrypt",
		"--ecc",
		"--profile", "shortlived",
	}
	if ipv6 = strings.TrimSpace(ipv6); ipv6 != "" {
		args = append(args, "-d", ipv6, "--listen-v6")
	}

	issue := exec.Command(acme, args...)
	issue.Stdout, issue.Stderr = os.Stdout, os.Stderr
	if err := issue.Run(); err != nil {
		return "", "", fmt.Errorf("acme.sh --issue failed: %w", err)
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")

	install := exec.Command(acme, "--installcert", "-d", ip, "--ecc",
		"--key-file", keyFile,
		"--fullchain-file", certFile,
		"--reloadcmd", "systemctl restart todorio 2>/dev/null || true",
	)
	install.Stdout, install.Stderr = os.Stdout, os.Stderr
	if err := install.Run(); err != nil {
		return "", "", fmt.Errorf("acme.sh --installcert failed: %w", err)
	}

	return certFile, keyFile, nil
}
