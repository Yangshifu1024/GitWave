# F009 · 应用内检查更新与自动更新

## 背景

GitWave 通过 GitHub Releases 分发（tag → CI 三平台构建 → draft release 手动发布），但用户没有任何应用内更新通道：发现新版只能手动来 Releases 页下载替换。Tauri 2 官方提供 `tauri-plugin-updater`：客户端持有 minisign 公钥，从清单（`latest.json`）取新版信息，校验签名后下载安装。仓库为公开仓库，清单可直接指向 `releases/latest/download/latest.json`，无需额外服务。

## 提议方案

- 引入 `tauri-plugin-updater` + `tauri-plugin-process`，CI 发版时产出 updater 产物与 `latest.json`（随 release 资产发布）
- 检查入口：启动时自动检查（默认开启，Settings 可关，本地存储）+ Settings「Check for updates」手动检查；启动检查失败一律静默
- 更新交互：发现新版弹更新窗（版本对比 + release notes 链接），用户确认后下载（带进度）→ 安装 → 提示重启
- 平台策略：
  - macOS（app.tar.gz）/ Windows（NSIS setup.exe）/ Linux AppImage：应用内完整更新
  - Linux deb/rpm：updater 插件在 Linux 仅支持 AppImage（`APPIMAGE` env），且系统路径需 root，无法自我替换 → 降级为「一键打开 Releases 页」手动下载

## 范围（不做清单）

- 不做强制更新 / 更新弹窗不可关闭
- 不做 deb/rpm 的 apt/rpm 托管仓库（如需要另立提案）
- 不做增量更新（Tauri updater 目前全量下载）
- 不做更新日志的完整渲染（只链接 GitHub release 页）

## 影响

- 涉及模块：`src-tauri`（插件注册 / 配置 / capabilities）、前端（Settings Updates 区块、更新弹窗、启动检查）、`.github/workflows/build.yml`（签名密钥 env + latest.json 组装）、README 发版清单
- 影响版本：v0.5.0
- 是否破坏向后兼容：否；但 0.4.0 及更早版本没有 updater，无法被自动更新——装上本特性的第一个版本是链式自动更新的起点
- 用户手动操作（一次性）：生成 minisign 密钥对并备份私钥（不可再生：GitHub secret 只写不读，换钥则已装用户永远收不到更新）；`gh secret set TAURI_SIGNING_PRIVATE_KEY`

## 决策

- 状态：接受
- 决策人：用户（yangzhenbiao）
- 决策日期：2026-08-30
- 关联决策：分支 `feature/f009-auto-update`；执行计划见 `docs/tasks/feat-auto-update/plan.md`；deb/rpm 走降级提示（用户确认），apt/rpm 仓库与强制更新明确不做
