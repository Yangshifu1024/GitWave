//! Host integration that needs no UI framework: locating and launching the
//! user's terminal and editors with a repository as the working directory.

/// Absolute path of `prog` if it sits in a PATH directory (std-only `which`).
fn find_in_path(prog: &str) -> Option<std::path::PathBuf> {
    let path = std::env::var_os("PATH")?;
    std::env::split_paths(&path)
        .map(|dir| dir.join(prog))
        .find(|candidate| candidate.is_file())
}

/// Full terminal launch argv per platform; the repo is also set as the
/// child's cwd, so entries that take no dir flag (cmd.exe, `$TERMINAL`,
/// kitty / alacritty / foot) still start in the repo.
#[must_use]
pub fn detect_terminal_command() -> Option<(String, Vec<String>)> {
    let dir_arg = |flags: &[&str]| -> Vec<String> { flags.iter().map(|f| f.to_string()).collect() };
    if cfg!(target_os = "windows") {
        if find_in_path("wt.exe").is_some() {
            Some(("wt.exe".into(), vec!["-d".into()]))
        } else {
            // Pre-Windows-Terminal boxes: a plain console on the repo dir.
            Some(("cmd.exe".into(), vec![]))
        }
    } else if cfg!(target_os = "macos") {
        let iterm = ["/Applications/iTerm.app", "~/Applications/iTerm.app"]
            .iter()
            .any(|p| {
                let p = std::path::Path::new(p);
                let p = p
                    .strip_prefix("~")
                    .map(|rest| {
                        std::env::var("HOME").map(|h| std::path::PathBuf::from(h).join(rest))
                    })
                    .unwrap_or_else(|_| Ok(p.to_path_buf()));
                p.map(|p| p.exists()).unwrap_or(false)
            });
        let prog = if iterm { "iTerm" } else { "Terminal" };
        Some(("open".into(), vec!["-a".into(), prog.into()]))
    } else {
        // Linux/BSD: $TERMINAL first, then common emulators. Flag-less
        // entries rely on the cwd set by the caller.
        if let Ok(t) = std::env::var("TERMINAL") {
            if !t.is_empty() {
                return Some((t, vec![]));
            }
        }
        let candidates: [(&str, &[&str]); 8] = [
            ("gnome-terminal", &["--working-directory"]),
            ("konsole", &["--workdir"]),
            ("xfce4-terminal", &["--working-directory"]),
            ("kgx", &["--working-directory"]),
            ("kitty", &[]),
            ("alacritty", &[]),
            ("foot", &[]),
            ("wezterm", &["start", "--cwd"]),
        ];
        candidates
            .iter()
            .find(|(prog, _)| find_in_path(prog).is_some())
            .map(|(prog, flags)| ((*prog).to_string(), dir_arg(flags)))
    }
}

/// Opens a repo working tree in a detected terminal emulator with the repo
/// as the working directory. Detached from the app: windows gets a new
/// console, unix a new process group.
pub fn open_in_terminal(path: &str) -> Result<(), String> {
    use std::process::Command;

    let dir = std::path::PathBuf::from(path);
    if !dir.is_dir() {
        return Err(format!("not a directory: {path}"));
    }
    let (prog, mut args) =
        detect_terminal_command().ok_or_else(|| "no terminal emulator found".to_string())?;
    // cmd.exe / `$TERMINAL` pass no dir flag — their argv stays empty so the
    // cwd (set below) is the only directory source; everything else takes the
    // repo as its final argument.
    if !args.is_empty() {
        args.push(dir.display().to_string());
    }
    let mut cmd = Command::new(&prog);
    cmd.args(&args).current_dir(&dir);
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        const CREATE_NEW_CONSOLE: u32 = 0x0000_0010;
        const CREATE_NO_WINDOW: u32 = 0x0800_0000;
        // wt.exe relays to WindowsTerminal.exe and exits; a hidden relay avoids
        // a console flash. cmd.exe needs a real new console of its own.
        cmd.creation_flags(if prog == "wt.exe" {
            CREATE_NO_WINDOW
        } else {
            CREATE_NEW_CONSOLE
        });
    }
    #[cfg(unix)]
    {
        use std::os::unix::process::CommandExt;
        cmd.process_group(0);
    }
    cmd.spawn()
        .map_err(|e| format!("failed to launch {prog}: {e}"))?;
    Ok(())
}

