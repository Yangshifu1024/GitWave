//! Discover locally installed terminals. IPC receives an ID; launch paths and
//! arguments are resolved again from this catalog when the user clicks.
use super::DetectedTool;
use std::path::{Path, PathBuf};
use std::process::Command;

struct Terminal {
    tool: DetectedTool,
    executable: PathBuf,
}

impl Terminal {
    fn new(id: &str, name: &str, executable: PathBuf) -> Self {
        Self {
            tool: DetectedTool {
                id: id.into(),
                name: name.into(),
            },
            executable,
        }
    }
}

pub(super) fn list() -> Vec<DetectedTool> {
    installed().into_iter().map(|t| t.tool).collect()
}

pub(super) fn prepare(path: &str, id: Option<&str>) -> Result<(Command, String), String> {
    let dir = std::path::absolute(path).map_err(|err| err.to_string())?;
    if !dir.is_dir() {
        return Err(format!("not a repository directory: {path}"));
    }
    let mut terminals = installed().into_iter();
    let selected = match id {
        Some(id) => terminals.find(|t| t.tool.id == id),
        None => terminals.next(), // Older callers used automatic selection.
    }
    .ok_or_else(|| format!("terminal unavailable: {}", id.unwrap_or("system default")))?;
    let mut command = launch_command(&selected, &dir);
    if command.get_current_dir().is_none() {
        command.current_dir(dir);
    }
    Ok((command, selected.tool.name))
}

#[cfg(all(unix, not(target_os = "macos")))]
const LINUX_TOOLS: &[(&str, &str, &str)] = &[
    ("gnome-terminal", "GNOME Terminal", "org.gnome.Terminal"),
    ("konsole", "Konsole", "org.kde.konsole"),
    ("xfce4-terminal", "Xfce Terminal", "org.xfce.Terminal"),
    ("kgx", "GNOME Console", "org.gnome.Console"),
    ("ptyxis", "Ptyxis", "org.gnome.Ptyxis"),
    ("ghostty", "Ghostty", "com.mitchellh.ghostty"),
    ("kitty", "kitty", "net.kovidgoyal.kitty"),
    ("alacritty", "Alacritty", "org.alacritty.Alacritty"),
    ("foot", "foot", ""),
    ("wezterm", "WezTerm", "org.wezfurlong.wezterm"),
    ("tilix", "Tilix", "com.gexperts.Tilix"),
    ("xterm", "XTerm", ""),
];

#[cfg(unix)]
pub(super) fn executable(path: &Path) -> bool {
    use std::os::unix::fs::PermissionsExt;
    path.metadata()
        .is_ok_and(|m| m.is_file() && m.permissions().mode() & 0o111 != 0)
}

#[cfg(all(unix, not(target_os = "macos")))]
fn installed() -> Vec<Terminal> {
    let path = std::env::var_os("PATH").unwrap_or_default();
    let mut dirs: Vec<_> = std::env::split_paths(&path)
        .filter(|p| p.is_absolute())
        .collect();
    dirs.push(PathBuf::from("/snap/bin"));
    let mut exports = vec![PathBuf::from("/var/lib/flatpak/exports/bin")];
    if let Some(data) = dirs::data_local_dir() {
        exports.insert(0, data.join("flatpak/exports/bin"));
    }
    linux_terminals(&dirs, &exports, std::env::var("TERMINAL").ok().as_deref())
}

