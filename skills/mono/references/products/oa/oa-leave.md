# 发起请假审批（请假套件 DDHolidayField）

> **触发：** 用户说"请假/请X天假/请年假/请事假/请病假/提交请假"等请假意图时，走本文档工作流（**不走 search-forms**）；补卡走 [oa-supply.md](oa-supply.md)「发起补卡审批」；外出走 [oa-goout.md](oa-goout.md)「发起外出审批」；出差仍按 attendance 域 `+get-approve-template` 的提交链接引导；加班走 [oa-overtime.md](oa-overtime.md)「发起加班审批」。

## 工作流（步骤 1-8 请假特有；9-10 复用 [oa.md](../oa.md)「发起审批实例」第 5-7 步）

```
1.【模板定位】dws attendance +get-approve-template --type leave
   → 请假模板列表（formName / processCode / submitUrl）
   · 单模板直接选定；多模板时必须交由用户选择，Agent 不得自行选定（按用户假期词匹配 formName 排序仅影响展示顺序，不改变选择权）；交互组件选项数上限 < 模板总数时改用纯文本/表格列出全量后由用户回复模板名
   · 选项 label 用模板名称（formName）原文；描述文案须在开头完整呈现模板名称原文（直接以名称开头或「模板「名称」：说明」格式均可）——交互组件在不同环境下以 label 或描述作为用户可见主文案，名称须两处都出现，不得只存在于 label；描述其余部分仅作简体中文辅助说明，不得出现 bizType、processCode 等技术标识
   · 返回空 → 告知用户企业未配置可发起的请假模板
   · submitUrl 仅作兜底（CLI 不支持的模板引导客户端提交）
2.【模板详情】dws oa approval form-schema --process-code <code>
   → 识别 DDHolidayField（componentName）+ props.options（leaveCode/unit/name）+ 其余控件（请假事由等）
   · 无 DDHolidayField → 降级普通表单流程（简单模式 --form-values 发起，无需步骤 5/6）
3.【先类型】dws attendance approve leave-types（--user 可选，缺省当前用户；代提交用其 userId）
   · 自动匹配：用户已明确类型词时匹配返回的 leaveName，唯一命中直接用
   · 无类型词、未命中或含糊 → 展示全部可用类型供选择，不预筛子集；选项格式：【类型名称】 (剩余 X 天/小时)，X 取 balance.remainQuota，单位取 quotaUnit 中文映射（day/halfDay→天、hour→小时）；无余额对象或 balanceHidden=true 时不追加括号（不展示“余额不可见”类文案）；选中后剩余≤0 → 提示「你的XX余额已用完」并终止，按 X 计（X 取 leaveViewUnit 中文映射）；面向用户的展示一律用中文文案，不得出现 hour/halfDay/day 等英文枚举；「全部」是硬约束：交互组件选项数上限 < 类型总数时，禁止挑“代表性子集”，必须放弃该组件改用纯文本/表格列出全量后由用户回复类型名；选项主标识必须是【类型名称】原文，不得截断、省略或被余额/状态描述取代
   · 类型一经确定，leaveCode 取自同一条目（与 leaveName 同源）
   · 哺乳假判定：类型条目 bizType === "breastfeeding_leave_new" → 明确拒绝并引导客户端（用步骤 1 的 submitUrl）；bizType 缺失时回退名称含「哺乳」；证明材料判定：leaveCertificate（enable/unit/duration/promptInformation）在 leave-types 响应中直接返回；enable=true 时步骤 5 拿到时长后**双向换算为小时**比较——阈值：leaveCertificate.unit=day → duration×24、hour → duration 原值；用户时长：unit ∈ {hour,halfHour,limitHour} → durationInHour 原值（**不乘 24**）、day/halfDay → durationInDay×24；时长 ≥ 阈值则同样拒绝并引导客户端
4.【再时间 + 事由】按选定类型的 leaveViewUnit 格式化起止时间；同时收集请假事由（按 form-schema 的 required 判定：必填则缺失必问，非必填未提供可跳过）；时间范围与事由的收集交互见 [oa.md](../oa.md)「交互优化原则」第 4 条
5.【后时长】dws attendance approve leave-duration --leave-code <leaveCode> --start <T1> --end <T2>
   → durationInHour / durationInDay / detailList / compressedValue / corpId （服务端权威，禁止本地估算）
   → 粒度校验：unit=halfHour → durationInHour 须为 0.5 的倍数、unit=limitHour → 须为整数，不满足则提示「时长不符合单位要求」并终止
6.【提交前校验】dws attendance approve leave-check --leave-code … --process-code … --start <T1'> --end <T2'> --duration-day <D> --duration-hour <H>
   · D/H 必须取自步骤 5 输出；T1'/T2' 为时刻转换后的值（day：起 00:00/止 23:59；halfDay 上午：起 00:00/止 12:00，下午：起 12:00/止 23:59；hour/halfHour/limitHour 原样）
   · success=false → 原样转告 errorMsg 并终止，不得跳过重试
7.【组装 value】value = [T1, T2, duration, unit, leaveName, attendTypeLabel]（JSON 数组字符串）
   · unit / leaveName = 步骤 3 选定类型的 leaveViewUnit / leaveName 原始值（中文映射不写入）
   · duration = unit ∈ {hour, halfHour, limitHour} ? durationInHour : durationInDay
   · attendTypeLabel = 套件 props.attendTypeLabel，无则取 props.push.pushTag + "类型"，均无为 ""
8.【组装条目】套件条目 {"id": props.id, "name": JSON.stringify(label 数组)（如 "[\"开始时间\",\"结束时间\"]"）, "value": 六元数组字符串, "extValue": extendValue字符串}
   + 其余控件条目（如 {"name":"请假事由","value":"…"}）
   · extendValue = JSON.stringify({...步骤5响应, key: leaveCode, leaveParams: [corpId, leaveCode, T1, T2, staffId]})
   · corpId 取步骤 5 响应回显；本人发起 staffId=null
9.【流程预演（可选；模板含必选自选审批人节点时必做）】forecast-process --request（required=true 时缺 targetSelectActioners 会被服务端拒绝；高级模式：套件条目无法用 --form-values 简单模式承载；--request 下 formComponentValues 与 create-instance 同形态即可，无需手动包二维，实测兼容）
10.【选人 + 确认 + 发起】复用 [oa.md](../oa.md)「发起审批实例」第 6-7 步：自选节点选人（targetSelectActioners 并入 payload）
    → 汇总确认（表单值 + 流程路径 + 审批人）→ create-instance --request '<组装后的完整 JSON>'
```

时间格式（与模板 unit 硬绑定）：

| unit | T1/T2 格式 | duration 取值 |
|---|---|---|
| hour / halfHour / limitHour | yyyy-MM-dd HH:mm | durationInHour |
| day | yyyy-MM-dd | durationInDay |
| halfDay | yyyy-MM-dd 上午/下午 | durationInDay（0.5 粒度） |

> **IMPORTANT：** 时长、detailList、compressedValue 一律以 `leave-duration` 服务端计算为准，严禁本地估算或手改（不支持 customDuration）；简单模式 `--form-values` 无法承载套件条目（value 为数组、含 extValue），步骤 9/10 必须走 `--request` 高级模式。

模板不支持 CLI 发起（哺乳假、需上传证明材料等）时的 `submitUrl` 兜底与链接展示规范，见 [oa.md](../oa.md)「发起审批实例」章节的「模板不支持 CLI 发起时：submitUrl 链接引导」小节。

字段级规范（id/name/value/extValue 组装细则与不支持边界）见 [oa-form-components.md](oa-form-components.md) 的 DDHolidayField 章节。
