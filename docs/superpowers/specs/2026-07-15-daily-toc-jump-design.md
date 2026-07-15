# 日报「今日看点」目录跳转 + 多标题展示

**日期**:2026-07-15 · **状态**:已实现并上线

## 需求

1. 看点卡片里的分区小标题(模型发布/更新等)点击后跳到下方对应正文分区。
2. 每个分区原来只露 1 条标题,信息太少;改为最多露 3 条,超出的收进「…等 N 篇」。

## 设计决策

- **展示密度**:每分区最多 `TOC_MAX = 3` 条标题(逐条可点进详情页);超过时末尾加「…等 N 篇」(N=该分区总数),点击同样跳到对应分区。右侧计数徽标保留。
- **跳转方式**:`scrollIntoView({behavior:'smooth'})`,**不用 URL hash 锚点**——hash 归视图路由(#daily/#all/#graph)所有,锚点会互相打架。
- **锚点 id**:正文分区按索引生成 `daily-sec-{i}`,避免中文 label 做 id 的转义问题。
- **落点体验**:`.paper-section` 加 `scroll-margin-top: 70px`,滚动后分区标题不贴死视口顶(也躲开移动端 55px 顶栏)。
- **可点性提示**:小标题改为 button(样式重置为原字体字重),hover 变强调色;「…等 N 篇」灰字小号,hover 同色。
- 快讯不进看点卡,导语/字数统计/正文排版不变。

## 实现

- `web/src/components/DailyView.tsx`:看点卡 slice(0,3) + ph-more;正文分区加 id;`jumpToSection()`。
- `web/src/index.css`:`.ph-cat`(button 化+hover)、`.ph-item`(block)、`.ph-more`、`.paper-section` scroll-margin。
- 测试:`DailyView.test.tsx` 新增 2 例(截断+尾行、点击滚动+锚点存在),全套 74/74 绿。
