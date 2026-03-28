# DWS 全产品快速测试手册

> 本手册基于 2026-03-26 的全量测试结果提炼，供Agent或人工快速执行。
> 所有命令已验证过参数格式，可直接复制执行，无需查阅参考文档。

## 执行规则

1. **所有命令必须加 `--format json`**
2. **危险操作（delete）必须加 `--yes`**
3. **按 Step 顺序执行**，每个 Step 的输出 ID 会被后续 Step 引用
4. **判定标准**：JSON 输出中 `"success": true` 或 `"status": "success"` 即为 PASS
5. **时间占位符**：`${TODAY}` = 当天日期（如 `2026-03-27`），`${NOW_PLUS_1H}` / `${NOW_PLUS_2H}` = ISO-8601 时间，`${NOW_PLUS_1H_MS}` / `${NOW_PLUS_2H_MS}` = 毫秒时间戳
6. **ID 占位符**：`${变量名}` 表示从前序步骤的返回中提取的值

## 前置准备

```bash
# 确认 dws 可用
dws --help
# 确认已登录（如果报认证错误，参考 global-reference.md）
dws contact user get-self --format json
```

---

## Step 1: contact（通讯录）— 5 条，全读，约 30 秒

```bash
# 1.1 获取当前用户 → 提取 ${USER_ID}
dws contact user get-self --format json
# 返回中提取: result.userId → ${USER_ID}

# 1.2 搜索用户（注意：参数是 --keyword 不是 --query）
dws contact user search --keyword "桓奇" --format json

# 1.3 获取用户详情
dws contact user get --ids ${USER_ID} --format json

# 1.4 搜索部门 → 提取 ${DEPT_ID}（注意：参数是 --keyword 不是 --query）
dws contact dept search --keyword "技术" --format json
# 返回中提取: 第一个 result[].deptId → ${DEPT_ID}

# 1.5 查看部门成员
dws contact dept list-members --ids ${DEPT_ID} --format json
```

**验证点**：1.1 返回含 userId 和 name；1.4 返回部门列表

---

## Step 2: aitable（AI表格）— 16 条，读写混合，约 2 分钟

```bash
# 2.1 搜索模板
dws aitable template search --query "项目管理" --format json

# 2.2 创建 Base → 提取 ${BASE_ID}
dws aitable base create --name "DWS测试表格" --format json
# 返回中提取: data.baseId → ${BASE_ID}

# 2.3 获取 Base 详情
dws aitable base get --base-id ${BASE_ID} --format json

# 2.4 更新 Base 名称
dws aitable base update --base-id ${BASE_ID} --name "DWS测试表格-已更新" --format json

# 2.5 列出 Base（注意：仅返回最近访问过的）
dws aitable base list --format json

# 2.6 搜索 Base
dws aitable base search --query "DWS测试" --format json

# 2.7 创建数据表 → 提取 ${TABLE_ID}
dws aitable table create --base-id ${BASE_ID} --name "测试数据表" --fields '[{"name":"姓名","type":"text"},{"name":"年龄","type":"number"},{"name":"城市","type":"text"}]' --format json
# 返回中提取: data.tableId → ${TABLE_ID}

# 2.8 获取数据表 → 提取已有字段的 ${FIELD_IDS}
dws aitable table get --base-id ${BASE_ID} --format json
# 返回中提取: 各字段的 fieldId

# 2.9 更新数据表名称
dws aitable table update --base-id ${BASE_ID} --table-id ${TABLE_ID} --name "测试数据表-已更新" --format json

# 2.10 创建新字段 → 提取 ${NEW_FIELD_IDS}
dws aitable field create --base-id ${BASE_ID} --table-id ${TABLE_ID} --fields '[{"fieldName":"邮箱","type":"text"},{"fieldName":"分数","type":"number"}]' --format json
# 返回中提取: data[].fieldId → ${NEW_FIELD_IDS}

# 2.11 获取字段详情
dws aitable field get --base-id ${BASE_ID} --table-id ${TABLE_ID} --format json

# 2.12 更新字段名称（用 ${NEW_FIELD_IDS} 中的第一个）
dws aitable field update --base-id ${BASE_ID} --table-id ${TABLE_ID} --field-id ${NEW_FIELD_ID_1} --name "电子邮箱" --format json

# 2.13 创建记录 → 提取 ${RECORD_IDS}
# 注意：字段名必须与 2.8 返回的实际字段名一致
dws aitable record create --base-id ${BASE_ID} --table-id ${TABLE_ID} --records '[{"cells":{"姓名":"张三","年龄":25,"城市":"杭州"}},{"cells":{"姓名":"李四","年龄":30,"城市":"北京"}}]' --format json
# 返回中提取: data[].recordId → ${RECORD_IDS}

# 2.14 查询所有记录
dws aitable record query --base-id ${BASE_ID} --table-id ${TABLE_ID} --format json

# 2.15 更新记录
dws aitable record update --base-id ${BASE_ID} --table-id ${TABLE_ID} --records '[{"recordId":"${RECORD_ID_1}","cells":{"城市":"上海"}}]' --format json

# 2.16 按 ID 查询验证更新
dws aitable record query --base-id ${BASE_ID} --table-id ${TABLE_ID} --record-ids ${RECORD_ID_1} --format json
```

