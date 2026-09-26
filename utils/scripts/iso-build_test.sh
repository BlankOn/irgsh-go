#!/usr/bin/env bash

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
script="$root/utils/scripts/iso-build.sh"

tests=0
sandbox=$(mktemp -d)
trap 'rm -rf "$sandbox"' EXIT
stubs="$sandbox/stubs"
today=$(date '+%Y%m%d')
archive_uri="http://arsip-dev.blankonlinux.id/sinambung/"

export GIT_CONFIG_GLOBAL=/dev/null
export GIT_CONFIG_NOSYSTEM=1

fail() {
	printf 'FAIL: %s\n' "$*" >&2
	exit 1
}

assert_equal() {
	[[ "$1" == "$2" ]] || fail "got <$1>, want <$2>"
}

assert_file() {
	[[ -f "$1" ]] || fail "missing file $1"
}

assert_absent() {
	[[ ! -e "$1" ]] || fail "unexpected path $1"
}

assert_contains() {
	grep -qF -- "$2" "$1" || fail "$1 does not contain <$2>"
}

write_stub() {
	printf '%s\n' "$2" > "$stubs/$1"
	chmod 0755 "$stubs/$1"
}

make_stubs() {
	mkdir -p "$stubs"
	for denied in sudo mount umount; do
		write_stub "$denied" "#!/bin/sh
echo \"$denied is not allowed: \$*\" >&2
exit 97"
	done
	write_stub curl '#!/bin/sh
exit 0'
	write_stub mv '#!/bin/bash
if [ -n "$MV_FAIL_SOURCE" ] && [ "$1" = "$MV_FAIL_SOURCE" ]; then
	exit 1
fi
for real in /usr/bin/mv /bin/mv; do
	[ -x "$real" ] && exec "$real" "$@"
done
exit 127'
	write_stub zsyncmake '#!/bin/bash
printf "%s\n" "$*" >> "$STUB_LOG/zsyncmake.calls"
[ -z "$ZSYNC_FAIL" ] || exit 1
while [ $# -gt 1 ]; do
	[ "$1" = -o ] && out=$2
	shift
done
: > "$out"'
	write_stub lb '#!/bin/bash
printf "%s\n" "$*" >> "$STUB_LOG/lb.calls"
case "$1" in
clean)
	[ -z "$LB_CLEAN_FAIL" ] || exit 1
	;;
config)
	[ -z "$LB_CONFIG_FAIL" ] || exit 1
	if [ -n "$LB_NO_MIRRORS" ]; then
		printf "LB_DISTRIBUTION=\"sinambung\"\n" > config/bootstrap
	else
		printf "LB_MIRROR_BOOTSTRAP=\"http://old.invalid/\"\nLB_PARENT_MIRROR_CHROOT=\"http://old.invalid/\"\nLB_DISTRIBUTION=\"sinambung\"\n" > config/bootstrap
	fi
	;;
build)
	cp config/bootstrap "$STUB_LOG/bootstrap.at-build"
	if [ -n "$LB_FAIL" ]; then
		echo "E: stub build failure"
		exit 1
	fi
	name=blankon-live-image-amd64
	[ -f variant ] && name="blankon-live-image-$(cat variant)-amd64"
	for suffix in contents files packages hybrid.iso; do
		printf "%s\n" "$suffix" > "$name.$suffix"
	done
	echo "P: Build completed successfully"
	;;
esac'
}

commit_all() {
	git -C "$1" add -A
	git -C "$1" -c user.name=IRGSH -c user.email=irgsh@example.invalid commit -qm "$2"
}

branch_from() {
	git -C "$1" checkout -q -b "$2" "$3"
}

