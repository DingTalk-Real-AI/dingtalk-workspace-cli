# yulan Open Platform App Command Routing

> Status: draft, 2026-06-05
> Branch: `feat/dws-devapp`
> Scope: open-source DWS integration for DingTalk Open Platform enterprise app management.

This document splits the application-side command set for yulan. It is written for the open-source DWS repository and must not contain personal MCP gateway keys, pre-release tokens, cookies, or production app data.

## 0. Current Repository Deliverables

The current branch contains the application-side command contract and Agent
routing assets. It does not by itself prove that the `devapp` runtime command
tree is already published in an MCP registry. Treat runtime availability as an
external release condition until the rollout checklist in section 13 passes.

| Artifact | Status | Evidence |
| --- | --- | --- |
| Main design document | Done in repo | `docs/devapp-yulan-command-routing.md` covers naming, command tree, P0 RPC contracts, permissions, Agent routing, rollout, and acceptance cases. |
| README discoverability | Done in repo | `README.md` and `README_zh.md` link to this design under Reference/参考文档. |
| Mono skill routing | Done in repo | `skills/mono/SKILL.md` routes Open Platform app intents to `devapp`. |
| Mono product reference | Done in repo | `skills/mono/references/products/devapp.md` summarizes app CRUD, permissions, credentials, events, versions, and troubleshooting. |
| Multi product skill | Done in repo | `skills/multi/dingtalk-devapp/` defines the standalone `dingtalk-devapp` skill and reference file. |
| Agent intent test cases | Done in repo | `test/skill_tests.md` includes 21 devapp cases plus devdoc/workbench counterexamples for the overloaded word `应用`; `test/skill_tests_results.md` reports those cases passing. |
| Skill test parser | Done in repo | `test/run_skill_tests.py` now preserves the current `#### dws ...` command section for each case. |
| Static/e2e skill verifier handling | Done in repo | `test/skill_static` and `test/skill_e2e` skip `dws devapp ...` as design-only until the MCP product is published. |
| Sensitive config guard | Done in repo | `.gitignore` excludes local `docs/mcp/serviceconfig-pre*` files so personal MCP gateway keys are not staged. |
| MCP HSF tool configuration | External dependency | Configure and publish tools on the DingTalk MCP developer platform using sections 3, 4, and 13. |
| DWS runtime command availability | External dependency | Verify with `dws cache refresh --product devapp`, `dws schema`, and `dws devapp --help` after MCP registry publication. |
| Full production readiness | Not yet proven | Requires section 13 release exit criteria, including MCP `tools/list`, DWS schema, APP/SNS permission smoke, and write guard smoke. |

State labels:

1. `Done in repo` means the open-source branch contains the artifact and it is
   covered by local checks.
2. `External dependency` means backend/MCP registry publication must happen
   outside this repository.
3. `Not yet proven` means do not claim the application command set is ready for
   Agent production usage until runtime smoke evidence exists.

## 1. Naming

Use `devapp` as the canonical open-source product id:

```text
dws devapp ...
```

`app` can be kept as a compatibility alias if the runtime overlay or hardcoded helper supports it. Do not introduce `opendev` or `apps` as new canonical roots; if they are needed for migration, make them redirect or hidden aliases.

Reasoning:

1. `devapp` is specific enough for Open Platform developer applications.
2. `app` is the PRD/user-facing shorthand and can remain as an alias.
3. The open-source DWS repository is protocol-first; command availability ultimately comes from service discovery or helper commands.

Branch context:

```text
branch: feat/dws-devapp
base: latest upstream/main has been merged into the local design branch
runtime: devapp command availability still depends on MCP registry publication
```

The checked-in open-source helper code does not hardcode a complete `dws devapp`
command tree. The command design below is the target contract for MCP overlay
and/or helper implementation.

## 2. Command Tree

```text
dws devapp
  list                         Search app list with listByCondition semantics
  get                          Get one app detail
  create                       Create an enterprise internal app
  update                       Update app base information
  delete                       Delete an enterprise internal app
  credentials get              Controlled credential read, especially appSecret/clientSecret
  webapp get                   Get web app configuration
  webapp config                Configure web app entry URLs
  permission list              List APP and SNS permissions
  permission search            Search permission candidates, alias over list semantics
  permission detail            Show one permission's full API coverage
  permission add               Apply permissions into current app version changes
  permission remove            Remove one authorized permission
  event list                   List event subscription config
  event config                 Configure callback URL, token, aesKey, event types
  version create               Save app version
  version check-approval       Check whether publishing needs approval and approvers
  version publish              Submit publish, possibly creating approval
  version status               Poll publish and approval status
```

Open-source examples should use `--format json` for agent consumption:

```bash
dws devapp list --name DemoApp --page 1 --page-size 20 --format json
dws devapp get --unified-app-id UNIFIED_APP_ID --format json
dws devapp permission list --unified-app-id UNIFIED_APP_ID --format json
```

For write operations, DWS should use the existing open-source safety model:

```bash
dws devapp create --name DemoApp --type internal --desc "internal app" --dry-run --format json
dws devapp create --name DemoApp --type internal --desc "internal app" --yes --format json
```

Do not send confirmation fields such as `confirmCreate`, `confirmUpdate`, `confirmDelete`, `confirmPermission`, or `confirmSensitive` to MCP or HSF. Confirmation is only a CLI/Agent guard.

## 3. Capability Matrix

| Area | CLI command | MCP tool | Backend HSF facade | Current backend status | Open-source DWS status |
| --- | --- | --- | --- | --- | --- |
| App query | `devapp list` | `list_open_dev_apps_by_condition` | `OpenInnerAppQueryFacade.listByCondition` | Implemented | Needs discovery/overlay/helper |
| App detail | `devapp get` | `get_open_dev_app_detail` | `OpenInnerAppQueryFacade.getDetail` | Implemented | Needs discovery/overlay/helper |
| App create | `devapp create` | `create_open_dev_app` | `OpenInnerAppManageFacade.create` | Implemented | Needs discovery/overlay/helper |
| App update | `devapp update` | `update_open_dev_app` | `OpenInnerAppManageFacade.update` | Implemented | Needs discovery/overlay/helper |
| App delete | `devapp delete` | `delete_open_dev_app` | `OpenInnerAppManageFacade.delete` | Implemented | Needs MCP config + DWS wiring |
| Credentials | `devapp credentials get` | `get_open_dev_app_credentials` | Credential facade pending | Pending | Spec only |
| Web app | `devapp webapp get/config` | `get/config_open_dev_app_webapp` | Webapp facade pending | Pending | Spec only |
| Permission list | `devapp permission list/search/detail` | `list_open_dev_app_permissions` | `OpenInnerAppPermissionFacade.list` | Implemented | Needs MCP config + DWS wiring |
| Permission apply | `devapp permission add` | `add_open_dev_app_permissions` | `OpenInnerAppPermissionFacade.apply` | Implemented | Needs MCP config + DWS wiring |
| Permission remove | `devapp permission remove` | `remove_open_dev_app_permission` | `OpenInnerAppPermissionFacade.remove` | Implemented | Needs MCP config + DWS wiring |
| Events | `devapp event list/config` | `list/config_open_dev_app_events` | Event facade pending | Pending | Spec only |
| Version publish | `devapp version create/check-approval/publish/status` | `create/check/publish/get_open_dev_app_version_*` | Version facade pending | Pending | Spec only |

