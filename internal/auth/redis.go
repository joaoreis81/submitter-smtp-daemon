package auth

import (
	"context"
	"crypto/subtle"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/logger"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/metrics"
)

// RedisAuthenticator authenticates users against Redis database
type RedisAuthenticator struct {
	redis   *redis.Client
	metrics *metrics.Metrics
	logger  *logger.Logger
}

// NewRedisAuthenticator creates a new Redis authenticator
func NewRedisAuthenticator(redis *redis.Client, metrics *metrics.Metrics, logger *logger.Logger) *RedisAuthenticator {
	return &RedisAuthenticator{
		redis:   redis,
		metrics: metrics,
		logger:  logger.WithComponent("auth-redis"),
	}
}

// Authenticate verifies username and password against Redis
func (a *RedisAuthenticator) Authenticate(username, password string) error {
	start := time.Now()
	ctx := context.Background()

	// Get password hash from Redis
	// Key format: users:{username}:password
	key := fmt.Sprintf("users:%s:password", username)

	storedPassword, err := a.redis.Get(ctx, key).Result()
	if err == redis.Nil {
		// User not found
		a.metrics.RecordAuthAttempt("redis", "failure", time.Since(start).Seconds())
		a.logger.WithUser(username).Warn("Authentication failed: user not found")
		return fmt.Errorf("invalid credentials")
	} else if err != nil {
		// Redis error
		a.metrics.RecordAuthAttempt("redis", "failure", time.Since(start).Seconds())
		a.logger.WithError(err).WithUser(username).Error("Redis error during authentication")
		return fmt.Errorf("authentication error")
	}

	// Compare passwords using constant-time comparison
	if subtle.ConstantTimeCompare([]byte(password), []byte(storedPassword)) != 1 {
		a.metrics.RecordAuthAttempt("redis", "failure", time.Since(start).Seconds())
		a.logger.WithUser(username).Warn("Authentication failed: invalid password")
		return fmt.Errorf("invalid credentials")
	}

	// Authentication successful
	a.metrics.RecordAuthAttempt("redis", "success", time.Since(start).Seconds())
	a.logger.WithUser(username).Info("Authentication successful")
	return nil
}

// GetUserInfo retrieves additional user information from Redis
func (a *RedisAuthenticator) GetUserInfo(username string) (map[string]string, error) {
	ctx := context.Background()

	// Get all user fields
	// Key format: users:{username}:*
	pattern := fmt.Sprintf("users:%s:*", username)

	keys, err := a.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return nil, fmt.Errorf("failed to get user keys: %w", err)
	}

	info := make(map[string]string)
	for _, key := range keys {
		val, err := a.redis.Get(ctx, key).Result()
		if err == nil {
			// Extract field name from key (users:{username}:{field})
			parts := splitKey(key)
			if len(parts) == 3 {
				info[parts[2]] = val
			}
		}
	}

	return info, nil
}

// splitKey splits a Redis key by colons
func splitKey(key string) []string {
	var parts []string
	current := ""
	for _, ch := range key {
		if ch == ':' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// SetPassword sets or updates a user's password in Redis
func (a *RedisAuthenticator) SetPassword(username, password string) error {
	ctx := context.Background()
	key := fmt.Sprintf("users:%s:password", username)

	// In production, you should hash the password (bcrypt, argon2, etc.)
	// For now, storing plaintext for simplicity
	err := a.redis.Set(ctx, key, password, 0).Err()
	if err != nil {
		return fmt.Errorf("failed to set password: %w", err)
	}

	a.logger.WithUser(username).Info("Password updated")
	return nil
}

// DeleteUser removes a user from Redis
func (a *RedisAuthenticator) DeleteUser(username string) error {
	ctx := context.Background()

	// Delete all user keys
	pattern := fmt.Sprintf("users:%s:*", username)
	keys, err := a.redis.Keys(ctx, pattern).Result()
	if err != nil {
		return fmt.Errorf("failed to find user keys: %w", err)
	}

	if len(keys) > 0 {
		err = a.redis.Del(ctx, keys...).Err()
		if err != nil {
			return fmt.Errorf("failed to delete user: %w", err)
		}
	}

	a.logger.WithUser(username).Info("User deleted")
	return nil
}

// UserExists checks if a user exists in Redis
func (a *RedisAuthenticator) UserExists(username string) (bool, error) {
	ctx := context.Background()
	key := fmt.Sprintf("users:%s:password", username)

	exists, err := a.redis.Exists(ctx, key).Result()
	if err != nil {
		return false, fmt.Errorf("failed to check user existence: %w", err)
	}

	return exists > 0, nil
}
