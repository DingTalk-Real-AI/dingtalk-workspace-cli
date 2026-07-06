# 个人消息事件自测流程

## 1. 确认登录

个人事件使用当前用户 OAuth 登录态。未登录或 token 失效时，先执行：

```bash
dws auth login
```

## 2. 查看事件说明

```bash
dws event schema user_im_message_receive_at
dws event schema user_im_message_receive_o2o
dws event schema user_im_message_receive_group
dws event schema user_im_message_receive_user
```

确认事件规则分别是 `at`、`singleChat`、`group`、`sender`。

## 3. 启动监听

### 被 @ 消息

```bash
dws event consume user_im_message_receive_at \
  --duration 10m \
  -f ndjson
```

触发方式：让任意可触达用户在群里 @ 当前登录用户。

### 指定单聊消息

优先用对端 `userId`：

```bash
dws event consume user_im_message_receive_o2o \
  --peer-user-id 507971 \
  --duration 10m \
  -f ndjson
```

如果只有 `unionId`：

```bash
dws event consume user_im_message_receive_o2o \
  --peer-union-id <unionId> \
  --duration 10m \
  -f ndjson
```

触发方式：让对端用户给当前登录用户发送一条单聊消息。

### 指定群消息

先拿到目标群的 `openConversationId`。如果只有群名：

```bash
dws chat search --query "群名" --format json
```

确认群后启动监听：

```bash
dws event consume user_im_message_receive_group \
  --open-conversation-id <openConversationId> \
  --duration 10m \
  -f ndjson
```

触发方式：让任意用户在该群发送一条消息。

### 指定发送人消息

优先用发送人的 `userId`。如果只有人名：

```bash
dws aisearch person --keyword "张三" --dimension name --format json
```

确认用户后启动监听：

```bash
dws event consume user_im_message_receive_user \
  --sender-user-id <userId> \
  --duration 10m \
  -f ndjson
```

如果只有 `unionId`：

```bash
dws event consume user_im_message_receive_user \
  --sender-union-id <unionId> \
  --duration 10m \
  -f ndjson
```

触发方式：让该发送人给当前用户发送单聊消息，或在当前用户能收到的会话里发消息。

## 4. 无输出排查

1. 确认事件码和必填参数正确。
2. 用 event status 加对应事件码查看订阅和本地连接状态。
3. 联调服务端时临时加 `--debug --debug-raw-events`，观察当前 personal stream bus 是否收到服务端推送。
4. 读取业务字段前先抓一条 `-f json --max-events 1` 样本；payload 以实际推送为准。

## 5. 停止监听

先查订阅 ID：

```bash
dws event status --event user_im_message_receive_at
dws event status --event user_im_message_receive_o2o
dws event status --event user_im_message_receive_group
dws event status --event user_im_message_receive_user
```

停止指定订阅：

```bash
dws event stop <subscribe_id>
```

清理当前身份下本地记录的全部个人订阅：

```bash
dws event stop --all
```

## 6. 安装到 Agent

安装 multi skill 的 event 子 skill：

```bash
dws skill setup --mode multi -s event --target codex --source <repo> --yes
```

安装 mono skill：

```bash
dws skill setup --mode mono --target codex --source <repo> --yes
```
