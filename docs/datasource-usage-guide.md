# AI 表格数据源指令使用指南

## 概述

dws 新增了 7 个 AI 表格数据源同步管理指令，用于将外部数据源（一期支持审批数据）接入 AI 表格，实现数据的自动同步。

所有指令均通过 `dws aitable +datasource-*` 前缀调用，操作对象是 AI 表格中的"数据源表"——一种由数据源同步创建的特殊数据表。

## 指令速览

| 指令 | 用途 | 读写 | 风险 |
|------|------|------|------|
| `+datasource-list-sources` | 列出数据源类型可用的来源信息（OA 返回 result/processCode、sourceType、sourceUrl） | 读 | low |
| `+datasource-get-fields` | 获取数据源来源的可同步字段结构 | 读 | low |
| `+datasource-create` | 创建数据源表并触发首次同步 | 写 | medium |
| `+datasource-update` | 更新已有数据源表的同步配置 | 写 | medium |
| `+datasource-sync` | 手动触发一次同步 | 写 | medium |
| `+datasource-sync-status` | 查询同步任务状态 | 读 | low |
| `+datasource-get-config` | 查看数据源表配置 | 读 | low |

## 前置条件

1. **登录认证**：执行 `dws auth login` 确保已登录
2. **获取 Base ID**：通过 `dws aitable +base-list` 或 `dws aitable +base-search --query "关键词"` 获取目标 AI 表格的 Base ID

---

## 1. 列出数据源可用来源

```
dws aitable +datasource-list-sources [flags]
```

列出指定数据源类型可用的来源信息。OA 审批类型返回当前 Base 可用的审批数据源条目（`sources` 数组，当前通常为单条），用于构造 `+datasource-create` / `+datasource-update` / `+datasource-get-fields` 的 `--source-config`。OA 场景下每条 source 的 `result` 字段是 JSON 字符串，需解析后得到 `approvals` 数组，再从中提取目标模板的 `processCode`、`name`、`iconUrl`、`url`。

### 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--base-id` | string | 是 | 目标 Base ID |
| `--datasource-type` | string | 是 | 数据源类型，目前支持审批（OA） |

### 示例

```bash
# 列出审批数据源来源，获取 result（JSON，解析后得到 approvals[].processCode）
dws aitable +datasource-list-sources \
  --base-id BASE123 \
  --datasource-type OA
```

### 返回值

返回 `sources` 数组，每个条目包含：

| 字段 | 说明 |
|------|------|
| `result` | OA 审批场景为 JSON 字符串，解析后得到 `approvals` 数组；每个 approval 含 `processCode`、`name`、`iconUrl`、`url` |
| `sourceType` | 数据源类型编号（OA 对应内部枚举值 2） |
| `sourceUrl` | 数据源访问链接，可选 |

`result` 本身不是 `processCode`，需要解析出 `approvals` 数组，再取目标模板的 `processCode`、`name`、`iconUrl`、`url` 原样填入 `--source-config`。

---

## 2. 获取数据源可同步字段

```
dws aitable +datasource-get-fields [flags]
```

获取指定数据源来源（如某个审批模板）的可同步字段列表，包括字段 ID、字段名称、字段类型和是否主键等信息。用于创建数据源前选择需要同步的字段。

### 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--base-id` | string | 是 | 目标 Base ID |
| `--datasource-type` | string | 是 | 数据源类型，目前支持审批（OA） |
| `--source-config` | string | 是 | 源配置 JSON 字符串，结构同 `+datasource-create` 的 `--source-config` |

### 示例

```bash
# 获取某审批模板的可同步字段
dws aitable +datasource-get-fields \
  --base-id BASE123 \
  --datasource-type OA \
  --source-config '{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}'
```

### 返回值

返回可同步字段列表，每个字段包含字段 ID、名称、类型和是否主键。字段 ID 可用于 `+datasource-create` / `+datasource-update` 的 `--field-ids` 参数。

---

## 3. 创建数据源表

```
dws aitable +datasource-create [flags]
```

为指定 AI 表格创建数据源同步配置，自动创建一张数据源表并触发首次全量同步。

