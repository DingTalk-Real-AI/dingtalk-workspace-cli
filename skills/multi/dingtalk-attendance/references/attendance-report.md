# 考勤报表导出参考 (attendance-report)

> 本文档由 `attendance.md` 路由调用。当用户提到"考勤报表"、"导出考勤"、"出勤汇总"、"考勤明细"、"迟到早退统计"、"全员考勤数据"、"某月考勤统计"、"考勤表格"、"考勤 Excel" 时，应阅读本文档执行。
> 不适用于：个人单日打卡查询（用 `attendance check record`）、班次查询（用 `attendance schedule get`）、假期余额（用 `vacation balance`）、审批进度（用 `oa`）。

##  强制门禁（必须先读完本文档才能执行）

**任何调用 `attendance_report_detail.py` / `attendance_report_monthly.py` / `attendance_report_daily.py` 的请求，都必须经过本文档定义的工作流，严禁绕过本文档直接拼脚本命令执行。**

违反将出现以下任一问题：
1. 未按"阶段 1"做人员解析 → `--users` 传入部门 ID 而非员工 userId，脚本虽内置回退但会浪费一次失败的接口调用
2. 未按"阶段 0"判断报表类型 → 用户说"汇总"被理解成"明细"，导致输出粒度错误
3. 未按"列选择"判断是否传 `--column-keywords` → 用户要"迟到情况报表"被输出成全字段默认报表
4. 未按"错误处理"规则处理 403 / `HSF_ILLEGALPARAMS` → 把环境错误当成业务错误反馈给用户
5. 未按"阶段 4"返回结果 → 把 Excel 内容贴在对话里，或者裸 userId 直接输出

**执行前自检（必须能在心中回答）**：
- [ ] 报表类型是？（明细 / 月度汇总 / 每日统计）
- [ ] 人员列表的来源是？（`aisearch person` 还是 `contact dept list-members`？）
- [ ] 列选择方式是？（预设报表关键词 / 自定义 `--column-keywords` / 默认列集合）
- [ ] 报错时如何向用户解释？

如果上述任何一项答不出，**回到本文档对应章节重新阅读**，禁止凭记忆/想象组装命令。

**前提**：当前用户必须是钉钉管理员，否则 report 系列接口返回 403 权限错误。

## 核心原则

Agent 解析用户意图（报表类型、人员范围、时间范围、关注维度），获取 userId 列表后，**直接调用对应的 Python 脚本 CLI 生成 Excel**。
- **脚本自包含**：数据查询（分批、分段、翻页）、字段解析、聚合计算、Excel 生成全部由脚本内部完成，Agent 不参与数据查询和计算
- **月度汇总 / 每日统计**：脚本内部调用 `report columns` + `report query-data`
- **明细**：脚本内部调用 `check result` + `check record`（数据源不同）
- 列选择是独立维度：用户未指定关注维度时脚本使用内置默认字段；用户指定了关注维度时 Agent 通过 `--column-keywords` 参数传给脚本

## 严格禁止 (NEVER DO)

- 禁止凭历史记忆复用 userId 等任何 ID，必须从当次命令返回值中提取
- 禁止用大模型口算/目测做考勤数据聚合（求和、计数、分组），必须通过 Python 脚本完成
- 禁止 Agent 直接调用 `report query-data` / `report columns` / `check result` / `check record`，这些由脚本内部自动完成
- 禁止 `dws` 命令缺省 `--format json`（Agent 仅在阶段 1 获取人员时直接调用 dws 命令）
- 禁止编造任何字段值或用户姓名
- 禁止直接输出裸 userId，脚本已内置 userId → 姓名转换

## 严格要求 (MUST DO)

- 所有 `dws` 命令必须携带 `--format json`
- 时间参数 `--start` / `--end` 格式必须为 `yyyy-MM-dd HH:mm:ss`
- 字段 ID 与字段名的映射必须从 `report columns` 实时建立，禁止硬编码
- 任何接口失败（含 403）必须向用户清晰报错，禁止静默吞掉

## 涉及工具