P0 for yulan is app CRUD and permissions. Credentials and web app config are yulan-adjacent but require additional backend contract. Events and version publish are lifecycle requirements but can be owned by a separate backend slice.

### 3.1 P0 RPC Contract

All P0 MCP tools should be configured as HSF-backed tools that call one facade
method with one business request object. Do not expose a separate
`BaseOpenRequestVO` to the Agent. The provider should receive caller identity
from MCP system context and merge it into the business request before executing
the facade method.

Common HSF mapping:

| Source | Target request field | Public CLI flag | Required | Notes |
| --- | --- | --- | --- | --- |
| MCP system org context | `corpId` | No | Yes | DingTalk corpId. Do not ask the user for internal `orgId`. |
| MCP system user context | `userId` | No | Yes | DingTalk user/staff id. Do not ask the user for internal `uid`. |
| CLI/MCP tool input | business fields | Yes | Per tool | Search, app locator, update fields, permission scopes. |

Common result wrapper:

```json
{
  "success": true,
  "result": {},
  "errorCode": "",
  "errorMsg": "",
  "arguments": []
}
```

MCP output configuration should keep the wrapper fields and configure success as
`ServiceResult.success == true`. DWS/Agent normalization can add `ok`, `reason`,
`summary`, and `nextAction`, but raw `errorCode/errorMsg` must remain visible.

### 3.2 P0 Tool Contracts

#### list_open_dev_apps_by_condition

| Item | Contract |
| --- | --- |
| CLI | `dws devapp list` |
| HSF facade | `OpenInnerAppQueryFacade.listByCondition` |
| Request shape | One object with system context and optional list filters. |
| Required public fields | None; default `currentPage=1`, `pageSize=20`. |
| Identity source | `corpId/userId` from MCP system context. |

Input fields:

| Field | Type | CLI flag | Required | Notes |
| --- | --- | --- | --- | --- |
| `currentPage` | number | `--page` | no | 1-based page number. |
| `pageSize` | number | `--page-size` | no | Default 20; cap by backend/overlay. |
| `appName` | string | `--name` | no | App name keyword. |
| `agentId` | number | `--agent-id` | no | Exact DingTalk app agentId. |
| `appId` | number | `--app-id` | no | Compatible app id. |
| `appKey` | string | `--app-key` | no | DingTalk appKey/clientId. |
| `customKey` | string | `--custom-key` | no | Custom key. |
| `appGroupId` | number | `--app-group-id` | no | App group id. |
| `creator` | string | `--creator` | no | Creator name keyword. |
| `robotName` | string | `--robot-name` | no | Bot name keyword. |
| `developType` | number/string | `--develop-type` | no | Use backend enum if schema exposes one. |
| `filterCoolApp` | number/boolean | `--filter-cool-app` | no | Only expose when backend semantics are documented. |
| `sortType` | string | `--sort` | no | Example: `gmt_modified`. |
| `sortOrder` | string | `--order` | no | `asc` or `desc`. |

Normalized response:

```json
{
  "ok": true,
  "operation": "app.list",
  "page": {"currentPage": 1, "pageSize": 20, "total": 0},
  "apps": [
    {
      "unifiedAppId": "UNIFIED_APP_ID",
      "appName": "DemoApp",
      "agentId": 123456,
      "appId": 123,
      "appKey": "dingxxx",
      "customKey": "custom-key",
      "creator": "Creator",
      "robotName": "BotName",
      "gmtModified": "2026-06-05T00:00:00+08:00"
    }
  ],
  "raw": {}
}
```

#### get_open_dev_app_detail

| Item | Contract |
| --- | --- |
| CLI | `dws devapp get` |
| HSF facade | `OpenInnerAppQueryFacade.getDetail` |
| Request shape | One object with system context and one or more app locator fields. |
| Required public fields | At least one locator. |
| Identity source | `corpId/userId` from MCP system context. |

Locator fields:

| Field | Type | CLI flag | Notes |
| --- | --- | --- | --- |
| `unifiedAppId` | string | `--unified-app-id` | Preferred. |
| `agentId` | number | `--agent-id` | Exact id. |
| `appId` | number | `--app-id` | Compatible id. |
| `appKey` | string | `--app-key` | appKey/clientId. |
| `customKey` | string | `--custom-key` | Custom key. |
| `appName` | string | `--name` | Fuzzy; must resolve uniquely before writes. |

Detail response should include identifiers and configuration summary, but not
full `clientSecret/appSecret`:

```json
{
  "ok": true,
  "operation": "app.get",
  "app": {
    "unifiedAppId": "UNIFIED_APP_ID",
    "appName": "DemoApp",
    "agentId": 123456,
    "appId": 123,
    "appKey": "dingxxx",
    "customKey": "custom-key",
    "developType": "internal",
    "creator": "Creator",
    "visibility": "partial_members",
    "status": "online",
    "gmtModified": "2026-06-05T00:00:00+08:00"
  },
  "raw": {}
}
```

#### create_open_dev_app

| Item | Contract |
| --- | --- |
| CLI | `dws devapp create` |
| HSF facade | `OpenInnerAppManageFacade.create` |
| Request shape | One object with system context and app base fields. |
| Required public fields | `appName`; default `type=internal`. |
| Write guard | CLI `--dry-run`, then `--yes`; no MCP confirm field. |

Input fields:

| Field | Type | CLI flag | Required | Notes |
| --- | --- | --- | --- | --- |
| `appName` | string | `--name` | yes | Enterprise internal app name. |
| `appDesc` | string | `--desc` | no | App description. |
| `type` | string | `--type` | no | P0 default: `internal`. |
| `appIcon` | string | `--icon` | no | Existing media/resource id if backend accepts it. |
| `hidden` | boolean | `--hidden` | no | Whether hidden from normal members. |

Response:

```json
{
  "ok": true,
  "operation": "app.create",
  "app": {
    "unifiedAppId": "UNIFIED_APP_ID",
    "appName": "DemoApp",
    "agentId": 123456,
    "appKey": "dingxxx"
  },
  "nextAction": "run devapp get or configure permissions",
  "raw": {}
}
```

#### update_open_dev_app

| Item | Contract |
| --- | --- |
| CLI | `dws devapp update` |
| HSF facade | `OpenInnerAppManageFacade.update` |
| Request shape | One object with system context, locator, and changed fields. |
| Required public fields | One app locator and at least one changed field. |
| Write guard | CLI `--dry-run`, then `--yes`; no MCP confirm field. |

Locator fields are the same as `get_open_dev_app_detail`, but the Agent should
prefer `unifiedAppId` after resolving a unique app.

Update fields:

| Field | Type | CLI flag | Notes |
| --- | --- | --- | --- |
| `appName` | string | `--name` | New app name. |
| `appDesc` | string | `--desc` | New app description. |
| `appIcon` | string | `--icon` | New icon resource id if backend accepts it. |
| `international` | object/string | `--international` | Only expose when schema describes the shape. |
| `supportHarmony` | boolean | `--support-harmony` | Harmony support switch if backend exposes it. |

Response should return the updated app summary plus a `changedFields` array when
available.

#### delete_open_dev_app

