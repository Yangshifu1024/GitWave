# fix-dmg-notarization

> tag 触发的 CI 构建产出的 dmg 自带公证票据，用户直接下载打开不再被 Gatekeeper 拦截。

## 根因

Tauri bundler（tauri-action 内部）的公证流程只覆盖 `.app`：签 .app → notarytool 公证 → staple 回 .app（tar.gz）。**dmg 仅 codesign，不公证、无票据**，直接下载 dmg 的用户会被 Gatekeeper 判 `rejected / source=Unnotarized Developer ID`（v0.8.2 实测）。这不是签名配置错误——dmg 内的 .app 本身完全正常，走应用内 updater 的用户（`.app.tar.gz`）不受影响。

## 修复（`.github/workflows/build.yml` macos job）

在 tauri-action 上传后新增 `Notarize and staple dmg` 步骤，对 draft release 里的 dmg 补办公证并替换资产：

1. `gh release download "$GITHUB_REF_NAME" --pattern '*.dmg' --clobber`——通配下载，不硬编码版本号；
2. `xcrun notarytool submit <dmg> --wait`——key 用 workspace 绝对路径的 `AuthKey.p8`（前序步骤已写入），`--key-id` / `--issuer` 复用 `APPLE_API_KEY` / `APPLE_API_ISSUER` secret，与 tauri-action 步骤同一套凭证；
3. `xcrun stapler staple` + `stapler validate`——钉票据并验证；
4. `gh release upload … --clobber` 回传覆盖 draft 资产。

约束：`APPLE_API_KEY` 为空则跳过（与 secrets 未配齐的仓库兼容）；notarytool / stapler 任一失败即 job 失败，不允许未公证 dmg 静默上架。

## 存量补救（v0.8.2，已执行）

线上 v0.8.2 已发布，dmg 无票据，手动补公证（与 CI 步骤同款命令）：

1. `gh release download v0.8.2 --pattern '*.dmg'` → `spctl` 复核确为 `rejected / Unnotarized Developer ID`；
2. `notarytool submit --wait` → Accepted（提交 ID `8eff7c32-5572-499e-8ab4-9f9e3adfc9c8`）；
3. `stapler staple` + `stapler validate` → `spctl -a -vv -t install` 转 `accepted / Notarized Developer ID`；
4. `gh release upload v0.8.2 --clobber` 回传，重新下载 sha256 一致（`bff9c5f7…`）且 validate 通过。

## 验证

- 单元层面：YAML 语法（`yaml.safe_load`）通过；步骤命令与上节手动补救完全一致（后者已实测）。
- 回归层面：下一 tag（v0.8.3）构建后，macos job 日志应出现 `Notarize and staple dmg` 步骤且成功，draft 的 dmg 重新下载后 `stapler validate` 通过。
