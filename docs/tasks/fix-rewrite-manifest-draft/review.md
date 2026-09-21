# review: fix-rewrite-manifest-draft

审查对象：`.github/workflows/build.yml` 的 `rewrite-manifest-urls` 任务（重写）、`docs/tasks/fix-rewrite-manifest-draft/plan.md`（新增）、`.agents/skills/gitwave-release/SKILL.md`（末尾补红灯处置）。
基线：`main` = `5b7ba72`；审查分支 `fix/rewrite-manifest-draft`。

## 第一轮：code-reviewer（结论「不通过」，2 条 🔴）

**审查方法**：逐行读 diff；本地 `bash -n` + `prettier --check`；把 run 块抠出成脚本，配真实 `jq` + 替身 `gh`/`curl` 跑 6 个场景；独立构造清单夹具验证 jq 与门禁；线上只读核对（draft、失败 run 与作业日志、macOS 作业日志、v0.9.2 已发布形态、`--hostname uploads.github.com` 行为、runner 镜像文档）。

**它的主要发现（全部有效）**

| 级别 | 问题 | 位置 |
|---|---|---|
| 🔴 | plan.md 把「CI 里 `gh release download` 恒 404」写成机制结论，但同一 run 的 macOS 作业在**同一个 draft** 上用 `gh release download` 成功了 → 结论被证伪 | plan.md:29 |
| 🔴 | `code=$(curl …)` 在 `set -e` 下对传输层失败（TLS 超时等）不会重试，命令替换的非零退出会直接终止脚本 → 「3 次重试」名不副实 | build.yml:300-312 |
| 🟡 | 门禁只断言 3 个平台键，未覆盖实际 8 个键与「URL 指向不存在文件」 | build.yml 门禁段 |
| 🟡 | 残留窗口（先删后传）未文档化；单作业重跑会打回旧形态未说明 | plan.md |
| 🟡 | 自证步骤用刚上传后的 `uploaded_id` 重查，API 列表滞后会造成假红灯 | build.yml |
| 🟡 | `SKILL.md` 缺清单核对失败时的处置口径 | SKILL.md |
| 🟡 | 仓库内无该任务的回归测试，也无 actionlint | — |
| 🟢 | `jq -S` 双侧同时解析失败会假通过；`gh api <整 URL>` 可行性未记录；若干措辞 | 多处 |

## 第一轮遗留项的处理

| 遗留项 | 处理 | 证据 |
|---|---|---|
| 🔴 根因措辞 | **接受并重写**：确定性 404 归到旧任务第二行 `gh api repos/$REPO/releases/tags/$TAG`（REST tags 端点对 draft 404，owner 令牌两次复测 404）；`gh release download/upload` 对 draft 可用（macOS 作业 + 本地实测 + 探针上传）；并把 runner 镜像自带 gh = 2.100.0（与本机同版）写进证据表，排除「版本差异」假说 | plan.md 证据表 / 根因 1 |
| 🔴 curl 不重试 | **实现改了**：删掉 delete+curl，替换改为 `gh release upload "$TAG" latest.json --repo "$REPO" --clobber`（macOS 作业本来就这么用，内部走 uploads.github.com）；curl 一删，该缺陷随之消失 | build.yml 替换步骤 |
| 🟡 门禁扩键 | 扩成三道：① URL 前缀必须是 `<tag 下载前缀>/`；② 每个 URL 的文件名必须对应真实存在的资产名；③ 三个主平台键必须在。全部在替换之前 | build.yml 三道门禁 |
| 🟡 残留窗口/重跑 | 写进 plan.md「已知限制」：`--clobber` 先删后传的窗口、单平台作业重跑会打回 asset-API 形态、重跑本任务请用 Re-run all jobs（`release_id` 取自上一次 attempt，为空则 fail-fast） | plan.md「已知限制」 |
| 🟡 自证假红灯 | 改为上传后 `gh release download … -D "$(mktemp -d)"` + `cmp -s` 逐字节自证；下载无重试的噪声风险记入「已知限制」 | build.yml 自证步骤 |
| 🟡 SKILL.md 口径 | 已补，并在第二轮后拆成「URL 不是下载链接/任务红灯 → 复刻 403」与「缺主键 → 该平台静默停更」两种后果 | SKILL.md 末段 |
| 🟡 无回归测试/actionlint | 本次不做，列入 plan.md「后续」第 1 条 | plan.md「后续」 |
| 🟢 各项 | `gh --version` 加注释说明是有意留痕；`GET /releases/{id}` 内嵌 assets 不分页截断写入「已知限制」；「gh 走 releases 列表」标注为推断；自证改为 `cmp` | build.yml / plan.md |

