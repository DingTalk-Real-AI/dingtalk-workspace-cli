# 发起外出审批（外出套件 DDBizSuite · attendance.goout）

> **触发：** 用户说"外出/公出/提交外出/帮我提外出申请"等外出意图时，走本文档工作流（**不走 search-forms**）；出差仍按 attendance 域 `+get-approve-template` 的提交链接引导（出差 = `--type out`，外出 = `--type travel`，注意分流）；加班走 [oa-overtime.md](oa-overtime.md)「发起加班审批」。

## 工作流（步骤 1-7 外出特有；8-9 复用 [oa-create.md](../oa-create.md) 的「流程预测与选人 → 执行前确认 → 创建与写后验证」）

```
1.【模板定位】dws attendance +get-approve-template --type travel
   → 外出模板列表（formName / processCode / submitUrl）
   · 单模板直接选定；多模板时必须交由用户选择，Agent 不得自行选定（按名称与"外出"匹配度排序仅影响展示顺序，不改变选择权）；交互组件选项数上限 < 模板总数时改用纯文本/表格列出全量后由用户回复模板名
   · 选项 label 用模板名称（formName）原文；描述文案须在开头完整呈现模板名称原文（直接以名称开头或「模板「名称」：说明」格式均可）——交互组件在不同环境下以 label 或描述作为用户可见主文案，名称须两处都出现，不得只存在于 label；描述其余部分仅作简体中文辅助说明，不得出现 attendance.goout、processCode 等技术标识
   · 返回空 → 告知用户企业未配置可发起的外出模板
   · submitUrl 仅作兜底（CLI 不支持的模板引导客户端提交）
2.【模板详情】dws oa approval form-schema --process-code <code>
   → 下钻 DDBizSuite（bizType=="attendance.goout"）：
     · children 子控件（按 bizAlias 识别）：type（外出类型 DDSelectField）/ startTime / finishTime（DDDateField）/ duration（时长 NumberField）/ traveler（同行人 InnerContactField）
     · 套件 props.childFieldVisible：type / traveler 可见性（false → 该控件不收集、不组装条目；type 不可见时 options 为空数组）
     · 套件 props.unit：type 不可见时的回落时长单位
   → 确认套件外控件（如外出事由 TextareaField，是否必收以 form-schema 的 required 为准）
3.【收集外出类型】childFieldVisible.type≠false 且 options 非空时：
   · 展示全部 options 供用户选择（选项格式：类型名称原文 + 按天/按半天/按小时 中文单位标注；unit 英文枚举不得进入用户话术）；选定后**有效单位 = 该 option 的 extension.unit**
   · 用户中途更换外出类型时：已收集的起止时间与时长全部作废，必须按新有效单位重新收集并重算（对齐客户端切换类型清空行为）
   · type 不可见（options 为空）→ 不收集类型、不组装 type 条目，有效单位 = 套件 props.unit
4.【收集时间 + 同行人 + 事由】（时间与事由的收集交互见 [oa-create.md](../oa-create.md)「交互优化原则」第 4 条）
   · 起止时间按有效单位收集（day → 日期；halfDay → 日期 + 上午/下午；hour → 日期 + 时刻）
   · traveler 可见时可收集同行人（姓名用 dws aisearch person 解析为 userId，上限固定 30——客户端硬编码覆盖模板 max 配置）；无同行人则跳过步骤 5、不组装 traveler 条目
   · 外出事由等套件外控件按 required 收集
5.【同行人校验（有同行人时必做）】dws attendance +check-companion-schedules --approve-type 2 --starts <T1> --ends <T2> --principal-users <uid1,uid2> --duration-unit <DAY|HOUR>
   · T1/T2：有效单位 hour → 精确时刻（yyyy-MM-dd HH:mm:ss）；day/halfDay → 日期（yyyy-MM-dd）
   · duration-unit：有效单位 hour → HOUR；day/halfDay → DAY
   · valid=false → 原样转告 title/alertInfo + 冲突同行人（userIds），请用户剔除后重试；不得跳过校验直接发起
6.【时长计算】dws attendance +calculate-approve-duration --biz-type 2 --approve-biz-type attendance.goout --duration-mode <M> --start <T1> --end <T2> [--half-start AM|PM --half-end AM|PM]
   · 外出固定 --biz-type 2 --approve-biz-type attendance.goout（--biz-type 5 不可用，报业务错误 C0002）
   · 有效单位 → --duration-mode：day → 1；halfDay → 2（必须同时传 --half-start/--half-end，上午=AM、下午=PM）；hour → 3（--start/--end 传 "yyyy-MM-dd HH:mm:00"）
   → durationInHour / durationInDay / detailList / compressedValue（服务端权威，禁止本地估算）
7.【组装条目】展平子控件条目（extract=true，无容器条目；bizAlias 不组装，服务端按 id 匹配）：
   · type（可见时）：{"id":子控件id,"name":"外出类型","value":"<option 显示值>","extValue":<重组 JSON 字符串 {"label":<option.value>,"key":<option.key>,"extension":<option.extension>}——键名是 label 而非 option 中的 value，禁止原样序列化 option 对象>}
   · startTime / finishTime：{"id":子控件id,"name":"开始时间"/"结束时间","value":<有效单位格式>}
   · duration：{"id":子控件id,"name":"时长","value":<数字：有效单位=hour → durationInHour，否则 durationInDay>,"extValue":<步骤 6 响应按 oa-form-components.md 外出套件映射表转换后的 JSON 字符串 + "_from"和"_to"（= 起止 value），禁止原样透传>}
   · traveler（可见且有同行人时）：{"id":子控件id,"name":"同行人","value":"<userId JSON 数组字符串>","extValue":<[{"emplId":uid,"name":姓名,"avatar":"","itemId":uid}] JSON 字符串>}
     —— value 必须是 userId 的 JSON 数组字符串（如 "[\"uid1\",\"uid2\"]"）；姓名显示值会触发服务端系统错误
   · 套件外控件条目（如 {"id":"外出事由","name":"外出事由","value":"…"}，控件 id 以当次 form-schema 为准，可能即中文 label）
8.【流程预演（必选）】forecast-process --request —— 外出模板常见必选自选审批人节点（required=true 时缺 targetSelectActioners 会被服务端拒绝）；自选结果组装字段为 actionerKey（取自本次 forecast 的 workflowActor.actorKey，禁止跨模板/跨流程复用）+ actionerStaffIds（userId）；字段名写成 activityId/actionerUserIds 会创建成功但流转挂起（tasks 返回空 taskIdList）
9.【选人 + 确认 + 发起】复用 [oa-create.md](../oa-create.md) 的「流程预测与选人 → 执行前确认 → 创建与写后验证」：自选节点选人（targetSelectActioners 并入 payload）
   → 汇总确认（外出类型 + 起止时间 + 时长 + 同行人 + 事由 + 流程路径 + 审批人）→ create-instance --request '<组装后的完整 JSON>'
```

有效单位与命令参数对照：

| 有效单位 | 条目 value 格式 | +calculate-approve-duration 参数 |
|---|---|---|
| day | yyyy-MM-dd | --duration-mode 1 --start <日期> --end <日期> |
| halfDay | yyyy-MM-dd 上午/下午 | --duration-mode 2 --start <日期> --end <日期> --half-start AM\|PM --half-end AM\|PM |
| hour | yyyy-MM-dd HH:mm | --duration-mode 3 --start "<日期> HH:mm:00" --end "<日期> HH:mm:00" |

> **IMPORTANT：** 时长、detailList、compressedValue 一律以 `+calculate-approve-duration` 服务端计算为准，严禁本地估算或手改（不支持手改时长）；外出套件为 extract=true 展平形态，不构造 DDBizSuite 容器条目；步骤 8/9 必须走 `--request` 高级模式。

模板不支持 CLI 发起时的 `submitUrl` 兜底与链接展示规范，见 [oa-create.md](../oa-create.md)「考勤审批套件」章节的 submitUrl 引导规范。

字段级规范见 [oa-form-components.md](oa-form-components.md) 的 DDBizSuite · attendance.goout（外出套件）章节。
