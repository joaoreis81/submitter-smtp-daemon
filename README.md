# Submitter SMTP Daemon

A production-grade SMTP submission server (outbound MTA) with comprehensive authentication, message processing pipeline, delivery management, and observability features. Built with Go 1.24.

## Overview

The Submitter SMTP Daemon is a high-performance outbound SMTP server designed for authenticated email submission. It receives messages from authenticated clients (MUAs), processes them through a multi-stage pipeline, and delivers them to destination mail servers with robust retry logic and comprehensive event tracking.

### Key Features

- **Multi-port TLS Support**: STARTTLS (port 587) and Implicit TLS (port 465) with SNI
- **Flexible Authentication**: Redis user database OR IP-based allowlist (CIDR support)
- **8-Stage Message Pipeline**: tmp → incoming → parsed → filtered → modified → signed → delivering → done/failed
- **Message Deduplication**: SHA256 hash-based duplicate detection with Redis
- **Delivery Queue**: SQLite-based queue with configurable retry schedules
- **Event Tracking**: Buffered ClickHouse logger for comprehensive analytics
- **Rate Limiting**: Multi-level (per-IP, per-user, global) with Redis backing
- **S3 Archival**: Message archival with date-based partitioning (configurable)
- **Comprehensive Observability**: Prometheus metrics, structured JSON logging, health checks
- **DKIM Signing**: Per-domain DKIM signature support (Phase 3)
- **Message Modification**: Header injection and content filtering (Phase 3)

## Architecture

### High-Level Flow

```
┌─────────────┐    ┌────────────────────┐    ┌───────────────┐    ┌────────────────┐
│   Client    │───▶│  SMTP Submission   │───▶│  Processing   │───▶│    Delivery    │
│  (MUA/App)  │    │   Server (587/465) │    │   Pipeline    │    │     Workers    │
└─────────────┘    └────────────────────┘    └───────────────┘    └────────────────┘
   Auth via              │                          │                      │
   Redis or IP           ├─ TLS/SNI                ├─ Parse              ├─ MX Lookup
                         ├─ Auth                   ├─ Dedup              ├─ SMTP Delivery
                         ├─ Rate Limit             ├─ Filter             ├─ Retry Logic
                         └─ Spool                  ├─ Modify             └─ Event Logging
                                                   ├─ Sign
                                                   └─ Queue
```

### 8-Stage Message Pipeline

1. **tmp**: Initial write staging area
2. **incoming**: Message accepted, metadata extracted
3. **parsed**: MIME parsing completed
4. **filtered**: Content filtering applied (anti-spam, policy)
5. **modified**: Headers added/removed, content transformation
6. **signed**: DKIM signature applied
7. **delivering**: Active delivery attempts
8. **done/failed**: Final state after delivery or permanent failure

### Core Components (Phase 1 & 2 Completed)

#### Phase 1: Foundation
- **SMTP Server** (`internal/smtpserver/`): Multi-listener SMTP submission server with SASL auth
- **Authentication** (`internal/auth/`): Redis user auth OR IP-based allowlist
- **Rate Limiting** (`internal/ratelimit/`): Redis-backed multi-level rate limiting
- **Spool** (`internal/spool/`): 8-stage atomic file-based message spooling
- **TLS Manager** (`internal/tlsmgr/`): SNI-based certificate selection with hot reload
- **Metrics** (`internal/metrics/`): 15+ Prometheus metrics
- **Logger** (`internal/logger/`): Structured JSON logging with context

#### Phase 2: Message Processing
- **Parser** (`internal/parser/`): MIME message parsing and metadata extraction
- **Deduplication** (`internal/dedup/`): SHA256 hash-based duplicate detection
- **Database** (`internal/db/`): SQLite database for messages and queue
- **Queue** (`internal/queue/`): Delivery queue with state management
- **ClickHouse** (`internal/clickhouse/`): Buffered event logger for analytics
- **Delivery Worker** (`internal/delivery/`): Parallel delivery with MX lookup and retry logic

