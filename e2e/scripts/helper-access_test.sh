#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
TEST_DIR="$(mktemp -d)"
TEST_CHECKOUT="$TEST_DIR/private"
TEST_SHARED="$TEST_DIR/shared"
cleanup() {
    chmod 700 "$TEST_CHECKOUT" 2>/dev/null || true
    rm -rf -- "$TEST_DIR"
}
trap 'cleanup' EXIT
if [ "$(id -u)" -eq 0 ]; then
    echo 'Helper access regression requires a non-root test account' >&2
    exit 1
fi

mkdir -p "$TEST_CHECKOUT/e2e/scripts" "$TEST_SHARED/builder"
sed "s|/tmp/irgsh-e2e|$TEST_SHARED|g" "$SCRIPT_DIR/../run.sh" > "$TEST_CHECKOUT/e2e/run.sh"
sed "s|/tmp/irgsh-e2e|$TEST_SHARED|g" "$SCRIPT_DIR/native-builder.sh" > "$TEST_CHECKOUT/e2e/scripts/native-builder.sh"
printf 'left by interrupted run\n' > "$TEST_SHARED/builder/partial"

id() {
    case "$*" in
        -un) printf 'irgsh-builder-e2e\n' ;;
        -Gn) printf 'fixture-preflight-failure\n' ;;
        *) command id "$@" ;;
    esac
}
sudo() {
    [ "$#" -eq 9 ] && [ "$*" = "-n -H -u irgsh-builder-e2e -- bash $7 $8 $TEST_SHARED" ] || return 64
    local status=0
    chmod 000 "$TEST_CHECKOUT"
    bash "$7" "$8" "$9" || status=$?
    chmod 700 "$TEST_CHECKOUT"
    return "$status"
}
docker() {
    case "$*" in
        'compose --project-name e2e logs --tail=50'|'compose --project-name e2e down --volumes --remove-orphans'|'rmi irgsh-e2e') return 0 ;;
        *) echo "Unexpected Docker operation: $*" >&2; return 64 ;;
    esac
}
export TEST_CHECKOUT TEST_SHARED
export -f id sudo docker

STATUS=0
bash "$TEST_CHECKOUT/e2e/run.sh" > "$TEST_DIR/main.log" 2>&1 || STATUS=$?
if [ "$STATUS" -ne 1 ] || ! grep -q 'must have only its own group' "$TEST_DIR/main.log"; then
    cat "$TEST_DIR/main.log" >&2
    echo "FAIL: shared helper did not reach preflight (exit $STATUS)" >&2
    exit 1
fi
test -f "$TEST_SHARED/builder/partial"
cmp "$TEST_CHECKOUT/e2e/scripts/native-builder.sh" "$TEST_SHARED/native-builder.sh"
test "$(stat -c %a "$TEST_SHARED/native-builder.sh")" = 444
test "$(stat -c %u "$TEST_SHARED/native-builder.sh")" = "$(id -u)"
test "$(stat -c %a "$TEST_SHARED")" = 755
# shellcheck disable=SC2329
(
    id() {
        case "$*" in
            -un|-Gn) printf 'irgsh-builder-e2e\n' ;;
            *) command id "$@" ;;
        esac
    }
    sbuild() {
        if [ "$PWD" != "$TEST_SHARED/builder" ] || [ ! -d "$PWD" ]; then
            echo "Fixture sbuild rejected BUILD_DIR=$PWD" >&2
            return 1
        fi
        printf '%s\n' "$PWD" > "$TEST_SHARED/tool-cwd"
        return 1
    }
    mmdebstrap() { return 95; }
    dpkg() { return 95; }
    newuidmap() { return 95; }
    newgidmap() { return 95; }
    export -f id sbuild mmdebstrap dpkg newuidmap newgidmap
    for operation in check init-base check-config worker; do
        rm -f "$TEST_SHARED/tool-cwd"
        mkdir "$TEST_SHARED/deleted-cwd"
        STATUS=0
        (
            cd "$TEST_SHARED/deleted-cwd"
            rmdir "$TEST_SHARED/deleted-cwd"
            bash "$TEST_SHARED/native-builder.sh" "$operation" "$TEST_SHARED"
        ) > "$TEST_DIR/cwd.log" 2>&1 || STATUS=$?
        if [ "$STATUS" -ne 1 ] || [ ! -f "$TEST_SHARED/tool-cwd" ]; then
            cat "$TEST_DIR/cwd.log" >&2
            echo "FAIL: $operation did not enter the owned workdir before sbuild" >&2
            exit 1
        fi
        test "$(< "$TEST_SHARED/tool-cwd")" = "$TEST_SHARED/builder"
    done
)
printf 'stale copy\n' > "$TEST_SHARED/stale"
mv -f "$TEST_SHARED/stale" "$TEST_SHARED/native-builder.sh"
bash "$TEST_CHECKOUT/e2e/run.sh" --cleanup > "$TEST_DIR/cleanup.log" 2>&1 || {
    cat "$TEST_DIR/cleanup.log" >&2
    exit 1
}
test ! -e "$TEST_SHARED"
echo 'Shared native helper access tests passed'
