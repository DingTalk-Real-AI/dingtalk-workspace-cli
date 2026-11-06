---
name: dingtalk-sport
description: >
  【MCP配置读取】本技能运行时必须先读取同目录下的 mcp-config.json 文件获取 MCP 服务 URL，
  然后使用这些 URL 调用 MCP 服务。严禁在未读取 mcp-config.json 的情况下直接调用 MCP。

  钉钉运动官方数据查询与活动运营服务，为企业管理员提供员工运动数据的查询与分析能力。
  数据来源于员工授权上传的钉钉运动步数，覆盖企业步数榜、部门排行、员工排行、运动趋势
  与健康概况，支撑企业运动文化运营的日常数据管理与活动复盘。同时支持创建企业的运动
  活动（写操作），包含走路、跑步和综合运动卡路里打卡，支持对创建的运动的数据分析以及
  分享到创建的群聊中。每当用户提到步数、步数榜、运动排行、运动比赛/活动、健康概况、
  活动复盘、开赛播报时，都应使用本技能。
compatibility: Requires python3 and network access
metadata:
  label: 企业运动健康服务（钉钉运动）
  version: "1.0"
  category: product
  requires:
    bins:
      - dws
      - python3
---

# 企业运动健康服务（钉钉运动）

## 概述

封装钉钉运动官方 MCP（mcpId=10816），提供两大能力：

1. **数据查询与分析**：企业步数榜、部门人均排行、部门内员工排行、员工多日明细、企业单日健康概况、区间总步数、比赛参与/达标趋势。
2. **运动活动管理**：创建走路/跑步/综合运动（卡路里）比赛并绑定钉钉群，查询比赛配置与各期实例，统计参与与达标人数，向绑定群发送开赛播报。

## MCP 服务配置

本技能依赖以下 MCP 服务，运行时**必须**先读取 `mcp-config.json` 获取服务 URL：

| 环境变量名 | MCP 服务 | mcpId |
|------------|----------|-------|
| `$DINGTALK_SPORT_HEALTH_URL` | 企业运动健康服务（钉钉运动） | 10816 |

### 配置读取方式

**约束：调用方 agent 必须在执行任何 MCP 调用前，先读取 mcp-config.json 文件**

```python
# 示例：读取 mcp-config.json 获取 MCP URL
import json

with open("mcp-config.json", "r", encoding="utf-8") as f:
    config = json.load(f)

# 获取对应服务的 URL（可能为空，由链路注入）
service_url = config["DINGTALK_SPORT_HEALTH_URL"]["url"]
```

## 工具总览（22 个）

| 类别 | 工具 | 用途 |
|------|------|------|
| 前置确认 | `get_user_org_list` | 当前用户所属企业列表（corpId、是否主企业） |
| 前置确认 | `get_org_step_participate_status` | 企业是否开通步数榜（查排行前先确认） |
| 榜单排行 | `list_emp_step_ranking` | 全企业·个人步数排行（D/W/M） |
| 榜单排行 | `list_dept_step_ranking` | 部门人均步数排行（仅 W/M） |
| 榜单排行 | `list_emp_ranking_in_dept` | 指定部门内·个人排行（D/W/M） |
| 步数明细 | `get_emp_step_ext_info` | 员工单日详情（含名次与运动开关状态） |
| 步数明细 | `get_emp_step_infos` | 员工多日步数明细（1~30 天） |
| 步数明细 | `get_dept_step_infos_by_dept_id` | 部门多日步数汇总明细（1~30 天） |
| 健康概况 | `get_org_health_step_info` | 企业单日概况（当日总步数/累计/参与人数） |
| 健康概况 | `get_org_total_step_count` | 企业多日区间步数总和（含首尾） |
| 活动创建 | `create_competition` | 创建运动比赛并绑定钉钉群，返回 ruleCode |
| 活动查询 | `get_competition_config` | 比赛配置详情（目标/周期/起止/报名/启用状态） |
| 活动查询 | `get_competition_rule_detail` | 规则详情（**仅创建人可查**） |
| 活动查询 | `list_competition_instances` | 全部期实例（期号/起止/状态） |
| 活动查询 | `get_current_competition_instance` | 当前进行中的一期（含 competitionCode） |
| 活动查询 | `get_next_competition_instance` | 下一期未开始的实例 |
| 活动统计 | `get_signup_count` | 累计报名人数（只增不减） |
| 活动统计 | `count_daily_participants` | 某天参与人数（有运动数据的人数） |
| 活动统计 | `count_total_participants` | 参与人数（当前/指定期） |
| 活动统计 | `count_reached_users` | 达到每日目标值的人数（DAY/WEEK/MONTH） |
| 活动统计 | `list_daily_participation_trend` | 开期至今每日参与/达标趋势（最多 90 天） |
| 活动播报 | `send_competition_group_msg` | 向比赛绑定群发开赛广播（**无幂等，只调一次**） |

各工具完整入参说明见 [references/tool-catalog.md](references/tool-catalog.md)。

## 工作流程

### 第零步：确认 MCP 可用（两条链路分别处理）

