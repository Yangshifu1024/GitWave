# fix: re-enable git2 `https` feature dropped by 0.21 default-features change

## Symptom

macOS 打包产物里 HTTPS fetch 弹窗报错:

> 获取失败: there is no TLS stream available; class=Ssl (16)

## Root cause

- 升级提交 `9df55f4`(chore(deps): bump git2 from 0.20.4 to 0.21.0)只显式补回了
  `ssh` / `vendored-libgit2` / `vendored-openssl`,漏了 `https`。
- git2 `0.20.4` 的 `default = ["ssh", "https"]`;`0.21.0` 改为 `default = []`,
  `https` 与 `ssh` 均需显式声明。`ssh` 被补回了,`https` 被静默丢掉。
- libgit2-sys `0.18.8+1.9.7` 的 build.rs 只有在 `https` feature 下才生成
  `GIT_HTTPS` 与(macOS 的)`GIT_SECURE_TRANSPORT` 定义。缺失时 vendored
  libgit2 没有任何 TLS 传输,HTTPS fetch 即报 `class=Ssl (16)`。

## Fix

1. `src-tauri/Cargo.toml`:git2 features 增加 `"https"`,注释说明 0.21 起
   default features 为空。
2. CI 护栏:`.github/workflows/test.yml` rust-test job 在 `cargo test` 后
   grep 构建产物 `git2_features.h`,断言 `#define GIT_HTTPS 1`,防止下次
   依赖升级再次静默丢 TLS。

## Verification

- 本机 `cargo build` 后 `git2_features.h` 含 `#define GIT_HTTPS 1`
  (macOS 同时含 `#define GIT_SECURE_TRANSPORT 1`)。
- `cargo test --all-targets` 通过。
