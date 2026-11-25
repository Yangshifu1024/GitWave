#!/usr/bin/env bash
set -Eeuo pipefail
shopt -s nullglob

previous=${1:?Pass the absolute path to the v0.11.0 DMG}
candidate=${2:?Pass the absolute path to the candidate DMG}
version=${3:?Pass the candidate version}
exporter=${4:-}
exporter_version=${5:-}
bridge=${6:-}
[[ $previous == /* && -f $previous && $candidate == /* && -f $candidate && $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
if [[ -n $exporter || -n $exporter_version ]]; then
  [[ $exporter == /* && -d $exporter && $exporter_version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]
fi
if [[ -n $bridge ]]; then
  [[ $bridge == /* && -f $bridge ]]
fi
old_sha=9faab64431f8301df580e6b6b5852a1d75babdd264e02e97ca959331459e8ee0
printf '%s  %s\n' "$old_sha" "$previous" | /usr/bin/shasum -a 256 -c -

umask 077
host_user_home=$(cd "${HOME:?}" && pwd -P)
work=$(mktemp -d /private/tmp/gitwave-macos-upgrade.XXXXXX)
profile_home="$work/user"
profile="$profile_home/Library/Application Support/GitWave"
db="$profile/state.db"
log="$work/desktop.log"
installed="$work/Applications/GitWave.app"
old_mount="$work/old"
new_mount="$work/new"
fixture_preferences='{"gitwave.language":"zh-CN","gitwave-theme":"dark","gitwave-font-sans":"思源黑体, Arial","gitwave.lastActiveWorkspaceId":"legacy-workspace","gitwave.lastActiveRepoId":"repo-two","gitwave-auto-update":"false"}'
pid=

app_running() {
  jobs -pr | /usr/bin/grep -Fxq "$pid"
}

stop_app() {
  # The script owns one background process, which may start just before a signal.
  if [[ -z $pid ]]; then
    pid=$(jobs -pr)
  fi
  if [[ -n $pid ]]; then
    if app_running; then
      kill "$pid" 2>/dev/null || true
    fi
    for _ in {1..5}; do
      app_running || break
      sleep 1
    done
    if app_running; then
      kill -KILL "$pid" 2>/dev/null || true
    fi
    wait "$pid" 2>/dev/null || true
    pid=
  fi
}

cleanup() {
  local status=$? detached=true disk mounts
  stop_app
  if (( status != 0 )) && [[ -f $log ]]; then
    cat "$log" >&2
  fi
  if mounts=$(/sbin/mount); then
    # Inspect actual mounts even when attach was interrupted before returning.
    for disk in "$profile" "$new_mount" "$old_mount"; do
      if /usr/bin/grep -Fq " on $disk (" <<< "$mounts" && ! /usr/bin/hdiutil detach "$disk"; then
        detached=false
        status=1
      fi
    done
  else
    detached=false
    status=1
  fi
  # Remove only a fixture whose mounted volumes were released.
  if $detached; then
    rm -rf "$work"
  fi
  exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM
trap 'printf "macOS upgrade verification failed at line %s\n" "$LINENO" >&2' ERR

mkdir -p "$work/Applications" "$work/runtime" "$old_mount" "$new_mount" "$profile"
script_dir=$(cd "$(dirname "$0")" && pwd -P)
/usr/bin/xcrun swiftc -module-cache-path "$work/swift-cache" -o "$work/check-window" "$script_dir/check-macos-window.swift"
cat > "$work/desktop.sb" <<'SBPL'
(version 1)
(allow default)
(deny network-outbound)
(deny file-read* file-write* (subpath (param "GITWAVE_TEST_HOST_HOME")))
SBPL
desktop=(/usr/bin/sandbox-exec -D "GITWAVE_TEST_HOST_HOME=$host_user_home" -f "$work/desktop.sb"
  /usr/bin/env -i "HOME=$profile_home" "CFFIXED_USER_HOME=$profile_home" "TMPDIR=$work/runtime/"
  PATH=/usr/bin:/bin:/usr/sbin:/sbin)
/usr/bin/hdiutil attach -quiet -readonly -nobrowse -noautoopen -mountpoint "$old_mount" "$previous"
/usr/bin/hdiutil attach -quiet -readonly -nobrowse -noautoopen -mountpoint "$new_mount" "$candidate"
candidate_app="$new_mount/GitWave.app"
upgrade_app=$candidate_app
if [[ -n $bridge ]]; then
  upgrade_app="$work/bridge/GitWave.app"
  mkdir -p "$upgrade_app"
  # The legacy macOS updater drops the archive's first path component.
  /usr/bin/tar -xzf "$bridge" --strip-components 1 -C "$upgrade_app"
  /usr/bin/codesign --verify --deep --strict "$candidate_app"
  /usr/bin/codesign --verify --deep --strict "$upgrade_app"
  disk_hash=$(/usr/bin/codesign --display --verbose=4 "$candidate_app" 2>&1 | /usr/bin/sed -n 's/^CDHash=//p')
  bridge_hash=$(/usr/bin/codesign --display --verbose=4 "$upgrade_app" 2>&1 | /usr/bin/sed -n 's/^CDHash=//p')
  if [[ -z $disk_hash || $bridge_hash != "$disk_hash" ]]; then
    printf '%s\n' 'The macOS bridge and candidate DMG contain different signed applications' >&2
    exit 1
  fi
fi
profile_image="$work/profile.dmg"
/usr/bin/hdiutil create -quiet -size 48m -fs APFS -layout NONE -volname GitWaveTestProfile "$profile_image"
/usr/bin/hdiutil attach -quiet -nobrowse -noautoopen -mountpoint "$profile" "$profile_image"

install_app() {
  local source=$1 expected_version=$2 expected_entry=$3
  local plist="$source/Contents/Info.plist"
  test "$(/usr/libexec/PlistBuddy -c 'Print CFBundleIdentifier' "$plist")" = desktop.gitwave.work
  test "$(/usr/libexec/PlistBuddy -c 'Print CFBundleShortVersionString' "$plist")" = "$expected_version"
  test "$(/usr/libexec/PlistBuddy -c 'Print CFBundleExecutable' "$plist")" = "$expected_entry"
  if [[ -d $installed ]]; then
    mv "$installed" "$(mktemp -d "$work/retired.XXXXXX")/GitWave.app"
  fi
  /usr/bin/ditto "$source" "$installed"
  /usr/bin/codesign --verify --deep --strict "$installed"
}

launch_app() {
  local entry=$1
  : > "$log"
  "${desktop[@]}" "$installed/Contents/MacOS/$entry" > "$log" 2>&1 &
  pid=$!
}

start_app() {
  local entry=$1
  launch_app "$entry"
  local _
  for _ in {1..30}; do
    if "$work/check-window" "$pid" &&
       /usr/sbin/lsof -a -p "$pid" -t "$db" 2>/dev/null | /usr/bin/grep -qx "$pid"; then
      return 0
    fi
    kill -0 "$pid" 2>/dev/null || break
    sleep 1
  done
  printf 'The installed %s process did not show its window with the isolated profile open\n' "$entry" >&2
  return 1
}

sql() {
  /usr/bin/sqlite3 -readonly -cmd '.timeout 1000' "$@"
}

check_low_space() {
  local filler="$profile/.test-disk-filler" available original current checkpoint _
  local reserve_kib=3072
  checkpoint=$(/usr/bin/sqlite3 "$db" 'PRAGMA wal_checkpoint(TRUNCATE)')
  test "${checkpoint%%|*}" = 0
  original=$(/usr/bin/shasum -a 256 "$db")
  available=$(/bin/df -kP "$profile" | /usr/bin/awk 'NR == 2 { print $4 }')
  # Leave room to open the original DB, but less than the backup requires.
  [[ $available =~ ^[0-9]+$ ]]
  (( available > reserve_kib && available <= 48 * 1024 ))
  /bin/dd if=/dev/zero of="$filler" bs=4096 count=$(( (available - reserve_kib) / 4 )) 2> "$work/fill.log"
  available=$(/bin/df -kP "$profile" | /usr/bin/awk 'NR == 2 { print $4 }')
  [[ $available =~ ^[0-9]+$ ]]
  (( available > 1024 && available * 1024 < $(/usr/bin/stat -f %z "$db") ))

  launch_app GitWave
  for _ in {1..30}; do
    if "$work/check-window" "$pid" &&
       /usr/bin/grep -Eqi 'back up the previous workspace database: .*database or disk is full' "$log"; then
      break
    fi
    kill -0 "$pid"
    sleep 1
  done
  "$work/check-window" "$pid"
  /usr/bin/grep -Eqi 'back up the previous workspace database: .*database or disk is full' "$log"
  stop_app

  current=$(/usr/bin/shasum -a 256 "$db")
  test "$current" = "$original"
  test ! -s "$db-wal"
  local failed_backups=( "$profile"/backup-before-nsgo-* )
  test "${#failed_backups[@]}" = 0
  # The checkpointed file can be inspected without creating WAL sidecars.
  local checkpointed="file:$db?immutable=1"
  test "$(sql "$checkpointed" 'PRAGMA quick_check')" = ok
  test "$(sql "$checkpointed" "SELECT COUNT(*) FROM app_settings WHERE key IN ('native.stable-profile.v1', 'native.imported-webview-preferences.v1')")" = 0
  rm "$filler"
  printf '%s\n' 'Verified that a full profile volume left the original database and upgrade state intact'
}

seed_webview() {
  local seed="$work/PreferenceFixture.app/Contents"
  local status=0 _
  mkdir -p "$seed/MacOS"
  cat > "$seed/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
  <key>CFBundleIdentifier</key><string>desktop.gitwave.work</string>
  <key>CFBundleExecutable</key><string>PreferenceFixture</string>
  <key>CFBundlePackageType</key><string>APPL</string>
</dict>
</plist>
PLIST
  /usr/bin/xcrun swiftc -parse-as-library -module-cache-path "$work/swift-cache" -o "$seed/MacOS/PreferenceFixture" \
    "$script_dir/seed-macos-legacy-webview.swift"
  /usr/bin/codesign --sign - "$work/PreferenceFixture.app"
  printf '%s\n' "$fixture_preferences" > "$work/preferences.json"
  "${desktop[@]}" "$seed/MacOS/PreferenceFixture" "$work/preferences.json" > "$log" 2>&1 &
  pid=$!
  for _ in {1..30}; do
    if ! app_running; then
      wait "$pid" || status=$?
      pid=
      return "$status"
    fi
    sleep 1
  done
  printf '%s\n' 'The WebView preference fixture did not finish' >&2
  return 1
}

check_export() {
  local expected="SELECT COUNT(*) FROM json_each('$fixture_preferences') AS wanted
    WHERE json_extract((SELECT value_json FROM app_settings WHERE key='legacy.webview-preferences.v1'),
      '\$.values.\"' || wanted.key || '\"') IS NOT wanted.value"
  local _
  for _ in {1..20}; do
    if [[ $(sql "$db" "$expected" 2>/dev/null || true) == 0 ]]; then
      test "$(sql "$db" "SELECT json_extract(value_json, '$.version') FROM app_settings WHERE key='legacy.webview-preferences.v1'")" = 1
      test "$(sql "$db" "SELECT COUNT(*) FROM app_settings WHERE key='legacy.webview-preferences.v1' AND json_type(value_json, '$.values.fixture-not-a-preference') IS NOT NULL")" = 0
      return 0
    fi
    kill -0 "$pid" 2>/dev/null || break
    sleep 1
  done
  printf '%s\n' 'The preference exporter did not write the WebView values to SQLite' >&2
  return 1
}

install_app "$old_mount/GitWave.app" 0.11.0 gitwave
start_app gitwave
test "$(sql "$db" 'SELECT COUNT(*) FROM schema_version')" = 4
stop_app

for fixture_repo in "$work/first repo" "$work/second repo"; do
  /usr/bin/env -i PATH=/usr/bin:/bin GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null \
    /usr/bin/git init --quiet --template= -b main "$fixture_repo"
done
/usr/bin/sqlite3 "$db" <<SQL
INSERT INTO workspaces (id, name, settings_json, last_active_repo_id, created_at, updated_at)
  VALUES ('legacy-workspace', 'Existing workspace', '{"future":{"keep":true}}', 'repo-one', 1, 1);
INSERT INTO repos (id, workspace_id, path, nickname, added_at, position) VALUES
  ('repo-one', 'legacy-workspace', '$work/first repo', 'First repository', 1, 0),
  ('repo-two', 'legacy-workspace', '$work/second repo', 'Second repository', 2, 1);
INSERT INTO app_settings VALUES ('test.backup-payload', json_quote(hex(zeroblob(4194304))), 1);
SQL

if [[ -n $exporter ]]; then
  seed_webview
  install_app "$exporter" "$exporter_version" gitwave
  start_app gitwave
  check_export
  printf '%s\n' 'Verified the Tauri exporter wrote the WebView preferences to SQLite'
  stop_app
else
  # The standalone DMG check can run before a preference exporter exists.
  /usr/bin/sqlite3 "$db" "INSERT INTO app_settings VALUES ('legacy.webview-preferences.v1', '{\"version\":1,\"values\":$fixture_preferences}', 1)"
fi

install_app "$upgrade_app" "$version" GitWave
check_low_space
start_app GitWave
for _ in {1..15}; do
  if [[ $(sql "$db" "SELECT value_json FROM app_settings WHERE key='native.imported-webview-preferences.v1'" 2>/dev/null || true) == 1 ]]; then
    break
  fi
  kill -0 "$pid"
  sleep 1
done
test "$(sql "$db" "SELECT value_json FROM app_settings WHERE key='native.imported-webview-preferences.v1'")" = 1
test "$(sql "$db" "SELECT value_json FROM app_settings WHERE key='native.last-active-workspace'")" = '"legacy-workspace"'
test "$(sql "$db" "SELECT json_extract(value_json, '$.ui_language'), json_extract(value_json, '$.theme') FROM app_settings WHERE key='native.desktop-preferences'")" = 'zh-CN|dark'
test "$(sql "$db" "SELECT json_extract(value_json, '$.ui_font') FROM app_settings WHERE key='native.desktop-preferences'")" = '思源黑体, Arial'
test "$(sql "$db" "SELECT value_json FROM app_settings WHERE key='native.auto-update'")" = false
test "$(sql "$db" "SELECT last_active_repo_id FROM workspaces WHERE id='legacy-workspace'")" = repo-two
backups=( "$profile"/backup-before-nsgo-*/state.db )
test "${#backups[@]}" = 1
backup=${backups[0]}
test "$(sql "$db" "SELECT json_extract(value_json, '$') FROM app_settings WHERE key='native.stable-profile.v1'")" = "${backup#"$profile"/}"
test "$(sql "$backup" "SELECT name FROM workspaces WHERE id='legacy-workspace'")" = 'Existing workspace'
test "$(sql "$backup" "SELECT last_active_repo_id FROM workspaces WHERE id='legacy-workspace'")" = repo-one
test "$(sql "$backup" "SELECT COUNT(*) FROM app_settings WHERE key='native.imported-webview-preferences.v1'")" = 0
stop_app

if [[ -n $bridge ]]; then
  install_app "$candidate_app" "$version" GitWave
  start_app GitWave
  test "$(sql "$db" "SELECT last_active_repo_id FROM workspaces WHERE id='legacy-workspace'")" = repo-two
  test "$(sql "$db" "SELECT value_json FROM app_settings WHERE key='native.imported-webview-preferences.v1'")" = 1
  stop_app
  printf '%s\n' 'Verified profile migration from the macOS bridge and reopening with the candidate DMG'
fi

/usr/bin/sqlite3 "$db" "UPDATE workspaces SET name='Edited after upgrade' WHERE id='legacy-workspace'"
install_app "$old_mount/GitWave.app" 0.11.0 gitwave
start_app gitwave
test "$(sql "$db" "SELECT name, json_extract(settings_json, '$.future.keep') FROM workspaces WHERE id='legacy-workspace'")" = 'Edited after upgrade|1'
test "$(sql "$db" "SELECT id, nickname FROM repos WHERE workspace_id='legacy-workspace' ORDER BY position")" = $'repo-one|First repository\nrepo-two|Second repository'
test "$(sql "$backup" 'PRAGMA quick_check')" = ok
test "$(sql "$db" 'PRAGMA quick_check')" = ok
printf '%s\n' 'Verified macOS DMG upgrade, preference import, and rollback on the live SQLite profile'