| 工具 | 用途 | 安全等级 |
|------|------|---------|
| `dws attendance report columns` | 获取当前管理员可见的考勤字段清单（字段 ID → 字段名） | 只读 |
| `dws attendance report query-data` | 按字段查询考勤数据（≤20 人/次，≤32 天/次） | 只读 |
| `dws attendance report query-leave` | 按假期名称查询假期数据（≤20 人/次，≤32 天/次），月度汇总/每日统计的"请假"列由脚本自动调用 | 只读 |
| `dws attendance approve list` | 查询审批单记录（考勤记录报表专用），支持类型：leave/trip/out/patch | 只读 |
| `dws oa approval detail` | 获取审批单详情（考勤记录报表专用），解析 formValueVOS / extValue | 只读 |
| `dws attendance check result` | 查询打卡结果（≤100 人/次，≤1 月，明细报表专用） | 只读 |
| `dws attendance check record` | 查询打卡流水（≤1 月，明细报表专用） | 只读 |
| `dws aisearch person` | 按姓名搜索用户获取 userId（搜人首选） | 只读 |
| `dws contact user get` | 批量查询 userId → 用户信息（姓名/部门/工号/职位） | 只读 |
| `dws contact dept search` | 搜索部门获取 deptId | 只读 |
| `dws contact dept list-members` | 获取部门成员 userId 列表 | 只读 |

## 意图判断

### 报表类型（五选一）

| 用户说 | 报表类型 | 映射脚本 |
|--------|---------|---------|
| "导出研发部 3 月份的考勤明细" / "每条打卡记录" / "明细" / "原始记录" | 明细 | `attendance_report_detail.py` |
| "生成研发部 3 月考勤汇总" / 用户明确说"月度汇总" / 用户未指明类型 | **月度汇总（默认）** | `attendance_report_monthly.py` |
| "导出研发部 3 月每天的出勤情况" / "按天统计" / "每日" / 用户明确说"每日统计" | 每日统计 | `attendance_report_daily.py` |
| "导出研发部 4 月的请假记录" / "补卡记录" / "出差记录" / "外出记录" / "xx记录" | 考勤记录 | `attendance_report_record.py` |
| "导出签到记录" / "签到报表" / "签到数据导出" / "签到明细" / "外勤签到" | 签到报表 | `attendance_report_checkin.py` |

> **默认报表类型**：用户未指明报表类型时，**默认走月度汇总**，事后告知"已按月度汇总输出，如需明细/每日统计/考勤记录请告知"。
>
> **考勤记录 vs 其他报表**：当用户明确提到"请假记录"/"补卡记录"/"出差记录"/"外出记录"时，走考勤记录报表（数据源为审批单）。而"请假报表"/"出差时长统计"等走月度汇总（数据源为 report query-data）。区别在于：考勤记录导出的是**审批单维度的原始数据**（含审批单状态、每天明细），月度汇总导出的是**按人按月聚合后的统计数据**。

### 列选择（独立维度，与报表类型正交）

> **"报表类型"与"列选择"是两个独立维度，需分别判断。**
> 例如用户说"帮我出一份加班报表"：报表类型未指明 → 默认月度汇总；列选择命中"加班报表" → 使用加班预设关键词。
> 例如用户说"帮我出每日的异常报表"：报表类型命中"每日" → 每日统计脚本；列选择命中"异常报表" → 使用异常预设关键词。
> 预设报表**不会改变报表类型的判断逻辑**，报表类型始终按下方「报表类型（三选一）」规则判断。

| 用户说 | 列选择方式 |
|--------|-----------|
| "帮我出一份考勤报表" / "导出考勤" / 未提及特定关注维度 | 不传 `--column-keywords`，使用脚本内置**默认列集合** |
| "加班报表" / "加班统计" / "加班时长报表" | 传 `--column-keywords`，使用下方「加班报表预设关键词」 |
| "请假报表" / "请假出差报表" / "请假外出统计" | 传 `--column-keywords`，使用下方「请假报表预设关键词」 |
| "异常报表" / "异常考勤" / "迟到早退报表" / "缺卡报表" | 传 `--column-keywords`，使用下方「异常报表预设关键词」 |
| 提及了其他自定义关注维度（如"工作时长报表"） | 传 `--column-keywords`，由 Agent 自行拼接关键词 |

