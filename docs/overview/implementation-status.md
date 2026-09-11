# 实现状态

最后更新：2026-09-09

## 已实现

- AI 资讯采集、富化、入库主链路已存在。
- 公共 HTTP 入口 `server/cmd/webserver/main.go` 已注册版本、健康检查、资讯列表、日报、热点、图谱和详情页。
- 前端已支持日报、精选、全部动态和图谱视图。
- 部署侧已有 `pulse`、`gendaily`、`hotpass` 三类定时任务。
- AI Cool 2.0 工具发现 PRD 已在本地整理，包含 Cooper 第 5 部分需求、图片、封闭链路 case、接口和前端方案。

## 部分实现或历史方案

- `docs/design/ai-cool-2.0-product-design.md` 和原型记录过工具雷达、评测和工具箱方向。
- `docs/design/ai-cool-2.0-product-technical-plan/README.md` 是历史技术方案，其中“首期只读、Git Markdown 报告导入”的口径已被本次 PRD 覆盖。
- `docs/superpowers/` 下存在历史 specs/plans，只用于追溯，不直接作为当前实现事实。

## 未实现

- 工具发现配置、GitHub 搜索、手动添加和去重入库。
- 工具状态机、测评记录、Cooper 链接保存和团队工具列表。
- 工具相关 API、前端页面、部署命令和 cron。
- 工具模块的数据表和 Store。
- 工具模块测试和端到端验收数据。

## 当前风险

- 仓库当前为 dirty 状态，包含本地 PRD、图片和 DevKit 文档上下文。
- 本次工具需求引入写操作，但首期不接 SSO/RBAC，只靠手填操作人留痕；公网部署前需要重新评估写接口开放范围。
- GitHub API 有限流和搜索上限，需要 token、分页上限、时间窗口拆分和失败记录。
- Cooper 链接首期只校验域名，无法证明文档真实存在或有权限访问。

## 下一步

按 DevKit 流程继续：

```text
sop-init 校验通过
  -> sop-clarify 固化 RequirementDocRef
  -> 用户审批需求
  -> sop-design 生成并审批技术 DesignDoc
  -> sop-plan 生成开发计划
```
