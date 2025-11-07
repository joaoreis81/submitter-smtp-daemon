package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config is the root configuration structure
type Config struct {
	Server           ServerConfig           `yaml:"server"`
	SMTP             SMTPConfig             `yaml:"smtp"`
	TLS              TLSConfig              `yaml:"tls"`
	Auth             AuthConfig             `yaml:"auth"`
	RateLimit        RateLimitConfig        `yaml:"rate_limit"`
	Spool            SpoolConfig            `yaml:"spool"`
	Deduplication    DeduplicationConfig    `yaml:"deduplication"`
	Modification     ModificationConfig     `yaml:"modification"`
	DKIM             DKIMConfig             `yaml:"dkim"`
	OutputValidation OutputValidationConfig `yaml:"output_validation"`
	Delivery         DeliveryConfig         `yaml:"delivery"`
	Database         DatabaseConfig         `yaml:"database"`
	ClickHouse       ClickHouseConfig       `yaml:"clickhouse"`
	S3               S3Config               `yaml:"s3"`
	Workers          WorkersConfig          `yaml:"workers"`
	Observability    ObservabilityConfig    `yaml:"observability"`
	Redis            RedisConfig            `yaml:"redis"`
}

// ServerConfig contains server identity information
type ServerConfig struct {
	Hostname string `yaml:"hostname"`
	Version  string `yaml:"version"`
}

// SMTPConfig contains SMTP server settings
type SMTPConfig struct {
	Listeners      []ListenerConfig `yaml:"listeners"`
	MaxMessageSize int64            `yaml:"max_message_size"`
	MaxRecipients  int              `yaml:"max_recipients"`
	ReadTimeout    time.Duration    `yaml:"read_timeout"`
	WriteTimeout   time.Duration    `yaml:"write_timeout"`
	DSN            DSNConfig        `yaml:"dsn"`
}

// ListenerConfig defines a single SMTP listener
type ListenerConfig struct {
	IP          string `yaml:"ip"`
	Port        int    `yaml:"port"`
	Mode        string `yaml:"mode"`         // starttls, implicit_tls, plaintext
	RequireTLS  bool   `yaml:"require_tls"`
	RequireAuth bool   `yaml:"require_auth"`
}

// DSNConfig contains DSN (RFC 3461) settings
type DSNConfig struct {
	Enabled        bool `yaml:"enabled"`
	MaxEnvidLength int  `yaml:"max_envid_length"`
}

// TLSConfig contains TLS/SSL settings
type TLSConfig struct {
	CertsDir            string   `yaml:"certs_dir"`
	MinVersion          string   `yaml:"min_version"`
	PreferServerCiphers bool     `yaml:"prefer_server_ciphers"`
	CipherSuites        []string `yaml:"cipher_suites"`
}

// AuthConfig contains authentication settings
type AuthConfig struct {
	Redis       RedisAuthConfig `yaml:"redis"`
	IPAllowlist IPAllowConfig   `yaml:"ip_allowlist"`
}

// RedisAuthConfig for Redis user database
type RedisAuthConfig struct {
	Enabled   bool     `yaml:"enabled"`
	Addresses []string `yaml:"addresses"`
	Password  string   `yaml:"password"`
	DB        int      `yaml:"db"`
	PoolSize  int      `yaml:"pool_size"`
}

// IPAllowConfig for IP-based authentication
type IPAllowConfig struct {
	Enabled bool     `yaml:"enabled"`
	IPs     []string `yaml:"ips"`
}

// RateLimitConfig contains rate limiting settings
type RateLimitConfig struct {
	Enabled bool             `yaml:"enabled"`
	User    UserRateLimits   `yaml:"user"`
	IP      IPRateLimits     `yaml:"ip"`
	Global  GlobalRateLimits `yaml:"global"`
}

// UserRateLimits defines per-user rate limits
type UserRateLimits struct {
	MessagesPerHour   int `yaml:"messages_per_hour"`
	RecipientsPerHour int `yaml:"recipients_per_hour"`
}

