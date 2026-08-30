# fix-ci-redundant-publish-job · code review

- 审查日期：2026-08-30
- 审查对象：分支 `fix/ci-redundant-publish-job` 未提交改动（基线 `main`）
- 涉及文件：`.github/workflows/build.yml`、`.agents/skills/gitwave-release/SKILL.md`、`docs/tasks/fix-ci-redundant-publish-job/plan.md`（未跟踪）
- 审查方法：git diff 逐行审查 + js-yaml 解析渲染 run block 验证 heredoc 落位 + 对照 tauri-action `action-v1.0.0` 源码（`src/upload-version-json.ts`）核实清单合并行为 + 全仓 grep 排查旧 job 名残留引用 + 版本链（package.json / tauri.conf.json = 0.5.0）核对。脚本实跑（v0.5.0 真实 latest.json 正例 + 删 darwin 键负例）、prettier 通过已另行验证；actionlint 本机未安装（`command -v` 确认），plan.md 第 47 行「如可用」的措辞与此一致。

## 1. 正确性 — 通过

**needs / 触发 / 环境**：`verify-manifest` 的 `needs: [prepare-release, build-macos, build-linux, build-windows]`（build.yml:157）语义正确——join 全部 build 保证拿到的是最终态清单（而非某 job 中途的快照），needs prepare-release 保证 draft release 已存在。`GITHUB_REF_NAME` 在 tag push 触发下即 tag 名，`.replace(/^v/, "")` 与 prepare job 的 `"v" + package.json version` 断言约定一致。ubuntu-22.04 runner 上 `gh` 与 `node` 均预装，且旧 publish job 用同样的免 setup 写法在 v0.5.0 真实跑过（失败点在正则，非工具缺失），有生产实证。

**删除 checkout 后无工作区依赖**：脚本只读 `manifest/latest.json` 与 env，版本对比对象从 package.json（需 checkout）改为 tag 名，删除 checkout 后无残留依赖。

**失败路径逐一核红**：
- draft 上无 latest.json：`gh release download` 无匹配资产时报错退出非零；即便 gh 静默，`fs.existsSync` 兜底 throw 显式信息「tauri-action did not upload the updater manifest」。双防线，均红且可读。
- version 不匹配 / 缺平台键 / 空 signature / 空 url：显式 throw，均红。
- JSON 损坏：`JSON.parse` SyntaxError，红且带位置信息。

**承重论断核实（关键）**：plan.md 「tauri-action 在构建阶段就会合并 release 上既有的全部 .sig 生成清单，最后完成的平台 job 产出最终态」——已对照 tauri-action `action-v1.0.0` 源码证实：`uploadVersionJSON` 先 `listReleaseAssets` 并下载既有 latest.json 起步合并 `platforms`（upload-version-json.ts:57-86），最后 delete + re-upload 覆盖（:276-300）。由此确认两个结论成立：(a) 并发 read-modify-write 窗口（两 job 几乎同时收尾互相错过对方 .sig）真实存在，verify 门禁恰好是它的兜底，plan.md 的残余风险表述准确；(b) 生成的清单含 `darwin-aarch64-app` 等后缀键，updater 按 `{target}-{arch}` 精确键命中、后缀键无害。

**版本链**：prepare 断言 `tag === v + package.json.version`，verify 断言 `manifest.version === tag 去 v`（version 源自 tauri.conf.json 经 tauri 构建），两条门禁传递性 cross-check 了 package.json 与 tauri.conf.json 的版本同步——比改动前仅单向断言更强。

**🟡 建议修：`manifest.platforms` 整体缺失时走 TypeError 分支**
- 位置：build.yml 校验脚本 for 循环处
- 描述：若 latest.json 是合法 JSON 但没有 `platforms` 字段（最可能的现实来源是有人手工往 draft 传了残缺清单），此处抛 `TypeError`——结果仍是红，但绕过了门禁精心设计的可读报错。
- 建议：for 循环前加 `if (!manifest.platforms) throw new Error("manifest has no platforms object");`

## 2. 安全性 — 通过

