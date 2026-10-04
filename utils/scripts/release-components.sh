#!/usr/bin/env bash

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root"

version=$(tr -d '\n' < VERSION)
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]

mkdir -p target
workspace=$(mktemp -d target/.components.XXXXXX)
trap 'rm -rf "$workspace"' EXIT
output="$workspace/output"
mkdir "$output"
epoch=$(git show -s --format=%ct HEAD)

for component in builder chief cli iso repo; do
	stage="$workspace/$component"
	mkdir -p "$stage/bin"
	CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=$version" -o "$stage/bin/irgsh-$component" "./cmd/$component"
	if [[ "$component" == iso ]]; then
		mkdir "$stage/share"
		install -m 0755 utils/scripts/iso-build.sh "$stage/share/iso-build.sh"
		tar -C "$stage" --sort=name --mtime="@$epoch" --owner=0 --group=0 --numeric-owner -cf - bin share | gzip -n > "$output/irgsh-${component}_${version}_linux_amd64.tar.gz"
	else
		tar -C "$stage" --sort=name --mtime="@$epoch" --owner=0 --group=0 --numeric-owner -cf - bin | gzip -n > "$output/irgsh-${component}_${version}_linux_amd64.tar.gz"
	fi
done

(cd "$output" && sha256sum ./*.tar.gz | sed 's# \./# #' > SHA256SUMS)
rm -rf target/components
mv "$output" target/components
