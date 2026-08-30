# fix-rebase-conflict-list · 修复方案（待排期）

> 状态：**待排期**。主缺陷（in-memory rebase 不落地 → 右键 Rebase 无声空转）已由
> `fix-pull-rebase-noop`（PR #4，d825a3a）修复；本任务只处理其遗留的次要缺陷。

## 现象

分支右键 "Rebase current onto this" 或 pull --rebase 命中冲突时，状态区提示
"hit conflicts" 但**不带冲突文件名**（`conflicts` 列表恒为空）。

## 根因

`src-tauri/src/infrastructure/git/rebase.rs` 的 `rebase_branch` 用
`opts.inmemory(true)` 做 in-memory rebase：冲突发生在 rebase 自己的内存 index
里，磁盘 index 从未被写入。而冲突路径却去读磁盘 index：

```rust
let idx = git2::Repository::open(wd)?.index()?;   // 磁盘 index，恒无冲突条目
let cit = idx.conflicts()?;
```

→ `conflicts()` 迭代不到任何条目，列表为空，前端只能显示泛化文案。

## 修复方向

冲突分支改用 `rebase.inmemory_index()`（git2 提供，返回当前内存 index），
从其 `conflicts()` 提取冲突路径；磁盘 index 路径删除。pull 路径
（`remote.rs::pull_integrate`）共用 `rebase_branch`，一并受益。

## 验证要点

- 构造必然冲突的 rebase（两分支改同一行），确认提示列出冲突文件名
- 既有 `rebase_*` / `pull_rebase_*` 回归测试不回归
