# feat-system-proxy · 技术方案（F013）

对应提案：[F013 · 支持系统代理](../../pm/features/F013-system-proxy.md)

## 现状与根因

四条网络路径，读系统代理的机制全部缺失：

1. **AI 请求**：reqwest 0.12.28 全局单例（`infrastructure/ai/provider.rs` 的
   `OnceLock<reqwest::Client>`，60s 超时）。`Cargo.toml` 以
   `default-features = false` 引入 reqwest，`system-proxy` 特性
   （读 Windows 注册表 / macOS SystemConfiguration，hyper-util
   `client-proxy-system`）被关闭 → 只剩环境变量，而 Windows 用户通常没有。
2. **Git 网络操作**：vendored libgit2 1.9.7（fetch / push / clone /
   submodule）。源码 `remote.c::git_remote__http_proxy` 只查 git config
   `http.proxy` / `remote.<name>.proxy`，回退环境变量；Windows 默认传输
   WinHTTP，**不读** WinInet 系统代理。
3. **LFS / 凭证 helper**：`git lfs`、`git credential fill` 子进程遵循系统
   git 规则（`http.proxy` config + 环境变量），应用未注入任何代理信息。
4. **更新检查**：tauri-plugin-updater 2.10.1 内部 reqwest 0.13.4，同样
   `default-features = false`；插件级 Builder 无代理配置入口（代理只能按次
   在 UpdaterBuilder 上设置，而 check 由前端 JS 发起）。

代码库中没有任何 proxy 处理逻辑。

## 方案：启动时代理桥接（env bridge）+ 显式设置三档

核心决策：**读取代理配置后写进程环境变量**（`HTTP_PROXY` / `HTTPS_PROXY` /
`NO_PROXY`），而不是逐路径配置 client。依据（已核对依赖源码）：

- reqwest（0.12 AI 单例 / 0.13 updater）：client 构建时默认
  `ProxyMatcher::system()`，**无条件先读环境变量**（`Matcher::from_env`），
  与 `system-proxy` 特性无关
- libgit2：config 查不到时回退环境变量（`http_proxy_env`，curl 语义，支持
  `no_proxy` 按主机豁免——比 `ProxyOptions::url()` 固定代理丢失豁免粒度更优）
- `git lfs` / `git credential fill` 子进程继承进程环境，系统 git 天然遵循

一处实现，四路全通；运行期改环境变量对 libgit2（每次操作重读）与新建
reqwest client 即时生效。

**优先级**：应用内手动设置 > 用户已有环境变量（不覆盖）> 系统代理。

### 代理探测（`infrastructure/proxy.rs` 新模块）

- **Windows**：`windows-registry` 读 `HKCU\Software\Microsoft\Windows\
  CurrentVersion\Internet Settings`（该 crate 已在依赖树中，hyper-util 同款）：
  `ProxyEnable=1` 时取 `ProxyServer`（兼容 `host:port` 与
  `http=a;b=..;socks=..` 两种格式；仅 socks → 视为未配置，见限制）、
  `ProxyOverride` 作绕过列表
- **macOS**：`system-configuration`（依赖树已有）读 SCDynamicStore 的
  `kSCPropNetProxiesHTTP*` / `HTTPS*`
- **Linux**：系统代理约定即环境变量 → 不探测（现状即「跟随系统」）

### NO_PROXY 规范化（防本地服务被误代理）

hyper-util 与 libgit2 的 no_proxy 都**不支持 IP 通配**（`127.*` 不匹配
`127.0.0.1`），`<local>` 无对应语义。转换规则：

- 始终注入 `localhost,127.0.0.1,::1`（保护本地 Ollama / LAN 服务）
- `*.x.com` → `x.com`（后缀匹配语义等价）；`<local>` 丢弃
- IP 通配（`127.*` / `192.168.*` 等）**丢弃**：两种 no_proxy 消费者都不
  支持通配或 CIDR，无法等价改写；HTTP(S) 局域网 git 服务器会被代理，是
  已知限制（SSH 局域网地址不走 HTTP 代理，不受影响）

### 设置存储（应用级全局设置，现有空白点）

- `app_settings(key TEXT PRIMARY KEY, value_json TEXT)` 表（走既有
  `migrations::apply` 机制），key = `proxy`
- `ProxySettings { mode: system|manual|off, manual_url: Option<String> }`，
  命令 `cmd_get_proxy_settings` / `cmd_set_proxy_settings`（保存 → 重算
  env → 立即 apply + 重建 AI client，无需重启）

### 应用与生效

- `lib.rs::run()` 最开头调用 `apply_to_env()`（先于插件与任何线程；
  edition 2021 下 `env::set_var` 安全）
- AI client（`provider.rs`）：`OnceLock<Client>` → `RwLock`，
  `cmd_set_proxy_settings` 后 rebuild；env 变更后新建的 client 自动带入
  新代理
- updater：由 env 桥接覆盖；reqwest 0.13 加直依赖开 `system-proxy` 特性
  做 feature unification 兜底（插件本身无代理入口）

### 已知限制（随提案文档）

- SOCKS-only 系统代理不应用于 git 路径（libgit2 无 SOCKS 支持）；reqwest
  路径加 `socks` 特性可走
- PAC 不支持；SSH 远程不涉及 HTTP 代理；代理鉴权用 URL 内嵌凭证形式
- ProxyOverride 中的 IP 通配（`192.168.*` 等）被丢弃（消费者不支持），
  HTTP(S) 局域网 git 服务器会被代理
- 运行期改写进程 env 与其他线程的 env 读取在 POSIX 上存在理论竞态，
  与 dotenv 类加载器同水平，接受（保存事件罕见；启动路径单线程）

## 实施步骤

1. `Cargo.toml`：reqwest 0.12 加 `"system-proxy"`；新增
   `windows-registry`、`system-configuration`（target-gated）、reqwest 0.13
   （仅 feature unification，注释说明）
2. `infrastructure/proxy.rs`（新）+ `infrastructure/mod.rs` 注册 + 单测
3. `persistence/sqlite.rs` 迁移 + `app_settings` 读写 + domain 结构 +
   `use_cases` + `lib.rs` 命令与启动桥接
4. `provider.rs` client 可热重建
5. 前端：`api.ts` 类型与包装、`SettingsModal` 网络设置节、i18n（en /
   zh-CN，保 parity）

## 验证

- `cargo fmt` / `cargo clippy --all-targets -- -D warnings` /
  `cargo test --all-targets`；前端 `tsc --noEmit` + `vitest run`
- 手动验收（Windows 真机，Clash）：
  - [ ] 开系统代理 → AI 生成与 fetch 在代理工具日志可见；更新检查可达
  - [ ] 手动模式填 `http://127.0.0.1:7890`（系统代理关）→ 同上生效
  - [ ] 关闭模式 → 直连；本地 Ollama 全程不走代理
  - [ ] 三档切换即时生效（不重启）；无系统代理时行为与现状一致

## 分支

`feature/system-proxy`（基于 main `5b905fd`）。
