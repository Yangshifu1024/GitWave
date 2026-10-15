# Workbench interaction follow-ups — review

Date: 2026-10-15. Reviewer: specialized code-reviewer agent.
Scope: [plan.md](./plan.md), plus the requested unified selection styling and branch indicator width adjustment. Existing Tide implementation outside these follow-ups was not re-audited. Source review was read-only; this report is the only reviewer-authored file.

## ✅ 优点

- **正确性**：Modal 在受控关闭时立即卸载，不再等待可能被取消的退出动画；隐藏下载保留 updater 状态。单文件文本 diff 使用完整高度链，多文件与图片布局保持原路径。History 保留完整拓扑、定位、分页和右键操作。
- **安全性**：本轮修改没有新增外发数据、Git 写入入口或跳过操作确认；关闭更新提示不会调用安装或重启。
- **性能**：DiffText 与 CommitGraph 保留虚拟滚动；列对齐和 Blame 响应式布局使用 CSS，无逐行尺寸测量。
- **可维护性**：关闭可靠性集中修复于公共 Modal；fillViewport 明确限定单文件文本预览，Blame 使用命名容器查询控制窄面板行为。
- **可读性**：分支名和 upstream 按可用空间展示并保留完整 title；History 元信息采用一致列宽；Blame 作者可截断、代码不换行、重复标题移除。
- **测试覆盖**：新增真实 Modal/Button 集成测试覆盖 X、稍后、Esc、动画取消和重新打开，以及下载隐藏后 ready 自动重开。DiffText 测试覆盖 unified/split、3000 行、resize、末行可达和 DOM 数量边界；现有 graph 几何、定位与 refs 测试得到保留和适配。
- **最佳实践**：保留受控组件状态、焦点清理路径、可访问名称和原有选择语义；选择样式统一时仍区分预览当前项与批量操作选中项。

## 🔴 严重问题（必须修复）

无未解决问题。

## 🟡 一般问题（建议修复）

本轮识别的两个 P2 均已修复并完成源码复查：

1. **Blame 窄面板固定列遮挡代码 — 已修复。** 原固定 sticky 归属列 20rem 加行号 3rem，在可支持的窄面板中会占满可视区域。当前小于 512px 的 Blame 容器使用 12rem 归属列、隐藏日期并取消两列 sticky，整行可以横向滚动；完整日期保留在 title。宽面板仍使用 20rem 与 sticky。
2. **文件列表 active / selected 样式混淆 — 已修复。** 原修改使 Cmd 取消选择后的预览当前项仍与批量选中项完全同样高亮。当前仅 selected 获得 accent 底色；active 且未 selected 仅保留左侧标线，因此批量操作范围可辨认。

## 🟢 优化建议（可选）

无。

## 验证证据与边界

以下浏览器结果由主代理提供，审查者复核对应源码；不是审查者独立执行的原生应用验收：

- 1024px 窗口、Changes 左栏 58%：Blame 面板约 345px，归属列约 191px，position 为 static；日期隐藏但完整 title 保留。
- History 不同 commit 行的 SHA、作者、时间列 x 坐标一致。
- 导航与文件选中样式为 radius 0、左线 3px、accent 10% 底色及正常主文字色。CommitGraph 的节点定位偏移同步调整为 11px。
- 动画取消复现说明匹配的关闭失败机制，不等于取得用户原始原生故障的运行轨迹。浏览器验证不代表 Windows/Linux 原生 WebView 覆盖。
- 主代理验证：`make check`、`pnpm build`、`git diff --check`通过。前端61文件/367测试及4个Node检查；Rust386测试通过、2个既有忽略、1个集成测试通过。包含最终tag优先变更；完整门禁和构建已再次通过。

## 📝 总体评价

七个维度审查后没有未解决的代码审查问题，两个发现的布局／交互回归均已修复。本轮变更可在完整质量门禁和浏览器验收通过后交付；保留退出动画被跳过这一有意取舍，以确保关闭立即释放遮罩。

## 追加审查：tag 优先展示

用户追加要求同一 commit 优先展示 tag。`mergeTrackedRefs` 在原有 tracked ref 合并完成后，对 `filter` / `map` 生成的新数组按 tag → local branch → remote branch 稳定排序；不会修改输入 refs，同类型顺序保持不变。`primaryRef = refs[0]` 与完整 refs 弹窗采用相同优先级，未展示的 refs 仍可由 `+N` 访问，local branch 的 synced 标识保留。

更新的测试验证 tag 主展示、完整弹窗排序及 synced local 保留；新增测试验证 remote branch 与 tag 共存时显示 tag 和 `+1`。主代理报告 8 个 graph 测试通过；最终完整门禁重跑由主代理补充。本次追加只读复查未发现新增正确性、安全、性能、可维护性、可读性、测试覆盖或最佳实践问题。

主代理最终确认：tag优先变更后的 `make check`（367前端测试、386 Rust单元测试及1集成测试）与生产构建均通过。
