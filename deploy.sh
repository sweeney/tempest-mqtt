#!/usr/bin/env bash
set -euo pipefail

# Usage: ./deploy.sh [user@host]
#
# Deploys tempest-mqtt as a systemd USER service (no sudo required).
# The binary runs as the logged-in user; config lives in ~/.config/tempest-mqtt.env
#
# One-time bootstrap (run once on the remote host to survive reboots):
#   sudo loginctl enable-linger $USER

REMOTE_HOST="${1:-sweeney@100.122.159.5}"
REMOTE_DIR="tempest-mqtt"
SERVICE="tempest-mqtt"
KEEP_VERSIONS=3

VERSION=$(date +%Y%m%d-%H%M%S)
REMOTE_BIN="tempest-mqtt-${VERSION}"

echo "==> Building for linux/amd64..."
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o tempest-mqtt-linux-amd64 ./cmd/tempest-mqtt/

echo "==> Uploading to ${REMOTE_HOST}..."
ssh "${REMOTE_HOST}" "mkdir -p ~/${REMOTE_DIR} ~/.config/systemd/user"
scp -q tempest-mqtt-linux-amd64 "${REMOTE_HOST}:~/${REMOTE_DIR}/${REMOTE_BIN}"
scp -q tempest-mqtt.service      "${REMOTE_HOST}:~/.config/systemd/user/tempest-mqtt.service"

ssh "${REMOTE_HOST}" "
  chmod +x ~/${REMOTE_DIR}/${REMOTE_BIN} && \
  ln -sfn ${REMOTE_BIN} ~/${REMOTE_DIR}/tempest-mqtt
"

echo "==> Configuring..."
# Ensure every required key exists in the env file; never overwrite existing values.
ssh "${REMOTE_HOST}" '
  f=~/.config/tempest-mqtt.env
  touch "$f"
  add_default() { grep -q "^$1=" "$f" || echo "$1=$2" >> "$f"; }
  add_default MQTT_BROKER_URL   tcp://localhost:1883
  add_default MQTT_CLIENT_ID    tempest-mqtt
  add_default MQTT_USERNAME     ""
  add_default MQTT_PASSWORD     ""
  add_default TOPIC_PREFIX      home
  add_default TEMPEST_UDP_PORT  50222
  add_default LOG_LEVEL         info
  echo "  env: $f"
'

echo "==> Enabling and restarting service..."
ssh "${REMOTE_HOST}" "
  systemctl --user daemon-reload && \
  systemctl --user enable ${SERVICE} && \
  systemctl --user restart ${SERVICE}
"

echo "==> Cleaning old versions (keeping ${KEEP_VERSIONS})..."
ssh "${REMOTE_HOST}" "
  cd ~/${REMOTE_DIR} && \
  ls -t tempest-mqtt-[0-9]* 2>/dev/null \
    | tail -n +$((KEEP_VERSIONS + 1)) \
    | xargs -r rm --
"

echo ""
echo "==> Deployed ${VERSION}:"
ssh "${REMOTE_HOST}" "systemctl --user status ${SERVICE} --no-pager -l | head -20"
echo ""
ssh "${REMOTE_HOST}" "journalctl --user -u ${SERVICE} -n 8 --no-pager"
echo ""
echo "Note: to start automatically after reboot, run once on the host:"
echo "  ssh ${REMOTE_HOST} 'sudo loginctl enable-linger \$USER'"
