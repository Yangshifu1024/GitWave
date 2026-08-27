# GitWave · Design Tokens

> 设计 token 定义：颜色 / 间距 / 字体 / 圆角 / 阴影 / 动效。
> 这些 token 由 Tailwind theme 扩展 + CSS 自定义属性双重承载；前端代码统一通过 Tailwind 类名访问。

## 1. Color（色板）

GitWave 提供**两套配色 palette**，共享同一组语义状态色（success / warning / danger 等），仅中性铬色与强调色不同：

| Palette | 气质 | 强调色 | 状态 |
|---|---|---|---|
| **native-blue** | macOS 系统窗口质感 | systemBlue `#007AFF` / dark `#0A84FF` | **默认** |
| **tide** | GitWave 青绿签名（Foam/Mist/Ink 冷灰系）| Tide `#1A8F8A` / dark `#3EBAB3` | 可选 |

命名色（Tide palette 品牌）：**Foam** `#F4F6F8` · **Mist** `#E6EBEF` · **Ink** `#1B2228` · **Tide** `#1A8F8A` · **Abyss** `#12161A` · **Coral** `#D64545`。

### 1.0 Palette 运行时机制

- `<html data-palette="...">` 承载 palette 维度；light/dark 维度仍由 `.light/.dark` class 与 `prefers-color-scheme` 承载（见 `src/styles/tokens.css`）
- 偏好持久化于 localStorage 键 `gitwave-palette`（缺省/非法值回落 native-blue）；启动预热见 `main.tsx` 的 `applyInitialPreferences()`
- 常量与读写：`src/lib/palette.ts`（`PALETTES` / `DEFAULT_PALETTE` / `normalizePalette`），React hook：`usePalette()`
- 设置入口：Toolbar ⋯ 菜单 → Settings… → Appearance → Color palette（即点即生效）

### 1.1 Light theme（native-blue 默认）

```
背景
  bg-primary        #ECECEE       系统窗口灰画布（应用根 / 顶栏 / 侧栏 / WorkingCopyBar）
  bg-secondary      #DFDFDF       层次色（gutter / hover / 菜单焦点）
  bg-elevated       #F4F4F5       抬升面（列表 hover / 菜单 / 模态）
  bg-panel          #F8F8F8       三栏面板画布（sidebar / history / inspector）
  bg-overlay        rgba(0,0,0,0.38)  模态遮罩

文本
  text-primary      #1B1B1D       近 label 黑
  text-secondary    #55555B       次要文本 / 标签
  text-muted        #85858B       辅助 / 占位
  text-inverse      #ffffff       深色按钮上的文字

边框 / 分隔
  border-subtle     #DBDBDD       浅分隔线
  border-default    #CBCBCF       控件边框
  border-strong     #A9A9AE       强分隔

强调色
  accent            #007AFF       systemBlue
  accent-hover      #0068D9       悬停

历史图 lane（--color-lane-1..5，Apple system colors）
  lane-1            #007AFF       blue
  lane-2            #32ADE6       teal
  lane-3            #5856D6       indigo
  lane-4            #AF52DE       purple
  lane-5            #8E8E93       gray
```

共享语义状态色（两套 palette 相同）：

```
  success           #2F9E6B       成功
  warning           #C47A1A       警告
  danger            #D64545       Coral
  info              #3D6B9A       信息
  status-active     #2F9E6B       仓库 active
  status-missing    #D64545       仓库 missing
  branch-remote     #7A8692       远程分支（灰）
  branch-ahead      #2F9E6B       ahead
  branch-behind     #C47A1A       behind
  branch-conflict   #D64545       conflict
  branch-local / branch-current = 各 palette 的 accent 值
```

### 1.2 Dark theme（native-blue 默认）

```
背景
  bg-primary        #262628       系统暗窗口画布（应用根 / 顶栏 / 侧栏 / WorkingCopyBar）
  bg-secondary      #202022       层次色
  bg-elevated       #313134       菜单 / 抬升面
  bg-panel          #262628       中 / 右面板画布（同 bg-primary，dark 不区分）
  bg-overlay        rgba(0,0,0,0.55)  模态遮罩

文本
  text-primary      #EFEFF1
  text-secondary    #A8A8AD
  text-muted        #78787E
  text-inverse      #202022

边框
  border-subtle     #333336
  border-default    #444448
  border-strong     #5C5C61

强调色（dark mode 提亮）
  accent            #0A84FF       systemBlue (dark)
  accent-hover      #409CFF

历史图 lane（dark）
  lane-1            #0A84FF   lane-2 #64D2FF   lane-3 #7D7AFF
  lane-4            #BF5AF2   lane-5 #98989D
```

### 1.3 Tide Studio palette 覆盖（`<html data-palette="tide">`）

仅覆盖与 native-blue 差异的 token；语义状态色不重复声明：

```
                  Light                    Dark
bg-primary        #F4F6F8 (Foam)           #161B20
bg-secondary      #E6EBEF (Mist)           #12161A (Abyss)
bg-elevated       #EEF1F4                  #1C2329
bg-panel          = bg-primary             = bg-primary
bg-overlay        rgba(27,34,40,0.40)      = 共享 rgba(0,0,0,0.55)
text-primary      #1B2228 (Ink)            #E8ECF0
text-secondary    #4A5560                  #B4BEC8
text-muted        #7A8692                  #8B97A3
text-inverse      #ffffff（同共享）          #12161A
border-subtle     #D5DCE2                  #2A323A
border-default    #C5CDD4                  #3A444E
border-strong     #9AA4AE                  #5A6570
accent            #1A8F8A (Tide)           #3EBAB3（提亮）
accent-hover      #157873                  #5EC9C2
branch-local/-current = accent 同值
lane-1..5         #1A8F8A #3D6B9A #4A5FA8 #5B56A8 #7A8692
                  #3EBAB3 #6B8FC4 #4A5FA8 #5B56A8 #8B97A3
```

### 1.4 Semantic 映射（Tailwind）

```
Tailwind 类              Token                 用途
bg-bg-primary         →   bg-primary
bg-bg-panel           →   bg-panel
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