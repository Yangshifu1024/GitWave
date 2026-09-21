# fix: rewrite-manifest-urls 在 draft 阶段失败

## 现象

v0.9.3 发版（tag `v0.9.3`，run 35610127064）：

- `Prepare release` / `Build (Windows|macOS|Linux)` 全部 success，draft release 里 12 个附件齐全、`latest.json` 签名非空；
- `Rewrite updater manifest URLs` 失败：日志里一条 `gh: Not Found (HTTP 404)`，随后 `Process completed with exit code 1`；
- 整体 build 工作流因此标红，draft 里的 `latest.json` 仍是 tauri-action 写的 asset-API 形态（8 个平台键全部指向 `https://api.github.com/.../releases/assets/<id>`）。**这个形态一旦 publish，就是 v0.9.2 那个 403 故障的复刻**，所以必须修。

## 复现与证据（2026-09-21，owner 权限，只读）

| 检查 | 命令 | 结果 |
|---|---|---|
| draft 存在 | `gh api repos/…/releases?per_page=5` | `v0.9.3 draft=true assets=12 id=393014569` |
| **按 tag 取 release（REST）** | `gh api repos/…/releases/tags/v0.9.3` | **404 Not Found**（两次复测均 404） |
| **`gh release download <tag>`** | 本地 gh 2.100.0 对同一 draft 执行 | **成功**（下载到的正是当时 draft 上那份 6452 字节、LF 行尾的 asset-API 形态清单） |
| **CI 里同一个 draft 上的 `gh release download`** | run 35610127064 的 `Build (macOS)` 作业日志（`gh release download "${GITHUB_REF_NAME}" --pattern '*.dmg'`） | **成功**：下载 dmg → 公证 `Accepted` → `stapler staple` → `validate` 全部通过 |
| **`gh release upload <tag>`** | 本地对同一 draft 传一个探针文件（随后删除该资产） | **成功**：新建资产 id 579262507，删除后 draft 回到 12 个附件 |
| draft 资产的 `browser_download_url` | `gh api repos/…/releases/393014569` | 12 个资产全部是 `…/releases/download/untagged-7e0657894d3614acef37/<file>` |
| 已发布 release 的地址形态 | 下载已发布的 v0.9.2 的 `latest.json` | `…/releases/download/v0.9.2/<file>`（8 个键） |
| 资产上传端点 | `gh api --method POST repos/…/releases/393014569/assets?name=…` | **404**（gh 发往 api.github.com）；`curl -X POST https://uploads.github.com/…/assets?name=…` → **201** |
| `gh api --hostname uploads.github.com` | 同上，换成该参数 | `error connecting to api.uploads.github.com`（gh 会把 host 拼上 `api.` 前缀） |
| 运行器自带 gh 版本 | `actions/runner-images` 的 Ubuntu2204-Readme（Image 20260907.292.1） | GitHub CLI **2.100.0**，与本地同版 |
| 失败日志只有一条 404 | run 35610127064 的作业 106371358338 | 脚本 dump @14:20:18.56，唯一一条 `gh: Not Found (HTTP 404)` @14:20:19.87 |

## 根因

1. **确定性 404：`gh api "repos/$REPO/releases/tags/$TAG"`（旧任务的第二行）。** REST 的「按 tag 取 release」端点在 release 仍是 draft 时返回 404。旧任务的**第一行** `gh release download "$TAG"` 本身没问题——gh CLI 不走这个 REST 路径（**推断**：它按 releases 列表匹配 tag 名，而列表对「有 push 权限的令牌」包含 draft；gh 源码未逐行核实），macOS 作业在同一个 run、同一个 draft 上用同一写法成功下载 dmg，本地 gh 2.100.0 亦成功。失败日志里只有一条 404，光看日志分不清是哪一行；上面两处反例把它钉在了第二行。
2. **draft 的 `browser_download_url` 是伪路径**：`releases/download/untagged-<hash>/<file>`，而已发布版本（v0.9.2）用的是 `releases/download/<tag>/<file>`。两者形态不一致，且 `untagged-<hash>` 是 draft 专用路径（**发布后它是否仍可下载未经验证**）——因此一律按 tag + 资产名自拼，不照抄 `browser_download_url`。
3. **替换既有资产不必绕 curl**：资产上传端点只存在于 `uploads.github.com`，`gh api` 一律打 `api.github.com` 因而 404（`gh api --hostname uploads.github.com` 会被拼成不存在的 `api.uploads.github.com`）；但 `gh release upload --clobber` 内部走的是正确的主机——macOS 作业用的就是它。

