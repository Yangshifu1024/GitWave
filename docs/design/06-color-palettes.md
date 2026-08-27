# GitWave · 配色方案（Palette Options）

> 状态：待选 · 2026-08-27  
> 布局与组件形态见 [`05-visual-redesign.md`](./05-visual-redesign.md)；**仅颜色可变**。  
> 交互预览：[`mockups/v2/index.html`](./mockups/v2/index.html) 顶栏 **Palette** 切换。

## 选型原则

- 不用纯白 `#FFFFFF`、不用系统蓝 `#007AFF`
- **Tide Lanes**（history 图 lane 渐变）每套方案独立定义，是各方案的签名色
- 语义色（success / warning / danger）跨方案保持一致，降低认知成本
- Light / Dark 成对设计，dark 不是简单反色

## 共享语义色（五套通用）

| Token | Light | Dark |
|---|---|---|
| success / ahead | `#2F9E6B` | `#3ECF8E` |
| warning / behind | `#C47A1A` | `#E0A04A` |
| danger / coral | `#D64545` | `#E05A5A` |

---

## A · Tide Studio（当前默认）

**气质**：冷灰画布 + 青绿 Tide，克制原生感。GitWave 原始提案。

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

---

## 对比摘要

| 方案 | 冷暖 | Accent 色相 | 适合印象 |
|---|---|---|---|
| **A Tide** | 冷 | 青绿 | 默认、平衡、原生 |
| **B Slate** | 冷 | 靛蓝 | 专业、IDE 感 |
| **C Copper** | 暖 | 铜橙 | 编辑、个性 |
| **D Harbor** | 冷 | 海蓝 | 品牌 Wave 延伸 |
| **E Moss** | 中性偏暖 | 苔藓绿 | 护眼、低调 |

## 选定后

1. 将选中方案的 Light/Dark 值写入 `01-tokens.md` §1
2. 更新 `src/styles/tokens.css` 的 `@theme` 块
3. `mockups/v2/index.html` 中将该方案设为 `data-palette` 默认

## 关联

- `01-tokens.md` — token 落地规范
- `05-visual-redesign.md` — 布局与组件（不变）
- `mockups/v2/index.html` — 五套配色实时切换
