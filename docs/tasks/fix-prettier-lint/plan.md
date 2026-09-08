# fix-prettier-lint · main 分支 lint 失败（prettier 格式）

> 状态：已修复（待 review / 合入）
> 问题（用户，CI 报告）：GitHub 上 main 分支的 lint 失败。

## 根因

### 直接原因

`src/components/RefBadgeContextMenu.tsx:198-200`（合并后位置）的 `title={...}` 三元
表达式被手动换行为三行，而 prettier（printWidth 100）要求合并为单行（合并后约
94 字符，在 100 以内）。三个平台（macOS / Ubuntu / Windows）的 `Frontend lint →
prettier check` 均因该文件退出码 1 失败。

该文件由 `feature/history-ref-merge`（F011 merge-into-current）引入。

### 流程原因

`feature/history-ref-merge` **未走 GitHub PR 合并**——git 历史中是本地 merge 提交
（`Merge branch "feature/history-ref-merge"`），GitHub merged PR 列表中无该分支。
lint.yml 在 `pull_request` 上同样触发，但绕过 PR 直接 push main 后该检查从未运行，
格式问题直接进入 main。

同理，此前 main 的 test 失败（`pnpm/action-setup` 报 "No pnpm version is
specified"）也是 `fix/ci-pnpm-package-manager` 本地 merge 直接 push 的；该问题已由
packageManager 字段声明修复。

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| 修复方式 | 对该文件跑 `prettier --write` | 纯格式修复，无逻辑改动；diff 仅 3 行合并为 1 行 |
| 合入方式 | 新分支 `fix/prettier-lint` 走 PR | 恢复 GitHub Flow；PR 上 lint / test 会重新运行验证 |
| 不改 workflow | lint.yml 的 PR 触发已存在 | 问题不在 workflow 配置，在合入方式绕过了它 |

## 改动清单

- `src/components/RefBadgeContextMenu.tsx`：`title={...}` 三元表达式合并为单行
  （prettier --write 产物，无逻辑改动）
- 新增本 `docs/tasks/fix-prettier-lint/plan.md`

## 验证

- `pnpm exec prettier --check .` 通过（All matched files use Prettier code style!）
- `pnpm lint`（eslint）通过
- `pnpm typecheck`（tsc --noEmit）通过

## 附注：流程改进建议（不在本次处理）

- 建议 main 启用 branch protection 的 required status checks（lint / test），
  使本地 merge 后直接 push main 不可行，强制所有改动经 PR 检查后合入
- AGENTS.md 已约定「所有 PR squash merge 合入 main」，两次本地 merge 均未遵守