// IPRateLimits defines per-IP rate limits
type IPRateLimits struct {
	ConnectionsPerMinute int `yaml:"connections_per_minute"`
	FailedAuthPerHour    int `yaml:"failed_auth_per_hour"`
}

// GlobalRateLimits defines global rate limits
type GlobalRateLimits struct {
	MessagesPerSecond     int `yaml:"messages_per_second"`
	ConcurrentConnections int `yaml:"concurrent_connections"`
}

// SpoolConfig contains spooling settings
type SpoolConfig struct {
	BasePath  string          `yaml:"base_path"`
	Retention RetentionConfig `yaml:"retention"`
}

// RetentionConfig defines how long to keep messages in each stage
type RetentionConfig struct {
	Incoming   time.Duration `yaml:"incoming"`
	Parsed     time.Duration `yaml:"parsed"`
	Filtered   time.Duration `yaml:"filtered"`
	Modified   time.Duration `yaml:"modified"`
	Signed     time.Duration `yaml:"signed"`
	Delivering time.Duration `yaml:"delivering"`
	Done       time.Duration `yaml:"done"`
	Failed     time.Duration `yaml:"failed"`
}

// DeduplicationConfig contains deduplication settings
type DeduplicationConfig struct {
	Enabled bool          `yaml:"enabled"`
	Window  time.Duration `yaml:"window"`
	Storage string        `yaml:"storage"` // redis, memory
}

// ModificationConfig contains message modification settings
type ModificationConfig struct {
	Enabled             bool                  `yaml:"enabled"`
	RulesReloadInterval time.Duration         `yaml:"rules_reload_interval"`
	Profiles            []ModificationProfile `yaml:"profiles"`
}

// ModificationProfile defines a set of modification rules
type ModificationProfile struct {
	Name         string       `yaml:"name"`
	Selector     string       `yaml:"selector"`
	AddHeaders   []HeaderRule `yaml:"add_headers"`
	InjectFooter FooterRule   `yaml:"inject_footer"`
}

// HeaderRule defines a header to add
type HeaderRule struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"`
}

// FooterRule defines footers to inject
type FooterRule struct {
	HTML string `yaml:"html"`
	Text string `yaml:"text"`
}

// DKIMConfig contains DKIM signing settings
type DKIMConfig struct {
	Enabled                bool         `yaml:"enabled"`
	KeysDir                string       `yaml:"keys_dir"`
	CanonicalizationHeader string       `yaml:"canonicalization_header"`
	CanonicalizationBody   string       `yaml:"canonicalization_body"`
	Domains                []DKIMDomain `yaml:"domains"`
}

// DKIMDomain defines per-domain DKIM configuration
type DKIMDomain struct {
	Domain     string `yaml:"domain"`
	Selector   string `yaml:"selector"`
	PrivateKey string `yaml:"private_key"`
}

// OutputValidationConfig contains output validation settings
type OutputValidationConfig struct {
	SPFCheck   bool `yaml:"spf_check"`
	DKIMVerify bool `yaml:"dkim_verify"`
}

// DeliveryConfig contains delivery settings
type DeliveryConfig struct {
	Workers        int               `yaml:"workers"`
	RetrySchedule  []time.Duration   `yaml:"retry_schedule"`
	MaxAttempts    int               `yaml:"max_attempts"`
	ConnectionPool PoolConfig        `yaml:"connection_pool"`
	MXCache        CacheConfig       `yaml:"mx_cache"`
	TLS            DeliveryTLSConfig `yaml:"tls"`
}

// PoolConfig contains connection pool settings
type PoolConfig struct {
	Enabled     bool          `yaml:"enabled"`
	MaxPerHost  int           `yaml:"max_per_host"`
	IdleTimeout time.Duration `yaml:"idle_timeout"`
}

// CacheConfig contains caching settings
type CacheConfig struct {
	Enabled bool          `yaml:"enabled"`
	TTL     time.Duration `yaml:"ttl"`
}

