# feat-heroui-modal-style · 实施计划

## 背景

用户反馈 Push 弹框观感与 HeroUI 官方样式不一致，要求参考官方样式修改弹框布局和样式。
项目实际使用 `@heroui/react@3.2.4`（HeroUI v3 / React Aria），共享弹框组件
`src/components/ui/Modal.tsx` 被 22 处使用方引用，其上叠了一层与官方默认值不同的
chrome 覆盖（rounded-xl / bg-bg-elevated / shadow-modal / 自拼 header 行）。

## 目标

弹框骨架对齐 HeroUI 官方 anatomy（`Modal.Header` / `Modal.Body` / `Modal.Footer` /
`Modal.CloseTrigger` 官方排布与默认样式），Push 弹框作为首个使用官方 footer 插槽的
使用方同步理顺内容区。

## 范围（用户拍板）

- 新分支 `feature/heroui-modal-style`
- 只动 2 个文件：`src/components/ui/Modal.tsx` + `src/components/ActionBar.tsx`
- 其余 21 个使用方靠 API 向后兼容自动受益，不在本任务迁移

## 改动点

1. **Modal.tsx 重构为官方 anatomy**
   - Dialog 移除 chrome 覆盖，落回官方 `.modal__dialog`（bg-overlay / shadow-overlay /
     p-6 / radius min(32px, --radius-3xl)）；仅保留项目响应式宽度类
     （90vw + 上限，官方 size 修饰符无此行为，属有意保留）
   - CloseTrigger 移出 header flex 行，按官方 `.modal__close-trigger` 绝对定位
     end-4 top-4；Header 加 `pe-10` 防长标题压钮
   - Heading 落官方默认（text-base font-medium text-foreground）
   - 新增可选 `footer?: ReactNode` prop → 官方 `Modal.Footer`
     （justify-end gap-2，`.modal__body + .modal__footer` mt-5）
   - Body 行为不变（含 -m-[3px]/p-[3px] 焦点环处理，注释保留）
2. **ActionBar Push 弹框**
   - 按钮（Cancel/Push）迁入 footer 插槽（首个参考用法）
   - Branch/To 信息行：标签右对齐→左对齐（消除截图中的锯齿缩进），text-xs→text-sm
     对齐官方 body 字号；去掉多余包裹 div，让 Body gap-3 统一节奏
   - Checkbox 去掉无效的 `className="text-text-primary"`（`.checkbox__content`
     自带 text-foreground，实证为死类）

## 不做清单（本任务边界）

- 其余 ~30 处手写 footer div 的插槽迁移（登记于 review.md 后续清单）
- 组件级测试补齐（需新增 @testing-library 依赖，另行立项）
- `--muted` 桥接值调整（影响全站，见 review.md 决策记录）

## 验证

eslint / tsc --noEmit / vite build / vitest 全绿；产物 CSS 确认含官方
`.modal__dialog` 规则、`pe-10`、`.modal__body+.modal__footer{mt-5}`。
真机观感由用户按 review.md 附带清单验收。
