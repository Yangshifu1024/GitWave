# GitWave · Component Inventory

> Primitive + composite 组件清单。所有组件位于 `web/src/components/` 或 `web/src/components/ui/`。

## 1. Primitive（自建 + Radix 包壳）

### 1.1 Button

**目的**：触发一个动作。

**API**：

```tsx
<Button variant="primary" size="md" disabled={false} onClick={...}>
  Save
</Button>
```

**Props**：

| 字段 | 类型 | 默认 | 备注 |
|---|---|---|---|
| variant | "primary" \| "secondary" \| "danger" \| "ghost" | "secondary" | |
| size | "sm" \| "md" | "md" | |
| disabled | boolean | false | |
| onClick | () => void | | |
| type | "button" \| "submit" | "button" | |

**样式（Tailwind class 名）**：

```
primary   bg-accent text-text-inverse hover:bg-accent-hover
secondary bg-bg-elevated border border-border-default hover:bg-bg-secondary
danger    bg-bg-elevated border border-danger text-danger hover:bg-danger hover:text-text-inverse
ghost     bg-transparent hover:bg-bg-secondary text-text-primary
```

**a11y**：

- 焦点态：2px `outline` 使用 `accent` 颜色
- 禁用态：`opacity: 0.5` + `cursor: not-allowed`

**底层**：直接 `button` 元素 + Tailwind class。无 Radix 包装。

### 1.2 Input

**目的**：单行文本输入。

**API**：

```tsx
<Input value={name} onChange={setName} placeholder="..." error={...} />
```

**Props**：

| 字段 | 类型 | 默认 | 备注 |
|---|---|---|---|
| value | string | | controlled |
| onChange | (v: string) => void | | |
| placeholder | string | | |
| type | "text" \| "search" \| "password" | "text" | |
| error | string | null | 设置后红色边框 + 下方提示 |
| disabled | boolean | false | |
| autoFocus | boolean | false | |
| onKeyDown | (e) => void | | 用于 Enter 提交 |

**a11y**：原生 `input` 已提供；error 用 `aria-invalid` + `aria-describedby`。

### 1.3 Modal

**目的**：聚焦用户注意力到单个对话框。

**底层**：**Radix Dialog**（替代当前 HTMLDialogElement 直接使用），拿到完整 a11y（focus trap、Escape 关闭、aria-modal）。

**API**：

```tsx
<Modal open={isOpen} onOpenChange={setIsOpen} title="...">
  ... 内容
</Modal>
```

**Props**：

| 字段 | 类型 | 默认 | 备注 |
|---|---|---|---|
| open | boolean | | controlled |
| onOpenChange | (open: boolean) => void | | |
| title | string | | 顶部标题 |
| description | string | null | a11y 描述 |
| size | "sm" (320px) \| "md" (480px) \| "lg" (720px) | "md" | |
| destructive | boolean | false | 危险样式按钮 |

### 1.4 Tooltip

**目的**：hover / focus 时显示简短提示。

**底层**：**Radix Tooltip**。

**API**：

```tsx
<Tooltip content="View commit details">
  <Button>...</Button>
</Tooltip>
```

### 1.5 Toast

**目的**：异步操作结果反馈（"已推送"、"克隆失败"）。

**底层**：**Radix Toast** + 自建 hook `useToast()`。

**API**：

```ts
const { toast } = useToast();
toast({ title: "Pushed", description: "3 commits", variant: "success" });
toast({ title: "Clone failed", description: error.message, variant: "danger" });
```

**Props**：

| 字段 | 类型 | 默认 | 备注 |
|---|---|---|---|
| title | string | | |
| description | string | null | |
| variant | "success" \| "danger" \| "info" \| "warning" | "info" | |
| duration | number (ms) | 4000 | |

### 1.6 Tabs

**目的**：二级导航（History / Branches / Stash / ...）。

**底层**：**Radix Tabs**。

**API**：

```tsx
<Tabs value={tab} onValueChange={setTab}>
  <TabsList>
    <TabsTrigger value="history">History</TabsTrigger>
    <TabsTrigger value="branches">Branches</TabsTrigger>
  </TabsList>
  <TabsContent value="history">...</TabsContent>
  <TabsContent value="branches">...</TabsContent>
</Tabs>
```

### 1.7 Split / Pane

**目的**：3-pane 布局 + 可拖拽 resize。

**底层**：手写（参考 react-resizable-panels 思路，但更简单）。

**API**：

```tsx
<Split direction="horizontal">
  <Pane initialSize={240} minSize={180} maxSize={360}>...sidebar</Pane>
  <ResizeHandle />
  <Pane initialSize={280} minSize={200} maxSize={400}>...nav</Pane>
  <ResizeHandle />
  <Pane>...main</Pane>
</Split>
```

**Props**：

| 字段 | 类型 | 默认 | 备注 |
|---|---|---|---|
| direction | "horizontal" \| "vertical" | "horizontal" | |
| pane.initialSize | number (px) | | 初始尺寸 |
| pane.minSize | number | 100 | |
| pane.maxSize | number | 无限 | |

