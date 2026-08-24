# Mail CLI 参数幻觉补充复核（2026-08-21）

独立候选基于线上 `main` `11934eed057267d97e7442ddd420c711ee1802dc`：
66 concepts / 395 overrides / 784 fixtures，生成与嵌入式 PreParse 通过。

57 个命令产生 53 alias、288 block、1 ambiguous。关键修正是：邮件搜索的 `subject` 不能自动改写成
`query`，因为 KQL/搜索表达式不能由标题值无损合成；收件人、规则 ID、标签 ID、模板 ID 分角色处理。
无实际生成效果的 cc/recipient 推测词已删除。
