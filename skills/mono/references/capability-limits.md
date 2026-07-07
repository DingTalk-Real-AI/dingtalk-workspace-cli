# 已知能力限制

遇到以下操作时，**不要重试或变通**，直接告知用户当前不支持并建议在钉钉客户端操作。

## chat

| 不支持的操作 | 说明 |
|------------|------|
| 撤回个人身份发送的消息 | 只有 `send-by-bot` 发送的消息才能通过 `recall-by-bot` 撤回。个人身份 (`chat message send`) 发送的消息无法通过 API 撤回 |

## doc

| 不支持的操作 | 说明 |
|------------|------|
| 文档权限管理 | ⚠️ 已通过 transitional helper 部分支持：`dws doc permission add/update/list` 可用（调 `add_permission`/`update_permission`/`list_permission` MCP tool）；待 mse 注册对应 toolOverride 后此条删除 |
| 删除整篇文档/文件 | ⚠️ 已通过 transitional helper 支持：`dws doc delete --node <ID> --yes`（调 `delete_document`）；待 mse 注册后此条删除 |
| 文档导出为 docx | ⚠️ 已通过 transitional helper 支持：`dws doc export --node <ID> --output ./x.docx`（内置渐进式退避轮询 + 自动下载，调 `submit_export_job` / `query_export_job` MCP tool）；待 mse 注册对应 toolOverride 后此条删除 |
| 媒体附件下载/插入 | ⚠️ 已通过 transitional helper 支持：`dws doc media download/insert`（前者调 `download_doc_attachment`，后者 3 步流水线）；待 mse 注册后 download 条可删，insert 因含本地 HTTP PUT 永久 helper |

## aitable

| 不支持的操作 | 说明 |
|------------|------|
| 创建公式/查找引用等高级字段类型 | 部分高级字段类型暂不支持 API 创建 |
| 自己 PUT 文件到 OSS 时 Content-Type 处理 | OSS 签名对 PUT 请求的 `Content-Type` 头有严格要求，处理不当会 `SignatureDoesNotMatch` / HTTP 403。这**不是 dws 限制**，是阿里云 OSS 行为。解决：直接用 `python scripts/aitable_import_via_task.py <baseId> <file>`，脚本已内置 prepare→PUT→import data 全流程和正确的头处理，**不要自己写 curl PUT**。（注意：`aitable import upload` 命令没有 `--file` flag、也不代做 PUT，只准备导入；能自动 PUT 的是上面的脚本。） |

## minutes

| 不支持的操作 | 说明 |
|------------|------|
| 跨听记的全局热词修正 | `replace-text` 仅修正**当前这一篇听记**的文字，不会影响后续新听记的语音识别结果。要让某个词长期不再被识别错，必须额外引导用户使用 `hot-word add` 添加个人热词 |

---

## 维护守则

> - 此文件随产品能力迭代更新。新增限制时按产品分类追加即可。
> - **当产品命令补齐后，必须同步删除对应的"不支持"条目**——否则 Agent 看到该条目会主动拒绝调用已存在的命令，造成可用能力被自我屏蔽。
> - 标注 ⚠️ 的条目表示"transitional helper 已临时支持"，待 mse 端 toolOverride 落地后整条删除。
