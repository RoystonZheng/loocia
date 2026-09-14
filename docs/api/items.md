# 资讯列表接口

## 用途

`GET /api/public/items` 提供 AI Cool 资讯列表，供精选、全部动态和来源筛选使用。接口匿名只读，响应结构保持兼容，不暴露 raw 阶段的 `source_role`。

## 请求

| 参数 | 类型 | 说明 |
|---|---|---|
| `mode` | `selected`、`all` | 默认 `selected`；`all` 返回全部可展示资讯 |
| `category` | `ai-models`、`ai-products`、`industry`、`paper`、`tip` | 可选分类 |
| `source_kind` | `rss`、`html`、`mp`、`aihot` | 可选来源类型筛选；未知值返回 400 |
| `q` | string | 至少 2 个字符才生效，搜索标题、摘要和正文 |
| `take` | 1-100 | 默认 50 |
| `cursor` | string | 上一页返回的 opaque cursor，原样回传 |

`source_kind=mp` 浏览历史公众号语料时不加默认 7 天窗口；`rss/html/aihot` 浏览仍使用默认 7 天窗口。搜索 `q` 继续跨全量归档。

## 响应

响应仍为 `ItemList`：

```json
{
  "count": 1,
  "hasNext": false,
  "nextCursor": null,
  "items": [
    {
      "id": "item-id",
      "title": "中文标题",
      "url": "https://example.com/article",
      "permalink": "/items/item-id",
      "source": "AIHOT · 二手线索",
      "selected": true
    }
  ]
}
```

`PublicItem` 不包含 `source_kind` 和 `source_role`。前端筛选通过请求参数完成，卡片继续展示 `source` 文案。

## 前端消费

`web/src/components/Feed.tsx` 的来源筛选为：

| 文案 | 请求 |
|---|---|
| 全部来源 | 不传 `source_kind` |
| RSS/Atom | `source_kind=rss` |
| 网页直采 | `source_kind=html` |
| 公众号 | `source_kind=mp` |
| AIHOT补漏 | `source_kind=aihot` |

点击来源、分类或搜索提交都会重置当前列表和 cursor。筛选无结果沿用“暂无资讯”，接口失败沿用“加载失败，请稍后重试”。

## 兼容性

- 不新增响应必填字段，已有消费者不需要改造。
- `source_role` 只服务 enrichment，不对外展示。
- 新精选门槛只影响新处理 items，历史数据不回刷。
