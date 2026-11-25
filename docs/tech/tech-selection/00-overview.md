# GitWave · 技术选型

当前桌面实现采用 [ADR 0007](../decisions/00-overview.md#0007--全量迁移到-go--nsgo)。具体依赖版本由根目录 `go.mod` 锁定。

| 维度 | 选择 | 用途 |
|---|---|---|
| 桌面 | Go + NSGo | macOS、Windows 和 Linux 原生控件、系统菜单及窗口事件 |
| Git | 官方 Git CLI + Git LFS | 仓库读写、网络传输、凭证 helper、系统 SSH |
| 本地持久化 | `database/sql` + `modernc.org/sqlite` | SQLite WAL、嵌入迁移及旧数据库兼容 |
| 凭证 | macOS Keychain、Windows Credential Manager、Linux Secret Service | AI 密钥和账号凭证 |
| 网络 | Go `net/http` 和代理解析 | OpenAI 兼容接口、Anthropic、Ollama、下载 |
| 更新 | Ed25519 签名清单和平台安装适配 | 校验包哈希、原位安装及重启，deb / rpm 打开发布页 |
| 测试 | Go testing、NSGo 原生窗口测试 | 状态与事务回归、真实 Git fixture、平台交互 |

开发环境需要 `go.mod` 指定的 Go 版本、Git 2.46 或更新版本及 Git LFS。当前 Linux 源码开发环境使用 GTK 4.14 或更新版本，打包可携带兼容运行库。平台构建与质量门禁见[工程约定](../engineering/00-overview.md)。
