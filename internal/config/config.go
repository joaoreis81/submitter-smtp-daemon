package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config represents the complete application configuration
type Config struct {
	Listeners   ListenersConfig   `yaml:"listeners"`
	TLS         TLSConfig         `yaml:"tls"`
	Backend     BackendConfig     `yaml:"backend"`
	Relay       RelayConfig       `yaml:"relay"`
	SMTP        SMTPConfig        `yaml:"smtp"`
	Security    SecurityConfig    `yaml:"security"`
	Limits      LimitsConfig      `yaml:"limits"`
	Observability ObservabilityConfig `yaml:"observability"`
}

type ListenersConfig struct {
	// Legacy format (backward compatible)
	Submission string `yaml:"submission"` // Port 587
	SMTPS      string `yaml:"smtps"`      // Port 465

	// New format: list of listeners with explicit TLS mode
	Listeners []ListenerConfig `yaml:"listeners"`
}

type ListenerConfig struct {
	Addr        string `yaml:"addr"`         // Address to bind (e.g., ":587", "127.0.0.1:587", "[::1]:587")
	ImplicitTLS bool   `yaml:"implicit_tls"` // true for SMTPS (port 465), false for STARTTLS (port 587)
}

type TLSConfig struct {
	MinVersion        string   `yaml:"min_version"`
	CertsDir          string   `yaml:"certs_dir"`
	CipherSuites      []string `yaml:"cipher_suites"`
	PreferServerCipher bool     `yaml:"prefer_server_cipher"`
}

type BackendConfig struct {
	BackendsFile  string        `yaml:"backends_file"`  // CSV file with per-user/domain backends
	Host         string        `yaml:"host"`
	Port         int           `yaml:"port"`
	UseStartTLS  bool          `yaml:"use_starttls"`
	ImplicitTLS  bool          `yaml:"implicit_tls"`
	SkipTLSVerify bool         `yaml:"skip_tls_verify"`
	Timeout      time.Duration `yaml:"timeout"`
	RelayTimeout time.Duration `yaml:"relay_timeout"` // Timeout for relay operations
}

type RelayConfig struct {
	Host          string        `yaml:"host"`
	Port          int           `yaml:"port"`
	UseStartTLS   bool          `yaml:"use_starttls"`
	ImplicitTLS   bool          `yaml:"implicit_tls"`
	SkipTLSVerify bool          `yaml:"skip_tls_verify"`
	Timeout       time.Duration `yaml:"timeout"`
	PoolEnabled   bool          `yaml:"pool_enabled"`
	PoolMaxIdle   int           `yaml:"pool_max_idle"`
	PoolMaxActive int           `yaml:"pool_max_active"`
}

type SMTPConfig struct {
	BannerHostname string              `yaml:"banner_hostname"`
	Capabilities   SMTPCapabilities    `yaml:"capabilities"`
	RequireTLS     bool                `yaml:"require_tls"`
	AllowInsecureAuth bool             `yaml:"allow_insecure_auth"`
}

type SMTPCapabilities struct {
	Pipelining          bool  `yaml:"pipelining"`
	Size                bool  `yaml:"size"`
	SizeLimit           int64 `yaml:"size_limit"`
	EnhancedStatusCodes bool  `yaml:"enhanced_status_codes"`
	EightBitMIME        bool  `yaml:"eight_bit_mime"`
	DSN                 bool  `yaml:"dsn"`
	Chunking            bool  `yaml:"chunking"`
	SMTPUTF8            bool  `yaml:"smtputf8"`
	BinaryMIME          bool  `yaml:"binary_mime"`
}

type SecurityConfig struct {
	IPAllowList       []string `yaml:"ip_allow_list"`
	IPDenyList        []string `yaml:"ip_deny_list"`
	RateLimitEnabled  bool     `yaml:"rate_limit_enabled"`
	RateLimitPerIP    int      `yaml:"rate_limit_per_ip"`
	RateLimitPerUser  int      `yaml:"rate_limit_per_user"`
	RateLimitWindow   time.Duration `yaml:"rate_limit_window"`
	ARCEnabled        bool     `yaml:"arc_enabled"`
}

