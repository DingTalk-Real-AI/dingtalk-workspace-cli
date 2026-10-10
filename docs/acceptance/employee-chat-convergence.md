# 数字员工发送收敛与本地联合验收

通用 `chat +messages-reply` 和 `chat +messages-send` 承载数字员工的消息发送。`channel capabilities` 和 `channel binding` 继续负责能力发现与绑定读取；旧 `channel reply/operator-private` 保留兼容入口，并与 chat 共用引用内容和回执处理。

## 调用约定

- `--body-stdin`：正文参数显式为 `-`，从标准输入读取最多 256 KiB 正文。未启用时，`-` 仍为字面量。
- `--wait-delivery`：个人文本、Markdown 或普通引用回复查询发送任务的回执，输出 `openMessageId`、`conversationId`、`deliveryStatus`、`idempotencyKey`。遵循命令现有输出契约；未来统一 envelope 时字段位于 `data`。
- `--employee-context`：JSON 含 `agentUuid`、`channel`、`bindingRevision`；要求显式 Profile、stdin、回执与幂等键。读取和发送前核对绑定仍为 bound/running，私聊目标必须是 connect 固定的主管。
- chat 原有用户确认保持生效。connect 已授权的宿主通过 `--yes` 执行自动回传；上下文只是绑定约束，不能代替授权。
- `local_agent` 的入站权限采用 DEAP 已发布可见范围（含所选部门子部门）；其他场景保留本地白名单。两者都不能替代固定主管审批身份。

DSH 在启动探测中发现 `capabilities.chatDelivery=true` 后选择通用 chat，否则使用兼容 channel 协议。发送失败或回执未知不会切换协议重发。DWS 自带 worker 与自身版本同步，直接使用通用 chat。

## 本地验收方法

在 DSH 仓库运行：

```sh
DWS_JOINT_BINARY=/absolute/path/to/built/dws node --test test/digital-employee-chat-delivery.test.mjs
```

使用真实 DWS 子进程、隔离 Profile/绑定目录、虚构凭据及本机 HTTP MCP。覆盖普通回复、主管私聊、未知状态不重发、过期绑定、停止运行、错误主管、查原消息期间绑定变化、源消息会话不符；拒绝场景断言发送调用数为零。另有模拟旧版/新版 DWS 的协商与无重发测试。

该验收不发送真实钉钉业务消息，也不证明云端账号权限或真实业务 E2E。


## DEAP 可见范围联合验收

- DWS 使用绑定的管理 Profile 查询 published；以员工 Profile 精确匹配消息发送者开放 ID。ALL 需确认企业成员，PARTIAL 支持成员及部门子树；部门遍历最多 256 个节点，查询失败或超限拒绝处理。
- `channel capabilities.visibilityAccess` 声明能力；`channel binding --stdin` 增加可选 senderOpenDingTalkId / senderName，返回 accessPolicy，DEAP 策略同时返回 allowed。名称仅用于检索，开放 ID 必须精确匹配。
- 内置 Agent 和 DSH 均在任务调度前判断；拒绝不启动 Agent，查询失败不写事件去重。旧绑定通过原管理 Profile 的 connect restart 迁移，不能用其他账号接管。
- 单元回归覆盖成员新增/撤销、operator 无聊天特权、ALL 企业成员确认、子部门、旧场景白名单、查询错误和发布身份冲突；Schema 检查覆盖最终绑定查询结果。
- 真实本机 DSH → DWS → DEAP 只读验收：已发布 PARTIAL 范围内新增成员，无需本地白名单，allowed=true；宿主连接与执行器 ready，A2UI 启用。新成员发消息后的实际回复送达待人工触发，此只读检查不等同于聊天 E2E。
