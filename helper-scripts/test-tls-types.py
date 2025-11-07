#!/usr/bin/env python3
"""
Test TLS type detection for both STARTTLS and implicit TLS
"""

import smtplib
import ssl
from email.mime.text import MIMEText
from datetime import datetime

PROXY_HOST = 'localhost'
FROM_EMAIL = 'joao@intesys.io'
TO_EMAIL = 'joao@intesys.com.br'
PASSWORD = 'GJPKPSDSJXSHFTOM'

def test_starttls():
    """Test STARTTLS on port 587"""
    print("Testing STARTTLS (port 587)...")
    context = ssl.create_default_context()
    context.check_hostname = False
    context.verify_mode = ssl.CERT_NONE

    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    msg = MIMEText(f'Testing STARTTLS at {timestamp}')
    msg['Subject'] = f'STARTTLS Test - {timestamp}'
    msg['From'] = FROM_EMAIL
    msg['To'] = TO_EMAIL

    with smtplib.SMTP(PROXY_HOST, 587, timeout=15) as server:
        server.ehlo()
        server.starttls(context=context)
        server.ehlo()
        server.login(FROM_EMAIL, PASSWORD)
        result = server.send_message(msg)

        if not result:
            print("✓ STARTTLS email sent successfully")
            return True
        else:
            print(f"✗ STARTTLS failed: {result}")
            return False

def test_implicit_tls():
    """Test implicit TLS on port 465"""
    print("Testing Implicit TLS (port 465)...")
    context = ssl.create_default_context()
    context.check_hostname = False
    context.verify_mode = ssl.CERT_NONE

    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    msg = MIMEText(f'Testing Implicit TLS at {timestamp}')
    msg['Subject'] = f'Implicit TLS Test - {timestamp}'
    msg['From'] = FROM_EMAIL
    msg['To'] = TO_EMAIL

    with smtplib.SMTP_SSL(PROXY_HOST, 465, timeout=15, context=context) as server:
        server.ehlo()
        server.login(FROM_EMAIL, PASSWORD)
        result = server.send_message(msg)

        if not result:
            print("✓ Implicit TLS email sent successfully")
            return True
        else:
            print(f"✗ Implicit TLS failed: {result}")
            return False

if __name__ == '__main__':
    print("\n=== TLS Type Detection Test ===\n")

    starttls_ok = test_starttls()
    print()
    implicit_ok = test_implicit_tls()

    print("\n=== Results ===")
    print(f"STARTTLS (587): {'PASS' if starttls_ok else 'FAIL'}")
    print(f"Implicit TLS (465): {'PASS' if implicit_ok else 'FAIL'}")
    print("\nCheck the received emails for X-Secure-Conn header:")
    print("  - Port 587 should have: X-Secure-Conn: STARTTLS")
    print("  - Port 465 should have: X-Secure-Conn: SSL")
