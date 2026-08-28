//! `gitwave-mcp` — Model Context Protocol server (stdio, read-only).
//!
//! Exposes GitWave's read-only repository queries (status / history /
//! branches / tags / remotes / file) to MCP clients such as Claude or
//! Cursor. No write tools exist, mirroring the P1 principle: this server
//! can never mutate a repository.
//!
//! Protocol: newline-delimited JSON-RPC 2.0 over stdin/stdout (MCP stdio).
//! Usage: `gitwave-mcp --repo <path-to-workdir>`

use std::io::{BufRead, Write};
use std::path::{Path, PathBuf};

use serde_json::{json, Value};

use gitwave_lib::infrastructure::git::{history, remote, tag, working_copy};

const PROTOCOL_VERSION: &str = "2024-11-05";
const SERVER_NAME: &str = "gitwave-mcp";

fn tool_defs() -> Value {
    json!({
        "tools": [
            {
                "name": "repo_status",
                "description": "Working copy status: current branch, upstream, ahead/behind, staged/unstaged/untracked files.",
                "inputSchema": { "type": "object", "properties": {} }
            },
            {
                "name": "commit_history",
                "description": "Recent commits across all branches, newest first.",
                "inputSchema": {
                    "type": "object",
                    "properties": {
                        "max": { "type": "integer", "description": "Max commits to return (default 50)." },
                        "filter": { "type": "string", "description": "Case-insensitive substring filter on message/author." }
                    }
                }
            },
            {
                "name": "list_branches",
                "description": "All local and remote-tracking branches with tips and ahead/behind.",
                "inputSchema": { "type": "object", "properties": {} }
            },
            {
                "name": "list_tags",
                "description": "All tags with their target commit and annotation.",
                "inputSchema": { "type": "object", "properties": {} }
            },
            {
                "name": "list_remotes",
                "description": "Configured remotes with fetch/push URLs.",
                "inputSchema": { "type": "object", "properties": {} }
            },
            {
                "name": "read_file",
                "description": "Read a text file from the repository working tree.",
                "inputSchema": {
                    "type": "object",
                    "properties": {
                        "path": { "type": "string", "description": "Repo-relative file path." }
                    },
                    "required": ["path"]
                }
            }
        ]
    })
}

fn call_tool(repo_path: &Path, name: &str, args: &Value) -> Result<Value, String> {
    use gitwave_lib::domain::error::AppError;
    let text = match name {
        "repo_status" => with_repo(repo_path, |repo| {
            serde_json::to_string_pretty(&working_copy::status(repo, "mcp")?)
                .map_err(|e| AppError::Unknown(format!("serialize: {e}")))
        })?,
        "commit_history" => with_repo(repo_path, |repo| {
            let max = args
                .get("max")
                .and_then(|v| v.as_u64())
                .unwrap_or(50)
                .min(1000);
            let filter = args.get("filter").and_then(|v| v.as_str());
            serde_json::to_string_pretty(&history::commit_log(repo, max as u32, filter)?)
                .map_err(|e| AppError::Unknown(format!("serialize: {e}")))
        })?,
        "list_branches" => with_repo(repo_path, |repo| {
            serde_json::to_string_pretty(&history::list_branches(repo)?)
                .map_err(|e| AppError::Unknown(format!("serialize: {e}")))
        })?,
        "list_tags" => with_repo(repo_path, |repo| {
            serde_json::to_string_pretty(&tag::list_tags(repo)?)
                .map_err(|e| AppError::Unknown(format!("serialize: {e}")))
        })?,
        "list_remotes" => with_repo(repo_path, |repo| {
            serde_json::to_string_pretty(&remote::list_remote_details(repo)?)
                .map_err(|e| AppError::Unknown(format!("serialize: {e}")))
        })?,
        "read_file" => {
            let path = args
                .get("path")
                .and_then(|v| v.as_str())
                .ok_or("missing argument: path")?;
            read_file(repo_path, path)?
        }
        other => return Err(format!("unknown tool: {other}")),
    };
    Ok(json!({ "content": [{ "type": "text", "text": text }] }))
}

fn with_repo<T>(
    repo_path: &Path,
    f: impl FnOnce(&git2::Repository) -> Result<T, gitwave_lib::domain::error::AppError>,
) -> Result<T, String> {
    let repo =
        git2::Repository::open(repo_path).map_err(|e| format!("cannot open repository: {e}"))?;
    f(&repo).map_err(|e| e.to_string())
}

