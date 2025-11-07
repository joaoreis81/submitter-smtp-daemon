package ratelimit

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/logger"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/metrics"
)

// Limiter manages rate limiting for IPs and users
type Limiter struct {
	redis   *redis.Client
	metrics *metrics.Metrics
	logger  *logger.Logger

	// Configuration
	userMessagesPerHour    int
	userRecipientsPerHour  int
	ipConnectionsPerMinute int
	ipFailedAuthPerHour    int
	globalMessagesPerSecond int
	globalConcurrentConns  int

	// In-memory fallback (if Redis unavailable)
	localBuckets map[string]*bucket
	mu           sync.RWMutex

	// Global connection counter
	currentConnections int
	connMu             sync.Mutex
}

// bucket represents a token bucket for local rate limiting
type bucket struct {
	count     int
	resetTime time.Time
}

// Config contains rate limiter configuration
type Config struct {
	Redis                   *redis.Client
	Metrics                 *metrics.Metrics
	Logger                  *logger.Logger
	UserMessagesPerHour     int
	UserRecipientsPerHour   int
	IPConnectionsPerMinute  int
	IPFailedAuthPerHour     int
	GlobalMessagesPerSecond int
	GlobalConcurrentConns   int
}

// New creates a new rate limiter
func New(cfg Config) *Limiter {
	return &Limiter{
		redis:                   cfg.Redis,
		metrics:                 cfg.Metrics,
		logger:                  cfg.Logger.WithComponent("ratelimit"),
		userMessagesPerHour:     cfg.UserMessagesPerHour,
		userRecipientsPerHour:   cfg.UserRecipientsPerHour,
		ipConnectionsPerMinute:  cfg.IPConnectionsPerMinute,
		ipFailedAuthPerHour:     cfg.IPFailedAuthPerHour,
		globalMessagesPerSecond: cfg.GlobalMessagesPerSecond,
		globalConcurrentConns:   cfg.GlobalConcurrentConns,
		localBuckets:            make(map[string]*bucket),
	}
}

// CheckIPConnection checks if an IP can make a new connection
func (l *Limiter) CheckIPConnection(ip string) error {
	// Check global concurrent connections
	l.connMu.Lock()
	if l.currentConnections >= l.globalConcurrentConns {
		l.connMu.Unlock()
		l.metrics.RecordRateLimitHit("global", "concurrent_connections")
		return fmt.Errorf("global connection limit reached")
	}
	l.connMu.Unlock()

	// Check per-IP connection rate
	key := fmt.Sprintf("ratelimit:ip:conn:%s", ip)
	window := time.Minute

	allowed, err := l.checkRedisLimit(key, l.ipConnectionsPerMinute, window)
	if err != nil {
		// Fall back to local rate limiting
		l.logger.WithError(err).Warn("Redis unavailable, using local rate limiting")
		allowed = l.checkLocalLimit(key, l.ipConnectionsPerMinute, window)
	}

	if !allowed {
		l.metrics.RecordRateLimitHit("ip", ip)
		return fmt.Errorf("connection rate limit exceeded for IP: %s", ip)
	}

	return nil
}

// IncrementConnection increments the global connection counter
func (l *Limiter) IncrementConnection() {
	l.connMu.Lock()
	l.currentConnections++
	l.connMu.Unlock()
}

// DecrementConnection decrements the global connection counter
func (l *Limiter) DecrementConnection() {
	l.connMu.Lock()
	l.currentConnections--
	l.connMu.Unlock()
}

// CheckIPFailedAuth checks if an IP has exceeded failed auth attempts
func (l *Limiter) CheckIPFailedAuth(ip string) error {
	key := fmt.Sprintf("ratelimit:ip:auth:%s", ip)
	window := time.Hour

	allowed, err := l.checkRedisLimit(key, l.ipFailedAuthPerHour, window)
	if err != nil {
		allowed = l.checkLocalLimit(key, l.ipFailedAuthPerHour, window)
	}

	if !allowed {
		l.metrics.RecordRateLimitHit("ip_auth", ip)
		return fmt.Errorf("failed auth rate limit exceeded for IP: %s", ip)
	}

	return nil
}

