# DSN (Delivery Status Notification) Implementation

## Summary

The SMTP Edge Proxy now fully supports DSN (RFC 3461) parameter forwarding from clients to backend servers.

## Features Implemented

### 1. DSN Parameter Storage
- Added `mailOpts *smtp.MailOptions` to store MAIL FROM DSN options
- Added `rcptOpts []*smtp.RcptOptions` to store RCPT TO DSN options (one per recipient)

### 2. DSN Validation
- **MAIL FROM validation**:
  - `RET` parameter: Must be "FULL" or "HDRS" (if present)
  - `ENVID` parameter: Any value accepted (envelope identifier)

- **RCPT TO validation**:
  - `NOTIFY` parameter: Must be "NEVER", "SUCCESS", "FAILURE", or "DELAY"
  - Validates that "NEVER" cannot be combined with other values
  - `ORCPT` parameter: Any value accepted (original recipient)

### 3. DSN Logging
All DSN parameters are logged when received and when forwarded:

**When received from client:**
```json
{"msg":"MAIL FROM options","dsn_ret":"FULL","dsn_envid":"test-123"}
{"msg":"RCPT TO options","recipient":"user@example.com","dsn_notify":"SUCCESS,FAILURE"}
```

**When forwarded to backend:**
```json
{"msg":"forwarding MAIL FROM DSN options to backend","from":"sender@example.com","dsn_ret":"FULL","dsn_envid":"test-123"}
{"msg":"forwarding RCPT TO DSN options to backend","recipient":"user@example.com","dsn_notify":"SUCCESS,FAILURE"}
```

### 4. DSN Forwarding
All DSN parameters from the client are forwarded to the backend server using the same authenticated connection.

## Configuration

DSN support is controlled by the `dsn` setting in `config.yaml`:

```yaml
smtp:
  capabilities:
    dsn: true  # Enable DSN (RFC 3461) support
```

## Testing

### Test Command
```bash
python3 test-dsn-final.py
```

### Expected Results
1. ✅ DSN extension advertised in EHLO
2. ✅ MAIL FROM with RET parameter accepted
3. ✅ RCPT TO with NOTIFY parameters accepted
4. ✅ DSN parameters logged when received
5. ✅ DSN parameters forwarded to backend
6. ✅ Message delivered successfully

### Log Verification
Check `/tmp/proxy-dsn.log` for:
- `"MAIL FROM options"` with DSN parameters
- `"RCPT TO options"` with DSN parameters
- `"forwarding MAIL FROM DSN options to backend"`
- `"forwarding RCPT TO DSN options to backend"`

## Supported DSN Parameters

### MAIL FROM Parameters
- **RET=FULL**: Request full message in bounce
- **RET=HDRS**: Request only headers in bounce
- **ENVID=<string>**: Envelope identifier for tracking

### RCPT TO Parameters
- **NOTIFY=NEVER**: Never send notification
- **NOTIFY=SUCCESS**: Notify on successful delivery
- **NOTIFY=FAILURE**: Notify on delivery failure
- **NOTIFY=DELAY**: Notify on delivery delay
- **ORCPT=<address>**: Original recipient address

## Files Modified

- `internal/proxy/backend.go`: Added DSN storage, validation, and logging
- `internal/relay/relay.go`: Added DSN forwarding to backend
- `cmd/main.go`: Added `EnableDSN` configuration (line 206)

## Validation Examples

### Valid DSN Commands
```
MAIL FROM:<sender@example.com> RET=FULL ENVID=abc123
RCPT TO:<user@example.com> NOTIFY=SUCCESS,FAILURE
RCPT TO:<user2@example.com> NOTIFY=NEVER
```

### Invalid DSN Commands (Will be rejected)
```
MAIL FROM:<sender@example.com> RET=INVALID  # Invalid RET value
RCPT TO:<user@example.com> NOTIFY=INVALID   # Invalid NOTIFY value
RCPT TO:<user@example.com> NOTIFY=NEVER,SUCCESS  # NEVER cannot be combined
```

## Error Handling

Invalid DSN parameters return SMTP error:
```
501 5.5.4 Invalid parameter: <error description>
```

## Performance Impact

Minimal - DSN parameters are only processed when present. No impact on messages without DSN parameters.
