# dingtalk-tag run — 数字员工执行查询

命令前缀 `dws dingtalk-tag run`。全部只读。

## 两个命令，入参完全一致

```
Usage:
  dws dingtalk-tag run run-status --agent-uuid <id> --source-id <来源ID> --source-type <类型>
  dws dingtalk-tag run trace      --agent-uuid <id> --source-id <来源ID> --source-type <类型>
Flags（三者均必填）:
  --agent-uuid     数字员工唯一标识
  --source-id      来源侧原始 ID（取法见下）
  --source-type    im_message | trigger_rule
Example:
  dws dingtalk-tag run run-status --agent-uuid <id> --source-id <openMessageId> --source-type im_message --format json
```

- `run-status` 返回 `result`（1 成功 / -1 失败 / 0 运行中 / 2 中止）、`execStatus`、`runId`、`messageId`
- `trace` 返回 Langfuse 原始 trace JSON，**含完整对话与模型输入输出**；服务端先做管理者 / 触发人两级授权，无权返回 `NO_PERMISSION`

**不接 `--run-id`**：调用方手上只会有来源侧原始 ID，拿不到 runId。runId 是**出参**，只用于人工去 SLS / Langfuse 控制台比对。

`trigger_rule` 的来源 ID 可能对应多次执行，只返回最新一次。

## source-id 取法

| source-type | source-id 是什么 | 典型场景 |
|---|---|---|
| `im_message` | 钉钉开放态 `openMessageId` | 单聊 / 群 @ 数字员工 / 群感知由消息发起 |
| `trigger_rule` | 群感知规则 ID | 群感知非消息发起（定时、状态变化等） |

### openMessageId ≠ openTaskId（最易踩的一处）

两者都是 Base64、长度相近、形态相似，但完全不是一回事：

| | 示例 | 谁产生 | 何时可得 |
|---|---|---|---|
| `openTaskId` | `hADDJMrCIo5KbRvvj3/JDq/GHbTZUkV6P72//NGIQgw=` | 钉钉发送接口 | 发送请求返回时（消息还没落地） |
| `openMessageId` | `msgeOkiAslxLfjM/fuYRe7C0A==` | 钉钉 IM 投递后 | 消息真正落地后 |

**只有 `openMessageId` 在来源映射里有记录**，拿 `openTaskId` 查必然得到 `NOT_FOUND`。

### 取法一：自己用 dws 发的消息（需要两跳）

`dws chat message send` 是异步的，返回时消息尚未投递，因此**只给 openTaskId**，必须再查一次换成 openMessageId：

```
# 1. 发消息 → openTaskId
dws chat message send --user <userId> --text "..." --format json
# → { "openTaskId": "hADDJMrCIo5KbRvvj3/JDq/GHbTZUkV6P72//NGIQgw=" }

# 2. 查发送状态 → openMessageId
dws chat message query-send-status --open-task-id "<openTaskId>" --format json
# → { "openMessageId": "msgeOkiAslxLfjM/fuYRe7C0A==",
#      "openConversationId": "cidn3a++FFQDhGwJSr2duuGICYgNmuYMFmjNlCEnPITScI=",
#      "sendStatus": "SUCCESS" }

# 3. 查执行状态
dws dingtalk-tag run run-status --agent-uuid <agentUuid> \
  --source-id "msgeOkiAslxLfjM/fuYRe7C0A==" --source-type im_message --format json
```

字段名是 `openMessageId`，**不是 `openMsgId`**（后者是服务端内部 JSON key 的写法，两边命名不一致，写解析代码时注意）。

第 2 跳顺带返回 `openConversationId`，做会话级关联时可直接用。

### 取法二：不是自己发的消息

用户 @ 数字员工、群里别人发的消息等，没有 openTaskId 可言（发送方不是你）。直接从消息列表取，每条消息都带 `openMessageId`：

```
# 单聊
dws chat message list --user <userId> --time "<起始时间>" --limit 20 --format json

# 群聊
dws chat message list --group <openConversationId> --time "<起始时间>" --limit 20 --format json
```

`--time` 必填。群 ID 用 `dws chat search --query "群名"` 查。

### trigger_rule 的取法

群感知不一定由某条消息发起（定时触发、群状态变化等），此时没有 openMessageId，用感知规则 ID 兜底反查。规则 ID 从该数字员工的群感知配置处获得。

## 查不到时的排查顺序

`NOT_FOUND / source mapping not found` 的可能原因，按概率排：

1. **传的是 openTaskId 而不是 openMessageId** —— 回看产生该值的命令和 JSON 字段名，不要仅凭字符串形态判断
2. **source-type 与 source-id 不匹配** —— 拿规则 ID 配了 `im_message`，或反之
3. **该来源确实没有映射** —— 非本平台链路触发，或该来源不属当前组织
4. **执行刚发生，映射还没写入** —— 稍等重试

另一种情况别误判：`run-status` **成功返回但 `result` / `execStatus` 为 null**，表示这次执行既没有终态记录也不在运行中，属于"无法判定"，不是查不到——此时 `runId` 与 `messageId` 是有值的。

## 硬约束

- 三个 flag 全必填，且不接 `--run-id`。
- 同一个 `openMessageId` 可能触发多个数字员工；必须分别传各自的 `agentUuid` 查询，返回的 run / trace 也可能不同。
- `trace` 返回完整对话内容，属敏感读：服务端已做两级授权，但不要把 trace 原文转发到非授权场域或粘贴到公共渠道。
- 不要传 `--org-id` / `--user-id`：identity 由可信登录态注入，不对 CLI 暴露。

## 跨产品协作

- 发消息、查会话消息、取 `openMessageId` / `openConversationId`：切换群聊与机器人能力，使用 `dws chat ...`。
- 数字员工本身的配置与发布：见 [`manage.md`](./manage.md)。