| Item | Contract |
| --- | --- |
| CLI | `dws devapp delete` |
| HSF facade | `OpenInnerAppManageFacade.delete` |
| Request shape | One object with system context and one unique app locator. |
| Required public fields | One unique app locator. |
| Write guard | CLI `--dry-run`, then `--yes`; no MCP confirm field. |

The Agent must display the resolved app summary before executing delete with
`--yes`. The response should include `deleted=true`, the app summary, and raw
backend fields.

#### list_open_dev_app_permissions

| Item | Contract |
| --- | --- |
| CLI | `dws devapp permission list/search/detail` |
| HSF facade | `OpenInnerAppPermissionFacade.list` |
| Request shape | One object with system context, app locator, and optional filters. |
| Required public fields | One app locator. |
| Web semantics | Equivalent to grouped `scope/list` for APP and SNS scopes. |

Input fields:

| Field | Type | CLI flag | Required | Notes |
| --- | --- | --- | --- | --- |
| `unifiedAppId` | string | `--unified-app-id` | one locator | Preferred. |
| `agentId` | number | `--agent-id` | one locator | Compatible. |
| `keyword` | string | `--keyword` | no | Maps to Web `key`. |
| `apiStatus` | string | `--api-status` | no | Web `apiStatus` filter. |
| `authStatus` | string | `--status` | no | `ALL/AUTHED/UNAUTHED`, post-filter. |
| `firstLevelType` | string | `--scope-type` | no | `APP/SNS`; empty returns both. |
| `scopeValue` | string | `--scope` | no | Detail mode and exact post-filter. |
| `limit` | number | `--limit` | no | Agent display cap; no server pagination. |

Normalized response should flatten `appScopes` and `snsScopes` into one
`permissions[]` list while preserving group metadata:

```json
{
  "ok": true,
  "operation": "permission.list",
  "mode": "list",
  "permissions": [
    {
      "scopeValue": "qyapi_robot_sendmsg",
      "scopeName": "企业内机器人发消息权限",
      "firstLevelType": "APP",
      "categoryTitle": "机器人",
      "authed": true,
      "canEdit": true,
      "requiredApproval": false,
      "allowedActions": ["view", "detail", "remove"],
      "apiCount": 4,
      "apiPreview": [{"name": "API name"}]
    }
  ],
  "summary": {"totalMatched": 1, "returnedCount": 1},
  "truncated": false,
  "raw": {}
}
```

#### add_open_dev_app_permissions

| Item | Contract |
| --- | --- |
| CLI | `dws devapp permission add` |
| HSF facade | `OpenInnerAppPermissionFacade.apply` |
| Request shape | One object with system context, app locator, and `scopeValues`. |
| Required public fields | One app locator and non-empty `scopeValues`. |
| Write guard | CLI `--dry-run`, then `--yes`; no MCP confirm field. |

Input fields:

| Field | Type | CLI flag | Required | Notes |
| --- | --- | --- | --- | --- |
| `unifiedAppId` | string | `--unified-app-id` | one locator | Preferred. |
| `agentId` | number | `--agent-id` | one locator | Compatible. |
| `scopeValues` | string[] | `--permissions` | yes | Comma-separated in CLI; must come from list response. |

Response must distinguish direct apply, version-staged apply, rejected scopes, and
skipped scopes:

```json
{
  "ok": true,
  "operation": "permission.add",
  "appliedScopeValues": ["Contact.User.mobile"],
  "directAppliedScopeValues": [],
  "versionStagedScopeValues": ["Contact.User.mobile"],
  "rejectedScopeValues": [],
  "skippedScopeValues": [],
  "nextAction": "run devapp version check-approval",
  "raw": {}
}
```

`requiredApproval=true` scopes must be accepted when editable and staged into the
current app version. Approver selection is handled by the version flow.

#### remove_open_dev_app_permission

| Item | Contract |
| --- | --- |
| CLI | `dws devapp permission remove` |
| HSF facade | `OpenInnerAppPermissionFacade.remove` |
| Request shape | One object with system context, app locator, and one `scopeValue`. |
| Required public fields | One app locator and one `scopeValue`. |
| Write guard | CLI `--dry-run`, then `--yes`; no MCP confirm field. |

Input fields:

| Field | Type | CLI flag | Required | Notes |
| --- | --- | --- | --- | --- |
| `unifiedAppId` | string | `--unified-app-id` | one locator | Preferred. |
| `agentId` | number | `--agent-id` | one locator | Compatible. |
| `scopeValue` | string | `--permission` | yes | Remove exactly one permission. |

Expected structured failures:

| Condition | Normalized reason |
| --- | --- |
| Scope is not currently authorized | `PERMISSION_NOT_AUTHED` |
| Scope cannot be edited | `PERMISSION_NOT_EDITABLE` |
| Scope cannot be found | `PERMISSION_NOT_FOUND` |

## 4. MCP Product Overlay

The MCP registry overlay should use a product id that matches the canonical command root:

```json
{
  "com.dingtalk.mcp.registry/cli": {
    "id": "devapp",
    "command": "devapp",
    "description": "开放平台应用管理",
    "prefixes": ["devapp", "app"],
    "groups": {
      "credentials": {"description": "应用凭证"},
      "permission": {"description": "应用权限"},
      "webapp": {"description": "网页应用"},
      "event": {"description": "事件订阅"},
      "version": {"description": "版本发布"}
    }
  }
}
```

Tool override examples:

