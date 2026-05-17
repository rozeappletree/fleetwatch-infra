#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DRIVER_APP_DIR="${DRIVER_APP_DIR:-$ROOT_DIR/driver_app}"
HSLWEB_ENV="$ROOT_DIR/envs/hslweb.env"

NGROK_BIN="${NGROK_BIN:-ngrok}"
NGROK_API="${NGROK_API:-http://127.0.0.1:4040/api/tunnels}"
MQTT_PORT="${MQTT_PORT:-1883}"
FRONTEND_PORT="${FRONTEND_PORT:-8080}"
RUN_FLUTTER="${RUN_FLUTTER:-1}"
SKIP_PUB_GET="${SKIP_PUB_GET:-0}"
TUNNEL_MODE="${TUNNEL_MODE:-auto}" # auto, ngrok, or lan

log() {
  printf '\033[1;34m[dev-tunnel]\033[0m %s\n' "$*"
}

fail() {
  printf '\033[1;31m[dev-tunnel]\033[0m %s\n' "$*" >&2
  exit 1
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || fail "Missing command: $1"
}

has_cmd() {
  command -v "$1" >/dev/null 2>&1
}

detect_lan_ip() {
  if command -v ip >/dev/null 2>&1; then
    ip route get 8.8.8.8 2>/dev/null | awk '{for (i=1; i<=NF; i++) if ($i=="src") {print $(i+1); exit}}'
    return
  fi

  hostname -I 2>/dev/null | awk '{print $1}'
}

wait_for_ngrok_tcp_url() {
  local attempt public_url

  for attempt in $(seq 1 40); do
    public_url="$(python3 - "$NGROK_API" <<'PY' 2>/dev/null || true
import json
import sys
import urllib.request

api = sys.argv[1]
with urllib.request.urlopen(api, timeout=2) as response:
    data = json.load(response)

for tunnel in data.get("tunnels", []):
    public_url = tunnel.get("public_url", "")
    if public_url.startswith("tcp://"):
        print(public_url)
        break
PY
)"

    if [ -n "$public_url" ]; then
      printf '%s\n' "$public_url"
      return 0
    fi

    sleep 0.5
  done

  return 1
}

write_hslweb_env() {
  local mqtt_host="$1"
  local mqtt_port="$2"
  local tmp_file

  tmp_file="$(mktemp)"

  if [ -f "$HSLWEB_ENV" ]; then
    grep -v -E '^(PUBLIC_MQTT_HOST|PUBLIC_MQTT_PORT)=' "$HSLWEB_ENV" > "$tmp_file" || true
  else
    printf 'API_HOST=localhost\n' > "$tmp_file"
  fi

  {
    printf '\n'
    printf '# Public MQTT endpoint written into /cdn/fleet-config.json by the frontend\n'
    printf '# container. Dev script updates this from ngrok or LAN mode.\n'
    printf 'PUBLIC_MQTT_HOST=%s\n' "$mqtt_host"
    printf 'PUBLIC_MQTT_PORT=%s\n' "$mqtt_port"
  } >> "$tmp_file"

  mv "$tmp_file" "$HSLWEB_ENV"
}

cleanup() {
  if [ -n "${NGROK_PID:-}" ] && kill -0 "$NGROK_PID" >/dev/null 2>&1; then
    log "Stopping ngrok tunnel"
    kill "$NGROK_PID" >/dev/null 2>&1 || true
  fi
}

trap cleanup EXIT INT TERM

cd "$ROOT_DIR"

require_cmd docker
require_cmd python3

case "$TUNNEL_MODE" in
  auto|ngrok|lan) ;;
  *) fail "Invalid TUNNEL_MODE=$TUNNEL_MODE. Use auto, ngrok, or lan." ;;
esac

if [ "$TUNNEL_MODE" = "ngrok" ]; then
  require_cmd "$NGROK_BIN"
fi

