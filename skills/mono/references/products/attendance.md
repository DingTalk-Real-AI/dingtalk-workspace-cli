# 考勤 (attendance) 命令参考

> **【命令可用性提示】** 当前 dws 已注册全部考勤子命令组（`record` / `check` / `approve` / `shift` / `schedule` / `class` / `adjustment` / `overtime` / `group` / `summary` / `rules` / `selfsetting` / `globalsetting` / `vacation` / `checkin` / `report` / `boss-check`）。查询与写操作大多可直接调用后端，不会再返回 `unknown command`，不要以"开源版不支持"为由拒答。个别命令返回受账号权限和组织数据影响：`report` 系列仅管理员可用；非管理员或数据为空时可能返回空列表或权限错误。执行前可用 `dws <cmd> --help` 或 `--dry-run` 验证参数。

> **【必读】日期范围严格计算规则 — 所有含 --start/--end 或 --from/--to 的命令均适用**
>
> **禁止随意猜测日期范围，必须按以下规则精确计算后再组装命令参数：**
>
> | 用户表达 | --start（含）| --end（含）| 说明 |
> |---------|------------|------------|------|
> | 本周 / 这周 / 当周 | 本周**周一** | 本周**周日** | 周一为一周的第一天，周日为最后一天 |
> | 上周 / 上一周 | 上周**周一** | 上周**周日** | 同上，往前推一周 |
> | 本月 / 这个月 / 当月 | 本月 **1 日** | 本月**最后一天**（28/29/30/31）| 必须计算当月实际天数 |
> | 上月 / 上个月 | 上月 **1 日** | 上月**最后一天** | 同上，往前推一个月 |
> | 今天 / 今日 | **今天** | **今天** | start == end |
> | 昨天 | **昨天** | **昨天** | start == end |
>
> **必须遵守：**
> 1. **本周一定是周一到周日**，严禁将"本周"解释为周一到周五或其他任意范围。
> 2. **本月一定是 1 日到该月实际最后一天**，严禁将"本月"截断为 1 日到 28 日或其他固定天数。
> 3. 计算日期时必须参考当前系统时间，不得使用硬编码或模糊估算。
> 4. 如果用户指定了具体日期范围（如"4月1日到4月15日"），直接使用用户给定的范围，不套用上述规则。
> 5. 日期格式统一按各命令要求：`shift list` / `check result` 等使用 `YYYY-MM-DD`；`report query-data` / `report query-leave` 等使用 `yyyy-MM-dd HH:mm:ss`（start 取当天 `00:00:00`，end 取当天 `23:59:59`）。

## 签到（checkin）— 优先路由

> **只要用户句中含"签到"二字（签到记录/签到数据/签到明细/外勤签到/导出签到/签到报表），就走本节，不要走 `check record`（打卡流水）。"签到" ≠ "打卡"，签到是外勤场景的独立功能。**

### 导出 vs 查询 判断规则（必须先判断）

| 用户说 | 路由 |
|--------|------|
| 含"导出"/"报表"/"Excel"/"表格"/"生成"中**任何一个** + 含"签到" | → **导出签到报表**（走脚本） |
| 不含上述导出关键词，仅"查签到"/"签到记录"/"看看签到" | → **查询签到记录**（走命令） |

> **示例**："帮我导出最近一个月的签到记录" → 含"导出" → 走报表脚本
> "查一下张三的签到记录" → 不含导出关键词 → 走查询命令

- **导出签到报表** → **必须先 `read_file` 读取 [attendance-report.md](./attendance-report.md) 后按其中的工作流执行（报表类型为"签到报表"）**，对应脚本 `attendance_report_checkin.py`
  - **严禁**绕过 `attendance-report.md` 直接调用脚本
  - **严禁**自己手动分段调用 `dws attendance checkin records` 来拼数据，必须用脚本
- **查询签到记录**（无需导出，仅查看） → `dws attendance checkin records`（数据源为 MCP 工具 `get_checkin_record`）

签到记录返回字段（`result.list` 数组，按 sortedProps 顺序）：
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

### 查询个人某日考勤详情
```
Usage:
  dws attendance record get [flags]
Example:
  dws attendance record get --user 011769261608 --date 2026-03-08
Flags:
      --user string  钉钉用户 ID (必填，别名 --users 亦可)
      --date string  查询日期, 格式 YYYY-MM-DD (必填)
```

查询单个用户在**某一天**的考勤详情（区别于 `check record`/`check result` 的多人时间段批量查询）。返回 `result` 对象含 `isHasSchedule`（当日是否有排班）、`isRest`（是否休息日）、`isUnSigned`（是否未打卡）、`recordList`（打卡明细）、`approveList`（当日审批单）、`workOvertime`、`workTimeDesc` 等字段。`--user` 只接受**单个** userId；查 userId 用 `dws contact user search --query "姓名"`。

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
  dws attendance schedule import --group-id 123456 \
    --schedules '[{"userId":"user001","classId":123,"workDate":"2026-04-22","isRest":"N"}]' \
    --yes
Flags:
      --group-id string   考勤组（必填，传入考勤组ID）；--help 主名为 --groupId，--group-id 为别名
      --schedules string  排班记录 JSON 数组（必填）；--help 主名为 --scheduleVOS，--schedules 为别名
      --yes               跳过确认提示；--help 主名为 --user-say-yes，--yes 为别名
