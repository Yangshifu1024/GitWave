# feat: Repository Tab 拖动排序 — 审查报告

- 审查代理：code-reviewer（7 维度：正确性 / 安全 / 性能 / 可维护性 / 可读性 / 测试覆盖 / 最佳实践）
- 审查范围：F005 全部 11 个改动文件（不含工作区内无关的 F006 字体设置改动）
- 审查基线：cargo test 189 通过 / vitest 78 通过 / F005 文件 tsc、eslint 干净
- 关联：[plan.md](./plan.md) · [F005 提案](../../pm/features/F005-repo-tab-drag-reorder.md)

## ✅ 优点

- **Migration 设计扎实**（`0003-repos-position.sql`）：`(added_at, id)` 回填 rank 子查询正确；apply() 单事务包住 DDL + 回填，原子生效；回填测试刻意覆盖插入顺序 ≠ added_at 顺序及 added_at 并列场景
- **`reorder_repos` 集合校验严密**（`workspace_repo.rs:258-283`）：长度相等 + 现有 id 全出现，在 id 为 PK 前提下严格等价于排列校验；校验在锁内、更新在事务内 + `affected == 0` 兜底自动回滚；配合 `AppContext.workspaces` 互斥锁，校验-写入无 TOCTOU 竞态
- **SQL 全参数化**，IPC 层无注入面；错误统一 `AppError::Protocol`
- **`add_repo` position 子查询正确**：VALUES 内子求值先于行插入，`COALESCE(-1)+1` 处理空表
- **拖动 hook 防御性好**：6px 阈值隔离点击/右键；rects/ids 数量护栏处理 refetch 竞态；`applyOrder` 对未知 id 追加末尾；被拖 id 消失时安全退出，最终由后端排列校验兜底——前后端防线闭环
- **suppressClickRef 生命周期正确**：清除延迟到 `setTimeout(0)`，覆盖 RAC press handler 与 window 监听的两种注册顺序
- **乐观更新符合 tanstack query 规范**（onMutate cancel+快照 / onError 回滚+ErrorAlert / onSettled invalidate）
- 纯函数抽离并单测；`position` 不泄露进 `RepoRef`，`ORDER BY position, added_at` 保留旧 tiebreaker

## 🔴 严重问题（必须修复）

无。

## 🟡 一般问题（建议修复）— 已全部处置

### 1. pointercancel 后 preview 不回滚 ✅ 已修复

- **位置**：`useTabDragReorder.ts` onCancel + `WorkspaceRepoTabs.tsx`
- **描述**：拖动中收到 pointercancel（macOS 触控板系统手势可触发）后 UI 停留在未提交的预览顺序，与 DB 不一致；后续 Move Left/Right 会以该 preview 为基准 commit，持久化未确认顺序
- **修复**：hook 新增 `onAbort` 回调（pointercancel 触发）；组件侧 `onAbort: () => setPreviewOrder(null)` 回退到存储顺序

### 2. missing tab 实际不可拖动，与 plan 承诺不符 ✅ 已澄清（文档修正）

- **位置**：`WorkspaceRepoTabs.tsx`；`plan.md`
- **描述**：HeroUI disabled 样式含 `pointer-events: none`，disabled tab 收不到 `pointerdown`，拖动路径对 missing tab 不成立；Move Left/Right 菜单路径可用
- **处置**：按审查建议采用文档修正方案——plan.md 明确"missing tab 仅支持菜单重排、不支持拖动"，消除"拖不动 = bug"的误解

### 3. 拖动状态机零自动化测试 ⚠️ 已记录，暂不补

- **位置**：`useTabDragReorder.test.ts`
- **描述**：3 个纯函数已覆盖，但状态机本身（阈值 engage、suppress 生命周期、pointercancel、to===from 不提交）无自动化用例
- **处置**：暂不补自动化测试——项目无 jsdom 依赖，且工作区正并行另一改动，不宜引入新 dev 依赖；已通过 🟡1/🟡4 修复缩小风险面，后续如引入 jsdom 可按审查建议补用例（press→move<6px 不 engage / engage 后 suppress=true / up 后异步归 false / pointercancel 触发 onAbort / rects 与 ids 数不一致跳帧）。以手动冒烟覆盖

### 4. 重复 pointerdown 无互斥 ✅ 已修复

- **位置**：`useTabDragReorder.ts` handlePointerDown 入口
- **描述**：拖动进行中第二个 pointerdown（多点触控）会覆盖 pressRef 并再挂一组 window 监听，两组 onMove 互相打架
- **修复**：入口处 `if (pressRef.current) return;`

## 🟢 优化建议（可选）— 处置记录

| 建议 | 处置 |
|---|---|
| 拖回原位松手跳过 commit | ✅ 已采纳：`PressState.originOrder` 比对，未变化走 `onAbort` 不发 IPC |
| plan 写 setPointerCapture 与实现（window 监听）不一致 | ✅ 已修正 plan.md 描述 |
| 菜单快速连点并发 mutation last-wins | ✅ 已采纳：Move Left/Right 在 `reorderMut.isPending` 时禁用 |
| commit 时先清 preview 可能闪一帧旧顺序 | ❌ 不采纳：onMutate 的 `setQueryData` 同步路径足够快，且保留 preview 至 onSettled 会与 onAbort 语义纠缠，复杂度大于收益 |
| 排列校验 O(n²) 改 HashSet | ❌ 不采纳：n 为 tab 数（<10），可读性优先 |
| `(workspace_id, position)` 唯一索引 | ❌ 暂不做：唯一性由唯一写入路径保证，排序有 added_at 兜底；如未来出现第二写入方再补 |

## 📝 总体评价

整体质量高：migration 回填、排列校验、乐观更新、拖动/点击隔离等易错点均做了正确防御，前后端测试全绿，无阻断性缺陷。本轮已修复全部 4 个 🟡 问题中的 3 个代码/文档问题并采纳 3 项 🟢 建议；遗留一项状态机自动化测试缺口（受 jsdom 依赖限制），由手动冒烟覆盖。

## 回归验证（修复后）

- `vitest`：78 通过（hook 纯函数 9 例含新增不变性）
- F005 文件 `tsc --noEmit`、ESLint：干净
- `cargo test`：189 通过（Rust 侧本轮无改动，复验确认）