#### Phase 3: Advanced Features (Planned)
- **DKIM Signing** (`internal/dkim/`): Per-domain DKIM signature
- **S3 Archival** (`internal/archive/`): Message archival with date partitioning
- **Message Modification** (`internal/modifier/`): Header and content transformation
- **SPF/DKIM Validation**: Output validation for deliverability

## Quick Start

### Prerequisites

- **Go 1.24+** (for building from source)
- **Docker & Docker Compose** (recommended for infrastructure)
- **TLS Certificates** in .pem format (certificate + key)
- **Redis** (for auth and rate limiting)
- **ClickHouse** (for event logging)
- **SQLite** (embedded, no separate installation)

### Infrastructure Setup

Start Redis and ClickHouse with Docker Compose:

```bash
# Start infrastructure services
docker-compose up -d

# Check services are running
docker-compose ps

# View logs
docker-compose logs -f
```

### Build and Run

```bash
# Clone the repository
git clone https://github.com/joaoreis81/submitter-smtp-daemon.git
cd submitter-smtp-daemon

# Build the binary
make build

# Create configuration (see Configuration section)
cp config.example.yaml config.yaml
# Edit config.yaml with your settings

# Initialize database schema
sqlite3 data/queue.db < internal/db/schema.sql

# Add test user to Redis
redis-cli SET "user:test@example.com:password" "your-password-hash"

# Grant permission to bind to privileged ports (<1024)
sudo setcap 'cap_net_bind_service=+ep' ./submitter-smtp-daemon

# Run the daemon
./submitter-smtp-daemon -config config.yaml
```

### Test the Daemon

```bash
# Test SMTP connectivity
printf "EHLO test\r\nQUIT\r\n" | nc localhost 587

# Send a test email (Python)
python3 << 'EOF'
import smtplib
from email.message import EmailMessage

msg = EmailMessage()
msg['From'] = 'test@example.com'
msg['To'] = 'recipient@example.com'
msg['Subject'] = 'Test Message'
msg.set_content('This is a test message.')

with smtplib.SMTP('localhost', 587) as s:
    s.starttls()
    s.login('test@example.com', 'your-password')
    s.send_message(msg)
    print("Message sent successfully!")
EOF
```

### Health Checks

```bash
# Liveness probe
curl http://localhost:8080/healthz

# Readiness probe
curl http://localhost:8080/readyz

# Prometheus metrics
curl http://localhost:9090/metrics
```

## Configuration

Configuration is managed through a YAML file with environment variable overrides. See `config.example.yaml` for complete reference.

### Basic Configuration

```yaml
server:
  hostname: "mail.example.com"

smtp:
  listeners:
    - addr: ":587"
      implicit_tls: false  # STARTTLS
    - addr: ":465"
      implicit_tls: true   # Implicit TLS
  read_timeout: 120s
  write_timeout: 120s
  max_message_size: 52428800  # 50MB
  max_recipients: 50

tls:
  certs_dir: "certs"
  min_version: "1.2"
  prefer_server_cipher: true

auth:
  method: "redis"  # or "ip"
  redis:
    enabled: true
    addresses: ["localhost:6379"]
    db: 0
    key_prefix: "user:"
  ip_allowlist:
    enabled: false
    allowed_ips: []

rate_limit:
  enabled: true
  user_messages_per_hour: 100
  user_recipients_per_hour: 500
  ip_connections_per_minute: 20
  ip_failed_auth_per_hour: 10
  global_messages_per_second: 50
  global_concurrent_connections: 1000

spool:
  base_path: "/var/spool/submitter-smtp"
  fsync: true
  cleanup:
    enabled: true
    interval: 1h
    retention_done: 24h
    retention_failed: 168h  # 7 days

deduplication:
  enabled: true
  window: 1h

database:
  path: "data/queue.db"
  max_connections: 10

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
  username: "default"
  password: ""
  buffer_size: 1000
  flush_interval: 30s
  compression: true

observability:
  log_level: "info"
  log_format: "json"
  metrics_enabled: true
  metrics_addr: ":9090"
  health_addr: ":8080"
```

### Environment Variable Overrides

Any configuration option can be overridden using environment variables:

