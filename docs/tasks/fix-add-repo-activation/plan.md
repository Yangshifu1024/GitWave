# fix-add-repo-activation · 添加仓库后跳到第一个 tab

> 状态：已修复（待冒烟 / review）
> 需求（用户，bug 报告）：在工作区中添加新的仓库后，没有切换到该仓库，而是切换到了第一个 tab。

## 根因

不是「添加后忘记切换」，而是**切换了又被抢回去**。

`src/components/WorkspaceRepoTabs.tsx` 的存活守卫 effect（原 174 行）有一条规则：

> `activeRepoId` 不在当前 `repos` 列表中 → 判定该选择已失效 → 回落到 `pickRestoredRepo(repos, null)`，
> 即第一个非 missing 的仓库。

而「添加一个仓库」实际是两个互相独立的异步往返：

| 往返 | 代价 | 典型落地顺序 |
|---|---|---|
| `setActiveRepo`（把选择移到新仓库） | 一次 sqlite 写入 | **先** |
| `listRepos`（刷新 `["repos", workspaceId]` 缓存） | 需逐个 stat 仓库路径 | 后 |

选择先落地时，缓存列表里还没有新仓库，守卫就把「刚设好的选择」误判成「已删除 / 失效」，
于是切回第一个 tab。等 `listRepos` 返回时，选择早已被改掉，就停在第一个了。

`WorktreePanel` 的 worktree 切换路径更严重：它 `setActiveRepoId` **先于** `invalidateQueries`，
那个窗口是必然触发的，不依赖时序运气。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 修复方式 | 新增 `upsertRepos` + `useSeedRepos`，在移动选择**之前**把新仓库写进查询缓存 | 让列表不再落后于选择，从根上消掉误判窗口；顺带新 tab 立即渲染，不用等 refetch |
| 是否改守卫本身 | 不改 | 守卫的职责（仓库消失 / 被删 / 持久化 id 失效时自动切走）是对的，误判源于输入数据陈旧，不是规则错了 |
| 是否改用 `isFetching` 跳过 | 否 | `WorktreePanel` 那条路径在选择落地时 `isFetching` 仍是 false，纯靠「正在拉取」判断会漏 |
| 批量添加语义 | seed 全部 `added`，激活最后一个 | 与既有「激活最后添加的那个」契约一致；新 tab 也全部立即可见 |
| refetch 是否还跑 | 照旧跑 | seed 只保证不落后；排序与字段仍以后端返回为准 |
| 单测 | 纯函数单测 + 组件级回归测试 | 回归测试已验证：去掉 seed 后两条立即失败（复现原 bug），而「missing 仓库仍要切走」那条照样通过 |

## 改动清单

- `src/lib/repoSelection.ts`：新增纯函数 `upsertRepos`（已存在则就地替换以保持 tab 位置，新的追加到末尾）
- `src/hooks/useSeedRepos.ts`：新增 hook，把新仓库 seed 进 `["repos", workspaceId]` 缓存
- `src/components/ActionBar.tsx`：init / clone / 批量本地添加三条路径在 `activateRepo` 前 seed
- `src/components/WorktreePanel.tsx`：worktree 切换路径补 seed（该路径顺序原本是反的）
- `src/lib/repoSelection.test.ts`、`src/components/WorkspaceRepoTabs.test.tsx`：新增测试

## 测试

- `make check` 全绿（prettier + eslint + clippy + tsc + 前端测试 + Rust 385 tests）
- 手动冒烟要点：
  - workspace 内已有仓库时添加一个新仓库 → 激活新 tab（不是第一个）
  - 空 workspace 添加第一个仓库 → 正确激活
  - 一次批量添加多个仓库 → 激活最后一个，全部 tab 可见
  - worktree 面板切换到另一个 worktree → 激活该 worktree
  - 删掉 / 移走一个仓库后重启或运行中消失 → 仍自动切到可用仓库（守卫未被削弱）
