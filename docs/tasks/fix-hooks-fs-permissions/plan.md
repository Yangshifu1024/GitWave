# fix: hooks.rs 使用不存在的 `std::fs::permissions` 导致 unix 构建失败

## 现象

macOS 上 `npm run tauri build` 在 Rust 编译阶段失败，前端 Vite 构建正常：

- `error[E0425]`：`src/infrastructure/git/hooks.rs:107` — cannot find function `permissions` in module `std::fs`
- `error[E0277]`：`src/infrastructure/git/hooks.rs:109` — `?` couldn't convert `std::io::Error` to `AppError`

Windows 构建不受影响。

## 根因

`write_hook` 的 `#[cfg(unix)]` 块内有两个错误：

1. `std::fs::permissions(&path)` 不存在——`permissions()` 是 `std::fs::Metadata` 的方法，不是 `std::fs` 模块函数
2. `std::fs::set_permissions(&path, perms)?` 产生 `std::io::Error`，而 `AppError`（`src/domain/error.rs`）只实现了 `From<git2::Error>`，`?` 无法自动转换

由于整段代码在 `#[cfg(unix)]` 条件编译块内，Windows 目标编译时该块被整体剔除，类型检查从未覆盖这段代码，错误一直潜伏；首次在 macOS/Linux 构建时暴露。

## 修复方案

用块内已导入的 `PermissionsExt::from_mode(0o755)` 直接构造权限，净效果与原代码（读权限 → `set_mode(0o755)` → 写回）一致；io 错误用与同函数 `std::fs::write` 相同的 `map_err(|e| AppError::Unknown(...))` 风格转换：

```rust
#[cfg(unix)]
{
    use std::os::unix::fs::PermissionsExt;
    std::fs::set_permissions(&path, std::fs::Permissions::from_mode(0o755))
        .map_err(|e| AppError::Unknown(format!("set hook permissions: {e}")))?;
}
```

仅改动 `src-tauri/src/infrastructure/git/hooks.rs` 一个文件；Windows 编译路径不受影响。

## 验证

- `cargo test hooks`：既有 3 个单测通过（list 全部 common hooks / write→read roundtrip 且 unix 下可执行位生效 / 路径穿越名称拒绝）
- `npm run tauri build` 全量跑通

## 回归要点

- 在 unix 上写入新 hook 后，文件权限应为 0o755（git 才会执行该 hook）
- hook 名称校验（路径穿越）行为不变
