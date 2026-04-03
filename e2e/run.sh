#!/usr/bin/env bash
# E2E test orchestrator for irgsh-go.
# Runs the full HACKING.md workflow inside Docker containers.
#
# Usage:
#   ./e2e/run.sh              # full run (build image, init, test)
#   ./e2e/run.sh --skip-init  # skip builder/repo init (reuse previous)
#   ./e2e/run.sh --cleanup    # remove all e2e data and images
#
# Prerequisites:
#   - Docker and docker compose
#   - Host Docker socket at /var/run/docker.sock
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
cd "$SCRIPT_DIR"
PROJECT_ROOT="$(cd .. && pwd)"
E2E_DIR="/tmp/irgsh-e2e"
IMAGE_NAME="irgsh-e2e"
COMPOSE="docker compose"
CONFIG_TEMPLATE="$SCRIPT_DIR/config.yaml.template"
CONFIG_FILE="$SCRIPT_DIR/config.yaml"

SKIP_INIT=false
CLEANUP=false

for arg in "$@"; do
    case "$arg" in
        --skip-init) SKIP_INIT=true ;;
        --cleanup)   CLEANUP=true ;;
        *)           echo "Unknown flag: $arg"; exit 1 ;;
    esac
done

cleanup() {
    echo "=== Stopping services ==="
    $COMPOSE down --remove-orphans 2>/dev/null || true
    echo "=== Removing e2e data ==="
    # Some files are root-owned (created by pbuilder inside Docker).
    # Use a container to remove them so cleanup works without sudo.
    if [ -d "$E2E_DIR" ]; then
        docker run --rm -v "$E2E_DIR:$E2E_DIR" "$IMAGE_NAME" rm -rf "$E2E_DIR" 2>/dev/null \
            || rm -rf "$E2E_DIR"
    fi
    echo "=== Removing Docker image ==="
    docker rmi "$IMAGE_NAME" 2>/dev/null || true
    docker rmi pbocker 2>/dev/null || true
    echo "=== Cleanup complete ==="
}

if [ "$CLEANUP" = true ]; then
    cleanup
    exit 0
fi

# ---- Step 1: Build Docker image ----
echo "=== Step 1: Building Docker image ==="
docker build -t "$IMAGE_NAME" -f Dockerfile "$PROJECT_ROOT"

# ---- Step 2: Create data directories ----
echo "=== Step 2: Preparing data directories ==="
mkdir -p "$E2E_DIR"/{chief,builder,repo,gnupg,iso}

# ---- Step 3: Generate GPG key ----
echo "=== Step 3: Generating GPG key ==="
GPG_KEY=$(docker run --rm \
    -v "$E2E_DIR/gnupg:$E2E_DIR/gnupg" \
    -v "$(pwd)/scripts:/scripts:ro" \
    "$IMAGE_NAME" \
    bash /scripts/setup-gpg.sh "$E2E_DIR/gnupg")

echo "GPG Key: $GPG_KEY"

# ---- Step 4: Generate config from template with GPG key ----
echo "=== Step 4: Generating config with GPG key ==="
sed "s/dist_signing_key: 'PLACEHOLDER'/dist_signing_key: '$GPG_KEY'/" \
    "$CONFIG_TEMPLATE" > "$CONFIG_FILE"

# ---- Step 5: Initialize builder and repo ----
if [ "$SKIP_INIT" = false ]; then
    echo "=== Step 5a: Initializing builder (this takes a while) ==="
    docker run --rm \
        --privileged \
        -v "$CONFIG_FILE:/etc/irgsh/config.yaml:ro" \
        -v "$(pwd)/scripts:/scripts:ro" \
        -v "$E2E_DIR/builder:$E2E_DIR/builder" \
        -v /var/run/docker.sock:/var/run/docker.sock \
        "$IMAGE_NAME" \
        bash /scripts/init-builder.sh \
    || { echo "Builder init failed (see above). You may retry with: $0"; exit 1; }

    echo "=== Step 5b: Initializing repo ==="
    docker run --rm \
        -v "$CONFIG_FILE:/etc/irgsh/config.yaml:ro" \
        -v "$(pwd)/scripts:/scripts:ro" \
        -v "$E2E_DIR/repo:$E2E_DIR/repo" \
        -v "$E2E_DIR/gnupg:$E2E_DIR/gnupg" \
        -e "GNUPGHOME=$E2E_DIR/gnupg" \
        "$IMAGE_NAME" \
        bash /scripts/init-repo.sh
else
    echo "=== Step 5: Skipping init (--skip-init) ==="
fi

# ---- Step 6: Start services ----
echo "=== Step 6: Starting services ==="
$COMPOSE up -d

echo "Waiting for chief to be healthy..."
for i in $(seq 1 30); do
    if $COMPOSE exec -T chief curl -sf http://localhost:8080/api/v1/version >/dev/null 2>&1; then
        echo "Chief is ready."
        break
    fi
    if [ "$i" -eq 30 ]; then
        echo "ERROR: Chief did not become healthy in time"
        $COMPOSE logs chief
        exit 1
    fi
    sleep 2
done

# Give builder and repo workers a moment to connect to Redis
sleep 5

echo "=== Services running ==="
$COMPOSE ps

# ---- Step 7: Run test ----
echo "=== Step 7: Running CLI test ==="
TEST_EXIT=0
docker run --rm \
    --network e2e_irgsh \
    -v "$E2E_DIR/gnupg:$E2E_DIR/gnupg" \
    -v "$(pwd)/scripts:/scripts:ro" \
    -e "GNUPGHOME=$E2E_DIR/gnupg" \
    "$IMAGE_NAME" \
    bash /scripts/run-test.sh "$GPG_KEY" \
    || TEST_EXIT=$?

# ---- Step 8: Show results ----
echo ""
echo "=== Results ==="
if [ $TEST_EXIT -eq 0 ]; then
    echo "E2E test PASSED"
else
    echo "E2E test FAILED (exit code: $TEST_EXIT)"
    echo ""
    echo "Service logs:"
    $COMPOSE logs --tail=50
fi

echo ""
echo "Services are still running. To stop:"
echo "  cd e2e && docker compose down"
echo ""
echo "To view dashboard: http://localhost:8080"
echo "To view repo:      http://localhost:8082/experimental/"

exit $TEST_EXIT
