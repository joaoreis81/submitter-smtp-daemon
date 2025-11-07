package parser

import (
	"bytes"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"time"

	"github.com/joaoreis81/submitter-smtp-daemon/internal/logger"
)

// MessageMetadata contains extracted message metadata
type MessageMetadata struct {
	MessageID   string
	From        string
	To          []string
	Cc          []string
	Bcc         []string
	Subject     string
	Date        time.Time
	ContentType string
	Size        int64
	Headers     map[string][]string
}

// Parser handles message parsing
type Parser struct {
	logger *logger.Logger
}

// New creates a new message parser
func New(log *logger.Logger) *Parser {
	return &Parser{
		logger: log.WithComponent("parser"),
	}
}

// Parse parses a raw email message and extracts metadata
func (p *Parser) Parse(data []byte) (*MessageMetadata, error) {
	// Parse message
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to parse message: %w", err)
	}

	metadata := &MessageMetadata{
		Headers: make(map[string][]string),
		Size:    int64(len(data)),
	}

	// Extract headers
	for key, values := range msg.Header {
		metadata.Headers[key] = values
	}

	// Extract common headers
	metadata.MessageID = msg.Header.Get("Message-ID")
	metadata.From = msg.Header.Get("From")
	metadata.Subject = msg.Header.Get("Subject")
	metadata.ContentType = msg.Header.Get("Content-Type")

	// Parse To addresses
	if to := msg.Header.Get("To"); to != "" {
		metadata.To = parseAddressList(to)
	}

	// Parse Cc addresses
	if cc := msg.Header.Get("Cc"); cc != "" {
		metadata.Cc = parseAddressList(cc)
	}

	// Parse Bcc addresses
	if bcc := msg.Header.Get("Bcc"); bcc != "" {
		metadata.Bcc = parseAddressList(bcc)
	}

	// Parse Date
	if dateStr := msg.Header.Get("Date"); dateStr != "" {
		if date, err := mail.ParseDate(dateStr); err == nil {
			metadata.Date = date
		}
	}

	// If no date, use current time
	if metadata.Date.IsZero() {
		metadata.Date = time.Now()
	}

	p.logger.WithFields(map[string]interface{}{
		"message_id": metadata.MessageID,
		"from":       metadata.From,
		"to_count":   len(metadata.To),
		"size":       metadata.Size,
	}).Debug("Message parsed successfully")

	return metadata, nil
}

// ParseBody reads and returns the message body
func (p *Parser) ParseBody(data []byte) (string, error) {
	msg, err := mail.ReadMessage(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to parse message: %w", err)
	}

	body, err := io.ReadAll(msg.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read body: %w", err)
	}

	return string(body), nil
}

// parseAddressList parses a comma-separated list of email addresses
func parseAddressList(addrList string) []string {
	addresses := []string{}

	// Parse using mail.ParseAddressList
	addrs, err := mail.ParseAddressList(addrList)
	if err != nil {
		// Fallback: simple split if parsing fails
		parts := strings.Split(addrList, ",")
		for _, part := range parts {
			addr := strings.TrimSpace(part)
			if addr != "" {
				addresses = append(addresses, addr)
			}
		}
		return addresses
	}

	// Extract addresses
	for _, addr := range addrs {
		addresses = append(addresses, addr.Address)
	}

	return addresses
}

// ExtractRecipients extracts all recipients from To, Cc, and Bcc headers
func (m *MessageMetadata) ExtractRecipients() []string {
	recipients := make([]string, 0)
	recipients = append(recipients, m.To...)
	recipients = append(recipients, m.Cc...)
	recipients = append(recipients, m.Bcc...)
	return recipients
}

// GetHeader returns a header value (first occurrence)
func (m *MessageMetadata) GetHeader(key string) string {
	if values, ok := m.Headers[key]; ok && len(values) > 0 {
		return values[0]
	}
	return ""
}

// GetHeaders returns all values for a header
func (m *MessageMetadata) GetHeaders(key string) []string {
	if values, ok := m.Headers[key]; ok {
		return values
	}
	return []string{}
}
