# refactor-unify-time-format · 审查报告

> 审查对象：把 `BranchList` / `ReflogPanel` / `BlameView` / `CommitInfoHeader` 的时间格式化收敛到 `src/lib/commitTime.ts`
> 代码版本：分支 `refactor/unify-time-format`，基线 `1728526`，**改动未提交**
> 审查基准：`docs/tasks/refactor-unify-time-format/plan.md`、`.codewave/tasks/20260921-193320-unify-time-format/requirement.md`
> 结论：**通过，可合并**（无 🔴）；遗留问题全部属「守卫强度」类，见下

## 审查范围

| 文件 | 状态 | 改动 |
|---|---|---|
| `src/lib/commitTime.ts` | 改 | 新增 `AbsoluteStyle` 样式表（5 个样式）+ `formatAbsoluteTime(time, style, locale?)`；缓存键改为 `${style}\|${locale ?? ""}\|${stamp}`，`stamp` 按 `granularity`（day/minute/second）推导；`formatCommitDate` 委托为 `formatAbsoluteTime(time, "date")`；`formatCommitTime` 与 `localDayKey` 一字未动 |
| `src/lib/commitTime.test.ts` | 改 | 新增 5 组用例：逐字节对照（5 样式 × 3 locale）、重复调用一致、跨 locale 不串味、跨粒度不串味、`NaN`/`±Infinity` 不抛 |
| `src/components/BranchList.tsx` | 改 | 删除本地 `formatTime`（含 `time<=0 → ""`）；行内改 `formatCommitTime`；tooltip 改 `formatAbsoluteTime(…, "date-time")` |
| `src/components/ReflogPanel.tsx` | 改 | 删除本地 `formatTime`；改 `formatAbsoluteTime(e.time, "md-hm")`（**无年份**、系统 locale） |
| `src/components/BlameView.tsx` | 改 | 删除本地 `formatTime`；两处（gutter + hover）改 `formatAbsoluteTime(…, "ymd", i18n.language)` |
| `src/components/CommitInfoHeader.tsx` | 改 | 删除本地 `formatDateTime`；改 `formatAbsoluteTime(data.time, "ymd-hm", i18n.language)` |
| `src/components/timeFormatCallSites.test.ts` | 新 | 7 条静态源码守卫 |

**未改动**（非目标，已核实）：`src/lib/commitMenu.ts`（剪贴板 `Date:` 文本）、`src/components/CommitGraph.tsx`、i18n 资源、`package.json`、`src-tauri/`。无新增依赖、无新增 i18n key。

## 七维度结论

| 维度 | 结论 |
|---|---|
| 正确性 | ✅ 6 处调用点的 `method` / `options` / `locale` 三要素逐点等价（见下表）；缓存键含样式与 locale 且粒度与标签对齐；无 🔴 |
| 安全 | ✅ 无涉密面；未改凭据 / 网络 / 文件系统；无新增攻击面 |
| 性能 | ✅ blame 千行微基准：1000 行 / 30 天场景 0.20ms vs 改造前 23.83ms（0.008×）；600 天 0.617×；1000 天 1.02×（512 上限触发 `clear`，最坏≈改造前，非回退） |
| 可维护性 | ✅ 样式表把「哪个调用点用哪种格式」收成一处常量；`formatCommitTime` 成为唯一相对时间实现 |
| 可读性 | ✅ 注释解释「为什么」（方法不可互换的引擎理由、粒度必须 ≥ 标签最细字段） |
| 测试覆盖 | ⚠️ 门禁全绿（32 文件 / 247 用例），但**组件渲染层零覆盖**（仓库无 jsdom / testing-library）⇒ 调用点的样式名与 locale 实参在自动化层面无守卫，见「🟡 与变异存活清单」 |
| 最佳实践 | ✅ 未改任何对外导出签名与行为；未越界改动；相对时间分档阈值未动 |

## 逐调用点等价性（独立核实）

| 调用点 | method | options | locale | 判定 |
|---|---|---|---|---|
| `BranchList.tsx:196-198` 相对分档 | 末档 `toLocaleDateString()` ≡ `(undefined)` | 无 | 系统 | 等价（唯一差异：`time<=0` 兜底被移除，当前调用点有 `> 0` 守卫 ⇒ 不可达） |
| `BranchList.tsx:184` tooltip | `toLocaleString` 未变 | 无（含秒） | 无参 ≡ `undefined` | 等价；`sha · 时间` 拼接与分隔符未动 |
| `ReflogPanel.tsx:191` | `toLocaleString` 未变 | `{month,day,hour,minute}` 逐字，**确无 `year`** | 系统 | 等价 |
| `BlameView.tsx:56, 76` | `toLocaleDateString` 未变 | `{year,month,day}` 逐字 | 仍 `i18n.language` | 等价（两处都改） |
| `CommitInfoHeader.tsx:172` | `toLocaleString` 未变 | `{year,month,day,hour,minute}` 逐字 | 仍 `i18n.language` | 等价 |

## 🟡 遗留（守卫强度类，建议后续单独立项）

均不构成本次交付缺陷，但会让**将来的**回归静默通过。按价值排序：

