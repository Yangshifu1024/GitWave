# feat-ui-native-polish · 去网页感 UI 打磨

> 目标：在当前布局不变的前提下，通过去卡片化、去边框化、降圆角、收紧材质层级，让界面更像原生桌面 Git 客户端。  
> 设计依据：用户反馈“太像网页”；当前实际 UI 结构（Toolbar / ActionBar / WorkspaceRepoTabs / 3-pane / WorkingCopyModal）。

## 决策

- 不推翻当前 IA：保留 Toolbar / ActionBar / Repo Tab 条 / 3-pane / WorkingCopyModal。
- 侧栏从 HeroUI Card 堆叠改成平面 Source List。
- Repo Tab 保留 Tab 形态，但去浏览器式阴影/大圆角/贯通效果。
- ActionBar 按钮、状态区平面化，减少漂浮感。
- 输入框、按钮、Modal 降圆角、去边框。
- RefBadge 不再用 HeroUI Chip，改为自绘轻量 badge。

## 改动清单

### P0｜侧栏去卡片化

| 文件 | 改动 |
|---|---|
| `src/components/ui/SidebarSection.tsx` | 移除 Card/Disclosure 卡片样式；header 改为 10px uppercase muted label；内容区全宽、无圆角、无 border |
| `src/components/ui/ListItem.tsx` | 高度 28px；平面选中态（左侧 2px accent 条 + 极淡背景）；hover 用 `black/5` 或 `white/5`；移除 Surface |
| `src/App.tsx` | aside 背景改为 `bg-bg-secondary`，间距收紧 |
| `src/components/BranchList.tsx` | Local/Remote 分组头改平面缩进 label，去除 button/chevron 样式 |

### P0｜Repo Tab 去浏览器化

| 文件 | 改动 |
|---|---|
| `src/components/ui/Tabs.tsx` | 移除阴影、移除 `rounded-t-md`、选中态改为底部 1px accent 线 + 淡背景、hover 只改文字色 |
| `src/components/WorkspaceRepoTabs.tsx` | Tab 条背景改 `bg-bg-secondary`；高度降到 `h-7`；移除容器底边线；missing 点更克制 |

### P0｜ActionBar 平面化

| 文件 | 改动 |
|---|---|
| `src/components/ActionBar.tsx` | 按钮去 `rounded-md`、hover 更淡；WorkspaceDropdown 去边框；状态区去 Card 包裹；减少 Separator |
| `src/components/ui/SectionAction.tsx` | 样式同步收紧（用于侧栏区头时更扁平） |

### P1｜控件去表单化

| 文件 | 改动 |
|---|---|
| `src/components/ui/Button.tsx` | 圆角降到 `rounded-sm`；secondary/ghost/danger 去边框 |
| `src/components/ui/Input.tsx` | search variant 背景与面板同色、无边框或仅底部线；默认 variant 降圆角 |
| `src/components/ui/Modal.tsx` | 圆角降到 `rounded-lg` 或 `rounded-md`；padding 从 p-6 降到 p-4 |
| `src/components/ui/DropdownMenu.tsx` / `ContextMenu.tsx` | 菜单项 padding 减小、hover 更淡 |

### P1｜History 细节收紧

| 文件 | 改动 |
|---|---|
| `src/components/RefBadge.tsx` | 自绘 span，圆角 `rounded-sm`，去 border，半透明背景 + 文字色 |
| `src/components/CommitGraph.tsx` | hover/selected 背景更淡；HEAD 高亮更克制 |

### P2｜全局材质收敛

| 文件 | 改动 |
|---|---|
| `src/styles/tokens.css` | `pane-edge-right` / `pane-edge-left` 调淡 |
| `src/App.tsx` / `ThreePaneLayout.tsx` | 面板分隔统一用 inset shadow，减少外边框 |
| `src/components/Toolbar.tsx` | Toolbar 底边线保留但变淡；ActionBar 顶边线移除 |

## 验证

```bash
pnpm format:check
pnpm lint
pnpm typecheck
pnpm test
pnpm build
pnpm tauri dev
```

验收点：
- [ ] 侧栏无卡片感
- [ ] Repo Tab 无浏览器标签感
- [ ] ActionBar 无漂浮卡片感
- [ ] 输入框/按钮/Modal 更原生
- [ ] Light/Dark × native-blue/tide 无崩坏
- [ ] 焦点态仍可识别

## 关联

- `docs/tasks/feat-ui-visual-polish/plan.md`：前期已完成的多次视觉迭代
- `docs/design/05-visual-redesign.md`：旧版 Native Studio v2 方案（与本任务范围不同）
