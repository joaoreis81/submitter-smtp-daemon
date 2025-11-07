package clickhouse

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/joaoreis81/submitter-smtp-daemon/internal/logger"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/metrics"
)

// Client is a buffered ClickHouse client
type Client struct {
	logger  *logger.Logger
	metrics *metrics.Metrics
	config  Config

	buffer     []*Event
	bufferMu   sync.Mutex
	stopChan   chan struct{}
	flushTimer *time.Ticker
}

// Config contains ClickHouse configuration
type Config struct {
	Addresses        []string
	Database         string
	Username         string
	Password         string
	BufferSize       int
	FlushInterval    time.Duration
	RetryMaxInterval time.Duration
	Compression      bool
	Logger           *logger.Logger
	Metrics          *metrics.Metrics
}

// Event represents an event to be logged to ClickHouse
type Event struct {
	// Event identification
	EventID     string
	EventTime   time.Time
	MsgID       string
	SessionID   string

	// Connection details
	ClientIP       string
	ClientPort     int
	ClientRDNS     string
	ListenerIP     string
	ListenerPort   int

	// TLS details
	TLSVersion  string
	TLSCipher   string
	SNIHostname string
	TLSMode     string

	// Authentication
	AuthUser     string
	AuthMethod   string
	AuthResult   string
	AuthDuration int

	// Message envelope
	MailFrom    string
	RcptTo      []string
	MessageSize int

	// Message metadata
	Subject        string
	MessageID      string
	Date           string
	FromHeader     string
	ToHeader       string

	// Processing pipeline
	Stage         string
	StageDuration int

	// Deduplication
	BodyHash            string
	DedupDuplicate      bool
	DedupOriginalMsgID  string

	// DKIM signing
	DKIMEnabled  bool
	DKIMSigned   bool
	DKIMSelector string
	DKIMDomain   string
	DKIMResult   string
	DKIMError    string

	// Modification
	Modified             bool
	ModificationProfile  string
	HeadersAdded         []string
	HeadersRemoved       []string

	// Delivery details
	Recipient         string
	MXHostname        string
	MXIP              string
	MXPort            int
	SMTPCode          int
	SMTPMessage       string
	DeliveryDuration  int
	DeliveryResult    string
	DeliveryAttempt   int
	NextRetryTime     time.Time

	// DSN details
	DSNRet    string
	DSNEnvid  string
	DSNNotify []string
	DSNOrcpt  string

	// Performance metrics
	TotalDuration int
	QueueWait     int

	// Rate limiting
	RateLimited     bool
	RateLimitType   string

	// Error tracking
	ErrorOccurred bool
	ErrorMessage  string
	ErrorCode     string

	// Additional metadata
	Tags         []string
	CustomFields map[string]string
}

// New creates a new buffered ClickHouse client
func New(cfg Config) *Client {
	client := &Client{
		logger:     cfg.Logger.WithComponent("clickhouse"),
		metrics:    cfg.Metrics,
		config:     cfg,
		buffer:     make([]*Event, 0, cfg.BufferSize),
		stopChan:   make(chan struct{}),
		flushTimer: time.NewTicker(cfg.FlushInterval),
	}

	// Start flush worker
	go client.flushWorker()

	client.logger.Info("ClickHouse client initialized (buffered mode)")
	return client
}

// LogEvent adds an event to the buffer
func (c *Client) LogEvent(event *Event) {
	c.bufferMu.Lock()
	defer c.bufferMu.Unlock()

	c.buffer = append(c.buffer, event)
	c.metrics.SetClickHouseBufferSize(len(c.buffer))

	// Flush if buffer is full
	if len(c.buffer) >= c.config.BufferSize {
		c.logger.Debug("Buffer full, triggering flush")
		go c.flush()
	}
}