**验证点**：2.16 返回的城市字段应为"上海"

---

## Step 3: calendar（日历）— 9 条，约 1 分钟

```bash
# 3.1 创建日程 → 提取 ${EVENT_ID}
dws calendar event create --title "DWS测试日程" --start "${NOW_PLUS_1H}" --end "${NOW_PLUS_2H}" --format json
# 返回中提取: result.eventId → ${EVENT_ID}

# 3.2 获取日程详情
dws calendar event get --id ${EVENT_ID} --format json

# 3.3 更新日程标题
dws calendar event update --id ${EVENT_ID} --title "DWS测试日程-已更新" --format json

# 3.4 查询日程列表
dws calendar event list --start "${TODAY}T00:00:00+08:00" --end "${TOMORROW}T00:00:00+08:00" --format json

# 3.5 查看参与者
dws calendar participant list --event ${EVENT_ID} --format json

# 3.6 搜索会议室
# 注意：按当前 skills/generated 文档，room search 需要毫秒时间戳，不接受 ISO-8601 字符串
dws calendar room search --start "${NOW_PLUS_1H_MS}" --end "${NOW_PLUS_2H_MS}" --format json

# 3.7 会议室分组列表
dws calendar room list-groups --format json

# 3.8 查询闲忙状态
dws calendar busy search --users ${USER_ID} --start "${TODAY}T00:00:00+08:00" --end "${TOMORROW}T00:00:00+08:00" --format json

# 3.9 删除日程（清理）
dws calendar event delete --id ${EVENT_ID} --yes --format json
```

---

## Step 4: todo（待办）— 6 条，约 30 秒

```bash
# 4.1 创建待办 → 提取 ${TASK_ID}
dws todo task create --title "DWS测试待办" --executors ${USER_ID} --priority 20 --format json
# 返回中提取: result.taskId → ${TASK_ID}

# 4.2 获取待办详情
dws todo task get --task-id ${TASK_ID} --format json

# 4.3 查询未完成待办列表
dws todo task list --page 1 --size 20 --status false --format json

# 4.4 更新待办
dws todo task update --task-id ${TASK_ID} --title "DWS测试待办-已更新" --priority 40 --format json

# 4.5 标记完成
dws todo task done --task-id ${TASK_ID} --status true --format json

# 4.6 删除待办（清理）
dws todo task delete --task-id ${TASK_ID} --yes --format json
```

---

## Step 5: chat（群聊与消息）— 1 条，约 5 秒

```bash
# 5.1 搜索机器人 → 提取 ${ROBOT_CODE}
dws chat bot search --page 1 --format json
# 返回中提取: robotList[0].robotCode → ${ROBOT_CODE}
```

---

## Step 6: report（日志）— 7 条，约 1 分钟

```bash
# 6.1 获取日志模板列表 → 提取 ${TEMPLATE_NAME} 和 ${TEMPLATE_ID}
dws report template list --format json
# 返回中提取: items[].report_template_name 和 report_template_id

# 6.2 获取模板详情 → 提取字段的 key/sort/type
dws report template detail --name "日报" --format json
# 返回中提取: result.report_template_fields[] 的 field_name/field_sort/field_type
# 以及 result.report_template_id → ${TEMPLATE_ID}

# 6.3 创建日志 → 提取 ${REPORT_ID}
# contents 的 key 必须与 6.2 返回的 field_name 完全一致
dws report create --template-id ${TEMPLATE_ID} --contents '[{"key":"今日完成工作","sort":"0","content":"DWS测试","contentType":"markdown","type":"1"},{"key":"未完成工作","sort":"1","content":"无","contentType":"markdown","type":"1"},{"key":"需协调工作","sort":"2","content":"无","contentType":"markdown","type":"1"}]' --format json
# 返回中提取: reportId → ${REPORT_ID}

# 6.4 查询已发送日志
dws report sent --cursor 0 --size 20 --format json

# 6.5 获取日志详情
dws report detail --report-id ${REPORT_ID} --format json

# 6.6 获取日志统计
dws report stats --report-id ${REPORT_ID} --format json

# 6.7 查询收件箱
dws report list --start "${TODAY}T00:00:00+08:00" --end "${TODAY}T23:59:59+08:00" --cursor 0 --size 20 --format json
```

