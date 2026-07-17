# 命令总览

## 命令总览


### 查询打卡结果
```
Usage:
  dws attendance check result [flags]
Example:
  dws attendance check result --users userId1,userId2 --start 2026-04-01 --end 2026-04-30 --limit 50
Flags:
      --start string  起始日期, 格式 YYYY-MM-DD (必填)
      --end string    结束日期, 格式 YYYY-MM-DD, 不超过 1 个月 (必填)
      --limit int     分页大小, 默认 100, 范围 1-1000 (可选)
      --offset int    分页偏移量, 默认 0 (可选)
      --users string  用户 ID 列表, 逗号分隔, 最多 100 个 (必填)
```

返回每条记录含：用户 ID、工作日期、时间结果（Normal/Late/Early/Absenteeism/NotSigned）、位置结果、计划打卡时间、实际打卡时间、打卡流水 ID。时间跨度不超过 1 个月，最多 100 人。

### 查询打卡流水
```
Usage:
  dws attendance check record [flags]
Example:
  dws attendance check record --users userId1 --start 2026-04-01 --end 2026-04-30
Flags:
      --start string  起始日期, 格式 YYYY-MM-DD (必填)
      --end string    结束日期, 格式 YYYY-MM-DD, 不超过 1 个月 (必填)
      --users string  用户 ID 列表, 逗号分隔 (必填)
```

返回每条记录含：用户 ID、实际打卡时间、打卡地址、打卡经纬度、打卡类型（OnDuty/OffDuty）、定位方式（Map/Wifi/etc）。时间跨度不超过 1 个月。

### 查询审批单（补卡/加班/请假/出差外出）
```
Usage:
  dws attendance approve list [flags]
Example:
  dws attendance approve list --users userId1 --types overtime,leave --start 2026-04-01 --end 2026-04-30
  dws attendance approve list --users userId1 --types trip --start 2026-04-01 --end 2026-04-30      # 同时返回出差与外出
  dws attendance approve list --users userId1 --types 加班,请假,补卡 --start 2026-04-01 --end 2026-04-30
Flags:
      --start string  起始日期, 格式 YYYY-MM-DD (必填)
      --end string    结束日期, 格式 YYYY-MM-DD (必填)
      --types string  审批类型, 逗号分隔: overtime/加班、trip/travel/business_trip/出差/外出、leave/请假、patch/repair-check/补卡 (必填)
      --users string  用户 ID 列表, 逗号分隔 (必填)
```

审批类型映射（关键词 → bizType）：
- `overtime` / `加班` → `1`
- `trip` / `travel` / `business_trip` / `business-trip` / `出差` / `外出` → `2`（**服务端查询接口 bizType=2 同时覆盖出差与外出，两者合并为同一类、不再细分**；传入任一别名都会返回这两类记录）
- `leave` / `请假` → `3`
- `patch` / `repair-check` / `repair_check` / `补卡` → `4`

> 查询不区分外出与出差，如果需要在提交入口区分外出（`TRAVEL`）与出差（`OUT`），请改用 `dws attendance approve templates --type travel|out`。

返回每条记录含：用户 ID、审批标签、审批子类型、审批类型、生效时间、时长、时长单位、流程实例 ID。

### 查询补卡/请假/加班/外出/出差审批提交链接 (必须走引导流程)
```
Usage:
  dws attendance approve templates [flags]
Example:
  dws attendance approve templates --type leave
  dws attendance approve templates --type REPAIR_CHECK
  dws attendance approve templates --type 加班
  dws attendance approve templates --type travel    # 外出，等价 --type TRAVEL
  dws attendance approve templates --type 出差       # 出差，等价 --type OUT
Flags:
      --type string      审批类型：repair-check/patch/补卡、leave/请假、overtime/加班、travel/外出、out/trip/出差，或 REPAIR_CHECK/LEAVE/OVERTIME/TRAVEL/OUT（必填）
```

当用户提到需要提交补卡、请假、加班、外出或出差时，优先使用该命令查询考勤审批表单模板提交链接，并引导用户点击返回的 `submitUrl` 提交。
`corpId` 和 `opUserId` 由系统参数自动注入，无需通过命令参数传入。
审批类型映射：补卡=`REPAIR_CHECK`，请假=`LEAVE`，加班=`OVERTIME`，外出=`TRAVEL`，出差=`OUT`（`trip` / `business_trip` / `business-trip` 亦映射为 `OUT`）。返回结果为列表，每条记录包含 `approveType`、`formName`、`processCode`、`submitUrl`。
#### 引导用户自主选择合适的表单模板流程
如果返回多个表单模板，必须将多个可用模板都返回给用户，并引导用户根据实际场景自主选择合适的模板提交：
- 请假场景：可根据 `formName` 将与用户请假类型更匹配的模板放在前面展示。例如用户明确说年假、事假、病假、调休时，将名称中包含对应假期类型的模板靠前；如果用户只泛化表达“请假”，将名称最通用的请假模板靠前，例如“请假”“员工请假”“通用请假”等，避免把专项或特殊场景模板放在最前。
- 补卡/加班场景：可将名称与“补卡”或“加班”最直接匹配的模板放在前面展示。
- 回复用户时不要直接裸露任何 `submitUrl`，所有返回的表单模板都必须使用 Markdown 可点击链接格式展示：`[formName](submitUrl)`，例如 `[员工请假](https://...)`。如存在更匹配的模板，可以放在列表前面，但不要只返回推荐模板，必须同时返回其它可用模板供用户选择，且每个模板都应是用户可直接点击的 Markdown 链接。

### 导入排班记录（排班 = 为员工安排工作日期和班次, 写场景接口，必须走二次确认流程）
```
Usage:
  dws attendance schedule import [flags]
Example:
  dws attendance schedule import --groupId 123456 \
    --scheduleVOS '[{"userId":"user001","classId":123,"workDate":"2026-04-22","checkBeginTime":"09:00","checkEndTime":"18:00"}]' \
    --user-say-yes
Flags:
      --groupId string       考勤组（必填，传入考勤组ID）
      --scheduleVOS string   排班记录 JSON 数组（必填）
      --user-say-yes         用户已确认，跳过交互式确认提示
```

为排班制考勤组导入排班记录。`--scheduleVOS` 为 JSON 数组，每条记录包含：
- `userId`: 员工ID
- `classId`: 班次ID
- `workDate`: 工作日期（YYYY-MM-DD），如 2026-04-22
- `checkBeginTime`: 开始打卡时间
- `checkEndTime`: 结束打卡时间
- `isRest`: 是否休息日 Y/N（可选）

#### AI 调用 `schedule import` 的二次确认流程

`schedule import` 是写操作，会为考勤组导入或变更员工排班。AI 调用时必须按以下流程执行，不得在未确认的情况下直接导入：

1. **识别写操作**：用户表达“导入排班 / 设置排班 / 安排排班 / 给员工排班 / 批量排班”等意图时，命中 `schedule import`。
2. **收集必要参数**：必须明确 `--groupId` 和 `--scheduleVOS`，并确认排班记录中的 `userId`、`classId`、`workDate`、`checkBeginTime`、`checkEndTime`、`isRest` 等字段。
3. **展示导入摘要并反问确认**：向用户展示考勤组 ID、导入员工数量、涉及日期范围、班次 ID 列表，以及排班记录明细摘要，并询问是否确认执行导入。
4. **用户确认后再执行导入**：只有用户明确确认后，才可以执行 `dws attendance schedule import ... --format json`。

确认话术示例：

```text
即将导入排班记录，请确认：
- 考勤组 ID：<GROUP_ID>
- 员工数量：<USER_COUNT>
- 日期范围：<START_DATE> ~ <END_DATE>
- 班次 ID：<CLASS_IDS>
- 排班明细：
  - <USER_ID>：<WORK_DATE> <CHECK_BEGIN_TIME>-<CHECK_END_TIME>，班次 <CLASS_ID>

是否确认执行导入？
```

如用户明确要求跳过确认，或命令中明确包含全局 `--yes`，可跳过二次确认。

