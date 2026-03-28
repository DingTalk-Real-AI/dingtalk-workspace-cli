---
name: dws-report
description: "钉钉日志MCP，包含获取日志模板、读取日志内容、写日志等功能."
metadata:
  version: 1.1.0
  openclaw:
    category: "productivity"
    requires:
      bins:
        - dws
    cliHelp: "dws report --help"
---

# report

> **PREREQUISITE:** Read `../dws-shared/SKILL.md` for auth, command patterns, and security rules. If missing, run `dws generate-skills` to create it.

- Display name: 钉钉日志
- Description: 钉钉日志MCP，包含获取日志模板、读取日志内容、写日志等功能
- Endpoint: `https://mcp-gw.dingtalk.com/server/01d5a7b815babb03626bf3e505bad4c1e36ecf66876eaf6a7a466d9d5ccc9900`
- Protocol: `2025-03-26`
- Degraded: `false`

```bash
dws report <group> <command> --json '{...}'
```

## Helper Commands

| Command | Tool | Description |
|---------|------|-------------|
| [`dws-report-create`](./create.md) | `create_report` | 创建日志 |
| [`dws-report-template-list`](./template/list.md) | `get_available_report_templates` | 获取当前员工可使用的日志模版信息，包含日志模板的名称、模板Id等 |
| [`dws-report-list`](./list.md) | `get_received_report_list` | 查询当前人收到的日志列表 |
| [`dws-report-detail`](./detail.md) | `get_report_entry_details` | 获取指定一篇日志的详情信息 |
| [`dws-report-stats`](./stats.md) | `get_report_statistics_by_id` | 获取日志统计数据，包括评论数量、点赞数量、已读数等 |
| [`dws-report-sent`](./sent.md) | `get_send_report_list` | 查询当前人创建的日志详情列表，包含日志的内容、日志名称、创建时间等信息 |
| [`dws-report-template-detail`](./template/detail.md) | `get_template_details_by_name` | 获取当前员工可使用的日志模版详情信息，包括日志模板Id、日志模板内字段的名称、字段类型、字段排序等 |

## API Tools

### `create_report`

- Canonical path: `report.create_report`
- CLI route: `dws report create`
- Description: 创建日志
- Required fields: `contents`, `ddFrom`, `templateId`, `toChat`
- Sensitive: `false`

### `get_available_report_templates`

- Canonical path: `report.get_available_report_templates`
- CLI route: `dws report template list`
- Description: 获取当前员工可使用的日志模版信息，包含日志模板的名称、模板Id等
- Required fields: none
- Sensitive: `false`

### `get_received_report_list`

- Canonical path: `report.get_received_report_list`
- CLI route: `dws report list`
- Description: 查询当前人收到的日志列表
- Required fields: `cursor`, `endTime`, `size`, `startTime`
- Sensitive: `false`

### `get_report_entry_details`

- Canonical path: `report.get_report_entry_details`
- CLI route: `dws report detail`
- Description: 获取指定一篇日志的详情信息
- Required fields: `report_id`
- Sensitive: `false`

### `get_report_statistics_by_id`

- Canonical path: `report.get_report_statistics_by_id`
- CLI route: `dws report stats`
- Description: 获取日志统计数据，包括评论数量、点赞数量、已读数等
- Required fields: `report_id`
- Sensitive: `false`

### `get_send_report_list`

- Canonical path: `report.get_send_report_list`
- CLI route: `dws report sent`
- Description: 查询当前人创建的日志详情列表，包含日志的内容、日志名称、创建时间等信息
- Required fields: `cursor`, `size`
- Sensitive: `false`

### `get_template_details_by_name`

- Canonical path: `report.get_template_details_by_name`
- CLI route: `dws report template detail`
- Description: 获取当前员工可使用的日志模版详情信息，包括日志模板Id、日志模板内字段的名称、字段类型、字段排序等
- Required fields: `report_template_name`
- Sensitive: `false`

## Discovering Commands

```bash
dws schema                       # list available products (JSON)
dws schema report                     # inspect product tools (JSON)
dws schema report.<tool>              # inspect tool schema (JSON)
```
