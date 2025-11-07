package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/auth"
	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/backend"
	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/config"
	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/metrics"
	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/relay"

	smtp "github.com/emersion/go-smtp"
	"github.com/emersion/go-sasl"
	"github.com/google/uuid"
)

// Backend implements smtp.Backend interface
type Backend struct {
	cfg             *config.Config
	authenticator   *auth.Authenticator
	relay           *relay.Relay
	metrics         *metrics.Metrics
	logger          *slog.Logger
	security        *SecurityChecker
	backendSelector *backend.Selector
}

// NewBackend creates a new SMTP backend
func NewBackend(
	cfg *config.Config,
	authenticator *auth.Authenticator,
	relay *relay.Relay,
	metrics *metrics.Metrics,
	logger *slog.Logger,
	backendSelector *backend.Selector,
) *Backend {
	return &Backend{
		cfg:             cfg,
		authenticator:   authenticator,
		relay:           relay,
		metrics:         metrics,
		logger:          logger,
		security:        NewSecurityChecker(cfg.Security.IPAllowList, cfg.Security.IPDenyList),
		backendSelector: backendSelector,
	}
}

// NewSession creates a new SMTP session
func (b *Backend) NewSession(c *smtp.Conn) (smtp.Session, error) {
	// Check IP allow/deny lists
	remoteAddr := c.Conn().RemoteAddr().String()
	ip := extractIP(remoteAddr)

	if !b.security.IsAllowed(ip) {
		b.logger.Warn("connection from denied IP", "ip", ip)
		return nil, smtp.ErrAuthRequired
	}

	requestID := uuid.New().String()

	// Check if TLS is already active (for logging purposes)
	tlsState, hasTLS := c.TLSConnectionState()

	// Get the local port to determine listener type
	localAddr := c.Conn().LocalAddr().String()

	// Build log fields
	logFields := []any{
		"request_id", requestID,
		"remote_addr", remoteAddr,
		"client_ip", ip,
		"local_addr", localAddr,
		"implicit_tls", hasTLS,
	}

	// Add TLS connection details if available
	if hasTLS {
		logFields = append(logFields,
			"tls_version", tlsVersionString(tlsState.Version),
			"tls_cipher", tls.CipherSuiteName(tlsState.CipherSuite),
			"tls_server_name", tlsState.ServerName,
		)
	}

	b.logger.Info("new session", logFields...)

	return &Session{
		backend:   b,
		requestID: requestID,
		conn:      c,
		clientIP:  ip,
		hostname:  c.Hostname(),
		logger:    b.logger.With("request_id", requestID),
		localAddr: localAddr,
	}, nil
}

// Session implements smtp.Session and smtp.AuthSession interfaces
type Session struct {
	backend        *Backend
	requestID      string
	conn           *smtp.Conn
	clientIP       string
	hostname       string
	authUser       string
	authPassword   string        // Store password for backend relay authentication
	backendClient  *smtp.Client  // Persistent authenticated backend connection
	mailFrom       string
	rcptTo         []string
	mailOpts       *smtp.MailOptions // DSN and other MAIL FROM options
	rcptOpts       []*smtp.RcptOptions // DSN and other RCPT TO options (one per recipient)
	logger         *slog.Logger
	localAddr      string        // Local address (includes port) to determine listener type
}

// DSN validation and logging helpers

// validateMailOptions validates MAIL FROM DSN options
func validateMailOptions(opts *smtp.MailOptions) error {
	if opts == nil {
		return nil
	}

	// Validate RET parameter (if present)
	if opts.Return != "" && opts.Return != "FULL" && opts.Return != "HDRS" {
		return fmt.Errorf("invalid RET value: %s (must be FULL or HDRS)", opts.Return)
	}

	// ENVID is just an identifier, any value is acceptable
	// Size and other parameters don't need validation here

	return nil
}

