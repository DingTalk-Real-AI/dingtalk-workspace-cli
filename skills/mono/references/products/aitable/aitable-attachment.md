# attachment — 附件上传

> **STOP — 不要使用钉盘 (drive) 上传！** 钉盘 fileId 无法写入 attachment 字段。必须使用以下流程。

> 本地文件用 `+attachment-put`（上传并写入已有记录）或以下上传流程；已有可直接下载的在线 URL，可在原子 `record create/update` 的附件字段直接传 `[{"url":"https://..."}]`。URL 转存为 best-effort 异步操作，成功回执只表示已受理，需独立回读并确认附件可用。

钉钉文档节点链接可直接写入 `[{"url":"https://alidocs.dingtalk.com/i/nodes/节点ID"}]`。省略 `isLink` 为默认链接模式；服务端校验源权限后保存真实名称、类型及节点链接，不导出、不转存、不扩大权限。

用户明确要求文件模式时，传 `[{"url":"https://alidocs.dingtalk.com/i/nodes/节点ID","isLink":false}]`，保留原始合法 URL 和 JSON 布尔 `false`，不要改成 `true`、字符串 `"false"` 或省略该属性，也不要编造 `filename/type` 或资源 ID。服务端校验源权限与下载/转存结果；权限或下载失败时保留错误，不自动回退链接模式，不扩大权限。准备输入不代表已验证源权限或已完成转存。附件数组整体替换，写后仍须独立回读。


## 准备附件上传

```
Usage:
  dws aitable attachment upload [flags]
Example:
  dws aitable attachment upload --base-id <BASE_ID> --file-name report.xlsx --size 204800
  dws aitable attachment upload --base-id <BASE_ID> --file-name photo.png --size 1024 --mime-type image/png
Flags:
      --base-id string     Base ID (必填)
      --file-name string   文件名，必须含扩展名 (必填)
      --size int           文件大小（字节），>0 (必填)
      --mime-type string   MIME type（不传时根据扩展名推断）
```

## 附件上传完整流程（推荐：使用脚本，2 步完成）

```bash
# 步骤 1: 使用脚本一键上传（内部自动完成 prepare + PUT）
python3 scripts/upload_attachment.py <BASE_ID> /path/to/report.pdf
# 输出: { "fileToken": "ft_xxx", "fileName": "report.pdf", "size": 204800 }

# 步骤 2: 在 record create/update 中使用 fileToken 写入
dws aitable record create --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"cells":{"fldAttachId":[{"fileToken":"ft_xxx"}]}}]' --format json
```

> `uploadUrl` 有时效性（`expiresAt`），脚本会自动在获取后立即上传，并原样携带返回的 `uploadHeaders`。

## 手动流程（不使用脚本）

```bash
# 1. 获取上传凭证
dws aitable attachment upload --base-id <BASE_ID> --file-name report.pdf --size 204800 --format json
# → 返回 uploadUrl、uploadHeaders、fileToken

# 2. PUT 上传：为 uploadHeaders 的每一项分别传入 -H
# 以下假设还返回 X-Upload-Signed；未返回时删除对应行，返回其他头时继续逐项添加 -H。
curl -X PUT "<uploadUrl>" \
  -H "Content-Type: <uploadHeaders 中 Content-Type 的原值>" \
  -H "X-Upload-Signed: <uploadHeaders 中 X-Upload-Signed 的原值>" \
  --data-binary @report.pdf

# 3. 写入记录
dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"recordId":"recXXX","cells":{"fldAttachId":[{"fileToken":"ft_xxx"}]}}]' --format json
```

Content-Type 必须采用本次返回的签名值；省略 mime-type 时也不自行猜测或追加 charset。若返回其他 uploadHeaders，同样原样携带。

## 在线 URL 写入

```bash
dws aitable record update --base-id <BASE_ID> --table-id <TABLE_ID> \
  --records '[{"recordId":"<RECORD_ID>","cells":{"<FIELD_ID>":[{"url":"https://example.com/report.pdf"}]}}]' --format json
```

