# 发起补卡审批（补卡套件 DDBizSuite · attendance.supply）

> **触发：** 用户说"补卡/忘打卡/补打卡/帮我补上次的卡"等补卡意图时，走本文档工作流（**不走 search-forms**）；外出走 [oa-goout.md](oa-goout.md)「发起外出审批」；出差仍按 attendance 域 `+get-approve-template` 的提交链接引导；加班走 [oa-overtime.md](oa-overtime.md)「发起加班审批」。

## 工作流（步骤 1-7 补卡特有；8-9 复用 [oa.md](../oa.md)「发起审批实例」第 5-7 步）

```
1.【模板定位】dws attendance +get-approve-template --type repair-check
   → 补卡模板列表（formName / processCode / submitUrl）
   · 单模板直接选定；多模板时必须交由用户选择，Agent 不得自行选定（名称与“补卡”最匹配的通用模板排前仅影响展示顺序，不改变选择权）；交互组件选项数上限 < 模板总数时改用纯文本/表格列出全量后由用户回复模板名
   · 选项 label 用模板名称（formName）原文；描述文案须在开头完整呈现模板名称原文（直接以名称开头或「模板「名称」：说明」格式均可）——交互组件在不同环境下以 label 或描述作为用户可见主文案，名称须两处都出现，不得只存在于 label；描述其余部分仅作简体中文辅助说明，不得出现 bizType、processCode 等技术标识
   · 返回空 → 告知用户企业未配置可发起的补卡模板
   · submitUrl 仅作兜底（CLI 不支持的模板引导客户端提交）
2.【模板详情】dws oa approval form-schema --process-code <code>
   → 下钻 DDBizSuite（bizType=="attendance.supply"）的 children 取子控件
     DDDateField（bizAlias=="userCheckTime"：id/format/label，format 默认 yyyy-MM-dd HH:mm）
   → 确认补卡理由控件（TextareaField，是否必收以 form-schema 的 required 为准）；图片控件（DDPhotoField）一期跳过并提示客户端补充
3.【定位缺卡（可选）】用户未给时间 → dws attendance record get --user <userId> --date <某日>
   （单日粒度，近 N 天按日循环查询）辅助定位缺卡时间
4.【班次匹配】dws attendance approve supply-plans --time "<yyyy-MM-dd HH:mm>"
   · plans 空 → 转告"该时间无异常班次"并终止，不重试
   · 单班次 → 展示 planTip 确认
   · 多班次 → 列出 planTip 供用户选择；推荐项排序：① 意图词匹配（用户所说日期+上午/下午/上班/下班与候选 workDate/checkType 对应）② 异常班次就近（先过滤查询时刻落在 timeRange 内的候选，再取其中非 freeCheck 且 timeResult≠Normal 者按 |查询时刻−checkDateTime| 最小）③ 其余
   · 意图词唯一命中时可自动选定，但选定 planTip 必须并入后续表单值/汇总确认显式展示供否决；无意图词、意图匹配不唯一、或 freeCheck 候选无 checkDateTime 可就近 → 必须手选（不得默认取首个）
   · 话术硬约束：面向用户的班次澄清/确认一律只含意图词命中依据与最终补卡时刻（如「意图词（08-20 + 下午→下班）唯一命中；最终补卡时刻 08-20 18:00」），选项标签用 planTip 原文；planId、workDate、checkType、timeResult、freeCheck、timeRange 夹取等技术字段与英文枚举不得进入用户话术（交互组件描述同理）
   · 硬底线：create-instance 前用户至少见过一次选定班次的 planTip——推荐排序只优化问的顺序，选定权始终在用户
   · 选定班次的 supplyDate 越出其 timeRange[0]/[1] 时，夹取到最近边界作为最终补卡时刻，并告知用户修正后的时刻
5.【收集理由】按 form-schema 的 required 判定：必填则缺失必问，非必填未提供可跳过；收集交互见 [oa.md](../oa.md)「交互优化原则」第 4 条
6.【提交前校验】dws attendance approve supply-check --timestamp <最终补卡时刻>
   · 最终补卡时刻 = 选定班次 supplyDate；越出 timeRange 时用步骤 4 的夹取值
   · 多班次须选定后再校验：各候选 supplyDate 由服务端按班次微调、可能不同，校验值依赖选择结果（候选 supplyDate 全相同时校验结果才与选择无关）
   · qualify=false → 原样转告 title/desc 并终止，不得跳过重试
7.【组装条目】套件子控件条目 {"id": 子控件props.id, "name": 子控件label,
   "value": 按子控件 format 格式化最终补卡时刻, "extValue": JSON字符串}
   · extValue = {planId?, planTip, planText, workDate, timeStamp(=workDate), userCheckTime(=最终补卡时刻)}
     （timeZoneInfo 不本地拼接：可选字段，服务端 supply-plans 响应不含时区数据）
   · bizAlias 不组装（MCP 通道无此字段，服务端按 id 匹配）；不构造 repairCheckTime（服务端回填）
   + 理由条目 {"id": 理由控件id, "name": "补卡理由", "value": "<用户输入>"}
8.【流程预演（可选；模板含必选自选审批人节点时必做）】forecast-process --request（required=true 时缺 targetSelectActioners 会被服务端拒绝；高级模式：套件条目无法用 --form-values 简单模式承载；--request 下 formComponentValues 与 create-instance 同形态即可，无需手动包二维，实测兼容）
9.【选人 + 确认 + 发起】复用 [oa.md](../oa.md)「发起审批实例」第 6-7 步：自选节点选人（targetSelectActioners 并入 payload）
   → 汇总确认（表单值 + 流程路径 + 审批人）→ create-instance --request '<组装后的完整 JSON>'
```

> **IMPORTANT：** 班次匹配与资格判定一律以服务端（supply-plans / supply-check）为准；value 必须按子控件 `format` 格式化（禁硬编码）；步骤 9 必须走 `--request` 高级模式。流程与选人无补卡特有逻辑，一律按 [oa.md](../oa.md)「发起审批实例」第 5-7 步及其执行摘要执行。

模板不支持 CLI 发起（含图片控件需上传证据等）时的 `submitUrl` 兜底与链接展示规范，见 [oa.md](../oa.md)「发起审批实例」章节的「模板不支持 CLI 发起时：submitUrl 链接引导」小节。

字段级规范见 [oa-form-components.md](oa-form-components.md) 的 DDBizSuite（补卡套件）章节。