### 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--base-id` | string | 是 | 目标 Base ID（通过 `+base-list` / `+base-search` 获取） |
| `--datasource-type` | string | 是 | 数据源类型，目前支持审批（OA） |
| `--source-config` | string | 是 | 源配置 JSON 字符串（格式见下方） |
| `--auto` | bool | 否 | 是否开启自动同步，默认 false；无论是否传入，CLI 都会把该字段下发给下游 |
| `--auto-sync-setting` | string | 否 | 自动同步频率配置 JSON 字符串，仅在 `--auto=true` 时生效，格式见下方 |
| `--field-ids` | stringSlice | 否 | 需要同步的字段 ID 列表，不传时同步全部字段 |

### source-config 格式（审批类）

审批数据源的 `--source-config` 是一个 JSON 对象字符串，包含以下字段：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `processCode` | string | 是 | 审批模板编码，对应 `+datasource-list-sources` 返回的 `result` |
| `name` | string | 是 | 数据源展示名称，须从 `+datasource-list-sources` 结果原样透传 |
| `iconUrl` | string | 是 | OA 审批图标 URL，须从 `+datasource-list-sources` 结果原样透传 |
| `url` | string | 是 | OA 审批跳转链接，须从 `+datasource-list-sources` 结果原样透传 |
| `dataType` | string | 是 | 数据时间范围类型：`time_range` / `start_time` / `recent_time` |
| `recentDays` | string | 当 dataType=recent_time 时必填 | 近 N 天：`7d` / `30d` / `1y` |
| `startDate` | string | 当 dataType=time_range 或 start_time 时必填 | 起始日期，格式 `yyyy-MM-dd` |
| `endDate` | string | 当 dataType=time_range 时必填 | 结束日期，格式 `yyyy-MM-dd` |
| `keepRemovedFields` | bool | 否 | 是否保留已删除字段，默认 false |

> 注：`splitParentTableField`、`enableDataSyncOaDetailList` 等字段为下游内部字段，无需传入，下游自动处理。

按 `dataType` 选择对应的时间参数组合：

| dataType | 需要的时间字段 | 说明 |
|----------|----------------|------|
| `recent_time` | `recentDays` | 同步近 N 天数据（7d/30d/1y） |
| `start_time` | `startDate` | 同步从某日期至今的数据 |
| `time_range` | `startDate` + `endDate` | 同步指定日期范围内的数据 |

### auto-sync-setting 格式

`--auto-sync-setting` 仅在 `--auto=true` 时生效，用于指定自动同步频率。不传时使用下游默认策略。

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `syncType` | string | 是 | `hourly`（按小时间隔）/ `scheduled`（定时触发） |
| `hourlyInterval` | int | hourly 时必填 | 正整数，小时间隔 |
| `scheduleType` | string | scheduled 时必填 | `daily` / `weekly` / `monthly` |
| `timeValue` | string | scheduled 时必填 | 触发时间，格式 `HH:mm` |
| `selectedMonthDays` | int[] | monthly 时必填 | 每月几号触发，1-31 |
| `selectedWeekdays` | int[] | weekly 时必填 | 每周哪几天触发，1=周一…7=周日 |
| `skipNonWorkingDay` | bool | 否 | 是否跳过非工作日，默认 false |

示例：`{"syncType":"scheduled","scheduleType":"daily","timeValue":"09:00"}`

### 示例

```bash
# 基本创建——同步近 30 天审批数据
dws aitable +datasource-create \
  --base-id BASE123 \
  --datasource-type OA \
  --source-config '{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}'

# 指定日期范围创建并开启自动同步
dws aitable +datasource-create \
  --base-id BASE123 \
  --datasource-type OA \
  --source-config '{"processCode":"PROC-XXXX","name":"采购申请","dataType":"time_range","startDate":"2025-01-01","endDate":"2025-12-31","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}' \
  --auto

# 指定同步字段（仅同步部分字段，field-ids 可通过 +datasource-get-fields 获取）
dws aitable +datasource-create \
  --base-id BASE123 \
  --datasource-type OA \
  --source-config '{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}' \
  --field-ids fldAAA,fldBBB,fldCCC
```

### 返回值

创建成功后返回新建数据源表 ID 和同步任务 ID，后续操作需要用到这两个 ID。

---

## 4. 更新数据源配置

```
dws aitable +datasource-update [flags]
```

更新已有数据源表的同步配置，支持更新源配置、自动同步开关和同步字段选择。更新后会自动触发一次同步。

