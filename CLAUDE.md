# Submitter SMTP Daemon - Development Guide

## Project Overview

The Submitter SMTP Daemon is a production-grade SMTP submission server (outbound MTA) designed for authenticated email sending. It receives messages from authenticated clients (MUAs or applications), processes them through a comprehensive multi-stage pipeline, and delivers them to destination mail servers with robust retry logic and complete event tracking.

**Key Distinction**: This is an **outbound** submission server, NOT an inbound relay proxy. It accepts messages from authenticated users and delivers them directly to recipient mail servers via MX lookup.

## Architecture

### Core Design: Outbound Submission Server

The daemon uses a **direct delivery** model:

1. Client connects to daemon (port 587 STARTTLS or 465 implicit TLS)
2. Client authenticates (Redis user DB or IP allowlist)
3. Client sends message (MAIL/RCPT/DATA)
4. Message spooled through 8-stage pipeline
5. Delivery workers pick up from queue
6. MX lookup for recipient domain
7. Direct SMTP delivery to destination MX server
8. Retry on temporary failures, give up on permanent failures

**Benefits:**
- No backend dependency for message acceptance
- Complete control over delivery logic
- Comprehensive event tracking from acceptance to delivery
- Flexible message modification and signing
- Independent retry schedules per recipient
- Full audit trail in ClickHouse

### 8-Stage Message Pipeline

```
1. tmp        → Initial write staging
2. incoming   → Message accepted, metadata extracted
3. parsed     → MIME parsing completed
4. filtered   → Content filtering applied (anti-spam, policy)
5. modified   → Headers added/removed, content transformation
6. signed     → DKIM signature applied
7. delivering → Active delivery attempts
8. done/failed → Final state
```

**Atomic operations**: All stage transitions use fsync for durability. Messages never lost during transitions.

### Component Architecture

```
┌──────────────────────────────────────────────────────────────────────┐
│                        SMTP Submission Layer                         │
│  • TLS/SNI (ports 587/465)                                          │
│  • SASL Auth (PLAIN/LOGIN)                                          │
│  • Rate Limiting (IP, user, global)                                 │
│  • Message spooling (tmp → incoming)                                │
└────────────────────┬─────────────────────────────────────────────────┘
                     │
┌────────────────────▼─────────────────────────────────────────────────┐
│                      Processing Pipeline                             │
│  • Parser: MIME parsing, metadata extraction                        │
│  • Deduplicator: SHA256 hash-based duplicate detection              │
│  • Queue: SQLite database for delivery queue                        │
│  • ClickHouse: Buffered event logging                               │
└────────────────────┬─────────────────────────────────────────────────┘
                     │
┌────────────────────▼─────────────────────────────────────────────────┐
│                       Delivery Layer                                 │
│  • Worker Pool: Parallel delivery workers                           │
│  • MX Lookup: DNS-based mail exchanger resolution                   │
│  • SMTP Delivery: Direct connection to destination MX               │
│  • Retry Logic: Configurable exponential backoff                    │
│  • Event Tracking: Success/failure/temp-fail logging                │
└──────────────────────────────────────────────────────────────────────┘
```

### Infrastructure Dependencies

**Required:**
- **Redis**: User authentication, rate limiting, deduplication
- **SQLite**: Delivery queue (embedded, no separate install)
- **ClickHouse**: Event logging and analytics

**Optional:**
- **MinIO/S3**: Message archival (Phase 3)

**Development:**
- Use `docker-compose.yml` in `deployments/` to start all infrastructure

## Implementation Status

### ✅ Phase 1: Foundation (Completed)

**Location**: `cmd/outbound/main.go` + core internal packages

1. **SMTP Server** (`internal/smtpserver/`)
   - Multi-listener support (587 STARTTLS, 465 implicit TLS)
   - SASL authentication (PLAIN, LOGIN)
   - Session management with goroutine-per-connection
   - Message acceptance with size limits
   - Integration with all Phase 1 components

2. **Authentication** (`internal/auth/`)
   - `redis.go`: Redis user authentication with bcrypt
   - `ipauth.go`: IP allowlist with CIDR support
   - Constant-time password comparison
   - Authentication metrics

