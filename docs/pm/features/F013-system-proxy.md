# F013 · 支持系统代理

## 背景

GitWave 的所有网络能力——AI 请求（commit message / PR 描述 / commit 解释）、Git 远程操作（fetch / pull / push / clone）、LFS、应用内更新检查——在系统配置了代理（Windows「系统代理」/ macOS 系统网络代理）的环境下全部不走代理，表现为请求直连超时失败。使用 Clash / v2rayN 等代理工具「系统代理模式」的用户（国内开发环境主流形态）无法正常使用任何联网功能。

根因（工程已验证）：四条网络路径读系统代理的开关或机制全部缺失——

1. AI 请求：reqwest 以 `default-features = false` 引入，`system-proxy` 特性（读 Windows 注册表 / macOS SystemConfiguration）被关闭
2. Git 网络操作：内嵌 libgit2 只认 git config `http.proxy` 与环境变量，从不读 Windows 系统代理
3. LFS / 凭证 helper：`git lfs`、`git credential fill` 子进程遵循系统 git 规则（config + 环境变量），应用未注入任何代理信息
4. 更新检查：tauri-plugin-updater 内部 reqwest 同样关闭了系统代理特性

另外，相当多用户的代理工具**未开启**「系统代理模式」（只监听本地端口），系统代理无从读起，这类用户需要在应用内手动指定代理地址。

## 提议方案

**新增「网络」设置节，代理三档模式（默认跟随系统）：**

1. **跟随系统**（默认）：自动读取系统代理——Windows 读用户级 Internet Settings（ProxyEnable / ProxyServer / ProxyOverride），macOS 读系统网络代理，Linux 遵循环境变量约定
2. **手动**：用户填写代理地址（如 `http://127.0.0.1:7890`）
3. **关闭**：不使用任何代理（用户自选系统环境变量仍生效）

**覆盖全部网络路径**：AI 请求、Git fetch / pull / push / clone、LFS、更新检查一致生效；设置变更即时生效，无需重启。

**本地回环豁免**：应用注入的代理始终跳过 `localhost` / `127.0.0.1` / `::1`，本地 Ollama 等 LAN 服务不受影响；系统代理绕过列表（ProxyOverride / 排除列表）被继承（通配语法按目标能力折衷，见技术方案）。

与产品原则不冲突：代理只是网络连通性配置，不改变「本地优先、隐私可控」边界（diff 不离开本机、AI 调用仍由用户 BYOK 主动发起）；属于基础设施补齐。

**明确不做**：PAC 脚本解析；SOCKS-only 代理应用于 git 路径（libgit2 不支持，HTTP 代理端口为常态）；SSH 协议远程的代理（ProxyCommand）；代理鉴权 UI（URL 内嵌凭证形式够用）。

## 影响

- 涉及模块：Rust 新增代理模块（系统代理探测 + 环境变量桥接）、应用级全局设置存储（现有空白，新增 `app_settings` 表 + get/set 命令）、AI HTTP 客户端可热重建、SettingsModal 网络设置节、i18n
- 影响版本：v0.7.x
- 是否破坏向后兼容：否（默认行为「跟随系统」仅在系统确实配置了代理时改变现状——而现状是联网功能不可用）

## 决策

- 状态：接受
- 决策人：杨师傅
- 决策日期：2026-09-02
- 关联决策：F009（更新检查路径同样纳入代理覆盖）；[docs/tech/engineering/00-overview.md](../../tech/engineering/00-overview.md) 错误文案约定「网络不可达，检查代理 / VPN」
