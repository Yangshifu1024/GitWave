#!/usr/bin/env bash
# Update the workspace version and refresh Cargo.lock. The version every crate
# inherits is `[workspace.package] version` in Cargo.toml. See
# .agents/skills/gitwave-release/SKILL.md.
#
# Usage: scripts/bump-version.sh 0.12.0
set -euo pipefail

fail() {
  echo "[bump] $*" >&2
  exit 1
}

[ "$#" -eq 1 ] || fail "usage: scripts/bump-version.sh <x.y.z>"
version=$1
# No leading zeros in any number: cargo rejects such a version, and only
# after it has been written to the manifest.
if [[ ! $version =~ ^v?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(-[0-9A-Za-z]+(\.[0-9A-Za-z]+)*)?$ ]]; then
  fail "'$version' is not a version such as 0.12.0"
fi
version=${version#v}

# Nowhere to bump outside this repository. The check also keeps a failed
# `git rev-parse` from turning into `cd ""`, which would leave the script in
# whatever directory it was called from.
toplevel=$(git rev-parse --show-toplevel 2> /dev/null) || fail "not inside a git repository"
cd "$toplevel"
command -v cargo > /dev/null || fail "cargo is not on PATH"
manifest=Cargo.toml
lockfile=Cargo.lock

# The lockfile has to agree with the manifests before the bump. If it does
# not, refreshing it would also record whatever else has drifted.
cargo metadata --format-version 1 --locked > /dev/null ||
  fail "Cargo.lock does not match the manifests yet; commit or discard that first"

# Only `[workspace.package]` carries a bare `version = "..."` line. A
# dependency's version is preceded by its name, and other tables are not
# looked at at all.
line=$(awk '
  $0 == "[workspace.package]" { inside = 1; next }
  inside && /^\[/ { exit }
  inside && /^version = "/ { print NR; exit }
' "$manifest")
[ -n "$line" ] || fail "$manifest: no version line under [workspace.package]"
current=$(sed -n "${line}p" "$manifest")
[[ $current =~ ^version\ =\ \"([0-9]+(\.[0-9]+)*(-[0-9A-Za-z.]+)?)\"$ ]] ||
  fail "$manifest line $line is not a bare version line: '$current'"
current=${BASH_REMATCH[1]}
if [ "$current" = "$version" ]; then
  echo "[bump] already at $version; nothing to do"
  exit 0
fi

# Both files are put back as they were whenever a later step fails.
work=$(mktemp -d "${TMPDIR:-/tmp}/gitwave-bump.XXXXXX") || fail "cannot create a temporary directory"
trap 'rm -rf "$work"' EXIT
cp "$manifest" "$work/Cargo.toml"
cp "$lockfile" "$work/Cargo.lock"
restore() {
  cat "$work/Cargo.toml" > "$manifest"
  cat "$work/Cargo.lock" > "$lockfile"
}

awk -v line="$line" -v version="$version" '
  NR == line { print "version = \"" version "\""; next }
  { print }
' "$manifest" > "$work/manifest.new"
cat "$work/manifest.new" > "$manifest"

# A plain metadata run re-resolves the crates whose manifest changed and
# writes their new version into the lockfile.
if ! cargo metadata --format-version 1 > /dev/null; then
  restore
  fail "cargo rejected the result; $manifest and $lockfile were put back"
fi

# Read both files back instead of trusting the edit. In the lockfile nothing
# but version lines may have moved, and each of them from the old version to
# the new one.
moved=$(diff "$work/Cargo.lock" "$lockfile" | grep '^[<>]' || true)
if [ -z "$moved" ]; then
  restore
  fail "refreshing $lockfile changed nothing; $manifest and $lockfile were put back"
fi
if printf '%s\n' "$moved" | grep -qvE "^[<>] version = \"($current|$version)\"$"; then
  restore
  printf '%s\n' "$moved" >&2
  fail "refreshing $lockfile moved more than the version lines; $manifest and $lockfile were put back"
fi
old_lines=$(printf '%s\n' "$moved" | grep -c "^< version = \"$current\"$" || true)
new_lines=$(printf '%s\n' "$moved" | grep -c "^> version = \"$version\"$" || true)
if [ "$old_lines" -eq 0 ] || [ "$old_lines" -ne "$new_lines" ]; then
  restore
  fail "not every crate reached $version; $manifest and $lockfile were put back"
fi
[ "$(sed -n "${line}p" "$manifest")" = "version = \"$version\"" ] ||
  { restore; fail "$manifest does not read back as $version; both files were put back"; }

echo "[bump] $manifest: $current -> $version"
echo "[bump] $lockfile: $new_lines version line(s) -> $version, nothing else moved"
git --no-pager diff --stat -- "$manifest" "$lockfile"