```bash
export SMTP_SERVER_HOSTNAME="mail.example.com"
export SMTP_RATE_LIMIT_USER_MESSAGES_PER_HOUR="200"
export SMTP_LOG_LEVEL="debug"
./submitter-smtp-daemon -config config.yaml
```

Environment variable format: `SMTP_<SECTION>_<KEY>` (uppercase, underscores)

## Authentication

The daemon supports two authentication methods:

### Redis-Based Authentication

Users stored in Redis with password hashes:

```bash
# Add user (bcrypt hash recommended)
redis-cli SET "user:john@example.com:password" "$2a$10$..."

# Add user with metadata
redis-cli HSET "user:john@example.com" password "$2a$10$..." \
  name "John Doe" quota "1000" enabled "true"

# Remove user
redis-cli DEL "user:john@example.com:password"
```

**Configuration:**
```yaml
auth:
  method: "redis"
  redis:
    enabled: true
    addresses: ["localhost:6379"]
    key_prefix: "user:"
```

### IP-Based Authentication

Allow specific IPs or CIDR ranges without password:

```yaml
auth:
  method: "ip"
  ip_allowlist:
    enabled: true
    allowed_ips:
      - "192.168.1.0/24"    # Local network
      - "10.0.0.5"          # Specific server
      - "2001:db8::/32"     # IPv6 range
```

**Use cases:**
- Internal application servers
- Trusted relay hosts
- Development/testing environments

## Features

### Message Processing Pipeline

#### 1. Acceptance Phase
- TLS/SSL connection established with SNI support
- SASL authentication (PLAIN, LOGIN)
- Rate limiting checks (IP, user, global)
- Message spooled to `tmp` → `incoming` stage

#### 2. Parsing Phase
- MIME message parsing
- Metadata extraction (From, To, Subject, Date, Message-ID)
- Header analysis
- Content-Type detection

#### 3. Deduplication Phase
- SHA256 hash calculation of message body
- Redis lookup for duplicate detection
- Configurable deduplication window (default: 1 hour)
- Links duplicate to original message ID

#### 4. Queueing Phase
- Message stored in SQLite database
- Per-recipient delivery queue entries created
- Initial delivery attempt scheduled
- State: `queued` → `pending` → `delivering` → `delivered`/`perm_fail`

#### 5. Delivery Phase
- Parallel worker pool (configurable workers)
- MX record lookup for recipient domain
- SMTP delivery to destination server
- Temporary failure handling with retry schedule
- Permanent failure detection (5xx SMTP codes)
- ClickHouse event logging

### Delivery States

| State | Description |
|-------|-------------|
| `queued` | Initial state after queueing |
| `pending` | Waiting for retry (next_attempt set) |
| `delivering` | Currently being delivered |
| `delivered` | Successfully delivered |
| `temp_fail` | Temporary failure, will retry |
| `perm_fail` | Permanent failure, no more retries |

### Retry Schedule

Default retry schedule (configurable):

```
Attempt 1: Immediate
Attempt 2: +5 minutes
Attempt 3: +15 minutes
Attempt 4: +1 hour
Attempt 5: +4 hours
Attempt 6: +24 hours (final)
```

After max attempts exceeded, delivery marked as `perm_fail`.

### Rate Limiting

Multi-level rate limiting with Redis backing and in-memory fallback:

**Per-User Limits:**
- Messages per hour (default: 100)
- Recipients per hour (default: 500)

**Per-IP Limits:**
- Connections per minute (default: 20)
- Failed auth attempts per hour (default: 10)

**Global Limits:**
- Messages per second (default: 50)
- Concurrent connections (default: 1000)

Rate limit exceeded response:
```
450 4.7.1 Rate limit exceeded
```

### Message Deduplication

SHA256-based deduplication with configurable window:

```yaml
deduplication:
  enabled: true
  window: 1h  # Detect duplicates within 1 hour
```

When duplicate detected:
- Original message ID returned
- Duplicate not queued for delivery
- Event logged to ClickHouse
- Metrics updated

### TLS and Certificates

#### Certificate Format

Certificates must be in **HAProxy-compatible .pem format**: certificate and private key in a single file.

