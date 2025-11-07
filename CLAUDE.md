# SMTP Edge Proxy - Development Guide

## Project Overview

The SMTP Edge Proxy is a production-grade SMTP submission proxy that provides TLS termination, multi-domain SNI support, and authenticated message relay. Built with Go 1.24 using the `go-smtp` library.

## Architecture

### Core Design: Authenticated Connection Reuse

The proxy uses a **single authenticated connection** model:

1. Client connects to proxy (port 587 STARTTLS or 465 implicit TLS)
2. Client authenticates with username/password
3. Proxy authenticates to backend using **user's credentials**
4. Connection to backend is **kept open** for the session
5. Client sends MAIL/RCPT/DATA commands
6. Proxy forwards commands to backend using **same authenticated connection**
7. Client disconnects → proxy closes backend connection

**Benefits:**
- Backend sees original user's identity
- Backend can apply per-user authorization rules
- No separate relay authentication needed
- Simplified security model
- Better performance (connection reuse)

### Component Architecture

```
Client → Proxy (TLS termination, SNI) → Backend (auth + relay)
         ↓
         - Rate limiting
         - IP allow/deny lists
         - DSN parameter validation
         - Message header injection
         - Metrics & logging
```

### Key Components

1. **TLS Manager** (`internal/tlsmgr/`)
   - SNI-based certificate selection
   - Automatic certificate loading from directory
   - Supports HAProxy .pem format (cert+key in one file)
   - Hot reload capable

2. **Backend Selector** (`internal/backend/`)
   - Per-user and per-domain backend routing
   - CSV-based configuration
   - Priority: exact user match → domain match → default fallback

3. **Authenticator** (`internal/auth/`)
   - Validates credentials against backend SMTP server
   - Returns authenticated connection for reuse
   - Integrates with rate limiter

4. **Relay Handler** (`internal/relay/`)
   - Forwards messages using authenticated connection
   - Adds X-Original-* headers for audit trail
   - Supports DSN parameter forwarding

5. **Rate Limiter** (`internal/ratelimit/`)
   - Per-IP and per-user failed auth tracking
   - Configurable windows and thresholds
   - Thread-safe with RWMutex

6. **Metrics** (`internal/metrics/`)
   - Prometheus metrics
   - Auth success/failure counters
   - Rate limit hits
   - Auth duration histograms

## Configuration

### Main Config: `config.yaml`

See `config.example.yaml` for complete reference.

**Key settings:**

```yaml
listeners:
  submission: ":587"  # STARTTLS
  smtps: ":465"       # Implicit TLS

backend:
  backends_file: "backends.csv"  # Per-user/domain routing
  host: "172.23.20.40"           # Default backend (fallback)
  port: 587
  use_starttls: false  # ⚠️ See Known Issues below
  timeout: 10s

smtp:
  banner_hostname: "edge-smtp.example.com"
  capabilities:
    dsn: true          # Enable DSN (RFC 3461)
    smtputf8: false
    binary_mime: false
    size_limit: 150000000  # 150MB

security:
  rate_limit_enabled: true
  rate_limit_per_ip: 10
  rate_limit_per_user: 100
  rate_limit_window: 1m

limits:
  max_recipients: 20
  read_timeout: 120s
  write_timeout: 120s
```

### Backend Routing: `backends.csv`

Format:
```csv
auth_selector,host,port,use_starttls,implicit_tls,skip_tls_verify,timeout,relay_timeout,active
user@example.com,mail1.backend.com,587,false,false,false,10s,30s,true
@domain.tld,mail2.backend.com,587,false,false,false,10s,30s,true
```

**Selection priority:**
1. Exact email match (`user@example.com`)
2. Domain match (`@domain.tld`)
3. Fall back to default backend in `config.yaml`

## Development

### Build

```bash
make build
```

### Run

```bash
# Grant capability to bind privileged ports (<1024)
sudo setcap 'cap_net_bind_service=+ep' ./smtp-edge-proxy

# Run with config
./smtp-edge-proxy -config config.yaml
```

### Test

```bash
# Unit tests
make test

# Coverage report
make test-coverage

# Lint
make lint
```

### Docker

```bash
make docker-build
make docker-run
```

## Known Issues & Workarounds

### ⚠️ Backend STARTTLS Timeout

**Issue**: The `go-smtp` library's `DialStartTLS()` function has timeout issues when connecting to backends.

**Symptoms**: Authentication hangs for 10-15 seconds then times out.

**Workaround**: Set `use_starttls: false` for backend connections in `config.yaml`. Client-facing ports (587/465) still support STARTTLS correctly.

**Status**: Library limitation, not a proxy bug.

### ⚠️ Configuration Limitations

Some config options are defined but not fully implemented:

- `idle_timeout` - Field exists but not connected to SMTP server
- `max_connections` - No enforcement (use OS limits or reverse proxy)
- `max_connections_per_ip` - No enforcement (use firewall or reverse proxy)
- `arc_enabled` - Not implemented

See `docs/CONFIG-STATUS.md` for full details.

## Features

### ✅ Implemented

1. **TLS/SSL Support**
   - STARTTLS on port 587
   - Implicit TLS (SMTPS) on port 465
   - SNI-based certificate selection
   - Configurable TLS versions and cipher suites

2. **Authentication**
   - SASL PLAIN and LOGIN
   - Backend credential verification
   - Connection reuse after auth

3. **Per-User Backend Routing**
   - CSV-based backend selection
   - User and domain-level routing
   - Default fallback backend

