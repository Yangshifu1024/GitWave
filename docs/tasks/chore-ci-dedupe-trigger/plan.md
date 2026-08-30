# chore(ci): lint/test push 触发收敛到 main

## 背景

`lint.yml` / `test.yml` 同时挂 `push: branches: ["**"]` 与 `pull_request: branches: [main]`。「push 功能分支 + 开 PR」会触发两轮完整 CI（每分支 24 job）。`concurrency.cancel-in-progress` 只能取消并行中的重复 run；push 轮通常在开 PR 前已跑完，防不住。

## 改动

`push` 触发收敛为 `branches: [main]`，PR 校验交给 `pull_request`（GitHub 官方推荐模式，与 `pages.yml` 一致）：

- push 功能分支：不跑 CI
- 开 / 更新 PR：跑一次
- squash 合入 main：再跑一次验证 main 本身

`build.yml`（仅 tags）与 `pages.yml`（仅 main）无重复，不动。

## 代价

未开 PR 的 WIP 分支不再有 CI；需要时开 draft PR 即可覆盖（draft PR 同样触发 `pull_request`）。
