#!/usr/bin/env bash
# Initialize repo: set up reprepro repository.
# Runs inside the irgsh-e2e container with GNUPG dir mounted.
set -euo pipefail

echo "=== Repo: initializing reprepro repository ==="

# InitRepo() prompts for confirmation; pipe 'y' to accept.
echo y | irgsh-repo --config /etc/irgsh/config.yaml init

echo "=== Repo initialization complete ==="
