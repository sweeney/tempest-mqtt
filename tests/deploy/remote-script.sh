#!/usr/bin/env bash
# Tests deploy/remote.sh against a fixture root. systemctl, journalctl,
# mosquitto_sub, ss, id and sleep are shims that record calls and play back a
# host's state; the payload "binary" is a script. FAIL_VERIFY makes one
# verification step fail so the restore path runs.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SRC="$HERE/../.."
BASH_BIN="${BASH_BIN:-$(command -v bash)}"
WORK="$(mktemp -d "${TMPDIR:-/tmp}/tempest-remote-test.XXXXXX")"
fails=0
trap '[ "$fails" -gt 0 ] && echo "work dir kept: $WORK" || rm -rf "$WORK"' EXIT
pass() { echo "  PASS  $*"; }
fail() { echo "  FAIL  $*"; fails=$((fails + 1)); }
check() { local d="$1"; shift; if "$@"; then pass "$d"; else fail "$d"; fi; }
not() { ! "$@"; }

SHIMS="$WORK/shims"; mkdir -p "$SHIMS"
shim() { printf '#!/usr/bin/env bash\nset -u\n%s\n' "$2" > "$SHIMS/$1"; chmod +x "$SHIMS/$1"; }
shim id '[ "${1:-}" = -u ] && { echo "${FAKE_UID:-0}"; exit 0; }; /usr/bin/id "$@"'
shim sleep 'true'
shim systemctl '
echo "systemctl $*" >> "$STATE/calls"
svc="${!#}"
case "$1" in
  is-active) [ -f "$STATE/svc-$svc-active" ] ;;
  is-enabled) if [ -f "$STATE/svc-$svc-enabled" ]; then echo enabled; else echo disabled; exit 1; fi ;;
  restart) [ "${FAIL_VERIFY:-}" = start ] || touch "$STATE/svc-$svc-active" ;;
  enable) [ "${FAIL_VERIFY:-}" = enable ] || touch "$STATE/svc-$svc-enabled" ;;
  disable) rm -f "$STATE/svc-$svc-enabled"; [ "$2" = --now ] && rm -f "$STATE/svc-$svc-active"; true ;;
  *) true ;;
esac'
shim journalctl '[ "${FAIL_VERIFY:-}" = journal ] || echo "tempest-mqtt[1]: level=INFO msg=\"connected to MQTT broker\""'
shim mosquitto_sub '
echo "mosquitto_sub $*" >> "$STATE/calls"
[ "${FAIL_VERIFY:-}" = sub ] && exit 27
echo "{\"timestamp\":1791990509,\"speed_ms\":0.32,\"direction_deg\":134}"'
shim ss '[ -f "$STATE/port-busy" ] && echo "UNCONN 0 0 0.0.0.0:50222 0.0.0.0:* users:((\"python3\",pid=9,fd=3))"; true'

# A payload tree as deploy.sh stages it, with a script for the binary.
make_payload() {  # $1 dir, $2 version marker, $3 site
  mkdir -p "$1/deploy"
  cp "$SRC/deploy/remote.sh" "$SRC/deploy/tempest-mqtt.service" "$1/deploy/"
  cp "$SRC/configs/$3.env" "$1/tempest-mqtt.env.example"
  printf '#!/usr/bin/env bash\n# %s\n[ "${FAIL_VERIFY:-}" = arch ] && exit 126\nexit 0\n' "$2" > "$1/tempest-mqtt"
  chmod +x "$1/tempest-mqtt"
}

new_root() {
  ROOT="$WORK/root-$1" STATE="$WORK/state-$1" P="$WORK/payload-$1"
  rm -rf "$ROOT" "$STATE" "$P"
  mkdir -p "$ROOT/etc/systemd/system" "$ROOT/usr/local/bin" "$ROOT/home/sweeney" "$STATE"
  touch "$STATE/svc-mosquitto-active"
  : > "$STATE/calls"
  make_payload "$P" v1 schoolhouse
}
run() {
  local envs=(); while [ "$1" != -- ]; do envs+=("$1"); shift; done; shift
  env ${envs[@]+"${envs[@]}"} ROOT="$ROOT" STATE="$STATE" CONFIRM_TTY="$STATE/answers" \
    PATH="$SHIMS:$PATH" "$BASH_BIN" "$P/deploy/remote.sh" "$@" > "$STATE/out" 2>&1
}
answer() { echo "$1" > "$STATE/answers"; }
out_has() { grep -qF -- "$1" "$STATE/out"; }
called() { grep -qF -- "$1" "$STATE/calls"; }
bin_is() { grep -q "# $1" "$ROOT/usr/local/bin/tempest-mqtt"; }

