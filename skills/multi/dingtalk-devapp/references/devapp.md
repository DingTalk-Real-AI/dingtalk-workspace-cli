# 开放平台应用 (devapp) 命令参考

管理钉钉开放平台企业内部应用。用于应用列表查询、应用详情、创建/修改/删除、成员管理、安全配置、凭证读取、权限查询/申请/取消，以及事件订阅和版本发布目标链路。

> 当前开源分支已内置 `dws devapp ...` helper 命令树，兼容别名为 `dws app ...`。本能力不依赖 MCP 服务发现或 registry overlay；真实调用 endpoint 由内部 edition 通过 `SupplementServers/StaticServers` 注入。`DINGTALK_DEVAPP_MCP_URL` 仅用于本地调试覆盖，严禁把 endpoint/key 写入仓库。
> 若 `dws devapp --help` 与本文冲突，以运行时输出为准。

## 命名

```bash
dws devapp ...
```

`dws app ...` 只作为兼容别名；不要把 `opendev/apps` 作为新的正式命令根。

## 核心规则

- 所有命令给 Agent 使用时加 `--format json`。
- 写操作先 `--dry-run` 展示计划；用户确认后才加 `--yes`。
- MCP/HSF 入参不接收 `confirmCreate/confirmUpdate/confirmDelete/confirmPermission/confirmMember/confirmSecurity`。
- 应用名、appKey、customKey 命中多条时不能默认选第一条。
- `corpId/userId` 由 MCP 系统上下文映射，用户不传 `orgId/uid`。
- `app get` 不读取完整 secret；`clientSecret/appSecret` 只能走 `credentials get`。
- 权限申请/取消只接受 `scopeValue`，不要把 API 名、API uuid、分组名直接传给写工具。

## 应用意图消歧

`应用` 不是 `devapp` 的充分条件。只有出现以下 OpenDev 信号时才选择本产品：

- `开放平台应用/开发者后台应用/企业内部应用/内部应用`。
- `agentId/clientId/appKey/customKey/appSecret/clientSecret`。
- `应用成员/开发者成员/成员类型/安全配置/IP 白名单/登录重定向 URL/端内免登 URL`。
- `应用权限/权限点/API 权限/APP 权限/SNS 个人权限/scopeValue`。
- `事件订阅/callbackUrl/token/aesKey` 且上下文是开放平台应用。
- `应用版本/版本发布/选审批人/发布审核` 且上下文是开发者后台应用。
- 当前对话明确在 yulan 的开放平台应用 CLI 化工作流里。

不要误用 `devapp` 的场景：

| 用户意图 | 应走产品/流程 |
| --- | --- |
| 开放平台接口文档、错误码、字段说明 | `devdoc` |
| 钉钉文档、云文档、知识库内文档 | `doc` 或知识库预检 |
| 工作台应用、`app001/appXYZ` 详情 | `workbench app` |
| MCP 服务、connector、HSF tool 创建/映射/上架 | OpenDev MCP 平台流程 |
| OA 审批单查看、同意、拒绝、撤销 | `oa` |

只说"应用"且缺少上下文时，先追问应用类型；不要默认执行创建、修改、删除。

## 应用定位

写操作前必须定位到唯一应用。

| 优先级 | 标识 | 处理 |
| --- | --- | --- |
| 1 | `unifiedAppId` | 直接使用。 |
| 2 | `agentId/appId` | 直接使用，必要时补详情。 |
| 3 | `appKey/clientId` | 先查询，唯一命中才继续。 |
| 4 | `customKey` | 先查询，唯一命中才继续。 |
| 5 | `appName/name` | 模糊搜索，写操作必须唯一命中。 |

候选展示字段固定使用 `appName/unifiedAppId/agentId/appKey/creator/gmtModified`。

## 应用列表

```bash
dws devapp list --page 1 --page-size 20 --format json
dws devapp list --name DemoApp --format json
dws devapp list --agent-id AGENT_ID --format json
dws devapp list --app-key dingxxx --format json
dws devapp list --creator 张三 --format json
dws devapp list --robot-name 小钉 --format json
dws devapp list --sort gmt_modified --order desc --format json
```

MCP tool: `list_open_dev_apps_by_condition`

