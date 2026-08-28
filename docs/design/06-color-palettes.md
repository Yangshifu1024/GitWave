# GitWave · 配色方案（Palette Options）

> 状态：**K · Native Blue（默认）与 A · Tide Studio 已实现**（2026-08-27），Settings → Appearance → Color palette 内切换；其余候选待评估。  
> 布局与组件形态见 [`05-visual-redesign.md`](./05-visual-redesign.md)；**仅颜色可变**。  
> 交互预览：[`mockups/v2/index.html`](./mockups/v2/index.html) 顶栏 **A–E / F–J / K** 切换。

## 选型原则

- 不用纯白 `#FFFFFF`；「不用系统蓝 `#007AFF`」适用于 Tide 及其余候选方案——**K · Native Blue 是显式例外**（应用默认 palette，用户 2026-08-27 拍板）
- **Tide Lanes**(history 图 lane 渐变)每套方案独立定义,是各方案的签名色;落地的 lane token 为 `--color-lane-1..5`
- 语义色(success / warning / danger)跨方案保持一致,降低认知成本
- Light / Dark 成对设计,dark 不是简单反色

## 共享语义色（全部方案通用）

| Token | Light | Dark |
|---|---|---|
| success / ahead | `#2F9E6B` | `#3ECF8E` |
| warning / behind | `#C47A1A` | `#E0A04A` |
| danger / coral | `#D64545` | `#E05A5A` |

---

# 第一轮 · A–E

## A · Tide Studio（可选 · 已实现）

**气质**：冷灰画布 + 青绿 Tide，克制原生感。GitWave 原始提案。运行时通过 `<html data-palette="tide">` 激活。

| Role | Light | Dark |
|---|---|---|
| Canvas (Foam) | `#F4F6F8` | `#161B20` |
| Sidebar (Mist) | `#E6EBEF` | `#12161A` |
| Elevated | `#EEF1F4` | `#1C2329` |
| Ink | `#1B2228` | `#E8ECF0` |
| Muted | `#7A8692` | `#8B97A3` |
| **Accent** | `#1A8F8A` | `#3EBAB3` |
| Hairline | `#D5DCE2` | `#2A323A` |
| **Lanes** | `#1A8F8A` → `#2A7B8C` → `#3D6B9A` → `#4A5FA8` | `#3EBAB3` → `#5BA4B8` → `#6B8FC4` → `#7A7AD1` |

---

## B · Slate Forge

**气质**：蓝灰中性底 + 靛蓝 accent，偏专业 IDE / Fork 系，但不碰系统蓝。

| Role | Light | Dark |
|---|---|---|
| Canvas | `#F3F4F6` | `#15171C` |
| Sidebar | `#E8EAED` | `#0F1115` |
| Elevated | `#EEEFF2` | `#1C1F26` |
| Ink | `#1A1D21` | `#E6E8ED` |
| Muted | `#6B7280` | `#8B919C` |
| **Accent** | `#5B6EAE` | `#7B8FD4` |
| Hairline | `#D4D7DE` | `#2A2E38` |
| **Lanes** | `#5B6EAE` → `#6B5B9A` → `#4A7B9A` → `#5A6A88` | `#7B8FD4` → `#8B7BC4` → `#6B9BB8` → `#7A8AA8` |

---

## C · Copper Ledger

**气质**：暖石色纸感 + 铜橙 accent，编辑感 / 账本感，辨识度高。

| Role | Light | Dark |
|---|---|---|
| Canvas | `#F6F3EE` | `#1A1816` |
| Sidebar | `#EDE8E0` | `#141210` |
| Elevated | `#F0EBE4` | `#221F1C` |
| Ink | `#2C2620` | `#E8E4DE` |
| Muted | `#8A7F72` | `#9A9088` |
| **Accent** | `#B87333` | `#D4924A` |
| Hairline | `#DDD5CA` | `#3A3530` |
| **Lanes** | `#B87333` → `#9A6B4F` → `#6B7A8C` → `#7A6B8A` | `#D4924A` → `#B88A5A` → `#7A8A9A` → `#9A8A9A` |

---

## D · Harbor Mist

**气质**：海雾蓝灰 + 港蓝 accent，与「Wave」品牌隐喻一致，清爽。