// DeliveryTLSConfig contains outbound TLS settings
type DeliveryTLSConfig struct {
	Enabled    bool   `yaml:"enabled"`
	Verify     bool   `yaml:"verify"`
	MinVersion string `yaml:"min_version"`
}

// DatabaseConfig contains SQLite settings
type DatabaseConfig struct {
	Path            string        `yaml:"path"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

// ClickHouseConfig contains ClickHouse settings
type ClickHouseConfig struct {
	Addresses        []string      `yaml:"addresses"`
	Database         string        `yaml:"database"`
	Username         string        `yaml:"username"`
	Password         string        `yaml:"password"`
	BufferSize       int           `yaml:"buffer_size"`
	FlushInterval    time.Duration `yaml:"flush_interval"`
	RetryMaxInterval time.Duration `yaml:"retry_max_interval"`
	Compression      bool          `yaml:"compression"`
}

// S3Config contains S3 archival settings
type S3Config struct {
	Enabled         bool   `yaml:"enabled"`
	Endpoint        string `yaml:"endpoint"`
	Region          string `yaml:"region"`
	Bucket          string `yaml:"bucket"`
	AccessKeyID     string `yaml:"access_key_id"`
	SecretAccessKey string `yaml:"secret_access_key"`
	PathPrefix      string `yaml:"path_prefix"`
	UseSSL          bool   `yaml:"use_ssl"`
	ArchiveOriginal bool   `yaml:"archive_original"`
	ArchiveFinal    bool   `yaml:"archive_final"`
}

// WorkersConfig defines worker pool sizes
type WorkersConfig struct {
	Parse    int `yaml:"parse"`
	Modify   int `yaml:"modify"`
	Sign     int `yaml:"sign"`
	Delivery int `yaml:"delivery"`
	Archive  int `yaml:"archive"`
}

// ObservabilityConfig contains logging and metrics settings
type ObservabilityConfig struct {
	LogLevel       string `yaml:"log_level"`
	LogFormat      string `yaml:"log_format"`
	MetricsEnabled bool   `yaml:"metrics_enabled"`
	MetricsListen  string `yaml:"metrics_listen"`
	HealthListen   string `yaml:"health_listen"`
}

// RedisConfig contains general Redis settings
type RedisConfig struct {
	Addresses    []string      `yaml:"addresses"`
	Password     string        `yaml:"password"`
	DB           int           `yaml:"db"`
	PoolSize     int           `yaml:"pool_size"`
	MaxRetries   int           `yaml:"max_retries"`
	DialTimeout  time.Duration `yaml:"dial_timeout"`
	ReadTimeout  time.Duration `yaml:"read_timeout"`
	WriteTimeout time.Duration `yaml:"write_timeout"`
}

// DefaultConfig returns a configuration with sensible defaults
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Hostname: "localhost",
			Version:  "1.0.0",
		},
		SMTP: SMTPConfig{
			Listeners: []ListenerConfig{
				{
					IP:          "0.0.0.0",
					Port:        587,
					Mode:        "starttls",
					RequireTLS:  true,
					RequireAuth: true,
				},
			},
			MaxMessageSize: 50 * 1024 * 1024, // 50MB
			MaxRecipients:  100,
			ReadTimeout:    120 * time.Second,
			WriteTimeout:   120 * time.Second,
			DSN: DSNConfig{
				Enabled:        true,
				MaxEnvidLength: 100,
			},
		},
		TLS: TLSConfig{
			CertsDir:            "./certs",
			MinVersion:          "1.2",
			PreferServerCiphers: true,
		},
		Auth: AuthConfig{
			Redis: RedisAuthConfig{
				Enabled:   true,
				Addresses: []string{"localhost:6379"},
				DB:        0,
				PoolSize:  10,
			},
			IPAllowlist: IPAllowConfig{
				Enabled: false,
				IPs:     []string{},
			},
		},
		RateLimit: RateLimitConfig{
			Enabled: true,
			User: UserRateLimits{
				MessagesPerHour:   1000,
				RecipientsPerHour: 10000,
			},
			IP: IPRateLimits{
				ConnectionsPerMinute: 60,
				FailedAuthPerHour:    10,
			},
			Global: GlobalRateLimits{
				MessagesPerSecond:     100,
				ConcurrentConnections: 1000,
			},
		},
		Spool: SpoolConfig{
			BasePath: "/var/spool/outbound",
			Retention: RetentionConfig{
				Incoming:   1 * time.Hour,
				Parsed:     1 * time.Hour,
				Filtered:   1 * time.Hour,
				Modified:   1 * time.Hour,
				Signed:     1 * time.Hour,
				Delivering: 24 * time.Hour,
				Done:       24 * time.Hour,
				Failed:     7 * 24 * time.Hour,
			},
		},
		Deduplication: DeduplicationConfig{
			Enabled: true,
			Window:  24 * time.Hour,
			Storage: "redis",
		},
		Modification: ModificationConfig{
			Enabled:             false,
			RulesReloadInterval: 30 * time.Second,
			Profiles:            []ModificationProfile{},
		},
		DKIM: DKIMConfig{
			Enabled:                true,
			KeysDir:                "./dkim",
			CanonicalizationHeader: "relaxed",
			CanonicalizationBody:   "relaxed",
			Domains:                []DKIMDomain{},
		},
		OutputValidation: OutputValidationConfig{
			SPFCheck:   false,
			DKIMVerify: true,
		},
		Delivery: DeliveryConfig{
			Workers: 16,
			RetrySchedule: []time.Duration{
				1 * time.Minute,
				5 * time.Minute,
				15 * time.Minute,
				1 * time.Hour,
				4 * time.Hour,
				8 * time.Hour,
				24 * time.Hour,
			},
			MaxAttempts: 7,
			ConnectionPool: PoolConfig{
				Enabled:     true,
				MaxPerHost:  10,
				IdleTimeout: 5 * time.Minute,
			},
			MXCache: CacheConfig{
				Enabled: true,
				TTL:     1 * time.Hour,
			},
			TLS: DeliveryTLSConfig{
				Enabled:    true,
				Verify:     true,
				MinVersion: "1.2",
			},
		},
		Database: DatabaseConfig{
			Path:            "./queue.db",
			MaxOpenConns:    25,
			MaxIdleConns:    5,
			ConnMaxLifetime: 5 * time.Minute,
		},
		ClickHouse: ClickHouseConfig{
			Addresses:        []string{"localhost:9000"},
			Database:         "outbound",
			Username:         "default",
			Password:         "",
			BufferSize:       100,
			FlushInterval:    10 * time.Second,
			RetryMaxInterval: 5 * time.Minute,
			Compression:      true,
		},
		S3: S3Config{
			Enabled:         false,
			Endpoint:        "",
			Region:          "us-east-1",
			Bucket:          "outbound-archive",
			PathPrefix:      "emails",
			UseSSL:          true,
			ArchiveOriginal: true,
			ArchiveFinal:    true,
		},
		Workers: WorkersConfig{
			Parse:    4,
			Modify:   4,
			Sign:     4,
			Delivery: 16,
			Archive:  4,
		},
		Observability: ObservabilityConfig{
			LogLevel:       "info",
			LogFormat:      "json",
			MetricsEnabled: true,
			MetricsListen:  ":9090",
			HealthListen:   ":8080",
		},
		Redis: RedisConfig{
			Addresses:    []string{"localhost:6379"},
			Password:     "",
			DB:           0,
			PoolSize:     50,
			MaxRetries:   3,
			DialTimeout:  5 * time.Second,
			ReadTimeout:  3 * time.Second,
			WriteTimeout: 3 * time.Second,
		},
	}
}

// Load loads configuration from a YAML file and applies environment overrides
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("reading config file: %w", err)
		}

		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file: %w", err)
		}
	}

	// Apply environment variable overrides
	applyEnvOverrides(cfg)

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return cfg, nil
}

// applyEnvOverrides applies environment variable overrides to the configuration
func applyEnvOverrides(cfg *Config) {
	// Server
	if v := os.Getenv("OUTBOUND_SERVER_HOSTNAME"); v != "" {
		cfg.Server.Hostname = v
	}

	// SMTP
	if v := os.Getenv("OUTBOUND_SMTP_MAX_MESSAGE_SIZE"); v != "" {
		if i, err := strconv.ParseInt(v, 10, 64); err == nil {
			cfg.SMTP.MaxMessageSize = i
		}
	}

	// Redis Auth
	if v := os.Getenv("OUTBOUND_REDIS_AUTH_ADDRESSES"); v != "" {
		cfg.Auth.Redis.Addresses = strings.Split(v, ",")
	}
	if v := os.Getenv("OUTBOUND_REDIS_AUTH_PASSWORD"); v != "" {
		cfg.Auth.Redis.Password = v
	}

	// General Redis
	if v := os.Getenv("OUTBOUND_REDIS_ADDRESSES"); v != "" {
		cfg.Redis.Addresses = strings.Split(v, ",")
	}
	if v := os.Getenv("OUTBOUND_REDIS_PASSWORD"); v != "" {
		cfg.Redis.Password = v
	}

	// ClickHouse
	if v := os.Getenv("OUTBOUND_CLICKHOUSE_ADDRESSES"); v != "" {
		cfg.ClickHouse.Addresses = strings.Split(v, ",")
	}
	if v := os.Getenv("OUTBOUND_CLICKHOUSE_PASSWORD"); v != "" {
		cfg.ClickHouse.Password = v
	}

	// S3
	if v := os.Getenv("OUTBOUND_S3_ENABLED"); v != "" {
		cfg.S3.Enabled = v == "true"
	}
	if v := os.Getenv("OUTBOUND_S3_ENDPOINT"); v != "" {
		cfg.S3.Endpoint = v
	}
	if v := os.Getenv("OUTBOUND_S3_BUCKET"); v != "" {
		cfg.S3.Bucket = v
	}
	if v := os.Getenv("OUTBOUND_S3_ACCESS_KEY_ID"); v != "" {
		cfg.S3.AccessKeyID = v
	}
	if v := os.Getenv("OUTBOUND_S3_SECRET_ACCESS_KEY"); v != "" {
		cfg.S3.SecretAccessKey = v
	}

	// Database
	if v := os.Getenv("OUTBOUND_DATABASE_PATH"); v != "" {
		cfg.Database.Path = v
	}

	// Spool
	if v := os.Getenv("OUTBOUND_SPOOL_BASE_PATH"); v != "" {
		cfg.Spool.BasePath = v
	}

	// Observability
	if v := os.Getenv("OUTBOUND_LOG_LEVEL"); v != "" {
		cfg.Observability.LogLevel = v
	}
}

// Validate validates the configuration
func (c *Config) Validate() error {
	if c.Server.Hostname == "" {
		return fmt.Errorf("server.hostname is required")
	}

	if len(c.SMTP.Listeners) == 0 {
		return fmt.Errorf("at least one SMTP listener is required")
	}

	for i, l := range c.SMTP.Listeners {
		if l.Port <= 0 || l.Port > 65535 {
			return fmt.Errorf("listener %d: invalid port %d", i, l.Port)
		}
		if l.Mode != "starttls" && l.Mode != "implicit_tls" && l.Mode != "plaintext" {
			return fmt.Errorf("listener %d: invalid mode %s", i, l.Mode)
		}
	}

	if c.SMTP.MaxMessageSize <= 0 {
		return fmt.Errorf("smtp.max_message_size must be positive")
	}

	if c.Auth.Redis.Enabled && len(c.Auth.Redis.Addresses) == 0 {
		return fmt.Errorf("auth.redis.addresses is required when Redis auth is enabled")
	}

	if c.Spool.BasePath == "" {
		return fmt.Errorf("spool.base_path is required")
	}

	if c.Database.Path == "" {
		return fmt.Errorf("database.path is required")
	}

	if c.ClickHouse.Database == "" {
		return fmt.Errorf("clickhouse.database is required")
	}

	return nil
}
