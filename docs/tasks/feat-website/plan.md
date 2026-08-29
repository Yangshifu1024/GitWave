# feat-website · 官网落地页 + GitHub Pages 部署 · 实施计划

> 提案：[F008](../../pm/features/F008-website.md)。分支 `feature/website`。

## 目标

在 `gitwave.work` 上线 GitWave 产品落地页，由 GitHub Actions 自动部署，下载入口指向 GitHub Releases latest。

## 变更清单

| 文件 | 说明 |
|---|---|
| `site/index.html` | 单页落地页（内嵌 CSS、零 JS、零构建）：hero / features ×6 / 隐私说明 / 下载区 / footer |
| `site/icon.png` | 复制自 `src-tauri/icons/icon.png`（512×512） |
| `site/favicon-32.png` | 复制自 `src-tauri/icons/32x32.png` |
| `site/CNAME` | `gitwave.work`（保域名设置） |
| `.github/workflows/pages.yml` | push main 且 `site/**` 变更时 upload-pages-artifact + deploy-pages |
| `docs/pm/features/F008-website.md` | PM 提案 |
| `README.md` | Documentation 节加官网链接 |

## 验证

- [ ] 本地：`open site/index.html` 检查 light/dark、移动端宽度、链接可达
- [ ] PR 合入 main 后 `pages` workflow 绿
- [ ] GitHub Settings → Pages → Source = "GitHub Actions"（部署前置条件）
- [ ] Custom domain 自动填入 `gitwave.work`，证书签发后勾选 Enforce HTTPS
- [ ] DNS：apex A ×4（185.199.108/109/110/111.153）+ `www` CNAME → `Yangshifu1024.github.io.`
- [ ] `curl -I https://gitwave.work` 200；`www` 301 到 apex

## 边界

- AI 不 commit / push / merge（AGENTS.md P1），人工 gate
- 不改 Tauri 应用代码；`site/` 与前端构建（`dist/`）互不影响
