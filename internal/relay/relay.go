package relay

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/config"
	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/metrics"

	smtp "github.com/emersion/go-smtp"
)

// Relay handles message relay to the backend SMTP server
type Relay struct {
	cfg     *config.RelayConfig
	metrics *metrics.Metrics
	logger  *slog.Logger
}

// New creates a new relay
func New(cfg *config.RelayConfig, metrics *metrics.Metrics, logger *slog.Logger) *Relay {
	return &Relay{
		cfg:     cfg,
		metrics: metrics,
		logger:  logger,
	}
}

// MessageMetadata contains metadata about the message being relayed
type MessageMetadata struct {
	ClientIP       string
	AuthUser       string
	ServerName     string
	TLSEnabled     bool
	TLSType        string // "STARTTLS", "SSL", or "false"
	EHLOHostname   string
}

// SendWithClient relays a message using an existing authenticated SMTP client
func (r *Relay) SendWithClient(ctx context.Context, client *smtp.Client, from string, to []string, mailOpts *smtp.MailOptions, rcptOpts []*smtp.RcptOptions, data io.Reader, meta MessageMetadata, logger *slog.Logger) error {
	start := time.Now()
	r.metrics.RelayAttempts.Inc()

	defer func() {
		r.metrics.RelayDuration.Observe(time.Since(start).Seconds())
	}()

	logger.Info("relaying message via authenticated backend connection",
		"from", from,
		"recipients", len(to),
		"auth_user", meta.AuthUser,
		"client_addr", meta.ClientIP,
		"tls_type", meta.TLSType)

	// MAIL FROM with DSN options
	logger.Debug("sending MAIL FROM", "from", from)
	if mailOpts != nil {
		// Log DSN parameters being forwarded
		logFields := []any{"from", from}
		if mailOpts.Return != "" {
			logFields = append(logFields, "dsn_ret", mailOpts.Return)
		}
		if mailOpts.EnvelopeID != "" {
			logFields = append(logFields, "dsn_envid", mailOpts.EnvelopeID)
		}
		if len(logFields) > 2 {
			logger.Info("forwarding MAIL FROM DSN options to backend", logFields...)
		}
	}
	if err := client.Mail(from, mailOpts); err != nil {
		r.metrics.RelayFailures.Inc()
		logger.Error("MAIL FROM failed",
			"error", err,
			"from", from)
		return fmt.Errorf("MAIL FROM: %w", err)
	}

	// RCPT TO with DSN options
	logger.Debug("sending RCPT TO", "recipients", len(to))
	for i, rcpt := range to {
		var opts *smtp.RcptOptions
		if i < len(rcptOpts) {
			opts = rcptOpts[i]
			// Log DSN parameters being forwarded
			if opts != nil && (opts.Notify != nil || opts.OriginalRecipient != "") {
				logFields := []any{"recipient", rcpt}
				if opts.Notify != nil && len(opts.Notify) > 0 {
					notifyStrs := make([]string, len(opts.Notify))
					for i, n := range opts.Notify {
						notifyStrs[i] = string(n)
					}
					logFields = append(logFields, "dsn_notify", strings.Join(notifyStrs, ","))
				}
				if opts.OriginalRecipient != "" {
					logFields = append(logFields, "dsn_orcpt", opts.OriginalRecipient)
				}
				logger.Info("forwarding RCPT TO DSN options to backend", logFields...)
			}
		}
		if err := client.Rcpt(rcpt, opts); err != nil {
			r.metrics.RelayFailures.Inc()
			logger.Error("RCPT TO failed",
				"error", err,
				"rcpt", rcpt)
			return fmt.Errorf("RCPT TO %s: %w", rcpt, err)
		}
	}

	// DATA
	wc, err := client.Data()
	if err != nil {
		r.metrics.RelayFailures.Inc()
		logger.Error("DATA command failed", "error", err)
		return fmt.Errorf("DATA: %w", err)
	}

	// Write metadata headers
	if err := r.writeMetadataHeaders(wc, meta); err != nil {
		_ = wc.Close()
		r.metrics.RelayFailures.Inc()
		return fmt.Errorf("write headers: %w", err)
	}

	// Stream message data
	bytesWritten, err := io.Copy(wc, data)
	if err != nil {
		_ = wc.Close()
		r.metrics.RelayFailures.Inc()
		logger.Error("failed to write message data", "error", err)
		return fmt.Errorf("write data: %w", err)
	}

	if err := wc.Close(); err != nil {
		r.metrics.RelayFailures.Inc()
		logger.Error("failed to close data writer", "error", err)
		return fmt.Errorf("close data: %w", err)
	}

	r.metrics.RelaySuccess.Inc()
	r.metrics.MessagesSent.Inc()
	r.metrics.BytesSent.Add(float64(bytesWritten))

	logger.Info("message relayed successfully via authenticated connection",
		"from", from,
		"recipients", len(to),
		"bytes", bytesWritten,
		"auth_user", meta.AuthUser)

	return nil
}