```bash
# Create certificate file
cat server.crt server.key > certs/mail.example.com.pem

# Or use certbot
cat /etc/letsencrypt/live/mail.example.com/fullchain.pem \
    /etc/letsencrypt/live/mail.example.com/privkey.pem \
    > certs/mail.example.com.pem
```

#### SNI Support

Automatic certificate selection based on client's SNI hostname:

```
certs/
├── mail.example.com.pem
├── smtp.company.net.pem
└── mail.another-domain.com.pem
```

Client connects with SNI `mail.example.com` → uses `mail.example.com.pem`

#### Hot Certificate Reload

Certificates automatically reloaded when modified (fsnotify):

```bash
# Update certificate (no restart needed)
cat new-cert.crt new-key.key > certs/mail.example.com.pem

# Or send SIGHUP
kill -HUP $(pidof submitter-smtp-daemon)
```

Active connections are NOT dropped during reload.

## Monitoring and Observability

### Structured Logging

All logs in JSON format with consistent fields:

```json
{
  "time": "2025-11-07T10:15:30.123Z",
  "level": "info",
  "component": "smtp-server",
  "msg": "Message accepted and spooled",
  "session_id": "550e8400-e29b-41d4-a716-446655440000",
  "message_id": "c89f5e1a-7c4e-4d8b-9a3c-1e5d7f9b2c4a",
  "client_ip": "192.168.1.100",
  "auth_user": "john@example.com",
  "message_size": 12345,
  "recipients": 2
}
```

**Key log fields:**
- `session_id`: SMTP session identifier
- `message_id`: Unique message identifier
- `client_ip`: Client IP address
- `auth_user`: Authenticated username
- `component`: Component generating the log

**Filter logs:**
```bash
# By message ID
./submitter-smtp-daemon | jq 'select(.message_id=="c89f5e1a-...")'

# By user
./submitter-smtp-daemon | jq 'select(.auth_user=="john@example.com")'

# Errors only
./submitter-smtp-daemon | jq 'select(.level=="error")'
```

### Prometheus Metrics

Comprehensive metrics exported on `:9090/metrics`:

#### Connection Metrics
```
smtp_connections_total{port,tls_mode} - Total connections
smtp_connections_current - Active connections (gauge)
smtp_connection_duration_seconds{port} - Connection duration histogram
smtp_connections_closed_total{port} - Closed connections
```

#### Authentication Metrics
```
smtp_auth_attempts_total{method,result} - Auth attempts by method/result
smtp_auth_duration_seconds{method} - Auth duration histogram
```

#### Message Metrics
```
smtp_messages_accepted_total{user} - Messages accepted by user
smtp_messages_rejected_total{reason} - Messages rejected by reason
smtp_message_size_bytes{user} - Message size histogram
```

#### Pipeline Metrics
```
smtp_pipeline_stage_duration_seconds{stage} - Stage duration histogram
smtp_messages_in_stage{stage} - Messages in each stage (gauge)
```

#### Delivery Metrics
```
smtp_delivery_attempts_total{result,mx_host} - Delivery attempts
smtp_delivery_duration_seconds{result} - Delivery duration histogram
```

#### Queue Metrics
```
smtp_queue_size{state} - Queue size by state (gauge)
```

#### Rate Limiting Metrics
```
smtp_rate_limit_hits_total{type} - Rate limit hits by type
```

#### ClickHouse Metrics
```
smtp_clickhouse_buffer_size - Current buffer size (gauge)
smtp_clickhouse_events_total{result} - Events logged
smtp_clickhouse_flush_duration_seconds - Flush duration histogram
```

#### S3 Metrics
```
smtp_s3_uploads_total{result} - S3 upload attempts
smtp_s3_upload_duration_seconds - Upload duration histogram
```

### ClickHouse Event Schema

Comprehensive event tracking with buffered writes:

