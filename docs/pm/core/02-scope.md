# GitWave · 范围与优先级

> 本文档定义功能优先级与各版本范围。
> §1–§2 是规划基线（写于 v0.1 之前）；§3 是截至 **v0.9.4** 的交付对照，不以规划原文当待办。

## 1. 范围优先级

### 1.1 必须（Must，14 项）

**Workspace 操作（4 项）**

| 编号 | 场景 | 验收标准 |
|---|---|---|
| W1 | Workspace CRUD（创建 / 打开 / 重命名 / 删除） | UI 弹窗 + switcher |
| W2 | 添加 repo（init 不自动 commit / clone HTTPS 走 git credential helper / clone SSH 走配置 key / 本地） | file picker + clone UI + init 流程；URL 自动识别协议；进度可见 / 错误明确 / 可重试 |
| W3 | 从 Workspace 移除 / 重新链接 repo | 确认对话框 + missing 检测 |
| W4 | Workspace 内切换 repo + 多 Workspace 切换 | 侧边栏点击切换主视图，单 active 模型 |

**Git 核心能力（6 项）**

| 编号 | 场景 | 验收标准 |
|---|---|---|
| 5 | clone / fetch / push / pull / branch / merge / 普通 rebase | 全套基础操作 |
| 6 | interactive rebase 拖拽 | pick / reword / edit / squash / fixup / drop |
| 7 | history 图 + 文件 diff + blame | 大仓 history 流畅，character-level diff |
| 8 | worktree 创建 / 切换 | UI 可视化 |
| 9 | stash（保存 / 应用 / 弹出 / diff） | 全套 |
| 10 | SSH key 管理（clone SSH 的前置） | 一键 add / test / 删除；列出已加载 key；测试连通性 |

**AI 协作能力（2 项）**

| 编号 | 场景 | 验收标准 |
|---|---|---|
| 11 | commit + AI 生成 message | BYOK + Ollama 都可，生成后必可编辑，多 commit 累积理解 |
| 12 | AI provider 配置 UI（Workspace-scoped） | OpenAI / Anthropic / Ollama 三选 |

**冲突解决（1 项）**

| 编号 | 场景 | 验收标准 |
|---|---|---|
| 13 | 3-way merge + AI conflict explain | 解释在面板里，不直接改代码（确定性引擎 + AI 双轨） |

**UI 能力（1 项）**

| 编号 | 场景 | 验收标准 |
|---|---|---|
| 14 | AI command palette | Cmd+K 自然语言驱动 Git 操作（**v0.1 may-slip → v0.2**；见 §2.1） |

### 1.2 应该（Should，6 项）

| 编号 | 场景 | 验收标准 |
|---|---|---|
| S1 | submodule init / update | 嵌套可视 |
| S2 | .gitignore 编辑器 | 语法高亮 |
| S3 | tag + annotated tag | 操作简洁 |
| S4 | PR 创建（GitHub） | 含 AI 描述生成 |
| S5 | 主题切换（light / dark / follow system） | 实时 |
| S6 | Workspace 导入 / 导出 | `.gitwave-workspace.json` |

### 1.3 可以（Could，后续版本，4 项）

| 编号 | 场景 | 备注 |
|---|---|---|
| N1 | Git LFS | 后续 |
| N2 | reflog 浏览器 + 误操作恢复（基础；AI 增强） | 后续 |
| N3 | hooks 编辑器 | 后续 |
| N4 | AI repo health | 后续 |

---

## 2. 版本范围与完成定义

### 2.1 v0.1（首个可用版本）

**范围**：见 §1.1 必须 14 项

**完成定义**：

- 14 个 must-have 全部可用（**例外**：项 14 AI command palette 已标注 may-slip，v0.1 可不交付，顺延 v0.2）
- 三个核心场景（commit → push、conflict 解决、Workspace 切换）端到端可走通
- 内部 dogfooding 时间合理，主要场景无崩溃
- AI 行为符合 P1（永不自动 commit / push / merge）

### 2.2 v0.2

**新增范围**：

