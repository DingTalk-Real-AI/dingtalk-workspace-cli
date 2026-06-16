# 事件订阅

> 概念锚点：把应用关心的事件推到你的回调地址。所有命令用 `--unified-app-id` 定位；写操作先 `--dry-run`，确认后再 `--yes`。

为企业内部应用订阅、查询、取消开放平台事件推送。

## 查询已订阅事件

```bash
dws dev app event list --unified-app-id ID --format json
dws dev app event list --unified-app-id ID --page-size 50 --format json
dws dev app event list --unified-app-id ID --cursor <token> --page-size 50 --format json
```

| CLI | 说明 |
|-----|------|
| `--unified-app-id` | 应用定位 |
| `--page-size` | 单页条数 |
| `--cursor` | 游标令牌：首次留空，续翻传上次返回的游标 |

出参字段：

- `events`：已订阅事件列表（每项含事件类型、回调地址等）。
- `pushType`：推送方式（如 HTTP 回调）。

游标分页：返回的游标为空表示已到末尾。

## 订阅事件

```bash
dws dev app event subscribe --unified-app-id ID --event-types chat_add_member_org --callback-url https://example.com/event --dry-run --format json
dws dev app event subscribe --unified-app-id ID --event-types chat_add_member_org,chat_remove_member_org --callback-url https://example.com/event --yes --format json
```

| CLI | 说明 |
|-----|------|
| `--unified-app-id` | 应用定位 |
| `--event-types` | 事件类型，多个逗号分隔 |
| `--callback-url` | 事件回调地址 |

**规则：**
- 写操作，先 `--dry-run` 预览，确认后 `--yes`。
- 一次可订阅多个事件类型（逗号分隔），共用同一回调地址。
- 事件类型取值以开放平台文档为准；不确定走 `dws dev doc search`。

## 取消订阅

```bash
dws dev app event unsubscribe --unified-app-id ID --event-types chat_add_member_org --dry-run --format json
dws dev app event unsubscribe --unified-app-id ID --event-types chat_add_member_org --yes --format json
```

| CLI | 说明 |
|-----|------|
| `--unified-app-id` | 应用定位 |
| `--event-types` | 要取消的事件类型，多个逗号分隔 |

**规则：**
- 写操作，先 `--dry-run` 预览，确认后 `--yes`。
- 取消前可先 `event list` 确认当前订阅，避免取消不存在的事件类型。
