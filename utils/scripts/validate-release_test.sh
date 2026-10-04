#!/usr/bin/env bash

set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
validator="$root/utils/scripts/validate-release.sh"
workdir=$(mktemp -d)
trap 'rm -rf "$workdir"' EXIT

cd "$workdir"
git init -q -b main
git config user.name test
git config user.email test@example.invalid
printf '2.3.2\n' > VERSION
git add VERSION
git commit -qm initial
main_commit=$(git rev-parse HEAD)

"$validator" v2.3.2 "$main_commit" main

if "$validator" v2.3.3 "$main_commit" main; then
	exit 1
fi

if "$validator" 2.3.2 "$main_commit" main; then
	exit 1
fi

git switch -qc side
printf 'side\n' > side
git add side
git commit -qm side
side_commit=$(git rev-parse HEAD)
git switch -q main

if "$validator" v2.3.2 "$side_commit" main; then
	exit 1
fi
