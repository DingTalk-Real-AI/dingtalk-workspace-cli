# 假期余额导出参考 (attendance-vacation)

> 本文档由 `attendance.md` 路由调用。当用户提到“导出假期余额”、“假期余额列表”、“所有假期规则余额”、“假期余额 Excel”、“年假/病假/调休余额导出”等诉求时，必须先阅读本文档，再调用配套脚本。

## 强制门禁

**任何调用 `attendance_vacation_balance.py` 的请求，都必须经过本文档定义的工作流，禁止只凭脚本路径或 `--help` 自行拼命令。**

执行前必须确认：
- 人员范围：指定员工 / 部门 / 多部门；缺失时必须追问
- 导出范围：默认导出所有假期规则余额；用户指定“年假/病假/调休”等时才传 `--leave-keywords`
- 输出形式：生成 Excel，不在对话中粘贴完整表格

## 核心原则

Agent 只负责解析人员范围并获取 userId 列表；假期规则查询、`leaveCode` 解析、假期规则单位解析、余额查询、字段解析、用户信息补齐、Excel 生成都由脚本完成。

`--leave-keywords` 是 `attendance_vacation_balance.py` 的脚本入参，只用于按假期名称筛选导出列；**不是** `dws attendance vacation balance` 的入参。`vacation balance` 当前只支持批量 `--users` 和单个 `--leave-code`，因此脚本必须先获取假期规则列表，再按每个匹配到的 `leaveCode` 分别查询余额。

脚本输出结构参考钉钉假期余额列表：
- 每名员工一行
- 基础列固定：`姓名`、`部门`、`入职时间`、`首次工作时间`
- 假期规则横向动态展开为多列，表头必须携带 `vacation types` 返回的规则单位，例如 `年假(天)`、`病假(天)`、`调休(小时)`
- 特殊值统一展示为 `不限制余额`、`不适用`、`未设置`
- 当某个假期规则 `leaveCode` 查询余额时接口返回“假期类型没有余额”类业务错误，表示该规则不限制余额，脚本应为该规则列填充 `不限制余额`
- 当余额记录中返回 `visible=false`，表示该员工不适用该假期规则，脚本应为该员工 + 该规则单元格填充 `不适用`
- 当接口返回“员工未设置入职时间”或“员工未设置首次参加工作时间”类业务错误，表示该假期规则依赖员工时间字段且当前员工缺失配置，脚本应为该员工 + 该规则单元格填充 `不适用`
- 当 `vacation types` 返回假期规则 `source=external`，表示该规则由开放接口写入；若这类外部规则调用余额接口失败且不是权限错误，脚本不应阻断导出，应为该员工 + 该规则单元格填充 `外部规则暂无余额，需通过接口初始化更新余额`

## 严格禁止 (NEVER DO)

- 禁止凭历史记忆复用 userId、deptId、leaveCode
- 禁止 Agent 手工汇总、转置或目测假期余额
- 禁止 Agent 自行只查单个 `leave-code` 再声称是“所有假期规则余额”；如需导出所有假期规则余额，必须交由脚本按假期规则列表逐个 `leaveCode` 查询并汇总
- 禁止直接输出裸 userId；脚本会通过 `contact user get` 转换姓名和部门
- 禁止把 Excel 明细内容完整贴在对话里，只返回路径和摘要

## 严格要求 (MUST DO)

- 所有 `dws` 命令必须携带 `--format json`
- 假期规则名称`--leave-keywords`与假期规则`leaveCode`的映射必须从 `vacation types` 实时建立，禁止硬编码
- 假期规则单位必须从 `vacation types` 实时读取，优先使用 `leaveViewUnit` 等展示单位字段，并在 Excel 表头中展示为 `假期名称(单位)`
- 假期规则来源必须从 `vacation types` 实时读取；当 `source=external` 时按外部接口写入规则处理
- 任何接口失败必须向用户清晰报错，禁止静默吞掉
- `vacation balance` 返回“假期类型没有余额”时不作为致命错误处理，应按该假期规则 `leaveCode` 生成 `不限制余额`
- `vacation balance` 返回员工维度 `visible=false` 时不作为查询失败处理，应按该员工 + 假期规则 `leaveCode` 生成 `不适用`
- `vacation balance` 返回“员工未设置入职时间”或“员工未设置首次参加工作时间”时不作为查询失败处理，应按该员工 + 假期规则 `leaveCode` 生成 `不适用`
- 对 `source=external` 的外部假期规则，`vacation balance` 查询失败且不是权限错误时不作为导出失败处理，应按该员工 + 假期规则 `leaveCode` 生成 `外部规则暂无余额，需通过接口初始化更新余额`

## 涉及工具