```json
{
  "toolOverrides": {
    "list_open_dev_apps_by_condition": {
      "cliName": "list",
      "description": "查询开放平台企业内部应用列表",
      "flags": {
        "currentPage": {"alias": "page", "default": 1},
        "pageSize": {"alias": "page-size", "default": 20},
        "appName": {"alias": "name"},
        "agentId": {"alias": "agent-id"},
        "appId": {"alias": "app-id"},
        "appKey": {"alias": "app-key"},
        "customKey": {"alias": "custom-key"},
        "creator": {"alias": "creator"},
        "robotName": {"alias": "robot-name"},
        "sortType": {"alias": "sort"},
        "sortOrder": {"alias": "order"}
      }
    },
    "get_open_dev_app_detail": {
      "cliName": "get",
      "description": "查询开放平台企业内部应用详情",
      "flags": {
        "unifiedAppId": {"alias": "unified-app-id"},
        "agentId": {"alias": "agent-id"},
        "appId": {"alias": "app-id"},
        "appKey": {"alias": "app-key"},
        "customKey": {"alias": "custom-key"},
        "appName": {"alias": "name"}
      }
    },
    "create_open_dev_app": {
      "cliName": "create",
      "description": "创建开放平台企业内部应用",
      "isSensitive": true,
      "flags": {
        "appName": {"alias": "name", "required": true},
        "appDesc": {"alias": "desc"},
        "type": {"alias": "type", "default": "internal"},
        "appIcon": {"alias": "icon"},
        "hidden": {"alias": "hidden", "type": "bool"}
      }
    },
    "update_open_dev_app": {
      "cliName": "update",
      "description": "修改开放平台企业内部应用基础信息",
      "isSensitive": true,
      "flags": {
        "unifiedAppId": {"alias": "unified-app-id"},
        "agentId": {"alias": "agent-id"},
        "appId": {"alias": "app-id"},
        "appKey": {"alias": "app-key"},
        "customKey": {"alias": "custom-key"},
        "currentAppName": {"alias": "current-name"},
        "appName": {"alias": "name"},
        "appDesc": {"alias": "desc"},
        "appIcon": {"alias": "icon"},
        "international": {"alias": "international"},
        "supportHarmony": {"alias": "support-harmony", "type": "bool"}
      }
    },
    "delete_open_dev_app": {
      "cliName": "delete",
      "description": "删除开放平台企业内部应用",
      "isSensitive": true,
      "flags": {
        "unifiedAppId": {"alias": "unified-app-id"},
        "agentId": {"alias": "agent-id"},
        "appId": {"alias": "app-id"},
        "appName": {"alias": "name"},
        "appKey": {"alias": "app-key"},
        "customKey": {"alias": "custom-key"}
      }
    },
    "list_open_dev_app_permissions": {
      "cliName": "list",
      "group": "permission",
      "description": "查询开放平台应用权限列表，默认同时返回 APP 应用权限和 SNS 个人权限",
      "flags": {
        "unifiedAppId": {"alias": "unified-app-id"},
        "agentId": {"alias": "agent-id"},
        "keyword": {"alias": "keyword"},
        "apiStatus": {"alias": "api-status"},
        "authStatus": {"alias": "status", "default": "ALL"},
        "firstLevelType": {"alias": "scope-type"},
        "scopeValue": {"alias": "scope"},
        "limit": {"alias": "limit", "default": "20"}
      }
    },
    "add_open_dev_app_permissions": {
      "cliName": "add",
      "group": "permission",
      "description": "申请开放平台应用权限点；需审核权限写入当前应用版本变更",
      "isSensitive": true,
      "flags": {
        "unifiedAppId": {"alias": "unified-app-id"},
        "agentId": {"alias": "agent-id"},
        "scopeValues": {
          "alias": "permissions",
          "type": "stringSlice",
          "required": true,
          "description": "待申请权限点 scopeValue，多个用逗号分隔"
        }
      }
    },
    "remove_open_dev_app_permission": {
      "cliName": "remove",
      "group": "permission",
      "description": "取消一个已开通的开放平台应用权限点",
      "isSensitive": true,
      "flags": {
        "unifiedAppId": {"alias": "unified-app-id"},
        "agentId": {"alias": "agent-id"},
        "scopeValue": {
          "alias": "permission",
          "required": true,
          "description": "待取消权限点 scopeValue"
        }
      }
    }
  }
}
```

`permission search/detail` are UX aliases over `list_open_dev_app_permissions`. If the runtime overlay only exposes dynamic commands, keep one canonical command:

```bash
dws devapp permission list --keyword "机器人发送消息"
dws devapp permission list --scope qyapi_robot_sendmsg
```

If a helper command is added, map aliases without creating new MCP tools:

| Helper command | Underlying tool | Forced/derived args |
| --- | --- | --- |
| `devapp permission search` | `list_open_dev_app_permissions` | Requires `keyword` or `scopeValue`; returns candidate summaries. |
| `devapp permission detail` | `list_open_dev_app_permissions` | Requires `scopeValue`; returns one scope with full `apiList`. |

## 5. App Identity Resolution

Application identity must be resolved before detail and write commands.

| Priority | Identifier | Rule |
| --- | --- | --- |
| 1 | `unifiedAppId` | Preferred unique id for write operations. |
| 2 | `agentId/appId` | Compatible exact identifier. |
| 3 | `appKey/clientId` | Call list/detail and require one result. |
| 4 | `customKey` | Call list/detail and require one result. |
| 5 | `appName/name` | Fuzzy search; write operations require one result. |

Agent rule:

1. Never choose the first row when multiple apps match.
2. Show candidates with `appName`, `unifiedAppId`, `agentId`, `appKey/clientId`, `creator`, and modified time.
3. Ask the user to pick `unifiedAppId` or `agentId` for write operations.

MCP calls must rely on system context for caller identity:

| System context | HSF request field | Notes |
| --- | --- | --- |
| Caller org | `corpId` | DingTalk corpId, not internal orgId. |
| Caller user | `userId` | DingTalk userId/staffId; backend resolves uid. |

Open-source DWS and Agent must not pass `orgId` or `uid` as user input.

## 6. App CRUD

### 6.1 List

```bash
dws devapp list --page 1 --page-size 20 --format json
dws devapp list --name DemoApp --format json
dws devapp list --agent-id AGENT_ID --format json
dws devapp list --app-key dingxxx --format json
dws devapp list --creator 张三 --format json
dws devapp list --robot-name 小钉 --format json
```

MCP payload:

```json
{
  "currentPage": 1,
  "pageSize": 20,
  "appName": "DemoApp",
  "sortType": "gmt_modified",
  "sortOrder": "desc"
}
```

Pagination is supported for app list through `currentPage/pageSize`, both 1-based. Search filters are optional. A baseline call should pass only pagination and sort fields.

### 6.2 Get

```bash
dws devapp get --unified-app-id UNIFIED_APP_ID --format json
dws devapp get --agent-id AGENT_ID --format json
dws devapp get --name DemoApp --format json
```

MCP payload:

```json
{
  "unifiedAppId": "UNIFIED_APP_ID"
}
```

`get` may show `clientId/appKey/agentId`, but it must not be used to read full `clientSecret/appSecret`. Secret read belongs to `credentials get`.

### 6.3 Create

```bash
dws devapp create --name DemoApp --desc "internal app" --type internal --dry-run --format json
dws devapp create --name DemoApp --desc "internal app" --type internal --yes --format json
```

Current P0 create supports enterprise internal application basics:

| Type | Supported now | Notes |
| --- | --- | --- |
| `internal` | Yes | Default enterprise internal app. |
| `h5` | Yes, as internal web capability flavor if backend accepts it | Use only if backend schema exposes it. |
| `robot` | Not as app-create P0 | Bot config is a separate capability. |
| `miniapp` | Not P0 | Needs separate capability/backend confirmation. |
| `isv/connector` | Not P0 | Not yulan app CRUD. |

MCP payload:

```json
{
  "appName": "DemoApp",
  "appDesc": "internal app",
  "type": "internal"
}
```

### 6.4 Update

```bash
dws devapp update --unified-app-id UNIFIED_APP_ID --name DemoApp2 --desc "new desc" --dry-run --format json
dws devapp update --unified-app-id UNIFIED_APP_ID --name DemoApp2 --desc "new desc" --yes --format json
```

At least one update field is required. Supported P0 fields:

| CLI flag | MCP field | Notes |
| --- | --- | --- |
| `--name` | `appName` | New app name. |
| `--desc` | `appDesc` | New description. |
| `--icon` | `appIcon` | Existing media id or backend-supported resource. |
| `--international` | `international` | JSON string if exposed. |
| `--support-harmony` | `supportHarmony` | Boolean if exposed. |

### 6.5 Delete

```bash
dws devapp delete --unified-app-id UNIFIED_APP_ID --dry-run --format json
dws devapp delete --unified-app-id UNIFIED_APP_ID --yes --format json
```

Delete is destructive. The CLI/Agent must show the target app summary before calling the tool with `--yes`.

MCP payload:

```json
{
  "unifiedAppId": "UNIFIED_APP_ID"
}
```

Valid locator fields are `unifiedAppId`, `agentId`, `appId`, `appName`, `appKey`, and `customKey`. Name/key locators must uniquely resolve.

## 7. Credentials

Target command:

