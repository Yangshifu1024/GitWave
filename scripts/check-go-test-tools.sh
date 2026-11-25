#!/usr/bin/env bash
set -euo pipefail

version=$(git version)
printf '%s\n' "$version"
if [[ ! $version =~ ^git[[:space:]]version[[:space:]]([0-9]+)\.([0-9]+) ]]; then
  echo 'Could not determine the Git version.' >&2
  exit 1
fi
if (( BASH_REMATCH[1] < 2 || (BASH_REMATCH[1] == 2 && BASH_REMATCH[2] < 46) )); then
  echo 'GitWave tests require Git 2.46 or newer.' >&2
  exit 1
fi
if ! git lfs version; then
  echo 'GitWave tests require git-lfs on PATH.' >&2
  exit 1
fi
