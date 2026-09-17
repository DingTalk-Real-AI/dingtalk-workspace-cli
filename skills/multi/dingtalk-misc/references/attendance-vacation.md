# 考勤假期、余额与变更流水

本 Reference 处理假期类型、余额、余额变更记录、假期规则修改、余额设置和假期余额 Excel。请假/加班/补卡审批表单与记录走 [attendance.md](attendance.md)；考勤统计 Excel 走 [attendance-report.md](attendance-report.md)。

已知下列路由时直接执行，不先查 vacation Help、产品 Schema 或 Shortcut 列表。只有参数/确认不确定时查一次精确 leaf compact Schema；只有真实 `unknown flag` 时查一次 leaf Help。

## Golden Route

| 用户意图 | 唯一推荐入口 | 关键边界 |
|---|---|---|
| 列出可用假期规则/类型 | `dws attendance vacation types --format json` | 无业务参数；从真实返回读取 leaveCode、名称、单位、来源和适用范围 |
| 查询某人某类当前余额 | `dws attendance vacation balance` | `--users <ids> --leave-code <code>`；当前 main 的 leave-code 必填 |
| 查询某人某类余额变更流水 | `dws attendance vacation records` | `--user <id> --leave-code <code> --start YYYY-MM-DD --end YYYY-MM-DD`；四项均必填 |
| 查询某时间段假期使用数据（管理员） | `dws attendance report query-leave` | `--users --leave-names --start/--end "YYYY-MM-DD HH:mm:ss"`；最多 20 人、32 天 |
| 修改假期规则 | `dws attendance vacation update-type` | 仅目标规则当前 `leaveStatisticType=freedom`；`--leave-code` 加至少一个修改字段；user_required |
| 设置员工假期余额 | `dws attendance vacation save-balance` | SET 替换，不是 ADD；user_required |
| 导出多员工/多假期余额 Excel | `python3 scripts/attendance_vacation_balance.py` | 脚本逐个真实 leaveCode 查询并横向汇总 |

## 只读查询

### 1. 解析人和假期规则

- 姓名先用 `dws aisearch person --query "<姓名>" --dimension name --format json`；零命中、多候选停止。
- 先调用 `vacation types`，按假期名称唯一匹配 leaveCode。不要猜 UUID，不跨 profile 复用历史 code。
- 用户要求年假和调休等多个类型时，对每个匹配 leaveCode 分别查询；不能省略 leave-code 后声称查询了全部。
- `corpId` / `opUserId` 由当前登录上下文注入，不向用户索要。

### 2. 当前余额

```bash
dws attendance vacation balance \
  --users <USER_ID_1,USER_ID_2> \
  --leave-code <LEAVE_CODE> \
  --format json
```

返回时同时展示假期名称、leaveCode 对应单位和真实余额。`visible=false` 表示该员工不适用该规则，不等于余额 0；“假期类型没有余额”按“不限制余额”处理；权限错误不做同义重试。

### 3. 变更流水

```bash
dws attendance vacation records \
  --user <USER_ID> --leave-code <LEAVE_CODE> \
  --start <YYYY-MM-DD> --end <YYYY-MM-DD> \
  --format json
```

按服务端真实时间、变更数量、原因和有效期展示；不要把空数组描述成“没有发放过”，只能说“该查询范围返回 0 条记录”。用户要求“今年”时使用当年 1 月 1 日到当前日期；不得查询未来日期补齐全年。

## 修改假期规则

`vacation update-type` 为 `confirmation=user_required`。流程：

1. `vacation types` 唯一定位规则并保存完整当前值。
2. 检查目标规则的精确 `leaveStatisticType`。只有值严格等于 `freedom` 时才能继续；其他值立即报告后端不支持，不调用写接口、不改写枚举、不查 Help、不重试。能力缺陷允许正确阻塞，不为追求 case 成功而绕过。
3. 仅组装用户要求的变更字段；展示名称/code、`当前值 → 新值`、适用范围影响。
4. 用户确认后执行；写后再次 `vacation types`，按同一 leaveCode 核对。