### 获取排班记录

**禁止直接调用 `dws attendance schedule get`。必须先 `read_file` 读取 [attendance-schedule.md](./attendance-schedule.md) 后按其中的「排班查询导出工作流」执行。**

- 任何"查询排班"、"查看排班"、"XX考勤组的排班"、"X月份的排班"场景，**一律走 attendance-schedule.md 工作流**，由脚本 `attendance_schedule_export.py` 统一处理
- 脚本自动处理：分批查询（超 20 人自动分批）、userId→姓名转换、classId→班次名称转换、排班表格式 Excel 输出
- 违反后果：人数多时接口超时/报错、输出裸 userId 和 classId 用户看不懂、无排班表格式
- **此处不提供 `schedule get` 的 Usage/Flags，防止绕过工作流直接拼命令。完整参数由 attendance-schedule.md 工作流中的脚本内部使用。**

### 查询当前用户可管理的所有班次详情
```
Usage:
  dws attendance class search [flags]
Example:
  dws attendance class search
  dws attendance class search --query "早班" --filter-type MINE_OWN
  dws attendance class search --page 1 --limit 50
Flags:
      --filter-type string   班次类型: ALL 全部班次 / MINE_OWN 我负责的 (可选)
      --query string         班次名称关键字, 模糊搜索 (可选)
      --page int             页码, 从 1 开始 (可选, 默认 1)
      --limit int            每页条数, 最大 200 (可选, 默认 20)
```

### 查询班次详情
```
Usage:
  dws attendance class get [flags]
Example:
  dws attendance class get --class-id 1170996821
Flags:
      --class-id int   班次 ID (必填)
```

根据班次 ID 查询该班次的完整详细信息。班次 ID 可从 `class search` 返回结果中提取，也有可能来源于用户手动输入。

### 创建班次 (写场景接口，必须走二次确认流程)
**强制执行流程**：此命令为写操作，Agent 调用时必须遵守以下流程：
1. 先向用户展示待执行操作的完整参数摘要，包括班次名称、上下班时间、休息时段等
2. 展示变更摘要并等待用户明确确认
3. 用户确认后，再传全局 `--yes` 执行命令

**禁止未经用户确认直接执行或自动添加 `--yes`。**
```
Usage:
  dws attendance class create [flags]
Example:
  dws attendance class create --name "早班" --class-vo '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"08:00","across":0},{"checkType":"OffDuty","checkTime":"17:00","across":0}]}]}' --timeout 10
  # 带休息时段（12:00-13:00 午休）
  dws attendance class create --name "测试CLI" --class-vo '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"09:00","across":0},{"checkType":"OffDuty","checkTime":"18:00","across":0}]}],"setting":{"topRestTimeList":[{"checkType":"OnDuty","checkTime":"12:00","across":0},{"checkType":"OffDuty","checkTime":"13:00","across":0}]}}' --timeout 10
Flags:
      --name string      班次名称 (必填)
      --owner string     班次负责人 userId (可选)
      --class-vo string  完整 TopAtClassVO JSON 字符串, 包含 sections 等复杂子对象 (必填)
```

创建一个新班次。`--name` 和 `--class-vo`（包含 `sections`）必填。`sections` 定义班次的上下班时间段，支持多段上下班，每段包含 `times` 数组（有且只能有两个对象：上班+下班）。由于保存班次耗时较久，建议加 `--timeout 10`。

`checkTime` 字段统一使用 "HH:mm" 格式（如 "09:00"、"17:30"），CLI 自动转换为服务端所需格式。

`--class-vo` 支持字段：
- `name`(string, 必填) `owner`(string, 可选)
- `sections`([]object, 必填): 每个对象含 `times`([]object)，每个 time 含 `checkType`(OnDuty/OffDuty, 必填) `checkTime`("HH:mm", 必填) `across`(0/1, 必填) `freeCheck`(bool) `beginMin`(number, -1不限制) `endMin`(number, -1不限制)
- `setting`(object, 可选): `seriousLateMinutes`(严重迟到分钟) `absenteeismLateMinutes`(旷工迟到分钟) `attendDays`(出勤天数) `topRestTimeList`([]object, 仅单段上下班时可用，最多3段: checkType/checkTime("HH:mm")/across)

### 更新班次 (写场景接口，必须走二次确认流程)
**强制执行流程**：此命令为写操作，Agent 调用时必须遵守以下流程：
1. 先向用户展示待执行操作的完整参数摘要，包括班次 ID、要修改的字段含义及新值
2. 展示变更摘要并等待用户明确确认
3. 用户确认后，再传全局 `--yes` 执行命令

**禁止未经用户确认直接执行或自动添加 `--yes`。**
```
Usage:
  dws attendance class update [flags]
Example:
  dws attendance class update --class-id 1170996821 --name "新早班" --timeout 10
  dws attendance class update --class-id 1170996821 --class-vo '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"08:30","across":0},{"checkType":"OffDuty","checkTime":"17:30","across":0}]}]}' --timeout 10
  # 带休息时段（12:00-13:00 午休）
  dws attendance class update --class-id 1170996821 --class-vo '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"09:00","across":0},{"checkType":"OffDuty","checkTime":"18:00","across":0}]}],"setting":{"topRestTimeList":[{"checkType":"OnDuty","checkTime":"12:00","across":0},{"checkType":"OffDuty","checkTime":"13:00","across":0}]}}' --timeout 10
Flags:
      --class-id int     班次 ID (必填)
      --name string      班次名称 (可选，不传则保持原值)
      --owner string     班次负责人 userId (可选，不传则保持原值)
      --class-vo string  完整 TopAtClassVO JSON 字符串，用于修改复杂子对象 (可选)
```

更新班次配置。`--class-id` 必填，其余均可选，仅需对要修改的字段进行赋值，未传字段会自动从已有配置补充。小改用单字段 flag（如 `--name`）；修改上下班时间、休息时段等复杂子对象时用 `--class-vo` 传入完整 JSON。`--class-vo` 与单字段 flag 同时传入时，单字段 flag 优先级更高。

`checkTime` 字段统一使用 "HH:mm" 格式（如 "09:00"、"17:30"），CLI 自动转换为服务端所需格式。

`--class-vo` 支持字段（均可选，只需包含要修改的字段）：
- `name`(string) `owner`(string)
- `sections`([]object): 每个对象含 `times`([]object)，每个 time 含 `checkType`(OnDuty/OffDuty) `checkTime`("HH:mm") `across`(0/1) `freeCheck`(bool) `beginMin`(number, -1不限制) `endMin`(number, -1不限制)
- `setting`(object): `seriousLateMinutes` `absenteeismLateMinutes` `attendDays` `topRestTimeList`([]object: checkType/checkTime("HH:mm")/across)

由于保存班次耗时较久，建议加 `--timeout 10`。

### 分页查询补卡规则，支持按名称搜素
```
Usage:
  dws attendance adjustment search [flags]
Example:
  dws attendance adjustment search --page 1 --limit 20
  dws attendance adjustment search --query "标准" --page 1 --limit 50
Flags:
      --page int     页码, 从 1 开始 (必填, 默认 1)
      --query string 补卡规则名称关键字, 模糊搜索 (可选)
      --limit int    每页条数, 200 以内 (必填, 默认 20)
```

### 查询补卡规则详情
```
Usage:
  dws attendance adjustment get [flags]
Example:
  dws attendance adjustment get --adjustment-id 12345
Flags:
      --adjustment-id int   补卡规则主键 ID (必填)
```

根据补卡规则主键 ID 查询对应的补卡规则详情。主键 ID 可从 `adjustment search` 返回结果中提取，也有可能来源于用户手动输入。**注意：已被删除或被更新覆盖的补卡规则无法查询到。**

### 分页查询加班规则，支持按名称搜素
```
Usage:
  dws attendance overtime search [flags]
Example:
  dws attendance overtime search --page 1 --limit 20
  dws attendance overtime search --query "节假日" --page 1 --limit 50
Flags:
      --page int     页码, 从 1 开始 (必填, 默认 1)
      --query string 加班规则名称关键字, 模糊搜索 (可选)
      --limit int    每页条数, 200 以内 (必填, 默认 20)
```

