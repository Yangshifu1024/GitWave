#!/usr/bin/env bash
set -euo pipefail

go_sources() {
  find "$@" \( -type d \( -name '.*' -o -name dist \) \) -prune -o -type f -name '*.go' -print0
}

if [[ ${1:-} == --write ]]; then
  shift
  go_sources "$@" | xargs -0 gofmt -w
  exit
fi

files=$(go_sources "$@" | xargs -0 gofmt -l)
if [[ -n $files ]]; then
  printf '%s\n' "$files" >&2
  exit 1
fi
