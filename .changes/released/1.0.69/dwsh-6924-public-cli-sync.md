---
category: Changed
---

- `chat message send-a2ui-card` 支持引用原消息，入群审批命令支持通过、删除、忽略、拒绝和拉黑。
- `whiteboard +insert-image` 支持在独立或内嵌白板插入本地 PNG/JPEG，并读回校验结果。
- `sheet info` 对空值、非对象和无效 JSON 返回结构化 API 错误。
- `event consume --group` 在提交订阅前拒绝逗号分隔的多群输入。
- AI 表格普通记录查询支持更大的服务页，在分页不完整时返回明确的恢复指引。
- `aicard chart` 支持离线查找预设和生成图表卡片，图表校验区分完整新卡与延迟注入数据的模板。