> **预设报表优先级**：当用户提到的关键词同时命中"预设报表"和一般自定义维度时，**优先使用预设报表的完整关键词列表**，确保列不遗漏。

### 易混淆场景

| 用户说 | 应路由到 |
|--------|---------|
| "今天打卡了吗" | `dws attendance record get`（单次查询，非报表） |
| "帮我排班" | `dws attendance schedule import`（排班写入，非报表；执行前确认） |
| "我的假期还剩多少" | `dws attendance vacation balance`（单次查询） |
| "帮我请假" | `dws oa`（审批流程，非报表） |
| "我这个月的考勤怎么样" | `dws attendance summary --user <USER_ID> --date <YYYY-MM-DD> --stats-type month`（个人统计，非报表） |

## 工作流

### 阶段 0: 参数解析与确认查询范围

1. **解析用户输入**（两个独立维度）：
   - **报表类型**（四选一）：明细 / 月度汇总（默认） / 每日统计 / 考勤记录
   - **列选择**（独立维度）：预定义列集合（默认） / 用户指定维度筛选（考勤记录不适用）
   - **人员维度**：指定员工 / 某个部门 / 多个部门（暂不支持全公司查询）
   - **时间维度**：本周 / 本月 / 自定义时间段
   - **记录子类型**（仅考勤记录）：leave(请假) / trip(出差) / out(外出) / patch(补卡)
2. **缺失信息处理**：
   - **报表类型** 缺失 → 默认走月度汇总（不追问），事后告知
   - **列选择**：用户提及了特定关注维度 → 传 `--column-keywords`；未提及 → 使用脚本内置默认字段集
   - **用户范围 / 时间范围** 缺失 → 追问，禁止猜测

### 阶段 1: 获取完整人员列表

**场景 A — 指定员工姓名**:
```bash
dws aisearch person --keyword "<员工姓名>" --dimension name --format json
```

**场景 B — 按部门查询**:
```bash
dws contact dept search --query "<部门名>" --format json
dws contact dept list-members --depts <deptId> --format json
```

**场景 C — 多个部门**: 对每个部门分别执行 B，汇总去重。

**场景 D — 全公司**: 暂不支持，引导用户指定部门。

**场景 E — 用户已给 userId 列表**: 直接跳过本步。

### 阶段 2: 列选择（决定脚本参数）

> Agent 不需要手动调用 `report columns`，字段获取由脚本内部完成。Agent 只需根据用户意图决定是否传 `--column-keywords` 参数。

**判断顺序**（优先级从高到低）：

1. **明细报表** → 列固定，不支持 `--column-keywords`
2. **用户提到预设报表关键词** → 传 `--column-keywords`，使用本文档「预设报表列集合」中定义的完整关键词列表：
   - "加班报表" / "加班统计" / "加班时长" → 使用「加班报表预设关键词」
   - "请假报表" / "请假出差" / "外出统计" → 使用「请假报表预设关键词」
   - "异常报表" / "迟到早退" / "缺卡报表" / "异常考勤" → 使用「异常报表预设关键词」
3. **用户提及了其他自定义关注维度** → 传 `--column-keywords "..."`
4. **用户未提及特定关注维度** → 不传 `--column-keywords`，脚本使用内置默认字段集

### 阶段 3: 调用脚本生成 Excel

#### 月度汇总 / 每日统计

```bash
python scripts/attendance_report_monthly.py \
  --users <userId1>,<userId2>,... \
  --start "<yyyy-MM-dd>" \
  --end "<yyyy-MM-dd>" \
  [--column-keywords "出勤天数,迟到次数,迟到时长,..."] \
  [--out 月度汇总_研发部_202604.xlsx]
```