// validateRcptOptions validates RCPT TO DSN options
func validateRcptOptions(opts *smtp.RcptOptions) error {
	if opts == nil {
		return nil
	}

	// Validate NOTIFY parameter (if present)
	if opts.Notify != nil {
		for _, notify := range opts.Notify {
			if notify != "NEVER" && notify != "SUCCESS" && notify != "FAILURE" && notify != "DELAY" {
				return fmt.Errorf("invalid NOTIFY value: %s (must be NEVER, SUCCESS, FAILURE, or DELAY)", notify)
			}
		}
		// Check for invalid combinations
		for _, notify := range opts.Notify {
			if notify == "NEVER" && len(opts.Notify) > 1 {
				return fmt.Errorf("NOTIFY=NEVER cannot be combined with other values")
			}
		}
	}

	// ORCPT is just an address, any value is acceptable

	return nil
}

// logMailOptions logs MAIL FROM DSN options
func logMailOptions(logger *slog.Logger, opts *smtp.MailOptions) {
	if opts == nil {
		return
	}

	logFields := []any{}
	if opts.Return != "" {
		logFields = append(logFields, "dsn_ret", opts.Return)
	}
	if opts.EnvelopeID != "" {
		logFields = append(logFields, "dsn_envid", opts.EnvelopeID)
	}
	if opts.Size != 0 {
		logFields = append(logFields, "size", opts.Size)
	}
	if opts.Auth != nil {
		logFields = append(logFields, "auth", *opts.Auth)
	}
	if opts.Body != "" {
		logFields = append(logFields, "body", opts.Body)
	}

	if len(logFields) > 0 {
		logger.Info("MAIL FROM options", logFields...)
	}
}

// logRcptOptions logs RCPT TO DSN options
func logRcptOptions(logger *slog.Logger, rcpt string, opts *smtp.RcptOptions) {
	if opts == nil {
		return
	}

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

	if len(logFields) > 1 { // More than just recipient
		logger.Info("RCPT TO options", logFields...)
	}
}

// AuthMechanisms returns supported auth mechanisms
func (s *Session) AuthMechanisms() []string {
	return []string{sasl.Plain, sasl.Login}
}

// Auth provides a SASL server for authentication
func (s *Session) Auth(mech string) (sasl.Server, error) {
	// Check TLS requirement
	if s.backend.cfg.SMTP.RequireTLS {
		if _, ok := s.conn.TLSConnectionState(); !ok {
			s.logger.Warn("authentication attempted without TLS", "ip", s.clientIP)
			return nil, smtp.ErrAuthRequired
		}
	}

	switch mech {
	case sasl.Plain:
		return sasl.NewPlainServer(func(identity, username, password string) error {
			return s.authenticate(username, password)
		}), nil
	case sasl.Login:
		return NewLoginServer(func(username, password string) error {
			return s.authenticate(username, password)
		}), nil
	default:
		return nil, smtp.ErrAuthUnsupported
	}
}

// authenticate verifies credentials against the backend and establishes persistent connection
func (s *Session) authenticate(username, password string) error {
	s.logger.Info("authentication attempt",
		"username", username,
		"ip", s.clientIP)

	// Select backend based on username
	backendCfg, err := s.backend.backendSelector.SelectBackend(username)
	if err != nil {
		s.logger.Error("no backend configured for user",
			"username", username,
			"error", err)
		return smtp.ErrAuthFailed
	}

	s.logger.Info("selected backend for user",
		"username", username,
		"backend", fmt.Sprintf("%s:%d", backendCfg.Host, backendCfg.Port))

	// Convert backend.BackendConfig to config.BackendConfig
	cfgBackend := &config.BackendConfig{
		Host:          backendCfg.Host,
		Port:          backendCfg.Port,
		UseStartTLS:   backendCfg.UseStartTLS,
		ImplicitTLS:   backendCfg.ImplicitTLS,
		SkipTLSVerify: backendCfg.SkipTLSVerify,
		Timeout:       backendCfg.Timeout,
		RelayTimeout:  backendCfg.RelayTimeout,
	}

	ctx, cancel := context.WithTimeout(context.Background(), backendCfg.Timeout)
	defer cancel()

	// Verify credentials and establish backend connection
	backendClient, err := s.backend.authenticator.VerifyAndConnect(ctx, username, password, s.clientIP, cfgBackend, s.logger)
	if err != nil {
		return err
	}

	// Store credentials and connection for message relay
	s.authUser = username
	s.authPassword = password
	s.backendClient = backendClient
	s.logger = s.logger.With("auth_user", username)
	s.logger.Info("session authenticated with backend connection established")

	return nil
}

