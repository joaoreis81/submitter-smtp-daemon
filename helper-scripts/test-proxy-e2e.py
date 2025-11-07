#!/usr/bin/env python3
"""
End-to-end test for SMTP Edge Proxy
Tests authentication and email sending through the proxy
"""

import smtplib
import ssl
import sys
from email.mime.text import MIMEText
from datetime import datetime

# Test configuration
PROXY_HOST = 'localhost'
PROXY_PORT = 587
FROM_EMAIL = 'joao@intesys.io'
TO_EMAIL = 'joao@intesys.com.br'
PASSWORD = 'GJPKPSDSJXSHFTOM'

def test_connection():
    """Test basic connection to proxy"""
    print("=" * 60)
    print("TEST 1: Basic Connection")
    print("=" * 60)
    try:
        with smtplib.SMTP(PROXY_HOST, PROXY_PORT, timeout=10) as server:
            code, msg = server.ehlo()
            print(f"✓ Connected successfully")
            print(f"  EHLO response: {code} {msg.decode()[:100]}")
            return True
    except Exception as e:
        print(f"✗ Connection failed: {e}")
        return False

def test_starttls():
    """Test STARTTLS upgrade"""
    print("\n" + "=" * 60)
    print("TEST 2: STARTTLS")
    print("=" * 60)
    try:
        context = ssl.create_default_context()
        context.check_hostname = False
        context.verify_mode = ssl.CERT_NONE

        with smtplib.SMTP(PROXY_HOST, PROXY_PORT, timeout=10) as server:
            server.ehlo()
            if server.has_extn('STARTTLS'):
                print(f"✓ STARTTLS extension advertised")
                server.starttls(context=context)
                print(f"✓ TLS negotiation successful")
                server.ehlo()  # Re-identify after STARTTLS
                return True
            else:
                print(f"✗ STARTTLS not advertised")
                return False
    except Exception as e:
        print(f"✗ STARTTLS failed: {e}")
        return False

def test_authentication():
    """Test authentication"""
    print("\n" + "=" * 60)
    print("TEST 3: Authentication")
    print("=" * 60)
    try:
        context = ssl.create_default_context()
        context.check_hostname = False
        context.verify_mode = ssl.CERT_NONE

        with smtplib.SMTP(PROXY_HOST, PROXY_PORT, timeout=10) as server:
            server.ehlo()
            server.starttls(context=context)
            server.ehlo()

            # Check AUTH methods
            if server.has_extn('AUTH'):
                auth_methods = server.esmtp_features.get('auth', '')
                print(f"✓ AUTH extension advertised: {auth_methods}")

            # Attempt login
            server.login(FROM_EMAIL, PASSWORD)
            print(f"✓ Authentication successful for {FROM_EMAIL}")
            return True
    except smtplib.SMTPAuthenticationError as e:
        print(f"✗ Authentication failed: {e}")
        return False
    except Exception as e:
        print(f"✗ Error during authentication: {e}")
        import traceback
        traceback.print_exc()
        return False

def test_send_email():
    """Test complete email sending"""
    print("\n" + "=" * 60)
    print("TEST 4: Send Email")
    print("=" * 60)
    try:
        context = ssl.create_default_context()
        context.check_hostname = False
        context.verify_mode = ssl.CERT_NONE

        # Create message
        timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
        msg = MIMEText(f'This is a test email sent through the SMTP Edge Proxy at {timestamp}')
        msg['Subject'] = f'SMTP Proxy Test - {timestamp}'
        msg['From'] = FROM_EMAIL
        msg['To'] = TO_EMAIL

        with smtplib.SMTP(PROXY_HOST, PROXY_PORT, timeout=15) as server:
            # Enable debug output
            # server.set_debuglevel(1)

            server.ehlo()
            server.starttls(context=context)
            server.ehlo()
            server.login(FROM_EMAIL, PASSWORD)

            # Send message
            result = server.send_message(msg)

            if result:
                print(f"✗ Some recipients were rejected:")
                for recipient, (code, message) in result.items():
                    print(f"  {recipient}: {code} {message}")
                return False
            else:
                print(f"✓ Email sent successfully")
                print(f"  From: {FROM_EMAIL}")
                print(f"  To: {TO_EMAIL}")
                print(f"  Subject: {msg['Subject']}")
                return True

    except Exception as e:
        print(f"✗ Failed to send email: {e}")
        import traceback
        traceback.print_exc()
        return False

def test_capabilities():
    """Test SMTP capabilities"""
    print("\n" + "=" * 60)
    print("TEST 5: SMTP Capabilities")
    print("=" * 60)
    try:
        context = ssl.create_default_context()
        context.check_hostname = False
        context.verify_mode = ssl.CERT_NONE

        with smtplib.SMTP(PROXY_HOST, PROXY_PORT, timeout=10) as server:
            server.ehlo()
            server.starttls(context=context)
            server.ehlo()

            print("✓ Capabilities after STARTTLS:")
            for key, value in server.esmtp_features.items():
                print(f"  {key.upper()}: {value if value else '(enabled)'}")

            return True
    except Exception as e:
        print(f"✗ Failed to check capabilities: {e}")
        return False

def main():
    """Run all tests"""
    print("\n")
    print("╔" + "=" * 58 + "╗")
    print("║" + " " * 10 + "SMTP EDGE PROXY - E2E TESTS" + " " * 20 + "║")
    print("╚" + "=" * 58 + "╝")
    print()

    results = {}

    # Run tests
    results['connection'] = test_connection()
    results['starttls'] = test_starttls()
    results['authentication'] = test_authentication()
    results['capabilities'] = test_capabilities()
    results['send_email'] = test_send_email()

    # Summary
    print("\n" + "=" * 60)
    print("TEST SUMMARY")
    print("=" * 60)

    passed = sum(1 for v in results.values() if v)
    total = len(results)

    for test_name, passed_test in results.items():
        status = "✓ PASS" if passed_test else "✗ FAIL"
        print(f"{status} - {test_name.replace('_', ' ').title()}")

    print()
    print(f"Results: {passed}/{total} tests passed")

    if passed == total:
        print("\n✓ All tests PASSED!")
        return 0
    else:
        print(f"\n✗ {total - passed} test(s) FAILED")
        return 1

if __name__ == '__main__':
    sys.exit(main())
