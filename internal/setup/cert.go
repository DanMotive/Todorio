package setup

// Self-signed certificate generation for HTTPS by bare IP or domain.
// Standard library only: ECDSA P-256, 10-year validity.
// The browser will show an "untrusted certificate" warning — that's expected for self-signed certs.

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// defaultCertHosts — localhost + every non-loopback IP on the machine (so the site works by bare VPS IP).
func defaultCertHosts() string {
	hosts := []string{"localhost", "127.0.0.1"}
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			ipNet, ok := a.(*net.IPNet)
			if !ok || ipNet.IP.IsLoopback() || ipNet.IP.IsLinkLocalUnicast() {
				continue
			}
			hosts = append(hosts, ipNet.IP.String())
		}
	}
	return strings.Join(hosts, ",")
}

// InstallCustomCert copies a user-provided certificate and private key into dir
// (as cert.pem / key.pem) so they can be referenced like any other Todorio
// certificate. Use this when you already have a certificate from your own CA
// or another ACME client, instead of generating a new one.
func InstallCustomCert(dir, certPath, keyPath string) (certFile, keyFile string, err error) {
	certPath = strings.TrimSpace(certPath)
	keyPath = strings.TrimSpace(keyPath)
	if certPath == "" || keyPath == "" {
		return "", "", fmt.Errorf("both a certificate file and a key file are required")
	}
	certBytes, err := os.ReadFile(certPath)
	if err != nil {
		return "", "", fmt.Errorf("reading certificate file: %w", err)
	}
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return "", "", fmt.Errorf("reading key file: %w", err)
	}
	if _, err := tls.X509KeyPair(certBytes, keyBytes); err != nil {
		return "", "", fmt.Errorf("certificate and key do not match or are invalid: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, certBytes, 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(keyFile, keyBytes, 0o600); err != nil {
		return "", "", err
	}
	return certFile, keyFile, nil
}

// GenerateSelfSigned creates a certificate for the given hosts (IPs and/or DNS names)
// and writes cert.pem / key.pem to dir. Returns the file paths.
func GenerateSelfSigned(dir string, hosts []string) (certFile, keyFile string, err error) {
	if len(hosts) == 0 {
		return "", "", fmt.Errorf("at least one host (IP or domain) is required")
	}
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", "", err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", "", err
	}
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   hosts[0],
			Organization: []string{"Todorio self-signed"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, h := range hosts {
		h = strings.TrimSpace(h)
		if h == "" {
			continue
		}
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	certFile = filepath.Join(dir, "cert.pem")
	keyFile = filepath.Join(dir, "key.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := os.WriteFile(certFile, certPEM, 0o644); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(keyFile, keyPEM, 0o600); err != nil {
		return "", "", err
	}
	return certFile, keyFile, nil
}
