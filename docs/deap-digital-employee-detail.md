# DEAP 数字员工详情参数

`dws dingtalk-tag manage detail` 通过 `--snapshot` 选择配置快照：

```text
dws dingtalk-tag manage detail --agent-uuid <agentUuid> --snapshot draft
dws dingtalk-tag manage detail --agent-uuid <agentUuid> --snapshot published
```

`--snapshot` 只接受 `draft` 或 `published`，默认 `draft`。CLI 映射到
`get_digital_employee_detail` 的 `snapshot` 字段。旧 `--type` 保留为兼容别名，
也发送 `snapshot`；登录和连接内部的详情查询使用相同字段。

详情响应中的 `type` 表示主程序类型（`open_code` / `local_agent`）。
`status` 表示发布状态：`online` 为已发布，`dev` / `offline` 为未发布；
它不能单独证明本次读取的是 published 快照，也不代表本地 Agent 正在运行。
