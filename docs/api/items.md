# 资讯列表接口

## 用途

`GET /api/public/items` 提供 AI Cool 资讯列表，供 AI 动态页的精选、评分、分类和来源筛选使用。接口匿名只读，响应结构保持兼容，不暴露 raw 阶段的 `source_role`。

## 请求

| 参数 | 类型 | 说明 |
|---|---|---|
| `mode` | `selected`、`all` | 默认 `selected`；`all` 返回全部可展示资讯 |
| `category` | repeated string：`ai-models`、`ai-products`、`industry`、`paper`、`tip` | 可选主题，多值在主题组内 OR；重复值去重，未知值返回 400 |
| `source_kind` | repeated string：`rss`、`html`、`mp`、`aihot` | 可选来源类型，多值在来源组内 OR；重复值去重，未知值返回 400 |
| `score_min` | 1-5 | 可选最低阅读评分，返回 `score >= score_min` 的文章；越界或非整数返回 400 |
| `q` | string | 至少 2 个字符才生效，搜索标题、摘要和正文 |
| `take` | 1-100 | 默认 50 |
| `cursor` | string | 上一页返回的 opaque cursor，原样回传 |

来源组与主题组同时出现时按 AND 组合；不传某一组等价于该组不限。前端以重复 query 参数发送多选，例如 `source_kind=rss&source_kind=mp&category=ai-models&category=paper`。单值写法继续兼容。

`source_kind=mp` 浏览历史公众号语料时不加默认 7 天窗口；只要来源多选中包含 `mp`，仍打开历史归档；其它来源类型仍使用默认 7 天窗口。对于缺失 `published_at` 的记录，时间窗口使用 `created_at` 兜底，但响应不会把入库时间伪装成 `publishedAt`。搜索 `q` 继续跨全量归档。

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
      "source": "Preferred Networks",
      "sourceKind": "aihot",
      "selected": true
    }
  ]
}
```

`PublicItem.source` 继续表示原文来源 / 原始发布方；`sourceKind` 表示采集通道，枚举为 `rss`、`html`、`mp`、`aihot`。前端卡片在 `sourceKind=aihot` 时展示 `AIHOT补漏` 标签，用于区分“从 AIHOT 补漏发现”与“原文来源”。`source_role` 仍不对外暴露。

## 前端消费

`web/src/components/Feed.tsx` 的文章筛选下拉菜单为：

| 文案 | 请求 |
|---|---|
| 全部文章 | `mode=all` |
| 精选 | `mode=selected` |
| 评分 3 分以上 | `mode=all&score_min=3` |
| 评分 4 分以上 | `mode=all&score_min=4` |
| 评分 5 分以上 | `mode=all&score_min=5` |

来源筛选为：

| 文案 | 请求 |
|---|---|
| 全部来源 | 不传 `source_kind` |
| RSS/Atom、网页直采、公众号、AIHOT补漏 | 分别追加对应的 `source_kind`，可同时选择多个 |

主题筛选为：

| 文案 | 请求 |
|---|---|
| 全部主题 | 不传 `category` |
| 模型发布/更新、产品发布/更新、行业动态、论文研究、技巧与观点 | 分别追加对应的 `category`，可同时选择多个 |

点击文章筛选、来源、主题或搜索提交都会重置当前列表和 cursor。AI 动态页始终保留当前热点区；筛选无结果沿用“暂无资讯”，接口失败沿用“加载失败，请稍后重试”。

## 兼容性

- `sourceKind` 是新增可选响应字段，已有消费者可忽略。
- `source_role` 只服务 enrichment，不对外展示。
- 新精选门槛只影响新处理 items，历史数据不回刷。