```bash
dws devapp credentials get --unified-app-id UNIFIED_APP_ID --format json
dws devapp credentials get --unified-app-id UNIFIED_APP_ID --show-secret --yes --format json
```

Current status:

1. Backend facade is pending.
2. `devapp get` can expose ids such as `clientId/appKey`, but must not expose full secrets.
3. Full secret read needs a dedicated audited tool.
4. CLI default output should be masked; full reveal requires explicit confirmation.

Target fields:

| Field | Notes |
| --- | --- |
| `unifiedAppId/agentId/appName` | App summary. |
| `clientId/appKey` | Non-secret identifiers. |
| `maskedClientSecret/maskedAppSecret` | Default display. |
| `clientSecret/appSecret` | Only when explicitly requested and allowed. |
| `readAuditId` | Backend audit reference. |

## 8. Permissions

### 8.1 Web Semantics

Permission list must align with the developer console endpoint:

```text
GET /openapp/unifiedapp/{unifiedAppId}/scope/list?from=inner&key={keyword}&apiStatus={apiStatus}
```

Returned groups:

```json
{
  "success": true,
  "data": {
    "snsScopes": [
      {"title": "个人权限", "firstLevelType": "SNS", "scopeList": []}
    ],
    "appScopes": [
      {"title": "通讯录管理", "firstLevelType": "APP", "scopeList": []}
    ]
  }
}
```

Do not split the main list into old `listAuthed/listAvailable` APIs. Use one list operation that returns both authorized and unauthorized scopes.

### 8.2 List, Search, Detail

```bash
dws devapp permission list --unified-app-id UNIFIED_APP_ID --format json
dws devapp permission list --unified-app-id UNIFIED_APP_ID --status unauthed --keyword "发送消息" --format json
dws devapp permission list --unified-app-id UNIFIED_APP_ID --scope-type SNS --format json
dws devapp permission list --unified-app-id UNIFIED_APP_ID --scope qyapi_robot_sendmsg --format json
```

MCP payload for normal list:

```json
{
  "unifiedAppId": "UNIFIED_APP_ID",
  "authStatus": "ALL",
  "limit": 20
}
```

MCP payload for search:

```json
{
  "unifiedAppId": "UNIFIED_APP_ID",
  "keyword": "机器人发送消息",
  "authStatus": "UNAUTHED",
  "limit": 10
}
```

MCP payload for detail:

```json
{
  "unifiedAppId": "UNIFIED_APP_ID",
  "scopeValue": "qyapi_robot_sendmsg",
  "limit": 1
}
```

Input fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `unifiedAppId` | string | one of app id fields | Preferred app locator. |
| `agentId` | number | one of app id fields | Compatible app locator. |
| `keyword` | string | no | Maps to Web `key`. |
| `apiStatus` | string | no | Maps to Web `apiStatus`. |
| `authStatus` | string | no | `ALL/AUTHED/UNAUTHED`, post-filter. |
| `firstLevelType` | string | no | `APP/SNS`; empty returns both. |
| `scopeValue` | string | no | Detail mode and exact post-filter. |
| `limit` | number | no | Default 20, hard cap 50. |

Permission list has no server pagination. Web also loads the full grouped list. Use `keyword` to reduce server-side results, then use `limit` to control Agent context.

Default response must flatten APP and SNS scopes into one stable `permissions[]` array:

```json
{
  "mode": "list",
  "permissions": [
    {
      "scopeValue": "qyapi_robot_sendmsg",
      "scopeName": "企业内机器人发消息权限",
      "scopeDesc": "企业内机器人发消息权限",
      "firstLevelType": "APP",
      "categoryTitle": "机器人",
      "authed": true,
      "canEdit": true,
      "authedStatusDesc": "已开通",
      "requiredApproval": false,
      "allowedActions": ["view", "detail", "remove"],
      "apiCount": 4,
      "apiPreview": {
        "name": "人与人会话中机器人发送普通消息"
      },
      "canApplyDirectly": false,
      "canRemove": true
    }
  ],
  "summary": {
    "totalMatched": 1,
    "returnedCount": 1,
    "authedCount": 1,
    "unauthedCount": 0,
    "requiredApprovalCount": 0
  },
  "truncated": false,
  "nextAction": "none"
}
```

`requiredApproval=true` must be shown and must still be appliable:

```json
{
  "scopeValue": "Contact.User.mobile",
  "scopeName": "个人手机号信息",
  "firstLevelType": "SNS",
  "authed": false,
  "canEdit": true,
  "requiredApproval": true,
  "allowedActions": ["view", "detail", "apply"],
  "canApplyDirectly": true,
  "displayMessage": "该权限可申请；申请后加入当前应用版本变更，发布时进入审核流程。"
}
```

Agent selection order:

1. Exact `scopeValue`.
2. Matched `apiList[].name`.
3. Scope name or description.
4. Category title and `firstLevelType`.

Agent must pass final `scopeValue` to add/remove. Do not pass API name, API uuid, or category title into write tools.

### 8.3 Add

```bash
dws devapp permission add --unified-app-id UNIFIED_APP_ID --permissions qyapi_robot_sendmsg --dry-run --format json
dws devapp permission add --unified-app-id UNIFIED_APP_ID --permissions qyapi_robot_sendmsg --yes --format json
```

MCP payload:

```json
{
  "unifiedAppId": "UNIFIED_APP_ID",
  "scopeValues": ["qyapi_robot_sendmsg"]
}
```

Rules:

1. Already authorized scopes are skipped or reported, not duplicated.
2. Missing scopes are rejected.
3. `canEdit=false` scopes are rejected.
4. `requiredApproval=true` scopes are accepted and staged into current app version changes.
5. Approver selection is not part of permission add; it belongs to version publish.

Expected response:

```json
{
  "applied": true,
  "appliedScopeValues": ["Contact.User.mobile"],
  "directAppliedScopeValues": [],
  "versionStagedScopeValues": ["Contact.User.mobile"],
  "rejectedScopeValues": [],
  "skippedScopeValues": [],
  "message": "权限申请成功；权限变更已加入应用版本，请在版本发布时继续审核流程。"
}
```

### 8.4 Remove

```bash
dws devapp permission remove --unified-app-id UNIFIED_APP_ID --permission qyapi_robot_sendmsg --dry-run --format json
dws devapp permission remove --unified-app-id UNIFIED_APP_ID --permission qyapi_robot_sendmsg --yes --format json
```

MCP payload:

```json
{
  "unifiedAppId": "UNIFIED_APP_ID",
  "scopeValue": "qyapi_robot_sendmsg"
}
```

Rules:

1. Remove only one scope per call.
2. `authed=false` returns `NOT_AUTHED`.
3. `canEdit=false` returns a no-edit reason.
4. Removing an authorized permission is not the same as withdrawing a pending approval.

## 9. Event And Version Boundary

Event and version commands are needed for a full application lifecycle but should not be folded into permission add.

Target event tools:

| CLI | MCP tool | Notes |
| --- | --- | --- |
| `devapp event list` | `list_open_dev_app_events` | Query current callback and subscribed event types. |
| `devapp event config` | `configure_open_dev_app_event` | Configure callbackUrl, token, aesKey, eventTypes. |

Target version tools:

