#!/usr/bin/env bash
# Verifies TCP + UDP echo through gpm's published stream port.
# Usage: ./test.sh            (defaults to 127.0.0.1:15432)
#        HOST=192.0.2.10 PORT=15432 ./test.sh
#
# Target host/port default from test/local.env if present (gitignored, real
# infra values for local/live runs), else test/local.env.example (synthetic
# defaults, committed). Either can be overridden per-invocation with HOST=/PORT=.
set -euo pipefail
cd "$(dirname "$0")"
if [ -f ../local.env ]; then
  . ../local.env
elif [ -f ../local.env.example ]; then
  . ../local.env.example
fi
HOST="${HOST:-${STREAM_TEST_HOST:-127.0.0.1}}"
PORT="${PORT:-${STREAM_TEST_PORT:-15432}}"

python3 - "$HOST" "$PORT" <<'PY'
import socket, sys
host, port = sys.argv[1], int(sys.argv[2])
msg = b"gpm-stream-test"

# TCP: connect through gpm -> echo -> back.
s = socket.create_connection((host, port), timeout=3)
s.sendall(msg)
got = s.recv(1024)
s.close()
assert got == msg, f"TCP echo mismatch: {got!r}"
print(f"TCP  {host}:{port} echo OK")

# UDP: one datagram round-trips through gpm's per-client session.
u = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
u.settimeout(3)
u.sendto(msg, (host, port))
got, _ = u.recvfrom(1024)
u.close()
assert got == msg, f"UDP echo mismatch: {got!r}"
print(f"UDP  {host}:{port} echo OK")
PY

echo "stream forwarding verified"
