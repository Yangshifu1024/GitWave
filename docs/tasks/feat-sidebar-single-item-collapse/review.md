# review · feat-sidebar-single-item-collapse

> 审查范围：RemotesPanel.tsx / WorktreePanel.tsx（+16/-1）及任务文档
> 维度：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践

## 结论

✅ 通过，无 🔴 / 🟡 问题，可合入。

## 分维度记录

| 维度 | 结论 | 说明 |
|---|---|---|
| 正确性 | ✅ | `Disclosure.defaultExpanded` 为 uncontrolled，仅靠 `defaultOpen` 变更不会重生效；按阈值 `items.length > 1` 加 key 重挂载后默认值正确重应用（沿用 TagsPanel 既有模式）。空态 / 加载态仍走 `collapsible={false}` 静态头，不受影响 |
| 安全 | ✅ | 纯 UI 默认态调整，不触碰数据、网络与凭证 |
| 性能 | ✅ | key 仅在跨阈值时变化，重挂载频率极低；列表行本身无状态 |
| 可维护性 | ✅ | 复用既有 `key + defaultOpen` 模式，未引入受控状态；决策记录见 plan.md |
| 可读性 | ✅ | 注释说明产品动机与模式出处，与周边注释密度一致 |
| 测试覆盖 | ✅（按现状） | 测试栈无 testing-library，组件渲染级断言不可行（同 fix-sidebar-tags-stale-on-repo-switch 的既有决策）；以 typecheck + lint + 161 单测 + plan.md 冒烟清单覆盖 |
| 最佳实践 | ✅ | 无新增 i18n 文案；Conventional Commits 命名；PR 关联任务文档 |

## 备查（非问题）

- 手动折叠多 remote 卡片后继续加 remote：key 不变（仍 "multi"），不重挂载，保持用户折叠态——符合"显式操作优先于默认值"。
- 手动展开单 remote 卡片：项数不变期间不重挂载，展开态保持——不会被闪回。
