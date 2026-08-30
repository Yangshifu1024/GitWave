# fix-ci-redundant-publish-job

> publish job 的自研 latest.json 生成逻辑整体删除，清单生产完全交给 tauri-action；CI 侧保留一个只读校验门禁，确保清单与 updater 兼容。

## 背景

v0.5.0（2026-08-30）发布 CI：三个 Build job 全绿，Publish release job 挂在生成 latest.json 的 node 脚本上：

```
Error: expected exactly one /^GitWave\.app\.tar\.gz\.sig$/ in sigs/, got 0
```

### 根因

正则按 bundler 本地产物名锚定全名（`GitWave.app.tar.gz.sig`），而 tauri-action 上传时会改名加版本号与架构（`GitWave_0.5.0_aarch64.app.tar.gz.sig`），于是 `sigs/` 命中 0 个直接 throw。linux/windows 正则是后缀匹配，侥幸命中。

### 关键实证：自研生成逻辑本来就是多余的

publish job 失败时间 09:42:15，而 release assets 里已有一份完整 latest.json（pub_date 09:41:32，最后一个跑完的 macOS build job 经 tauri-action 上传）：三平台主键 `darwin-aarch64`（.app.tar.gz）/ `linux-x86_64`（AppImage）/ `windows-x86_64`（NSIS setup）齐全，签名与 assets 一一对应。即 tauri-action 在构建阶段就会合并 release 上既有的全部 `.sig` 生成清单，最后完成的平台 job 产出最终态。

与 updater 的兼容性核对（`src-tauri/tauri.conf.json:59`）：

- endpoint `releases/latest/download/latest.json` 与 tauri-action 上传的资产名一致，draft 转正即生效
- updater 按 `{target}-{arch}` 精确找键，清单三主键齐全且指向正确的产物类型；`-nsis` / `-appimage` / `-deb` 等后缀键会被忽略
- 清单 URL 为 `api.github.com/.../assets/<id>` 形态，release 转正后 updater 可用（tauri-action 官方 GitHub Releases 配方的标准行为）

## 改动

| 文件 | 改动 |
|---|---|
| `.github/workflows/build.yml` | `publish` job 改为 `verify-manifest`（Verify release）：删掉下载 `*.sig` / node 生成 latest.json / upload 三步与 checkout，换成一步只读校验——从 draft 拉 latest.json，断言 `version === tag 去 v 前缀`、三平台键存在、signature/url 非空；缺文件时明确报 "tauri-action did not upload the updater manifest" |
| `.agents/skills/gitwave-release/SKILL.md` | 第 10 / 52 行措辞同步：latest.json 由 tauri-action 构建时生成上传，verify job 只校验；「publish 后 latest.json 生效」的行为表述不变 |
| `README.md` | 不改——"Publishing the draft also publishes latest.json" 描述的用户可见行为仍成立 |

校验门禁的存在理由：三个 build job 并行，各自结尾的清单快照只含「当时已在 release 上的 .sig」，最后完成者虽是最终态，但极端并发窗口（两个 job 几乎同时收尾、互相错过对方的 .sig）可能产出缺平台的清单——只读校验让这种静默毁掉 updater 的情形在 draft 阶段就红掉。

## 决策记录

- **（2026-08-30 第三轮后，终局）verify 门禁整体删除**：门禁三次发版零捕获、两次自己绊倒发版——第二轮因无 checkout 时 gh 找不到仓库，第三轮因 job 级 `permissions: contents: read` 使 GITHUB_TOKEN 失去 push 资格（**GitHub 的 draft release 仅对有 push 权限的凭证可见**，故 `release not found`；第一轮 workflow 级 contents:write 能下载草稿附件为对照实证）。三轮同时实证 tauri-action 清单从未缺平台。补偿：发布前人工核对 latest.json（三平台键 + 签名非空）写入 gitwave-release 技能清单。原「verify 只校验不重传」条目随之作废。

- **推翻 feat-tauri-action-release-migration 的「不用 includeUpdaterJson、保留自研 manifest」**：该决策基于自研校验更严格的假设，但实跑证明 tauri-action 反正会上传 latest.json（行为上等价于 includeUpdaterJson 默认开启），自研版本反而因资产命名假设错误成为故障点；严格性诉求由 verify 门禁承接（version 对 tag + 三平台键 + 签名非空）。
- **verify 只校验不重传**：发现问题时红掉让人工介入，而不是 CI 侧二次生成再覆盖——避免两套清单生产逻辑并存，源头唯一。
- **「不自动转正、人工 publish」约定不变**：job 改名后仍停在 draft，人工检查附件 / latest.json / release notes 后手动发布。
- **历史文档不回写**：feat-auto-update、feat-tauri-action-release-migration、F009 提案中的 publish job 描述按历史记录保留，以本文档为准。

## 验证

- 校验脚本逻辑用 v0.5.0 真实 latest.json（`gh release download` 所得）实跑通过
- `npx prettier --check` 覆盖 build.yml；workflow 语法 actionlint（如可用）
- 下一版 tag 构建时观察 Verify release job 为绿灯即为端到端验收