### 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--base-id` | string | 是 | 目标 Base ID |
| `--table-id` | string | 是 | 已存在的数据源表 ID（由 `+datasource-create` 返回） |
| `--source-config` | string | 否 | 新的源配置 JSON 字符串，不传时保持原有配置。结构同 `+datasource-create` |
| `--auto` | bool | 否 | 是否开启自动同步；仅显式设置时下发给下游，省略时保持原有自动同步开关不变 |
| `--auto-sync-setting` | string | 否 | 自动同步频率配置 JSON 字符串，仅在显式设置 `--auto=true` 时生效；省略时保持原频率配置 |
| `--field-ids` | stringSlice | 否 | 需要同步的字段 ID 列表，不传时保持现有字段配置 |

### 示例

```bash
# 更换审批模板并调整时间范围
dws aitable +datasource-update \
  --base-id BASE123 \
  --table-id TBL456 \
  --source-config '{"processCode":"PROC-YYYY","name":"出差申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}'

# 开启自动同步
dws aitable +datasource-update \
  --base-id BASE123 \
  --table-id TBL456 \
  --auto

# 更新同步字段范围
dws aitable +datasource-update \
  --base-id BASE123 \
  --table-id TBL456 \
  --field-ids fldAAA,fldDDD
```

> 注意：`--table-id` 指向的是数据源表（由 `+datasource-create` 创建），不是普通数据表。

---

## 5. 触发手动同步

```
dws aitable +datasource-sync [flags]
```

对已有数据源表触发一次手动同步。单次最多 5 张表，每张表独立提交，部分失败不影响其他表。

### 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--base-id` | string | 是 | 目标 Base ID |
| `--table-ids` | stringSlice | 是 | 待触发同步的数据源表 ID 列表（1-5 个） |

### 示例

```bash
# 同步单张表
dws aitable +datasource-sync \
  --base-id BASE123 \
  --table-ids TBL1

# 批量同步多张表（逗号分隔，最多 5 个）
dws aitable +datasource-sync \
  --base-id BASE123 \
  --table-ids TBL1,TBL2,TBL3
```

### 返回值

返回每个表的同步任务 ID，可通过 `+datasource-sync-status` 查询最终结果。

---

## 6. 查询同步状态

```
dws aitable +datasource-sync-status [flags]
```

按任务 ID 查询数据源表的同步任务状态。与 `+datasource-sync` / `+datasource-create` / `+datasource-update` 配对使用——这些指令触发同步后返回任务 ID，本指令通过任务 ID 查询最终结果。

### 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--base-id` | string | 是 | 目标 Base ID |
| `--table-id` | string | 是 | 数据源表 ID |
| `--task-ids` | stringSlice | 是 | 待查询的同步任务 ID 列表（1-5 个） |

### 示例

```bash
# 按任务 ID 查询（批量，最多 5 个）
dws aitable +datasource-sync-status \
  --base-id BASE123 \
  --table-id TBL456 \
  --task-ids TASK1,TASK2
```

---

## 7. 获取数据源配置

```
dws aitable +datasource-get-config [flags]
```

获取指定数据源表的同步配置信息，包括源配置、同步模式、自动同步开关和同步状态。

### 参数

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `--base-id` | string | 是 | 目标 Base ID |
| `--table-id` | string | 是 | 数据源表 ID |

### 示例

```bash
dws aitable +datasource-get-config \
  --base-id BASE123 \
  --table-id TBL456
```

---

## 典型工作流

### 场景一：从零接入审批数据

```bash
# 0. 获取 Base ID
dws aitable +base-search --query "我的项目表"

# 1. 列出可用审批数据源来源，解析 result JSON 获取 approvals[].processCode
dws aitable +datasource-list-sources \
  --base-id BASE123 \
  --datasource-type OA
# → 返回 sources[0].result 为 JSON 字符串，解析后取 approvals[0].processCode=PROC-XXXX

# 2. 查看可同步字段（可选，用于指定 field-ids）
dws aitable +datasource-get-fields \
  --base-id BASE123 \
  --datasource-type OA \
  --source-config '{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}'

# 3. 创建数据源表（创建后自动触发首次同步）
dws aitable +datasource-create \
  --base-id BASE123 \
  --datasource-type OA \
  --source-config '{"processCode":"PROC-XXXX","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}'
# → 返回 tableId=TBL456, taskId=TASK001

# 4. 查询首次同步是否完成
dws aitable +datasource-sync-status \
  --base-id BASE123 \
  --table-id TBL456 \
  --task-ids TASK001

# 5. 确认配置
dws aitable +datasource-get-config \
  --base-id BASE123 \
  --table-id TBL456
```