echo "0. every config sets every variable the unit passes as a flag"
# A missing key would reach the daemon as an empty flag value, not a default.
unit_vars="$(grep -o '\${[A-Z_]*}' "$SRC/deploy/tempest-mqtt.service" | tr -d '${}' | sort -u)"
check "the unit uses variables" test -n "$unit_vars"
for cfg in "$SRC"/configs/*.env "$SRC/deploy/tempest-mqtt.env.example"; do
  missing=""; for v in $unit_vars; do grep -q "^$v=" "$cfg" || missing="$missing $v"; done
  check "${cfg#"$SRC/"} has them all${missing:+ (missing:$missing)}" test -z "$missing"
done

echo "1. --check on a host without tempest-mqtt (not root)"
new_root check
run FAKE_UID=1000 -- --check install && pass "exits 0" || fail "failed: $(cat "$STATE/out")"
for p in "install /usr/local/bin/tempest-mqtt" "install /etc/tempest-mqtt/tempest-mqtt.env" "install /etc/systemd/system/tempest-mqtt.service" "enable at boot + (re)start tempest-mqtt" "site: schoolhouse (topics climate/schoolhouse/...)"; do
  check "plan: $p" out_has "$p"
done
check "nothing installed" not test -e "$ROOT/usr/local/bin/tempest-mqtt"
check "no service touched" not grep -qE 'systemctl (restart|enable)' "$STATE/calls"

echo "2. install"
new_root inst; answer y
run -- install && pass "exits 0" || fail "failed: $(cat "$STATE/out")"
check "binary installed" bin_is v1
check "config installed from the site's" cmp -s "$SRC/configs/schoolhouse.env" "$ROOT/etc/tempest-mqtt/tempest-mqtt.env"
# GNU stat first: on Linux, `stat -f` reports the filesystem and still prints.
check "config not world-readable" bash -c "[ \"\$(stat -c %a '$ROOT/etc/tempest-mqtt/tempest-mqtt.env' 2>/dev/null || stat -f %Lp '$ROOT/etc/tempest-mqtt/tempest-mqtt.env')\" = 640 ]"
check "system unit installed" cmp -s "$SRC/deploy/tempest-mqtt.service" "$ROOT/etc/systemd/system/tempest-mqtt.service"
check "daemon-reload, enable, restart" bash -c "grep -q 'daemon-reload' '$STATE/calls' && grep -q 'enable tempest-mqtt' '$STATE/calls' && grep -q 'restart tempest-mqtt' '$STATE/calls'"
check "verified enabled at boot" out_has "active, enabled at boot"
check "verified live wind on the local broker" called "mosquitto_sub -h 127.0.0.1 -p 1883 -t climate/schoolhouse/wind/rapid -C 1 -W 30"

echo "3. re-run: nothing to do"
: > "$STATE/calls"
run -- install && pass "exits 0" || fail "failed: $(cat "$STATE/out")"
check "nothing to do" out_has "installed, enabled and running"
check "no restart" not called "restart"
chmod 000 "$ROOT/etc/tempest-mqtt/tempest-mqtt.env"
run FAKE_UID=1000 -- --check install && pass "--check without root, root-only config: exits 0" || fail "--check, unreadable config: $(cat "$STATE/out")"
chmod 640 "$ROOT/etc/tempest-mqtt/tempest-mqtt.env"

echo "4. a unit disabled by hand is re-enabled"
rm "$STATE/svc-tempest-mqtt-enabled"
run -- --check install || true
check "plan: enable" out_has "enable at boot + (re)start tempest-mqtt"
run -- install && pass "exits 0" || fail "failed: $(cat "$STATE/out")"
check "enabled again" test -e "$STATE/svc-tempest-mqtt-enabled"

echo "5. upgrade keeps an edited config"
echo "LOG_LEVEL=debug  # edited on the host" >> "$ROOT/etc/tempest-mqtt/tempest-mqtt.env"
make_payload "$P" v2 schoolhouse; answer y; : > "$STATE/calls"
run -- install && pass "exits 0" || fail "failed: $(cat "$STATE/out")"
check "binary upgraded" bin_is v2
check "edited config kept" grep -q "edited on the host" "$ROOT/etc/tempest-mqtt/tempest-mqtt.env"
check "old binary backed up" bash -c "grep -q '# v1' '$ROOT'/var/backups/tempest-mqtt/*/usr/local/bin/tempest-mqtt"
check "restarted" called "restart tempest-mqtt"

