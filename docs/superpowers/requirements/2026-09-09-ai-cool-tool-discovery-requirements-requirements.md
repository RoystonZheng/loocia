# AI Cool Tool Discovery Requirements

## Clarified Requirements

AI Cool 本期要在当前 ai-cool 仓库中新增工具发现与沉淀模块。产品基线以 Cooper 文档 2209600482571 第五部分为准，优先级高于历史技术方案中关于只读工具雷达、Git Markdown 评测报告导入、GitHub Trending 方向、复杂详情页或离线报告导入的描述。在 DevKit 当前阶段只产出需求、设计和计划，不实现生产代码。

首期必须跑通一个闭环：发现配置或手动添加 GitHub 仓库生成候选工具；系统按 GitHub repository node_id 去重并合并来源；候选进入已发现工具列表；用户可以把候选标记为不处理，也可以开始测评；测评过程只记录 AI Cool 内的状态和外部 Cooper 测评文档链接；测评最终只有纳入团队工具和不纳入两种结果；团队工具列表只展示已纳入工具。发现配置页、已发现工具页、测评中页和团队工具页都属于首期上线范围。

工具内部状态只能有 discovered、evaluating、included、excluded 四种，不设置 observe 状态。discovered 表示仓库已被发现但尚未决定是否测评。evaluating 表示用户已选择测评，AI Cool 已记录测评人、操作人、开始时间，并可记录或等待关联 Cooper 测评文档链接。included 表示测评完成且明确纳入团队工具。excluded 表示已发现阶段的不处理，或测评阶段的不纳入；excluded 记录保留用于去重和审计，但不进入已发现、测评中、团队工具等主列表。

GitHub 关键词发现必须使用官方 Repository Search API，在仓库名称、描述、Topics 和 README 中检索英文词组，例如 "coding agent" in:name,description,topics,readme、"agent skills" in:name,description,topics,readme、"browser agent" in:name,description,topics,readme。关键词发现用于覆盖未正确填写 Topic 的仓库，并支持团队按阶段方向做专项发现；发现结果只代表候选，不代表 AI Cool 推荐或背书。

GitHub Topic 发现同样必须使用官方 Repository Search API，通过 topic: 限定符检索，例如 topic:ai-agents、topic:agent-skills、topic:model-context-protocol、topic:browser-automation。Topic 发现用于覆盖已知类别，但 Topic 由仓库作者维护，不能替代关键词发现。一条发现配置只对应一种发现方式，关键词配置和 Topic 配置不能混在同一条配置里。

关键词配置和 Topic 配置都要遵循同一套执行语义：首次执行做有限页数的基线扫描；首次成功后不再反复全量扫描；后续按配置 last_success_at 做增量检索，并向前重叠 24 小时，避免调度延迟或 GitHub 索引延迟导致漏查。增量检索必须包含 created:>时间，用于找上次成功后新创建的仓库，也必须包含 pushed:>时间，用于补充近期重新活跃且现在命中条件的旧仓库。重叠窗口内再次返回的仓库只更新最近命中时间和来源信息，不重复创建候选。

手动添加入口必须允许团队成员粘贴 GitHub 仓库 URL。AI Cool 先解析 URL 并调用官方 GitHub Repository API 获取仓库事实，展示仓库预览卡片，用户确认后才加入已发现。预览卡片至少展示仓库名称、owner/repo、当前可用的一句话信息，并提示仓库信息会在提交后同步。提交时按 node_id 去重：已存在则合并 manual 来源并返回已有工具；不存在则创建 discovered 工具。手动添加用于补充文章、群聊、同事推荐或浏览 GitHub 时发现的工具。首期不解析 GitHub Trending 页面。

发现配置页首期必须对前端用户可用。配置字段包括配置名称、发现方式、检索词或 Topics、触发方式、启用状态、运行信息。检索词和 Topics 在配置内要支持新增、查看、编辑和删除。触发方式支持手动执行和每周执行。运行信息至少展示最近执行时间、结果数量、失败原因。页面需要支持搜索配置或检索词，展示配置总数、已启用数、已停用数，展示 GitHub API 状态，支持新增配置、编辑配置、启用、停用、手动执行和删除配置。