可用修改字段：`--name`、`--unit day|halfDay|hour`、`--paid`、`--per-hours`、`--when-can-leave entry|formal`、`--visibility-rules '<JSON数组>'`。至少传一项。

可见范围约定：

- 不传 `--visibility-rules`：不修改。
- 指定范围：`[{"type":"staff|dept|label|employee_type","visible":["id1"]}]`。
- 全公司：必须显式使用哨兵 `[{"type":"dept","visible":["-1"]}]`。
- `[]`、`[{}]` 或空 visible 无效，禁止用来表达全公司或清空。

临时修改单位/一天折算时长后恢复时，确认计划必须同时覆盖修改和恢复；修改后回读，再用步骤 1 保存值恢复并回读。恢复失败报告 partial，不得宣称完成。

## 设置假期余额

`vacation save-balance` 是 SET：传入新总余额会替换当前余额。即使用户说“增加 3 天”，也必须先读取当前值再计算新总值。

1. 用 `vacation balance --users <单个目标> --leave-code <code>` 查询当前值和单位。
2. 计算并展示：目标员工、假期、当前余额、新总余额、差额、原因、可选有效期。
3. 获得明确确认后执行：

```bash
dws attendance vacation save-balance \
  --target <USER_ID> --leave-code <LEAVE_CODE> \
  --num <NEW_TOTAL> --reason "<REASON>" \
  [--start <YYYY-MM-DD> --end <YYYY-MM-DD>] \
  --format json
```

4. 再次调用 `vacation balance` 验证新总值。CLI 会把实际单位数量乘以 100 发送（例如 8→800），Agent 传用户可读的 8，不自行乘 100。

可能提交后超时不得盲目重试，先读余额对账。批量调整需逐员工保留成功/失败/未知账本；不要把一人成功扩展成全部成功。

## 假期余额 Excel

### 1. 解析范围

- 人员范围必填：指定员工、部门/多部门或明确 userId 列表。完整解析并去重，禁止只取部门第一页。
- 用户未指定假期类型时导出全部可见规则，不传 `--leave-keywords`；指定类型时传名称关键词，脚本实时映射 leaveCode。

### 2. 执行

```bash
python3 scripts/attendance_vacation_balance.py \
  --users <USER_ID_1,USER_ID_2> \
  [--leave-keywords "年假,病假,调休"] \
  [--out <相对路径.xlsx>]
```

`--leave-keywords` 是脚本参数，不是 `dws attendance vacation balance` 参数。脚本自动执行：

- `vacation types` 建立名称、leaveCode、单位、source 和列顺序。
- 对每个匹配规则逐个调用 `vacation balance --leave-code`。
- userId→姓名/部门/入职信息。
- 每名员工一行，假期规则横向展开并在表头保留单位。

特殊值必须保真：

- 服务端表示无余额上限 → `不限制余额`。
- `visible=false` 或员工缺少规则依赖的入职/首次工作时间 → `不适用`。
- `source=external` 且非权限类余额错误 → `外部规则暂无余额，需通过接口初始化更新余额`。
- 真实无返回且无上述证据 → `未设置`，不得填 0。

返回脚本 stdout 中的路径、员工数、规则列数、筛选关键词和 warning，不粘贴完整 Excel。只允许工作目录内安全相对路径，默认不覆盖。

## 错误最短路径

1. 假期名称零命中/多候选：停止，展示候选；禁止按列表首项继续。
2. 权限不足：说明目标员工不在管理范围或需要管理员权限，不切换组织/身份。
3. leaveCode 为空或过期：重新调用一次 `vacation types`；仍不存在则停止，不猜 code。
4. 写请求可能已提交：先按同一 user/code 回读对账；禁止自动重放。
5. Excel 脚本依赖缺失：报告缺失依赖，不自动联网安装。
6. 返回结构漂移：脚本明确提示后才用一次 `--inspect`；不要先加载产品 Schema。