```

为排班制考勤组导入排班记录。`--schedules` 为 JSON 数组，每条记录包含：
- `userId`: 员工ID（必填）
- `classId`: 班次ID（必填）
- `workDate`: 工作日期（YYYY-MM-DD），如 2026-04-22（必填）
- `isRest`: 是否休息日 Y/N（**必填**，服务端要求传入）
- `checkBeginTime` / `checkEndTime`: 开始/结束打卡时间（可传，但当前不会进入后端 payload，实际打卡时段以 `classId` 对应班次为准）

#### AI 调用 `schedule import` 的二次确认流程

`schedule import` 是写操作，会为考勤组导入或变更员工排班。AI 调用时必须按以下流程执行，不得在未确认的情况下直接导入：

1. **识别写操作**：用户表达“导入排班 / 设置排班 / 安排排班 / 给员工排班 / 批量排班”等意图时，命中 `schedule import`。
2. **收集必要参数**：必须明确 `--group-id` 和 `--schedules`，并确认排班记录中的 `userId`、`classId`、`workDate`、`checkBeginTime`、`checkEndTime`、`isRest` 等字段。
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
2. 使用 `ask_human` 或返回待确认状态，等待用户明确确认
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
2. 使用 `ask_human` 或返回待确认状态，等待用户明确确认
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

**注意：本命令当前拿不到有效的补卡规则详情，不要依赖它。** 服务端对任意 `--adjustment-id`（含不存在的 ID）都返回同一个 `{"success":true,"有效期类型":"..."}`，不返回规则明细，也无法据此判断规则是否存在；且 `adjustment search` 返回的默认补卡规则 `entityVO.id` 为 `null`，`search → get` 取 id 的链路走不通。补卡规则内容请直接看 `adjustment search` 的返回结果。

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
2. 使用 `ask_human` 或返回待确认状态，等待用户明确确认
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
      --add-depts string          添加考勤部门 ID 列表, 逗号分隔, 最多 20 个 (可选)
      --remove-depts string       删除考勤部门 ID 列表, 逗号分隔, 最多 20 个 (可选)
```

对指定考勤组的成员进行增删操作。--group-id 必填，其余参数均为可选，但至少需要传入一个变更项，否则命令拒绝执行。每次调用各参数最多传 20 个 ID。"无需考勤"人员指考勤组内豁免打卡的成员（如高管）。

### 更新考勤组配置 (写场景接口，必须走二次确认流程)
**强制执行流程**：此命令为写操作，Agent 调用时必须遵守以下流程：
1. 先向用户展示待执行操作的完整参数摘要，包括考勤组 ID、要修改的字段含义及新值
2. 使用 `ask_human` 或返回待确认状态，等待用户明确确认
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
2. 使用 `ask_human` 或返回待确认状态，等待用户明确确认
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

> **新建考勤组并同时添加成员的工作流**：`group create` 不支持在创建时直接传入成员列表。若用户需要在新建考勤组后立即添加成员，必须先执行 `group create` 创建考勤组，从返回结果中提取 `groupId`，再执行 `group update-members --group-id <GROUP_ID> --add-users ...` 完成成员添加。

### 查询某个人的考勤统计摘要
```
Usage:
  dws attendance summary [flags]
Example:
  dws attendance summary --user USER_ID --date 2026-03-12 --stats-type week
  dws attendance summary --user USER_ID --date "2026-03-12 15:00:00" --stats-type month
Flags:
      --date string        查询日期, 格式 YYYY-MM-DD 或 YYYY-MM-DD HH:mm:ss (必填)
      --stats-type string  统计类型: week 周统计 / month 月统计 (必填)
      --user string        钉钉用户 ID (必填)
```

`summary` 必须同时传 `--user`、`--date`、`--stats-type`，缺一即报错（如 C0002）。`--stats-type` 只能是 `week`（周统计）或 `month`（月统计）。

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
2. 使用 `ask_human` 或返回待确认状态，等待用户明确确认
3. 用户确认后，再传全局 `--yes` 执行命令

**禁止未经用户确认直接执行或自动添加 `--yes`。**

```
Usage:
  dws attendance selfsetting save [flags]
Example:
  # 开启打卡结果通知（Agent 调用时，必须先完成 ask_human 二次确认，确认后再追加 --yes 执行）
  dws attendance selfsetting save --setting-scene checkResultNotify --user <USER_ID> --check-result-msg 1 --yes --format json
  # 更新极速打卡设置（Agent 调用时，必须先完成 ask_human 二次确认，确认后再追加 --yes 执行）
  dws attendance selfsetting save --setting-scene fastCheck --user <USER_ID> --onduty-check-type 3 --voice-remind-switch=true --yes --format json
  # 更新打卡提醒设置（Agent 调用时，必须先完成 ask_human 二次确认，确认后再追加 --yes 执行）
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
                                               Agent 调用时传入前必须已完成 ask_human 二次确认
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
4. **展示待写入数据并等待确认**：向用户展示目标用户、设置场景、要更新的字段、当前值、新值和最终命令参数摘要。必须调用 `ask_human` 或返回待确认状态，并等待用户明确确认。
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

禁止在未获得用户明确确认前执行保存；禁止为了推进流程自动添加全局 `--yes`。即使用户在最初需求中表达“直接改/不用问”，Agent 也必须先通过 `ask_human` 或待确认状态展示完整参数摘要并获得明确确认后，才允许追加 `--yes` 执行。

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
      --leave-code string  假期规则 code (必填，服务端要求非空，不传返回 INVALID_PARAMS)
```

