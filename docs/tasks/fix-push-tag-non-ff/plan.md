# fix-push-tag-non-ff · 推送 tag 报 NotFastForward 且整批失败

> 状态：已实现（待冒烟）
> 现象：推送 tag 失败，报 libgit2 原文「cannot push non-fastforwardable
> reference; class=Reference (4); code=NotFastForward (-11)」。

## 根因（实测比对本地/远端 refs 修正）

用户推 v0.7.2 时**远端并没有 v0.7.2**——真正被拒的是同批次的旧 tag：
本地 `v0.6.2` → `28e309f`，而远端 `v0.6.2` → `4f47971`（另有笔误 tag
`0.6.2` → `4f47971`）。「推送所有标签」把全部本地 tag 打包进一次 push，
tag 更新天然不可 fast-forward（任何指向不同对象的同名 ref 都被 libgit2
拒绝，git CLI 同样要求 --force），**一个冲突 tag 使整批原子失败**，
无辜的 v0.7.2 被连坐，且错误不指名是哪个 tag。

## 改动

1. **逐 tag 容错**（`src-tauri/src/infrastructure/git/remote.rs`
   `push_with_options`，返回值 `Result<()>` → `Result<PushOutcome>`）：
   - 快路径不变：branch + tags 整批一次推（unchanged tag 是远端 no-op，
     常见场景仍是单次网络操作）；
   - 撞到 `ErrorCode::NotFastForward` 且 tags 启用时拆分重试：先单推分支
     （分支被拒属真失败，直接报错——本地落后/历史不一致），再整批推 tag，
     仍失败则**逐 tag 单推**：成功的照常送达，NotFastForward 的记入
     `PushOutcome.skipped_tags` 点名返回；
   - 认证/取消/超时等其余错误照旧立即中止；多次尝试复用同一进度回调
     （`attach_transfer_progress` 改借用法）与凭证生命周期（approve/reject
     与 run_with_credentials 语义一致）。
2. **可操作错误码**：分支非快进拒绝映射 `git.push_non_fast_forward`
   （文案改为分支口径：先拉取同步，或勾选「强制推送」），不再透出 libgit2
   原文；`git.push_failed` 兜底不变。
3. **force 覆盖 tag**：tag refspec 在 `opts.force` 时加 `+` 前缀（与
   `git push --force` 一致），作为冲突 tag 的显式覆盖路径。
4. **前端**：`pushRemote` 返回 `PushSummary { skippedTags }`；
   `useRemoteSync` 在有跳过时状态区显示 info 级
   `status.sync.pushedSkipped`（点名 tag + 提示强制推送可覆盖），无跳过
   仍走 `status.sync.pushed` 成功文案。BranchList 的分支直推忽略 summary，
   行为不变。

## 第二轮修订（用户实测 + 拍板）

第一轮实测暴露两个后续问题，据此转向：

1. **重试梯子撞上凭证 gate 一次性语义**：`GitCredentialHelper` 的
   `FillOnce` 在同一次操作内只允许一次 helper 查询，第二次 take 返回
   `AlreadyQueried`/`Empty` → 重试梯子第二次网络尝试直接报「推送认证失败：
   no credentials available」。修复：`take()` 改为**答案重放**——helper 一旦
   给出答案，该答案就是本次操作的凭证（后续 401 回合、重试梯子都复用，
   不再触发 helper 二次查询/弹窗）；helper 未给出答案时维持原 fail-fast。
   既有测试 `fill_once_queries_helper_only_on_first_take` 断言同步更新。
2. **tag 推送语义改为「只推当前提交上的 tag」**（用户拍板，替代全量）：
   `push_with_options` 的 `opts.tags` 不再遍历全部 `refs/tags/*`，只收集
   peeled 后指向被推分支 tip commit 的 tag（release 流程：给发布提交打
   tag、随分支一起推）。从源头消除「一个旧 tag 冲突连坐整批」；
   拆分重试 + skippedTags 报告保留作为兜底（如重复发同名版）。
   前端勾选框文案改为「推送此提交上的标签」并显示 tip 上的 tag 名
   （`pushCommitTags(WithTip)`，移除 `pushAllTags(WithTip)`），无 tip tag
   时置灰。

## 第三轮修订（凭证误删事故 + 加固）

用户实测第二版仍报「no credentials available」——排查确认：第一版重试梯子
对 gate 自生成的假 Auth 错误（`AlreadyQueried`）调用了 `creds.reject()` →
`git credential reject` **把 osxkeychain 里 github.com 的存储凭证抹掉了**
（`security find-internet-password -s github.com` 确认条目已消失），此后
每次 fill 均为空。属本分支中间版本造成的数据损失，用户需在终端重新输入
一次 GitHub 凭证恢复 keychain。

加固：`push_once` 增加 `allow_reject` 参数——仅第一次尝试允许 reject
（凭证此时尚未证明有效）；重试尝试一律不 reject（走到重试说明凭证已通过
协商认证，后续 401 视为瞬态，绝不能抹掉系统凭证存储）。

## 已知边界

- 分支被拒时 tags 不再尝试（分支是推送主体，错误先解决；避免半交付状态
  混淆）。tag 优先的场景可稍后单独推。
- 冲突 tag 的覆盖是整批 force（git `-f --tags` 同语义）；单个 tag 的外科
  手术式恢复 = 删除远端同名 tag 后重推。

## 测试

- `cargo fmt --check` / `cargo test`（246）全绿；`npm test`（144，含 i18n
  parity）/ `typecheck` / `prettier` 全绿。
- 手工冒烟（`npm run tauri dev`，GitWave 仓库自吃狗粮）：
  - 推送所有标签（本地存在 v0.6.2 ≠ 远端）：状态区显示「已推送到 origin；
    已跳过与远端不一致的标签：v0.6.2（…）」info 文案，v0.7.2 实际到达远端
    （`git ls-remote --tags origin` 验证）；
  - 勾选「强制推送」重推：v0.6.2 被覆盖为本地指向，无跳过提示；
  - 正常分支 push、新 tag 推送：行为不变。
