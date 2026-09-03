# fix-git-fetch-system-proxy · 修复方案

## 现象

F013（系统代理，v0.7.4 合入）上线后，设置「跟随系统 / 手动」代理时 AI 请求与更新检查已走代理，但 fetch / pull / push / clone 仍直连——系统配置代理（如 Clash「系统代理」模式）且无环境变量的用户，git 网络操作全部超时失败。

## 根因

F013 的 env 桥接（`infrastructure/proxy.rs` 把解析后的代理写入进程
`HTTP_PROXY` / `HTTPS_PROXY` / `NO_PROXY`）依赖 libgit2 的 config → env
回退链。但应用侧所有 libgit2 网络操作都使用 git2-rs 默认 options：

- git2-rs 的 `FetchOptions` / `PushOptions` 在序列化为 raw options
  （`Binding::raw()`）时，对未设置的 proxy 取 `Default::default()`，其中
  `git_proxy_options.kind = GIT_PROXY_NONE`
- libgit2 对 `GIT_PROXY_NONE` 的语义是「**不使用任何代理，忽略一切配置与
  env**」——回退链根本不执行
- 仅当 `GIT_PROXY_AUTO` 时 libgit2 才按 git CLI 语义解析：`http.proxy` /
  `remote.<name>.proxy` config 优先，回退 `HTTPS_PROXY` / `https_proxy` /
  `HTTP_PROXY` / `http_proxy` env，`NO_PROXY` / `no_proxy` 豁免单主机

因此 F013 实际只覆盖了 AI 请求（reqwest 0.12 单例）与更新检查（updater
插件的 reqwest 0.13）两条路径；fetch / pull / push / clone / submodule
五类操作从未生效。`feat-system-proxy` review 遗留的真机验收（Clash 下
fetch 走代理）正是该缺陷的直接暴露。

## 修复

一处 helper + 五处接线（均在 `infrastructure/git/`）：

1. `remote.rs` 新增 `attach_auto_proxy`：经本地 trait
   `ProxyOptionsCarrier`（`FetchOptions` / `PushOptions` 各自实现，0.20
   的两个 builder API 无公共 trait）给 options 挂上
   `ProxyOptions::new().auto()`（即 `GIT_PROXY_AUTO`），恢复 git CLI 代理
   语义。挂接点：
   - `fetch`（fetch / pull 共用）
   - `push_with_options`（含 non-fast-forward 隔离重试闭包）
   - `delete_remote_branch`
2. `repo_adapter.rs::clone_with_creds`（clone HTTPS / SSH 共用）
3. `submodule.rs`：`update_submodule` 改用 `opts.fetch(fo)`（注入代理的
   同时保留原 `allow_fetch(true)` 语义）与 `add_submodule` 的 `sm.clone`

`NO_PROXY` 由 F013 桥接保证恒含 `localhost` / `127.0.0.1` / `::1`，本地
Ollama 等 LAN 服务不受影响；三档设置（跟随系统 / 手动 / 关闭）对 git
路径的行为与 git CLI 一致。

## 已知限制（沿袭 F013）

- SOCKS-only 系统代理不应用于 git 路径（libgit2 无 SOCKS 传输）
- Windows `ProxyOverride` 的 IP 通配（`192.168.*` 等）被丢弃（libgit2 与
  hyper-util 的 no_proxy 均不支持），HTTP(S) 局域网 git 服务器会被代理
- 用户 git config 中的 `http.proxy` 优先于应用注入的 env（git 语义：
  config > env），属预期行为

## 验证

- `cargo fmt` + `cargo clippy --all-targets -- -D warnings`：零警告
- `cargo test --all-targets`：277 passed / 2 ignored（本地 fetch / push /
  clone / submodule 集成测试全部走新代码路径，无回归）
- `tsc --noEmit` 零错误；`vitest run` 148 passed（前端零改动）
- 真机（Windows + Clash 系统代理）：fetch / push 应在代理工具连接日志
  可见——`feat-system-proxy` 遗留验收项一并补上，需用户执行

## 分支

`fix/git-fetch-system-proxy`（基于 main `29e9bad`）。