参数说明：
- `--users`（必填）：逗号分隔的 userId 列表
- `--start`（必填）：开始日期，支持 `YYYY-MM-DD` 或 `YYYY-MM-DD HH:mm:ss`
- `--end`（必填）：结束日期，同上
- `--column-keywords`（可选）：逗号分隔的字段名关键词。不传则使用脚本内置默认字段集。预设报表（加班/请假/异常）也通过本参数传入对应的预设关键词列表
- `--out`（可选）：输出文件名，不传则自动生成

脚本内部自动处理：`report columns` 获取字段清单 → 按预设/关键词匹配 → `report query-data` 分批分段查询 → `contact user get` 姓名映射 → 聚合计算 → 生成 Excel

#### 明细

```bash
python scripts/attendance_report_detail.py \
  --users <userId1>,<userId2>,... \
  --start "<yyyy-MM-dd>" \
  --end "<yyyy-MM-dd>" \
  [--out 考勤明细_研发部_202604.xlsx]
```

- **没有 `--column-keywords`**，明细列固定
- 数据来源不同：`check result` + `check record`（非 `report query-data`）
- 分批限制：≤100 人/次（而非 20 人）

#### 考勤记录

```bash
python scripts/attendance_report_record.py \
  --type <leave|trip|out|patch> \
  --users <userId1>,<userId2>,... \
  --start "<yyyy-MM-dd>" \
  --end "<yyyy-MM-dd>" \
  [--out 请假记录_研发部_202604.xlsx]
```

参数说明：
- `--type`（必填）：记录类型，支持 `leave`(请假) / `trip`(出差) / `out`(外出) / `patch`(补卡)
- `--users`（必填）：逗号分隔的 userId 列表
- `--start`（必填）：开始日期 `YYYY-MM-DD`
- `--end`（必填）：结束日期 `YYYY-MM-DD`
- `--out`（可选）：输出文件名，不传则自动生成
- **没有 `--column-keywords`**，列由 `--type` 决定

脚本内部自动处理：`attendance approve list` 获取审批摘要 → `oa approval detail` 获取详情 → 解析 DDHolidayField / extValue → 按天拆行 → `contact user get` 姓名映射 → 生成 Excel

**记录类型选择规则**（Agent 需从用户意图中判断）：

| 用户说 | --type 值 |
|--------|----------|
| "请假记录" / "年假记录" / "调休记录" / "病假记录" | `leave` |
| "出差记录" | `trip` |
| "外出记录" | `out` |
| "补卡记录" | `patch` |

#### 签到报表

```bash
python scripts/attendance_report_checkin.py \
  --users <userId1>,<userId2>,... \
  --start "<yyyy-MM-dd>" \
  --end "<yyyy-MM-dd>" \
  [--out 签到报表_研发部_20260401_20260407.xlsx]
```

参数说明：
- `--users`（必填）：逗号分隔的 userId 列表
- `--start`（必填）：开始日期 `YYYY-MM-DD` 或 `YYYY-MM-DD HH:mm:ss`
- `--end`（必填）：结束日期，同上
- `--out`（可选）：输出文件名，不传则自动生成
- **没有 `--column-keywords`**，签到报表列固定

脚本内部自动处理：`attendance checkin records` 分批分段查询（每批 50 人，每段 7 天）→ `contact user get` 姓名/部门映射 → 时间戳转日期+时间 → 图片列展开（最多 9 张）→ 生成 Excel

> **注意**：签到接口时间限制为 7 天（不同于考勤报表的 32 天），脚本会自动按 7 天分段查询。

#### 脚本执行注意事项

- 脚本依赖 `openpyxl`，若未安装需先 `pip install openpyxl`
- 脚本摘要输出到 stdout，进度日志输出到 stderr
- 首次调试可加 `--inspect` 参数查看首条记录原始结构
- 脚本执行失败（exit ≠ 0）时，stderr 中有具体错误信息

### 阶段 4: 返回结果给用户

- 将脚本 stdout 输出的摘要信息原样转告用户
- 如果脚本输出 warning，原样转告用户
- 如果走的是默认月度汇总，追加："已按月度汇总输出，如需明细/每日统计请告知"
- **不要把 Excel 内容贴在对话里**，只给路径和摘要

