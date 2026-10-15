# Codex 使用指南：把 AI Cool 配置成通用资讯框架

这个项目的采集链路不要求主题必须是 AI。使用者可以把自己的信息需求、官方站点和分类写入一个 source plan，再让 Codex 协助发现可用的 RSS、Atom、JSON API 或网页列表入口。

## 快速流程

1. 先读 `docs/overview/README.md`、`docs/design/README.md`、`docs/api/README.md` 和本文件。
2. 在 `source-plan.json` 的 `profile` 中写清主题、语言、关键词和分类；不要把 token、Cookie 或密码写入文件。
3. 为每个官方站点执行：

   ```bash
   cd server
   go run ./cmd/discover-sources -request '我需要跟踪欧洲能源政策和电价变化' \
     -seed https://example.org -out ../source-candidates.json
   ```

4. 查看 `source-candidates.json` 的证据，再把确认过的候选写成 `source-plan.json`。优先使用 RSS/Atom，其次是公开 JSON API，最后才使用带 CSS selector 的 HTML 列表。
5. 运行采集 smoke：

   ```bash
   AIHOT_DATABASE_URL='postgres://...' \
     go run ./cmd/pulse -sources ../source-plan.json -ingest-only
   ```

## Source plan 示例

```json
{
  "profile": {
    "name": "能源政策雷达",
    "description": "跟踪欧盟能源政策、批发电价、监管公告和有数据的分析",
    "language": "zh-CN",
    "include_keywords": ["energy", "electricity", "能源", "电价"],
    "exclude_keywords": ["招聘", "活动"],
    "categories": ["policy", "market", "research", "general"]
  },
  "sources": [
    {
      "name": "监管机构 Feed",
      "url": "https://example.org/feed.xml",
      "adapter": "rss",
      "source_kind": "rss",
      "source_role": "official",
      "max_items_per_run": 30
    },
    {
      "name": "公开数据 API",
      "url": "https://example.org/api/articles",
      "adapter": "json_api",
      "source_kind": "api",
      "items_path": "data.items",
      "title_path": "headline",
      "url_path": "url",
      "content_path": "summary",
      "published_at_path": "published_at",
      "header_env": {"Authorization": "ENERGY_API_TOKEN"}
    },
    {
      "name": "研究机构列表页",
      "url": "https://example.org/research",
      "adapter": "html_list",
      "source_kind": "html",
      "item_selector": "article",
      "link_selector": "a[href]",
      "title_selector": "h2",
      "date_selector": "time"
    }
  ]
}
```

`header_env` 的值是环境变量名，不是凭证本身。发现命令只做有限探测，不会爬完整站点、绕过登录或猜测私有接口。候选仍需根据官方页面、接口文档和响应证据确认。

历史上的 `[{"name":"...","url":"..."}]` 配置仍然兼容，会按 RSS 来源处理。
