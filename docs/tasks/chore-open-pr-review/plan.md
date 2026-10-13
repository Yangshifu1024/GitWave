# chore-open-pr-review · 清理未合并 PR

## 范围与授权

2026-10-13，用户要求处理 GitWave 所有未合并 PR，并明确确认「审查、修复并合并通过的 PR」。本轮快照有 12 个 PR：#150–#160 与 #162，均以 main 为目标。只处理这些 PR 与其合并冲突，不升应用版本、不创建 release、不修改 tag。

## 处理方案

1. 读取每个 PR 的完整 diff、精确 head、状态和 CI；按 [.agents/agents/code-reviewer.md](../../../.agents/agents/code-reviewer.md) 分运行时、工具链和 Tauri 三组进行七维审查。
2. 在隔离 worktree 组合全部改动。#153 与 #159 的 pnpm peer snapshots 存在文本冲突：保留 typescript-eslint 8.71.1 与 ESLint 10.12.0，统一全部 ESLint peer 引用。
3. 用 `pnpm install --frozen-lockfile`、`make check` 与 `pnpm build` 验证组合。保存通过检查的文件散列，最终合并前逐文件比对，避免遗漏或意外回退其它升级。
4. 补上每个 Dependabot PR 缺少的任务计划链接；保留原有 release notes 与说明。
5. 三平台原 PR 的 CI 全绿且 head 未变化后，依次使用 merge commit 合并其它 11 个 PR。合并期间 Dependabot 自动更新 #153；新 lockfile 同时改动了 7 个原本不属于 TypeScript ESLint 升级的 React Aria 引用。将该 PR 的 lockfile 恢复到已测试且独立审查通过的组合快照，追加本轮报告，不改写 PR 历史；最新 head 的 CI 全绿后再使用 merge commit 合入 main。整个处理不 force push，不 squash。
6. 查询 GitHub 逐个确认 MERGED，再核对开放 PR 清单和最终 main 的 CI。

## 验证结果

- 全部 12 个原 PR 的 12 项三平台 CI 检查均通过。
- 组合 frozen-lockfile 安装通过，lockfile 无重写。
- 组合 `make check` 通过：格式、npm/Rust Tauri 版本兼容、ESLint、clippy、TypeScript、cargo check；Node 4 tests、Vitest 53 files / 337 tests、Rust 385 tests 与 macOS 真实 bundle restart 1 test 通过，2 个既有 Rust tests ignored。
- 组合 `pnpm build` 通过，无构建 warning。
- 三组 code-reviewer 审查没有阻断性代码问题；精确 PR heads 和证据见 [review.md](./review.md)。

## 验证边界

实际本地验证环境为 macOS，三平台证据来自各 PR 的远程 CI。Playwright 尚无 E2E 套件，本任务没有实测 submenu / Select 的完整浏览器交互，也没有实测签名 GitWave 包的下载 → 安装 → restart 全链路；不要把依赖审查或测试通过表述为这些端到端场景已完成。新 main 的 CI 在合并后单独核对。
