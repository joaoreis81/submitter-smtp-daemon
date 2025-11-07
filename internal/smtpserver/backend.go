package smtpserver

import (
	"crypto/tls"

	"github.com/emersion/go-smtp"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/auth"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/config"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/logger"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/metrics"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/ratelimit"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/spool"
)

// Backend implements smtp.Backend
type Backend struct {
	config       *config.Config
	spool        *spool.Spool
	redisAuth    *auth.RedisAuthenticator
	ipAuth       *auth.IPAuthenticator
	rateLimiter  *ratelimit.Limiter
	metrics      *metrics.Metrics
	logger       *logger.Logger
}

// BackendConfig contains all dependencies for the SMTP backend
type BackendConfig struct {
	Config      *config.Config
	Spool       *spool.Spool
	RedisAuth   *auth.RedisAuthenticator
	IPAuth      *auth.IPAuthenticator
	RateLimiter *ratelimit.Limiter
	Metrics     *metrics.Metrics
	Logger      *logger.Logger
}

// NewBackend creates a new SMTP backend
func NewBackend(cfg BackendConfig) *Backend {
	return &Backend{
		config:      cfg.Config,
		spool:       cfg.Spool,
		redisAuth:   cfg.RedisAuth,
		ipAuth:      cfg.IPAuth,
		rateLimiter: cfg.RateLimiter,
		metrics:     cfg.Metrics,
		logger:      cfg.Logger.WithComponent("smtp-backend"),
	}
}

// AnonymousLogin is called when the client doesn't authenticate
func (b *Backend) AnonymousLogin(state *smtp.ConnectionState) (smtp.Session, error) {
	// We don't support anonymous login for outbound SMTP
	return nil, smtp.ErrAuthRequired
}

// Login is called for authenticated sessions
func (b *Backend) Login(state *smtp.ConnectionState, username, password string) (smtp.Session, error) {
	// This method is not used in our implementation
	// We handle authentication in Session.Auth instead
	return nil, smtp.ErrAuthRequired
}

// CreateServer creates a new SMTP server with the given configuration
func CreateServer(backend *Backend, cfg *config.Config, tlsConfig *tls.Config) *smtp.Server {
	s := smtp.NewServer(backend)

	// Server settings
	s.Domain = cfg.Server.Hostname
	s.ReadTimeout = cfg.SMTP.ReadTimeout
	s.WriteTimeout = cfg.SMTP.WriteTimeout
	s.MaxMessageBytes = int(cfg.SMTP.MaxMessageSize)
	s.MaxRecipients = cfg.SMTP.MaxRecipients

	// SMTP capabilities
	s.EnableSMTPUTF8 = false // Not supported yet
	s.EnableBINARYMIME = false
	s.EnableREQUIRETLS = false

	// Enable AUTH
	s.EnableAuth = func(mech string, conn *smtp.Conn) bool {
		// Support PLAIN and LOGIN
		if mech == "PLAIN" || mech == "LOGIN" {
			return true
		}
		return false
	}

	// TLS configuration
	if tlsConfig != nil {
		s.TLSConfig = tlsConfig
	}

	// Authentication required
	s.AuthDisabled = false

	return s
}
