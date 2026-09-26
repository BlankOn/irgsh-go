#!/usr/bin/env bash
set -euo pipefail

check_sbuild_version() {
    local output version
    output=$(sbuild --version) || return 1
    version=$(sed -n 's/^sbuild (Debian sbuild) \([^ ]*\).*/\1/p' <<< "$output")
    if [ -z "$version" ] || ! dpkg --compare-versions "$version" ge 0.87.0; then
        echo 'sbuild >= 0.87.0 is required for unshare_mmdebstrap_auto_create' >&2
        return 1
    fi
    printf '%s\n' "$output"
}

check_sbuild_config() {
    if [ ! -f "$1" ] || [ ! -r "$1" ]; then
        echo "Missing or unreadable generated sbuild configuration: $1" >&2
        return 1
    fi
    SBUILD_CONFIG="$1" sbuild --version >/dev/null
}

check_builder_directory() {
    local directory="$1" foreign
    if [ -L "$directory" ] || [ ! -d "$directory" ] || [ "$(stat -c %u "$directory")" != "$(id -u)" ]; then
        echo "Builder workdir must be an owned directory: $directory" >&2
        return 1
    fi
    foreign=$(find "$directory" ! -uid "$(id -u)" -print -quit) || return 1
    if [ -n "$foreign" ]; then
        echo 'Builder workdir contains foreign-owned output; inspect it before cleanup' >&2
        return 1
    fi
}

check_subordinate_range() {
    awk -F: '$1 == "irgsh-builder-e2e" {
        valid = NF == 3 && $2 ~ /^[0-9]+$/ && $3 ~ /^[0-9]+$/ && $3 >= 65536 && $2 <= 4294967294 && $3 - 1 <= 4294967294 - $2
        if (valid) print
        exit
    } END { exit !valid }' "$1"
}

check_builder_host() {
    local directory="$1" tool helper socket
    if [ "$(id -u)" -eq 0 ] || [ "$(id -un)" != irgsh-builder-e2e ]; then
        echo 'Native E2E builder must run as irgsh-builder-e2e' >&2
        return 1
    fi
    if [ "$(id -Gn)" != irgsh-builder-e2e ] || [ "$(id -g)" -eq 0 ]; then
        echo 'Native E2E builder must have only its own group' >&2
        return 1
    fi
    check_builder_directory "$directory"
    for tool in sbuild mmdebstrap dpkg newuidmap newgidmap; do
        command -v "$tool" >/dev/null || { echo "Missing native builder prerequisite: $tool" >&2; return 1; }
    done
    check_sbuild_version
    for helper in newuidmap newgidmap; do
        helper="$(command -v "$helper")"
        if [ "$(stat -Lc %u "$helper")" != 0 ] || [ ! -u "$helper" ]; then
            echo 'Mapping helpers must be root-owned and setuid' >&2
            return 1
        fi
        stat -Lc '%n owner=%U mode=%a' "$helper"
    done
    for tool in /etc/subuid /etc/subgid; do
        check_subordinate_range "$tool" || {
            echo "The first irgsh-builder-e2e row in $tool must have exactly three fields and at least 65536 IDs ending at or below 4294967294" >&2
            return 1
        }
    done
    for socket in /run/docker.sock /run/containerd/containerd.sock /run/podman/podman.sock /run/user/"$(id -u)"/{docker.sock,podman/podman.sock}; do
        if [ -r "$socket" ] || [ -w "$socket" ]; then
            echo "Native builder can access a runtime socket: $socket" >&2
            return 1
        fi
    done
    if [ -n "${DOCKER_HOST:-}${CONTAINER_HOST:-}" ]; then
        echo 'Native builder must not inherit a container runtime endpoint' >&2
        return 1
    fi
    cat /etc/os-release
    uname -sr
    id
    mmdebstrap --version
}

supervise_builder() (
    worker_pid=
    # shellcheck disable=SC2329
    stop_builder() {
        local status=$? worker_status=0
        trap - EXIT
        if [ -n "$worker_pid" ]; then
            kill -TERM "$worker_pid" 2>/dev/null || true
            wait "$worker_pid" || worker_status=$?
            if [ "$status" -eq 0 ] && [ "$worker_status" -ne 143 ]; then
                status=$worker_status
            fi
        fi
        exit "$status"
    }
    trap 'stop_builder' EXIT
    trap 'exit 130' INT
    trap 'exit 143' TERM
    "$@" &
    worker_pid=$!
    while kill -0 "$worker_pid" 2>/dev/null; do
        if read -r -t 1; then
            return 0
        elif [ "$?" -eq 1 ]; then
            return 0
        fi
    done
    wait "$worker_pid"
)

if [ "${BASH_SOURCE[0]}" = "$0" ]; then
    operation="${1:?operation required}"
    directory="${2:?E2E directory required}"
    [ "$directory" = /tmp/irgsh-e2e ] || { echo 'Unexpected E2E directory' >&2; exit 1; }
    if [ "$(id -u)" -eq 0 ] || [ "$(id -un)" != irgsh-builder-e2e ]; then
        echo 'Native E2E builder must run as irgsh-builder-e2e' >&2
        exit 1
    fi
    check_builder_directory "$directory/builder"
    case "$operation" in
        cleanup) find "$directory/builder" -mindepth 1 -maxdepth 1 -exec rm -rf -- {} + ;;
        check|init-base|check-config|worker)
            cd "$directory/builder"
            check_builder_host "$directory/builder"
            export IRGSH_CONFIG_PATH="$directory/builder-config.yaml"
            export PORT=18081
            case "$operation" in
                init-base) exec "$directory/bin/irgsh-builder" init-base ;;
                check-config)
                    for config in "$directory"/builder/bases/*/sbuild.conf; do
                        check_sbuild_config "$config"
                    done
                    ;;
                worker) supervise_builder "$directory/bin/irgsh-builder" ;;
            esac
            ;;
        *) echo "Unknown native builder operation: $operation" >&2; exit 1 ;;
    esac
fi
