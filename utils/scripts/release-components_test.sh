#!/usr/bin/env bash

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root"

make release-components

version=$(tr -d '\n' < VERSION)
workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

expected_files=$'SHA256SUMS\nirgsh-builder_'"$version"$'_linux_amd64.tar.gz\nirgsh-chief_'"$version"$'_linux_amd64.tar.gz\nirgsh-cli_'"$version"$'_linux_amd64.tar.gz\nirgsh-iso_'"$version"$'_linux_amd64.tar.gz\nirgsh-repo_'"$version"'_linux_amd64.tar.gz'
actual_files=$(find target/components -maxdepth 1 -type f -printf '%f\n' | sort)
[[ "$actual_files" == "$expected_files" ]]

(cd target/components && sha256sum --check SHA256SUMS)

for component in builder chief cli iso repo; do
	archive="target/components/irgsh-${component}_${version}_linux_amd64.tar.gz"
	case "$component" in
	iso)
		expected_members=$'bin/\nbin/irgsh-iso\nshare/\nshare/iso-build.sh'
		;;
	*)
		expected_members=$'bin/\nbin/irgsh-'"$component"
		;;
	esac
	actual_members=$(tar -tzf "$archive" | sort)
	[[ "$actual_members" == "$expected_members" ]]
	mkdir "$workdir/$component"
	tar -xzf "$archive" -C "$workdir/$component"
	actual_version=$("$workdir/$component/bin/irgsh-$component" --version | awk 'END { print $NF }')
	[[ "$actual_version" == "$version" ]]
done
