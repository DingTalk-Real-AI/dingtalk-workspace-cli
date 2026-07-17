# 注意事项

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
- `adjustment search` 返回结果已包含全量属性，无需再调用 `adjustment get`；`adjustment get` 仅在需要按已知 adjustmentId 精确查询时使用
- `overtime search` 返回结果已包含全量属性，无需再调用 `overtime get`；`overtime get` 仅在需要按已知 overtimeId 查询时使用（包括已删除/被覆盖的历史记录）
- `adjustment search` / `overtime search` 分页字段为 `--page` 和 `--limit`，不传时自动使用默认值 1 / 20
- `group search` 的分页字段为 `--page` 和 `--limit`，不传时自动使用默认值 1 / 20
- `group get` 的 `--group-id` 必填，返回考勤组全量字段；如仅需成员/地址/蓝牙/Wifi，优先使用 `group filtered-get` 节省成本。**返回结果中如含成员 userId 列表，必须调用 `dws contact user get --ids <userId1>,<userId2>,...`（支持逗号分隔传多个 ID），将 userId 转换为员工姓名后再输出；不得直接输出裸 userId。**
- `group update-members` 的 --group-id 必填，其余参数均可选，但至少需传一个变更项；各参数每次最多 20 个 ID；`--add-extra-users` 和 `--remove-extra-users` 操作的是"无需考勤"豁免名单，不影响考勤组主成员列表
- `group update` 的 --group-id 必填，其余均可选，至少需指定一个修改项；仅需对要修改的字段赋値，未传字段会从已有配置自动补充；修改打卡地址/wifi/蓝牙等复杂子对象时用 `--group-vo` 传入完整 JSON；`--group-vo` 与单字段 flag 同时传入时单字段 flag 优先级更高
- `group create` 的 `--name` 和 `--type` 必填，`--type` 必须为 FIXED/TURN/NONE 之一；type=FIXED 时 `--group-vo` 必须包含 `workDayClassList`（非空）和 `defaultClassId`（非 null）；由于保存考勤组耗时较久，建议加 `--timeout 10`
- `group filtered-get` 的 `--group-id` 必填，`--member/--position/--wifi/--bles` 均可选，默认 false。**返回结果中如含成员 userId 列表，必须调用 `dws contact user get --ids <userId1>,<userId2>,...`（支持逗号分隔传多个 ID），将 userId 转换为员工姓名后再输出；不得直接输出裸 userId。**
- `summary` 必须同时传 `--user`、`--date`、`--stats-type`（`week` 周统计 / `month` 月统计）；`--date` 支持 `YYYY-MM-DD` 或 `YYYY-MM-DD HH:mm:ss`
- `rules` 的 `--date` 支持 YYYY-MM-DD 或 yyyy-MM-dd HH:mm:ss 两种格式
- `selfsetting get/save` 的 `--setting-scene` 必须是 `checkRemind`、`fastCheck`、`checkResultNotify`、`lackRemind`、`personalAttendStatNotify`、`bossAttendStatNotify` 之一
- `selfsetting get/save` 的 MCP 入参 `userId` 为必填；CLI 的 `--user` 也必填，必须显式传入目标用户 ID
- `selfsetting save` 必须传入与 `--setting-scene` 对应的至少一个设置字段；不同场景的字段不能混用
- `selfsetting save` 是敏感写操作，必须先执行 `selfsetting get` 查询当前值，并向用户展示目标用户、设置场景、修改字段、“当前值 → 新值”和最终命令参数摘要，等待用户明确确认；确认后才允许追加全局 `--yes` 执行保存。禁止未经确认直接执行或自动添加 `--yes`
- `selfsetting get/save` 不需要传 `--corp-id` / `--op-user`，`corpId` 和 `opUserId` 由当前登录上下文自动补齐
- `report columns` 无需额外参数，corpId 和 operatorId 由系统自动传入
- `report query-data` 和 `report query-leave` 的 `--start/--end` 格式: yyyy-MM-dd HH:mm:ss，间隔不超过 32 天，最多 20 人
- report 系列接口仅对管理员开放
- 用户 ID 需从 `contact user get-self` 或 `aisearch person` 获取
- 考勤组 ID 需从 `rules` 命令返回结果中获取
- `vacation types` 无需任何参数，认证信息自动注入
- `vacation balance` 的 `--users` 为目标员工 ID 列表，逗号分隔；`--leave-code` 选填，可通过 `vacation types` 获取
- `vacation records` 的 `--start/--end` 使用 YYYY-MM-DD 格式，CLI 自动转换为毫秒时间戳；`--leave-code` 选填
- `vacation balance` 和 `vacation records` 的认证参数（corpId、opUserId）由系统自动注入，无需手动传入
- `vacation update-type` 的 `--leave-code` 必填；其他字段均为可选，但至少需传一个更新字段
- `vacation update-type` 的 `--visibility-rules` 为 JSON 数组字符串，**HSF 反序列化无法区分「未传」与「空数组 `[]`」**，所以约定了哨兵语义：
  - 不传 → 不修改可见范围
  - `[{"type":"dept","visible":["-1"]}]` → **哨兵**，改为全公司可见（服务端落库为空）
  - `[{"type":"staff|dept|label|employee_type","visible":["id1",...]}, ...]` → 改为指定范围
  - 空数组 `[]`、`[{}]`、`visible` 为空等无效写法 → **CLI 报错**（服务端会静默忽略，故在 CLI 提前拦截）
  - **「清空可见范围」必须显式传哨兵值 `["-1"]`，不能用 `[]`**
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
2. 展示变更摘要并等待用户明确确认
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
1. 查询排班记录：`dws attendance schedule get --users USER_ID --start DATE --end DATE`
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
dws attendance schedule get --users 03642229451220076 --start 2026-05-13 --end 2026-05-13 -f json

# 步骤2：使用返回的 id 作为 --plan-id 改签
# 假设返回 id: 948964045503 (OffDuty 下班打卡)
dws attendance boss-check --plan-id 948964045503 --result Normal --user-say-yes

# 同时修改打卡时间
dws attendance boss-check --plan-id 948964045503 --time "2026-05-13 18:00" --result Normal --yes
```