本技能强依赖 MCP 服务「企业运动健康服务（钉钉运动）」（mcpId=10816）。首次使用时按当前链路自检：

- **宿主直连链路**（技能由 Agent 在宿主内触发）：若可用工具列表中没有该服务的 22 个工具（`get_org_health_step_info`、`create_competition` 等），说明宿主未连接该 MCP。引导用户在宿主的 MCP 市场添加「企业运动健康服务（钉钉运动）」连接器后再试；不要跳过自检直接编造调用结果。
- **脚本链路**（通过 `scripts/call_mcp.py` 调用）：读 `mcp-config.json`，若 `url` 为空且 `.env`/环境变量中无 `DINGTALK_SPORT_HEALTH_URL`，引导用户访问 `https://mcp.dingtalk.com/#/detail?mcpId=10816&detailType=marketMcpDetail`，登录钉钉后复制 StreamableHTTP URL，填入技能目录 `.env`（格式 `DINGTALK_SPORT_HEALTH_URL=<url>`）或系统环境变量。配置一次后自动加载。
- 企业侧还有一个独立开关：`get_org_step_participate_status` 返回 `false` 表示企业未开通步数榜（正常业务状态，见场景 A）。

### 第一步：读取配置

读取 `mcp-config.json` 获取 `$DINGTALK_SPORT_HEALTH_URL`；URL 为空说明链路未注入，提示用户而非硬编码 URL。

### 第二步：统一入参规范

- **日期一律 yyyyMMdd 8 位纯数字**（`20261105`，不支持 `2026-11-05`）。
- 周榜/月榜的 `statDate` 传该周/月内任意一天，服务端自动换算所属周/月。
- 排行翻页：首页 `cursor=0`，翻页传上一页返回的 `nextCursor`，`size` 1~100。
- 员工 `userId` 为钉钉 staffId 纯数字，取自 `list_emp_step_ranking` 返回结果；部门 `deptId` 取自 `list_dept_step_ranking` 返回结果。

### 场景 A：步数数据查询与分析

1. 查排行前先调 `get_org_step_participate_status`：返回 `false` 说明企业未开榜（正常业务状态，不是链路故障），应提示用户开通，不要反复重试排行接口。
2. 按需求选工具（选型表）：

| 用户问法 | 工具 |
|----------|------|
| 企业今天走了多少步 / 多少人参与 | `get_org_health_step_info` |
| 某段时间总步数 | `get_org_total_step_count` |
| 员工步数榜（日/周/月） | `list_emp_step_ranking`（statType=D/W/M） |
| 部门排行 | `list_dept_step_ranking`（仅 W/M，无日榜） |
| 某部门内员工排行 | 先 `list_dept_step_ranking` 拿 deptId，再 `list_emp_ranking_in_dept` |
| 某人今天步数和名次 | `get_emp_step_ext_info`（仅单日） |
| 某人最近 N 天步数 | `get_emp_step_infos`（1~30 天，逗号分隔） |
| 某部门最近 N 天表现 | `get_dept_step_infos_by_dept_id` |

3. 数据拉齐后按输出要求汇总。

### 场景 B：创建运动比赛并播报

1. **先与用户确认要素**（缺一项就问）：比赛名称、运动类型（WALK 走路 / RUN 跑步 / COMPREHENSIVE 综合运动）、每日目标值 `goalValue`（单位随类型：步/米/千卡）、达标天数 `targetDayValue`、比赛类型（FIXED 固定 / RECURRING 循环）、绑定群（`groupId`）、起止日期、是否需报名 `needSignup`、是否允许自由加入 `allowFreeJoin`。
2. **入参自检**（创建是真实写操作，参数错了再建无法删除，务必先校验）：
   - FIXED 必填 `firstPeriodEndDate`；RECURRING 必传 `periodType`（WEEK/MONTH/CUSTOM）与 `cycleCount`（1~12，一次性生成全部期）。
   - `targetDayValue` **不得超过单期实际天数**（WEEK=7 / MONTH=28 / FIXED·CUSTOM 按首期起止推算）。超标会报「运动目标设置有误」创建失败。
   - MONTH 周期时首期开始日的「日」必须 ≤28。
   - `dailyLimit` 如传必须 ≥ `goalValue`。
   - `sportScope` 是预留字段（服务端恒覆盖为 ALL），不要传。
   - `coverImageUrl` **优先使用官方默认头图**（见 [references/tool-catalog.md](references/tool-catalog.md) 中的默认图列表），不要自行找图或生图。
3. 调用 `create_competition`（入参为嵌套对象 `CompetitionCreateMcpRequest`，各字段按字符串传值），返回 `ruleCode` 与 `status`：`status=1` 已生效（操作人是群主/管理员）；`status=0` 待群主审批（普通成员创建）。
4. 需要开赛播报时调 `send_competition_group_msg`（服务端按 ruleCode 自动定位绑定群，无需传群参数）：**一次创建只调用一次**，重复调用会重复推送。返回 `true` 仅代表链路执行完成，不保证消息一定可见。
5. 严禁用试探性参数反复创建；查询已有比赛用 `get_competition_config`。

### 场景 C：比赛数据分析与复盘

