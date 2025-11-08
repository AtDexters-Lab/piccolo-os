import random
import socket
from contextlib import closing

LOW, HIGH = 20000, 60000
for _ in range(100):
    port = random.randint(LOW, HIGH)
    with closing(socket.socket(socket.AF_INET, socket.SOCK_STREAM)) as s:
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        try:
            s.bind(('127.0.0.1', port))
        except OSError:
            continue
        print(port)
        raise SystemExit(0)
print(0)