make_variant_repo() {
	variant_repo="$sandbox/variant-repo"
	git init -q "$variant_repo"
	git -C "$variant_repo" checkout -q -b variant-gnome
	mkdir -p "$variant_repo/config/common/includes.chroot/etc/blankon" \
		"$variant_repo/config/common/bootloaders/syslinux_common" \
		"$variant_repo/config/gnome/package-lists" \
		"$variant_repo/auto"
	printf 'gnome\n' > "$variant_repo/variant"
	printf 'ARCHIVE_HOST="arsip-dev.blankonlinux.id"\nARCHIVE_SUITE="sinambung"\nARCHIVE_URI="http://${ARCHIVE_HOST}/${ARCHIVE_SUITE}/"\n' \
		> "$variant_repo/config/common/includes.chroot/etc/blankon/archive.conf"
	printf '<svg>BUILD_NUMBER</svg>\n' > "$variant_repo/config/common/bootloaders/syslinux_common/splash.svg"
	printf 'common\n' > "$variant_repo/config/common/common-marker"
	printf 'gnome-shell\n' > "$variant_repo/config/gnome/package-lists/desktop.list.chroot"
	printf '#!/bin/sh\n' > "$variant_repo/auto/config"
	commit_all "$variant_repo" "variant base"
	variant_first=$(git -C "$variant_repo" rev-parse HEAD)
	printf 'second\n' > "$variant_repo/config/common/second-marker"
	commit_all "$variant_repo" "variant second"
	variant_tip=$(git -C "$variant_repo" rev-parse HEAD)

	branch_from "$variant_repo" feature/iso "$variant_tip"
	printf 'feature\n' > "$variant_repo/config/common/feature-marker"
	commit_all "$variant_repo" "feature"

	branch_from "$variant_repo" off-branch "$variant_first"
	printf 'off\n' > "$variant_repo/off-marker"
	commit_all "$variant_repo" "off branch"
	variant_off=$(git -C "$variant_repo" rev-parse HEAD)

	branch_from "$variant_repo" no-archive "$variant_tip"
	rm "$variant_repo/config/common/includes.chroot/etc/blankon/archive.conf"
	commit_all "$variant_repo" "no archive"

	branch_from "$variant_repo" bad-archive "$variant_tip"
	printf 'ARCHIVE_URI="http://arsip.invalid/a#b&c"\n' > "$variant_repo/config/common/includes.chroot/etc/blankon/archive.conf"
	commit_all "$variant_repo" "bad archive"

	branch_from "$variant_repo" empty-archive "$variant_tip"
	printf 'ARCHIVE_URI=""\n' > "$variant_repo/config/common/includes.chroot/etc/blankon/archive.conf"
	commit_all "$variant_repo" "empty archive"

	branch_from "$variant_repo" no-common "$variant_tip"
	rm -r "$variant_repo/config/common"
	commit_all "$variant_repo" "no common"

	branch_from "$variant_repo" no-auto "$variant_tip"
	rm -r "$variant_repo/auto"
	commit_all "$variant_repo" "no auto"

	git -C "$variant_repo" -c user.name=IRGSH -c user.email=irgsh@example.invalid tag -a pinned-tag -m pinned "$variant_first"
	variant_tag_object=$(git -C "$variant_repo" rev-parse pinned-tag)

	branch_from "$variant_repo" bad-variant "$variant_tip"
	printf 'Gnome!\n' > "$variant_repo/variant"
	commit_all "$variant_repo" "bad variant"

	branch_from "$variant_repo" missing-variant "$variant_tip"
	printf 'kde\n' > "$variant_repo/variant"
	commit_all "$variant_repo" "missing variant"

	git -C "$variant_repo" checkout -q variant-gnome
}

make_legacy_repo() {
	legacy_repo="$sandbox/legacy-repo"
	git init -q "$legacy_repo"
	git -C "$legacy_repo" checkout -q -b legacy-plain
	mkdir -p "$legacy_repo/config/bootloaders/syslinux_common"
	printf '<svg>BUILD_NUMBER</svg>\n' > "$legacy_repo/config/bootloaders/syslinux_common/splash.svg"
	printf 'legacy\n' > "$legacy_repo/config/legacy-marker"
	commit_all "$legacy_repo" "legacy plain"
	branch_from "$legacy_repo" legacy-archive legacy-plain
	mkdir -p "$legacy_repo/config/includes.chroot/etc/blankon"
	printf 'ARCHIVE_URI="%s"\n' "$archive_uri" > "$legacy_repo/config/includes.chroot/etc/blankon/archive.conf"
	commit_all "$legacy_repo" "legacy archive"
}

run_build() {
	local work=$1
	shift
	mkdir -p "$work/out"
	rm -f "$work/lb.calls" "$work/zsyncmake.calls" "$work/bootstrap.at-build"
	printf 'BUILD_JAHITAN_PATH=%q\nBUILD_PUBLISH_URL=%q\nBUILD_LOCKFILE=%q\nTELEGRAM_BOT_KEY=\n' \
		"$work/out" "https://example.invalid/iso" "$work/build.lock" > "$work/.env"
	(cd "$work" && PATH="$stubs:$PATH" STUB_LOG="$work" bash "$script" "$@") > "$work/run.log" 2>&1
}

expect_failure() {
	local work=$1
	local message=$2
	shift 2
	if run_build "$work" "$@"; then
		fail "build succeeded: $*"
	fi
	assert_contains "$work/run.log" "$message"
	assert_absent "$work/out/current"
}