### 查询加班规则详情
```
Usage:
  dws attendance overtime get [flags]
Example:
  dws attendance overtime get --overtime-id 12345
Flags:
      --overtime-id int   加班规则主键 ID (必填)
```

根据加班规则主键 ID 查询对应的加班规则详情。主键 ID 可从 `overtime search` 返回结果中提取，也有可能来源于用户手动输入。**已被删除或更新覆盖的加班规则也可以查到。**

### 查询考勤组列表
```
Usage:
  dws attendance group search [flags]
Example:
  dws attendance group search --query "研发"
  dws attendance group search --type FIXED --limit 50
  dws attendance group search --page 1 --limit 20
Flags:
      --query string         考勤组名称关键字, 模糊搜索 (可选)
      --page int             页码, 从 1 开始 (必填, 默认 1)
      --limit int            每页条数, 200 以内 (必填, 默认 20)
      --query-ble            是否查询蓝牙设备列表 (可选, 默认 false)
      --query-position       是否查询地理定位和 Wifi 名称 (可选, 默认 false)
      --type string          考勤组类型: FIXED 固定班制 / TURN 排班制 / NONE 自由工时 (可选)
```

### 查询考勤组全量信息
```
Usage:
  dws attendance group get [flags]
Example:
  dws attendance group get --group-id 123456
Flags:
      --group-id int   考勤组 ID (必填)
```

根据考勤组 ID 查询该考勤组的全量信息。考勤组 ID 可从 `group search` 返回结果中提取，也有可能来源于用户手动输入。如果只需查询成员、打卡地址、蓝牙、Wifi 子集，请使用 `group filtered-get` 以节省查询成本。
返回结果中如含成员 userId 列表，必须调用 `dws contact user get --ids <userId1>,<userId2>,...`（支持逗号分隔传多个 ID），将 userId 转换为员工姓名后再输出；不得直接输出裸 userId

### 按需查询考勤组部分信息
```
Usage:
  dws attendance group filtered-get [flags]
Example:
  dws attendance group filtered-get --group-id 123456 --member
  dws attendance group filtered-get --group-id 123456 --position --wifi
Flags:
      --group-id int     考勤组 ID (必填)
      --member           是否查询考勤组成员信息 (可选, 默认 false)
      --position         是否查询打卡地址 (可选, 默认 false)
      --wifi             是否查询打卡 Wifi (可选, 默认 false)
      --bles             是否查询打卡蓝牙 (可选, 默认 false)
```

强烈建议在仅需查询成员、打卡地址、蓝牙、Wifi 时调用该命令，避免全量查询带来的性能开销。考勤组 ID 可从 `group search` 返回结果中提取，也有可能来源于用户手动输入。
返回结果中如含成员 userId 列表，必须调用 `dws contact user get --ids <userId1>,<userId2>,...`（支持逗号分隔传多个 ID），将 userId 转换为员工姓名后再输出；不得直接输出裸 userId

### 更新考勤组成员 (写场景接口，必须走二次确认流程)
**强制执行流程**：此命令为写操作，Agent 调用时必须遵守以下流程：
1. 先向用户展示待执行操作的完整参数摘要，包括考勤组 ID、要添加/移除的成员列表
2. 展示变更摘要并等待用户明确确认
3. 用户确认后，再传全局 `--yes` 执行命令

**禁止未经用户确认直接执行或自动添加 `--yes`。**

```
Usage:
  dws attendance group update-members [flags]
Example:
  dws attendance group update-members --group-id 123456 --add-users userId1,userId2
  dws attendance group update-members --group-id 123456 --remove-users userId1
  dws attendance group update-members --group-id 123456 --add-depts deptId1 --remove-users userId2
Flags:
      --group-id int              考勤组 ID (必填)
      --add-users string          添加考勤人员 userId 列表, 逗号分隔, 最多 20 个 (可选)
      --remove-users string       删除考勤人员 userId 列表, 逗号分隔, 最多 20 个 (可选)
      --add-extra-users string    添加无需考勤的人员 userId 列表, 逗号分隔, 最多 20 个 (可选)
      --remove-extra-users string 删除无需考勤的成员 userId 列表, 逗号分隔, 最多 20 个 (可选)
      --add-depts string          添加考勤部门 ID 列表, 逗号分隔, 最多 20 个 (可选)，若要添加全公司，根部门id为-1
      --remove-depts string       删除考勤部门 ID 列表, 逗号分隔, 最多 20 个 (可选)，全公司根部门id为-1
```

对指定考勤组的成员进行增删操作。--group-id 必填，其余参数均为可选，但至少需要传入一个变更项，否则命令拒绝执行。每次调用各参数最多传 20 个 ID。"无需考勤"人员指考勤组内豁免打卡的成员（如高管）。

### 更新考勤组配置 (写场景接口，必须走二次确认流程)
**强制执行流程**：此命令为写操作，Agent 调用时必须遵守以下流程：
1. 先向用户展示待执行操作的完整参数摘要，包括考勤组 ID、要修改的字段含义及新值
2. 展示变更摘要并等待用户明确确认
3. 用户确认后，再传全局 `--yes` 执行命令

**禁止未经用户确认直接执行或自动添加 `--yes`。**
```
Usage:
  dws attendance group update [flags]
Example:
  dws attendance group update --group-id 123456 --name "研发考勤组" --timeout 10
  dws attendance group update --group-id 123456 --owner userId1 --timeout 10
  dws attendance group update --group-id 123456 --classIds '[1374234767]' --timeout 10
  dws attendance group update --group-id 123456 --group-vo '{"positions":[{"title":"总部","address":"北京市...","latitude":39.9,"longitude":116.4,"offset":200}]}' --timeout 10
Flags:
      --group-id int               考勤组 ID (必填)
      --name string                考勤组名称 (可选)
      --type string                考勤组类型：FIXED 固定班制 / TURN 排班制 / NONE 自由工时 (可选)
      --owner string               考勤组主负责人 userId (可选)
      --enable-outside-check       是否允许外勤打卡 true/false (可选)
      --classIds string            所选班次 id 列表, JSON 数组格式, 如 '[123,456]' (可选)
      --group-vo string            完整 groupVO JSON 字符串, 用于修改复杂子对象 (可选)
```

更新考勤组配置。--group-id 必填，其余均可选，但至少需指定一个修改项。仅需对要修改的字段进行赋値，其余字段会自动从已有配置补充。小改用单字段 flag；修改打卡地址、wifi、蓝牙设备、循环排班等复杂子对象时用 `--group-vo` 传入完整 JSON。`--group-vo` 与单字段 flag 同时传入时，单字段 flag 优先级更高。

