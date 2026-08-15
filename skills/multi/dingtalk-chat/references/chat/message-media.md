# message-media：特殊消息与资源下载

> 返回入口：[DingTalk Chat Skill](../../SKILL.md)

只用于位置、联系人名片、底层 mediaId/fileId 和消息资源下载。普通文本、Markdown、文件、
图片、音频和视频发送继续按根 Skill 使用 `+dm`、`+send-to-group` 或 `+messages-send --file`，
不读取本文件。

## 默认边界

- 用户身份普通文件/音视频：`dws chat +messages-send --as user --file <相对路径>`。
- 已知资源引用单独下载：`dws chat +messages-resource-download`。
- 从消息中定位并下载资源：在定位消息的 `+chat-messages`、`+search-msg` 或
  `+messages-mget` 同一次调用中加 `--download-resources`。
- 只有 Shortcut 尚未发布的位置、联系人名片或真实底层媒体字段，才使用原子 fallback。

<!-- dws-intent: chat.send.advanced -->`dws chat +messages-send` 的 user 文件能力不能外推给 Bot/Webhook；
机器人富媒体边界读取 [chat-bot.md](chat-bot.md)，不得静默改成当前用户身份。

## 位置与联系人名片

位置消息必须确认纬度、经度、地址名称和地图缩略图 mediaId：

```bash
dws chat message send --group <openConversationId> --msg-type location \
  --latitude <纬度> --longitude <经度> --location-name <地址名称> \
  --map-thumbnail-url "@mediaId"
```

联系人名片的 `--contact-id` 必须是联系人 `openDingTalkId`，不能把 userId 直接代入：

```bash
dws chat message send --group <openConversationId> \
  --msg-type profile --contact-id <openDingTalkId>
```

用户要求真实发送结果时，保留发送返回的 `openTaskId`，再执行：

```bash
dws chat message query-send-status --open-task-id <openTaskId> --format json
```

检查真实 `sendStatus`、`openMessageId` 和 `openConversationId`。

原子 `message send` 只在 Shortcut 缺少真实必需字段时使用。群聊目标用 `--group`；单聊目标
用 `--user` 或 `--open-dingtalk-id`，三者通常互斥。发送前核对接收对象、消息类型和资源来源。

## 资源下载

公开 `+messages-resource-download` 使用工作目录内安全相对路径，默认不覆盖；完整文件先写入
临时落盘再原子发布。覆盖必须由用户显式传 `--overwrite`，读取和下载不需要 `--yes`。

任务要求从某条消息中定位并下载资源时，优先在限定会话、消息或时间范围的查询中加
`--download-resources --output-dir <目录>`，并检查下载 ledger。`+messages-resource-download`
只用于已经持有完整、真实且属于当前组织/profile 的独立资源引用、无需再定位消息的场景。
若 `fileId` 返回 `RESOURCE_NOT_FOUND`，不得把同一个 ID 改称 `mediaId` 重试，也不得原样
重复调用；应回到消息查询并使用 `--download-resources`。

底层 fallback：

```bash
dws chat message download-media --type mediaId --resource-id <mediaId> \
  --message-id <openMessageId> --open-conversation-id <openConversationId> \
  --output ./downloads/
```

`resource-id`、`message-id` 和会话 ID 必须来自同一 profile 下的真实消息查询结果。
当前没有 Range/断点续传；失败时保留 ledger 或错误，显式重试整个文件，不拼接残片。

## 完成与错误

- 查询并下载时同时检查消息完整性和每项下载 ledger；单项失败不抹掉已取得消息。
- 文件/音视频发送失败先确认工作目录内相对路径可读，不恢复独立上传再提取 mediaId 的旧默认链路。
- 位置参数不完整时先向用户确认，不猜经纬度或缩略图。
- 名片发送失败时确认 `--contact-id` 是 openDingTalkId。
- 下载目标存在时默认停止；只有用户明确允许覆盖时才传 `--overwrite`。