调用 MCP 工具 get_leave_balance_quota 查询指定员工的假期余额。例如：查询某员工年假还剩多少、病假额度等。`--leave-code` 可通过 `vacation types` 获取。**注意：`--help` 虽标"选填"，但服务端要求 `leaveCode` 非空，实际必填；不传会返回 `INVALID_PARAMS`（corpId、opUserId、leaveCode、targetUserIds 不能为空）。** 若要一次查所有假期规则余额，必须走 [attendance-vacation.md](./attendance-vacation.md) 工作流脚本逐个规则遍历。认证信息（corpId、opUserId）由系统自动注入。

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

调用 MCP 工具 get_leave_balance_records_v2 查询指定员工的假期余额变更记录。例如：查询某员工年假变更历史、请假扣减记录等。`--leave-code` 必填，可通过 `vacation types` 获取。认证信息（corpId、opUserId）由系统自动注入。

### 更新假期规则（写场景接口，必须走二次确认流程）

**强制执行流程**：此命令为写操作，Agent 调用时必须遵守以下流程：
1. 先向用户展示待执行操作的完整参数摘要
2. 使用 `ask_human` 或返回待确认状态，等待用户明确确认
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

  # 更新适用范围
  dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --visibility-rules '[{"type":"dept","visible":["1","2","3"]}]'
Flags:
      --leave-code string        假期编码（必填）
      --name string              假期名称（可选）
      --unit string              假期单位：day/halfDay/hour（可选）
      --paid bool                是否带薪假期（可选，默认 false）
      --per-hours int            一天折算小时数（可选）
      --when-can-leave string    新员工请假规则：entry/formal（可选）
      --visibility-rules string  适用范围规则 JSON 数组（可选）
      --user-say-yes             用户已确认，跳过交互式确认提示
                                 Agent 调用时传 true 前必须完成用户二次确认
```

调用 MCP 工具 save_leave_type 更新已有假期规则。`--leave-code` 必填，指定要更新的假期规则编码。其他字段均为可选，仅需传入要修改的字段。除 `--leave-code` 外，必须至少传入一个更新字段。

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
    --operator-corp-id corp001 --operator-staff-id op001 --staff-ids user001,user002 --start "2026-04-01 00:00:00" --end "2026-04-07 00:00:00"
Flags:
      --end string                结束时间, 格式 yyyy-MM-dd HH:mm:ss（必填）
      --operator-corp-id string   操作者企业 ID（必填）
      --operator-staff-id string  操作者员userID（必填）
      --staff-ids string          目标员工userID 列表, 逗号分隔（必填），员工数最多100个人
      --start string              开始时间, 格式 yyyy-MM-dd HH:mm:ss（必填），开始到结束时间限制在7天
```

调用 MCP 工具 get_checkin_record 查询指定员工在一段时间内的签到记录。权限说明：Boss/超级管理员可查看全公司员工，子管理员可查看管理范围内员工，部门主管可查看所管理部门员工，普通员工只能查询自己。接口单次最多返回100条签到记录。

## 意图判断

用户说"打卡记录/出勤/考勤" → `check record`
用户说"指定用户打卡结果/考勤结果/迟到早退/缺卡异常" → `check result`
用户说"指定用户打卡流水/打卡详情/打卡时间地点/打卡记录详情" → `check record`
用户说"审批单/请假记录/加班记录/出差记录/补卡记录" → `approve list`
用户说"查询排班记录/获取排班详情/查看排班/排班表/导出排班/导出排班表/排班导出/XX考勤组的排班" → **必须先 `read_file` 读取 [attendance-schedule.md](./attendance-schedule.md) 后按其中的「排班查询导出工作流」执行**。
  - **优先级**：只要用户句中含"排班"二字且意图是查看/导出，就走本条，不要走 `attendance-report.md`。"导出排班表" ≠ "导出考勤报表"
  - **严禁**绕过 `attendance-schedule.md` 直接调用 `dws attendance schedule get` 命令
  - 脚本自动处理：分批查询（超 20 人自动分批）、userId→姓名转换、classId→班次名称转换、排班表格式 Excel 输出
用户说"排班/导入排班/安排排班/设置排班/安排班次/调班/换班/排休" → **必须先 `read_file` 读取 [attendance-schedule.md](./attendance-schedule.md) 后按其中的「排班导入工作流」执行**。
  - **严禁**绕过 `attendance-schedule.md` 直接调用 `dws attendance schedule import` 命令
  - **严禁**仅凭命令 `--help` 或本文件中的命令参考自行组装排班命令
  - 该文档定义了：考勤组类型校验（必须为 TURN）、班次校验（必须属于该考勤组）、排班回显确认、错误处理等约束，缺一不可
  - 违反约束的后果：排错班次、排错人员、排班数据覆盖无法回退
