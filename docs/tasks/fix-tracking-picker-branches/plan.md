# fix: tracking 弹框下拉缺失常见远程分支

## 缺陷

设置跟踪分支（Set tracking branch）弹框的下拉菜单中不出现 `origin/main`、`origin/master` 等常见远程分支，导致无法为无 upstream 的分支设置最常用的 tracking 目标。

## 根因

`src/components/BranchList.tsx` 的 tracking `<Select>` 复用了侧边栏的去重远程分支列表：`visibleBranches = filterRemoteBranches(branches)` → `remoteBranches`。该去重（短名与本地分支重名的 remote 分支被隐藏）是侧边栏展示语义（feat-sidebar-branch-dedupe），被误复用到选择器——但 `refs/remotes/origin/main` 真实存在，本应可选。

## 修复

- `src/lib/branchNames.ts`：新增纯函数 `allRemoteBranches` —— 全量 `kind === "remote"` 分支，仅排除短名精确等于 `HEAD` 的符号引用（`origin/HEAD`；git 禁止分支名为 `HEAD`，嵌套的 `feat/HEAD` 不受影响）。
- `src/components/BranchList.tsx`：tracking 弹框 options 改用 `allRemoteBranches(branches)`；侧边栏 `remoteGroups` / `renderGroup` / 选中恢复判定仍用去重列表，行为零变化。
- `src/lib/branchNames.test.ts`：补 5 个回归用例（本地 `main` 存在时仍列出 `origin/main`、多 remote 同短名保留、`origin/HEAD` 排除、嵌套 `feat/HEAD` 不误伤、纯本地仓库返回空）。

## 验证

- `pnpm test`：180 passed（branchNames 22）；`pnpm typecheck` / `pnpm lint` / `pnpm format:check` 全绿。
- code-reviewer 审查通过（无 🔴；🟡-1 测试加固建议已采纳）。
- 手工回归点：弹框下拉可选 `origin/main` 并成功设置/清除 upstream；侧边栏本地 `main` 旁仍不显示重复的 `origin/main`。

## 分支与提交

- 分支 `fix/tracking-picker-branches`（基线 main@25afe02）
- commit：`fix(branches): list all remote refs in the tracking picker`