test_variant_branch_tip() {
	local work="$sandbox/variant-tip"
	local image=blankon-live-image-gnome-amd64
	run_build "$work" "$variant_repo" variant-gnome || fail "variant build failed: $(tail -n 20 "$work/run.log")"
	assert_equal "$(cat "$work/out/current/current.txt")" "$today-1"
	assert_file "$work/out/current/$image.hybrid.iso"
	assert_equal "$(cat "$work/out/current/$image.hybrid.iso.sha256sum")" "$(sha256sum "$work/out/current/$image.hybrid.iso" | cut -d' ' -f1)  $image.hybrid.iso"
	assert_contains "$work/zsyncmake.calls" "-u https://example.invalid/iso/current/$image.hybrid.iso"
	assert_file "$work/config/common-marker"
	assert_file "$work/config/second-marker"
	assert_file "$work/config/package-lists/desktop.list.chroot"
	assert_absent "$work/config/common"
	assert_file "$work/auto/config"
	assert_equal "$(cat "$work/variant")" gnome
	assert_equal "$(grep -c "^LB_.*MIRROR_.*=\"$archive_uri\"$" "$work/bootstrap.at-build")" 2
	assert_contains "$work/config/bootloaders/syslinux_common/splash.svg" "$today-1"
	assert_contains "$work/run.log" "[ ISO BUILD ] repo=$variant_repo branch=variant-gnome commit=$variant_tip layout=variant variant=gnome archive=$archive_uri"
	((tests += 1))
}

test_pinned_commit_on_slash_branch() {
	local work="$sandbox/pinned"
	run_build "$work" "$variant_repo" feature/iso "$variant_first" || fail "pinned build failed: $(tail -n 20 "$work/run.log")"
	assert_contains "$work/run.log" "commit=$variant_first layout=variant"
	assert_absent "$work/config/second-marker"
	assert_absent "$work/config/feature-marker"
	((tests += 1))
}

test_rejected_revisions() {
	expect_failure "$sandbox/off-branch" "is not on branch variant-gnome" "$variant_repo" variant-gnome "$variant_off"
	expect_failure "$sandbox/missing-commit" "Failed to checkout commit" "$variant_repo" variant-gnome 0123456789abcdef0123456789abcdef01234567
	expect_failure "$sandbox/missing-branch" "Failed to clone" "$variant_repo" no-such-branch
	expect_failure "$sandbox/tag-object" "HEAD does not match commit $variant_tag_object" "$variant_repo" variant-gnome "$variant_tag_object"
	((tests += 1))
}

test_rejected_variant_layouts() {
	expect_failure "$sandbox/bad-variant" "Invalid variant: Gnome!" "$variant_repo" bad-variant
	assert_absent "$sandbox/bad-variant/lb.calls"
	expect_failure "$sandbox/missing-variant" "config/kde is missing" "$variant_repo" missing-variant
	expect_failure "$sandbox/no-archive" "archive.conf is missing" "$variant_repo" no-archive
	if grep -q '^build' "$sandbox/no-archive/lb.calls"; then
		fail "lb build ran without archive.conf"
	fi
	expect_failure "$sandbox/bad-archive" "invalid ARCHIVE_URI" "$variant_repo" bad-archive
	expect_failure "$sandbox/empty-archive" "invalid ARCHIVE_URI" "$variant_repo" empty-archive
	expect_failure "$sandbox/no-common" "config/common is missing" "$variant_repo" no-common
	expect_failure "$sandbox/no-auto" "auto is missing" "$variant_repo" no-auto
	LB_NO_MIRRORS=1 expect_failure "$sandbox/no-mirrors" "no mirror entries" "$variant_repo" variant-gnome
	((tests += 1))
}

test_legacy_layout() {
	local work="$sandbox/legacy-archive"
	local image=blankon-live-image-amd64
	run_build "$work" "$legacy_repo" legacy-archive || fail "legacy build failed: $(tail -n 20 "$work/run.log")"
	assert_file "$work/out/current/$image.hybrid.iso"
	assert_contains "$work/zsyncmake.calls" "-u https://example.invalid/iso/current/$image.hybrid.iso"
	assert_file "$work/config/legacy-marker"
	assert_equal "$(grep -c "^LB_.*MIRROR_.*=\"$archive_uri\"$" "$work/bootstrap.at-build")" 2
	assert_contains "$work/run.log" "layout=legacy variant=none archive=$archive_uri"

	work="$sandbox/legacy-plain"
	run_build "$work" "$legacy_repo" legacy-plain || fail "legacy build failed: $(tail -n 20 "$work/run.log")"
	assert_contains "$work/run.log" "archive.conf is absent; using the mirrors from config/bootstrap"
	assert_equal "$(grep -c '^LB_.*MIRROR_.*="http://old.invalid/"$' "$work/bootstrap.at-build")" 2
	assert_contains "$work/run.log" "layout=legacy variant=none archive=config/bootstrap"
	((tests += 1))
}