**键盘**：双击 ResizeHandle 重置；箭头键微调（v0.2）。

### 1.8 ListItem

**目的**：统一列表项结构（hover / selected / action slot / badge slot）。

**API**：

```tsx
<ListItem
  selected={active === ws.id}
  onClick={() => setActive(ws.id)}
  leading={<Icon />}
  trailing={
    <>
      <StatusBadge status="missing" />
      <Button size="sm" variant="ghost">edit</Button>
    </>
  }
>
  <span className="name">{ws.name}</span>
</ListItem>
```

**Props**：

| 字段 | 类型 | 默认 | 备注 |
|---|---|---|---|
| selected | boolean | false | 高亮态 |
| onClick | () => void | | |
| leading | ReactNode | null | 左侧 slot（图标 / avatar） |
| trailing | ReactNode | null | 右侧 slot（badges + actions） |

### 1.9 StatusBadge

**目的**：紧凑状态展示。

**API**：

```tsx
<StatusBadge variant="missing" />
<StatusBadge variant="active" />
<StatusBadge variant="conflict" />
<StatusBadge variant="ahead" suffix="↑3" />
<StatusBadge variant="behind" suffix="↓2" />
```

**Props**：

| 字段 | 类型 | 默认 | 备注 |
|---|---|---|---|
| variant | "active" \| "missing" \| "ahead" \| "behind" \| "conflict" | | |
| suffix | string | null | 显示在标签右侧的额外文本 |

**颜色**：见 `01-tokens.md` §1.1 status-* / branch-*。

### 1.10 ContextMenu

**目的**：右键菜单。

**底层**：**Radix ContextMenu**。

**API**：

```tsx
<ContextMenuTrigger>
  <CommitRow />
</ContextMenuTrigger>
<ContextMenuContent>
  <ContextMenuItem onSelect={copySha}>Copy SHA</ContextMenuItem>
  <ContextMenuItem onSelect={createBranch}>Create branch</ContextMenuItem>
  <ContextMenuSeparator />
  <ContextMenuItem destructive onSelect={resetHard}>Hard reset</ContextMenuItem>
</ContextMenuContent>
```

### 1.11 KeyHint

**目的**：快捷键提示（Cmd+K 等）。

**API**：

```tsx
<KeyHint keys={["⌘", "K"]} />
<KeyHint keys={["⌘", "⇧", "P"]} />
```

**样式**：圆角 pill + 等宽字体 + 灰色背景 + 1px 边框。

### 1.12 EmptyState

**目的**：空数据时的引导。

**API**：

```tsx
<EmptyState
  icon={<GitBranch size={32} />}
  title="No branches"
  description="Create a branch to start a new line of work."
  action={<Button>New branch</Button>}
/>
```

## 2. Composite（业务组件）

| 组件 | 位置 | 用途 |
|---|---|---|
| `WorkspaceSwitcher` | `components/WorkspaceSwitcher.tsx` | sidebar 顶部，列出 workspaces + 操作 |
| `RepoTree` | `components/RepoTree.tsx` | sidebar 树状展开当前 workspace 的 repos |
| `FeatureNav` | `components/FeatureNav.tsx` | 中栏二级导航（Tabs） |
| `CommitGraph` | `components/CommitGraph.tsx` | Main 上半部分，virtual scroll 历史图（Sprint 3）|
| `DiffViewer` | `components/DiffViewer.tsx` | Main 下半部分，文件 diff + 语法高亮（Sprint 3）|
| `BranchTree` | `components/BranchTree.tsx` | Branches tab 内容（Sprint 3）|
| `BlameView` | `components/BlameView.tsx` | 文件 blame 行内注释（Sprint 3）|
| `ConflictResolver` | `components/ConflictResolver.tsx` | 3-way merge UI（Sprint 6）|
| `CommandPalette` | `components/CommandPalette.tsx` | Cmd+K 浮层（Sprint 6）|
| `WorkingCopyBar` | `components/WorkingCopyBar.tsx` | 底部复合组件：branch 状态 + 文件列表 + commit 框（Sprint 4）|
| `SyncButtons` | `components/SyncButtons.tsx` | 顶部 Fetch / Pull / Push 三按钮（Sprint 4）|

## 3. Working Copy 相关 primitive（新增）

### 3.1 BranchIndicator

**目的**：显示当前 branch + ahead/behind chip。

**API**：

```tsx
<BranchIndicator
  branch="feature/foo"
  ahead={2}
  behind={3}
  upstream="origin/feature/foo"
/>
<BranchIndicator branch="(detached)" sha="abc1234" />  {/* detached HEAD */}
```

**Props**：

| 字段 | 类型 | 默认 | 备注 |
|---|---|---|---|
| branch | string | | 当前 branch 名或 `"(detached)"` |
| ahead | number | 0 | 本地领先远程多少 commit |
| behind | number | 0 | 远程领先本地多少 commit |
| upstream | string \| null | null | upstream 名（如 `origin/main`） |
| sha | string | null | detached 时显示短 SHA |

**样式**：chip 形式 + green `↑N` chip + orange `↓N` chip。detached 时灰显。

