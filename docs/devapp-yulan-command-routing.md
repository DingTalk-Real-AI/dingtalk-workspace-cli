# yulan Open Platform App Command Routing

> Status: draft, 2026-06-05
> Branch: `feat/dws-devapp`
> Scope: open-source DWS integration for DingTalk Open Platform enterprise app management.

This document splits the application-side command set for yulan. It is written for the open-source DWS repository and must not contain personal MCP gateway keys, pre-release tokens, cookies, or production app data.

## 0. Current Repository Deliverables

The current branch contains the application-side command contract, Agent
routing assets, and a hardcoded `devapp` helper command tree. `devapp` does not
depend on MCP service discovery or registry overlay publication. Runtime calls
resolve the MCP endpoint from the internal edition's `SupplementServers` or
`StaticServers` hook; local debug may override it with
`DINGTALK_DEVAPP_MCP_URL`. Do not commit the endpoint URL or its `?key=` value.

| Artifact | Status | Evidence |
| --- | --- | --- |
| Main design document | Done in repo | `docs/devapp-yulan-command-routing.md` covers naming, command tree, P0 RPC contracts, permissions, Agent routing, rollout, and acceptance cases. |
| README discoverability | Done in repo | `README.md` and `README_zh.md` link to this design under Reference/参考文档. |
| Mono skill routing | Done in repo | `skills/mono/SKILL.md` routes Open Platform app intents to `devapp`. |
| Mono product reference | Done in repo | `skills/mono/references/products/devapp.md` summarizes app CRUD, permissions, credentials, events, versions, and troubleshooting. |
| Multi product skill | Done in repo | `skills/multi/dingtalk-devapp/` defines the standalone `dingtalk-devapp` skill and reference file. |
| Agent intent test cases | Done in repo | `test/skill_tests.md` includes 21 devapp cases, devdoc/workbench counterexamples, and routing clarification cases for the overloaded word `应用`; `test/skill_tests_results.md` reports those cases passing. |
| Skill test parser | Done in repo | `test/run_skill_tests.py` now preserves the current `#### dws ...` command section for each case. |
| Static/e2e skill verifier handling | Done in repo | `test/skill_static` and `test/skill_e2e` cover routing docs; runtime smoke uses an externally configured MCP endpoint. |
| Sensitive config guard | Done in repo | `.gitignore` excludes local `docs/mcp/serviceconfig-pre*` files so personal MCP gateway keys are not staged. |
| MCP HSF tool configuration | External dependency | Configure and publish tools on the DingTalk MCP developer platform using sections 3, 4, and 13. |
| DWS runtime command availability | Done in repo | `dws devapp ...` and alias `dws app ...` are hardcoded helper commands. No service discovery is required for this command tree. |
| Full production readiness | Not yet proven | Requires section 13 release exit criteria, including MCP `tools/list`, DWS schema, APP/SNS permission smoke, and write guard smoke. |

State labels:

1. `Done in repo` means the open-source branch contains the artifact and it is
   covered by local checks.
2. `External dependency` means backend/MCP tool publication must happen outside
   this repository.
3. `Not yet proven` means do not claim the application command set is ready for
   Agent production usage until runtime smoke evidence exists.

## 1. Naming

Use `devapp` as the canonical open-source product id:

```text
dws devapp ...
```

`app` is kept as a compatibility alias by the hardcoded helper. Do not
introduce `opendev` or `apps` as new canonical roots; if they are needed for
migration, make them redirect or hidden aliases.

Reasoning:

1. `devapp` is specific enough for Open Platform developer applications.
2. `app` is the PRD/user-facing shorthand and can remain as an alias.
3. The open-source DWS repository is protocol-first; `devapp` uses helper commands so it can run without service discovery.

Branch context:

```text
branch: feat/dws-devapp
base: latest upstream/main has been merged into the local design branch
runtime: devapp command availability does not depend on MCP service discovery
```

The checked-in open-source helper code owns the `dws devapp` command tree. The
MCP endpoint is owned by the internal edition layer, typically through
`edition.SupplementServers`; `DINGTALK_DEVAPP_MCP_URL` remains a local debug
override.

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
  permission list              List APP and SNS permissions
  permission search            Search permission candidates, alias over list semantics
  permission detail            Show one permission's full API coverage
  permission add               Apply permissions into current app version changes
  permission remove            Remove authorized permissions
  event list                   List subscribable events and current subscription state
  event subscribe              Subscribe events by eventCodes
  event unsubscribe            Unsubscribe events by eventCodes
  robot create                 Create a new agent app + robot synchronously
  robot submit                 Submit async robot-create task (supports retry by taskId)
  robot result                 Query async robot-create task result
  robot get                    Get robot config of an existing app
  robot config                 Create or update robot config on an existing app
  robot enable                 Enable / re-enable robot capability
  robot disable                Disable robot capability
  version create               Save app version
  version list                 List app versions (cursor paged)
  version get                  Get one version detail
  version check-approval       Precheck publish approval requirements (no publish)
  version publish              Submit publish, possibly creating approval
  version status               Poll publish and approval status
