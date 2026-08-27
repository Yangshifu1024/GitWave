# GitWave · 主界面视觉重设计（v2）

> 状态：提案 · 2026-08-27  
> 校对稿：[`mockups/v2/index.html`](./mockups/v2/index.html)

## 1. 问题诊断

当前实现与 v1 校对稿均存在明显的「网页味道」，根因不在 3-pane IA，而在 **chrome 层叠与组件形态**：

| 症状 | 根因 | 表现 |
|---|---|---|
| 像 shadcn 后台 | `ListItem` 使用 `rounded-md` + hover 圆角块 | 侧栏像 Web 列表，不像 macOS Source List |
| 平面无层次 | Toolbar / Sidebar / WC Bar 同为 `bg-secondary` | 三栏+底栏糊成一片灰 |
| 控件漂浮感 | `Button` secondary 带边框；Sync 堆在 Toolbar 中央 | 操作与作用域错位；应贴近 REPOS / BRANCHES |
| 焦点环网页化 | `ring-2 ring-accent ring-inset` | 与原生 2px Tide outline 不一致 |
| Graph 像表格 | commit 行选中为圆角色块 | 缺少「时间轴」连续感 |
| Inspector 像文档页 | 平铺标题 + 无边距 diff gutter | 不像 Fork / Tower 的代码审阅面板 |

**保留不变**：3-pane IA、Tide Lanes 签名色、Foam/Mist/Ink 色板、Working Copy Bar 行为。

## 2. 设计方向（三选一）

### A · Native Studio（推荐）

以 **macOS Source List + 侧栏作用域操作** 为骨架，GitWave 只在 Tide Lanes 与 accent 上发声。

- 侧栏：全宽行、无圆角、选中仅左侧 3px Tide 条 + 8% Tide 底
- **Sync 按作用域下沉**：`Fetch` → REPOS 标题栏；`Pull` / `Push` → BRANCHES 标题栏
- **Workspace 列表**：侧栏最顶部 WORKSPACES section（REPOS 上方），Source List 一行一点击切换，不用 Toolbar 下拉
- 工具栏：**居中** `Workspace - Repository - branch` 上下文标题 + ⌘K / 主题 / 溢出菜单
- 材质：Sidebar Mist → Canvas Foam → Inspector Elevated（三层明度差）
- 分隔：pane 之间用 **1px inset shadow**，不用硬边框叠边框

**优点**：与 PM §1.10 macOS 优先一致；学习成本低。  
**代价**：Windows/Linux 需用同一套 token 模拟，不能依赖 NSVisualEffectView。

### B · Terminal Workshop

深色为主、等宽字体渗透 UI 标签、更高对比。  
**优点**：极客感强。 **缺点**：与「macOS 原生优先」冲突；浅色用户流失。

### C · Editorial Graph

报纸式细线网格 + 密集排版，history 像时间线版面。  
**优点**：辨识度高。 **缺点**：与 diff 阅读场景冲突；开发成本高。

**推荐 A**，B/C 可作为主题变体后续探索。

## 3. 材质与层次（Layer Stack）

```
z 高 ─────────────────────────────────────────
     │ Modal / Popover（shadow-modal）
     │ Working Copy Bar dirty（向上 shadow-subtle）
     │ Toolbar（Mist + 底 hairline）
     ├─ Inspector（Elevated，左 inset shadow）
     ├─ History Canvas（Foam，无框）
     └─ Sidebar（Mist，右 inset shadow）
z 低 ─────────────────────────────────────────
```

### 3.1 色阶（Light）

| 区域 | Token | 值 | 说明 |
|---|---|---|---|
| Sidebar | `bg-secondary` | `#E6EBEF` Mist | 比画布深半档 |
| Canvas | `bg-primary` | `#F4F6F8` Foam | history 主舞台 |
| Inspector | `bg-elevated` | `#EEF1F4` | 略抬升，承接 diff |
| Toolbar / WC | `bg-secondary` | Mist | 与侧栏同族，靠 shadow 区分 |

Dark 模式：Abyss 侧栏 / `#161B20` 画布 / `#1C2329` inspector，关系不变。

### 3.2 分隔语法

- **Pane 之间**：`box-shadow: inset -1px 0 0 var(--hair)`（sidebar 右侧、inspector 左侧）
- **Section 之间**：8px 透明间距 + 10px uppercase label（无横线）
- **禁止**：相邻区域各画一条 `border`，造成 2px 黑缝

## 4. 组件重设计要点

### 4.1 Source List + Section Sync（替代当前 ListItem / Toolbar Sync）

```
┌──────────────────────────┐
│ WORKSPACES             +  │  ← Workspace 列表，REPOS 上方
│▌Client Work            ●  │
│ Personal                  │
│ Side Projects             │
│ REPOS          Fetch  +  │
│▌gitwave                ●  │
│ notes                     │
│ BRANCHES   Pull↓1 Push↑2 +│
│▌main                HEAD   │
└──────────────────────────┘
```

**Workspaces section**

- 位置：侧栏 **最顶部**，在 REPOS 之前；与下方 sections 同样式（10px caps label + `+`）
- 交互：点击行 → `selectWorkspace`；选中行 Source List 高亮（3px Tide 左条）
- 行内 hover 显示次要操作（AI / Rename / Delete），与现有 `WorkspaceSwitcherDropdown variant="sidebar"` 一致
- **不放** Toolbar 下拉；Toolbar 不再显示 workspace 名

**列表行（Repos / Branches 等同理）**

