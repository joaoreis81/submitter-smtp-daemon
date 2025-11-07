package smtpserver

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/google/uuid"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/auth"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/logger"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/metrics"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/ratelimit"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/spool"
)

// Session represents an SMTP session
type Session struct {
	backend        *Backend
	sessionID      string
	clientIP       string
	authUser       string
	authenticated  bool
	mailFrom       string
	rcptTo         []string
	tlsState       *tls.ConnectionState
	logger         *logger.Logger
	startTime      time.Time
}

// NewSession creates a new SMTP session
func (b *Backend) NewSession(conn *smtp.Conn) (smtp.Session, error) {
	sessionID := uuid.New().String()
	clientIP := extractIP(conn.Conn().RemoteAddr().String())

	// Check IP connection rate limit
	if err := b.rateLimiter.CheckIPConnection(clientIP); err != nil {
		b.logger.WithClientIP(clientIP).Warn("Connection rate limit exceeded")
		return nil, &smtp.SMTPError{
			Code:    421,
			Message: "Rate limit exceeded",
		}
	}

	// Increment global connection counter
	b.rateLimiter.IncrementConnection()

	// Record connection metrics
	tlsMode := "plaintext"
	if conn.TLSConnectionState() != nil {
		tlsMode = "tls"
	}
	b.metrics.RecordConnection(fmt.Sprintf("%d", b.config.SMTP.Listeners[0].Port), tlsMode)

	sess := &Session{
		backend:   b,
		sessionID: sessionID,
		clientIP:  clientIP,
		logger:    b.logger.WithSessionID(sessionID).WithClientIP(clientIP),
		startTime: time.Now(),
	}

	// Log TLS state if available
	if conn.TLSConnectionState() != nil {
		sess.tlsState = conn.TLSConnectionState()
		sess.logger = sess.logger.WithFields(map[string]interface{}{
			"tls_version": tlsVersionName(sess.tlsState.Version),
			"tls_cipher":  tls.CipherSuiteName(sess.tlsState.CipherSuite),
		})
	}

	sess.logger.Info("New SMTP session started")
	return sess, nil
}

// Auth handles authentication
func (s *Session) Auth(mech string) (smtp.SASLServer, error) {
	return &saslServer{
		session: s,
		mech:    mech,
	}, nil
}

// Mail handles MAIL FROM command
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	// Require authentication
	if !s.authenticated {
		s.logger.Warn("MAIL command without authentication")
		return &smtp.SMTPError{
			Code:    530,
			Message: "Authentication required",
		}
	}

	// Check user message rate limit
	if err := s.backend.rateLimiter.CheckUserMessages(s.authUser); err != nil {
		s.logger.WithUser(s.authUser).Warn("User message rate limit exceeded")
		s.backend.metrics.RecordMessageRejected("rate_limited")
		return &smtp.SMTPError{
			Code:    450,
			Message: "Rate limit exceeded",
		}
	}

	// Check global message rate limit
	if err := s.backend.rateLimiter.CheckGlobalMessages(); err != nil {
		s.logger.Warn("Global message rate limit exceeded")
		s.backend.metrics.RecordMessageRejected("rate_limited")
		return &smtp.SMTPError{
			Code:    450,
			Message: "Rate limit exceeded",
		}
	}

	s.mailFrom = from
	s.logger.WithFields(map[string]interface{}{
		"mail_from": from,
	}).Debug("MAIL FROM accepted")

	return nil
}

// Rcpt handles RCPT TO command
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	if s.mailFrom == "" {
		return &smtp.SMTPError{
			Code:    503,
			Message: "MAIL command required first",
		}
	}

	// Check max recipients
	if len(s.rcptTo) >= s.backend.config.SMTP.MaxRecipients {
		s.logger.Warnf("Too many recipients: %d", len(s.rcptTo)+1)
		return &smtp.SMTPError{
			Code:    452,
			Message: "Too many recipients",
		}
	}

	s.rcptTo = append(s.rcptTo, to)
	s.logger.WithFields(map[string]interface{}{
		"rcpt_to":       to,
		"rcpt_count":    len(s.rcptTo),
	}).Debug("RCPT TO accepted")

	return nil
}

// Data handles DATA command and message content
func (s *Session) Data(r io.Reader) error {
	if len(s.rcptTo) == 0 {
		return &smtp.SMTPError{
			Code:    503,
			Message: "RCPT command required first",
		}
	}

	// Check user recipient rate limit
	if err := s.backend.rateLimiter.CheckUserRecipients(s.authUser, len(s.rcptTo)); err != nil {
		s.logger.WithUser(s.authUser).Warn("User recipient rate limit exceeded")
		s.backend.metrics.RecordMessageRejected("rate_limited")
		return &smtp.SMTPError{
			Code:    450,
			Message: "Recipient rate limit exceeded",
		}
	}

	// Read message data
	var buf bytes.Buffer
	limitedReader := io.LimitReader(r, s.backend.config.SMTP.MaxMessageSize+1)
	n, err := io.Copy(&buf, limitedReader)
	if err != nil {
		s.logger.WithError(err).Error("Failed to read message data")
		s.backend.metrics.RecordMessageRejected("read_error")
		return &smtp.SMTPError{
			Code:    451,
			Message: "Failed to read message",
		}
	}

	// Check message size
	if n > s.backend.config.SMTP.MaxMessageSize {
		s.logger.Warnf("Message too large: %d bytes", n)
		s.backend.metrics.RecordMessageRejected("too_large")
		return &smtp.SMTPError{
			Code:    552,
			Message: "Message too large",
		}
	}

	// Generate message ID
	msgID := uuid.New().String()
	s.logger = s.logger.WithMessageID(msgID)

	// Write to spool
	if err := s.backend.spool.Write(msgID, buf.Bytes()); err != nil {
		s.logger.WithError(err).Error("Failed to write message to spool")
		s.backend.metrics.RecordMessageRejected("spool_error")
		return &smtp.SMTPError{
			Code:    451,
			Message: "Failed to accept message",
		}
	}

	// Record metrics
	s.backend.metrics.RecordMessageAccepted(s.authUser, n)
	s.backend.metrics.RecordStage("incoming", 0) // Duration tracked separately

	s.logger.WithFields(map[string]interface{}{
		"message_size": n,
		"recipients":   len(s.rcptTo),
	}).Info("Message accepted and spooled")

	return nil
}

