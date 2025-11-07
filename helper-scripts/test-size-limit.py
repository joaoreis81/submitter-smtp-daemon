#!/usr/bin/env python3
"""
Test SIZE limit enforcement
"""

import smtplib
import ssl
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart
from datetime import datetime

PROXY_HOST = 'localhost'
PROXY_PORT = 587
FROM_EMAIL = 'joao@intesys.io'
TO_EMAIL = 'joao@intesys.com.br'
PASSWORD = 'GJPKPSDSJXSHFTOM'

def test_size_limit():
    """Test if messages exceeding SIZE limit are rejected"""
    print("Testing SIZE limit enforcement (1MB limit)...\n")

    context = ssl.create_default_context()
    context.check_hostname = False
    context.verify_mode = ssl.CERT_NONE

    # Create a message larger than 1MB (let's try 2MB)
    large_body = "A" * (2 * 1024 * 1024)  # 2MB of 'A' characters

    timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
    msg = MIMEText(large_body)
    msg['Subject'] = f'SIZE Test - Large Message - {timestamp}'
    msg['From'] = FROM_EMAIL
    msg['To'] = TO_EMAIL

    try:
        with smtplib.SMTP(PROXY_HOST, PROXY_PORT, timeout=15) as server:
            # Check advertised SIZE limit
            resp = server.ehlo()
            ehlo_response = resp[1].decode() if isinstance(resp[1], bytes) else str(resp[1])
            print("EHLO capabilities:")
            for line in ehlo_response.split('\n'):
                if 'SIZE' in line:
                    print(f"  {line}")

            server.starttls(context=context)
            server.ehlo()
            server.login(FROM_EMAIL, PASSWORD)

            print(f"\nAttempting to send 2MB message (limit is 1MB)...")
            result = server.send_message(msg)

            if not result:
                print("✗ FAIL: Message was accepted (should have been rejected!)")
                return False
            else:
                print(f"✗ FAIL: Unexpected result: {result}")
                return False

    except smtplib.SMTPDataError as e:
        print(f"✓ PASS: Message rejected with DATA error: {e}")
        return True
    except smtplib.SMTPResponseException as e:
        print(f"✓ PASS: Message rejected: {e}")
        return True
    except Exception as e:
        print(f"? ERROR: Unexpected exception: {type(e).__name__}: {e}")
        return False

if __name__ == '__main__':
    print("=" * 60)
    print("SIZE LIMIT ENFORCEMENT TEST")
    print("=" * 60 + "\n")

    result = test_size_limit()

    print("\n" + "=" * 60)
    if result:
        print("RESULT: SIZE limit is enforced ✓")
    else:
        print("RESULT: SIZE limit is NOT enforced ✗")
    print("=" * 60)
