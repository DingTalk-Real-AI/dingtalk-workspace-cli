# 卡片回调边界

当前 DWS lower interface 没有卡片按钮/action callback 的订阅、验签或回复能力，
`callback_supported=false`。因此：

- 不生成 callback URL、签名密钥或虚构的监听命令；
- 不把 `dws event consume` 当作卡片 callback 的替代；
- 用户必须使用按钮交互时，停止并说明当前不支持，等待平台接口和 Runtime 正式发布。

卡片的 create/update 能力不代表 callback 可用。