| CLI | MCP | 说明 |
| --- | --- | --- |
| `--page` | `currentPage` | 1-based。 |
| `--page-size` | `pageSize` | 分页大小。 |
| `--name/--keyword` | `appName` | 应用名搜索。 |
| `--agent-id` | `agentId` | 精确定位。 |
| `--app-id` | `appId` | 兼容字段。 |
| `--app-key` | `appKey` | appKey/clientId。 |
| `--custom-key` | `customKey` | 自定义 key。 |
| `--creator` | `creator` | 创建人。 |
| `--robot-name` | `robotName` | 机器人名称。 |
| `--sort` | `sortType` | 如 `gmt_modified`。 |
| `--order` | `sortOrder` | `asc/desc`。 |

## 应用详情

```bash
dws devapp get --unified-app-id UNIFIED_APP_ID --format json
dws devapp get --agent-id AGENT_ID --format json
dws devapp get --name DemoApp --format json
```

MCP tool: `get_open_dev_app_detail`

详情可以展示 `agentId/clientId/appKey`，但不能展示完整 `clientSecret/appSecret`。

## 创建应用

```bash
dws devapp create --name DemoApp --desc "内部应用" --type internal --dry-run --format json
dws devapp create --name DemoApp --desc "内部应用" --type internal --yes --format json
```

MCP tool: `create_inner_app`

当前 P0 只支持企业内部应用基础创建：

| 类型 | 支持 | 说明 |
| --- | --- | --- |
| `internal` | 是 | 默认企业内部应用；`--type internal` 只做 CLI guard，不下发 MCP。 |
| `h5` | 否 | 当前 helper 不通过创建接口直接创建 H5 类型；网页应用能力通过 `webapp config` 单独配置，再用 `webapp get` 查询。 |
| `robot` | 否 | 机器人配置是独立能力。 |
| `miniapp/isv/connector` | 否 | 不属于 yulan P0 创建。 |

## 修改应用

```bash
dws devapp update --unified-app-id UNIFIED_APP_ID --name DemoApp2 --desc "新描述" --dry-run --format json
dws devapp update --unified-app-id UNIFIED_APP_ID --name DemoApp2 --desc "新描述" --yes --format json
```

MCP tool: `update_inner_app`

至少提供一个更新字段：`appName/appDesc/appIcon/international/supportHarmony`。

## 删除应用

```bash
dws devapp delete --unified-app-id UNIFIED_APP_ID --dry-run --format json
dws devapp delete --unified-app-id UNIFIED_APP_ID --yes --format json
```

MCP tool: `delete_inner_app`

删除前必须展示被删应用摘要。允许的定位字段是 `unifiedAppId/agentId/appId/appName/appKey/customKey`，但名称和 key 必须唯一命中。

## 凭证读取

```bash
dws devapp credentials get --unified-app-id UNIFIED_APP_ID --format json
dws devapp credentials get --unified-app-id UNIFIED_APP_ID --show-secret --yes --format json
```

目标 MCP tool: `get_open_dev_app_credentials`

当前后端 facade 待补。默认输出应为脱敏值；完整 `clientSecret/appSecret` 需要显式确认和审计。

## 网页应用配置

```bash
dws devapp webapp get --agent-id AGENT_ID --format json
dws devapp webapp config --agent-id AGENT_ID --homepage-link https://example.com/mobile --pc-homepage-link https://example.com/pc --omp-link https://example.com/admin --dry-run --format json
dws devapp webapp config --agent-id AGENT_ID --homepage-link https://example.com/mobile --pc-homepage-link https://example.com/pc --omp-link https://example.com/admin --yes --format json
```

MCP tools: `get_webapp_config`, `set_webapp_config`

规则：

- `webapp config` 是写操作，必须先 `--dry-run`，用户确认后再 `--yes`。
- `webapp get` 在未配置网页应用前可能只返回 `agentId`。
- `webapp config` 成功后再调用 `webapp get`，应返回 `agentId/h5PageType/homepageLink/pcHomepageLink/ompLink`。
- `h5PageType` 未显式传入时，后端实测默认返回 `all`。

## 成员管理

