#!/usr/bin/env bash
set -euo pipefail

repository=$1
tag=$2
expected=$3
reference="refs/tags/$tag"
refs=$(git ls-remote --exit-code -- "$repository" "$reference" "$reference^{}")
actual=$(awk -v ref="$reference" '
  $2 == ref { direct = $1 }
  $2 == ref "^{}" { peeled = $1 }
  END {
    if (peeled != "") print peeled
    else if (direct != "") print direct
    else exit 1
  }
' <<< "$refs")
if [[ $actual != "$expected" ]]; then
  printf '%s resolves to %s on the remote, expected %s\n' "$tag" "$actual" "$expected" >&2
  exit 1
fi
