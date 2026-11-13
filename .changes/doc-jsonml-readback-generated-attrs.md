---
category: Fixed
---

- **JSONML 写后回读误报** — `doc +update --doc-format jsonml` 的 `overwrite` 与 `block_replace` 内容已正确写入，却总是以 `doc_write_verification_failed` 报错。原因是服务端回读时会补上作者没写的内容：根节点的页面设置 `sectPr`，以及代码块里一个空的文本 span。写后回读现在忽略这两处服务端补全；代码文本、`fold` 等作者写的属性照旧严格比对。
