# 发起加班审批（加班套件 DDBizSuite · attendance.batchovertime）

> **触发：** 用户说"加班/提交加班/帮我提加班申请/代XX提交加班"等加班意图时。**时长计算**：calculate_approve_duration 支持 durationInHour/durationInDay/detailList/modifiedDate 入参（CLI 暴露 --duration-in-hour/--duration-in-day/--detail-list/--modified-date）——歧义窗口（班中起始/跨天）传 --detail-list 逐日明细可算（服务端按逐日求和、采信提议值不裁决截断），无歧义窗口服务端可自算。**时长结果须经用户手动确认**。

## 工作流（时长计算结果须经用户手动确认。步骤 1-8 加班特有，9-10 复用 [oa.md](../oa.md)「发起审批实例」第 5-7 步）

```
1.【模板定位】dws attendance +get-approve-template --type overtime
   → 加班模板列表（formName / processCode / submitUrl）
   · 单模板直接选定；多模板时必须交由用户选择，Agent 不得自行选定（选项 label 与描述规范同请假/补卡/外出：label 用模板名称原文、描述开头完整呈现模板名称原文）
   · 返回空 → 告知用户企业未配置可发起的加班模板
   · submitUrl 仅作兜底（CLI 不支持的模板引导客户端提交）
2.【模板详情】dws oa approval form-schema --process-code <code>
   → 下钻 DDBizSuite（bizType=="attendance.batchovertime"；schema 无 extract → 容器包裹提交，与补卡/外出展平形态相反）：
     · 套件 props：id（容器条目 id 主键）/ label / unit（模板静态单位，可被步骤 4 动态单位覆盖）/ childFieldVisible（子字段可见性过滤：某 bizAlias 值 === false → 该子控件不收集不组装；仅 === false 生效，缺 key 即显示；规则详见 [oa-form-components.md](oa-form-components.md) 加班套件章节）
     · children 子控件（按 bizAlias 识别）：partner（加班人 InnerContactField，单/多选与上限以当次 form-schema props 为准）/ startTime / finishTime（DDDateField）/ everyDayDuration（TableField 多天明细，含 overtimeDate + overtimeDuration 两子控件）/ duration（总时长 NumberField）/ compensation（加班补偿 DDSelectField，manual 补偿时必选）
     · invisible/hidden/纯展示控件不收集、不组装条目：type 加班类型（多天按 detailList dayType 优先级 holiday>restDay>workDay 自动回填，CLI 不组装）、partnerTip、punchDetails 打卡明细（纯展示）
   → 无 everyDayDuration 明细组件 → 旧版模板，降级 submitUrl 引导
   → 确认套件外控件（如加班原因 TextField，required 以当次 form-schema 为准）
3.【收集加班人】默认本人；代提交/批量经 dws aisearch person 解析为 userId；单/多选与上限以当次 form-schema partner props 为准；套件 `childFieldVisible.partner===false` → 模板无加班人字段：跳过收集、不组装 partner 条目（仅发起人本人），代提交/批量意图直接告知该模板不支持
4.【单位校验（必做）】dws attendance +get-complex-overtime-setting --users <加班人 uid[,uid...]> [--work-date yyyy-MM-dd]
   → interactMode：1=day / 2=halfDay / 3=hour = 有效单位；与模板 props.unit 不一致时以本结果为准（起止格式、时长取值字段随之切换）并告知用户「单位按员工组规则生效」
   · reason 非空 → 该员工组禁止加班，原样转告并终止，不重试
   · workDate 语义=加班日期（初始为当前日 0 点，选定起止后为起始日 0 点）
5.【收集时间 + 事由】起止时间按有效单位收集（day → 日期；halfDay → 日期 + 上午/下午；hour → 日期 + 时刻；事由收集交互见 [oa.md](../oa.md)「交互优化原则」第 4 条的事由/理由规范）
   · 支持多日加班：跨度上限 7 天（前端按跨度拦截、服务端按时长 ≤7 天），超出提示拆分；多日逐日时长经步骤 6 两阶段确认收集
6.【时长计算（两阶段确认）】dws attendance +calculate-approve-duration --biz-type 1 --new-overtime --duration-mode <M> --start <T1> --end <T2> [--half-start AM|PM --half-end AM|PM] [--principal-users <uid,...>]
   · **时长计算口径**：歧义窗口（班中起始/跨天需逐日拆分）必须携带 --detail-list 逐日明细（JSON 数组，逐项 workDate 为毫秒 number——字符串形态会被服务端静默忽略返回 durationInHour=0，CLI 透传前统一归一化）+ durationInHour|durationInDay；服务端按逐日求和为总时长（总时长字段透传但不参与计算），--duration-in-hour（hour）/--duration-in-day（day/halfDay）与 --modified-date 可选携带；**服务端直接采信逐日提议值、不做裁决截断**——逐日值必须来自用户手动确认；无歧义窗口（整段班次外单日）服务端可自算成功（--start/--end 须完整秒级 HH:mm:ss），自算结果同样必须经下方硬约束由用户手动确认
   · 加班固定 --biz-type 1 --new-overtime；--duration-mode 由有效单位决定（day→1；halfDay→2 必须同传 --half-start/--half-end；hour→3，--start/--end 传 "yyyy-MM-dd HH:mm:00"）
   · 一阶段（起止确定后调用）：无歧义窗口服务端自算成功（durationInHour/detailList/compressedValue 齐备）；歧义窗口（班中起始/跨天）不带 --detail-list 时服务端返回 durationInHour=0 + 逐日骨架（detailList 给出 workDate/班次段/逐日起止窗），不报错——判歧义依据为「durationInHour=0 且窗口跨天/班中起始」→ 进入多日确认
   · **时长必须经用户手动确认（硬约束）**：一阶段结果（自算时长或逐日骨架）必须向用户完整展示（数值 + 单位中文口径，多日含逐日明细），经用户显式确认或输入逐日时长后方可进入二阶段与组装；禁止静默采信服务端结果直接组装提交；用户有异议 → 回步骤 5 重新收集时间并重新计算
   · **时长确认的交互形态**：遵循 [oa.md](../oa.md)「交互优化原则」第 5 条选择澄清优先（宿主有组件时用选择澄清，禁止退化为纯文本填空）。
   · 多日确认：一阶段判定跨天 → 向用户展示逐日骨架并由用户**手动确认/输入每天加班时长**（选择权在用户，交互形态见上条）→ 二阶段调用（--duration-in-hour/--duration-in-day=用户确认总时长（按有效单位选）+ --detail-list=逐日明细 JSON，逐项 workDate 为 yyyy-MM-dd HH:mm:ss 字符串或毫秒时间戳，CLI 归一化为毫秒透传）由服务端确认——服务端采信逐日提议值、不裁决截断
   · 单日确认：一阶段判定单日 → 向用户展示服务端计算时长并由用户**手动确认/修改** → 二阶段调用（duration=确认值）确认
   · --principal-users 不传时服务端默认按发起人计算（本人发起不传）；代提交/批量必传全量加班人；响应 excludePrincipalUserIds 非空 → 转告被排除人员，用户确认后以剩余人员继续
   · 响应外层 overtimeDurationStatus≠0 → 原样转告 message 并终止；userMessage 非空 → 并入汇总确认展示
   · 响应 approveAlertInfo（班次冲突 title/content/detailKey）非空 → 原样转告；明细级 detailList[].approveInfo.overtimeDurationStatus：0=回填 / 1=回填并提示 message / 2=该日禁止加班 → 终止并转告
   → durationInHour / durationInDay / detailList / compressedValue / featureMap（服务端权威，禁止本地估算或手造）
7.【补偿方式】步骤 6 响应 overtimeRedressBy=="manual" → 请用户选转调休 / 加班费；否则跳过；manual 但用户弃选 → 终止（required）。发起前汇总仅在 manual 选值时展示补偿方式；空条目（服务端自动配置）不展示该字段
8.【组装条目】容器包裹形态（与补卡/外出展平相反；字段级规范见 [oa-form-components.md](oa-form-components.md) 加班套件章节）：
   · 容器条目：{"id":套件 props.id,"name":套件 props.label,"value":<JSON.stringify(children 条目数组)>,"extValue":""}
   · children 按字段规范组装：partner / startTime / finishTime / duration（value=总时长字符串，hour → durationInHour，否则 durationInDay）/ compensation 按步骤 7
   · 套件外控件（如加班原因）按通用规则组装
9.【流程预演（必选）】forecast-process --request（必选自选审批人节点 required=true 时缺 targetSelectActioners 会被服务端拒绝；组装字段为 actionerKey + actionerStaffIds）
10.【选人 + 确认 + 发起】复用 [oa.md](../oa.md)「发起审批实例」第 6-7 步 → 汇总确认（加班人 + 起止时间 + 时长 + 事由 + 流程路径 + 审批人；补偿方式仅在 manual 选值时列入）→ create-instance --request '<组装后的完整 JSON>'
```