4. **DSN (Delivery Status Notifications)**
   - RFC 3461 implementation
   - Parameter validation (RET, ENVID, NOTIFY, ORCPT)
   - Forwarding to backend
   - Comprehensive logging

5. **Security**
   - IP allow/deny lists
   - Rate limiting (per-IP and per-user)
   - TLS enforcement options
   - Authenticated relay only

6. **Message Headers**
   - `X-Original-Client-IP` - Client IP address
   - `X-Original-Auth-User` - Authenticated username
   - `X-Original-Server-Name` - Server hostname
   - `X-Edge-Received-TLS` - TLS status (true/false)
   - `X-Secure-Conn` - Connection type (STARTTLS/SSL/false)
   - `X-Original-EHLO` - Client EHLO hostname

7. **Observability**
   - Structured JSON logging (slog)
   - Request ID tracking
   - Prometheus metrics
   - Health checks (`/healthz`, `/readyz`)

### ⚠️ Partially Implemented

- **Connection Pooling** - Config exists but not implemented
- **Idle Timeout** - Config exists but not wired up
- **ARC Headers** - Config exists but not implemented

### ❌ Not Implemented

- Global connection limiting
- Per-IP connection limiting
- Connection pooling to backend

## Testing

### Test Backend

**Host**: `172.23.20.40` or `mail.intesys.io`
**Port**: 587
**Test Credentials**:
- User: `joao@intesys.io`
- Password: `GJPKPSDSJXSHFTOM`
- Recipient: `joao@intesys.com.br`

### Manual Tests

```bash
# Test STARTTLS connection
printf "EHLO test\r\nQUIT\r\n" | nc localhost 587

# Test AUTH capabilities
python3 << 'EOF'
import smtplib, ssl
s = smtplib.SMTP('localhost', 587)
s.starttls(context=ssl._create_unverified_context())
s.ehlo()
print("AUTH:", s.has_extn('AUTH'))
print("DSN:", s.has_extn('DSN'))
print("SIZE:", s.esmtp_features.get('size'))
EOF
```

Test scripts available in `helper-scripts/` directory.

## Performance

### Concurrency Model

- **Goroutine-per-connection** architecture
- Non-blocking I/O
- Thread-safe rate limiter and TLS manager
- Independent session handling (no shared state)

### Expected Throughput

**Single instance:**
- Concurrent connections: 1,000+
- Auth throughput: 100-200/sec (backend limited)
- Message throughput: 50-100/sec (backend limited)

**With connection pooling (if implemented):**
- Auth throughput: 500-1,000/sec
- Message throughput: 200-500/sec

### Scaling

For >500 msg/sec:
1. Deploy 4-8 instances
2. Use HAProxy/Nginx for load balancing
3. Implement connection pooling to backend
4. Use fast backend (SSD, close network proximity)

See `docs/PERFORMANCE-ANALYSIS.md` for detailed analysis.

## Code Structure

```
cmd/
  main.go                   # Entry point, server setup
internal/
  auth/
    auth.go                 # Backend authentication
  backend/
    selector.go             # Per-user backend routing
  config/
    config.go               # Configuration structs & loading
  metrics/
    metrics.go              # Prometheus metrics
  proxy/
    backend.go              # SMTP protocol handler, session management
  ratelimit/
    ratelimit.go            # Rate limiting
  relay/
    relay.go                # Message forwarding with headers
  tlsmgr/
    manager.go              # TLS certificate management
```

## Debugging

### Enable Debug Logs

```yaml
observability:
  log_level: "debug"
```

### Filter Logs by Request ID

```bash
./smtp-edge-proxy -config config.yaml | jq 'select(.request_id=="<REQUEST_ID>")'
```

### Check Metrics

```bash
curl http://localhost:9090/metrics | grep smtp_proxy
```

### Health Checks

```bash
curl http://localhost:8080/healthz  # Liveness
curl http://localhost:8080/readyz   # Readiness (checks certs loaded)
```

## Common Development Tasks

### Add New SMTP Capability

1. Add config field in `internal/config/config.go` (`SMTPCapabilities` struct)
2. Set capability in `cmd/main.go` (`createSMTPServer` function)
3. If needs validation/processing, add to `internal/proxy/backend.go`

### Add New Metric

1. Define metric in `internal/metrics/metrics.go`
2. Instrument code where metric should be recorded
3. Test with `curl http://localhost:9090/metrics`

### Add New Configuration Option

1. Add to struct in `internal/config/config.go`
2. Add to `config.example.yaml`
3. Add environment variable override in `applyEnvOverrides()` (optional)
4. Update validation in `Validate()` method

## Documentation

- `README.md` - Complete user documentation
- `docs/DSN-IMPLEMENTATION.md` - DSN feature details
- `docs/HEADERS.md` - Message headers added by proxy
- `docs/LOGGING.md` - Logging format and request tracking
- `docs/PERFORMANCE-ANALYSIS.md` - Concurrency and scaling
- `docs/CONFIG-STATUS.md` - Configuration implementation status
- `docs/QUICK-START.md` - Quick start guide
- `docs/TEST-RESULTS.md` - Test results and verification

## Git Workflow

### Commit Standards

- Use conventional commits format
- Include issue/ticket references
- Sign commits if required

### Ignored Files

- `config.yaml` - Contains sensitive configuration
- `certs/` - Contains TLS certificates
- `backends.csv` - May contain sensitive routing info
- Binary artifacts

See `.gitignore` for complete list.
