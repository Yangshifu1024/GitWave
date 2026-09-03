# feat-collapse-commit-message · 审查报告

> 审查人：code-reviewer 代理 · 分支 `feature/collapse-commit-message` · 审查对象：工作区未提交改动
> 结论：**可合入**（🔴 0 / 🟡 1 / 🟢 4；🟡 与可立即执行的 🟢 已在审查后修复并复验）

## 主要结论（按维度）

- **正确性**：通过。className 覆盖链路经 `cn`（twMerge）与 `Button.tsx:74` 合并顺序验证有效；折叠态 truncate 的 flex 收缩链（row `justify-between` → Button `flex-1 min-w-0` → span `min-w-0 truncate` → Chip `shrink-0`）可靠单行省略；subject 本就取 `message_full` 首行，折叠隐藏 body 语义正确；aria-expanded 与图标状态映射正确
- **安全**：通过。无新增输入面，subject 经 React 转义
- **性能**：通过。无多余计算与滥用 memo
- **可维护性 / 可读性**：通过；状态作用域注释准确（App.tsx wrapper key 为 repoId，切 sha 不重挂载）
- **测试覆盖**：与仓库现状一致（无组件渲染测试设施）；i18n 双语同步由 `parity.test.ts` 守护；148 用例全过
- **最佳实践**：复用 DiffViewer / ChangesPanel 既有折叠惯例

## 问题与处理

| 级别 | 问题 | 处理 |
|---|---|---|
| 🟡 | aria-label 整体替换可见文本（WCAG 2.5.3 Label-in-Name）：带可见文本的折叠按钮应让名称来自内容（ChangesPanel 先例），而非 aria-label | **已修复**：删除 aria-label，保留 `aria-expanded` + `title` |
| 🟢 | `group/group-hover` 非仓库惯例（全仓仅此文件使用） | **已修复**：改为 Button 上 `text-text-muted hover:text-text-secondary`，chevron 继承 currentColor（对齐 DiffViewer） |
| 🟢 | aria-label/title 三元重复、两个图标分支 className 逐字重复 | **已修复**：aria-label 删除后仅 title 一处三元；chevron 提取 `const Chevron = messageExpanded ? ChevronDown : ChevronRight` |
| 🟢 | 注释可补充「切仓库 tab 时 wrapper remount 会重置折叠状态」 | **已修复**：注释已补充 |
| 🟢 | 容器 `select-text` 下标题双击会触发两次切换、按钮内文本不可拖选（大点击区的既定取舍，有 copyInfo/copySha 替代复制路径） | 记录于此，不改 |

## 修复后复验

`npm run typecheck` ✅ · `npm test` 22 文件 / 148 用例全过 ✅ · `npm run lint` ✅ · prettier ✅
