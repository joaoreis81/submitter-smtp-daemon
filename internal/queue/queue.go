package queue

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/db"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/logger"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/parser"
)

// Queue manages the delivery queue
type Queue struct {
	db     *db.DB
	logger *logger.Logger
}

// DeliveryState represents the state of a delivery attempt
type DeliveryState string

const (
	StateQueued    DeliveryState = "queued"
	StateDelivering DeliveryState = "delivering"
	StateDelivered  DeliveryState = "delivered"
	StateTempFail   DeliveryState = "temp_fail"
	StatePermFail   DeliveryState = "perm_fail"
)

// QueueItem represents a delivery queue item
type QueueItem struct {
	DlvID        string
	MsgID        string
	Recipient    string
	MXHostname   string
	MXIP         string
	State        DeliveryState
	Attempts     int
	MaxAttempts  int
	NextAttempt  time.Time
	LastError    string
	SMTPCode     int
	SMTPMessage  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeliveredAt  *time.Time
}

// Message represents a message in the database
type Message struct {
	MsgID           string
	SessionID       string
	AuthUser        string
	ClientIP        string
	MailFrom        string
	RcptTo          []string
	MessageSize     int64
	BodyHash        string
	DedupOriginal   string
	CreatedAt       time.Time
	CurrentStage    string
	Subject         string
	MessageIDHeader string
	DateHeader      string
}

// New creates a new queue manager
func New(database *db.DB, log *logger.Logger) *Queue {
	return &Queue{
		db:     database,
		logger: log.WithComponent("queue"),
	}
}

