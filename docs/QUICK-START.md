# SMTP Edge Proxy - Quick Start Guide

## Prerequisites

- Go 1.24+
- TLS certificates in .pem format (in `certs/` directory)
- Access to backend SMTP server

## 1. Configure Backend Servers

Edit `config.yaml`:

```yaml
backend:
  host: "172.23.20.40"      # Your SMTP server
  port: 587                  # Authentication port
  use_starttls: true

relay:
  host: "172.23.20.40"       # Your SMTP server
  port: 25                    # Relay port
  use_starttls: false
```

## 2. Build

```bash
make build
# or
go build -o smtp-edge-proxy ./cmd
```

## 3. Grant Port Permissions

Required for ports 587 and 465:

```bash
sudo setcap 'cap_net_bind_service=+ep' ./smtp-edge-proxy
```

## 4. Start Proxy

```bash
./smtp-edge-proxy -config config.yaml
```

## 5. Test

```bash
# Test connection
printf "EHLO test\r\nQUIT\r\n" | nc localhost 587

# Test AUTH after STARTTLS
python3 << 'EOF'
import smtplib, ssl
s = smtplib.SMTP('localhost', 587)
s.starttls(context=ssl._create_unverified_context())
s.ehlo()
print("AUTH supported:", s.has_extn('AUTH'))
print("Methods:", s.esmtp_features.get('auth', ''))
EOF
```

## 6. Monitor Logs

Logs are JSON formatted to stdout:

```bash
./smtp-edge-proxy -config config.yaml | jq -r '"\(.time) [\(.level)] \(.msg)"'
```

## Health Checks

```bash
# Liveness
curl http://localhost:8080/healthz

# Readiness
curl http://localhost:8080/readyz

# Metrics
curl http://localhost:9090/metrics
```

## Common Issues

### Port Permission Denied
```bash
sudo setcap 'cap_net_bind_service=+ep' ./smtp-edge-proxy
```

### Backend Timeout
Increase timeout in `config.yaml`:
```yaml
backend:
  timeout: 30s
```

### Certificate Not Found
Place certificates in `certs/` directory with `.pem` extension:
```bash
ls certs/
# mail.intesys.io.pem
# mail.example.com.pem
```

## Documentation

- `README.md` - Complete documentation
- `docs/BACKEND-CONFIG.md` - Backend configuration guide
- `TEST-RESULTS.md` - Test results
- `CLAUDE.md` - Development guide

## Production Deployment

### SystemD Service

```bash
sudo cp systemd/smtp-edge-proxy.service /etc/systemd/system/
sudo systemctl enable smtp-edge-proxy
sudo systemctl start smtp-edge-proxy
sudo systemctl status smtp-edge-proxy
```

### Docker

```bash
make docker-build
make docker-run
```

## Support

For issues: Check logs with JSON parsing:

```bash
./smtp-edge-proxy -config config.yaml 2>&1 | jq 'select(.level=="ERROR")'
```

Set log level to debug in `config.yaml`:

```yaml
observability:
  log_level: "debug"
```
