# record update — 更新记录

`--records` 的完整值是记录数组 `[{"recordId":"<原记录ID>","cells":{}}]`，实际 cells 按原请求填写。原子入口的 --records-file 文件顶层同样是数组，不给未发布此 flag 的 Shortcut 添加它。`{"records":[...]}` 是 RPC 外层对象，不能整个传给该 flag 或作为文件顶层；草稿必须展示数组本身。

## 命令格式

```
Usage:
  dws aitable record update [flags]
Example:
  dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
    --records '[{"recordId":"recXXX","cells":{"fldStatusId":"已完成"}}]'
Flags:
      --base-id string        Base ID (必填)
      --records string        待更新记录 JSON 数组，单次最多 100 条；cells key 支持 fieldId 或当前表内唯一字段名，推荐 fieldId (必填，与 --records-file 二选一)
      --records-file string   从文件读取 records JSON（替代 --records，适合超长数据或 Windows 环境）
      --table-id string       Table ID (必填)
```

只需传入需修改的字段，未传入的保持原值。每条记录必须含 recordId 和 cells。


## cells key：优先使用 fieldId，也支持唯一字段名

`cells` 的 key 有两种写法：

- fieldId（推荐）：不受字段重命名或重名影响，通过 `field get` 获取。
- 当前表内唯一的字段名：按名称精确匹配；如果存在同名字段，必须改用 fieldId。

同一字段同时通过 fieldId 和字段名传入时，fieldId 对应的值优先。“推荐 fieldId”不等于“只允许 fieldId”；不要把本轮已经给出的合法唯一字段名当成缺少字段 ID。

```bash
# 推荐：fieldId
dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"recordId":"recXXX","cells":{"fldStatusId":"已完成"}}]' --format json

# 便捷写法：当前表内唯一字段名
dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"recordId":"recXXX","cells":{"状态":"已完成"}}]' --format json
```

## 推荐参数形式

公开、稳定的批量入口是 `--records`（或 `--records-file`），格式为 JSON 数组；即使只改一条记录，推荐也包在数组里。CLI 仍保留隐藏的 `--record-id` + `--cells` 兼容入口，但它不会出现在常规帮助中，自动化脚本应优先使用 `--records`。

| 不推荐或无效写法 | 推荐写法 |
|---|---|
| `--record-id recXXX --cells '{"fldX":"值"}'`（隐藏兼容入口） | `--records '[{"recordId":"recXXX","cells":{"fldX":"值"}}]'` |
| `--id recXXX --data '{"fldX":"值"}'` | 同上 |
| `--record-id recXXX --field fldX --value "新值"` | 同上 |

## 单条更新模板（直接复制）

```bash
dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"recordId":"<RECORD_ID>","cells":{"<FIELD_ID>":"新值"}}]' --format json

# 从更新响应的 data.recordIds[] 提取成功记录 ID，并回读确认真实值
dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID> \
  --record-ids <RECORD_ID> --format json
```

更新响应不返回“受影响字段”；以 `data.recordIds[]` 确定成功记录，再用查询回读验证。

## 附件更新边界

节点附件默认保存链接；显式文件模式传 `[{"url":"https://alidocs.dingtalk.com/i/nodes/节点ID","isLink":false}]`，保留 JSON 布尔 `false`，不要省略、改为链接模式或编造文件元数据。源权限与下载/转存由服务端校验，失败保留原错误，不自动回退链接；写后独立回读。

附件清空优先使用 `[]`。同一批 `update_records` 不能同时包含附件清空（值为 `[]` 或 `null`）与附件新增/替换。有这两类变更时必须拆成两次 `record update` 调用，每次分别回读；DWS 不会根据 cell 形状猜测字段类型并自动拆批。

## 已提交更新的只读恢复

写入结果为 `unknown`、`PARTIALLY_COMPLETED`，或附件 `[]` 清空提交后独立读回失败时，保留原状态、`retryable=false` 和原 Base/Table/recordId，不重放更新，不按 `nextOffset` 继续写入。缺少 `groupOutcomes`、`reconcileTool` 或零失败计数字段，不证明原记录未被写入。

