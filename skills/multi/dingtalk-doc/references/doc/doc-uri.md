# 文档地址 (URI)

## 文档地址 (URI)

| 资源 | URI 格式 |
|------|----------|
| 文档节点 | `https://alidocs.dingtalk.com/i/nodes/{dentryUuid}` |
| edit / preview 链接 | `https://alidocs.dingtalk.com/document/{edit\|preview}?...&dentryKey={key}` |

> **操作后请返回文档 URI**：每次执行 create / read / update 等操作后，从返回数据中提取 `docUrl` 直接返回；缺失时用 `doc info --node <ID>` 补查。
