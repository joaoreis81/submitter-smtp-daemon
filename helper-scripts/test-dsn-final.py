#!/usr/bin/env python3
"""
Final DSN test - send complete email with DSN parameters
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

def test_dsn_complete():
    """Send a complete email with DSN parameters using send_message"""
    print("Testing complete DSN email with send_message()...\n")

    context = ssl.create_default_context()
    context.check_hostname = False
    context.verify_mode = ssl.CERT_NONE

    try:
        with smtplib.SMTP(PROXY_HOST, PROXY_PORT, timeout=15) as server:
            # Connect and authenticate
            server.ehlo()

            # Check DSN support
            if not server.has_extn('DSN'):
                print("✗ FAIL: DSN not supported")
                return False

            print("✓ DSN extension available")

            server.starttls(context=context)
            server.ehlo()
            server.login(FROM_EMAIL, PASSWORD)
            print("✓ Authenticated")

            # Create message
            timestamp = datetime.now().strftime("%Y-%m-%d %H:%M:%S")
            msg = MIMEText(f'''DSN Forwarding Test

Time: {timestamp}

This message was sent through the SMTP Edge Proxy with DSN parameters.

The proxy should have:
1. Accepted MAIL FROM with RET=FULL parameter
2. Accepted RCPT TO with NOTIFY=SUCCESS,FAILURE parameters
3. Forwarded both parameters to the backend server

Check the backend server logs to verify the DSN parameters were received.
''')
            msg['Subject'] = f'DSN Forwarding Test - {timestamp}'
            msg['From'] = FROM_EMAIL
            msg['To'] = TO_EMAIL

            # Send with manual MAIL/RCPT to include DSN parameters
            print("\nSending MAIL FROM with RET=FULL...")
            server.docmd('MAIL FROM:<{}> RET=FULL ENVID=test-{}'.format(FROM_EMAIL, timestamp.replace(' ', '-').replace(':', '')))

            print("Sending RCPT TO with NOTIFY=SUCCESS,FAILURE...")
            server.docmd('RCPT TO:<{}> NOTIFY=SUCCESS,FAILURE'.format(TO_EMAIL))

            print("Sending message data...")
            code, response = server.docmd('DATA')
            print(f"DATA response: {code} {response.decode() if isinstance(response, bytes) else response}")

            # Send the actual message
            for line in msg.as_string().split('\n'):
                if line and line[0] == '.':
                    line = '.' + line
                server.send((line + '\r\n').encode())
            server.send(b'\r\n.\r\n')

            code, response = server.getreply()
            print(f"✓ Message sent: {code} {response.decode() if isinstance(response, bytes) else response}")

            return code == 250

    except Exception as e:
        print(f"✗ ERROR: {type(e).__name__}: {e}")
        import traceback
        traceback.print_exc()
        return False

if __name__ == '__main__':
    print("=" * 60)
    print("DSN COMPLETE EMAIL TEST")
    print("=" * 60 + "\n")

    result = test_dsn_complete()

    print("\n" + "=" * 60)
    if result:
        print("RESULT: DSN email sent successfully ✓")
        print("\nCheck /tmp/proxy-dsn.log for:")
        print("  - 'MAIL FROM options' with dsn_ret")
        print("  - 'RCPT TO options' with dsn_notify")
        print("  - 'forwarding MAIL FROM DSN options to backend'")
        print("  - 'forwarding RCPT TO DSN options to backend'")
    else:
        print("RESULT: DSN email test FAILED ✗")
    print("=" * 60)
