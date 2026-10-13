#!/usr/bin/env bash
# Run from your Mac. Builds tempest-mqtt for the target host, copies it with
# this directory and the site's config, and runs deploy/remote.sh there (under
# sudo, which asks for your password). Installs a system service that starts
# at boot.
#
#   deploy/deploy.sh --site NAME --check     what is installed, what would change (read-only, no sudo)
#   deploy/deploy.sh --site NAME             install / upgrade; shows the plan and asks first
#   deploy/deploy.sh --site NAME --yes       ... without asking
#   deploy/deploy.sh --site NAME rollback [--check]   stop and remove it (config kept)
#
#   --site NAME        required: a site in deploy/sites, with its config in
#                      configs/NAME.env. There is no default, so a deploy
#                      always names the host it changes.
#   --host USER@HOST   override the site's ssh target
#
# deploy/sites is local and not in git: copy deploy/sites.example and fill in
# your hosts. TEMPEST_SITES names another file.
#
# The binary is built for whatever `uname -m` the host reports.
# (home on garibaldi still runs the older per-user install: ./deploy.sh.)
set -euo pipefail

SRC="$(cd "$(dirname "$0")/.." && pwd)"
SITE="" HOST="" ARGS=() CHECK=0
while [ $# -gt 0 ]; do
  case "$1" in
    --site) SITE="${2:?--site needs a name}"; shift ;;
    --host) HOST="${2:?--host needs USER@HOST}"; shift ;;
    --check|--dry-run) CHECK=1; ARGS+=(--check) ;;
    -h|--help) sed -n '2,21p' "$0"; exit 0 ;;
    *) ARGS+=("$1") ;;
  esac
  shift
done

die() { echo "ERROR: $*" >&2; exit 1; }

if [ -z "$SITE" ]; then
  sites="$(cd "$SRC/configs" && ls *.env | sed 's/\.env$//' | tr '\n' ' ')"
  die "--site NAME is required (sites: ${sites% })"
fi
CONFIG="$SRC/configs/$SITE.env"
[ -f "$CONFIG" ] || die "no config for site '$SITE' (expected configs/$SITE.env)"
if [ -z "$HOST" ]; then
  SITES="${TEMPEST_SITES:-$SRC/deploy/sites}"
  [ -f "$SITES" ] || die "no $SITES: copy deploy/sites.example to deploy/sites and fill in your hosts (or pass --host)"
  HOST="$(awk -v s="$SITE" '$1 == s {print $2; exit}' "$SITES")"
  [ -n "$HOST" ] || die "site '$SITE' is not in $SITES; add it or pass --host"
fi

ssh -o ConnectTimeout=10 -o BatchMode=yes "$HOST" true || die "cannot ssh to $HOST"
ARCH="$(ssh "$HOST" uname -m)"
case "$ARCH" in
  aarch64|arm64) GOARCH=arm64 ;;
  x86_64|amd64)  GOARCH=amd64 ;;
  *) die "$HOST is $ARCH; tempest-mqtt is deployed for arm64 and amd64 only" ;;
esac

STAGE="$(mktemp -d "${TMPDIR:-/tmp}/tempest-mqtt-deploy.XXXXXX")"
trap 'rm -rf "$STAGE"' EXIT
echo "site $SITE on $HOST: building tempest-mqtt $(git -C "$SRC" describe --always --dirty 2>/dev/null || echo dev) for linux/$GOARCH"
(cd "$SRC" && GOOS=linux GOARCH="$GOARCH" CGO_ENABLED=0 go build -trimpath \
  -ldflags "-s -w" -o "$STAGE/tempest-mqtt" ./cmd/tempest-mqtt)
mkdir -p "$STAGE/deploy"
cp "$SRC/deploy/remote.sh" "$SRC/deploy/tempest-mqtt.service" "$STAGE/deploy/"
cp "$CONFIG" "$STAGE/tempest-mqtt.env.example"   # remote.sh installs it if the host has none

REMOTE_DIR="$(ssh "$HOST" mktemp -d /tmp/tempest-mqtt.XXXXXX)"
trap 'rm -rf "$STAGE"; ssh "$HOST" rm -rf "$REMOTE_DIR" >/dev/null 2>&1 || true' EXIT
tar -C "$STAGE" -cf - . | ssh "$HOST" tar -C "$REMOTE_DIR" -xf -

echo "--- $HOST ---"
if [ "$CHECK" = 1 ]; then
  ssh "$HOST" bash "$REMOTE_DIR/deploy/remote.sh" ${ARGS[@]+"${ARGS[@]}"}
else
  ssh -t "$HOST" sudo bash "$REMOTE_DIR/deploy/remote.sh" ${ARGS[@]+"${ARGS[@]}"}
fi
