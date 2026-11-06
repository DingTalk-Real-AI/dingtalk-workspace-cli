# 工具目录：企业运动健康服务（钉钉运动）

所有工具通过 `scripts/call_mcp.py call "$DINGTALK_SPORT_HEALTH_URL" <tool_name> --params '{...}'` 调用。

通用约定：
- 日期格式一律 `yyyyMMdd` 8 位纯数字（如 `20261105`）。
- 周榜/月榜 `statDate` 传该周期内任意一天，服务端换算所属周/月。
- 排行翻页：首页 `cursor=0`，翻页传上一页返回的 `nextCursor`，`size` 1~100。
- 返回 `null` 的含义见各工具说明（参数非法 / 无进行中比赛 / 配置不存在）。

---

## 一、前置确认

### get_user_org_list（用户所属企业列表）

获取当前调用用户所属企业列表，返回企业 corpId、企业名称、是否主企业。入参：无。

用途：多组织场景确认用户所属企业。注意排行等数据工具查的是平台自动注入的当前企业。

### get_org_step_participate_status（企业是否开通步数榜）

入参：无。返回布尔值：`true`=已开通，`false`=未开通。

用途：查排行前先确认，避免把未开榜企业的空列表误判为链路故障。

---

## 二、榜单排行

### list_emp_step_ranking（全企业·员工步数排行）

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| statDate | string | ✅ | 统计日期，yyyyMMdd；周月榜传周期内任一天 |
| statType | string | ✅ | D=日榜 / W=周榜 / M=月榜 |
| cursor | number | ✅ | 分页游标，首页传 0 |
| size | number | ✅ | 每页条数 1~100 |

返回：userId、昵称、部门、当日与周期步数、点赞数。

### list_dept_step_ranking（部门人均步数排行）

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| statDate | string | ✅ | yyyyMMdd；W/M 榜传周期内任一天 |
| statType | string | ✅ | **仅 W / M**（部门榜无日榜） |
| cursor | number | ✅ | 首页传 0 |
| size | number | ✅ | 1~100 |

返回：部门ID（deptId）、名称、周或月人均总步数，按人均降序。deptId 供 `list_emp_ranking_in_dept` 与 `get_dept_step_infos_by_dept_id` 使用。

### list_emp_ranking_in_dept（部门内员工排行）

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| deptId | number | ✅ | 取自 list_dept_step_ranking 结果 |
| statDate | string | ✅ | yyyyMMdd |
| statType | string | ✅ | D / W / M |
| cursor | number | ✅ | 首页传 0 |
| size | number | ✅ | 1~100 |

返回：userId、昵称、当日/周期步数、点赞数。不按是否达标过滤。

---

## 三、步数明细

### get_emp_step_ext_info（员工单日步数详情）

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| userId | string | ✅ | 钉钉 staffId 纯数字，取自 list_emp_step_ranking |
| statDate | string | ✅ | yyyyMMdd，仅单日 |

返回：含名次与钉钉运动开关状态。查超过 1 天必须改用 `get_emp_step_infos`。

### get_emp_step_infos（员工多日步数明细）

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| userId | string | ✅ | 同上 |
| statDates | string | ✅ | 逗号分隔日期串，每项 yyyyMMdd，1~30 个，如 `20261030,20261031,20261101` |

返回：每行含日期、当日步数、部门、昵称，不含名次。

### get_dept_step_infos_by_dept_id（部门多日步数汇总明细）

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| deptId | number | ✅ | 取自 list_dept_step_ranking 结果 |
| statDates | string | ✅ | 逗号分隔日期串，1~30 个 |

返回：每行含日期、部门总步数、总人数。

---

## 四、健康概况

### get_org_health_step_info（企业单日步数概况）

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| statDate | string | ✅ | yyyyMMdd |

返回：当日总步数、累计总步数、参与人数。单日查询首选。

### get_org_total_step_count（企业区间总步数）

