---
name: dingtalk-devapp
description: 钉钉开放平台应用管理。Use when 用户说 开发者后台应用/开放平台应用/企业内部应用/查应用/创建应用/修改应用/删除应用/应用成员/安全配置/IP 白名单/登录重定向/端内免登/agentId/clientId/appKey/appSecret/应用权限/权限点/事件订阅/版本发布/审核。Distinct from dingtalk-devdoc(开放平台文档搜索) and dingtalk-doc(钉钉云文档)。命令前缀：dws devapp（兼容别名可为 dws app）。
cli_version: ">=1.0.15"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉开放平台应用 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。接口、命名与跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the root `dws` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ `dws devapp ...` 是开源分支内置 helper 命令，不依赖 MCP 服务发现；真实调用 endpoint 由内部 edition 通过 `SupplementServers/StaticServers` 注入，`DINGTALK_DEVAPP_MCP_URL` 仅作本地调试覆盖。不得把 endpoint/key 写进仓库。执行前用 `dws devapp --help` 或 `--dry-run` 验证。

> 命令参考：[devapp.md](references/devapp.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "查应用 / 搜索应用 / 分页查应用" | `dws devapp list --name "<应用名>" --format json` |
| "看应用详情 / agentId / clientId / appKey" | `dws devapp get --unified-app-id <UNIFIED_APP_ID> --format json` |
| "创建企业内部应用" | `dws devapp create --name "<名称>" --type internal --dry-run --format json`，确认后加 `--yes` |
| "修改应用名称 / 描述 / 图标" | 先唯一定位应用，再 `dws devapp update --unified-app-id <UNIFIED_APP_ID> ... --dry-run`，确认后加 `--yes` |
| "删除应用" | 先唯一定位并展示风险，再 `dws devapp delete --unified-app-id <UNIFIED_APP_ID> --yes --format json` |
| "查看应用成员 / 开发者成员" | `dws devapp member list --app-id <UNIFIED_APP_ID> --format json` |
| "添加应用成员 / 开发者成员" | `dws devapp member add --app-id <UNIFIED_APP_ID> --users <userId1,userId2> --member-type DEVELOPER --dry-run --format json`，确认后加 `--yes` |
| "移除应用成员 / 开发者成员" | `dws devapp member remove --app-id <UNIFIED_APP_ID> --users <userId> --member-type DEVELOPER --dry-run --format json`，确认后加 `--yes` |
| "配置 IP 白名单 / 登录重定向 URL / 端内免登 URL" | `dws devapp security config --app-id <UNIFIED_APP_ID> ... --dry-run --format json`，确认后加 `--yes` |
| "查询 / 搜索权限点" | `dws devapp permission list --unified-app-id <UNIFIED_APP_ID> --keyword "<关键词>" --format json` |
| "这个权限覆盖哪些 API" | `dws devapp permission list --unified-app-id <UNIFIED_APP_ID> --scope <scopeValue> --format json` |
| "申请权限 / 开通权限" | 先从列表拿 `scopeValue`，再 `dws devapp permission add --unified-app-id <UNIFIED_APP_ID> --permissions <scopeValue> --dry-run`，确认后加 `--yes` |
| "取消权限 / 移除权限" | `dws devapp permission remove --unified-app-id <UNIFIED_APP_ID> --permission <scopeValue> --dry-run`，确认后加 `--yes` |
| "拿 appSecret / clientSecret" | `dws devapp credentials get ...`；后端未发布时说明缺口，不能用 `get` 冒充 |

## 应用意图消歧

`应用` 是泛词。只有下列信号明确时才使用本 skill：

- 用户说 `开放平台应用`、`开发者后台应用`、`企业内部应用`、`内部应用`。
- 用户提到 `agentId/clientId/appKey/appSecret/customKey`。
- 用户要处理 `应用成员/开发者成员/成员类型/安全配置/IP 白名单/登录重定向 URL/端内免登 URL`。
- 用户要处理 `应用权限/权限点/API 权限/APP 或 SNS 权限/事件订阅/应用版本/发布审核`。
- 当前上下文明确是 yulan 的 OpenDev 应用 CLI 化工作流。

不要把以下请求路由到 `devapp`：

- 开放平台接口文档、错误码、字段说明 → `dingtalk-devdoc`。
- 钉钉文档、云文档、知识库内文档 → `dingtalk-doc` 或知识库预检。
- 工作台应用、`app001/appXYZ` 这类工作台 app id → `workbench app`。
- MCP 服务、connector、HSF tool 创建/映射/上架 → OpenDev MCP 平台流程。
- OA 审批单查看、同意、拒绝、撤销 → `dingtalk-oa`。

如果用户只说 `应用` 且没有上下文，先追问应用类型，不要默认执行创建、修改、删除。

## 硬约束

- 所有命令加 `--format json`。
- 写操作先 `--dry-run`，确认后才加 `--yes`。
- 不把 `confirmCreate/confirmUpdate/confirmDelete/confirmPermission/confirmMember/confirmSecurity` 作为 MCP 参数。
- 应用名、appKey、customKey 命中多条时不能默认取第一条。
- 权限申请/取消只接受 `permissions[].scopeValue`，不要传 API 名、API uuid 或分组名。
- 权限列表默认同时展示 APP 应用权限和 SNS 个人权限；只有用户明确要求时才加 `--scope-type APP|SNS`。
- `requiredApproval=true` 的权限仍可申请；申请后加入当前应用版本变更，发布时继续审核。
- `app get` 不读取完整 secret；完整 secret 只能走 `credentials get` 并按审计/确认规则处理。

## 跨产品协作

- 开放平台接口文档、错误码、字段说明 → 切到 `dingtalk-devdoc`。
- 审批单处理、OA 审批详情 → 切到 `dingtalk-oa`。
- 钉钉云文档读写 → 切到 `dingtalk-doc`。