| 工具 | 用途 | 安全等级 |
|------|------|---------|
| `dws attendance vacation types` | 获取当前可见的假期规则清单，用于建立假期名称/关键词 → `leaveCode` 的映射，读取 `leaveViewUnit` 等规则单位、`source` 规则来源，并决定 Excel 假期列顺序 | 只读 |
| `dws attendance vacation balance` | 按批量 `--users` + 单个 `--leave-code` 查询员工假期余额；由脚本按匹配到的 `leaveCode` 逐个调用 | 只读 |
| `dws aisearch person` | 按姓名搜索用户获取 userId（搜人首选） | 只读 |
| `dws contact user get` | 批量查询 userId → 用户信息（姓名/部门/入职时间等），用于 Excel 基础列补齐 | 只读 |
| `dws contact dept search` | 搜索部门获取 deptId | 只读 |
| `dws contact dept list-members` | 获取部门成员 userId 列表 | 只读 |

## 工作流

### 阶段 0：参数解析

1. 识别人员范围：
   - 指定员工姓名：用 `dws aisearch person --keyword "<姓名>" --dimension name --format json` 获取 userId
   - 指定部门：用 `dws contact dept search --query "<部门名>" --format json`，再用 `dws contact dept list-members --depts <deptId> --format json` 获取成员
   - 已提供 userId：直接使用
2. 识别假期列范围：
   - 未指定假期类型：导出所有假期规则余额，不传 `--leave-keywords`
   - 指定假期类型：传 `--leave-keywords "年假,病假"`

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

### 阶段 2：调用脚本生成 Excel

```bash
python scripts/attendance_vacation_balance.py \
  --users <userId1>,<userId2>,... \
  [--leave-keywords "年假,病假,调休"] \
  [--out 假期余额列表.xlsx]
```

脚本内部自动执行：
1. `dws attendance vacation types --format json` 获取假期规则列表、`leaveCode`、展示单位、规则来源 `source` 和列顺序
2. 对匹配到的每个假期规则，调用 `dws attendance vacation balance --users <批量用户> --leave-code <单个leaveCode> --format json` 查询余额；`vacation balance` 不支持 `--leave-keywords`
3. 处理特殊业务返回：
   - 返回“假期类型没有余额”类业务错误：该 `leaveCode` 对应列填充 `不限制余额`
   - 返回员工维度 `visible=false`：该员工 + 该 `leaveCode` 单元格填充 `不适用`
   - 返回“员工未设置入职时间”或“员工未设置首次参加工作时间”：该员工 + 该 `leaveCode` 单元格填充 `不适用`
   - `source=external` 的外部假期规则查询失败且不是权限错误：该员工 + 该 `leaveCode` 单元格填充 `外部规则暂无余额，需通过接口初始化更新余额`
4. `dws contact user get --ids <批量用户> --format json` 获取姓名、部门等信息
5. 生成 Excel：`attendance_vacation_balance_<yyyyMMdd_HHmmss>.xlsx`

### 阶段 3：返回结果

向用户返回脚本 stdout 摘要即可，必须包含：
- 输出文件路径
- 员工数量
- 假期规则列数
- 如传了 `--leave-keywords`，说明筛选关键词

不要粘贴 Excel 全量内容。

## 错误处理

| 错误 | 处理方式 |
|------|---------|
| 权限不足 | 提示当前账号无权查询目标员工假期余额，需管理员或管理范围权限 |
| 人员范围缺失 | 追问员工或部门，禁止猜测 |
| 无假期规则 | 提示未匹配到假期规则，建议先执行 `dws attendance vacation types --format json` 验证 |
| 假期类型没有余额 | 不作为失败返回；按对应假期规则 `leaveCode` 填充 `不限制余额` |
| `visible=false` | 不作为失败返回；按对应员工 + 假期规则 `leaveCode` 填充 `不适用` |
| 员工未设置入职时间 / 首次参加工作时间 | 不作为失败返回；说明该规则依赖员工时间字段，按对应员工 + 假期规则 `leaveCode` 填充 `不适用` |
| 外部假期规则查询失败 | 当假期规则 `source=external` 且失败不是权限错误时，不作为导出失败；按对应员工 + 假期规则 `leaveCode` 填充 `外部规则暂无余额，需通过接口初始化更新余额` |
| openpyxl 缺失 | 提示执行 `pip install openpyxl` |
| 接口返回结构不确定 | 使用脚本 `--inspect` 重新执行一次，查看首条原始结构 |

## 配套脚本

| 脚本 | 场景 | CLI 参数 |
|------|------|---------|
| [attendance_vacation_balance.py](../scripts/attendance_vacation_balance.py) | 假期余额列表 Excel 导出；脚本按假期名称关键词筛选规则，并逐个 `leaveCode` 调用余额查询 | `--users [--leave-keywords] [--out] [--inspect]` |
