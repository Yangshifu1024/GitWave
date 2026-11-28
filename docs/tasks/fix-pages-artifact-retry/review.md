# Pages 产物重试审查

## ✅ 优点

部署通过 `needs.upload.outputs.artifact_name` 使用实际上传的名称。仅重跑失败的部署时沿用原产物；重跑全部任务时，上传名称包含新尝试序号，旧产物仍在同一次工作流运行中也不会重名。

## 🔴 严重问题（必须修复）

未发现阻塞问题。

## 🟡 一般问题（建议修复）

若上传产物尚未可见，可重跑失败的部署。上传动作默认保留产物一天；若产物已经过期或被删除，应重跑全部任务以生成新产物。[上传动作配置](https://github.com/actions/upload-pages-artifact/blob/v3/action.yml)

## 🟢 优化建议（可选）

没有额外建议。

## 📝 总体评价

`actionlint` 通过。已核对名称输入输出和权限：上传仅需读取仓库，部署仅授予 Pages 写入与 OIDC 令牌签发，并继续使用 `github-pages` 环境。静态检查未触发线上部署。[部署动作说明](https://github.com/actions/deploy-pages/tree/v4)