// Send relays a message to the backend
func (r *Relay) Send(ctx context.Context, from string, to []string, data io.Reader, meta MessageMetadata) error {
	start := time.Now()
	r.metrics.RelayAttempts.Inc()

	defer func() {
		r.metrics.RelayDuration.Observe(time.Since(start).Seconds())
	}()

	backendAddr := fmt.Sprintf("%s:%d", r.cfg.Host, r.cfg.Port)
	r.logger.Info("starting message relay",
		"from", from,
		"recipients", len(to),
		"backend", backendAddr,
		"auth_user", meta.AuthUser)

	// Dial backend
	dialStart := time.Now()
	r.logger.Debug("connecting to relay backend",
		"backend", backendAddr,
		"use_starttls", r.cfg.UseStartTLS,
		"implicit_tls", r.cfg.ImplicitTLS)

	c, _, err := r.dialBackend(ctx)
	if err != nil {
		r.metrics.RelayFailures.Inc()
		r.logger.Error("failed to connect to relay backend",
			"error", err,
			"from", from,
			"backend", backendAddr,
			"dial_duration", time.Since(dialStart))
		return fmt.Errorf("connect to backend: %w", err)
	}
	defer func() {
		_ = c.Quit()
	}()

	r.logger.Info("connected to relay backend",
		"backend", backendAddr,
		"dial_duration", time.Since(dialStart))

	// MAIL FROM
	r.logger.Debug("sending MAIL FROM", "from", from)
	if err := c.Mail(from, nil); err != nil {
		r.metrics.RelayFailures.Inc()
		r.logger.Error("MAIL FROM failed",
			"error", err,
			"from", from,
			"backend", backendAddr)
		return fmt.Errorf("MAIL FROM: %w", err)
	}

	// RCPT TO
	r.logger.Debug("sending RCPT TO", "recipients", len(to))
	for _, rcpt := range to {
		if err := c.Rcpt(rcpt, nil); err != nil {
			r.metrics.RelayFailures.Inc()
			r.logger.Error("RCPT TO failed",
				"error", err,
				"rcpt", rcpt,
				"backend", backendAddr)
			return fmt.Errorf("RCPT TO %s: %w", rcpt, err)
		}
	}

	// DATA
	wc, err := c.Data()
	if err != nil {
		r.metrics.RelayFailures.Inc()
		r.logger.Error("DATA command failed", "error", err)
		return fmt.Errorf("DATA: %w", err)
	}

	// Write metadata headers
	if err := r.writeMetadataHeaders(wc, meta); err != nil {
		_ = wc.Close()
		r.metrics.RelayFailures.Inc()
		return fmt.Errorf("write headers: %w", err)
	}

	// Stream message data
	bytesWritten, err := io.Copy(wc, data)
	if err != nil {
		_ = wc.Close()
		r.metrics.RelayFailures.Inc()
		r.logger.Error("failed to write message data", "error", err)
		return fmt.Errorf("write data: %w", err)
	}

	if err := wc.Close(); err != nil {
		r.metrics.RelayFailures.Inc()
		r.logger.Error("failed to close data writer", "error", err)
		return fmt.Errorf("close data: %w", err)
	}

	r.metrics.RelaySuccess.Inc()
	r.metrics.MessagesSent.Inc()
	r.metrics.BytesSent.Add(float64(bytesWritten))

	r.logger.Info("message relayed successfully",
		"from", from,
		"recipients", len(to),
		"bytes", bytesWritten,
		"auth_user", meta.AuthUser)

	return nil
}

// writeMetadataHeaders writes X-Original-* headers to track edge connection metadata
func (r *Relay) writeMetadataHeaders(w io.Writer, meta MessageMetadata) error {
	var headers strings.Builder

	headers.WriteString(fmt.Sprintf("X-Original-Client-IP: %s\r\n", meta.ClientIP))
	headers.WriteString(fmt.Sprintf("X-Original-Auth-User: %s\r\n", meta.AuthUser))
	headers.WriteString(fmt.Sprintf("X-Original-Server-Name: %s\r\n", meta.ServerName))
	headers.WriteString(fmt.Sprintf("X-Edge-Received-TLS: %v\r\n", meta.TLSEnabled))
	headers.WriteString(fmt.Sprintf("X-Secure-Conn: %s\r\n", meta.TLSType))

	if meta.EHLOHostname != "" {
		headers.WriteString(fmt.Sprintf("X-Original-EHLO: %s\r\n", meta.EHLOHostname))
	}

	_, err := io.WriteString(w, headers.String())
	return err
}

// dialBackend establishes a connection to the relay backend
func (r *Relay) dialBackend(ctx context.Context) (*smtp.Client, net.Conn, error) {
	address := fmt.Sprintf("%s:%d", r.cfg.Host, r.cfg.Port)

	var c *smtp.Client
	var err error
	var conn net.Conn

	// Use appropriate dial method based on configuration
	if r.cfg.ImplicitTLS {
		// Implicit TLS (SMTPS)
		tlsConf := &tls.Config{
			ServerName:         r.cfg.Host,
			InsecureSkipVerify: r.cfg.SkipTLSVerify,
		}
		c, err = smtp.DialTLS(address, tlsConf)
		if err != nil {
			return nil, nil, fmt.Errorf("dial TLS: %w", err)
		}
		conn = nil
	} else if r.cfg.UseStartTLS {
		// STARTTLS
		tlsConf := &tls.Config{
			ServerName:         r.cfg.Host,
			InsecureSkipVerify: r.cfg.SkipTLSVerify,
		}
		c, err = smtp.DialStartTLS(address, tlsConf)
		if err != nil {
			return nil, nil, fmt.Errorf("dial STARTTLS: %w", err)
		}
		conn = nil
	} else {
		// Plain SMTP
		c, err = smtp.Dial(address)
		if err != nil {
			return nil, nil, fmt.Errorf("dial: %w", err)
		}
		conn = nil
	}

	return c, conn, nil
}
