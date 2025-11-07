# Message Headers Added by SMTP Edge Proxy

The proxy adds the following headers to all relayed messages:

## Connection Metadata Headers

### X-Original-Client-IP
The IP address of the client that connected to the proxy.

**Example:**
```
X-Original-Client-IP: 192.168.1.100
```

### X-Original-Auth-User
The username used for SMTP authentication.

**Example:**
```
X-Original-Auth-User: user@example.com
```

### X-Original-Server-Name
The server name (hostname) from the EHLO/HELO command.

**Example:**
```
X-Original-Server-Name: mail.example.com
```

### X-Original-EHLO
The EHLO hostname provided by the client (if available).

**Example:**
```
X-Original-EHLO: mail.example.com
```

## Security Headers

### X-Edge-Received-TLS
Boolean indicating whether TLS was used for the client connection.

**Values:**
- `true` - Connection used TLS encryption
- `false` - Connection was plaintext

**Example:**
```
X-Edge-Received-TLS: true
```

### X-Secure-Conn
Indicates the type of TLS encryption used for the client connection.

**Values:**
- `STARTTLS` - Connection upgraded to TLS via STARTTLS command (port 587)
- `SSL` - Connection used implicit TLS from the start (port 465, SMTPS)
- `false` - No encryption was used

**Examples:**

Port 587 with STARTTLS:
```
X-Secure-Conn: STARTTLS
```

Port 465 with implicit TLS:
```
X-Secure-Conn: SSL
```

Plaintext connection:
```
X-Secure-Conn: false
```

## Complete Example

Here's what the headers look like for a message sent via STARTTLS on port 587:

```
X-Original-Client-IP: 192.168.1.100
X-Original-Auth-User: joao@intesys.io
X-Original-Server-Name: mail.intesys.io
X-Edge-Received-TLS: true
X-Secure-Conn: STARTTLS
X-Original-EHLO: mail.intesys.io
```

And for a message sent via implicit TLS on port 465:

```
X-Original-Client-IP: 192.168.1.100
X-Original-Auth-User: joao@intesys.io
X-Original-Server-Name: mail.intesys.io
X-Edge-Received-TLS: true
X-Secure-Conn: SSL
X-Original-EHLO: mail.intesys.io
```

## Use Cases

These headers allow the backend mail server to:

1. **Track original client identity** - Know which IP address initiated the connection
2. **Audit authentication** - See which user sent the message
3. **Enforce security policies** - Verify TLS was used for the edge connection
4. **Distinguish connection types** - Differentiate between STARTTLS and implicit TLS
5. **Debugging** - Trace message path through the proxy infrastructure