```

Robot, version, and event commands are hardcoded helper leaves over the `op-app`
MCP server (same pinned endpoint as the rest of `devapp`, no service discovery).

Open-source examples should use `--format json` for agent consumption:

```bash
dws devapp list --name DemoApp --page-size 20 --format json
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
| App query | `devapp list` | `list_dev_app` | `OpenInnerAppQueryFacade.listByCondition` | Implemented | Hardcoded helper |
| App detail | `devapp get` | `get_dev_app` | `OpenInnerAppQueryFacade.getDetail` | Implemented | Hardcoded helper |
| App create | `devapp create` | `create_dev_app` | `OpenInnerAppManageFacade.create` | Implemented | Hardcoded helper |
| App update | `devapp update` | `update_dev_app` | `OpenInnerAppManageFacade.update` | Implemented | Hardcoded helper |
| App delete | `devapp delete` | `delete_dev_app` | `OpenInnerAppManageFacade.delete` | Implemented | Hardcoded helper |
| Credentials | `devapp credentials get` | `get_dev_app_credentials` | `OpenInnerAppQueryFacade.getCredentials` | Implemented | Hardcoded helper |
| Web app | `devapp webapp get/config` | `get_extension_webapp_config`/`set_extension_webapp_config` | Webapp facade | Implemented | Hardcoded helper; locator field is `unifiedAppId`; config fields are `h5PageType/homepageUrl/pcHomepageUrl/ompUrl`. |
| Permission list | `devapp permission list/search/detail` | `list_dev_app_permissions` | `OpenInnerAppPermissionFacade.list` | Implemented | Hardcoded helper |
| Permission apply | `devapp permission add` | `apply_dev_app_permissions` | `OpenInnerAppPermissionFacade.apply` | Implemented | Hardcoded helper |
| Permission remove | `devapp permission remove` | `remove_dev_app_permissions` | `OpenInnerAppPermissionFacade.remove` | Implemented | Hardcoded helper |
| Events | `devapp event list/subscribe/unsubscribe` | `list_dev_app_events`, `subscribe_dev_app_events`, `unsubscribe_dev_app_events` | Implemented | Hardcoded helper |
| Robot create | `devapp robot create/submit/result` | `create_dingtalk_robot`/`submit_robot_create_task`/`query_robot_create_result` | Implemented | Hardcoded helper |
| Robot config | `devapp robot get/config/enable/disable` | `get_extension_robot_config`, `set_extension_robot_config`, `enable_dev_app_robot`, `disable_dev_app_robot` | Implemented | Hardcoded helper |
| Version | `devapp version create/list/get/check-approval/publish/status` | `create_dev_app_version`, `list_dev_app_versions`, `get_dev_app_version_detail`, `publish_dev_app_version`, `get_dev_app_version_status` | Implemented | Hardcoded helper |

P0 for yulan is app CRUD, permissions, credentials, and web app config. Robot,
version, and event subscription are now all implemented as hardcoded helper
leaves over the same `op-app` MCP server.

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

Naming convention:

- Public CLI flags use user-facing names: `--name`, `--desc`, `--icon`, `--homepage-url`.
- MCP app create/update use the current backend request field names: `name`, `desc`, `icon`.
- MCP app list/detail use the simplified locator/search fields: `unifiedAppId`, `appKey`, `name`.
- The CLI helper keeps public flags stable, for example `--name` -> `name` for create/update/list/detail.

### 3.2 P0 Tool Contracts

#### list_dev_app

| Item | Contract |
| --- | --- |
| CLI | `dws devapp list` |
| HSF facade | `OpenInnerAppQueryFacade.listByCondition` |
| Request shape | One object with system context and optional list filters. |
| Required public fields | None; default `pageSize=20`. |
| Identity source | `corpId/userId` from MCP system context. |

Input fields:

| Field | Type | CLI flag | Required | Notes |
| --- | --- | --- | --- | --- |
| `pageSize` | number | `--page-size` | no | Default 20; cap by backend. |
| `cursor` | string | `--cursor` | no | Cursor from previous response; empty for first page. |
| `name` | string | `--name` | no | App name keyword. |

Normalized response:

```json
{
  "ok": true,
  "operation": "app.list",
  "page": {"pageSize": 20, "nextCursor": ""},
  "apps": [
    {
      "unifiedAppId": "UNIFIED_APP_ID",
      "name": "DemoApp",
      "appKey": "dingxxx",
      "creator": "Creator",
      "robotName": "BotName",
      "modifiedAt": "2026-06-05 00:00:00"
    }
  ],
  "raw": {}
}
```

#### get_dev_app

| Item | Contract |
| --- | --- |
| CLI | `dws devapp get` |
| HSF facade | `OpenInnerAppQueryFacade.getDetail` |
| Request shape | One object with system context and `unifiedAppId`. |
| Required public fields | `unifiedAppId`. |
| Identity source | `corpId/userId` from MCP system context. |

Locator field:

| Field | Type | CLI flag | Notes |
| --- | --- | --- | --- |
| `unifiedAppId` | string | `--unified-app-id` | Required app identity. |

Detail response may include identifiers, configuration summary, and credentials.
Treat `clientSecret/appSecret` as sensitive values in all user-visible output:

```json
{
  "ok": true,
  "operation": "app.get",
  "app": {
    "unifiedAppId": "UNIFIED_APP_ID",
    "name": "DemoApp",
    "appKey": "dingxxx",
    "developType": "internal",
    "creator": "Creator",
    "visibility": "partial_members",
    "status": "online",
    "gmtModified": "2026-06-05T00:00:00+08:00"
  },
  "raw": {}
}
```

#### create_dev_app

| Item | Contract |
| --- | --- |
| CLI | `dws devapp create` |
| HSF facade | `OpenInnerAppManageFacade.create` |
| Request shape | One object with system context and app base fields. |
| Required public fields | `name`; `--type` is a CLI-only guard and currently must be `internal`. |
| Write guard | CLI `--dry-run`, then `--yes`; no MCP confirm field. |

