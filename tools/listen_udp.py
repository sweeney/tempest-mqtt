import socket
import json
import time
import sys

PORT = 50222

sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
sock.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)
sock.bind(('', PORT))
sock.settimeout(20)

print(f'Bound to 0.0.0.0:{PORT}', flush=True)
print('Waiting up to 20s for packets...', flush=True)
start = time.time()

try:
    while True:
        data, addr = sock.recvfrom(4096)
        print(f'Got packet from {addr[0]}:{addr[1]}', flush=True)
        try:
            msg = json.loads(data.decode())
            print(json.dumps(msg, indent=2), flush=True)
        except Exception:
            print(f'Raw: {data[:200]}', flush=True)
        print(flush=True)
except socket.timeout:
    elapsed = time.time() - start
    print(f'No packets received in {elapsed:.1f}s', flush=True)
finally:
    sock.close()
