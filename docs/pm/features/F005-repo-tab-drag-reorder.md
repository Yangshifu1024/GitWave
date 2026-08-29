# F005 · Repository Tab 拖动排序

## 背景

Workspace 内多仓库以 tab 形式展示（`WorkspaceRepoTabs`），当前顺序固定为仓库的加入顺序（`repos.added_at`）。最常用的仓库不一定最先加入，用户无法把高频仓库排到前面；多仓库（5+）场景下，每次定位目标仓库都要从头扫一遍 tab 条。

## 提议方案

允许用户按住某个 Repository Tab 拖动，实时预览目标位置，松手后顺序立即生效并持久化。

- 拖动仅重排 tab 顺序：不改变 active repo 选中态，不移动仓库目录本身
- 顺序持久化到 workspace 存储（SQLite `repos.position`），重启后保持
- 提供非指针替代路径：tab 右键菜单 Move Left / Move Right
- 不做：跨 Workspace 拖动、拖出新窗口、拖动分屏等扩展

## 影响

- 涉及模块：`src-tauri`（persistence / use_cases / commands / migration）、`src/components/WorkspaceRepoTabs.tsx`、`src/components/ui/Tabs.tsx`、`src/lib/api.ts`
- 影响版本：v0.2.x
- 是否破坏向后兼容：否（新增 `position` 列按 `added_at` 回填，旧数据初始顺序与现状一致）

## 决策

- 状态：接受
- 决策人：用户（直接提出该需求）
- 决策日期：2026-08-29
- 关联决策：F002-repo-ingestion（`repos` 表结构）
