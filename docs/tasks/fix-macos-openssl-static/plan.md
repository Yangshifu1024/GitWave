# fix-macos-openssl-static

> 修复 v0.3.0 签名版 macOS 包启动即崩：动态链接 Homebrew OpenSSL。

## 现象

用户从 Release 下载 dmg 安装后启动即崩（SIGABRT）：

```
Library not loaded: /opt/homebrew/*/libssl.3.dylib
Reason: code signature ... not valid for use in process:
mapping process and mapped file (non-platform) have different Team IDs
```

## 根因

`git2` 的 `ssh` feature 引入 `libssh2-sys` → `openssl-sys`。`libgit2` 已 vendored，但 OpenSSL 没有：CI 的 macos runner 装有 Homebrew OpenSSL，构建时**动态链接**了 `/opt/homebrew/opt/openssl@3/lib/libssl.3.dylib`。用户机器上：

- 未装 Homebrew → 直接缺库崩
- 装了（如本机）→ hardened runtime 的 library validation 拒绝加载不同 Team ID 签名的 dylib → 崩

分发产物必须自包含，不允许依赖 CI runner 的 Homebrew 路径。

## 改动

`src-tauri/Cargo.toml`：`git2` 增加 `vendored-openssl` feature → `openssl-sys/vendored`（新增 `openssl-src` crate 到 lock），OpenSSL 从源码编译并静态链入二进制。

影响面：

- macOS：修复本 bug，`otool -L` 不再出现 `/opt/homebrew`
- Linux：同样变静态 → deb/rpm 不再依赖发行版 libssl3，可移植性更好；首次 CI 构建多 ~2-4 分钟（编译 OpenSSL），rust-cache 缓存后恢复
- Windows：openssl-sys 仅 cfg(unix) 生效，无影响

## 验证

本地 `cargo build --release` 后 `otool -L src-tauri/target/release/gitwave | grep -i ssl` 应无输出；CI 产物安装后正常启动。
