# feat-wcbar-discard-ignore · unstaged 文件右键菜单 Discard / Ignore

> 状态：实施完成（待 code review）
> 需求（用户，2026-08-27）：提交框（WorkingCopyBar）unstaged 文件右键菜单增加 Discard 与 Ignore。
> 追加决策（用户确认）：Ignore 也弹窗确认，且提供三种忽略范围（全路径 / 所在目录 / 扩展名）；Ignore 写入仓库根 `.gitignore`。
> 分支：`feature/ui-native-studio-v2`

## 决策记录

| 决策点 | 结论 | 说明 |
|---|---|---|
| Ignore 写入目标 | repo 根 `.gitignore` | 主流客户端行为,随仓库共享 |
| Ignore 范围选择 | Modal 内 radio 三选一(File path / Directory / Extension) | 比子菜单多一次点击少一次弹窗;范围与确认合并为一次交互 |
| Ignore 可用性 | 仅 `kind === "untracked"` | `.gitignore` 不影响已跟踪文件,tracked 下提供是无效操作 → disabled + title 说明 |
| Discard 确认 | 自绘 Modal(danger 按钮) | WorkspaceList 删除先例;untracked 文案改为"永久删除文件" |
| Discard 对 renamed | 菜单项置灰 | rename 的 discard 语义复杂,v1 防误操作 |
| Discard 实现 | tracked=checkout_index(force,pathspec,update_index false);untracked=remove_file | ≡ `git restore <path>`;恢复 index 版本而非 HEAD,兼容部分暂存状态 |
| 批量操作 | v1 仅单文件 | 多选集合参与后续版本再说 |

## 改动清单

### Rust(src-tauri)
- `infrastructure/git/working_copy.rs`:新增 `discard_worktree_changes` / `ignore_path`(幂等 append;缺文件视为空)+ 5 个测试(modified 还原 / untracked 删除 / 删除态重建 / ignore 生效且 status 清空 / 幂等)
- `application/use_cases.rs`:use case `discard_changes` / `ignore_path`(active_repo_path + open_repo 范式);alias 导入
- `application/mod.rs` + `lib.rs`:re-export、import、`cmd_discard_changes` / `cmd_ignore_path` 定义 + generate_handler 注册(6 步接线链)

### 前端
- `src/lib/api.ts`:`discardChanges` / `ignorePath`
- `src/hooks/useWorkingCopy.ts`:`discard(paths)` / `ignore(pattern)` mutations(invalidate working-copy query;setActionError)
- `src/lib/ignorePattern.ts`:纯函数 `deriveIgnorePatterns(path)` → `{full, dir?, ext?}`(根目录无 dir、无扩展名/点文件无 ext)
- `src/components/ChangesPanel.tsx`:
  - FileSection props 增 `onDiscardFile/onIgnoreFile`,仅 Unstaged 实例传入;行外包 ContextMenu(asChild 不产生 DOM,listbox option 结构不变),radix trigger 与 row onClick 共存
  - 待确认动作 state `{type: "discard"|"ignore", file}`;`DiscardConfirmModal`(danger)/ `IgnoreScopeModal`(radio 三选一,展示 pattern code)

## 测试

- Rust:cargo test 111 通过(clippy 零警告),含新增 7 个(discard modified 还原 / untracked 删除 / 删除态重建 / 部分暂存恢复自 index / ignore 生效且 status 清空 / 幂等 / 拒绝逃逸路径);test_helpers 临时目录加进程内计数器根治并发撞名
- 前端:vitest 34 通过(含 deriveIgnorePatterns 4 个);mutation/UI 无 jsdom 设施不建自动化(项目现状盲区)

## 审查修复记录(code-reviewer 🔴/🟡)

- R5:`ignore_path` 读 `.gitignore` 仅 NotFound 视为空;非 UTF-8 等读取错误报错中止,不再静默清空重写整份文件
- S1:discard 前拒绝绝对路径与含 `..` 的段(`PathBuf::join` 绝对路径会替换 base;`starts_with` 为组件级比较不识别 `..`)
- D2:`status_file` 命中 `CONFLICTED` 直接拒绝 discard(冲突态 stage 0 缺失,防止误删盘)
- T2:补部分暂存回归(index v2 → worktree v3 → discard 得 v2 且 staged 条目保留)锁死 update_index(false)
- 其余:disable_pathspec_match(true) 防 glob 语义;`.gitignore` 自身在 UI 层禁用 Ignore
- 实现迭代记录:`Index::get_path` 对畸形路径会 panic 且 stage=-1 语义不符预期,tracked/untracked 判定改用 `repo.status_file()`

## 验收

- [x] cargo fmt/clippy/test 全绿;npm typecheck/test/lint/build/format 全绿
- [ ] 手动冒烟(dev):右键两项及置灰逻辑;Discard 确认后 modified 还原 / untracked 删除;Ignore 三种 pattern 写入 .gitignore 且刷新后消失;.gitignore 本身以 untracked 出现在列表属 Git 标准行为
- [x] code-reviewer 审查(review.md):1 项 🔴 与 🟡 建议已全部修复