| CLI | MCP tool | Notes |
| --- | --- | --- |
| `devapp version create` | `create_open_dev_app_version` | Save current version draft. |
| `devapp version check-approval` | `check_open_dev_app_version_approval` | Return whether approval is required and candidate approvers. |
| `devapp version publish` | `publish_open_dev_app_version` | Submit publish and create approval when needed. |
| `devapp version status` | `get_open_dev_app_version_status` | Poll publish and approval state. |

Stitching rule:

```text
permission add requiredApproval=true
  -> versionStagedScopeValues returned
  -> devapp version check-approval
  -> choose approver if required
  -> devapp version publish
  -> devapp version status
```

Approval details may only be visible in the DingTalk app. CLI should return audit id, approval url if backend provides one, and a clear status.

## 10. Agent Routing

### 10.1 Application Intent Disambiguation

`应用` is an overloaded word in DingTalk. A yulan-style Agent must not route every
generic app request to `devapp`. Use `devapp` only when the request is about
DingTalk Open Platform / developer backend enterprise internal applications.

Positive `devapp` signals:

| Signal | Route |
| --- | --- |
| `开放平台应用`, `开发者后台应用`, `企业内部应用`, `内部应用` | `devapp` |
| `agentId`, `clientId`, `appKey`, `customKey`, `appSecret`, `clientSecret` | `devapp` |
| `应用权限`, `权限点`, `API 权限`, `APP/SNS 权限`, `scopeValue` | `devapp permission ...` |
| `事件订阅`, `callbackUrl`, `token`, `aesKey` in an app context | `devapp event ...` |
| `应用版本`, `版本发布`, `选审批人`, `发布审核` in developer backend context | `devapp version ...` |
| yulan's current `开放平台应用 CLI 化 / dws app` workstream | `devapp` |

Non-`devapp` signals:

| Request shape | Correct route |
| --- | --- |
| `开放平台接口文档`, `API 怎么调用`, `错误码`, `字段说明` | `devdoc` |
| `钉钉文档`, `云文档`, `知识库里的文档`, document read/write/export | `doc` or `wiki` preflight |
| `工作台应用`, `应用 app001`, `钉钉工作台上有哪些应用` | `workbench app ...` |
| `OA 审批单`, `同意/拒绝/撤销审批`, approval task detail | `oa` |
| `MCP 服务`, `connector`, `创建工具`, `HSF 映射`, `上架 MCP` | OpenDev MCP platform workflow, not `dws devapp` |
| `AI 应用`, `智能体`, `机器人对话能力` without developer backend identifiers | Ask or route to the relevant product; do not default to `devapp` |

If the user says only `应用` and gives no developer-backend identifiers, inspect
the surrounding context. If there is still no context, ask one short clarifying
question: whether they mean an Open Platform enterprise internal app, a workbench
app, a document/app capability, or an MCP connector/tool.

When the user is already in this yulan workstream, prefer `devapp` for ambiguous
phrases such as `应用查询`, `创建应用`, `应用权限`, or `应用版本` because the active
domain is OpenDev enterprise internal applications. This preference is not a
global DWS rule; it is scoped to this developer-platform application CLI design.

Agent should use this state machine:

```text
user intent
  -> normalize command
  -> collect slots
  -> resolve unique app
  -> for permission intents, resolve scopeValue
  -> dry-run plan for writes
  -> require user confirmation or --yes
  -> call MCP tool
  -> normalize ServiceResult
  -> report next action
```

Intent table:

| User intent | Command | Tool |
| --- | --- | --- |
| "查应用/搜索应用" | `devapp list` | `list_open_dev_apps_by_condition` |
| "看应用详情/clientId/agentId" | `devapp get` | `get_open_dev_app_detail` |
| "创建应用" | `devapp create` | `create_open_dev_app` |
| "修改名称/描述/图标" | `devapp update` | `update_open_dev_app` |
| "删除应用" | `devapp delete` | `delete_open_dev_app` |
| "查权限/搜索 API 权限" | `permission list` | `list_open_dev_app_permissions` |
| "这个权限覆盖哪些 API" | `permission list --scope` | `list_open_dev_app_permissions` |
| "申请权限/开通权限" | `permission add` | `add_open_dev_app_permissions` |
| "取消权限/移除权限" | `permission remove` | `remove_open_dev_app_permission` |
| "拿 appSecret/clientSecret" | `credentials get` | Pending |
| "配置事件订阅" | `event config` | Pending |
| "发布/审核/选审批人" | `version ...` | Pending |

ServiceResult handling:

| Result | Agent behavior |
| --- | --- |
| `success=true` with `result` | Show structured result and next action. |
| `success=false` | Preserve `errorCode/errorMsg`; do not rewrite as a generic failure. |
| `versionStagedScopeValues` non-empty | Tell user to continue version approval/publish. |
| multiple app candidates | Stop and ask for a unique id. |
| permission list `truncated=true` | Ask for a narrower keyword or status filter. |

## 11. Agent Invocation Runbook

This section is the operational rulebook for yulan-style Agents. It is meant to
avoid the common failure mode where an Agent knows the right business concept but
calls the wrong DWS product, stale MCP tool, or over-broad permission list.

### 11.1 Discovery Preflight

Before running a devapp command in a new environment, the Agent should verify the
runtime surface instead of assuming the branch document has already become a
published MCP command tree.

```bash
dws schema --jq '.products[] | select(.id=="devapp") | {id, command, tool_count: (.tools | length)}'
dws devapp --help
dws schema devapp.list_open_dev_apps_by_condition --jq '.tool.flag_overlay'
```

If the product or tool is missing:

1. Run `dws cache refresh` once and retry schema inspection.
2. If it is still missing, report `DEVAPP_NOT_DISCOVERED`.
3. Do not reroute the task to `devdoc`, `doc`, or generic `mcp` commands unless
   the user is asking for documentation rather than application management.

If a command flag in this document conflicts with `--help` or `dws schema`, use
the runtime schema. The document defines the target contract; discovery defines
what is currently callable.

### 11.2 Command Construction

Use this order when constructing a call:

1. Identify the product as `devapp` only when the request matches section 10.1
   positive signals or the active yulan OpenDev app workstream. Do not use
   `devapp` for every bare `应用`.
2. Resolve application identity before any detail or write operation.
3. Prefer CLI flags exposed by `dws schema` over raw `--json`.
4. Use raw `--json` only when a required MCP field has no CLI flag yet.
5. Always add `--format json` for Agent execution.
6. For writes, call `--dry-run --format json` first, then call the same command
   with `--yes --format json` only after user confirmation or an explicit
   higher-level instruction.

Do not add confirmation booleans to MCP payloads. These belong to the CLI layer:

| Bad MCP field | Correct DWS guard |
| --- | --- |
| `confirmCreate=true` | `dws devapp create ... --yes` |
| `confirmUpdate=true` | `dws devapp update ... --yes` |
| `confirmDelete=true` | `dws devapp delete ... --yes` |
| `confirmPermission=true` | `dws devapp permission add/remove ... --yes` |
| `confirmSecret=true` | `dws devapp credentials get --show-secret --yes` |

### 11.3 Business Recipes

