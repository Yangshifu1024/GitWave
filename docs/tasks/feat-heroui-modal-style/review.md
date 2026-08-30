# feat-heroui-modal-style · 审查报告

审查人：code-reviewer 代理（基于 git diff + @heroui/react 3.2.4 / react-aria 源码实证 + 产物 CSS）。

## 结论

**通过，无 🔴**。机制层疑虑逐项实证解除：

- Fragment 直返 Header/Body/Footer 符合 RAC 用法；`aria-labelledby` 经 `HeadingContext`
  注入，与嵌套层级无关（react-aria useDialog.mjs）
- 初始焦点在 dialog 自身（tabIndex=-1），首个 Tab 停 CloseTrigger；绝对定位不改 tab 序
- **dark mode 无冲突**：`.modal__dialog` 的 `bg-overlay` 经 tokens.css 桥接
  `--overlay: var(--color-bg-elevated)` 解析，与被移除的 `bg-bg-elevated` 取色完全一致，
  表面色视觉零变化；`--radius-3xl` 已确认存在（实际 24px）
- `onPress={close}` 与 slot 机制双触发幂等无害（旧代码同款，保留）

## 审查发现与处置

| 级别 | 发现 | 处置 |
|---|---|---|
| 🟡 | Body 文字继承官方 `.modal__body` 的 `text-muted`（桥接 `--color-text-muted`，对比度 ~3:1） | **定性修正**：Body 组件一行未动，`.modal__body` 的 muted 旧代码同样生效，**非本次回归**，是官方设计取向的既有行为。当前各弹框 Body 以表单控件（自带显式颜色）为主，暂保持官方默认；若未来出现大段纯文本 Body 再显式补 `text-text-secondary` |
| 🟡 | footer 插槽（mt-5）与手写 footer（mt-3）并存期观感差异，同文件内 Push vs Pull 可感知 | 属用户拍板的范围边界（方案 A：只迁 Push）。登记后续清单，不阻塞 |
| 🟡 | 共享 Modal 无组件级测试（a11y 关联 / footer 插槽 / 关闭行为零覆盖） | 需引入 @testing-library，超出本任务范围，登记后续清单 |
| 🟢 | Pull 弹框 Checkbox 同款死类 `text-text-primary` | 已顺手同删（保持一致性） |
| 🟢 | sizeClasses 与官方 size 修饰符双轨 | 有意保留（响应式 90vw 官方无），已在 plan.md 注明 |

## 后续清单（非阻塞）

1. 剩余 ~30 处手写 footer div 迁移到 `footer` 插槽（ActionBar 内 6 处优先：
   Pull / Stash / Fetch / Hooks 等同族弹框，跨文件 ReflogPanel / BranchList 随后）
2. Modal 最小冒烟测试 ×3：aria-labelledby 指向 Heading / footer 插槽渲染与缺省不渲染 /
   CloseTrigger 触发 onOpenChange(false)
3. `--muted` 桥接值是否加深（对齐 text-secondary）——全站 HeroUI muted 场景统一决策

## 用户真机验收清单

- [ ] Push 弹框：标题 16px medium（非大号加粗）、圆角约 24px、关闭钮右上角浮动
- [ ] Push 弹框：Branch/To 标签左对齐、值列对齐、按钮区与内容间距 20px
- [ ] 全 app 任意弹框（Settings / Stash / Workspace 切换等）开合正常、无布局破损
- [ ] 暗色模式弹框底色正常（应与改前一致）
- [ ] 长标题弹框（如含路径的确认框）标题不被关闭钮遮挡