| Role | Light | Dark |
|---|---|---|
| Canvas | `#F0F4F7` | `#121A20` |
| Sidebar | `#E2E9EF` | `#0D1218` |
| Elevated | `#E8EFF5` | `#1A2329` |
| Ink | `#152028` | `#E4EBF0` |
| Muted | `#6B7D8C` | `#8A9AA8` |
| **Accent** | `#2B7A9E` | `#4BA3C7` |
| Hairline | `#CDD8E2` | `#28343E` |
| **Lanes** | `#2B7A9E` → `#2A8F8A` → `#3D6B9A` → `#4A6FA8` | `#4BA3C7` → `#4BB8A8` → `#6B8FC4` → `#7A8FC8` |

---

## E · Moss Terminal

**气质**：橄榄灰绿底 + 苔藓绿 accent，低饱和、长时间看不累，偏终端 / 极客但不全黑。

| Role | Light | Dark |
|---|---|---|
| Canvas | `#F2F3F0` | `#161816` |
| Sidebar | `#E6E8E3` | `#111411` |
| Elevated | `#ECEEE9` | `#1C201C` |
| Ink | `#1E221C` | `#E4E8E4` |
| Muted | `#71786E` | `#8A9288` |
| **Accent** | `#4D7350` | `#6B9B6E` |
| Hairline | `#D5DAD4` | `#2A302A` |
| **Lanes** | `#4D7350` → `#5A7A6B` → `#4A6B7A` → `#6B6B7A` | `#6B9B6E` → `#7AAA8B` → `#6B9BA8` → `#8A8A9A` |

| **Lanes** | `#4D7350` → `#5A7A6B` → `#4A6B7A` → `#6B6B7A` | `#6B9B6E` → `#7AAA8B` → `#6B9BA8` → `#8A8A9A` |

---

# 第二轮 · F–J（2026-08-27 新增）

> 针对第一轮「偏灰、偏保守」的反馈：accent 对比更强、色相跨度更大，材质层次（Canvas / Sidebar 差）略拉大。

## F · Ember Signal

**气质**：中性暖灰 + **朱红** accent，能量感强，适合想要「有态度」但不全黑的产品。

| Role | Light | Dark |
|---|---|---|
| Canvas | `#FAF9F8` | `#141210` |
| Sidebar | `#ECEAE8` | `#0E0D0C` |
| Elevated | `#F2F0EE` | `#1E1C1A` |
| Ink | `#1A1816` | `#ECEAE8` |
| Muted | `#7A746E` | `#8A8480` |
| **Accent** | `#D44D2B` | `#E86545` |
| Hairline | `#DDD8D4` | `#322E2C` |
| **Lanes** | `#D44D2B` → `#C45A38` → `#8B5A4A` → `#7A4A5A` | `#E86545` → `#D87858` → `#A87868` → `#987888` |

---

## G · Violet Circuit

**气质**：薰衣草灰底 + **电紫** accent，数码感 / 电路板感，与 A–E 的「自然色」完全不同。

| Role | Light | Dark |
|---|---|---|
| Canvas | `#F6F5FA` | `#13121A` |
| Sidebar | `#ECEAF2` | `#0D0C12` |
| Elevated | `#F0EEF6` | `#1C1A26` |
| Ink | `#1A1820` | `#EAE8F0` |
| Muted | `#78728A` | `#888498` |
| **Accent** | `#6E56CF` | `#8B7AE8` |
| Hairline | `#DCD6E8` | `#2A2838` |
| **Lanes** | `#6E56CF` → `#7A5CB8` → `#5A6A9A` → `#8A6A9A` | `#8B7AE8` → `#9A7AD8` → `#7A8AB8` → `#AA8AAA` |

---

## H · Jade Current

**气质**：薄荷灰绿底 + **翡翠** accent，比 Tide 更饱和、比 Moss 更清澈，仍有「水 / 波」联想。

| Role | Light | Dark |
|---|---|---|
| Canvas | `#F4F7F6` | `#101816` |
| Sidebar | `#E6EEEB` | `#0A100E` |
| Elevated | `#ECF2F0` | `#182220` |
| Ink | `#121A18` | `#E4ECE8` |
| Muted | `#6A7874` | `#7A8A84` |
| **Accent** | `#2A9D8F` | `#3ECFB8` |
| Hairline | `#CDD9D4` | `#24302C` |
| **Lanes** | `#2A9D8F` → `#3A8A7A` → `#4A7A8A` → `#5A6A7A` | `#3ECFB8` → `#4AB8A0` → `#5A98A8` → `#6A8898` |

---

## I · Ochre Archive

**气质**：羊皮纸色 + **赭金** accent。与 C Copper 不同：更偏档案 / 博物馆金标，而非铜锈橙。

