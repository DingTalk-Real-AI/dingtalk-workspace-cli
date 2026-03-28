# Canonical Product: report

Generated from shared Tool IR. Do not edit by hand.

- Display name: 钉钉日志
- Description: 钉钉日志MCP，包含获取日志模板、读取日志内容、写日志等功能
- Server key: `379b7411e5ab4e32`
- Endpoint: `https://mcp-gw.dingtalk.com/server/01d5a7b815babb03626bf3e505bad4c1e36ecf66876eaf6a7a466d9d5ccc9900`
- Protocol: `2025-03-26`
- Degraded: `false`

## Tools

- `create`
  - Path: `report.create_report`
  - CLI route: `dws report create`
  - Description: 创建日志
  - Flags: `--contents`, `--dd-from`, `--template-id`, `--to-chat`, `--to-user-ids`
  - Schema: `skills/generated/docs/schema/report/create_report.json`
- `template list`
  - Path: `report.get_available_report_templates`
  - CLI route: `dws report template list`
  - Description: 获取当前员工可使用的日志模版信息，包含日志模板的名称、模板Id等
  - Flags: none
  - Schema: `skills/generated/docs/schema/report/get_available_report_templates.json`
- `list`
  - Path: `report.get_received_report_list`
  - CLI route: `dws report list`
  - Description: 查询当前人收到的日志列表
  - Flags: `--cursor`, `--end`, `--size`, `--start`
  - Schema: `skills/generated/docs/schema/report/get_received_report_list.json`
- `detail`
  - Path: `report.get_report_entry_details`
  - CLI route: `dws report detail`
  - Description: 获取指定一篇日志的详情信息
  - Flags: `--report-id`
  - Schema: `skills/generated/docs/schema/report/get_report_entry_details.json`
- `stats`
  - Path: `report.get_report_statistics_by_id`
  - CLI route: `dws report stats`
  - Description: 获取日志统计数据，包括评论数量、点赞数量、已读数等
  - Flags: `--report-id`
  - Schema: `skills/generated/docs/schema/report/get_report_statistics_by_id.json`
- `sent`
  - Path: `report.get_send_report_list`
  - CLI route: `dws report sent`
  - Description: 查询当前人创建的日志详情列表，包含日志的内容、日志名称、创建时间等信息
  - Flags: `--cursor`, `--end`, `--modified-end`, `--modified-start`, `--template-name`, `--size`, `--start`
  - Schema: `skills/generated/docs/schema/report/get_send_report_list.json`
- `template detail`
  - Path: `report.get_template_details_by_name`
  - CLI route: `dws report template detail`
  - Description: 获取当前员工可使用的日志模版详情信息，包括日志模板Id、日志模板内字段的名称、字段类型、字段排序等
  - Flags: `--name`
  - Schema: `skills/generated/docs/schema/report/get_template_details_by_name.json`