`--group-vo` 支持字段（均可选，只需包含要修改的字段）：
- 基础：`name`(名称) `type`(FIXED/TURN/NONE) `owner`(主负责人 userId) `managerList`([]string 子负责人) `skipHolidays`(bool，只在固定班制和自由工时生效) `defaultGroup`(bool) `classIds`([]number，所选班次 id，只有固定班制和排班制才有班次，自由工时没有)
- 打卡范围：`trimDistance`(微调距离) `enablePositionOfGps/Wifi/Ble`(bool)
- 打卡地址：`positions`([]对象: title/address/latitude/longitude/offset，其中 offset 为该地址允许的打卡范围米)
- Wifi：`wifis`([]对象: ssid/macAddr/groupId)
- 蓝牙：`bleDeviceVOList`([]对象: name/deviceUid/sn/productType/devServId)
- 外勤：`enableOutsideCheck`(bool) `enableOutsideCameraCheck/Remark/Apply`(bool) `outsideCheckApproveMode`(NO_NEED_APPROVE/APPROVE_FIRST/CHECK_FIRST/APPROVE_EVERYTIME) `outSideCheckApplyType`(1全天/2上班/3下班) `forbidHideOutSideAddress`(bool) `enableOutSideUpdateNormalCheck`(下班时允许外勤卡更新内勤卡) `enableOnDutyNormalUpdateOutsideCheck`(上班时允许内勤卡更新外勤卡)
- 打卡方式：`enableCameraCheck/openCameraCheck` `openFaceCheck` `enableFaceStrictMode` `enableFaceBeauty`(bool) `onlyMachineCheck`(bool) `permitMaxBeaconCount`(number) `disableCheckWhenRest`(bool，休息日打卡需审批，只在固定班制和排班制生效)
- 固定班制设置（FIXED）：`defaultClassId`(number) `workDayClassList`([]number，共7个值代表周日到周六每天的班次id，为0表示当天休息，如[0,1279240003,0,0,0,0,0]表示只有周一上班)
- 排班制设置（TURN）：`disableCheckWithoutSchedule`(bool，true=未排班时禁止打卡) `enableEmpSelectClass`(未排班时员工可选班次) `enableScheduleAutoMatch`(未排班时系统自动匹配)
  - 循环排班（非必填，不设置则由管理员手动排班）：`cycleDays`(number) `startCycleDate`(时间戳，毫秒) `cycleScheduleList`([]对象: cycleName/groupId/isValid(Y/N)/itemList[{classId/className/isValid}])
- 自由工时设置（NONE）：`workDays`([1-7]，1=周一7=周日) `freeCheckDayStartMinOffset`(number，距0点分钟数) `freeCheckCoreTime`(最短工作时长，分钟) `freeCheckDemandWorkMinutes`(要求打卡时长，分钟) `freeCheckSettingVO`(对象: freeCheckType(CYCLE上下班交替/MAX_TIME_UPDATE最大时间打卡)/freeWorkDayLackSwitch/freeOnDutyLackMinOffset/freeOffDutyLackMinOffset/delimitOffsetMinutesBetweenDays/freeCheckGapVO{onOffCheckGapMinutes/offOnCheckGapMinutes}) `freeGroupSpecialDayVO`(对象: specialOnDutyDays[]/specialOffDutyDays[])

### 创建考勤组 (写场景接口，必须走二次确认流程)
**强制执行流程**：此命令为写操作，Agent 调用时必须遵守以下流程：
1. 先向用户展示待执行操作的完整参数摘要，包括考勤组名称、类型、班次列表等
2. 展示变更摘要并等待用户明确确认
3. 用户确认后，再传全局 `--yes` 执行命令

**禁止未经用户确认直接执行或自动添加 `--yes`。**
```
Usage:
  dws attendance group create [flags]
Example:
  dws attendance group create --name "研发考勤组" --type FIXED --group-vo '{"defaultClassId":1170996821,"workDayClassList":[0,1170996821,0,0,0,0,0]}' --timeout 10
  dws attendance group create --name "自由工时分组" --type NONE --timeout 10
Flags:
      --name string      考勤组名称 (必填)
      --type string      考勤组类型：FIXED 固定班制 / TURN 排班制 / NONE 自由工时 (必填)
      --owner string     考勤组主负责人 userId (可选)
      --group-vo string  完整 groupVO JSON 字符串，用于传入复杂子对象 (可选)
```

创建一个新的考勤组。`--name` 和 `--type` 必填，`--type` 必须为 FIXED/TURN/NONE 之一。

**条件必填（type=FIXED 固定班制时）**：`--group-vo` 必须包含 `workDayClassList`（工作日班次列表，不能为空，共 7 个值，代表周日到周六每天的班次 ID，为 0 表示当天休息）和 `defaultClassId`（默认班次 ID，不能为 null）。

`--group-vo` 可用字段说明（与 `group update` 的 `--group-vo` 一致，均可选，只需包含要设置的字段）：

【基础信息】
  id            number    考勤组 id（创建时无需传入，由服务端分配）
  name          string    考勤组名称（必填）
  type          string    考勤组类型：FIXED（固定班制）/ TURN（排班制）/ NONE（自由工时）（必填）
  owner         string    考勤组主负责人 userId
  managerList   []string  考勤组子负责人 userId 列表
  skipHolidays  bool      节假日自动排休（只有固定班制和自由工时考勤组生效）
  defaultGroup  bool      是否默认考勤组
  classIds      []number  所选班次 id 列表(只有固定班制和排班制才有班次，自由工时没有)

【打卡范围与定位】
  trimDistance                      number    定位允许微调距离（米）
  enablePositionOfGps               bool      打卡是否允许开启 GPS 定位
  enablePositionOfWifi              bool      打卡是否允许开启 Wifi 定位
  enablePositionOfBle               bool      打卡是否允许开启蓝牙定位
  enableMacCheck                    bool      开启 MAC 地址校验
  checkDistanceType                 string    打卡距离类型：NORMAL(正常) / OUTER_ADDRESS(仅允许在外勤地址打卡)
  wifiCompanyId                     number    允许打卡的 Wifi 公司 ID

【打卡地址】
  positions  []object  打卡地址列表，每个对象字段：
    title     string  地址名称
    address   string  详细地址
    latitude  number  纬度
    longitude number  经度
    offset    number  该地址允许的打卡范围（米）

【打卡 Wifi】
  wifis  []object  打卡 Wifi 列表，每个对象字段：
    ssid    string  Wifi 名称
    macAddr string  MAC 地址
    groupId number  所属考勤组 ID

【蓝牙设备】
  bleDeviceVOList  []object  蓝牙设备列表，每个对象字段：
    name         string  设备名称
    deviceUid    string  设备 ID
    sn           string  序列号
    productType  string  产品类型
    devServId    number  设备服务 ID

【外勤打卡设置】
  enableOutsideCheck                 bool    是否允许外勤打卡
  openOutsideCameraCheck             bool    外勤打卡是否开启拍照
  enableOutsideRemark                bool    是否允许外勤备注
  enableOutsideApply                 bool    外勤打卡是否需审批
  forbidHideOutSideAddress           bool    禁止隐藏外勤打卡地址
  enableOutSideUpdateNormalCheck     bool    下班时允许外勤卡更新内勤卡
  enableOnDutyNormalUpdateOutsideCheck  bool    上班时允许内勤卡更新外勤卡
  outsideCheckApproveMode            string  外勤审批模式：NO_NEED_APPROVE / APPROVE_FIRST / CHECK_FIRST / APPROVE_EVERYTIME
  outSideCheckApplyType              number  外勤打卡申请类型：1 全天 / 2 上班 / 3 下班

【打卡方式】
  openCameraCheck                    bool    是否开启拍照打卡
  enableFaceStrictMode               bool    是否开启人脸严格模式
  enableFaceBeauty                   bool    是否开启人脸美颜
  onlyMachineCheck                   bool    是否仅允许考勤机打卡
  permitMaxBeaconCount               number  允许最大蓝牙信标数量
  disableCheckWhenRest               bool    休息日打卡需审批（只在固定班制和排班制生效）

【固定班制设置（FIXED）】
  defaultClassId      number    默认班次 ID
  workDayClassList    []number  工作日班次列表，共 7 个值代表周日到周六每天的班次 id，为 0 表示当天休息。
                               例如 [0,1279240003,0,0,0,0,0] 表示只有周一上班

【排班制设置（TURN）】
  disableCheckWithoutSchedule  bool    未排班时禁止打卡
  enableEmpSelectClass         bool    未排班时员工可选班次
  enableScheduleAutoMatch      bool    未排班时系统自动匹配
  cycleDays                    number  循环排班周期天数
  startCycleDate               number  循环排班开始日期（时间戳，毫秒）
  cycleScheduleList            []object  循环排班列表：
    cycleName  string  循环排班名称
    groupId    number  考勤组 ID
    isValid    string  是否生效：Y/N
    itemList   []object  循环排班明细：
      classId    number  班次 ID
      className  string  班次名称
      isValid    string  是否生效：Y/N

