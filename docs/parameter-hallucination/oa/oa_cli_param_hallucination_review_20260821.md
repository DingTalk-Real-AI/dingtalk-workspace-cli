# OA CLI 参数幻觉补充复核（2026-08-21）

独立候选基于线上 `main` `11934eed057267d97e7442ddd420c711ee1802dc`：
66 concepts / 367 overrides / 746 fixtures，生成与嵌入式 PreParse 通过。

28 个命令产生 45 alias、124 block、23 ambiguous。只保留审批实例、流程 code、任务 ID、抄送人员列表、
审批意见等可复用且可证明的中央角色。`form-values` 与 `request` 两种 JSON 模式只属于同一局部端点，
现已下沉为精确命令 guard。任务/实例/流程标识符以及 raw JSON payload 不互换；未来审批事件监听
属于 Event，不通过 OA 参数兜底代替。