用户说"班次定义/班次列表/有哪些班次/我负责的班次" → `class search`（返回结果已包含全量属性，无需再调 get）
用户说"班次详情/某个班次的具体信息" → `class search --name "..."`（search 直出，直接返回详情）。`class get` 仅在需要按已知 classId 精确查询时使用
用户说"更新班次/修改班次/班次改名/修改上下班时间" → `class update`
用户说"补卡规则/补卡设置" → `adjustment search`（返回结果已包含全量属性，无需再调 get）
用户说"补卡规则详情/某条补卡规则的具体信息" → `adjustment search --name "..."`（search 直出，已含全量属性）。**不要用 `adjustment get`**：当前服务端对任意 id 都只返回"有效期类型"、拿不到规则明细
用户说"加班规则/加班设置/加班计算" → `overtime search`（返回结果已包含全量属性，无需再调 get）
用户说"加班规则详情/某条加班规则的具体信息" → `overtime search --name "..."`（search 直出）。如需查已删除/被覆盖的历史记录 → `overtime get`
用户说"考勤组列表/有哪些考勤组" → `group search`
用户说"考勤组详情/全量考勤组信息" → `group get`,若返回结果中含成员 userId 列表，则对每个 userId 调用 `dws contact user get --user-ids <userId>`（或等价通讯录查询），在最终输出中展示员工姓名而非裸 userId
用户说"考勤组成员/打卡地址/打卡wifi/打卡蓝牙" → `group filtered-get`（按需查询，节省成本）,若返回结果中含成员 userId 列表，则对每个 userId 调用 `dws contact user get --user-ids <userId>`（或等价通讯录查询），在最终输出中展示员工姓名而非裸 userId
用户说"更新考勤组成员/添加考勤人员/删除考勤人员/添加考勤部门/删除考勤部门/加入考勤组/移出考勤组/设置无需考勤/取消无需考勤" → `group update-members`
用户说"修改考勤组/更新考勤组配置/考勤组改名/改变考勤组绑定的班次/修改打卡范围/设置考勤组负责人" → `group update`
用户说"创建考勤组/新建考勤组/添加考勤组" → `group create`
用户说"考勤汇总/统计" → `summary`
用户说"考勤组/考勤规则/打卡规则" → `rules`
用户说"查询个人规则设置/查看打卡提醒/查看极速打卡/查看缺卡提醒/查看打卡结果通知/查看个人考勤统计通知/查看团队考勤统计通知" → `selfsetting get`
用户说"更新个人规则设置/保存打卡提醒/修改极速打卡/关闭缺卡提醒/开启打卡结果通知/设置个人考勤统计通知/设置团队考勤统计通知" → `selfsetting save`
用户说"考勤字段/考勤列" → `report columns`
用户说"考勤数据/查询考勤报表数据" → `report query-data`（单次查询场景，非导出）
  **导出考勤/导出报表/生成考勤报表/出勤汇总导出/考勤明细导出/迟到早退统计导出/全员考勤数据导出/月度考勤报表/考勤表格/考勤 Excel** → **必须先 `read_file` 读取 [attendance-report.md](./attendance-report.md) 后按其中的工作流执行**。
  - **排除**：如果用户说的是"导出**排班**表"/"导出**排班**"/"**排班**导出"，这属于**排班查询导出**，应路由到 [attendance-schedule.md](./attendance-schedule.md)，而非本条。判断标准：句中含"排班"二字 → 走排班；不含"排班"或明确说"考勤报表/考勤数据/出勤统计" → 走报表。
  - **严禁**绕过 `attendance-report.md` 直接调用 `python scripts/attendance_report_*.py` 任何脚本
  - **严禁**仅凭脚本 `--help` 或本文件"自动化脚本"表格里的脚本路径就推断参数自行组装命令
  - 该文档定义了：报表类型默认值、列选择策略（`--column-keywords`）、阶段 1 人员获取流程、错误处理、输出摘要规范，缺一不可
  - 违反约束的后果：报表数据不全、列错位、人员遗漏、用户得到错误结果
用户说"假期数据/年假/病假/请假记录" → `report query-leave`
用户说"假期/我的假期/假期规则" → `vacation types`
用户说"病假余额/年假余额/事假剩余假期"等查询指定假期规则的余额 → `vacation balance`
用户说"导出假期余额/假期余额列表/所有假期规则余额/假期余额 Excel/年假病假调休余额导出"等全部假期规则余额的查询 → **必须先 `read_file` 读取 [attendance-vacation.md](./attendance-vacation.md) 后按其中工作流执行**
用户说"假期变更/假期记录/请假扣减" → `vacation records`
用户说"更新假期规则/修改假期类型/编辑假期规则" → `vacation update-type --leave-code <LEAVE_CODE>`
用户说"设置假期余额/调整假期额度/更新假期余额" → 先调用 `vacation balance` 获取当前余额，计算修改后的值，再调用 `vacation save-balance`
用户说"增加假期余额/发放年假/给员工加年假" → 先调用 `vacation balance` 获取当前余额，加上要增加的天数，再调用 `vacation save-balance` 设置新总额度
用户说"签到/签到记录" → `checkin records`

