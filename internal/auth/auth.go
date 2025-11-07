package auth

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"time"

	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/config"
	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/metrics"
	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/ratelimit"

	smtp "github.com/emersion/go-smtp"
	"github.com/emersion/go-sasl"
)

// Authenticator handles SMTP authentication verification against a backend
type Authenticator struct {
	cfg       *config.BackendConfig
	metrics   *metrics.Metrics
	limiter   *ratelimit.Limiter
	logger    *slog.Logger
}

// New creates a new authenticator
func New(cfg *config.BackendConfig, metrics *metrics.Metrics, limiter *ratelimit.Limiter, logger *slog.Logger) *Authenticator {
	return &Authenticator{
		cfg:     cfg,
		metrics: metrics,
		limiter: limiter,
		logger:  logger,
	}
}

// Verify authenticates credentials against the backend SMTP server
func (a *Authenticator) Verify(ctx context.Context, username, password, clientIP string) error {
	start := time.Now()
	a.metrics.AuthAttempts.Inc()

	defer func() {
		a.metrics.AuthDuration.Observe(time.Since(start).Seconds())
	}()

	backendAddr := fmt.Sprintf("%s:%d", a.cfg.Host, a.cfg.Port)
	a.logger.Info("starting backend authentication",
		"username", username,
		"ip", clientIP,
		"backend", backendAddr,
		"use_starttls", a.cfg.UseStartTLS)

	// Check rate limits
	if !a.limiter.CheckIP(clientIP) {
		a.metrics.RateLimitHits.Inc()
		a.logger.Warn("rate limit exceeded for IP",
			"ip", clientIP,
			"username", username)
		return smtp.ErrAuthFailed
	}

	if !a.limiter.CheckUser(username) {
		a.metrics.RateLimitHits.Inc()
		a.logger.Warn("rate limit exceeded for user",
			"username", username,
			"ip", clientIP)
		return smtp.ErrAuthFailed
	}

	// Dial backend
	dialStart := time.Now()
	a.logger.Debug("connecting to auth backend",
		"backend", backendAddr,
		"use_starttls", a.cfg.UseStartTLS,
		"implicit_tls", a.cfg.ImplicitTLS,
		"timeout", a.cfg.Timeout)

	c, _, err := a.dialBackend(ctx)
	if err != nil {
		a.metrics.AuthFailures.Inc()
		a.logger.Error("failed to connect to auth backend",
			"error", err,
			"username", username,
			"backend", backendAddr,
			"dial_duration", time.Since(dialStart),
			"total_duration", time.Since(start))
		return smtp.ErrAuthFailed
	}
	defer func() {
		_ = c.Close()
	}()

	a.logger.Info("connected to auth backend",
		"backend", backendAddr,
		"dial_duration", time.Since(dialStart))

	// Check AUTH extension
	if ok, _ := c.Extension("AUTH"); !ok {
		a.metrics.AuthFailures.Inc()
		a.logger.Error("backend does not support AUTH extension",
			"backend", backendAddr)
		return smtp.ErrAuthFailed
	}

	a.logger.Debug("backend supports AUTH, attempting authentication",
		"username", username,
		"backend", backendAddr)

	// Attempt authentication using SASL PLAIN
	authStart := time.Now()
	saslClient := sasl.NewPlainClient("", username, password)
	if err := c.Auth(saslClient); err != nil {
		a.metrics.AuthFailures.Inc()
		a.logger.Info("backend authentication rejected",
			"username", username,
			"ip", clientIP,
			"backend", backendAddr,
			"error", err,
			"auth_duration", time.Since(authStart),
			"total_duration", time.Since(start))
		return smtp.ErrAuthFailed
	}

	a.metrics.AuthSuccess.Inc()
	a.logger.Info("backend authentication successful",
		"username", username,
		"ip", clientIP,
		"backend", backendAddr,
		"auth_duration", time.Since(authStart),
		"total_duration", time.Since(start))

	return nil
}

