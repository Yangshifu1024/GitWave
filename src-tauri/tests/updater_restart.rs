//! Exercise the real Tauri restart in an isolated bundle, never the installed GitWave app.
//! Regression for https://tauri.app/release/tauri/all-versions/#2122.
#![cfg(target_os = "macos")]

use std::{
    fs,
    io::Write,
    path::{Path, PathBuf},
    process::{Command, Stdio},
    time::{Duration, Instant, SystemTime, UNIX_EPOCH},
};

const TEST_NAME: &str = "updated_bundle_restarts_with_independent_stdio_and_process_group";
const BUNDLE: &str = "GitWaveRestartFixture.app";

struct FixtureDir(PathBuf);

impl Drop for FixtureDir {
    fn drop(&mut self) {
        let _ = fs::remove_dir_all(&self.0);
    }
}

fn wait_for_file(path: &Path) -> bool {
    let deadline = Instant::now() + Duration::from_secs(15);
    while Instant::now() < deadline {
        if path.exists() {
            return true;
        }
        std::thread::sleep(Duration::from_millis(50));
    }
    false
}

fn process_group(pid: u32) -> String {
    let output = Command::new("/bin/ps")
        .args(["-o", "pgid=", "-p", &pid.to_string()])
        .output()
        .unwrap();
    assert!(output.status.success());
    let group = String::from_utf8(output.stdout).unwrap().trim().to_string();
    assert!(!group.is_empty());
    group
}

#[test]
fn updated_bundle_restarts_with_independent_stdio_and_process_group() {
    let exe = std::env::current_exe().unwrap();
    if exe
        .parent()
        .unwrap()
        .ends_with(format!("{BUNDLE}/Contents/MacOS"))
    {
        let root = exe.ancestors().nth(4).unwrap();
        if !root.join("restarting").exists() {
            fs::write(root.join("first-group"), process_group(std::process::id())).unwrap();
            fs::write(root.join("restarting"), "").unwrap();
            tauri::process::restart(&tauri::Env::default());
        }

        // Wait until the old process exits and the launcher stops reading its pipes.
        assert!(wait_for_file(&root.join("pipes-closed")));
        std::io::stdout().write_all(b"restarted stdout\n").unwrap();
        std::io::stderr().write_all(b"restarted stderr\n").unwrap();
        assert_ne!(
            process_group(std::process::id()),
            fs::read_to_string(root.join("first-group")).unwrap()
        );
        assert_eq!(std::env::args().nth(1).as_deref(), Some(TEST_NAME));
        fs::write(root.join("report"), "ok").unwrap();
        return;
    }

    // Clean up after success and failures. Canonicalize the /var symlink:
    // Tauri's macOS executable security check deliberately rejects symlink paths.
    let nonce = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap()
        .as_nanos();
    let root = std::env::temp_dir().canonicalize().unwrap().join(format!(
        "gitwave-restart-fixture-{}-{nonce}",
        std::process::id()
    ));
    fs::create_dir(&root).unwrap();
    let _cleanup = FixtureDir(root.clone());
    let contents = root.join(BUNDLE).join("Contents");
    let macos = contents.join("MacOS");
    fs::create_dir_all(&macos).unwrap();
    let name = exe.file_name().unwrap().to_str().unwrap();
    let bundled = macos.join(name);
    fs::copy(&exe, &bundled).unwrap();
    fs::write(
        contents.join("Info.plist"),
        format!(
            r#"<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0"><dict>
<key>CFBundleExecutable</key><string>{name}</string>
<key>CFBundleIdentifier</key><string>work.gitwave.restart-fixture-{}</string>
<key>CFBundlePackageType</key><string>APPL</string>
</dict></plist>"#,
            std::process::id()
        ),
    )
    .unwrap();

    let mut child = Command::new(bundled)
        .args([TEST_NAME, "--exact", "--nocapture", "--test-threads=1"])
        .stdin(Stdio::null())
        .stdout(Stdio::piped())
        .stderr(Stdio::piped())
        .spawn()
        .unwrap();
    let deadline = Instant::now() + Duration::from_secs(15);
    let status = loop {
        if let Some(status) = child.try_wait().unwrap() {
            break status;
        }
        if Instant::now() >= deadline {
            child.kill().unwrap();
            child.wait().unwrap();
            panic!("restart did not exit the first instance");
        }
        std::thread::sleep(Duration::from_millis(50));
    };
    assert!(status.success(), "first instance failed: {status}");
    drop(child.stdout.take());
    drop(child.stderr.take());
    fs::write(root.join("pipes-closed"), "").unwrap();
    assert!(
        wait_for_file(&root.join("report")),
        "restarted bundle did not survive the old instance exiting and closing its pipes"
    );
    assert_eq!(fs::read_to_string(root.join("report")).unwrap(), "ok");
}
