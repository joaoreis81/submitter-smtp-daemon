package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the outbound daemon
type Metrics struct {
	// SMTP Connection metrics
	ConnectionsTotal     *prometheus.CounterVec
	ConnectionsCurrent   prometheus.Gauge
	ConnectionDuration   *prometheus.HistogramVec

	// TLS metrics (kept from edge proxy)
	TLSHandshakes      prometheus.Counter
	TLSHandshakeErrors prometheus.Counter
	CertReloads        prometheus.Counter
	CertReloadErrors   prometheus.Counter

	// Authentication metrics
	AuthAttempts     *prometheus.CounterVec
	AuthDuration     *prometheus.HistogramVec

	// Message acceptance metrics
	MessagesAccepted *prometheus.CounterVec
	MessagesRejected *prometheus.CounterVec
	MessageSize      *prometheus.HistogramVec

	// Pipeline stage metrics
	StageTotal    *prometheus.CounterVec
	StageDuration *prometheus.HistogramVec

	// Deduplication metrics
	DedupChecks     prometheus.Counter
	DedupDuplicates prometheus.Counter

	// DKIM metrics
	DKIMSigningTotal *prometheus.CounterVec
	DKIMSigningDuration prometheus.Histogram

	// Delivery metrics
	DeliveryAttempts *prometheus.CounterVec
	DeliveryDuration *prometheus.HistogramVec
	DeliveryQueue    prometheus.Gauge

	// S3 archival metrics
	S3UploadTotal    *prometheus.CounterVec
	S3UploadDuration prometheus.Histogram

	// ClickHouse metrics
	ClickHouseEventsTotal *prometheus.CounterVec
	ClickHouseBufferSize  prometheus.Gauge

	// Rate limiting metrics
	RateLimitHits *prometheus.CounterVec

	// Error metrics
	ErrorsTotal *prometheus.CounterVec

	// Data metrics
	BytesReceived prometheus.Counter
	BytesSent     prometheus.Counter
}

// New creates and registers all Prometheus metrics
func New() *Metrics {
	return &Metrics{
		// SMTP Connection metrics
		ConnectionsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "smtp_connections_total",
				Help: "Total number of SMTP connections",
			},
			[]string{"listener", "tls_mode"},
		),
		ConnectionsCurrent: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "smtp_connections_current",
				Help: "Current number of active SMTP connections",
			},
		),
		ConnectionDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "smtp_connection_duration_seconds",
				Help:    "SMTP connection duration in seconds",
				Buckets: prometheus.DefBuckets,
			},
			[]string{"listener"},
		),

		// TLS metrics
		TLSHandshakes: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "tls_handshakes_total",
				Help: "Total number of TLS handshakes",
			},
		),
		TLSHandshakeErrors: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "tls_handshake_errors_total",
				Help: "Total number of TLS handshake errors",
			},
		),
		CertReloads: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "cert_reloads_total",
				Help: "Total number of certificate reloads",
			},
		),
		CertReloadErrors: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "cert_reload_errors_total",
				Help: "Total number of certificate reload errors",
			},
		),

		// Authentication metrics
		AuthAttempts: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "smtp_auth_attempts_total",
				Help: "Total number of authentication attempts",
			},
			[]string{"method", "result"}, // method: redis, ip; result: success, failure, rate_limited
		),
		AuthDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "smtp_auth_duration_seconds",
				Help:    "Authentication duration in seconds",
				Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5},
			},
			[]string{"method"},
		),

		// Message acceptance metrics
		MessagesAccepted: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "smtp_messages_accepted_total",
				Help: "Total number of messages accepted",
			},
			[]string{"user"},
		),
		MessagesRejected: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "smtp_messages_rejected_total",
				Help: "Total number of messages rejected",
			},
			[]string{"reason"}, // reason: auth_required, rate_limited, too_large, invalid_recipient
		),
		MessageSize: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "smtp_message_size_bytes",
				Help:    "Size of accepted messages in bytes",
				Buckets: []float64{1024, 10240, 102400, 1048576, 10485760, 52428800}, // 1KB to 50MB
			},
			[]string{"user"},
		),

		// Pipeline stage metrics
		StageTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "pipeline_stage_total",
				Help: "Total number of messages processed through each pipeline stage",
			},
			[]string{"stage"}, // incoming, parsed, filtered, modified, signed, delivering, delivered, failed
		),
		StageDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "pipeline_stage_duration_seconds",
				Help:    "Duration of each pipeline stage in seconds",
				Buckets: []float64{.001, .005, .01, .05, .1, .5, 1, 5, 10},
			},
			[]string{"stage"},
		),

		// Deduplication metrics
		DedupChecks: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "dedup_checks_total",
				Help: "Total number of deduplication checks performed",
			},
		),
		DedupDuplicates: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "dedup_duplicates_total",
				Help: "Total number of duplicate messages detected",
			},
		),

		// DKIM metrics
		DKIMSigningTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "dkim_signing_total",
				Help: "Total number of DKIM signing operations",
			},
			[]string{"domain", "result"}, // result: success, failure
		),
		DKIMSigningDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "dkim_signing_duration_seconds",
				Help:    "DKIM signing duration in seconds",
				Buckets: []float64{.001, .005, .01, .05, .1},
			},
		),

		// Delivery metrics
		DeliveryAttempts: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "delivery_attempts_total",
				Help: "Total number of delivery attempts",
			},
			[]string{"result"}, // success, temp_fail, perm_fail
		),
		DeliveryDuration: promauto.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "delivery_duration_seconds",
				Help:    "Delivery duration in seconds",
				Buckets: []float64{.1, .5, 1, 2, 5, 10, 30, 60},
			},
			[]string{"mx_domain"},
		),
		DeliveryQueue: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "delivery_queue_length",
				Help: "Current number of messages in delivery queue",
			},
		),

		// S3 archival metrics
		S3UploadTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "s3_upload_total",
				Help: "Total number of S3 upload attempts",
			},
			[]string{"status"}, // success, failure
		),
		S3UploadDuration: promauto.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "s3_upload_duration_seconds",
				Help:    "S3 upload duration in seconds",
				Buckets: []float64{.1, .5, 1, 2, 5, 10, 30},
			},
		),

		// ClickHouse metrics
		ClickHouseEventsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "clickhouse_events_total",
				Help: "Total number of events sent to ClickHouse",
			},
			[]string{"status"}, // success, failure
		),
		ClickHouseBufferSize: promauto.NewGauge(
			prometheus.GaugeOpts{
				Name: "clickhouse_buffer_size",
				Help: "Current size of ClickHouse event buffer",
			},
		),

		// Rate limiting metrics
		RateLimitHits: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "rate_limit_hits_total",
				Help: "Total number of rate limit hits",
			},
			[]string{"type", "identifier"}, // type: ip, user, global
		),

		// Error metrics
		ErrorsTotal: promauto.NewCounterVec(
			prometheus.CounterOpts{
				Name: "errors_total",
				Help: "Total number of errors by type",
			},
			[]string{"component", "error_type"},
		),

		// Data metrics
		BytesReceived: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "bytes_received_total",
				Help: "Total bytes received from clients",
			},
		),
		BytesSent: promauto.NewCounter(
			prometheus.CounterOpts{
				Name: "bytes_sent_total",
				Help: "Total bytes sent to recipients",
			},
		),
	}
}