// Mail handles MAIL FROM command
func (s *Session) Mail(from string, opts *smtp.MailOptions) error {
	s.logger.Info("MAIL FROM", "from", from)

	if s.authUser == "" {
		s.logger.Warn("MAIL FROM without authentication")
		return smtp.ErrAuthRequired
	}

	// Validate DSN options
	if err := validateMailOptions(opts); err != nil {
		s.logger.Warn("invalid MAIL FROM options", "error", err)
		return &smtp.SMTPError{
			Code:         501,
			EnhancedCode: smtp.EnhancedCode{5, 5, 4},
			Message:      fmt.Sprintf("Invalid parameter: %v", err),
		}
	}

	// Log DSN options if present
	logMailOptions(s.logger, opts)

	// Store for relay
	s.mailFrom = from
	s.mailOpts = opts
	s.rcptTo = nil
	s.rcptOpts = nil

	return nil
}

// Rcpt handles RCPT TO command
func (s *Session) Rcpt(to string, opts *smtp.RcptOptions) error {
	s.logger.Info("RCPT TO", "to", to)

	// Validate DSN options
	if err := validateRcptOptions(opts); err != nil {
		s.logger.Warn("invalid RCPT TO options", "recipient", to, "error", err)
		return &smtp.SMTPError{
			Code:         501,
			EnhancedCode: smtp.EnhancedCode{5, 5, 4},
			Message:      fmt.Sprintf("Invalid parameter: %v", err),
		}
	}

	// Log DSN options if present
	logRcptOptions(s.logger, to, opts)

	// Note: MaxRecipients limit is enforced by go-smtp library (smtp.Server.MaxRecipients)
	// No need to check here as the library will reject excess recipients before calling this method

	s.rcptTo = append(s.rcptTo, to)
	s.rcptOpts = append(s.rcptOpts, opts)
	return nil
}

// Data handles message DATA
func (s *Session) Data(r io.Reader) error {
	s.logger.Info("DATA",
		"from", s.mailFrom,
		"recipients", len(s.rcptTo))

	s.backend.metrics.MessagesReceived.Inc()

	// Ensure we have an authenticated backend connection
	if s.backendClient == nil {
		s.logger.Error("no authenticated backend connection available")
		return &smtp.SMTPError{
			Code:         451,
			EnhancedCode: smtp.EnhancedCode{4, 0, 0},
			Message:      "Authentication required",
		}
	}

	// Create metadata for relay
	tlsState, hasTLS := s.conn.TLSConnectionState()
	tlsEnabled := hasTLS && tlsState.Version != 0

	// Determine TLS type based on listener configuration
	// Check if this is an implicit TLS listener by examining the local port
	// We determine this by checking which listener was used based on the config
	var tlsType string
	if !tlsEnabled {
		tlsType = "false"
	} else {
		// Determine if this connection used implicit TLS or STARTTLS
		// by checking the listener configuration from the config
		// Port 465 is typically implicit TLS, port 587 is STARTTLS
		// But we should check the actual listener config, not assume by port

		// Get the listener configuration from the config
		isImplicitTLS := false
		for _, listenerCfg := range s.backend.cfg.Listeners.GetAllListeners() {
			if listenerCfg.Addr == s.localAddr ||
			   strings.HasSuffix(s.localAddr, listenerCfg.Addr) ||
			   (strings.HasPrefix(listenerCfg.Addr, ":") && strings.HasSuffix(s.localAddr, listenerCfg.Addr)) {
				isImplicitTLS = listenerCfg.ImplicitTLS
				break
			}
		}

		if isImplicitTLS {
			tlsType = "SSL"  // Implicit TLS
		} else {
			tlsType = "STARTTLS"  // Upgraded via STARTTLS
		}
	}

	meta := relay.MessageMetadata{
		ClientIP:     s.clientIP,
		AuthUser:     s.authUser,
		ServerName:   s.hostname,
		TLSEnabled:   tlsEnabled,
		TLSType:      tlsType,
		EHLOHostname: s.hostname,
	}

	// Relay message using the authenticated backend connection
	relayTimeout := s.backend.cfg.Backend.RelayTimeout
	if relayTimeout == 0 {
		relayTimeout = 30 * time.Second // Default if not set
	}
	ctx, cancel := context.WithTimeout(context.Background(), relayTimeout+60*time.Second)
	defer cancel()

	if err := s.backend.relay.SendWithClient(ctx, s.backendClient, s.mailFrom, s.rcptTo, s.mailOpts, s.rcptOpts, r, meta, s.logger); err != nil {
		s.logger.Error("relay failed", "error", err)
		return &smtp.SMTPError{
			Code:         451,
			EnhancedCode: smtp.EnhancedCode{4, 0, 0},
			Message:      "Temporary failure, please try again later",
		}
	}

	return nil
}

