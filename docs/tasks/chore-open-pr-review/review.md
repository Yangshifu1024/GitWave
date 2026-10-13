# chore-open-pr-review · Code Review

审查日期：2026-10-13（Asia/Shanghai）。依据 `.agents/agents/code-reviewer.md`，三组 reviewer 分别审查完整 PR diff、实际使用点、上游发布资料与包元数据；主代理负责组合验证、CI 和最终合并核验。

## ✅ 优点

12 个 PR 均未发现阻断性代码问题。依赖保持现有受支持的 major/minor，npm/Rust Tauri 配对正确；updater 安装恢复与 core macOS restart 修复互补。完整组合 frozen 安装、质量门禁和前端构建通过。

| PR | Reviewed head | Change | Result |
|---|---|---|---|
| [#150](https://github.com/Yangshifu1024/GitWave/pull/150) | `776516c0e4f398e228a287f88bc628d99b6bb549` | chore(deps): bump tauri-plugin-updater from 2.13.1 to 2.13.2 in /src-tauri | Passed |
| [#151](https://github.com/Yangshifu1024/GitWave/pull/151) | `16fb678b79e1e9218346f4553c862a56bd43c9d8` | chore(deps): bump react-i18next from 17.0.15 to 17.0.16 | Passed |
| [#152](https://github.com/Yangshifu1024/GitWave/pull/152) | `174c63667867d32faf8adb3c8b089f03911663d0` | chore(deps): bump react-aria-components from 1.21.1 to 1.22.0 | Passed |
| [#153](https://github.com/Yangshifu1024/GitWave/pull/153) | `de1377128dd74ebddd14c110f7c57ea19418c29d` | chore(deps): bump typescript-eslint from 8.70.1 to 8.71.1 | Passed |
| [#154](https://github.com/Yangshifu1024/GitWave/pull/154) | `f1ccd46ea8030a01df99691b8fd842274d8185ac` | chore(deps): bump @playwright/test from 1.63.0 to 1.64.0 | Passed |
| [#155](https://github.com/Yangshifu1024/GitWave/pull/155) | `93aba84e35c8bff94faa7f4f0a5be141e4ddd654` | chore(deps): bump @tanstack/react-query from 5.103.2 to 5.104.1 | Passed |
| [#156](https://github.com/Yangshifu1024/GitWave/pull/156) | `8546d9ff85111e5ff6759543fbb616c93e190046` | chore(deps): bump @tauri-apps/api from 2.12.0 to 2.12.1 | Passed |
| [#157](https://github.com/Yangshifu1024/GitWave/pull/157) | `09d22603f0b47f984852320c79edc6f71b90ae4d` | chore(deps): bump jsdom from 30.1.1 to 30.1.2 | Passed |
| [#158](https://github.com/Yangshifu1024/GitWave/pull/158) | `6074f9067113400d2bc2aeff63898e720c24b38f` | chore(deps): bump @tauri-apps/plugin-dialog from 2.8.0 to 2.8.1 | Passed |
| [#159](https://github.com/Yangshifu1024/GitWave/pull/159) | `d2bd70ed6606fe3e4794febc35e6ed6ed8751cef` | chore(deps): bump eslint from 10.11.0 to 10.12.0 | Passed |
| [#160](https://github.com/Yangshifu1024/GitWave/pull/160) | `70d846be694edefcea49575369ab9e0b0c32f9e2` | chore(deps): bump @vitejs/plugin-react from 6.1.1 to 6.1.2 | Passed |
| [#162](https://github.com/Yangshifu1024/GitWave/pull/162) | `59b367161ce1d3758b2a8c27681ed9e091fa0c0a` | fix(updater): restore app relaunch after updates on macOS | Passed |

## 🔴 严重问题（必须修复）

无。

## 🟡 一般问题（已处理）

- Dependabot PR 描述缺少任务计划链接：保留原正文并补上本任务 plan/review 链接和组合验证证据。
- #153 与 #159 同时修改 ESLint peer snapshots：保留 typescript-eslint 8.71.1 与 ESLint 10.12.0；全部 snapshots 引用完整且无重复 key。reviewer 独立核查原 PR 的包元数据与依赖未丢失，确认不残留 8.70.1 / 10.11.0。

## 七维结论

| 维度 | 结论 |
|---|---|
| 正确性 | 所有目标版本、调用点与 peer 范围一致；组合锁文件已独立复核。 |
| 安全性 | 依赖 integrity 对应官方元数据；不新增项目权限、数据发送或启动脚本。未执行全面 vulnerability advisory audit。 |
| 性能 | 未增加应用层额外网络请求或轮询；前端生产构建通过，未进行性能 benchmark。 |
| 可维护性 | 使用上游版本与生成的依赖图；保留各 PR 原历史，用 merge commit 合并。 |
| 可读性 | 变更主要为版本与 lockfile，restart 状态和重试职责明确。 |
| 测试覆盖 | 组合 337 前端、385 Rust 与 macOS 真实 restart 测试通过；无 Playwright E2E 套件。 |
| 最佳实践 | 冻结安装、完整门禁、精确 head 的三平台 CI 和任务链接；冲突解决单独复核。 |

## 🟢 优化建议与边界

下次维护运行说明时，可把已有 jsdom 要求的 Node 22.22.2 最低 patch 写清楚；这不是本轮升级引入的回归。RAC submenu / Select 的完整浏览器交互与签名安装包更新流程未实测，沿用明确的发布验收边界。

## 验证证据

- 每个原 PR 的 12 项三平台 lint/test 检查均 SUCCESS。
- 组合 `pnpm install --frozen-lockfile`、`make check`、`pnpm build` 全通过；Node 4 tests、Vitest 53 files / 337 tests、Rust 385 tests + macOS restart 1 test passed，2 个既有 Rust tests ignored。
- reviewer 独立检查上游包、peer/engine/integrity、原始及最终 lock 图，并执行 restart 相关 14 个前端测试和真实 macOS bundle test。
- 最终组合 `pnpm-lock.yaml` SHA-256：`aec07ccffd441a711e03f76d8fbd4661f044a6ac1f3f4fb175fdfe96a130f516`；最终 merge tree 必须与通过验证的候选代码文件逐一比对。

## 📝 总体评价

批准这 12 个 PR 与所列锁文件冲突解决。主代理在发布各 merge commit 时核实 PR head、CI 和 tree，并在合并后核对 GitHub 状态与 main CI；本报告不把局部/组合验证扩展为真实签名包更新验收。

## 分组审查详情


# Runtime dependency PR review

Date: 2026-10-13 (Asia/Shanghai)

Reviewer role: `.agents/agents/code-reviewer.md`. This review is read-only with respect to product source and Git state. The main agent owns combined quality gates, PR metadata, and merge decisions.

| PR | Reviewed head | Upgrade | Code verdict |
| --- | --- | --- | --- |
| #151 | `16fb678b79e1e9218346f4553c862a56bd43c9d8` | react-i18next 17.0.15 → 17.0.16 | No blocking code finding |
| #152 | `174c63667867d32faf8adb3c8b089f03911663d0` | react-aria-components 1.21.1 → 1.22.0 | No blocking code finding; overlay interaction validation boundary below |
| #155 | `93aba84e35c8bff94faa7f4f0a5be141e4ddd654` | @tanstack/react-query / query-core 5.103.2 → 5.104.1 | No blocking code finding |

All three snapshots have base `11c8e5c86c49fb4cbf5a64b3142f0d90af00f979` and only modify dependency manifests/lockfiles. Review snapshots were supplied in `/private/tmp/gitwave-open-prs-20261013/{151,152,155}.{diff,json}`.

## ✅ 优点

- #151: The official [17.0.16 changelog](https://raw.githubusercontent.com/i18next/react-i18next/v17.0.16/CHANGELOG.md) describes a secondary-namespace selector fix in `Trans`. A direct comparison of the published 17.0.15/17.0.16 npm tarballs confirmed that the changed JavaScript modules are `Trans` and `TransWithoutContext`. GitWave uses string keys through `useTranslation`, merges resources into a single `translation` namespace (`src/i18n/resources.ts:1`), and has no `Trans` or selector call site. Its current path is therefore outside the changed behavior. i18next 26.4.2, React 19.3.0, and TypeScript 6.0.3 satisfy the new package's peer ranges.
- #152: The package and transitive changes match Adobe's [v1.22.0 release](https://react-aria.adobe.com/releases/v1-22-0): react-aria 3.53.0, react-stately 3.51.0, @react-types/shared 3.37.0, and @internationalized/number 3.6.9. HeroUI 3.2.6 accepts RAC `^1.21.1` and react-aria `^3.52.1`; React/React DOM 19.3.0 also satisfy the published peer ranges. The lockfile routes both the direct `SubmenuTrigger` import and HeroUI through the same RAC 1.22.0 instance, preserving shared contexts.
- #155: The upstream [react-query changelog at the release commit](https://raw.githubusercontent.com/TanStack/query/29859ae60c8dca0a5cdbf8abccc775b655cf43e2/packages/react-query/CHANGELOG.md) and [query-core changelog](https://raw.githubusercontent.com/TanStack/query/29859ae60c8dca0a5cdbf8abccc775b655cf43e2/packages/query-core/CHANGELOG.md) describe a codemod packaging fix, Vite 8 build migration, and aligned dependency releases. A direct comparison of published source in both old/new npm tarballs found comments, formatting, and a local `result` → `resultRef` rename, with no apparent execution-semantic change. React Query retains React `^18 || ^19`; the repository uses React 19 and Vite 8.
- All four directly reviewed new package integrity fields (RAC, react-i18next, react-query, query-core) match live npm registry metadata and the PR lockfile entries. This verifies registry/lockfile agreement; it is not a full dependency security audit.

## 🔴 严重问题（必须修复）

No critical code issue found in these three PR snapshots.

## 🟡 一般问题（建议修复）

- **位置**: PR #151, #152, and #155 descriptions.
  **描述**: The supplied Dependabot bodies do not link a proposal or `docs/tasks/<task-name>/plan.md`, which is required by the repository's AGENTS.md and CONTRIBUTING.md.
  **建议**: Add the actual plan link for this dependency-review task to each PR body before merging. This is a repository-process requirement, not a runtime code defect.

## 🟢 优化建议（可选）

- **位置**: #152, `src/components/ui/ContextMenu.tsx:195` and `src/components/ui/Select.tsx:49`.
  **描述**: Adobe [PR #10685](https://github.com/adobe/react-spectrum/pull/10685) clears `PopoverContext` within a popover to prevent inherited trigger/ARIA metadata leaking into an independent nested popover. GitWave's submenu establishes its own `SubmenuTrigger`, so static inspection finds no dependency on the removed parent context. However, the existing ContextMenu guard test checks source shape rather than actual submenu opening/focus behavior; a pure typecheck cannot prove this interaction.
  **建议**: Smoke-check a context submenu with pointer and ArrowRight/Escape, along with Select within Settings and modal close/reopen. Existing `src/components/ui/Modal.test.tsx` exercises actual HeroUI/RAC dialog behavior and should be included in the main agent's combined test run. This is an explicitly unverified interaction boundary, not an observed regression.

## 七维审查

| Dimension | Result |
| --- | --- |
| Correctness | Manifest/lock changes and peer ranges are consistent. #151 changed path is unused. #152 context change is compatible by static inspection. #155 published source comparison found no semantic change. |
| Security | No application permission, IPC, network, or privacy behavior is modified by these diffs. New package identities and lock integrity values match npm registry metadata. A vulnerability audit of all transitive dependencies was not performed by this reviewer. |
| Performance | No added app-level loops, requests, or cache policies. New Sheet exports are unused; no project import was added. Bundle size and browser performance remain outside this review's execution scope. |
| Maintainability | Upgrades stay in the current supported major versions and keep HeroUI/RAC peer resolution aligned. #152 retains some older react-aria/react-stately versions for unrelated consumers; this was already possible in the graph and is not evidence of a context split for HeroUI/SubmenuTrigger. |
| Readability | Changes are confined to generated lock metadata and dependency version fields; no readability regression in product code. |
| Test coverage | Existing real QueryClient tests cover sidebar queries, diff requests, repo tabs, hooks, and AI mutations. Modal has a real HeroUI reopen test. Many application tests mock i18n/UI wrappers, so they cannot substantiate full language/menu interaction coverage. No tests were executed by this reviewer. |
| Best practices | Published peer ranges, package exports, and lock integrity are retained. Require frozen lock installation, combined CI-equivalent gates, and current PR checks before merging. Add the required task links. |

## 验证边界与组合风险

**Verified in this review**: read all three complete diffs; inspected project dependency usages and actual Modal test; read official upstream release material; fetched versioned npm package metadata; compared published react-i18next/query-core/react-query source from old/new tarballs; inspected Adobe PR #10685's source/test patch through the GitHub API.

**Static conclusions**: no blocking code finding, peer compatibility, shared RAC instance for HeroUI/direct submenu use, no apparent semantic changes in Query source, and no direct impact from the react-i18next Trans change.

**Not executed here**: installation, `make check`, bundled frontend build, browser/WebView smoke tests, macOS/Windows/Linux runtime, and remote CI. The main agent runs the combined checks and must record those results separately. Do not present this report alone as proof of those passes.

The three upgrades do not depend on each other beyond shared React peers and use disjoint package entries. Their individual package.json edits sit close together, and #152 changes long HeroUI peer snapshot keys, so merging other dependency PRs can cause textual lockfile conflicts. Resolve with the declared package versions intact, run a frozen install, inspect the regenerated dependency graph, and validate the combined revision. No execution-level cross-upgrade incompatibility was found.

## 📝 总体评价

Approve the code changes of #151, #152, and #155, conditioned on the main agent confirming the combined quality gates and each PR's required checks. Add a task-plan link to each description before merge. RAC overlay interactions carry the explicitly stated browser/WebView validation boundary; no regression was observed or asserted by this static review.


# Tooling dependency review

Reviewed 2026-10-13 under `.agents/agents/code-reviewer.md`. Review is read-only; no branch, source, remote, or GitHub comment was changed.

## ✅ 优点

- All five PRs contain only the stated direct development dependency upgrade and its lockfile changes. Exact reviewed heads are listed below.
- The resolved ESLint, TypeScript, parser, plugin, Vite, and React plugin versions satisfy the relevant published peer ranges.
- Each original PR lockfile was parsed as pnpm 12's multiple YAML documents. Every snapshot dependency reference resolved, every snapshot had a matching package entry, and all newly introduced package integrities matched the official npm registry. The 26 new package-version entries also satisfy local Node 24.21.0.

| PR | Reviewed head | Scope and compatibility | Conclusion |
| --- | --- | --- | --- |
| #153 | `de1377128dd74ebddd14c110f7c57ea19418c29d` | typescript-eslint 8.70.1 → 8.71.1, with all 11 package-family entries upgraded consistently. Published peers support ESLint `^8.57.0 || ^9.0.0 || ^10.0.0` and TypeScript `>=4.8.4 <6.1.0`; therefore ESLint 10.11.0/10.12.0 and TypeScript 6.0.3 are supported. Existing flat config uses recommended type-checked rules and an explicit TS project. Upstream fixes include promise checks, merged-declaration unused-variable handling, and type traversal bounds. | No blocking issue found; approve subject to final combination lint/test gates. |
| #154 | `f1ccd46ea8030a01df99691b8fd842274d8185ac` | Playwright 1.63.0 → 1.64.0; test/core/runtime packages upgrade together. Node minimum stays 20. Release behavior changes concern screen emulation, JSX compilation, snapshot update behavior, and hidden iframes. No Playwright imports, config, or E2E suite exists in this repository, so those changes affect no current test scenario. | No blocking issue found; approval covers dependency compatibility, not E2E execution. |
| #157 | `09d22603f0b47f984852320c79edc6f71b90ae4d` | jsdom 30.1.1 → 30.1.2, plus CSS and URL dependency updates. Vitest optional-peer snapshot follows the new jsdom version. New data-urls/whatwg-url/tr46 minimums are less restrictive than jsdom's existing Node requirement, which remains `^22.22.2 || ^24.15.0 || >=26.0.0`. Upstream corrects DOM/CSS performance regressions, computed-style invalidation, focus behavior, and leaks. The repository has 19 tests explicitly selecting jsdom. | No blocking issue found; require the final Vitest run for behavioral coverage. |
| #159 | `d2bd70ed6606fe3e4794febc35e6ed6ed8751cef` | ESLint 10.11.0 → 10.12.0. All peer snapshot references move consistently. `@eslint/js` 10.0.1 and react-hooks 7.1.1 both allow ESLint 10, as does typescript-eslint 8.70.1/8.71.1. Node range is `^20.19.0 || ^22.13.0 || >=24`; no config migration is introduced. Release changes add APIs and fix rule false positives. | No blocking issue found; require lint against the combined resolved graph. |
| #160 | `70d846be694edefcea49575369ab9e0b0c32f9e2` | plugin-react 6.1.1 → 6.1.2. Vite peer remains `^8.0.0`, satisfied by Vite 8.3.1. `oxc-transform-react` peer becomes `^0.152.0`, but remains optional and only matters when enabling the compiler; current `vite.config.ts` uses plain `react()`. HMR compound-component handling improves. | No blocking issue found; require frontend build on the combined dependency graph. |

## 🔴 严重问题（必须修复）

None found in the reviewed diffs. This conclusion is static and metadata-based; it does not replace the final installation, quality gates, or three-platform CI.

## 🟡 一般问题（建议修复）

- **Location:** PR descriptions #153/#154/#157/#159/#160.
  **Description:** Saved Dependabot descriptions contain no project proposal/task link. `AGENTS.md` requires every PR to reference a proposal or task, so descriptions should link the shared dependency-review plan before merging.
  **Recommendation:** Add the parent task's `docs/tasks/chore-.../plan.md` and relevant validation evidence to each PR description.
- **Location:** `package.json`, `docs/tech/engineering/00-overview.md`, CI Node selection.
  **Description:** Development documentation says Node 22 without its minimum patch version, while jsdom already needs Node 22.22.2. This is an existing baseline condition, not a regression caused by #157. CI's `node-version: 22` selects the current Node 22 release; verify its resolved patch from CI rather than treating every Node 22 version as supported.
  **Recommendation:** When next maintaining runtime guidance, document the actual engine floor. Local validation here used supported Node 24.21.0.

## 🟢 优化建议（可选）

- The already-documented absence of Playwright E2E means #154 should not be presented as end-to-end coverage. A future E2E task can introduce configuration, browser installation, and real scenarios independently.

## 📝 总体评价

Correctness and best practices were checked through actual configuration, lockfile graph integrity, and official peer/engine metadata; no incompatibility was identified. Security review found no new custom scripts or remote transmission path in these diffs, and verified package integrity, but did not perform a comprehensive advisory audit; performance improvements are upstream claims, not repository benchmarks. Maintainability/readability are appropriate for generated dependency changes, and current unit/component tests cover the active tools; the parent agent must still run the resolved combination's frozen installation, full `make check`, frontend build, and inspect CI before merging.

### Primary sources

- [typescript-eslint 8.71.1 release](https://github.com/typescript-eslint/typescript-eslint/releases/tag/v8.71.1) and [published package source](https://raw.githubusercontent.com/typescript-eslint/typescript-eslint/v8.71.1/packages/typescript-eslint/package.json).
- [Playwright 1.64.0 release](https://github.com/microsoft/playwright/releases/tag/v1.64.0) and [published package source](https://raw.githubusercontent.com/microsoft/playwright/v1.64.0/packages/playwright-test/package.json).
- [jsdom 30.1.2 release](https://github.com/jsdom/jsdom/releases/tag/v30.1.2) and [published package source](https://raw.githubusercontent.com/jsdom/jsdom/v30.1.2/package.json).
- [ESLint 10.12.0 release](https://github.com/eslint/eslint/releases/tag/v10.12.0) and [published package source](https://raw.githubusercontent.com/eslint/eslint/v10.12.0/package.json).
- [plugin-react 6.1.2 release](https://github.com/vitejs/vite-plugin-react/releases/tag/plugin-react%406.1.2) and [published package source](https://raw.githubusercontent.com/vitejs/vite-plugin-react/plugin-react%406.1.2/packages/plugin-react/package.json).
- Integrity and engine metadata independently checked against version-specific resources at `https://registry.npmjs.org/<package>/<version>`.

### Supplemental review: final combined lockfile

Read-only review of `/private/tmp/gitwave-open-prs-20261013/candidate/pnpm-lock.yaml`, SHA-256 `aec07ccffd441a711e03f76d8fbd4661f044a6ac1f3f4fb175fdfe96a130f516`.

- Both pnpm YAML documents parse without duplicate-key errors. They contain 9 package-manager package/snapshot entries and 503 application dependency package/snapshot entries. Every importer reference and every dependency/optional-dependency snapshot reference resolves; every snapshot has its matching package entry.
- The complete typescript-eslint family contains exactly 11 package entries, all at 8.71.1; their metadata matches the reviewed #153 head exactly. All 11 corresponding snapshots match #153 with only their ESLint 10.11.0 peer references upgraded to 10.12.0.
- Exactly one ESLint package entry exists, at 10.12.0. Its metadata and dependency snapshot match the reviewed #159 head exactly. All 8 snapshots containing an ESLint peer, and all explicit ESLint dependency edges, resolve to 10.12.0.
- No `10.11.0` or `8.70.1` text remains anywhere in the combined lockfile; no old ESLint/typescript-eslint version is retained as a duplicate or orphan.
- The parent agent reports that the exact combined candidate passed frozen installation, `make check`, and `pnpm build`, and that #162 has all 12 CI checks green. Those commands were not rerun by this read-only reviewer.

**Conclusion:** the #153/#159 conflict resolution is correct and approved. A normal merge commit containing current main and the original #153 head preserves the original PR ancestry; before publishing it, compare its resolved lockfile and complete tree with the tested candidate. This review confirms the candidate content, not a future commit or remote update.


# Open PR review — Tauri dependencies and macOS updater restart

审查日期：2026-10-13。依据 `.agents/agents/code-reviewer.md`，本次只读审查源码、锁文件、官方发行包和上游实现；没有修改产品代码、分支或远程，也没有发表 GitHub 评论。

| PR | 审查 head | 改动 | 代码结论 |
|---|---|---|---|
| #150 | `776516c0e4f398e228a287f88bc628d99b6bb549` | Rust `tauri-plugin-updater` 2.13.1 → 2.13.2 | 通过；须核对正式 CI |
| #156 | `8546d9ff85111e5ff6759543fbb616c93e190046` | npm `@tauri-apps/api` 2.12.0 → 2.12.1 | 通过；须核对正式 CI |
| #158 | `6074f9067113400d2bc2aeff63898e720c24b38f` | npm `@tauri-apps/plugin-dialog` 2.8.0 → 2.8.1 | 通过；须核对正式 CI |
| #162 | `59b367161ce1d3758b2a8c27681ed9e091fa0c0a` | Tauri 2.12.2 与更新后 restart 反馈、重试和回归测试 | 通过；须核对正式 CI |

原始 PR diff 与元数据来自 `/private/tmp/gitwave-open-prs-20261013/{150,156,158,162}.{diff,json}`。源码为 `/Users/yangzhenbiao/Works/Private/GitWave`，当前 head 核实为 #162 的 head。组合验证 worktree 当时仍在由主代理处理 lockfile 冲突，因此本报告不把其中的冲突文件当作已完成的组合结果；正式组合门禁和三平台 CI 由主代理执行及核对。

## ✅ 优点

### #150 — Rust updater 2.13.2

| 维度 | 审查与证据 |
|---|---|
| 正确性 | PR 仅更新 updater 的版本、checksum，并增加既有 `libc` 依赖。核查本机 registry 的 2.13.1/2.13.2 实现：macOS 安装在同卷暂存，支持 `renamex_np(RENAME_SWAP)` 原子交换；不支持交换时执行带恢复的 rename，并在恢复失败时保留旧 bundle，避免安装失败删除现有应用。 |
| 安全性 | 签名验证仍先于安装；新增 macOS 路径经 `CString` 处理，`unsafe` 调用有明确生命周期说明。提权安装命令分别使用 shell 和 AppleScript 字符串引用；本项目的 updater 签名公钥、HTTPS endpoint 和 capability 均未扩张。 |
| 性能 | 新增操作发生在安装阶段，无日常刷新或渲染成本；原子交换替代原来的逐步移动，提高安装期间路径稳定性。 |
| 可维护性 | 属于上游 patch，未引入项目私有 installer。`reqwest` 仍为 0.13，本项目 `reqwest-updater-unification` 的 `system-proxy` feature 合并继续成立。 |
| 可读性 | 仅有锁文件变化；新增依赖与上游 macOS bundle swap 实现对应，范围可解释。 |
| 测试覆盖 | 上游源码包含 bundle 替换、失败恢复、保留旧 bundle 与引用处理测试；项目未另加模拟安装测试。组合本地门禁和本 PR 正式 CI 仍须由主代理确认。 |
| 最佳实践 | npm updater 已锁定 2.13.2；Rust 升到同版本，且其 Tauri 依赖仍为 2.12。采用框架提供的安装与签名机制。 |

上游依据：[updater 2.13.2 release](https://github.com/tauri-apps/plugins-workspace/releases/tag/updater-v2.13.2)、[macOS 安装修复提交](https://github.com/tauri-apps/plugins-workspace/commit/2a9c29e004325db674fd904089749c38f4aac824)。具体实现以本机 `tauri-plugin-updater-2.13.2/src/updater.rs:1390`、`:1542`、`:1562`、`:1588`、`:1616` 及 `Cargo.toml:106` 为复核证据。

### #156 — npm API 2.12.1

| 维度 | 审查与证据 |
|---|---|
| 正确性 | importer、package resolution、snapshot 和五个 plugin 的 API 引用一致更新到 2.12.1。直接下载 npm 官方 2.12.0/2.12.1 包并比较 62 个 JS/typing 文件：仅 `webview.d.ts`、`window.d.ts` 的 transparency 文档注释变化，运行时 JS 没有变化。 |
| 安全性 | 新 integrity 与 npm registry 官方值完全一致；未增加网络目标、权限或运行时依赖。此结论不等同于对全部既有依赖完成漏洞审计。 |
| 性能 | 运行时 JS 未变，无新增开销。 |
| 可维护性 | 保持单一 API resolved version，五个 plugin 共享 2.12.1；没有遗留 2.12.0 snapshot 引用。 |
| 可读性 | 锁文件修改集中且一致，未修改项目业务代码。 |
| 测试覆盖 | 本次静态对照运行时源码与版本检查；全部前端、组合门禁和正式 CI 由主代理确认。 |
| 最佳实践 | npm API 2.12.1 与 Rust Tauri 2.12.1/#162 的 2.12.2 同 major/minor；允许 patch 不同，符合项目版本门禁。 |

上游依据：[API 2.12.1 release](https://github.com/tauri-apps/tauri/releases/tag/@tauri-apps%2Fapi-v2.12.1)、[npm 官方元数据](https://registry.npmjs.org/@tauri-apps%2fapi/2.12.1)。核对的 integrity 为 `sha512-DeyFHa3wynpyoqTDikDEDGTJIq4LQ5USfolQGRmGIWT6JMADyxZBTDa5cAdT3tDg73rUXufPaCwN7aXBos4OnQ==`。

### #158 — npm dialog 2.8.1

| 维度 | 审查与证据 |
|---|---|
| 正确性 | importer、package resolution 和 snapshot 一致改为 2.8.1；官方包仍要求 `@tauri-apps/api ^2.12.0`，兼容 #156 的 2.12.1。对比 2.8.0/2.8.1 官方 tarball 的四个 dist 文件，内容完全相同。 |
| 安全性 | integrity 与官方 registry 相同，未扩张 dialog capability，也没有更改路径选择或授权行为。 |
| 性能 | runtime dist 完全相同，无新增开销。 |
| 可维护性 | Rust dialog 2.8.1 与 JS 2.8.1 同 major/minor；不需要为了 patch 对齐而额外升级 Rust 插件。 |
| 可读性 | 四处锁文件差异直接对应同一版本升级。 |
| 测试覆盖 | 用官方 package 比对确认本次无 JS runtime 改动；前端组合和三平台 CI 仍由主代理核对。 |
| 最佳实践 | 保留现有官方 dialog API 和权限，版本范围、snapshot 与 paired crate 的 minor 均吻合。 |

上游依据：[dialog-js 2.8.1 release](https://github.com/tauri-apps/plugins-workspace/releases/tag/dialog-js-v2.8.1)、[npm 官方元数据](https://registry.npmjs.org/@tauri-apps%2fplugin-dialog/2.8.1)。核对的 integrity 为 `sha512-/DtE3B62JgpYuWFL5ATXM6KOXUq7pca7cJWOgSqULhZsgyjLk0PWKek1dbRDvhNBW6flNsOfWphsKnb+738Dmw==`。

### #162 — macOS restart 修复

| 维度 | 审查与证据 |
|---|---|
| 正确性 | `useRestartApp` 经共享 store 的 `beginRestart` 拒绝未安装或重复请求；失败调用 `finishRestart`，保留 `ready`、目标版本和 notes，并重新打开 modal。成功安装的 retry 不会重新下载安装。Tauri 最低版本提升到包含官方 macOS LaunchServices restart 修复的 2.12.2。 |
| 安全性 | JS → `plugin:process\|restart` → Rust `request_restart` 的既有路径不变；main window 已有 `process:allow-restart`。未新建 shell 启动器或开放新权限。真实测试只复制测试可执行文件到临时 bundle，不运行或重启已安装 GitWave。 |
| 性能 | 新增的 `restarting` 状态仅在用户点击时变动；共享 guard 防止并行 restart 请求，无后台定时器或常态性能影响。 |
| 可维护性 | 根因修复交由 Tauri 官方进程实现；项目增加的状态动作职责单一，重启错误与下载错误保持各自的重试路径。 |
| 可读性 | 状态名、hook 与按钮标签清晰；英中提示均齐全，错误元素提供 `role=alert`。任务 plan/review 说明了旧客户端首次升级和签名验收边界。 |
| 测试覆盖 | 本次独立运行三个前端文件：14 个测试通过。另执行真实 macOS `cargo test --locked --offline --test updater_restart`：1 个测试通过，0.17 秒。fixture 验证旧进程退出后新实例能够写 stdio、拥有独立 process group 且参数保留。 |
| 最佳实践 | Rust Tauri 2.12.2 与 npm API 2.12.0/#156 的 2.12.1 同 major/minor；官方 restart 路径和 capability 成立，最低版本约束防止重新解析到旧实现。 |

项目证据：`src/hooks/useUpdater.ts:166`、`src/stores/updaterStore.ts:91`、`src/components/UpdateModal.tsx:88`、`src-tauri/Cargo.toml:27`、`src-tauri/src/lib.rs:2018`、`src-tauri/capabilities/default.json:14`、`src-tauri/tests/updater_restart.rs`。上游依据：[Tauri 2.12.2 release](https://tauri.app/release/tauri/all-versions/#2122)。本机 registry 中 `tauri-2.12.2/src/process.rs:92` 使用 `/usr/bin/open -n`，fallback 将 stdio 设为 null 并建立新的 process group。

## 🔴 严重问题（必须修复）

未发现可复现的产品代码严重问题或这四个 PR 之间的版本兼容阻塞。

## 🟡 一般问题（建议修复）

- **位置**：Dependabot PR #150、#156、#158 的 description。
- **描述**：原始 metadata 中未见本仓库 task/proposal 链接，尚未满足 `AGENTS.md` / `CONTRIBUTING.md` 的 PR 描述约定。这是交付资料问题，不是依赖代码缺陷。
- **建议**：由主代理在合并前补上本次依赖审查/处理的共享任务 plan 链接，并核实三平台正式 CI 与合并策略；reviewer 没有自行改写远程内容。

## 🟢 优化建议（可选）

- 下次发布验收用签名 GitWave 安装包执行下载 → 安装 → restart 完整流程，并包含有空格的安装路径、同卷/其他卷和需要提权的安装位置。当前隔离 bundle 测试证明进程 restart 机制，不证明签名安装包的完整 updater 链路。
- 旧客户端升级至修复版时第一次 restart 仍由旧进程执行，可能需要手动打开一次；这一点已在 #162 描述及 plan 中说明。

## 📝 总体评价

四个 PR 的改动范围明确，npm/Rust 目标版本兼容，updater 的安装恢复与 Tauri 的重启修复互补；未发现需要源码修复后才能合并的问题。最终组合锁文件已由本 reviewer 复核，主代理已报告组合 `make check` / build 通过和 #162 正式 CI 全绿；可在补齐任务引用及其余 PR 正式 CI 通过后按 merge commit 合并。未把源码对比、局部测试或隔离 fixture 结论扩展为生产签名包更新验收。

### 本次独立验证记录

- 四个精确 base/head 的 `git diff --check`：全部 exit 0。
- `node node_modules/vitest/vitest.mjs run src/hooks/useUpdater.test.tsx src/components/UpdateModal.test.tsx src/stores/updaterStore.test.ts`：3 files / 14 tests passed。
- `cargo test --locked --offline --test updater_restart`：sandbox 中 `/bin/ps` 被系统拒绝，fixture 首实例退出 101；只读 `/bin/ps -o pgid= -p 1` 同样确认 operation not permitted。受控 sandbox escalation 后原命令通过，1 passed / 0.17 秒；没有改动测试来规避断言。
- 调用项目 `findVersionProblems` 核查目标版本集合（API 2.12.1、dialog 2.8.1、opener 2.7.0、os/process 2.4.0、updater 2.13.2）与 #162 的 Rust lock：返回 `[]`。#150 将 Rust updater 2.13.1 提升到 2.13.2，同 minor 关系保持。
- 官方 npm 元数据与两项 PR 的完整 integrity 一致；API 的 62 个 JS/typing 文件只有两项注释变化，dialog 的四个 dist 文件无变化。
- 本 reviewer 没有重复执行全量 `make check`，没有代替主代理核查正式 CI。初次审查时组合文件仍有冲突；收到主代理完成通知后，已独立完成下列最终锁文件复核。

### 最终组合与冲突解决复核

- `/Users/yangzhenbiao/.codex/worktrees/open-pr-review/GitWave/pnpm-lock.yaml` 与 `/private/tmp/gitwave-open-prs-20261013/candidate/pnpm-lock.yaml` 逐字节相同。SHA-256：`aec07ccffd441a711e03f76d8fbd4661f044a6ac1f3f4fb175fdfe96a130f516`。无 conflict markers，API 和 dialog 的 package resolution / snapshot 分别只出现 2.12.1 与 2.8.1。
- 最终 Rust resolved versions：Tauri 2.12.2、dialog 2.8.1、process 2.4.0、updater 2.13.2。对应已安装 npm：API 2.12.1、dialog 2.8.1、process 2.4.0、updater 2.13.2。Rust dialog 的另一项升级也已包含在最终组合中。
- 在组合 worktree 独立运行 `node scripts/check-tauri-versions.mjs`：`Tauri npm/Rust versions match.`；`git diff --check`：exit 0。
- 主代理提供的全量验证证据：12 PR 组合通过 frozen-lock 安装、`make check`（337 前端、385 Rust、真实 restart 1；既有 ignored 2）和 `pnpm build`；#162 的全部 12 项 CI 检查全绿。这些为主代理执行结果，本 reviewer 仅独立核验本报告列明的局部测试和最终锁文件。
- 主代理拟为 #153/#159 的锁文件冲突采用该已验证候选文件，同时保留 merge commit 的两个 parent；其余 11 项按正常 GitHub merge commit 合入，不重写 PR 历史。就本 reviewer 负责的 Tauri 版本和 restart 组合而言，没有发现这一冲突解决方案带来的兼容问题；实际 commit parent 和其余依赖的语义由主代理及对应 reviewer 核验。

## 最终 PR 刷新复核

其它 11 个 PR 合并后，Dependabot 自动将 #153 更新为 `78a8099ec3315d38ffadd974855b4f9fb4d3a755`，基于最新 main。新 lockfile 额外改动了 7 个 React Aria / Spectrum 依赖引用；本任务追加提交将锁文件恢复为上述已通过完整验证的候选内容，使最终 PR 相对 main 仅保留原定 typescript-eslint 升级与本轮处理报告。全部 17 个候选产品/依赖文件逐项核对，除任务目录计数因新增报告按实更新外，内容一致；不以旧 PR head 的 CI 代替最新 head 的检查。