| User request | Agent recipe |
| --- | --- |
| "查 DemoApp 应用" | `devapp list --name DemoApp`; if one result, optionally `devapp get`; if multiple, show candidates. |
| "这个 agentId 是哪个应用" | `devapp get --agent-id AGENT_ID`; if backend requires list semantics, use `devapp list --agent-id AGENT_ID`. |
| "创建企业内部应用" | `devapp create --name ... --type internal --dry-run`; after confirmation, repeat with `--yes`. |
| "修改应用描述/图标" | Resolve `unifiedAppId`; `devapp update --unified-app-id ... --desc/icon ... --dry-run`; after confirmation, `--yes`. |
| "删除应用" | Resolve unique app; show app summary; `devapp delete ... --dry-run`; after confirmation, `--yes`. |
| "查询机器人发消息权限" | `devapp permission list --keyword "机器人发消息"`; if candidates are broad, require `scopeValue`. |
| "这个 API 需要哪个权限" | Search by API name; inspect `apiPreview`; if uncertain, use `permission list --scope SCOPE` for detail. |
| "申请通讯录手机号权限" | Resolve app; `permission list --keyword "手机号"`; choose `scopeValue`; `permission add --permissions SCOPE --dry-run`; then `--yes`. |
| "权限已进版本，继续发布" | `version check-approval`; choose approver if needed; `version publish --yes`; poll `version status`. |
| "拿 clientSecret/appSecret" | Use only `credentials get`; if unsupported, report `CREDENTIALS_UNSUPPORTED`, never fall back to `devapp get`. |

### 11.4 Permission Candidate Selection

Permission list is intentionally one grouped list, not separate
`listAuthed/listAvailable` calls. The Agent must reduce the result set before
writing:

1. If the user gives a `scopeValue`, use exact match.
2. If the user gives an API name, search with `keyword`, then match
   `apiList[].name` or `apiPreview.name`.
3. If the user gives a human permission name, match `scopeName/scopeDesc`.
4. If multiple scopes remain, show `scopeValue`, `scopeName`, `firstLevelType`,
   `authed`, `requiredApproval`, and `apiPreview`, then ask for one.
5. For list output, cap the displayed list with `limit`; for detail output, call
   with `scopeValue` and return the full API coverage for one scope only.

`requiredApproval=true` is not a blocker for permission add. It means the add
operation stages the permission into the current application version and the
approval decision happens during version publish.

### 11.5 Normalized Errors

DWS may return CLI-layer errors (`category/reason/hint/actions`) or MCP/HSF
business errors (`ServiceResult.success=false`, `errorCode`, `errorMsg`). The
Agent should normalize behavior without hiding raw backend fields.

| Normalized reason | Typical source | Agent behavior |
| --- | --- | --- |
| `DEVAPP_NOT_DISCOVERED` | `unknown command`, schema product missing, discovery exit code | Run `dws cache refresh` once; if still missing, say MCP product/tool is not published or not visible to this account. |
| `DEVAPP_TOOL_NOT_PUBLISHED` | Product exists but one tool is missing | Report the missing MCP tool key and avoid substituting a different operation. |
| `AUTH_CONTEXT_MISSING` | Backend cannot resolve `corpId/userId` | Tell the user to re-login or check organization context; do not ask for internal `orgId/uid`. |
| `APP_NOT_FOUND` | Empty list/detail result | Ask for another app identifier; do not create or update implicitly. |
| `APP_AMBIGUOUS` | Multiple apps match name/key | Show candidate ids and stop before write. |
| `PERMISSION_NOT_FOUND` | Scope search has no exact candidate | Ask for another keyword or API name. |
| `PERMISSION_ALREADY_AUTHED` | Add receives an already authorized scope | Report skipped scope and continue only for remaining unauth scopes. |
| `PERMISSION_NOT_AUTHED` | Remove receives an unauthorized scope | Report that there is nothing to remove. |
| `PERMISSION_NOT_EDITABLE` | `canEdit=false` or backend no-edit error | Show the no-edit reason; do not retry as a write. |
| `VERSION_APPROVAL_REQUIRED` | `check-approval` says approver is required | Ask for approver or use returned candidate when the user gave an explicit approver. |
| `CREDENTIALS_UNSUPPORTED` | Credential facade/tool missing | State that the credential tool is not published; do not leak or infer secrets from detail fields. |

Raw `errorCode/errorMsg` must remain in JSON output or the Agent response so the
backend owner can diagnose actual HSF failures.

### 11.6 Output Shape For Agent Consumption

For every command, prefer a stable shape that has a small summary and raw backend
details:

```json
{
  "ok": true,
  "operation": "permission.add",
  "app": {
    "unifiedAppId": "UNIFIED_APP_ID",
    "agentId": 123456,
    "appName": "DemoApp"
  },
  "summary": {
    "appliedCount": 1,
    "skippedCount": 0,
    "rejectedCount": 0,
    "requiresVersionPublish": true
  },
  "result": {},
  "raw": {
    "success": true,
    "errorCode": "",
    "errorMsg": ""
  },
  "nextAction": "run devapp version check-approval"
}
```

For failures:

```json
{
  "ok": false,
  "operation": "app.get",
  "reason": "APP_AMBIGUOUS",
  "message": "Multiple applications match the provided name.",
  "candidates": [],
  "raw": {
    "success": false,
    "errorCode": "BACKEND_ERROR_CODE",
    "errorMsg": "backend message"
  }
}
```

The `raw` object is allowed to include backend fields, but it must not include
tokens, cookies, MCP gateway keys, full app secrets, or unrelated page request
headers.

## 12. Open-Source DWS Implementation Notes

Two implementation paths are valid.

Dynamic discovery path:

1. Publish MCP tools with the `devapp` CLI metadata.
2. Ensure the Streamable HTTP URL is discoverable by DWS without dropping query parameters.
3. Use `dws schema devapp.<tool>` and `dws devapp ... --help` as the source of truth.
4. Use `isSensitive=true` for create, update, delete, permission add, permission remove, credentials full secret read, event config, and version publish.

Hardcoded helper path:

1. Add `internal/helpers/devapp.go`, following `internal/helpers/devdoc.go`.
2. Register the root with `RegisterPublic`.
3. Route helper invocations with `executor.NewHelperInvocation`.
4. Preserve dynamic discovery as the authority when the same leaf exists; helper-only leaves can provide aliases such as `permission search/detail`.
5. Add tests for dry-run payloads, `--yes` guard, app resolution, and permission routing.

Recommended helper root:

```text
Name(): "devapp"
Aliases: ["app"] if supported by cobra and no conflict
```

Do not manually edit generated command indexes unless the generator is part of the change. The runtime command surface should be checked with:

```bash
dws devapp --help
dws schema --jq '.products[] | select(.id=="devapp")'
dws schema devapp.list_open_dev_apps_by_condition
```

## 13. Rollout And Verification

Use this checklist when moving from backend HSF implementation to usable DWS
commands. The goal is to prove that the same operation works at the MCP layer,
the DWS dynamic schema layer, and the Agent recipe layer.

### 13.1 MCP Tool Configuration Checklist

For each P0 tool:

1. Configure on the pre-release MCP developer platform before online release.
2. Use the deterministic tool key from section 3.2.
3. Select HSF and fill interface, version, and method exactly.
4. Generate API input/output from the HSF method.
5. Keep a single business request object. Remove or avoid separate
   `BaseOpenRequestVO` public mapping.
6. Configure tool input JSON Schema with only Agent-facing fields.
7. Map `corpId/userId` from system context to the business request object.
8. Review every recommended mapping manually, especially fields named
   `agentId`, `appId`, `appKey`, or `userId`.