## 修复方案

改动点共三处：

| 文件 | 改动 |
|---|---|
| `.github/workflows/build.yml` | `rewrite-manifest-urls` 任务按下面 6 条重写 |
| `.agents/skills/gitwave-release/SKILL.md` | 末尾补「清单核对失败时不得发布」的红灯处置（发布流程是人/agent 照着这个技能走的，故障处置必须写在流程里，而不是只躺在 workflow 注释里） |
| `docs/tasks/fix-rewrite-manifest-draft/plan.md` | 本文件 |

任务重写的 6 条：

1. `needs` 加入 `prepare-release`，用它的 `release_id` 输出；**所有直连 API 的调用按 release id**，只有 `gh release download/upload` 继续用 tag（它们对 draft 可用）。
2. 取回清单：`gh release download "$TAG" --repo "$REPO" --pattern latest.json --clobber`（实测可用，沿用既有写法）。
3. 改写 URL：`asset id → https://github.com/<repo>/releases/download/<tag>/<asset 名>`，**由 tag + 资产名自拼**，不用 `browser_download_url`。
4. 门禁全部放在替换**之前**：① 所有 `platforms.*.url` 必须以 `<tag 下载前缀>/` 开头；② 每个 URL 的文件名必须对应一个真实存在的资产名；③ `darwin-aarch64` / `linux-x86_64` / `windows-x86_64` 三个键必须在。
5. 替换：`gh release upload "$TAG" latest.json --repo "$REPO" --clobber`——一次调用完成覆盖，没有手写 delete/curl。
6. 自证：把清单重新下载到 `mktemp -d` 出来的目录，用 `cmp -s` 与本地逐字节比较，不一致即失败。

## 如何修复 draft 的清单（recipe）

当 `rewrite-manifest-urls` 红灯、而 draft 已经建好时（本次 v0.9.3 就是这么救回来的）：

```bash
REPO=<owner>/<repo>; TAG=vX.Y.Z
REL=$(gh api "repos/$REPO/releases?per_page=20" --jq ".[] | select(.tag_name==\"$TAG\") | .id")   # draft 只能这样找，tag 端点会 404
base="https://github.com/$REPO/releases/download/$TAG"
mkdir -p fix && (cd fix && gh release download "$TAG" --repo "$REPO" --pattern latest.json --clobber)
assets=$(gh api "repos/$REPO/releases/$REL" --jq '[.assets[] | {key: (.id|tostring), value: .name}] | from_entries')
jq --argjson assets "$assets" --arg base "$base" '
  .platforms |= with_entries(.value.url = (
    (.value.url | sub("^.*/releases/assets/"; "")) as $id
    | if $assets[$id] then ($base + "/" + $assets[$id]) else .value.url end))' \
  fix/latest.json > fix/fixed.json
# 门禁：所有 url 以 $base/ 开头、文件名都是真实资产、三个主键在；再比对除 url 外无变化
# 资产管理名以「本地文件名」为准，所以务必让文件名就叫 latest.json
mkdir -p up && cp fix/fixed.json up/latest.json && (cd up && gh release upload "$TAG" latest.json --repo "$REPO" --clobber)
# 回读核对：再下载一次，确认 8 个键都是普通下载链接、signature 非空
```

坑：`gh release upload` 用**本地文件名**作为资产名——本次我先传了 `latest-0.9.3.fixed-lf.json`，在 draft 上多出一个多余资产，随后删除并按 `latest.json` 重传。

## 已知限制