## 第二轮审查

原本计划继续用 code-reviewer 复审修订版，但该角色连续两次因基础设施错误失败（`E_SUBAGENT: 认证失败：Upstream request failed: [server_error] Upstream response was not valid JSON (HTTP 403)`），因此改用 `reviewer`（方案对齐）角色复审，**如实记录这次角色替换**。

**结论：通过（方案对齐）。无 🔴，无方案外代码改动。** 它自行复现并确认：`/releases/tags/v0.9.3` 404、draft 资产 `untagged-<hash>` 伪路径、`--hostname uploads.github.com` 拼成 `api.uploads.github.com`、runner gh 2.100.0、三道门禁夹具行为、改写幂等、失败作业日志只含一条 404。

它给出的 8 条 🟡/🟢（均为文档一致性）与处理：

| # | 问题 | 处理 |
|---|---|---|
| 1 | plan.md 把 6635 B 当成「draft 下载可用」的证据，实际原始清单是 6452 B（6635 B 是修复后的 CRLF 版） | 已改为 6452 B（且说明当时下载到的正是 asset-API 形态清单） |
| 2 | 「publish 后失效」属未验证断言 | 降级为「与已发布版本地址形态不一致，且 `untagged-<hash>` 是 draft 专用路径，发布后是否可用未验证，故一律自拼」 |
| 3 | 重跑限制没写清 `release_id` 来源 | 「已知限制」补 Re-run all jobs 的要求与 fail-fast 行为 |
| 4 | plan.md 低估验证面 | 新增「本次验证面」5 条（结构断言、真实 draft 演练、门禁 5 夹具、改写夹具逐字节比对、prettier），并注明脚本仅在 `.codewave/temps/` 不入库 |
| 5 | SKILL.md 把两类故障混成一句 | 已拆开表述（403 破损 vs 该平台停更） |
| 6 | SKILL.md 指向的「repair recipe」在 plan.md 里不存在 | plan.md 新增「如何修复 draft 的清单（recipe）」小节，含可直接执行的命令与「资产名以本地文件名为准」这个坑 |
| 7 | SKILL.md 改动属方案外 | plan.md 增「改动点」表，写明三处文件与为何把处置写进技能 |
| 8 | 演练在 Windows/Git Bash 下进行，上传产物为 CRLF | plan.md 注明；并已把 draft 上的清单**归一化为 LF 重传**（6596 B、0 CR，JSON 语义不变） |

顺带发现并要求记录的坑（已在 recipe 中写明）：`gh release upload` 用本地文件名作为资产名，我第一次归一化后误以 `latest-0.9.3.fixed-lf.json` 上传，在 draft 上多出一个多余资产，随即删除并按 `latest.json` 重传（draft 回到 12 附件）。

## 合并前的最终验证（本轮执行）

| 项 | 结果 |
|---|---|
| 从 YAML 抠出 run 块 + `bash -n` + 结构串/顺序断言 | 通过 |
| 对真实 draft（release id 393014569）原样演练 run 块 | 退出码 0，`latest.json rewritten and verified` |
| 三道门禁 5 个夹具（good / 坏主机 / 坏文件名 / 缺键 / asset-API 形态） | good 绿，其余 4 个红且错误信息正确 |
| 改写段落夹具（asset-API → tag 形式） | 8 个 URL 全改对，`version`/`pub_date`/`signature` 未变，逐字节等于已上传的修正版 |
| `pnpm exec prettier --check .` | 通过 |
| draft 终态 | `tag=v0.9.3 draft=true assets=12`；清单 6596 B、0 CR、8 键全为 `releases/download/v0.9.3/<资产名>`、signature 非空 |

## 结论

**通过（可提交 / 可提 PR）。** 第一轮 2 条 🔴 均已闭合（根因重写 + 实现改为 `gh release upload --clobber`），第二轮无 🔴；剩余 🟡 均为文档，已逐条处理，其中「仓库内回归测试 / actionlint」与「README 发版清单补核对步骤」作为后续事项记录在 plan.md。