配置删除首期支持，但必须是软删除。删除后的配置默认不在普通配置列表展示，不能再被手动执行、自动周跑、编辑或启用；历史运行记录、发现来源、候选工具和审计事件必须保留。删除配置不能删除已经发现的工具，也不能从工具来源历史里抹掉该配置曾经命中的事实。这样既满足原型图中的删除入口，也保证发现链路可追溯。

发现配置固定只管理关键词和 Topic。高级 GitHub 查询参数首期不暴露给前端用户。系统级默认规则是只处理公开仓库、排除归档仓库、默认排除 Fork，并始终用 node_id 做仓库身份去重。团队准备研究浏览器操作工具时，应创建两条同组的一次性配置：一条关键词配置，包含 browser agent、web agent、browser automation、computer use 等检索词；一条 Topic 配置，包含 browser-agent、browser-automation 等 Topics。两条配置可立即手动执行一次，如果结果持续有价值，再改为每周执行。

已发现工具页必须合并关键词、Topic 和手动添加结果。默认排序是首次发现时间倒序，同一批次按当前 Stars 倒序。页面必须提供三种透明排序：最新发现、当前 Stars、近 7 天 Stars 增长。页面还要支持搜索工具、仓库或发现来源，支持按来源筛选，展示待处理数量、关键词命中数量、Topic 命中数量、手动添加数量、最近同步时间和结果数量。

近 7 天 Stars 增长只能来自 AI Cool 自己保存的 Stars 快照，只能作为已入池工具的排序和展示信号，不能产生新工具，不能叫 GitHub Trending，也不能当成纳入依据。计算时取距当前 6 到 8 天内最接近 7 天的历史快照；没有合适快照时显示暂无。第一周没有历史增量，从第二次快照开始才能计算近 7 天增长。

已发现工具的每一行至少展示工具名称、owner/repo、GitHub 链接入口、一句话介绍、发现来源、当前 Stars、近 7 天 Stars 增长、最近提交时间，以及开始测评和不处理操作。不处理是行内操作入口，可以用图标承载，但必须有明确确认和原因记录。开始测评只能从 discovered 状态触发；不处理也只能从 discovered 状态触发。

一句话介绍优先使用 GitHub Description。Description 为空或过于模糊时，AI Cool 可以从 README 开头生成临时中文摘要，但必须标记为临时描述。AI 不自动测评、不决定是否纳入。工具纳入团队工具前，测评人必须确认最终一句话说明，格式统一为它是什么、解决什么问题、适合什么场景。

开始测评时，用户必须手填测评人和操作人。Cooper 测评文档由用户自己创建和维护，AI Cool 不创建、不编辑、不读取 Cooper 文档正文，也不校验文档权限。AI Cool 只保存 Cooper URL，并且只校验 URL 属于预期 Cooper 域名。开始测评时可以同时填写 Cooper URL，也可以先进入测评中后再关联或更新 Cooper URL；但完成纳入或不纳入前必须已有 Cooper URL，确保测评结论能追溯到外部文档。

测评中页必须展示正在测评的工具，并支持搜索工具或测评人。页面顶部展示测评中数量、已关联文档数量和待关联文档数量。列表字段至少包括工具名称、owner/repo、当前介绍、测评人、Cooper 测评记录、开始时间和测评结论操作。已关联文档的工具展示查看测评文档入口；未关联文档的工具展示关联文档入口。测评结论操作只有纳入和不纳入。

推荐的外部 Cooper 测评模板包括这些内容：工具名称与版本；测评目标，包括为什么测评和希望解决的实际问题；使用环境，包括使用版本、安装或接入方式、账号、费用和权限要求；实际任务记录，包括测试任务、操作过程、实际结果、失败或人工接管情况；结论，包括能解决什么问题、主要限制和风险、是否建议纳入团队工具。这个模板只约束外部 Cooper 文档建议结构，不要求 AI Cool 做站内测评编辑器。

测评完成后只有纳入团队工具和不纳入两个选择。完成为 included 时，系统记录 result=included、完成时间、纳入时间、测评人确认的一句话说明，并让工具进入团队工具列表。完成为 excluded 时，系统记录 result=excluded、完成时间、不纳入原因、excluded_stage=evaluating 和 Cooper 链接，工具不进入团队工具列表。已发现阶段的不处理同样转为 excluded，但 excluded_stage=discovered，并记录不处理原因。

