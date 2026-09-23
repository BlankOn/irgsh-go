#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/native-builder.sh"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_DIR"' EXIT

mkdir "$TEST_DIR/owned"
check_builder_directory "$TEST_DIR/owned"
ln -s "$TEST_DIR/owned" "$TEST_DIR/link"
if check_builder_directory "$TEST_DIR/link"; then
    echo 'FAIL: symlink workdir accepted' >&2
    exit 1
fi

if [ "$(id -u)" -ne 0 ]; then
    mkdir "$TEST_DIR/owned/unreadable"
    chmod 000 "$TEST_DIR/owned/unreadable"
    STATUS=0
    check_builder_directory "$TEST_DIR/owned" || STATUS=$?
    chmod 700 "$TEST_DIR/owned/unreadable"
    test "$STATUS" -ne 0
fi

coproc STOP_TEST {
    # shellcheck disable=SC2016
    supervise_builder bash -c 'trap '\''printf stopped > "$1/stopped"; exit 0'\'' TERM; printf ready > "$1/ready"; while :; do sleep 0.1; done' bash "$TEST_DIR"
}
TEST_PID=$STOP_TEST_PID
TEST_INPUT=${STOP_TEST[1]}
for ((attempt=0; attempt<100; attempt++)); do
    [ ! -f "$TEST_DIR/ready" ] || break
    sleep 0.05
done
test -f "$TEST_DIR/ready"
exec {TEST_INPUT}>&-
wait "$TEST_PID"
test -f "$TEST_DIR/stopped"

coproc FAIL_TEST { supervise_builder bash -c 'exit 42'; }
TEST_PID=$FAIL_TEST_PID
TEST_INPUT=${FAIL_TEST[1]}
STATUS=0
wait "$TEST_PID" || STATUS=$?
exec {TEST_INPUT}>&-
test "$STATUS" -eq 42

if [ "$(id -un)" != irgsh-builder-e2e ]; then
    if bash "$SCRIPT_DIR/native-builder.sh" check /tmp/irgsh-e2e >"$TEST_DIR/preflight.log" 2>&1; then
        echo 'FAIL: wrong builder account accepted' >&2
        exit 1
    fi
    grep -q 'must run as irgsh-builder-e2e' "$TEST_DIR/preflight.log"
fi

echo 'Native builder supervisor tests passed'
