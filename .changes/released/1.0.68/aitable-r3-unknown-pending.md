---
category: Fixed
---

- **AI 表格写入结果 unknown 保持 pending**：`+record-write-result` 收到合法的 `state=unknown` 回执时返回 `pending`，不再误报失败；保留原 baseId/tableId/clientToken 和只读恢复命令，不重放写入，也不把未知结果当作未写入。坏身份、坏 ID 或部分确认的 ID 仍然拒绝。