echo "6. failures restore"
for mode in journal sub start enable; do
  new_root "fresh-$mode"; answer y
  if run FAIL_VERIFY="$mode" -- install; then fail "$mode (fresh): should fail"; continue; fi
  check "$mode (fresh): binary and unit removed" bash -c "[ ! -e '$ROOT/usr/local/bin/tempest-mqtt' ] && [ ! -e '$ROOT/etc/systemd/system/tempest-mqtt.service' ]"
  check "$mode (fresh): disabled" not test -e "$STATE/svc-tempest-mqtt-enabled"
  check "$mode (fresh): config kept" test -f "$ROOT/etc/tempest-mqtt/tempest-mqtt.env"
done
new_root upg-fail; answer y
run -- install || fail "first install failed"
make_payload "$P" v2 schoolhouse
if run FAIL_VERIFY=sub -- install; then fail "failed upgrade: should fail"; fi
check "failed upgrade: v1 back" bin_is v1
check "failed upgrade: restarted on v1" out_has "previous version restored and restarted"

echo "7. preflight refusals"
new_root arch; answer y
run FAIL_VERIFY=arch -- install && fail "wrong arch: should refuse" || { out_has "wrong architecture" && pass "binary that does not run refused"; }
new_root mosq; answer y; rm "$STATE/svc-mosquitto-active"
run -- install && fail "no mosquitto: should refuse" || { out_has "mosquitto is not running" && pass "no local mosquitto refused"; }
new_root useru; answer y
mkdir -p "$ROOT/home/sweeney/.config/systemd/user"; touch "$ROOT/home/sweeney/.config/systemd/user/tempest-mqtt.service"
run -- --check install && fail "per-user install: should refuse" || { out_has "a per-user tempest-mqtt is installed" && pass "per-user install present refused"; }
new_root port; answer y; touch "$STATE/port-busy"
run -- install && fail "busy port: should refuse" || { out_has "UDP port 50222 is already in use" && pass "port in use refused"; }
new_root site; answer y
run -- install || fail "first install failed"
sed -i.bak 's/^TOPIC_PREFIX=.*/TOPIC_PREFIX=home/' "$P/tempest-mqtt.env.example"
run -- --check install && fail "other site: should refuse" || { out_has "runs tempest-mqtt for 'schoolhouse', but this deploy is for 'home'" && pass "payload for another site refused"; }
check "refusals installed nothing" bash -c "! ls '$WORK'/root-{arch,mosq,useru,port}/usr/local/bin/tempest-mqtt 2>/dev/null"
new_root noroot; answer y
run FAKE_UID=1000 -- install && fail "not root: should refuse" || { out_has "install needs root" && pass "install without root refused"; }

echo "8. prompt"
new_root prompt; answer n
run -- install && fail "answer n: should exit non-zero" || pass "answer n: exits non-zero"
check "answer n: nothing installed" not test -e "$ROOT/usr/local/bin/tempest-mqtt"
rm -f "$STATE/answers"
run -- install && fail "no tty: should refuse" || { out_has "re-run with --yes" && pass "no tty, no --yes: refuses"; }
run -- --yes install && pass "--yes: installs without a prompt" || fail "--yes: $(cat "$STATE/out")"

echo "9. rollback"
new_root rb; answer y
run -- install || fail "install failed"
run -- --check rollback && out_has "remove /usr/local/bin/tempest-mqtt" && pass "--check lists the removal" || fail "rollback --check: $(cat "$STATE/out")"
check "--check removed nothing" test -e "$ROOT/usr/local/bin/tempest-mqtt"
run -- rollback && pass "rollback exits 0" || fail "rollback failed: $(cat "$STATE/out")"
check "binary and unit removed" bash -c "[ ! -e '$ROOT/usr/local/bin/tempest-mqtt' ] && [ ! -e '$ROOT/etc/systemd/system/tempest-mqtt.service' ]"
check "disabled" not test -e "$STATE/svc-tempest-mqtt-enabled"
check "config kept" test -f "$ROOT/etc/tempest-mqtt/tempest-mqtt.env"
run -- rollback && out_has "nothing to do" && pass "second rollback: nothing to do" || fail "second rollback: $(cat "$STATE/out")"

echo
if [ $fails -eq 0 ]; then echo "all passed"; else echo "$fails failed"; exit 1; fi