Input fields:

| Field | Type | CLI flag | Required | Notes |
| --- | --- | --- | --- | --- |
| `name` | string | `--name` | yes | Enterprise internal app name. |
| `desc` | string | `--desc` | no | App description. |
| `icon` | string | `--icon` | no | Existing media/resource id if backend accepts it. |

Response:

```json
{
  "ok": true,
  "operation": "app.create",
  "app": {
    "unifiedAppId": "UNIFIED_APP_ID",
    "name": "DemoApp",
    "appKey": "dingxxx"
  },
  "nextAction": "run devapp get or configure permissions",
  "raw": {}
}
```

#### update_dev_app

| Item | Contract |
| --- | --- |
| CLI | `dws devapp update` |
| HSF facade | `OpenInnerAppManageFacade.update` |
| Request shape | One object with system context, locator, and changed fields. |
| Required public fields | One app locator and at least one changed field. |
| Write guard | CLI `--dry-run`, then `--yes`; no MCP confirm field. |

Locator fields are the same as `get_dev_app`, but the Agent should
prefer `unifiedAppId` after resolving a unique app.

Update fields:

| Field | Type | CLI flag | Notes |
| --- | --- | --- | --- |
| `name` | string | `--name` | New app name. |
| `desc` | string | `--desc` | New app description. |
| `icon` | string | `--icon` | New icon resource id if backend accepts it. |
| `international` | object/string | `--international` | Only expose when schema describes the shape. |
| `supportHarmony` | boolean | `--support-harmony` | Harmony support switch if backend exposes it. |

Response should return the updated app summary plus a `changedFields` array when
available.

#### delete_dev_app

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

#### list_dev_app_permissions

| Item | Contract |
| --- | --- |
| CLI | `dws devapp permission list/search/detail` |
| HSF facade | `OpenInnerAppPermissionFacade.list` |
| Request shape | One object with system context, app locator, and optional filters. |
| Required public fields | `unifiedAppId`. |
| Web semantics | Equivalent to grouped `scope/list` for APP and SNS scopes. |

Input fields:

