# review · fix-hide-origin-head

> 审查人：code-reviewer（2026-09-10）
> 改动：`src-tauri/src/infrastructure/git/history.rs`（`is_remote_head` + 三处过滤 + 测试）、`src-tauri/src/application/use_cases.rs`（palette 枚举过滤）、`docs/tasks/fix-hide-origin-head/plan.md`

## 结论：可合入（无 🔴；🟡 三条已随本 PR 落实）

三处远程分支枚举调用点全覆盖（list_branches / collect_commit_refs / AI palette），
`commit_log` 的 `branches(None)` 未过滤但 symref 已被 `target()==None` 天然跳过、
直连形态无可见影响（该 tip 即默认分支 tip，已被 origin/main 覆盖）。
按名查找路径（`resolve_ref_oid`、`find_branch`）正确地不过滤——显式引用
`origin/HEAD` 时仍可解析，与 checkout 防御（branch.rs）形成纵深。

## 审查要点核实

| 项 | 结论 |
|---|---|
| `ends_with("/HEAD")` 误伤真实分支 | 理论可能但可接受：git 保留 `refs/remotes/<remote>/HEAD` 给默认分支指针；裸 "HEAD" 被 `git branch` 拒绝；主流客户端同样隐藏。取舍已写入 rustdoc |
| 覆盖完整性 | 全量 grep `BranchType::Remote` 复核，见上 |
| use_cases.rs `matches!` 写法 | 正确，仅滤远程不误伤本地 `foo/HEAD` |
| collect_commit_refs 过滤位置 | `target()` 之前等价且更直白 |
| 安全 / 性能 | 无影响；顺带消除 symref 空 sha 垃圾行的开销 |

## 问题清单及处理

- 🟡 rustdoc 缺误伤取舍说明 → **已修**（`is_remote_head` doc 补 3 行）
- 🟡 测试断言用被测函数自证（`any(is_remote_head)`）→ **已修**（改字面量 `!names.contains(&"origin/HEAD")`）
- 🟡 直连 ref 形态未测 → **已修**（新增 `list_branches_hides_direct_ref_origin_head`）
- 🟢 `collect_commit_refs` 过滤后多余分号 → **已修**（与 list_branches 写法对齐）
- 🟢 `is_remote_head` 缺纯函数单测 → **已修**（新增 `is_remote_head_matches_default_branch_refs_only`）
- 🟢 plan.md "check-ref-format 拒绝裸 HEAD" 表述不精确 → **已修**（改为 `git branch` 分支名校验含 symref 歧义检查）
- 🟢 commit_log 直连形态可加注释防困惑 / 测试 cleanup 位置与兄弟测试不一致 → 未处理（前者无可见影响，后者当前写法失败时不泄漏临时目录反而更稳）

## 验证（落实 🟡 后复跑）

- [x] `cargo fmt` / `clippy --all-targets` 0 警告
- [x] `cargo test`：297 通过，0 失败（含新增 2 个测试）
