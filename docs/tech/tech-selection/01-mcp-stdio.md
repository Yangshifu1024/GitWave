# MCP Server 选型与范围记录(gitwave-mcp)

> 状态:已实现(2026-08-29,v0.3 M4)。对应 roadmap:`docs/tech/planning/roadmap-v0.3.md` M4。

## 上下文

v0.3 计划包含"内置 MCP server(只读)"。Rust 生态的 MCP SDK(rmcp / 官方
rust-sdk)在规划时仍处于 0.x 快速演进期,而 GitWave 需要的只是 stdio 传输下的
最小工具子集。

## 备选

1. **引入 rmcp / 官方 rust-sdk**:协议合规免费,但引入仍在不稳定版本的依赖,
   且我们的工具都是"打开仓库 → 调 infrastructure 纯函数 → 返回 JSON 文本",
   SDK 的能力(资源订阅、sampling、transports)大多用不上。
2. **手写 stdio JSON-RPC(已选)**:MCP stdio 传输即换行分隔的 JSON-RPC 2.0,
   只需实现 initialize / tools/list / tools/call 三个方法与通知忽略,
   零新增依赖(tauri crate 已带 serde_json)。

## 决策

手写最小协议层(`src-tauri/src/bin/gitwave-mcp.rs`),协议版本锁定
`2024-11-05`。若后续需要 SSE/HTTP 传输或资源订阅,再评估迁移 SDK——
`call_tool` 与协议层已分离,迁移面仅限消息编解码。

## 范围记录

- **只读承诺(P1 对齐)**:仅有 repo_status / commit_history / list_branches /
  list_tags / list_remotes / read_file 六个查询工具;`read_file` 经
  canonicalize 强制留在工作树内并拒绝 `.git` 内部文件(config 可能含带凭证的
  remote URL)。
- **范围缩水**:roadmap 原列 `get_diff` / `blame` 两个工具未随 M4 交付
  (diff 有 staged/unstaged 两种形态需要先定义协议参数),挂起至有真实客户端
  需求时补充。
- **分发缺口**:Tauri bundle 只打包主应用,`gitwave-mcp` 二进制不随
  deb/appimage 安装——当前通过源码构建使用;随安装包分发挂后续。

## 接入配置(客户端侧)

```jsonc
// Claude / Cursor 的 mcpServers 配置
{
  "mcpServers": {
    "gitwave": {
      "command": "<path-to>/gitwave-mcp",
      "args": ["--repo", "<repo 工作树路径>"]
    }
  }
}
```

## 后果

+ 零依赖、可审计、升级不被 SDK 版本绑架
+ 只读边界以代码结构保证(没有写工具可调用)
− 协议演进需手动跟进(spec 变更时检查三个方法是否受影响)
− 手写实现需自带回归测试(已有 initialize/tools_list/调用/逃逸用例)