// VerifyAndConnect authenticates credentials and returns an authenticated SMTP client
// Uses provided backendCfg for backend connection settings
func (a *Authenticator) VerifyAndConnect(ctx context.Context, username, password, clientIP string, backendCfg *config.BackendConfig, logger *slog.Logger) (*smtp.Client, error) {
	start := time.Now()
	a.metrics.AuthAttempts.Inc()

	defer func() {
		a.metrics.AuthDuration.Observe(time.Since(start).Seconds())
	}()

	backendAddr := fmt.Sprintf("%s:%d", backendCfg.Host, backendCfg.Port)
	logger.Info("starting backend authentication with persistent connection",
		"username", username,
		"client_addr", clientIP,
		"backend", backendAddr,
		"use_starttls", backendCfg.UseStartTLS)

	// Check rate limits
	if !a.limiter.CheckIP(clientIP) {
		a.metrics.RateLimitHits.Inc()
		logger.Warn("rate limit exceeded for IP",
			"client_addr", clientIP,
			"username", username)
		return nil, smtp.ErrAuthFailed
	}

	if !a.limiter.CheckUser(username) {
		a.metrics.RateLimitHits.Inc()
		logger.Warn("rate limit exceeded for user",
			"username", username,
			"client_addr", clientIP)
		return nil, smtp.ErrAuthFailed
	}

	// Dial backend
	dialStart := time.Now()
	logger.Debug("connecting to auth backend",
		"backend", backendAddr,
		"use_starttls", backendCfg.UseStartTLS,
		"implicit_tls", backendCfg.ImplicitTLS,
		"timeout", backendCfg.Timeout)

	c, _, err := a.dialBackendWithConfig(ctx, backendCfg)
	if err != nil {
		a.metrics.AuthFailures.Inc()
		logger.Error("failed to connect to auth backend",
			"error", err,
			"username", username,
			"backend", backendAddr,
			"dial_duration", time.Since(dialStart),
			"total_duration", time.Since(start))
		return nil, smtp.ErrAuthFailed
	}

	logger.Info("connected to auth backend",
		"backend", backendAddr,
		"dial_duration", time.Since(dialStart))

	// Check AUTH extension
	if ok, _ := c.Extension("AUTH"); !ok {
		a.metrics.AuthFailures.Inc()
		logger.Error("backend does not support AUTH extension",
			"backend", backendAddr)
		_ = c.Close()
		return nil, smtp.ErrAuthFailed
	}

	logger.Debug("backend supports AUTH, attempting authentication",
		"username", username,
		"backend", backendAddr)

	// Attempt authentication using SASL PLAIN
	authStart := time.Now()
	saslClient := sasl.NewPlainClient("", username, password)
	if err := c.Auth(saslClient); err != nil {
		a.metrics.AuthFailures.Inc()
		logger.Info("backend authentication rejected",
			"username", username,
			"client_addr", clientIP,
			"backend", backendAddr,
			"error", err,
			"auth_duration", time.Since(authStart),
			"total_duration", time.Since(start))
		_ = c.Close()
		return nil, smtp.ErrAuthFailed
	}

	a.metrics.AuthSuccess.Inc()
	logger.Info("backend authentication successful, keeping connection for relay",
		"username", username,
		"client_addr", clientIP,
		"backend", backendAddr,
		"auth_duration", time.Since(authStart),
		"total_duration", time.Since(start))

	// Return authenticated client for message relay
	return c, nil
}

// dialBackend establishes a connection to the backend SMTP server
func (a *Authenticator) dialBackend(ctx context.Context) (*smtp.Client, net.Conn, error) {
	return a.dialBackendWithConfig(ctx, a.cfg)
}

// dialBackendWithConfig establishes a connection using specific backend configuration
func (a *Authenticator) dialBackendWithConfig(ctx context.Context, cfg *config.BackendConfig) (*smtp.Client, net.Conn, error) {
	address := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)

	var c *smtp.Client
	var err error

	// Use appropriate dial method based on configuration
	if cfg.ImplicitTLS {
		// Implicit TLS (SMTPS)
		tlsConf := &tls.Config{
			ServerName:         cfg.Host,
			InsecureSkipVerify: cfg.SkipTLSVerify,
		}
		c, err = smtp.DialTLS(address, tlsConf)
		if err != nil {
			return nil, nil, fmt.Errorf("dial TLS: %w", err)
		}
	} else if cfg.UseStartTLS {
		// STARTTLS
		tlsConf := &tls.Config{
			ServerName:         cfg.Host,
			InsecureSkipVerify: cfg.SkipTLSVerify,
		}
		c, err = smtp.DialStartTLS(address, tlsConf)
		if err != nil {
			return nil, nil, fmt.Errorf("starttls: %w", err)
		}
	} else {
		// Plain SMTP
		c, err = smtp.Dial(address)
		if err != nil {
			return nil, nil, fmt.Errorf("dial: %w", err)
		}
	}

	return c, nil, nil
}