## 输出文件结构

### 月度汇总（双 sheet，自动生成）

`attendance_report_monthly.py` 输出的 Excel 文件包含 **2 个 sheet**：

| Sheet 名 | 布局 | 用途 |
|---------|------|------|
| `月度汇总` | 每人 1 行，列为基础信息 + 聚合字段 + 请假展开 + 考勤结果按天展开 | 整月数据汇总速览 |
| `日历表` | 每人 3 行（班次名称/考勤结果/工作时长），列为基础信息 + 指标 + 1日~N日 | 钉钉日历视图，逐日查看 |

**日历表结构示意**：

| 姓名 | 考勤组 | 部门 | 指标 | 1日 | 2日 | ... | 30日 |
|------|--------|------|------|------|------|------|------|
| 张三 | 研发组 | 技术部 | 班次名称 | 早班 | 早班 | ... | 休息 |
|       |       |       | 考勤结果 | 正常 | 迟到 | ... | — |
|       |       |       | 工作时长 | 8 | 7.5 | ... | 0 |
| 李四 | 研发组 | 技术部 | 班次名称 | 晚班 | 晚班 | ... | 早班 |
| ... | ... | ... | ... | ... | ... | ... | ... |

- 基础列（姓名/考勤组/部门）已纵向 3 行合并
- 日历表的 3 个指标字段（`班次名称`/`考勤结果`/`工作时长`）由脚本**强制**追加到 `report query-data` 查询字段中（即使用户的 `--column-keywords` 没包含），确保日历表非空
- 日历表数据来源与月度汇总相同（同一次 `report query-data` 调用），不会增加接口次数

## 预定义列集合

### 月度汇总（3 个基础信息列 + 18 个考勤数据列）

**基础信息列**（脚本自动从 `contact user get` 和原始记录中提取）：

| 序号 | 字段名称 | 数据来源 |
|------|---------|---------|
| 1 | 姓名 | `contact user get --ids` |
| 2 | 考勤组 | `attendance group search` + `filtered-get --member` 反向映射 |
| 3 | 部门 | `contact user get --ids` |

**考勤数据列**（从 `report columns` 中按名称精确匹配）：

| 序号 | 字段名称 |
|------|---------|
| 4 | 出勤天数 |
| 5 | 休息天数 |
| 6 | 工作时长 |
| 7 | 迟到次数 |
| 8 | 迟到时长 |
| 9 | 严重迟到次数 |
| 10 | 严重迟到时长 |
| 11 | 旷工迟到次数 |
| 12 | 早退次数 |
| 13 | 早退时长 |
| 14 | 上班缺卡次数 |
| 15 | 下班缺卡次数 |
| 16 | 旷工天数 |
| 17 | 出差时长 |
| 18 | 外出时长 |
| 19 | 请假（按假期类型展开为 4 列：`请假-事假`、`请假-调休`、`请假-病假`、`请假-年假`，值为月度求和；数据由脚本通过 `report query-leave` 单独查询） |
| 20 | 加班-审批单统计 |
| 21 | 考勤结果（按天展开为多列：1日/2日/.../31日，每列显示当天考勤状态） |

### 每日统计（4 个基础信息列 + 31 个考勤数据列）

**基础信息列**：

| 序号 | 字段名称 | 数据来源 |
|------|---------|---------|
| 1 | 姓名 | `contact user get --ids` |
| 2 | 考勤组 | `attendance group search` + `filtered-get --member` 反向映射 |
| 3 | 部门 | `contact user get --ids` |
| 4 | 日期 | 查询日期 |

**考勤数据列**：

