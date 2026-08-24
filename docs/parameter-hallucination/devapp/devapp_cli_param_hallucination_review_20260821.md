# DevApp CLI 参数幻觉补充复核（2026-08-21）

独立候选基于线上 `main` `11934eed057267d97e7442ddd420c711ee1802dc`：
66 concepts / 356 overrides / 716 fixtures，生成与嵌入式 PreParse 通过。

19 个命令产生 22 alias、101 block、10 ambiguous。与 Dev 的同域实体已统一，删除重复的
`devapp_version_id/devapp_app_name/devapp_icon_media_id/devapp_member_type` 中央概念；命令局部角色仍用
scoped alias，避免跨产品扩散。