- `gh release upload --clobber` 内部是先删后传：若删除成功而上传失败，draft 会短暂没有 `latest.json`。此时任务红灯，而 publish 是人工步骤，正常情况下不会有人在红灯状态下发布；但**没有任何兜底能保证 draft 始终带着清单**。
- **单作业重跑会打回旧形态**：只重跑某个平台作业（例如 `Build (Windows)`）会让 tauri-action 重新上传它的清单，把 URL 打回 asset-API 形态，需要再重跑本任务。重跑本任务请用 **Re-run all jobs**：本任务依赖 `needs.prepare-release.outputs.release_id`（build.yml:238/247），只重跑单个 job 时该输出取自上一次 attempt，为空则 `: "${RELEASE_ID:?…}"` 直接红灯（fail-safe，但看起来像修复没生效）。整体重跑安全：改写本身幂等（URL 已是 tag 形态时 `sub("^.*/releases/assets/"; "")` 匹配不到 → 保留原值 → 门禁通过）。
- 最后一步「重新下载 + `cmp`」没有重试：CDN/最终一致性抖动会让 job 变红（fail-safe，但会多一次噪声红灯）。
- 直连 `GET /releases/{id}` 的内嵌 `assets` 数组不会分页截断（用另一个 42 资产的发布版核对过），因此不需要 `--paginate`。
- 任务里保留的 `gh --version` 是有意留痕（便于事后判断是否为 gh 行为差异），不是调试残留。
- 仓库内没有针对这个任务的自动化测试：`make check` 不覆盖 workflow 文件，也没有 actionlint、没有 shellcheck（本机也没有）。CI 之外只在本地验证过，见下。

## 本次验证面（脚本都在 `.codewave/temps/`，不入库）

1. 用 python 按缩进从 YAML 抠出该任务的 run 块 → `bash -n`；并断言关键结构串存在、且「门禁 → 上传 → 自证」顺序正确。
2. **对真实 draft（v0.9.3，release id 393014569）原样跑了一遍 run 块**：打印 `gh version 2.100.0`，8 个 URL 全为 tag 形式，末行 `latest.json rewritten and verified`，退出码 0。
3. **三道门禁的 5 个夹具**（把门禁段落原样抠出来跑）：good 绿；把某个 URL 换成 `example.com` 红；把某个 URL 换成不存在的资产名红；删掉 `windows-x86_64` 键红；原始 asset-API 形态清单红（报 8 条）。
4. **改写段落夹具**：把 `jq --argjson assets` 到 `mv latest.rewritten.json latest.json` 原样抠出来，对**原始 asset-API 形态**清单执行 → 8 个 URL 全部变成 `https://github.com/<repo>/releases/download/<tag>/<资产名>`，`version` / `pub_date` / `signature` 未变（`jq -S` 归一化 diff 为空），门禁绿，结果与我手工修好并已上传的清单逐字节一致（`cmp -s`）。
5. `pnpm exec prettier --check .github/workflows/build.yml` 通过。

注：第 2、4 项是在 Windows / Git Bash 下跑的，本地 `jq` 产出 CRLF，因此当时上传到 draft 的清单是 CRLF 版；随后已归一化为 LF 重传（6596 字节、0 个 CR，JSON 语义不变）。CI 在 Linux 上产出的是 LF，两者语义等价。

## 回归测试点

- 下一次 tag 推送后：`rewrite-manifest-urls` 绿；draft 的 `latest.json` 里 `platforms.*.url` 全部是 `https://github.com/<owner>/<repo>/releases/download/<tag>/<file>`，且文件名与资产一一对应。
- `version` / `signature` 与 tauri-action 的原始产物完全一致（本任务只改 `url`）。
- 发布该 draft 后，用不带任何请求头的普通请求拉一个 `platforms.*.url`，应得 200/206 而不是 403；顺带确认 `untagged-<hash>` 形态的旧地址在发布后是否仍可用（本次没有历史样本可验）。
- 幂等：对同一 tag 重跑本任务（Re-run all jobs），结果不变、仍为绿。

## 本次发布的补救（已完成）

v0.9.3 的 draft 已手工修正：8 个 `platforms.*.url` 改写成 `releases/download/v0.9.3/<资产名>`，`version` / `signature` 未动；随后重跑本任务的 run 块做幂等演练（退出码 0），并把清单归一化为 LF 重传。draft 最终状态：`tag=v0.9.3 draft=true assets=12`，清单 6596 字节、0 CR、8 个键全为普通下载链接、signature 非空。

## 后续（本次不做）

1. 把「抠出 run 块 + 门禁夹具 + 改写夹具」落成仓库内可重跑测试，或在 CI 增加 actionlint，让 jq 表达式与三道门禁进入常规门禁覆盖范围。
2. `README.md` 的「Cutting a release」清单里补一条「publish 前核对 `latest.json`」（现已写进 `.agents/skills/gitwave-release/SKILL.md`，但 README 被该技能描述为发布清单的 source of truth，两处应对齐）。
3. 给「自证」这一步的重新下载加一次重试，减少 CDN 抖动造成的噪声红灯。