#[cfg(all(unix, not(target_os = "macos")))]
fn linux_terminals(
    dirs: &[PathBuf],
    exports: &[PathBuf],
    preferred: Option<&str>,
) -> Vec<Terminal> {
    let lookup = |bin: &str| dirs.iter().map(|p| p.join(bin)).find(|p| executable(p));
    let mut tools: Vec<_> = LINUX_TOOLS
        .iter()
        .filter_map(|&(id, name, app)| {
            let path = lookup(id)
                .or_else(|| exports.iter().map(|p| p.join(app)).find(|p| executable(p)))?;
            Some(Terminal::new(id, name, path))
        })
        .collect();
    if let Some(value) = preferred.filter(|v| !v.is_empty()) {
        let candidate = Path::new(value);
        let path = if candidate.is_absolute() {
            executable(candidate).then(|| candidate.to_owned())
        } else if candidate.components().count() == 1 {
            lookup(value)
        } else {
            None
        };
        if let Some(path) = path {
            let resolved = path.canonicalize().ok();
            // Prefer the exact executable before resolving symlinks. Snap app
            // aliases can all resolve to the same dispatcher binary.
            let same_tool = tools.iter().position(|t| t.executable == path).or_else(|| {
                tools.iter().position(|t| {
                    resolved
                        .as_ref()
                        .is_some_and(|p| t.executable.canonicalize().as_ref().ok() == Some(p))
                })
            });
            if let Some(i) = same_tool {
                tools[..=i].rotate_right(1);
            } else if let Some(name) = path.file_name().and_then(|n| n.to_str()) {
                let id = format!("env:{value}");
                tools.insert(0, Terminal::new(&id, name, path.clone()));
            }
        }
    }
    tools
}

#[cfg(all(unix, not(target_os = "macos")))]
fn launch_command(terminal: &Terminal, dir: &Path) -> Command {
    let mut cmd = Command::new(&terminal.executable);
    match terminal.tool.id.as_str() {
        "konsole" => {
            cmd.arg("--workdir").arg(dir);
        }
        "wezterm" => {
            cmd.args(["start", "--cwd"]).arg(dir);
        }
        "ghostty" => {
            cmd.arg(format!("--working-directory={}", dir.display()));
        }
        "gnome-terminal" | "kgx" | "xfce4-terminal" | "ptyxis" | "tilix" | "alacritty" | "foot" => {
            cmd.arg("--working-directory").arg(dir);
        }
        "kitty" => {
            cmd.arg("--directory").arg(dir);
        }
        _ => {} // xterm and a custom $TERMINAL inherit the child's cwd.
    }
    cmd
}

#[cfg(target_os = "macos")]
fn installed() -> Vec<Terminal> {
    let mut dirs = vec![
        PathBuf::from("/Applications"),
        PathBuf::from("/System/Applications/Utilities"),
        PathBuf::from("/Applications/Utilities"),
    ];
    if let Some(home) = dirs::home_dir() {
        dirs.insert(0, home.join("Applications"));
    }
    [
        ("iterm2", "iTerm2", "iTerm.app", None),
        ("terminal", "Terminal", "Terminal.app", None),
        ("ghostty", "Ghostty", "Ghostty.app", None),
        ("warp", "Warp", "Warp.app", None),
        ("wezterm", "WezTerm", "WezTerm.app", Some("wezterm")),
        ("kitty", "kitty", "kitty.app", Some("kitty")),
        ("alacritty", "Alacritty", "Alacritty.app", Some("alacritty")),
    ]
    .into_iter()
    .filter_map(|(id, name, app, cli)| {
        let path = dirs.iter().map(|p| p.join(app)).find(|p| p.is_dir())?;
        let path = if let Some(cli) = cli {
            let bin = path.join("Contents/MacOS").join(cli);
            executable(&bin).then_some(bin)?
        } else {
            path
        };
        Some(Terminal::new(id, name, path))
    })
    .collect()
}

#[cfg(target_os = "macos")]
fn launch_command(terminal: &Terminal, dir: &Path) -> Command {
    match terminal.tool.id.as_str() {
        "wezterm" | "kitty" | "alacritty" => {
            let mut cmd = Command::new(&terminal.executable);
            match terminal.tool.id.as_str() {
                "wezterm" => cmd.args(["start", "--cwd"]),
                "kitty" => cmd.arg("--directory"),
                _ => cmd.arg("--working-directory"),
            }
            .arg(dir);
            cmd
        }
        _ => {
            let mut cmd = Command::new("/usr/bin/open");
            cmd.arg("-a").arg(&terminal.executable);
            if terminal.tool.id == "warp" {
                let mut url =
                    tauri::Url::parse("warp://action/new_window").expect("static Warp URI");
                url.query_pairs_mut()
                    .append_pair("path", &dir.to_string_lossy());
                cmd.arg(url.as_str());
            } else {
                cmd.arg(dir);
            }
            cmd
        }
    }
}

