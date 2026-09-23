#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"
PROJECT_ROOT="$(cd .. && pwd)"
E2E_DIR=/tmp/irgsh-e2e
IMAGE_NAME=irgsh-e2e
BUILDER_USER=irgsh-builder-e2e
COMPOSE=(docker compose --project-name e2e)
CONFIG_FILE="$E2E_DIR/config.yaml"
BUILDER_PID=
BUILDER_INPUT=
COPY_CONTAINER=
SKIP_INIT=false
CLEANUP=false

for arg in "$@"; do
    case "$arg" in
        --skip-init) SKIP_INIT=true ;;
        --cleanup) CLEANUP=true ;;
        *) echo "Unknown flag: $arg" >&2; exit 1 ;;
    esac
done

if [ "$(id -u)" -eq 0 ]; then
    echo 'Run the E2E orchestrator as a non-root account' >&2
    exit 1
fi

install -d -m 0755 "$E2E_DIR"
install -m 0444 "$SCRIPT_DIR/scripts/native-builder.sh" "$E2E_DIR/native-builder.sh"

native_builder() {
    sudo -n -H -u "$BUILDER_USER" -- bash "$E2E_DIR/native-builder.sh" "$1" "$E2E_DIR"
}

stop_builder() {
    local status=0
    if [ -n "$BUILDER_PID" ]; then
        exec {BUILDER_INPUT}>&-
        wait "$BUILDER_PID" || status=$?
        BUILDER_PID=
    fi
    return "$status"
}

# shellcheck disable=SC2329
finish() {
    local status=$?
    trap - EXIT
    stop_builder || status=$?
    if [ -n "$COPY_CONTAINER" ]; then
        docker rm "$COPY_CONTAINER" >/dev/null || status=$?
    fi
    if [ "$status" -ne 0 ]; then
        if [ -f "$E2E_DIR/builder.log" ]; then
            tail -n 200 "$E2E_DIR/builder.log"
        fi
        "${COMPOSE[@]}" logs --tail=50 || true
    fi
    exit "$status"
}
trap 'finish' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

if [ "$CLEANUP" = true ]; then
    "${COMPOSE[@]}" down --volumes --remove-orphans
    if [ -d "$E2E_DIR/builder" ]; then
        native_builder cleanup
    fi
    rm -rf -- "$E2E_DIR"
    docker rmi "$IMAGE_NAME" 2>/dev/null || true
    exit 0
fi

native_builder check
mkdir -p "$E2E_DIR/bin"
git -C "$PROJECT_ROOT" rev-parse HEAD

echo 'Building E2E image'
docker build -t "$IMAGE_NAME" -f Dockerfile "$PROJECT_ROOT"
COPY_CONTAINER=$(docker create "$IMAGE_NAME")
docker cp "$COPY_CONTAINER:/usr/local/bin/irgsh-builder" "$E2E_DIR/bin/irgsh-builder"
docker rm "$COPY_CONTAINER" >/dev/null
COPY_CONTAINER=
chmod 755 "$E2E_DIR/bin/irgsh-builder"
cp "$SCRIPT_DIR/builder-config.yaml.template" "$E2E_DIR/builder-config.yaml"

cp "$SCRIPT_DIR/config.yaml.template" "$CONFIG_FILE"
GPG_KEY=$("${COMPOSE[@]}" run --rm --no-deps -T \
    -v "$SCRIPT_DIR/scripts:/scripts:ro" \
    --entrypoint bash repo /scripts/setup-gpg.sh "$E2E_DIR/gnupg")
sed "s/dist_signing_key: 'PLACEHOLDER'/dist_signing_key: '$GPG_KEY'/" \
    "$SCRIPT_DIR/config.yaml.template" > "$CONFIG_FILE"

if [ "$SKIP_INIT" = false ]; then
    echo 'Initializing native builder base'
    native_builder init-base >"$E2E_DIR/builder.log" 2>&1
    echo 'Initializing repository'
    "${COMPOSE[@]}" run --rm --no-deps -T \
        -v "$SCRIPT_DIR/scripts:/scripts:ro" \
        --entrypoint bash repo /scripts/init-repo.sh
fi

native_builder check-config
"${COMPOSE[@]}" up -d
for ((attempt=1; attempt<=30; attempt++)); do
    if curl -sf http://127.0.0.1:8080/api/v1/version >/dev/null; then
        break
    fi
    if [ "$attempt" -eq 30 ]; then
        echo 'Chief did not become healthy in time' >&2
        exit 1
    fi
    sleep 2
done

coproc BUILDER_SUPERVISOR { native_builder worker >>"$E2E_DIR/builder.log" 2>&1; }
BUILDER_PID=$BUILDER_SUPERVISOR_PID
BUILDER_INPUT=${BUILDER_SUPERVISOR[1]}
for ((attempt=1; attempt<=30; attempt++)); do
    kill -0 "$BUILDER_PID" 2>/dev/null || { echo 'Native builder exited before readiness' >&2; exit 1; }
    if curl -sf http://127.0.0.1:18081/api/v1/version >/dev/null; then
        break
    fi
    if [ "$attempt" -eq 30 ]; then
        echo 'Native builder did not become healthy in time' >&2
        exit 1
    fi
    sleep 2
done

TEST_EXIT=0
docker run --rm \
    --network e2e_irgsh \
    -v "e2e_gnupg:$E2E_DIR/gnupg" \
    -v "$SCRIPT_DIR/scripts:/scripts:ro" \
    -e "GNUPGHOME=$E2E_DIR/gnupg" \
    "$IMAGE_NAME" bash /scripts/run-test.sh "$GPG_KEY" || TEST_EXIT=$?
stop_builder || TEST_EXIT=$?
native_builder check || TEST_EXIT=$?
if [ "$TEST_EXIT" -eq 0 ]; then
    echo 'E2E test PASSED'
else
    echo "E2E test FAILED (exit code: $TEST_EXIT)" >&2
fi
exit "$TEST_EXIT"
