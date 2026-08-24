# Dev CLI 参数幻觉补充复核（2026-08-21）

独立候选基于线上 `main` `11934eed057267d97e7442ddd420c711ee1802dc`：
71 concepts / 370 overrides / 749 fixtures，生成与嵌入式 PreParse 通过。

36 个命令产生 39 alias、161 block、26 ambiguous。Dev/DevApp 共用 app name、version ID、icon media ID、
member type 等真正同域概念；`member-role` 只保留为成员命令局部别名。机器人 clientId、taskId、appKey
保持不同标识符命名空间，不把无角色 `id/key` 自动提升。
