# feat: 可点击元素 hover 显示手型指针

状态：已实现

## 需求来源

用户 2026-08-28：所有用户可操作点击的元素，hover 时鼠标指针改为手型。分支：`feature/theme-design`（沿用）。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 实现方式 | tokens.css 末尾一条全局 `@layer base` 规则 | Tailwind v4 preflight 已不给 button 默认 pointer；逐组件加类 = 30+ 处 churn 且会漏 Radix 运行时节点 |
| 覆盖面 | 原生 button/select/a/summary/label[for] + ARIA 角色 button/menuitem(±checkbox/radio)/option/radio/tab/combobox | Radix 菜单项等是运行时注入的 div + role，必须按角色覆盖 |
| 禁用态 | 保持默认箭头（`:not(:disabled)` / `:not([aria-disabled="true"])` 排除） | 不可操作元素不显示手型；如需 🚫 改一行 |
| 文本输入 | 不在列表，保留 I-beam | — |
| 既有 cursor-pointer | 10 处保留不动 | 冗余但无害 |

## 改动清单

- `src/styles/tokens.css`：末尾新增 @layer base cursor 规则块（仅此一文件）

## 验证

- `npm run build` 后 grep dist CSS 确认 `cursor:pointer` 规则生成
- 真机抽查：齿轮/下拉项/chips/文件行/设置导航/radix 菜单手型；禁用按钮箭头；输入框 I-beam
- 纯 CSS 改动按先例跳过独立 reviewer 轮
