#!/bin/sh
set -eu

# Defaults; allow overrides via args or env
HOST="${1:-${SERVER_HOST:-server}}"
PORT="${2:-${SERVER_PORT:-12345}}"

MSG="${3:-tp0-echo-$$-$(date +%s)}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose-dev.yaml}"
NC_TIMEOUT="${NC_TIMEOUT:-3}"

# Find the server container's network
CID="$(docker compose -f "$COMPOSE_FILE" ps -q "$HOST" 2>/dev/null || true)"
[ -n "$CID" ] || CID="$HOST"

# Get first attached network name
NET="$(docker inspect -f '{{range $k,$v := .NetworkSettings.Networks}}{{println $k}}{{end}}' "$CID" 2>/dev/null | awk 'NR==1{print $1}')"

if [ -z "$NET" ]; then
  echo "action: test_echo_server | result: fail"
  exit 1
fi

# Run a one-shot BusyBox container on that network and use its nc
# -w sets both connect and I/O timeout in BusyBox nc
RESP="$(docker run --rm --network "$NET" busybox sh -c "
  printf '%s\n' '$MSG' | nc -w $NC_TIMEOUT '$HOST' '$PORT' 2>/dev/null
" 2>/dev/null || true)"

# Trim CR/LF to compare pure token
RESP_TRIMMED="$(printf '%s' "$RESP" | tr -d '\r\n')"

if [ "$RESP_TRIMMED" = "$MSG" ]; then
  echo "action: test_echo_server | result: success"
  exit 0
else
  echo "action: test_echo_server | result: fail"
  exit 1
fi