- Windows 适配
- Git LFS / 完整 submodule / reflog 浏览器 / hooks 编辑器
- AI PR 描述 / AI history 解释 / AI command palette（Cmd+K）
- 自定义 prompt template UI（commit / conflict / PR 三套）
- Per-repo AI rules（`.gitwave/` 目录）
- Provider 故障转移

**完成定义**：

- macOS + Windows 同时可用
- v0.1 全量功能在 Win 上跑通
- AI 进阶能力端到端可用
- 用户能从 v0.1 平滑升级到 v0.2

### 2.3 v0.3

**新增范围**：

- AI repo health dashboard
- AI 误操作恢复（reflog 语义化 + 恢复建议）
- 协作（remote / PR / lightweight review）
- Linux 稳定版

**完成定义**：

- 三平台可用
- AI 智能能力端到端可用
- 协作能力走通

---

## 3. 截至 v0.9.4 的交付对照

规划原文不改；本表说明每一项现在是否已在产品里。

### 3.1 Must（§1.1）

全部 14 项已交付，含原 may-slip 的项 14（AI command palette，随 v0.2）。

### 3.2 Should（§1.2）

| 编号 | 场景 | 状态 |
|---|---|---|
| S1 | submodule init / update | **已交付**（含 add / recursive update / deinit / status） |
| S2 | .gitignore 编辑器 | **已交付**（纯文本编辑；无语法高亮） |
| S3 | tag + annotated tag | **已交付** |
| S4 | PR 创建（GitHub） | **部分**：AI 生成可编辑 title / body + Copy；**不**调 GitHub API 建 PR |
| S5 | 主题切换（light / dark / follow system） | **已交付**（另有 Native Blue / Tide 配色） |
| S6 | Workspace 导入 / 导出 | **已交付**（`.gitwave-workspace.json`） |

### 3.3 Could（§1.3）

| 编号 | 场景 | 状态 |
|---|---|---|
| N1 | Git LFS | **已交付** |
| N2 | reflog 浏览器 + 误操作恢复 | **已交付**（时间线 + 确认后 reset / 建恢复分支 + AI 解释） |
| N3 | hooks 编辑器 | **已交付**（只编辑，不执行；与 libgit2 边界一致） |
| N4 | AI repo health | **已交付** |

### 3.4 版本完成定义

| 版本 | 规划完成定义 | 现状 |
|---|---|---|
| v0.1 | 14 must-have（palette 可顺延）、三核心场景、P1 | **已交付** |
| v0.2 | 双平台 + AI 进阶 + 平滑升级 | **已交付** |
| v0.3 | 三平台 + AI 智能 + 协作走通 | **已交付**（协作 = remote 管理；PR/MR 仍未做） |

### 3.5 v0.8.8–v0.9.4

v0.3 之后按功能提案合入、tag 发版；下表按版本号列出代表性交付。

| 版本 | 交付内容 |
|---|---|
| v0.8.8 | 自动刷新全部仓库（一次刷新工作区里的每个仓库，间隔可在设置里配置，默认 5 分钟）；前端工具链升级到 Vite 8.3.0 / TypeScript 6 |
| v0.9.0 | 一次添加多个本地仓库（添加对话框可多选文件夹；跳过已加入的路径与本批内的重复；不可用的路径会报告，但不中断整批） |
| v0.9.1 | 仓库标签栏溢出时用鼠标滚轮横向滚动，并把激活标签带回视野 |
| v0.9.2 | stash 面板重做（每条 stash 常驻一排带标签的操作按钮 View / Apply / Apply & delete / Delete；详情窗口把文件列表与 diff 并排；应用前检查工作区是否有未提交改动；丢弃前确认；以未跟踪方式保存的文件也出现在 stash 内容里） |
| v0.9.3 | 提交列表在大仓库滚动时保持流畅（削减每次滚动的重渲染开销）；所有面板的时间格式统一走一个共享格式化函数；更新清单改用普通下载链接（修掉 v0.9.2 的更新失败）；前端工具链的 eslint js 升到 10、图标库升到 lucide 1.x |
| v0.9.4 | 未解决的 index 冲突即使没有 MERGE_HEAD 也显示在「变更」计数并可直接进入冲突解决面板；pull 在 stash / fetch 前检查冲突与未完成的 Git 操作，给出明确提示 |