【自由工时设置（NONE）】
  workDays                         []number  工作日，[1-7]，1=周一，7=周日
  freeCheckDayStartMinOffset       number    距 0 点分钟数
  freeCheckCoreTime                number    最短工作时长（分钟）
  freeCheckDemandWorkMinutes       number    要求打卡时长（分钟）
  freeCheckSettingVO               object    自由打卡设置：
    freeCheckType                   string    打卡类型：CYCLE（上下班交替）/ MAX_TIME_UPDATE（最大时间打卡）
    freeWorkDayLackSwitch           bool      工作日缺卡开关
    freeOnDutyLackMinOffset         number    上班缺卡分钟数偏移
    freeOffDutyLackMinOffset        number    下班缺卡分钟数偏移
    delimitOffsetMinutesBetweenDays number    跨天切割分钟数偏移
    freeCheckGapVO                  object    打卡间隔：
      onOffCheckGapMinutes  number  上班到下班最小间隔（分钟）
      offOnCheckGapMinutes  number  下班到上班最小间隔（分钟）
  freeGroupSpecialDayVO            object    特殊日期设置：
    specialOnDutyDays    []number  特殊上班日期
    specialOffDutyDays   []number  特殊休息日期

当使用 `--group-vo` 传入完整 JSON 时，`--name`、`--type`、`--owner` 仍可覆写 JSON 中的同名字段。由于保存考勤组耗时较久，建议加 `--timeout 10`。

> **创建成功后的跳转链接**：`group create` 调用成功后，CLI 会在标准输出之后额外打印一条钉钉 PC 端跳转链接（仅在响应中同时包含 `corpId` 与 `groupId`/`id` 时输出），格式为 `dingtalk://dingtalkclient/page/link?url=https%3A%2F%2Fhrmregister.dingtalk.com%2Fsubapp%2Fattend%2Findex%3Fcode%3Dattend%26corpId%3D{corpId}%26ddtab%3Dtrue%26from%3Dattend%23%2FgroupModify%3Fid%3D{groupId}`，用于用户在钉钉 PC 客户端一键打开该考勤组详情页验证创建结果。Agent 遇到该跳转链接时应原样呈现给用户，不要对链接进行二次编码或裁剪。

> **新建考勤组并同时添加成员的工作流**：`group create` 不支持在创建时直接传入成员列表。若用户需要在新建考勤组后立即添加成员，必须先执行 `group create` 创建考勤组，从返回结果中提取 `groupId`，再执行 `group update-members --group-id <GROUP_ID> --add-users ...` 完成成员添加。

### 查询某个人的考勤统计摘要
```
Usage:
  dws attendance summary [flags]
Example:
  dws attendance summary --user USER_ID --date 2026-03-12 --stats-type week
Flags:
      --date string         查询日期，格式 YYYY-MM-DD 或 YYYY-MM-DD HH:mm:ss（必填）
      --stats-type string   统计类型：week（周统计）/ month（月统计）（必填）
      --user string         钉钉用户 ID（必填）
```

### 查询考勤组与考勤规则
```
Usage:
  dws attendance rules [flags]
Example:
  dws attendance rules --date 2026-03-14
  dws attendance rules --date "2026-03-14 09:00:00"
Flags:
      --date string   考勤日期, 格式 YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss (必填)
```

查询考勤组/考勤规则。例如：我属于哪个考勤组、打卡范围是什么、弹性工时怎么算。

### 查询个人规则设置
```
Usage:
  dws attendance selfsetting get [flags]
Example:
  dws attendance selfsetting get --setting-scene checkRemind --user <USER_ID> --format json
  dws attendance selfsetting get --setting-scene fastCheck --user <USER_ID> --format json
Flags:
      --setting-scene string   查询设置项: checkRemind/fastCheck/checkResultNotify/lackRemind/personalAttendStatNotify/bossAttendStatNotify (必填)
      --user string            查询用户 ID (必填)
```

调用 MCP 工具 query_self_setting 查询个人规则设置，包括打卡提醒、极速打卡、打卡结果通知、缺卡提醒、个人考勤统计通知、团队考勤统计通知等设置项。MCP 入参 `userId` 必填；CLI 的 `--user` 也必填，必须显式传入目标用户 ID。认证信息 `corpId` 和 `opUserId` 由当前登录上下文自动注入，无需手动传入。

`--setting-scene` 枚举值：
- `checkRemind`: 打卡提醒
- `fastCheck`: 极速打卡
- `checkResultNotify`: 打卡结果通知
- `lackRemind`: 缺卡提醒
- `personalAttendStatNotify`: 个人考勤统计通知
- `bossAttendStatNotify`: 团队考勤统计通知

返回 `ServiceResult`，包含 `success`、`code`、`message`、`result`。`result` 可能根据 `--setting-scene` 仅返回对应设置项相关字段。常见字段包括：
- `checkRemind`: `checkRemindSetting`、`checkRemindUserOnDuty`、`checkRemindUserOffDuty`、`enableOndutyCheckRemindOfPc`、`enableOffdutyCheckRemindOfPc`
- `fastCheck`: `ondutyCheckType`、`offdutyCheckType`、`ondutyRemindStartMin`、`ondutyRemindEndMin`、`offdutyRemindStartMin`、`offdutyRemindEndMin`、`fastCheckLateNeedConfirm`、`canUpdateOffDuty`、`voiceRemindSwitch`、`vibrationRemindSwitch`
- `checkResultNotify`: `checkResultMsg`, 取值 0 表示关闭, 1 表示开启
- `lackRemind`: `lackSendTodoMsg`、`lackRemindUser`, 取值 0 表示关闭, `null` 或 1 表示开启
- `personalAttendStatNotify`: `personDailyReportSwitch`、`personWeekReportType`、`personMonthReportType`
- `bossAttendStatNotify`: `bossPushStartMin`、`bossWeekReportType`、`bossMonthReportType`

其中周报/月报通知渠道枚举值：0 表示全关闭，1 表示工作通知，2 表示钉邮，3 表示工作通知和钉邮。

### 更新保存个人规则设置 (写场景接口，必须走二次确认流程)
**强制执行流程**：此命令为写操作，Agent 调用时必须遵守以下流程：
1. 先向用户展示待执行操作的完整参数摘要，包括目标用户、设置场景、当前值、新值和最终命令参数
2. 展示变更摘要并等待用户明确确认
3. 用户确认后，再传全局 `--yes` 执行命令

**禁止未经用户确认直接执行或自动添加 `--yes`。**