// Reset resets the session state
func (s *Session) Reset() {
	s.mailFrom = ""
	s.rcptTo = nil
	s.logger.Debug("Session reset")
}

// Logout handles session cleanup
func (s *Session) Logout() error {
	duration := time.Since(s.startTime).Seconds()

	// Decrement global connection counter
	s.backend.rateLimiter.DecrementConnection()

	// Record connection close metrics
	s.backend.metrics.RecordConnectionClose(fmt.Sprintf("%d", s.backend.config.SMTP.Listeners[0].Port), duration)

	s.logger.WithFields(map[string]interface{}{
		"duration_seconds": duration,
		"authenticated":    s.authenticated,
	}).Info("SMTP session ended")

	return nil
}

// saslServer implements SASL authentication
type saslServer struct {
	session  *Session
	mech     string
	username string
	password string
	step     int
}

// Next handles SASL authentication steps
func (s *saslServer) Next(response []byte) (challenge []byte, done bool, err error) {
	switch s.mech {
	case "PLAIN":
		return s.nextPlain(response)
	case "LOGIN":
		return s.nextLogin(response)
	default:
		return nil, false, fmt.Errorf("unsupported SASL mechanism: %s", s.mech)
	}
}

// nextPlain handles SASL PLAIN authentication
func (s *saslServer) nextPlain(response []byte) ([]byte, bool, error) {
	// PLAIN format: \x00username\x00password
	parts := bytes.Split(response, []byte{0})
	if len(parts) != 3 {
		s.session.logger.Warn("Invalid SASL PLAIN format")
		return nil, false, fmt.Errorf("invalid PLAIN format")
	}

	username := string(parts[1])
	password := string(parts[2])

	return s.authenticate(username, password)
}

// nextLogin handles SASL LOGIN authentication
func (s *saslServer) nextLogin(response []byte) ([]byte, bool, error) {
	s.step++

	switch s.step {
	case 1:
		// Request username
		return []byte("Username:"), false, nil
	case 2:
		// Receive username
		s.username = string(response)
		return []byte("Password:"), false, nil
	case 3:
		// Receive password
		s.password = string(response)
		return s.authenticate(s.username, s.password)
	default:
		return nil, false, fmt.Errorf("invalid LOGIN step")
	}
}

// authenticate performs the actual authentication
func (s *saslServer) authenticate(username, password string) ([]byte, bool, error) {
	start := time.Now()
	logger := s.session.logger.WithUser(username)

	// Check if IP is in allowlist (no password required)
	if s.session.backend.ipAuth != nil && s.session.backend.ipAuth.IsAllowed(s.session.clientIP) {
		s.session.authUser = username
		s.session.authenticated = true
		s.session.backend.metrics.RecordAuthAttempt("ip", "success", time.Since(start).Seconds())
		logger.Info("Authentication successful via IP allowlist")
		return nil, true, nil
	}

	// Check IP failed auth rate limit
	if err := s.session.backend.rateLimiter.CheckIPFailedAuth(s.session.clientIP); err != nil {
		logger.Warn("IP failed auth rate limit exceeded")
		s.session.backend.metrics.RecordAuthAttempt("redis", "rate_limited", time.Since(start).Seconds())
		return nil, false, &smtp.SMTPError{
			Code:    421,
			Message: "Too many failed authentication attempts",
		}
	}

	// Authenticate against Redis
	if err := s.session.backend.redisAuth.Authenticate(username, password); err != nil {
		logger.Warn("Authentication failed")
		return nil, false, &smtp.SMTPError{
			Code:    535,
			Message: "Authentication failed",
		}
	}

	// Authentication successful
	s.session.authUser = username
	s.session.authenticated = true
	logger.Info("Authentication successful")

	return nil, true, nil
}

// extractIP extracts IP address from remote addr string
func extractIP(remoteAddr string) string {
	// remoteAddr format: "IP:port" or "[IPv6]:port"
	parts := strings.Split(remoteAddr, ":")
	if len(parts) >= 2 {
		// Handle IPv6 with brackets
		ip := strings.Trim(parts[0], "[]")
		return ip
	}
	return remoteAddr
}

// tlsVersionName returns a human-readable TLS version name
func tlsVersionName(version uint16) string {
	switch version {
	case tls.VersionTLS10:
		return "TLS1.0"
	case tls.VersionTLS11:
		return "TLS1.1"
	case tls.VersionTLS12:
		return "TLS1.2"
	case tls.VersionTLS13:
		return "TLS1.3"
	default:
		return fmt.Sprintf("Unknown(0x%x)", version)
	}
}