// ─── External editors (F015) ─────────────────────────────────────────────

/// Supported editors in default order; the first detected one leads the
/// ActionBar dropdown.
const SUPPORTED_EDITORS: [(&str, &str); 3] = [
    ("vscode", "Visual Studio Code"),
    ("zed", "Zed"),
    ("vscodium", "VSCodium"),
];

/// A locally installed editor offered in the ActionBar dropdown.
#[derive(serde::Serialize)]
pub struct DetectedEditor {
    pub id: String,
    pub name: String,
}

/// Well-known Windows install roots for `id`, in lookup order (pure — roots
/// are passed in so tests can pin the order without touching the environment).
#[cfg(windows)]
fn windows_exe_candidates(
    id: &str,
    local_app_data: &str,
    program_files: &str,
    program_files_x86: &str,
) -> Vec<String> {
    match id {
        "vscode" => vec![
            format!(r"{local_app_data}\Programs\Microsoft VS Code\Code.exe"),
            format!(r"{program_files}\Microsoft VS Code\Code.exe"),
            format!(r"{program_files_x86}\Microsoft VS Code\Code.exe"),
        ],
        "zed" => vec![format!(r"{local_app_data}\Programs\Zed\Zed.exe")],
        "vscodium" => vec![
            format!(r"{local_app_data}\Programs\VSCodium\VSCodium.exe"),
            format!(r"{program_files}\VSCodium\VSCodium.exe"),
        ],
        _ => Vec::new(),
    }
}

/// Where `id` is installed and the argv that opens a directory in it; `None`
/// when the editor is not installed (or the id is unknown). The repo dir is
/// appended as the final argument by the caller, which also sets it as cwd —
/// all three editors accept a folder path that way.
#[cfg(windows)]
fn editor_launch(id: &str) -> Option<(String, Vec<String>)> {
    let env = |k: &str| std::env::var(k).unwrap_or_default();
    windows_exe_candidates(
        id,
        &env("LOCALAPPDATA"),
        &env("ProgramFiles"),
        &env("ProgramFiles(x86)"),
    )
    .into_iter()
    .find(|p| std::path::Path::new(p).is_file())
    .map(|exe| (exe, Vec::new()))
    .or_else(|| match id {
        // Scoop / portable installs only expose the PATH shim; `.cmd` must
        // relay through cmd.exe (spawned windowless by `open_in_editor`).
        "vscode" => {
            find_in_path("code.cmd").map(|_| ("cmd".into(), vec!["/c".into(), "code.cmd".into()]))
        }
        "zed" => find_in_path("zed.exe").map(|_| ("zed.exe".into(), Vec::new())),
        "vscodium" => find_in_path("codium.cmd")
            .map(|_| ("cmd".into(), vec!["/c".into(), "codium.cmd".into()])),
        _ => None,
    })
}

#[cfg(target_os = "macos")]
fn editor_launch(id: &str) -> Option<(String, Vec<String>)> {
    let app = match id {
        "vscode" => "Visual Studio Code",
        "zed" => "Zed",
        "vscodium" => "VSCodium",
        _ => return None,
    };
    let home = std::env::var("HOME").unwrap_or_default();
    let installed = [
        format!("/Applications/{app}.app"),
        format!("{home}/Applications/{app}.app"),
    ]
    .iter()
    .any(|p| std::path::Path::new(p).exists());
    installed.then(|| ("open".to_string(), vec!["-a".to_string(), app.to_string()]))
}

#[cfg(all(unix, not(target_os = "macos")))]
fn editor_launch(id: &str) -> Option<(String, Vec<String>)> {
    let bin = match id {
        "vscode" => "code",
        "zed" => "zed",
        "vscodium" => "codium",
        _ => return None,
    };
    find_in_path(bin).map(|_| (bin.to_string(), Vec::new()))
}