| Field | Type | CLI flag | Required | Notes |
| --- | --- | --- | --- | --- |
| `unifiedAppId` | string | `--unified-app-id` | yes | Unified app id. |
| `keyword` | string | `--keyword` | no | Maps to Web `key`. |
| `apiStatus` | string | `--api-status` | no | Web `apiStatus` filter. |
| `authStatus` | string | `--status` | no | `ALL/AUTHED/UNAUTHED`, post-filter. |
| `scopeType` | string | `--scope-type` | no | `APP/SNS`; empty returns both. |
| `scopeValue` | string | `--scope` | no | Detail mode and exact post-filter. |
| `pageSize` | number | `--page-size` | no | Return count cap; default 20. |
| `cursor` | string | `--cursor` | no | Cursor from previous response; empty for first page. |

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
      "scopeType": "APP",
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
  "nextCursor": "",
  "raw": {}
}
```

#### apply_dev_app_permissions

| Item | Contract |
| --- | --- |
| CLI | `dws devapp permission add` |
| HSF facade | `OpenInnerAppPermissionFacade.apply` |
| Request shape | One object with system context, app locator, and `scopeValues`. |
| Required public fields | `unifiedAppId` and non-empty `scopeValues`. |
| Write guard | CLI `--dry-run`, then `--yes`; no MCP confirm field. |

Input fields:

| Field | Type | CLI flag | Required | Notes |
| --- | --- | --- | --- | --- |
| `unifiedAppId` | string | `--unified-app-id` | yes | Unified app id. |
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

#### remove_dev_app_permissions

| Item | Contract |
| --- | --- |
| CLI | `dws devapp permission remove` |
| HSF facade | `OpenInnerAppPermissionFacade.remove` |
| Request shape | One object with system context, app locator, and `scopeValues`. |
| Required public fields | `unifiedAppId` and non-empty `scopeValues`. |
| Write guard | CLI `--dry-run`, then `--yes`; no MCP confirm field. |

Input fields:

| Field | Type | CLI flag | Required | Notes |
| --- | --- | --- | --- | --- |
| `unifiedAppId` | string | `--unified-app-id` | yes | Unified app id. |
| `scopeValues` | string[] | `--permissions` | yes | Remove one or more permissions. |

Expected structured failures:

| Condition | Normalized reason |
| --- | --- |
| Scope is not currently authorized | `PERMISSION_NOT_AUTHED` |
| Scope cannot be edited | `PERMISSION_NOT_EDITABLE` |
| Scope cannot be found | `PERMISSION_NOT_FOUND` |

## 4. Hardcoded Helper Mapping

The open-source branch hardcodes the following command root and tool mapping.
This JSON is kept as a compact mapping reference only; `devapp` runtime command
availability must not depend on MCP registry publication or service discovery.

```json
{
  "com.dingtalk.dws/helper/cli": {
    "id": "devapp",
    "command": "devapp",
    "description": "开放平台应用管理",
    "aliases": ["app"],
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
    "list_dev_app": {
      "cliName": "list",
      "description": "查询开放平台企业内部应用列表",
      "flags": {
        "pageSize": {"alias": "page-size", "default": 20},
        "cursor": {"alias": "cursor"},
        "name": {"alias": "name"}
      }
    },
    "get_dev_app": {
      "cliName": "get",
      "description": "查询开放平台企业内部应用详情",
      "flags": {
        "unifiedAppId": {"alias": "unified-app-id", "required": true}
      }
    },
    "create_dev_app": {
      "cliName": "create",
      "description": "创建开放平台企业内部应用",
      "isSensitive": true,
      "flags": {
        "name": {"alias": "name", "required": true},
        "desc": {"alias": "desc"},
        "icon": {"alias": "icon"}
      }
    },
    "update_dev_app": {
      "cliName": "update",
      "description": "修改开放平台企业内部应用基础信息",
      "isSensitive": true,
      "flags": {
        "unifiedAppId": {"alias": "unified-app-id", "required": true},
        "name": {"alias": "name"},
        "desc": {"alias": "desc"},
        "icon": {"alias": "icon"}
      }
    },
    "delete_dev_app": {
      "cliName": "delete",
      "description": "删除开放平台企业内部应用",
      "isSensitive": true,
      "flags": {
        "unifiedAppId": {"alias": "unified-app-id", "required": true}
      }
    },
    "list_dev_app_permissions": {
      "cliName": "list",
      "group": "permission",
      "description": "查询开放平台应用权限列表，默认同时返回 APP 应用权限和 SNS 个人权限",
      "flags": {
        "unifiedAppId": {"alias": "unified-app-id"},
        "keyword": {"alias": "keyword"},
        "apiStatus": {"alias": "api-status"},
        "authStatus": {"alias": "status", "default": "ALL"},
        "scopeType": {"alias": "scope-type"},
        "scopeValue": {"alias": "scope"},
        "pageSize": {"alias": "page-size", "default": "20"},
        "cursor": {"alias": "cursor"}
      }
    },
    "apply_dev_app_permissions": {
      "cliName": "add",
      "group": "permission",
      "description": "申请开放平台应用权限点；需审核权限写入当前应用版本变更",
      "isSensitive": true,
      "flags": {
        "unifiedAppId": {"alias": "unified-app-id"},
        "scopeValues": {
          "alias": "permissions",
          "type": "stringSlice",
          "required": true,
          "description": "待申请权限点 scopeValue，多个用逗号分隔"
        }
      }
    },
    "remove_dev_app_permissions": {
      "cliName": "remove",
      "group": "permission",
      "description": "取消一个或多个已开通的开放平台应用权限点",
      "isSensitive": true,
      "flags": {
        "unifiedAppId": {"alias": "unified-app-id"},
        "scopeValues": {
          "alias": "permissions",
          "type": "stringSlice",
          "required": true,
          "description": "待取消权限点 scopeValue，多个用逗号分隔"
        }
      }
    }
  }
}
```

`permission search/detail` are UX aliases over `list_dev_app_permissions`. The helper exposes one canonical MCP call and keeps alias behavior at the CLI layer:

```bash
dws devapp permission list --keyword "机器人发送消息"
dws devapp permission list --scope qyapi_robot_sendmsg
```

If a helper command is added, map aliases without creating new MCP tools:

| Helper command | Underlying tool | Forced/derived args |
| --- | --- | --- |
| `devapp permission search` | `list_dev_app_permissions` | Requires `keyword` or `scopeValue`; returns candidate summaries. |
| `devapp permission detail` | `list_dev_app_permissions` | Requires `scopeValue`; returns one scope with full `apiList`. |

## 5. App Identity Resolution

Application identity must be resolved before detail and write commands.

| Priority | Identifier | Rule |
| --- | --- | --- |
| 1 | `unifiedAppId` | Preferred unique id for write operations. |
| 2 | `appKey/clientId` | Call list/detail and require one result. |
| 3 | `name` | Fuzzy search; write operations require one result. |

Agent rule:

1. Never choose the first row when multiple apps match.
2. Show candidates with `name`, `unifiedAppId`, `appKey/clientId`, `creator`, and modified time.
3. Ask the user to pick `unifiedAppId` for write operations when multiple apps match.

MCP calls must rely on system context for caller identity:

| System context | HSF request field | Notes |
| --- | --- | --- |
| Caller org | `corpId` | DingTalk corpId, not internal orgId. |
| Caller user | `userId` | DingTalk userId/staffId; backend resolves uid. |

Open-source DWS and Agent must not pass `orgId` or `uid` as user input.

## 6. App CRUD

### 6.1 List

```bash
dws devapp list --page-size 20 --format json
dws devapp list --name DemoApp --format json
dws devapp list --page-size 20 --cursor NEXT_CURSOR --format json
```

MCP payload:

```json
{
  "pageSize": 20,
  "name": "DemoApp"
}
```

Pagination is cursor based. A baseline call should pass only `pageSize`; use the response `nextCursor` for the next page.

### 6.2 Get

```bash
dws devapp get --unified-app-id UNIFIED_APP_ID --format json
```

MCP payload:

```json
{
  "unifiedAppId": "UNIFIED_APP_ID"
}
```

`get` may show `clientId/appKey/clientSecret/appSecret`; callers must treat secret fields as sensitive. Active secret reads still prefer `credentials get`.

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
  "name": "DemoApp",
  "desc": "internal app"
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
| `--name` | `name` | New app name. |
| `--desc` | `desc` | New description. |
| `--icon` | `icon` | Existing media id or backend-supported resource. |
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

Valid public locator field is `unifiedAppId`.

## 7. Credentials

Target command:

```bash
dws devapp credentials get --unified-app-id UNIFIED_APP_ID --format json
```

Current status:

1. Backend facade is `OpenInnerAppQueryFacade.getCredentials`.
2. MCP tool is `get_dev_app_credentials`.
3. CLI sends only application locator fields and does not add `showSecret`, `confirmSecret`, or masking fields.
4. The MCP response follows the backend credential contract and may include `clientSecret/appSecret`; callers must treat command output as sensitive.

Target fields:

| Field | Notes |
| --- | --- |
| `unifiedAppId/name` | App summary. |
| `clientId/appKey` | Non-secret identifiers. |
| `clientSecret/appSecret` | Returned by the dedicated credentials tool; treat as sensitive output. |
| `currentSecretStatus` | Current secret state. |
| `pendingExpireTask` | Pending secret-expiration task information when present. |

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
      {"title": "个人权限", "scopeType": "SNS", "scopeList": []}
    ],
    "appScopes": [
      {"title": "通讯录管理", "scopeType": "APP", "scopeList": []}
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
  "pageSize": 20
}
```

