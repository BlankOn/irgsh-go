#!/usr/bin/env bash

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
source "$root/utils/deploy/irgsh-deploy-request"
source "$root/utils/deploy/irgsh-deploy"

tests=0
sandbox=$(mktemp -d)
trap 'rm -rf "$sandbox"' EXIT

fail() {
	printf 'FAIL: %s\n' "$*" >&2
	exit 1
}

assert_equal() {
	[[ "$1" == "$2" ]] || fail "got <$1>, want <$2>"
}

assert_fails() {
	if "$@" >/dev/null 2>&1; then
		fail "command succeeded: $*"
	fi
}

test_request_parser() {
	assert_equal "$(parse_request 'deploy chief v2.3.2')" $'chief\nv2.3.2'
	assert_fails parse_request 'deploy repo v2.3.2;id'
	assert_fails parse_request 'deploy ../repo v2.3.2'
	assert_fails parse_request 'deploy repo https://example.invalid/a'
	assert_fails parse_request 'deploy repo v2.3.2 plus'
	assert_fails parse_request $'deploy repo v2.3.2\nid'
	assert_fails main unexpected
	sudo_deploy() {
		printf 'argc=%d\n' "$#"
		/usr/bin/cat
	}
	assert_equal "$(run_request 'deploy repo v2.3.2')" $'argc=0\nrepo\nv2.3.2'
	((tests += 1))
}

file_uid() {
	printf '%s\n' "${test_uid:-0}"
}

file_mode() {
	printf '%s\n' "${test_mode:-600}"
}

write_target() {
	local path=$1
	local contents=$2
	printf '%s' "$contents" > "$path"
}

test_target_validation() {
	local target="$sandbox/target"
	write_target "$target" $'irgsh-repo@verbeek.service http://127.0.0.1:8082/api/v1/version\nirgsh-repo@rani.service http://[::1]:8083/api/v1/version\n'
	load_targets repo "$target"
	assert_equal "${target_units[*]}" 'irgsh-repo@verbeek.service irgsh-repo@rani.service'
	assert_equal "${target_urls[*]}" 'http://127.0.0.1:8082/api/v1/version http://[::1]:8083/api/v1/version'
	ln -s "$target" "$sandbox/target-link"
	assert_fails load_targets repo "$sandbox/target-link"
	test_uid=1000
	assert_fails load_targets repo "$target"
	test_uid=0
	test_mode=620
	assert_fails load_targets repo "$target"
	test_mode=600
	write_target "$target" $'redis.service http://127.0.0.1:8082/api/v1/version\n'
	assert_fails load_targets repo "$target"
	write_target "$target" $'irgsh-repo.service https://example.invalid/api/v1/version\n'
	assert_fails load_targets repo "$target"
	write_target "$target" $'irgsh-repo.service http://127.0.0.1:0/api/v1/version\n'
	assert_fails load_targets repo "$target"
	((tests += 1))
}

make_release_dir() {
	local component=$1
	local version=$2
	local dir=$3
	mkdir -p "$dir/bin"
	printf '#!/usr/bin/env bash\nprintf "irgsh-go version %s\\n"\n' "$version" > "$dir/bin/irgsh-$component"
	chmod 0755 "$dir/bin/irgsh-$component"
	if [[ "$component" == iso ]]; then
		mkdir "$dir/share"
		printf '#!/usr/bin/env bash\nexit 0\n' > "$dir/share/iso-build.sh"
		chmod 0755 "$dir/share/iso-build.sh"
	fi
}

make_archive() {
	local component=$1
	local version=$2
	local archive=$3
	local extra=${4:-}
	local dir="$sandbox/archive-$component-$version-${RANDOM}"
	make_release_dir "$component" "$version" "$dir"
	if [[ -n "$extra" ]]; then
		printf 'unexpected\n' > "$dir/$extra"
	fi
	if [[ "$component" == iso ]]; then
		tar -C "$dir" -czf "$archive" bin share ${extra:+"$extra"}
	else
		tar -C "$dir" -czf "$archive" bin ${extra:+"$extra"}
	fi
}

test_archive_validation() {
	local archive="$sandbox/repo.tar.gz"
	make_archive repo 2.3.2 "$archive"
	validate_archive repo "$archive"
	local extracted="$sandbox/extracted"
	mkdir "$extracted"
	tar -xzf "$archive" -C "$extracted"
	assert_equal "$(binary_release_version "$extracted/bin/irgsh-repo")" '2.3.2'
	make_archive repo 2.3.2 "$sandbox/unexpected.tar.gz" extra
	assert_fails validate_archive repo "$sandbox/unexpected.tar.gz"
	((tests += 1))
}