/// Repo-relative file read. Hardened for the MCP threat model: lexical
/// checks (absolute paths, `..` segments) plus canonicalization so symlinks
/// inside the worktree cannot resolve outside it, and a blanket refusal to
/// read `.git` internals (config may carry credential-bearing URLs).
fn read_file(repo_path: &Path, rel: &str) -> Result<String, String> {
    if Path::new(rel).is_absolute() || rel.split(['/', '\\']).any(|seg| seg == "..") {
        return Err("path escapes worktree".into());
    }
    let abs = repo_path.join(rel);
    if !abs.starts_with(repo_path) {
        return Err("path escapes worktree".into());
    }
    let canonical_root =
        std::fs::canonicalize(repo_path).map_err(|e| format!("resolve worktree: {e}"))?;
    let canonical = std::fs::canonicalize(&abs).map_err(|e| format!("resolve {rel}: {e}"))?;
    if !canonical.starts_with(&canonical_root) {
        return Err("path escapes worktree".into());
    }
    let inside_git_dir = canonical
        .strip_prefix(&canonical_root)
        .map(|rel| {
            rel.components()
                .any(|c| c.as_os_str().eq_ignore_ascii_case(".git"))
        })
        .unwrap_or(true);
    if inside_git_dir {
        return Err("reading .git internals is not allowed".into());
    }
    std::fs::read_to_string(&canonical).map_err(|e| format!("read {rel}: {e}"))
}

/// Handle one JSON-RPC message. Returns the response for requests; `None`
/// for notifications.
fn handle(msg: &Value, repo_path: &Path) -> Option<Value> {
    let method = msg.get("method")?.as_str()?.to_string();
    let id = msg.get("id").cloned();

    // Notifications have no id — acknowledge nothing.
    id.as_ref()?;

    let result: Result<Value, String> = match method.as_str() {
        "initialize" => Ok(json!({
            "protocolVersion": PROTOCOL_VERSION,
            "capabilities": { "tools": {} },
            "serverInfo": { "name": SERVER_NAME, "version": env!("CARGO_PKG_VERSION") }
        })),
        "ping" => Ok(json!({})),
        "tools/list" => Ok(tool_defs()),
        "tools/call" => {
            let name = msg["params"]["name"].as_str().unwrap_or("").to_string();
            let args = if msg["params"]["arguments"].is_null() {
                json!({})
            } else {
                msg["params"]["arguments"].clone()
            };
            match call_tool(repo_path, &name, &args) {
                Ok(content) => Ok(content),
                Err(e) => Ok(json!({
                    "content": [{ "type": "text", "text": e }],
                    "isError": true
                })),
            }
        }
        other => Err(format!("method not found: {other}")),
    };

    Some(match result {
        Ok(result) => json!({ "jsonrpc": "2.0", "id": id, "result": result }),
        Err(error) => {
            json!({ "jsonrpc": "2.0", "id": id, "error": { "code": -32603, "message": error } })
        }
    })
}

fn repo_path_from_args() -> Result<PathBuf, String> {
    let mut args = std::env::args().skip(1);
    while let Some(arg) = args.next() {
        if arg == "--repo" {
            let path = args.next().ok_or("--repo requires a value")?;
            let path = PathBuf::from(path);
            if !path.is_dir() {
                return Err(format!("not a directory: {}", path.display()));
            }
            return Ok(path);
        }
        if arg == "--help" || arg == "-h" {
            println!("gitwave-mcp --repo <path>: read-only MCP server for a git repository");
            std::process::exit(0);
        }
    }
    Err("missing --repo <path>".into())
}

