# fix-ssh-add-agent-not-running · 修复方案

> 日期:2026-09-01 · 分支:`fix/ssh-add-agent-not-running`(worktree `../GitWave-ssh-add-agent-fix`)

## 现象

设置 → SSH 密钥,添加密钥(如 `C:\Users\<user>\.ssh\id_rsa.pub`)报错:

```
ssh-add 执行失败(退出码 2):Error connecting to agent: No such file or directory
```

同时页面底部空状态显示「ssh-agent 中没有密钥」,掩盖了 agent 根本未运行的事实。且要求用户手动启动 Windows 服务的门槛过高。

## 根因(tester 分析)

**确定原因:**

1. **环境层(直接根因)**:Windows「OpenSSH Authentication Agent」服务未运行(系统默认禁用)。`infrastructure/ssh/keys.rs` spawn 裸名 `ssh-add`,PATH 命中 `System32\OpenSSH\ssh-add.exe`,它通过命名管道 `\\.\pipe\openssh-ssh-agent` 连接 agent——服务未运行则管道不存在,报 `Error connecting to agent: No such file or directory`,退出码 2。
2. **代码层(友好提示失效)**:`add()` 的 agent 检测只匹配 Unix 经典文案(`could not open a connection` / `no agent`),Windows OpenSSH 的 `error connecting to agent: ...` 不命中,落入通用 `SSH_ADD_EXIT` 分支;项目已有的错误码 `ssh.agent.not_running`(文案含正确解法)从未被触发。
3. **误导性空状态**:`list_loaded()` 把「agent 不可达」与「agent 运行但无密钥」一律吞成空列表。

**次要隐患(同链路):**

4. 表单不拦 `.pub` 公钥——ssh-add 需要私钥,agent 修好后会报原始的 invalid format 错误。
5. 输入框 placeholder 为 `~/.ssh/id_ed25519`,但 `PathBuf::from(path)` 不展开 `~`。
6. zh `ssh.add.hint` 声称「agent 可能会通过终端提示输入」——实际 spawn 的 stdin 为 null,不可能提示,文案失实。

**约束对齐**:不违背 [ADR 0003 凭证策略](../../tech/decisions/00-overview.md)(SSH 走 ssh-agent;libgit2 的 `ssh_key_from_agent` 在 Windows 连同一系统 agent,故继续 System32 OpenSSH 生态,不改用 Git Bash 的 ssh-add)。与 `docs/tech/planning/roadmap-v0.2.md` M3 已规划的「Windows named pipe 说明/回退、报错文案改善」一致。与 `fix-fetch-gcm-credential-prompt` 中「GUI spawn 无控制终端/环境变量缺失」属同类问题模式。

## 修复方案

用户已选定:**错误分类修复 + 一键 UAC 启动服务按钮**(不静默提权)。

### 1. `src-tauri/src/infrastructure/ssh/keys.rs`

- 抽取可单测的分类函数 `is_agent_unreachable(stderr_lower: &str) -> bool`:在现有两个子串基础上增加 `"error connecting to agent"`(覆盖 Windows 文案);`add()` 与 `delete()` 共用,命中 → `AGENT_NOT_RUNNING`(passphrase 分支保持在前)。
- `list_loaded()` 改返回 `SshKeyList { agent_running: bool, keys: Vec<SshKey> }`:命令成功或 exit 1 + `the agent has no identities` → `agent_running: true`(后者 keys 为空);stderr 命中 agent 不可达 → `agent_running: false`。
- 新增 `expand_tilde(path: &str) -> PathBuf`(用 `dirs::home_dir()`,仅展开前缀 `~` / `~/`)。
- `add()` 前置校验:扩展名为 `pub` → 返回新错误码 `ssh.key.public_file`,不 spawn。
- 新增单元测试:分类函数覆盖 Windows / Unix / 无密钥 / passphrase 样例;`expand_tilde` 对绝对路径恒等、`~` 前缀展开为 home。