release_root="$sandbox/releases"
current_root="$sandbox/current"
target_root="$sandbox/targets"
lock_path="$sandbox/deploy.lock"
drain_timeout=2
health_timeout=2
poll_interval=1
mkdir -p "$release_root" "$current_root" "$target_root"

fixture_archive=
fixture_checksums=
attestation_fails=0
stop_sticks=0
stop_deactivates=0
fail_health_version=
declare -A unit_state=()
declare -A running_version=()
declare -A url_unit=()
start_log=
stop_log=
event_log=

download_file() {
	local url=$1
	local destination=$2
	if [[ "$url" == */SHA256SUMS ]]; then
		cp "$fixture_checksums" "$destination"
	else
		cp "$fixture_archive" "$destination"
	fi
}

verify_attestation() {
	[[ "$attestation_fails" -eq 0 ]]
}

set_release_permissions() {
	find "$1" -type d -exec chmod 0755 {} +
	chmod 0755 "$1/bin/irgsh-$2"
	if [[ "$2" == iso ]]; then
		chmod 0755 "$1/share/iso-build.sh"
	fi
}

release_uid() {
	printf '0\n'
}

unit_is_active() {
	[[ "${unit_state[$1]:-inactive}" == active ]]
}

unit_is_inactive() {
	[[ "${unit_state[$1]:-inactive}" == inactive ]]
}

stop_unit() {
	stop_log+="$1 "
	if [[ "$stop_deactivates" -eq 1 ]]; then
		unit_state[$1]=deactivating
	elif [[ "$stop_sticks" -eq 0 ]]; then
		unit_state[$1]=inactive
	fi
}