**验证点**：6.5 返回的内容应含"DWS测试"

---

## Step 7: ding（DING消息）— 2 条，约 15 秒

> 需要有效的 ${ROBOT_CODE}（从 Step 5.1 获取或用户提供）

```bash
# 7.1 发送应用内 DING → 提取 ${OPEN_DING_ID}
# 注意：参数是 --content 不是 --text，--type 值为 app
# 注意：${USER_ID} 必须是完整的 userId（从 1.1 返回的 orgEmployeeModel.userId 提取，如 2039500828850772），不能用 staffId
dws ding message send --robot-code ${ROBOT_CODE} --type app --users ${USER_ID} --content "DWS测试DING" --format json
# 返回中提取: result.openDingId → ${OPEN_DING_ID}

# 7.2 撤回 DING
dws ding message recall --robot-code ${ROBOT_CODE} --id ${OPEN_DING_ID} --format json
```

**注意**：仅测试 `--type app`（应用内），不测试 sms/call（有通信费用）

---

## Step 8: attendance（考勤）— 4 条，约 20 秒

```bash
# 8.1 查询个人考勤
dws attendance record get --user ${USER_ID} --date ${TODAY} --format json

# 8.2 查询排班信息（日期间隔不超过 7 天）
dws attendance shift list --users ${USER_ID} --start ${WEEK_START} --end ${WEEK_END} --format json

# 8.3 查询考勤统计 [已知缺陷: 当前 source build 仅暴露 --date/--user；直接调用会返回 C0002“统计类型错误”，且 --json 传 QueryUserAttendVO.statsType 会被本地 schema 拒绝]
dws attendance summary --user ${USER_ID} --date "${TODAY} 15:00:00" --format json

# 8.4 查询考勤规则
dws attendance rules --date ${TODAY} --format json
```

---

## Step 9: workbench（工作台）— 1 条，约 5 秒

```bash
# 9.1 查看工作台应用列表
dws workbench app list --format json
```

---

## Step 10: devdoc（开发文档）— 1 条，约 5 秒

```bash
# 10.1 搜索开发文档
dws devdoc article search --keyword "OAuth2" --page 1 --size 5 --format json
```

---

## Step 11: 清理测试产物 — 约 30 秒

按逆序清理 aitable 产物（其他产品已在各 Step 中即时清理）：

```bash
# 11.1 删除测试记录
dws aitable record delete --base-id ${BASE_ID} --table-id ${TABLE_ID} --record-ids ${RECORD_ID_1},${RECORD_ID_2} --yes --format json

# 11.2 删除新增字段
dws aitable field delete --base-id ${BASE_ID} --table-id ${TABLE_ID} --field-id ${NEW_FIELD_ID_1} --yes --format json
dws aitable field delete --base-id ${BASE_ID} --table-id ${TABLE_ID} --field-id ${NEW_FIELD_ID_2} --yes --format json

# 11.3 删除数据表（注意：最后一个表无法删除，会报 "cannot delete the last sheet"，这是预期行为）
dws aitable table delete --base-id ${BASE_ID} --table-id ${TABLE_ID} --yes --format json

# 11.4 删除整个 Base（会连带删除所有表和数据）
dws aitable base delete --base-id ${BASE_ID} --yes --format json
```

**注意**：11.3 如果是最后一个表会失败，直接执行 11.4 删除整个 Base 即可

---

## 已知陷阱与文档勘误

