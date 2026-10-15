# 通用来源框架

## 背景和目标

原有链路围绕 AI 资讯编排，来源扩展主要复用 RSS、网页和补漏接口。通用来源框架把“关注什么”和“从哪里获取”拆开，让使用者只需提供主题、分类和站点入口，就能在本地生成并审阅来源候选。

## 设计

```text
用户需求 + 官方站点
        |
        v
discover-sources（有限探测，输出证据）
        |
        v
source-plan.json（profile + sources）
        |
        v
RSS/Atom、JSON API、HTML list -> RawItem -> enrichment -> items/API
```

`profile` 保存主题描述、语言、关键词过滤和分类；`sources` 保存传输适配器。来源适配器包括：

| adapter | source_kind | 适用场景 |
|---|---|---|
| `rss` | `rss` | RSS 2.0、Atom、Feed JSON 可转换的订阅源 |
| `json_api` | `api` | 公开 JSON 列表接口，字段用 dotted path 映射 |
| `html_list` | `html` | 没有 feed/API 的公开列表页，使用 CSS selector |
| `mp_corpus` | `mp` | 现有本地公众号语料 |
| `aihot_v1_items` | `aihot` | 现有 AIHOT 补漏接口 |

JSON API 的请求头只允许引用环境变量名。发现阶段不保存或输出 token、Cookie 和响应正文，只保存候选 URL、适配器、置信度与证据。

## 兼容和边界

- 旧的来源数组配置仍能加载，默认 profile 保持原 AI 资讯行为。
- 分类改为安全 slug，数据库不再限制为五个 AI 分类；旧分类继续有效。
- `source_role` 仍只有 `official`、`professional`、`discovery`，用于来源可信度口径。
- 发现命令只探测种子页、页面声明的 alternate feed 和少量约定路径，不代替站点协议审查、API 申请或凭证管理。
- 现有 AIHOT、日报和前端 AI 分类入口保持兼容；通用分类可以通过公共 API 查询，前端未知分类按 slug 展示。
