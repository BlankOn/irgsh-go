#!/usr/bin/env bash
# Generate a passphraseless GPG key for e2e testing.
# Outputs the key fingerprint to stdout.
set -euo pipefail

GNUPG_DIR="${1:-/tmp/irgsh-e2e/gnupg}"

mkdir -p "$GNUPG_DIR"
chmod 700 "$GNUPG_DIR"

# Configure GPG for headless (no TTY) usage
cat > "$GNUPG_DIR/gpg.conf" <<CONF
no-tty
batch
pinentry-mode loopback
CONF

cat > "$GNUPG_DIR/gpg-agent.conf" <<CONF
allow-loopback-pinentry
allow-preset-passphrase
CONF

# If a key already exists, reuse it
EXISTING=$(GNUPGHOME="$GNUPG_DIR" gpg --list-keys --with-colons 2>/dev/null \
    | grep '^fpr:' | head -1 | cut -d: -f10 || true)
if [ -n "$EXISTING" ]; then
    echo "$EXISTING"
    exit 0
fi

# Generate a new key
GNUPGHOME="$GNUPG_DIR" gpg --batch --gen-key <<EOF
%no-protection
Key-Type: RSA
Key-Length: 2048
Subkey-Type: RSA
Subkey-Length: 2048
Name-Real: IRGSH E2E Test
Name-Email: e2e@irgsh.test
Expire-Date: 0
%commit
EOF

# Extract fingerprint
FPR=$(GNUPGHOME="$GNUPG_DIR" gpg --list-keys --with-colons 2>/dev/null \
    | grep '^fpr:' | head -1 | cut -d: -f10)

echo "$FPR"