```sql
CREATE TABLE outbound_events (
    event_id UUID,
    event_time DateTime64(3),
    msg_id String,
    session_id String,

    -- Connection
    client_ip String,
    client_port UInt16,
    listener_port UInt16,

    -- TLS
    tls_version String,
    tls_cipher String,
    sni_hostname String,

    -- Authentication
    auth_user String,
    auth_method String,
    auth_result String,

    -- Message
    mail_from String,
    rcpt_to Array(String),
    message_size UInt32,
    subject String,

    -- Delivery
    recipient String,
    mx_hostname String,
    mx_ip String,
    smtp_code UInt16,
    smtp_message String,
    delivery_result String,
    delivery_attempt UInt8,

    -- Performance
    stage String,
    stage_duration UInt32,
    total_duration UInt32
) ENGINE = MergeTree()
PARTITION BY toYYYYMM(event_time)
ORDER BY (event_time, client_ip, auth_user, msg_id)
TTL event_time + INTERVAL 90 DAY;
```

Query examples:

```sql
-- Delivery success rate by domain
SELECT
    domain,
    countIf(delivery_result = 'success') * 100.0 / count() as success_rate
FROM outbound_events
WHERE delivery_result != ''
  AND event_time >= now() - INTERVAL 1 DAY
GROUP BY splitByChar('@', recipient)[2] as domain
ORDER BY count() DESC
LIMIT 10;

-- Average delivery time by MX host
SELECT
    mx_hostname,
    avg(delivery_duration) / 1000 as avg_delivery_seconds,
    count() as deliveries
FROM outbound_events
WHERE delivery_result = 'success'
  AND event_time >= now() - INTERVAL 1 HOUR
GROUP BY mx_hostname
ORDER BY deliveries DESC
LIMIT 20;

-- Top senders by volume
SELECT
    auth_user,
    count(DISTINCT msg_id) as messages,
    sum(arrayLength(rcpt_to)) as recipients,
    sum(message_size) / 1024 / 1024 as total_mb
FROM outbound_events
WHERE stage = 'accepted'
  AND event_time >= now() - INTERVAL 1 DAY
GROUP BY auth_user
ORDER BY messages DESC
LIMIT 10;
```

### Health Endpoints

**Liveness probe:**
```bash
curl http://localhost:8080/healthz
# {"status":"ok"}
```

**Readiness probe:**
```bash
curl http://localhost:8080/readyz
# {"status":"ready","checks":{"redis":"ok","clickhouse":"ok","database":"ok"}}
```

**Metrics endpoint:**
```bash
curl http://localhost:9090/metrics
```

## Deployment

### Systemd Service

```bash
sudo tee /etc/systemd/system/submitter-smtp-daemon.service > /dev/null <<'EOF'
[Unit]
Description=Submitter SMTP Daemon
Documentation=https://github.com/joaoreis81/submitter-smtp-daemon
After=network-online.target redis.service
Wants=network-online.target

[Service]
Type=simple
User=smtp
Group=smtp
WorkingDirectory=/opt/submitter-smtp-daemon
ExecStart=/usr/local/bin/submitter-smtp-daemon -config /etc/submitter-smtp-daemon/config.yaml
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5s
TimeoutStopSec=30s

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=submitter-smtp-daemon

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/spool/submitter-smtp /var/lib/submitter-smtp-daemon
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictRealtime=true
RestrictNamespaces=true
RestrictSUIDSGID=true
LockPersonality=true

# Resource limits
LimitNOFILE=65536
LimitNPROC=4096

[Install]
WantedBy=multi-user.target
EOF

# Create user and directories
sudo useradd -r -s /bin/false -d /opt/submitter-smtp-daemon smtp
sudo mkdir -p /opt/submitter-smtp-daemon/{certs,data}
sudo mkdir -p /var/spool/submitter-smtp
sudo mkdir -p /etc/submitter-smtp-daemon
sudo chown -R smtp:smtp /opt/submitter-smtp-daemon /var/spool/submitter-smtp

# Deploy files
sudo cp submitter-smtp-daemon /usr/local/bin/
sudo cp config.yaml /etc/submitter-smtp-daemon/
sudo cp certs/*.pem /opt/submitter-smtp-daemon/certs/
sudo chown -R smtp:smtp /opt/submitter-smtp-daemon/certs

# Start service
sudo systemctl daemon-reload
sudo systemctl enable submitter-smtp-daemon
sudo systemctl start submitter-smtp-daemon
sudo systemctl status submitter-smtp-daemon
```