| Role | Light | Dark |
|---|---|---|
| Canvas | `#F7F5F2` | `#1A1814` |
| Sidebar | `#EDEAE5` | `#12100C` |
| Elevated | `#F1EEE8` | `#242018` |
| Ink | `#242018` | `#ECE8E0` |
| Muted | `#847C70` | `#908880` |
| **Accent** | `#9A7B1A` | `#C49A2A` |
| Hairline | `#DDD6CA` | `#363028` |
| **Lanes** | `#9A7B1A` → `#8A6A2A` → `#6A7A5A` → `#7A6A5A` | `#C49A2A` → `#B08A3A` → `#8A9A6A` → `#9A8A6A` |

---

## J · Rosesteel

**气质**：钢灰底 + **玫瑰红** accent，lane 收束到钢蓝。Git 客户端里极少见的配色，辨识度高。

| Role | Light | Dark |
|---|---|---|
| Canvas | `#F3F4F6` | `#12161C` |
| Sidebar | `#E8EAEE` | `#0C0F14` |
| Elevated | `#EEF0F4` | `#1A2028` |
| Ink | `#141820` | `#E8EAEE` |
| Muted | `#6E7888` | `#8890A0` |
| **Accent** | `#C4486A` | `#E06888` |
| Hairline | `#D4D8E0` | `#2A3038` |
| **Lanes** | `#C4486A` → `#A85878` → `#6A7A9A` → `#7A6A8A` | `#E06888` → `#C87898` → `#8898B8` → `#9888A8` |

---

## 对比摘要

### 第一轮 A–E

| 方案 | 冷暖 | Accent 色相 | 适合印象 |
|---|---|---|---|
| **A Tide** | 冷 | 青绿 | 默认、平衡、原生 |
| **B Slate** | 冷 | 靛蓝 | 专业、IDE 感 |
| **C Copper** | 暖 | 铜橙 | 编辑、个性 |
| **D Harbor** | 冷 | 海蓝 | 品牌 Wave 延伸 |
| **E Moss** | 中性偏暖 | 苔藓绿 | 护眼、低调 |

### 第二轮 F–J

| 方案 | 冷暖 | Accent 色相 | 适合印象 |
|---|---|---|---|
| **F Ember** | 暖 | 朱红 | 有能量、态度鲜明 |
| **G Violet** | 冷 | 电紫 | 数码、电路感 |
| **H Jade** | 冷 | 翡翠 | 清澈、Wave 水体 |
| **I Ochre** | 暖 | 赭金 | 档案、博物馆感 |
| **J Rosesteel** | 冷 | 玫瑰红 | 高辨识、偏设计工具 |

---

# 已落地 · K

## K · Native Blue（默认 · 已实现）

**气质**：macOS 系统窗口质感（近 `windowBackgroundColor` 灰阶）+ **systemBlue** accent。lanes 取 Apple system colors（蓝/青/靛/紫/灰），贴近原生图表观感。2026-08-27 与 PM 确认后成为应用默认；「不用系统蓝」原则的显式例外。

| Role | Light | Dark |
|---|---|---|
| Canvas | `#ECECEC` | `#262628` |
| Sidebar / Toolbar | `#DFDFDF` | `#202022` |
| Elevated | `#F4F4F5` | `#313134` |
| Ink | `#1B1B1D` | `#EFEFF1` |
| Muted | `#85858B` | `#78787E` |
| **Accent** | `#007AFF` (systemBlue) | `#0A84FF` (systemBlue dark) |
| Hairline | `#DBDBDD` | `#333336` |
| **Lanes** | `#007AFF` → `#32ADE6` → `#5856D6` → `#AF52DE` → `#8E8E93` | `#0A84FF` → `#64D2FF` → `#7D7AFF` → `#BF5AF2` → `#98989D` |

落地细节见 [`01-tokens.md`](./01-tokens.md) §1。

> 2026-08-28 dark 材质分层（见 [`07-theme.md`](./07-theme.md) §3.1）：dark 下 `bg-panel` 提至窗口与 elevated 之间——K `#2A2A2D`、A `#191F25`；light 不变。diff 语义色 token（`diff-add/del/hunk`）同日新增。

## 选定后

1. 将选中方案的 Light/Dark 值写入 `01-tokens.md` §1
2. 更新 `src/styles/tokens.css` 的 `@theme` 块
3. `mockups/v2/index.html` 中将该方案设为 `data-palette` 默认

## 关联

- `01-tokens.md` — token 落地规范
- `05-visual-redesign.md` — 布局与组件（不变）
- `mockups/v2/index.html` — 十套配色实时切换（A–E + F–J）