```
Usage:
  dws attendance selfsetting save [flags]
Example:
  # 开启打卡结果通知（执行前必须展示变更摘要并获得用户明确确认，确认后再追加 --yes）
  dws attendance selfsetting save --setting-scene checkResultNotify --user <USER_ID> --check-result-msg 1 --yes --format json
  # 更新极速打卡设置（执行前必须展示变更摘要并获得用户明确确认，确认后再追加 --yes）
  dws attendance selfsetting save --setting-scene fastCheck --user <USER_ID> --onduty-check-type 3 --voice-remind-switch=true --yes --format json
  # 更新打卡提醒设置（执行前必须展示变更摘要并获得用户明确确认，确认后再追加 --yes）
  dws attendance selfsetting save --setting-scene checkRemind --user <USER_ID> --check-remind-user-on-duty=false \
    --check-remind-setting '{"onDutyRemind":{"openRemind":true,"remindMinutes":10}}' --yes --format json
Flags:
      --setting-scene string                   更新设置项: checkRemind/fastCheck/checkResultNotify/lackRemind/personalAttendStatNotify/bossAttendStatNotify (必填)
      --user string                            更新用户 ID (必填)
      --check-remind-setting string            打卡提醒 DING 渠道设置 JSON
      --check-remind-user-on-duty              打卡提醒工作通知渠道：用户个人上班打卡提醒开关
      --check-remind-user-off-duty             打卡提醒工作通知渠道：用户个人下班打卡提醒开关
      --enable-onduty-check-remind-of-pc       PC 端弹窗渠道：上班打卡提醒开关
      --enable-offduty-check-remind-of-pc      PC 端弹窗渠道：下班打卡提醒开关
      --onduty-check-type int                  上班极速打卡方式：1 提醒打卡，2 不提醒且不自动打卡，3 自动打卡
      --offduty-check-type int                 下班极速打卡方式：1 提醒打卡，2 不提醒且不自动打卡，3 自动打卡
      --onduty-remind-start-min int            上班打卡提醒开始时间，单位：分钟
      --onduty-remind-end-min int              上班打卡提醒结束时间，单位：分钟
      --offduty-remind-start-min int           下班打卡提醒开始时间，单位：分钟
      --offduty-remind-end-min int             下班打卡提醒结束时间，单位：分钟
      --fast-check-late-need-confirm           迟到时是否需要二次确认
      --can-update-off-duty                    是否允许用户更新下班打卡设置
      --voice-remind-switch                    极速打卡提示音开关
      --vibration-remind-switch                极速打卡震动提醒开关
      --check-result-msg int                   打卡结果通知开关：0 关闭，1 开启
      --lack-send-todo-msg int                 缺卡提醒待办渠道：0 关闭，null 或 1 开启
      --lack-remind-user int                   缺卡提醒工作通知渠道：0 关闭，null 或 1 开启
      --person-daily-report-switch int         个人考勤统计日报推送开关：0 关闭，1 开启
      --person-week-report-type int            个人考勤统计周报通知渠道：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮
      --person-month-report-type int           个人考勤统计月报通知渠道：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮
      --boss-push-start-min int                团队考勤统计日报推送开始时间，单位：分钟；-1 表示关闭日报推送
      --boss-week-report-type int              团队考勤统计周报通知渠道：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮
      --boss-month-report-type int             团队考勤统计月报通知渠道：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮
      --yes                                    用户已确认，跳过交互式确认提示
                                               传入前必须已获得用户明确确认
```

调用 MCP 工具 save_self_setting 更新保存个人规则设置，请求体封装在 `RuleMcpSaveSelfSettingRequest` 中。`settingScene` 必填；MCP 入参 `userId` 必填，CLI 的 `--user` 也必填，必须显式传入目标用户 ID。认证信息 `corpId` 和 `opUserId` 由当前登录上下文自动注入，无需手动传入。

`selfsetting save` 必须按 `--setting-scene` 传入对应场景的字段，且至少一个字段有值：
- `checkRemind`: `checkRemindSetting`、`checkRemindUserOnDuty`、`checkRemindUserOffDuty`、`enableOndutyCheckRemindOfPc`、`enableOffdutyCheckRemindOfPc`
- `fastCheck`: `ondutyCheckType`、`offdutyCheckType`、`ondutyRemindStartMin`、`ondutyRemindEndMin`、`offdutyRemindStartMin`、`offdutyRemindEndMin`、`fastCheckLateNeedConfirm`、`canUpdateOffDuty`、`voiceRemindSwitch`、`vibrationRemindSwitch`
- `checkResultNotify`: `checkResultMsg`
- `lackRemind`: `lackSendTodoMsg`、`lackRemindUser`
- `personalAttendStatNotify`: `personDailyReportSwitch`、`personWeekReportType`、`personMonthReportType`
- `bossAttendStatNotify`: `bossPushStartMin`、`bossWeekReportType`、`bossMonthReportType`

返回 `ServiceResult`，包含 `success`、`code`、`message`、`result`。其中 `result` 为 boolean，表示保存是否成功。

#### 强制执行流程：Agent 调用 `selfsetting save`

`selfsetting save` 是写操作，会修改用户个人规则设置。Agent 调用时 **必须按以下流程执行**，**禁止**在未确认的情况下直接提交：

1. **识别写操作**：用户表达“更新个人规则设置 / 保存打卡提醒 / 修改极速打卡 / 关闭缺卡提醒 / 开启打卡结果通知 / 设置个人考勤统计通知 / 设置团队考勤统计通知”等意图时，命中 `selfsetting save`。
2. **收集必要参数**：必须明确 `--user`、`--setting-scene`，以及对应场景下将要修改的字段和值。
3. **前置查询当前设置**：执行 `dws attendance selfsetting get --setting-scene <SCENE> --user <USER_ID> --format json`，获取当前配置，用于确认目标用户和当前值。
4. **展示待写入数据并等待确认**：向用户展示目标用户、设置场景、要更新的字段、当前值、新值和最终命令参数摘要，并等待用户明确确认。
5. **用户确认后再执行保存**：**只有用户明确确认后**，才可以追加全局 `--yes` 执行 `dws attendance selfsetting save ... --yes --format json`。

确认话术示例：

```text
即将更新个人考勤规则设置，请确认：
- 用户 ID：<USER_ID>
- 设置场景：checkResultNotify
- 修改内容：
  - 打卡结果通知(checkResultMsg)：关闭 → 开启

是否确认执行更新？
```

禁止在未获得用户明确确认前执行保存；禁止为了推进流程自动添加全局 `--yes`。即使用户在最初需求中表达“直接改/不用问”，也必须先展示完整参数摘要并获得明确确认，才允许追加 `--yes` 执行。

### 查询全局规则设置（仅管理员）
```
Usage:
  dws attendance globalsetting get [flags]
Example:
  dws attendance globalsetting get --scope 企业 --setting-scene checkRemind --format json
  dws attendance globalsetting get --scope 全公司 --setting-scene bossAttendStatNotify --format json
Flags:
      --setting-scene string   查询设置项: checkRemind/fastCheck/checkResultNotify/lackRemind/personalAttendStatNotify/bossAttendStatNotify (必填)
      --scope string           全局范围确认，必须明确输入：企业/全公司/所有人（必填）
```

调用 MCP 工具 `query_global_setting` 查询全局规则设置，请求体封装在 `RuleMcpQueryGlobalSettingRequest` 中。`settingScene` 必填；CLI 必须通过 `--scope` 明确输入 `企业`、`全公司` 或 `所有人`，用于确认查询的是全局范围。认证信息 `corpId` 和 `opUserId` 由当前登录上下文自动注入，无需手动传入。该接口仅管理员可以调用。

返回 `ServiceResult`，包含 `success`、`code`、`message`、`result`。`result` 常见字段包括：
- `checkRemindCorp`: 打卡提醒企业总开关
- `checkRemindPcCorp`: 打卡提醒 PC 端企业总开关
- `fastCheckCorp`: 极速打卡企业总开关
- `enableCheckCertPush`: 打卡结果通知企业总开关
- `lackRemindCorp`: 缺卡提醒企业总开关
- `enablePersonalDailyReport`: 个人考勤统计通知日报企业总开关
- `enablePersonalWeeklyReport`: 个人考勤统计通知周报企业开关，钉邮渠道
- `enablePersonalWeeklyReportCard`: 个人考勤统计通知周报企业开关，工作通知渠道
- `enablePersonalMonthlyReport`: 个人考勤统计通知月报企业总开关
- `bossDailyReportType`: 团队考勤统计通知日报发送渠道类型，0 全关闭，1 开启
- `bossWeeklyReportType`: 团队考勤统计通知周报发送渠道类型，0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮
- `bossMonthlyReportType`: 团队考勤统计通知月报发送渠道类型，0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮

### 更新保存全局规则设置（写场景接口，仅管理员，必须走二次确认流程）
**强制执行流程**：此命令为写操作，Agent 调用时必须先向用户展示待执行操作的完整参数摘要，等待用户明确确认后，才能追加 `--yes` 执行。