## 核心工作流

```bash
# 导入排班记录
dws attendance schedule import --group-id 123456 \
  --schedules '[{"userId":"user001","classId":123,"workDate":"2026-04-22","isRest":"N"}]' \
  --yes --format json

#  获取排班记录 — 禁止直接调用，必须走 attendance-schedule.md 排班查询导出工作流
# python scripts/attendance_schedule_export.py --users user001,user002 --start 2026-04-01 --end 2026-04-30

# 查询可管理的班次列表
dws attendance class search --format json
dws attendance class search --query "早班" --filter-type MINE_OWN --format json

# 查询班次详情
dws attendance class get --class-id 1170996821 --format json

# 更新班次
dws attendance class update --class-id 1170996821 --name "新早班" --timeout 10 --format json
dws attendance class update --class-id 1170996821 --class-vo '{"sections":[{"times":[{"checkType":"OnDuty","checkTime":"08:30","across":0},{"checkType":"OffDuty","checkTime":"17:30","across":0}]}]}' --timeout 10 --format json

# 查询补卡规则
dws attendance adjustment search --page 1 --limit 20 --format json
dws attendance adjustment search --query "标准" --page 1 --limit 20 --format json

# 查询补卡规则详情
dws attendance adjustment get --adjustment-id 12345 --format json

# 查询加班规则
dws attendance overtime search --page 1 --limit 20 --format json

# 查询加班规则详情
dws attendance overtime get --overtime-id 12345 --format json

# 查询考勤组列表
dws attendance group search --query "研发" --page 1 --limit 20 --format json
dws attendance group search --type FIXED --page 1 --limit 20 --format json

# 查询考勤组全量信息
dws attendance group get --group-id 123456 --format json

# 按需查询考勤组成员/地址/蓝牙/Wifi
dws attendance group filtered-get --group-id 123456 --member --format json
dws attendance group filtered-get --group-id 123456 --position --wifi --format json

# 更新考勤组成员
dws attendance group update-members --group-id 123456 --add-users userId1,userId2 --timeout 10 --format json
dws attendance group update-members --group-id 123456 --remove-users userId1 --timeout 10 --format json
dws attendance group update-members --group-id 123456 --add-depts deptId1 --remove-users userId2 --timeout 10 --format json

# 更新考勤组配置
dws attendance group update --group-id 123456 --name "研发考勤组" --timeout 10 --format json
dws attendance group update --group-id 123456 --classIds '[1374234767]' --timeout 10 --format json
dws attendance group update --group-id 123456 --group-vo '{"positions":[{"title":"总部","address":"北京市","latitude":39.9,"longitude":116.4,"offset":200}]}' --timeout 10 --format json

# 创建考勤组
dws attendance group create --name "研发考勤组" --type FIXED --group-vo '{"defaultClassId":1170996821,"workDayClassList":[0,1170996821,0,0,0,0,0]}' --timeout 10 --format json
dws attendance group create --name "自由工时分组" --type NONE --timeout 10 --format json

# 查看考勤统计摘要（--stats-type 必填：week 周统计 / month 月统计）
dws attendance summary --user <USER_ID> --date 2026-03-12 --stats-type week --format json

# 查看考勤组和规则
dws attendance rules --date 2026-03-14 --format json

# 查看指定用户的打卡提醒设置
dws attendance selfsetting get --setting-scene checkRemind --user <USER_ID> --format json

# 查看指定用户的极速打卡设置
dws attendance selfsetting get --setting-scene fastCheck --user <USER_ID> --format json

# 开启指定用户的打卡结果通知
dws attendance selfsetting save --setting-scene checkResultNotify --user <USER_ID> --check-result-msg 1 --format json

# 更新指定用户的极速打卡设置
dws attendance selfsetting save --setting-scene fastCheck --user <USER_ID> \
  --onduty-check-type 3 --voice-remind-switch=true --format json

# 获取考勤字段列表（管理员）
dws attendance report columns --format json

# 根据字段查询考勤数据（管理员）
dws attendance report query-data --users userId1,userId2 \
  --columns 1001,1002 --start "2026-03-01 00:00:00" --end "2026-03-31 23:59:59" --format json

# 查询用户假期数据（管理员）
dws attendance report query-leave --users userId1,userId2 \
  --leave-names 年假,病假 --start "2026-03-01 00:00:00" --end "2026-03-31 23:59:59" --format json

# 查看假期规则列表
dws attendance vacation types --format json

# 查看指定员工假期余额
dws attendance vacation balance --users userId1,userId2 --leave-code <假期类型code> --format json

# 查看指定员工某类假期余额
dws attendance vacation balance --users userId1 --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --format json

# 查看指定员工假期余额变更记录
dws attendance vacation records --user USER_ID --leave-code <假期类型code> --start 2026-04-01 --end 2026-04-22 --format json

# 更新假期规则名称
dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 \
  --name "事假（修改版）" --format json

# 更新假期单位
dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 \
  --unit hour --per-hours 8 --format json

# 更新适用范围
dws attendance vacation update-type --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 \
  --visibility-rules '[{"type":"dept","visible":["1","2","3"]}]' --format json

# 设置员工假期余额完整流程
# 1. 查询当前余额
dws attendance vacation balance --users user001 \
  --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --format json

# 2. 根据查询结果计算新值（如当前5天，要设置为8天）

# 3. 执行设置（SET操作，会替换当前余额）
dws attendance vacation save-balance --target user001 \
  --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 \
  --num 8 --reason "年度发放" --start 2024-01-01 --end 2024-12-31 --format json

# 增加员工假期余额完整流程（ADD场景）
# 1. 查询当前余额（假设返回5天）
dws attendance vacation balance --users user001 \
  --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 --format json

# 2. 计算增加后的新值（5 + 3 = 8天）

# 3. 设置新总额度
dws attendance vacation save-balance --target user001 \
  --leave-code a1b2c3d4-e5f6-7890-abcd-ef1234567890 \
  --num 8 --reason "绩效奖励发放3天" --format json

# 查询签到记录
dws attendance checkin records --operator-corp-id corp001 --operator-staff-id op001 --staff-ids user001,user002 \
  --start "2026-04-01 00:00:00" --end "2026-04-07 00:00:00" --format json
```