### 2. `src-tauri/src/domain/error_codes/infra.rs`

- 新增 `KEY_PUBLIC_FILE = "ssh.key.public_file"`、`AGENT_START_FAILED = "ssh.agent.start_failed"`。

### 3. `src-tauri/src/application/use_cases.rs` + `src-tauri/src/lib.rs`

- `list_ssh_keys` / `cmd_list_ssh_keys` 返回类型改为 `SshKeyList`;`add_ssh_key` / `delete_ssh_key` 改用 `expand_tilde`。
- 新增 `cmd_start_ssh_agent_service`(仅 Windows):经
  `powershell -NoProfile -Command "Start-Process cmd -Verb RunAs -WindowStyle Hidden -ArgumentList '/c sc config ssh-agent start= auto & sc start ssh-agent'"`
  触发一次 UAC 确认,将服务设为自动并启动。命令串固定、无用户输入;powershell spawn 失败或非 Windows → `AGENT_START_FAILED`。异步不等待提升进程结果,由前端轮询确认。

### 4. 前端 `src/lib/api.ts` + `src/components/SshKeyManager.tsx`

- api.ts:新增 `SshKeyList` 类型(serde 输出 snake_case `agent_running`)、`startSshAgentService()`。
- SshKeyManager:`agent_running === false` 时渲染「ssh-agent 未运行」警告型空状态(复用 EmptyState)+ 「启动 ssh-agent 服务」按钮:点击 → disabled「等待授权…」→ 每 2s 轮询 `listSshKeys` 至多 15s,`agent_running` 变 true 即刷新;超时提示重试 / 手动指引。

### 5. i18n(zh-CN + en 同步,过 parity 测试)

- `errors-infra.json`:两语言新增 `ssh.key.public_file`、`ssh.agent.start_failed`。
- `ssh.json`:两语言新增 `agentDown.title / description / startButton / starting / retryHint`;修正 `add.hint` passphrase 描述(带口令密钥请在终端 ssh-add,或改用无口令密钥)。

### 有意的行为变化

- `cmd_list_ssh_keys` IPC 返回结构由 `Vec<SshKey>` 变为 `{ agent_running, keys }`(前后端同 PR 内同步,无兼容负担)。
- Windows 上 agent 未运行时,SSH 密钥页从「可添加但报原始错误」变为「显示警告态 + 一键启动」。
- 添加 `.pub` 公钥由「原始 ssh-add 错误」变为明确的前置拦截提示。

## 回归测试要点

**单元测试(不触网、不依赖 agent 状态):**

- `is_agent_unreachable`:`error connecting to agent: no such file or directory`(Windows)/ `could not open a connection to your authentication agent.`(Unix)→ true;`the agent has no identities.` / passphrase 类 → false。
- `expand_tilde`:绝对路径恒等;`~` / `~/...` 展开为 home 前缀。
- 既有测试(`list_loaded_is_ok_even_without_agent` 等)随返回结构更新后仍通过。

**手动验收(真机 Windows):**

- [ ] 服务停止时:列表显示「ssh-agent 未运行」警告态 + 启动按钮;添加私钥 → 友好提示「启动 OpenSSH Agent 服务」
- [ ] 点「启动 ssh-agent 服务」→ UAC 确认后 ≤15s 内列表自动恢复;拒绝 UAC → 超时后显示重试/手动指引
- [ ] 服务运行后:添加 `id_rsa` 私钥成功、列表出现指纹、`ssh -T git@github.com` 通过
- [ ] 添加 `.pub` → 明确提示使用私钥文件
- [ ] 输入 `~/.ssh/id_rsa` → 正常展开并添加
- [ ] 带 passphrase 密钥 → 终端指引提示
- [ ] 服务设为 auto 后重启系统,agent 自启,一切正常

## 分支

`fix/ssh-add-agent-not-running`,从 `main` 拉出,在独立 worktree `../GitWave-ssh-add-agent-fix` 实施。AI 不自动 commit / push。
