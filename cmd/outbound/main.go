package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/emersion/go-smtp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/redis/go-redis/v9"

	"github.com/joaoreis81/submitter-smtp-daemon/internal/auth"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/config"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/logger"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/metrics"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/ratelimit"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/smtpserver"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/spool"
	"github.com/joaoreis81/submitter-smtp-daemon/internal/tlsmgr"
)

func main() {
	// Parse command-line flags
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log := logger.New(logger.Config{
		Level:  cfg.Observability.LogLevel,
		Format: cfg.Observability.LogFormat,
	})
	log.Infof("Starting SMTP Outbound Daemon v%s", cfg.Server.Version)

	// Initialize metrics
	met := metrics.New()
	log.Info("Metrics initialized")

	// Initialize Redis client
	redisClient := redis.NewClient(&redis.Options{
		Addr:         cfg.Redis.Addresses[0], // Use first address for now
		Password:     cfg.Redis.Password,
		DB:           cfg.Redis.DB,
		PoolSize:     cfg.Redis.PoolSize,
		MaxRetries:   cfg.Redis.MaxRetries,
		DialTimeout:  cfg.Redis.DialTimeout,
		ReadTimeout:  cfg.Redis.ReadTimeout,
		WriteTimeout: cfg.Redis.WriteTimeout,
	})

	// Test Redis connection
	ctx := context.Background()
	if err := redisClient.Ping(ctx).Err(); err != nil {
		log.Warnf("Redis connection failed (will use fallback): %v", err)
	} else {
		log.Info("Redis connected successfully")
	}

	// Initialize TLS manager
	tlsManager, err := tlsmgr.New(cfg.TLS.CertsDir, log)
	if err != nil {
		log.Fatalf("Failed to initialize TLS manager: %v", err)
	}
	log.Infof("TLS manager initialized with %d certificates", tlsManager.CertCount())

	// Start TLS certificate watcher
	stopTLS := make(chan struct{})
	if err := tlsManager.StartWatcher(stopTLS); err != nil {
		log.Warnf("Failed to start TLS watcher: %v", err)
	}

	// Initialize spool
	spoolCfg := spool.Config{
		BasePath: cfg.Spool.BasePath,
		Retention: map[spool.Stage]time.Duration{
			spool.StageIncoming:   cfg.Spool.Retention.Incoming,
			spool.StageParsed:     cfg.Spool.Retention.Parsed,
			spool.StageFiltered:   cfg.Spool.Retention.Filtered,
			spool.StageModified:   cfg.Spool.Retention.Modified,
			spool.StageSigned:     cfg.Spool.Retention.Signed,
			spool.StageDelivering: cfg.Spool.Retention.Delivering,
			spool.StageDone:       cfg.Spool.Retention.Done,
			spool.StageFailed:     cfg.Spool.Retention.Failed,
		},
	}
	sp, err := spool.New(spoolCfg, log)
	if err != nil {
		log.Fatalf("Failed to initialize spool: %v", err)
	}
	log.Info("Spool initialized")

	// Start spool cleanup worker
	stopSpool := make(chan struct{})
	go sp.StartCleanupWorker(1*time.Hour, stopSpool)

	// Initialize rate limiter
	rateLimiter := ratelimit.New(ratelimit.Config{
		Redis:                   redisClient,
		Metrics:                 met,
		Logger:                  log,
		UserMessagesPerHour:     cfg.RateLimit.User.MessagesPerHour,
		UserRecipientsPerHour:   cfg.RateLimit.User.RecipientsPerHour,
		IPConnectionsPerMinute:  cfg.RateLimit.IP.ConnectionsPerMinute,
		IPFailedAuthPerHour:     cfg.RateLimit.IP.FailedAuthPerHour,
		GlobalMessagesPerSecond: cfg.RateLimit.Global.MessagesPerSecond,
		GlobalConcurrentConns:   cfg.RateLimit.Global.ConcurrentConnections,
	})
	log.Info("Rate limiter initialized")

	// Start rate limiter cleanup worker
	stopRateLimit := make(chan struct{})
	go rateLimiter.StartCleanupWorker(stopRateLimit)

	// Initialize Redis authenticator
	redisAuth := auth.NewRedisAuthenticator(redisClient, met, log)
	log.Info("Redis authenticator initialized")

	// Initialize IP authenticator (if enabled)
	var ipAuth *auth.IPAuthenticator
	if cfg.Auth.IPAllowlist.Enabled {
		ipAuth, err = auth.NewIPAuthenticator(cfg.Auth.IPAllowlist.IPs, log)
		if err != nil {
			log.Fatalf("Failed to initialize IP authenticator: %v", err)
		}
		ips, cidrs := ipAuth.GetAllowedCount()
		log.Infof("IP authenticator initialized with %d IPs and %d CIDR ranges", ips, cidrs)
	}

	// Create SMTP backend
	backend := smtpserver.NewBackend(smtpserver.BackendConfig{
		Config:      cfg,
		Spool:       sp,
		RedisAuth:   redisAuth,
		IPAuth:      ipAuth,
		RateLimiter: rateLimiter,
		Metrics:     met,
		Logger:      log,
	})

	// Start health/metrics HTTP server
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		// Check if we have certificates
		if tlsManager.CertCount() == 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("No TLS certificates loaded"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Ready"))
	})

	if cfg.Observability.MetricsEnabled {
		healthMux.Handle("/metrics", promhttp.Handler())
	}

	healthServer := &http.Server{
		Addr:    cfg.Observability.HealthListen,
		Handler: healthMux,
	}

	go func() {
		log.Infof("Starting health/metrics server on %s", cfg.Observability.HealthListen)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Errorf("Health server error: %v", err)
		}
	}()

	// Create and start SMTP servers for each listener
	for _, listener := range cfg.SMTP.Listeners {
		// Create SMTP server
		var tlsConfig *tls.Config
		if listener.Mode != "plaintext" {
			tlsConfig = tlsManager.GetTLSConfig()
		}

		smtpServer := smtpserver.CreateServer(backend, cfg, tlsConfig)

		// Start server in goroutine
		addr := fmt.Sprintf("%s:%d", listener.IP, listener.Port)
		go func(addr string, mode string, srv *smtp.Server) {
			log.Infof("Starting SMTP server on %s (mode: %s)", addr, mode)

			var err error
			switch mode {
			case "starttls":
				err = srv.ListenAndServe(addr)
			case "implicit_tls":
				err = srv.ListenAndServeTLS(addr)
			case "plaintext":
				srv.TLSConfig = nil // Disable TLS
				err = srv.ListenAndServe(addr)
			}

			if err != nil {
				log.Errorf("SMTP server error on %s: %v", addr, err)
			}
		}(addr, listener.Mode, smtpServer)
	}

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	sig := <-sigChan

	log.Infof("Received signal %s, shutting down gracefully...", sig)

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop background workers
	close(stopTLS)
	close(stopSpool)
	close(stopRateLimit)

	// Shutdown health server
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		log.Errorf("Health server shutdown error: %v", err)
	}

	// Close Redis connection
	if err := redisClient.Close(); err != nil {
		log.Errorf("Redis close error: %v", err)
	}

	log.Info("Shutdown complete")
}
