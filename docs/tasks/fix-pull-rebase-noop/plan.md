# fix-pull-rebase-noop · 修复方案

## 现象

Pull 弹框勾选 "Rebase instead of merge" 后点 Pull：状态区显示成功
（"Pulled from origin"），但本地分支没有任何变化——远端新提交没有过来。

## 根因（tester 分析）

`src-tauri/src/infrastructure/git/rebase.rs` 的 `rebase_branch` 是 **in-memory
rebase**（`opts.inmemory(true)`，注释明言 "Don't touch the workdir"）：重写出的
提交只存在于对象库中，**分支 ref、HEAD、index、工作区都由调用方负责落地**，
落地凭据就是返回值里的 `new_head`。

而它的两个调用方都没落地：

1. **Pull 的 rebase 分支**（`remote.rs::pull_integrate`，本缺陷）：拿到
   `RebaseResult` 后只判断 `Conflicts`，`Clean`/`new_head` 直接丢弃并
   `return Ok(())` → 成功提示 + 无声空转。
2. `rebase_branch` 内部对"本地严格落后"（`ahead == 0`）返回
   `AlreadyUpToDate`，但该场景正确语义是 fast-forward——pull 路径里
   rebase 检查在 FF 检查**之前**，把最常见的"落后"场景也变成空转。

次要问题：`new_head` 全仓零消费方（grep 实证），Branches 面板的独立
Rebase（`use_cases::rebase_branch`）同样不落 ref，是同一根因的另一处表症。

## 修复方案

1. **`rebase.rs` 新增 `finalize_rebase(repo, oid)`**：把当前分支 ref 指到
   `oid` 并 `set_head` + force checkout（与 pull 的 FF 块同一套落地动作）。
2. **`remote.rs::pull_integrate` 重排**：
   - FF 判断提前到 rebase 之前（`git pull --rebase` 在可 FF 时就是 FF）；
   - rebase 分支：先查 `worktree_is_dirty`（脏工作区直接报 Protocol 错，
     引导勾选 "Stash and reapply" 或先提交——真 git 语义，防止 force
     checkout 吞掉未提交改动）；`Clean` 时用 `finalize_rebase` 落地；
     `Conflicts`/`AlreadyUpToDate` 维持原语义。
   - 勾选 stash 时 `pull_with_options` 顶部已先 stash，走到这里工作区必干净，
     守卫不误伤。
3. **`use_cases.rs::rebase_branch`（独立 Rebase）**：同样的脏区守卫 +
   `Clean` 时 `finalize_rebase`，让 Branches 面板的 Rebase 真正生效。
4. **回归测试**：local-path remote 真实 fetch/pull——落后 FF、分叉重写、
   脏区拒绝三类；`finalize_rebase` 移 ref 单测。

## 分支建议

缺陷是存量问题（与 feature/heroui-modal-style 的样式改动无关），推荐独立
`fix/pull-rebase-noop` 分支提交；因改动全在 Rust 端、与工作树中未提交的
Modal 样式改动互不相交，先在当前树上实现，由用户决定提交去向。

## 审查与验证

code-reviewer 审查抓到一个 🔴 并已修复：初版把 FF 块排到脏区守卫之前，
`rebase=true + 脏工作区 + 可 FF` 会 force checkout 吞掉未提交改动——
守卫已提到 FF 之前，并对齐真 git 的前置拒绝语义。🟡 两条也已修：
`use_cases` 改用 `clone()` 保住 `RebaseResult.new_head` 契约（Clean 必带值）；
plain pull 的 FF 路径补上同款脏区守卫（原实现 force checkout 同样可能
吞改动，属顺手加固，**是有意的行为变化**：以前"能拉"的脏区场景现在会
报错并引导勾选 Stash）。

最终行为：

- 工作区干净：落后 → FF；分叉 + Rebase → 重写落地；分叉无 Rebase →
  报冲突引导；已最新 → 成功早退
- 工作区脏（无论 FF 可否）：一律报 Protocol 错，提示勾选
  "Stash and reapply" 或先提交；勾选 stash 后自动 stash → 操作 → 还原
- Conflicts：in-memory rebase abort，本地提交原样保留（原语义不变）

验证：`make check` 全绿；196 个 Rust 测试通过，含 7 个新回归测试
（落后 FF、分叉重写、脏区拒绝 ×3、stash 流程、finalize 移 ref）。

遗留（🟢，非阻塞）：in-memory rebase 冲突时从磁盘 index 取冲突文件列表
恒为空（in-memory 模式磁盘 index 无冲突项），文案仍正确但文件名缺失；
`worktree_is_dirty` 对纯 untracked 也判脏（比 git 严，方向保守安全）。

## 提交归属

3 个 Rust 文件与 feature/heroui-modal-style 的样式改动无关，建议按
`fix/pull-rebase-noop` 独立提交（与 src/ 下样式改动拆开）。

## 真机验收清单

- [ ] 勾选 Rebase 后 Pull：本地分支实际前进（FF）或重写（分叉），状态区
      成功提示与实际一致
- [ ] 有未提交改动且未勾选 Stash 时 Pull：报错提示干净工作区，本地改动
      原样保留
- [ ] 勾选 Stash and reapply + Rebase：改动自动还原
- [ ] Branches 面板独立 Rebase：现在会真正移动分支（此前也是无声空转）
