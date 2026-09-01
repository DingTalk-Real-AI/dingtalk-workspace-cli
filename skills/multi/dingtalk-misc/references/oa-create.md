# OA 发起审批

仅在用户要真实创建、提交或发起审批时读取本文件。先读 [oa.md](oa.md) 的角色、错误和写操作契约；不要为普通 OA 查询加载本文件。

## 目录

- [按需依赖](#按需依赖)
- [不可降级与单次写入](#不可降级与单次写入)
- [创建闭环](#创建闭环)
- [表单值与能力边界](#表单值与能力边界)
- [考勤审批套件](#考勤审批套件)
- [流程预测与选人](#流程预测与选人)
- [执行前确认](#执行前确认)
- [创建与写后验证](#创建与写后验证)
- [错误收敛](#错误收敛)

## 按需依赖

| 阶段 | 必读内容 |
|---|---|
| 常规表单与自选审批节点 | 只使用本文件与 `scripts/oa_create_preflight.py` 的紧凑输出，不再加载控件/节点全集 |
| 请假审批 | 先用 `dws attendance +get-approve-template --type leave` 定位模板，再按本文件“请假”闭环执行 |
| 补卡审批 | 先用 `dws attendance +get-approve-template --type repair-check` 定位模板，再按本文件“补卡”闭环执行 |
| 紧凑表单输出出现 `needsComponentReference=true` | 只在 [oa-form-components.md](oa/oa-form-components.md) 中定位对应 `componentName` 小节 |
| 紧凑预测输出出现 `needsNodeReference=true`，或用户明确要求覆盖模板默认流程 | 只读取 [oa-process-nodes.md](oa/oa-process-nodes.md) 的对应节点或参数映射小节 |
| 表单包含本地附件 | [oa-attachments.md](oa-attachments.md) |

不要默认同时加载控件、节点和附件全集。先运行投影脚本，再根据其显式布尔字段选择一份精确依赖；禁止为常见 `TextField`、`TableField`、`target_select` 预读完整 reference。需要定位控件时先用 `rg -n '<componentName>|^### ' references/oa/oa-form-components.md` 找到小节，只读取命中范围；零命中即按未知控件边界处理，不打开全文。

## 不可降级与单次写入

把用户确认的创建摘要视为不可变提交契约，并在首次写入前生成一份本地核对清单：模板、部门、全部核心表单字段、审批人、抄送人和附件。尤其注意：用户明确要求的金额、日期、地点、数量、物品/车辆明细和备注都是核心字段，即使 Schema 标记为非必填也不可省略。

对同一份确认摘要只调用一次 `create-instance`：

- 写入前必须完成 Schema 解析、人员解析、`forecast-process` 和本地完整性检查；不得先创建再靠删字段试错。
- 返回 `success: true` 和 `processInstanceId` 后立即停止创建，转入回读；不得再创建“更完整”的第二张，也不得自行撤销重建。
- 返回 `business_error`、`系统错误` 或其他未明确标记 `retryable=true` 的错误时立即停止。通用 `hint`（例如检查参数）不是字段级修正依据；不得改 `deptId`、切换简单/高级模式、删字段、改人员或追加 Help/列表查询后继续碰运气。
- 只有服务端明确指出一个具体字段错误、当前 Schema 给出唯一修正值且能确定实例未创建时，才可本地修正；必须重新展示完整摘要并取得新的确认后，才能进行第二次创建。
- 写成功后若回读发现核心字段缺失或人员不符，报告“不完整实例”并请求用户决定是否撤销；不得自行撤销，也不得把它算作完成。

如果 `TableField` 随完整 payload 返回非可重试业务错误，不得删除整张明细或子字段来换取接口成功。保留 Trace ID，明确该模板当前无法通过 CLI 完整创建。

## 创建闭环

1. 常规审批用 `search-forms` 获取真实模板；已有 `processCode` 时可跳过搜索。请假、补卡改用对应的 `attendance +get-approve-template` 精确定位，不走 `search-forms`。
2. 每次创建前重新调用 `form-schema`，不得复用旧 Schema；默认在本地投影控件摘要，且必须通过 `oa_create_preflight.py form-schema` 完成，禁止临时编写 `jq`/Python 解析器或先读取原始 Schema。
3. 按投影中的 `valueKind`、必填项、选项和明细子控件一次性组装 payload；只有 `needsComponentReference=true` 才定位控件 reference 的对应小节。
4. 对人员姓名使用 `dws aisearch person` 解析并消歧真实 userId。
5. 在本地检查 JSON、`blockers`、必填字段、用户核心字段和模板选项；`blockers` 会递归包含 `TableField` 的必填子控件，非空时停止创建。将用户明确要求的字段标记为不可删除，不用真实写接口试探。
6. 调用 `forecast-process`，展示审批路径并处理自选节点。
7. 一次性展示创建摘要；`create-instance` 的最终 Schema 要求用户确认，确认后才追加 `--yes`。
8. 创建成功后从真实返回取得 `processInstanceId`，再用 `detail` 和必要的 `tasks/records` 回读。

顺序是硬约束：`form-schema → 本地完整 payload → forecast-process → 一次 create-instance → 回读`。不得在 `forecast-process` 前创建；不得在失败后回退到缺字段 payload。

`forecast-process` 必须使用已核验的真实 `deptId` 和准备提交的最终 `form-values`，任一值变化后重新预测；缺少的必填业务值必须追问，不得自行补齐。否则前置未满足，不能将失败归因于 API。

```bash
dws oa approval search-forms --query "<模板关键词>" --format json
dws oa approval form-schema --process-code <processCode> --format json \
  | python3 scripts/oa_create_preflight.py form-schema --process-code <processCode>
dws oa approval forecast-process --process-code <processCode> --dept-id <deptId> --form-values '<JSON对象>' --format json \
  | python3 scripts/oa_create_preflight.py forecast
dws oa approval create-instance --process-code <processCode> --dept-id <deptId> --form-values '<JSON对象>' --yes --format json
dws oa approval detail --instance-id <processInstanceId> --format json
```

脚本只做只读投影，不调用 `create-instance`，并保留完整错误对象。只有脚本明确返回 `projection_error` 时才读取一次原始 JSON；不要同时执行投影版和原始版，也不要用临时解析器替代脚本。投影只压缩读取结果，不改变业务字段。

投影和阻断必须按 `componentName`、`required` 与节点 `actorType` 判断，适用于所有模板。禁止按模板名称、`processCode` 或某个业务字段 label 写日常报销、用车、物品领用等特例；新增控件能力时更新通用映射并补跨模板回归测试。

搜索返回多个相近模板时，展示真实名称和 `processCode` 让用户选择。不得因为模板名称近似而改用另一个模板。

## 表单值与能力边界

`form-schema.result.content` 是控件定义，不是可以直接提交的 payload。投影脚本保留：

- `componentName`；
- `props.label`、`props.required`、`props.options`；
- `TableField.children`；
- 控件是否只读、隐藏或由系统计算。

提交简单模式时，`--form-values` 是 JSON 对象，key 必须与可写控件 label 完全一致。优先按紧凑输出的 `valueKind` 组装；`support=client_only|unknown` 的必填字段进入 `blockers` 并停止，非必填字段进入 `optionalUnavailable` 并在摘要中明确跳过。只有 `needsComponentReference=true` 时才定位 [oa-form-components.md](oa/oa-form-components.md) 的对应小节。

### 未知控件的硬边界

只有控件 reference 明确给出稳定 value 格式时才自动提交。遇到未知控件或未定义稳定格式的控件：

- 必填：停止并说明当前 Skill 无法安全自动提交，请用户改用钉钉客户端或等待能力补齐。
- 非必填：取得用户同意后才可跳过，并在创建摘要中显式列出。

`DDHolidayField` 和 `DDBizSuite · attendance.supply` 只按下方考勤套件闭环提交，必须走高级 `--request`；不能套用 `DDDateRangeField` 或普通 `DDDateField` 的简单模式。

### 明细与核心字段

`TableField` 的每一行必须包含用户要求的核心子字段。创建前逐项对照原始需求，特别检查物品名称、数量、金额、日期、费用类型和备注；不能因为接口接受 payload 就认为业务字段完整。

若用户需求落在 `TableField` 中，父明细控件和相应子字段整体都是核心内容。即使父控件或子控件的 `required` 为 `false`，也不得为绕过服务端错误而删除、拆成顶层字段或提交空行。

禁止创建“测试实例”验证字段格式。若 payload 无法在本地确定，停止并报告，不向真实审批系统写探测数据。

## 考勤审批套件

请假与补卡沿用本文件的单次写入、最终预测、摘要确认和写后回读规则。模板不支持 CLI 时，展示 `+get-approve-template` 返回的全部可用模板及 `[formName](submitUrl)`，由用户在客户端提交；不得只给一个猜测的推荐项。

### 请假：DDHolidayField

1. `dws attendance +get-approve-template --type leave --format json` 获取 `formName/processCode/submitUrl`。多模板时全量展示并让用户选，不自行决定。
2. 对选定模板调用 `form-schema` 并运行预检投影；确认存在 `DDHolidayField`，读取其 `id`、label 数组、`attendTypeLabel` 和 options。
3. `dws attendance approve leave-types --format json` 获取真实 `leaveCode`、类型名、展示单位和余额。用户没有唯一明确类型时全量展示并让其选择；余额已用完则停止。哺乳假或达到 `leaveCertificate` 证明材料阈值时改用 `submitUrl`。
4. 收集起止时间及 Schema 要求的其他字段；调用 `leave-duration` 获取服务端权威的 `durationInHour/durationInDay/detailList/compressedValue/corpId`，再把这些真实值传给 `leave-check`。校验失败时原样交付 `errorMsg` 并停止。
5. 按 [DDHolidayField](oa/oa-form-components.md#ddholidayfield请假套件) 组装包含 `id/name/value/extValue` 的完整请求。`value` 使用 `[T1,T2,duration,unit,leaveName,attendTypeLabel]`；`extValue` 基于 `leave-duration` 返回值补 `key` 和 `leaveParams`，不得本地估算时长。
6. 用最终 `--request` 调用 `forecast-process`，处理所有自选节点；摘要确认后只调用一次 `create-instance --request '<完整JSON>' --yes`。

时间格式必须跟随类型单位：`hour/halfHour/limitHour` 用 `yyyy-MM-dd HH:mm`，`day` 用 `yyyy-MM-dd`，`halfDay` 用 `yyyy-MM-dd 上午/下午`。`leave-check` 的日/半日起止时刻按当前 CLI Help 转为 00:00、12:00 或 23:59。

### 补卡：DDBizSuite · attendance.supply

1. `dws attendance +get-approve-template --type repair-check --format json` 获取模板；多模板时全量展示并让用户选。
2. 调用 `form-schema` 和预检投影，下钻 `DDBizSuite`（`bizType=attendance.supply`）的 `DDDateField` 子控件，保留其 `id/label/format`；同时收集 Schema 要求的补卡理由等字段。若必须上传客户端证据而 CLI 无法提供，改用 `submitUrl`。
3. 用户未给具体缺卡时间时，可按日调用 `dws attendance record get` 辅助定位；随后调用 `dws attendance approve supply-plans --time '<yyyy-MM-dd HH:mm>' --format json`。`plans` 为空即停止，多班次必须展示全部 `planTip` 让用户选择，不默认取第一条。
4. 选定班次后，把真实 `supplyDate` 传给 `dws attendance approve supply-check --timestamp <毫秒时间戳> --format json`；`qualify=false` 时原样交付 `title/desc` 并停止。
5. 按 [补卡套件](oa/oa-form-components.md#ddbizsuite--attendancesupply补卡套件) 组装子控件 `id/name/value/extValue`。`value` 按 Schema 的 format 格式化；`extValue` 使用选定计划的 `planId/planTip/planText/workDate/supplyDate`，不得猜测班次数据。
6. 用最终 `--request` 重新预测，处理自选节点；摘要确认后只调用一次 `create-instance --request '<完整JSON>' --yes`。

请假、补卡任一最终字段、部门或人员发生变化，都必须重新 `forecast-process`。写成功后的验收只读 `detail/tasks/records`，不能再次创建。

## 流程预测与选人

```bash
dws oa approval forecast-process --process-code <processCode> --dept-id <deptId> --form-values '<JSON对象>' --format json
```

简单模式的 `process-code`、`dept-id`、`form-values` 必须一次传齐。先从当前用户信息确定 `deptId`；不得用一次缺参调用来发现约束。

检查 `workflowActivityRuleVOs`：

- 展示节点名称、类型和已确定处理人。
- 常见 `targetSelect: true` 直接使用紧凑输出的 `targetSelections`：`actorType=approver` 绑定用户指定审批人，`actorType=notifier` 绑定用户指定抄送人；`required=false` 的 `bizHandler` 只有用户明确指定时才填写。
- 将 `targetSelections[].actorKey` 作为 `targetSelectActioners[].actionerKey`，人员 userId 放入 `actionerStaffIds`；不得按节点顺序猜角色。
- 只有 `needsNodeReference=true` 或用户明确要求覆盖模板默认流程时，才定位 [oa-process-nodes.md](oa/oa-process-nodes.md) 的对应小节。
- 用户已经指定人员时，先映射到对应节点；只追问仍未覆盖的节点，不重复询问。
- 多个自选节点一次性收集选人结果。
- 只要预测中存在 `targetSelect: true`，就必须使用高级 `--request`；简单模式的 `--approvers` / `--cc-list` 不能替代模板自选节点，也不得作为失败后的降级路径。

需要 `targetSelectActioners`、`directAppointedApprovers` 或其他高级字段时使用 `--request`，不要同时传简单模式 flags：

```bash
dws oa approval create-instance --request '<完整请求JSON>' --yes --format json
```

若不需要高级字段，优先使用 `--process-code`、`--form-values` 和可选 `--approvers`、`--cc-list`，减少手写请求结构。

## 执行前确认

一次性展示：

- 模板名称和 `processCode`；
- 所有将提交的表单值，特别是必填项和明细行；
- 明确跳过的非必填控件；
- 预测出的审批路径；
- 审批人、抄送人及其真实 userId 对应姓名；
- 附件名称和元数据；
- 创建真实审批、可能进入正式流程的影响。

用户先前说“我确认提交”时，只有当随后解析出的模板、字段、人员和流程与其描述完全一致，才能视为对该精确摘要的确认；发现新增选项、未知控件或对象歧义时必须重新确认。

## 创建与写后验证

简单模式示例：

```bash
dws oa approval create-instance \
  --process-code <processCode> \
  --dept-id <deptId> \
  --form-values '<JSON对象>' \
  --approvers "<userId1,userId2>" \
  --approvers-action-type OR \
  --cc-list "<userId3>" \
  --cc-position START \
  --yes --format json
```

只有返回明确成功并包含真实 `processInstanceId` 才能宣称接口已创建。此时禁止再次调用 `create-instance`，随后至少执行：

```bash
dws oa approval detail --instance-id <processInstanceId> --format json
```

回读核对模板、核心表单值、发起人和状态；用户指定了审批人或抄送人时，再用 `tasks` / `records` 或详情中的节点信息验证真实绑定。回读不一致、部分成功或未知状态必须如实保留，且在用户决定前不得自动撤销或重建。

## 错误收敛

- 本地 JSON/重复 key/缺失必填项：不调用写接口，修正后重新展示摘要。
- 服务端返回精确字段校验错误：只按该字段和当前 Schema 修正；不得顺带删掉用户核心字段。修正后的完整摘要必须重新确认，原确认不授权第二次写入。
- `business_error`、`系统错误`、重复错误或任何未标记 `retryable=true` 的错误：首次即停止，不连续改变 payload、模板、部门或人员碰运气。
- `retryable=true`：按服务端节奏重试，保持同一业务对象和 payload；重试后仍失败则报告未创建。
- 没有实例 ID 或回读证据时，不得把搜索、预测或人员定位成功描述为审批已提交。
