# datasource — 数据源同步管理

将外部数据源（当前仅支持 OA 审批）同步到 AI 表格。完整链路：list-sources → (OA) 选择模板 → get-fields → create → sync-status → sync / update / get-config。

## 命令一览

| 命令 | 用途 | 读/写 |
|------|------|-------|
| `+datasource-list-sources` | 列出可用数据源条目，获取 processCode/name/iconUrl/url | 读 |
| `+datasource-get-fields` | 获取可同步字段列表，用于决定 field-ids | 读 |
| `+datasource-create` | 创建数据源表并触发首次全量同步 | 写 |
| `+datasource-update` | 更新已有数据源表的同步配置 | 写 |
| `+datasource-sync` | 手动触发一次同步（最多 5 张表） | 写 |
| `+datasource-sync-status` | 查询同步任务状态（RUNNING/FINISHED/FAILED） | 读 |
| `+datasource-get-config` | 获取数据源表当前同步配置 | 读 |

## 典型工作流

```
Step 0    列出可用来源         +datasource-list-sources --base-id <B> --datasource-type OA
                              → 返回 approvals 数组（通常含多个审批模板）

Step 0.5  (OA 审批) 选择模板  list-sources 返回的 approvals 数组通常含多个审批模板，需确定目标：
                              - 用户已指定名称（如"采购申请"）→ 按 name 精确/模糊匹配
                                · 唯一命中 → 提取该条的 processCode/name/iconUrl/url，继续
                                · 多候选 → 列出匹配项让用户消歧
                                · 零命中 → 停止，提示用户确认名称或从完整列表选择
                              - 用户未指定 → 列出候选模板清单（name + processCode），等用户选择
                              - 禁止：未匹配直接选第一项、或凭记忆猜 processCode
                              注：此步骤针对 OA 审批数据源（多模板场景）；其他数据源类型的
                              list-sources 返回结构可能不同，按实际 result 解析即可，
                              不一定需要选择步骤。

Step 1    (可选) 获取可同步字段  +datasource-get-fields --base-id <B> --datasource-type OA --source-config '<JSON>'
                              → 决定需要同步哪些字段，得到 field-ids

Step 2    创建数据源           +datasource-create --base-id <B> --datasource-type OA --source-config '<JSON>'
                              → sourceConfig 中的 processCode/name/iconUrl/url 来自 Step 0.5 选中的模板
                              → 返回 tableId + taskId

Step 3    查询同步结果         +datasource-sync-status --base-id <B> --table-id <T> --task-ids <TASK_ID>
                              → FINISHED=完成，FAILED=看 errorCode 排查，RUNNING=轮询

Step 4    (后续) 手动触发同步   +datasource-sync --base-id <B> --table-ids <T1>,<T2>
                              → 返回新 taskId，再用 sync-status 查结果

Step 5    (后续) 更新配置       +datasource-update --base-id <B> --table-id <T> --source-config '<JSON>' --auto
                              → 更新后自动触发一次同步

查看当前配置                  +datasource-get-config --base-id <B> --table-id <T>
```

## sourceConfig 字段协议

以下字段协议仅适用于 OA 审批数据源（datasourceType=OA），其他数据源类型待后续开放。

### 须从 list-sources 原样透传（必填）

| 字段 | 类型 | 说明 |
|------|------|------|
| processCode | String | OA 审批流程编码 |
| name | String | 展示名称 |
| iconUrl | String | 图标 URL |
| url | String | 跳转链接 |

### 调用方自行设置

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| dataType | String | 是 | 数据范围类型：`time_range` / `start_time` / `recent_time` |
| recentDays | String | 条件 | dataType=recent_time 时有效，取值 `7d`/`30d`/`1y`，默认 `30d` |
| startDate | String | 条件 | dataType=time_range 或 start_time 时有效，`yyyy-MM-dd`，默认 30 天前 |
| endDate | String | 条件 | dataType=time_range 时有效，`yyyy-MM-dd`，默认当天 |
| keepRemovedFields | Boolean | 否 | 是否保留已删除字段，默认 false |
| splitParentTableField | Boolean | 否 | 是否拆分父表字段 |

> list-sources 返回的 keepRemovedFields / splitParentTableField / enableDataSyncOaDetailList 不要透传，由调用方按需设置。

### 三种 dataType 最小示例

