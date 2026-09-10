# Review · feat-open-external-tools（F014）

- 分支：`feature/external-open`（对比 `main`，含未跟踪 docs）
- 审查人：code-reviewer
- 日期：2026-09-10
- 范围：`src-tauri/src/lib.rs`、`src/lib/api.ts`、`src/components/WorkspaceRepoTabs.tsx`、`src/i18n/locales/{en,zh-CN}/workspace.json`、`docs/pm/features/F014-open-external-tools.md`、`docs/tasks/feat-open-external-tools/plan.md`

## 总体结论

功能实现思路清晰：文件管理器走 `app.opener().open_path`（与既有 `open_data_dir` 同路由）、终端走 `std::process::Command` 直接 spawn（无 shell 拼接，注入风险低）、前端禁用逻辑与错误展示复用现有 `actionError` / `ErrorAlert` 通道，i18n 中英对齐。存在 1 个必须修复的样式 token 问题（`bg-bg-hover` 不存在，hover 反馈完全失效）和若干建议项。

## 🔴 必须修复

### R1. `hover:bg-bg-hover` token 不存在，hover 背景静默失效

`WorkspaceRepoTabs.tsx` 两个按钮使用了 `hover:bg-bg-hover`，但 `src/styles/tokens.css`（Tailwind v4 `@theme`）中定义的 bg token 只有 `bg-primary / bg-secondary / bg-elevated / bg-panel / bg-overlay`，**没有 `--color-bg-hover`**。Tailwind v4 对未知 token 不生成规则，该类是无效类，按钮 hover 时没有任何背景反馈。全仓库搜索确认 `bg-bg-hover` 仅本文件出现。

代码库同类小图标按钮的既有约定（见 `src/components/ui/PathInput.tsx:77`）是：

```tsx
className="flex size-6 items-center justify-center rounded text-text-muted transition-colors hover:bg-bg-secondary hover:text-text-primary disabled:cursor-default disabled:opacity-50"
```

建议改为 `hover:bg-bg-secondary`，同时将 `disabled:opacity-40` 对齐为既有的 `disabled:opacity-50`（可选）。

## 🟡 建议修复

### R2. Windows 无 wt.exe 时缺少 cmd.exe 回退，与注释 / PM 文档不符

`detect_terminal_command()` 在 Windows 分支无条件返回 `wt.exe`；`open_in_terminal` 中的注释写 "cmd.exe is the fallback console"、`creation_flags` 也区分了非 wt.exe 的情况，但代码路径里 cmd.exe 永远不会被选中。PM 文档 F014 明确承诺「不可用时回退 cmd.exe」。在未安装 Windows Terminal 的旧 Windows 上，用户会直接收到 "failed to launch wt.exe"。建议在 Windows 分支用 `find_in_path("wt.exe")`（或检查 `WindowsApps` 别名）探测，失败时回退 `cmd.exe` 并用 `CREATE_NEW_CONSOLE`。

注意：`wt.exe` 通常位于 `WindowsApps` 的执行别名（0 字节 reparse point），`Path::is_file()` 一般返回 true，但建议实测验证探测方式可靠。

### R3. macOS iTerm 检测只查 `/Applications`，忽略 `~/Applications`

`Path::new("/Applications/iTerm.app").exists()` 漏掉用户级安装（Homebrew cask 默认装到 `/Applications`，但用户手动安装常在 `~/Applications`）。建议两处都检查，或直接信任 `open -a iTerm` 的解析（`open` 自带搜索路径，失败再报错）。

### R4. macOS `open -a iTerm <dir>` 行为不确定

`open -a Terminal <dir>` 在现代 macOS 上会以该目录为 cwd 打开窗口，但 iTerm2 对「目录参数」的处理历史上不一致（可能忽略参数或打开默认 profile 的 home 目录）。建议对 iTerm 分支做一次真机验证；若不可靠，可改为 `open -a iTerm .`（配合 `current_dir`，代码已设置）——注意 `open` 的非绝对参数解析基于 cwd，目前实现依赖这一点，恰好可行，但值得在注释中写明。

### R5. `detect_terminal_command` 为 `pub(crate)` 级别的纯函数但无单元测试

仓库 Rust 侧已有测试先例（`src-tauri/src/domain/blame.rs`、`diff.rs`）。`find_in_path` 和 Linux 候选表逻辑可通过注入 `PATH` / 环境变量做单元测试（`detect_terminal_command` 直接读 env，可测试性尚可）。至少建议为 `find_in_path` 补测试；Linux 终端候选表则建议真机 / CI 矩阵验证（gnome-terminal `--working-directory`、konsole `--workdir`、wezterm `start --cwd` 的参数形式各不相同，属易错点）。

## 🟢 备注

- **注入风险**：路径始终以 `Command::arg` / argv 形式传递，不经过 shell，目录名含空格 / 特殊字符是安全的。`open_in_file_manager` 复用 tauri opener，与 `open_data_dir` 同一面。路径来源为应用自己登记的 repo 路径，且两个命令都先 `is_dir()` 校验，风险可控。
- **TOCTOU**：`is_dir()` 与后续 open/spawn 之间存在竞态，但对本地单用户场景可忽略；目录在校验后被删除会以错误形式反馈，可接受。
- **进程分离**：Windows 用 `creation_flags`、Unix 用 `process_group(0)`，退出 GitWave 不会连带杀掉终端，处理正确。`wt.exe` 用 `CREATE_NO_WINDOW` 避免中转控制台闪烁的细节处理得当。
- **前端禁用逻辑**：`externalRepo = activeRepo && activeRepo.status !== "missing"` 与 PM 文档「无活跃仓库或仓库缺失时禁用」一致；`renderedRepos.find(...)` 与现有 tab 渲染数据源一致。
- **错误展示**：`openExternal` 的 catch 走 `setActionError` → `ErrorAlert`，与 relink / remove 的既有路径一致，无新增 UI 债。
- **i18n parity**：en / zh-CN 两个 locale 键完全对齐（`openInFileManager` / `openInTerminal`），无缺漏。
- **api.ts**：两个 wrapper 与相邻 `openDataDir` 风格一致，含 JSDoc。
- **文档**：F014 提案与 plan.md 齐备，归属目录符合 AGENTS.md 约定（feature → `docs/pm/features/`，任务 → `docs/tasks/`）。唯一不一致是 R2 提到的 cmd.exe 回退承诺未兑现。

## 结论

修复 🔴 R1（样式 token）后可合入；建议同 PR 内一并处理 🟡 R2（文档承诺的行为缺口），R3–R5 可作为后续小项。

## 修复记录（2026-09-10）

- R1 ✅ `hover:bg-bg-hover` → `hover:bg-bg-secondary`（对齐 PathInput 既有约定）
- R2 ✅ Windows 增加 cmd.exe 回退：PATH 中无 `wt.exe` 时启动 `cmd.exe`（新控制台，cwd = 仓库）
- R3 ✅ macOS iTerm 检测同时检查 `/Applications` 与 `~/Applications`
- R4 ⏳ 需 macOS 真机验证 `open -a Terminal <dir>` 行为
- R5 ⏸ 未补充 Rust 侧单测，留待后续任务
