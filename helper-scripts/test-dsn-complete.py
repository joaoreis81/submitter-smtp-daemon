#!/usr/bin/env python3
"""
Comprehensive DSN extension test - validates DSN parameter forwarding
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

def test_dsn_forwarding():
    """Test DSN parameter forwarding to backend"""
    print("Testing DSN parameter forwarding...\n")

    context = ssl.create_default_context()
    context.check_hostname = False
    context.verify_mode = ssl.CERT_NONE

    try:
        with smtplib.SMTP(PROXY_HOST, PROXY_PORT, timeout=15) as server:
            server.set_debuglevel(1)  # Show SMTP conversation

            # Initial EHLO
            resp = server.ehlo()
            ehlo_response = resp[1].decode() if isinstance(resp[1], bytes) else str(resp[1])

            print("\n" + "="*60)
            print("EHLO Capabilities:")
            print("="*60)
            for line in ehlo_response.split('\n'):
                print(f"  {line}")

            # Check DSN support
            if 'DSN' not in ehlo_response:
                print("\n✗ FAIL: DSN not advertised")
                return False

            print("\n✓ DSN extension is advertised")

            # STARTTLS
            print("\n" + "="*60)
            print("Starting TLS...")
            print("="*60)
            server.starttls(context=context)
            server.ehlo()

            # Authenticate
            print("\n" + "="*60)
            print("Authenticating...")
            print("="*60)
            server.login(FROM_EMAIL, PASSWORD)
            print("✓ Authenticated")

            # Test 1: MAIL FROM with RET=FULL
            print("\n" + "="*60)
            print("Test 1: MAIL FROM with RET=FULL")
            print("="*60)
            try:
                code, msg = server.docmd('MAIL FROM:<{}> RET=FULL'.format(FROM_EMAIL))
                print(f"✓ MAIL FROM accepted: {code} {msg.decode() if isinstance(msg, bytes) else msg}")
            except Exception as e:
                print(f"✗ MAIL FROM failed: {e}")
                return False

            # Test 2: RCPT TO with NOTIFY=SUCCESS,FAILURE
            print("\n" + "="*60)
            print("Test 2: RCPT TO with NOTIFY=SUCCESS,FAILURE")
            print("="*60)
            try:
                code, msg = server.docmd('RCPT TO:<{}> NOTIFY=SUCCESS,FAILURE'.format(TO_EMAIL))
                print(f"✓ RCPT TO accepted: {code} {msg.decode() if isinstance(msg, bytes) else msg}")
            except Exception as e:
                print(f"✗ RCPT TO failed: {e}")
                return False

            # Test 3: Send actual message with DSN parameters
            print("\n" + "="*60)
            print("Test 3: Sending message with DSN parameters")
            print("="*60)

            timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            msg = MIMEText(f'DSN forwarding test at {timestamp}\\n\\nThis message was sent with DSN parameters:\\n- RET=FULL\\n- NOTIFY=SUCCESS,FAILURE')
            msg['Subject'] = f'DSN Forwarding Test - {timestamp}'
            msg['From'] = FROM_EMAIL
            msg['To'] = TO_EMAIL

            try:
                code, msg_response = server.docmd('DATA')
                print(f"DATA command: {code} {msg_response.decode() if isinstance(msg_response, bytes) else msg_response}")

                # Send message
                server.send(msg.as_string().encode())
                server.send(b'\\r\\n.\\r\\n')

                # Get response
                code, msg_response = server.getreply()
                print(f"✓ Message sent: {code} {msg_response.decode() if isinstance(msg_response, bytes) else msg_response}")

                if code == 250:
                    return True
                else:
                    print(f"✗ Unexpected response code: {code}")
                    return False

            except Exception as e:
                print(f"✗ DATA failed: {e}")
                import traceback
                traceback.print_exc()
                return False

    except Exception as e:
        print(f"✗ ERROR: {type(e).__name__}: {e}")
        import traceback
        traceback.print_exc()
        return False

if __name__ == '__main__':
    print("=" * 60)
    print("DSN PARAMETER FORWARDING TEST")
    print("=" * 60)

    result = test_dsn_forwarding()

    print("\n" + "=" * 60)
    if result:
        print("RESULT: DSN forwarding test PASSED ✓")
        print("\nCheck the proxy logs (/tmp/proxy-dsn.log) to verify:")
        print("  - DSN parameters logged when received from client")
        print("  - DSN parameters forwarded to backend")
    else:
        print("RESULT: DSN forwarding test FAILED ✗")
    print("=" * 60)
