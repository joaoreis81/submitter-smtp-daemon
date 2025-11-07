package ratelimit

import (
	"sync"
	"time"
)

// Limiter implements a simple token bucket rate limiter
type Limiter struct {
	perIPLimit   int
	perUserLimit int
	window       time.Duration
	ipBuckets    map[string]*bucket
	userBuckets  map[string]*bucket
	mu           sync.RWMutex
	cleanupTicker *time.Ticker
	stopCh       chan struct{}
}

type bucket struct {
	count      int
	lastReset  time.Time
}

// New creates a new rate limiter
func New(perIPLimit, perUserLimit int, window time.Duration) *Limiter {
	l := &Limiter{
		perIPLimit:   perIPLimit,
		perUserLimit: perUserLimit,
		window:       window,
		ipBuckets:    make(map[string]*bucket),
		userBuckets:  make(map[string]*bucket),
		stopCh:       make(chan struct{}),
	}

	// Only create ticker and cleanup goroutine if rate limiting is enabled
	if window > 0 {
		l.cleanupTicker = time.NewTicker(window)
		// Start cleanup goroutine
		go l.cleanup()
	}

	return l
}

// CheckIP checks if the IP is within rate limits
func (l *Limiter) CheckIP(ip string) bool {
	if l.perIPLimit == 0 {
		return true // Rate limiting disabled
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	b, exists := l.ipBuckets[ip]
	if !exists {
		l.ipBuckets[ip] = &bucket{count: 1, lastReset: time.Now()}
		return true
	}

	// Reset bucket if window has passed
	if time.Since(b.lastReset) > l.window {
		b.count = 1
		b.lastReset = time.Now()
		return true
	}

	if b.count >= l.perIPLimit {
		return false
	}

	b.count++
	return true
}

// CheckUser checks if the user is within rate limits
func (l *Limiter) CheckUser(username string) bool {
	if l.perUserLimit == 0 {
		return true // Rate limiting disabled
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	b, exists := l.userBuckets[username]
	if !exists {
		l.userBuckets[username] = &bucket{count: 1, lastReset: time.Now()}
		return true
	}

	// Reset bucket if window has passed
	if time.Since(b.lastReset) > l.window {
		b.count = 1
		b.lastReset = time.Now()
		return true
	}

	if b.count >= l.perUserLimit {
		return false
	}

	b.count++
	return true
}

// cleanup removes old buckets periodically
func (l *Limiter) cleanup() {
	// This should only be called if cleanupTicker is not nil
	for {
		select {
		case <-l.cleanupTicker.C:
			l.mu.Lock()
			now := time.Now()

			// Clean IP buckets
			for ip, b := range l.ipBuckets {
				if now.Sub(b.lastReset) > l.window*2 {
					delete(l.ipBuckets, ip)
				}
			}

			// Clean user buckets
			for user, b := range l.userBuckets {
				if now.Sub(b.lastReset) > l.window*2 {
					delete(l.userBuckets, user)
				}
			}

			l.mu.Unlock()

		case <-l.stopCh:
			l.cleanupTicker.Stop()
			return
		}
	}
}

// Close stops the rate limiter
func (l *Limiter) Close() {
	// Only close if ticker exists (rate limiting was enabled)
	if l.cleanupTicker != nil {
		close(l.stopCh)
	}
}

// ResetIP resets the rate limit for an IP
func (l *Limiter) ResetIP(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.ipBuckets, ip)
}

// ResetUser resets the rate limit for a user
func (l *Limiter) ResetUser(username string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.userBuckets, username)
}