if [ "$RUN_FLUTTER" = "1" ]; then
  require_cmd flutter
  [ -d "$DRIVER_APP_DIR" ] || fail "Driver app folder not found: $DRIVER_APP_DIR"
fi

CONFIG_HOST="${CONFIG_HOST:-$(detect_lan_ip)}"
[ -n "$CONFIG_HOST" ] || fail "Could not detect LAN IP. Run with CONFIG_HOST=your.laptop.ip"

CONFIG_URL="${CONFIG_URL:-http://$CONFIG_HOST:$FRONTEND_PORT/cdn/fleet-config.json}"

log "Starting backend services"
docker compose up -d mosquitto redis mqtt locations_api

PUBLIC_MQTT_HOST="$CONFIG_HOST"
PUBLIC_MQTT_PORT="$MQTT_PORT"
USING_TUNNEL="lan"

if [ "$TUNNEL_MODE" != "lan" ] && has_cmd "$NGROK_BIN"; then
  log "Opening ngrok TCP tunnel to local MQTT port $MQTT_PORT"
  "$NGROK_BIN" tcp "$MQTT_PORT" --log=stdout > /tmp/fleetwatch-ngrok.log 2>&1 &
  NGROK_PID="$!"

  if PUBLIC_URL="$(wait_for_ngrok_tcp_url)"; then
    PUBLIC_URL="${PUBLIC_URL#tcp://}"
    PUBLIC_MQTT_HOST="${PUBLIC_URL%:*}"
    PUBLIC_MQTT_PORT="${PUBLIC_URL##*:}"
    USING_TUNNEL="ngrok"
  else
    sed -n '1,120p' /tmp/fleetwatch-ngrok.log >&2 || true
    if [ "$TUNNEL_MODE" = "ngrok" ]; then
      fail "Could not read ngrok TCP URL. ngrok TCP may require account verification."
    fi

    log "ngrok TCP unavailable; falling back to LAN mode."
    log "Phone must be on the same Wi-Fi/network as this machine."
    cleanup
    unset NGROK_PID
  fi
elif [ "$TUNNEL_MODE" = "ngrok" ]; then
  fail "Missing command: $NGROK_BIN"
else
  log "Using LAN mode. Phone must be on the same Wi-Fi/network as this machine."
fi

log "MQTT endpoint ($USING_TUNNEL): $PUBLIC_MQTT_HOST:$PUBLIC_MQTT_PORT"
log "Writing envs/hslweb.env"
write_hslweb_env "$PUBLIC_MQTT_HOST" "$PUBLIC_MQTT_PORT"

log "Recreating frontend so it publishes the new discovery JSON"
docker compose up -d --build --force-recreate frontend

log "Discovery URL: $CONFIG_URL"
python3 - "$CONFIG_URL" <<'PY' || true
import sys
import urllib.request

url = sys.argv[1]
try:
    with urllib.request.urlopen(url, timeout=5) as response:
        print(response.read().decode("utf-8"))
except Exception as exc:
    print(f"Could not fetch discovery JSON yet: {exc}", file=sys.stderr)
PY

if [ "$RUN_FLUTTER" != "1" ]; then
  if [ "$USING_TUNNEL" = "ngrok" ]; then
    log "RUN_FLUTTER=0 set, leaving backend and tunnel running until this script exits."
    wait "$NGROK_PID"
  else
    log "RUN_FLUTTER=0 set, backend is ready in LAN mode."
    log "Press Ctrl+C when done if you want this script to return now."
  fi
  exit 0
fi

cd "$DRIVER_APP_DIR"

if [ "$SKIP_PUB_GET" != "1" ]; then
  log "Refreshing Flutter dependencies"
  flutter pub get
fi

log "Starting driver app with FLEET_CONFIG_URL=$CONFIG_URL"
flutter run --dart-define="FLEET_CONFIG_URL=$CONFIG_URL" "$@"
