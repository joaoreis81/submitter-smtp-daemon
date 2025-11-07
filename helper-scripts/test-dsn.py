#!/usr/bin/env python3
"""
Test DSN extension forwarding to backend
"""

import smtplib
import ssl
from email.mime.text import MIMEText
from datetime import datetime

PROXY_HOST = 'localhost'
PROXY_PORT = 587
FROM_EMAIL = 'joao@intesys.io'
TO_EMAIL = 'joao@intesys.com.br'
PASSWORD = 'GJPKPSDSJXSHFTOM'

def test_dsn_support():
    """Test if DSN extension is advertised and forwarded"""
    print("Testing DSN extension support...\n")

    context = ssl.create_default_context()
    context.check_hostname = False
    context.verify_mode = ssl.CERT_NONE

    try:
        with smtplib.SMTP(PROXY_HOST, PROXY_PORT, timeout=15) as server:
            # Check if DSN is advertised
            resp = server.ehlo()
            ehlo_response = resp[1].decode() if isinstance(resp[1], bytes) else str(resp[1])

            dsn_supported = False
            print("EHLO capabilities:")
            for line in ehlo_response.split('\n'):
                print(f"  {line}")
                if 'DSN' in line:
                    dsn_supported = True

            if not dsn_supported:
                print("\n✗ FAIL: DSN extension not advertised")
                return False

            print("\n✓ DSN extension is advertised")

            server.starttls(context=context)
            server.ehlo()
            server.login(FROM_EMAIL, PASSWORD)

            # Try to send with DSN parameters
            # Python's smtplib doesn't directly support DSN parameters in send_message,
            # so we'll use the lower-level mail() and rcpt() methods

            print("\nAttempting to send email with DSN parameters...")

            timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            msg = MIMEText(f'DSN test at {timestamp}')
            msg['Subject'] = f'DSN Test - {timestamp}'
            msg['From'] = FROM_EMAIL
            msg['To'] = TO_EMAIL

            # Send MAIL FROM with RET parameter
            try:
                server.docmd('MAIL FROM:<{}> RET=FULL'.format(FROM_EMAIL))
                print("  ✓ MAIL FROM with RET=FULL accepted")
            except smtplib.SMTPResponseException as e:
                print(f"  ✗ MAIL FROM with DSN parameters rejected: {e}")
                return False

            # Send RCPT TO with NOTIFY parameter
            try:
                server.docmd('RCPT TO:<{}> NOTIFY=SUCCESS,FAILURE,DELAY'.format(TO_EMAIL))
                print("  ✓ RCPT TO with NOTIFY parameters accepted")
            except smtplib.SMTPResponseException as e:
                print(f"  ✗ RCPT TO with DSN parameters rejected: {e}")
                return False

            # Send the message data
            try:
                server.docmd('DATA')
                server.docmd(msg.as_string() + '\r\n.')
                print("  ✓ Message sent successfully with DSN parameters")
                return True
            except smtplib.SMTPResponseException as e:
                print(f"  ✗ DATA command failed: {e}")
                return False

    except Exception as e:
        print(f"? ERROR: Unexpected exception: {type(e).__name__}: {e}")
        import traceback
        traceback.print_exc()
        return False

if __name__ == '__main__':
    print("=" * 60)
    print("DSN EXTENSION FORWARDING TEST")
    print("=" * 60 + "\n")

    result = test_dsn_support()

    print("\n" + "=" * 60)
    if result:
        print("RESULT: DSN extension is supported ✓")
        print("\nNote: This test verifies that DSN parameters are accepted.")
        print("Check the backend server to confirm DSN parameters are forwarded.")
    else:
        print("RESULT: DSN extension has issues ✗")
    print("=" * 60)