// flush sends buffered events to ClickHouse
func (c *Client) flush() {
	c.bufferMu.Lock()
	if len(c.buffer) == 0 {
		c.bufferMu.Unlock()
		return
	}

	// Copy buffer and reset
	events := make([]*Event, len(c.buffer))
	copy(events, c.buffer)
	c.buffer = c.buffer[:0]
	c.bufferMu.Unlock()

	c.logger.Debugf("Flushing %d events to ClickHouse", len(events))

	// In a real implementation, this would use clickhouse-go to insert events
	// For now, we'll just log and record metrics
	// TODO: Implement actual ClickHouse insertion

	for _, event := range events {
		c.metrics.RecordClickHouseEvent("success")
		c.logger.WithFields(map[string]interface{}{
			"event_id": event.EventID,
			"msg_id":   event.MsgID,
			"stage":    event.Stage,
		}).Debug("Event logged (buffered)")
	}

	c.logger.Infof("Flushed %d events to ClickHouse", len(events))
}

// flushWorker periodically flushes the buffer
func (c *Client) flushWorker() {
	for {
		select {
		case <-c.flushTimer.C:
			c.flush()
		case <-c.stopChan:
			// Final flush
			c.flush()
			c.logger.Info("ClickHouse flush worker stopped")
			return
		}
	}
}

// Close stops the client and flushes remaining events
func (c *Client) Close() error {
	close(c.stopChan)
	c.flushTimer.Stop()
	time.Sleep(100 * time.Millisecond) // Allow flush worker to complete
	return nil
}

// GetBufferSize returns the current buffer size
func (c *Client) GetBufferSize() int {
	c.bufferMu.Lock()
	defer c.bufferMu.Unlock()
	return len(c.buffer)
}

// LogConnectionEvent logs a connection event
func (c *Client) LogConnectionEvent(sessionID, clientIP, tlsVersion, tlsCipher, sniHostname, tlsMode string, listenerPort int) {
	event := &Event{
		EventID:      fmt.Sprintf("%s-conn", sessionID),
		EventTime:    time.Now(),
		SessionID:    sessionID,
		ClientIP:     clientIP,
		TLSVersion:   tlsVersion,
		TLSCipher:    tlsCipher,
		SNIHostname:  sniHostname,
		TLSMode:      tlsMode,
		ListenerPort: listenerPort,
		Stage:        "connection",
	}
	c.LogEvent(event)
}

// LogAuthEvent logs an authentication event
func (c *Client) LogAuthEvent(sessionID, authUser, authMethod, authResult string, duration time.Duration) {
	event := &Event{
		EventID:      fmt.Sprintf("%s-auth", sessionID),
		EventTime:    time.Now(),
		SessionID:    sessionID,
		AuthUser:     authUser,
		AuthMethod:   authMethod,
		AuthResult:   authResult,
		AuthDuration: int(duration.Milliseconds()),
		Stage:        "authentication",
	}
	c.LogEvent(event)
}

// LogMessageEvent logs a message acceptance event
func (c *Client) LogMessageEvent(msgID, sessionID, authUser, clientIP, mailFrom string, rcptTo []string, size int) {
	event := &Event{
		EventID:     fmt.Sprintf("%s-accept", msgID),
		EventTime:   time.Now(),
		MsgID:       msgID,
		SessionID:   sessionID,
		AuthUser:    authUser,
		ClientIP:    clientIP,
		MailFrom:    mailFrom,
		RcptTo:      rcptTo,
		MessageSize: size,
		Stage:       "accepted",
	}
	c.LogEvent(event)
}

// LogDeliveryEvent logs a delivery event
func (c *Client) LogDeliveryEvent(msgID, recipient, mxHostname, mxIP, result string, smtpCode int, smtpMessage string, duration time.Duration, attempt int) {
	event := &Event{
		EventID:          fmt.Sprintf("%s-%s-dlv", msgID, recipient),
		EventTime:        time.Now(),
		MsgID:            msgID,
		Recipient:        recipient,
		MXHostname:       mxHostname,
		MXIP:             mxIP,
		SMTPCode:         smtpCode,
		SMTPMessage:      smtpMessage,
		DeliveryDuration: int(duration.Milliseconds()),
		DeliveryResult:   result,
		DeliveryAttempt:  attempt,
		Stage:            "delivery",
	}
	c.LogEvent(event)
}
