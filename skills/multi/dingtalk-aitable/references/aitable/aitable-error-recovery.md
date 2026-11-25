# AI 表格错误恢复指南

> 当 CLI 命令返回错误时，按本文档的映射表判断恢复动作。

## 1. 响应信封与结果判定

```json
{
  "status": "error",
  "summary": "Failed to create records",
  "trace_id": "2104a64c17790723347215232e085e"
}
```

DWS 运行时可能返回统一 `ok/outcome`、旧 `status` 或 JSON boolean `success` 信封，并可能包在 `data` / `result` 中。按以下顺序归一化：

1. 合法且不矛盾的 `ok/outcome`；
2. 已知旧 `status`；
3. JSON boolean `success`。

`outcome` 只接受 `success/pending/partial_failure/failure`；`ok=true` 只能与 `success/pending` 组合。`partial_failure` 可以写在 stdout 并返回退出码 7，`failure` 可以写在 stderr 并返回非零码；必须先解析两个输出通道的结构化信封，再处理退出码。信封矛盾、无法识别或写回结果不完整时按 `unknown` 处理，不假定成功。`summary/trace_id` 可用于诊断，不能代替对象回读。

## 2. 常见错误与恢复动作

### 2.1 记录操作错误

| 错误现象 / summary | 原因 | 恢复动作 |
|-------------------|------|---------|
| `Failed to create records` | cellValue 格式错误或字段类型不匹配 | 先 `field get` 确认字段类型，再按 [cell-value](./aitable-cell-value.md) 规范重构值 |
| `record not found` | record-id 不存在或已删除 | 用 `record query` 重新查询确认目标记录 |
| rating 字段写入超出 max | 值超出字段配置范围 | 检查字段 config 的 min/max，确保值在范围内 |
| singleSelect 写入对象格式但 id 不存在 | option id 无效 | 改用 name 字符串写入（推荐），或先 `field get` 获取有效 option id |

### 2.2 字段操作错误

| 错误现象 / summary | 原因 | 恢复动作 |
|-------------------|------|---------|
| `Failed to create field` | config 格式错误或必填项缺失 | 检查 [field-properties](./aitable-field-properties.md) 中该类型的必填 config |
| `field not found` | field-id 不存在 | 用 `field get` 获取最新字段列表 |
| formula 创建失败 | 公式语法错误或引用字段名不匹配 | 先 `field get` 确认字段精确名称，再检查公式语法（见 [formula-guide](./aitable-formula-guide.md)） |
| 删除主字段失败 | 主字段（第一列）不可删除 | 改为更新字段名或类型，不能删除 |

### 2.3 Base/Table 操作错误

| 错误现象 / summary | 原因 | 恢复动作 |
|-------------------|------|---------|
| `base not found` | base-id 错误或无权限 | 确认 base-id 正确；尝试 `base list` 或 `base search` 重新定位 |
| `table not found` | table-id 错误 | 用 `table get --base-id <baseId>` 不带 table-ids 查看所有表 |
| 表名重复 | 同 Base 下已存在同名表 | 系统会自动续号（如"原名 1"），无需额外处理 |

### 2.4 视图操作错误

| 错误现象 / summary | 原因 | 恢复动作 |
|-------------------|------|---------|
| `view not found` | view-id 错误 | 用 `view get --base-id <baseId> --table-id <tableId>` 查看所有视图 |
| 删除最后一个视图 | 表至少保留一个视图 | 不可删除唯一视图 |

### 2.5 filters/sort 错误

| 错误现象 / summary | 原因 | 恢复动作 |
|-------------------|------|---------|
| filters 无效被忽略 | 根节点不是 and/or，或 operands 格式错误 | 确保 filters 根节点是 `{"operator":"and"/"or", "operands":[...]}` 结构 |
| sort 无效 | fieldId 不存在 | 先 `field get` 确认字段 ID |
| 筛选结果为空 | 条件过严或字段值不匹配 | 放宽条件验证；注意 singleSelect 筛选值用 option name 或 id |

### 2.6 导入导出错误

| 错误现象 / summary | 原因 | 恢复动作 |
|-------------------|------|---------|
| 导出任务超时 | 数据量大，异步任务未完成 | 用 `export data --task-id <taskId>` 轮询直到完成 |
| 导入文件格式错误 | 不支持的文件格式或文件损坏 | 确认文件为 .xlsx 格式且未加密 |

## 3. 重试策略

### 3.1 先查真实状态

任何写调用报错、超时、断连或返回 `pending/unknown/partial_failure` 时，立即停止依赖步骤，用真实 ID、recordId、唯一键或精确范围做最小只读检查：

- 真实状态已达到全部预期：标记“回执异常但结果已确认”，不重试。
- 状态未达预期但已知部分副作用：从 shortcut checkpoint/未完成项续跑，不重放成功项。此分支不适用于记录写入的 `unknown`、`PARTIALLY_COMPLETED`、附件 `unconfirmed` 或 `retryable=false`；这些情况停止写入，checkpoint/nextOffset 不证明任何记录未执行，只能保留原目标做只读核对。
- 无法确认副作用：保持 `unknown`，禁止原样重放非幂等或幂等性未知的写操作。