团队工具页只展示完成测评且明确纳入的工具。列表字段至少包括工具名称、owner/repo、团队使用说明、测评人、纳入时间、GitHub 链接和 Cooper 测评链接。页面要支持搜索团队工具，展示已纳入数量、测评人数、更新时间和结果数量。首期不建设复杂分类、评分、评论、推荐理由页、社区、投票、收藏或工具详情推荐页。工具数量增加后，再根据真实使用情况决定是否增加搜索和分类之外的能力。

去重和状态流转是硬约束。GitHub node_id 是仓库唯一标识，仓库改名或 URL 变化不能创建新工具。同一仓库被多个关键词或 Topic 命中时，只保留一个工具，发现来源合并保存。重复命中要更新最近发现或最近命中时间。已经处于 evaluating、included 或 excluded 的工具再次命中时，不改变状态，也不会重新出现在已发现列表。excluded 工具继续保留以避免反复被发现。

每周只为 discovered、evaluating 和 included 工具保存 Stars 快照；excluded 工具不需要继续保存新快照。发现运行层必须记录分页上限、实际页数、结果数量、incomplete_results、截断情况、GitHub rate limit 信息、reset 时间、bad query、网络或 API 错误。部分失败或不完整的 GitHub 运行必须记录为部分或失败状态，不能破坏已有工具数据，也不能静默伪装为完整成功。

实现后续设计和计划时必须贴合当前仓库形态。后端延续 Go 加 PostgreSQL、模块内 schema.sql 和 EnsureSchema、共享 pgxpool、publicapi handler、双 HTTP 入口注册的模式。工具域要独立于现有 raw_items 和 items，因为工具候选需要稳定仓库身份、合并来源、状态历史、测评记录和 Stars 快照。前端延续 React、Vite、TypeScript、hash route 和自维护 CSS，新增左侧工具导航和上述四个页面。部署计划要包含 discovertools 命令和每周 cron 集成。

必须能从需求中推导出的闭环 case 包括：新建关键词配置；新建 Topic 配置；在配置内新增、查看、编辑、删除检索词或 Topic；编辑配置；启用配置；停用配置；软删除配置并保留历史；手动执行配置；每周执行启用配置；手动添加 GitHub URL 并先预览后提交；关键词、Topic、手动来源重复命中合并；已发现工具开始测评且支持稍后关联 Cooper 文档；已发现工具不处理；测评中工具关联或更新 Cooper 文档；测评完成为纳入；测评完成为不纳入；团队工具列表只展示 included 工具；已发现列表按最新发现、当前 Stars、近 7 天 Stars 增长排序；缺少 Stars 快照时显示暂无；记录 GitHub 限流、不完整、坏查询和失败信息；并发状态操作必须被拒绝或串行化，例如重复开始测评、完成非 evaluating 工具、删除正在执行中的配置。

## Sources

- External: `https://cooper.didichuxing.com/didocs/2209600482571` at `section 5 local snapshot read 2026-09-09` — Baseline tool discovery and evaluation requirements including statuses, GitHub discovery modes, UI pages, evaluation record, team tools, deduplication rules, and prototype UI signals
- External: `codex-thread:01a07b13-0116-7c53-8703-40cc9e6a6ebd` at `2026-09-09` — User confirmed section 5 is authoritative, evaluator and operator are manually entered, users create Cooper documents outside AI Cool, discovery configuration page is first-release scope, and configuration deletion should be first-release scope
- Repository: `server/cmd/webserver/main.go` at `f425836b709266ff12dd1741684dcd5925de8ebd` — Current Go HTTP entrypoint and public route registration pattern
- Repository: `web/src/App.tsx` at `f425836b709266ff12dd1741684dcd5925de8ebd` — Current frontend hash route and view switching pattern
- Repository: `web/package.json` at `f425836b709266ff12dd1741684dcd5925de8ebd` — Current React Vite TypeScript test and lint stack
- Repository: `docs/design/ai-cool-2.0-product-technical-plan/README.md` at `f425836b709266ff12dd1741684dcd5925de8ebd` — Historical first phase read-only and report-import plan superseded by Cooper section 5 for this module