3. **Rate Limiting** (`internal/ratelimit/`)
   - Redis-backed with in-memory fallback
   - Per-user: messages/hour, recipients/hour
   - Per-IP: connections/minute, failed auth/hour
   - Global: messages/second, concurrent connections
   - Automatic cleanup of expired entries

4. **Spool** (`internal/spool/`)
   - 8-stage directory structure
   - Atomic writes with fsync
   - Stage transitions with move operations
   - Automatic cleanup for old messages
   - Read/write/move operations

5. **TLS Manager** (`internal/tlsmgr/`)
   - SNI-based certificate selection
   - HAProxy .pem format support (cert+key)
   - Hot reload with fsnotify
   - Fallback to first certificate if SNI not matched

6. **Metrics** (`internal/metrics/`)
   - 15+ Prometheus metrics
   - Connection, auth, message, pipeline, delivery metrics
   - Histogram and gauge support
   - Label-based filtering

7. **Logger** (`internal/logger/`)
   - Zerolog-based structured logging
   - Context methods: WithSessionID, WithMessageID, WithUser
   - Component-based filtering
   - JSON output for production

8. **Configuration** (`internal/config/`)
   - YAML-based with env var overrides
   - Validation on load
   - Comprehensive coverage of all components
   - Example config in `config.example.yaml`

### ✅ Phase 2: Message Processing (Completed)

**Location**: Internal packages for processing and delivery

1. **Parser** (`internal/parser/`)
   - MIME message parsing
   - Metadata extraction (From, To, Subject, Date, Message-ID)
   - Header analysis
   - Content-Type detection
   - Size calculation

2. **Deduplication** (`internal/dedup/`)
   - SHA256 hash of message body
   - Redis storage with TTL (configurable window)
   - Links duplicate to original message ID
   - Metrics for duplicate detection

3. **Database** (`internal/db/`)
   - `schema.go`: SQLite schema for messages and delivery_queue tables
   - `db.go`: Database connection management
   - WAL mode for better concurrency
   - Connection pooling

4. **Queue** (`internal/queue/`)
   - Message insertion (AddMessage)
   - Per-recipient delivery queue entries (EnqueueDelivery)
   - State transitions: queued → pending → delivering → delivered/perm_fail
   - GetPendingDeliveries with next_attempt filtering
   - UpdateDeliveryState for status updates
   - Retry scheduling integration

5. **ClickHouse** (`internal/clickhouse/`)
   - Buffered event logger (reduces write load)
   - Configurable buffer size and flush interval
   - Event types: connection, auth, message, delivery
   - Automatic flush on buffer full or timer
   - Graceful shutdown with final flush
   - TODO: Actual ClickHouse connection (currently logs only)

6. **Delivery Worker** (`internal/delivery/`)
   - Worker pool with configurable count
   - Polling loop for pending deliveries
   - MX record lookup (highest priority)
   - SMTP delivery to destination server
   - Retry logic with exponential backoff
   - Temporary vs permanent failure detection
   - State updates via queue
   - ClickHouse event logging for all attempts

### 🚧 Phase 3: Advanced Features (Planned)

**Not yet implemented:**

1. **DKIM Signing** (`internal/dkim/`)
   - Per-domain DKIM keys
   - RSA key support (2048/4096 bit)
   - Signature generation and injection
   - Header canonicalization
   - Body hash calculation

2. **S3 Archival** (`internal/archive/`)
   - Message upload to S3/MinIO
   - Date-based partitioning (YYYY/MM/DD)
   - Compression (gzip)
   - Retention policies
   - Parallel upload workers

3. **Message Modification** (`internal/modifier/`)
   - Header injection/removal
   - Content transformation
   - Policy-based modification
   - Configurable profiles

4. **SPF/DKIM Validation**
   - Outbound validation for deliverability
   - Pre-delivery checks
   - Warning logs for issues

5. **Admin API**
   - REST API for queue management
   - Retry/delete operations
   - Queue inspection
   - Statistics endpoint

## Configuration

### Main Config: `config.yaml`

See `config.example.yaml` for complete reference with all options.

**Key sections:**

