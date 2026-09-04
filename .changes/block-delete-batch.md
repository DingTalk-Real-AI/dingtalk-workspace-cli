---
category: Added
---

- **文档块批量删除** — `doc block delete` 的 `--block-id` 支持逗号分隔一次删除多个块
  （单次最多 50 个）。采用尽力而为语义：单个 blockId 未找到不阻塞其余块的删除，
  未找到的在 `notFoundBlockIds` 中列出；仅当全部未找到时整体失败。
