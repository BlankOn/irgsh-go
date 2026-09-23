#!/usr/bin/env bash

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
sandbox=$(mktemp -d)
trap 'command rm -rf "$sandbox"' EXIT
export PROVISION_TEST_ROOT="$sandbox"
export PROVISION_EXISTING=0 PROVISION_GROUP_EXIT=0 PROVISION_REPO_EXIT=0 PROVISION_BUILDER_EXIT=0

for file in debian/irgsh.postinst utils/scripts/init.sh install.sh; do
	sed -e "s|/var/lib/irgsh|$sandbox/var/lib/irgsh|g" \
		-e "s|/var/log/irgsh|$sandbox/var/log/irgsh|g" \
		-e "s|/etc/irgsh|$sandbox/etc/irgsh|g" \
		-e "s|/etc/subuid|$sandbox/etc/subuid|g" \
		-e "s|/etc/subgid|$sandbox/etc/subgid|g" \
		-e "s|/usr/share/irgsh|$sandbox/usr/share/irgsh|g" \
		-e "s|/usr/bin/|$sandbox/usr/bin/|g" \
		-e "s|/lib/systemd/system/|$sandbox/lib/systemd/system/|g" \
		-e "s|/tmp/pubkey|$sandbox/pubkey|g" \
		"$root/$file" > "$sandbox/${file##*/}"
done
mkdir -p "$sandbox"/{etc/irgsh,var/lib/irgsh/{chief,repo,iso,gnupg/private-keys-v1.d,builder,builder-rani},var/log/irgsh,target}
touch "$sandbox/etc/irgsh/config.yaml" "$sandbox/var/lib/irgsh/repo/init.log" "$sandbox/target/release.tar.gz"
printf 'another instance\n' > "$sandbox/var/lib/irgsh/builder-rani/state"
printf 'other:100000:65536\n' > "$sandbox/etc/subuid"
printf 'other:200000:65536\n' > "$sandbox/etc/subgid"

record() {
	local arg
	for arg; do
		printf '%s\t' "${arg//"$PROVISION_TEST_ROOT"/}" >> "$PROVISION_LOG"
	done
	printf '\n' >> "$PROVISION_LOG"
}

