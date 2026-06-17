# 事件订阅

管理开放平台企业内部应用的事件订阅：列出可订阅事件、按事件码列表批量订阅/退订。

> 所有命令通过 `--unified-app-id` 定位应用；`corpId` / `userId` 由 MCP 系统上下文注入，CLI 不传。订阅/退订是写操作，先 `--dry-run`，确认后再 `--yes`。

## 可订阅事件列表

```bash
dws devapp event list --unified-app-id <unifiedAppId> --format json
```

MCP tool: `list_dev_app_events`。返回 `pushType`（当前推送类型）和 `events[]`，每项含：

| 字段 | 含义 |
|------|------|
| `eventCode` | 事件码，订阅/退订时传入，如 `user_add_org` |
| `eventName` | 事件名称，如「通讯录用户增加」 |
| `subscribed` | 当前是否已订阅 |

常见事件码示例：`user_add_org`(通讯录用户增加) / `user_modify_org`(用户更改) / `user_leave_org`(用户离职) / `org_dept_create`(部门创建) / `org_dept_modify`(部门修改) / `org_dept_remove`(部门删除)。以 `event list` 实际返回为准。

## 订阅事件

```bash
dws devapp event subscribe --unified-app-id <unifiedAppId> --event-codes user_add_org,org_dept_modify --dry-run --format json
dws devapp event subscribe --unified-app-id <unifiedAppId> --event-codes user_add_org,org_dept_modify --yes --format json
```

MCP tool: `subscribe_dev_app_events`（入参 `unifiedAppId` + `eventCodes`，一次订阅多个事件码）。

## 退订事件

```bash
dws devapp event unsubscribe --unified-app-id <unifiedAppId> --event-codes user_add_org,org_dept_modify --dry-run --format json
dws devapp event unsubscribe --unified-app-id <unifiedAppId> --event-codes user_add_org,org_dept_modify --yes --format json
```

MCP tool: `unsubscribe_dev_app_events`（入参 `unifiedAppId` + `eventCodes`，一次退订多个事件码）。

## 字段映射

| CLI | MCP | 必填 | 说明 |
|-----|-----|------|------|
| `--unified-app-id` | `unifiedAppId` | 是 | 统一应用 ID |
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

详见 [version.md](version.md)。

## 单步流程

```text
1. 查看可订阅事件及当前状态
   dws devapp event list --unified-app-id <ID> --format json
   → 从返回里挑 eventCode

2. 订阅
   dws devapp event subscribe --unified-app-id <ID> --event-codes <CODE1>,<CODE2> --dry-run --format json
   → 确认后加 --yes

3. 验证
   dws devapp event list --unified-app-id <ID> --format json
   → 确认对应事件 subscribed=true（灰度应用需先发布版本）
```

## 错误处理

| 情况 | 处理 |
|------|------|
| `--event-codes` 缺失 | CLI 直接报错，先 `event list` 取事件码 |
| 订阅后 `subscribed` 仍为 false | 灰度应用需先 `version publish` 发布版本 |
| `ServiceResult.success=false` | 透传 `errorCode/errorMsg` |