// AddMessage adds a message to the database
func (q *Queue) AddMessage(msg *Message) error {
	rcptJSON, err := json.Marshal(msg.RcptTo)
	if err != nil {
		return fmt.Errorf("failed to marshal recipients: %w", err)
	}

	query := `
		INSERT INTO messages (
			msg_id, session_id, auth_user, client_ip, mail_from, rcpt_to,
			message_size, body_hash, dedup_original, created_at, current_stage,
			subject, message_id_header, date_header
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = q.db.GetConn().Exec(query,
		msg.MsgID,
		msg.SessionID,
		msg.AuthUser,
		msg.ClientIP,
		msg.MailFrom,
		string(rcptJSON),
		msg.MessageSize,
		msg.BodyHash,
		msg.DedupOriginal,
		msg.CreatedAt.Unix(),
		msg.CurrentStage,
		msg.Subject,
		msg.MessageIDHeader,
		msg.DateHeader,
	)

	if err != nil {
		return fmt.Errorf("failed to insert message: %w", err)
	}

	q.logger.WithMessageID(msg.MsgID).Debug("Message added to database")
	return nil
}

// EnqueueDelivery adds a delivery task to the queue
func (q *Queue) EnqueueDelivery(msgID, recipient string, maxAttempts int) error {
	dlvID := uuid.New().String()
	now := time.Now()

	query := `
		INSERT INTO delivery_queue (
			dlv_id, msg_id, recipient, state, attempts, max_attempts,
			next_attempt, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := q.db.GetConn().Exec(query,
		dlvID,
		msgID,
		recipient,
		StateQueued,
		0,
		maxAttempts,
		now.Unix(),
		now.Unix(),
		now.Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to enqueue delivery: %w", err)
	}

	q.logger.WithMessageID(msgID).WithRecipient(recipient).Debug("Delivery enqueued")
	return nil
}

// GetPendingDeliveries retrieves deliveries ready to be attempted
func (q *Queue) GetPendingDeliveries(limit int) ([]*QueueItem, error) {
	query := `
		SELECT dlv_id, msg_id, recipient, mx_hostname, mx_ip, state, attempts,
		       max_attempts, next_attempt, last_error, smtp_code, smtp_message,
		       created_at, updated_at, delivered_at
		FROM delivery_queue
		WHERE state IN ('queued', 'temp_fail') AND next_attempt <= ?
		ORDER BY next_attempt ASC
		LIMIT ?
	`

	rows, err := q.db.GetConn().Query(query, time.Now().Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending deliveries: %w", err)
	}
	defer rows.Close()

	items := make([]*QueueItem, 0, limit)
	for rows.Next() {
		item := &QueueItem{}
		var nextAttempt, createdAt, updatedAt int64
		var deliveredAt sql.NullInt64
		var mxHostname, mxIP, lastError, smtpMessage sql.NullString
		var smtpCode sql.NullInt64

		err := rows.Scan(
			&item.DlvID,
			&item.MsgID,
			&item.Recipient,
			&mxHostname,
			&mxIP,
			&item.State,
			&item.Attempts,
			&item.MaxAttempts,
			&nextAttempt,
			&lastError,
			&smtpCode,
			&smtpMessage,
			&createdAt,
			&updatedAt,
			&deliveredAt,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan delivery item: %w", err)
		}

		item.MXHostname = mxHostname.String
		item.MXIP = mxIP.String
		item.LastError = lastError.String
		item.SMTPMessage = smtpMessage.String
		if smtpCode.Valid {
			item.SMTPCode = int(smtpCode.Int64)
		}
		item.NextAttempt = time.Unix(nextAttempt, 0)
		item.CreatedAt = time.Unix(createdAt, 0)
		item.UpdatedAt = time.Unix(updatedAt, 0)
		if deliveredAt.Valid {
			t := time.Unix(deliveredAt.Int64, 0)
			item.DeliveredAt = &t
		}

		items = append(items, item)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating delivery items: %w", err)
	}

	return items, nil
}

// UpdateDeliveryState updates the state of a delivery
func (q *Queue) UpdateDeliveryState(dlvID string, state DeliveryState, mxHostname, mxIP, errorMsg string, smtpCode int, smtpMessage string, nextAttempt time.Time) error {
	var deliveredAt sql.NullInt64
	if state == StateDelivered {
		deliveredAt = sql.NullInt64{Int64: time.Now().Unix(), Valid: true}
	}

	query := `
		UPDATE delivery_queue
		SET state = ?, mx_hostname = ?, mx_ip = ?, last_error = ?, smtp_code = ?,
		    smtp_message = ?, next_attempt = ?, updated_at = ?, delivered_at = ?,
		    attempts = attempts + 1
		WHERE dlv_id = ?
	`

	_, err := q.db.GetConn().Exec(query,
		state,
		mxHostname,
		mxIP,
		errorMsg,
		smtpCode,
		smtpMessage,
		nextAttempt.Unix(),
		time.Now().Unix(),
		deliveredAt,
		dlvID,
	)

	if err != nil {
		return fmt.Errorf("failed to update delivery state: %w", err)
	}

	return nil
}

// GetMessage retrieves a message by ID
func (q *Queue) GetMessage(msgID string) (*Message, error) {
	query := `
		SELECT msg_id, session_id, auth_user, client_ip, mail_from, rcpt_to,
		       message_size, body_hash, dedup_original, created_at, current_stage,
		       subject, message_id_header, date_header
		FROM messages
		WHERE msg_id = ?
	`

	row := q.db.GetConn().QueryRow(query, msgID)

	msg := &Message{}
	var rcptJSON string
	var createdAt int64
	var dedupOriginal sql.NullString

	err := row.Scan(
		&msg.MsgID,
		&msg.SessionID,
		&msg.AuthUser,
		&msg.ClientIP,
		&msg.MailFrom,
		&rcptJSON,
		&msg.MessageSize,
		&msg.BodyHash,
		&dedupOriginal,
		&createdAt,
		&msg.CurrentStage,
		&msg.Subject,
		&msg.MessageIDHeader,
		&msg.DateHeader,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("message not found: %s", msgID)
	} else if err != nil {
		return nil, fmt.Errorf("failed to get message: %w", err)
	}

	// Unmarshal recipients
	if err := json.Unmarshal([]byte(rcptJSON), &msg.RcptTo); err != nil {
		return nil, fmt.Errorf("failed to unmarshal recipients: %w", err)
	}

	msg.DedupOriginal = dedupOriginal.String
	msg.CreatedAt = time.Unix(createdAt, 0)

	return msg, nil
}

// GetQueueStats returns queue statistics
func (q *Queue) GetQueueStats() (map[string]int, error) {
	query := `
		SELECT state, COUNT(*) as count
		FROM delivery_queue
		GROUP BY state
	`

	rows, err := q.db.GetConn().Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get queue stats: %w", err)
	}
	defer rows.Close()

	stats := make(map[string]int)
	for rows.Next() {
		var state string
		var count int
		if err := rows.Scan(&state, &count); err != nil {
			return nil, fmt.Errorf("failed to scan stats: %w", err)
		}
		stats[state] = count
	}

	return stats, nil
}

// UpdateMessageStage updates the current stage of a message
func (q *Queue) UpdateMessageStage(msgID, stage string) error {
	query := `UPDATE messages SET current_stage = ? WHERE msg_id = ?`
	_, err := q.db.GetConn().Exec(query, stage, msgID)
	if err != nil {
		return fmt.Errorf("failed to update message stage: %w", err)
	}
	return nil
}

// ConvertMetadataToMessage converts parser metadata to a queue Message
func ConvertMetadataToMessage(msgID, sessionID, authUser, clientIP, mailFrom string, rcptTo []string, metadata *parser.MessageMetadata, bodyHash string) *Message {
	return &Message{
		MsgID:           msgID,
		SessionID:       sessionID,
		AuthUser:        authUser,
		ClientIP:        clientIP,
		MailFrom:        mailFrom,
		RcptTo:          rcptTo,
		MessageSize:     metadata.Size,
		BodyHash:        bodyHash,
		CreatedAt:       time.Now(),
		CurrentStage:    "parsed",
		Subject:         metadata.Subject,
		MessageIDHeader: metadata.MessageID,
		DateHeader:      metadata.Date.Format(time.RFC3339),
	}
}
