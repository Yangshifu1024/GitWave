# feat: commit modal 样式调整

状态：已实现

## 需求来源

用户 2026-08-28 三点：左右栏各占一半；左栏文件列表文件名用 UI 字体（非 mono）且小一号；commit message 框至少 6 行。分支：`feature/theme-design`（沿用）。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 栅格 | `grid-cols-[minmax(340px,420px)_1fr]` → `grid-cols-2` | modal 为 xl 宽（1200px），等分各 ~590px 可用 |
| 文件名字体 | `FileListItem` 根 `text-sm font-mono` → `text-xs`（默认 sans） | 共享组件：inspector 的 bar 布局列表同步生效，保持一致；`+n/-n` 统计保留自身 `font-mono text-xs` |
| 消息框高度 | `rows={2}` → `rows={6}` | text-sm 行高 20px × 6 + padding ≈ 136px；bar/modal 两种布局统一 |

## 改动清单

- `src/components/ui/WorkingCopyModal.tsx`：栅格等分
- `src/components/ui/FileListItem.tsx`：文件名 sans + text-xs
- `src/components/ui/CommitMessageBox.tsx`：rows 6

## 需求 2（2026-08-28 追加）

AI Generate 按钮去文字保留 ✨ 图标，移到 Commit 按钮前（右侧按钮组内，Amend 之后）。

- `CommitMessageBox`：底部行改 `justify-end`，AI Generate 为 ghost icon-only（`p-1`，保留 title/aria-label），移除左侧占位 span

## 需求 3（2026-08-28 追加）

首行超长警告误触发：条件用的是整条消息 `value.length >= 72`，文案却是 "first line > 72"，带 body 的正常消息（首行未超）也被标红。

- 修复：改判 `firstLineLength = value.split("\n", 1)[0].length > 72`（git subject 72 惯例只看首行），警告时顺带显示首行实际长度

## 验证

- typecheck / lint / prettier / vitest / build
- 真机：打开 Local Changes modal 查看布局、字体、消息框高度
