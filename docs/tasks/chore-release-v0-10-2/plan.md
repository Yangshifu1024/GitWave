# chore-release-v0-10-2 · 发布重启修复与依赖升级

## 范围

用户要求发版，并明确选择补丁版本 **v0.10.2**。发布基线为 v0.10.1（`89bf325`）；待发布 main 为 `eaed9f2`，包含 #150–#160 与 #162 的 12 个 merge commit。

- macOS 更新安装后 restart 退出却未重新打开的问题：Tauri 2.12.2 官方修复，失败提示、重试与重复点击保护。
- 11 项已合并依赖升级：Rust updater、npm Tauri API / dialog、React Aria Components / React Query / react-i18next、ESLint / typescript-eslint / jsdom / Playwright / Vite React 插件。
- 行为与组合验证见 [重启任务](../fix-updater-restart/plan.md) 和 [PR 审查](../chore-open-pr-review/review.md)。

## 执行

1. 确认 main 干净、已同步远程；发布基线 v0.10.1 的本地 / 远程 tag 一致，v0.10.2 尚未占用。历史 v0.9.3 tag 的 fetch 存在本地 / 远程差异，不覆写历史 tag。
2. 修改文件前运行完整 `make check`，通过后用 `pnpm bump 0.10.2` 只更新四处版本，确认 Cargo.lock 仅改 root crate 版本。
3. 同步 README、官网的版本 / 日期 / 更新说明与既有功能卡片，以及 PM 核心文档；保留历史发布记录。图标没有变化，不重新生成或复制资源。
4. 格式与完整质量门禁通过后，由 code-reviewer 审查版本与文档同步，结果记录在 `review.md`。
5. 本地 commit：`chore: bump version to v0.10.2`。依照 [release 技能](../../../.agents/skills/gitwave-release/SKILL.md)，具体 main + tag 推送范围需用户确认；tag 必须指向该发布 commit。
6. 推送后构建三平台 draft release。检查 macOS dmg / app.tar.gz、Windows NSIS、Linux deb / rpm / AppImage 和 updater signatures；latest.json 版本正确，含三平台 keys，签名非空，URL 均为本 tag 下真实资产的普通下载链接。
7. draft 发布会使已有安装从 updater 收到此版本。按 release 技能，最终发布由用户手动确认；未发布前不把 draft 描述为已向用户分发。

## 验证边界

发版前 main 和版本 / 文档更改后的 `make check` 均已通过（337 前端、385 Rust + 1 个真实 macOS bundle restart 测试，2 个既有 ignored 测试）。Cargo.lock 仅改 gitwave 的版本，pnpm lockfile 未变；官网 HTML 格式与任务计数已核对。三平台正式安装包及签名 / 公证证据以本 tag 的 release CI 与资产为准；独立临时 bundle 不能替代签名包完整下载 → 安装 → restart 验收。旧客户端第一次升级到修复版时，其 restart 仍由旧进程执行，可能需手动打开一次。
