# feat-commit-message-type-chips · 提交消息类型标签（feat / fix / chore…）

> 状态：实施完成（待手动冒烟；未提交）
> 需求（用户，2026-08-28）：提交弹框中，提交消息上方增加 feat / fix / chore 等标准消息
> 标题头；点击后消息框内出现对应的 `feat: ` 内容，光标放在内容之后。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 类型集合 | Conventional Commits 标准十类：feat / fix / chore / docs / style / refactor / perf / test / build / ci | flex-wrap 自动换行，窄列（Modal 左列）下两行排布 |
| 插入行为 | 剥离已有 conventional 前缀（`/^…:\s*/`）再拼 `type: `——重复点击**替换**前缀而非叠加 | 光标经 rAF 定位到前缀之后（`type.length + 2`），受控值下一帧渲染后 setSelectionRange |
| 位置 | CommitMessageBox 组件内部（消息 textarea 上方） | 组件拥有 textarea ref，插入 + 光标定位内聚；该组件仅 WorkingCopyModal 使用 |
| 样式 | mono 小号文字 chip，border-subtle，hover accent | 与整体 token 体系一致 |

## 改动清单

- `src/components/ui/CommitMessageBox.tsx`：`COMMIT_TYPES` 常量 + `applyType`（前缀替换 / 光标定位）+ 类型 chips 行

## 验证

- [x] `npm run typecheck` / `lint` / `format:check` / `test`（43）/ `build` 全绿
- [ ] 手动冒烟：空消息点击 feat → `feat: ` 且光标在冒号空格后；已有 `fix: x` 时点 chore → 替换为 `chore: x`；AI 生成后再点类型 → 前缀加在生成文本前
