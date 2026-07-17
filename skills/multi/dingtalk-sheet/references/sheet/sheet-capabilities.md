# 当前能力边界

## 当前能力边界

- **公式**：支持通过 `range update` 写入、通过 `range read --value-render-option formula` 回读公式文本、通过 `raw_value` / `formatted_value` 回读计算结果；当前没有聚合式 `formula-verify`，不能只凭写入成功声称公式零错误。
- **视觉规范**：当前不维护独立的表格视觉方案文档；样式、条件格式、图表分别按对应子文档执行。
- **未暴露或未确认能力**：迷你图、历史 changeset、评论/批注等能力未在本入口承诺；只有出现稳定 DWS 命令和可回读语义后再补充。