```
Usage:
  dws attendance globalsetting save [flags]
Example:
  dws attendance globalsetting save --scope 企业 --setting-scene checkRemind --check-remind-corp=true --yes --format json
  dws attendance globalsetting save --scope 全公司 --setting-scene fastCheck --fast-check-corp=false --yes --format json
  dws attendance globalsetting save --scope 所有人 --setting-scene bossAttendStatNotify --boss-daily-report-type 1 --boss-weekly-report-type 3 --yes --format json
Flags:
      --setting-scene string                   更新设置项: checkRemind/fastCheck/checkResultNotify/lackRemind/personalAttendStatNotify/bossAttendStatNotify (必填)
      --scope string                           全局范围确认，必须明确输入：企业/全公司/所有人（必填）
      --check-remind-corp                      打卡提醒企业总开关
      --check-remind-pc-corp                   打卡提醒 PC 端弹窗企业总开关
      --fast-check-corp                        极速打卡企业总开关
      --enable-check-cert-push                 打卡结果通知企业总开关
      --lack-remind-corp                       缺卡提醒企业总开关
      --enable-personal-daily-report           个人考勤统计通知日报企业总开关
      --enable-personal-weekly-report          个人考勤统计通知周报企业开关，钉邮渠道
      --enable-personal-weekly-report-card     个人考勤统计通知周报企业开关，工作通知渠道
      --enable-personal-monthly-report         个人考勤统计通知月报企业总开关
      --boss-daily-report-type int             团队考勤统计通知日报发送渠道类型：0 全关闭，1 开启
      --boss-weekly-report-type int            团队考勤统计通知周报发送渠道类型：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮
      --boss-monthly-report-type int           团队考勤统计通知月报发送渠道类型：0 全关闭，1 工作通知，2 钉邮，3 工作通知和钉邮
      --yes                                    用户已确认，跳过交互式确认提示
```

调用 MCP 工具 `save_global_setting` 更新保存全局规则设置，请求体封装在 `RuleMcpSaveGlobalSettingRequest` 中。`settingScene` 必填；CLI 必须通过 `--scope` 明确输入 `企业`、`全公司` 或 `所有人`，用于确认更新的是全局范围。认证信息 `corpId` 和 `opUserId` 由当前登录上下文自动注入，无需手动传入。该接口仅管理员可以调用。

`globalsetting save` 必须按 `--setting-scene` 传入对应场景的字段，且至少一个字段有值：
- `checkRemind`: `checkRemindCorp`、`checkRemindPcCorp`
- `fastCheck`: `fastCheckCorp`
- `checkResultNotify`: `enableCheckCertPush`
- `lackRemind`: `lackRemindCorp`
- `personalAttendStatNotify`: `enablePersonalDailyReport`、`enablePersonalWeeklyReport`、`enablePersonalWeeklyReportCard`、`enablePersonalMonthlyReport`
- `bossAttendStatNotify`: `bossDailyReportType`、`bossWeeklyReportType`、`bossMonthlyReportType`

返回 `ServiceResult`，其中 `result` 为 boolean，表示保存是否成功。

### 获取企业考勤字段列表（仅管理员）
```
Usage:
  dws attendance report columns
Example:
  dws attendance report columns
```

根据操作者的列权限，过滤并返回其有权查看的考勤字段列表。操作者必须是管理员，否则返回权限错误。

### 根据字段查询考勤数据（仅管理员）
```
Usage:
  dws attendance report query-data [flags]
Example:
  dws attendance report query-data \
    --users userId1,userId2 --columns 1001,1002 --start "2026-03-01 00:00:00" --end "2026-03-31 23:59:59"
Flags:
      --columns string   字段 ID 列表, 逗号分隔, 可通过 report columns 获取（必填）
      --end string       结束日期, 格式 yyyy-MM-dd HH:mm:ss（必填）
      --start string     开始日期, 格式 yyyy-MM-dd HH:mm:ss（必填）
      --users string     目标用户 ID 列表, 逗号分隔, 最多 20 人（必填）
```

根据字段查询考勤数据，含列权限过滤和用户查看权限校验。--users 最多 20 人，--start 到 --end 不超过 32 天。

### 查询用户假期数据（仅管理员）
```
Usage:
  dws attendance report query-leave [flags]
Example:
  dws attendance report query-leave \
    --users userId1,userId2 --leave-names 年假,病假 --start "2026-03-01 00:00:00" --end "2026-03-31 23:59:59"
Flags:
      --end string          结束日期, 格式 yyyy-MM-dd HH:mm:ss（必填）
      --leave-names string  假期类型名称列表, 逗号分隔, 不填则查询所有假期类型（选填）
      --start string        开始日期, 格式 yyyy-MM-dd HH:mm:ss（必填）
      --users string        目标用户 ID 列表, 逗号分隔, 最多 20 人（必填）
```

查询用户假期数据，含用户查看权限校验。--users 最多 20 人，--start 到 --end 不超过 32 天。

### 查询当前用户假期规则列表
```
Usage:
  dws attendance vacation types
Example:
  dws attendance vacation types
Flags:
  无
```

调用 MCP 工具 get_leave_types 查询当前用户可用的假期规则列表。例如：年假、事假、病假等假期类型及对应规则。请求体封装在 McpLeaveTypeRequest 中，认证信息（corpId、opUserId）由系统自动注入，无需手动传入。

### 查询指定员工假期余额
```
Usage:
  dws attendance vacation balance [flags]
Example:
  dws attendance vacation balance --users userId1,userId2 --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890
Flags:
      --users string       目标员工 ID 列表, 逗号分隔 (必填)
      --leave-code string  假期规则 code (选填，不传则查询所有假期规则余额)
```

调用 MCP 工具 get_leave_balance_quota 查询指定员工的假期余额。例如：查询某员工年假还剩多少、病假额度等。`--leave-code` 可通过 `vacation types` 获取；不传 `--leave-code` 时查询所有假期规则余额。认证信息（corpId、opUserId）由系统自动注入。

如用户需要“所有假期规则余额  / 导出假期余额列表 / 所有假期规则余额 Excel / 按截图样式导出假期余额”，必须先读取 [attendance-vacation.md](./attendance-vacation.md)，再按其中工作流调用脚本生成 Excel。

### 查询指定员工假期余额变更记录
```
Usage:
  dws attendance vacation records [flags]
Example:
  dws attendance vacation records --user USER_ID --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --start 2026-04-01 --end 2026-04-22
Flags:
      --user string        指定查询员工 ID (必填)
      --leave-code string  假期规则 code (必填, 不传则无法查询)
      --start string       查询开始日期, 格式 YYYY-MM-DD (必填)
      --end string         查询结束日期, 格式 YYYY-MM-DD (必填)
```

调用 MCP 工具 get_leave_balance_records 查询指定员工的假期余额变更记录。例如：查询某员工年假变更历史、请假扣减记录等。`--leave-code` 可通过 `vacation types` 获取。认证信息（corpId、opUserId）由系统自动注入。

### 更新假期规则（写场景接口，必须走二次确认流程）

**强制执行流程**：此命令为写操作，Agent 调用时必须遵守以下流程：
1. 先向用户展示待执行操作的完整参数摘要
2. 展示变更摘要并等待用户明确确认
3. 用户确认后，再传 `--user-say-yes=true` 执行命令

**禁止未经用户确认直接执行或自动添加 `--user-say-yes=true`。**

```
Usage:
  dws attendance vacation update-type [flags]
Example:
  # 更新假期规则名称
  dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --name "事假（修改版）"

  # 更新假期单位
  dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --unit hour --per-hours 8

  # 改为指定部门可见
  dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --visibility-rules '[{"type":"dept","visible":["1","2","3"]}]'

  # 改为全公司可见（哨兵约定：必须显式传 "-1"，空数组 [] 不生效）
  dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --visibility-rules '[{"type":"dept","visible":["-1"]}]'
Flags:
      --leave-code string        假期编码（必填）
      --name string              假期名称（可选）
      --unit string              假期单位：day/halfDay/hour（可选）
      --paid bool                是否带薪假期（可选，默认 false）
      --per-hours int            一天折算小时数（可选）
      --when-can-leave string    新员工请假规则：entry/formal（可选）
      --visibility-rules string  适用范围规则 JSON 数组（可选）
                                 - 不传：不修改原有可见范围
                                 - 改为指定范围：[{"type":"staff|dept|label","visible":["id1",...]}, ...]
                                 - 改为全公司可见（哨兵）：必须显式传 [{"type":"dept","visible":["-1"]}]
                                 - 空数组 []、[{}]、visible 为空等 → CLI 报错（HSF 端会静默忽略）
      --user-say-yes             用户已确认，跳过交互式确认提示
                                 Agent 调用时传 true 前必须完成用户二次确认
```

