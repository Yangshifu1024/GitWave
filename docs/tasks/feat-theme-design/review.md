# feat-theme-design · review

> 审查对象：分支 `feature/theme-design`（未 commit / 未 push）  
> 设计依据：[`docs/design/07-theme.md`](../../design/07-theme.md)

## 验证结果

| 检查 | 结果 |
|---|---|
| `npm run typecheck` | ✅ 通过 |
| `npm run test` | ✅ 43 / 43 通过 |
| `npm run lint` | ✅ 通过 |
| `npm run build` | ✅ 通过（chunk >500kB 警告为既有问题，与本次无关） |
| 构建产物 CSS 检查 | ✅ `.animate-in/.animate-out/.duration-fast/.duration-base/.duration-slow`、`gw-enter/gw-exit` keyframes、`prefers-reduced-motion`、diff token（light 实体值 + dark `#e05a5a24` / `#3ecf8e24` 透明 tint）、dark panel `#2a2a2d` / tide `#191f25`、平台字体栈、tabular-nums 全部生成 |
| 浏览器视觉验证（vite dev + IAB） | ✅ 亮色三层材质正常；暗色 panel/窗口分层生效；More 菜单与 Settings Modal 正常打开（未卡在透明态）；菜单元素 computed style 实测 `animation-name: gw-enter`、`duration: 0.12s`、`timing: cubic-bezier(0,0,0.2,1)` |

## 七维度审查摘要

- **正确性**：⚠️→✅ 关键坑已排——Tailwind v4 无 `--duration-*` 工具类命名空间且 `@theme` 未用变量会被剪枝，故时长变量放 `:root`、`duration-fast/base/slow` 用 `@utility` 手写、`animate-in/out` 的 easing 带 fallback，构建产物已逐一验证。keyframes 动画 `transform`，与 v4 的 `translate` 属性定位（Modal 居中）正确合成；Modal 的 v3 `slide-from-left-1/2` hack 已同步移除（v4 下会造成双重偏移）。
- **安全**：无新增依赖、无网络资源（本地优先）；`prefers-reduced-motion` 降级时动画仍触发 `animationend`，Radix 状态收尾不受影响。
- **性能**：动画仅 opacity/transform（合成器友好）；无 JS 改动逻辑；CSS 体积 89.22kB（与改动前同量级）。
- **可维护性**：动效/diff 色全部 token 化，组件不再散写 `duration-150/200`；`01-tokens.md` / `06-color-palettes.md` / `07-theme.md` 三处文档同步。
- **可读性**：tokens.css 新增段落均带「为什么」注释（命名空间缺失、剪枝、transform 合成）。
- **测试覆盖**：既有 43 项单测全绿（palette/diff 解析不受影响）；CSS 行为以构建产物 grep + 浏览器 computed style 验证。
- **最佳实践**：对齐 GitHub Primer（diffBlob 只染底不染字）、Material 3 / Fluent 2（时长与曲线）、系统字体栈（Fork/Tower/GitHub Desktop 惯例）。

## 遗留 / 建议（不阻塞）

1. `--duration-slow` 尚无使用方（预留 token）。
2. `ease-standard` 已定义，后续可在全局 transition 上替换默认 easing。
3. TabsContent 的 `animate-out` 因 Radix 非强制挂载实际不可见（无行为影响）。
4. 未验证真实 Tauri WebView 下的字体回退表现（本机 WebView2 应命中 Segoe UI Variable Text / Cascadia Mono），建议下次 `tauri dev` 时目检。
