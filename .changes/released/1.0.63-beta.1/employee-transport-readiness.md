---
category: Fixed
---

- 修正数字员工接入的事件传输就绪判断：使用上游真实状态并报告断线重连，区分 IPC、Agent 初始化和事件连接；DSH 状态交叉验证同一员工的 Event Bus，旧版或未知连接不再报告传输就绪。
- 数字员工本地 Agent 不再固定指定事件来源为 `digital_employee`，改为沿用 Event 默认规则：优先读取 `DWS_STREAM_SOURCE_ID`，未设置时开源版使用 `open`；升级后需重启员工 worker 生效。
- 个人 Stream 正确响应系统 ping/disconnect，不将系统控制帧投递为业务消息；订阅查询兼容 `items`/`list` 及字符串/整数时间戳。
- 修正 Windows 下正常停止可能被误报为消费者异常退出，以及损坏的任务/绑定目录被误当作不存在的问题；补充跨平台身份、绑定回执、上传失败、重试隔离和日志脱敏回归测试。
