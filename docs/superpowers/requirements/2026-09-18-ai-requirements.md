# 合并 AI 动态导航并增加精选与评分筛选

## Clarified Requirements

资讯导航合并：将首页左侧的“精选资讯”和“全部 AI 动态”合并为一个“AI 动态”入口，页面默认展示全量 AI 资讯；保留旧的 #all 与 #selected 地址兼容，旧的 #selected 应进入 AI 动态并默认使用“精选”筛选，新的侧栏只展示一个 AI 动态入口。

热点保留：AI 动态页面顶部始终保留“当前热点”区域，不因进入全量动态或切换筛选而隐藏。热点区域继续使用现有热点数据和跳转行为。

筛选方式：保留现有来源筛选、分类筛选和关键词检索。在关键词检索控件旁增加一个原生下拉菜单，提供“全部文章”“精选”“评分 3 分以上”“评分 4 分以上”“评分 5 分以上”选项。默认选中“全部文章”。“精选”沿用现有 selected 语义；评分选项按文章 score >= 对应分数筛选，筛选变化后重置列表和分页并重新请求数据。

接口与兼容性：公共资讯列表接口新增可选 score_min 查询参数，取值 1 到 5；无参数时保持现有行为，mode=selected 继续兼容并表示 selected=true。前端 AI 动态页默认请求 mode=all，并将下拉筛选映射为 mode=all 或 mode=selected 以及 score_min。不得改变文章评分、精选判定、热点数据结构或详情页内容。

验收标准：左侧仅显示一个“AI 动态”资讯入口；进入 AI 动态后可看到当前热点和资讯列表；下拉菜单位于检索控件旁且五个选项可用；选择“精选”请求 selected 模式，选择 3/4/5 分以上请求 score_min=3/4/5，列表会刷新且分页游标重置；旧 #all 和 #selected 地址不报错；来源、分类、关键词筛选和移动端布局不回归。

## Sources

- Repository: `web/src/components/Feed.tsx` at `f75d20720496b375ef4dd38446a6ca1a759c3acd` — 现有资讯页热点、来源分类筛选、关键词检索和分页行为。
- Repository: `web/src/components/Sidebar.tsx` at `f75d20720496b375ef4dd38446a6ca1a759c3acd` — 现有精选资讯与全部 AI 动态侧栏入口。
- Repository: `server/internal/publicapi/params.go` at `f75d20720496b375ef4dd38446a6ca1a759c3acd` — 公共资讯列表查询参数解析和 mode 兼容规则。
- Repository: `server/internal/items/query.go` at `f75d20720496b375ef4dd38446a6ca1a759c3acd` — 资讯列表数据库筛选与分页查询。
