# GitWave · Design Tokens

> 设计 token 定义：颜色 / 间距 / 字体 / 圆角 / 阴影 / 动效。
> 这些 token 由 Tailwind theme 扩展 + CSS 自定义属性双重承载；前端代码统一通过 Tailwind 类名访问。

## 1. Color（色板）

### 1.1 Light theme

```
背景
  bg-primary        #ffffff       主背景（窗口）
  bg-secondary      #fafafa       面板背景（sidebar / nav）
  bg-elevated       #ffffff       卡片 / 列表项
  bg-overlay        rgba(0,0,0,0.35)  模态遮罩

文本
  text-primary      #1f1f1f       主要文本
  text-secondary    #555555       次要文本 / 标签
  text-muted        #888888       辅助 / 占位
  text-inverse      #ffffff       深色背景上的文字

边框 / 分隔
  border-subtle     #e3e3e3       浅分隔线
  border-default    #cccccc       控件边框
  border-strong     #999999       强分隔

状态色
  accent            #007aff       macOS systemBlue（accent-color）
  accent-hover      #0066cc       悬停态
  success           #2ecc71       成功
  warning           #f39c12       警告
  danger            #c0392b       危险 / 删除 / missing
  info              #3498db       信息提示

仓库 / 分支状态（语义）
  status-active     #2ecc71       仓库 active
  status-missing    #c0392b       仓库 missing
  branch-local      #007aff       本地分支
  branch-current    #5856d6       当前 HEAD（紫）
  branch-remote     #8e8e93       远程分支（灰）
  branch-ahead      #34c759       ahead（绿）
  branch-behind     #ff9500       behind（橙）
  branch-conflict   #ff3b30       conflict（红）
```

### 1.2 Dark theme

```
背景
  bg-primary        #1a1a1a       主背景
  bg-secondary      #232323       面板背景
  bg-elevated       #2a2a2a       卡片 / 列表项
  bg-overlay        rgba(0,0,0,0.5)  模态遮罩

文本
  text-primary      #f0f0f0       主要文本
  text-secondary    #c0c0c0       次要文本
  text-muted        #8e8e93       辅助
  text-inverse      #1f1f1f       深色背景

边框
  border-subtle     #2a2a2a
  border-default    #3a3a3a
  border-strong     #5a5a5a

状态色（dark mode 调亮）
  accent            #0a84ff       macOS darkBlue
  accent-hover      #409cff
  success           #30d158
  warning           #ff9f0a
  danger            #ff453a
  info              #64d2ff
```

### 1.3 Semantic 映射（Tailwind）

```
Tailwind 类              Token                 用途
bg-bg-primary         →   bg-primary
bg-bg-secondary       →   bg-secondary
text-text-secondary   →   text-secondary
border-border-default →   border-default
text-accent           →   accent
bg-danger             →   danger
...
```

## 2. Spacing（间距）

```
0     none
1     4px    紧贴元素 / 内 padding
2     8px    表单字段间距 / 列表项内 padding
3     12px   段落间距
4     16px   卡片内边距 / 节区上下间距
6     24px   大节区 / 模态内 padding
8     32px   顶部 / 底部边距
12    48px   topbar 高度
```

Tailwind: 默认 spacing scale 已被默认覆盖到 4px 网格；用 `p-1` (=4px) / `gap-2` (=8px) / `p-4` (=16px) 等。

## 3. Typography（字体）

### 3.1 Font families

```
font-sans
  macOS:    -apple-system, BlinkMacSystemFont, "SF Pro Text"
  Windows:  "Segoe UI"
  Linux:    Cantarell, Ubuntu, "Helvetica Neue"
  fallback: system-ui, -apple-system, sans sans-serif

font-mono
  macOS:    "SF Mono", Menlo, Monaco
  Windows:  Consolas, "Cascadia Code"
  Linux:    "JetBrains Mono", "DejaVu Sans Mono"
  fallback: ui-monospace, monospace
```

### 3.2 Scale

```
text-xs      12px / 1.4     标签 / 提示
text-sm      13px / 1.5     正文 / 列表项
text-base    14px / 1.5     默认正文
text-md      16px / 1.5     小标题 / 强调
text-lg      20px / 1.3     标题
text-xl      28px / 1.2     大标题
```

### 3.3 Weight

```
font-normal  400   正文
font-medium  500   强调
font-bold    600   标题
```

## 4. Radius（圆角）

```
radius-sm    4px    标签 / badge
radius-md    6px    按钮 / 输入框
radius-lg    8px    卡片
radius-xl    12px   模态 / 浮层
radius-full  9999px 头像 / pill 按钮
```

## 5. Shadow（阴影）

```
shadow-subtle
  0 1px 2px rgba(0,0,0,0.04), 0 1px 1px rgba(0,0,0,0.06)
  → 列表项 hover / 按钮 active

shadow-modal
  0 12px 32px rgba(0,0,0,0.18), 0 2px 4px rgba(0,0,0,0.08)
  → 模态 / popover / dropdown

shadow-overlay
  0 0 0 1px rgba(0,0,0,0.04), 0 4px 16px rgba(0,0,0,0.08)
  → focus ring / 高亮边框
```

Dark mode 阴影用更深底色 + 更高 alpha：

```
shadow-subtle-dark
  0 1px 2px rgba(0,0,0,0.3), 0 1px 1px rgba(0,0,0,0.4)

shadow-modal-dark
  0 12px 32px rgba(0,0,0,0.6), 0 2px 4px rgba(0,0,0,0.4)
```

## 6. Motion（动效）

```
duration-fast    120ms    颜色 / 透明度切换
duration-base    200ms    默认 transition（hover / focus / 状态变化）
duration-slow    300ms    模态进出 / pane 切换
duration-spring  spring   模态 spring（macOS 风格 0.85 damping）

ease-default    cubic-bezier(0.4, 0, 0.2, 1)   ease-out
ease-in         cubic-bezier(0.4, 0, 1, 1)
ease-out        cubic-bezier(0, 0, 0.2, 1)
```

## 7. Z-Index（层级）

```
z-base        0        内容
z-elevated    10       sticky 工具栏 / dropdown 内容
z-popover     100      popover / context menu / tooltip
z-modal       1000     modal / dialog backdrop
z-toast       2000     toast 通知
```

## 8. Token 实施

### 8.1 Tailwind theme 扩展（v4 CSS-first）

在 `src/styles/tokens.css` 用 `@theme` 块定义：

```css
@import "tailwindcss";

@theme {
  /* colors */
  --color-bg-primary: #ffffff;
  --color-bg-secondary: #fafafa;
  ...
  /* spacing */
  --spacing: 4px;
  ...
  /* radius */
  --radius-sm: 4px;
  --radius-md: 6px;
  ...
  /* shadows */
  --shadow-subtle: 0 1px 2px rgba(0,0,0,0.04), ...;
  ...
}

@media (prefers-color-scheme: dark) {
  @theme {
    --color-bg-primary: #1a1a1a;
    ...
  }
}
```

### 8.2 dark mode 手动切换

`<html>` 元素加 `class="dark"` 切换。使用 `@variant dark (...)` 选择器。

## 9. 关联

- `00-overview.md`：项目级设计总览
- `02-components.md`：组件如何使用这些 token
- `docs/tech/decisions/0005-ui-library-stack.md`：库选择 ADR