```yaml
server:
  hostname: "mail.example.com"

smtp:
  listeners:
    - addr: ":587"
      implicit_tls: false
    - addr: ":465"
      implicit_tls: true
  max_message_size: 52428800  # 50MB
  max_recipients: 50

tls:
  certs_dir: "certs"
  min_version: "1.2"

auth:
  method: "redis"  # or "ip"
  redis:
    enabled: true
    addresses: ["localhost:6379"]
    key_prefix: "user:"

rate_limit:
  enabled: true
  user_messages_per_hour: 100
  user_recipients_per_hour: 500
  ip_connections_per_minute: 20

spool:
  base_path: "/var/spool/submitter-smtp"
  fsync: true
  cleanup:
    enabled: true
    interval: 1h
    retention_done: 24h
    retention_failed: 168h

delivery:
  workers: 10
  poll_interval: 5s
  max_attempts: 5
  retry_schedule:
    - 5m
    - 15m
    - 1h
    - 4h
    - 24h

clickhouse:
  addresses: ["http://localhost:8123"]
  database: "submitter"
  buffer_size: 1000
  flush_interval: 30s
```

### Environment Variable Overrides

Format: `SMTP_<SECTION>_<KEY>` (uppercase, underscores)

```bash
export SMTP_SERVER_HOSTNAME="mail.example.com"
export SMTP_RATE_LIMIT_USER_MESSAGES_PER_HOUR="200"
export SMTP_LOG_LEVEL="debug"
```

## Development

### Build

```bash
# Standard build
make build

# Build with race detector (recommended for development)
go build -race -o submitter-smtp-daemon ./cmd/outbound

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 make build
```

### Run

```bash
# Start infrastructure (Redis, ClickHouse, MinIO)
docker-compose up -d

# Grant capability to bind privileged ports
sudo setcap 'cap_net_bind_service=+ep' ./submitter-smtp-daemon

# Run with config
./submitter-smtp-daemon -config config.yaml

# Or with debug logging
SMTP_LOG_LEVEL=debug ./submitter-smtp-daemon -config config.yaml
```

### Test

```bash
# Unit tests
go test ./...

# Coverage report
go test -cover ./... | grep -E '^ok|^FAIL'

# Generate HTML coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Lint
golangci-lint run

# Test SMTP connectivity
printf "EHLO test\r\nQUIT\r\n" | nc localhost 587
```

### Docker

```bash
# Build image
docker build -t submitter-smtp-daemon:latest .

# Run with docker-compose
docker-compose up -d

# View logs
docker-compose logs -f submitter-smtp-daemon
```

## Code Structure

```
cmd/
  outbound/
    main.go                     # Entry point, wiring all components

internal/
  auth/
    redis.go                    # Redis user authentication
    ipauth.go                   # IP allowlist authentication

  clickhouse/
    client.go                   # Buffered ClickHouse event logger

  config/
    config.go                   # Configuration structs, loading, validation

  db/
    schema.go                   # SQLite schema definition
    db.go                       # Database connection management

  dedup/
    dedup.go                    # SHA256 deduplication with Redis

  delivery/
    worker.go                   # Delivery worker pool, MX lookup, SMTP delivery

  logger/
    logger.go                   # Structured logging with zerolog

  metrics/
    metrics.go                  # Prometheus metrics definitions

  parser/
    parser.go                   # MIME message parser

  queue/
    queue.go                    # Delivery queue management

  ratelimit/
    ratelimit.go                # Redis-backed rate limiting

  smtpserver/
    backend.go                  # SMTP backend implementation
    session.go                  # SMTP session handling, SASL auth

  spool/
    spool.go                    # 8-stage message spooling

  tlsmgr/
    manager.go                  # TLS certificate management, SNI

deployments/
  docker-compose.yml            # Development infrastructure
  clickhouse/
    init.sql                    # ClickHouse schema

config.example.yaml             # Example configuration
go.mod                          # Go module definition
```

## Key Design Decisions

### 1. Why SQLite for Queue?

**Pros:**
- Embedded, no separate server
- ACID transactions
- WAL mode for good concurrency
- Simple deployment
- Built-in query capabilities

**Cons:**
- Not suitable for multi-instance deployment (each instance needs own DB)
- For multi-instance, would need Redis or PostgreSQL

**Decision**: SQLite is perfect for single-instance deployment. For multi-instance, Phase 3 can add Redis queue option.

