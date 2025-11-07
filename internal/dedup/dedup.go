package dedup

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/logger"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/metrics"
)

// Deduplicator handles message deduplication
type Deduplicator struct {
	redis   *redis.Client
	metrics *metrics.Metrics
	logger  *logger.Logger
	window  time.Duration
	enabled bool
}

// Config contains deduplication configuration
type Config struct {
	Redis   *redis.Client
	Metrics *metrics.Metrics
	Logger  *logger.Logger
	Window  time.Duration
	Enabled bool
}

// New creates a new deduplicator
func New(cfg Config) *Deduplicator {
	return &Deduplicator{
		redis:   cfg.Redis,
		metrics: cfg.Metrics,
		logger:  cfg.Logger.WithComponent("dedup"),
		window:  cfg.Window,
		enabled: cfg.Enabled,
	}
}

// Check checks if a message is a duplicate based on its content hash
// Returns the original message ID if duplicate, empty string if unique
func (d *Deduplicator) Check(msgID string, body []byte) (originalMsgID string, isDuplicate bool, err error) {
	if !d.enabled {
		return "", false, nil
	}

	// Generate hash of message body
	hash := d.hashMessage(body)

	ctx := context.Background()
	key := fmt.Sprintf("dedup:%s", hash)

	// Check if hash exists in Redis
	originalMsgID, err = d.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		// Not a duplicate - store this message ID with the hash
		err = d.redis.Set(ctx, key, msgID, d.window).Err()
		if err != nil {
			d.logger.WithError(err).Warn("Failed to store dedup hash in Redis")
			// Don't fail the message, just log the error
			return "", false, nil
		}

		d.metrics.RecordDedupCheck(false)
		d.logger.WithMessageID(msgID).WithFields(map[string]interface{}{
			"hash": hash,
		}).Debug("Message is unique")

		return "", false, nil
	} else if err != nil {
		// Redis error - don't block the message
		d.logger.WithError(err).Warn("Redis error during dedup check")
		return "", false, nil
	}

	// It's a duplicate
	d.metrics.RecordDedupCheck(true)
	d.logger.WithMessageID(msgID).WithFields(map[string]interface{}{
		"hash":         hash,
		"original_msg": originalMsgID,
	}).Info("Duplicate message detected")

	return originalMsgID, true, nil
}

// hashMessage generates a SHA256 hash of the message body
// Note: Using SHA256 instead of BLAKE3 for now as it's in stdlib
// Can be replaced with BLAKE3 later if needed
func (d *Deduplicator) hashMessage(body []byte) string {
	hash := sha256.Sum256(body)
	return hex.EncodeToString(hash[:])
}

// Delete removes a deduplication entry
func (d *Deduplicator) Delete(hash string) error {
	if !d.enabled {
		return nil
	}

	ctx := context.Background()
	key := fmt.Sprintf("dedup:%s", hash)

	err := d.redis.Del(ctx, key).Err()
	if err != nil && err != redis.Nil {
		return fmt.Errorf("failed to delete dedup entry: %w", err)
	}

	return nil
}

// Stats returns deduplication statistics
func (d *Deduplicator) Stats() (total int64, err error) {
	if !d.enabled {
		return 0, nil
	}

	ctx := context.Background()
	keys, err := d.redis.Keys(ctx, "dedup:*").Result()
	if err != nil {
		return 0, fmt.Errorf("failed to get dedup stats: %w", err)
	}

	return int64(len(keys)), nil
}
