#!/usr/bin/env bash
set -euo pipefail

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
    for helper in newuidmap newgidmap; do
        helper="$(command -v "$helper")"
        if [ "$(stat -Lc %u "$helper")" != 0 ] || [ ! -u "$helper" ]; then
            echo 'Mapping helpers must be root-owned and setuid' >&2
            return 1
        fi
        stat -Lc '%n owner=%U mode=%a' "$helper"
    done
    for tool in /etc/subuid /etc/subgid; do
        awk -F: '$1 == "irgsh-builder-e2e" && $2 ~ /^[0-9]+$/ && $3 ~ /^[0-9]+$/ && $3 >= 65536 && $2 + $3 <= 4294967296 { print; found=1 } END { exit !found }' "$tool" || {
            echo "Missing valid builder subordinate range: $tool" >&2
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
    sbuild --version
    mmdebstrap --version
}

supervise_builder() (
    worker_pid=
    # shellcheck disable=SC2329
    stop_builder() {
        local status=$?
        trap - EXIT
        if [ -n "$worker_pid" ]; then
            kill -TERM "$worker_pid" 2>/dev/null || true
            wait "$worker_pid" || true
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
        check|init-base|worker)
            check_builder_host "$directory/builder"
            export IRGSH_CONFIG_PATH="$directory/builder-config.yaml"
            export PORT=18081
            cd "$directory/builder"
            case "$operation" in
                init-base) exec "$directory/bin/irgsh-builder" init-base ;;
                worker) supervise_builder "$directory/bin/irgsh-builder" ;;
            esac
            ;;
        *) echo "Unknown native builder operation: $operation" >&2; exit 1 ;;
    esac
fi
