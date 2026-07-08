# 事件订阅

管理开放平台企业内部应用的事件订阅：列出可订阅事件、按事件码列表批量订阅/退订。

> 所有命令通过 `--unified-app-id` 定位应用；`corpId` / `userId` 由 MCP 系统上下文注入，CLI 不传。订阅/退订是写操作，先 `--dry-run`，确认后再 `--yes`。

## 可订阅事件列表

**搜索优先**：定位事件码时优先用 `--keyword` 搜索；只有用户明确要「全部事件」时才不带 `--keyword` 翻全量。

```bash
# 首选：按关键词搜索定位事件码
dws dev app event list --unified-app-id <unifiedAppId> --keyword 通讯录 --page-size 20 --format json
# 兜底：用户明确要全部事件时才翻全量
dws dev app event list --unified-app-id <unifiedAppId> --page-size 20 --format json
```

MCP tool: `list_dev_app_events`。`--keyword` 按事件码或事件名称模糊匹配。返回 `pushType=STREAM`、`events[]`、`hasMore`、`nextCursor`、`pageSize`；`hasMore=true` 时下一页继续传 `--cursor <nextCursor>`，逐页处理。`events[]` 每项含：

| 字段 | 含义 |
|------|------|
| `eventCode` | 事件码，订阅/退订时传入，如 `user_add_org` |
| `eventName` | 事件名称，如「通讯录用户增加」 |
| `subscribed` | 当前是否已订阅 |

常见事件码示例：`user_add_org`(通讯录用户增加) / `user_modify_org`(用户更改) / `user_leave_org`(用户离职) / `org_dept_create`(部门创建) / `org_dept_modify`(部门修改) / `org_dept_remove`(部门删除)。以 `event list` 实际返回为准。

## 订阅事件

```bash
dws dev app event subscribe --unified-app-id <unifiedAppId> --event-codes user_add_org,org_dept_modify --dry-run --format json
dws dev app event subscribe --unified-app-id <unifiedAppId> --event-codes user_add_org,org_dept_modify --yes --format json
```

MCP tool: `subscribe_dev_app_events`（入参 `unifiedAppId` + 必填数组 `eventCodes`，一次订阅多个事件码）。成功返回 `success/operation=SUBSCRIBE/unifiedAppId/eventCodes/needsPublish/versionRequiredAction`；失败时补 `errorCode/errorMsg/reason/retryable/action`。

## 退订事件

```bash
dws dev app event unsubscribe --unified-app-id <unifiedAppId> --event-codes user_add_org,org_dept_modify --dry-run --format json
dws dev app event unsubscribe --unified-app-id <unifiedAppId> --event-codes user_add_org,org_dept_modify --yes --format json
```

MCP tool: `unsubscribe_dev_app_events`（入参 `unifiedAppId` + 必填数组 `eventCodes`，一次退订多个事件码）。成功返回 `success/operation=UNSUBSCRIBE/unifiedAppId/eventCodes/needsPublish/versionRequiredAction`；失败时补 `errorCode/errorMsg/reason/retryable/action`。

## 字段映射

| CLI | MCP | 必填 | 说明 |
|-----|-----|------|------|
| `--unified-app-id` | `unifiedAppId` | 是 | 统一应用 ID |
| `--keyword` | `keyword` | list 可选 | 事件搜索关键词，匹配事件码或事件名称 |
| `--cursor` | `cursor` | list 可选 | 服务端返回的下一页游标 |
| `--page-size` | `pageSize` | list 可选 | 单页数量 |
| `--event-codes` | `eventCodes` | subscribe/unsubscribe 必填 | 事件码列表，逗号分隔，取自 `event list` |

## 灰度应用需发布版本生效（重要）

对**灰度统一应用**，`subscribe`/`unsubscribe` 只会把变更**暂存到版本元数据**，不会立即生效，需后续走版本发布链路：

```text
event subscribe/unsubscribe   订阅变更写入版本元数据
  → version create            创建新版本
  → version check-approval     预检审批
  → version publish            发布后订阅变更才生效
  → version status             轮询发布/审批状态
```

详见版本发布文档。

## 单步流程

```text
1. 定位事件码（搜索优先）
   dws dev app event list --unified-app-id <ID> --keyword <关键词> --format json
   → 从返回里挑 eventCode
   → 用户明确要全部事件时，才不带 --keyword 翻全量（逐页）

2. 订阅（批量/全量前先列候选 eventCode 给用户确认）
   dws dev app event subscribe --unified-app-id <ID> --event-codes <CODE1>,<CODE2> --dry-run --format json
   → 确认后加 --yes

3. 验证
   dws dev app event list --unified-app-id <ID> --keyword <关键词> --format json
   → 确认对应事件 subscribed=true（灰度应用需先发布版本）
```

## 错误处理

| 情况 | 处理 |
|------|------|
| `--event-codes` 缺失 | CLI 直接报错，先 `event list` 取事件码 |
| 订阅后 `subscribed` 仍为 false | 灰度应用需先 `version publish` 发布版本 |
| 返回提示「长链接未在线」（泛化错误 `reason=business_error`、`server_error_code=-1`，无 STREAM_NOT_CONNECTED 码） | 先执行 `dev connect` 建联，再重试订阅 |
| `ServiceResult.success=false` | 透传 `errorCode/errorMsg` |
