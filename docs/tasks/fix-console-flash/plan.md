# fix: push 按钮按下后弹出命令行窗口（Windows）

状态：已实现

## 事故（2026-08-28，用户真机）

Windows 上点击 Push 按钮后,应用界面外闪出一个命令行(cmd/conhost)窗口。

## 根因

push 的调用链是:前端 SyncButtons → Tauri `cmd_push` → git2(libgit2, vendored)推送,推送本身
不启动外部进程。但远端为 HTTPS 时,git2 的凭据回调 `GitCredentialHelper` 会 spawn
`git credential fill` 向系统凭据管理器取账号密码(`credentials.rs::query_helper`)。

Windows 上,GUI(无控制台)进程用 `std::process::Command` 启动控制台程序时如果不设置
`CREATE_NO_WINDOW`(0x08000000)标志,子进程会分配一个新控制台窗口并前置到前台——即用户看到的
命令行窗口。

同隐患存在于 SSH 功能的 `ssh-add` / `ssh -T` 调用(`ssh/keys.rs`),一并修复。

全仓库排查确认只有上述 3 处 `std::process::Command` spawn 点(push 本身用 vendored libgit2 不 spawn)。

## 修复

- 新增 `src-tauri/src/infrastructure/process.rs`:`hidden_command()` / `hide_console_window()`,
  Windows 下为 `Command` 设置 `creation_flags(CREATE_NO_WINDOW)`;非 Windows 为空操作
- `credentials.rs`:`git credential fill` 改用 `hidden_command`
- `ssh/keys.rs`:`ssh-add -l` / `ssh-add` / `ssh-add -d` / `ssh -T` 改用 `hidden_command`
- `infrastructure/mod.rs`:注册 `pub mod process;`

## 验证

- `cargo check` 通过
- `git credential fill`、`ssh-add`、`ssh` 均改为经 `hidden_command` 派生,界面不再闪现控制台
- 需重新构建(tauri dev / tauri build)后真机验证:HTTPS push 不再弹窗
