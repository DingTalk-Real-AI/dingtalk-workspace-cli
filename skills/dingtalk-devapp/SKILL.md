---
name: dingtalk-devapp
description: 钉钉开放平台应用管理。Use when 用户说 开发者后台应用/开放平台应用/企业内部应用/查应用/创建应用/修改应用/删除应用/停用应用/启用应用/应用成员/安全配置/IP白名单/登录重定向/端内免登/agentId/clientId/appKey/appSecret/应用权限/权限点/创建机器人/智能体机器人/机器人配置/机器人回调/应用版本/版本发布/发布审核/选审批人/事件订阅/订阅事件/退订事件/eventCode。Distinct from dingtalk-devdoc(开放平台文档搜索) and dingtalk-doc(钉钉云文档)。命令前缀：dws devapp（兼容别名 dws app）。
cli_version: ">=1.0.15"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉开放平台应用管理 Skill

> `dws devapp ...` 是内置 helper 命令，不依赖 MCP 服务发现。执行前用 `dws devapp --help` 验证可用。

## 意图消歧

`应用` 是泛词。只有出现以下信号才路由到 devapp：

- `开放平台应用/开发者后台应用/企业内部应用/内部应用`
- `agentId/clientId/appKey/appSecret/customKey`
- `应用权限/权限点/scopeValue/应用成员/安全配置/IP 白名单`
- `创建机器人/智能体机器人/机器人配置/机器人回调地址`（→ `robot`）
- `应用版本/版本发布/发布审核/选审批人`（→ `version`）
- `事件订阅/订阅事件/退订事件/eventCode/可订阅事件`（→ `event`）

不要路由到 devapp 的：接口文档→`devdoc`、钉钉文档→`doc`、工作台应用→`workbench app`、审批→`oa`。只说 `应用` 无上下文时先追问。

## 核心规则

1. 所有命令加 `--format json`。
2. 写操作先 `--dry-run`，确认后才加 `--yes`。
3. 应用名/appKey/customKey 命中多条时展示候选，不取第一条。
4. 权限申请/取消只接受 `scopeValue`，不传 API 名或分组名。
5. 主动读取密钥走 `credentials get`；任何 `devapp get`/详情返回里的 `clientSecret/appSecret` 都按敏感凭证脱敏，不向用户展开。

## 开放平台文档 RAG / 错误码排查

- 任何产品执行中，只要用户问开放平台 API、接口参数、字段含义、权限点、回调、SDK、配额、错误码，或命令返回上游 OpenAPI/SDK 错误，必须先用 `dws devdoc article search --query "<关键词>" --format json` 做官方文档 RAG。
- 查询词优先保留原始 API 名、能力名、权限点、完整错误码和 message；首轮形如 `errcode <code> <message>`，无结果再换 `<产品/场景> <错误码>`、`<接口名> 参数`。
- 本地 CLI 错误（如 `unknown command` / `unknown flag` / 认证 / recovery）仍按 root `dws` skill 的错误处理执行；`devdoc` 用于开放平台业务错误码和接口语义排查。
- `devdoc` 只查钉钉开放平台开发者文档，不查业务数据；排查结论必须基于命中条目的标题、摘要或链接，不能编造错误原因或不存在的命令。

## 渐进式参考

按使用频率和复杂度分层，按需加载：

| 层 | 参考文档 | 覆盖命令 |
|----|---------|---------|
| 基础 | [app-crud.md](references/app-crud.md) | list / get / create / update / delete / inactive / active |
| 凭证与网页应用 | [credentials-webapp.md](references/credentials-webapp.md) | credentials get / webapp get / webapp config |
| 权限管理 | [permissions.md](references/permissions.md) | permission list / permission add / permission remove |
| 成员与安全 | [member-security.md](references/member-security.md) | member list / add / remove / security config |
| 机器人 | [robot.md](references/robot.md) | robot create / submit / result / get / config / update / enable / offline |
| 版本发布 | [version.md](references/version.md) | version create / list / get / check-approval / publish / status |
| 事件订阅 | [event.md](references/event.md) | event list / subscribe / unsubscribe |
| 操作流程 | [workflows.md](references/workflows.md) | 创建应用全流程 / 权限全流程 / 生命周期管理 |