附件写入会整体覆盖原列表；清空使用 `[]`。上游链路可能丢弃显式 `null`，不要依赖它清空；省略附件字段仍保持原值。同一批 `update_records` 不能混合附件清空与新增/替换，应分开请求。不要将临时下载链接当作持久资源身份，也不要仅凭转存受理回执宣称文件已可下载。

`+record-update`、`+record-upsert` 和 `+record-upsert-by-key` 直接写入 fileToken 后，只有独立读回的稳定 fileToken/resourceId 能对应输入时才确认 verified；filename、size 或新出现的 resourceId 本身不能证明与上传 token 相同。无法对应或读回失败时，CLI 返回 unknown、`ATTACHMENT_WRITE_UNCONFIRMED`、`retryable=false` 和原记录 ID/唯一键的只读核对命令；未确认项不计为失败，`failedCount` 为零时可省略。已知该回执属于零值省略时，省略 failedCount 与显式 failedCount=0 表达同一明确失败计数。计数结论为“明确失败计数为 0”；整体结果为“仍有未确认项，保持原 unknown/unconfirmed 并只读核对”。合并报告为“明确失败计数为 0，仍有未确认项”；计数为零成立，全部成功或全部失败均未据此得到证明，不重放写入。`+record-upsert` 的创建分组保留原 clientToken。读回不一致错误不回显附件值或 fileToken。

附件 `[]` 清空已提交但独立读回失败时也禁止重放，以免删除首次提交后他人新增的附件；保留原记录 ID 只读核对。CLI 按字段目录区分附件与普通多选字段；目录不可读或字段无法唯一解析时，不能据空数组排除附件清空，应保守核对。已评审恢复错误只输出安全固定消息及允许的错误码/恢复目标，不透传上游原始诊断。

当前字段值不证明首次写入的历史结果：为空不能单独归因于本次清空，基线外新增附件也不能证明首次清空已生效。缺少原操作的独立证据时，保持首次写入未确认，保留当前附件，不从现状宣称首次操作成功或重放清空。

分别报告原写入回执、当前值核验和历史归因。当前值已核验须同时具备字段类型/附件模式契约成立、独立回读符合原请求期望、非空附件所需稳定身份对应三项适用证据；这同样适用于后来建立 fileToken/resourceId 对应的非空附件，不只适用于 `[]` 清空。URL可访问或可下载只补充当前可用性，仍需核实该附件模式所需的稳定身份对应。清空不要求非空附件身份；缺对应证据的非空附件仍未确认。当前值已核验不证明首次写入由原请求造成，也不把原 unknown/unconfirmed 回执改成 verified，无须重新写入刷新回执。当前为空或非空只描述当前状态；缺少独立的原操作证据时，首次写入是否生效及历史归因保持未确认，不为任一历史经过分配概率。清空的目标值可重复设为 `[]`，值级幂等与并发安全分别判断；这不改变该入口已发布的幂等性契约。未知写入的停写依据是结果未核实及可能覆盖并发新增附件。原范围仍未核实时，下一步是按原 ID 只读核对；本次恢复只列记录/字段的只读回读、有创建组时的原 token 只读对账，以及比对原请求期望值。补附件、重新上传和 +attachment-put 等附件写入口归入另立新写入，其前提是原未知范围已完成必要的独立只读核对、当前值与差异已明确，并有独立的新写入意图；“如需补附件”或新增授权本身不满足该核对前提。读回失败不转为申请新授权后重发，新授权本身不解除本恢复流程的停写义务。

`+record-query` 提供业务记录值，当前值核验依据实际字段值及相应空值/稳定身份对应证据，不以 `attachmentVerification` 为必要条件。该入口未承诺返回此诊断字段，不能要求必须返回 `attachmentVerification=verified`；缺少该诊断字段本身不构成核验失败，返回它也不能代替上述证据。Shortcut 回执的该字段描述本次自动核验状态；原回执的 verified/unconfirmed 与后续当前值是否已核验分别报告。

### 记录 Shortcut 的未确认边界

以下说明仅针对带独立读回校验的 `+record-update`、`+record-upsert`、`+record-upsert-by-key`、`+record-batch-create` 和 `+record-bulk-patch`，不把本地 Shortcut 校验外推为原子写接口的返回保证。