只读核对使用 `dws aitable +record-query --base-id <BASE_ID> --table-id <TABLE_ID> --record-ids <RECORD_ID> --format json`。使用原回执或 checkpoint 中的 Base、Table 和记录 ID。创建组按实际返回的 `acceptedRecordIds` 回读，更新组按原 `requestedRecordIds` 回读。已知创建记录 ID 时，即使缺少原 token，仍可按 ID 核对当前记录。

读回只证明读取时的字段值，不证明历史操作归因。当前为空，不能单独证明是本次清空造成的；当前出现基线外新增附件，也不能区分首次清空成功后新增与首次未生效后新增。缺少原操作的独立证据时保持首次写入未确认，保留当前附件，不重放清空，不从现状推断首次操作已成功。 创建 clientToken 只作用于创建分组，不保护更新；纯更新有无该 token 都不能据此安全重放。更新停写依据是结果未知及可能覆盖并发修改，不能归因于缺少 token，也不暗示补 token 就安全。原范围未核实时继续停写并只读核对；本次恢复只列记录/字段的只读回读、有创建组时的原 token 只读对账，以及比对原请求期望值。补附件、重新上传和 +attachment-put 等附件写入口归入另立新写入，其前提是原未知范围已完成必要的独立只读核对、当前值与差异已明确，并有独立的新写入意图；“如需补附件”或新增授权本身不满足该核对前提。新授权不证明原范围已核实，不解除本恢复流程停写义务。另立新更新遵守该入口已发布契约，不凭空要求 token 或幂等参数，也不作为本次恢复的重发步骤。 分别记录原写入入口与本轮 Schema 查询入口。原写入入口只取明确叙述原写入调用的文字，或明确标识该原调用的回执/checkpoint路径；用户准许或本轮执行的 Schema 命令及其 Help/Schema 内容只属于契约发现，其 cli-path 不填入原写入入口。仅称“混合 upsert”而未明示原写入路径时，原入口记为“原入口未提供”。原写入路径已明确给出时再按下列规则记录：`+record-update` 或 `+record-upsert` 为记录 Shortcut，`record update` 或 `record upsert` 为原子命令。已给路径直接作为评审前提，线上调用真实性另记未验证；原入口未给才记“原入口未提供”，先说明共同恢复事实，特有保证分别按“若为原子入口/若为 Shortcut”说明。Schema 查询入口单独记录，原入口字段保持本轮给定值。

分别报告原写入回执、当前值核验和历史归因。当前值已核验须同时具备字段类型/附件模式契约成立、独立回读符合原请求期望、非空附件所需稳定身份对应三项适用证据；这同样适用于后来建立 fileToken/resourceId 对应的非空附件，不只适用于 `[]` 清空。URL可访问或可下载只补充当前可用性，仍需核实该附件模式所需的稳定身份对应。清空不要求非空附件身份；缺对应证据的非空附件仍未确认。当前值已核验不证明首次写入由原请求造成，也不把原 unknown/unconfirmed 回执改成 verified，无须重新写入刷新回执。当前为空或非空只描述当前状态；缺少独立的原操作证据时，首次写入是否生效及历史归因保持未确认，不为任一历史经过分配概率。清空的目标值可重复设为 `[]`，值级幂等与并发安全分别判断；这不改变该入口已发布的幂等性契约。未知写入的停写依据是结果未核实及可能覆盖并发新增附件。原范围仍未核实时，下一步是按原 ID 只读核对；读回失败不转为申请新授权后重发，新授权本身不解除本恢复流程的停写义务。

`+record-query` 提供业务记录值，当前值核验依据实际字段值及相应空值/稳定身份对应证据，不以 `attachmentVerification` 为必要条件。该入口未承诺返回此诊断字段，不能要求必须返回 `attachmentVerification=verified`；缺少该诊断字段本身不构成核验失败，返回它也不能代替上述证据。Shortcut 回执的该字段描述本次自动核验状态；原回执的 verified/unconfirmed 与后续当前值是否已核验分别报告。

## 引号转义提示

- Linux/macOS：外层用单引号 `'[...]'`，内部 JSON 用双引号即可
- Windows PowerShell：外层用双引号 `"[...]"`，内部双引号需转义为 `\"`
- 或将 JSON 写入临时文件，用 `--records-file ./records.json` 规避转义
