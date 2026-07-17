# 复合工作流

## 复合工作流

### 机器人发消息后撤回（完整流程）

`recall-by-bot` 通过机器人接口撤回机器人发出的消息（需要 `--robot-code` + `--keys`）。`chat message recall` 通过 IM 接口撤回当前用户自己发出的消息（需要 `--conversation-id` + `--msg-id`）。

```bash
# Step 1: 查我的机器人 — 提取 robot-code
dws chat bot search --format json

# Step 2: 用机器人发消息 — 提取返回中的 processQueryKey
dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --title "通知" --text "内容" --format json

# Step 3: 用同一个 robot-code + processQueryKey 撤回
dws chat message recall-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --keys <processQueryKey> --format json
```

### 机器人发群消息（含机器人不在群内的处理）

机器人通过 `send-by-bot --group` 发群消息时，如果返回"机器人不存在"错误，说明该机器人尚未加入目标群，需先邀请进群再发送。

```bash
# Step 1: 查我的机器人 — 提取 robot-code
dws chat bot search --format json

# Step 2: 尝试发送，若报"机器人不存在"则执行 Step 3
dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --title "通知" --text "内容" --format json

# Step 3: 邀请机器人进群
dws chat group members add-bot --id <openconversation_id> --robot-code <robot-code>

# Step 4: 重新发送
dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --title "通知" --text "内容" --format json
```

### 给机器人发单聊消息（必须先用 find 拿 openDingTalkId）

给机器人发单聊消息时，必须先用 `chat bot find` 搜索机器人拿到 `openDingTalkId`，再用 `chat message send --open-dingtalk-id` 发送。不能用 `chat bot search`，因为 search 不返回 `openDingTalkId`。

```bash
# Step 1: 搜索机器人 — 提取 openDingTalkId（必须用 find，search 没有此字段）
dws chat bot find --query "玉澜" --format json

# Step 2: 用 openDingTalkId 发单聊消息
dws chat message send --open-dingtalk-id <openDingTalkId> --text "你好" --format json
```

### 机器人 @指定人发群消息

通过 `--at-user-ids` 传入 userId 列表或 `--at-open-dingtalk-ids` 传入 openDingtalkId 列表来 @指定成员，多个用逗号分隔。`--text` 中需包含 `@userId` 或 `@openDingtalkId` 文本（不要用尖括号，不要用姓名）。通过 `--at-all` @所有人。

```bash
# Step 1: 搜人获取 userId
dws contact user search --query "张三" --format json

# Step 2: 用 userId 发送并 @（注意 text 中 @userId）
dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --at-user-ids userId1,userId2 \
  --title "提醒" --text "@userId1 @userId2 请查收本周报告" --format json

# 或者用 openDingtalkId 发送并 @
dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --at-open-dingtalk-ids openDingtalkId1,openDingtalkId2 \
  --title "提醒" --text "@openDingtalkId1 @openDingtalkId2 请查收本周报告" --format json

# @所有人
dws chat message send-by-bot --robot-code <robot-code> --group <openconversation_id> \
  --at-all --title "通知" --text "请所有人注意" --format json
```


### 发送图片 / 文件（统一一条命令）

**`dws chat message send --msg-type file --file-path <本地路径>`** 适用于所有发图片/文件场景，任意扩展名。CLI 内部完成上传与发送，无需任何前置工具调用。

```bash
# 群聊
dws chat message send --group <openConversationId> --msg-type file --file-path ./screenshot.png --format json
dws chat message send --group <openConversationId> --msg-type file --file-path ./report.pdf    --format json

# 单聊（推荐 --open-dingtalk-id）
dws chat message send --open-dingtalk-id <openDingTalkId> --msg-type file --file-path ./screenshot.png --format json
```

**带文字说明**：在上一步发完文件后，再补一条文本消息即可。不要尝试把文字塞进 `--msg-type file` 命令（该命令不读 `--text`）。

```bash
dws chat message send --open-dingtalk-id <openDingTalkId> --msg-type file --file-path ./screenshot.png --format json
dws chat message send --open-dingtalk-id <openDingTalkId> --text "这是本周数据汇总" --format json
```

**旧链路（仅当上游已经持有 `dt_media_upload` 返回的 mediaId 时才用）**：

```bash
dws chat message send --group <openConversationId> --msg-type image --media-id "@lQLPD4JNnliqBq3NBQDNA8Cw" --format json
```

#### 创建并推送流式卡片 — 向群聊或单聊发送流式卡片消息

群聊传 --group，单聊传 --receiver，二者互斥。

**注意：send-card 必须和 update-card 搭配使用。** 创建卡片时无需传入内容，后续通过 update-card 更新内容，最后一次更新必须将 --flow-status 设为 3（finish），否则卡片会一直处于"生成中"的加载状态。
flow-status 取值：1=处理中(PROCESSING)，2=输入中(INPUTTING)，3=完成(FINISH)，4=执行中(EXECUTING)，5=错误(ERROR)。
```
Usage:
  dws chat message send-card [flags]
Example:
  dws chat message send-card --group <openConversationId>
  dws chat message send-card --receiver <openDingTalkId>
  # 查询群 ID: dws chat search --query "群名"
  # 查询人员: dws contact user search --query "姓名" --format json
Flags:
      --group string      群聊 openConversationId（群聊时必填，与 --receiver 互斥）
      --receiver string   单聊接收者 openDingTalkId（单聊时必填，与 --group 互斥）
```

#### 流式更新卡片内容 — 更新已发送的流式卡片内容

--biz-id 为 send-card 返回的业务 ID，--flow-status 控制流式状态。
flow-status 取值：1=处理中(PROCESSING)，2=输入中(INPUTTING)，3=完成(FINISH)，4=执行中(EXECUTING)，5=错误(ERROR)。

**最后一次更新必须将 --flow-status 设为 3（finish），否则卡片会一直处于"生成中"的加载状态。**
```
Usage:
  dws chat message update-card [flags]
Example:
  dws chat message update-card --biz-id <bizId> --content "更新的卡片内容" --flow-status 2
  dws chat message update-card --biz-id <bizId> --content "最终内容" --flow-status 3
Flags:
      --biz-id string    卡片业务 ID (必填)
      --content string   卡片消息内容 (必填)
      --flow-status int  流式状态 (必填)
```
