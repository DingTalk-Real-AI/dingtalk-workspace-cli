# 考勤排班导入与排班表导出

只在以下两类任务加载本 Reference：

- 写：导入排班、给员工排班、调班、换班、排休、临时安排后恢复。
- 读：明确要求把排班导出为 Excel/排班表。

只需查看排班数据或对照班次时直接使用 `attendance schedule get` / `attendance shift list`，按 [attendance.md](attendance.md) 执行，不加载本文件。

## 入口选择

| 意图 | 唯一推荐入口 | 说明 |
|---|---|---|
| 导入/修改/恢复排班 | `python3 scripts/attendance_schedule_import.py` | 脚本负责组类型、班次归属、记录格式和确认门禁；不要手拼底层 import payload |
| 导出排班 Excel | `python3 scripts/attendance_schedule_export.py` | 脚本分批查询、补齐姓名/班次名并生成日历表 |
| 仅查看 JSON | `dws attendance schedule get --users ... --start ... --end ... --format json` | 无需脚本和 Excel |
| 仅看 7 天计划上下班/休息日 | `dws attendance shift list --users ... --start ... --end ... --format json` | 最多 50 人、7 天 |

已知上述入口时不要先查 Help、产品 Schema 或 Shortcut 列表。只有脚本真实报 `unknown command/flag` 时查一次对应 leaf Help；参数/确认不明时只查一次精确 leaf compact Schema。

`schedule get` 的 CLI 日期入参仍使用 `YYYY-MM-DD` 或 `YYYY-MM-DD HH:mm:ss`；CLI 会把日期规范化为上游要求的 datetime 字符串。不要自行换算或传 Unix 毫秒时间戳。

性能优化只能消除重复发现和重复查询，不能省略排班记录、日期、班次身份、休息状态、失败项或写后回读。需要完整范围时必须覆盖所有目标用户和日期；空记录、休息日和查询不完整必须分别表达。

## 排班写入 Golden Route

### 1. 解析真实目标

- 人名：`dws aisearch person --query "<姓名>" --dimension name --format json`。零命中或多候选时停止。
- 明确给出考勤组名称：`dws attendance group search --query "<名称>" --type TURN --page 1 --limit 20 --format json`，必须唯一命中。
- 只给“我的考勤组”：可用 `dws attendance rules --date <YYYY-MM-DD> --format json` 取得当前用户 groupId；该命令不能据此推断其他员工的考勤组。
- 部门成员：先唯一解析部门，再 `dws contact dept list-members --ids <deptId> --format json`；不要把 deptId 当 userId。

所有 ID 都必须来自本轮同一 profile 的真实返回，禁止复用历史 ID。

### 2. 校验考勤组与班次

执行：

```bash
dws attendance group get --group-id <GROUP_ID> --format json
```

必须满足：

- `groupVO.type == TURN`。FIXED/NONE 不能导入排班。
- 目标用户属于该组；需要时用 `group filtered-get --group-id <GROUP_ID> --member` 验证。
- 班次只能来自该组的 `shiftVOList` / `classIds`。先在组返回的班次中按名称匹配；零命中或多候选停止。
- 只有组结果缺班次名称时，才按组内已有 classId 调 `class get` 补齐；禁止用企业全局 `class search` 随意选一个同名班次。

### 3. 写前保存与预览

临时修改/恢复或覆盖已有日期时，先保存原排班：

```bash
dws attendance schedule get --users <USER_IDS> --start <START> --end <END> --format json
```

构建非空 JSON 数组，每项至少包含：

```json
{"userId":"user001","workDate":"2026-08-31","classId":123456,"isRest":"N"}
```

排休使用真实约定 `isRest="Y"`，并保留脚本/当前接口要求的 classId。向用户展示完整预览：考勤组、员工姓名、日期、星期、班次名称/排休、覆盖范围。不能只显示 userId/classId。

`schedule import` 是 `confirmation=user_required`。只有用户确认了这份完整预览后，才能进入执行；确认前可用脚本 `--dry-run` 做零写入校验。

### 4. 执行

```bash
python3 scripts/attendance_schedule_import.py \
  --group-id <GROUP_ID> \
  --schedules '<JSON数组>' \
  --confirm
```

脚本会二次校验 TURN、组内班次和记录格式，再调用当前真实 leaf（底层参数为 `--groupId/--scheduleVOS`）。Agent 不要绕过脚本改用旧的 `--group-id/--schedules` 原子命令写法。

### 5. 回读、恢复与结果

- 写后用相同 users/date 调 `schedule get`，逐项核对 userId、workDate、classId/isRest；只凭脚本退出码不能宣称成功。
- 用户要求“临时修改后恢复”时，用步骤 3 保存的原排班恢复，再次回读。原来无排班与休息日必须按真实结构区分，禁止编造默认班次。
- 修改已提交但回读失败时，不要盲目重放；先保留目标范围并再次只读对账。无法确定时报告 commit-unknown。
- 批量中部分员工失败时逐项列出成功/失败/未知；不得把部分成功表述为全部成功。

## 排班表 Excel 导出

### 1. 收集范围

- 员工范围必填：指定员工、部门、考勤组成员或明确 userId 列表。
- 日期范围必填，格式 `YYYY-MM-DD`。本周=周一到周日，本月=1 日到真实月末；使用当前会话时区。
- 人员/日期缺失时一次性询问；不追问输出文件名，缺省交给脚本生成。

### 2. 执行

```bash
python3 scripts/attendance_schedule_export.py \
  --users <USER_ID_1,USER_ID_2> \
  --start <YYYY-MM-DD> \
  --end <YYYY-MM-DD> \
  [--output <相对路径.xlsx>]
```

脚本自动完成：按 20 人分批调用 `schedule get`、userId→姓名、classId→班次名称、按人×日期生成 Excel。不要在 Agent 中重复查同一批排班或手工拼表。

只允许工作目录内安全相对输出路径，默认不覆盖已有文件。向用户返回脚本 stdout 的路径、人数、日期范围、记录数和 warning，不粘贴完整 Excel 内容。

## 错误最短路径

1. 考勤组非 TURN、目标不在组内、班次不属于组：写入前停止，展示真实冲突，不换同名目标。
2. 权限错误：提示需要考勤管理员/对应管理范围，不切 profile、不改用底层 API。
3. `unknown flag`：只查真实失败的 leaf Help 一次；不要尝试旧 flag 组合。
4. 脚本依赖缺失：报告缺失依赖，不自动联网安装。
5. 空/异常返回：保留员工和日期范围，按失败或不完整处理；禁止把空结构当作“全部休息”。