MCP payload for search:

```json
{
  "unifiedAppId": "UNIFIED_APP_ID",
  "keyword": "机器人发送消息",
  "authStatus": "UNAUTHED",
  "pageSize": 10
}
```

MCP payload for detail:

```json
{
  "unifiedAppId": "UNIFIED_APP_ID",
  "scopeValue": "qyapi_robot_sendmsg",
  "pageSize": 1
}
```

Input fields:

| Field | Type | Required | Notes |
| --- | --- | --- | --- |
| `unifiedAppId` | string | yes | Unified app id. |
| `keyword` | string | no | Maps to Web `key`. |
| `apiStatus` | string | no | Maps to Web `apiStatus`. |
| `authStatus` | string | no | `ALL/AUTHED/UNAUTHED`, post-filter. |
| `scopeType` | string | no | `APP/SNS`; empty returns both. |
| `scopeValue` | string | no | Detail mode and exact post-filter. |
| `pageSize` | number | no | Default 20, hard cap 50. |
| `cursor` | string | no | Cursor from previous response; empty for first page. |

Permission list uses cursor pagination. Use `keyword` to reduce server-side results and `pageSize/cursor` to control Agent context.

Default response must flatten APP and SNS scopes into one stable `permissions[]` array:

```json
{
  "mode": "list",
  "permissions": [
    {
      "scopeValue": "qyapi_robot_sendmsg",
      "scopeName": "企业内机器人发消息权限",
      "scopeDesc": "企业内机器人发消息权限",
      "scopeType": "APP",
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
  "scopeType": "SNS",
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
4. Category title and `scopeType`.

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
dws devapp permission remove --unified-app-id UNIFIED_APP_ID --permissions qyapi_robot_sendmsg --dry-run --format json
dws devapp permission remove --unified-app-id UNIFIED_APP_ID --permissions qyapi_robot_sendmsg --yes --format json
```

MCP payload:

```json
{
  "unifiedAppId": "UNIFIED_APP_ID",
  "scopeValues": ["qyapi_robot_sendmsg"]
}
```

Rules:

1. Remove only one scope per call.
2. `authed=false` returns `NOT_AUTHED`.
3. `canEdit=false` returns a no-edit reason.
4. Removing an authorized permission is not the same as withdrawing a pending approval.

## 9. Event And Version Boundary

Event and version commands are needed for a full application lifecycle but should not be folded into permission add.

Implemented event tools (verified against the `op-app` MCP `tools/list`). The
real model is "list subscribable events + subscribe/unsubscribe events by
`eventCodes`", not a single callbackUrl/token/aesKey config call:

| CLI | MCP tool | Notes |
| --- | --- | --- |
| `dev app event list` | `list_dev_app_events` | Args `unifiedAppId/keyword/cursor/pageSize`; `keyword` searches `eventCode` or `eventName`; returns `pushType=STREAM` + `events[]` (`eventCode`/`eventName`/`subscribed`) + `hasMore/nextCursor/pageSize`. Read. |
| `dev app event subscribe` | `subscribe_dev_app_events` | Args `unifiedAppId` + required array `eventCodes`. Returns `success/operation/eventCodes/needsPublish/versionRequiredAction`; failures include `errorCode/errorMsg/reason/retryable/action` such as `STREAM_NOT_CONNECTED`. Write (`--dry-run`/`--yes`). |
| `dev app event unsubscribe` | `unsubscribe_dev_app_events` | Args `unifiedAppId` + required array `eventCodes`. Returns `success/operation/eventCodes/needsPublish/versionRequiredAction`; failures include `errorCode/errorMsg/reason/retryable/action`. Write. |

Note: for gray unified apps, subscribe/unsubscribe are staged into version
metadata and only take effect after `version publish`. The event callback URL
itself is not part of this model; it lives in robot config (`eventCallbackUrl`,
exposed as `robot config --event-url`).

Implemented version tools (verified against the `op-app` MCP `tools/list`):

| CLI | MCP tool | Notes |
| --- | --- | --- |
| `devapp version create` | `create_dev_app_version` | Save a new version from current config. Omit `version` by default so the server auto-increments from the latest released version; only pass `version` when the user explicitly requests a higher version. |
| `devapp version list` | `list_dev_app_versions` | Cursor-paged list (`cursor`, `pageSize`; response `items/nextCursor/hasMore`). |
| `devapp version get` | `get_dev_app_version_detail` | One version detail by `versionId`. |
| `devapp version check-approval` | `publish_dev_app_version` (`precheckOnly=true`) | Precheck approval requirement / approvers via `requiresApproval/publishable/approvalMode/approvalCandidates`; does not publish. |
| `devapp version publish` | `publish_dev_app_version` (`precheckOnly=false`) | Publish or submit approval; inspect `published/approvalSubmitted/processId/versionStatus`. |
| `devapp version status` | `get_dev_app_version_status` | Poll publish and approval state. |

