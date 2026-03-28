---
name: dws-calendar
description: "钉钉日历: 支持创建日程，查询日程，约空闲会议室等能力."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws calendar --help"
---

# calendar

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

- Display name: 钉钉日历
- Description: 支持创建日程，查询日程，约空闲会议室等能力
- Endpoint: `https://mcp-gw.dingtalk.com/server/3cb83d4ac411227c44c1abde4e4bfbae0ea2c172b83a78a33ffc3821d0d1be47`
- Protocol: `2025-03-26`
- Degraded: `false`

```bash
dws calendar <group> <command> --json '{...}'
```

## Helper Commands

| Command | Tool | Description |
|---------|------|-------------|
| [`dws-calendar-participant-add`](./participant/add.md) | `add_calendar_participant` | 向已存在的指定日程添加参与者，支持批量添加多人，可设置参与者类型和通知方式 |
| [`dws-calendar-room-add`](./room/add.md) | `add_meeting_room` | 添加会议室 |
| [`dws-calendar-event-create`](./event/create.md) | `create_calendar_event` | 创建新的日程，支持设置时间、参与者、提醒等完整功能 |
| [`dws-calendar-event-delete`](./event/delete.md) | `delete_calendar_event` | 删除指定日程，组织者删除将通知所有参与者，参与者删除仅从自己日历移除 |
| [`dws-calendar-room-delete`](./room/delete.md) | `delete_meeting_room` | 移除日程中预约的会议室 |
| [`dws-calendar-event-get`](./event/get.md) | `get_calendar_detail` | 获取我的日历指定日程的详细信息 |
| [`dws-calendar-participant-list`](./participant/list.md) | `get_calendar_participants` | 获取指定日程的所有参与者列表及其状态信息 |
| [`dws-calendar-event-list`](./event/list.md) | `list_calendar_events` | 仅允许查询当前用户指定时间范围内的日程列表，最多返回100条 |
| [`dws-calendar-room-list-groups`](./room/list-groups.md) | `list_meeting_room_groups` | 分页查询当前企业下的会议室分组列表，返回每个分组的名称（groupName）、唯一 ID（groupId）及其父分组 ID（parentId，0 表示根分组）。结果按组织架构权限过滤，仅包含调用者有权限查看的分组。 |
| [`dws-calendar-room-search`](./room/search.md) | `query_available_meeting_room` | 根据时间筛选出符合闲忙条件的会议室列表。 |
| [`dws-calendar-busy-search`](./busy/search.md) | `query_busy_status` | 查询指定用户在给定时间范围内的闲忙状态，返回其日历中已占用时间段的详细日程信息（如标题、开始/结束时间），不包含具体日程内容细节（如参与人、地点），以保护隐私。结果受组织可见性策略控制：仅当调用者有权限查看该用户日历时方可获取有效数据。适用于安排会议前快速确认他人可用时间。 |
| [`dws-calendar-participant-delete`](./participant/delete.md) | `remove_calendar_participant` | 从已存在的指定日程中移除参与者，支持批量移除多人 |
| [`dws-calendar-event-update`](./event/update.md) | `update_calendar_event` | 修改现有日程的信息，支持更新标题、时间、地点等任意字段，需要组织者权限。（修改参与人需要使用给日程添加参与人或给日程删除参与人工具） |

## API Tools

### `add_calendar_participant`

- Canonical path: `calendar.add_calendar_participant`
- CLI route: `dws calendar participant add`
- Description: 向已存在的指定日程添加参与者，支持批量添加多人，可设置参与者类型和通知方式
- Required fields: `attendeesToAdd`, `eventId`
- Sensitive: `false`

### `add_meeting_room`

- Canonical path: `calendar.add_meeting_room`
- CLI route: `dws calendar room add`
- Description: 添加会议室
- Required fields: `eventId`, `roomIds`
- Sensitive: `false`

### `create_calendar_event`

- Canonical path: `calendar.create_calendar_event`
- CLI route: `dws calendar event create`
- Description: 创建新的日程，支持设置时间、参与者、提醒等完整功能
- Required fields: `endDateTime`, `startDateTime`, `summary`
- Sensitive: `false`

### `delete_calendar_event`

- Canonical path: `calendar.delete_calendar_event`
- CLI route: `dws calendar event delete`
- Description: 删除指定日程，组织者删除将通知所有参与者，参与者删除仅从自己日历移除
- Required fields: `eventId`
- Sensitive: `true`

### `delete_meeting_room`

- Canonical path: `calendar.delete_meeting_room`
- CLI route: `dws calendar room delete`
- Description: 移除日程中预约的会议室
- Required fields: `eventId`, `roomIds`
- Sensitive: `true`

### `get_calendar_detail`

- Canonical path: `calendar.get_calendar_detail`
- CLI route: `dws calendar event get`
- Description: 获取我的日历指定日程的详细信息
- Required fields: `eventId`
- Sensitive: `false`

### `get_calendar_participants`

- Canonical path: `calendar.get_calendar_participants`
- CLI route: `dws calendar participant list`
- Description: 获取指定日程的所有参与者列表及其状态信息
- Required fields: `eventId`
- Sensitive: `false`

### `list_calendar_events`

- Canonical path: `calendar.list_calendar_events`
- CLI route: `dws calendar event list`
- Description: 仅允许查询当前用户指定时间范围内的日程列表，最多返回100条
- Required fields: none
- Sensitive: `false`

### `list_meeting_room_groups`

- Canonical path: `calendar.list_meeting_room_groups`
- CLI route: `dws calendar room list-groups`
- Description: 分页查询当前企业下的会议室分组列表，返回每个分组的名称（groupName）、唯一 ID（groupId）及其父分组 ID（parentId，0 表示根分组）。结果按组织架构权限过滤，仅包含调用者有权限查看的分组。
- Required fields: none
- Sensitive: `false`

### `query_available_meeting_room`

- Canonical path: `calendar.query_available_meeting_room`
- CLI route: `dws calendar room search`
- Description: 根据时间筛选出符合闲忙条件的会议室列表。
- Required fields: `endTime`, `startTime`
- Sensitive: `false`

### `query_busy_status`

- Canonical path: `calendar.query_busy_status`
- CLI route: `dws calendar busy search`
- Description: 查询指定用户在给定时间范围内的闲忙状态，返回其日历中已占用时间段的详细日程信息（如标题、开始/结束时间），不包含具体日程内容细节（如参与人、地点），以保护隐私。结果受组织可见性策略控制：仅当调用者有权限查看该用户日历时方可获取有效数据。适用于安排会议前快速确认他人可用时间。
- Required fields: `endTime`, `startTime`, `userIds`
- Sensitive: `false`

### `remove_calendar_participant`

- Canonical path: `calendar.remove_calendar_participant`
- CLI route: `dws calendar participant delete`
- Description: 从已存在的指定日程中移除参与者，支持批量移除多人
- Required fields: `attendeesToRemove`, `eventId`
- Sensitive: `true`

### `update_calendar_event`

- Canonical path: `calendar.update_calendar_event`
- CLI route: `dws calendar event update`
- Description: 修改现有日程的信息，支持更新标题、时间、地点等任意字段，需要组织者权限。（修改参与人需要使用给日程添加参与人或给日程删除参与人工具）
- Required fields: `eventId`
- Sensitive: `false`

## Discovering Commands

```bash
dws schema                       # list available products (JSON)
dws schema calendar                     # inspect product tools (JSON)
dws schema calendar.<tool>              # inspect tool schema (JSON)
```