- heredoc 用 `<<'NODE'`（带引号），脚本内的模板字面量 `${...}` 与反引号不会被 shell 展开（js-yaml 渲染后 `NODE` 终止符 strip 至行首，已逐行验证）；shell 侧 `${GITHUB_REF_NAME}` 已加引号，无注入面。
- 脚本只做字段存在性/相等性比较，不执行清单内容、不回显 signature 内容，无敏感信息泄露。
- draft release 读取权限：`GH_TOKEN: ${{ github.token }}` 足够读取 draft，该访问模式在 v0.5.0 已被旧 publish job 生产验证。

## 3. 性能 — 通过

门禁只增加一次 gh API 下载（数 KB JSON），叠加在 ~45 分钟的三平台构建之后，开销可忽略；删除 checkout 反而略省。

## 4. 可维护性 — 通过

净删约 33 行，删除了本次事故根因所在的「正则锚定 bundler 本地产物名」逻辑，清单生产源头唯一（tauri-action），CI 侧只留无状态校验——符合 plan.md「只校验不重传、避免两套生产逻辑并存」的决策。全仓 grep 确认无旧 `Publish release` job 名的活性残留（历史任务文档按 plan.md 决策保留，README.md:85 的用户可见行为描述「publish 同时发布 latest.json」在改动后仍为真，不改正确）。

**🟢（可选）脚本内联在 YAML 中不可单测**
- 描述：校验逻辑只能靠下一个真实 tag 端到端验收（plan.md 已如此约定），无法进常规测试流水线。
- 建议：若未来逻辑变复杂，可抽到 `.github/scripts/verify-latest-json.mjs` 以获得 lint/单测能力；当前体量内联保持 workflow 自包含，属合理取舍。

## 5. 可读性 — 通过

门禁存在理由的注释讲清了 why；各失败信息（缺文件/version/平台/签名）均可直接指导排障；成功日志列出实际生效的平台键。

**🟢（可选）成功日志会包含后缀键**
- 描述：tauri-action 会写入 `darwin-aarch64-app` / `windows-x86_64-nsis` 等后缀键，日志会列出超过 3 个键，初看可能与断言的三主键混淆（无害，仅信息性）。

## 6. 测试覆盖 — 通过（有已知边界）

正例（v0.5.0 真实 latest.json）与负例（删 darwin 键）已实跑验证；缺文件路径依赖 gh 行为，下一版 tag 构建时顺带观察一次真实绿灯（plan.md 已列为端到端验收）。无自动化测试属内联脚本的固有边界，见 4 中可选项。

## 7. 最佳实践 — 通过

**🟢（可选）verify job 收紧权限并加超时**
- 描述：job 继承 workflow 级 `contents: write`，但本 job 只需读；且未设 `timeout-minutes`（build job 均为 45）。
- 建议：job 级加 `permissions: contents: read` 与 `timeout-minutes: 10`，实现最小权限并防挂死。

**🟢（可选）ubuntu-22.04 镜像临近弃用**
- 位置：build-linux 与 verify-manifest 的 `runs-on`（main 既有写法，非本次 diff 引入）
- 描述：GitHub 官方公告 ubuntu-22.04 镜像 2026-09-17 起进入 deprecation，2027-04-17 完全停止支持。近期值得整体迁到 ubuntu-24.04（需一并验证 libwebkit2gtk-4.1 等 Linux 依赖在 24.04 的包名）。
- 参考：https://github.com/actions/runner-images/issues/14254

**🟢（可选）SKILL.md 校验清单措辞漏「url 非空」**
- 描述：与脚本实际行为相比少一项，措辞轻微不同步（不影响正确性）。

其余一致性核对均通过：SKILL.md 两处措辞与 workflow 实际行为及「草稿转正后 `releases/latest/download/latest.json` 路由才生效」（latest 路由不含 draft）逐点吻合；plan.md 引用 `tauri.conf.json:59` 行号精确，endpoint 资产名与 tauri-action 写死的 `latest.json` 文件名一致；`createUpdaterArtifacts: true`（tauri.conf.json:46）确证 build job 会产出清单。

## 复发记录（2026-08-30 · 第二轮）