## 上下文传递表
| 操作 | 提取 | 用于 |
|------|------|------|
| `contact user get-self` | `userId` | summary 的 --user |
| `rules` | `groupId` | schedule import 的 --group-id |
| `schedule import` | `classId` | schedule import 的 schedules 中的 classId |
| `contact user search` | `userId` | schedule import/get 的 userId |

| `contact user get-self` / `contact user search` | `userId` | summary 的 --user, vacation records 的 --user；selfsetting get/save 的 --user（必填） |
| 当前登录上下文 | `corpId`, `opUserId` | selfsetting get/save 自动补齐 MCP 入参, CLI 不需要传 `--corp-id` / `--op-user` |
| `vacation types` | `leaveCode` | vacation balance 的 --leave-code, vacation records 的 --leave-code |
## 注意事项
**Agent 使用引导**：
- 执行 vacation 子命令前，**必须先调用** `dws attendance vacation --help` 查看完整子命令列表和参数说明
- 新增命令可能不在 Agent 缓存中，直接猜测命令会失败
- 正确流程：查看帮助 → 选择命令 → 查看命令详细参数（如 `dws attendance vacation update-type --help`）→ 执行

- `record get` 的 `--date` 格式: YYYY-MM-DD（如 `2026-03-08`），CLI 自动转换为毫秒时间戳
- `shift list` 查询班次信息，`--start/--end` 使用 YYYY-MM-DD 格式，间隔不超过 7 天
- `schedule import` 导入排班记录 — 必须通过 [attendance-schedule.md](./attendance-schedule.md) 排班导入工作流执行
- `schedule get` 查询排班记录 — 必须通过 [attendance-schedule.md](./attendance-schedule.md) 排班查询导出工作流执行（脚本自动分批、姓名转换、排班表格式导出）
- `schedule import` 是写操作，AI 调用时必须先展示导入摘要并引导用户二次确认；用户明确确认后才允许执行。用户明确要求跳过确认或命令包含全局 `--yes` 时，可跳过二次确认
- `class search` 所有参数均为可选，不填时返回全部可管理班次（默认第 1 页，每页 20 条）
- **概念区分**：班次是员工当天打卡安排；排班是为排班制考勤组导入的排班记录；班次定义是考勤管理员创建的工作时间规则
- `class get` 的 `--class-id` 必填，班次 ID 可从 `class search` 结果中提取
- `class search` 返回结果已包含全量属性，无需再调用 `class get`；`class get` 仅在需要按已知 classId 精确查询时使用
- `class update` 的 `--class-id` 必填，其余均可选，仅需对要修改的字段赋值，未传字段会自动从已有配置补充；由于保存班次耗时较久，建议加 `--timeout 10`
- `adjustment search` 返回结果已包含全量属性；`adjustment get` 当前服务端对任意 id 都只返回"有效期类型"、无规则明细，不可用，补卡规则详情一律看 `adjustment search` 返回
- `overtime search` 返回结果已包含全量属性，无需再调用 `overtime get`；`overtime get` 仅在需要按已知 overtimeId 查询时使用（包括已删除/被覆盖的历史记录）
- `adjustment search` / `overtime search` 分页字段为 `--page` 和 `--limit`，不传时自动使用默认值 1 / 20
- `group search` 的分页字段为 `--page` 和 `--limit`，不传时自动使用默认值 1 / 20
- `group get` 的 `--group-id` 必填，返回考勤组全量字段；如仅需成员/地址/蓝牙/Wifi，优先使用 `group filtered-get` 节省成本。**返回结果中如含成员 userId 列表，必须调用 `dws contact user get --ids <userId1>,<userId2>,...`（支持逗号分隔传多个 ID），将 userId 转换为员工姓名后再输出；不得直接输出裸 userId。**
- `group update-members` 的 --group-id 必填，其余参数均可选，但至少需传一个变更项；各参数每次最多 20 个 ID；`--add-extra-users` 和 `--remove-extra-users` 操作的是"无需考勤"豁免名单，不影响考勤组主成员列表
- `group update` 的 --group-id 必填，其余均可选，至少需指定一个修改项；仅需对要修改的字段赋値，未传字段会从已有配置自动补充；修改打卡地址/wifi/蓝牙等复杂子对象时用 `--group-vo` 传入完整 JSON；`--group-vo` 与单字段 flag 同时传入时单字段 flag 优先级更高
- `group create` 的 `--name` 和 `--type` 必填，`--type` 必须为 FIXED/TURN/NONE 之一；type=FIXED 时 `--group-vo` 必须包含 `workDayClassList`（非空）和 `defaultClassId`（非 null）；由于保存考勤组耗时较久，建议加 `--timeout 10`
- `group filtered-get` 的 `--group-id` 必填，`--member/--position/--wifi/--bles` 均可选，默认 false。**返回结果中如含成员 userId 列表，必须调用 `dws contact user get --ids <userId1>,<userId2>,...`（支持逗号分隔传多个 ID），将 userId 转换为员工姓名后再输出；不得直接输出裸 userId。**
- `summary` 必须同时传 `--user`、`--date`、`--stats-type`（week 周统计 / month 月统计），三者缺一即报错；`--date` 支持 `YYYY-MM-DD` 或 `yyyy-MM-dd HH:mm:ss`
- `rules` 的 `--date` 支持 YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss 两种格式
- `selfsetting get/save` 的 `--setting-scene` 必须是 `checkRemind`、`fastCheck`、`checkResultNotify`、`lackRemind`、`personalAttendStatNotify`、`bossAttendStatNotify` 之一
- `selfsetting get/save` 的 MCP 入参 `userId` 为必填；CLI 的 `--user` 也必填，必须显式传入目标用户 ID
- `selfsetting save` 必须传入与 `--setting-scene` 对应的至少一个设置字段；不同场景的字段不能混用
- `selfsetting save` 是敏感写操作，AI 调用时必须先执行 `selfsetting get` 查询当前值，并向用户展示目标用户、设置场景、修改字段、“当前值 → 新值”和最终命令参数摘要；必须调用 `ask_human` 或返回待确认状态等待用户明确确认；用户确认后才允许追加全局 `--yes` 执行保存。禁止未经确认直接执行或自动添加 `--yes`
- `selfsetting get/save` 不需要传 `--corp-id` / `--op-user`，`corpId` 和 `opUserId` 由当前登录上下文自动补齐
- `report columns` 无需额外参数，corpId 和 operatorId 由系统自动传入
- `report query-data` 和 `report query-leave` 的 `--start/--end` 格式: yyyy-MM-dd HH:mm:ss，间隔不超过 32 天，最多 20 人
- report 系列接口仅对管理员开放
- 用户 ID 需从 `contact user get-self` 或 `aisearch person` 获取
- 考勤组 ID 需从 `rules` 命令返回结果中获取
- `vacation types` 无需任何参数，认证信息自动注入
- `vacation balance` 的 `--users` 为目标员工 ID 列表，逗号分隔；`--leave-code` 服务端要求非空、实际必填（`--help` 标"选填"不准），不传返回 `INVALID_PARAMS`，可通过 `vacation types` 获取
- `vacation records` 的 `--start/--end` 使用 YYYY-MM-DD 格式，CLI 自动转换为毫秒时间戳；`--leave-code` 必填（不传无法查询），底层 MCP 工具为 `get_leave_balance_records_v2`
- `vacation balance` 和 `vacation records` 的认证参数（corpId、opUserId）由系统自动注入，无需手动传入
- `vacation update-type` 的 `--leave-code` 必填；其他字段均为可选，但至少需传一个更新字段
- `vacation update-type` 的 `--visibility-rules` 为 JSON 数组字符串，格式：`[{"type":"dept","visible":["1","2","3"]}]`，type 可取值 staff/label/dept
- `vacation save-balance` 是 **SET 接口**而非 ADD 接口：传入值会直接替换当前余额，而非累加
- `vacation save-balance` 的 `--num` 输入为实际天数（如 8 或 7.5），内部会乘以 100 传给 MCP（如 800 或 750）
- `vacation save-balance` 执行前需先调用 `vacation balance` 查询当前余额，再计算新值，避免误操作
- `vacation save-balance` 的 `--start/--end` 使用 YYYY-MM-DD 格式，CLI 自动转换为毫秒时间戳
- `vacation update-type` 和 `vacation save-balance` 执行前会展示待写入数据，需用户输入 yes/y 确认后提交
- 假期编码为 UUID 格式字符串，可通过 `vacation types` 命令查询获取