Note: the MCP `precheckOnly` field is the server-side approval precheck and maps to
`version check-approval`. It is distinct from the CLI global `--dry-run` (preview
without execution). `check-approval` is read-only (no write guard); `publish`
uses the standard `--dry-run`/`--yes` write guard.

Implemented robot tools:

| CLI | MCP tool | Notes |
| --- | --- | --- |
| `devapp robot create` | `create_dingtalk_robot` | Synchronously create a new agent app + robot. |
| `devapp robot submit` | `submit_robot_create_task` | Async submit; supports retry via original `taskId`. |
| `devapp robot result` | `query_robot_create_result` | Poll async task by `taskId`. |
| `devapp robot get` | `get_extension_robot_config` | Read robot config of an existing app. |
| `devapp robot config` | `set_extension_robot_config` | Create or update robot config on an existing app. |
| `devapp robot enable` | `enable_dev_app_robot` | Enable / re-enable robot capability. |
| `devapp robot disable` | `disable_dev_app_robot` | Disable robot capability. |

Robot create/submit and config/enable/disable are write operations and
use the `--dry-run`/`--yes` guard; `get` and `result` are reads.

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
| `unifiedAppId`, `clientId`, `appKey`, `appSecret`, `clientSecret` | `devapp` |
| `应用权限`, `权限点`, `API 权限`, `APP/SNS 权限`, `scopeValue` | `devapp permission ...` |
| `事件订阅`, `订阅事件`, `退订事件`, `eventCode`, `可订阅事件` in an app context | `devapp event ...` |
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
| "查应用/搜索应用" | `devapp list` | `list_dev_app` |
| "看应用详情/clientId/appKey" | `devapp get` | `get_dev_app` |
| "创建应用" | `devapp create` | `create_dev_app` |
| "修改名称/描述/图标" | `devapp update` | `update_dev_app` |
| "删除应用" | `devapp delete` | `delete_dev_app` |
| "查权限/搜索 API 权限" | `permission list` | `list_dev_app_permissions` |
| "这个权限覆盖哪些 API" | `permission list --scope` | `list_dev_app_permissions` |
| "申请权限/开通权限" | `permission add` | `apply_dev_app_permissions` |
| "取消权限/移除权限" | `permission remove` | `remove_dev_app_permissions` |
| "拿 appSecret/clientSecret" | `credentials get` | `get_dev_app_credentials` |
| "查可订阅事件/订阅/退订事件" | `event list/subscribe/unsubscribe` | `list_open_dev_app_events`, `subscribe/unsubscribe_open_dev_app_event` |
| "发布/审核/选审批人" | `version ...` | `publish_open_dev_app_version` etc. |

Yulan invocation examples:

| User wording | Expected first command | Reason |
| --- | --- | --- |
| "查一下开发者后台应用 DemoApp" | `dws devapp list --name DemoApp --format json` | OpenDev context plus app name means listByCondition search. |
| "把应用图标换成 ICON_RESOURCE" | `dws devapp update --unified-app-id ... --icon ICON_RESOURCE --dry-run --format json` | Icon change is app base info update and must be guarded. |
| "已确认删除这个开放平台应用" | `dws devapp delete --unified-app-id ... --yes --format json` | Destructive write only after unique app resolution and confirmation. |
| "手机号权限要申请，虽然需要审核" | `dws devapp permission add --permissions Contact.User.mobile --dry-run/--yes` | `requiredApproval=true` is still appliable; approval moves to version publish. |
| "订阅通讯录用户增加事件" | `dws dev app event list ...` then `dws dev app event subscribe --event-codes user_add_org --dry-run --format json` | Pick eventCode from list, then subscribe; gray apps need a version publish to take effect. |
| "保存版本并看审核状态" | `version create`, `check-approval`, `publish`, `status` | Version flow owns approver selection and publish audit state. |
| "创建 MCP 服务并配置 HSF tool" | OpenDev MCP platform workflow, not `dws devapp create` | MCP connector/tool is a platform artifact, not an enterprise internal app. |

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

### 11.1 Runtime Preflight

Before running a devapp command in a new environment, the Agent should verify the
hardcoded helper surface. Endpoint configuration is an internal edition concern;
the Agent should not require users to paste or export MCP gateway keys.

```bash
dws devapp --help
dws app --help
```

If the command or endpoint is missing:

1. If `dws devapp --help` fails, report `DEVAPP_COMMAND_UNAVAILABLE`.
2. If execution reports `endpoint_not_resolved`, report
   `DEVAPP_ENDPOINT_NOT_CONFIGURED` and ask the owner to check the internal
   edition `SupplementServers/StaticServers` injection. Local developers may
   override with `DINGTALK_DEVAPP_MCP_URL`, but Agents should not request it
   from end users.
3. Do not reroute the task to `devdoc`, `doc`, or generic `mcp` commands unless
   the user is asking for documentation rather than application management.

If a command flag in this document conflicts with `--help`, use `--help`. The
document defines the target contract; the checked-in helper code defines the
current callable command surface.