### 3.2 FileListItem

**目的**：单个文件变更行（unstaged / staged 都用）。

**API**：

```tsx
<FileListItem
  change={{
    path: "src/api.ts",
    kind: "modified",
    staged: false,
    additions: 3,
    deletions: 1,
  }}
  onClick={() => showDiff(change)}
  onStageToggle={() => toggleStage(change)}
/>
```

**Props**：

| 字段 | 类型 | 默认 | 备注 |
|---|---|---|---|
| change | `FileChange` | | 见 `04-working-copy.md` §3 |
| onClick | () => void | | 点击 → 在 Main 显示此文件 diff |
| onStageToggle | () => void | | 点击 status icon 触发 |
| selected | boolean | false | 高亮态（被 Main diff 选中） |

**样式**：

```
┌─────────────────────────────────────────┐
│ M  src/api.ts                  +3 -1     │
└─────────────────────────────────────────┘
```

status 字符：M（modified） / A（added） / D（deleted） / ?（untracked） / R（renamed） / C（copied）。

### 3.3 StatusIcon

**目的**：单字符 + 颜色标识文件状态。

**API**：

```tsx
<StatusIcon kind="modified" staged={false} />
```

**Props**：

| 字段 | 类型 | 默认 | 备注 |
|---|---|---|---|
| kind | FileStatusKind | | |
| staged | boolean | false | staged 状态决定颜色：unstaged 蓝 / staged 绿 |

**颜色**：

| kind | 字符 | 颜色 |
|---|---|---|
| modified | M | orange |
| added | A | green |
| deleted | D | red |
| untracked | ? | gray |
| renamed | R | blue |
| copied | C | purple |

### 3.4 CommitMessageBox

**目的**：多行 commit message 输入。

**API**：

```tsx
<CommitMessageBox
  value={message}
  onChange={setMessage}
  onSubmit={() => commit(message)}
  onAiGenerate={() => aiGenerate()}
  amendMessage={amendMode ? currentHeadMessage : null}
  disabled={!hasStagedChanges}
/>
```

**Props**：

| 字段 | 类型 | 默认 | 备注 |
|---|---|---|---|
| value | string | | controlled |
| onChange | (v: string) => void | | |
| onSubmit | () => void | | Enter / ⌘Enter 触发 |
| onAiGenerate | () => void | | AI 生成按钮回调（Sprint 4 接入） |
| amendMessage | string \| null | null | Amend 模式时预填当前 HEAD message |
| disabled | boolean | false | 没有 staged 时禁用 |

**样式**：

- 多行 textarea（min 3 行，max 8 行）
- placeholder："feat: ..."（Conventional Commits 风格）
- 下方两个按钮：`Commit`（主） / `Amend`（次，Sprint 4+） / AI 生成（icon）

### 3.5 SyncButtons

**目的**：Fetch / Pull / Push 三按钮组，置于 Topbar。

**API**：

```tsx
<SyncButtons
  ahead={2}
  behind={3}
  onFetch={() => fetch()}
  onPull={() => pull()}
  onPush={() => push()}
  inProgress={{ fetch: false, pull: false, push: true }}
/>
```

**Props**：

| 字段 | 类型 | 默认 | 备注 |
|---|---|---|---|
| ahead | number | 0 | Push 按钮显示 `↑N` badge |
| behind | number | 0 | Pull 按钮显示 `↓N` badge |
| onFetch / onPull / onPush | () => void | | 三个回调 |
| inProgress | object | {} | 哪个操作正在进行（spinner 状态） |

**样式**：

```
[Fetch] [Pull↓3] [Push↑2]
```

`↑N` chip 绿色 / `↓N` chip 橙色。当 ahead/behind = 0 时，对应按钮灰显（但仍可点击触发 fetch 拉新数据）。

### 3.6 WorkingCopyBar

**目的**：底部复合组件，聚合 BranchIndicator + Unstaged/Staged 列表 + CommitMessageBox + 全局操作。

**API**：

```tsx
<WorkingCopyBar
  repoId={activeRepoId}
  initialHeight={120}
/>
```

内部管 state：自身高度（可拖拽）、本地 dirty 状态。

**Props**：

| 字段 | 类型 | 默认 | 备注 |
|---|---|---|---|
| repoId | string \| null | null | 当前 active repo id；null 时隐藏整 bar |
| initialHeight | number | 120 | 首次展开高度 |

## 4. 主题

所有组件使用 `useTheme()` hook：

```ts
const { theme, setTheme } = useTheme();
// theme: "light" | "dark" | "system"
// setTheme("dark") 切换；"system" 跟随 prefers-color-scheme
```

`<html>` 元素上挂 `class="dark"` 或 `class="light"`；Tailwind dark 变体生效。

## 5. 关联

- `00-overview.md`：决策背景
- `01-tokens.md`：颜色 / 间距 / 字体 / 阴影
- `03-layout.md`：3-pane 布局规格
- `04-working-copy.md`：Working Copy Bar 完整规格 + 数据模型
- `docs/tech/decisions/0005-ui-library-stack.md`：库选择 ADR