调用 MCP 工具 save_leave_type 更新已有假期规则。`--leave-code` 必填，指定要更新的假期规则编码。其他字段均为可选，仅需传入要修改的字段。除 `--leave-code` 外，必须至少传入一个更新字段。

**`--visibility-rules` HSF 反序列化约定（必读）**：

| 入参形态 | 业务语义 | CLI / 服务端处理 |
|---|---|---|
| 不传 | 不修改可见范围 | 服务端保留原有 `visibilityRules` |
| `[{"type":"dept","visible":["-1"]}]`（哨兵） | 改为全公司可见 | 服务端落库为空数组（全公司语义） |
| `[{"type":"staff","visible":["uid1"]}, ...]` 等有效规则 | 改为指定可见范围 | 覆盖原有 `visibilityRules` |
| 空数组 `[]` / `[{}]` / `visible` 为空 | — （反例） | **CLI 直接报错**，避免 HSF 端被静默忽略 |

- `type` 可取 `dept`（部门 ID，`-1` 是根部门 = 全公司哨兵）、`staff`（userId）、`label`（角色 ID）。
- 哨兵识别采用宽松匹配：传入列表中任意一条规则满足 `type=dept` 且 `visible` 包含 `"-1"`，即视为全公司可见，其它规则被忽略。
- 若意图是“改为全公司可见”，**必须**显式传哨兵值 `["-1"]`，**不能**传空数组 `[]`。

####  强制执行流程：Agent 调用 `vacation update-type`

`vacation update-type` 是写操作，会修改假期规则配置。Agent 调用时 **必须按以下流程执行**，**禁止**在未确认的情况下直接提交：

1. **识别写操作**：用户表达"更新假期规则 / 修改假期类型 / 编辑假期规则"等意图时，命中 `vacation update-type`。
2. **收集必要参数**：必须明确 `--leave-code`，以及至少一个更新字段（`--name`、`--unit`、`--paid`、`--per-hours`、`--when-can-leave`、`--visibility-rules`）。
3. **前置查询当前规则**：需先调用 `vacation types` 确认该规则是否存在及当前配置。
4. **展示待写入数据并等待确认**：向用户展示假期编码、要更新的字段及新值，并询问是否确认执行。**必须等待用户明确确认**。
5. **用户确认后再执行保存**：**只有用户明确确认后**，才可以传 `--user-say-yes=true` 执行 `dws attendance vacation update-type ... --format json`。

确认话术示例：

```text
即将更新假期规则，请确认：
- 假期编码：a1b2c3d4-e5f6-7890-abcd-ef1234567890
- 更新内容：
  - 名称：事假 → 事假（修改版）

是否确认执行更新？
```

如用户明确要求跳过确认，可传 `--user-say-yes=true`；否则默认必须等待确认。

#### 强制执行流程：Agent 调用 `vacation save-balance`

`vacation save-balance` 是写操作，会直接替换员工的假期余额（SET 接口，而非 ADD）。Agent 调用时 **必须按以下流程执行**，**禁止**在未确认的情况下直接提交：

1. **识别写操作**：用户表达"设置假期余额 / 调整假期额度 / 更新假期余额 / 增加假期余额 / 发放年假 / 给员工加年假"等意图时，命中 `vacation save-balance`。
2. **前置查询当前余额**：必须先调用 `vacation balance --users <target> --leave-code <code>` 获取当前余额，因为这是 SET 接口，传入值会直接替换而非累加。
3. **收集必要参数**：必须明确 `--target`（目标员工）、`--leave-code`（假期编码）、`--num`（新余额数量）、`--reason`（变更原因），以及可选参数 `--start/--end`（有效期）。
4. **计算变更并展示确认**：向用户展示目标员工、假期类型、当前余额、新余额、差额（增加或减少）、变更原因、有效期等，并询问是否确认执行。**必须等待用户明确确认**。
5. **用户确认后再执行保存**：**只有用户明确确认后**，才可以传 `--user-say-yes=true` 执行 `dws attendance vacation save-balance ... --format json`。

确认话术示例：

**设置余额场景**：
```text
即将设置员工假期余额，请确认：
- 目标员工：张三（user001）
- 假期类型：年假（leaveCode: a1b2c3d4-e5f6-7890-abcd-ef1234567890）
- 当前余额：5 天
- 新余额：8 天
- 变更差额：+3 天（增加）
- 变更原因：年度发放
- 有效期：2024-01-01 至 2024-12-31

是否确认执行设置？
```

**减少余额场景**：
```text
即将设置员工假期余额，请确认：
- 目标员工：李四（user002）
- 假期类型：年假（leaveCode: a1b2c3d4-e5f6-7890-abcd-ef1234567890）
- 当前余额：10 天
- 新余额：2 天
- 变更差额：-8 天（减少）
- 变更原因：请假扣减

注意：此操作将大幅减少余额，请确认是否继续？

是否确认执行设置？
```

如用户明确要求跳过确认，可传 `--user-say-yes=true`；否则默认必须等待确认。

### 设置员工假期余额
```
Usage:
  dws attendance vacation save-balance [flags]
Example:
  # 设置员工年假余额为8天
  dws attendance vacation save-balance --target user001 \
    --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --num 8 --reason "年度发放"

  # 设置带有效期的假期余额
  dws attendance vacation save-balance --target user001 \
    --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --num 8 --reason "年度发放" \
    --start 2024-01-01 --end 2024-12-31
Flags:
      --target string     目标员工工号（必填）
      --leave-code string 假期编码（必填）
      --num string        余额数量，如8天传8，7.5天传7.5（必填）
      --reason string     变更原因，最长100字符（必填）
      --start string      有效期开始日期 YYYY-MM-DD（可选）
      --end string        有效期结束日期 YYYY-MM-DD（可选）
      --user-say-yes      用户已确认，跳过交互式确认提示
                          Agent 调用时传 true 前必须完成用户二次确认
```

**重要：这是设置（SET）接口，传入的值会替换当前余额，而非增加（ADD）。**余额数量在传递给 MCP 时会自动乘以 100（如 8 天传 800）。执行前会展示待写入数据，需用户确认后提交。

### 查询指定员工的签到记录
```
Usage:
  dws attendance checkin records [flags]
Example:
  dws attendance checkin records \
    --operator-staff-id op001 --staff-ids user001,user002 --start "2026-04-01 00:00:00" --end "2026-04-07 00:00:00"
Flags:
      --end string                结束时间, 格式 yyyy-MM-dd HH:mm:ss（必填）
      --operator-corp-id string   操作者企业 ID（必填）
      --operator-staff-id string  操作者员userID（必填）
      --staff-ids string          目标员工userID 列表, 逗号分隔（必填），员工数最多100个人
      --start string              开始时间, 格式 yyyy-MM-dd HH:mm:ss（必填），开始到结束时间限制在7天
```

调用 MCP 工具 get_checkin_record 查询指定员工在一段时间内的签到记录。权限说明：Boss/超级管理员可查看全公司员工，子管理员可查看管理范围内员工，部门主管可查看所管理部门员工，普通员工只能查询自己。接口单次最多返回100条签到记录。

返回结构：`result.list` 为签到记录数组，每条记录包含以下字段（按 sortedProps 顺序）：
- `corpId`（string）：企业 ID
- `name`（string）：用户名称
- `userId`（string）：用户 ID
- `timestamp`（number）：签到时间
- `place`（string）：签到地点
- `detailPlace`（string）：签到详细地点
- `longitude`（number）：签到地点经度
- `latitude`（number）：签到地点纬度
- `remark`（string）：备注信息
- `imageList`（string[]）：图片列表
- `customers`（string）：拜访客户
- `checkinType`（string）：签到类型
- `mobileId`（string）：设备 ID

顶层还包含 `success`（boolean）和 `arguments`（object[]）字段。