| 序号 | 字段名称 |
|------|---------|
| 5 | 班次 |
| 6 | 上班1打卡时间 |
| 7 | 上班1打卡结果 |
| 8 | 下班1打卡时间 |
| 9 | 下班1打卡结果 |
| 10 | 上班2打卡时间 |
| 11 | 上班2打卡结果 |
| 12 | 下班2打卡时间 |
| 13 | 下班2打卡结果 |
| 14 | 上班3打卡时间 |
| 15 | 上班3打卡结果 |
| 16 | 下班3打卡时间 |
| 17 | 下班3打卡结果 |
| 18 | 关联的审批单 |
| 19 | 出勤天数 |
| 20 | 休息天数 |
| 21 | 工作时长 |
| 22 | 迟到次数 |
| 23 | 迟到时长 |
| 24 | 严重迟到次数 |
| 25 | 严重迟到时长 |
| 26 | 旷工迟到次数 |
| 27 | 早退次数 |
| 28 | 早退时长 |
| 29 | 上班缺卡次数 |
| 30 | 下班缺卡次数 |
| 31 | 旷工天数 |
| 32 | 出差时长 |
| 33 | 外出时长 |
| 34 | 请假（按假期类型展开为 4 列：`请假-事假`、`请假-调休`、`请假-病假`、`请假-年假`；数据由脚本通过 `report query-leave` 单独查询） |
| 35 | 加班-审批单统计 |

### 预设报表：加班报表

3 个基础信息列（姓名/考勤组/部门）+ 以下考勤数据列：

> 原始配置中的 TITLE_COLUMN（如"加班时长（转调休）"）为分组标题，脚本不支持父子列结构，已打平为叶子字段。

| 序号 | 字段名称 | 说明 |
|------|---------|------|
| 4 | 加班-审批单统计 | — |
| 5 | 加班总时长 | — |
| 6 | 考勤结果 | 按天展开 |

**对应 `--column-keywords`**：`加班-审批单统计,加班总时长,考勤结果`

### 预设报表：请假报表

3 个基础信息列（姓名/考勤组/部门）+ 以下考勤数据列：

| 序号 | 字段名称 | 说明 |
|------|---------|------|
| 4 | 请假 | 按假期类型自动展开（事假/调休/病假/年假等），数据由脚本通过 `report query-leave` 单独查询 |
| 5 | 出差时长 | — |
| 6 | 外出时长 | — |
| 7 | 考勤结果 | 按天展开 |

**对应 `--column-keywords`**：`请假,出差时长,外出时长,考勤结果`

### 预设报表：异常报表

3 个基础信息列（姓名/考勤组/部门）+ 以下考勤数据列：

> 原始配置中的 TITLE_COLUMN（如"迟到"、"早退"、"缺卡"）为分组标题，脚本不支持父子列结构，已打平为叶子字段。

| 序号 | 字段名称 | 说明 |
|------|---------|------|
| 4 | 迟到次数 | 原属分组「迟到」 |
| 5 | 迟到时长 | 同上 |
| 6 | 严重迟到次数 | 同上 |
| 7 | 严重迟到时长 | 同上 |
| 8 | 旷工迟到次数 | 同上 |
| 9 | 早退次数 | 原属分组「早退」 |
| 10 | 早退时长 | 同上 |
| 11 | 上班缺卡次数 | 原属分组「缺卡」 |
| 12 | 下班缺卡次数 | 同上 |
| 13 | 旷工天数 | — |
| 14 | 考勤结果 | 按天展开 |

**对应 `--column-keywords`**：`迟到次数,迟到时长,严重迟到次数,严重迟到时长,旷工迟到次数,早退次数,早退时长,上班缺卡次数,下班缺卡次数,旷工天数,考勤结果`

### 明细（3 个基础信息列 + 10 个打卡字段列）

**基础信息列**：

| 序号 | 字段名称 | 数据来源 |
|------|---------|---------|
| 1 | 姓名 | `contact user get --ids` |
| 2 | 考勤组 | `attendance group search` + `filtered-get --member` 反向映射 |
| 3 | 部门 | `contact user get --ids` |

**打卡字段列**（以打卡流水为主表，每条流水一行；通过打卡时间关联 `check result` 获取考勤时间和打卡结果）：

