#!/usr/bin/env bash
set -euo pipefail

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq
apt-get install -y --no-install-recommends \
  build-essential dpkg-dev pkg-config ninja-build python3-venv gettext sassc \
  libcurl4-openssl-dev libffi-dev libxml2-dev libexpat1-dev libpcre2-dev libmount-dev libselinux1-dev \
  libpango1.0-dev libcairo2-dev libdrm-dev libgdk-pixbuf-2.0-dev libepoxy-dev libgraphene-1.0-dev \
  libxkbcommon-dev libfontconfig1-dev libx11-dev libxext-dev libxi-dev libxrandr-dev libxcursor-dev \
  libxdamage-dev libxfixes-dev libxinerama-dev libpng-dev libtiff-dev libjpeg-dev libcups2-dev libatspi2.0-dev \
  zlib1g-dev xz-utils ca-certificates patchelf rpm
export PATH="/go/bin:$PATH" GOTOOLCHAIN=local GOFLAGS="-mod=readonly -buildvcs=false"
mkdir /workspace
tar -C /source --exclude=.nsgo --exclude=dist -cf - cmd internal go.mod go.sum LICENSE | tar -C /workspace -xf -
cd /workspace
if [[ -n ${GITWAVE_RELEASE_VERSION:-} ]]; then
  go run ./cmd/prepare-unix-setup -out /output/candidate -release-version "$GITWAVE_RELEASE_VERSION"
else
  go run ./cmd/prepare-unix-setup -out /output
  debs=( /output/releases/*.deb )
  test "${#debs[@]}" = 1
  go run ./cmd/prepare-linux-rpm -deb "${debs[0]}" -out /output/releases
fi
chown -R --reference=/output /output
