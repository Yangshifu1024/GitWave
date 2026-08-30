# GitWave

> [English](./README.md) | 简体中文

> 本地优先、AI 协作的 Git 客户端。官网：**[gitwave.work](https://gitwave.work)** · 产品范围见 `docs/pm/core/01-features.md`，工程决策见 `docs/tech/`。

**状态：** v0.6.0 —— 三平台构建（macOS / Windows / Linux）由 tag 触发的 CI 产出，macOS 构建已签名公证，应用内自动更新由 GitHub Releases 提供，界面完整支持中文 / English 双语，AI 回复语言可选。

## 下载

macOS（Apple silicon，已签名公证）、Windows（NSIS）与 Linux（deb / rpm / AppImage）安装包见 <https://gitwave.work> 与 [GitHub Releases](https://github.com/Yangshifu1024/GitWave/releases/latest) 页面。

## 功能

- **Workspace 管理** —— 多工作区、仓库 Tab 拖动排序、按工作区隔离的 AI 上下文；Workspace 是抽象概念，不依赖任何目录
- **工作副本** —— 暂存 / 取消暂存、丢弃、忽略、带 conventional commit 类型徽标的提交、commit message AI 辅助
- **分支与同步** —— 新建 / 切换 / 删除 / 重命名、带确认的 push / pull、同步状态区、merge（ff 与 no-ff）及冲突面板
- **历史** —— 提交图（Fork 风格连线）、commit 详情、blame、reflog、tag
- **Diff 查看器** —— 分栏与统一视图、Shiki 语法高亮、按 hunk 操作
- **高级 Git** —— stash、交互式 rebase、worktree、子模块、LFS、远程管理、.gitignore 编辑器、Git hooks 面板、仓库健康检查
- **AI 协作** —— BYOK 供应商配置、commit 解释、AI 起草 PR 描述；除非你主动发送给所选供应商，diff 不离开本机
- **自动更新** —— 应用内检查更新、签名下载、一键安装（macOS / Windows / AppImage）；deb / rpm 安装提示前往 releases 页面更新
- **SSH 密钥管理** —— 生成 / 导入密钥、按仓库配置 SSH
- **平台体验** —— 命令面板、菜单栏应用模式、主题与字体设置、界面中英双语、AI 回复语言（中 / 日 / 韩 / 英）

## 技术栈

- **前端：** React 19 + TypeScript + Vite 7、Tailwind CSS 4 + HeroUI v3、zustand、TanStack Query / Virtual
- **后端：** Rust + [Tauri 2](https://tauri.app)、清洁分层（`domain` / `application` / `infrastructure`）、`git2`（静态链接 libgit2 + libssh2 + OpenSSL）—— 不依赖系统 Git
- **测试：** Vitest（单元）、Playwright（e2e）

## 快速开始

前置要求：

- Rust stable（[rustup](https://rustup.rs)）
- Node.js ≥ 20
- macOS：Xcode command line tools（`xcode-select --install`）
- Linux：`webkit2gtk-4.1-dev`、`build-essential`、`cmake`、`curl`、`wget`、`file`、`libssl-dev`、`libxdo-dev`、`libayatana-appindicator3-dev`、`librsvg2-dev`、`patchelf`
- Windows：WebView2 runtime + MSVC build tools

```bash
npm install
npm run tauri dev
```

## 脚本

| 命令 | 作用 |
|---|---|
| `npm run dev` | Vite 开发服务器（仅前端，无 IPC） |
| `npm run build` | TypeScript 检查 + Vite 生产构建 |
| `npm run tauri dev` | Tauri 开发模式（前端 + Rust 核心） |
| `npm run tauri build` | Tauri 生产构建（.dmg / .exe / .deb / .rpm / .AppImage） |
| `npm run lint` | ESLint（`lint:fix` 自动修复） |
| `npm run format:check` | Prettier 检查（不写入） |
| `npm run format` | Prettier 写入 |
| `npm run typecheck` | TypeScript 检查 |
| `npm test` | Vitest（单元） |
| `npm run test:e2e` | Playwright e2e 测试 |

Rust 命令（在 `src-tauri/` 内执行）：

| 命令 | 作用 |
|---|---|
| `cargo check --all-targets` | 类型检查 |
| `cargo clippy --all-targets -- -D warnings` | 严格 lint |
| `cargo test --all-targets` | 运行全部测试 |
| `cargo fmt` | 格式化 Rust 源码 |

## CI

工作流位于 `.github/workflows/`：

- **lint / test** —— 每次 push 与 PR 触发：`rust-lint` + `frontend-lint`、`rust-test` + `frontend-test`，均为 macOS / Ubuntu / Windows 矩阵
- **build** —— tag 推送时触发（`v*` 或任意 tag）：构建 macOS（aarch64）、Linux（deb / rpm / AppImage）与 Windows（NSIS）；三者全绿后创建**草稿 GitHub release**，附全部产物与自动生成的 release notes。macOS 构建经仓库 secrets（`APPLE_*`、`KEYCHAIN_PASSWORD`）签名公证，OpenSSL/libgit2 静态链接，二进制自包含

### 发版流程

1. 同步四处版本号：`package.json`、`src-tauri/tauri.conf.json`、`src-tauri/Cargo.toml`、`src-tauri/Cargo.lock`（`gitwave` 条目）
2. 提交后打 tag 并推送：
   ```bash
   git tag -a v0.x.0 -m "v0.x.0"
   git push origin main v0.x.0
   ```
3. CI 全绿后在 Releases 找到草稿，审阅 notes 并发布
   - 发布草稿同时会发布 `latest.json` —— 应用内更新器轮询的清单文件；存量安装会从这里获取新版本
   - 更新器产物（`.app.tar.gz` / `.sig` / `-setup.exe.sig` / `latest.json`）由 CI 产出；本地 `tauri build` 现在需要导出 `TAURI_SIGNING_PRIVATE_KEY`（如密钥加密还需 `TAURI_SIGNING_PRIVATE_KEY_PASSWORD`）：`export TAURI_SIGNING_PRIVATE_KEY=$(cat ~/.tauri/gitwave.key)`

按 AGENTS.md 约定，**AI 代理禁止自动 commit / push / merge** —— 每次进入 `main` 的变更由人工把关。

## 文档

- 官网 —— <https://gitwave.work>（落地页与下载）
- `docs/pm/core/` —— 产品管理（功能、范围、路线）
- `docs/tech/` —— 工程决策（架构、选型、ADR、约定）
- `docs/design/` —— UI/UX 总览（3-pane 布局、tokens、组件）
- `docs/tasks/` —— 按任务归档的计划与审查
- `AGENTS.md` —— 工作流规则与代理边界

## 许可证

[MIT](./LICENSE) © Yangzhenbiao
