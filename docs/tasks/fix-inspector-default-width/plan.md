# fix-inspector-default-width · 右侧栏默认宽度与最小宽度一致

> 状态：已完成
> 需求（用户）：修改右侧栏默认宽度，与其最小宽度一致。

## 现状与根因

右侧栏（Inspector）宽度定义在 `src/components/ui/ThreePaneLayout.tsx`：
默认宽度 `initialInspectorWidth = 500`，最小宽度 `inspectorMin = 360`，两者不一致，
导致启动时右侧栏比可拖到的最小宽度更宽。唯一调用处 `src/App.tsx` 未传显式宽度 props，
改默认参数即可全局生效。

设计文档旁证：`docs/design/00-overview.md` 与 v2 mockup（`--inspector-w: 360px`）
均为 ~360px，仅 `docs/design/03-layout.md` 残留旧值 ~500px。

## 改动清单

- `src/components/ui/ThreePaneLayout.tsx`：`initialInspectorWidth` 默认值 `500` → `360`，
  与 `inspectorMin = 360` 一致。启动宽度与双击分隔条的重置宽度同步变为 360px，
  拖拽范围（360–720）不变，无持久化逻辑受影响。
- `docs/design/03-layout.md`：同步 3 处 `Inspector (~500px)` → `~360px`，
  与 00-overview / v2 mockup 对齐。

## 测试

- `npm run typecheck` 通过（改动为常量默认值，无类型面变化）
- 手动冒烟要点：
  - 启动后右侧栏初始约 360px
  - 拖拽分隔条：最小到 360px、最大到 720px，中栏正常吸收剩余空间
  - 双击右侧分隔条：宽度重置回 360px
