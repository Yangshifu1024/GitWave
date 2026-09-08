# Plan · fix-ci-pnpm-package-manager

## 现象

2026-09-08 17:22 起同一 push 批次触发的 CI 全线失败：

- `main` push：lint **failure**（Frontend lint ×3 OS，`pnpm/action-setup@v4` 报 `No pnpm version is specified`）；test 同模式预期失败（concurrency 之前的 cancelled 不是失败）。
- tag `v0.7.8` push：build **failure**（macOS / Windows 同报错；Linux 死在 `Dependencies lock file is not found`）。
- 同日 16:52 的 PR #38 CI 还能通过 → runner 镜像当日更新后，`pnpm/action-setup@v4` 不再从环境兜底 pnpm 版本，行为收紧。

## 根因

1. `package.json` 缺少 `"packageManager"` 字段。`pnpm/action-setup@v4` 在 action 配置未给 `version` 时，要求 `package.json` 提供 `packageManager`，否则直接报错退出。仓库 4 个 workflow（lint/test/build 各 frontend job）全部未指定 `version`。
2. `build.yml` 的 `build-linux` job：`actions/setup-node` 用了 `cache: npm`（仓库无 package-lock.json → lock file not found），且缺少 `pnpm/action-setup@v4` 步骤，后续 `pnpm install` 无法运行。为三 job 中与其他两个不一致的残留配置。

## 修复（2 文件）

1. `package.json`：增加 `"packageManager": "pnpm@12.3.4"`（对齐本地实际版本）。
2. `.github/workflows/build.yml` `build-linux`：补 `- uses: pnpm/action-setup@v4`，`cache: npm` → `cache: pnpm`，与 macOS/Windows job 对齐。

## 验证

- 本地：`pnpm typecheck` ✅、`pnpm test`（161/161）✅。
- CI：合入 main 后 push 触发 lint/test 应全绿；`v0.7.8` tag 需删除重打（或推新 tag）才能重新触发 build（tag 不跟随分支移动）。

## 回归要点

- 确认 4 个 workflow 中所有 `pnpm/action-setup@v4` 均能解析到 `packageManager`（lint/test/build + pages 如有）。
- `build-linux` 的 pnpm install 使用 `--frozen-lockfile` 与 lockfile v9 兼容（pnpm ≥ 9）。
