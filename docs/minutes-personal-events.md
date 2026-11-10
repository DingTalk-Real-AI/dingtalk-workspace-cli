# 听记个人事件

CLI 提供两个听记事件的注册、Schema 和输出映射。客户端支持不代表当前服务环境已经开通投递；服务端可用性及端到端投递仍需单独验证。

| 事件 | 含义 |
| --- | --- |
| `user_minutes_task_created` | 听记创建完成；不表示纪要已生成 |
| `user_minutes_summary_generated` | 默认纪要首次生成完成；不表示后续每次修改都会发送事件 |

```bash
dws event list --category minutes
dws event schema user_minutes_task_created --flatten
dws event schema user_minutes_summary_generated --flatten
dws event consume user_minutes_summary_generated --flatten --duration 10m -f ndjson
dws minutes get summary --id <taskUuid>
```

读取内容时使用有权限的同一账号及组织配置。CLI 不会因为收到事件自动读取正文或执行业务写入。

## 扁平输出

| 字段 | 含义 |
| --- | --- |
| `schema_version` | 业务载荷版本，首版为 1 |
| `task_uuid` | 听记标识，用于查询命令的 `--id` |
| `creator_open_dingtalk_id` | 创建者开放身份标识，输出为字符串 |
| `biz_id` | 业务事件标识，消费端可用于幂等处理 |
| `event_time` | 业务发生时间，毫秒 |
| `event_id` | 平台事件标识，不假定等于 `biz_id` |

`--flatten` 另保留 `type`、`timestamp` 和 `subscribe_id` 元数据，不输出路由身份字段。默认输出保留传输信封。

创建者标识接受正 int64 或非空字符串；正整数转换为十进制字符串以保持精度。缺失、null、0、负数和布尔值无效。该转换只改变表示，不进行身份映射；不要把接收人的路由身份当作创建者开放身份。

## 消费边界

- 两个事件属于 `minutes`，使用用户身份和 `all` 订阅规则，无额外用户、群、角色或关键词过滤。
- 同一事件可能重复或乱序到达。不要等待创建事件后才处理纪要事件，也不要依靠投递顺序推导业务状态。
- 不应假定订阅会补推历史；取消后在途消息仍可能到达。
- 事件不携带听记或纪要正文。收到生成通知后，使用 `task_uuid` 和现有查询命令读取获授权的内容。
- 事件开通范围、接收对象、创建者身份编码、跨客户端订阅与去重语义由服务端契约决定，客户端 Schema 不能证明这些能力已经部署。