type LimitsConfig struct {
	MaxRecipients       int           `yaml:"max_recipients"`
	MaxConnections      int           `yaml:"max_connections"`
	MaxConnectionsPerIP int           `yaml:"max_connections_per_ip"`
	ReadTimeout         time.Duration `yaml:"read_timeout"`
	WriteTimeout        time.Duration `yaml:"write_timeout"`
	IdleTimeout         time.Duration `yaml:"idle_timeout"`
}

type ObservabilityConfig struct {
	LogLevel          string `yaml:"log_level"`
	LogFormat         string `yaml:"log_format"` // "json" or "text"
	MetricsEnabled    bool   `yaml:"metrics_enabled"`
	MetricsAddr       string `yaml:"metrics_addr"`
	HealthAddr        string `yaml:"health_addr"`
	TracingEnabled    bool   `yaml:"tracing_enabled"`
	TracingEndpoint   string `yaml:"tracing_endpoint"`
	TracingServiceName string `yaml:"tracing_service_name"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Listeners: ListenersConfig{
			Submission: ":587",
			SMTPS:      ":465",
		},
		TLS: TLSConfig{
			MinVersion:         "1.2",
			CertsDir:           "certs",
			CipherSuites:       []string{},
			PreferServerCipher: true,
		},
		Backend: BackendConfig{
			BackendsFile:  "backends.csv",
			Host:         "172.23.20.40",
			Port:         587,
			UseStartTLS:  true,
			ImplicitTLS:  false,
			SkipTLSVerify: false,
			Timeout:      10 * time.Second,
			RelayTimeout: 30 * time.Second,
		},
		Relay: RelayConfig{
			Host:          "172.23.20.40",
			Port:          25,
			UseStartTLS:   false,
			ImplicitTLS:   false,
			SkipTLSVerify: false,
			Timeout:       30 * time.Second,
			PoolEnabled:   false,
			PoolMaxIdle:   10,
			PoolMaxActive: 100,
		},
		SMTP: SMTPConfig{
			BannerHostname:    "edge-smtp.intesys.io",
			RequireTLS:        true,
			AllowInsecureAuth: false,
			Capabilities: SMTPCapabilities{
				Pipelining:          true,
				Size:                true,
				SizeLimit:           150000000, // 150MB
				EnhancedStatusCodes: true,
				EightBitMIME:        true,
				DSN:                 true,
				Chunking:            true,
				SMTPUTF8:            true,
				BinaryMIME:          true,
			},
		},
		Security: SecurityConfig{
			IPAllowList:      []string{},
			IPDenyList:       []string{},
			RateLimitEnabled: true,
			RateLimitPerIP:   10,
			RateLimitPerUser: 100,
			RateLimitWindow:  1 * time.Minute,
			ARCEnabled:       false,
		},
		Limits: LimitsConfig{
			MaxRecipients:       200,
			MaxConnections:      1000,
			MaxConnectionsPerIP: 10,
			ReadTimeout:         120 * time.Second,
			WriteTimeout:        120 * time.Second,
			IdleTimeout:         300 * time.Second,
		},
		Observability: ObservabilityConfig{
			LogLevel:           "info",
			LogFormat:          "json",
			MetricsEnabled:     true,
			MetricsAddr:        ":9090",
			HealthAddr:         ":8080",
			TracingEnabled:     false,
			TracingEndpoint:    "",
			TracingServiceName: "smtp-edge-proxy",
		},
	}
}

// LoadConfig loads configuration from a YAML file and applies environment variable overrides
func LoadConfig(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read config file: %w", err)
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parse config file: %w", err)
		}
	}

	// Apply environment variable overrides
	applyEnvOverrides(cfg)

	return cfg, nil
}

// applyEnvOverrides applies environment variable overrides to the configuration
func applyEnvOverrides(cfg *Config) {
	// Listeners
	if v := os.Getenv("SMTP_LISTENER_SUBMISSION"); v != "" {
		cfg.Listeners.Submission = v
	}
	if v := os.Getenv("SMTP_LISTENER_SMTPS"); v != "" {
		cfg.Listeners.SMTPS = v
	}

	// TLS
	if v := os.Getenv("SMTP_TLS_MIN_VERSION"); v != "" {
		cfg.TLS.MinVersion = v
	}
	if v := os.Getenv("SMTP_TLS_CERTS_DIR"); v != "" {
		cfg.TLS.CertsDir = v
	}

	// Backend
	if v := os.Getenv("SMTP_BACKEND_HOST"); v != "" {
		cfg.Backend.Host = v
	}
	if v := os.Getenv("SMTP_BACKEND_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Backend.Port = port
		}
	}
	if v := os.Getenv("SMTP_BACKEND_USE_STARTTLS"); v != "" {
		cfg.Backend.UseStartTLS = parseBool(v)
	}
	if v := os.Getenv("SMTP_BACKEND_SKIP_TLS_VERIFY"); v != "" {
		cfg.Backend.SkipTLSVerify = parseBool(v)
	}

	// Relay
	if v := os.Getenv("SMTP_RELAY_HOST"); v != "" {
		cfg.Relay.Host = v
	}
	if v := os.Getenv("SMTP_RELAY_PORT"); v != "" {
		if port, err := strconv.Atoi(v); err == nil {
			cfg.Relay.Port = port
		}
	}

	// SMTP
	if v := os.Getenv("SMTP_BANNER_HOSTNAME"); v != "" {
		cfg.SMTP.BannerHostname = v
	}
	if v := os.Getenv("SMTP_REQUIRE_TLS"); v != "" {
		cfg.SMTP.RequireTLS = parseBool(v)
	}
	if v := os.Getenv("SMTP_SIZE_LIMIT"); v != "" {
		if size, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.SMTP.Capabilities.SizeLimit = size
		}
	}

	// Security
	if v := os.Getenv("SMTP_IP_ALLOW_LIST"); v != "" {
		cfg.Security.IPAllowList = strings.Split(v, ",")
	}
	if v := os.Getenv("SMTP_IP_DENY_LIST"); v != "" {
		cfg.Security.IPDenyList = strings.Split(v, ",")
	}
	if v := os.Getenv("SMTP_RATE_LIMIT_ENABLED"); v != "" {
		cfg.Security.RateLimitEnabled = parseBool(v)
	}

	// Observability
	if v := os.Getenv("SMTP_LOG_LEVEL"); v != "" {
		cfg.Observability.LogLevel = v
	}
	if v := os.Getenv("SMTP_LOG_FORMAT"); v != "" {
		cfg.Observability.LogFormat = v
	}
	if v := os.Getenv("SMTP_METRICS_ADDR"); v != "" {
		cfg.Observability.MetricsAddr = v
	}
	if v := os.Getenv("SMTP_HEALTH_ADDR"); v != "" {
		cfg.Observability.HealthAddr = v
	}
}

func parseBool(s string) bool {
	s = strings.ToLower(s)
	return s == "true" || s == "1" || s == "yes" || s == "on"
}

// GetAllListeners returns all configured listeners (both legacy and new format)
func (c *ListenersConfig) GetAllListeners() []ListenerConfig {
	listeners := make([]ListenerConfig, 0)

	// Add new format listeners if configured
	if len(c.Listeners) > 0 {
		listeners = append(listeners, c.Listeners...)
	} else {
		// Fall back to legacy format
		if c.Submission != "" {
			listeners = append(listeners, ListenerConfig{
				Addr:        c.Submission,
				ImplicitTLS: false,
			})
		}
		if c.SMTPS != "" {
			listeners = append(listeners, ListenerConfig{
				Addr:        c.SMTPS,
				ImplicitTLS: true,
			})
		}
	}

	return listeners
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.TLS.CertsDir == "" {
		return fmt.Errorf("tls.certs_dir is required")
	}
	if c.Backend.Host == "" {
		return fmt.Errorf("backend.host is required")
	}
	if c.Relay.Host == "" {
		return fmt.Errorf("relay.host is required")
	}
	if c.SMTP.BannerHostname == "" {
		return fmt.Errorf("smtp.banner_hostname is required")
	}

	// Validate that at least one listener is configured
	listeners := c.Listeners.GetAllListeners()
	if len(listeners) == 0 {
		return fmt.Errorf("at least one listener must be configured")
	}

	return nil
}
