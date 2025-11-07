#!/usr/bin/env python3
"""
Test RCPTMAX limit enforcement
"""

import smtplib
import ssl
from datetime import datetime

PROXY_HOST = 'localhost'
PROXY_PORT = 587
FROM_EMAIL = 'joao@intesys.io'
TO_EMAIL = 'joao@intesys.com.br'
PASSWORD = 'GJPKPSDSJXSHFTOM'

def test_rcptmax_limit():
    """Test if recipient limit is enforced"""
    print("Testing RCPTMAX limit enforcement (3 recipients limit)...\n")

    context = ssl.create_default_context()
    context.check_hostname = False
    context.verify_mode = ssl.CERT_NONE

    try:
        with smtplib.SMTP(PROXY_HOST, PROXY_PORT, timeout=15) as server:
            # Check advertised RCPTMAX limit
            resp = server.ehlo()
            ehlo_response = resp[1].decode() if isinstance(resp[1], bytes) else str(resp[1])
            print("EHLO capabilities:")
            for line in ehlo_response.split('\n'):
                if 'LIMITS' in line or 'RCPTMAX' in line:
                    print(f"  {line}")

            server.starttls(context=context)
            server.ehlo()
            server.login(FROM_EMAIL, PASSWORD)

            # Try to send to 4 recipients (limit is 3)
            recipients = [
                'recipient1@example.com',
                'recipient2@example.com',
                'recipient3@example.com',
                'recipient4@example.com'  # This should be rejected
            ]

            print(f"\nAttempting to send to {len(recipients)} recipients (limit is 3)...")

            server.mail(FROM_EMAIL)

            accepted_count = 0
            rejected_count = 0

            for i, rcpt in enumerate(recipients, 1):
                try:
                    code, msg = server.rcpt(rcpt)
                    # 2xx codes mean success, 4xx/5xx mean failure
                    if 200 <= code < 300:
                        accepted_count += 1
                        print(f"  ✓ Recipient {i}: {rcpt} accepted (code: {code})")
                    else:
                        rejected_count += 1
                        print(f"  ✗ Recipient {i} rejected: {rcpt} (code: {code}, msg: {msg})")
                except smtplib.SMTPRecipientsRefused as e:
                    rejected_count += 1
                    print(f"  ✗ Recipient {i} rejected: {rcpt} - {e}")
                    break
                except smtplib.SMTPResponseException as e:
                    rejected_count += 1
                    print(f"  ✗ Recipient {i} rejected: {rcpt} - {e}")
                    break
                except Exception as e:
                    rejected_count += 1
                    print(f"  ✗ Recipient {i} rejected: {rcpt} - {type(e).__name__}: {e}")
                    break

            # Don't actually send the message
            server.rset()

            print(f"\nAccepted: {accepted_count}, Rejected: {rejected_count}")

            if accepted_count == 3 and rejected_count == 1:
                print("✓ PASS: RCPTMAX limit is enforced correctly")
                return True
            elif accepted_count > 3:
                print(f"✗ FAIL: More than 3 recipients accepted ({accepted_count})")
                return False
            else:
                print(f"? UNEXPECTED: {accepted_count} accepted, {rejected_count} rejected")
                return False

    except Exception as e:
        print(f"? ERROR: Unexpected exception: {type(e).__name__}: {e}")
        return False

if __name__ == '__main__':
    print("=" * 60)
    print("RCPTMAX LIMIT ENFORCEMENT TEST")
    print("=" * 60 + "\n")

    result = test_rcptmax_limit()

    print("\n" + "=" * 60)
    if result:
        print("RESULT: RCPTMAX limit is enforced ✓")
    else:
        print("RESULT: RCPTMAX limit is NOT enforced ✗")
    print("=" * 60)