### 11.2 Command Construction

Use this order when constructing a call:

1. Identify the product as `devapp` only when the request matches section 10.1
   positive signals or the active yulan OpenDev app workstream. Do not use
   `devapp` for every bare `应用`.
2. Resolve application identity before any detail or write operation.
3. Prefer checked-in CLI flags over raw `--json`.
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
| `confirmSecret=true` | Do not send; `credentials get` has no confirm/show-secret payload field. |

### 11.3 Business Recipes

| User request | Agent recipe |
| --- | --- |
| "查 DemoApp 应用" | `devapp list --name DemoApp`; if one result, optionally `devapp get`; if multiple, show candidates. |
| "这个 appKey 是哪个应用" | 当前 CLI 不用 appKey 定位详情；先让用户提供 `unifiedAppId`，或用 `devapp list --name` 缩小候选后再 `devapp get --unified-app-id ...`。 |
| "创建企业内部应用" | `devapp create --name ... --type internal --dry-run`; after confirmation, repeat with `--yes`. |
| "修改应用描述/图标" | Resolve `unifiedAppId`; `devapp update --unified-app-id ... --desc/icon ... --dry-run`; after confirmation, `--yes`. |
| "删除应用" | Resolve unique app; show app summary; `devapp delete ... --dry-run`; after confirmation, `--yes`. |
| "查询机器人发消息权限" | `devapp permission list --keyword "机器人发消息"`; if candidates are broad, require `scopeValue`. |
| "这个 API 需要哪个权限" | Search by API name; inspect `apiPreview`; if uncertain, use `permission list --scope SCOPE` for detail. |
| "申请通讯录手机号权限" | Resolve app; `permission list --keyword "手机号"`; choose `scopeValue`; `permission add --permissions SCOPE --dry-run`; then `--yes`. |
| "权限已进版本，继续发布" | `version check-approval`; choose approver if needed; `version publish --yes`; poll `version status`. |
| "拿 clientSecret/appSecret" | Use only `credentials get`; never fall back to `devapp get`. |

### 11.4 Permission Candidate Selection

Permission list is intentionally one grouped list, not separate
`listAuthed/listAvailable` calls. The Agent must reduce the result set before
writing:

1. If the user gives a `scopeValue`, use exact match.
2. If the user gives an API name, search with `keyword`, then match
   `apiList[].name` or `apiPreview.name`.
3. If the user gives a human permission name, match `scopeName/scopeDesc`.
4. If multiple scopes remain, show `scopeValue`, `scopeName`, `scopeType`,
   `authed`, `requiredApproval`, and `apiPreview`, then ask for one.
5. For list output, cap the displayed list with `pageSize`; for detail output, call
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
| `DEVAPP_COMMAND_UNAVAILABLE` | `unknown command` for `dws devapp` | Report that the local CLI build does not contain the devapp helper. Do not try cache refresh as the primary fix. |
| `DEVAPP_ENDPOINT_NOT_CONFIGURED` | `endpoint_not_resolved`, missing internal edition endpoint | Ask the owner to check `SupplementServers/StaticServers`; local debug may use `DINGTALK_DEVAPP_MCP_URL`, but do not ask users for gateway keys. |
| `DEVAPP_TOOL_NOT_PUBLISHED` | Product exists but one tool is missing | Report the missing MCP tool key and avoid substituting a different operation. |
| `AUTH_CONTEXT_MISSING` | Backend cannot resolve `corpId/userId` | Tell the user to re-login or check organization context; do not ask for internal `orgId/uid`. |
| `APP_NOT_FOUND` | Empty list/detail result | Ask for another app identifier; do not create or update implicitly. |
| `APP_AMBIGUOUS` | Multiple apps match name/key | Show candidate ids and stop before write. |
| `PERMISSION_NOT_FOUND` | Scope search has no exact candidate | Ask for another keyword or API name. |
| `PERMISSION_ALREADY_AUTHED` | Add receives an already authorized scope | Report skipped scope and continue only for remaining unauth scopes. |
| `PERMISSION_NOT_AUTHED` | Remove receives an unauthorized scope | Report that there is nothing to remove. |
| `PERMISSION_NOT_EDITABLE` | `canEdit=false` or backend no-edit error | Show the no-edit reason; do not retry as a write. |
| `requiresApproval=true` | `check-approval` says approval is required | Use `approvalMode/approvalCandidates`; ask for approver only in `SELECT_APPROVER` mode. |
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
    "name": "DemoApp"
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

The open-source branch uses a hardcoded helper path for `devapp`; do not depend
on service discovery for command visibility.

1. Add `internal/helpers/devapp.go`, following `internal/helpers/devdoc.go`.
2. Register the root with `RegisterPublic`.
3. Route helper invocations with `executor.NewHelperInvocation`.
4. Keep command aliases such as `app` and `permission search/detail` in the
   helper layer, not as separate MCP tools.
5. Add tests for dry-run payloads, `--yes` guard, app resolution, and permission routing.

Internal edition endpoint injection:

```go
SupplementServers: func() []edition.ServerInfo {
    return []edition.ServerInfo{
        {
            ID:       "devapp",
            Name:     "钉钉开放平台应用管理",
            Endpoint: loadDevappMCPEndpointFromInternalConfig(),
            Prefixes: []string{"devapp", "app"},
        },
    }
}
```

The endpoint loader must read from internal config or secret storage. Do not put
the Streamable HTTP URL or `?key=` value in this repository. `StaticServers` is
also valid for an internal build that wants to skip Market discovery entirely.