PR #9 合入后重打 tag 首跑，Verify release 挂：`failed to run git: fatal: not a git repository`。根因是 review 漏网：**无 checkout 的 job 里 `gh` 靠 cwd 的 git remote 推断仓库**，空工作区无 `.git` 直接死。审查时核对了 gh/node 预装与 token 权限，没推演「删 checkout 后工具对工作区的隐式依赖」这一层。

修复：step env 加 `GH_REPO: ${{ github.repository }}`（显式指定，一行）。修复后新草稿 v0.5.0 的 latest.json 已用与 CI 逐字相同的门禁脚本本地验证通过。

**教训（供后续 review 检查单）**：删 checkout / 改 job 工作区布局时，必须过一遍所有 step 工具的隐式假设——cwd 内容、git remote、env、镜像预装，缺一即显式补齐。

## 复发记录 · 终局（2026-08-30 · 第三轮）

**现象**：run 33307123595（sha 0ab5cd4，PR #10 修复后）三平台构建全绿，Verify release 报 `release not found`。

**真根因**：第二轮审查修复时按 🟢 建议加的 job 级 `permissions: contents: read`——GitHub 规定 **draft release 仅对有 push 权限的凭证可见**，`contents: read` 让 GITHUB_TOKEN 失去该资格，`gh release download` 找不到草稿。对照实证：第一轮旧 publish job 用 workflow 级 `contents: write` 时能正常下载草稿附件。

**误判更正**：此前一度怀疑「verify 在 builds 结束前抢跑、needs 被违反」。API 精确时间戳证伪：builds 10:42:39–10:47:42（缓存全热后每平台仅 3–5 分钟），verify 10:47:44 启动——最后一个 build 完成后 2 秒，needs 语义正常。教训：下结论前先取 API 的 per-job started_at/completed_at，不要用相对推算；同理 `git rev-parse <tag>` 对 annotated tag 返回的是 tag 对象 SHA（62cd601）而非 commit，不是异常。

**决定**：verify-manifest job 整体删除（用户确认方向）。门禁三轮零捕获、两次绊倒发版，且理论收益（tauri-action 并发合并窗口缺平台）从未兑现；tauri-action 清单三轮全部完整。补偿：gitwave-release 技能发版链路加入「publish 前核对 latest.json 三平台键 + 签名非空」的人工步骤。

**验证**：build.yml 删除后 js-yaml 解析 4 job 结构正确、prettier 全绿、全仓无 verify-manifest 残留；现有 draft v0.5.0（12 assets + latest.json，构建自 main 顶端）的清单本地核验通过，直接人工发布，无需第四轮 CI。

**教训（review 检查单追加）**：① 权限收紧类改动必须核对被访问资源的可见性规则（draft release ↔ push 权限）；② 删 checkout 后逐工具排查隐式依赖（第二轮教训）；③ CI 结论以 API 时间戳为准，不以推测拼时间线。

## 📝 总体结论

**CLEAN**（无 🔴；1 个 🟡、5 个 🟢）。

改动方向正确且必要：删掉了本次事故根因（正则锚定资产命名假设）所在的冗余生成逻辑，交给有生产实证的 tauri-action，同时用只读门禁补回「清单缺平台静默毁掉 updater」的防线，残余风险（并发合并窗口）表述与 tauri-action 源码行为核对一致。

## 修复落地（审查后同日）

- ✅ 🟡 `platforms` 存在性守卫：已加 `if (!manifest.platforms) throw new Error("manifest has no platforms object")`，负例实跑确认报错可读
- ✅ 🟢 job 级 `permissions: contents: read` + `timeout-minutes: 10`：已加
- ✅ 🟢 SKILL.md 措辞补「url 非空」：已改为「signature/url 非空」
- ⏸ 🟢 抽脚本为独立文件：当前体量维持内联，逻辑变复杂时再抽
- ⏸ 🟢 成功日志含后缀键：信息性无害，保留（全键列表反而便于排障）
- ⏸ 🟢 ubuntu-22.04 → 24.04：超出本任务范围（涉及 Linux 构建依赖包名验证），另开任务跟进

修复后回归：prettier 三文件全绿；校验脚本从 build.yml 原样抽出实跑——正例（v0.5.0 真实清单）绿灯、负例（无 platforms 对象 / 缺 darwin 键）均如期红且报错可读。
