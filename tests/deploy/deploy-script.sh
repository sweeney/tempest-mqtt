#!/usr/bin/env bash
# Tests deploy/deploy.sh with shims for ssh and go: which host each site goes
# to, the GOARCH picked from the host's `uname -m`, the config staged for the
# site, sudo only outside --check, and the refusals. The sites are
# deploy/sites.example's, not your local deploy/sites. Nothing leaves this
# machine.
set -euo pipefail

HERE="$(cd "$(dirname "$0")" && pwd)"
SRC="$HERE/../.."
WORK="$(mktemp -d "${TMPDIR:-/tmp}/tempest-deploy-test.XXXXXX")"
fails=0
trap '[ "$fails" -gt 0 ] && echo "work dir kept: $WORK" || rm -rf "$WORK"' EXIT
pass() { echo "  PASS  $*"; }
fail() { echo "  FAIL  $*"; fails=$((fails + 1)); }
check() { local d="$1"; shift; if "$@"; then pass "$d"; else fail "$d"; fi; }

SHIMS="$WORK/shims"; mkdir -p "$SHIMS"
# ssh: records the host and command; plays a host whose arch is FAKE_ARCH.
# remote.sh is not run, but what it would get is copied out for inspection.
cat > "$SHIMS/ssh" <<'EOF'
#!/usr/bin/env bash
set -u
tty=""
while [ $# -gt 0 ]; do
  case "$1" in -o) shift 2 ;; -t) tty="-t "; shift ;; *) break ;; esac
done
host="$1"; shift
echo "ssh ${tty}$host $*" >> "$T/calls"
[ "${FAKE_SSH_FAIL:-}" = 1 ] && exit 255
case "$1" in
  true) exit 0 ;;
  uname) echo "$FAKE_ARCH" ;;
  mktemp) mkdir -p "$T/remote"; mktemp -d "$T/remote/tempest-mqtt.XXXXXX" ;;
  tar) tar "${@:2}" ;;
  rm) exit 0 ;;
  bash|sudo)
    for a in "$@"; do case "$a" in */remote.sh) dir="$(dirname "$(dirname "$a")")" ;; esac; done
    cp "$dir/tempest-mqtt.env.example" "$T/staged-config.env"
    cp "$dir/tempest-mqtt" "$T/staged-bin"
    cp "$dir/deploy/tempest-mqtt.service" "$T/staged-unit" ;;
esac
EOF
# go: records GOARCH and writes a placeholder binary to -o.
cat > "$SHIMS/go" <<'EOF'
#!/usr/bin/env bash
echo "go GOOS=$GOOS GOARCH=$GOARCH $*" >> "$T/calls"
while [ $# -gt 0 ]; do [ "$1" = -o ] && { echo "tempest-mqtt for $GOARCH" > "$2"; }; shift; done
EOF
chmod +x "$SHIMS/ssh" "$SHIMS/go"

run() {  # run NAME ARCH -- deploy.sh args
  T="$WORK/$1"; rm -rf "$T"; mkdir -p "$T"; : > "$T/calls"
  local arch="$2"; shift 3
  env T="$T" FAKE_ARCH="$arch" ${FAKE_SSH_FAIL:+FAKE_SSH_FAIL=$FAKE_SSH_FAIL} PATH="$SHIMS:$PATH" \
    TEMPEST_SITES="${SITES_FILE:-$SRC/deploy/sites.example}" \
    bash "$SRC/deploy/deploy.sh" "$@" > "$T/out" 2>&1
}
called() { grep -qF -- "$1" "$T/calls"; }
out_has() { grep -qF -- "$1" "$T/out"; }

echo "1. --check: arm64 host, read-only"
run check aarch64 -- --site schoolhouse --check && pass "exits 0" || fail "failed: $(cat "$T/out")"
check "built for arm64" called "go GOOS=linux GOARCH=arm64"
check "to the site's host" called "ssh pi@100.64.0.10 uname"
check "remote.sh run without sudo, with --check" called "ssh pi@100.64.0.10 bash $T/remote/"
check "no sudo" bash -c "! grep -q 'sudo' '$T/calls'"
check "--check passed on" grep -q 'remote.sh --check' "$T/calls"
check "site config staged" cmp -s "$SRC/configs/schoolhouse.env" "$T/staged-config.env"
check "system unit staged" cmp -s "$SRC/deploy/tempest-mqtt.service" "$T/staged-unit"
check "the built binary staged" grep -q "for arm64" "$T/staged-bin"

echo "2. install: amd64 host, under sudo with a tty"
run inst x86_64 -- --site schoolhouse --yes && pass "exits 0" || fail "failed: $(cat "$T/out")"
check "built for amd64" called "go GOOS=linux GOARCH=amd64"
check "sudo with a tty" called "ssh -t pi@100.64.0.10 sudo bash"
check "--yes passed on" grep -q 'remote.sh --yes' "$T/calls"

echo "3. --host overrides; rollback passed through"
run host aarch64 -- --site schoolhouse --host me@example rollback --check || fail "failed: $(cat "$T/out")"
check "to --host" called "ssh me@example uname"
check "rollback passed on" grep -q 'remote.sh rollback --check' "$T/calls"

echo "4. refusals"
run nosite aarch64 -- --check && fail "no --site: should refuse" || { out_has "--site NAME is required (sites: schoolhouse)" && pass "no --site refused, sites listed"; }
run badsite aarch64 -- --site nowhere --check && fail "unknown site: should refuse" || { out_has "no config for site 'nowhere'" && pass "unknown site refused"; }
SITES_FILE="$WORK/empty-sites"; : > "$SITES_FILE"
run notlisted aarch64 -- --site schoolhouse --check && fail "unlisted site: should refuse" || { out_has "is not in" && pass "site missing from sites file refused"; }
SITES_FILE=""
run arch armv7l -- --site schoolhouse --check && fail "armv7: should refuse" || { out_has "is armv7l" && pass "unsupported arch refused"; }
FAKE_SSH_FAIL=1 run nossh aarch64 -- --site schoolhouse --check && fail "no ssh: should refuse" || { out_has "cannot ssh to" && pass "unreachable host refused"; }
check "nothing built for refusals" bash -c "! grep -qh '^go ' '$WORK'/{nosite,badsite,notlisted,arch,nossh}/calls"

echo
if [ $fails -eq 0 ]; then echo "all passed"; else echo "$fails failed"; exit 1; fi