| # | 产品 | 陷阱 | 正确做法 |
|---|------|------|----------|
| 1 | contact | 文档写 `--query`，实际 CLI 是 `--keyword` | 用 `--keyword` |
| 2 | ding message send | `--users` 需传完整 userId（如 `2039500828850772`），不能用 staffId | 从 `contact user get-self` 返回的 `userId` 字段提取 |
| 3 | aitable table delete | 无法删除 base 中最后一个表 | 直接用 `base delete` 删除整个 base |
| 4 | aitable base list | 仅返回最近访问过的 base | 新创建的可能不在列表中，用 `base search` 替代 |
| 5 | attendance summary | 当前 source build 只有 `--date` / `--user`；直接调用会报 `C0002 统计类型错误`，而 `--json` 传 `QueryUserAttendVO.statsType` 又会被本地 schema 拒绝 `is not allowed` | 记录为未解缺陷，不做本地特判；保留 `dws attendance summary --user ... --date ... --format json` 的复现命令 |
| 6 | calendar event get/update/delete | 文档写 `--event-id`，实际 CLI 是 `--id` | 用 `--id ${EVENT_ID}` |
| 7 | calendar participant list | 文档写 `--event-id`，实际 CLI 是 `--event` | 用 `--event ${EVENT_ID}` |
| 8 | calendar 闲忙查询 | 命令不是 `calendar freebusy`，而是 `calendar busy search` | 用 `dws calendar busy search --users ... --start ... --end ...` |
| 9 | calendar room search | skills/generated 文档要求 `--start`/`--end` 为毫秒时间戳，不接受 ISO-8601 字符串 | 用 `${NOW_PLUS_1H_MS}` / `${NOW_PLUS_2H_MS}` |
| 10 | todo task create | 服务端 NullPointerException，无法创建待办 | 已知服务端 bug，4.2/4.4/4.5/4.6 依赖此步骤均无法执行 | |

## 快速结果记录模板

执行完每条命令后，在下表中填写结果：

| Step | 命令 | 预期 | 实际 | PASS/FAIL |
|------|------|------|------|-----------|
| 1.1 | contact user get-self | 返回 userId | | |
| 1.2 | contact user search | 返回用户列表 | | |
| 1.3 | contact user get | 返回用户详情 | | |
| 1.4 | contact dept search | 返回部门列表 | | |
| 1.5 | contact dept list-members | 返回成员列表 | | |
| 2.1 | aitable template search | 返回模板列表 | | |
| 2.2 | aitable base create | 返回 baseId | | |
| 2.3 | aitable base get | 返回 base 详情 | | |
| 2.4 | aitable base update | success=true | | |
| 2.5 | aitable base list | 返回 base 列表 | | |
| 2.6 | aitable base search | 能搜到测试 base | | |
| 2.7 | aitable table create | 返回 tableId | | |
| 2.8 | aitable table get | 返回字段列表 | | |
| 2.9 | aitable table update | success | | |
| 2.10 | aitable field create | 返回新 fieldId | | |
| 2.11 | aitable field get | 返回字段详情 | | |
| 2.12 | aitable field update | success | | |
| 2.13 | aitable record create | 返回 recordId | | |
| 2.14 | aitable record query | 返回记录列表 | | |
| 2.15 | aitable record update | success | | |
| 2.16 | aitable record query (id) | 验证更新值 | | |
| 3.1 | calendar event create | 返回 eventId | | |
| 3.2 | calendar event get | 返回日程详情 | | |
| 3.3 | calendar event update | success | | |
| 3.4 | calendar event list | 返回日程列表 | | |
| 3.5 | calendar participant list | 返回参与者 | | |
| 3.6 | calendar room search | 返回会议室 | | |
| 3.7 | calendar room list-groups | 返回分组 | | |
| 3.8 | calendar busy search | 返回闲忙时段 | | |
| 3.9 | calendar event delete | success | | |
| 4.1 | todo task create | 返回 taskId | | |
| 4.2 | todo task get | 返回待办详情 | | |
| 4.3 | todo task list | 返回待办列表 | | |
| 4.4 | todo task update | success | | |
| 4.5 | todo task done | success | | |
| 4.6 | todo task delete | success | | |
| 5.1 | chat bot search | 返回机器人列表 | | |
| 6.1 | report template list | 返回模板列表 | | |
| 6.2 | report template detail | 返回字段定义 | | |
| 6.3 | report create | 返回 reportId | | |
| 6.4 | report sent | 返回已发送列表 | | |
| 6.5 | report detail | 返回日志详情 | | |
| 6.6 | report stats | 返回统计数据 | | |
| 6.7 | report list | 返回收件箱 | | |
| 7.1 | ding message send | 返回 openDingId | | |
| 7.2 | ding message recall | success | | |
| 8.1 | attendance record get | 返回考勤记录 | | |
| 8.2 | attendance shift list | 返回排班信息 | | |
| 8.3 | attendance summary | [已知FAIL] | | |
| 8.4 | attendance rules | 返回考勤规则 | | |
| 9.1 | workbench app list | 返回应用列表 | | |
| 10.1 | devdoc article search | 返回文档列表 | | |
| 11.1 | aitable record delete | deletedCount>0 | | |
| 11.2 | aitable field delete | deleted=true | | |
| 11.3 | aitable table delete | [预期FAIL] | | |
| 11.4 | aitable base delete | success | | |

## 基准数据（2026-03-26）

- **总测试条数**: 65（含 4 条清理）
- **预计执行时间**: 6-8 分钟
