package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/auth"
	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/backend"
	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/config"
	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/metrics"
	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/proxy"
	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/ratelimit"
	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/relay"
	"gitlab.7lan.net/emails/email-daemons/submission-proxy/internal/tlsmgr"

	smtp "github.com/emersion/go-smtp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	configFile = flag.String("config", "", "Path to configuration file")
	version    = "dev"
)

func main() {
	flag.Parse()

	// Load configuration
	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	// Initialize logger
	logger := initLogger(cfg.Observability.LogLevel, cfg.Observability.LogFormat)
	logger.Info("starting smtp-edge-proxy",
		"version", version,
		"config_file", *configFile)

	// Initialize metrics
	m := metrics.New("smtp_proxy")

	// Initialize TLS manager
	tlsMinVersion := tlsmgr.ParseTLSVersion(cfg.TLS.MinVersion)
	tlsMgr, err := tlsmgr.New(cfg.TLS.CertsDir, tlsMinVersion, logger)
	if err != nil {
		log.Fatalf("Failed to initialize TLS manager: %v", err)
	}
	defer tlsMgr.Close()

	logger.Info("loaded certificates",
		"count", tlsMgr.GetCertCount(),
		"domains", tlsMgr.GetDomains())

	// Initialize rate limiter
	var limiter *ratelimit.Limiter
	if cfg.Security.RateLimitEnabled {
		limiter = ratelimit.New(
			cfg.Security.RateLimitPerIP,
			cfg.Security.RateLimitPerUser,
			cfg.Security.RateLimitWindow,
		)
		defer limiter.Close()
	} else {
		limiter = ratelimit.New(0, 0, 0) // Disabled
	}

	// Initialize backend selector
	backendSelector, err := backend.NewSelector(cfg.Backend.BackendsFile, logger)
	if err != nil {
		log.Fatalf("Failed to initialize backend selector: %v", err)
	}

	// Initialize authenticator
	authenticator := auth.New(&cfg.Backend, m, limiter, logger)

	// Initialize relay
	relayHandler := relay.New(&cfg.Relay, m, logger)

	// Initialize SMTP backend
	smtpBackend := proxy.NewBackend(cfg, authenticator, relayHandler, m, logger, backendSelector)

	// Start health/metrics server
	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", healthHandler)
	healthMux.HandleFunc("/readyz", readyHandler(tlsMgr))
	healthMux.Handle("/metrics", promhttp.Handler())

	healthServer := &http.Server{
		Addr:    cfg.Observability.HealthAddr,
		Handler: healthMux,
	}

	go func() {
		logger.Info("health server listening", "addr", cfg.Observability.HealthAddr)
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("health server error", "error", err)
		}
	}()

	// Setup signal handling
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	var wg sync.WaitGroup

	// Get all configured listeners
	listeners := cfg.Listeners.GetAllListeners()

	// Start SMTP servers for each listener
	smtpServers := make([]*smtp.Server, len(listeners))
	wg.Add(len(listeners))

	for i, listenerCfg := range listeners {
		idx := i
		lcfg := listenerCfg
		server := createSMTPServer(cfg, smtpBackend, tlsMgr.GetTLSConfig(), lcfg.ImplicitTLS)
		server.Addr = lcfg.Addr
		smtpServers[idx] = server

		go func() {
			defer wg.Done()

			if lcfg.ImplicitTLS {
				// Implicit TLS (SMTPS) - wrap listener with TLS
				logger.Info("smtps server listening (implicit TLS)", "addr", lcfg.Addr)

				ln, err := net.Listen("tcp", lcfg.Addr)
				if err != nil {
					logger.Error("failed to create SMTPS listener", "addr", lcfg.Addr, "error", err)
					return
				}

				tlsListener := tls.NewListener(ln, tlsMgr.GetTLSConfig())
				if err := server.Serve(tlsListener); err != nil && err != smtp.ErrServerClosed {
					logger.Error("smtps server error", "addr", lcfg.Addr, "error", err)
				}
			} else {
				// STARTTLS mode
				logger.Info("submission server listening (STARTTLS)", "addr", lcfg.Addr)
				if err := server.ListenAndServe(); err != nil && err != smtp.ErrServerClosed {
					logger.Error("submission server error", "addr", lcfg.Addr, "error", err)
				}
			}
		}()
	}

	// Wait for signals
	<-ctx.Done()
	logger.Info("shutting down")

	// Graceful shutdown
	logger.Info("shutting down gracefully")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown health server
	if err := healthServer.Shutdown(shutdownCtx); err != nil {
		logger.Error("health server shutdown error", "error", err)
	}

	// Shutdown all SMTP servers
	for i, server := range smtpServers {
		if err := server.Close(); err != nil {
			logger.Error("smtp server close error", "index", i, "addr", server.Addr, "error", err)
		}
	}

	wg.Wait()
	logger.Info("shutdown complete")
}

func createSMTPServer(cfg *config.Config, backend *proxy.Backend, tlsConfig *tls.Config, implicitTLS bool) *smtp.Server {
	s := smtp.NewServer(backend)

	// For STARTTLS mode, set TLS config (implicit TLS wraps the listener instead)
	if !implicitTLS {
		s.TLSConfig = tlsConfig
	}

	s.Domain = cfg.SMTP.BannerHostname
	s.ReadTimeout = cfg.Limits.ReadTimeout
	s.WriteTimeout = cfg.Limits.WriteTimeout
	s.MaxRecipients = cfg.Limits.MaxRecipients
	s.AllowInsecureAuth = cfg.SMTP.AllowInsecureAuth

	// Configure SMTP capabilities based on config
	s.EnableSMTPUTF8 = cfg.SMTP.Capabilities.SMTPUTF8
	s.EnableBINARYMIME = cfg.SMTP.Capabilities.BinaryMIME
	s.EnableDSN = cfg.SMTP.Capabilities.DSN
	s.MaxMessageBytes = cfg.SMTP.Capabilities.SizeLimit

	return s
}

func initLogger(level, format string) *slog.Logger {
	var logLevel slog.Level
	switch level {
	case "debug":
		logLevel = slog.LevelDebug
	case "info":
		logLevel = slog.LevelInfo
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{
		Level: logLevel,
	}

	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK\n")
}

func readyHandler(tlsMgr *tlsmgr.Manager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if tlsMgr.GetCertCount() == 0 {
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, "No certificates loaded\n")
			return
		}
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "Ready\n")
	}
}