fn main() {
    let repo_path = match repo_path_from_args() {
        Ok(p) => p,
        Err(e) => {
            eprintln!("{e}");
            std::process::exit(2);
        }
    };

    let stdin = std::io::stdin();
    let mut stdout = std::io::stdout().lock();
    for line in stdin.lock().lines() {
        let Ok(line) = line else { break };
        if line.trim().is_empty() {
            continue;
        }
        let Ok(msg) = serde_json::from_str::<Value>(&line) else {
            continue;
        };
        if let Some(response) = handle(&msg, &repo_path) {
            let Ok(mut out) = serde_json::to_string(&response) else {
                continue;
            };
            out.push('\n');
            if stdout.write_all(out.as_bytes()).is_err() {
                break; // client closed the pipe
            }
            let _ = stdout.flush();
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use git2::Repository;

    fn test_repo() -> PathBuf {
        let dir = std::env::temp_dir().join(format!(
            "gitwave-mcp-test-{}-{}",
            std::process::id(),
            std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_nanos()
        ));
        {
            let repo = Repository::init(&dir).unwrap();
            std::fs::write(dir.join("a.txt"), "hello\n").unwrap();
            let mut index = repo.index().unwrap();
            index.add_path(Path::new("a.txt")).unwrap();
            let tree_id = index.write_tree().unwrap();
            let tree = repo.find_tree(tree_id).unwrap();
            let sig = git2::Signature::now("T", "t@l").unwrap();
            repo.commit(Some("HEAD"), &sig, &sig, "init commit", &tree, &[])
                .unwrap();
        }
        dir
    }

    #[test]
    fn initialize_returns_protocol_version() {
        let repo_path = PathBuf::from("/does/not/matter");
        let msg = json!({ "jsonrpc": "2.0", "id": 1, "method": "initialize", "params": {} });
        let resp = handle(&msg, &repo_path).unwrap();
        assert_eq!(resp["result"]["protocolVersion"], PROTOCOL_VERSION);
        assert_eq!(resp["result"]["serverInfo"]["name"], SERVER_NAME);
    }

    #[test]
    fn notifications_return_none() {
        let msg = json!({ "jsonrpc": "2.0", "method": "notifications/initialized" });
        assert!(handle(&msg, Path::new("/x")).is_none());
    }

    #[test]
    fn tools_list_exposes_read_only_tools() {
        let msg = json!({ "jsonrpc": "2.0", "id": 2, "method": "tools/list" });
        let resp = handle(&msg, Path::new("/x")).unwrap();
        let tools = resp["result"]["tools"].as_array().unwrap();
        assert_eq!(tools.len(), 6);
        for t in tools {
            let n = t["name"].as_str().unwrap();
            assert!(!n.contains("write") && !n.contains("reset") && !n.contains("push"));
        }
    }

    #[test]
    fn commit_history_and_read_file_work() {
        let dir = test_repo();
        let msg = json!({ "jsonrpc": "2.0", "id": 3, "method": "tools/call",
            "params": { "name": "commit_history", "arguments": { "max": 10 } } });
        let resp = handle(&msg, &dir).unwrap();
        let text = resp["result"]["content"][0]["text"].as_str().unwrap();
        assert!(
            text.contains("init commit"),
            "history should include the commit: {text}"
        );

        let msg = json!({ "jsonrpc": "2.0", "id": 4, "method": "tools/call",
            "params": { "name": "read_file", "arguments": { "path": "a.txt" } } });
        let resp = handle(&msg, &dir).unwrap();
        assert_eq!(resp["result"]["content"][0]["text"], "hello\n");

        // Escape guard: `..` and absolute paths are rejected.
        let msg = json!({ "jsonrpc": "2.0", "id": 6, "method": "tools/call",
            "params": { "name": "read_file", "arguments": { "path": ".git/config" } } });
        let resp = handle(&msg, &dir).unwrap();
        assert_eq!(
            resp["result"]["isError"], true,
            "git internals must be refused"
        );

        // Symlink escape (unix only — creating symlinks on Windows needs
        // privileges; the canonicalize guard is platform-independent).
        #[cfg(unix)]
        {
            std::os::unix::fs::symlink("/etc/hostname", dir.join("evil-link")).unwrap();
            let msg = json!({ "jsonrpc": "2.0", "id": 7, "method": "tools/call",
                "params": { "name": "read_file", "arguments": { "path": "evil-link" } } });
            let resp = handle(&msg, &dir).unwrap();
            assert_eq!(
                resp["result"]["isError"], true,
                "symlink escape must be refused"
            );
        }

        let msg = json!({ "jsonrpc": "2.0", "id": 5, "method": "tools/call",
            "params": { "name": "read_file", "arguments": { "path": "../secret" } } });
        let resp = handle(&msg, &dir).unwrap();
        assert_eq!(resp["result"]["isError"], true);
        std::fs::remove_dir_all(&dir).ok();
    }
}
