package backend

import (
	"encoding/csv"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

// BackendConfig represents a backend server configuration from CSV
type BackendConfig struct {
	AuthSelector    string
	Host            string
	Port            int
	UseStartTLS     bool
	ImplicitTLS     bool
	SkipTLSVerify   bool
	Timeout         time.Duration
	RelayTimeout    time.Duration
	Active          bool
}

// Selector handles backend selection based on authenticated user
type Selector struct {
	userBackends   map[string]*BackendConfig // Exact email matches
	domainBackends map[string]*BackendConfig // Domain matches (@domain.tld)
	logger         *slog.Logger
}

// NewSelector creates a new backend selector by loading the CSV file
func NewSelector(csvPath string, logger *slog.Logger) (*Selector, error) {
	s := &Selector{
		userBackends:   make(map[string]*BackendConfig),
		domainBackends: make(map[string]*BackendConfig),
		logger:         logger,
	}

	if err := s.loadCSV(csvPath); err != nil {
		return nil, fmt.Errorf("load backends CSV: %w", err)
	}

	return s, nil
}

// loadCSV reads and parses the backends CSV file
func (s *Selector) loadCSV(csvPath string) error {
	file, err := os.Open(csvPath)
	if err != nil {
		return fmt.Errorf("open CSV file: %w", err)
	}
	defer file.Close()

	reader := csv.NewReader(file)
	reader.TrimLeadingSpace = true

	// Read all records
	records, err := reader.ReadAll()
	if err != nil {
		return fmt.Errorf("read CSV: %w", err)
	}

	if len(records) < 2 {
		return fmt.Errorf("CSV file is empty or has no data rows")
	}

	// Skip header row
	for i, record := range records[1:] {
		lineNum := i + 2 // +2 because we skipped header and arrays are 0-indexed

		if len(record) < 9 {
			s.logger.Warn("skipping malformed CSV line",
				"line", lineNum,
				"columns", len(record))
			continue
		}

		// Parse backend configuration
		backend, err := s.parseBackendConfig(record, lineNum)
		if err != nil {
			s.logger.Warn("skipping invalid backend config",
				"line", lineNum,
				"error", err)
			continue
		}

		// Only load active backends
		if !backend.Active {
			s.logger.Debug("skipping inactive backend",
				"auth_selector", backend.AuthSelector,
				"line", lineNum)
			continue
		}

		// Store in appropriate map
		authSelector := strings.TrimSpace(backend.AuthSelector)
		if strings.HasPrefix(authSelector, "@") {
			// Domain-based selector
			s.domainBackends[authSelector] = backend
			s.logger.Info("loaded domain backend",
				"domain", authSelector,
				"host", backend.Host,
				"port", backend.Port)
		} else {
			// User-based selector
			s.userBackends[authSelector] = backend
			s.logger.Info("loaded user backend",
				"user", authSelector,
				"host", backend.Host,
				"port", backend.Port)
		}
	}

	s.logger.Info("backends loaded",
		"user_backends", len(s.userBackends),
		"domain_backends", len(s.domainBackends))

	return nil
}

// parseBackendConfig parses a CSV record into a BackendConfig
func (s *Selector) parseBackendConfig(record []string, lineNum int) (*BackendConfig, error) {
	// Parse port
	port, err := strconv.Atoi(strings.TrimSpace(record[2]))
	if err != nil {
		return nil, fmt.Errorf("invalid port: %w", err)
	}

	// Parse boolean fields
	useStartTLS := parseBool(record[3])
	implicitTLS := parseBool(record[4])
	skipTLSVerify := parseBool(record[5])
	active := parseBool(record[8])

	// Parse timeouts
	timeout, err := time.ParseDuration(strings.TrimSpace(record[6]))
	if err != nil {
		return nil, fmt.Errorf("invalid timeout: %w", err)
	}

	relayTimeout, err := time.ParseDuration(strings.TrimSpace(record[7]))
	if err != nil {
		return nil, fmt.Errorf("invalid relay_timeout: %w", err)
	}

	return &BackendConfig{
		AuthSelector:  strings.TrimSpace(record[0]),
		Host:          strings.TrimSpace(record[1]),
		Port:          port,
		UseStartTLS:   useStartTLS,
		ImplicitTLS:   implicitTLS,
		SkipTLSVerify: skipTLSVerify,
		Timeout:       timeout,
		RelayTimeout:  relayTimeout,
		Active:        active,
	}, nil
}

// SelectBackend selects the appropriate backend for a given username
// Priority: exact user match → domain match → error
func (s *Selector) SelectBackend(username string) (*BackendConfig, error) {
	username = strings.ToLower(strings.TrimSpace(username))

	// Try exact user match first
	if backend, ok := s.userBackends[username]; ok {
		s.logger.Debug("selected user-specific backend",
			"user", username,
			"host", backend.Host,
			"port", backend.Port)
		return backend, nil
	}

	// Extract domain from username
	parts := strings.SplitN(username, "@", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid email format: %s", username)
	}

	domain := "@" + parts[1]

	// Try domain match
	if backend, ok := s.domainBackends[domain]; ok {
		s.logger.Debug("selected domain backend",
			"user", username,
			"domain", domain,
			"host", backend.Host,
			"port", backend.Port)
		return backend, nil
	}

	return nil, fmt.Errorf("no backend configured for user or domain: %s", username)
}

// parseBool parses a boolean value from string
func parseBool(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1" || s == "yes" || s == "on"
}
