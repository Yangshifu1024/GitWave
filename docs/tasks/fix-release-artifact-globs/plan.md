# fix-release-artifact-globs

> 修复 v0.3.0 release 草稿只挂上 Windows 附件的问题。

## 根因

`upload-artifact` 把多个 glob path 的**公共祖先目录**作为 artifact 内部根目录：

- macOS 上传了 `bundle/macos/*` + `bundle/dmg/*` → 公共祖先 `bundle/` → artifact 内部多一层 `macos/`、`dmg/`（已验证：下载 artifact zip 可见 `dmg/GitWave_0.3.0_aarch64.dmg`）
- Linux 三个 path → 内部多一层 `deb/`、`rpm/`、`appimage/`
- Windows 单个 path `bundle/nsis/*` → 文件在 artifact 根

release job 的 `artifacts/gitwave-{macos,linux}/*` 只命中**目录**，softprops 只上传文件 → `Pattern does not match any files`；因未开 `fail_on_unmatched_files`，静默跳过，job 仍 success（v0.3.0 运行 33254943118 日志可证）。

## 改动（`.github/workflows/build.yml` release job）

1. `files` glob 对齐真实层级：`artifacts/gitwave-macos/dmg/*.dmg`、`artifacts/gitwave-linux/{deb,rpm,appimage}/*`。
2. macOS 用 `*.dmg` 精确匹配——`bundle/dmg/` 里混有 Tauri 打包脚本 `bundle_dmg.sh`，宽 glob 会误传；`.app` 目录不作为分发物（dmg 才是）。
3. `fail_on_unmatched_files: true`：未匹配直接 fail，杜绝残缺草稿。

## 验证

重跑 v0.3.0 release job（或下一 tag），草稿 Assets 应含 dmg + deb + rpm + AppImage + exe 五类文件。
