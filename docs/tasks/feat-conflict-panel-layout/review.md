# review — feat/conflict-panel-layout

审查方式：AGENTS code-reviewer 代理（7 维度），审查对象为提交前的工作区全量 diff。
审查结论：**无 🔴 严重问题；6 个 🟡 已全部修复；🟢 采纳 4 项、暂缓若干**（见下）。

## 🟡 问题与修复

| # | 问题 | 修复 |
|---|---|---|
| 1 | merge 在面板打开期间被外部结束时，`open` 仍为 true、内部状态不清；下次 merge 会带旧 editor 复活面板 | auto-close effect 增加 `open && !active → onClose()`（reset effect 随之清理），并在 reset 中补 `setError(null)` |
| 2 | 面板关闭后迟到的异步回调（openFile / Explain / resolve / abort）会用旧数据复活已清空状态 | 引入 `openRef` 代际守卫，所有 await 续体在 setState 前检查 `openRef.current` |
| 3 | `gotoHunk` 先设 `scrollTop` 再 `focus()+setSelectionRange`，Chromium 会把 selection 末端滚进视口覆盖显式滚动 | 调整顺序：focus → selection → 最后赋 `scrollTop` |
| 4 | hook 的 `onError` 参数无调用方传入（死参数），轮询失败全静默 | 删除该参数；注释明确"轮询失败静默、下一 tick 重试"是约定。核查确认旧版同样不向用户展示轮询错误（旧面板 `!active` 提前 return 在 ErrorAlert 之前），非行为回归 |
| 5 | Escape / ✕ 无确认丢弃未保存的手工合并编辑 | 新增 `seedRef` 脏检查 + `requestClose`：内容偏离播种版本时弹 `Modal` 确认（Keep editing / Discard edits）；Use ours / Use theirs 同步更新 seed。自动关闭（最后一个冲突已 resolve）不弹确认 |
| 6 | 每按键 O(n×m) 全量重高亮，lockfile 级冲突文件会卡 | backdrop 派生链（regions/lines/highlighted）改用 `useDeferredValue(editor)`；`highlighted` 改为 regions 排序前提下的单指针线性扫描 |

## 🟢 采纳

- `SEPARATOR` 正则加 `\r?`，CRLF 文件下 `=======` 行正常分类（附测试）
- `- 8` 魔数提为 `EDITOR_PAD_Y` 并注释（对应 py-2）
- `classifyConflictLine` 直接单测 + CRLF 用例（测试 55 → 58）
- `files.length === 0` 列表分支补"瞬态防御"注释

## 🟢 暂缓（记录不阻塞）

- 经典滚动条平台 textarea 出滚动条后与 backdrop 约一滚动条宽的水平错位（Win11/macOS overlay 无感）
- 面板打开无初始焦点（其他模态有 autoFocus 约定）
- abort 逻辑在 banner 与 panel 各一份、错误通道不一致（toast vs ErrorAlert）
- 遮罩点击不关闭（与脏编辑安全刻意取舍，保持现状）

## 验证

- `tsc` typecheck 0 错误；`eslint` 0 问题；vitest **58/58**；`vite build` ✓
- 修复过程中 lint 曾抓到 `useMemo` 位于 early return 之后（rules-of-hooks），已移至前置（reviewer 确认现无违规）
