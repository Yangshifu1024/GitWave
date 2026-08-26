# chore-rename-bundle-id

> 把 Tauri bundle identifier 从 `com.gitwave.app` 改为 `desktop.gitwave.work`，对齐已注册的 `gitwave.work` 域名。

## 状态

草案。

## 决策

| 项 | 决定 | 来源 |
|---|---|---|
| Tauri identifier | `desktop.gitwave.work` | 用户选 — 现代 reverse-DNS，明确是 desktop 端 |
| `[package].name` / `[lib].name` / `package.json.name` | 不改 | 用户选 — crate 内部名与 bundle ID 解耦 |
| `noreply@gitwave.local` git signature | 不改 | 用户选 — `.local` 占位够用 |
| 测试 tmp 目录 `gitwave-...` 前缀 | 不改 | 用户选 — 与域名无关 |
| `productName: "GitWave"` 显示名 | 不改 | — 显示名独立于 bundle ID |

## 改动清单

唯一文件唯一一行：

```diff
   "productName": "GitWave",
   "version": "0.1.0",
-  "identifier": "com.gitwave.app",
+  "identifier": "desktop.gitwave.work",
```

## 影响

- macOS `.app` bundle ID：`desktop.gitwave.work`
- Windows MSI / NSIS ProductCode：跟随 identifier
- Linux `.desktop` StartupWMClass：`desktop.gitwave.work`
- v0.1 尚未发布，旧 `com.gitwave.app` 没有装机量，无升级迁移问题
- 本地构建：`identifier` 变化会让 `tauri-build` 重新生成 build cache，第一次构建会清旧 `target/`

## 回归验证

- `cargo tauri build` 三 OS 跑通
- `npm run tauri dev` 在 macOS / Linux 启动后，`Info.plist`（macOS）或 `.desktop`（Linux）含 `desktop.gitwave.work`
- 不改任何源码 / workflow / 文档

## 参考

- Tauri 2 identifier 规则：<https://v2.tauri.app/reference/config/#identifier>
- Apple Bundle ID 命名指南：<https://developer.apple.com/documentation/bundleresources/information_property_list/cfbundleidentifier>