```bash
dws devapp member list --app-id UNIFIED_APP_ID --format json
dws devapp member add --app-id UNIFIED_APP_ID --users userId1,userId2 --member-type DEVELOPER --dry-run --format json
dws devapp member add --app-id UNIFIED_APP_ID --users userId1,userId2 --member-type DEVELOPER --yes --format json
dws devapp member remove --app-id UNIFIED_APP_ID --users userId1 --member-type DEVELOPER --dry-run --format json
dws devapp member remove --app-id UNIFIED_APP_ID --users userId1 --member-type DEVELOPER --yes --format json
```

MCP tools: `list_open_dev_app_members`, `add_open_dev_app_members`, `remove_open_dev_app_members`

| CLI | MCP | 说明 |
| --- | --- | --- |
| `--app-id` | `unifiedAppId` | 开放平台统一应用 ID。 |
| `--users` | `memberUserIds` | 添加/移除的用户 userId 列表，支持英文逗号或分号分隔，空白会裁剪。 |
| `--member-type` | `memberType` | 成员类型，如 `DEVELOPER`；值使用后端枚举。 |

规则：

- `member list` 是查询操作，不需要 `--dry-run` 或 `--yes`。
- `member add/remove` 是写操作，必须先 `--dry-run`，用户确认后再 `--yes`。

## 安全配置

```bash
dws devapp security config --app-id UNIFIED_APP_ID --ip-whitelist 103.211.230.150 --dry-run --format json
dws devapp security config --app-id UNIFIED_APP_ID --redirect-url https://example.com/callback --yes --format json
dws devapp security config --app-id UNIFIED_APP_ID --ip-whitelist 103.211.230.150,103.211.230.151 --redirect-url https://example.com/callback --sso-url https://example.com/sso --dry-run --format json
```

MCP tool: `update_app_security_config`

| CLI | MCP | 说明 |
| --- | --- | --- |
| `--app-id` | `unifiedAppId` | 开放平台统一应用 ID。 |
| `--ip-whitelist` | `ipWhiteList` | IP 白名单，支持英文逗号或分号分隔。 |
| `--redirect-url` | `redirectUrls` | 登录重定向 URL，支持英文逗号或分号分隔。 |
| `--sso-url` | `otherAuthUrls` | 端内免登 URL，支持英文逗号或分号分隔。 |

规则：

- `security config` 是写操作，必须先 `--dry-run`，用户确认后再 `--yes`。
- 只下发显式提供且非空的列表字段；未提供的配置不会被覆盖。
- 当前不表达清空列表语义，不要用空字符串、`[]` 或占位值试图清空配置。

## 权限列表 / 搜索 / 详情

权限列表对齐开发者后台：

```text
GET /openapp/unifiedapp/{unifiedAppId}/scope/list?from=inner&key={keyword}&apiStatus={apiStatus}
```

默认必须同时返回 APP 应用权限和 SNS 个人权限。权限列表没有服务端分页；用 `keyword` 缩小结果，用 `limit` 控制 Agent 上下文。

```bash
dws devapp permission list --unified-app-id UNIFIED_APP_ID --format json
dws devapp permission list --unified-app-id UNIFIED_APP_ID --status unauthed --keyword "机器人发送消息" --format json
dws devapp permission list --unified-app-id UNIFIED_APP_ID --scope-type SNS --format json
dws devapp permission list --unified-app-id UNIFIED_APP_ID --scope qyapi_robot_sendmsg --format json
```

MCP tool: `list_open_dev_app_permissions`

| CLI | MCP | 说明 |
| --- | --- | --- |
| `--unified-app-id` | `unifiedAppId` | 应用定位。 |
| `--agent-id` | `agentId` | 应用定位。 |
| `--keyword` | `keyword` | 映射 Web `key`。 |
| `--api-status` | `apiStatus` | 映射 Web `apiStatus`。 |
| `--status` | `authStatus` | `all/authed/unauthed`。 |
| `--scope-type` | `firstLevelType` | `APP/SNS`；为空返回两者。 |
| `--scope` | `scopeValue` | 单权限详情模式。 |
| `--limit` | `limit` | 默认 20，建议硬上限 50。 |

Agent 选择顺序：

1. 精确匹配 `scopeValue`。
2. 匹配 `apiList[].name`。
3. 匹配权限名称/描述。
4. 匹配分类标题和 `APP/SNS` 类型。

列表模式只返回 `apiPreview`；完整 `apiList` 只在 `--scope` 详情模式返回。

`requiredApproval=true` 的权限也可申请；申请后加入当前应用版本变更，发布时进入审核流程。

