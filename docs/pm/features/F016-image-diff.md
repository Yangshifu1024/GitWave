# F016 · DiffView 图片 diff（左右分栏对比）

## 背景

当前 diff 视图把所有文件当文本渲染：图片文件没有 hunk，只会显示「no hunk detail / 无变更内容」占位。用户修改图片资源（设计稿、图标、截图）后，无法在 GitWave 里直观看到前后差异，必须切换到外部工具对比，打断工作流。

## 提议方案

当 diff 中的文件是图片（按扩展名识别：png / jpg / jpeg / gif / webp / bmp / ico / svg）时，`FileDiffView` 渲染左右分栏的图片对比，替代 hunk 区：

- **左栏 = 旧版本，右栏 = 新版本**，每栏带「旧版本 / 新版本」小标签，图片 `object-contain` 限高显示。
- **单侧情形**：新增文件只显示右栏（左栏「新增」空态）；删除文件只显示左栏（右栏「已删除」空态）。
- 覆盖 commit diff 视图与工作副本 diff 视图（两者共用同一渲染组件）。
- 数据链路：复用 `FileDiff.old_sha / new_sha`（git blob OID），后端新增按 OID 读取 blob 字节、或回退读取工作目录文件的命令，base64 经 IPC 返回，前端以 `data:` URL 渲染（CSP 已允许 `img-src data:`）。

### 行为细节

- 单个版本超过 20 MiB 不渲染，显示「图片过大」占位。
- 图片加载失败（LFS 指针文件、损坏数据）显示「无法显示该图片」占位；LFS 支持不在本期范围。
- 纯只读操作：不涉及 git 状态修改，不违背「AI 不自动 commit/push/merge」原则。

## 影响

- 涉及模块：DiffViewer（前端）、Tauri command / git2 blob 读取（后端）、i18n（en / zh-CN）
- 影响版本：0.7.x
- 是否破坏向后兼容：纯新增功能，无破坏性变更

## 决策

- 状态：接受
- 决策人：用户
- 决策日期：2026-09-13
- 关联决策：无（独立于 F014 / F015 的外部打开能力，属 diff 渲染增强）
