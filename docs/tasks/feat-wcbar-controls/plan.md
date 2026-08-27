# feat-wcbar-controls · 提交框隐藏与最大化按钮

> 状态：实施完成（待 code review）
> 需求（用户，2026-08-27）：提交框在分支名称后增加两个按钮——① 隐藏：隐藏提交框；② 最大：将提交框高度拉到窗口一半。
> 分支：`feature/ui-native-studio-v2`

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 「提交框」主体 | `ui/WorkingCopyBar.tsx` dirty 面板 | 头部行即分支名所在行（BranchIndicator） |
| 按钮位置 | 头部行右端（计数之后） | 面板级控制惯例居右；长分支名不受挤压。字面“分支名后”以行为单位理解 |
| 「隐藏」语义 | 折叠为 32px 状态条 | 完全消失会失去恢复入口；细条保留分支名 + unstaged/staged 计数 + ChevronUp 恢复按钮 |
| 「最大」语义 | 高度 220px ↔ 50vh toggle | 与 inspector maximize（Maximize2/Minimize2）同交互同图标 |
| 联动规则 | 收起时自动退出最大化 | 折叠态无可还原的最大化身体 |
| 状态存放 | `useLayoutStore`（zustand，非持久） | 会话内 UI 态;切换 workspace/repo 时复位,与 inspectorMaximized 同策略（App.tsx reset effect） |
| clean 态 | 不加按钮 | clean 时只有 32px 状态条,无提交框可操作 |

## 改动清单

- `src/stores/layoutStore.ts` — 新增 `wcBarCollapsed` / `wcBarMaximized` 及 toggle/set;set/toggle 收起时联动清最大化
- `src/components/ui/WorkingCopyBar.tsx` — dirty 头部行新增 Hide/Maximize 两枚 ghost 图标按钮(Tooltip + aria-pressed,样式对齐 DiffViewer);新增 collapsed 渲染分支;height 改为条件值(50vh/220px)
- `src/App.tsx` — reset effect 扩展为同时复位两个新状态
- `src/stores/layoutStore.test.ts` — 新增:toggle 独立性、collapse 清 maximize、重复 set 幂等(zustand node 环境直测 store)
- `docs/design/04-working-copy.md` §3 — 增补控件规格

## 验收

- [x] typecheck / test(30)/ lint / build / format 全绿
- [ ] 手动冒烟(dev):dirty 态出现两枚按钮;Hide → 细条可恢复;Max → 半窗高往返;切 repo 后自动展开复原 —— 由用户目验
- [x] code-reviewer 审查通过(review.md)
