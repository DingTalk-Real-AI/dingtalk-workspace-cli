# 数字员工发送收敛与本地联合验收

通用 `chat +messages-reply` 和 `chat +messages-send` 承载数字员工的消息发送。`channel capabilities` 和 `channel binding` 继续负责能力发现与绑定读取；已发布的 `channel reply` 和 `channel operator-private` 保留兼容入口，复用统一发送回执；新版宿主发送使用 chat。

## 调用约定

- `--body-stdin`：正文参数显式为 `-`，从标准输入读取最多 256 KiB 正文。未启用时，`-` 仍为字面量。
- `--wait-delivery`：个人文本、Markdown 或普通引用回复查询发送任务的回执，输出 `openMessageId`、`conversationId`、`deliveryStatus`、`idempotencyKey`。遵循命令现有输出契约；未来统一 envelope 时字段位于 `data`。
- `--employee-context`：JSON 含 `agentUuid`、`channel`、`bindingRevision`；要求显式 Profile、stdin、回执与幂等键。读取和发送前核对绑定仍为 bound/running，私聊目标必须是 connect 固定的主管。
- chat 原有用户确认保持生效。connect 已授权的宿主通过 `--yes` 执行自动回传；上下文只是绑定约束，不能代替授权。
- 正式绑定的 `local_agent` 由平台保证私聊投递权限、群内 @ 可回复；本地不重复查询可见范围。其他场景保留本地白名单，聊天权限不能替代固定主管审批身份。

DSH 启动要求 `capabilities.chatDelivery=true` 与 `capabilities.bindingAccessPolicy=true`；缺少能力时明确提示升级 DWS 并停止启动。发送失败或回执未知不会切换协议重发。DWS 自带 worker 与自身版本同步，直接使用通用 chat。

## 本地验收方法

在 DSH 仓库运行：

```sh
DWS_JOINT_BINARY=/absolute/path/to/built/dws node --test test/digital-employee-chat-delivery.test.mjs
```

使用真实 DWS 子进程、隔离 Profile/绑定目录、虚构凭据及本机 HTTP MCP。覆盖普通回复、主管私聊、未知状态不重发、过期绑定、停止运行、错误主管、查原消息期间绑定变化、源消息会话不符；拒绝场景断言发送调用数为零。另有仅声明新版能力的 DWS 接入、旧版拒绝启动与无重发测试。

该验收不发送真实钉钉业务消息，也不证明云端账号权限或真实业务 E2E。


## 平台入站权限联合验收

- `channel binding --stdin` 在启动时返回本地权威绑定及 `accessPolicy`，不接收 sender 或 conversationType，不返回 allowed。正式服务端绑定为 `platform`，无回执的本地通道为 `local_allowlist`。损坏、过期、解绑或未完成服务端操作的绑定不授权启动。
- 平台确认：移出私聊可见范围后停止投递；群内 @ 可回复；从 local_agent 修改类型后停止旧绑定投递、需重新绑定。本地测试只验证对此投递契约的消费，不宣称模拟验证了平台行为。
- 单元回归覆盖旧绑定缺主管字段也可租约启动、原生启动不枚举账号和查询远端、受管私聊及群 @ 不查可见范围、普通群消息和自身消息忽略、非受管通道仍执行白名单、主管历史字段不升级授权、过期 revision/解绑/损坏绑定拒绝。
- 旧的主管字段迁移、可见范围成员解析及部门展开代码已删除。新绑定不再保存 supervisorProfile，旧字段仅兼容读取；固定 operator 审批和当前主管管理权限不变。
- DSH 内部 `connect --local-lease` 在宿主确认 `released` 并成功移除隔离标记后输出统一成功结果 `status:released`、退出码 0；未确认释放仍失败并保留隔离标记。
- `platform` 必须返回 `selfOpenDingTalkId` 供插件排除自身消息；新绑定从已验证的 adapter 配置读取。旧 DSH 配置缺自身 ID 时只在启动阶段用员工 Profile 查询自己一次，核对 token 精确身份并在返回前重核绑定；不要求主管登录、不写新迁移字段。回归覆盖错配 token、查询失败、旧 adapter 版本和查询期间换绑。
- 真正钉钉业务 E2E 需要另外记录真实入站、模型执行、回复回执和 IM 读回；本轮修改前的 1010/10101 记录不代表本版本已完成现场验收。
