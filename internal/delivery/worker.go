package delivery

import (
	"context"
	"fmt"
	"net"
	"net/smtp"
	"strings"
	"time"

	"github.com/joaoreis81/submitter-smtp-daemon/internal/clickhouse"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/logger"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/metrics"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/queue"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/spool"
)

// Worker handles message delivery
type Worker struct {
	queue      *queue.Queue
	spool      *spool.Spool
	clickhouse *clickhouse.Client
	metrics    *metrics.Metrics
	logger     *logger.Logger
	config     WorkerConfig
	stopChan   chan struct{}
}

// WorkerConfig contains delivery worker configuration
type WorkerConfig struct {
	Workers       int
	RetrySchedule []time.Duration
	MaxAttempts   int
	PollInterval  time.Duration
}

// New creates a new delivery worker
func New(q *queue.Queue, sp *spool.Spool, ch *clickhouse.Client, met *metrics.Metrics, log *logger.Logger, cfg WorkerConfig) *Worker {
	return &Worker{
		queue:      q,
		spool:      sp,
		clickhouse: ch,
		metrics:    met,
		logger:     log.WithComponent("delivery"),
		config:     cfg,
		stopChan:   make(chan struct{}),
	}
}

// Start starts the delivery worker pool
func (w *Worker) Start() {
	w.logger.Infof("Starting %d delivery workers", w.config.Workers)

	for i := 0; i < w.config.Workers; i++ {
		go w.workerLoop(i)
	}
}

// Stop stops all delivery workers
func (w *Worker) Stop() {
	w.logger.Info("Stopping delivery workers")
	close(w.stopChan)
}

// workerLoop is the main loop for a delivery worker
func (w *Worker) workerLoop(workerID int) {
	w.logger.Infof("Delivery worker %d started", workerID)

	ticker := time.NewTicker(w.config.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			w.processBatch(workerID)
		case <-w.stopChan:
			w.logger.Infof("Delivery worker %d stopped", workerID)
			return
		}
	}
}

// processBatch processes a batch of pending deliveries
func (w *Worker) processBatch(workerID int) {
	// Get pending deliveries
	items, err := w.queue.GetPendingDeliveries(10) // Process 10 at a time
	if err != nil {
		w.logger.WithError(err).Error("Failed to get pending deliveries")
		return
	}

	if len(items) == 0 {
		return
	}

	w.logger.Debugf("Worker %d processing %d deliveries", workerID, len(items))

	for _, item := range items {
		w.processDelivery(item)
	}
}

// processDelivery processes a single delivery
func (w *Worker) processDelivery(item *queue.QueueItem) {
	start := time.Now()
	log := w.logger.WithMessageID(item.MsgID).WithRecipient(item.Recipient)

	log.Info("Processing delivery")

	// Read message from spool
	msgData, err := w.spool.Read(spool.StageSigned, item.MsgID)
	if err != nil {
		// Try other stages if not in signed
		msgData, err = w.spool.Read(spool.StageModified, item.MsgID)
		if err != nil {
			msgData, err = w.spool.Read(spool.StageParsed, item.MsgID)
			if err != nil {
				log.WithError(err).Error("Failed to read message from spool")
				w.updateDeliveryFailed(item, "spool_read_error", err.Error(), 0, "")
				return
			}
		}
	}

	// Lookup MX records
	mxHost, mxIP, err := w.lookupMX(item.Recipient)
	if err != nil {
		log.WithError(err).Warn("MX lookup failed")
		w.updateDeliveryTempFail(item, mxHost, mxIP, err.Error(), 0, "")
		return
	}

	log.Infof("Delivering to MX: %s (%s)", mxHost, mxIP)

	// Attempt delivery
	smtpCode, smtpMessage, err := w.deliver(mxHost, item.Recipient, msgData)
	duration := time.Since(start)

	if err != nil {
		// Determine if temp or perm failure
		if smtpCode >= 500 && smtpCode < 600 {
			// Permanent failure
			log.WithError(err).Warnf("Permanent delivery failure: %d %s", smtpCode, smtpMessage)
			w.updateDeliveryFailed(item, mxHost, mxIP, err.Error(), smtpCode, smtpMessage)
			w.metrics.RecordDelivery("perm_fail", mxHost, duration.Seconds())
			w.clickhouse.LogDeliveryEvent(item.MsgID, item.Recipient, mxHost, mxIP, "perm_fail", smtpCode, smtpMessage, duration, item.Attempts+1)
		} else {
			// Temporary failure or network error
			log.WithError(err).Warnf("Temporary delivery failure: %d %s", smtpCode, smtpMessage)
			w.updateDeliveryTempFail(item, mxHost, mxIP, err.Error(), smtpCode, smtpMessage)
			w.metrics.RecordDelivery("temp_fail", mxHost, duration.Seconds())
			w.clickhouse.LogDeliveryEvent(item.MsgID, item.Recipient, mxHost, mxIP, "temp_fail", smtpCode, smtpMessage, duration, item.Attempts+1)
		}
		return
	}

	// Success!
	log.Info("Delivery successful")
	w.updateDeliverySuccess(item, mxHost, mxIP, smtpCode, smtpMessage)
	w.metrics.RecordDelivery("success", mxHost, duration.Seconds())
	w.clickhouse.LogDeliveryEvent(item.MsgID, item.Recipient, mxHost, mxIP, "success", smtpCode, smtpMessage, duration, item.Attempts+1)
}

