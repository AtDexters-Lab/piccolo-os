#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
PICCOLO_DIR=$(cd "$SCRIPT_DIR/../.." && pwd)
UI_DIR="$PICCOLO_DIR/ui-next"

if [[ $# -ge 1 ]]; then
  CMD="$1"
  shift
else
  CMD="e2e"
fi
CMD_ARGS=("$@")

if [[ -n "${PICCOLO_E2E_PORT:-}" ]]; then
  PORT="$PICCOLO_E2E_PORT"
else
  PORT=$(python3 - <<'PY'
import random, socket
from contextlib import closing
for _ in range(200):
    port = random.randint(20000, 60000)
    with closing(socket.socket(socket.AF_INET, socket.SOCK_STREAM)) as s:
        try:
            s.bind(('127.0.0.1', port))
        except OSError:
            continue
        print(port)
        raise SystemExit(0)
print(0)
PY
)
  if [[ "$PORT" == "0" || -z "$PORT" ]]; then
    echo "Failed to allocate ephemeral port" >&2
    exit 1
  fi
fi
BASE_URL="http://127.0.0.1:${PORT}"
STATE_DIR=${PICCOLO_E2E_STATE_DIR:-}
LOG_FILE=${PICCOLO_E2E_LOG:-$PICCOLO_DIR/test-results/piccolod-e2e.log}

mkdir -p "$PICCOLO_DIR/test-results"
if [[ -z "$STATE_DIR" ]]; then
  STATE_DIR=$(mktemp -d "$PICCOLO_DIR/.e2e-state-XXXXXX")
  REMOVE_STATE=1
else
  mkdir -p "$STATE_DIR"
  REMOVE_STATE=0
fi

SERVER_PID=""
cleanup() {
  local exit_code=$?
  if [[ -n "$SERVER_PID" ]] && kill -0 "$SERVER_PID" 2>/dev/null; then
    kill "$SERVER_PID" 2>/dev/null || true
    wait "$SERVER_PID" 2>/dev/null || true
  fi
  if [[ ${REMOVE_STATE:-0} -eq 1 && -d "$STATE_DIR" ]]; then
    sleep 2
    rm -rf "$STATE_DIR"
  fi
  exit $exit_code
}
trap cleanup EXIT

cd "$PICCOLO_DIR"
echo "==> Building piccolod + ui"
make build

echo "==> Launching piccolod on $BASE_URL (state: $STATE_DIR)"
PICCOLO_STATE_DIR="$STATE_DIR" PORT="$PORT" PICCOLO_DISABLE_MDNS="1" ./piccolod >"$LOG_FILE" 2>&1 &
SERVER_PID=$!

ready=0
for attempt in {1..60}; do
  if curl -fs "$BASE_URL/api/v1/health/live" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1

done
if [[ $ready -ne 1 ]]; then
  echo "Piccolod did not become ready; see $LOG_FILE" >&2
  tail -n 200 "$LOG_FILE" >&2 || true
  exit 1
fi

echo "==> Running npm run $CMD against $BASE_URL"
cd "$UI_DIR"
if [[ ${#CMD_ARGS[@]} -gt 0 ]]; then
  CMD_ARRAY=(npm run "$CMD" -- "${CMD_ARGS[@]}")
else
  CMD_ARRAY=(npm run "$CMD")
fi
if ! PICCOLO_BASE_URL="$BASE_URL" "${CMD_ARRAY[@]}"; then
  echo "Playwright failed; tailing server log from $LOG_FILE" >&2
  tail -n 200 "$LOG_FILE" >&2 || true
  exit 1
fi

echo "==> Tests finished; logs saved to $LOG_FILE"
