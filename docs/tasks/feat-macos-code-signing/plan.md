# feat-macos-code-signing

> CI 构建 macOS 产物用 Developer ID 证书签名并公证（notarize + staple），用户下载 dmg 后可直接打开。

## 背景

此前 macos job 无签名凭证，Tauri 走 ad-hoc 签名，用户从 Releases 下载的 dmg 被 Gatekeeper 拦截（「已损坏」/需手动放行）。

## 凭证方案

签名 + App Store Connect API key 公证（用户已选定 API key 方式）。

GitHub Secrets：

| Secret | 内容 |
|---|---|
| `APPLE_CERTIFICATE` | Developer ID Application `.p12` 的 base64 |
| `APPLE_CERTIFICATE_PASSWORD` | `.p12` 导出密码 |
| `APPLE_SIGNING_IDENTITY` | `Developer ID Application: <name> (TEAMID)` 完整身份串 |
| `APPLE_API_ISSUER` | App Store Connect API Issuer ID |
| `APPLE_API_KEY` | API Key ID |
| `APPLE_API_KEY_P8` | `.p8` 私钥文件内容（只能下载一次） |
| `KEYCHAIN_PASSWORD` | 临时钥匙串密码（任意强随机串） |

## 改动（`.github/workflows/build.yml` macos job）

1. `Import signing certificate`：`security create-keychain` 建 runner 临时钥匙串并导入 `.p12`（`set-key-partition-list` 免交互授权 codesign），证书文件用后即删，不落盘。
2. `Write App Store Connect API key`：把 `APPLE_API_KEY_P8` secret 写为 workspace 根的 `AuthKey.p8`——`APPLE_API_KEY_PATH` 是路径不是 secret，故在 CI 生成。**必须是绝对路径**（`${{ github.workspace }}/AuthKey.p8`）：bundler 在临时目录下调用 notarytool，相对路径会报 `The file couldn't be opened`（首跑 v0.3.0 已踩坑）。
3. `tauri build` step 注入 `APPLE_SIGNING_IDENTITY` + 公证三变量；Tauri bundler 自动完成：签 .app → notarytool 公证 → staple 回 dmg。

`tauri.conf.json` 无需改动。

## 注意

- secrets 未配齐前推 tag，macos job 会在导入步骤失败（预期行为，fail_on_unmatched_files 同理防静默）。
- 当前仅构建 aarch64；Intel 需后续加 x86_64 / universal target，签名流程不变。

## 验证

下一 tag 构建后：`codesign --verify --deep --strict -v GitWave.app`、`spctl -a -vv -t install`、`stapler verify <dmg>`。
