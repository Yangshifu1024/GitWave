# feat-tag-release-notes

> Tag 推送触发三平台构建，全部通过后创建含全部构建物与更新记录的 Release 草稿。

## 现状对照

`.github/workflows/build.yml` 已覆盖需求三点中的两点，本次仅补第 3 点缺失的更新记录：

| 需求 | 现状 |
|---|---|
| tag 推送触发 build | 已有：`on: push: tags: ["**"]` |
| 全部 build 通过后上传构建物 | 已有：`release` job `needs: [build-macos, build-linux, build-windows]`，产物经 upload/download-artifact 附到 release |
| Release 草稿含构建物 + 版本更新 commit | `draft: true` + `files:` 已附全部产物，但 release 无 body（更新记录）← **本次缺口** |

## 改动

仅改 `.github/workflows/build.yml` release job 的 Publish 步骤，新增一行：

```yaml
generate_release_notes: true
```

由 GitHub 在创建 draft 时自动生成 release notes：相对上一 tag 的 PR 列表（What's Changed）+ Full Changelog compare 链接（链接覆盖全部 commit，含直接 push 的）。

决策记录：曾考虑本地 `git log` 全量生成（能逐条列出直接 push 的 commit）与两者结合，最终选 GitHub 自动生成 —— 样式标准（PR 链接、compare 链接），逐条 commit 可通过 compare 链接查看；后续如需切换为 git log 方案，改用 `body_path` 即可。

## 验证

- 本地：YAML 语法解析通过
- 真实验证：推测试 tag（如 `v0.2.0-test`，删除即回滚）触发 Actions，全绿后检查 Releases 页 draft —— 应含三平台构建物 + 自动生成的更新记录，确认后手动 publish