test_failures_keep_current() {
	local work="$sandbox/shared"
	LB_FAIL=1 expect_failure "$sandbox/lb-failure" "lb build did not complete successfully" "$variant_repo" variant-gnome
	ZSYNC_FAIL=1 expect_failure "$sandbox/publish-failure" "Failed to publish blankon-live-image-gnome-amd64" "$variant_repo" variant-gnome
	LB_CLEAN_FAIL=1 expect_failure "$sandbox/clean-failure" "lb clean failed" "$legacy_repo" legacy-plain
	LB_CONFIG_FAIL=1 expect_failure "$sandbox/config-failure" "lb config failed" "$legacy_repo" legacy-plain
	if grep -q '^build' "$sandbox/clean-failure/lb.calls" "$sandbox/config-failure/lb.calls"; then
		fail "lb build ran after a failed setup step"
	fi

	run_build "$work" "$variant_repo" variant-gnome || fail "variant build failed: $(tail -n 20 "$work/run.log")"
	run_build "$work" "$legacy_repo" legacy-plain || fail "legacy build failed: $(tail -n 20 "$work/run.log")"
	assert_absent "$work/variant"
	assert_absent "$work/auto"
	assert_equal "$(cat "$work/out/current/current.txt")" "$today-2"
	if LB_FAIL=1 run_build "$work" "$variant_repo" variant-gnome; then
		fail "failing build succeeded"
	fi
	assert_equal "$(cat "$work/out/current/current.txt")" "$today-2"
	if ZSYNC_FAIL=1 run_build "$work" "$variant_repo" variant-gnome; then
		fail "failing publish succeeded"
	fi
	assert_equal "$(cat "$work/out/current/current.txt")" "$today-2"
	assert_absent "$work/out/current.new"
	if MV_FAIL_SOURCE="$work/out/current.new" run_build "$work" "$variant_repo" variant-gnome; then
		fail "failing swap succeeded"
	fi
	assert_contains "$work/run.log" "Failed to publish blankon-live-image-gnome-amd64"
	assert_equal "$(cat "$work/out/current/current.txt")" "$today-2"
	assert_absent "$work/out/current.old"
	run_build "$work" "$variant_repo" variant-gnome || fail "variant build failed: $(tail -n 20 "$work/run.log")"
	assert_equal "$(cat "$work/out/current/current.txt")" "$today-6"
	assert_absent "$work/out/current.old"
	((tests += 1))
}

test_build_lock() {
	local work="$sandbox/locked"
	mkdir -p "$work"
	exec 8> "$work/build.lock"
	flock -n 8 || fail "cannot take the test lock"
	expect_failure "$work" "Build already in progress" "$variant_repo" variant-gnome
	exec 8>&-
	printf '1\n' > "$work/build.lock"
	run_build "$work" "$variant_repo" variant-gnome || fail "stale lock blocked the build: $(tail -n 20 "$work/run.log")"
	assert_equal "$(cat "$work/out/current/current.txt")" "$today-1"
	((tests += 1))
}

test_symlinked_output() {
	local work="$sandbox/symlinked"
	mkdir -p "$work/published"
	ln -s published "$work/out"
	run_build "$work" "$variant_repo" variant-gnome || fail "first build failed: $(tail -n 20 "$work/run.log")"
	run_build "$work" "$variant_repo" variant-gnome || fail "second build failed: $(tail -n 20 "$work/run.log")"
	assert_equal "$(cat "$work/published/current/current.txt")" "$today-2"
	((tests += 1))
}

test_unusable_lock() {
	local work="$sandbox/unusable-lock"
	mkdir -p "$work/build.lock"
	expect_failure "$work" "cannot open lock file $work/build.lock" "$variant_repo" variant-gnome
	if grep -q "Build already in progress" "$work/run.log"; then
		fail "an unusable lock file was reported as a running build"
	fi
	((tests += 1))
}

test_no_privileged_commands() {
	local log
	for log in "$sandbox"/*/run.log; do
		if grep -qE '(sudo|mount|umount) is not allowed' "$log"; then
			fail "$log used a privileged command"
		fi
	done
	((tests += 1))
}

make_stubs
make_variant_repo
make_legacy_repo
test_variant_branch_tip
test_pinned_commit_on_slash_branch
test_rejected_revisions
test_rejected_variant_layouts
test_legacy_layout
test_failures_keep_current
test_build_lock
test_symlinked_output
test_unusable_lock
test_no_privileged_commands
printf '%d ISO build script tests passed\n' "$tests"
