# OA 发起审批

仅在用户要真实创建、提交或发起审批时读取本文件。先读 [oa.md](oa.md) 的角色、错误和写操作契约；不要为普通 OA 查询加载本文件。

## 目录

- [按需依赖](#按需依赖)
- [创建闭环](#创建闭环)
- [表单值与能力边界](#表单值与能力边界)
- [流程预测与选人](#流程预测与选人)
- [执行前确认](#执行前确认)
- [创建与写后验证](#创建与写后验证)
- [错误收敛](#错误收敛)

## 按需依赖

| 阶段 | 必读内容 |
|---|---|
| 解析并组装表单控件 | [oa-form-components.md](oa/oa-form-components.md) |
| `forecast-process` 返回自选节点，或用户要求覆盖模板审批路径 | [oa-process-nodes.md](oa/oa-process-nodes.md) |
| 表单包含本地附件 | [oa-attachments.md](oa-attachments.md) |

不要默认同时加载控件、节点和附件全集。先读 Schema，再根据真实控件和预测结果选择依赖。

## 创建闭环

1. 用 `search-forms` 获取真实模板；已有 `processCode` 时可跳过搜索。
2. 每次创建前重新调用 `form-schema`，不得复用旧 Schema。
3. 递归解析可写控件、必填项、选项和明细子控件；读取控件 reference 后一次性组装 payload。
4. 对人员姓名使用 `dws aisearch person` 解析并消歧真实 userId。
5. 在本地检查 JSON、必填字段、用户核心字段和模板选项，不用真实写接口试探。
6. 调用 `forecast-process`，展示审批路径并处理自选节点。
7. 一次性展示创建摘要；`create-instance` 的最终 Schema 要求用户确认，确认后才追加 `--yes`。
8. 创建成功后从真实返回取得 `processInstanceId`，再用 `detail` 和必要的 `tasks/records` 回读。

```bash
dws oa approval search-forms --query "<模板关键词>" --format json
dws oa approval form-schema --process-code <processCode> --format json
dws oa approval forecast-process --process-code <processCode> --dept-id <deptId> --form-values '<JSON对象>' --format json
dws oa approval create-instance --process-code <processCode> --dept-id <deptId> --form-values '<JSON对象>' --yes --format json
dws oa approval detail --instance-id <processInstanceId> --format json
```

搜索返回多个相近模板时，展示真实名称和 `processCode` 让用户选择。不得因为模板名称近似而改用另一个模板。

## 表单值与能力边界

`form-schema.result.content` 是控件定义，不是可以直接提交的 payload。解析时至少保留：

- `componentName`；
- `props.label`、`props.required`、`props.options`；
- `TableField.children`；
- 控件是否只读、隐藏或由系统计算。

提交简单模式时，`--form-values` 是 JSON 对象，key 必须与可写控件 label 完全一致。人员、部门、选项、明细和附件必须按 [oa-form-components.md](oa/oa-form-components.md) 的明确格式组装。

### 未知控件的硬边界

只有控件 reference 明确给出稳定 value 格式时才自动提交。遇到未知控件或未定义稳定格式的控件：

- 必填：停止并说明当前 Skill 无法安全自动提交，请用户改用钉钉客户端或等待能力补齐。
- 非必填：取得用户同意后才可跳过，并在创建摘要中显式列出。

`DDHolidayField` 当前没有公开、稳定且经 CLI 验证的 value 契约，不能套用 `DDDateRangeField`。包含必填 `DDHolidayField` 的请假模板暂不自动提交。

### 明细与核心字段

`TableField` 的每一行必须包含用户要求的核心子字段。创建前逐项对照原始需求，特别检查物品名称、数量、金额、日期、费用类型和备注；不能因为接口接受 payload 就认为业务字段完整。

禁止创建“测试实例”验证字段格式。若 payload 无法在本地确定，停止并报告，不向真实审批系统写探测数据。

## 流程预测与选人

```bash
dws oa approval forecast-process --process-code <processCode> --dept-id <deptId> --form-values '<JSON对象>' --format json
```

检查 `workflowActivityRuleVOs`：

- 展示节点名称、类型和已确定处理人。
- 发现 `targetSelect: true` 时，读取 [oa-process-nodes.md](oa/oa-process-nodes.md)，使用 `workflowActor.actorKey` 组装 `targetSelectActioners`。
- 用户已经指定人员时，先映射到对应节点；只追问仍未覆盖的节点，不重复询问。
- 多个自选节点一次性收集选人结果。

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

只有返回明确成功并包含真实 `processInstanceId` 才能宣称创建成功。随后至少执行：

```bash
dws oa approval detail --instance-id <processInstanceId> --format json
```

回读核对模板、核心表单值、发起人和状态。需要验证任务或流程记录时再读取 `tasks` / `records`。回读不一致、部分成功或未知状态必须如实保留。

## 错误收敛

- 本地 JSON/重复 key/缺失必填项：不调用写接口，修正后重新展示摘要。
- 服务端返回精确字段校验错误：只按该字段和当前 Schema 修正一次；不得顺带删掉用户核心字段。
- 重复或未标记 `retryable=true` 的业务错误：停止，不连续改变 payload、模板或人员碰运气。
- `retryable=true`：按服务端节奏重试，保持同一业务对象和 payload；重试后仍失败则报告未创建。
- 没有实例 ID 或回读证据时，不得把搜索、预测或人员定位成功描述为审批已提交。
