package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the SMTP proxy
type Metrics struct {
	// Connection metrics
	CurrentConnections prometheus.Gauge
	TotalConnections   prometheus.Counter
	ConnectionDuration prometheus.Histogram

	// TLS metrics
	TLSHandshakes      prometheus.Counter
	TLSHandshakeErrors prometheus.Counter
	CertReloads        prometheus.Counter
	CertReloadErrors   prometheus.Counter

	// Authentication metrics
	AuthAttempts prometheus.Counter
	AuthSuccess  prometheus.Counter
	AuthFailures prometheus.Counter
	AuthDuration prometheus.Histogram

	// Relay metrics
	RelayAttempts prometheus.Counter
	RelaySuccess  prometheus.Counter
	RelayFailures prometheus.Counter
	RelayDuration prometheus.Histogram

	// Data transfer metrics
	BytesReceived    prometheus.Counter
	BytesSent        prometheus.Counter
	MessagesReceived prometheus.Counter
	MessagesSent     prometheus.Counter

	// Rate limiting metrics
	RateLimitHits prometheus.Counter

	// Error metrics
	Errors prometheus.Counter
}

// New creates a new Metrics instance and registers all metrics
func New(namespace string) *Metrics {
	m := &Metrics{
		CurrentConnections: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Name:      "current_connections",
			Help:      "Current number of active SMTP connections",
		}),
		TotalConnections: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "connections_total",
			Help:      "Total number of SMTP connections",
		}),
		ConnectionDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "connection_duration_seconds",
			Help:      "Duration of SMTP connections in seconds",
			Buckets:   prometheus.DefBuckets,
		}),
		TLSHandshakes: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "tls_handshakes_total",
			Help:      "Total number of TLS handshakes",
		}),
		TLSHandshakeErrors: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "tls_handshake_errors_total",
			Help:      "Total number of TLS handshake errors",
		}),
		CertReloads: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cert_reloads_total",
			Help:      "Total number of certificate reloads",
		}),
		CertReloadErrors: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "cert_reload_errors_total",
			Help:      "Total number of certificate reload errors",
		}),
		AuthAttempts: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "auth_attempts_total",
			Help:      "Total number of authentication attempts",
		}),
		AuthSuccess: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "auth_success_total",
			Help:      "Total number of successful authentications",
		}),
		AuthFailures: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "auth_failures_total",
			Help:      "Total number of failed authentications",
		}),
		AuthDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "auth_duration_seconds",
			Help:      "Duration of authentication attempts in seconds",
			Buckets:   prometheus.DefBuckets,
		}),
		RelayAttempts: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "relay_attempts_total",
			Help:      "Total number of relay attempts",
		}),
		RelaySuccess: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "relay_success_total",
			Help:      "Total number of successful relays",
		}),
		RelayFailures: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "relay_failures_total",
			Help:      "Total number of failed relays",
		}),
		RelayDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: namespace,
			Name:      "relay_duration_seconds",
			Help:      "Duration of relay operations in seconds",
			Buckets:   prometheus.DefBuckets,
		}),
		BytesReceived: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "bytes_received_total",
			Help:      "Total bytes received from clients",
		}),
		BytesSent: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "bytes_sent_total",
			Help:      "Total bytes sent to backend",
		}),
		MessagesReceived: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "messages_received_total",
			Help:      "Total messages received from clients",
		}),
		MessagesSent: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "messages_sent_total",
			Help:      "Total messages sent to backend",
		}),
		RateLimitHits: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "rate_limit_hits_total",
			Help:      "Total number of rate limit hits",
		}),
		Errors: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: namespace,
			Name:      "errors_total",
			Help:      "Total number of errors",
		}),
	}

	return m
}
