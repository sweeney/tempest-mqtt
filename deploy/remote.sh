#!/usr/bin/env bash
# Runs ON THE TARGET HOST. Normally invoked by deploy.sh, which builds
# tempest-mqtt for the host, copies this tree and the site's config over (as
# tempest-mqtt.env.example) and runs it under sudo.
#
#   remote.sh [--check] [--yes] [install]   install / upgrade (default)
#   remote.sh [--check] [--yes] rollback    stop and remove it (config kept)
#
# --check   report what is installed and what would change; changes nothing
#           and needs no root.
# --yes     skip the confirmation prompt.
#
# Installs a system unit, enabled for multi-user.target, so it starts at boot
# with no login and no linger. The config is installed only if there is none,
# so edits on the host survive upgrades. Safe to re-run: a run with nothing to
# change restarts nothing. A failed verification puts the previous install back.
#
# ROOT (env) prefixes every path, for the tests. Empty on the host.
set -euo pipefail

ROOT="${ROOT:-}"
CONFIRM_TTY="${CONFIRM_TTY:-/dev/tty}"
HERE="$(cd "$(dirname "$0")" && pwd)"
PAYLOAD="$(cd "$HERE/.." && pwd)"

SVC=tempest-mqtt
BIN="$ROOT/usr/local/bin/tempest-mqtt"
CFG_DIR="$ROOT/etc/tempest-mqtt"
CFG="$CFG_DIR/tempest-mqtt.env"
UNIT="$ROOT/etc/systemd/system/tempest-mqtt.service"
BACKUP_ROOT="$ROOT/var/backups/tempest-mqtt"

CHECK=0 YES=0 CMD=""
while [ $# -gt 0 ]; do
  case "$1" in
    --check|--dry-run) CHECK=1 ;;
    --yes|-y) YES=1 ;;
    install|rollback) CMD="$1" ;;
    -h|--help) sed -n '2,18p' "$0"; exit 0 ;;
    *) echo "unknown argument: $1" >&2; exit 2 ;;
  esac
  shift
done
CMD="${CMD:-install}"

say()  { echo "==> $*"; }
info() { echo "    $*"; }
die()  { echo "ERROR: $*" >&2; exit 1; }

confirm() {
  [ "$YES" = 1 ] && return 0
  if ! { exec 3<"$CONFIRM_TTY"; } 2>/dev/null; then
    die "no terminal to confirm on; re-run with --yes to proceed without a prompt"
  fi
  local reply; printf '%s [y/N] ' "$1"; read -r reply <&3 || true; exec 3<&-
  [[ "$reply" =~ ^[Yy]$ ]] || { echo "aborted, nothing changed"; exit 1; }
}
need_root() { [ "$(id -u)" = 0 ] || die "$CMD needs root (deploy.sh runs it under sudo)"; }
same_file() { [ -f "$2" ] && cmp -s "$1" "$2"; }
sha() { [ -f "$1" ] && sha256sum "$1" | cut -c1-12 || echo none; }

# env_get FILE KEY: a value from an env file (no quotes or expansion needed here).
env_get() { sed -n "s/^$2=//p" "$1" | tail -1 | sed -e 's/^"\(.*\)"$/\1/'; }