| 序号 | 字段名称 | 数据来源 |
|------|---------|---------|
| 4 | 考勤日期 | `check record` |
| 5 | 考勤时间 | `check result`（班次规定的上/下班时间，按打卡时间关联） |
| 6 | 打卡时间 | `check record`（实际打卡时间） |
| 7 | 打卡结果 | `check result`（正常/迟到/早退/缺卡等，按打卡时间关联） |
| 8 | 打卡地址 | `check record` |
| 9 | 打卡备注 | `check record` |
| 10 | 异常打卡原因 | `check record` |
| 11 | 打卡图片 | `check record` |
| 12 | 打卡设备 | `check record` |
| 13 | 管理员修改备注 | `check record` |

### 签到报表（3 个基础信息列 + 11 个签到字段列 + 9 个图片列）

**基础信息列**：

| 序号 | 字段名称 | 数据来源 |
|------|---------|---------|
| 1 | 姓名 | `contact user get --ids` |
| 2 | 部门 | `contact user get --ids` |
| 3 | 完整部门 | `contact user get --ids` |

**签到字段列**（每条签到记录一行，数据来源均为 `attendance checkin records`）：

| 序号 | 字段名称 | 数据来源 |
|------|---------|---------|
| 4 | 签到来源 | `checkinType` |
| 5 | 日期 | `timestamp`（毫秒时间戳转日期） |
| 6 | 时间 | `timestamp`（毫秒时间戳转时间） |
| 7 | 经度 | `longitude` |
| 8 | 纬度 | `latitude` |
| 9 | 地点 | `place` |
| 10 | 详细地址 | `detailPlace` |
| 11 | 拜访客户 | `customers` |
| 12 | 客户部门名称 | 预留（签到接口暂无此字段） |
| 13 | 工作内容 | `remark` |
| 14 | 手机标识 | `mobileId` |

**图片列**（从 `imageList` 数组展开，最多 9 列）：

| 序号 | 字段名称 |
|------|---------|
| 15 | 图片1 |
| 16 | 图片2 |
| ... | ... |
| 23 | 图片9 |

## 分批查询规则（脚本内部自动处理）

| 维度 | 限制 | 脚本自动处理方式 |
|------|------|---------|
| 人数超限（月度/每日） | `query-data` 最多 20 人/次 | 自动按 5 人一批分批 |
| 人数超限（明细） | `check result` 最多 100 人/次 | 自动按 100 人一批分批 |
| 人数超限（签到） | `checkin records` 最多 100 人/次 | 自动按 50 人一批分批 |
| 时间超限（月度/每日） | `--start` 到 `--end` 不超过 32 天 | 自动按月分段 |
| 时间超限（明细） | `--start` 到 `--end` 不超过 1 个月 | 自动按月分段 |
| 时间超限（签到） | `--start` 到 `--end` 不超过 7 天 | 自动按 7 天分段 |
| 分页（明细打卡结果） | `check result` 单次最多 1000 条 | 自动翻页 |

## 错误处理

| 错误 | 原因 | 处理方式 |
|------|------|---------|
| 权限错误（403） | 当前账号非管理员 | 提示需要管理员权限，不要重试 |
| userId 无效 | 用户 ID 错误或已离职 | 脚本跳过并在摘要中标注 |
| 时间区间超长 | 接口可能性能不佳 | 提示"超过 1 年的数据建议分阶段导出" |
| openpyxl 未安装 | 环境缺包 | 输出 `pip install openpyxl` 安装提示 |
| 脚本执行失败 | 接口异常/配置问题 | 将 stderr 错误信息转告用户，可加 `--inspect` 重试 |

## 使用示例

### 示例 1: 团队月度汇总（默认）
**用户说**: "帮我生成研发组 4 月的考勤报表"

```bash
# 1. 获取部门成员
dws contact dept search --query "研发组" --format json
dws contact dept list-members --depts <deptId> --format json

# 2. 调用脚本（默认月度汇总，不传 --column-keywords）
python scripts/attendance_report_monthly.py \
  --users userId1,userId2,... \
  --start "2026-04-01" --end "2026-04-30"
```

### 示例 2: 加班报表（预设）
**用户说**: "帮我出一份研发组 4 月的加班报表"

