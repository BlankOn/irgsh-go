#!/usr/bin/env bash

set -euo pipefail

[[ $# -eq 3 ]]

tag=$1
commit=$2
main_ref=$3
version=$(tr -d '\n' < VERSION)

[[ "$tag" == "v$version" ]]
git merge-base --is-ancestor "$commit" "$main_ref"