使用原回执或 checkpoint 中的 Base、Table 和记录 ID。创建组按实际返回的 `acceptedRecordIds` 回读，更新组按原 `requestedRecordIds` 回读。已知创建记录 ID 时，即使缺少原 token，仍可按 ID 核对当前记录。

附件 unconfirmed 回执中，未确认项不计为失败；`failedCount` 为零时可省略。已知该回执属于零值省略时，省略 failedCount 与显式 failedCount=0 表达同一明确失败计数。计数结论为“明确失败计数为 0”；整体结果为“仍有未确认项，保持原 unknown/unconfirmed 并只读核对”。合并报告为“明确失败计数为 0，仍有未确认项”；计数为零成立，全部成功或全部失败均未据此得到证明。

分别报告原写入回执、当前值核验和历史归因。当前值已核验须同时具备字段类型/附件模式契约成立、独立回读符合原请求期望、非空附件所需稳定身份对应三项适用证据；这同样适用于后来建立 fileToken/resourceId 对应的非空附件，不只适用于 `[]` 清空。URL可访问或可下载只补充当前可用性，仍需核实该附件模式所需的稳定身份对应。清空不要求非空附件身份；缺对应证据的非空附件仍未确认。当前值已核验不证明首次写入由原请求造成，也不把原 unknown/unconfirmed 回执改成 verified，无须重新写入刷新回执。当前为空或非空只描述当前状态；缺少独立的原操作证据时，首次写入是否生效及历史归因保持未确认，不为任一历史经过分配概率。清空的目标值可重复设为 `[]`，值级幂等与并发安全分别判断；这不改变该入口已发布的幂等性契约。未知写入的停写依据是结果未核实及可能覆盖并发新增附件。原范围仍未核实时，下一步是按原 ID 只读核对；读回失败不转为申请新授权后重发，新授权本身不解除本恢复流程的停写义务。

`+record-query` 提供业务记录值，当前值核验依据实际字段值及相应空值/稳定身份对应证据，不以 `attachmentVerification` 为必要条件。该入口未承诺返回此诊断字段，不能要求必须返回 `attachmentVerification=verified`；缺少该诊断字段本身不构成核验失败，返回它也不能代替上述证据。Shortcut 回执的该字段描述本次自动核验状态；原回执的 verified/unconfirmed 与后续当前值是否已核验分别报告。

### 混合记录写入回执

`error.details.groupOutcomes` 是记录写入的分组错误信息，原子 `record upsert` 与 `+record-upsert` 均可保留上游提供的该字段；它不是 Shortcut 专属标记，也不与正常成功回执的 `data.createdRecordIds/updatedRecordIds` 冲突。只有含创建的 `+record-upsert` 保证写前自动保存恢复文件；这不表示原子入口不支持分组错误投影，也不证明线上已实际返回某种合成回执组合。

只按本轮给定字段判断，不从指南示例补出 `writeIntentFiles`、`reconcileTool` 或 `clientToken`。创建组的已知 `acceptedRecordIds` 与更新组的原 `requestedRecordIds` 分别按原 ID 只读回读，核对各组期望 cells；更新组 `acceptedRecordIds=[]` 不证明未执行，创建组附件未确认也不抹掉已接收 ID。只有原创建 token 已知时才另列 token 对账；原恢复文件已知则首次查询必须带上，未提供的 token/文件说明缺口，不虚构、不把缺失文件当成已有 ID 无法回读的理由。创建 token 不能证明更新完成，禁止重放整批 upsert 或按差额补写。

含创建的 `+record-upsert` 每次用独立 UUID 文件名保存恢复文件，不覆盖此前文件；文件仅用于绑定原请求，不含 cells，也不证明写入成功。文件名与 clientToken 独立；原身份、端点、Base/Table 固定时，创建去重按 clientToken 核对，恢复文件名不参与创建去重。保持原 token、只换文件名时，创建分组继续沿用原 clientToken 的同一去重绑定和覆盖范围，文件名变化仅涉及本地恢复文件定位；去重是否实际命中须按原 token 只读核对，不能据此断言创建去重失效或成功，重复更新仍可能覆盖并发修改；换新 token（无论文件名保持还是更换）可能失去原创建请求去重绑定，增加重复创建风险，原写入可能尚未生效，因此不等于必然重复。不要将换 token 与换文件合并成同一结论，也不猜测重跑覆盖旧文件。unknown 或创建 ID 不完整时仍停止写入，本恢复流程不提供新 token/新文件补建路径；另一次写入授权不证明原范围已核实。逐 ID 回读后与原请求的期望 cells 核对；原输入缺失时说明无法核对值，不以恢复文件代替原请求。附件成功边界仍需字段类型与稳定身份对应，清空按空值契约核对；只读结果形成当前值核验，保留原回执，不推断首次操作的历史归因。

