# message-media：特殊消息与资源下载

> 返回入口：[DingTalk Chat Skill](../../SKILL.md)

用于本地图文、图片路径错误、位置、联系人名片及消息资源下载。普通文本和文件发送按根 Skill 路由。

## 发送本地图文

用户要求将图片与文字放在同一条消息中时，使用 `+messages-send --markdown`；要求发送文件附件时使用 `--file`。

1. **准备图片。** 取得用户附件的真实路径，确认执行 DWS 的进程能够读取。图片引用使用该进程工作目录内的相对路径；任务工作区与工具执行目录可能不同，以实际执行环境为准。
2. **组织正文。** 用 `![图片说明](相对路径)` 插入图片，保持用户指定的文字、图片顺序。例如：

   ```markdown
   请看这张图片：
   ![现场照片](uploads/photo.png)
   以上是今天的现场情况。
   ```

   示例路径替换为第一步确认的图片路径。
3. **发送。** 将正文传给 `dws chat +messages-send --markdown`，CLI 自动上传图片并生成内联图文。接收对象、发送身份及确认要求沿用根 Skill 的发送流程。
4. **验收。** 返回 `openTaskId` 时，用 `+messages-query-send-status` 查询投递结果；根据实际状态报告已发送、处理中或失败。成功后简洁说明发送对象和内容，群名、人名用于展示，内部 ID 用于后续调用。

### 图片读取失败

文件不存在时核对附件位置与执行目录映射；无读取权限时检查进程访问权限；需要相对路径时按已确认的执行目录转换。工作目录位置从宿主执行上下文获取，修正后重试。可控制执行目录时，在同一次 shell 调用中切换到图片所在目录再执行；工具转发场景使用宿主提供的路径映射。若仍无法访问，向用户说明附件读取受阻及所需操作。用户可见回复仅包含业务结果、必要的失败原因和下一步；文件定位与工具诊断在内部完成。

`--dry-run` 的 `imageCount` 只表示识别到图片引用；可读性需实际检查。收到发送任务回执后，继续查询原任务，避免重复投递。

进展反馈围绕发送结果；仅在需要用户协助时说明具体障碍和所需操作。

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
dws chat message send --conversation-id <openConversationId> --msg-type location \
  --latitude <纬度> --longitude <经度> --location-name <地址名称> \
  --map-thumbnail-url "@mediaId"
```

联系人名片的 `--contact-id` 必须是联系人 `openDingTalkId`，不能把 userId 直接代入：

```bash
dws chat message send --conversation-id <openConversationId> \
  --msg-type profile --contact-id <openDingTalkId>
```

用户要求真实发送结果时，保留发送返回的 `openTaskId`，再执行：

```bash
dws chat +messages-query-send-status --open-task-id <openTaskId> --format json
```

检查真实 `sendStatus`、`openMessageId` 和 `openConversationId`。

原子 `message send` 只在 Shortcut 缺少真实必需字段时使用。群聊目标用 `--conversation-id`；单聊目标
用 `--user` 或 `--open-dingtalk-id`，三者通常互斥。发送前核对接收对象、消息类型和资源来源。

## 资源下载

公开 `+messages-resource-download` 使用工作目录内安全相对路径，默认不覆盖；完整文件先写入
临时落盘再原子发布。覆盖必须由用户显式传 `--overwrite`，读取和下载不需要 `--yes`。

任务要求从某条消息中定位并下载资源时，优先在限定会话、消息或时间范围的查询中加
`--download-resources --output-dir <目录>`，并检查下载 ledger。`+messages-resource-download`
只用于已经持有完整、真实且属于当前组织/profile 的独立资源引用、无需再定位消息的场景。
若 `fileId` 返回 `RESOURCE_NOT_FOUND`，不得把同一个 ID 改称 `mediaId` 重试，也不得原样
重复调用；应回到消息查询并使用 `--download-resources`。

已有完整资源引用时使用公开 Shortcut：

```bash
dws chat +messages-resource-download --type mediaId --resource-id <mediaId> \
  --message-id <openMessageId> --open-conversation-id <openConversationId> \
  --output ./downloads/
```

原子 `message download-media` 仅用于 Shortcut 未发布的底层字段或原始响应。

`resource-id`、`message-id` 和会话 ID 必须来自同一 profile 下的真实消息查询结果。
当前没有 Range/断点续传；失败时保留 ledger 或错误，显式重试整个文件，不拼接残片。

## 完成与错误

- 查询并下载时同时检查消息完整性和每项下载 ledger；单项失败不抹掉已取得消息。
- 文件/音视频读取失败按上面的“图片读取失败”流程核对路径。
- 位置参数不完整时先向用户确认，不猜经纬度或缩略图。
- 名片发送失败时确认 `--contact-id` 是 openDingTalkId。
- 下载目标存在时默认停止；只有用户明确允许覆盖时才传 `--overwrite`。
