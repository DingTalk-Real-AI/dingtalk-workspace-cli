---
category: Fixed
---

- **markdown @人 写后回读误报** — 写入含 `[@姓名](alidocs-mcp://doc/mention?openDingTalkId=…)` 的 markdown 时，`doc +create` 与 `doc +update --command append|overwrite` 会以 `doc_write_verification_failed` 报错，而内容其实已正确写入。原因是写后回读把写入原文与服务端改写后的正文比对，而服务端会把该私有协议改写成钉钉个人资料链接。现在写后回读的语义指纹对 mention 私有协议与改写后的 profile 链接发同一个目标无关令牌，仅在预期内容确实含该协议时启用；显示文本与节点顺序仍参与比对，因此漏写、改标签或顺序错乱依旧判定失败。原子命令 `doc update` 无写后回读，行为不变。