### Docker Deployment

```yaml
# docker-compose.yml
version: '3.8'

services:
  submitter-smtp-daemon:
    image: submitter-smtp-daemon:latest
    container_name: submitter-smtp-daemon
    restart: unless-stopped
    ports:
      - "587:587"
      - "465:465"
      - "8080:8080"
      - "9090:9090"
    volumes:
      - ./config.yaml:/config/config.yaml:ro
      - ./certs:/certs:ro
      - spool-data:/var/spool/submitter-smtp
      - queue-data:/data
    environment:
      - SMTP_LOG_LEVEL=info
    depends_on:
      - redis
      - clickhouse
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/healthz"]
      interval: 30s
      timeout: 5s
      retries: 3

  redis:
    image: redis:7-alpine
    restart: unless-stopped
    volumes:
      - redis-data:/data
    command: redis-server --appendonly yes

  clickhouse:
    image: clickhouse/clickhouse-server:23-alpine
    restart: unless-stopped
    volumes:
      - clickhouse-data:/var/lib/clickhouse
      - ./deployments/clickhouse/init.sql:/docker-entrypoint-initdb.d/init.sql:ro
    ulimits:
      nofile:
        soft: 262144
        hard: 262144

volumes:
  spool-data:
  queue-data:
  redis-data:
  clickhouse-data:
```

Run with:
```bash
docker-compose up -d
docker-compose logs -f submitter-smtp-daemon
```

## Troubleshooting

### Common Issues

#### 1. Port Permission Denied

**Error:**
```
listen tcp :587: bind: permission denied
```

**Solution:**
```bash
sudo setcap 'cap_net_bind_service=+ep' ./submitter-smtp-daemon
```

#### 2. Redis Connection Failed

**Error in logs:**
```json
{"level":"error","component":"auth","msg":"Redis connection failed"}
```

**Solutions:**
1. Check Redis is running: `redis-cli ping`
2. Verify Redis address in config.yaml
3. Check Redis authentication if enabled
4. Test connection: `redis-cli -h localhost -p 6379 ping`

#### 3. ClickHouse Connection Failed

**Solution:**
```bash
# Check ClickHouse is running
curl http://localhost:8123/

# Test query
echo "SELECT 1" | curl 'http://localhost:8123/' --data-binary @-

# Check database exists
echo "SHOW DATABASES" | curl 'http://localhost:8123/' --data-binary @-
```

#### 4. Delivery Failures

**Check delivery worker logs:**
```bash
./submitter-smtp-daemon | jq 'select(.component=="delivery")'
```

**Check queue status:**
```bash
sqlite3 data/queue.db "SELECT state, COUNT(*) FROM delivery_queue GROUP BY state;"
```

**Common delivery issues:**
- MX lookup failures → Check DNS resolution
- Connection timeouts → Check firewall rules
- Authentication errors → Check credentials
- Rate limiting → Slow down sending rate

#### 5. High Memory Usage

**Check current metrics:**
```bash
curl http://localhost:9090/metrics | grep smtp_connections_current
curl http://localhost:9090/metrics | grep smtp_clickhouse_buffer_size
```

**Solutions:**
1. Reduce ClickHouse buffer size
2. Reduce delivery worker count
3. Enable spool cleanup
4. Check for memory leaks in logs

### Debug Logging

Enable debug logging:

```yaml
observability:
  log_level: "debug"
```

Filter debug logs:
```bash
# Auth debugging
./submitter-smtp-daemon | jq 'select(.component=="auth")'

# Delivery debugging
./submitter-smtp-daemon | jq 'select(.component=="delivery")'

# Rate limit debugging
./submitter-smtp-daemon | jq 'select(.component=="ratelimit")'
```

## Development

### Project Structure