### 2. Why Redis for Auth and Rate Limiting?

**Pros:**
- Fast in-memory operations
- TTL support for automatic cleanup
- Atomic operations
- Shared across multiple instances
- Pub/sub for user updates

**Cons:**
- External dependency
- Memory-based (need persistence config)

**Decision**: Redis is industry standard for this use case. In-memory fallback provides graceful degradation.

### 3. Why ClickHouse for Events?

**Pros:**
- Column-oriented for analytics
- Excellent compression
- Fast aggregation queries
- Handles billions of rows
- Time-series optimized

**Cons:**
- Requires separate server
- More complex than SQLite/PostgreSQL

**Decision**: Email event data is perfect for ClickHouse. Buffering reduces write load. Essential for production observability.

### 4. Why 8-Stage Pipeline?

**Pros:**
- Clear separation of concerns
- Easy to debug (inspect stage directories)
- Atomic transitions prevent data loss
- Parallel processing possible
- Easy to add/remove stages

**Cons:**
- More complex than single directory
- More disk I/O for stage moves

**Decision**: Production email systems need robust pipeline. Stage separation is worth the complexity.

### 5. Why Not Backend Relay?

**Original design** (edge proxy): Client → Proxy → Backend MTA
**Current design**: Client → Daemon → Destination MX (direct)

**Rationale:**
- Submission servers should deliver directly
- No backend dependency for message acceptance
- Complete control over delivery logic
- Better for multi-tenant scenarios
- Easier to scale horizontally

## Common Development Tasks

### Add New Configuration Option

1. Add field to appropriate struct in `internal/config/config.go`
2. Add to `config.example.yaml` with comments
3. Add validation in `Validate()` method
4. Add environment variable override in `applyEnvOverrides()`
5. Document in `README.md`

Example:
```go
// In internal/config/config.go
type DeliveryConfig struct {
    Workers      int           `yaml:"workers"`
    MaxAttempts  int           `yaml:"max_attempts"`
    NewOption    string        `yaml:"new_option"`  // Add here
}

// In Validate() method
if cfg.Delivery.NewOption == "" {
    return fmt.Errorf("delivery.new_option is required")
}

// In applyEnvOverrides()
if v := os.Getenv("SMTP_DELIVERY_NEW_OPTION"); v != "" {
    cfg.Delivery.NewOption = v
}
```

### Add New Metric

1. Define metric in `internal/metrics/metrics.go` struct
2. Initialize in `New()` function with `promauto.New*`
3. Instrument code where metric should be recorded
4. Test with `curl http://localhost:9090/metrics | grep metric_name`

Example:
```go
// In Metrics struct
NewCounter prometheus.Counter

// In New() function
NewCounter: promauto.NewCounter(prometheus.CounterOpts{
    Name: "smtp_new_metric_total",
    Help: "Description of new metric",
}),

// In code
metrics.NewCounter.Inc()
```

### Add New Pipeline Stage

1. Add stage constant in `internal/spool/spool.go`
2. Ensure stage directory created in `New()`
3. Add processing logic in appropriate component
4. Use `spool.Move(msgID, fromStage, toStage)` for transitions
5. Update metrics for stage tracking

Example:
```go
// In internal/spool/spool.go
const (
    // ... existing stages
    StageNewStage Stage = "new_stage"
)

// In New()
stages := []Stage{StageIncoming, ..., StageNewStage}

// In processing component
if err := spool.Move(msgID, StageParsed, StageNewStage); err != nil {
    return fmt.Errorf("failed to move to new stage: %w", err)
}
```

### Add New Delivery Retry Strategy

Current strategy is fixed schedule in config. To add dynamic strategy:

1. Create new function in `internal/delivery/worker.go`
2. Add strategy type to config
3. Update `calculateNextRetry()` to use strategy
4. Document in `README.md`

Example:
```go
// Add to config
type RetryStrategy string

const (
    RetryFixed       RetryStrategy = "fixed"
    RetryExponential RetryStrategy = "exponential"
)

// In worker
func (w *Worker) calculateNextRetry(attempts int) time.Time {
    switch w.config.RetryStrategy {
    case RetryFixed:
        return w.calculateFixedRetry(attempts)
    case RetryExponential:
        return w.calculateExponentialRetry(attempts)
    default:
        return w.calculateFixedRetry(attempts)
    }
}
```

