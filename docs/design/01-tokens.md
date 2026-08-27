# GitWave · Design Tokens

> 设计 token 定义：颜色 / 间距 / 字体 / 圆角 / 阴影 / 动效。
> 这些 token 由 Tailwind theme 扩展 + CSS 自定义属性双重承载；前端代码统一通过 Tailwind 类名访问。

## 1. Color（色板）

命名色（品牌）：**Foam** `#F4F6F8` · **Mist** `#E6EBEF` · **Ink** `#1B2228` · **Tide** `#1A8F8A` · **Abyss** `#12161A` · **Coral** `#D64545`。

### 1.1 Light theme

```
背景
  bg-primary        #F4F6F8       Foam 主画布（history / inspector）
  bg-secondary      #E6EBEF       Mist 侧栏 / 工具栏
  bg-elevated       #EEF1F4       抬升面（列表 hover / inspector）
  bg-overlay        rgba(27,34,40,0.40)  模态遮罩

文本
  text-primary      #1B2228       Ink
  text-secondary    #4A5560       次要文本 / 标签
  text-muted        #7A8692       辅助 / 占位
  text-inverse      #ffffff       深色 / Tide 按钮上的文字

边框 / 分隔
  border-subtle     #D5DCE2       浅分隔线
  border-default    #C5CDD4       控件边框
  border-strong     #9AA4AE       强分隔

状态色
  accent            #1A8F8A       Tide
  accent-hover      #157873       悬停
  success           #2F9E6B       成功
  warning           #C47A1A       警告
  danger            #D64545       Coral
  info              #3D6B9A       信息（靛蓝族，不用系统蓝）

仓库 / 分支状态（语义）
  status-active     #2F9E6B       仓库 active
  status-missing    #D64545       仓库 missing
  branch-local      #1A8F8A       本地分支（Tide）
  branch-current    #1A8F8A       当前 HEAD（Tide）
  branch-remote     #7A8692       远程分支（灰）
  branch-ahead      #2F9E6B       ahead
  branch-behind     #C47A1A       behind
  branch-conflict   #D64545       conflict
```

### 1.2 Dark theme

```
背景
  bg-primary        #161B20       略抬升画布
  bg-secondary      #12161A       Abyss 侧栏 / 工具栏
  bg-elevated       #1C2329       inspector / 菜单
  bg-overlay        rgba(0,0,0,0.55)  模态遮罩

文本
  text-primary      #E8ECF0
  text-secondary    #B4BEC8
  text-muted        #8B97A3
  text-inverse      #12161A

边框
  border-subtle     #2A323A
  border-default    #3A444E
  border-strong     #5A6570

状态色（dark mode 调亮）
  accent            #3EBAB3       Tide 提亮
  accent-hover      #5EC9C2
  success           #3ECF8E
  warning           #E0A04A
  danger            #E05A5A
  info              #6B8FC4
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
  SHA / 路径 / diff: "IBM Plex Mono"
  fallback: "SF Mono", Menlo, Monaco, Consolas, "Cascadia Code", ui-monospace
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