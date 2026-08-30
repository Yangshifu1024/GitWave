---
name: gitwave-release
description: GitWave 版本升级与发布同步流程。每当用户要升级/bump 版本号、发新版、或把新版本同步到 README 和 site/ 官网时使用——包括「升级版本到 x.y.z」「更新 README 和 site」「准备发版」这类说法；即使用户只提了其中一部分（如只改版本号），也应主动检查其余同步点，因为版本号分散多处且不同步会直接导致发版失败。
---

# GitWave 版本升级与发布同步

将 GitWave 升级到新版本，并保持所有用户可见面一致。

版本号硬编码在 4 处，同步不是洁癖而是**发版链路的硬约束**：CI 断言 `tag === "v" + package.json 的 version`（prepare job 门禁），updater 清单 latest.json 由 tauri-action 在构建时生成上传。README 的 "Cutting a release" 小节是发版清单的 source of truth，动手前可对照。

## 步骤

### 1. 确认目标版本与本次变更内容

- 从用户指令或上下文确定新版本号（语义化版本）。
- 用 `git log <上个 tag>..HEAD --oneline` 浏览本版本包含的变更，判断 README Features / site 卡片是否需要体现新功能——不是每次发版都要改功能文案，但用户可感知的新能力应该出现。

### 2. 升级版本号（4 处，缺一不可）

| 文件 | 位置 | 说明 |
|---|---|---|
| `package.json` | `"version"` | 改完执行 `npm i` 同步 package-lock.json（CI 用 `npm ci`，锁文件与 package.json 不一致会直接失败） |
| `src-tauri/tauri.conf.json` | `"version"` | Tauri 打包版本 |
| `src-tauri/Cargo.toml` | `version = "…"` | Rust crate 版本 |
| `src-tauri/Cargo.lock` | `name = "gitwave"` 条目 | **不要手改**，改完 Cargo.toml 后跑 `cargo check --manifest-path src-tauri/Cargo.toml` 自动同步 |

### 3. 更新 README.md

- 文件头部 Status 行：`**Status:** vX.Y.Z — …`，把本版本值得用户感知的能力融进这句话（例如 v0.5.0 加了 "in-app auto-updates served from GitHub Releases"）。
- Features 列表：新功能加独立 bullet；已有能力不重复罗列。
- 下载小节如提及版本号/平台能力，一并核对。

### 4. 更新 site/index.html（官网，push main 后自动部署 GitHub Pages）

- hero 徽标：`<div class="badge">vX.Y.Z · <发版当日日期></div>`——内容为**当前版本号 + 当前年月日**（不再写平台签名文案）。日期格式：en 页用 `YYYY-MM-DD`（如 `2026-08-30`），zh-CN 页用 `YYYY年M月D日`（如 `2026年8月30日`）。两页都要改。
- 下载区：`Latest release: vX.Y.Z`（zh 页「最新版本：vX.Y.Z」）
- 功能卡片：新功能视体量补进对应卡片文案（如小能力可追加进 "Batteries included" 清单，避免为单点功能新开卡片破坏 6 卡网格节奏）。

### 5. 验证

```bash
# 旧版本号残留检查（依赖包自身恰好的版本号不算，重点看 gitwave 自己的）
grep -rn "<旧版本>" README.md site/ package.json package-lock.json src-tauri/tauri.conf.json src-tauri/Cargo.toml
grep -A 1 'name = "gitwave"' src-tauri/Cargo.lock   # 确认已同步为新版本
cargo check --manifest-path src-tauri/Cargo.toml    # 锁文件同步 + 编译无恙
```

## 约束（来自 AGENTS.md，不可越过）

- **改完即止**：禁止 commit / push / tag，这些由用户执行；提醒用户文件留在当前分支未提交即可。
- 发版链路提醒（用户操作）：commit → 合入 main → `git tag -a vX.Y.Z && git push origin main vX.Y.Z` → CI 三平台构建出草稿 release（tauri-action 构建时生成并上传 latest.json）→ **publish 前核对草稿**：assets 齐全、latest.json 三平台键（darwin-aarch64 / linux-x86_64 / windows-x86_64）存在且签名非空 → 手动 publish（草稿转正后 latest.json 对 updater 生效，装了 updater 的老客户端即开始收到更新）。
- **tag 打点时机（硬约束）**：tag 快照的是打 tag 那一刻 HEAD 的已提交状态，不含未提交改动；必须在 bump commit 合入 main、成为 HEAD 之后再打。push 新 commit 不会移动已有 tag；打错指向时只能删 tag 重打重推（`git tag -d vX.Y.Z` + `git push origin :refs/tags/vX.Y.Z`），CI 的 prepare-release 会拦截 tag 名与 package.json 版本不一致。
