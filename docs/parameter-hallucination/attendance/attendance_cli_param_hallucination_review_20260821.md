# Attendance CLI 参数幻觉补充复核（2026-08-21）

基线为线上 `main` `11934eed057267d97e7442ddd420c711ee1802dc`。本目录独立候选已重建为
65 concepts / 394 overrides / 793 fixtures，并完成 fresh generate 与嵌入式 PreParse 验证。

本轮保留 52 个有实际生成差异的命令：新增 62 个 alias、312 个 block、21 个 ambiguous。
主要覆盖分页、人员 ID、班次/规则/请假码/日期角色；删除了没有生成效果的推测词和无法证明值域的
setting-scene 等概念。`page-number` 的共享词表已纳入独立候选，不再依赖别的产品先合并。
调整规则 ID、加班规则 ID、offset 和报表列 ID 均只覆盖同一逻辑端点的两条 CLI 路径，现已下沉为
精确命令 override；原有 alias 与 block 生成行为保持不变。

人员角色二次复核后进一步收紧：`attendance +get-checkin-record` 与 `attendance checkin records`
同时包含操作人单值和目标员工列表，通用 `user/user-id/userid/uid/staff-id` 无法唯一指向操作人，
现统一返回 ambiguous；仅 `operator-user-id/operator-id` 可归一到 `operator-staff-id`，
`target-user-ids/user-id-list` 可归一到 `staff-ids`。`attendance record get` 是单用户查询，复数
`users/user-ids` 保持 block，不再静默降为 `user`。

不能自动解决：日期格式、枚举翻译、复杂排班/审批 JSON、单复数 ID 值内容转换。旧版详细问题表仍见
`attendance_cli_param_hallucination_analysis_20260811.md`；本文件记录最新基线复核结论。
