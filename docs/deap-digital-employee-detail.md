# DEAP 数字员工详情参数

`dws dingtalk-tag manage detail` 通过 `--snapshot` 选择配置快照：

```text
dws dingtalk-tag manage detail --agent-uuid <agentUuid> --snapshot draft
dws dingtalk-tag manage detail --agent-uuid <agentUuid> --snapshot published
```

`--snapshot` 只接受 `draft` 或 `published`，默认 `draft`。CLI 映射到
`get_digital_employee_detail` 的 `snapshot` 字段。登录和连接内部的详情查询使用相同字段。

详情响应中的 `type` 表示主程序类型（`open_code` / `local_agent`）。
响应的 `snapshot` 表示本次读取的配置来源（`draft` / `published`）。
`status` 表示生命周期：`online` 为已发布，`dev` 为开发中，`offline` 为下架；
它不能单独证明本次读取的是 published 快照，也不代表本地 Agent 正在运行。

未成功发布（`status=dev`）的员工查询 `published` 返回 `NOT_FOUND`，不返回成功的草稿对象。曾发布后下架的员工仍可读取保留的已发布配置，返回 `snapshot=published`、`status=offline`。这一服务端契约需要对应 OpenAPI / Studio 修复部署后生效。
