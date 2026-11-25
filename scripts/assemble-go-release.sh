#!/usr/bin/env bash
set -euo pipefail

artifacts=$1
bundle=$2
version=$3
bridge_version=$4

mkdir -p "$bundle"
mkdir "$bundle/assets"

copy_checked() {
  local dir=$1 name=$2 destination=$3 digest
  digest=$(cd "$dir" && sha256sum -- "$name")
  if [[ $(cat "$dir/$name.sha256") != "$digest" ]]; then
    printf 'Checksum mismatch for %s\n' "$dir/$name" >&2
    return 1
  fi
  cp -- "$dir/$name" "$dir/$name.sha256" "$destination/"
}

mac="gitwave-$version-macos-arm64"
linux="gitwave-$version-linux-x64"
windows="gitwave-$version-windows-x64-setup.exe"
for suffix in dmg zip; do
  copy_checked "$artifacts/macos/releases" "$mac.$suffix" "$bundle/assets"
done
for suffix in AppImage deb rpm; do
  copy_checked "$artifacts/linux/releases" "$linux.$suffix" "$bundle/assets"
done
copy_checked "$artifacts/windows" "$windows" "$bundle/assets"
copy_checked "$artifacts/windows" GitWave.exe "$bundle"

tar -czf "$bundle/assets/$mac-git-source.tar.gz" -C "$artifacts/macos" git-source
tar -czf "$bundle/assets/$linux-source.tar.gz" -C "$artifacts/linux" git-source gtk-source
(
  cd "$bundle/assets"
  for archive in "$mac-git-source.tar.gz" "$linux-source.tar.gz"; do
    sha256sum -- "$archive" > "$archive.sha256"
  done
)

if [[ $bridge_version == "$version" ]]; then
  archive="GitWave_${version}_aarch64.app.tar.gz"
  copy_checked "$artifacts/macos-legacy" "$archive" "$bundle/assets"
  cp -- "$artifacts/macos-legacy/$archive.sig" "$bundle/assets/"
fi
