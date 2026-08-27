# GitWave · 主界面设计校对稿

高保真视觉稿。**以 HTML 为唯一像素源**——浏览器打开即可审阅、切换状态，不维护 PNG 导出。

## v2 · Native Studio（当前推荐）

> 设计 rationale 见 [`../05-visual-redesign.md`](../05-visual-redesign.md)

| 屏 | HTML |
|---|---|
| Light · History（干净工作副本） | [v2/index.html](./v2/index.html)#history |
| Light · Dirty（展开 Working Copy Bar） | [v2/index.html](./v2/index.html)#dirty |
| Dark · History | [v2/index.html](./v2/index.html)#dark |
| Empty（无 workspace） | [v2/index.html](./v2/index.html)#empty |

浏览器打开 [`v2/index.html`](./v2/index.html)，顶栏切换四屏；**Palette** 按钮切换五套配色（详见 [`../06-color-palettes.md`](../06-color-palettes.md)）。

**v2 核心变化**：Workspace 列表置顶 · Source List 全宽无圆角 · Sync 在 REPOS/BRANCHES 标题栏 · pane 材质分层 · Inspector diff gutter。

## v1（历史参考）

| 屏 | HTML |
|---|---|
| Light · History | [index.html](./index.html)#history |
| Light · Dirty | [index.html](./index.html)#dirty |
| Dark · History | [index.html](./index.html)#dark |
| Empty | [index.html](./index.html)#empty |

核对：没有居中 Logo、没有浏览器 Tab、没有纯白底、没有系统蓝 `#007aff`。
