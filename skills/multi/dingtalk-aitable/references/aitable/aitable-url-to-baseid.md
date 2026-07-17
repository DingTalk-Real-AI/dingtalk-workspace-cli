# URL → baseId 提取

## URL → baseId 提取

用户提供 `https://alidocs.dingtalk.com/i/nodes/{baseId}` 链接时：
1. 提取 `/nodes/` 后的路径段作为 `baseId`
2. 去掉尾部的查询参数（`?` 及其后内容）
3. 传入 `--base-id` 参数

> 如果该 URL 来自 `dws aitable` 返回或已在当前链路 probe 过，可直接复用；
> 如果是用户直接提供的原始 `alidocs` URL，则先按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) probe，确认 `extension=able` 后再继续。