附件未确认回执提供 `checkpoint.attachmentReadbackCommand` 时，按其中已知记录 ID（含已创建 ID）独立核对附件字段；它与原 token 的创建对账分别进行，不能只查 `+record-write-result` 就宣布附件确认。已有更新组继续使用原 `updateReadbackCommand`。无可用 token 但有已知附件记录 ID 时，`nextCommand` 可直接指向该记录查询；缺失 ID 不推测补出。

`--write-intent` 只有在显式传入时才做文件绑定校验；确无原文件可省略，省略不意味着必定 validation，也不保证远端查询成功。已有原文件仍须首次查询携带；纯更新不能用创建 token 证明生效，与缺少文件无关。Schema 属于本机契约发现，`+record-query` 是业务只读调用；两者不能混称。 创建 clientToken 只作用于创建分组，不保护更新；纯更新有无该 token 都不能据此安全重放。更新停写依据是结果未知及可能覆盖并发修改，不能归因于缺少 token，也不暗示补 token 就安全。原范围未核实时继续停写并只读核对；本次恢复只列记录/字段的只读回读、有创建组时的原 token 只读对账，以及比对原请求期望值。补附件、重新上传和 +attachment-put 等附件写入口归入另立新写入，其前提是原未知范围已完成必要的独立只读核对、当前值与差异已明确，并有独立的新写入意图；“如需补附件”或新增授权本身不满足该核对前提。新授权不证明原范围已核实，不解除本恢复流程停写义务。另立新更新遵守该入口已发布契约，不凭空要求 token 或幂等参数，也不作为本次恢复的重发步骤。 分别记录原写入入口与本轮 Schema 查询入口。原写入入口只取明确叙述原写入调用的文字，或明确标识该原调用的回执/checkpoint路径；用户准许或本轮执行的 Schema 命令及其 Help/Schema 内容只属于契约发现，其 cli-path 不填入原写入入口。仅称“混合 upsert”而未明示原写入路径时，原入口记为“原入口未提供”。原写入路径已明确给出时再按下列规则记录：`+record-update` 或 `+record-upsert` 为记录 Shortcut，`record update` 或 `record upsert` 为原子命令。已给路径直接作为评审前提，线上调用真实性另记未验证；原入口未给才记“原入口未提供”，先说明共同恢复事实，特有保证分别按“若为原子入口/若为 Shortcut”说明。Schema 查询入口单独记录，原入口字段保持本轮给定值。

### 3.2 可重试的错误

- 只读调用遇到网络中断、超时、5xx 或限流，可做有上限退避重试。
- 写操作只有在真实状态证明副作用尚未发生，且当前 Runtime Schema 明确具有幂等契约、稳定唯一键或可续跑 checkpoint 时，才能重试。
- 导出/导入任务未完成时先查询任务或目标的真实状态；仅当当前 Runtime Schema/Help 明确支持原 ID 续等时，才复用同一 taskId/importId。无法确认时保持 `pending/unknown`，不重新 prepare、PUT 或发起新任务。
- 同一 Base 写入默认串行；不用并发重放解决冲突。

### 3.3 不可重试的错误（立即停止）

| 错误类型 | 原因 | 处理方式 |
|---------|------|---------|
| 权限不足 / 403 | 用户对该 Base 无权限 | 停止操作，提示用户确认权限 |
| 参数格式错误 | 请求结构不合法 | 修正参数后重试，不要原样重试 |
| 资源不存在 / 404 | ID 错误或资源已删除 | 重新查询定位资源 |
| 非幂等写入效果未知 | 服务端可能已经执行 | 先回读；无法确认时保持 unknown |
| 稳定重现的类型污染/UI 语义错误 | 同一请求不会因重放恢复 | 停止受影响分支并保留证据 |

### 3.4 重试前检查清单

在重试前，先确认：
1. ❓ 错误是暂时性的还是永久性的？
2. ❓ 参数有没有明显错误需要修正？
3. ❓ 是否需要先查询最新状态再重试？

## 4. 调试技巧

### 4.1 使用 --verbose 获取详细信息

```bash
dws aitable record create \
  --base-id <baseId> \
  --table-id <tableId> \
  --records '[...]' \
  --verbose --format json
```

`--verbose` 会输出请求/响应的详细信息，帮助定位问题。

### 4.2 使用 --dry-run 预览

```bash
dws aitable record create \
  --base-id <baseId> \
  --table-id <tableId> \
  --records '[...]' \
  --dry-run --format json
```

`--dry-run` 只预览不执行，适合在不确定参数是否正确时先验证。

## 5. 错误预防最佳实践

1. **写记录前先读字段结构** — `field get` 确认字段类型和 ID
2. **写字段前先读 field-properties** — 确认 config 的必填项和格式
3. **formula 字段先确认引用字段名** — `[字段名]` 必须精确匹配
4. **options 更新传完整列表** — 更新 singleSelect/multipleSelect 的 options 是全量覆盖
5. **大批量操作分批执行** — 单次最多 100 条记录
6. **使用 --format json** — 确保输出可解析，方便错误判断
