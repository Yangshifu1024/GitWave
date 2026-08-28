# feat: 左侧栏视觉层级调整

状态：已实现第一版

## 需求来源

用户 2026-08-28：L1 分区标题（Workspace/Repos/Branches…）不明显；各级字体无区分。分支：`feature/theme-design`（沿用）。

## 三级阶梯（方案确认后落地）

| 层级 | 元素 | 样式 |
|---|---|---|
| L1 分区标题 | SidebarSection 头部（Workspaces/Repos/Branches/Stash/Tags/Remotes…） | `text-[11px]` semibold uppercase **tracking-wider**，色 muted → **text-secondary**，chevron 14→12 |
| L2 条目主名 | workspace / 仓库 / 分支名 | 统一 **text-sm sans text-primary**（选中 font-medium） |
| L3 辅助信息 | sha / 路径 / upstream / 计数 | text-xs mono muted（不动） |

分组头（Branches 内 Local/Remote）= 次级 label：保持 text-xs muted uppercase，仅对齐 SidebarSection 的 px-3 py-1.5。

## 改动清单

- `src/components/ui/SidebarSection.tsx`：L1 头部样式（全部一级分区统一生效）
- `src/components/BranchList.tsx`：Local/Remote 分组头 padding 对齐
- `src/components/RepoList.tsx`：仓库名 `font-mono text-xs` → `text-sm` sans，统一 primary 色

## 修订 2（2026-08-28，截图反馈"还是不够清晰"）

第一版问题：L1（11px secondary）与 L2 分组头（12px muted）几乎同大同灰，且 L2 反而更大。第二版拉开三档：

- **L1 分区标题**：色改 **text-primary（近黑加粗小标签）**，成为层级锚点；hover 只保留底色反馈
- **L2 分组头（LOCAL/ORIGIN）**：12px → **10px**、py-1、**缩进 pl-7**、chevron 12→10——明显小于 L1 且有嵌套感
- 内容行不动（14px regular primary 与 L1 靠字号/字重区分）

三档最终形态：L1 = 11px bold 近黑 caps；条目 = 14px regular；L2 = 10px muted 缩进 caps。

## 修订 3（2026-08-28，用户指定字号规格）

用户给定四级字号：L1 16 / L2 14 / L3 12 / 内容 12。

- L1 分区标题：`text-base`（16px）bold caps primary，chevron 12→14
- L2 分组头（LOCAL/ORIGIN）：`text-sm`（14px）muted，chevron 10→12，保留缩进
- 内容条目：`text-xs`（12px）——`ListItem` 根 `text-sm`→`text-xs`（workspace 行等生效）、BranchList 分支名、RepoList 仓库名同步
- L3 辅助信息：已为 text-xs（12px），不动
- 注意：`ListItem` 为共享组件，根字号收紧影响所有列表行（SshKeyManager 行内已有显式字号，不受影响）

## 修订 4（2026-08-28，按 HIG / M3 调研定稿）

16px caps 标题违反 macOS/M3"分组标题小于内容"的通则，按调研改为窄字号带方案（11/13/11/11）：

- L1 分区标题：**11px** semibold 大写 tracking-wider **text-muted**（macOS source-list 标题惯例），chevron 12
- 内容条目：**13px** regular primary（macOS Body 13）——`ListItem` 根、RepoList、BranchList 分支名同步
- L2 分组头（LOCAL/ORIGIN）：**11px** semibold muted + 缩进 pl-6（与 L1 同款，靠缩进表从属）
- L3 辅助信息：**11px** mono muted（自 text-xs 12px 下调）

层级表达 = 大小写/颜色/缩进/字重，不再依赖大字号。依据：Apple HIG Typography（macOS Body 13 / Subheadline 11）、M3 Type scale（Label 14/12/11，导航组标题无大字号角色）。

## 验证

- typecheck / prettier / eslint / vitest / build
- 真机：侧栏三级层级肉眼可辨；Branches 分组头与分区头同侧距
