# 文档地址 (URI)

## 文档地址 (URI)

| 资源 | URI 格式 |
|------|----------|
| Base 文档 | `https://alidocs.dingtalk.com/i/nodes/{baseId}` |
| 指定数据表 | `https://alidocs.dingtalk.com/i/nodes/{baseId}?iframeQuery=sheetId%3D{tableId}` |
| 指定数据表+视图 | `https://alidocs.dingtalk.com/i/nodes/{baseId}?iframeQuery=sheetId%3D{tableId}%26viewId%3D{viewId}` |
| 模板预览 | `https://docs.dingtalk.com/table/template/{templateId}` |

> **操作后请返回文档 URI**：返回链接时必须带上当前操作的数据表 tableId，让用户点击后直接看到目标数据表，而不是落在空白的默认表。
> - 已知 tableId + viewId 时（view create 返回、view get 中提取）：拼接 `https://alidocs.dingtalk.com/i/nodes/{baseId}?iframeQuery=sheetId%3D{tableId}%26viewId%3D{viewId}`
> - 已知 tableId 时（table create 返回、base get 中提取、record 操作所用的 tableId）：拼接 `https://alidocs.dingtalk.com/i/nodes/{baseId}?iframeQuery=sheetId%3D{tableId}`
> - 仅有 baseId、无明确 tableId 时（如 base list/search）：拼接 `https://alidocs.dingtalk.com/i/nodes/{baseId}`
>
> 补充：如果 URL 不是来自 `aitable` 命令返回，而是用户直接贴的原始 `alidocs` URL，先按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) probe，确认是 `able` 后再按 AI 表格处理。
