# fix: AI 隐私面收敛与后端稳定性加固

状态：待实现（Batch 2，全量修复 4 批次之二）

分支：`fix/ai-privacy-stability`（从 `main` 切出，squash merge）

## 背景

源码审查结论：无命令注入/RCE（git 全走 libgit2 argv，凭据走 stdin），风险集中在
**密钥出网面**（scrubber 覆盖不足、base_url 不强制 https、错误体落日志）与
**panic 链**（static RwLock 内 `.expect`、22 处 mutex `.expect("poisoned")`），
另有凭据单键冲突、clone 先删后验、同步锁缺口、进程组残留等问题。

## 修复步骤

### 2.1 scrubber 扩展（隐私最高优）
- `ai/scrubber.rs:4-27` 新增：`AKIA/ASIA[0-9A-Z]{16}`、`AIza[0-9A-Za-z_-]{35}`、`xox[baprs]-`、
  `glpat-`、`github_pat_/gho_/ghu_`、`npm_`、`Bearer\s+\S+`、`(?i)token\s*[:=]\s*\S+`、
  `aws_secret/private_key`；`ghp_/sk-` 改大小写不敏感。
- 密钥片段用正则替换为 `[REDACTED]`（保留行结构）；整行关键字（password/secret 等）维持整行丢弃。
- 测试：每类密钥一行 `+` diff 行断言脱敏 + 正常代码行保留（8+ 例）。

### 2.2 base_url 强制 https + 错误体不进日志
- `openai/anthropic_endpoint`（`provider.rs:57-77`）入口 `Url::parse` + scheme 校验
  （`localhost` / `127.0.0.1` 例外），非法 → `Protocol(UNSUPPORTED_PROVIDER)`（码复用）；Ollama 不动。
- `anthropic_no_text_error`（358-380）message 只留 `stop_reason` + 长度，去掉 `content[0..200]`；
  `http_error` detail 截断 200 字符（`use_cases.rs:785` 的 `warn!(error=%e)` 只打 message，修 message 即止血）。
- 测试：`http://` 被拒；`https://` 与 localhost 放行。

### 2.3 `client()` 去 panic + 含 key 结构体脱敏 Debug
- `ai/provider.rs:93-113`：`client() -> Result<Arc<Client>>`，3 处 `expect` →
  锁用 `PoisonError::into_inner`、build 失败用 `network_with`；新增 `error_codes/infra.rs` 码
  `ai.http.client_build` + 前端 errors json；调用方 `:138/:211` 加 `?`。
- `AiGenerateRequest` / `ProviderAttempt`（11/25）/ `ResolvedAiProvider`（`use_cases.rs:614`）
  手写 `Debug`，key 字段打 `***`（仿 `credentials.rs:497-503`）。
- 测试：坏 proxy env 下 `client()` 返回 `Err` 不 panic（全局单例污染：测后调
  `rebuild_http_client` 恢复；串行跑并文档注明）。

### 2.4 重试语义修正（顺手，`http_error:43-55` 一处）
- 400/422 → `Protocol`（不重试）；429/5xx → `Network`（保留一次重试）；60s 超时不动（文档化）。
- 测试：映射单测 5 例。

### 2.5 Linux 文件权限 + 日志保留期
- `persistence/sqlite.rs:42-59` 建库后 `#[cfg(unix)] chmod 0600`（含 `-wal` / `-shm`）；
  `observability/tracing.rs:25` 建目录后启动清理 `mtime > 30 天` 的 `app.*` 日志 + 日志文件 0600；
  全用 `cfg(unix)` gate，Windows 原路径。
- 测试：unix 单测断言权限位；保留期逻辑抽纯函数单测。

### 2.6 keyring features + 凭据多账号（默认自动迁移；不同意可砍，退为废弃旧键）
- `Cargo.toml:50`：`default-features = false`，按 `cfg(macos)` / `cfg(windows)` /
  `cfg(all(unix, not(macos)))` 分平台 `apple-native` / `windows-native` / `sync-secret-service`
  （仿 reqwest:54）。
- `credentials.rs:172-189` vault 键改为 `host + '\x1f' + user`（小写）；读：新键 miss → 读旧 host 键 →
  命中则写新键删旧键；修 IPv6 `[...]` 解析。
- 测试：同 host 双账号共存；旧键迁移一次；`ssh://git@[::1]:2222/x` 解析正确。

### 2.7 clone 先验后删 + 同步锁统一 + poison 风格统一
- `use_cases.rs:409-430`：dest 存在且非空、且不是失败 clone 残留（无有效 `.git`）→
  新错 `codes::git::CLONE_DEST_NOT_EMPTY`（+ 前端文案 + 确认框，二次确认后重调 `replace_dest=true`）。
