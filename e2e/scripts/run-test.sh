#!/usr/bin/env bash
# Run the CLI test inside a container on the irgsh Docker network.
# Submits a test package and polls until completion.
set -euo pipefail

GPG_KEY="$1"

echo "=== Configuring CLI ==="
mkdir -p /root/.irgsh/tmp
irgsh-cli config --chief http://chief:8080 --key "$GPG_KEY"

echo "=== Submitting test package ==="
# Use bromo-theme, the canonical test package from HACKING.md.
irgsh-cli package \
    --experimental \
    --source https://github.com/BlankOn/bromo-theme.git \
    --package https://github.com/BlankOn-packages/bromo-theme.git \
    --ignore-checks 2>&1 | tee /tmp/submit.log

# The CLI prints:
#   Submission succeeded. Pipeline ID:
#   <pipeline-id>
# Extract the line after "Pipeline ID:"
PIPELINE_ID=$(grep -A1 'Pipeline ID:' /tmp/submit.log | tail -1 | tr -d '[:space:]')

if [ -z "$PIPELINE_ID" ]; then
    echo "ERROR: Could not extract pipeline ID from submission output"
    cat /tmp/submit.log
    exit 1
fi

echo "=== Pipeline ID: $PIPELINE_ID ==="

# Poll for completion
MAX_WAIT=600  # 10 minutes
ELAPSED=0
INTERVAL=10

while [ $ELAPSED -lt $MAX_WAIT ]; do
    sleep $INTERVAL
    ELAPSED=$((ELAPSED + INTERVAL))

    STATUS=$(curl -sf "http://chief:8080/api/v1/status?uuid=$PIPELINE_ID" 2>/dev/null || echo '{}')
    STATE=$(echo "$STATUS" | grep -o '"state":"[^"]*"' | head -1 | cut -d'"' -f4)

    echo "[${ELAPSED}s] State: ${STATE:-unknown}"

    case "$STATE" in
        DONE)
            echo "=== Build completed successfully ==="
            echo "$STATUS"
            exit 0
            ;;
        FAILED)
            echo "=== Build FAILED ==="
            echo "$STATUS"
            echo ""
            echo "--- Build log ---"
            curl -sf "http://chief:8080/logs/${PIPELINE_ID}.build.log" 2>/dev/null || echo "(not available)"
            echo ""
            echo "--- Repo log ---"
            curl -sf "http://chief:8080/logs/${PIPELINE_ID}.repo.log" 2>/dev/null || echo "(not available)"
            exit 1
            ;;
    esac
done

echo "=== TIMEOUT after ${MAX_WAIT}s ==="
exit 1