有效单位与命令参数对照：

| 有效单位（interactMode） | 起止条目 value 格式 | +calculate-approve-duration 参数 |
|---|---|---|
| day（interactMode=1） | yyyy-MM-dd | --duration-mode 1 --start <日期> --end <日期> |
| halfDay（interactMode=2） | yyyy-MM-dd 上午/下午 | --duration-mode 2 --start <日期> --end <日期> --half-start AM\|PM --half-end AM\|PM |
| hour（interactMode=3） | yyyy-MM-dd HH:mm | --duration-mode 3 --start "<日期> HH:mm:00" --end "<日期> HH:mm:00" |

> **IMPORTANT：**
> - 加班套件为**容器包裹**形态（schema 无 extract）：容器条目 value=stringify(children)，与补卡/外出的展平形态相反，先例不可跨类型推用。
> - 时长、detailList、compressedValue 一律以 `+calculate-approve-duration --biz-type 1 --new-overtime` 服务端计算为准，严禁本地估算/手改；**任何来源的时长结果都必须经用户手动确认（步骤 6 硬约束），未确认不得进入组装与发起**；服务端采信逐日提议值、不做裁决截断。

模板不支持 CLI 发起时的 `submitUrl` 兜底与链接展示规范，见 [oa.md](../oa.md)「发起审批实例」章节的「模板不支持 CLI 发起时：submitUrl 链接引导」小节。

字段级规范见 [oa-form-components.md](oa-form-components.md) 的 DDBizSuite · attendance.batchovertime（加班套件）章节。
