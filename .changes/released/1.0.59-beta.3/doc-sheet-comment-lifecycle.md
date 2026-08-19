---
category: Added
---

- **Doc and Sheet comment lifecycle commands** — adds `comment batch-query`,
  `comment resolve`, `comment restore`, and the lightweight
  `comment react-reply` to both `dws doc` and `dws sheet`. The two domains share
  the same `doc-comment` MCP capabilities; batch queries preserve input order
  for repeated `topicId:commentKey` references, while reaction replies require
  DingTalk reaction names such as `憨笑` or `鼓掌` rather than raw Unicode emoji.
