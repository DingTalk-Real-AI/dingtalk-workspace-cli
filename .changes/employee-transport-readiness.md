---
category: Fixed
---

- 修正数字员工接入的事件传输就绪判断：使用上游真实状态并报告断线重连，区分 IPC、Agent 初始化和事件连接；DSH 状态交叉验证同一员工的 Event Bus，旧版或未知连接不再报告传输就绪。
- 个人 Stream 正确响应系统 ping/disconnect，不将系统控制帧投递为业务消息；订阅查询兼容 `items`/`list` 及字符串/整数时间戳。
