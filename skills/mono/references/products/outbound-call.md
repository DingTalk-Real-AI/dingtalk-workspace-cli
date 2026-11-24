# AI 语音外呼

命令入口：`dws outbound-call create`、`dws outbound-call detail`。

## 选择与准备

- AI 语音外呼使用本产品；电话 DING、短信 DING 使用 `dws ding`。
- `--callee` 填写被呼叫人的钉钉 userId，每次仅支持一名被呼叫人。只有姓名时，先通过通讯录查询确认 userId。
- 发起前确认被呼叫人、任务说明和开场白；创建会真实呼叫用户，不能自动重试。
- 已有 taskId 时使用 `detail` 查询，不要再次创建。
- 实际执行时一次只能选择一个 profile，不支持逗号分隔的多个身份。dry-run 只预览本地请求，不解析或验证远端身份。

## 创建

为 `create` 生成 `task`、`opening` 时，建议 LLM 默认以**当前用户的 AI 助手**身份代为呼叫，语言风格亲切、友好、自然。`task` 中写明助手身份、语气、通话目标、结束条件和礼貌收尾要求；`opening` 先问好，再说明“我是某某的 AI 助手”和来电目的，礼貌询问或确认。使用已确认的当前用户姓名或称呼，不冒充用户本人或编造身份。

`task` 的结束条件应贴合通话目标：告知类任务在对方确认知情后结束，询问类任务在获得所需答复后结束；对方表示不方便或不愿继续时也应礼貌结束。收尾时简短致谢、道别，等对方说完再挂断，避免只播完开场白就挂断，或在目标完成后反复追问。

以下话术中的“张三”代表当前用户，“XX”代表被呼叫人，使用时替换为已确认的姓名或称呼。

先预览单个外呼：
```bash
dws outbound-call create --callee 'USER_ID' --task '作为张三的 AI 助手，用亲切、友好的语气提醒XX明天上午九点开会，并询问是否方便参加。得到是否参加的答复后，简短致谢并道别。' --opening 'XX，你好！我是张三的 AI 助手，想和你确认一下明天上午九点的会议，请问方便参加吗？' --dry-run
```
告警通知可使用严肃、简洁的语气：
```bash
dws outbound-call create --callee 'USER_ID' --task '作为张三的 AI 助手，用严肃的语气通知XX尽快处理告警信息。告警内容：机器A出现了故障。确保XX知情。对方确认知情后，或表示不方便、不愿继续通话时，礼貌道别结束通话。' --opening 'XX，你好！机器A出现了故障问题，请马上处理。' --dry-run
```
`--callee`、`--task`、`--opening` 必须一起提供，且均为非空字符串。

- `--callee`：被呼叫人的钉钉 userId，原样发送给服务端。示例中的 `USER_ID` 为占位符，需替换为真实 userId。
- `--task`：本次外呼需要完成的事项与要求，包括需要告知的信息、需要询问或确认的问题，以及通话结束条件和礼貌收尾要求。
- `--opening`：外呼开场白，用于向被呼叫人问好，并说明身份和来电目的。

任务说明和开场白保留原文，分别映射到服务端的 `prompt`、`greeting` 字段。输入上限 1 MiB。

用户确认后，以相同参数将 `--dry-run` 替换为 `--yes` 执行。dry-run 完成本地校验并显示请求，远端调用次数为零。

## 查询与结果

```bash
dws outbound-call detail --task-id TASK_ID --format json
```

接口业务回执放在统一输出的 `data` 中，按接口提供的业务字段返回。创建回执能识别单一 taskId 时，返回 `outcome=pending`，`meta.operation.next_command` 提供查询指令。回执未提供单一 taskId 时保留业务结果，不生成虚构的任务 ID。创建接口调用成功仅代表取得回执，不代表通话完成。

详情中的 callStatus、errMsg、callStartTs、callEndTs、taskUrl 等字段在服务端提供时原样返回；查询的 `outcome=success` 不表示通话成功。不在本地猜测状态枚举，不自动轮询。

明确的接口失败返回 `failure`。创建请求的提交结果无法确认时（例如未能确认请求未发送的超时或断连、HTTP 5xx、响应协议或回执无法解析），返回 `submission_unknown`，不能自动重试；先核实任务记录，已有 taskId 时调用 detail。明确未发送的失败、认证、权限及参数拒绝保留原分类。