start_unit() {
	start_log+="$1 "
	unit_state[$1]=active
	local component=${1#irgsh-}
	component=${component%%@*}
	component=${component%.service}
	running_version[$1]=$(binary_release_version "$(readlink -f "$current_root/$component")/bin/irgsh-$component")
}

health_version() {
	local unit=${url_unit[$1]}
	if [[ -n "$fail_health_version" && "${running_version[$unit]:-}" == "$fail_health_version" ]]; then
		printf '0.0.0\n'
	else
		printf '%s\n' "${running_version[$unit]:-}"
	fi
}

sleep_for() {
	return 0
}

log_event() {
	event_log+="$*"$'\n'
}

set_fixture() {
	local component=$1
	local version=$2
	local embedded=${3:-$version}
	fixture_archive="$sandbox/irgsh-${component}_${version}_linux_amd64.tar.gz"
	fixture_checksums="$sandbox/SHA256SUMS-$component-$version"
	make_archive "$component" "$embedded" "$fixture_archive"
	printf '%s  %s\n' "$(sha256sum "$fixture_archive" | awk '{print $1}')" "$(basename "$fixture_archive")" > "$fixture_checksums"
}

set_symlink_fixture() {
	local dir="$sandbox/symlink-release"
	rm -rf "$dir"
	mkdir -p "$dir/bin"
	ln -s /bin/true "$dir/bin/irgsh-repo"
	fixture_archive="$sandbox/irgsh-repo_2.3.2_linux_amd64.tar.gz"
	fixture_checksums="$sandbox/SHA256SUMS-repo-2.3.2"
	tar -C "$dir" -czf "$fixture_archive" bin
	printf '%s  %s\n' "$(sha256sum "$fixture_archive" | awk '{print $1}')" "$(basename "$fixture_archive")" > "$fixture_checksums"
}

reset_deployment() {
	rm -rf "$release_root" "$current_root" "$target_root"
	mkdir -p "$release_root/repo/v2.3.1" "$current_root" "$target_root"
	make_release_dir repo 2.3.1 "$release_root/repo/v2.3.1"
	ln -s "$release_root/repo/v2.3.1" "$current_root/repo"
	write_target "$target_root/repo" $'irgsh-repo@verbeek.service http://127.0.0.1:8082/api/v1/version\nirgsh-repo@rani.service http://127.0.0.1:8083/api/v1/version\n'
	unit_state=([irgsh-repo@verbeek.service]=active [irgsh-repo@rani.service]=inactive)
	running_version=([irgsh-repo@verbeek.service]=2.3.1)
	url_unit=([http://127.0.0.1:8082/api/v1/version]=irgsh-repo@verbeek.service [http://127.0.0.1:8083/api/v1/version]=irgsh-repo@rani.service)
	start_log=
	stop_log=
	event_log=
	attestation_fails=0
	stop_sticks=0
	stop_deactivates=0
	fail_health_version=
	test_uid=0
	test_mode=600
}

test_request_input() {
	read_request <<< $'repo\nv2.3.2'
	assert_equal "$component" repo
	assert_equal "$version" v2.3.2
	assert_fails read_request <<< $'repo\nv2.3.2\nextra'
	assert_fails read_request <<< $'../repo\nv2.3.2'
	((tests += 1))
}

test_lock() {
	exec 8> "$lock_path"
	flock -n 8
	assert_fails acquire_lock
	flock -u 8
	exec 8>&-
	acquire_lock
	flock -u 9
	((tests += 1))
}

test_preflight_failures_do_not_drain() {
	reset_deployment
	set_fixture repo 2.3.2
	attestation_fails=1
	assert_fails deploy_component repo v2.3.2
	assert_equal "$stop_log" ''
	attestation_fails=0
	printf '0  %s\n' "$(basename "$fixture_archive")" > "$fixture_checksums"
	assert_fails deploy_component repo v2.3.2
	assert_equal "$stop_log" ''
	set_fixture repo 2.3.2 9.9.9
	assert_fails deploy_component repo v2.3.2
	assert_equal "$stop_log" ''
	set_symlink_fixture
	assert_fails deploy_component repo v2.3.2
	assert_equal "$stop_log" ''
	reset_deployment
	set_fixture repo 2.3.2
	make_release_dir repo 2.3.2 "$release_root/repo/v2.3.2"
	printf '#!/usr/bin/env bash\n:\nprintf "irgsh-go version 2.3.2\\n"\n' > "$release_root/repo/v2.3.2/bin/irgsh-repo"
	chmod 0755 "$release_root/repo/v2.3.2/bin/irgsh-repo"
	assert_fails deploy_component repo v2.3.2
	assert_equal "$stop_log" ''
	((tests += 1))
}

test_healthy_deployment_and_noop() {
	reset_deployment
	set_fixture repo 2.3.2
	deploy_component repo v2.3.2
	assert_equal "$(readlink -f "$current_root/repo")" "$release_root/repo/v2.3.2"
	assert_equal "$start_log" 'irgsh-repo@verbeek.service '
	assert_equal "${unit_state[irgsh-repo@rani.service]}" inactive
	start_log=
	stop_log=
	deploy_component repo v2.3.2
	assert_equal "$start_log$stop_log" ''
	((tests += 1))
}

test_unhealthy_noop_fails() {
	reset_deployment
	set_fixture repo 2.3.1
	fail_health_version=2.3.1
	assert_fails deploy_component repo v2.3.1
	assert_equal "$start_log$stop_log" ''
	((tests += 1))
}

test_drain_timeout_keeps_current_release() {
	reset_deployment
	set_fixture repo 2.3.2
	stop_sticks=1
	assert_fails deploy_component repo v2.3.2
	assert_equal "$(readlink -f "$current_root/repo")" "$release_root/repo/v2.3.1"
	((tests += 1))
}

test_deactivating_unit_keeps_current_release() {
	reset_deployment
	set_fixture repo 2.3.2
	stop_deactivates=1
	assert_fails deploy_component repo v2.3.2
	assert_equal "$(readlink -f "$current_root/repo")" "$release_root/repo/v2.3.1"
	assert_equal "${unit_state[irgsh-repo@verbeek.service]}" deactivating
	((tests += 1))
}

test_switch_failure_restarts_old_release() {
	reset_deployment
	set_fixture repo 2.3.2
	ln -s occupied "$current_root/.repo.$$"
	assert_fails deploy_component repo v2.3.2
	rm "$current_root/.repo.$$"
	assert_equal "$(readlink -f "$current_root/repo")" "$release_root/repo/v2.3.1"
	assert_equal "${unit_state[irgsh-repo@verbeek.service]}" active
	assert_equal "${running_version[irgsh-repo@verbeek.service]}" 2.3.1
	((tests += 1))
}

test_health_failure_rolls_back() {
	reset_deployment
	set_fixture repo 2.3.2
	fail_health_version=2.3.2
	assert_fails deploy_component repo v2.3.2
	assert_equal "$(readlink -f "$current_root/repo")" "$release_root/repo/v2.3.1"
	assert_equal "${running_version[irgsh-repo@verbeek.service]}" 2.3.1
	[[ "$event_log" == *'rolled back repo to 2.3.1'* ]] || fail "missing rollback success log"
	((tests += 1))
}

test_older_release_uses_same_transaction() {
	reset_deployment
	set_fixture repo 2.3.0
	deploy_component repo v2.3.0
	assert_equal "$(readlink -f "$current_root/repo")" "$release_root/repo/v2.3.0"
	((tests += 1))
}

test_request_parser
test_target_validation
test_archive_validation
test_request_input
test_lock
test_preflight_failures_do_not_drain
test_healthy_deployment_and_noop
test_unhealthy_noop_fails
test_drain_timeout_keeps_current_release
test_deactivating_unit_keeps_current_release
test_switch_failure_restarts_old_release
test_health_failure_rolls_back
test_older_release_uses_same_transaction

printf '%d deployment tests passed\n' "$tests"