// Reset resets the session state
func (s *Session) Reset() {
	s.logger.Debug("session reset")
	s.mailFrom = ""
	s.rcptTo = nil
	s.mailOpts = nil
	s.rcptOpts = nil
}

// Logout ends the session
func (s *Session) Logout() error {
	s.logger.Info("session logout")

	// Close backend connection if it exists
	if s.backendClient != nil {
		s.logger.Debug("closing backend connection")
		if err := s.backendClient.Quit(); err != nil {
			s.logger.Warn("error closing backend connection", "error", err)
		}
		s.backendClient = nil
	}

	return nil
}

// extractIP extracts the IP address from a remote address string
func extractIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}

// SecurityChecker handles IP allow/deny list checking
type SecurityChecker struct {
	allowList []string
	denyList  []string
}

// NewSecurityChecker creates a new security checker
func NewSecurityChecker(allowList, denyList []string) *SecurityChecker {
	return &SecurityChecker{
		allowList: allowList,
		denyList:  denyList,
	}
}

// IsAllowed checks if an IP is allowed to connect
func (sc *SecurityChecker) IsAllowed(ip string) bool {
	// If allow list is configured, IP must be in it
	if len(sc.allowList) > 0 {
		allowed := false
		for _, allowedIP := range sc.allowList {
			if matchesIP(ip, allowedIP) {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}

	// Check deny list
	for _, deniedIP := range sc.denyList {
		if matchesIP(ip, deniedIP) {
			return false
		}
	}

	return true
}

// matchesIP checks if an IP matches a pattern (supports CIDR notation)
func matchesIP(ip, pattern string) bool {
	// Check for CIDR notation
	if strings.Contains(pattern, "/") {
		_, network, err := net.ParseCIDR(pattern)
		if err != nil {
			return false
		}
		ipAddr := net.ParseIP(ip)
		if ipAddr == nil {
			return false
		}
		return network.Contains(ipAddr)
	}

	// Exact match
	return ip == pattern
}

// NewLoginServer creates a SASL LOGIN server
func NewLoginServer(auth func(username, password string) error) sasl.Server {
	return &loginServer{auth: auth}
}

type loginServer struct {
	auth     func(username, password string) error
	username string
	password string
	step     int
}

func (s *loginServer) Next(response []byte) (challenge []byte, done bool, err error) {
	switch s.step {
	case 0:
		s.step = 1
		return []byte("Username:"), false, nil
	case 1:
		s.username = string(response)
		s.step = 2
		return []byte("Password:"), false, nil
	case 2:
		s.password = string(response)
		if err := s.auth(s.username, s.password); err != nil {
			return nil, true, err
		}
		return nil, true, nil
	default:
		return nil, true, errors.New("unexpected LOGIN state")
	}
}

// tlsVersionString converts TLS version constant to string
func tlsVersionString(version uint16) string {
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
		return fmt.Sprintf("unknown(0x%04x)", version)
	}
}

