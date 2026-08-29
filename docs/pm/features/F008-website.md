# F008 · 项目官网（GitHub Pages + 自定义域名）

## 背景

GitWave 已有 tag 触发的三平台 CI 与 draft release，但缺少面向用户的入口：README 无下载链接，潜在用户只能翻仓库找 Releases。项目已持有域名 `gitwave.work`（app identifier `desktop.gitwave.work` 亦暗示此域名归属），适合部署官网并承接下载入口。

## 提议方案

- 在 `site/` 目录新建纯静态产品落地页（零构建、无 npm 依赖）：hero + 功能亮点 + 隐私说明 + 三平台下载区（链接 GitHub Releases latest）
- 以 GitHub Actions workflow（`actions/deploy-pages`）在 `main` 上 `site/**` 变更时自动部署
- 绑定自定义域名 `gitwave.work`（apex 为主域，`www` CNAME 并 301 到 apex）；`site/CNAME` 保证部署不丢域名
- 视觉对齐 `docs/design/00-overview.md` token：Native Blue `#007AFF`、SF Pro 字体栈、圆角 8/12、subtle 阴影、暗色跟随系统

## 范围（不做清单）

- 不做文档站 / 博客 / changelog 页（后续可在 `site/` 基础上演进）
- 不做访问统计 / 第三方脚本（隐私原则）
- 不放 app 截图（mockup 非最终 UI，待真机截图再补）

## 影响

- 涉及模块：新增 `site/`、`.github/workflows/pages.yml`；README Documentation 节加官网链接；应用代码零改动
- 影响版本：v0.4.x
- 是否破坏向后兼容：否
- 用户手动操作：GitHub Settings → Pages → Source 选 "GitHub Actions"；DNS 后台加 apex A 记录 ×4 + `www` CNAME；部署后勾选 Enforce HTTPS

## 决策

- 状态：接受
- 决策人：用户（yangzhenbiao）
- 决策日期：2026-08-30
- 关联决策：分支 `feature/website`；执行计划见 `docs/tasks/feat-website/plan.md`
