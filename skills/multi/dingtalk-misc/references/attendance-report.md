# 考勤 Excel / 报表导出

仅当用户明确要求“导出、Excel、表格、生成报表”时加载本 Reference。单次 JSON 查询、个人摘要、排班表和假期余额表分别走 [attendance.md](attendance.md)、[attendance-schedule.md](attendance-schedule.md)、[attendance-vacation.md](attendance-vacation.md)。

已知脚本入口时直接执行，不先查 Help、产品 Schema 或原子命令。脚本内部负责数据查询、分批、分页、聚合、姓名补齐和 Excel 生成；Agent 不重复这些远程读取，也不目测聚合。

性能优化只能消除重复发现、重复查询和 Agent 目测聚合，不能删减用户要求的人员、日期、字段、分页、失败项或 warning。脚本输出 partial/incomplete 时必须原样保留，不能用已有部分数据宣称完整。

## 报表类型 Golden Route

| 用户意图 | 脚本 | 说明 |
|---|---|---|
| 未指定粒度的“考勤报表”/明确月度汇总 | `attendance_report_monthly.py` | 默认；按人汇总并生成日历 Sheet |
| 每日统计/按天汇总 | `attendance_report_daily.py` | 按人按日 |
| 打卡明细/原始上下班明细 | `attendance_report_detail.py` | 数据源是打卡结果 + 打卡流水 |
| 请假/补卡/出差/外出审批记录表 | `attendance_report_record.py` | 审批单粒度；必须选择一种记录类型 |
| 签到/外勤签到报表 | `attendance_report_checkin.py` | 签到不是上下班打卡 |

“请假记录”走 record；“请假时长统计/请假报表”在未要求审批单明细时走 monthly/daily，并通过列关键词选取假期字段。“导出排班表”不属于本文件。

## 1. 解析范围

- 人员范围必填。姓名用 `dws aisearch person --query "<姓名>" --dimension name --format json`；部门先唯一解析 deptId，再获取完整成员；用户已给 userId 时直接使用。
- 零命中、多候选或部门分页未完成时停止，禁止默认第一项或只导出第一页成员。
- 时间范围必填，脚本参数使用 `YYYY-MM-DD`。本周=周一到周日，本月=1 日到真实月末；自定义范围按用户原值。
- 用户未指定报表粒度时默认 monthly，不追问；执行结果中说明“已按月度汇总生成”。人员或时间缺失时一次性询问。
- 所有解析和脚本执行保持同一 profile；不得复用历史 userId/deptId。

## 2. 选择列

monthly/daily 支持 `--column-keywords`；detail/record/checkin 列固定，不传该参数。

| 关注维度 | `--column-keywords` |
|---|---|
| 未指定 | 省略，使用脚本默认列 |
| 加班 | `加班-审批单统计,加班总时长,考勤结果` |
| 请假/出差/外出 | `请假,出差时长,外出时长,考勤结果` |
| 异常/迟到早退/缺卡 | `迟到次数,迟到时长,严重迟到次数,严重迟到时长,旷工迟到次数,早退次数,早退时长,上班缺卡次数,下班缺卡次数,旷工天数,考勤结果` |
| 其他明确字段 | 使用用户原词组成逗号列表 |

字段 ID 由脚本实时调用 `attendance report columns` 建立映射，禁止硬编码。无权限字段、未匹配字段和无数据字段必须分别保留在 warning 中，不能都说成 0。

## 3. 执行脚本

### 月度汇总

```bash
python3 scripts/attendance_report_monthly.py \
  --users <USER_IDS> --start <YYYY-MM-DD> --end <YYYY-MM-DD> \
  [--column-keywords "<关键词>"] [--out <相对路径.xlsx>]
```

### 每日统计

```bash
python3 scripts/attendance_report_daily.py \
  --users <USER_IDS> --start <YYYY-MM-DD> --end <YYYY-MM-DD> \
  [--column-keywords "<关键词>"] [--out <相对路径.xlsx>]
```

### 打卡明细

```bash
python3 scripts/attendance_report_detail.py \
  --users <USER_IDS> --start <YYYY-MM-DD> --end <YYYY-MM-DD> \
  [--out <相对路径.xlsx>]
```

### 审批记录

```bash
python3 scripts/attendance_report_record.py \
  --type <leave|trip|out|patch> \
  --users <USER_IDS> --start <YYYY-MM-DD> --end <YYYY-MM-DD> \
  [--out <相对路径.xlsx>]
```

映射：请假/年假/调休/病假=`leave`，出差=`trip`，外出=`out`，补卡=`patch`。用户同时要求多种记录时按类型分别生成或明确给出多个文件，禁止把一种类型冒充完整集合。

### 签到报表

```bash
python3 scripts/attendance_report_checkin.py \
  --users <USER_IDS> --start <YYYY-MM-DD> --end <YYYY-MM-DD> \
  [--out <相对路径.xlsx>]
```

仅使用工作目录内安全相对输出路径；默认不覆盖已有文件。`--inspect` 仅在真实返回结构漂移、脚本明确提示时使用一次，不作为正常流程。

## 脚本负责的边界

- [attendance_report_common.py](../scripts/attendance_report_common.py) 是上述导出脚本共享的内部模块，不可单独执行。
- monthly/daily：实时列权限、最多 20 人/请求、最多 32 天/请求，按月切片并聚合。
- detail：打卡结果自动翻页、打卡流水按用户/月份分批，区分无流水与 NotSigned/Absenteeism。
- record：查询考勤审批摘要、按真实实例取详情并拆行；不代替用户提交审批。
- checkin：最多 7 天一段并分批查询，按签到时间/地点整理；不混用 check record。
- 所有脚本：userId→姓名、原始单位保留、失败/warning 统计、xlsx 输出。

Agent 不应先手动执行 `report columns/query-data`、`check result/record` 或 `approve list` 再让脚本重复查询。只有脚本报出一个明确、可复现的结构问题时，才做一次精确 leaf Schema/Help 审计。

## 结果与失败

- 返回脚本 stdout 中的输出路径、人员数、日期范围、记录数、使用列和 warning；不要把整个 Excel 粘贴到对话。
- 只有文件成功写入且脚本完成数据核对后报告成功。部分用户/字段/日期失败时报告 partial，不把其余数据当成完整报表。
- 403/权限错误：说明需要考勤管理员或相应管理范围，不重试、不切身份。
- 无人员、无可见字段、日期非法：本地停止，零业务查询；让用户修正输入。
- 无业务数据：可以生成空表但必须明确“查询成功、无记录”；不能转换成“所有人正常”。
- 脚本依赖缺失：报告缺失依赖，不自动联网安装。
- 远端超时且请求可能已完成时，先检查目标文件和脚本摘要；不要重新发起整批导出造成重复查询。
