# GitWave · 工程约定

> 工程实践与质量门禁。Git 工作流与分支 / Commit / PR 规则见 `AGENTS.md`。

## 代码风格与本地门禁

- Go 文件用 gofmt。pre-commit 检查改动中的 Go 文件。
- `make check` 运行 gofmt 检查、go vet 和 Go 测试。逐项运行可用 `make fmt-check`、`make lint` 和 `make test`。
- commit message 遵循 Conventional Commits。Markdown 和用户可见文案在审查时核对。

## 测试策略

| 层 | 范围 | 命令 |
|---|---|---|
| 单元及模型 | History、Diff、工作台状态、AI prompt | `go test ./...` |
| 集成 | 临时 Git 仓库、SQLite、HTTP 和代理 | `make test`、`make test-race` |
| 原生窗口 | 点击、滚动、焦点、确认、取消、错误恢复 | `go -C cmd/gitwave tool nsgo test -target macos` |
| 安装与升级 | 预览打包、旧数据迁移、失败回退 | 见三端候选工作流与迁移验收记录 |

## 错误处理与日志

### 错误分类

| 类别 | 例子 | 用户文案策略 |
|---|---|---|
| **网络** | fetch / push 失败 | "网络不可达，检查代理 / VPN" |
| **凭证** | auth 失败 | "凭证被拒绝，请检查 SSH key / HTTPS helper 配置" |
| **权限** | EACCES | "权限不足，尝试以管理员 / 调整文件权限" |
| **版本冲突** | rebase / merge 冲突 | "需要手动解决冲突，已为你打开冲突面板" |
| **协议** | 远端协议不支持 | 明确说明支持范围（git / https / ssh / ssh+git） |
| **未知** | 其他 | "未知错误，已记录，请反馈 issue（附 trace id）" |

错误返回至发起操作的界面。错误文案和本地诊断应避免写入凭证、diff 或用户仓库内容。应用无自动崩溃上报。

## CI/CD

### CI（GitHub Actions）

| Workflow | Runner | 校验内容 |
|---|---|---|
| `go-desktop.yml` | macOS + Ubuntu + Windows | Go 静态检查、竞态测试、NSGo 预览包与安装验证 |
| `go-release.yml` | 手动从 `main` 发起 | 三端候选、签名清单及 draft release |
| `pages.yml` | Ubuntu | 发布 `site/` 到 GitHub Pages |

### Release

- 偏好导出版使用 Git 检查点上的标签工作流。核对三端资产和草稿的 `latest.json` 后发布。
- 从 `main` 运行 `go-release.yml`。三端正式候选通过验收后，工作流验证文件摘要并组装 Go 更新清单及两份旧格式清单。启用草稿时，输入的版本须与 `make version` 显示的版本一致，远端现有标签必须指向构建提交。
- macOS 安装包与更新 ZIP 使用 Developer ID 签名和公证，Windows NSIS 使用 Authenticode，Linux 提供 deb、rpm 和 AppImage。签名配置和版本交接见 [全量迁移计划](../../tasks/refactor-nsgo-native-ui/plan.md#三端包装更新与回退)。
- 发布草稿后，Go 客户端读取签名的 `gitwave-update.json`。旧客户端按 `latest.json` 和 `gitwave-legacy-transition.json` 依次过渡。macOS、Windows 和 AppImage 支持应用内安装，deb/rpm 打开公告版本的发布页。

## 仓库目录约定

- `go.mod` 和 `go.sum` 位于根目录。
- `cmd/gitwave/` 保存 NSGo 入口、配置和应用资源。其他 `cmd/` 子目录是打包与更新工具。
- `internal/` 保存仓库、持久化、凭证、AI 和更新适配。
- `scripts/` 保存质量门禁和打包验证脚本。`site/` 是独立官网。

## 安全

- **API key**：仅 OS Keychain；内存中按需解密使用，不落盘
- **diff scrubber**：发送 AI 前 regex + token 扫描（GitHub token / AWS key / 私钥 / 常见 PII）
- **上传前必经 scrubber**；scrubber 可被用户显式禁用，但每次禁用需用户确认
- **不收集**任何用户代码、提交内容、commit message 至服务端（除非用户主动通过 AI 调用上传）
- **本地优先**：diff 默认不离开本机；所有网络调用显式列出

## 性能预算

- 应用冷启动到首个可交互界面：< 1s（macOS Apple Silicon 基线）
- history 图打开到首次可交互：< 500ms（10k commit 仓）
- 单文件 diff 渲染：< 200ms（1MB 文件）
- 后台 git 操作不阻塞 UI：Go context 和可取消的 Git 子进程

## 文档同步

- 任何用户可感知行为变更 → 同步更新 `docs/pm/core/`
- 任何技术选型 / 架构变更 → 同步更新 `docs/tech/`
- 任何具体 PR / 任务执行过程 → 同步更新 `docs/tasks/<任务名>/`
- 见 `AGENTS.md` "技术文档归属"小节的判定规则
