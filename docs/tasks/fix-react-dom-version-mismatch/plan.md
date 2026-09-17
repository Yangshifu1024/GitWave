# fix: 升级 0.8.8 后应用无法启动（react / react-dom 版本不一致）

状态：已修复（待出补丁版本）。

## 事故（2026-09-17，用户真机）

Windows 上 `D:\App\GitWave\gitwave.exe` 自动更新到 0.8.8 后无法运行：双击无任何界面，进程在后台起停多次（`app.log.2026-09-17` 中 21:16–21:19 连续 5 次启动），事件日志无崩溃记录，日志只有正常启动的 3 行。

## 复现与定位

1. 直接运行 0.8.8 可执行文件：进程存活、WebView2 子进程正常拉起，`app.log` 只记录启动前 3 行。
2. 枚举该进程的顶层窗口：

   ```
   hwnd=1050624 visible=False class='Tauri Window' title='GitWave'
   ```

   主窗口已创建但 `IsWindowVisible=False`。窗口在 `tauri.conf.json` 里是 `"visible": false`（为避免首帧白屏），由前端 `useTitlebarActivation()` 调用 `invoke("activate_and_show")` 后再 `window.show()`。所以「窗口不显示」= 前端在调用它之前就挂了。

3. 用 WebView2 远程调试（`WEBVIEW2_ADDITIONAL_BROWSER_ARGUMENTS=--remote-debugging-port=9222`）接 CDP 抓前端异常，得到唯一一条：

   ```
   Error: Minified React error #527; ... ?args[]=19.3.0&args[]=19.2.8
   ```

   `React error #527` = react 与 react-dom 版本不一致。

4. 前端 `#root` 的 `childElementCount = 0`，`main.tsx` 的 `createRoot(...).render(...)` 抛错，React 树没有挂载，`activate_and_show` 永远不会被调用 → 窗口保持隐藏。

## 根因

dependabot PR #88 只升级了 `react`（`^19.1.0` → `^19.3.0`）+ `@types/react`，没有同步升级 `react-dom`（仍是 `^19.1.0`）。锁文件因此解析成：

| 包 | 版本 |
|---|---|
| react | 19.3.0 |
| react-dom | 19.2.8 |

React 要求 `react` 与 `react-dom` 版本完全一致（`react-dom@19.3.0` 的 peer 也是 `react: ^19.3.0`）。0.8.7 及之前两者一致，所以正常；0.8.8 第一次带上这组不一致的依赖，因此上线即挂。

CI 没拦住的原因：前端测试跑在 vitest 的 node 环境，没有任何用例真正渲染 React，也不存在依赖版本一致性的断言；构建（`vite build`）阶段不做版本校验。

## 修复

1. `package.json`：`react-dom` `^19.1.0` → `^19.3.0`，与 `react` 对齐；`@types/react-dom` 同步到 `^19.3.0`。
2. `pnpm install`：锁文件解析为 react / react-dom 均为 `19.3.0`。
3. 新增回归测试 `src/lib/reactVersionParity.test.ts`：读取 `node_modules/{react,react-dom}/package.json`，断言两者版本完全相同。这是本次事故的最小充分断言——版本一致就不会有 #527，不一致就一定在启动时炸。

## 回归验证

- `pnpm test`：193 个用例通过（含新增 parity 用例）；把 `react-dom` 退回 `19.2.8` 时该用例失败。
- `pnpm typecheck`：通过。
- `pnpm lint`：0 error（40 条既有 warning 与本次改动无关）。
- `pnpm format:check`：通过。
- 真机：用修复后的依赖重新构建并运行，主窗口正常显示（见下）。

| 风险 | 影响 | 缓解 |
|---|---|---|
| 依赖一致性只在版本号层面校验，peer 范围（如 `^19.3.0`）不拦 | 未来 react 主版本升级时仍可能飘 | parity 用例断言「完全相等」而非「满足 range」，任何不一致都会红 |
| 0.8.8 已发布且自动更新已下发给用户 | 已装 0.8.8 的用户打不开应用 | 需要发补丁版本（0.8.9）让 updater 能拉回；受影响用户也可重装 0.8.7 |

## 关联

- `docs/tasks/feat-auto-update/plan.md`：自动更新链路（本次把坏版本直接推给了用户）
- `docs/pm/features/F<number>.md`：待补依赖升级相关的发布检查项
- 上游依据：`react-dom@19.3.0` 的 `peerDependencies.react = ^19.3.0`
