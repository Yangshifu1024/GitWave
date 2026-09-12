# fix: 前端同步槽 / 切换 / 更新检查竞态与展示层失真

状态：待实现（Batch 3，全量修复 4 批次之三）

分支：`fix/frontend-sync-races`（从 `main` 切出，squash merge；3.1 依赖后端改动，与 Batch 2 的
`SyncProgress` 改动无文件冲突，可并行，合入时注意 `remote.rs` / `lib.rs` 先后顺序）

## 背景

前端 `src/lib` + `src/stores` 审查无崩溃级硬伤，风险集中在三类竞态
（syncStore 按操作名匹配槽位、workspaceStore 跨区残留、updater 双检查乱序回写），
另有 `useRemoteSync` 监听失败静默、ConflictPanel 切换覆盖、DiffViewer 陈旧 diff 等展示层问题。

## 修复步骤

### 3.1 `syncStore` 按请求实例匹配（跨层，本批最先）
- 后端 `infrastructure/git/remote.rs:34-39 SyncProgress` 加 `pub request_id: String`；
  `attach_transfer_progress:138-144` 构造带入；fetch/pull/push 签名加参；
  `lib.rs:1293/1342/1412` 三处 emit 闭包 `move` 捕获。旧字段全保留。
- 前端 `api.ts:355 SyncProgress` 加 `requestId`；`syncStore.ts:81-118`：
  `startOp(op, remote?, requestId?)` 存 `activeRequestId`，`updateProgress` 首行 `requestId`
  不符直接丢弃，`endOp` 按 id 匹配。
- 测试：`syncStore.test.ts` 加交错单测（push 进行中插入 delete 事件 → push 槽位不受影响）；
  后端 remote.rs 所有构造 `SyncProgress` 处编译影响一次修完。

### 3.2 / 3.3 / 3.4 标准竞态修复
- `WorkspaceRepoTabs.tsx:114 activateRepo` 加 `myWs/myRepo` 序号守卫（仿 154-174 `staleId` 对比）；
  守卫抽纯函数单测（组件测试太重不做）。
- `updaterStore.ts` 加 `checkEpoch`，`useUpdater.ts:70 checkForUpdate` await 后比对 epoch 再写状态；
  `fail/markReady` 顺手清理残留字段（见 3.6）。
- `useRemoteSync.ts:42-63`：双通道任一失败即回滚已挂通道并指数退避重试（1s 起，上限 30s）；
  注册以模块级 promise 去重（StrictMode 双挂载/并发挂载共享一次注册），
  存 `UnlistenFn` 并导出 `teardownSyncProgressListener` 供 unmount/HMR 清理。

### 3.5 展示层失真
- `ConflictPanel.tsx:66 openFile` 加 `seqRef` 守卫（请求序号，不符丢弃；面板 open 变化时自增失效旧请求）。
- `DiffViewer.tsx:359 signature` 加入 `${additions}:${deletions}` + `workingCopy.sha` 前缀
  （`api.ts:966-985 FileChange/WorkingCopy` 已有字段，零后端改动）。
- `InteractiveRebaseDialog` / `PrDescriptionModal` 加 open 会话序号守卫（仿 ConflictPanel）。

### 3.6 🟢 前端项打包（不单独 PR，随本批）
- localStorage 写 try/catch（`usePalette.ts:22`、`useTheme.ts:65`）。
- `SshKeyManager.tsx`：`deleteMut` 加 `onError` 提示；startAgent 2s 轮询卸载时清理。
- `useTabDragReorder.ts:167-175` 卸载清理命令式 window 监听。
- `lib/ignorePattern.ts` 空路径早退；`lib/diff.ts:36` 反斜杠仅 Windows 替换；
  `lib/conflictMarkers.ts` `{7}` 锚尾 + `lineStartOffset` 缓存 split；
  `lib/fonts.ts:50` 加 U+2028/29；`api.ts:147 formatAppError` 白名单过滤 params key；
  `lib/commitMenu.ts` time 非法防御；`authPromptStore` 卸载时 cancel 等待者。
- `lib/branchNames.ts` 保持（注释已声明 display/best-effort，后端兜底）。

## 验证

- `pnpm typecheck` + `pnpm lint` + `pnpm test`（新增交错/序号单测全过）
- `cd src-tauri && cargo test --all-targets`（3.1 后端改动编译 + 单测）
- 手动：push 与删远端分支交错 → 进度条不串台；工作区切换中完成 repo 激活 → 无跨区错乱；
  更新检查双触发 → 结果不回退；冲突文件快切 → 内容不错位；外部改文件 → diff 自动刷新

## 实施偏差（以实现为准）

- 3.1 改为**前端生成 id 全链路透传**（starter 在 startOp 与 invoke options 用同一 id，
  经 cmd → use_cases → remote 打到事件；lib.rs 保留服务端 fallback 生成）。
  原因：纯后端打标 + 前端 adoption 在"新 op 已 startOp 但首个事件未到"窗口会被旧实例
  反杀（auto-refresh fetch 与手动 push 交错即触发）；starter-known id 无此窗口。
  触及签名：`remote::fetch/pull_with_options/push_with_options`、
  `use_cases::fetch/pull/push`、`cmd_fetch/pull/push` += request id；
  `delete_remote_branch` 无事件，不动。
- `formatAppError` 由 allowlist 改为** blocklist**：保留键名本身是字母数字，
  allowlist 拦不住 `keySeparator`（已由新增单测证实）；`count` 保留给复数模板，
  且数值一并透传（复数规则需要 number）。
- 双 Modal 会话守卫对缺失 context **fail-closed**（`!context ||` 短路：context 缺失时放弃写入，
  不再等价旧行为的放行）；`nextSyncRequestId` 以 wall clock + 随机盐为种子防 HMR 复用。
- `ConflictPanel` 面板 open 变化（关闭）时自增 `seqRef` 失效在途请求，与 plan 3.5 原设计一致。
- `diff.ts` 用 `platform.ts isWindows()`（`process` 在 webview 不可用）；
  slash 测试按 UA 分测 Win/POSIX。
- `CommitExplainModal` 顺手加同款 session 守卫（与 PrDescriptionModal 同形）。
- `conflictMarkers` 锚定改为 `7 + (空白|行尾)`（`<<<<<<< HEAD` 仍命中，
  `>>>>>>>>>>` 不再误报）；`lineStartOffsets` 一次预计算，调用方改用。
- 未动项：`branchNames`（已有 best-effort 注释）、`palette/document` 守卫、
  `BlameView`/`CommitGraph` 性能（备查）、`useMacTitlebarWindow`（理论问题）。
