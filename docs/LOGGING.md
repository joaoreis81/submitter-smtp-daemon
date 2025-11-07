# Logging Improvements

## Overview

All logs now include consistent `request_id` tracking and detailed connection information for debugging.

## Request ID Tracking

Every log entry in a transaction includes the same `request_id`:

```json
{
  "request_id": "5899490e-66fc-4d4d-a0b3-3d15db65aebd",
  "msg": "new session",
  ...
}
{
  "request_id": "5899490e-66fc-4d4d-a0b3-3d15db65aebd",
  "msg": "authentication attempt",
  ...
}
{
  "request_id": "5899490e-66fc-4d4d-a0b3-3d15db65aebd",
  "msg": "backend authentication successful",
  ...
}
{
  "request_id": "5899490e-66fc-4d4d-a0b3-3d15db65aebd",
  "msg": "relaying message",
  ...
}
```

## Connection Details

### Client Connection

```json
{
  "remote_addr": "[::1]:42542",     // Full address with port
  "client_ip": "::1",                // IP only (for filtering)
  "client_addr": "::1"               // Used in auth/relay logs
}
```

### Backend Connection

```json
{
  "backend": "172.23.20.40:587"     // Backend server with port
}
```

## TLS/Encryption Details

### On Session Start (Implicit TLS)

```json
{
  "implicit_tls": true,
  "tls_version": "TLS1.3",
  "tls_cipher": "TLS_AES_128_GCM_SHA256",
  "tls_server_name": "localhost"
}
```

### On Message Relay

```json
{
  "tls_type": "SSL"           // or "STARTTLS" or "false"
}
```

## Complete Transaction Example

```json
{"time":"2025-10-07T13:40:02.305986098-03:00","level":"INFO","msg":"new session","request_id":"5899490e-66fc-4d4d-a0b3-3d15db65aebd","remote_addr":"[::1]:42542","client_ip":"::1","implicit_tls":true,"tls_version":"TLS1.3","tls_cipher":"TLS_AES_128_GCM_SHA256","tls_server_name":"localhost"}

{"time":"2025-10-07T13:40:02.307458974-03:00","level":"INFO","msg":"authentication attempt","request_id":"5899490e-66fc-4d4d-a0b3-3d15db65aebd","username":"joao@intesys.io","ip":"::1"}

{"time":"2025-10-07T13:40:02.307620504-03:00","level":"INFO","msg":"starting backend authentication with persistent connection","request_id":"5899490e-66fc-4d4d-a0b3-3d15db65aebd","username":"joao@intesys.io","client_addr":"::1","backend":"172.23.20.40:587","use_starttls":false}

{"time":"2025-10-07T13:40:02.360796369-03:00","level":"INFO","msg":"connected to auth backend","request_id":"5899490e-66fc-4d4d-a0b3-3d15db65aebd","backend":"172.23.20.40:587","dial_duration":53080003}

{"time":"2025-10-07T13:40:02.547213098-03:00","level":"INFO","msg":"backend authentication successful, keeping connection for relay","request_id":"5899490e-66fc-4d4d-a0b3-3d15db65aebd","username":"joao@intesys.io","client_addr":"::1","backend":"172.23.20.40:587","auth_duration":121045104,"total_duration":239584051}

{"time":"2025-10-07T13:40:02.547328408-03:00","level":"INFO","msg":"session authenticated with backend connection established","request_id":"5899490e-66fc-4d4d-a0b3-3d15db65aebd","auth_user":"joao@intesys.io"}

{"time":"2025-10-07T13:40:02.550619804-03:00","level":"INFO","msg":"MAIL FROM","request_id":"5899490e-66fc-4d4d-a0b3-3d15db65aebd","auth_user":"joao@intesys.io","from":"joao@intesys.io"}

{"time":"2025-10-07T13:40:02.551645153-03:00","level":"INFO","msg":"RCPT TO","request_id":"5899490e-66fc-4d4d-a0b3-3d15db65aebd","auth_user":"joao@intesys.io","to":"joao@intesys.com.br"}

{"time":"2025-10-07T13:40:02.552598082-03:00","level":"INFO","msg":"DATA","request_id":"5899490e-66fc-4d4d-a0b3-3d15db65aebd","auth_user":"joao@intesys.io","from":"joao@intesys.io","recipients":1}

{"time":"2025-10-07T13:40:02.55280245-03:00","level":"INFO","msg":"relaying message via authenticated backend connection","request_id":"5899490e-66fc-4d4d-a0b3-3d15db65aebd","from":"joao@intesys.io","recipients":1,"backend":"172.23.20.40:25","auth_user":"joao@intesys.io","client_addr":"::1","tls_type":"SSL"}

{"time":"2025-10-07T13:40:02.73666963-03:00","level":"INFO","msg":"message relayed successfully via authenticated connection","request_id":"5899490e-66fc-4d4d-a0b3-3d15db65aebd","from":"joao@intesys.io","recipients":1,"bytes":242,"auth_user":"joao@intesys.io"}

{"time":"2025-10-07T13:40:02.737710762-03:00","level":"INFO","msg":"session logout","request_id":"5899490e-66fc-4d4d-a0b3-3d15db65aebd","auth_user":"joao@intesys.io"}
```

## Filtering by Request ID

To trace a specific transaction:

```bash
./smtp-edge-proxy -config config.yaml | jq 'select(.request_id=="5899490e-66fc-4d4d-a0b3-3d15db65aebd")'
```

## TLS Version Values

- `TLS1.0` - TLS 1.0 (deprecated)
- `TLS1.1` - TLS 1.1 (deprecated)
- `TLS1.2` - TLS 1.2
- `TLS1.3` - TLS 1.3 (recommended)

## TLS Cipher Examples

- `TLS_AES_128_GCM_SHA256` - TLS 1.3 cipher
- `TLS_AES_256_GCM_SHA384` - TLS 1.3 cipher
- `TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256` - TLS 1.2 cipher
- `TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384` - TLS 1.2 cipher

## Connection Type Values

- `SSL` - Implicit TLS (port 465, SMTPS)
- `STARTTLS` - Upgraded via STARTTLS (port 587)
- `false` - No encryption (plaintext)
