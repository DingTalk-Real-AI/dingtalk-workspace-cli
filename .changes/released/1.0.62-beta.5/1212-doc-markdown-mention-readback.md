---
category: Fixed
---

- **markdown @人 写后回读误报** — 写入含 `[@姓名](alidocs-mcp://doc/mention?openDingTalkId=…)` 的 markdown 时，`doc +create` 与 `doc +update --command append|overwrite` 会以 `doc_write_verification_failed` 报错，而内容其实已正确写入。原因是写后回读把写入原文与服务端改写后的正文比对，而服务端会把该私有协议改写成钉钉个人资料链接。现在写后回读改为**按位置配对**：只有预期正文中写了 mention 私有协议的那个位置，才允许回读侧是个人资料链接；其余链接——包括作者自己写的普通个人资料链接——仍保留完整目标并严格比对。显示文本与节点顺序照旧参与比对，漏写、改标签或顺序错乱依旧判定失败。原子命令 `doc update` 无写后回读，行为不变。
- **@人 目标身份不再被隐含声明为已验证** — 回读能证明 mention 链接落在作者写的位置、显示文本未变，但证明不了它解析到了哪个人：`openDingTalkId` 与改写后的 `staffId` 是不同值且无本地映射。含 mention 的写入结果因此在与 `verified` 同级处声明作用域：`verificationScope="partial"`、`unverified=["mention_targets"]`，verify 步骤状态由 `success` 降为 `partial` 并带 `scope="partial"`（只按 `steps[].status` 推进、不认识 scope 字段的既有消费者因此也不会再把它读成完整核验成功），另有 `verification.mentionTargetsVerified=false` 与一条说明性 warning，并把 `verified` 置为 `false`（操作本身仍 `status=success`）：回读无法确定 @ 到了谁，就不宣称已验证。另有 `unverifiableLocally=["mention_targets"]` 表明该缺口不是"还没查"而是"回读查不出来"，重读文档不会得到新信息。warning 只透两条事实：@人链接指向的具体人员需用户自行核对，正文其余部分（含该链接的位置与显示文本）均已通过回读校验。不含 mention 的写入输出完全不变。