function [ {
	if [[ $# == 4 && $1 == "$EUID" && $2 == '!=' && $3 == 0 && $4 == ']' ]]; then
		return 1
	fi
	builtin [ "$@"
}

getent() { [[ "$PROVISION_EXISTING" == 1 ]]; }
addgroup() { record addgroup "$@"; }
adduser() {
	record adduser "$@"
	if [[ "$*" == 'irgsh-builder irgsh' ]]; then
		return "$PROVISION_GROUP_EXIT"
	fi
}
chown() { record chown "$@"; }
chmod() { record chmod "$@"; }
chgrp() { record chgrp "$@"; }
install() { record install "$@"; }
apt() { record apt "$@"; }
cp() { record cp "$@"; }
rm() { record rm "$@"; }
tar() { record tar "$@"; }
systemctl() { record systemctl "$@"; }
killall() { record killall "$@"; }
gpg() { record gpg "$@"; }
curl() { record curl "$@"; return 95; }
docker() { record docker "$@"; return 95; }
sudo() { record sudo "$@"; return 95; }
useradd() { record useradd "$@"; return 95; }
groupadd() { record groupadd "$@"; return 95; }
usermod() { record usermod "$@"; return 95; }
userdel() { record userdel "$@"; return 95; }
groupdel() { record groupdel "$@"; return 95; }
deluser() { record deluser "$@"; return 95; }
delgroup() { record delgroup "$@"; return 95; }
gpasswd() { record gpasswd "$@"; return 95; }
su() {
	record su "$@"
	local command='' account=${!#}
	while (( $# )); do
		if [[ $1 == -c ]]; then command=$2; break; fi
		shift
	done
	command=${command//"$PROVISION_TEST_ROOT"/}
	case "$command" in
	*irgsh-repo*)
		if [[ "$account" != irgsh || ( "$command" != 'GNUPGHOME=/var/lib/irgsh/gnupg irgsh-repo -c /etc/irgsh/config.yaml init' && "$command" != 'irgsh-repo -c /etc/irgsh/config.yaml init > /dev/null' ) ]]; then
			printf 'repository initialization needs irgsh and its explicit config\n' >&2
			return 64
		fi
		return "$PROVISION_REPO_EXIT"
		;;
	*irgsh-builder*)
		[[ "$account" == irgsh-builder && "$command" == 'irgsh-builder init-base' ]] || return 64
		return "$PROVISION_BUILDER_EXIT"
		;;
	*gpg\ -K*) printf 'TEST-KEY\n' ;;
	esac
}
export -f record '[' getent addgroup adduser chown chmod chgrp install apt cp rm tar systemctl killall gpg curl docker sudo useradd groupadd usermod userdel groupdel deluser delgroup gpasswd su

fail() {
	printf 'FAIL: %s\n' "$*" >&2
	exit 1
}

expect_call() {
	local expected
	expected=$(printf '%s\t' "$@")
	grep -Fxq "$expected" "$PROVISION_LOG" || fail "missing command: $*"
}

reject_calls() {
	if grep -E "$1" "$PROVISION_LOG"; then fail "unexpected command matching $1"; fi
}

check_ownership() {
	reject_calls 'builder-rani|^(useradd|groupadd|usermod|userdel|groupdel|deluser|delgroup|gpasswd|sudo|curl|docker)[[:space:]]|--add-subuids|--add-subgids|[[:space:]](docker|root)[[:space:]]'
	reject_calls $'^(chown|chmod|rm).*\t(-[[:alpha:]]*[rR][[:alpha:]]*|--recursive)\t.*\t/var/lib/irgsh(/builder)?\t$'
	[[ $(< "$sandbox/var/lib/irgsh/builder-rani/state") == 'another instance' ]] || fail 'another builder state changed'
	[[ $(< "$sandbox/etc/subuid") == 'other:100000:65536' ]] || fail 'subordinate UID ranges changed'
	[[ $(< "$sandbox/etc/subgid") == 'other:200000:65536' ]] || fail 'subordinate GID ranges changed'
}

run_case() {
	local name=$1 expected=$2 file=$3 input=$4 status
	export PROVISION_LOG="$sandbox/$name.commands"
	: > "$PROVISION_LOG"
	set +e
	(cd "$sandbox" && printf '%s' "$input" | bash "$sandbox/$file" configure) > "$sandbox/$name.output" 2>&1
	status=$?
	set -e
	if [[ $status != "$expected" ]]; then
		cat "$sandbox/$name.output" >&2
		fail "$name exit $status; want $expected"
	fi
	check_ownership
}

check_account() {
	expect_call addgroup --system irgsh-builder
	expect_call adduser --system --home /var/lib/irgsh/builder --no-create-home --ingroup irgsh-builder --disabled-password --shell /usr/sbin/nologin --gecos 'IRGSH Builder' irgsh-builder
	expect_call adduser irgsh-builder irgsh
	expect_call install -d -o irgsh-builder -g irgsh-builder -m 0755 /var/lib/irgsh/builder
}

run_case postinst-fresh 0 irgsh.postinst ''
check_account
[[ $(grep -c '^adduser' "$PROVISION_LOG") == 3 ]] || fail 'unexpected account or group grant'

export PROVISION_EXISTING=1
run_case postinst-repeated 0 irgsh.postinst ''
reject_calls '^addgroup|^adduser[[:space:]]--system'
expect_call adduser irgsh-builder irgsh
expect_call install -d -o irgsh-builder -g irgsh-builder -m 0755 /var/lib/irgsh/builder
[[ $(grep -c '^adduser' "$PROVISION_LOG") == 1 ]] || fail 'unexpected repeated group grant'

export PROVISION_GROUP_EXIT=43
run_case postinst-group-failure 43 irgsh.postinst ''
reject_calls '^(chown|chmod|chgrp|install)[[:space:]]'
export PROVISION_GROUP_EXIT=0

run_case init-success 0 init.sh ynnyy
expect_call su -c 'GNUPGHOME=/var/lib/irgsh/gnupg irgsh-repo -c /etc/irgsh/config.yaml init' -s /bin/bash irgsh
expect_call su -s /bin/bash -c 'irgsh-builder init-base' irgsh-builder
[[ $(grep '^su' "$PROVISION_LOG" | cut -f3) == $'GNUPGHOME=/var/lib/irgsh/gnupg irgsh-repo -c /etc/irgsh/config.yaml init\n/bin/bash' ]] || fail 'repo must initialize before builder base'

export PROVISION_REPO_EXIT=44
run_case init-repo-failure 44 init.sh ynnyy
reject_calls 'irgsh-builder[[:space:]]init-base'
if grep -q 'Initialization done!' "$sandbox/init-repo-failure.output"; then fail 'failed repo reported success'; fi
export PROVISION_REPO_EXIT=0 PROVISION_BUILDER_EXIT=45
run_case init-builder-failure 45 init.sh ynnyy
expect_call su -s /bin/bash -c 'irgsh-builder init-base' irgsh-builder
if grep -q 'Initialization done!' "$sandbox/init-builder-failure.output"; then fail 'failed builder reported success'; fi
export PROVISION_BUILDER_EXIT=0 PROVISION_EXISTING=0

run_case installer-success 0 install.sh ''
check_account
expect_call su -c 'irgsh-repo -c /etc/irgsh/config.yaml init > /dev/null' -s /bin/bash irgsh
expect_call systemctl enable irgsh-builder

export PROVISION_REPO_EXIT=44
run_case installer-repo-failure 44 install.sh ''
reject_calls '^systemctl[[:space:]]enable'
if grep -q 'Happy hacking!' "$sandbox/installer-repo-failure.output"; then fail 'failed installer reported success'; fi

printf 'PASS: 8 provisioning scenarios\n'
