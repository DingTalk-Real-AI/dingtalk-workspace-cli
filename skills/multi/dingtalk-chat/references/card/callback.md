# 卡片回调边界

`callback_supported=false` 仅描述 `dingtalk-chat` 的卡片创建/更新 lower interface：
它不提供服务端 callback URL 注册、签名密钥/验签或 callback 回复能力。因此：

- 不生成 callback URL、签名密钥、验签或回复命令；
- 需要接收并处理服务端 HTTP callback 时，停止并说明当前尚未开放该能力。

监听当前 OAuth 用户收到的互动卡片操作事件是另一条已开放能力，路由到
[`dingtalk-event`](../../../dingtalk-event/SKILL.md)：

```bash
dws event consume user_card_action_triggered --flatten -f ndjson
```

该命令通过个人事件长连接订阅和消费回调事件，不会创建 callback URL，也不提供服务端
验签或 callback 回复。卡片 create/update、个人事件监听和服务端 callback 接入是三个不同边界。
