# style-sidebar-tag-sha-parentheses · Tag 行 sha 用括号包裹并与 tag 名留间隔

> 状态：已完成
> 需求（用户）：左侧栏中 tag 列表，tag 名后的 sha 应该使用括号包裹，并与 tag 名有间隔。

## 改动

`src/components/TagsPanel.tsx`：tag 行 name/sha 两个 span 原先是 inline 排列（无间距、截断失效），
改为 flex 容器（`gap-2` 间隔、`min-w-0 flex-1` 让长 tag 名正确截断），sha 渲染为 `({short})`。

## 测试

- typecheck / lint / prettier 全绿；纯样式改动，手动冒烟确认视觉即可。