```json
// recent_time — 最近一段时间
{"processCode":"PROC-xxxx","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"...","url":"..."}

// time_range — 指定起止日期
{"processCode":"PROC-xxxx","name":"采购申请","dataType":"time_range","startDate":"2025-01-01","endDate":"2025-12-31","iconUrl":"...","url":"..."}

// start_time — 从某日期至今
{"processCode":"PROC-xxxx","name":"采购申请","dataType":"start_time","startDate":"2025-06-01","iconUrl":"...","url":"..."}
```

## autoSyncSetting 频率配置

仅在 `--auto=true` 时生效。不传时使用下游默认自动同步策略。

| 字段 | 必填 | 说明 |
|------|------|------|
| syncType | 是 | `hourly`（按小时间隔）/ `scheduled`（定时触发） |
| hourlyInterval | hourly 时 | 正整数，小时间隔 |
| scheduleType | scheduled 时 | `daily` / `weekly` / `monthly` |
| timeValue | scheduled 时 | `HH:mm` 触发时间 |
| selectedMonthDays | monthly 时必填 | 每月几号触发，1-31 |
| selectedWeekdays | weekly 时必填 | 每周哪几天触发，1=周一…7=周日 |
| skipNonWorkingDay | 否 | 是否跳过非工作日，默认 false |

示例：`{"syncType":"scheduled","scheduleType":"daily","timeValue":"09:00"}`

## 命令详情

### +datasource-list-sources — 列出可用数据源条目

```bash
dws aitable +datasource-list-sources --base-id BASE_ID --datasource-type OA --format json
```

| flag | 必填 | 说明 |
|------|------|------|
| `--base-id` | 是 | 目标 Base ID |
| `--datasource-type` | 是 | 数据源类型，当前仅支持 `OA` |

返回每条条目包含 `result`（下游原始 JSON 字符串）和 `sourceType`（OA 审批对应 2）。OA 审批场景下 result 为 approvals 数组：

```json
{
  "approvals": [
    {
      "processCode": "PROC-xxxx",
      "name": "采购申请",
      "iconUrl": "https://...",
      "url": "https://...",
      "keepRemovedFields": false,
      "splitParentTableField": false,
      "enableDataSyncOaDetailList": false
    }
  ]
}
```

调用方应自行解析 result，提取目标模板字段后构造 sourceConfig。

### +datasource-get-fields — 获取可同步字段列表

```bash
dws aitable +datasource-get-fields --base-id BASE_ID --datasource-type OA \
  --source-config '{"processCode":"PROC-xxxx","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"...","url":"..."}' \
  --format json
```

| flag | 必填 | 说明 |
|------|------|------|
| `--base-id` | 是 | 目标 Base ID |
| `--datasource-type` | 是 | 数据源类型，当前仅支持 `OA` |
| `--source-config` | 是 | 源配置 JSON 字符串，结构同 create 的 --source-config |

返回字段列表（字段 ID、名称、类型等），用于在 create/update 中指定 `--field-ids`。

### +datasource-create — 创建数据源表

```bash
dws aitable +datasource-create --base-id BASE_ID --datasource-type OA \
  --source-config '{"processCode":"PROC-xxxx","name":"采购申请","dataType":"recent_time","recentDays":"30d","iconUrl":"...","url":"..."}' \
  --format json

# 开启自动同步 + 自定义频率
dws aitable +datasource-create --base-id BASE_ID --datasource-type OA \
  --source-config '...' --auto \
  --auto-sync-setting '{"syncType":"scheduled","scheduleType":"daily","timeValue":"09:00"}' \
  --format json
```

| flag | 必填 | 说明 |
|------|------|------|
| `--base-id` | 是 | 目标 Base ID |
| `--datasource-type` | 是 | 数据源类型，当前仅支持 `OA` |
| `--source-config` | 是 | 源配置 JSON 字符串（见上方字段协议） |
| `--auto` | 否 | 是否开启自动同步，默认 false；无论是否传入，CLI 都会把该字段下发给下游 |
| `--auto-sync-setting` | 否 | 自动同步频率配置 JSON 字符串，仅 --auto=true 时生效 |
| `--field-ids` | 否 | 需要同步的字段 ID 列表，不传时同步全部字段 |

返回新建数据源表 tableId 和同步任务 taskId。创建后自动触发一次全量同步，需用 `+datasource-sync-status` 查最终结果。

### +datasource-update — 更新数据源配置