// RecordConnection records a new SMTP connection
func (m *Metrics) RecordConnection(listener, tlsMode string) {
	m.ConnectionsTotal.WithLabelValues(listener, tlsMode).Inc()
	m.ConnectionsCurrent.Inc()
}

// RecordConnectionClose records a closed SMTP connection
func (m *Metrics) RecordConnectionClose(listener string, duration float64) {
	m.ConnectionsCurrent.Dec()
	m.ConnectionDuration.WithLabelValues(listener).Observe(duration)
}

// RecordAuthAttempt records an authentication attempt
func (m *Metrics) RecordAuthAttempt(method, result string, duration float64) {
	m.AuthAttempts.WithLabelValues(method, result).Inc()
	m.AuthDuration.WithLabelValues(method).Observe(duration)
}

// RecordMessageAccepted records an accepted message
func (m *Metrics) RecordMessageAccepted(user string, size int64) {
	m.MessagesAccepted.WithLabelValues(user).Inc()
	m.MessageSize.WithLabelValues(user).Observe(float64(size))
	m.BytesReceived.Add(float64(size))
}

// RecordMessageRejected records a rejected message
func (m *Metrics) RecordMessageRejected(reason string) {
	m.MessagesRejected.WithLabelValues(reason).Inc()
}

// RecordStage records a message passing through a pipeline stage
func (m *Metrics) RecordStage(stage string, duration float64) {
	m.StageTotal.WithLabelValues(stage).Inc()
	m.StageDuration.WithLabelValues(stage).Observe(duration)
}

// RecordDedupCheck records a deduplication check
func (m *Metrics) RecordDedupCheck(isDuplicate bool) {
	m.DedupChecks.Inc()
	if isDuplicate {
		m.DedupDuplicates.Inc()
	}
}

// RecordDKIMSigning records a DKIM signing operation
func (m *Metrics) RecordDKIMSigning(domain, result string, duration float64) {
	m.DKIMSigningTotal.WithLabelValues(domain, result).Inc()
	m.DKIMSigningDuration.Observe(duration)
}

// RecordDelivery records a delivery attempt
func (m *Metrics) RecordDelivery(result, mxDomain string, duration float64) {
	m.DeliveryAttempts.WithLabelValues(result).Inc()
	m.DeliveryDuration.WithLabelValues(mxDomain).Observe(duration)
}

// RecordS3Upload records an S3 upload
func (m *Metrics) RecordS3Upload(status string, duration float64) {
	m.S3UploadTotal.WithLabelValues(status).Inc()
	m.S3UploadDuration.Observe(duration)
}

// RecordClickHouseEvent records a ClickHouse event
func (m *Metrics) RecordClickHouseEvent(status string) {
	m.ClickHouseEventsTotal.WithLabelValues(status).Inc()
}

// RecordRateLimitHit records a rate limit hit
func (m *Metrics) RecordRateLimitHit(limitType, identifier string) {
	m.RateLimitHits.WithLabelValues(limitType, identifier).Inc()
}

// RecordError records an error
func (m *Metrics) RecordError(component, errorType string) {
	m.ErrorsTotal.WithLabelValues(component, errorType).Inc()
}

// SetDeliveryQueueSize sets the current delivery queue size
func (m *Metrics) SetDeliveryQueueSize(size int) {
	m.DeliveryQueue.Set(float64(size))
}

// SetClickHouseBufferSize sets the current ClickHouse buffer size
func (m *Metrics) SetClickHouseBufferSize(size int) {
	m.ClickHouseBufferSize.Set(float64(size))
}