1. **调用点参数无守卫**：`timeFormatCallSites.test.ts` 只断言「含 `formatAbsoluteTime(`」，不校验**样式实参与 locale 实参** ⇒ 「ReflogPanel 误用 `ymd-hm`（多出年份）」「BlameView / CommitInfoHeader 丢掉 `i18n.language`（退回系统 locale）」「CommitInfoHeader 丢时分」这类回归在 247 个测试下全绿（见变异 S1/S2/S3）。建议：空白归一后断言完整调用串。
2. **`method` 这一最该守的属性恰恰没有守卫**：V8 上带 options 时两个方法逐字节相同，故「方法互换」无鉴别力（变异 S5 存活），而 plan 正是把它列为引擎相关风险。建议：对 `ABSOLUTE_STYLES` 每个样式块做 method 源码断言。
3. **回流守卫可被绕过**：只禁 `toLocaleDateString(` / `toLocaleString(` 两种拼写；`toLocaleTimeString(`、`new Intl.DateTimeFormat(locale, opts).format(date)` 均可回流而不报错；新增组件也不在覆盖范围内。建议放宽为 `toLocale\w+\(|Intl\.DateTimeFormat` 或按 glob 枚举组件。
4. **`BranchList` 列表行的 `> 0` 守卫无守卫**：删掉后会渲染出孤立的 ` · 1970/1/1`（变异 S4 存活）。
5. **「locale 默认必须为 `undefined`」的断言依赖宿主 locale**：在 en-US 宿主上「默认被改成 en」不可检测。建议补源码级断言（render 路径不得对 locale 兜默认值）。
6. **本地日守卫锚定变松**：`getFullYear()/getMonth()/getDate()` 的断言只要求这些记号在文件里出现，未锚定到 `granularity === "day"` 分支。
7. **缓存治理无测试**：删掉缓存或删掉 `>512 clear()` 都不报错（变异 S6/S7 存活）；正确性无影响，最坏≈改造前。
8. 测试文件头注释称「全仓」但实际不含 `lib/commitMenu.ts`（本轮显式非目标），措辞宜改为「每个 UI 展示面」。

## 🟢 说明

- `?? ""` 仅用于缓存键归一（`toLocaleString("")` 在 V8 会抛 `RangeError`，故不存在与 `undefined` 的语义碰撞）。
- 512 上限的 `Map` 现在同时承载按分钟 / 按秒的键，活跃键集略超阈值时会反复 `clear`（blame 日标签被连带清空）；正确性无影响。
- `formatCommitTime` 已不含 `time <= 0 → ""`，建议在 JSDoc 写明「非正时间由调用点守卫」，避免下一处调用点误以为它返回空串。

## 测试结论（tester 实跑）

- 被测版本：`refactor/unify-time-format` @ `1728526`，dirty。
- `pnpm format:check` ✅ · `pnpm lint` ✅（0 error / 40 warning，**逐文件与基线对照确认本次零新增**）· `pnpm typecheck` ✅ · `pnpm test` ✅ **32 文件 / 247 用例**。
- 后端 `src-tauri` 零改动，`cargo` 门禁未执行。
- **等价性独立验证**（不复述开发者结论）：直接 import 真实 `commitTime.ts`，旧表达式逐字抄自 HEAD，在 **4 个时区**（宿主 UTC+8 / UTC / UTC+9 / UTC−5）× **3 档 locale**（系统 / `en` / `zh-CN`）× 全部分档边界与「同本地日多时刻」「跨本地午夜」「`time=0` / 负数 / 未来时间」场景下比较，**差异 0 处**。

### 变异测试存活清单（最有价值的部分）

被杀死：`md-hm` 粒度改回 `day`、`ymd` 默认 locale 改 `en`（**仅非 en 宿主**）、删 `ymd-hm` 的 `year`、`md-hm` 偷加 `year`、`date-time` 偷加 options、把本地日键退回 UTC 日桶（**仅非 UTC 宿主**）、把内联 `toLocaleString(` 写回组件。

**存活（现有测试拦不住）**：S1 组件换样式名（BlameView `ymd→md-hm`）、S2 组件丢 locale 实参、S3 CommitInfoHeader `ymd-hm→ymd`、S4 删 `BranchList` 的 `> 0` 守卫、S5 `method` 互换、S6 删缓存、S7 删淘汰上限、S8 UTC 宿主下的 UTC 日桶回归。

## 必须真机确认

1. **非 V8 引擎**（macOS WKWebView/JSC、Linux WebKitGTK）：带 options 时 `toLocaleDateString` 与 `toLocaleString` 是否仍逐字节一致 —— `method` 字段是唯一防线，测试无鉴别力。开 blame 槽位/悬停与 inspector 头部比对。
2. **英文系统 locale**：`date` / `date-time` 两个样式仍随系统 locale（不显式传参的路径）。
3. **切语言不重载**：blame gutter 与 inspector 头部立即变新语言、无旧语言残留。
4. **组件渲染层**（自动化无法覆盖）：ReflogPanel 时间不含年份、CommitInfoHeader 有时分、BlameView 有年份、`time=0` / 无 upstream 分支不出现孤立 ` · `。
5. **真实数据不串味**：同一本地日多次 reset/checkout 的 reflog 记录显示各自真实时刻。
6. 千行 blame 滚动体感无回退。