#[cfg(windows)]
fn installed() -> Vec<Terminal> {
    let candidates = [
        (
            "wt",
            "Windows Terminal",
            "wt.exe",
            r"Microsoft\WindowsApps\wt.exe",
        ),
        (
            "wezterm",
            "WezTerm",
            "wezterm-gui.exe",
            r"WezTerm\wezterm-gui.exe",
        ),
        (
            "alacritty",
            "Alacritty",
            "alacritty.exe",
            r"Alacritty\alacritty.exe",
        ),
        ("pwsh", "PowerShell 7", "pwsh.exe", r"PowerShell\7\pwsh.exe"),
        (
            "powershell",
            "Windows PowerShell",
            "powershell.exe",
            r"System32\WindowsPowerShell\v1.0\powershell.exe",
        ),
        ("cmd", "Command Prompt", "cmd.exe", r"System32\cmd.exe"),
        ("git-bash", "Git Bash", "git-bash.exe", r"Git\git-bash.exe"),
    ];
    candidates
        .into_iter()
        .filter_map(|(id, name, exe, relative)| {
            let roots: &[&str] = match id {
                "cmd" | "powershell" => &["SystemRoot"],
                "wt" => &["LOCALAPPDATA"],
                _ => &["ProgramFiles", "ProgramFiles(x86)", "LOCALAPPDATA"],
            };
            let path = roots
                .iter()
                .filter_map(|key| {
                    let root = PathBuf::from(std::env::var_os(key)?);
                    if !root.is_absolute() {
                        return None;
                    }
                    let base = if *key == "LOCALAPPDATA" && id != "wt" {
                        root.join("Programs")
                    } else {
                        root
                    };
                    Some(base.join(relative))
                })
                .find(|p| p.is_file())
                .or_else(|| super::find_in_path(exe))?;
            Some(Terminal::new(id, name, path))
        })
        .collect()
}

