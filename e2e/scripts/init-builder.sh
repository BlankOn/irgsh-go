#!/usr/bin/env bash
# Initialize builder: create pbuilder base.tgz and build pbocker Docker image.
# Runs inside the irgsh-e2e container with Docker socket mounted.
set -euo pipefail

export IRGSH_CONFIG_PATH=/etc/irgsh/config.yaml

echo "=== Builder: init-base (pbuilder create) ==="
irgsh-builder init-base

echo "=== Builder: init-builder (pbocker Docker image) ==="
irgsh-builder init-builder

echo "=== Builder initialization complete ==="