### 改签打卡记录

**命令**: `dws attendance boss-check`

**功能**: 改签打卡记录，管理员可修改员工的打卡时间、打卡结果等信息。

**强制执行流程**: 此命令为写操作，Agent 调用时必须遵守以下流程：
1. 先向用户展示待执行操作的完整参数摘要
2. 使用 `ask_human` 或返回待确认状态，等待用户明确确认
3. 用户确认后，再传 `--user-say-yes=true` 执行命令
4. **禁止**未经用户确认直接执行或自动添加 `--user-say-yes=true`

**参数**:
| 参数 | 必填 | 说明 | 来源 |
|------|------|------|------|
| --plan-id | y* | 排班ID（与 --result-id 二选一） | `dws attendance schedule get` 返回的 `id` 字段 |
| --result-id | y* | 打卡结果ID（与 --plan-id 二选一，优先使用） | **暂不支持**（record get 未返回此字段） |
| --time | n | 新打卡时间，格式 yyyy-MM-dd HH:mm | - |
| --result | n | 打卡结果枚举值 | - |
| --absent-min | n | 缺勤时长（分钟） | - |
| --remark | n | 备注，最长500字符 | - |
| --user-say-yes | n | 用户已确认，跳过交互式确认提示 | - |

**获取 planId 步骤**:
1. 查询排班记录：`dws attendance schedule get --userIdList USER_ID --workDateBegin DATE --workDateEnd DATE`
2. 从返回结果中找到对应打卡类型（OnDuty=上班，OffDuty=下班）的记录
3. 使用该记录的 `id` 字段作为 `--plan-id` 参数
4. 示例返回：`{"id": 948964045503, "checkType": "OffDuty", ...}` → `--plan-id 948964045503`

