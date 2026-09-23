#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
grep -q 'runs-on: ubuntu-24.04' "$SCRIPT_DIR/../../.github/workflows/e2e.yaml"
grep -q 'apt-get install -y -t noble-backports sbuild' "$SCRIPT_DIR/../../.github/workflows/e2e.yaml"
source "$SCRIPT_DIR/native-builder.sh"
(
    sbuild() { printf 'sbuild (Debian sbuild) %s (test fixture)\n' "$TEST_VERSION"; }
    dpkg() {
        [ "$*" = "--compare-versions $TEST_VERSION ge 0.87.0" ] || return 64
        [ "$TEST_VERSION" != 0.85.10ubuntu0.3 ]
    }
    TEST_VERSION=0.85.10ubuntu0.3
    if check_sbuild_version; then
        echo 'FAIL: old sbuild version accepted' >&2
        exit 1
    fi
    TEST_VERSION=0.87.0
    check_sbuild_version
)
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_DIR"' EXIT

check_mapping() {
    local expected="$1" rows="$2" status=0 output
    printf '%s\n' "$rows" > "$TEST_DIR/subids"
    output=$(check_subordinate_range "$TEST_DIR/subids") || status=$?
    if [ "$expected" = valid ]; then
        [ "$status" -eq 0 ] && [ -n "$output" ] || { echo "FAIL: valid mapping rejected: $rows" >&2; exit 1; }
    else
        [ "$status" -ne 0 ] && [ -z "$output" ] || { echo "FAIL: invalid mapping accepted: $rows" >&2; exit 1; }
    fi
}

check_mapping valid 'irgsh-builder-e2e:4294901759:65536'
check_mapping valid $'irgsh-builder-e2e:100000:65536\nirgsh-builder-e2e:bad:1'
check_mapping valid $'irgsh-builder-e2e-other:100000:1\nirgsh-builder-e2e:200000:65536'
for first in \
    'irgsh-builder-e2e:4294901760:65536' \
    'irgsh-builder-e2e:4294967296:65536' \
    'irgsh-builder-e2e:1:18446744073709551615' \
    'irgsh-builder-e2e:100000:65535' \
    'irgsh-builder-e2e:bad:65536' \
    'irgsh-builder-e2e: 100000:65536' \
    'irgsh-builder-e2e:100000:65536 ' \
    'irgsh-builder-e2e:100000:65536:extra' \
    'irgsh-builder-e2e'; do
    check_mapping invalid "$first"
    check_mapping invalid "$first"$'\nirgsh-builder-e2e:200000:65536'
done
check_mapping invalid 'irgsh-builder-e2e-other:100000:65536'

(
    sbuild() {
        [ "${SBUILD_CONFIG:-}" = "$TEST_DIR/test.conf" ] || return 64
        return "$PARSER_STATUS"
    }
    printf '1;\n' > "$TEST_DIR/test.conf"
    PARSER_STATUS=42
    STATUS=0
    check_sbuild_config "$TEST_DIR/test.conf" || STATUS=$?
    test "$STATUS" -eq 42
    PARSER_STATUS=0
    check_sbuild_config "$TEST_DIR/test.conf"
    if [ "$(id -u)" -ne 0 ]; then
        chmod 000 "$TEST_DIR/test.conf"
        STATUS=0
        check_sbuild_config "$TEST_DIR/test.conf" || STATUS=$?
        chmod 600 "$TEST_DIR/test.conf"
        test "$STATUS" -ne 0
    fi
)

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

for outcome in default 42; do
    rm -f "$TEST_DIR/ready"
    coproc TERM_TEST {
        # shellcheck disable=SC2016
        supervise_builder bash -c 'if [ "$2" != default ]; then trap "exit $2" TERM; fi; printf ready > "$1/ready"; while :; do sleep 0.1; done' bash "$TEST_DIR" "$outcome"
    }
    TEST_PID=$TERM_TEST_PID
    TEST_INPUT=${TERM_TEST[1]}
    for ((attempt=0; attempt<100; attempt++)); do
        [ ! -f "$TEST_DIR/ready" ] || break
        sleep 0.05
    done
    test -f "$TEST_DIR/ready"
    exec {TEST_INPUT}>&-
    STATUS=0
    wait "$TEST_PID" || STATUS=$?
    if [ "$outcome" = default ]; then
        test "$STATUS" -eq 0
    else
        test "$STATUS" -eq 42
    fi
done

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
