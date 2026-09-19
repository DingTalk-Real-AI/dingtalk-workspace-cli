# DEAP 数字员工详情参数

`dws dingtalk-tag manage detail` 保持单一命令，通过 `--type` 选择要读取的配置版本：

```text
dws dingtalk-tag manage detail --agent-uuid <agentUuid> --type draft
dws dingtalk-tag manage detail --agent-uuid <agentUuid> --type published
```

`--type` 只接受 `draft` 或 `published`，默认值为 `draft`。CLI 将参数原样映射到
`get_digital_employee_detail` 的 `type` 字段，不增加新的命令或客户端接口。

详情响应沿用已有的字符串字段 `status` 表示当前发布状态：

- `online`：已发布。
- `dev`、`offline`：未发布。

`type` 表示本次读取的配置版本，`status` 表示数字员工当前是否处于已发布状态。