**打卡结果枚举值**:
- Normal: 正常
- TimesResultA: 迟到
- TimesResultB: 早退
- TimesResultC: 缺卡
- TimesResultD: 迟到+早退
- TimesResultE: 缺卡+早退
- TimesResultF: 迟到+缺卡

**示例**:
```bash
# 步骤1：获取排班记录的 planId
dws attendance schedule get --userIdList 03642229451220076 --workDateBegin 2026-05-13 --workDateEnd 2026-05-13 -f json

# 步骤2：使用返回的 id 作为 --plan-id 改签
# 假设返回 id: 948964045503 (OffDuty 下班打卡)
dws attendance boss-check --plan-id 948964045503 --result Normal --user-say-yes

# 同时修改打卡时间
dws attendance boss-check --plan-id 948964045503 --time "2026-05-13 18:00" --result Normal --yes
```

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [attendance_my_record.py](../../scripts/attendance_my_record.py) | 查看我今天/指定日期的考勤记录 | `python attendance_my_record.py today` |
| [attendance_team_shift.py](../../scripts/attendance_team_shift.py) | 查询团队成员本周排班 | `python attendance_team_shift.py --users userId1,userId2` |
| [attendance_report_common.py](../../scripts/attendance_report_common.py) | 考勤报表导出公共模块（不可单独执行） | — |
| [attendance_vacation_balance.py](../../scripts/attendance_vacation_balance.py) | 假期余额列表 Excel 导出 | **禁止直接调用**，必须先读 [attendance-vacation.md](./attendance-vacation.md) 按工作流执行 |
| attendance_report_detail.py | 考勤报表 — **明细粒度** |  **禁止直接调用**，必须先读 [attendance-report.md](./attendance-report.md) 按工作流执行 |
| attendance_report_monthly.py | 考勤报表 — **月度汇总** |  **禁止直接调用**，必须先读 [attendance-report.md](./attendance-report.md) 按工作流执行 |
| attendance_report_daily.py | 考勤报表 — **每日统计** |  **禁止直接调用**，必须先读 [attendance-report.md](./attendance-report.md) 按工作流执行 |
| attendance_schedule_import.py | 排班导入（含校验、回显、执行） | **禁止直接调用**，必须先读 [attendance-schedule.md](./attendance-schedule.md) 按工作流执行 |
| attendance_schedule_export.py | 排班查询导出（分批查询、排班表 Excel） | **禁止直接调用**，必须先读 [attendance-schedule.md](./attendance-schedule.md) 按工作流执行 |

> 说明：
> - `attendance_report_*.py` 三个脚本由 [attendance-report.md](./attendance-report.md) 工作流编排使用，自动处理 `--users` 超过 20 人分批、`--start/--end` 超过 32 天按月切片，输出 `attendance_report_<startDate>_<endDate>_<粒度>.xlsx`
> - `attendance_schedule_import.py` 由 [attendance-schedule.md](./attendance-schedule.md) 排班导入工作流编排使用，自动处理考勤组校验、班次校验、排班回显确认
> - `attendance_schedule_export.py` 由 [attendance-schedule.md](./attendance-schedule.md) 排班查询导出工作流编排使用，自动处理分批查询（超 20 人自动分批）、userId→姓名转换、classId→班次名称转换、输出排班表格式 Excel

## 严格约束
- 不要凭历史记忆复用 userId / classId / leaveCode / groupId / instanceId 等任何 ID，每次必须从当次命令返回值中提取
- 不要猜测命令，先查询明确命令，不要使用记忆中未经证实（即不存在于本.md文件中的指令）或执行失败（指令传参有误或flag不正确）的的命令
- 制定 plan 并自我审查，严格按 plan 执行
- 涉及超过 3 条记录的聚合（求和、分组、计数、排序、跨字段计算）时必须落 Python 脚本处理，禁止用大模型口算或目测。脚本里如果用到 mcp，先提前看下 mcp 返回的结构，避免执行异常
- 遇到时长字段时，注意区分单位是秒、分钟还是小时
- 遇到意图不清晰的场景不要猜测，主动询问用户明确意图
- 如果查询结果很多时，不要自作主张省略，必须明确告知用户或者用表格或展示所有。