写入受理后，含附件形状的批次在任一字段读回失败时都可能返回 `ATTACHMENT_WRITE_UNCONFIRMED`；该错误码不代表一定是附件字段的问题。用 `verification.readback` 区分 pending/mismatch/unavailable；有 `mismatchedFieldId` 时按实际差异字段核对，非附件字段也可能不一致。保持 unknown、retryable=false，不把读回不一致当作原写入未发生。

非空附件若只传 `url`（普通 URL、文档节点默认链接或显式 `isLink=false`），输入没有可与读回对应的 fileToken/resourceId，即使 URL 相同或文件可下载，现有稳定身份校验仍不会确认 verified。已受理且进入读回校验的这类写入预期为 unknown、`ATTACHMENT_WRITE_UNCONFIRMED`、非零退出和 retryable=false；不是可以重试的确定失败，不得改成链接模式、重新上传或重写来刷新 verified。

附件未确认回执提供 `checkpoint.attachmentReadbackCommand` 时，按其中已知记录 ID（含已创建 ID）独立核对附件字段；它与原 token 的创建对账分别进行，不能只查 `+record-write-result` 就宣布附件确认。已有更新组继续使用原 `updateReadbackCommand`。无可用 token 但有已知附件记录 ID 时，`nextCommand` 可直接指向该记录查询；缺失 ID 不推测补出。

沿回执 `nextCommand`，使用原 Base/Table 和已知记录 ID/唯一键只读核对各字段；创建 token 仅核对创建组，更新另按原 ID 读取。URL 可访问只补充当前可用性证据，缺稳定身份对应仍保留 unconfirmed；原回执、当前值核验与历史归因分别记录。清空 `[]` 仍按空值契约核对，不受非空附件身份要求限制。

用户或回执给出的 fileToken/resourceId 原样保留；不能凭英文、短名或合成代称的字面形状判为无效或占位符。缺少输入 token 与读回 resourceId 的对应证据，只能说明对应未确认，不证明 resourceId 无效。

错误诊断不回显附件值或 fileToken，只约束错误详情与 verification 诊断，不规定业务 `+record-query` 的返回必定包含或不包含 token；业务读回以实际回执和已发布契约为准，不能由诊断脱敏推断业务字段被删除。

原子 `record update/upsert` 不执行上述 Shortcut 的本地稳定身份校验；不能仅因输入为 URL-only，就预测原子入口固定返回 unknown、ATTACHMENT_WRITE_UNCONFIRMED、非零退出或 retryable=false。按实际上游回执解释状态，写后仍沿原 ID 独立只读核对，不凭受理回执宣布附件已确认。分别记录原写入入口与本轮 Schema 查询入口。原写入入口只取明确叙述原写入调用的文字，或明确标识该原调用的回执/checkpoint路径；用户准许或本轮执行的 Schema 命令及其 Help/Schema 内容只属于契约发现，其 cli-path 不填入原写入入口。仅称“混合 upsert”而未明示原写入路径时，原入口记为“原入口未提供”。原写入路径已明确给出时再按下列规则记录：`+record-update` 或 `+record-upsert` 为记录 Shortcut，`record update` 或 `record upsert` 为原子命令。已给路径直接作为评审前提，线上调用真实性另记未验证；原入口未给才记“原入口未提供”，先说明共同恢复事实，特有保证分别按“若为原子入口/若为 Shortcut”说明。Schema 查询入口单独记录，原入口字段保持本轮给定值。

## 移除附件

- 清空字段：`dws aitable +attachment-remove --base-id <B> --table-id <T> --record-id <R> --field-id <F> --clear-all`。
- 按文件名移除：同一命令改用 `--remove-name <文件名>`，从回读结果解析真实 resourceId 后删除并验证；目标缺 resourceId 时仅在剩余项都有 fileToken 的情况下使用替换路径，否则停止。
- 已知 resourceId：`dws aitable attachment remove --base-id <B> --table-id <T> --record-id <R> --field-id <F> --resource-ids <RESOURCE_IDS>`，随后独立回读。

按 Runtime 要求确认删除范围后执行。服务端删除仍有并发覆盖窗口，避免同时修改同一附件字段。
