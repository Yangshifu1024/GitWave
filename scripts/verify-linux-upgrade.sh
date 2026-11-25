#!/usr/bin/env bash
set -euo pipefail

if [[ ${1:-} == --seed-profile ]]; then
  fixture="${XDG_DATA_HOME:?}/GitWave"
  mkdir -p "$fixture"
  printf '%s\n' 'existing GitWave profile' > "$fixture/profile-marker"
  sqlite3 "$fixture/state.db" <<'SQL'
CREATE TABLE schema_version (version INTEGER PRIMARY KEY NOT NULL);
INSERT INTO schema_version VALUES (1), (2), (3), (4);
CREATE TABLE workspaces (
  id TEXT PRIMARY KEY NOT NULL, name TEXT NOT NULL, settings_json TEXT NOT NULL DEFAULT '{}',
  last_active_repo_id TEXT, created_at INTEGER NOT NULL, updated_at INTEGER NOT NULL
);
CREATE TABLE repos (
  id TEXT PRIMARY KEY NOT NULL, workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
  path TEXT NOT NULL, nickname TEXT, settings_override_json TEXT, added_at INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'active', missing_at INTEGER, position INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE app_settings (key TEXT PRIMARY KEY NOT NULL, value_json TEXT NOT NULL, updated_at INTEGER NOT NULL);
INSERT INTO workspaces (id, name, created_at, updated_at) VALUES ('legacy-workspace', 'Existing workspace', 1, 1);
SQL
  exit 0
fi

if [[ ${1:-} == --check-profile ]]; then
  fixture="${XDG_DATA_HOME:?}/GitWave"
  test "$(cat "$fixture/profile-marker")" = 'existing GitWave profile'
  shopt -s nullglob
  backups=( "$fixture"/backup-before-nsgo-*/state.db )
  test "${#backups[@]}" = 1
  for db in "$fixture/state.db" "${backups[0]}"; do
    test "$(sqlite3 -readonly "$db" "SELECT name FROM workspaces WHERE id = 'legacy-workspace'")" = 'Existing workspace'
  done
  record="SELECT value_json FROM app_settings WHERE key = 'native.stable-profile.v1'"
  test "$(sqlite3 -readonly "$fixture/state.db" "$record")" = "\"${backups[0]#"$fixture"/}\""
  test -z "$(sqlite3 -readonly "${backups[0]}" "$record")"
  exit 0
fi

if [[ ${1:-} == --desktop ]]; then
  export GDK_BACKEND=x11 GSK_RENDERER=cairo GTK_A11Y=none
  exec dbus-run-session -- xvfb-run -a bash -euo pipefail -c '
    "$1" >/tmp/gitwave-startup.log 2>&1 &
    pid=$!
    cleanup() {
      kill "$pid" 2>/dev/null || true
      wait "$pid" 2>/dev/null || true
    }
    trap cleanup EXIT
    for attempt in {1..30}; do
      if xdotool search --onlyvisible --name "^GitWave$" >/dev/null 2>&1 &&
         bash "$2" --check-profile; then
        exit 0
      fi
      kill -0 "$pid" 2>/dev/null || break
      sleep 1
    done
    printf "%s\n" "GitWave did not open its window and migrate the seeded profile" >&2
    cat /tmp/gitwave-startup.log >&2
    exit 1
  ' verify-desktop "$2" "$0"
fi

previous=$(cd "$1" && pwd)
packages=$(cd "$2" && pwd)
version=$3
script=$(cd "$(dirname "$0")" && pwd)/$(basename "$0")

(
  cd "$previous"
  printf '%s\n' \
    '4c260d25a2ce9a67c64cbb3d4f7d89a6fe54757f741d9e85b4aeb49483b119f8  GitWave_0.11.0_amd64.deb' \
    '776a666e188c6dc1ee7b9020aef808a507936ed1e3059fb21edde0eca43ce70b  GitWave-0.11.0-1.x86_64.rpm' | sha256sum -c -
)
(
  cd "$packages"
  base="gitwave-$version-linux-x64"
  for checksum in "$base.deb.sha256" "$base.rpm.sha256" "$base.AppImage.sha256"; do
    sha256sum -c "$checksum"
  done
)

docker run --rm --platform linux/amd64 -e RELEASE_VERSION="$version" \
  -v "$script:/verify.sh:ro" -v "$previous:/previous:ro" -v "$packages:/packages:ro" ubuntu:22.04 bash -euo pipefail -c '
    export DEBIAN_FRONTEND=noninteractive XDG_DATA_HOME=/tmp/gitwave-data
    old=/previous/GitWave_0.11.0_amd64.deb
    candidate=/packages/gitwave-$RELEASE_VERSION-linux-x64.deb
    apt-get update -qq
    apt-get install -y --no-install-recommends "$old" dbus xauth xvfb xdotool fonts-dejavu-core sqlite3
    bash /verify.sh --seed-profile
    test "$(dpkg-query -W -f="\${Version}" git-wave)" = 0.11.0
    test "$(readlink -f /usr/bin/gitwave)" = /usr/bin/gitwave
    apt-get install -y --no-install-recommends "$candidate"
    test "$(dpkg-query -W -f="\${Status} \${Version}" git-wave)" = "install ok installed $RELEASE_VERSION"
    test "$(dpkg-query -S /usr/bin/gitwave)" = "git-wave: /usr/bin/gitwave"
    test "$(readlink -f /usr/bin/gitwave)" = /opt/gitwave/GitWave
    test -x /usr/bin/gitwave
    grep -Fx "NoDisplay=true" /usr/share/applications/GitWave.desktop
    grep -Fx "Exec=\"/opt/gitwave/GitWave\"" /usr/share/applications/GitWave.desktop
    bash /verify.sh --desktop /usr/bin/gitwave
    bash /verify.sh --check-profile
    mkdir /tmp/appimage
    cd /tmp/appimage
    image=/packages/gitwave-$RELEASE_VERSION-linux-x64.AppImage
    "$image" --appimage-extract >/dev/null
    XDG_DATA_HOME=/tmp/gitwave-appimage-data bash /verify.sh --seed-profile
    XDG_DATA_HOME=/tmp/gitwave-appimage-data APPIMAGE="$image" APPDIR="$PWD/squashfs-root" \
      bash /verify.sh --desktop "$PWD/squashfs-root/AppRun"
    XDG_DATA_HOME=/tmp/gitwave-appimage-data bash /verify.sh --check-profile
    apt-get install -y --allow-downgrades --no-install-recommends "$old"
    test "$(dpkg-query -W -f="\${Status} \${Version}" git-wave)" = "install ok installed 0.11.0"
    test "$(readlink -f /usr/bin/gitwave)" = /usr/bin/gitwave
    bash /verify.sh --check-profile
    echo "Verified DEB upgrade, AppImage launch, and DEB rollback"
  '

docker run --rm --platform linux/amd64 -e RELEASE_VERSION="$version" \
  -v "$script:/verify.sh:ro" -v "$previous:/previous:ro" -v "$packages:/packages:ro" fedora:43 bash -euo pipefail -c '
    export XDG_DATA_HOME=/tmp/gitwave-data
    old=/previous/GitWave-0.11.0-1.x86_64.rpm
    candidate=/packages/gitwave-$RELEASE_VERSION-linux-x64.rpm
    dnf -y --setopt=localpkg_gpgcheck=0 --setopt=install_weak_deps=False install \
      "$old" dbus-daemon xorg-x11-server-Xvfb xdotool xorg-x11-xauth dejavu-sans-fonts sqlite
    bash /verify.sh --seed-profile
    test "$(rpm -q --queryformat "%{VERSION}\n" git-wave)" = 0.11.0
    test "$(readlink -f /usr/bin/gitwave)" = /usr/bin/gitwave
    dnf -y --setopt=localpkg_gpgcheck=0 --setopt=install_weak_deps=False install "$candidate"
    test "$(rpm -q --queryformat "%{VERSION}\n" git-wave)" = "$RELEASE_VERSION"
    test "$(rpm -qf --queryformat "%{NAME}\n" /usr/bin/gitwave)" = git-wave
    test "$(readlink -f /usr/bin/gitwave)" = /opt/gitwave/GitWave
    test -x /usr/bin/gitwave
    grep -Fx "NoDisplay=true" /usr/share/applications/GitWave.desktop
    grep -Fx "Exec=\"/opt/gitwave/GitWave\"" /usr/share/applications/GitWave.desktop
    bash /verify.sh --desktop /usr/bin/gitwave
    bash /verify.sh --check-profile
    rpm -U --oldpackage "$old"
    test "$(rpm -q --queryformat "%{VERSION}\n" git-wave)" = 0.11.0
    test "$(readlink -f /usr/bin/gitwave)" = /usr/bin/gitwave
    bash /verify.sh --check-profile
    echo "Verified RPM upgrade and rollback"
  '