// CheckUserMessages checks if a user can send more messages
func (l *Limiter) CheckUserMessages(user string) error {
	key := fmt.Sprintf("ratelimit:user:msg:%s", user)
	window := time.Hour

	allowed, err := l.checkRedisLimit(key, l.userMessagesPerHour, window)
	if err != nil {
		allowed = l.checkLocalLimit(key, l.userMessagesPerHour, window)
	}

	if !allowed {
		l.metrics.RecordRateLimitHit("user_messages", user)
		return fmt.Errorf("message rate limit exceeded for user: %s", user)
	}

	return nil
}

// CheckUserRecipients checks if a user can send to more recipients
func (l *Limiter) CheckUserRecipients(user string, count int) error {
	key := fmt.Sprintf("ratelimit:user:rcpt:%s", user)
	window := time.Hour

	// Get current count
	ctx := context.Background()
	currentStr, err := l.redis.Get(ctx, key).Result()
	var current int
	if err == nil {
		fmt.Sscanf(currentStr, "%d", &current)
	}

	if current+count > l.userRecipientsPerHour {
		l.metrics.RecordRateLimitHit("user_recipients", user)
		return fmt.Errorf("recipient rate limit exceeded for user: %s", user)
	}

	// Increment by count
	pipe := l.redis.Pipeline()
	pipe.IncrBy(ctx, key, int64(count))
	pipe.Expire(ctx, key, window)
	_, err = pipe.Exec(ctx)
	if err != nil {
		l.logger.WithError(err).Warn("Failed to update recipient rate limit in Redis")
	}

	return nil
}

// CheckGlobalMessages checks if the global message rate is exceeded
func (l *Limiter) CheckGlobalMessages() error {
	key := "ratelimit:global:msg"
	window := time.Second

	allowed, err := l.checkRedisLimit(key, l.globalMessagesPerSecond, window)
	if err != nil {
		allowed = l.checkLocalLimit(key, l.globalMessagesPerSecond, window)
	}

	if !allowed {
		l.metrics.RecordRateLimitHit("global", "messages")
		return fmt.Errorf("global message rate limit exceeded")
	}

	return nil
}

// checkRedisLimit checks a rate limit using Redis
func (l *Limiter) checkRedisLimit(key string, limit int, window time.Duration) (bool, error) {
	ctx := context.Background()

	// Use Redis INCR with expiration
	pipe := l.redis.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)

	_, err := pipe.Exec(ctx)
	if err != nil {
		return false, err
	}

	count, err := incr.Result()
	if err != nil {
		return false, err
	}

	return count <= int64(limit), nil
}

// checkLocalLimit checks a rate limit using local memory (fallback)
func (l *Limiter) checkLocalLimit(key string, limit int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()

	// Get or create bucket
	b, exists := l.localBuckets[key]
	if !exists || now.After(b.resetTime) {
		b = &bucket{
			count:     0,
			resetTime: now.Add(window),
		}
		l.localBuckets[key] = b
	}

	// Increment and check
	b.count++
	return b.count <= limit
}

// StartCleanupWorker starts a background worker to clean up old local buckets
func (l *Limiter) StartCleanupWorker(stop <-chan struct{}) {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.cleanupLocalBuckets()
		case <-stop:
			return
		}
	}
}

// cleanupLocalBuckets removes expired buckets from local memory
func (l *Limiter) cleanupLocalBuckets() {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	deleted := 0

	for key, bucket := range l.localBuckets {
		if now.After(bucket.resetTime.Add(5 * time.Minute)) {
			delete(l.localBuckets, key)
			deleted++
		}
	}

	if deleted > 0 {
		l.logger.Debugf("Cleaned up %d expired rate limit buckets", deleted)
	}
}

// GetCurrentConnections returns the current number of connections
func (l *Limiter) GetCurrentConnections() int {
	l.connMu.Lock()
	defer l.connMu.Unlock()
	return l.currentConnections
}