```bash
# 仅开启自动同步
dws aitable +datasource-update --base-id BASE_ID --table-id TABLE_ID --auto --format json

# 更新源配置
dws aitable +datasource-update --base-id BASE_ID --table-id TABLE_ID \
  --source-config '{"processCode":"PROC-yyyy","name":"出差申请","dataType":"recent_time","recentDays":"30d","iconUrl":"...","url":"..."}' \
  --format json
```

| flag | 必填 | 说明 |
|------|------|------|
| `--base-id` | 是 | 目标 Base ID |
| `--table-id` | 是 | 已有数据源表 ID（sync=true） |
| `--source-config` | 否 | 新的源配置 JSON 字符串，不传时保持原配置；传入时整体覆盖 |
| `--auto` | 否 | 是否开启自动同步，不传时保持原设置 |
| `--auto-sync-setting` | 否 | 自动同步频率配置 JSON 字符串，仅 --auto=true 时生效；不传时保持原频率配置 |
| `--field-ids` | 否 | 需要同步的字段 ID 列表，不传时同步全部字段 |

更新后自动触发一次全量同步，返回新 taskId。

### +datasource-sync — 手动触发同步

```bash
dws aitable +datasource-sync --base-id BASE_ID --table-ids TBL1,TBL2 --format json
```

| flag | 必填 | 说明 |
|------|------|------|
| `--base-id` | 是 | 目标 Base ID |
| `--table-ids` | 是 | 待同步的数据源表 ID 列表（sync=true），1-5 个 |

返回结果包含文档链接，可打开查看同步进度。每张表独立提交，部分失败不影响其他表。

### +datasource-sync-status — 按任务 ID 查询同步状态

```bash
dws aitable +datasource-sync-status --base-id BASE_ID --table-id TABLE_ID --task-ids TASK1,TASK2 --format json
```

| flag | 必填 | 说明 |
|------|------|------|
| `--base-id` | 是 | 目标 Base ID |
| `--table-id` | 是 | 数据源表 ID（sync=true） |
| `--task-ids` | 是 | 同步任务 ID 列表（由 create/update/sync 返回），1-5 个 |

任务状态：`RUNNING`（进行中）、`FINISHED`（完成）、`FAILED`（失败，含 errorCode + errorMessage）。

### +datasource-get-config — 获取数据源配置

```bash
dws aitable +datasource-get-config --base-id BASE_ID --table-id TABLE_ID --format json
```

| flag | 必填 | 说明 |
|------|------|------|
| `--base-id` | 是 | 目标 Base ID |
| `--table-id` | 是 | 数据源表 ID（sync=true） |

返回当前同步配置详情（sourceConfig、是否自动同步、同步状态等）。仅适用于数据源表，普通表会报错。

## 错误码与排查

| 场景 | 表现 | 排查 |
|------|------|------|
| 同步运行中重复触发 | errorCode=4014，status=FAILED | 幂等冲突，稍后重试即可 |
| 非数据源表触发 sync | 参数错误返回 | 确认 table 的 sync=true，用 `+base-get` / `+table-list` 检查 |
| sourceConfig 缺必填字段 | 创建/更新失败 | 检查 processCode/name/iconUrl/url 是否从 list-sources 原样透传 |
| dataType 与时间字段不匹配 | 创建失败 | recent_time 需 recentDays；time_range 需 startDate+endDate；start_time 需 startDate |

## 能力边界

| 能力 | 状态 |
|------|------|
| OA 审批数据源 | 已支持 |
| 其他数据源类型 | 待后续开放 |
| 全量同步 | 已支持 |
| 增量同步 | 待后续开放 |
| 自动同步 | 已支持（--auto + autoSyncSetting） |
| 删除数据源表 | 走普通表删除，不走 datasource 命令 |

## 注意事项

- sourceConfig 是 **JSON 字符串**（不是 JSON 对象），CLI flag 传入时需要用单引号包裹
- list-sources 返回的 keepRemovedFields / splitParentTableField / enableDataSyncOaDetailList 不要透传，由调用方按需设置
- create/update 后自动触发一次同步，返回 taskId；用 sync-status 查最终结果
- sync 单次最多 5 张表，超出拆分多次调用
- sync-status 单次最多 5 个 taskId，超出拆分多次调用
- get-config 仅适用于数据源表（sync=true），普通表会报错