9. Keep `ServiceResult` output wrapper and success rule
   `ServiceResult.success == true`.
10. Remove blank error-text optimization rows; preserve raw
    `errorCode/errorMsg`.
11. Save draft and verify the page shows a new draft version.
12. Debug with a baseline call, then one filter/write scenario.
13. Publish only after debug passes.

P0 debug inputs:

| Tool | Baseline debug input |
| --- | --- |
| `list_open_dev_apps_by_condition` | `{"currentPage":1,"pageSize":10}` |
| `get_open_dev_app_detail` | `{"unifiedAppId":"UNIFIED_APP_ID"}` |
| `create_open_dev_app` | `{"appName":"DemoApp","appDesc":"internal app","type":"internal"}` |
| `update_open_dev_app` | `{"unifiedAppId":"UNIFIED_APP_ID","appDesc":"new desc"}` |
| `delete_open_dev_app` | `{"unifiedAppId":"UNIFIED_APP_ID"}` |
| `list_open_dev_app_permissions` | `{"unifiedAppId":"UNIFIED_APP_ID","authStatus":"ALL","limit":20}` |
| `add_open_dev_app_permissions` | `{"unifiedAppId":"UNIFIED_APP_ID","scopeValues":["SCOPE_VALUE"]}` |
| `remove_open_dev_app_permission` | `{"unifiedAppId":"UNIFIED_APP_ID","scopeValue":"SCOPE_VALUE"}` |

For destructive debug cases, use a disposable application and permission scope.
Do not use production data in long-lived examples.

### 13.2 MCP Direct Smoke

After publish, verify the service through MCP before using DWS:

1. Run MCP `initialize`.
2. Run `tools/list` and confirm all expected P0 tool keys exist.
3. Call `list_open_dev_apps_by_condition` with only pagination.
4. Call one exact locator scenario, such as `get_open_dev_app_detail`.
5. Call permission list with `authStatus=ALL` and confirm APP and SNS scopes are
   both represented.
6. Confirm failures preserve `errorCode/errorMsg`.

Do not store the Streamable HTTP URL or `?key=` value in repository files,
README, skills, or issue text.

### 13.3 DWS Discovery Smoke

After the MCP service is published or the registry overlay is updated, refresh
DWS and verify discovery:

```bash
dws cache refresh --product devapp
dws schema --jq '.products[] | select(.id=="devapp") | {id, command, tool_count: (.tools | length)}'
dws schema devapp.list_open_dev_apps_by_condition --jq '.tool.flag_overlay'
dws devapp --help
```

Expected:

1. Product id is `devapp`.
2. Optional command alias `app` may exist, but `devapp` is canonical.
3. P0 tools appear in schema or equivalent command leaves appear in help.
4. `--format json` is available through global output flags.
5. Sensitive writes are marked or guarded so Agent uses `--dry-run` then `--yes`.

If `devapp` is missing after refresh, treat it as
`DEVAPP_NOT_DISCOVERED`. If `devapp` exists but a P0 tool is missing, treat it as
`DEVAPP_TOOL_NOT_PUBLISHED`.

### 13.4 Agent Smoke

Use these smoke cases to prove the yulan invocation problem is solved for an
Agent. They intentionally cover routing, narrowing, write guard, and backend
error propagation.

| Scenario | Smoke command | Pass criteria |
| --- | --- | --- |
| Discover product | `dws schema --jq '.products[] | select(.id=="devapp")'` | Product exists and exposes devapp CLI metadata. |
| List baseline | `dws devapp list --page 1 --page-size 10 --format json` | JSON contains page summary and app array. |
| Search by name | `dws devapp list --name DemoApp --format json` | One result opens path to get; multiple results return candidates. |
| Detail by id | `dws devapp get --unified-app-id UNIFIED_APP_ID --format json` | Response includes ids but no full secret. |
| Create guard | `dws devapp create --name DemoApp --type internal --format json` | Refuses without `--yes` or shows dry-run requirement. |
| Create dry-run | `dws devapp create --name DemoApp --type internal --dry-run --format json` | Prints payload and does not call HSF. |
| Permission search | `dws devapp permission list --unified-app-id UNIFIED_APP_ID --keyword "发送消息" --format json` | Returns candidate scopes and no write happens. |
| Permission detail | `dws devapp permission list --unified-app-id UNIFIED_APP_ID --scope SCOPE_VALUE --format json` | Returns full API coverage for one scope. |
| Permission approval | `dws devapp permission add --unified-app-id UNIFIED_APP_ID --permissions SCOPE_VALUE --dry-run --format json` | Shows direct/staged/rejected plan; `requiredApproval=true` is allowed. |
| Backend failure | Invalid app id or scope | JSON preserves `errorCode/errorMsg` and normalized reason. |

### 13.5 Release Exit Criteria

The application-side command set is ready for yulan Agent usage only when all of
the following are true:

1. P0 MCP tools are published and visible through `tools/list`.
2. DWS `schema` shows `devapp` with P0 tools or equivalent helper leaves.
3. App list/detail/create/update/delete smoke cases pass.
4. Permission list/add/remove smoke cases pass for APP and SNS scopes.
5. `requiredApproval=true` permission add stages into version changes instead of
   being rejected by the Agent.
6. Missing credential/event/version tools are reported as unsupported/pending,
   not silently emulated by other commands.
7. No repository doc, skill, log, or fixture contains MCP gateway keys, cookies,
   access tokens, full app secrets, or production app data.
8. Existing skill setup and generator tests pass.

## 14. Acceptance Cases

| Case | Expected behavior |
| --- | --- |
| `dws devapp list --name DemoApp --format json` | Calls listByCondition and returns pagination summary. |
| `dws devapp get --name DemoApp --format json` with multiple matches | Stops and returns candidates. |
| `dws devapp create --name T --type internal --dry-run --format json` | Prints payload only; no MCP call. |
| `dws devapp create --name T --type internal --format json` | Refuses because write confirmation is missing. |
| `dws devapp create --name T --type internal --yes --format json` | Calls `create_open_dev_app`. |
| `dws devapp update --unified-app-id X --desc Y --yes --format json` | Calls `update_open_dev_app`. |
| `dws devapp delete --unified-app-id X --format json` | Refuses because delete confirmation is missing. |
| `dws devapp permission list --unified-app-id X --format json` | Returns flattened APP and SNS permissions. |
| `dws devapp permission list --unified-app-id X --keyword "发送消息" --format json` | Returns candidate `permissions[]`; no automatic apply. |
| `dws devapp permission add --unified-app-id X --permissions Contact.User.mobile --yes --format json` | Applies or stages permission; `requiredApproval=true` is allowed. |
| `dws devapp permission remove --unified-app-id X --permission qyapi_robot_sendmsg --yes --format json` | Removes one authorized scope or returns a structured reason. |
| `dws devapp credentials get --unified-app-id X --format json` | If backend pending, clearly reports unsupported; does not use `devapp get` as a secret fallback. |
| `查应用 appXYZ 的详情` without OpenDev context | Does not blindly route to `devapp`; uses context such as `workbench app get` or asks. |
| `查询开放平台 API 错误码` | Routes to `devdoc`, not `devapp`. |
| `创建 MCP 服务并配置 HSF tool` | Uses OpenDev MCP platform workflow, not `dws devapp create`. |
