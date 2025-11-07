# SMTP Edge Proxy

A production-grade SMTP submission proxy with TLS termination, SNI support, per-user backend routing, and authenticated relay capabilities.

## Overview

The SMTP Edge Proxy is a high-performance, feature-rich submission proxy designed to sit at the edge of your email infrastructure. It provides TLS termination with SNI-based multi-domain certificate selection, authenticates users against backend SMTP servers, and intelligently routes messages based on user or domain matching rules.

### Key Features

- **Multi-port TLS Support**: STARTTLS (port 587) and Implicit TLS (port 465)
- **SNI-based Certificate Selection**: Host multiple domains on a single proxy
- **Per-User/Domain Backend Routing**: Route different users to different backend servers
- **Authenticated Connection Reuse**: Single authenticated connection per session
- **Message Header Injection**: Track edge connection metadata (IP, user, TLS status)
- **DSN Support**: Full RFC 3461 Delivery Status Notification forwarding
- **Rate Limiting**: Per-IP and per-user failed authentication tracking
- **Comprehensive Observability**: Prometheus metrics, structured JSON logging, health checks
- **Hot Certificate Reload**: Update certificates without dropping connections

## Table of Contents

- [Quick Start](#quick-start)
- [Installation](#installation)
- [Configuration](#configuration)
- [Backend Routing](#backend-routing)
- [TLS and Certificates](#tls-and-certificates)
- [Features](#features)
- [Monitoring and Metrics](#monitoring-and-metrics)
- [Deployment](#deployment)
- [Troubleshooting](#troubleshooting)
- [Architecture](#architecture)
- [Development](#development)
- [Possible Fixes/TODOs](#possible-fixestodos)
- [Enhancement Suggestions](#enhancement-suggestions)

## Quick Start

### Prerequisites

- Go 1.24 or later
- TLS certificates in .pem format (certificate + key in one file)
- Access to backend SMTP server(s)

### Build and Run

```bash
# Clone the repository
git clone <repository-url>
cd submission-proxy

# Build the binary
make build

# Grant permission to bind to privileged ports (<1024)
sudo setcap 'cap_net_bind_service=+ep' ./smtp-edge-proxy

# Run with default configuration
./smtp-edge-proxy -config config.yaml
```

### Test the Proxy

```bash
# Test STARTTLS connection
printf "EHLO test\r\nQUIT\r\n" | nc localhost 587

# Test Implicit TLS connection
openssl s_client -connect localhost:465 -servername mail.example.com

# Send a test email (requires Python 3)
python3 test-proxy-e2e.py
```

### Health Checks

```bash
# Liveness probe
curl http://localhost:8080/healthz

# Readiness probe (checks if certificates are loaded)
curl http://localhost:8080/readyz

# Prometheus metrics
curl http://localhost:9090/metrics
```

## Installation

### From Source

**Requirements:**
- Go 1.24+
- Git

```bash
# Clone and build
git clone <repository-url>
cd submission-proxy
make build

# The binary will be created as ./smtp-edge-proxy
```

### Using Docker

```bash
# Build Docker image
make docker-build

# Run with Docker
docker run --rm \
  -p 587:587 \
  -p 465:465 \
  -p 8080:8080 \
  -p 9090:9090 \
  -v $(pwd)/certs:/certs:ro \
  -v $(pwd)/config.yaml:/config/config.yaml:ro \
  -v $(pwd)/backends.csv:/backends.csv:ro \
  smtp-edge-proxy
```

### System Service (systemd)

```bash
# Copy binary to system location
sudo cp smtp-edge-proxy /usr/local/bin/

# Create service file
sudo tee /etc/systemd/system/smtp-edge-proxy.service > /dev/null <<EOF
[Unit]
Description=SMTP Edge Proxy
After=network.target

[Service]
Type=simple
User=smtp-proxy
Group=smtp-proxy
ExecStart=/usr/local/bin/smtp-edge-proxy -config /etc/smtp-edge-proxy/config.yaml
Restart=on-failure
RestartSec=5s

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/log/smtp-edge-proxy

[Install]
WantedBy=multi-user.target
EOF

# Enable and start
sudo systemctl daemon-reload
sudo systemctl enable smtp-edge-proxy
sudo systemctl start smtp-edge-proxy
sudo systemctl status smtp-edge-proxy
```

## Configuration

Configuration is managed through a YAML file. See `configs/config.example.yaml` for a complete reference.

### Basic Configuration

Create a `config.yaml` file:

```yaml
# Listener configuration
listeners:
  submission: ":587"  # STARTTLS port
  smtps: ":465"       # Implicit TLS port

# TLS configuration
tls:
  min_version: "1.2"
  certs_dir: "certs"
  prefer_server_cipher: true

# Backend server (default/fallback)
backend:
  backends_file: "backends.csv"
  host: "mail.backend.example.com"
  port: 587
  use_starttls: false  # See Known Issues
  timeout: 10s
  relay_timeout: 30s

# SMTP capabilities
smtp:
  banner_hostname: "edge-smtp.example.com"
  require_tls: false
  allow_insecure_auth: false
  capabilities:
    dsn: true
    smtputf8: false
    binary_mime: false
    size_limit: 150000000  # 150MB

# Security settings
security:
  rate_limit_enabled: true
  rate_limit_per_ip: 10      # Failed auths per IP
  rate_limit_per_user: 100   # Failed auths per user
  rate_limit_window: 1m
  ip_allow_list: []          # Empty = allow all
  ip_deny_list: []

# Connection limits
limits:
  max_recipients: 20
  read_timeout: 120s
  write_timeout: 120s
  idle_timeout: 300s

# Observability
observability:
  log_level: "info"       # debug, info, warn, error
  log_format: "json"
  metrics_enabled: true
  metrics_addr: ":9090"
  health_addr: ":8080"
```

### Multiple Listeners

The proxy supports binding to multiple addresses and interfaces:

```yaml
listeners:
  listeners:
    - addr: ":587"              # All interfaces, STARTTLS
      implicit_tls: false
    - addr: ":465"              # All interfaces, Implicit TLS
      implicit_tls: true
    - addr: "127.0.0.1:2587"    # IPv4 localhost only
      implicit_tls: false
    - addr: "[::1]:3587"        # IPv6 localhost only
      implicit_tls: false
    - addr: "192.168.1.10:587"  # Specific IP address
      implicit_tls: false
```

### Environment Variable Overrides

Any configuration option can be overridden using environment variables:

```bash
export SMTP_BACKEND_HOST="mail.example.com"
export SMTP_BACKEND_PORT="587"
export SMTP_LOG_LEVEL="debug"
./smtp-edge-proxy -config config.yaml
```

Environment variable format: `SMTP_<SECTION>_<KEY>` (uppercase, underscores)

## Backend Routing

The proxy supports sophisticated per-user and per-domain backend routing through a CSV configuration file.

### backends.csv Format

```csv
auth_selector,backend_host,backend_port,use_starttls,implicit_tls,skip_tls_verify,timeout,relay_timeout,active
user@example.com,mail1.backend.com,587,false,false,false,10s,30s,1
@domain.tld,mail2.backend.com,465,false,true,false,10s,30s,1
@company.net,mail3.backend.com,587,true,false,false,15s,45s,1
```

### Field Descriptions

- **auth_selector**: Match pattern
  - `user@example.com` - Exact email match (highest priority)
  - `@domain.tld` - Domain match (medium priority)
  - Empty or missing - Falls back to default backend in config.yaml
- **backend_host**: Backend SMTP server hostname or IP
- **backend_port**: Backend port (587=submission, 465=smtps, 25=smtp)
- **use_starttls**: `true` to upgrade with STARTTLS, `false` for plain/implicit TLS
- **implicit_tls**: `true` for implicit TLS (SMTPS), `false` otherwise
- **skip_tls_verify**: `true` to skip certificate verification (use for self-signed certs)
- **timeout**: Connection timeout (e.g., "10s", "30s")
- **relay_timeout**: Message relay timeout
- **active**: `1` to enable, `0` to disable this route

### Routing Priority

1. **Exact email match**: `user@example.com` (highest priority)
2. **Domain match**: `@example.com`
3. **Default backend**: From `config.yaml` (fallback)

### Example Configuration

```csv
auth_selector,backend_host,backend_port,use_starttls,implicit_tls,skip_tls_verify,timeout,relay_timeout,active
admin@example.com,mail-admin.internal,587,false,false,true,10s,30s,1
@vip-domain.com,mail-premium.internal,465,false,true,false,10s,30s,1
@example.com,mail.backend.com,587,false,false,false,10s,30s,1
```

In this example:
- `admin@example.com` routes to `mail-admin.internal`
- All `@vip-domain.com` users route to `mail-premium.internal` via implicit TLS
- Other `@example.com` users route to `mail.backend.com`
- Users from other domains use the default backend from `config.yaml`

## TLS and Certificates

### Certificate Format

Certificates must be in **HAProxy-compatible .pem format**: certificate and private key in a single file.

```bash
# Create a certificate file
cat server.crt server.key > certs/mail.example.com.pem

# Or use certbot
cat /etc/letsencrypt/live/mail.example.com/fullchain.pem \
    /etc/letsencrypt/live/mail.example.com/privkey.pem \
    > certs/mail.example.com.pem
```

### Certificate Directory Structure

```
certs/
├── mail.example.com.pem
├── mail.another-domain.com.pem
├── smtp.company.net.pem
└── ...
```

### SNI (Server Name Indication) Support

The proxy automatically selects the correct certificate based on the client's SNI hostname:

1. Client connects and sends SNI hostname (e.g., `mail.example.com`)
2. Proxy matches SNI hostname to certificate filename
3. If no match found, uses the first loaded certificate as fallback

### Hot Certificate Reload

Certificates are monitored with `fsnotify` and automatically reloaded when modified:

```bash
# Update certificate (no restart needed)
cat new-cert.crt new-key.key > certs/mail.example.com.pem

# Or send SIGHUP to reload all certificates
kill -HUP $(pidof smtp-edge-proxy)
```

**Note**: Active TLS connections are NOT dropped during reload. Only new connections use the updated certificates.

### TLS Configuration Options

```yaml
tls:
  min_version: "1.2"           # Minimum TLS version (1.0, 1.1, 1.2, 1.3)
  certs_dir: "certs"           # Certificate directory path
  cipher_suites: []            # Empty = Go defaults (recommended)
  prefer_server_cipher: true   # Prefer server cipher suite order
```

### Generating Self-Signed Certificates (Testing Only)

```bash
# Generate self-signed certificate
openssl req -x509 -newkey rsa:4096 -nodes \
  -keyout server.key -out server.crt \
  -days 365 -subj "/CN=mail.example.com"

# Combine into .pem format
cat server.crt server.key > certs/mail.example.com.pem
```

## Features

### SMTP Capabilities

The proxy supports all modern SMTP extensions:

#### Always Enabled (Not Configurable)
- **PIPELINING**: Command pipelining for better performance
- **SIZE**: Message size negotiation
- **ENHANCEDSTATUSCODES**: RFC 3463 enhanced status codes
- **8BITMIME**: 8-bit MIME content transfer
- **CHUNKING**: BDAT chunking for large messages

#### Configurable Capabilities
- **DSN**: Delivery Status Notifications (RFC 3461)
- **SMTPUTF8**: UTF-8 in email addresses (RFC 6531)
- **BINARYMIME**: Binary MIME content (RFC 3030)
- **AUTH PLAIN**: Plain authentication mechanism
- **AUTH LOGIN**: Login authentication mechanism
- **STARTTLS**: TLS upgrade on port 587

### Delivery Status Notifications (DSN)

Full RFC 3461 implementation with parameter validation and forwarding:

**MAIL FROM Parameters:**
- `RET=FULL` - Request full message in bounce
- `RET=HDRS` - Request only headers in bounce
- `ENVID=<string>` - Envelope identifier for tracking

**RCPT TO Parameters:**
- `NOTIFY=NEVER` - Never send notification
- `NOTIFY=SUCCESS` - Notify on successful delivery
- `NOTIFY=FAILURE` - Notify on delivery failure
- `NOTIFY=DELAY` - Notify on delivery delay
- `ORCPT=<address>` - Original recipient address

Example:
```smtp
MAIL FROM:<sender@example.com> RET=FULL ENVID=abc123
RCPT TO:<user@example.com> NOTIFY=SUCCESS,FAILURE
```

### Message Headers Added

The proxy adds the following headers to all relayed messages for audit and tracking:

| Header | Description | Example |
|--------|-------------|---------|
| `X-Original-Client-IP` | Client IP address | `192.168.1.100` |
| `X-Original-Auth-User` | Authenticated username | `user@example.com` |
| `X-Original-Server-Name` | Server hostname | `mail.example.com` |
| `X-Original-EHLO` | Client EHLO hostname | `mail.client.com` |
| `X-Edge-Received-TLS` | TLS used (boolean) | `true` or `false` |
| `X-Secure-Conn` | Connection type | `STARTTLS`, `SSL`, or `false` |

**Example headers in a relayed message:**
```
X-Original-Client-IP: 192.168.1.100
X-Original-Auth-User: user@example.com
X-Original-Server-Name: mail.example.com
X-Edge-Received-TLS: true
X-Secure-Conn: STARTTLS
X-Original-EHLO: mail.client.com
```

### Rate Limiting

Failed authentication attempts are tracked per-IP and per-user:

```yaml
security:
  rate_limit_enabled: true
  rate_limit_per_ip: 10      # Max failed auths per IP in window
  rate_limit_per_user: 100   # Max failed auths per user in window
  rate_limit_window: 1m      # Time window (e.g., 1m, 5m, 1h)
```

When limits are exceeded:
- Client receives: `421 4.7.1 Too many failed authentication attempts`
- Metrics counter incremented: `smtp_proxy_rate_limit_hits_total`
- Event logged with request ID

### IP Filtering

Allow/deny lists support individual IPs and CIDR ranges:

```yaml
security:
  # Allow only these IPs/ranges (empty = allow all)
  ip_allow_list:
    - "192.168.1.0/24"
    - "10.0.0.5"

  # Block these IPs/ranges
  ip_deny_list:
    - "192.0.2.0/24"    # TEST-NET-1
    - "198.51.100.0/24" # TEST-NET-2
```

**Note**: If `ip_allow_list` is not empty, only IPs in the list are allowed. `ip_deny_list` is checked after allow list.

### Security Features

- **No AUTH before TLS**: Configurable requirement for TLS before authentication
- **Credential Redaction**: Passwords never appear in logs
- **Authenticated Relay Only**: Backend connection authenticated with user's credentials
- **TLS Enforcement**: Optionally require TLS for all client connections
- **Backend TLS Verification**: Configurable per-backend certificate validation

## Monitoring and Metrics

### Structured Logging

All logs are output in JSON format with consistent fields:

```json
{
  "time": "2025-10-07T13:40:02.305986098-03:00",
  "level": "INFO",
  "msg": "new session",
  "request_id": "5899490e-66fc-4d4d-a0b3-3d15db65aebd",
  "remote_addr": "[::1]:42542",
  "client_ip": "::1",
  "implicit_tls": true,
  "tls_version": "TLS1.3",
  "tls_cipher": "TLS_AES_128_GCM_SHA256",
  "tls_server_name": "localhost"
}
```

**Key log fields:**
- `request_id`: Unique identifier for each session (tracks full transaction)
- `client_ip`: Client IP address
- `auth_user`: Authenticated username
- `backend`: Backend server used
- `tls_type`: Connection encryption type (`STARTTLS`, `SSL`, `false`)

**Filter logs by request ID:**
```bash
./smtp-edge-proxy -config config.yaml | jq 'select(.request_id=="5899490e-66fc-4d4d-a0b3-3d15db65aebd")'
```

**Show only errors:**
```bash
./smtp-edge-proxy -config config.yaml | jq 'select(.level=="ERROR")'
```

### Prometheus Metrics

The proxy exports comprehensive Prometheus metrics on the configured metrics port (default: 9090).

**Access metrics:**
```bash
curl http://localhost:9090/metrics
```

#### Key Metrics

**Connection Metrics:**
```
smtp_proxy_current_connections - Active connections (gauge)
smtp_proxy_connections_total - Total connections (counter)
smtp_proxy_connection_duration_seconds - Connection duration histogram
```

**Authentication Metrics:**
```
smtp_proxy_auth_success_total - Successful authentications (counter)
smtp_proxy_auth_failures_total - Failed authentications (counter)
smtp_proxy_auth_duration_seconds - Auth duration histogram
```

**Relay Metrics:**
```
smtp_proxy_relay_success_total - Successful message relays (counter)
smtp_proxy_relay_failures_total - Failed message relays (counter)
smtp_proxy_bytes_received_total - Bytes received from clients (counter)
smtp_proxy_bytes_sent_total - Bytes sent to backends (counter)
```

**TLS Metrics:**
```
smtp_proxy_tls_handshakes_total - TLS handshake count (counter)
smtp_proxy_cert_reloads_total - Certificate reload count (counter)
```

**Rate Limiting Metrics:**
```
smtp_proxy_rate_limit_hits_total{type="ip"} - IP rate limit hits (counter)
smtp_proxy_rate_limit_hits_total{type="user"} - User rate limit hits (counter)
```

### Health Endpoints

**Liveness probe** (always returns 200 if running):
```bash
curl http://localhost:8080/healthz
# Response: {"status":"ok"}
```

**Readiness probe** (checks if certificates are loaded):
```bash
curl http://localhost:8080/readyz
# Response: {"status":"ready","certificates_loaded":5}
# or: {"status":"not ready","error":"no certificates loaded"}
```

**Prometheus metrics endpoint:**
```bash
curl http://localhost:9090/metrics
```

### Prometheus Integration Example

Add to `prometheus.yml`:

```yaml
scrape_configs:
  - job_name: 'smtp-edge-proxy'
    static_configs:
      - targets: ['localhost:9090']
    scrape_interval: 15s
```

### Grafana Dashboard Queries

**Authentication success rate:**
```promql
rate(smtp_proxy_auth_success_total[5m]) /
  (rate(smtp_proxy_auth_success_total[5m]) + rate(smtp_proxy_auth_failures_total[5m]))
```

**Average relay duration:**
```promql
rate(smtp_proxy_relay_duration_seconds_sum[5m]) /
  rate(smtp_proxy_relay_duration_seconds_count[5m])
```

**Active connections:**
```promql
smtp_proxy_current_connections
```

## Deployment

### Systemd Service

Full systemd service example with security hardening:

```bash
sudo tee /etc/systemd/system/smtp-edge-proxy.service > /dev/null <<EOF
[Unit]
Description=SMTP Edge Proxy
Documentation=https://github.com/yourusername/submission-proxy
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=smtp-proxy
Group=smtp-proxy
WorkingDirectory=/opt/smtp-edge-proxy
ExecStart=/usr/local/bin/smtp-edge-proxy -config /etc/smtp-edge-proxy/config.yaml
ExecReload=/bin/kill -HUP \$MAINPID
Restart=on-failure
RestartSec=5s
TimeoutStopSec=30s

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=smtp-edge-proxy

# Security hardening
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/log/smtp-edge-proxy
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

# Create user
sudo useradd -r -s /bin/false -d /opt/smtp-edge-proxy smtp-proxy

# Create directories
sudo mkdir -p /opt/smtp-edge-proxy/{certs,config}
sudo mkdir -p /var/log/smtp-edge-proxy
sudo chown -R smtp-proxy:smtp-proxy /opt/smtp-edge-proxy /var/log/smtp-edge-proxy

# Deploy files
sudo cp smtp-edge-proxy /usr/local/bin/
sudo cp config.yaml /etc/smtp-edge-proxy/
sudo cp backends.csv /etc/smtp-edge-proxy/
sudo cp certs/*.pem /opt/smtp-edge-proxy/certs/

# Start service
sudo systemctl daemon-reload
sudo systemctl enable smtp-edge-proxy
sudo systemctl start smtp-edge-proxy
```

### Docker Deployment

**Using Docker Compose:**

```yaml
version: '3.8'

services:
  smtp-edge-proxy:
    image: smtp-edge-proxy:latest
    container_name: smtp-edge-proxy
    restart: unless-stopped
    ports:
      - "587:587"
      - "465:465"
      - "8080:8080"
      - "9090:9090"
    volumes:
      - ./config.yaml:/config/config.yaml:ro
      - ./backends.csv:/backends.csv:ro
      - ./certs:/certs:ro
    environment:
      - SMTP_LOG_LEVEL=info
    healthcheck:
      test: ["CMD", "curl", "-f", "http://localhost:8080/healthz"]
      interval: 30s
      timeout: 5s
      retries: 3
      start_period: 10s
```

**Run with Docker Compose:**
```bash
docker-compose up -d
docker-compose logs -f
```

### Kubernetes Deployment

**Example deployment manifest:**

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: smtp-edge-proxy
  namespace: mail-system
spec:
  replicas: 3
  selector:
    matchLabels:
      app: smtp-edge-proxy
  template:
    metadata:
      labels:
        app: smtp-edge-proxy
    spec:
      containers:
      - name: smtp-edge-proxy
        image: smtp-edge-proxy:latest
        ports:
        - containerPort: 587
          name: submission
        - containerPort: 465
          name: smtps
        - containerPort: 8080
          name: health
        - containerPort: 9090
          name: metrics
        volumeMounts:
        - name: config
          mountPath: /config
          readOnly: true
        - name: backends
          mountPath: /backends.csv
          subPath: backends.csv
          readOnly: true
        - name: certs
          mountPath: /certs
          readOnly: true
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8080
          initialDelaySeconds: 10
          periodSeconds: 30
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8080
          initialDelaySeconds: 5
          periodSeconds: 10
        resources:
          requests:
            memory: "128Mi"
            cpu: "100m"
          limits:
            memory: "512Mi"
            cpu: "500m"
      volumes:
      - name: config
        configMap:
          name: smtp-edge-proxy-config
      - name: backends
        configMap:
          name: smtp-edge-proxy-backends
      - name: certs
        secret:
          secretName: smtp-tls-certs
---
apiVersion: v1
kind: Service
metadata:
  name: smtp-edge-proxy
  namespace: mail-system
spec:
  type: LoadBalancer
  selector:
    app: smtp-edge-proxy
  ports:
  - name: submission
    port: 587
    targetPort: 587
  - name: smtps
    port: 465
    targetPort: 465
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
# Grant capability to bind privileged ports
sudo setcap 'cap_net_bind_service=+ep' ./smtp-edge-proxy

# Verify
getcap ./smtp-edge-proxy
# Should show: ./smtp-edge-proxy = cap_net_bind_service+ep
```

#### 2. Backend Authentication Timeout

**Error in logs:**
```json
{"level":"ERROR","msg":"backend auth failed","error":"timeout"}
```

**Solutions:**

**Option 1:** Disable STARTTLS for backend (recommended workaround)
```yaml
backend:
  use_starttls: false  # Use plain connection to backend
```

**Option 2:** Increase timeout
```yaml
backend:
  timeout: 30s  # Increase from default 10s
```

**Option 3:** Use backend port 25 instead of 587
```yaml
backend:
  port: 25  # Use SMTP port instead of submission
```

#### 3. Certificate Not Found

**Error:**
```json
{"level":"WARN","msg":"no certificates found in directory","path":"certs"}
```

**Solutions:**

1. Verify certificate directory exists:
```bash
ls -la certs/
```

2. Check certificate format (must be .pem):
```bash
# Certificates should end with .pem
ls certs/*.pem
```

3. Verify certificate has both cert and key:
```bash
# Should show both BEGIN CERTIFICATE and BEGIN PRIVATE KEY
cat certs/mail.example.com.pem
```

4. Check file permissions:
```bash
chmod 644 certs/*.pem
```

#### 4. SNI Certificate Selection Not Working

**Issue:** Client always receives the same certificate regardless of SNI hostname.

**Debug:**
```bash
# Test with specific SNI hostname
openssl s_client -connect localhost:465 -servername mail.example.com -showcerts

# Check which certificate is returned
openssl s_client -connect localhost:465 -servername mail.example.com 2>/dev/null | \
  openssl x509 -noout -subject -issuer
```

**Solution:** Ensure certificate filename matches SNI hostname:
```
# Correct naming
certs/mail.example.com.pem

# Incorrect naming
certs/certificate.pem
certs/ssl-cert.pem
```

#### 5. Rate Limiting Too Aggressive

**Issue:** Legitimate users being rate limited.

**Solution:** Adjust rate limiting parameters:
```yaml
security:
  rate_limit_per_ip: 50      # Increase from default 10
  rate_limit_per_user: 200   # Increase from default 100
  rate_limit_window: 5m      # Increase window from 1m to 5m
```

Or disable rate limiting:
```yaml
security:
  rate_limit_enabled: false
```

#### 6. High Memory Usage

**Check current connections:**
```bash
curl http://localhost:9090/metrics | grep smtp_proxy_current_connections
```

**Monitor connection duration:**
```bash
./smtp-edge-proxy -config config.yaml | jq 'select(.msg=="connection closed")'
```

**Solutions:**

1. Decrease idle timeout:
```yaml
limits:
  idle_timeout: 60s  # Reduce from 300s
```

2. Implement connection limits (requires code changes - see TODOs)

3. Use OS-level limits:
```bash
ulimit -n 4096  # Limit file descriptors
```

#### 7. Backend Connection Errors

**Error:**
```json
{"level":"ERROR","msg":"failed to connect to backend","backend":"mail.example.com:587"}
```

**Debug checklist:**

1. Test backend connectivity:
```bash
telnet mail.example.com 587
```

2. Check DNS resolution:
```bash
nslookup mail.example.com
```

3. Verify firewall rules:
```bash
# Check if port is accessible
nc -zv mail.example.com 587
```

4. Check backend authentication:
```bash
# Test manual auth
openssl s_client -starttls smtp -connect mail.example.com:587
# Then:
# EHLO test
# AUTH PLAIN base64(username:password)
```

### Debug Logging

Enable debug logging for detailed troubleshooting:

```yaml
observability:
  log_level: "debug"
```

**Filter debug logs for a specific component:**
```bash
# Auth debugging
./smtp-edge-proxy -config config.yaml | jq 'select(.msg | contains("auth"))'

# Relay debugging
./smtp-edge-proxy -config config.yaml | jq 'select(.msg | contains("relay"))'

# TLS debugging
./smtp-edge-proxy -config config.yaml | jq 'select(.msg | contains("tls") or contains("cert"))'
```

### Performance Troubleshooting

**Check metrics for bottlenecks:**

```bash
# High authentication failures
curl -s http://localhost:9090/metrics | grep smtp_proxy_auth_failures_total

# High relay failures
curl -s http://localhost:9090/metrics | grep smtp_proxy_relay_failures_total

# Long connection durations
curl -s http://localhost:9090/metrics | grep smtp_proxy_connection_duration_seconds
```

**Monitor active connections:**
```bash
watch -n 5 'curl -s http://localhost:9090/metrics | grep smtp_proxy_current_connections'
```

## Architecture

### Request Flow

```
┌─────────┐         ┌──────────────────┐         ┌─────────────┐
│ Client  │────────▶│  SMTP Edge Proxy │────────▶│   Backend   │
│         │  TLS    │                  │  Auth + │   Server    │
│ :587/465│         │  • TLS Term      │  Relay  │             │
└─────────┘         │  • SNI Selection │         └─────────────┘
                    │  • Authentication│
                    │  • Rate Limiting │
                    │  • Header Inject │
                    │  • DSN Validation│
                    └──────────────────┘
```

**Step-by-step flow:**

1. **Client connects** to proxy on port 587 (STARTTLS) or 465 (implicit TLS)
2. **TLS negotiation** with SNI-based certificate selection
3. **Client sends EHLO** - proxy advertises capabilities
4. **Client authenticates** - proxy validates credentials against backend
5. **Backend connection established** and authenticated (kept open)
6. **Client sends MAIL FROM** - DSN parameters validated and stored
7. **Client sends RCPT TO** - DSN parameters validated, recipient limit checked
8. **Client sends DATA** - message received from client
9. **Proxy injects headers** (X-Original-Client-IP, X-Original-Auth-User, etc.)
10. **Message relayed** via authenticated backend connection with DSN parameters
11. **Backend response** returned to client
12. **Client disconnects** - backend connection closed

### Key Design: Authenticated Connection Reuse

The proxy uses a **single authenticated connection** per session:

- Client authenticates → proxy authenticates to backend with **user's credentials**
- Backend connection **kept open** for the entire session
- All messages in the session use the **same authenticated connection**
- Backend sees the **original user's identity**
- Backend can apply **per-user rules** (quotas, filters, policies)

**Benefits:**
- Backend authorization works correctly
- No separate relay authentication needed
- Better performance (connection reuse)
- Simplified security model
- Full audit trail

### Component Architecture

```
smtp-edge-proxy/
├── cmd/main.go              # Application entry, server setup
├── internal/
│   ├── auth/                # Backend authentication
│   │   └── auth.go          # Validates creds, returns connection
│   ├── backend/             # Per-user backend selection
│   │   └── selector.go      # CSV-based routing logic
│   ├── config/              # Configuration management
│   │   └── config.go        # YAML + env var handling
│   ├── metrics/             # Prometheus metrics
│   │   └── metrics.go       # Metric definitions
│   ├── proxy/               # SMTP protocol handler
│   │   └── backend.go       # Session management, SMTP commands
│   ├── ratelimit/           # Rate limiting
│   │   └── ratelimit.go     # Per-IP and per-user tracking
│   ├── relay/               # Message forwarding
│   │   └── relay.go         # Header injection, relay logic
│   └── tlsmgr/              # TLS certificate management
│       └── manager.go       # SNI selection, hot reload
```

### Concurrency Model

- **Goroutine-per-connection** architecture
- Each client connection runs in its own goroutine
- Non-blocking I/O for efficient resource usage
- Thread-safe components (TLS manager, rate limiter, metrics)
- No shared state between sessions

**Expected throughput:**
- Concurrent connections: 1,000+
- Authentication rate: 100-200/sec (backend limited)
- Message relay rate: 50-100/sec (backend limited)

**Scaling:** Run multiple instances behind a load balancer (HAProxy, Nginx) for higher throughput.

## Development

### Project Structure

```
.
├── cmd/
│   └── main.go                    # Application entry point
├── internal/
│   ├── auth/                      # Authentication verification
│   ├── backend/                   # Per-user backend routing
│   ├── config/                    # Configuration management
│   ├── metrics/                   # Prometheus metrics
│   ├── proxy/                     # SMTP server backend
│   ├── ratelimit/                 # Rate limiting
│   ├── relay/                     # Message relay
│   └── tlsmgr/                    # TLS/SNI certificate management
├── configs/
│   └── config.example.yaml        # Example configuration
├── certs/                         # TLS certificates (*.pem)
├── docs/                          # Documentation
│   ├── DSN-IMPLEMENTATION.md
│   ├── HEADERS.md
│   ├── LOGGING.md
│   ├── CONFIG-STATUS.md
│   ├── QUICK-START.md
│   └── PERFORMANCE-ANALYSIS.md
├── Makefile                       # Build automation
├── Dockerfile                     # Container image
├── go.mod                         # Go module definition
├── go.sum                         # Dependency checksums
├── CLAUDE.md                      # AI development guide
└── README.md                      # This file
```

### Building

```bash
# Standard build
make build

# Build with race detector (for development)
make build-race

# Cross-compile for Linux
GOOS=linux GOARCH=amd64 make build

# Build Docker image
make docker-build
```

### Testing

```bash
# Run unit tests
make test

# Run with coverage
make test-coverage

# View coverage in browser
go tool cover -html=coverage.out

# Lint code
make lint

# Run end-to-end tests
python3 test-proxy-e2e.py
```

### Code Quality

```bash
# Format code
make fmt

# Run linters
make lint

# Static analysis
go vet ./...

# Check for common issues
staticcheck ./...
```

### Dependencies

Built with:
- **Go 1.24**
- `github.com/emersion/go-smtp` - SMTP protocol library
- `github.com/fsnotify/fsnotify` - File system notifications
- `github.com/prometheus/client_golang` - Prometheus metrics
- `gopkg.in/yaml.v3` - YAML configuration
- `log/slog` - Structured logging (stdlib)

### Adding New Features

**To add a new SMTP capability:**

1. Add config field in `internal/config/config.go`:
```go
type SMTPCapabilities struct {
    NewFeature bool `yaml:"new_feature"`
    // ...
}
```

2. Set capability in `cmd/main.go`:
```go
if cfg.SMTP.Capabilities.NewFeature {
    s.EnableNewFeature = true
}
```

3. Implement handling in `internal/proxy/backend.go` if needed

**To add a new metric:**

1. Define in `internal/metrics/metrics.go`:
```go
var NewMetricCounter = promauto.NewCounter(prometheus.CounterOpts{
    Name: "smtp_proxy_new_metric_total",
    Help: "Description of new metric",
})
```

2. Instrument code where metric should be recorded:
```go
metrics.NewMetricCounter.Inc()
```

**To add a new configuration option:**

1. Add to struct in `internal/config/config.go`
2. Add to `configs/config.example.yaml`
3. Add environment variable override in `applyEnvOverrides()` (optional)
4. Update validation in `Validate()` method

## Possible Fixes/TODOs

### Known Issues

1. **Backend STARTTLS Timeout Issue**
   - **Status**: Known library limitation
   - **Issue**: The `go-smtp` library's `DialStartTLS()` has timeout handling issues
   - **Impact**: Authentication may hang 10-15 seconds then timeout
   - **Workaround**: Set `use_starttls: false` for backend connections
   - **Long-term fix**: Switch to different SMTP library or implement custom STARTTLS

2. **Idle Timeout Not Applied**
   - **Status**: Configuration bug
   - **Issue**: `idle_timeout` field exists in config but is not connected to SMTP server
   - **Impact**: Idle connections don't timeout as configured
   - **Fix needed**: Add `s.MaxIdleSeconds = int(cfg.Limits.IdleTimeout.Seconds())` in `cmd/main.go`
   - **Priority**: High

3. **Connection Limits Not Enforced**
   - **Status**: Not implemented
   - **Issue**: `max_connections` and `max_connections_per_ip` have no effect
   - **Impact**: No enforcement of connection limits
   - **Workaround**: Use OS limits (`ulimit`) or reverse proxy rate limiting
   - **Fix needed**: Implement connection tracking and rejection logic
   - **Priority**: Medium

4. **ARC Headers Not Implemented**
   - **Status**: Configuration exists but feature not implemented
   - **Issue**: `arc_enabled` config field has no effect
   - **Impact**: Low - ARC is an advanced email authentication feature
   - **Fix needed**: Implement ARC header generation and DKIM signing integration
   - **Priority**: Low

### Bugs to Fix

1. **Fix idle timeout not being applied**
   ```go
   // In cmd/main.go, add after WriteTimeout line:
   s.MaxIdleSeconds = int(cfg.Limits.IdleTimeout.Seconds())
   ```

2. **Add connection limit enforcement**
   - Implement atomic counter for active connections
   - Check against `max_connections` before accepting new connection
   - Implement per-IP connection tracking for `max_connections_per_ip`

3. **Improve error handling for backend connection failures**
   - Add circuit breaker pattern for failing backends
   - Implement exponential backoff for retries
   - Better error messages to clients

4. **Backend connection pool not implemented**
   - Config has `pool_enabled: false` but feature doesn't exist
   - Would significantly improve performance under load
   - Requires connection lifecycle management

### Testing Gaps

1. **Add unit tests for:**
   - Rate limiter edge cases
   - DSN parameter validation
   - Backend selector priority logic
   - TLS certificate hot reload

2. **Add integration tests for:**
   - Multiple concurrent connections
   - Backend failure scenarios
   - Rate limit enforcement
   - SNI certificate selection

3. **Add load tests for:**
   - Sustained message throughput
   - Connection pool exhaustion
   - Memory usage under load

## Enhancement Suggestions

### High Priority

1. **Connection Pooling to Backend**
   - Reuse backend connections across sessions
   - Significant performance improvement
   - Reduces backend connection count
   - Implementation complexity: High

2. **Circuit Breaker for Backend Failures**
   - Stop sending requests to failing backends
   - Automatic recovery attempts
   - Better error handling for clients
   - Implementation complexity: Medium

3. **Health Check for Backends**
   - Periodic backend connectivity checks
   - Mark unhealthy backends as down
   - Automatic failover to healthy backends
   - Implementation complexity: Medium

4. **Configuration Reload Without Restart**
   - Hot reload of config.yaml (like certificate reload)
   - Update backends.csv without restart
   - Zero-downtime configuration changes
   - Implementation complexity: Medium

### Medium Priority

5. **Redis-based Rate Limiting**
   - Distributed rate limiting across multiple instances
   - Persistent rate limit state
   - Better accuracy under load
   - Implementation complexity: Medium

6. **Message Queue for Offline Backend**
   - Queue messages when backend is unavailable
   - Retry delivery automatically
   - Reduces client-visible failures
   - Implementation complexity: High

7. **Per-Backend Metrics**
   - Track auth/relay success per backend
   - Monitor backend latency
   - Identify problematic backends
   - Implementation complexity: Low

8. **DKIM Signing**
   - Sign outgoing messages with DKIM
   - Improve email deliverability
   - Per-domain key management
   - Implementation complexity: High

9. **Greylisting Support**
   - Implement greylisting for spam reduction
   - Configurable greylist duration
   - Whitelist for known good senders
   - Implementation complexity: Medium

10. **Webhook Notifications**
    - Send webhooks for specific events
    - Failed authentications, rate limit hits, etc.
    - Integration with monitoring systems
    - Implementation complexity: Low

### Low Priority

11. **Web UI for Configuration**
    - Web-based configuration editor
    - Real-time metrics dashboard
    - Backend routing management
    - Implementation complexity: High

12. **LDAP/Active Directory Integration**
    - Alternative authentication backend
    - Support for corporate directories
    - Group-based routing rules
    - Implementation complexity: High

13. **Message Filtering Rules**
    - Content-based routing
    - Block/allow lists for recipients
    - Size-based routing
    - Implementation complexity: Medium

14. **OpenTelemetry Tracing**
    - Full distributed tracing support
    - Integration with Jaeger/Zipkin
    - Request flow visualization
    - Implementation complexity: Medium

15. **IPv6 Support Improvements**
    - Better IPv6 address handling
    - IPv6-specific rate limiting
    - IPv6 prefix matching in IP filters
    - Implementation complexity: Low

### Performance Optimizations

16. **Worker Pool Architecture**
    - Alternative to goroutine-per-connection
    - Better for >10,000 concurrent connections
    - Lower memory footprint
    - Implementation complexity: High

17. **Async Logging**
    - Non-blocking log writes
    - Buffered log output
    - Reduces latency impact
    - Implementation complexity: Medium

18. **Zero-Copy Message Streaming**
    - Reduce memory allocations during relay
    - Direct streaming from client to backend
    - Lower memory usage
    - Implementation complexity: High

## License

Copyright © 2025 InteSys/EmailProfissional

## Support

For issues, questions, and development guidance:
- Check the [documentation](docs/)
- Review [CLAUDE.md](CLAUDE.md) for development guide
- Check logs with JSON filtering: `./smtp-edge-proxy | jq 'select(.level=="ERROR")'`
- Enable debug logging: Set `observability.log_level: "debug"` in config

## Contributing

Contributions are welcome! Please:
1. Fork the repository
2. Create a feature branch
3. Write tests for new functionality
4. Ensure all tests pass: `make test`
5. Run linters: `make lint`
6. Submit a pull request

## Acknowledgments

Built with:
- [go-smtp](https://github.com/emersion/go-smtp) by Simon Ser
- [fsnotify](https://github.com/fsnotify/fsnotify)
- [Prometheus Go client](https://github.com/prometheus/client_golang)
