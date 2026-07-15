# 图谱视图改版：宋体墨色 + 子图动效 — Design

**Date:** 2026-07-15
**Status:** Approved (co-designed with user, visual companion 四轮选型)
**Goal:** 解决用户反馈的三个问题：①词云配色/字体难看 → 宋体纯墨报纸风；②子图字体重叠、无动效 → 防重叠三招 + 焦点切换平滑过渡；③详情页侧栏缺图谱项 → 已修（commit 6f0fd62，随本次一起部署）。

用户拍板的选型：词云皮肤 = **宋体墨色·纯墨**（四选一 D + 用色四选一 1）；子图动效 = **焦点切换平滑过渡**（Connected Papers/staged-animation 式，含 hover 聚焦调光；四选一 B）。

---

## ① 词云皮肤（GraphView）

**纸底卡片**：词云 svg 与下方面板放进纸色底卡片。新 CSS token（两主题都定义）：
- light: `--gv-paper: #faf9f6`、边框沿用 `--border`
- dark: `--gv-paper: #16181d`（≈ --card）

**字体**：词云文字用现有 `--serif`（Songti SC / SimSun）。

**墨阶（热度→颜色+字重）**：不再按 entity/topic 分色（用户明确选了纯墨）。按 `count/maxCount` 分 4 档：

| 档 | 阈值(占比) | light 色 | dark 色 | 字重 |
|---|---|---|---|---|
| 特热 | ≥0.66 | #1a1a1a | #e8e8e4 | 900 |
| 热 | ≥0.4 | #3d3d3b | #c9c9c4 | 700 |
| 中 | ≥0.18 | #6a6a63 | #9a9a92 | 400 |
| 冷 | <0.18 | #98988f | #6b6b64 | 400 |

实现为 CSS 类 `gv-ink-1..4`（色值挂 CSS var，主题切换自动跟随），组件只算档位。字号映射（14~48px √scale）不变。hover/selected 样式保留（下划线+加深）。

**入场**：词云整体 0.3s 淡入（CSS，一次性）。

## ② 子图（TermPanel）

**防重叠三招（必做）**：
1. 标签描边光晕：`.gv-node text { paint-order: stroke; stroke: var(--gv-paper); stroke-width: 3px; }`
2. 碰撞半径算上文字：`forceCollide().radius(d => Math.max(d.r, labelHalfWidth(d.id)) + 10)`，`labelHalfWidth` 按字符估宽（CJK ≈ fontSize、latin/数字 ≈ 0.62×fontSize，fontSize=12）
3. 边缘留白：clamp 时 x 方向按 `max(r, labelHalfWidth)`、y 方向下边按 `r+16`（标签在节点下方）收边界

**墨色**：焦点节点 `#1a1a1a`(light)/`#e8e8e4`(dark)，邻居按 weight/maxW 用同一套 `gv-ink` 4 档；边灰 `--rail` 不变；标签宋体 12px。不再按 entity/topic 分色。

**动效 = 平滑过渡 + hover**：
- **焦点切换（核心）**：点邻居后不重画。新布局用旧位置做种子（把上一轮各 term 的 x/y 作为 d3-force 节点初始坐标，固定迭代后得到新终态），节点 `<g>` 用 `transform: translate(x,y)` 定位 + `transition: transform .6s cubic-bezier(.4,0,.2,1), opacity .4s`：
  - 两轮都在的 term：从旧位置平滑滑到新位置
  - 退场 term：opacity→0，400ms 后从 DOM 移除（组件里保留一帧 exiting 列表）
  - 新进 term：opacity 0→1
  - 边层整体淡出→淡入（焦点变了所有边都换，不做逐条位移）
- **hover 聚焦调光**：悬停邻居节点 → 该节点与其到焦点的边加深（full opacity），其余节点/边降到 0.3；离开恢复。纯 CSS(:hover 配合 sibling 选择器做不到跨元素，需组件 state：`hovered` term + 类名切换)。
- 首次进入面板：沿用现有淡入。
- `prefers-reduced-motion: reduce` 时关掉位移过渡（accessibility 底线）。

**实现要点**：`layoutGraph` 增加可选参数 `seed?: Map<string,{x,y}>`（旧位置），内部把匹配节点的初始 x/y 设为种子（d3-force 尊重初始坐标）。TermPanel 维护 `positions: Map<term,{x,y}>` 跨数据更新传递。

## ③ 详情页侧栏（已完成）

commit 6f0fd62：SSR 侧栏加「图谱」项，SPA 加 hash 定位（`/#all` `/#daily` `/#graph`），App 初始视图读 hash、切视图写 hash。随本次部署上线。

## 测试

- 前端：现有测试全保；新增/调整——墨阶分档函数单测（阈值边界）；TermPanel 切换焦点时共同节点仍在 DOM（不重建，key 稳定）+ 退场节点 400ms 后移除（fake timers）；hover 设置调光类名；labelHalfWidth 估宽单测。d3-cloud/布局 mock 方式不变。
- 视觉：部署前 `npm run build` + 浏览器人工过一遍亮/暗主题。

## 明确不做

拖拽/实时仿真、canvas 图形库、词云形状变化、entity/topic 分色（本轮明确移除）、复古褐墨底色。

## 发布

前端为主（index.css + GraphView/TermPanel/graphLayout/cloudLayout 字体参数）+ 详情页 Go 模板（已提交）。交叉编译 server（含 6f0fd62 的 detailpage 变更）→ Melos 原子替换重启；`rsync web/dist/`。