```bash
python scripts/attendance_report_monthly.py \
  --users userId1,userId2,... \
  --start "2026-04-01" --end "2026-04-30" \
  --column-keywords "加班-审批单统计,加班总时长,考勤结果"
```

### 示例 3: 请假报表（预设）
**用户说**: "帮我导出研发组 4 月的请假出差情况"

```bash
python scripts/attendance_report_monthly.py \
  --users userId1,userId2,... \
  --start "2026-04-01" --end "2026-04-30" \
  --column-keywords "请假,出差时长,外出时长,考勤结果"
```

### 示例 4: 异常报表（预设）
**用户说**: "帮我出研发组 4 月的异常考勤报表"

```bash
python scripts/attendance_report_monthly.py \
  --users userId1,userId2,... \
  --start "2026-04-01" --end "2026-04-30" \
  --column-keywords "迟到次数,迟到时长,严重迟到次数,严重迟到时长,旷工迟到次数,早退次数,早退时长,上班缺卡次数,下班缺卡次数,旷工天数,考勤结果"
```

### 示例 5: 自定义维度筛选
**用户说**: "帮我出一份研发组 4 月的工作时长报表"

```bash
python scripts/attendance_report_monthly.py \
  --users userId1,userId2,... \
  --start "2026-04-01" --end "2026-04-30" \
  --column-keywords "工作时长"
```

### 示例 6: 每日统计
**用户说**: "帮我出一份研发组 4 月每天的出勤情况"

```bash
python scripts/attendance_report_daily.py \
  --users userId1,userId2,... \
  --start "2026-04-01" --end "2026-04-30"
```

### 示例 7: 明细报表
**用户说**: "帮我导出研发组 4 月的考勤明细"

```bash
python scripts/attendance_report_detail.py \
  --users userId1,userId2,... \
  --start "2026-04-01" --end "2026-04-30"
```

### 示例 8: 请假记录
**用户说**: "帮我导出研发组 4 月的请假记录"

```bash
python scripts/attendance_report_record.py \
  --type leave \
  --users userId1,userId2,... \
  --start "2026-04-01" --end "2026-04-30"
```

### 示例 9: 出差记录
**用户说**: "帮我导出研发组 4 月的出差记录"

```bash
python scripts/attendance_report_record.py \
  --type trip \
  --users userId1,userId2,... \
  --start "2026-04-01" --end "2026-04-30"
```

### 示例 10: 补卡记录
**用户说**: "帮我导出研发组 5 月的补卡记录"

```bash
python scripts/attendance_report_record.py \
  --type patch \
  --users userId1,userId2,... \
  --start "2026-05-01" --end "2026-05-31"
```

### 示例 11: 签到报表
**用户说**: "帮我导出研发组上周的签到记录"

```bash
python scripts/attendance_report_checkin.py \
  --users userId1,userId2,... \
  --start "2026-05-26" --end "2026-06-01"
```

## 配套脚本

| 脚本 | 报表类型 | 数据来源 | CLI 参数 |
|------|---------|---------|---------|
| [attendance_report_detail.py](../scripts/attendance_report_detail.py) | 明细 | `check result` + `check record` | `--users --start --end [--out]` |
| [attendance_report_monthly.py](../scripts/attendance_report_monthly.py) | 月度汇总（默认） | `report columns` + `report query-data` | `--users --start --end [--column-keywords] [--out]` |
| [attendance_report_daily.py](../scripts/attendance_report_daily.py) | 每日统计 | `report columns` + `report query-data` | `--users --start --end [--column-keywords] [--out]` |
| [attendance_report_record.py](../scripts/attendance_report_record.py) | 考勤记录 | `attendance approve list` + `oa approval detail` | `--type --users --start --end [--out]` |
| [attendance_report_checkin.py](../scripts/attendance_report_checkin.py) | 签到报表 | `attendance checkin records` | `--users --start --end [--out]` |
| [attendance_report_common.py](../scripts/attendance_report_common.py) | 公共模块（不可单独执行） | — | — |
