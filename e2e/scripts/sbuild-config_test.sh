#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
source "$SCRIPT_DIR/native-builder.sh"
TEST_DIR="$(mktemp -d)"
trap 'rm -rf "$TEST_DIR"' EXIT

check_sbuild_version
# shellcheck disable=SC2016
printf '%s\n' '$unshare_mmdebstrap_auto_create = 0;' '$unshare_tmpdir_template = '\''/tmp/tmp.sbuild.XXXXXXXXXX'\'';' '1;' > "$TEST_DIR/valid.conf"
check_sbuild_config "$TEST_DIR/valid.conf"
# shellcheck disable=SC2016
printf '%s\n' '$irgsh_unknown_sbuild_option = 1;' '1;' > "$TEST_DIR/invalid.conf"
if check_sbuild_config "$TEST_DIR/invalid.conf"; then
    echo 'FAIL: sbuild accepted an unsupported configuration option' >&2
    exit 1
fi
if check_sbuild_config "$TEST_DIR/missing.conf"; then
    echo 'FAIL: missing sbuild configuration accepted' >&2
    exit 1
fi
echo 'Real sbuild configuration tests passed'