/// Editors installed on this machine, in default order (first = default).
#[must_use]
pub fn detect_editors() -> Vec<DetectedEditor> {
    SUPPORTED_EDITORS
        .iter()
        .filter(|(id, _)| editor_launch(id).is_some())
        .map(|(id, name)| DetectedEditor {
            id: (*id).to_string(),
            name: (*name).to_string(),
        })
        .collect()
}

/// Opens a repo working tree in the chosen editor. Detached like the terminal
/// opener; the dir is both the child's cwd and its final argument.
pub fn open_in_editor(path: &str, editor_id: &str) -> Result<(), String> {
    use std::process::Command;

    let dir = std::path::PathBuf::from(path);
    if !dir.is_dir() {
        return Err(format!("not a directory: {path}"));
    }
    let (prog, mut args) =
        editor_launch(editor_id).ok_or_else(|| format!("editor not found: {editor_id}"))?;
    args.push(dir.display().to_string());
    let mut cmd = Command::new(&prog);
    cmd.args(&args).current_dir(&dir);
    #[cfg(windows)]
    {
        use std::os::windows::process::CommandExt;
        const CREATE_NO_WINDOW: u32 = 0x0800_0000;
        // The `cmd /c code.cmd` shim would flash a console window; editor
        // exes spawned directly open windowless.
        if prog == "cmd" {
            cmd.creation_flags(CREATE_NO_WINDOW);
        }
    }
    #[cfg(unix)]
    {
        use std::os::unix::process::CommandExt;
        cmd.process_group(0);
    }
    cmd.spawn()
        .map_err(|e| format!("failed to launch {prog}: {e}"))?;
    Ok(())
}

#[cfg(test)]
mod editor_tests {
    use super::{detect_editors, editor_launch, SUPPORTED_EDITORS};

    #[test]
    fn supported_editors_have_stable_default_order() {
        let ids: Vec<_> = SUPPORTED_EDITORS.iter().map(|(id, _)| *id).collect();
        assert_eq!(ids, ["vscode", "zed", "vscodium"]);
        assert!(SUPPORTED_EDITORS.iter().all(|(_, name)| !name.is_empty()));
    }

    #[test]
    fn unknown_editor_id_is_not_launchable() {
        assert!(editor_launch("notepad").is_none());
        assert!(editor_launch("").is_none());
    }

    #[test]
    fn detected_editors_are_a_subset_in_supported_order() {
        let detected = detect_editors();
        let order: Vec<_> = SUPPORTED_EDITORS.iter().map(|(id, _)| *id).collect();
        assert!(detected.iter().all(|e| order.contains(&e.id.as_str())));
        let detected_ids: Vec<_> = detected.iter().map(|e| e.id.as_str()).collect();
        let mut sorted = detected_ids.clone();
        sorted.sort_by_key(|id| order.iter().position(|o| o == id).unwrap());
        assert_eq!(detected_ids, sorted);
    }

    #[cfg(windows)]
    #[test]
    fn windows_exe_candidates_cover_well_known_install_roots() {
        use super::windows_exe_candidates;
        let (lad, pf, pfx86) = (
            r"C:\Users\u\AppData\Local",
            r"C:\Program Files",
            r"C:\Program Files (x86)",
        );
        assert_eq!(
            windows_exe_candidates("vscode", lad, pf, pfx86),
            vec![
                format!(r"{lad}\Programs\Microsoft VS Code\Code.exe"),
                format!(r"{pf}\Microsoft VS Code\Code.exe"),
                format!(r"{pfx86}\Microsoft VS Code\Code.exe"),
            ]
        );
        assert_eq!(
            windows_exe_candidates("zed", lad, pf, pfx86),
            vec![format!(r"{lad}\Programs\Zed\Zed.exe")]
        );
        assert_eq!(
            windows_exe_candidates("vscodium", lad, pf, pfx86),
            vec![
                format!(r"{lad}\Programs\VSCodium\VSCodium.exe"),
                format!(r"{pf}\VSCodium\VSCodium.exe"),
            ]
        );
        assert!(windows_exe_candidates("notepad", lad, pf, pfx86).is_empty());
    }
}