Recommended helper root:

```text
Name(): "devapp"
Aliases: ["app"] if supported by cobra and no conflict
```

Do not manually edit generated command indexes unless the generator is part of the change. The runtime command surface should be checked with:

```bash
dws devapp --help
dws app --help
```

## 13. Rollout And Verification

Use this checklist when moving from backend HSF implementation to usable DWS
commands. The goal is to prove that the same operation works at the MCP layer,
the DWS helper layer, and the Agent recipe layer.

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
   `unifiedAppId`, `appKey`, `name`, or `userId`.
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
| `list_dev_app` | `{"pageSize":10}` |
| `get_dev_app` | `{"unifiedAppId":"UNIFIED_APP_ID"}` |
| `create_dev_app` | `{"name":"DemoApp","desc":"internal app"}` |
| `update_dev_app` | `{"unifiedAppId":"UNIFIED_APP_ID","desc":"new desc"}` |
| `delete_dev_app` | `{"unifiedAppId":"UNIFIED_APP_ID"}` |
| `list_dev_app_permissions` | `{"unifiedAppId":"UNIFIED_APP_ID","authStatus":"ALL","pageSize":20}` |
| `apply_dev_app_permissions` | `{"unifiedAppId":"UNIFIED_APP_ID","scopeValues":["SCOPE_VALUE"]}` |
| `remove_dev_app_permissions` | `{"unifiedAppId":"UNIFIED_APP_ID","scopeValues":["SCOPE_VALUE"]}` |

For destructive debug cases, use a disposable application and permission scope.
Do not use production data in long-lived examples.

### 13.2 MCP Direct Smoke

After publish, verify the service through MCP before using DWS:

1. Run MCP `initialize`.
2. Run `tools/list` and confirm all expected P0 tool keys exist.
3. Call `list_dev_app` with only pagination.
4. Call one exact locator scenario, such as `get_dev_app`.
5. Call permission list with `authStatus=ALL` and confirm APP and SNS scopes are
   both represented.
6. Confirm failures preserve `errorCode/errorMsg`.

Do not store the Streamable HTTP URL or `?key=` value in repository files,
README, skills, or issue text.

### 13.3 DWS Helper Smoke

After the MCP service is published and the internal edition injects the endpoint
through `SupplementServers` or `StaticServers`, verify the hardcoded helper path:

```bash
dws devapp --help
dws app --help
dws devapp list --page 1 --page-size 10 --format json
```

Expected:

1. Product id is `devapp`.
2. Command alias `app` exists, but `devapp` is canonical.
3. P0 command leaves appear in help.
4. `--format json` is available through global output flags.
5. Sensitive writes are marked or guarded so Agent uses `--dry-run` then `--yes`.

If `devapp` is missing, treat it as `DEVAPP_COMMAND_UNAVAILABLE`. If command
execution reports `endpoint_not_resolved`, check the internal edition endpoint
injection. Local debug may use `DINGTALK_DEVAPP_MCP_URL` as a temporary override.
If the endpoint is configured but a backend tool is unavailable, treat it as
`DEVAPP_TOOL_NOT_PUBLISHED`.

### 13.4 Agent Smoke

Use these smoke cases to prove the yulan invocation problem is solved for an
Agent. They intentionally cover routing, narrowing, write guard, and backend
error propagation.

| Scenario | Smoke command | Pass criteria |
| --- | --- | --- |
| Discover product | `dws devapp --help` | Helper command exists and exposes devapp CLI metadata. |
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
2. `dws devapp --help` shows P0 helper leaves and `dws app --help` resolves as
   the compatibility alias.
3. App list/detail/create/update/delete smoke cases pass.
4. Permission list/add/remove smoke cases pass for APP and SNS scopes.
5. `requiredApproval=true` permission add stages into version changes instead of
   being rejected by the Agent.
6. Credential access is handled only by `credentials get`; event subscription
   (`event list/subscribe/unsubscribe`) and version flow are implemented over
   `op-app`, and any still-missing tool is reported as unsupported, not silently
   emulated by other commands.
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
| `dws devapp create --name T --type internal --yes --format json` | Calls `create_dev_app`. |
| `dws devapp update --unified-app-id X --desc Y --yes --format json` | Calls `update_dev_app`. |
| `dws devapp delete --unified-app-id X --format json` | Refuses because delete confirmation is missing. |
| `dws devapp permission list --unified-app-id X --format json` | Returns flattened APP and SNS permissions. |
| `dws devapp permission list --unified-app-id X --keyword "发送消息" --format json` | Returns candidate `permissions[]`; no automatic apply. |
| `dws devapp permission add --unified-app-id X --permissions Contact.User.mobile --yes --format json` | Applies or stages permission; `requiredApproval=true` is allowed. |
| `dws devapp permission remove --unified-app-id X --permissions qyapi_robot_sendmsg --yes --format json` | Removes authorized scopes or returns a structured reason. |
| `dws devapp credentials get --unified-app-id X --format json` | Calls `get_dev_app_credentials`; does not use `devapp get` as a secret fallback. |
| `查应用 appXYZ 的详情` without OpenDev context | Does not blindly route to `devapp`; uses context such as `workbench app get` or asks. |
| `查询开放平台 API 错误码` | Routes to `devdoc`, not `devapp`. |
| `创建 MCP 服务并配置 HSF tool` | Uses OpenDev MCP platform workflow, not `dws devapp create`. |
