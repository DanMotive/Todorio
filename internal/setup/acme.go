package setup

// Trusted Let's Encrypt certificates for a bare server IP address (RFC 8738 ACME IP
// identifiers), issued via acme.sh in standalone HTTP-01 mode — the same approach
// used by 3x-ui's "Let's Encrypt SSL Certificate for IP Address" flow.
//
// These certificates use Let's Encrypt's "shortlived" ACME profile (~6 days validity)
// and are renewed automatically by the cron job acme.sh installs for itself.

import (
	"fmt"
	"io"
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

// runAcme runs an acme.sh command, streaming its output to the terminal (so progress is
// still visible live) while also capturing it, so callers can inspect it for known,
// recoverable error messages (e.g. an older acme.sh that doesn't support a flag yet).
func runAcme(acme string, args []string) (output string, err error) {
	cmd := exec.Command(acme, args...)
	var buf strings.Builder
	cmd.Stdout = io.MultiWriter(os.Stdout, &buf)
	cmd.Stderr = io.MultiWriter(os.Stderr, &buf)
	err = cmd.Run()
	return buf.String(), err
}

// IssueLetsEncryptIPCert obtains a trusted Let's Encrypt certificate for a bare server IP
// address using acme.sh in standalone mode on httpPort (port 80 by default must be open
// and reachable from the internet for HTTP-01 validation). ipv6 may be empty to issue for
// the IPv4 address only.
//
// It first tries the "shortlived" ACME profile (~6 days validity, auto-renews frequently)
// like 3x-ui's IP certificate flow. Older acme.sh installs don't know this flag yet
// ("Unknown parameter: --profile") — in that case it automatically retries without it,
// which issues a standard ~90-day certificate instead. shortlived reports which one was
// obtained, so callers can show the correct validity period to the user.
func IssueLetsEncryptIPCert(dir, ip, ipv6 string, httpPort int) (certFile, keyFile string, shortlived bool, err error) {
	if ip == "" {
		return "", "", false, fmt.Errorf("server IP is required")
	}
	acme, err := acmeShPath()
	if err != nil {
		return "", "", false, err
	}

	baseArgs := []string{
		"--issue", "--standalone",
		"-d", ip,
		"--httpport", fmt.Sprintf("%d", httpPort),
		"--server", "letsencrypt",
		"--ecc",
	}
	if ipv6 = strings.TrimSpace(ipv6); ipv6 != "" {
		baseArgs = append(baseArgs, "-d", ipv6, "--listen-v6")
	}

	// alreadyIssued recognizes acme.sh's idempotent "nothing to do" response: it already
	// holds an unexpired certificate for this domain and is skipping reissuance rather
	// than failing. That's not an error — the existing certificate is still installed below.
	alreadyIssued := func(out string) bool {
		return strings.Contains(out, "Skipping") && strings.Contains(out, "Next renewal time")
	}

	shortlivedArgs := append(append([]string{}, baseArgs...), "--profile", "shortlived")
	output, runErr := runAcme(acme, shortlivedArgs)
	shortlived = runErr == nil
	if runErr != nil && alreadyIssued(output) {
		runErr = nil
		shortlived = false
	}
	if runErr != nil {
		if strings.Contains(output, "Unknown parameter") && strings.Contains(output, "--profile") {
			fmt.Println(term.Yellow("WARN"), "This acme.sh install does not support the shortlived profile yet — retrying with a standard ~90-day certificate.")
			output, runErr = runAcme(acme, baseArgs)
			shortlived = false
			if runErr != nil && alreadyIssued(output) {
				fmt.Println(term.Cyan("INFO"), "A still-valid certificate for this IP already exists — reusing it instead of reissuing.")
				runErr = nil
			}
		}
		if runErr != nil {
			return "", "", false, fmt.Errorf("acme.sh --issue failed: %w", runErr)
		}
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", false, err
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
		return "", "", false, fmt.Errorf("acme.sh --installcert failed: %w", err)
	}

	return certFile, keyFile, shortlived, nil
}