## 申请权限

```bash
dws devapp permission add --unified-app-id UNIFIED_APP_ID --permissions qyapi_robot_sendmsg --dry-run --format json
dws devapp permission add --unified-app-id UNIFIED_APP_ID --permissions qyapi_robot_sendmsg --yes --format json
```

MCP tool: `apply_open_dev_app_permissions`

- `scopeValues` 必须来自权限列表返回的 `permissions[].scopeValue`。
- 已开通权限跳过或提示，不重复申请。
- 不存在、不可编辑权限拒绝。
- 需审核权限允许申请，写入版本变更。
- 不在权限 add 内选审批人，审批放到版本发布链路。

## 取消权限

```bash
dws devapp permission remove --unified-app-id UNIFIED_APP_ID --permission qyapi_robot_sendmsg --dry-run --format json
dws devapp permission remove --unified-app-id UNIFIED_APP_ID --permission qyapi_robot_sendmsg --yes --format json
```

MCP tool: `remove_open_dev_app_permission`

一次只取消一个权限点。未开通返回 `NOT_AUTHED`；不可编辑返回 no-edit 原因。取消已开通权限不等于撤销待审批申请。

## 事件订阅和版本发布

目标命令：

```bash
dws devapp event list --unified-app-id APP_ID --format json
dws devapp event config --unified-app-id APP_ID --callback-url https://example.com/callback --events EVENT_A,EVENT_B --dry-run --format json
dws devapp version create --unified-app-id APP_ID --version 1.0.0 --desc "release" --format json
dws devapp version check-approval --unified-app-id APP_ID --version-id VERSION_ID --format json
dws devapp version publish --unified-app-id APP_ID --version-id VERSION_ID --approver USERID --yes --format json
dws devapp version status --unified-app-id APP_ID --version-id VERSION_ID --format json
```

目标工具：`list_open_dev_app_events`、`configure_open_dev_app_event`、`create_open_dev_app_version`、`check_open_dev_app_version_approval`、`publish_open_dev_app_version`、`get_open_dev_app_version_status`。

权限申请中的 `requiredApproval=true` 只把权限写入版本变更，后续由版本链路负责审核选人和发布。

## Agent 工作流

```text
用户意图
  -> 归一化命令
  -> 收集应用标识
  -> 解析唯一应用
  -> 权限意图再解析 scopeValue
  -> 写操作先 dry-run
  -> 用户确认后加 --yes
  -> 调 MCP tool
  -> 透传 ServiceResult.errorCode/errorMsg
```

## 调用与排错

新环境先做运行时预检：

```bash
dws devapp --help
dws app --help
```

如果 `dws devapp --help` 不存在，说明本地 CLI 构建没有 devapp helper；如果执行时报 `endpoint_not_resolved`，让内部发行版负责人检查 `SupplementServers/StaticServers` 注入。不要改用 `devdoc/doc` 处理应用管理请求。

错误处理：

| 情况 | Agent 行为 |
| --- | --- |
| `unknown command` | 报告 `DEVAPP_COMMAND_UNAVAILABLE`，提示使用包含 devapp helper 的 CLI 构建。 |
| `endpoint_not_resolved` | 报告 `DEVAPP_ENDPOINT_NOT_CONFIGURED`，要求检查内部 edition endpoint 注入；本地调试可临时用 `DINGTALK_DEVAPP_MCP_URL`。 |
| 后端 tool 缺失 | 报告缺失的 MCP tool key，不用相近命令替代。 |
| 多应用命中 | 展示 `appName/unifiedAppId/agentId/appKey/creator/gmtModified`，停止写操作。 |
| 权限候选过多 | 加 `--keyword`、`--scope-type` 或 `--scope` 缩小；不要直接申请第一条。 |
| `requiredApproval=true` | 可以申请；申请后进入版本变更，发布时审核。 |
| `credentials get` 不可用 | 报告凭证工具未发布，不用 `app get` 推断 secret。 |
| `ServiceResult.success=false` | 原样透传 `errorCode/errorMsg`。 |

## 仓库内完整设计

如果正在源码仓库中开发，完整 helper 映射、P0 RPC 出入参契约、Agent 裁剪规则、上线验收清单和验收用例见：

```text
docs/devapp-yulan-command-routing.md
```
