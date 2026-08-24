# Sheet CLI 参数幻觉补充复核（2026-08-21）

独立候选基于线上 `main` `11934eed057267d97e7442ddd420c711ee1802dc`：
70 concepts / 368 overrides / 870 fixtures，生成与嵌入式 PreParse 通过。

89 个命令产生 106 alias、1876 block、19 ambiguous。重点修复了旧草稿把数值型 DingDrive `space-id`
与文档知识空间 `workspace-id` 混成一个 concept 的问题；当前 Sheet 创建/导入/模板命令只绑定
`workspace_id`。工作表显示名不再进入 worksheet-ID concept，A1 range、源/目标 tab 与维度轴/长度保持分域。
