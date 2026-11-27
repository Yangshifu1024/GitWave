# 从 GitWave 创建 GitHub PR

[产品路线图第 10 项](../../pm/core/03-roadmap.md#25-后续功能优先级)

## 使用流程

在 PR 描述窗口编辑标题和正文，选择「核对 GitHub PR」查看 origin 对应的仓库、当前 head 分支和短 SHA。可修改 base 分支。选择创建后，成功结果显示本次 PR 的 URL，点击即可打开。用户也可直接复制草稿。

## 依赖与失败处理

需已安装官方 GitHub CLI（`gh`），并以有权创建 PR 的账号登录 github.com。仓库须有指向 github.com 的 origin。head 是当前检出的本地分支，base 是同一远端仓库中的另一分支。

确认时重新读取本地目标。先通过 `gh api` 查询 origin 仓库的同名 head 分支，仅当其远端 SHA 与已核对的本地 SHA 一致时才创建。将 head、base、标题和多行 Markdown 正文序列化为 JSON，经标准输入传给 `gh api --method POST --input -`。使用响应中的 PR 编号构造链接。草稿在失败后可继续编辑。

## 验证

后端测试使用临时 Git 仓库和注入的模拟 API 验证远端 SHA、创建请求的 JSON、成功链接、目标切换和 origin 校验。前端测试通过模拟 IPC 验证关闭草稿、失败后保留编辑、再次提交及点击成功链接。测试不发布远端 PR。GitLab MR、Issue 关联和轻量评审单独排期。
