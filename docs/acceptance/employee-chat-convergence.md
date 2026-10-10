# 数字员工发送收敛与本地联合验收

通用 `chat +messages-reply` 和 `chat +messages-send` 承载数字员工的消息发送。`channel capabilities` 和 `channel binding` 继续负责能力发现与绑定读取；旧 `channel reply/operator-private` 保留兼容入口，并与 chat 共用引用内容和回执处理。

## 调用约定

- `--body-stdin`：正文参数显式为 `-`，从标准输入读取最多 256 KiB 正文。未启用时，`-` 仍为字面量。
- `--wait-delivery`：个人文本、Markdown 或普通引用回复查询发送任务的回执，输出 `openMessageId`、`conversationId`、`deliveryStatus`、`idempotencyKey`。遵循命令现有输出契约；未来统一 envelope 时字段位于 `data`。
- `--employee-context`：JSON 含 `agentUuid`、`channel`、`bindingRevision`；要求显式 Profile、stdin、回执与幂等键。读取和发送前核对绑定仍为 bound/running，私聊目标必须是 connect 固定的主管。
- chat 原有用户确认保持生效。connect 已授权的宿主通过 `--yes` 执行自动回传；上下文只是绑定约束，不能代替授权。
- `--allowed-users` 管入站触发权限，不能替代固定主管身份。

DSH 在启动探测中发现 `capabilities.chatDelivery=true` 后选择通用 chat，否则使用兼容 channel 协议。发送失败或回执未知不会切换协议重发。DWS 自带 worker 与自身版本同步，直接使用通用 chat。

## 本地验收方法

在 DSH 仓库运行：

```sh
DWS_JOINT_BINARY=/absolute/path/to/built/dws node --test test/digital-employee-chat-delivery.test.mjs
```

使用真实 DWS 子进程、隔离 Profile/绑定目录、虚构凭据及本机 HTTP MCP。覆盖普通回复、主管私聊、未知状态不重发、过期绑定、停止运行、错误主管、查原消息期间绑定变化、源消息会话不符；拒绝场景断言发送调用数为零。另有模拟旧版/新版 DWS 的协商与无重发测试。

该验收不发送真实钉钉业务消息，也不证明云端账号权限或真实业务 E2E。
