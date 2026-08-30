# feat-tauri-action-release-migration

> 发布流水线从「手动 tauri build + 自研签名/TSA 代码」整体迁移到 tauri-apps/tauri-action + releaseId 直传，对齐 clash-verge-rev 的成熟模式。

## 背景

v0.3.0–v0.5.0 发版期间连环踩坑：手动 keychain 导入、TSA 限流（apple 黑洞 → ts.ssl.com token 被 notary 拒收 → sectigo 拒答事务）、.p8 PEM 损坏与相对路径。调研 clash-verge-rev（同 Tauri 2、持续发货）结论：仓库零签名定制，6 个 APPLE_* secrets 透传给 tauri-action 即可，默认 Apple TSA 常态可用。

## 新流水线

| Job | 职责 |
|---|---|
| `prepare-release` | 校验 tag == package.json version（门禁前移，贵构建前早失败）→ 创建 draft release → 输出 release_id |
| `build-macos/linux/windows` | tauri-action@`action-v1.0.0`（pin，不 floating）构建并直传 release；macOS 透传 APPLE_* 六件套，`APPLE_API_KEY_PATH` 用绝对路径；job 级 `timeout-minutes: 45` 兜底 |
| `publish` | 从 draft 下载 `*.sig` → 生成 latest.json（原脚本逻辑保留：每平台恰好一个 .sig 的严格校验）→ upload → **停在这里**：release 保持 draft，人工检查附件/latest.json/release notes 后手动点 Publish（不自动转正，符合最初「草稿」需求） |

## 决策记录

- **不用 `includeUpdaterJson`**：F009 自研 manifest 的校验（tag 门禁、单 .sig 断言）比上游默认严格，保留自研；tauri-action 只负责构建和上传。
- **公证凭证留 API key 三件套**：v0.3.0 已端到端验证，零新配置；换 APPLE_ID 三件套是后续可选简化（改 3 行 env + 3 个新 secret）。
- **删除自研 TSA wrapper 与 smoke test**：回到 bundler 默认 Apple TSA。今天（2026-08-30）的限流是间歇故障而非常态；若复发，wrapper 方案可随时回归（bundler 按命令名调 codesign，PATH wrapper 可拦截）。
- **删除 keychain 手动导入 / 预热编译 / upload-download-artifact 中转**：全部由 tauri-action 内部化或不再需要。

## 运维注意

- 重跑前若存在同名旧 draft（带陈旧附件），先手动删除 draft——tauri-action 上传不清附件。
- macOS 首跑失败优先看 tauri-action 日志的 codesign/notarytool 段落（args 已带 --verbose）。
