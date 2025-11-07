#!/bin/bash

# Test email sending through the SMTP proxy
# Usage: ./test-email.sh

set -e

echo "Testing SMTP Edge Proxy..."
echo

# Test connection and STARTTLS
echo "1. Testing STARTTLS connection..."
(echo "EHLO test.local"; sleep 1; echo "STARTTLS"; sleep 1; echo "QUIT") | \
  openssl s_client -starttls smtp -connect 127.0.0.1:2587 -servername mail.intesys.io 2>&1 | grep -E "(EHLO|STARTTLS|250|220)" | head -10
echo

# Send actual email
echo "2. Sending test email..."
python3 - <<'EOF'
import smtplib
from email.mime.text import MIMEText
import ssl

# Email configuration
SMTP_SERVER = "172.23.20.40"
SMTP_PORT = 587
FROM_EMAIL = "joao@intesys.io"
TO_EMAIL = "joao@intesys.com.br"
PASSWORD = "GJPKPSDSJXSHFTOM"

# Create message
msg = MIMEText("This is a test email sent through the SMTP edge proxy.")
msg['Subject'] = 'SMTP Proxy Test Email'
msg['From'] = FROM_EMAIL
msg['To'] = TO_EMAIL

# Send email
try:
    context = ssl.create_default_context()
    with smtplib.SMTP(SMTP_SERVER, SMTP_PORT) as server:
        server.set_debuglevel(1)
        server.starttls(context=context)
        server.login(FROM_EMAIL, PASSWORD)
        server.send_message(msg)
        print("\n✓ Email sent successfully!")
except Exception as e:
    print(f"\n✗ Failed to send email: {e}")
    exit(1)
EOF

echo
echo "✓ All tests passed!"