- `border-radius: 0`；`height: 28px`；`padding: 0 12px 0 9px`
- Selected：`border-left: 3px solid Tide` + `background: tide/8%`
- Hover（未选中）：`background: ink/4%`（light）/ `white/4%`（dark）

**Section 标题栏操作**

| Section | 操作 | 作用域 | 禁用条件 |
|---|---|---|---|
| REPOS | `Fetch` | 当前 active repo 的全部 remote | 无 active repo |
| BRANCHES | `Pull ↓N` | 当前 HEAD branch vs upstream | behind = 0 或 detached |
| BRANCHES | `Push ↑N` | 当前 HEAD branch vs upstream | ahead = 0 或 detached |

- 按钮形态：10px 文字链 + 可选 `↓N` / `↑N` badge（ahead 绿 / behind 橙），与 `+` 并列于标题行右侧
- 无 active repo 时 REPOS Fetch 与 BRANCHES Pull/Push 均灰显
- 快捷键不变：`⌘⇧F` / `⌘⇧P` / `⌘⇧U`

### 4.2 Toolbar

```
                    Client Work - gitwave - main                    ⌘K  ☀
```

- **居中标题**：`{workspace} - {repository} - {branch}`，只读，反映当前上下文；切换在侧栏列表
- 格式：`-` 分隔，两侧空格；workspace 段 `font-weight: 500`
- 无 workspace / repo / branch 时显示占位（如 `Select workspace`），仍居中
- 左右：左侧 traffic lights（macOS）；右侧 KeyHint + 主题 + 溢出菜单
- **不放** Sync；不放品牌 Logo / 版本号

### 4.3 History Graph（Tide Lanes）

- 行高 28px；图区域固定 56px 宽
- Lane 线宽 1.75px；节点 r=3.5，描边 1.5px 画布色（防糊）
- 选中行：**整行** Tide/10% 底 + 左侧 2px Tide（与侧栏语法一致，略细）
- SHA：`11px IBM Plex Mono`，muted
- Message：`13px`，primary；选中 `font-weight: 500`
- Meta（作者·时间）：`11px`，muted，右对齐

### 4.4 Inspector

```
┌─ Commit header ─────────────────────┐
│ feat(graph): tide lane curves       │  13px semibold
│ a4f12c8 · yang · 2 hours ago        │  11px mono muted
├─ File: src/.../CommitGraph.tsx  +18 ─┤  file chip row
│  git diff gutter                    │
│   10 │ const ROW_H = 28;            │
│   11 │- old line                    │  del: coral/12%
│   11 │+ new line                    │  add: success/12%
└─────────────────────────────────────┘
```

- Header 与 diff 之间：`1px hairline`
- Diff gutter：36px 行号列，`bg: ink/3%`；hunk header 用 mono 10px
- 不做卡片圆角包裹整个 inspector

### 4.5 Working Copy Bar

- **Clean**：32px 单行；文字 muted；右侧 sync 摘要
- **Dirty**：顶栏 32px + 三列网格；列标题 10px caps
- 整体 `box-shadow: 0 -1px 0 hairline, 0 -4px 12px ink/4%` — 轻微「抽屉」感
- Commit 按钮：唯一 solid Tide CTA；Stage/Unstage 为 ghost

## 5. 字体与节奏

| 用途 | 字体 | 大小 |
|---|---|---|
| UI 正文 | SF Pro Text / system | 13px |
| Section label | system | 10px / 600 / tracking 0.08em |
| SHA / path / diff | IBM Plex Mono | 11–12px |
| Commit message | system | 13px (500 when selected) |

行高：列表/图 **28px 网格**；chrome 区域 **1.25**。

## 6. 与 v1 校对稿差异摘要

| 维度 | v1 | v2 |
|---|---|---|
| 列表选中 | 有时圆角块 | 全宽 Source List |
| Workspace 切换 | Toolbar 下拉 | **侧栏 WORKSPACES 列表**（REPOS 上方） |
| Sync 位置 | Toolbar 三按钮 | **REPOS: Fetch** / **BRANCHES: Pull·Push** |
| Pane 分隔 | `border-right` | inset shadow |
| Inspector | 简单 header | file chip + gutter diff |
| WC Bar | 平贴 | 轻微上阴影 |
| 签名 | Tide Lanes | Tide Lanes + **层叠材质** |

## 7. 交付物

| 文件 | 说明 |
|---|---|
| `mockups/v2/index.html` | 四屏可交互高保真稿（Light History / Dirty / Dark / Empty） |
| 本文档 | 设计 rationale + 实施对照表 |

审阅方式：浏览器打开 `mockups/v2/index.html`，不导出 PNG。

## 8. 实施优先级（供后续 plan 引用）

1. **P0** — Source List 重写（`ListItem` radius=0、全宽选中）+ pane inset shadow
2. **P0** — Workspace 列表置顶（WORKSPACES section）+ Sync 下沉至 REPOS/BRANCHES 标题栏
3. **P0** — Toolbar 居中 `Workspace - Repository - branch` 标题
4. **P1** — Inspector diff gutter + file header row
5. **P1** — WC Bar 上阴影 + 列标题规范
6. **P2** — 移除全局 `ring-offset` 样式，统一 `:focus-visible` 为 2px Tide outline

## 9. 关联

- `00-overview.md` — IA 与品牌基调
- `01-tokens.md` — 色板（v2 不新增色，只调整用法）
- `03-layout.md` — 尺寸与快捷键（不变）
- `06-color-palettes.md` — 五套配色方案（待选）
- `mockups/v2/index.html` — 像素源