## Debugging

### Enable Debug Logging

```yaml
observability:
  log_level: "debug"
```

Or via environment:
```bash
SMTP_LOG_LEVEL=debug ./submitter-smtp-daemon -config config.yaml
```

### Filter Logs by Component

```bash
# Auth debugging
./submitter-smtp-daemon | jq 'select(.component=="auth")'

# Delivery debugging
./submitter-smtp-daemon | jq 'select(.component=="delivery")'

# Rate limit debugging
./submitter-smtp-daemon | jq 'select(.component=="ratelimit")'

# All errors
./submitter-smtp-daemon | jq 'select(.level=="error")'
```

### Check Metrics

```bash
# All metrics
curl -s http://localhost:9090/metrics | grep ^smtp_

# Specific metric
curl -s http://localhost:9090/metrics | grep smtp_delivery_attempts_total

# Connection count
curl -s http://localhost:9090/metrics | grep smtp_connections_current
```

### Inspect Queue

```bash
# Queue state distribution
sqlite3 data/queue.db "SELECT state, COUNT(*) FROM delivery_queue GROUP BY state;"

# Failed deliveries
sqlite3 data/queue.db "SELECT * FROM delivery_queue WHERE state='perm_fail' LIMIT 10;"

# Pending retries
sqlite3 data/queue.db "SELECT * FROM delivery_queue WHERE state='pending' ORDER BY next_attempt LIMIT 10;"

# Recent successful deliveries
sqlite3 data/queue.db "SELECT * FROM delivery_queue WHERE state='delivered' ORDER BY updated_at DESC LIMIT 10;"
```

### Inspect Spool

```bash
# Count messages in each stage
for stage in tmp incoming parsed filtered modified signed delivering done failed; do
    count=$(ls -1 /var/spool/submitter-smtp/$stage 2>/dev/null | wc -l)
    echo "$stage: $count"
done

# View message in stage
cat /var/spool/submitter-smtp/incoming/<message-id>

# Check for stuck messages
find /var/spool/submitter-smtp/delivering -type f -mmin +60
```

### Query ClickHouse

```bash
# Recent events
echo "SELECT * FROM outbound_events ORDER BY event_time DESC LIMIT 10 FORMAT Pretty" | \
  curl 'http://localhost:8123/' --data-binary @-

# Delivery success rate
echo "SELECT
    countIf(delivery_result = 'success') * 100.0 / count() as success_rate
FROM outbound_events
WHERE delivery_result != ''
  AND event_time >= now() - INTERVAL 1 HOUR" | \
  curl 'http://localhost:8123/' --data-binary @-

# Top senders
echo "SELECT auth_user, count(*) as messages
FROM outbound_events
WHERE stage = 'accepted'
  AND event_time >= now() - INTERVAL 1 HOUR
GROUP BY auth_user
ORDER BY messages DESC
LIMIT 10 FORMAT Pretty" | \
  curl 'http://localhost:8123/' --data-binary @-
```

### Health Checks

```bash
# Liveness
curl http://localhost:8080/healthz
# Expected: {"status":"ok"}

# Readiness (checks dependencies)
curl http://localhost:8080/readyz
# Expected: {"status":"ready","checks":{"redis":"ok","clickhouse":"ok","database":"ok"}}
```

## Testing Strategy

### Unit Tests

Each package should have tests for:
- Configuration validation
- Error handling
- Edge cases
- Concurrency safety

Example test file: `internal/queue/queue_test.go`

### Integration Tests

Test cross-component interactions:
- SMTP session → spool → queue
- Queue → delivery worker → MX lookup
- Rate limiting → authentication flow

### Manual Tests

```bash
# Test authentication
python3 test-scripts/test-auth.py

# Test message submission
python3 test-scripts/test-submit.py

# Test rate limiting
python3 test-scripts/test-ratelimit.py

# Test delivery
python3 test-scripts/test-delivery.py
```

### Load Tests

Use tools like:
- **smtp-source** (from Postfix)
- **k6** with custom SMTP script
- **Apache JMeter** with SMTP sampler