#[cfg(windows)]
fn launch_command(terminal: &Terminal, dir: &Path) -> Command {
    use std::os::windows::process::CommandExt;
    let mut cmd = Command::new(&terminal.executable);
    match terminal.tool.id.as_str() {
        "wt" => {
            cmd.arg("-d").arg(dir);
        }
        "wezterm" => {
            cmd.args(["start", "--cwd"]).arg(dir);
        }
        "alacritty" => {
            cmd.arg("--working-directory").arg(dir);
        }
        "git-bash" => {
            cmd.arg(format!("--cd={}", dir.display()));
        }
        "cmd" if matches!(dir.components().next(), Some(std::path::Component::Prefix(prefix)) if matches!(prefix.kind(), std::path::Prefix::UNC(..) | std::path::Prefix::VerbatimUNC(..))) =>
        {
            cmd.args(["/d", "/e:on", "/v:off", "/k"])
                .raw_arg(r#"pushd "%GITWAVE_REPO_DIR%""#)
                .env("GITWAVE_REPO_DIR", dir);
            if let Some(root) = std::env::var_os("SystemRoot") {
                cmd.current_dir(root);
            }
        }
        _ => {} // Shells start in a new console and inherit the repo cwd.
    }
    const CREATE_NEW_CONSOLE: u32 = 0x10;
    const CREATE_NO_WINDOW: u32 = 0x08000000;
    cmd.creation_flags(
        if matches!(terminal.tool.id.as_str(), "pwsh" | "powershell" | "cmd") {
            CREATE_NEW_CONSOLE
        } else {
            CREATE_NO_WINDOW
        },
    );
    cmd
}

#[cfg(all(test, unix, not(target_os = "macos")))]
mod tests {
    use super::*;
    use std::{
        fs,
        os::unix::fs::{symlink, PermissionsExt},
    };

    fn write_executable(path: &Path) {
        fs::write(path, "#!/bin/sh\nexit 0\n").unwrap();
        fs::set_permissions(path, fs::Permissions::from_mode(0o755)).unwrap();
    }

    #[test]
    fn discovers_all_installed_tools_and_deduplicates_the_preferred_alias() {
        let temp = tempfile::tempdir().unwrap();
        let root = temp.path();
        write_executable(&root.join("konsole"));
        write_executable(&root.join("kitty"));
        fs::write(root.join("kgx"), "not executable").unwrap();
        symlink(root.join("kitty"), root.join("my-terminal")).unwrap();
        let dirs = vec![root.to_owned()];
        let tools = linux_terminals(&dirs, &[], Some("my-terminal"));
        assert_eq!(
            tools.iter().map(|t| t.tool.id.as_str()).collect::<Vec<_>>(),
            ["kitty", "konsole"]
        );
        write_executable(&root.join("custom-terminal"));
        let custom = linux_terminals(&dirs, &[], Some("custom-terminal"));
        assert_eq!(custom[0].tool.id, "env:custom-terminal");
        assert_eq!(custom.len(), 3);
        assert_eq!(linux_terminals(&dirs, &[], Some("missing")).len(), 2);
    }

    #[test]
    fn exported_flatpak_uses_catalog_id_and_passes_repo_as_one_argument() {
        let temp = tempfile::tempdir().unwrap();
        let bin = temp.path().join("org.gnome.Ptyxis");
        write_executable(&bin);
        let tools = linux_terminals(&[], &[temp.path().to_owned()], None);
        assert_eq!(tools.len(), 1);
        assert_eq!(tools[0].tool.id, "ptyxis");
        let dir = Path::new("/tmp/repo with spaces 合并");
        let cmd = launch_command(&tools[0], dir);
        assert_eq!(cmd.get_program(), bin.as_os_str());
        assert_eq!(
            cmd.get_args().collect::<Vec<_>>(),
            ["--working-directory".as_ref(), dir.as_os_str()]
        );
    }
}

#[cfg(all(test, unix))]
mod launch_tests {
    use super::*;
    #[tokio::test]
    async fn propagates_immediate_failure_and_preserves_child_directory() {
        let tmp = tempfile::tempdir().unwrap();
        let dir = tmp.path().join("repository 中文 & spaces");
        std::fs::create_dir(&dir).unwrap();
        let mut ok = Command::new("/bin/sh");
        ok.current_dir(&dir).args(["-c", "pwd -P > cwd.txt"]);
        crate::launch_external(ok, "test launcher").await.unwrap();
        assert_eq!(
            std::fs::read_to_string(dir.join("cwd.txt")).unwrap().trim(),
            dir.canonicalize().unwrap().to_str().unwrap()
        );
        let mut failed = Command::new("/bin/sh");
        failed.args(["-c", "exit 9"]);
        assert!(crate::launch_external(failed, "test launcher")
            .await
            .unwrap_err()
            .contains("test launcher"));
    }
}

#[cfg(all(test, target_os = "macos"))]
mod mac_tests {
    use super::*;
    #[test]
    fn warp_url_round_trips_repository_path() {
        let dir = Path::new("/tmp/测试 & spaces/#repo");
        let cmd = launch_command(
            &Terminal::new("warp", "Warp", "/Applications/Warp.app".into()),
            dir,
        );
        let url = tauri::Url::parse(cmd.get_args().last().unwrap().to_str().unwrap()).unwrap();
        assert_eq!(url.query_pairs().next().unwrap().1, dir.to_str().unwrap());
    }
}

#[cfg(all(test, windows))]
mod windows_tests {
    use super::*;
    #[test]
    fn explicit_directory_is_a_single_argument() {
        let dir = Path::new(r"C:\work\repo 中文 & spaces");
        let cmd = launch_command(
            &Terminal::new("wt", "Windows Terminal", "wt.exe".into()),
            dir,
        );
        assert_eq!(
            cmd.get_args().collect::<Vec<_>>(),
            ["-d".as_ref(), dir.as_os_str()]
        );
    }
}
