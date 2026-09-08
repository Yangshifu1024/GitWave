# feat-topbar-status-area-resize

TopBar UI 调整：状态区域扩宽减高、移除应用标题、ActionBar 计数按钮防跳动。

## 需求

1. **状态区域尺寸**：`SyncStatusArea` 从 48px 高 × 288px 宽调整为 24px 高 × 576px 宽（高度减半、宽度加倍）。
2. **移除标题栏**：移除 TopBar 中居中显示 "GitWave vX.X" 的 `ToolbarAppTitle` 组件（保留菜单栏、拖拽区域等其余顶部栏功能）。
3. **计数按钮防跳动**：ActionBar 右侧 Changes / Pull / Push 按钮当前"有数字才显示 `(N)`"，切换仓库时按钮宽度突变导致界面跳动。改为始终渲染固定宽度计数槽位：有数字（> 0）时显示 `(N)`，无数字 / 无仓库时槽位留空占位；宽度按 3 位数预留（`min-w-9` + `tabular-nums`），1~3 位数字切换按钮总宽度不变。

## 实施范围

| 文件 | 改动 |
| --- | --- |
| `src/components/SyncStatusArea.tsx` | `h-12 w-72`（288×48）→ `h-9 w-144`（576×36，宽度加倍；高度先减半至 24px、后应需求 +50% 至 36px）；文本 `line-clamp-2` → `truncate`；取消按钮 `h-6 w-6` → `h-5 w-5`（图标 3.5 → 3）；底部进度条 / 状态色条 `h-1`（4px）→ `h-0.5`（2px） |
| `src/components/Toolbar.tsx` | 移除 `ToolbarAppTitle` 的 import 与渲染 |
| `src/components/ToolbarAppTitle.tsx` | 删除文件（仅 Toolbar.tsx 引用） |
| `src/components/ActionBar.tsx` | `ActionBarButton` 新增 `count?: number \| null` prop，渲染固定宽度 `(N)` 槽位；Changes / Pull / Push 始终传 count；Fetch / Stash 不传 |
| `src/i18n/locales/{en,zh-CN}/changes.json` | 删除 `changes.actionBar.changesWithCount` |
| `src/i18n/locales/{en,zh-CN}/commits.json` | 删除 `commits.sync.pullWithCount`、`commits.sync.pushWithCount` |
| `src/stores/statusAreaStore.ts` | 操作结果状态新增 15 秒空闲自动重置（`STATUS_TTL_MS`）：每次 `setStatus` 重启计时器，到期清空回落到空闲态（分支名 + 灰色底条）；`clearStatus` 取消计时器 |
| `src/components/CommandPalette.tsx` | 面板背景从不透明的反例 `bg-bg-overlay`（半透明 scrim 色，内容透出）改为浮层标准表面 `bg-bg-elevated` + `border-border-default` + `shadow-modal`；遮罩对齐 Modal（`bg-bg-overlay backdrop-blur-sm`）；移除与 Input 前缀重复的放大镜图标；输入框改为头部行内透明无边框；意图确认卡改为内嵌圆角卡片；结果行圆角高亮、快捷键提示键帽化；无匹配时显示 Ask AI 提示而非空白 |

## 验证

- `pnpm lint`、`pnpm typecheck`、`pnpm test`（含 locale parity、statusAreaHitTesting 测试）
- 人工确认：切换仓库时右侧按钮组不发生横向位移