Target metrics:
- Messages/sec acceptance rate
- Delivery throughput
- Memory usage under load
- Queue growth rate

## Performance Expectations

### Single Instance

**Message Acceptance:**
- Rate: 100-200 msg/sec
- Latency: <100ms (acceptance)
- Concurrent connections: 1,000+

**Message Delivery:**
- Rate: 50-100 msg/sec (limited by destination MX)
- Latency: 1-5 seconds (including MX lookup)
- Worker pool: 10-50 workers

**Bottlenecks:**
1. Destination MX server speed (biggest factor)
2. DNS resolution (MX lookup)
3. Disk I/O (spool fsync)
4. Redis connection pool

### Scaling

**Horizontal (multiple instances):**
- Load balance ports 587/465 (HAProxy/Nginx)
- Each instance has own SQLite queue
- Shared Redis for auth/rate limiting/deduplication
- Shared ClickHouse for events

**Vertical (bigger machine):**
- Increase worker count (delivery.workers)
- Increase Redis connection pool
- Faster disk for spool (SSD)

## Common Issues

### Issue: Messages stuck in "delivering" stage

**Cause**: Worker crashed during delivery

**Fix**:
1. Check worker logs for errors
2. Check queue state: `state='delivering' AND updated_at < (now() - 1 hour)`
3. Reset state: `UPDATE delivery_queue SET state='pending' WHERE ...`

### Issue: High memory usage

**Cause**: ClickHouse buffer too large or too many workers

**Fix**:
1. Reduce `clickhouse.buffer_size` (default: 1000)
2. Reduce `delivery.workers` (default: 10)
3. Enable spool cleanup with shorter retention

### Issue: Rate limit false positives

**Cause**: Redis connection issues or incorrect limits

**Fix**:
1. Check Redis connection: `redis-cli ping`
2. Verify limits in config match expected usage
3. Check rate limit metrics: `smtp_rate_limit_hits_total`

### Issue: Delivery failures

**Cause**: Network issues, MX problems, or destination rejection

**Fix**:
1. Check delivery logs: `jq 'select(.component=="delivery")'`
2. Verify MX lookup: `dig MX example.com`
3. Test direct connection: `telnet mx.example.com 25`
4. Check ClickHouse for SMTP codes: `delivery_result='perm_fail'`

## Git Workflow

### Commit Standards

Use conventional commits:
- `feat:` - New feature
- `fix:` - Bug fix
- `refactor:` - Code refactoring
- `docs:` - Documentation only
- `test:` - Test additions/changes
- `chore:` - Build, dependencies, etc.

Examples:
```
feat: Add DKIM signing support
fix: Correct retry schedule calculation
docs: Update README with new metrics
refactor: Simplify queue state transitions
```

### Ignored Files

`.gitignore` excludes:
- `config.yaml` - Contains sensitive config
- `certs/` - TLS certificates
- `data/` - SQLite database files
- `spool/` - Message spool directories
- Binary artifacts

## Documentation

- **README.md** - User-facing documentation
- **CLAUDE.md** - This file (AI development guide)
- **config.example.yaml** - Complete configuration reference
- **deployments/clickhouse/init.sql** - ClickHouse schema
- **internal/db/schema.sql** - SQLite schema

## Support

For development questions:
1. Check this file (CLAUDE.md) first
2. Review README.md for user-facing docs
3. Examine code comments
4. Check ClickHouse events for runtime behavior
5. Enable debug logging for detailed trace

## Future Enhancements (Phase 3+)

Priority features for next phase:
1. **DKIM signing** - Critical for deliverability
2. **S3 archival** - Compliance and debugging
3. **Message modification** - Header injection, filtering
4. **Admin API** - Queue management, statistics
5. **SPF/DKIM validation** - Pre-delivery checks
6. **Webhook notifications** - Integration with external systems
7. **Redis queue option** - For multi-instance deployment
8. **Connection pooling** - Reuse SMTP connections to same MX
9. **Greylisting** - Reduce spam (inbound feature)
10. **LDAP auth** - Alternative to Redis auth

Each enhancement should follow the established patterns:
- Configuration in `config.yaml`
- Metrics in `internal/metrics/`
- Logging with component context
- Tests for new functionality
- Documentation updates
