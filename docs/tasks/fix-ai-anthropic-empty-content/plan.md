# fix: AI generate 报 "anthropic returned empty content"

状态：已修复

## 现象（用户真机）

Anthropic provider + 智谱 `https://open.bigmodel.cn/api/anthropic` + `glm-5.3-flash`，generate commit message 报 `Unknown: unknown error: anthropic returned empty content`。前一报错 `1214 modelCode 不存在`（model 带了 Claude Code 私有的 `[1m]` 后缀）已由用户改配置解决，本条是其后继。

## 根因

`provider.rs` 的 `anthropic_chat` 只取 `content[0]["text"]`。GLM 5.x 是混合思考模型，Anthropic 兼容端点返回的 content 数组首块是 `{"type":"thinking","thinking":...}`（无 `text` 字段），真正的回答在其后的 text 块中 → 首块取不到 text 被误判为空。

## 修复

`src-tauri/src/infrastructure/ai/provider.rs`：

- 新增 `anthropic_content_text(&Value)`：遍历全部 content 块，仅拼接 `text` 字段（thinking 块天然跳过），trim 后为空则 None
- `anthropic_chat` 改用该函数；仍为空时报错附 `stop_reason` + 截断 200 字符的原始 content JSON，便于后续诊断
- 新增 `mod tests` 回归用例：纯 text 块 / thinking 块在前 + 多 text 块拼接 / 仅 thinking 块与无 content 字段返回 None

## 验证

- `cargo test`（含新用例）、`cargo clippy --all-targets`、`cargo fmt --check`
- 真机：用户重试 generate commit message