| 参数 | 类型 | 必填 | 说明 |
|------|------|------|------|
| beginStatDate | string | ✅ | yyyyMMdd，含当天 |
| endStatDate | string | ✅ | yyyyMMdd，含首尾两天 |

仅返回一个总步数，不含其他指标。

---

## 五、活动创建

### create_competition（创建企业运动比赛）

**真实写操作，会向绑定群长期生效，禁止试探性调用。** 查询已有比赛用 get_competition_config。

入参结构：顶层 `userId` + `CompetitionCreateMcpRequest` 对象（各字段按字符串传值）：

| 字段 | 必填 | 说明 |
|------|------|------|
| competitionName | ✅ | 比赛名称 |
| sportType | ✅ | WALK 走路（goalValue 单位=步）/ RUN 跑步（=米）/ COMPREHENSIVE 综合运动（=千卡） |
| competitionType | ✅ | FIXED 固定不循环 / RECURRING 周期循环 |
| firstPeriodStartDate | ✅ | 首期开始，严格 yyyyMMdd；MONTH 时「日」必须 ≤28 |
| firstPeriodEndDate | FIXED 必填 | 结束日期 yyyyMMdd |
| periodType | RECURRING 必传 | WEEK / MONTH / CUSTOM。FIXED 不要传 |
| cycleCount | RECURRING 必传 | 循环总场次 1~12，创建时一次性生成全部期。FIXED 不要传 |
| goalValue | ✅ | 每日目标值 >0，单位随 sportType（步/米/千卡） |
| targetDayValue | ✅ | 周期内需达标天数 >0，**不得超过单期天数**（WEEK=7 / MONTH=28 / FIXED·CUSTOM 按首期起止推算） |
| groupId | ✅（与 openConversationId 至少一个） | 绑定群 id。群必须属于服务绑定的企业 |
| openConversationId | 备选 | 群会话 id |
| needSignup | ✅ | 是否需报名确认（规则级跨期生效）：true=须手动报名；false=自动视为已报名 |
| allowFreeJoin | ✅ | 是否允许非群成员凭分享链接报名；false 时仅群成员可报名 |
| coverImageUrl | ✅ | 封面图，**优先用下方官方默认图** |
| dailyLimit | 可选 | 每日运动量上限，单位同 goalValue，必须 ≥ goalValue |
| sessionId | 可选 | 会话 id |
| sportScope | ❌ 不要传 | 预留字段，服务端恒覆盖为 ALL |

返回：新比赛 `ruleCode` 与 `status`（1=已生效，群主/管理员创建；0=待群主审批，普通成员创建）。

**官方默认封面图**（直接使用，不要自行找图/生图）：

```
# 公共缺省头图（所有类型可共用）
https://gw.alicdn.com/imgextra/i2/O1CN01Pa9N1djducB1WPcg_!!6000000007915-2-tps-750-580.png

# 走路头图组
https://gw.alicdn.com/imgextra/i4/O1CN01BsCyvgi98kJ1WPcg_!!6000000007633-2-tps-750-580.png
https://gw.alicdn.com/imgextra/i4/O1CN014nnKhTFIxAJ1WPcg_!!600000000777-2-tps-750-580.png

# 跑步头图组
https://gw.alicdn.com/imgextra/i1/O1CN01ypZ66nwns5B1WPcg_!!6000000003383-2-tps-750-580.png

# 综合运动头图组
https://gw.alicdn.com/imgextra/i3/O1CN01PFo0welyHiB1WPcg_!!600000000329-2-tps-750-580.png
https://gw.alicdn.com/imgextra/i4/O1CN01kulSdTeRTdB1WPcg_!!6000000002214-2-tps-750-580.png
https://gw.alicdn.com/imgextra/i3/O1CN01pprYpSr69TB1WPcg_!!600000000628-2-tps-750-580.png
```

---

## 六、活动查询

### get_competition_config（比赛配置详情）

| 参数 | 必填 | 说明 |
|------|------|------|
| ruleCode | ✅ | 活动 ruleCode |