```
.
├── cmd/
│   └── outbound/
│       └── main.go              # Application entry point
├── internal/
│   ├── auth/                    # Authentication (Redis, IP)
│   ├── clickhouse/              # ClickHouse event logger
│   ├── config/                  # Configuration management
│   ├── db/                      # SQLite database
│   ├── dedup/                   # Message deduplication
│   ├── delivery/                # Delivery workers
│   ├── logger/                  # Structured logging
│   ├── metrics/                 # Prometheus metrics
│   ├── parser/                  # MIME parser
│   ├── queue/                   # Delivery queue
│   ├── ratelimit/               # Rate limiting
│   ├── smtpserver/              # SMTP server
│   ├── spool/                   # Message spooling
│   └── tlsmgr/                  # TLS certificate management
├── deployments/
│   ├── docker-compose.yml       # Development infrastructure
│   └── clickhouse/
│       └── init.sql             # ClickHouse schema
├── config.example.yaml          # Example configuration
├── Makefile                     # Build automation
├── Dockerfile                   # Container image
├── go.mod                       # Go module
├── CLAUDE.md                    # AI development guide
└── README.md                    # This file
```

### Building

```bash
# Standard build
make build

# Build with race detector
go build -race -o submitter-smtp-daemon ./cmd/outbound

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 make build
```

### Testing

```bash
# Run unit tests
go test ./...

# Run with coverage
go test -cover ./...

# Generate coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run linters
golangci-lint run
```

### Adding New Features

**To add a new configuration option:**

1. Add to struct in `internal/config/config.go`
2. Add to `config.example.yaml`
3. Add validation in `Validate()` method
4. Add environment variable override in `applyEnvOverrides()`

**To add a new metric:**

1. Define in `internal/metrics/metrics.go`
2. Initialize in `New()` function
3. Instrument code where metric should be recorded

**To add a new pipeline stage:**

1. Add stage constant in `internal/spool/spool.go`
2. Create stage directory in spool
3. Add processing logic in appropriate component
4. Update metrics and logging

## Implementation Status

### ✅ Phase 1: Foundation (Completed)
- SMTP server with TLS/SNI support
- Redis and IP-based authentication
- Multi-level rate limiting with Redis
- 8-stage atomic file spooling
- TLS certificate management with hot reload
- Comprehensive metrics and logging
- Configuration system with env var overrides

### ✅ Phase 2: Message Processing (Completed)
- MIME message parser with metadata extraction
- SHA256-based deduplication with Redis
- SQLite queue database with schema
- Delivery queue with state management
- Buffered ClickHouse event logger
- Delivery worker pool with MX lookup
- Retry logic with configurable schedule
- Temporary vs permanent failure handling

### 🚧 Phase 3: Advanced Features (Planned)
- DKIM signing with per-domain keys
- S3 message archival with date partitioning
- Message modification engine (headers, content)
- SPF/DKIM output validation
- Advanced filtering and policy engine
- Webhook notifications for events
- Admin API for queue management

## Performance Expectations

**Single Instance:**
- Concurrent connections: 1,000+
- Message acceptance: 100-200 msg/sec
- Delivery throughput: 50-100 msg/sec (MX limited)
- Average latency: <100ms (acceptance)

**Scaling:**
- Deploy multiple instances behind load balancer
- Shared Redis for rate limiting and deduplication
- Shared ClickHouse for centralized logging
- Independent delivery queues per instance

## License

Copyright © 2025 InteSys/EmailProfissional

## Support

For issues and questions:
- Review [CLAUDE.md](CLAUDE.md) for development guide
- Check logs with JSON filtering
- Enable debug logging
- Monitor Prometheus metrics
- Query ClickHouse events for troubleshooting

## Contributing

Contributions welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Write tests for new functionality
4. Ensure all tests pass
5. Run linters
6. Submit a pull request

## Acknowledgments

Built with:
- [go-smtp](https://github.com/emersion/go-smtp) - SMTP protocol library
- [zerolog](https://github.com/rs/zerolog) - Structured logging
- [go-redis](https://github.com/redis/go-redis) - Redis client
- [Prometheus Go client](https://github.com/prometheus/client_golang) - Metrics
- [fsnotify](https://github.com/fsnotify/fsnotify) - File system notifications