1. `ruleCode` 来源：`create_competition` 返回值，或用户直接提供。
2. `get_competition_config` 看整体配置；`get_current_competition_instance` 拿当期 `competitionCode` 与本期结束日期（回答「本期什么时候结束」）；`get_next_competition_instance` 回答「下一期什么时候开始」。
3. 按需统计：`get_signup_count`（累计报名）、`count_daily_participants`（某天参与，`statDate` 缺省今天）、`count_total_participants`（参与人数）、`count_reached_users`（达标人数，`rankType` 仅 DAY/WEEK/MONTH，TOTAL 口径不支持）、`list_daily_participation_trend`（逐日参与/达标趋势，`competitionCode` 缺省当期）。
4. 汇总成复盘报告：活动基本信息 → 报名/参与漏斗 → 逐日趋势 → 达标情况 → 结论建议。

### 输出要求

- 榜单类结果以表格输出（名次、昵称、部门、步数等），注明统计口径（日榜/周榜/月榜、统计日期）。
- 人数/步数等单值指标直接给结论，附统计日期。
- 比赛复盘按「基本信息 → 参与趋势 → 达标分析 → 建议」结构输出。
- 接口返回 `null` 的含义按工具说明解释（参数非法 / 无进行中的比赛 / 配置不存在），如实告知用户，不要臆造数据。

## 关键约束与常见坑

1. **`targetDayValue` 上限**：超过活动期实际天数会报 QW_COMPETITION_CREATE_FAILED「运动目标设置有误」（与 goalValue 单位无关）。FIXED 场景按首期起止天数推算上限。
2. **封面图用官方默认图**：字段说明自带公共缺省头图与各类型头图组，直接使用，不要自行生图/找图。
3. **绑定群限制**：`groupId` 与 `openConversationId` 至少传一个（schema 要求 groupId 必传），绑定的群必须属于该 MCP 服务所绑定的企业，跨企业群创建会失败。
4. **`send_competition_group_msg` 无幂等**：重复调用重复推送，一次创建只调一次。
5. **`get_competition_rule_detail` 仅创建人可查**：操作人不是创建人会被拒，改用 `get_competition_config`。
6. **部门榜无日榜**：`list_dept_step_ranking` 仅支持 W/M，日榜需求走 `list_emp_step_ranking`。
7. **开榜前置检查**：未开榜企业查排行返回空列表，是业务状态而非故障。
8. **`count_reached_users` 不支持 TOTAL**：「每日目标值」口径仅 DAY/WEEK/MONTH。
9. **操作人 userId**：是该 MCP 服务所绑定企业内的服务侧 userId（测试企业场景下不是主企业工号），沿用当前会话已验证可用的值。

## MCP 调用示例

```bash
# 1. 先读取 mcp-config.json 中的 URL（执行任何 MCP 调用前必须先做）
DINGTALK_SPORT_HEALTH_URL=$(python3 -c "import json;print(json.load(open('mcp-config.json'))['DINGTALK_SPORT_HEALTH_URL']['url'])")

# 2. 验证服务连通
python3 scripts/call_mcp.py list "$DINGTALK_SPORT_HEALTH_URL"

# 3. 企业单日步数概况
python3 scripts/call_mcp.py call "$DINGTALK_SPORT_HEALTH_URL" get_org_health_step_info \
  --params '{"statDate": "20261105"}'

# 4. 全企业员工周榜（statDate 传该周任一天）
python3 scripts/call_mcp.py call "$DINGTALK_SPORT_HEALTH_URL" list_emp_step_ranking \
  --params '{"statDate": "20261105", "statType": "W", "cursor": 0, "size": 20}'

# 5. 创建走路比赛（真实写操作，参数确认后再调用）
python3 scripts/call_mcp.py call "$DINGTALK_SPORT_HEALTH_URL" create_competition \
  --params '{
    "userId": "操作人userId",
    "CompetitionCreateMcpRequest": {
      "competitionName": "走路挑战赛",
      "sportType": "WALK",
      "competitionType": "FIXED",
      "firstPeriodStartDate": "20261105",
      "firstPeriodEndDate": "20261130",
      "goalValue": "8000",
      "targetDayValue": "18",
      "groupId": "绑定的群id",
      "coverImageUrl": "https://gw.alicdn.com/imgextra/i2/O1CN01Pa9N1djducB1WPcg_!!6000000007915-2-tps-750-580.png",
      "needSignup": "false",
      "allowFreeJoin": "false"
    }
  }'

# 6. 比赛逐日参与/达标趋势（competitionCode 缺省为当前进行中的一期）
python3 scripts/call_mcp.py call "$DINGTALK_SPORT_HEALTH_URL" list_daily_participation_trend \
  --params '{"ruleCode": "从创建返回或用户获取"}'
```

## 参考文档

| 文档 | 说明 | 何时阅读 |
|------|------|----------|
| [mcp-config.json](mcp-config.json) | MCP 服务配置（url 由链路注入） | 每次运行首先读取 |
| [references/tool-catalog.md](references/tool-catalog.md) | 22 个工具完整入参说明与默认封面图 | 调用具体工具前 |