返回：名称/运动项目/每日目标值/达标天数目标/周期配置/整体起止日期/是否需报名/累计报名人数/启用状态。`null`=corpId 无法解码或配置不存在。

### get_competition_rule_detail（比赛规则详情·仅创建人）

| 参数 | 必填 | 说明 |
|------|------|------|
| userId | ✅ | 操作人 userId（必须是比赛创建人，否则被拒） |
| ruleCode | ✅ | 活动 ruleCode |

返回：名称、头图、运动项目、比赛类型、周期类型、循环次数、整体起止、首期起止、每日目标值、达标天数目标、每日上限、是否需报名/自由加入、绑定群 cid 与群名、报名人数、规则状态 status（0 待审批/1 已生效）、比赛状态 competitionStatus（0 未开始/1 进行中/2 已结束）。

### list_competition_instances（全部期实例）

| 参数 | 必填 | 说明 |
|------|------|------|
| ruleCode | ✅ | ruleCode |

返回：期号/标题/起止日期/状态，按 cycleNo 升序。

### get_current_competition_instance（当前进行中的一期）

| 参数 | 必填 | 说明 |
|------|------|------|
| ruleCode | ✅ | 活动 ruleCode |

返回：含 competitionCode 与本期结束日期（回答「本期什么时候结束」）。`null`=当前没有进行中的实例。

### get_next_competition_instance（下一期未开始的实例）

| 参数 | 必填 | 说明 |
|------|------|------|
| ruleCode | ✅ | 活动 ruleCode |

返回：含开始日期（回答「下一期什么时候开始」）。

---

## 七、活动统计

### get_signup_count（累计报名人数）

| 参数 | 必填 | 说明 |
|------|------|------|
| ruleCode | ✅ | 活动 ruleCode |

任何报名路径首次成功时自增，只增不减。`null`=配置不存在。

### count_daily_participants（某天参与人数）

| 参数 | 必填 | 说明 |
|------|------|------|
| ruleCode | ✅ | 活动 ruleCode |
| statDate | 可选 | 统计日期，缺省为今天 |

查询当前进行中的一期比赛某天的参与人数（当天有运动数据的人数）。`null`=参数非法或没有进行中的实例。

### count_total_participants（参与人数）

| 参数 | 必填 | 说明 |
|------|------|------|
| ruleCode | ✅ | 活动 ruleCode |
| competitionCode | 可选 | 当前活动期 |

### count_reached_users（达标人数）

| 参数 | 必填 | 说明 |
|------|------|------|
| ruleCode | ✅ | 活动 ruleCode |
| rankType | ✅ | 仅 DAY / WEEK / MONTH（TOTAL 口径不支持） |
| queryDate | ✅ | 统计时间 |
| competitionCode | ✅ | 活动当前期 |

返回达到每日目标值（goalValue）的人数。`null`=参数非法或当前没有进行中的比赛。

### list_daily_participation_trend（逐日参与/达标趋势）

| 参数 | 必填 | 说明 |
|------|------|------|
| ruleCode | ✅ | 活动实例 |
| competitionCode | 可选 | 缺省为当前进行中的一期 |

返回开期至今每天的参与人数与达标人数，日期升序，最多 90 天，可生成活动进展曲线。`null`/空数组=参数非法或实例尚未开始。

---

## 八、活动播报

### send_competition_group_msg（发送比赛群消息）

| 参数 | 必填 | 说明 |
|------|------|------|
| userId | ✅ | 操作人 userId |
| ruleCode | ✅ | 活动实例 code |

服务端按 ruleCode 定位当期比赛实例并取其绑定群，**无需指定群**。规则已生效（status=1）发开赛广播（文字+活动卡片）；待审批（status=0）发 at 群主的审批提醒（仅群主与发起可见）。

**注意**：无幂等，重复调用重复推送，一次创建只调一次；返回 `true` 仅代表链路执行完成，不代表消息一定可见。