BACKUP=""
backup() {
  if [ -z "$BACKUP" ]; then
    # Unique per run: two runs in the same second must not share (and nest) one.
    mkdir -p "$BACKUP_ROOT"; BACKUP="$(mktemp -d "$BACKUP_ROOT/$(date +%Y%m%d-%H%M%S).XXXX")"
    info "backing up to ${BACKUP#"$ROOT"}"
  fi
  local p; for p in "$@"; do
    [ -e "$p" ] || continue
    mkdir -p "$BACKUP$(dirname "${p#"$ROOT"}")"
    cp -p "$p" "$BACKUP${p#"$ROOT"}"
  done
}

# ---------------------------------------------------------------------------
preflight() {
  say "preflight"
  local f; for f in tempest-mqtt deploy/tempest-mqtt.service tempest-mqtt.env.example; do
    [ -f "$PAYLOAD/$f" ] || die "payload missing $f"
  done
  # Go's -h exits 0; a binary for another architecture does not run at all.
  "$PAYLOAD/tempest-mqtt" -h >/dev/null 2>&1 || die "the payload binary does not run here (wrong architecture?)"
  info "payload binary runs here ($(sha "$PAYLOAD/tempest-mqtt"))"
  command -v mosquitto_sub >/dev/null || die "mosquitto_sub is needed to verify the install (apt install mosquitto-clients)"

  # A per-user install (the old deploy.sh) would run alongside this one and
  # every reading would be published twice.
  local u; for u in "$ROOT"/home/*/.config/systemd/user/tempest-mqtt.service "$ROOT"/root/.config/systemd/user/tempest-mqtt.service; do
    [ -e "$u" ] && die "a per-user tempest-mqtt is installed (${u#"$ROOT"}); remove it first or it and this one would both publish"
  done
  info "no per-user tempest-mqtt"

  # The config is kept across upgrades, so a payload for another site would
  # never be installed, and the deploy would silently verify the wrong topics.
  local want have; want="$(env_get "$PAYLOAD/tempest-mqtt.env.example" TOPIC_PREFIX)"
  [ -n "$want" ] || die "the payload config has no TOPIC_PREFIX"
  if [ -f "$CFG" ] && [ ! -r "$CFG" ]; then
    info "(cannot read /etc/tempest-mqtt/tempest-mqtt.env without root: site check left to the install)"
  elif [ -f "$CFG" ]; then
    have="$(env_get "$CFG" TOPIC_PREFIX)"
    [ "$have" = "$want" ] || die "this host runs tempest-mqtt for '$have', but this deploy is for '$want' (wrong --site or --host?)"
  fi
  info "site: $want (topics climate/$want/...)"

  local cfg port; cfg="$(config_in_force)"
  case "$(env_get "$cfg" MQTT_BROKER_URL)" in
    *://127.0.0.1:*|*://localhost:*)
      systemctl is-active --quiet mosquitto || die "the config publishes to a local broker, but mosquitto is not running"
      info "local mosquitto active" ;;
  esac
  # Something else on the UDP port would take the broadcasts (or this would).
  port="$(env_get "$cfg" TEMPEST_UDP_PORT)"
  if ! systemctl is-active --quiet "$SVC" && ss -Hulnp "sport = :$port" 2>/dev/null | grep -q .; then
    die "UDP port $port is already in use: $(ss -Hulnp "sport = :$port" | head -1)"
  fi
  info "UDP $port free (or ours)"
}

config_in_force() { if [ -r "$CFG" ]; then echo "$CFG"; else echo "$PAYLOAD/tempest-mqtt.env.example"; fi; }

PENDING=()
plan_install() {
  PENDING=()
  same_file "$PAYLOAD/tempest-mqtt" "$BIN" || PENDING+=("install /usr/local/bin/tempest-mqtt ($(sha "$PAYLOAD/tempest-mqtt"), now: $(sha "$BIN"))")
  [ -f "$CFG" ] || PENDING+=("install /etc/tempest-mqtt/tempest-mqtt.env from the site config (kept on later upgrades)")
  same_file "$PAYLOAD/deploy/tempest-mqtt.service" "$UNIT" || PENDING+=("install /etc/systemd/system/tempest-mqtt.service")
  if [ "$(systemctl is-enabled "$SVC" 2>/dev/null)" != enabled ] || ! systemctl is-active --quiet "$SVC" || [ ${#PENDING[@]} -gt 0 ]; then
    PENDING+=("enable at boot + (re)start tempest-mqtt")
  fi
}

verify() {
  say "verifying"
  local prefix url hostport host port user pass topic got i
  prefix="$(env_get "$CFG" TOPIC_PREFIX)"
  for i in $(seq 10); do systemctl is-active --quiet "$SVC" && break; sleep 1; done
  systemctl is-active --quiet "$SVC" || { echo "    tempest-mqtt is not active"; return 1; }
  [ "$(systemctl is-enabled "$SVC" 2>/dev/null)" = enabled ] || { echo "    tempest-mqtt is not enabled at boot"; return 1; }
  info "active, enabled at boot"
  for i in $(seq 20); do
    journalctl -u "$SVC" --since "@$STARTED_AT" --no-pager 2>/dev/null | grep -q "connected to MQTT broker" && break
    sleep 1
  done
  journalctl -u "$SVC" --since "@$STARTED_AT" --no-pager 2>/dev/null | grep -q "connected to MQTT broker" \
    || { echo "    tempest-mqtt has not logged 'connected to MQTT broker' since its start"; return 1; }
  info "connected to the broker"
  # rapid_wind is not retained and arrives every 3-15 s, so one within 30 s
  # proves the hub's broadcasts are being received and published now.
  url="$(env_get "$CFG" MQTT_BROKER_URL)"; hostport="${url#*://}"
  host="${hostport%:*}"; port="${hostport##*:}"
  user="$(env_get "$CFG" MQTT_USERNAME)"; pass="$(env_get "$CFG" MQTT_PASSWORD)"
  topic="climate/$prefix/wind/rapid"
  got="$(mosquitto_sub -h "$host" -p "$port" ${user:+-u "$user" -P "$pass"} -t "$topic" -C 1 -W 30 2>/dev/null || true)"
  [ -n "$got" ] || { echo "    nothing on $topic within 30 s (is the hub broadcasting on this LAN?)"; return 1; }
  info "live $topic: $got"
}

restore() {
  echo "!!> verification failed; restoring the previous tempest-mqtt" >&2
  echo "    recent log:" >&2; journalctl -u "$SVC" -n 10 --no-pager 2>/dev/null | sed 's/^/      /' >&2 || true
  if [ -n "$BACKUP" ] && [ -f "$BACKUP/usr/local/bin/tempest-mqtt" ]; then cp -p "$BACKUP/usr/local/bin/tempest-mqtt" "$BIN"; fi
  if [ -n "$BACKUP" ] && [ -f "$BACKUP/etc/systemd/system/tempest-mqtt.service" ]; then
    cp -p "$BACKUP/etc/systemd/system/tempest-mqtt.service" "$UNIT"
    systemctl daemon-reload; systemctl restart "$SVC" || true
    echo "    previous version restored and restarted" >&2
  else
    systemctl disable --now "$SVC" 2>/dev/null || true
    rm -f "$UNIT" "$BIN"; systemctl daemon-reload
    echo "    tempest-mqtt removed (it was not installed before); config kept" >&2
  fi
}

do_install() {
  [ "$CHECK" = 1 ] || need_root
  preflight
  plan_install
  say "plan"
  if [ ${#PENDING[@]} -eq 0 ]; then info "nothing to do: tempest-mqtt $(sha "$BIN") installed, enabled and running"; return 0; fi
  local p; for p in "${PENDING[@]}"; do info "- $p"; done
  if [ "$CHECK" = 1 ]; then echo "(--check: nothing changed)"; return 0; fi
  confirm "Apply these changes on $(hostname)?"

  backup "$BIN" "$UNIT"
  say "installing"
  if ! same_file "$PAYLOAD/tempest-mqtt" "$BIN"; then
    install -m 755 "$PAYLOAD/tempest-mqtt" "$BIN.new"; mv -f "$BIN.new" "$BIN"; info "installed /usr/local/bin/tempest-mqtt"
  fi
  if [ ! -f "$CFG" ]; then
    # 0640: it may hold a broker password. The dynamic user never reads it;
    # systemd does, as root.
    mkdir -p "$CFG_DIR"; install -m 640 "$PAYLOAD/tempest-mqtt.env.example" "$CFG"; info "installed /etc/tempest-mqtt/tempest-mqtt.env"
  fi
  if ! same_file "$PAYLOAD/deploy/tempest-mqtt.service" "$UNIT"; then
    install -m 644 "$PAYLOAD/deploy/tempest-mqtt.service" "$UNIT"; info "installed tempest-mqtt.service"
  fi
  systemctl daemon-reload
  systemctl enable "$SVC" >/dev/null 2>&1
  STARTED_AT="$(date +%s)"
  systemctl restart "$SVC"
  verify || { restore; exit 1; }
  say "done"
  info "watch it: journalctl -u tempest-mqtt -f"
}

do_rollback() {
  [ "$CHECK" = 1 ] || need_root
  PENDING=()
  { systemctl is-active --quiet "$SVC" || [ "$(systemctl is-enabled "$SVC" 2>/dev/null)" = enabled ]; } && PENDING+=("stop + disable tempest-mqtt")
  [ -f "$UNIT" ] && PENDING+=("remove /etc/systemd/system/tempest-mqtt.service")
  [ -f "$BIN" ] && PENDING+=("remove /usr/local/bin/tempest-mqtt")
  say "rollback plan"
  if [ ${#PENDING[@]} -eq 0 ]; then info "nothing to do: tempest-mqtt is not installed"; return 0; fi
  local p; for p in "${PENDING[@]}"; do info "- $p"; done
  info "(kept: /etc/tempest-mqtt/tempest-mqtt.env, and retained climate/... topics on the broker)"
  if [ "$CHECK" = 1 ]; then echo "(--check: nothing changed)"; return 0; fi
  confirm "Remove tempest-mqtt from $(hostname)?"
  backup "$BIN" "$UNIT"
  systemctl disable --now "$SVC" 2>/dev/null || true
  rm -f "$UNIT" "$BIN"
  systemctl daemon-reload
  say "removed"; info "previous files kept in ${BACKUP#"$ROOT"}"
}

case "$CMD" in
  install) do_install ;;
  rollback) do_rollback ;;
esac
