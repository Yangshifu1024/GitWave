# GitWave · 系统架构

桌面程序及 NSGo 配置位于 `cmd/gitwave/`。界面、会话状态和 Go 用例在同一进程运行。平台 UI 由 NSGo 接入 AppKit、Win32 和 GTK。

| 目录 | 职责 |
|---|---|
| `cmd/gitwave/` | History / Changes 工作台、Workspace 切换、仓库会话、确认流程、系统菜单和设置 |
| `internal/gitcli/`、`internal/gittool/` | Git CLI 参数和输出、仓库事务、凭证提示、随包 Git 与 Git LFS 定位 |
| `internal/workspace/` | SQLite 持久化、旧数据迁移、Workspace 导入导出及桌面偏好 |
| `internal/ai/`、`internal/proxy/`、`internal/sshclient/` | AI provider、脱敏、代理解析与 SSH 客户端 |
| `internal/appupdate/`、`cmd/prepare-*-setup/` | 更新清单验证、平台安装与重启、打包 |

## 工作台和事务

- Workspace 可同时打开多个，每次仅一个活动仓库。SQLite 按 Workspace 与仓库 ID 恢复选择、设置和顺序。
- 切换 Workspace 时，工作台恢复该组仓库及最后活动仓库，再异步加载 History、Changes 和侧栏。晚到结果按发起仓库与请求校验，写操作完成状态仍归给发起仓库。
- Git 操作通过上下文取消。改变文件、索引或引用的确认绑定 `HEAD`、索引及相关路径状态，确认过期时保留工作区。
- History 和 diff 仅渲染可见行。大文件先加载有界预览，超限时提供明确状态。后台自动刷新按偏好扫描当前 Workspace，手动刷新面向活动仓库。

## 数据、AI 与更新

- 正式程序沿用 `GitWave/state.db` 和原有 SQLite 迁移。第一次打开旧数据前保存包含已提交 WAL 的在线备份。预览使用独立数据目录。
- AI 请求在本地构造 prompt 并扫描敏感内容，按 Workspace 设置选择 provider。建议填回界面，Git 写入仍由用户确认。
- 更新器验证 GitHub Releases 中的 Ed25519 签名清单、下载长度和 SHA-256。旧版偏好迁移使用签名过渡包，生产切换步骤见[迁移计划](../../tasks/refactor-nsgo-native-ui/plan.md#三端包装更新与回退)。