- `pull:2493` / `push:2527` / 删远端分支入口复用 `workspace_fetch_lock(ws)`（2408-2418；fetch 保持）。
- 全部 `.expect("poisoned")`（use_cases 22 处 + `credentials.rs:207` + provider 遗留）→
  `unwrap_or_else(PoisonError::into_inner)`（仿 2413/2429）。
- 测试：非空 dest 被拒；残留 `.git` 可清；锁互斥单测。

### 2.8 URL userinfo strip + ssh 加固 + 冲突恢复出口
- 新增 `strip_url_credentials()`，进 `AppError` message/params 前调用
 （sweep `repo_adapter.rs:113/120/125`、credentials.rs、`submodule.rs:65`）。
- `test_connection`（`ssh/keys.rs:283-322`）：复用 `process.rs wait_with_output_timeout` 加超时；
  host/user 首字符 `-` 拒绝；`ssh-add -l` 改 `-L` 解析真实路径。
- `revert/cherry-pick` 冲突分支：`reset(Hard)` 回 HEAD + `cleanup_state`（模块头注释同步更新）。
- `rebase.rs:119-133` 冲突改读 `Rebase::inmemory_index`（修"冲突路径恒空"）。
- `abort_merge` 补清 `MERGE_RR` / `SQUASH_MSG` / `ORIG_HEAD`。
- 测试：`https://user:token@host` 不进错误串；`-oProxyCommand` 式 host 被拒；rebase 冲突路径非空。

### 2.9 `process.rs` 大输出死锁 + 进程组 kill
- `wait_with_output_timeout`：wait 前起双线程排空 stdout/stderr；超时 kill 改进程组
 （Unix `setsid` + `killpg`，Windows `taskkill /T /F /PID`）。
- 测试：大输出 fixture（20 万行）无假超时；超时后无残留进程。

## 验证

- `cd src-tauri && cargo test --all-targets` + `cargo clippy` + `cargo fmt --check`
- `pnpm test`（i18n parity，凡动错误码必跑）
- 手动：含 AWS key 的 diff 点 AI 生成 → 外发 prompt 已脱敏；配 `http://` base_url 被拒；
  坏代理 env 下 AI 调用报错不崩；`ssh -T` 黑洞主机可超时返回
- 合并前调 `@code-reviewer` 出 `review.md`（7 维度）

## 实施偏差（以实现为准）

- 2.6 `entry()` 参数由 host 改为不透明 key（`https/{key}`）；读路径：URL 带 userinfo 时先精确键、
  未中再读旧 host 键（payload 用户名一致才迁移）；无 userinfo 时只读旧键。
- clone 无前端调用者：`CLONE_DEST_NOT_EMPTY` 只做后端 + 中英 i18n，确认框待 clone UI 出现时再接。
- `process.rs` Unix 保持 `child.kill()`（无 nix/libc 依赖，注释注明 grandchildren 自建组残留局限）；
  `ssh-add -L` 按索引对齐为 best-effort，拿不到路径时 path 为空（前端显示待后续跟进）。
- `sqlite` / `tracing` 只 chmod 存量文件（WAL 与新建滚动日志继承 umask，注释注明；全程 0600 需
  wrapper/巡检，后续项）。
- `revert_commit` 冲突为纯内存操作，无需 reset（只修 `cherry_pick_commit` 冲突路径）。
- 400/422 → Protocol 的真实语义：不同 provider 重试，且整条 failover 链 fail-fast
  （配置类错误 fail-fast 符合 chain 既有策略；`should_failover` 注释已按此修正）。
- scrubber 无 regex 依赖：整行 `[REDACTED]`（非片段替换）；`token` 仅赋值/定界形式命中，
  另加 spaceless 匹配 `token =`；裸 `asia` 因误杀 prose/时区而移除（STS 上下文由
  `aws_`/`secret`/`sessiontoken`/`accesskeyid` 覆盖）。
- clone 守卫：失败残留也走确认流程（不做 `.git` 自动识别——确认框本身就是显式确认）。
- `ResolvedAiProvider` 去掉 `Serialize`（永不过 IPC，只有无 key 的 `AiGenerateOutcome` 给前端）。
- `process.rs` 成功路径 join 加 5s 上限（防 daemon 孙进程继承管道）；`ssh-add -l/-L` 加 15s 超时；
  `proxy.rs` env 锁纳入 poison 统一。
