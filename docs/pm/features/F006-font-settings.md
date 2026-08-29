# F006 · 字体设置（UI 字体 + Mono 字体）

## 背景

GitWave 的界面字体（平台原生 sans）与代码字体（Roboto Mono 兜底）目前写死在主题 token 中，用户无法更换。使用辅助字体阅读中文、偏好特定等宽字体（JetBrains Mono / Fira Code 等）看 diff 的用户，只能改系统级字体替换，成本高且影响面超出 GitWave。同类开发者工具（VS Code、终端模拟器）均将字体作为基础外观设置。

## 提议方案

在 Settings → Appearance 中增加 Fonts 设置项，允许用户分别配置 UI 字体与 Mono 字体：

- 两项均为自由文本输入（字体族名，支持逗号分隔多个），带实时预览样张（中英文 + 数字），可一键重置为默认
- 点击 Save 后立即生效并持久化：正文界面随 UI 字体变化，diff / blame / 提交图 / 冲突编辑器等场景随 Mono 字体变化，无需重启
- 输入的字体名作为字体链首选，保留既有平台回退链（含 CJK 回退）；输入未安装的字体时静默回退默认链
- 设置为应用级偏好，重启后保持
- 不做：系统字体枚举下拉、字号设置、按 workspace 级字体

## 影响

- 涉及模块：`src/styles/tokens.css`、`src/lib/fonts.ts`（新）、`src/hooks/useFonts.ts`（新）、`src/main.tsx`、`src/components/SettingsModal.tsx`；无 Rust 侧改动
- 影响版本：v0.2.x
- 是否破坏向后兼容：否（无设置时行为与现状完全一致）

## 决策

- 状态：接受
- 决策人：用户（直接提出该需求；交互方式选自由文本输入 + 显式 Save，分支选独立 feature 分支）
- 决策日期：2026-08-29
- 关联决策：F005-repo-tab-drag-reorder（同批 v0.2.x 外观/交互改进）
