# GitWave · 主界面视觉重设计（v2）

> 状态：提案 · 2026-08-27  
> 校对稿：[`mockups/v2/index.html`](./mockups/v2/index.html)

## 1. 问题诊断

当前实现与 v1 校对稿均存在明显的「网页味道」，根因不在 3-pane IA，而在 **chrome 层叠与组件形态**：

| 症状 | 根因 | 表现 |
|---|---|---|
| 像 shadcn 后台 | `ListItem` 使用 `rounded-md` + hover 圆角块 | 侧栏像 Web 列表，不像 macOS Source List |
| 平面无层次 | Toolbar / Sidebar / WC Bar 同为 `bg-secondary` | 三栏+底栏糊成一片灰 |
| 控件漂浮感 | `Button` secondary 带边框；Sync 为三个独立 ghost 按钮 | 像网页表单，不像工具栏 Segmented Control |
| 焦点环网页化 | `ring-2 ring-accent ring-inset` | 与原生 2px Tide outline 不一致 |
| Graph 像表格 | commit 行选中为圆角色块 | 缺少「时间轴」连续感 |
| Inspector 像文档页 | 平铺标题 + 无边距 diff gutter | 不像 Fork / Tower 的代码审阅面板 |

**保留不变**：3-pane IA、Tide Lanes 签名色、Foam/Mist/Ink 色板、Working Copy Bar 行为。

## 2. 设计方向（三选一）

### A · Native Studio（推荐）

以 **macOS Source List + 工具栏 Segmented Control** 为骨架，GitWave 只在 Tide Lanes 与 accent 上发声。

- 侧栏：全宽行、无圆角、选中仅左侧 3px Tide 条 + 8% Tide 底
- 工具栏：Workspace 胶囊 + 路径条 + **Sync 三段控件**（Fetch | Pull | Push）
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

### 4.1 Source List（替代当前 ListItem）

```
┌──────────────────────────┐
│ REPOS                 +  │  ← 10px caps label, no border
│▌gitwave            ●    │  ← 3px left bar when selected
│ notes                   │  ← full-width row, h=28, radius=0
└──────────────────────────┘
```

- `border-radius: 0`；`height: 28px`；`padding: 0 12px 0 9px`
- Selected：`border-left: 3px solid Tide` + `background: tide/8%`
- Hover（未选中）：`background: ink/4%`（light）/ `white/4%`（dark）
- Trailing badge：11px pill，不用带边框的 secondary Button

### 4.2 Toolbar

```
[ Client Work ▾ ]  gitwave › main  ↑2 ↓1   │ Fetch │ Pull↓1 │ Push↑2 │   ⌘K  ☀
```

- Workspace：**胶囊**（`border-radius: 6px`，`bg: elevated`，无描边）
- Repo path：`gitwave › main` 用 `›` 分隔，secondary 色
- Sync：**Segmented control** — 三段共享外框 `1px hairline`，中间竖线分隔；disabled 段 `opacity: 0.35`
- 右侧：KeyHint 用 **嵌入式 kbd**（非浮动 badge）

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
| Sync | 三个独立文字按钮 | Segmented control |
| Pane 分隔 | `border-right` | inset shadow |
| Inspector | 简单 header | file chip + gutter diff |
| WC Bar | 平贴 | 轻微上阴影 |
| 签名 | Tide Lanes | Tide Lanes + **层叠材质** |

## 7. 交付物

| 文件 | 说明 |
|---|---|
| `mockups/v2/index.html` | 四屏可交互高保真稿（Light History / Dirty / Dark / Empty） |
| `mockups/v2/*.png` | 导出静帧（浏览器打开 HTML 后截图） |
| 本文档 | 设计 rationale + 实施对照表 |

## 8. 实施优先级（供后续 plan 引用）

1. **P0** — Source List 重写（`ListItem` radius=0、全宽选中）+ pane inset shadow
2. **P0** — Toolbar Sync segmented + Workspace 胶囊
3. **P1** — Inspector diff gutter + file header row
4. **P1** — WC Bar 上阴影 + 列标题规范
5. **P2** — 移除全局 `ring-offset` 样式，统一 `:focus-visible` 为 2px Tide outline

## 9. 关联

- `00-overview.md` — IA 与品牌基调
- `01-tokens.md` — 色板（v2 不新增色，只调整用法）
- `03-layout.md` — 尺寸与快捷键（不变）
- `mockups/v2/index.html` — 像素源
