import socket
import json
import time
import sys

PORT = 50222
DURATION = int(sys.argv[1]) if len(sys.argv) > 1 else 180
OUT = sys.argv[2] if len(sys.argv) > 2 else 'captured_events.jsonl'

sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
sock.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
sock.setsockopt(socket.SOL_SOCKET, socket.SO_BROADCAST, 1)
sock.bind(('', PORT))
sock.settimeout(1)

start = time.time()
count = 0
types_seen = set()

print(f'Capturing UDP from port {PORT} for {DURATION}s -> {OUT}', flush=True)

with open(OUT, 'w') as f:
    while time.time() - start < DURATION:
        try:
            data, addr = sock.recvfrom(4096)
            msg = json.loads(data.decode())
            msg['_captured_at'] = time.time()
            msg['_source_ip'] = addr[0]
            f.write(json.dumps(msg) + '\n')
            f.flush()
            count += 1
            t = msg.get('type', '?')
            if t not in types_seen:
                types_seen.add(t)
                print(f'  [new] {t} from {addr[0]}', flush=True)
            else:
                print(f'  [{count:4d}] {t}', flush=True)
        except socket.timeout:
            continue
        except Exception as e:
            print(f'  Error: {e}', flush=True)

sock.close()
print(f'\nDone. {count} events captured. Types: {sorted(types_seen)}', flush=True)