// lookupMX performs MX record lookup for a recipient
func (w *Worker) lookupMX(recipient string) (mxHost, mxIP string, err error) {
	// Extract domain from recipient
	parts := strings.Split(recipient, "@")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid recipient address: %s", recipient)
	}
	domain := parts[1]

	// Lookup MX records
	mxRecords, err := net.LookupMX(domain)
	if err != nil {
		return "", "", fmt.Errorf("MX lookup failed: %w", err)
	}

	if len(mxRecords) == 0 {
		return "", "", fmt.Errorf("no MX records found for %s", domain)
	}

	// Use the highest priority MX (lowest number)
	mxHost = strings.TrimSuffix(mxRecords[0].Host, ".")

	// Resolve MX hostname to IP
	ips, err := net.LookupIP(mxHost)
	if err != nil || len(ips) == 0 {
		return mxHost, "", fmt.Errorf("failed to resolve MX host %s: %w", mxHost, err)
	}

	mxIP = ips[0].String()
	return mxHost, mxIP, nil
}

// deliver attempts to deliver a message via SMTP
func (w *Worker) deliver(mxHost, recipient string, msgData []byte) (smtpCode int, smtpMessage string, err error) {
	// Connect to MX server
	addr := fmt.Sprintf("%s:25", mxHost)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Simple SMTP delivery
	client, err := smtp.Dial(addr)
	if err != nil {
		return 0, "", fmt.Errorf("failed to connect to %s: %w", addr, err)
	}
	defer client.Close()

	// Say HELO
	if err := client.Hello("submitter-smtp-daemon"); err != nil {
		return 0, "", fmt.Errorf("HELO failed: %w", err)
	}

	// Try STARTTLS if supported
	if ok, _ := client.Extension("STARTTLS"); ok {
		// In production, you would configure TLS properly
		// For now, skip TLS or use a basic config
	}

	// Extract sender from message (simplified - should parse properly)
	sender := "postmaster@localhost" // Default sender

	// MAIL FROM
	if err := client.Mail(sender); err != nil {
		return 0, "", fmt.Errorf("MAIL FROM failed: %w", err)
	}

	// RCPT TO
	if err := client.Rcpt(recipient); err != nil {
		return 0, "", fmt.Errorf("RCPT TO failed: %w", err)
	}

	// DATA
	wc, err := client.Data()
	if err != nil {
		return 0, "", fmt.Errorf("DATA command failed: %w", err)
	}

	_, err = wc.Write(msgData)
	if err != nil {
		wc.Close()
		return 0, "", fmt.Errorf("failed to write message data: %w", err)
	}

	err = wc.Close()
	if err != nil {
		return 0, "", fmt.Errorf("failed to close data writer: %w", err)
	}

	// QUIT
	client.Quit()

	// Successful delivery
	return 250, "OK", nil
}

// updateDeliverySuccess marks a delivery as successful
func (w *Worker) updateDeliverySuccess(item *queue.QueueItem, mxHost, mxIP string, smtpCode int, smtpMessage string) {
	err := w.queue.UpdateDeliveryState(
		item.DlvID,
		queue.StateDelivered,
		mxHost,
		mxIP,
		"",
		smtpCode,
		smtpMessage,
		time.Time{}, // No next attempt
	)

	if err != nil {
		w.logger.WithError(err).Error("Failed to update delivery state")
	}
}

// updateDeliveryFailed marks a delivery as permanently failed
func (w *Worker) updateDeliveryFailed(item *queue.QueueItem, mxHost, mxIP, errorMsg string, smtpCode int, smtpMessage string) {
	err := w.queue.UpdateDeliveryState(
		item.DlvID,
		queue.StatePermFail,
		mxHost,
		mxIP,
		errorMsg,
		smtpCode,
		smtpMessage,
		time.Time{}, // No next attempt
	)

	if err != nil {
		w.logger.WithError(err).Error("Failed to update delivery state")
	}
}

// updateDeliveryTempFail marks a delivery as temporarily failed with retry
func (w *Worker) updateDeliveryTempFail(item *queue.QueueItem, mxHost, mxIP, errorMsg string, smtpCode int, smtpMessage string) {
	// Calculate next retry time
	nextAttempt := w.calculateNextRetry(item.Attempts)

	// Check if we've exceeded max attempts
	if item.Attempts+1 >= item.MaxAttempts {
		w.updateDeliveryFailed(item, mxHost, mxIP, errorMsg, smtpCode, smtpMessage)
		return
	}

	err := w.queue.UpdateDeliveryState(
		item.DlvID,
		queue.StateTempFail,
		mxHost,
		mxIP,
		errorMsg,
		smtpCode,
		smtpMessage,
		nextAttempt,
	)

	if err != nil {
		w.logger.WithError(err).Error("Failed to update delivery state")
	}

	w.logger.WithMessageID(item.MsgID).WithRecipient(item.Recipient).Infof("Scheduled retry at %s (attempt %d/%d)", nextAttempt, item.Attempts+1, item.MaxAttempts)
}

// calculateNextRetry calculates the next retry time based on attempt number
func (w *Worker) calculateNextRetry(attempts int) time.Time {
	if attempts < len(w.config.RetrySchedule) {
		return time.Now().Add(w.config.RetrySchedule[attempts])
	}
	// Use last retry interval for any attempts beyond the schedule
	return time.Now().Add(w.config.RetrySchedule[len(w.config.RetrySchedule)-1])
}
