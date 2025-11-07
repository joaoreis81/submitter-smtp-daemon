package tlsmgr

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// Manager handles TLS certificate management with hot reload support
type Manager struct {
	certsDir  string
	certs     map[string]*tls.Certificate
	certsMu   sync.RWMutex
	watcher   *fsnotify.Watcher
	logger    *slog.Logger
	stopCh    chan struct{}
	minVersion uint16
}

// New creates a new TLS certificate manager
func New(certsDir string, minVersion uint16, logger *slog.Logger) (*Manager, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("create file watcher: %w", err)
	}

	m := &Manager{
		certsDir:   certsDir,
		certs:      make(map[string]*tls.Certificate),
		watcher:    watcher,
		logger:     logger,
		stopCh:     make(chan struct{}),
		minVersion: minVersion,
	}

	// Load initial certificates
	if err := m.loadCertificates(); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("load initial certificates: %w", err)
	}

	// Watch directory for changes
	if err := watcher.Add(certsDir); err != nil {
		watcher.Close()
		return nil, fmt.Errorf("watch certs directory: %w", err)
	}

	// Start watching for changes
	go m.watchCertificates()

	return m, nil
}

// loadCertificates loads all .pem files from the certs directory
func (m *Manager) loadCertificates() error {
	files, err := os.ReadDir(m.certsDir)
	if err != nil {
		return fmt.Errorf("read certs directory: %w", err)
	}

	newCerts := make(map[string]*tls.Certificate)
	certCount := 0

	for _, file := range files {
		if file.IsDir() || !strings.HasSuffix(file.Name(), ".pem") {
			continue
		}

		certPath := filepath.Join(m.certsDir, file.Name())

		// Load certificate from HAProxy-compatible .pem file (contains both cert and key)
		cert, err := tls.LoadX509KeyPair(certPath, certPath)
		if err != nil {
			m.logger.Warn("failed to load certificate",
				"file", file.Name(),
				"error", err)
			continue
		}

		// Parse certificate to extract domains from CN and SANs
		domains, err := m.extractDomainsFromCert(certPath)
		if err != nil {
			m.logger.Warn("failed to parse certificate domains",
				"file", file.Name(),
				"error", err)
			continue
		}

		if len(domains) == 0 {
			m.logger.Warn("no domains found in certificate",
				"file", file.Name())
			continue
		}

		// Register certificate for all domains found in the certificate
		for _, domain := range domains {
			domain = strings.ToLower(strings.TrimSpace(domain))
			newCerts[domain] = &cert
			m.logger.Info("loaded certificate",
				"domain", domain,
				"file", file.Name())
		}

		certCount++
	}

	if len(newCerts) == 0 {
		return fmt.Errorf("no valid certificates found in %s", m.certsDir)
	}

	// Atomically replace certificates
	m.certsMu.Lock()
	m.certs = newCerts
	m.certsMu.Unlock()

	m.logger.Info("certificates loaded",
		"certificates", certCount,
		"domains", len(newCerts))
	return nil
}

// extractDomainsFromCert parses a certificate file and extracts all domains
// from the Common Name (CN) and Subject Alternative Names (SANs)
func (m *Manager) extractDomainsFromCert(certPath string) ([]string, error) {
	// Read certificate file
	certData, err := os.ReadFile(certPath)
	if err != nil {
		return nil, fmt.Errorf("read certificate: %w", err)
	}

	// Decode PEM block
	var certPEM *pem.Block
	for {
		certPEM, certData = pem.Decode(certData)
		if certPEM == nil {
			break
		}
		if certPEM.Type == "CERTIFICATE" {
			break
		}
	}

	if certPEM == nil {
		return nil, fmt.Errorf("no CERTIFICATE block found in PEM")
	}

	// Parse certificate
	cert, err := x509.ParseCertificate(certPEM.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse certificate: %w", err)
	}

	domains := make([]string, 0)

	// Add Common Name (CN) if present
	if cert.Subject.CommonName != "" && cert.Subject.CommonName != "*" {
		domains = append(domains, cert.Subject.CommonName)
	}

	// Add Subject Alternative Names (SANs)
	domains = append(domains, cert.DNSNames...)

	return domains, nil
}

// watchCertificates watches for certificate file changes
func (m *Manager) watchCertificates() {
	for {
		select {
		case event, ok := <-m.watcher.Events:
			if !ok {
				return
			}

			// Reload on any write or create event for .pem files
			if (event.Op&fsnotify.Write == fsnotify.Write ||
			    event.Op&fsnotify.Create == fsnotify.Create) &&
			   strings.HasSuffix(event.Name, ".pem") {
				m.logger.Info("certificate change detected", "file", event.Name)
				if err := m.loadCertificates(); err != nil {
					m.logger.Error("failed to reload certificates", "error", err)
				} else {
					m.logger.Info("certificates reloaded successfully")
				}
			}

		case err, ok := <-m.watcher.Errors:
			if !ok {
				return
			}
			m.logger.Error("watcher error", "error", err)

		case <-m.stopCh:
			return
		}
	}
}

// GetCertificate returns a certificate for the given SNI hostname
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	sni := strings.ToLower(strings.TrimSpace(hello.ServerName))

	m.certsMu.RLock()
	defer m.certsMu.RUnlock()

	// Try exact match first
	if cert, ok := m.certs[sni]; ok {
		return cert, nil
	}

	// If SNI is empty or not found, return first available certificate
	if sni == "" {
		m.logger.Debug("no SNI provided, using fallback certificate")
	} else {
		m.logger.Debug("certificate not found for SNI, using fallback", "sni", sni)
	}

	for domain, cert := range m.certs {
		m.logger.Debug("using fallback certificate", "domain", domain)
		return cert, nil
	}

	return nil, fmt.Errorf("no certificates available")
}

// GetTLSConfig returns a TLS configuration for the proxy
func (m *Manager) GetTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion:               m.minVersion,
		GetCertificate:           m.GetCertificate,
		PreferServerCipherSuites: true,
	}
}

// Reload triggers a manual certificate reload
func (m *Manager) Reload() error {
	m.logger.Info("manual certificate reload triggered")
	return m.loadCertificates()
}

// Close stops the certificate watcher
func (m *Manager) Close() error {
	close(m.stopCh)
	return m.watcher.Close()
}

// GetCertCount returns the number of loaded certificates
func (m *Manager) GetCertCount() int {
	m.certsMu.RLock()
	defer m.certsMu.RUnlock()
	return len(m.certs)
}

// GetDomains returns a list of domains with loaded certificates
func (m *Manager) GetDomains() []string {
	m.certsMu.RLock()
	defer m.certsMu.RUnlock()

	domains := make([]string, 0, len(m.certs))
	for domain := range m.certs {
		domains = append(domains, domain)
	}
	return domains
}

// ParseTLSVersion parses a TLS version string to the corresponding constant
func ParseTLSVersion(version string) uint16 {
	switch version {
	case "1.0":
		return tls.VersionTLS10
	case "1.1":
		return tls.VersionTLS11
	case "1.2":
		return tls.VersionTLS12
	case "1.3":
		return tls.VersionTLS13
	default:
		return tls.VersionTLS12
	}
}