### 场景二：更换审批模板后重新同步

```bash
# 1. 更新源配置（更新后自动触发一次同步）
dws aitable +datasource-update \
  --base-id BASE123 \
  --table-id TBL456 \
  --source-config '{"processCode":"PROC-NEW","name":"新审批模板","dataType":"recent_time","recentDays":"30d","iconUrl":"https://example.com/icon.png","url":"https://example.com/oa"}'

# 2. 查询同步状态（更新后会返回新的 taskId）
dws aitable +datasource-sync-status \
  --base-id BASE123 \
  --table-id TBL456 \
  --task-ids TASK002
```

### 场景三：手动触发日常同步

```bash
# 仅触发同步，不修改配置
dws aitable +datasource-sync \
  --base-id BASE123 \
  --table-ids TBL456

# 查询结果（sync 会返回 taskId）
dws aitable +datasource-sync-status \
  --base-id BASE123 \
  --table-id TBL456 \
  --task-ids TASK001
```

### 场景四：开启自动同步后确认

```bash
# 1. 更新配置，开启自动同步
dws aitable +datasource-update \
  --base-id BASE123 \
  --table-id TBL456 \
  --auto

# 2. 确认配置已更新
dws aitable +datasource-get-config \
  --base-id BASE123 \
  --table-id TBL456
# → 返回中应显示 auto=true
```

---

## 通用选项

以下全局选项可在所有指令中使用：

| 选项 | 说明 |
|------|------|
| `-f, --format` | 输出格式：json（默认）/ table / raw / pretty / ndjson / csv |
| `--jq` | jq 表达式过滤输出（如 `.tableId` 或 `.status`） |
| `--fields` | 筛选输出字段（逗号分隔） |
| `--dry-run` | 预览操作内容，不实际执行 |
| `--profile` | 指定组织或账号 |
| `--timeout` | HTTP 请求超时时间（秒，默认 30） |
| `--debug` | 显示调试日志 |
| `-v, --verbose` | 显示详细日志 |

### 输出过滤示例

```bash
# 只取 tableId
dws aitable +datasource-create ... --jq '.tableId'

# 只取同步状态
dws aitable +datasource-sync-status ... --jq '.status'

# table 格式查看
dws aitable +datasource-get-config ... -f table
```

---

## 注意事项

1. **推荐流程**：先 `+datasource-list-sources` 解析 `result` JSON 获取 `approvals[].processCode`，再 `+datasource-get-fields` 查看可同步字段，最后 `+datasource-create` 创建数据源表。

2. **数据源表 vs 普通数据表**：`+datasource-create` 创建的是"数据源表"，它由数据源同步驱动数据写入。`+datasource-update` 和 `+datasource-sync` 仅适用于数据源表，不可对普通数据表使用。

3. **datasource-type 透传**：CLI 层不对 `--datasource-type` 做枚举校验，目前一期仅支持 `OA`（审批）。后续支持其他类型时由服务端控制，CLI 无需修改。

4. **source-config 格式**：`--source-config` 必须是合法 JSON 字符串。审批数据源需要原样透传 `processCode`（从 `+datasource-list-sources` 返回的 `result` JSON 中解析 `approvals[]` 提取）、`name`、`iconUrl`、`url`，设置 `dataType`（时间范围类型），并按 `dataType` 提供对应的时间参数（`recentDays` / `startDate` / `endDate`）。

5. **同步限制**：`+datasource-sync` 单次最多 5 张表；`+datasource-sync-status` 单次最多查询 5 个任务 ID。

6. **创建即同步**：`+datasource-create` 和 `+datasource-update` 在操作完成后会自动触发一次同步，无需额外调用 `+datasource-sync`。

7. **自动同步**：`--auto` 开启后，数据源表会按 `--auto-sync-setting` 指定的频率自动定期同步；未指定频率时使用服务端默认策略。关闭 `--auto` 后仅能通过 `+datasource-sync` 手动触发。
