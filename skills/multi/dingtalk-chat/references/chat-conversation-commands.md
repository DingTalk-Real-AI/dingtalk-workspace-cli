# 会话查询与媒体命令

### list-top-conversations (置顶会话)

#### 拉取置顶会话列表

拉取当前用户的置顶会话列表。分页参数 --limit 指定每页数量，--cursor 传分页游标（首次不传或传 0）。返回结果中 hasMore=true 时用 nextCursor 作为下次 --cursor 继续翻页。
```
Usage:
  dws chat list-top-conversations [flags]
Example:
  dws chat list-top-conversations --limit 1000
  dws chat list-top-conversations --limit 1000 --cursor <nextCursor>
Flags:
      --limit int        每页返回数量（默认 1000）
      --cursor int64     分页游标（首次不传或传 0，翻页传 nextCursor）
      --exclude-muted    是否排除已设置免打扰的会话（默认 false）

注意:
  - 用户询问"置顶会话"时，直接调用此命令返回置顶会话列表即可
  - 用户询问"置顶消息"时，需两步：先调用此命令拉取置顶会话列表获取各会话的 openConversationId，再用 `chat message list --group <openConversationId>` 分别拉取每个会话内的消息
  - 翻页：hasMore=true 时，用返回的 nextCursor 作为下次 --cursor
```

### download-media (下载消息资源)

#### 下载消息中的资源（图片/视频/语音等）到本地

下载聊天消息中的图片、视频、语音等资源到本地文件。流程：先获取下载 URL，再 HTTP GET 下载。
```
Usage:
  dws chat message download-media [flags]
Example:
  dws chat message download-media --type mediaId --resource-id <mediaId> --message-id <openMessageId> --open-conversation-id <openConversationId> --output ./downloads/
  dws chat message download-media --type mediaId --resource-id <mediaId> --message-id <openMessageId> --open-conversation-id <openConversationId> --output ./photo.jpg
Flags:
      --type string                  资源类型: mediaId (必填)
      --resource-id string           资源 ID，mediaId 类型时为消息中的 mediaId 值 (必填)
      --message-id string            消息 openMessageId (必填)
      --open-conversation-id string  会话 openConversationId (必填)
      --output string                本地保存路径，文件或目录 (必填)

注意:
  - resource-id 从 `dws chat message list` 返回的消息内容中获取 mediaId
  - message-id 从 `dws chat message list` 返回的 openMessageId
  - open-conversation-id 从 `dws chat search` 获取 openConversationId
  - --output 如果指定目录，文件名会从下载 URL 中自动推断
```

### search-common (搜索共同群)

#### 搜索共同群 — 查询指定人共同所在的群聊

根据昵称列表搜索共同群聊。--nicks 指定要搜索的人员昵称（逗号分隔，必填）。--match-mode 控制匹配模式：AND 表示所有人都在群里，OR 表示任一人在群里（默认 AND）。分页参数 --limit（默认 20）和 --cursor（默认 "0"）始终传递；hasMore=true 时用返回的 nextCursor 作为下次 --cursor 继续翻页。
```
Usage:
  dws chat search-common [flags]
Example:
  dws chat search-common --nicks "风雷,山乔" --limit 20 --cursor 0
  dws chat search-common --nicks "天鸡,乐函" --match-mode OR --limit 20 --cursor 0
  dws chat search-common --nicks "风雷,山乔,天鸡" --limit 10 --cursor <nextCursor>
Flags:
      --nicks string        要搜索的昵称列表，逗号分隔 (必填)
      --match-mode string   匹配模式：AND=所有人都在群里，OR=任一人在群里（默认 AND）
      --limit int           每页返回数量（默认 20）
      --cursor string       分页游标（默认 "0"，翻页传 nextCursor）
      --exclude-muted       是否排除已设置免打扰的群聊（默认 false）

注意:
  - --nicks 传人员昵称（花名），逗号分隔，如 "风雷,山乔"
  - --match-mode AND 表示群里必须包含所有指定的人；OR 表示包含任意一人即可
  - 翻页：hasMore=true 时，用返回的 nextCursor 作为下次 --cursor
```

### conversation-info (获取会话基础信息)

#### 获取会话基础信息

获取指定会话的基础信息。发送本地文件消息请优先使用 `dws chat message send --msg-type file --file-path <本地文件>`，CLI 不再要求调用方获取或传递 spaceId。
```
Usage:
  dws chat conversation-info [flags]
Example:
  dws chat conversation-info --group <openConversationId> --format json
  dws chat conversation-info --user <userId> --format json
  dws chat conversation-info --open-dingtalk-id <openDingTalkId> --format json
Flags:
      --group string              群聊 openConversationId（群聊时使用）
      --user string               单聊对方 userId（单聊时使用）
      --open-dingtalk-id string   单聊对方 openDingTalkId（单聊时使用）

注意:
  - --group、--user、--open-dingtalk-id 互斥，必须且只能指定其一
  - --group 的别名: --id, --chat, --conversation-id (均可替代 --group)
  - 文件发送不再依赖调用方读取 newCSpaceIdIM；使用 `dws chat message send --msg-type file --file-path <本地文件>` 会自动上传到会话文件空间
```

#### 合并转发多条消息 — 将多条消息合并后转发到目标会话（源/目标会话均支持单聊/群聊）
```
Usage:
  dws chat message combine-forward [flags]
Example:
  dws chat message combine-forward --src-conversation-id <srcOpenCid> --msg-ids <id1>,<id2>,<id3> --dest-conversation-id <destOpenCid>
  dws chat message combine-forward --src-conversation-id <srcOpenCid> --msg-ids <id1>,<id2> --dest-conversation-id <destOpenCid> --uuid <idempotencyKey>
Flags:
      --src-conversation-id string    源会话 openConversationId (必填)
      --msg-ids string                源消息 openMessageId 列表，逗号分隔 (必填)
      --dest-conversation-id string   目标会话 openConversationId (必填)
      --uuid string                   幂等键（可选）

注意:
  - 与 chat message forward 区别: forward 转单条，combine-forward 合并多条为一条转发
  - --msg-ids 多个消息 ID 用逗号分隔，无顺序要求
```

#### 转发话题消息 — 将话题消息从源会话转发到目标会话
```
Usage:
  dws chat message forward-topic [flags]
Example:
  dws chat message forward-topic --src-msg-id <srcOpenMessageId> --src-conversation-id <srcOpenConversationId> --src-thread-id <srcOpenConvThreadId> --dest-conversation-id <destOpenConversationId>
Flags:
      --src-msg-id string               源消息 openMessageId (必填，要转发的消息)
      --src-conversation-id string      源会话 openConversationId (必填，消息所在的会话)
      --src-thread-id string            话题 ID (必填，格式: convThread + 加密后的convThreadId)
      --dest-conversation-id string     目标会话 openConversationId (必填，转发到的会话)

注意:
  - 与 chat message forward 区别: forward 转发普通单条消息，forward-topic 专用于转发话题消息
  - --src-thread-id 格式为 "convThread" + 加密后的 convThreadId，可通过 dws chat message list 返回的话题信息获取
```

#### 钉住某条消息（Pin） — 将指定消息设置为钉住状态
```
Usage:
  dws chat message set-pin-msg [flags]
Example:
  dws chat message set-pin-msg --open-conversation-id <openConversationId> --msg-id <openMessageId>
Flags:
      --open-conversation-id string    (必填)会话 openConversationId（支持群聊/单聊）
      --msg-id string                  (必填)消息 openMessageId

注意:
  - 钉住消息后，会话成员均可在会话中看到被钉住的消息
```

#### 取消钉住某条消息（Unpin） — 取消指定消息的钉住状态
```
Usage:
  dws chat message unset-pin-msg [flags]
Example:
  dws chat message unset-pin-msg --open-conversation-id <openConversationId> --msg-id <openMessageId>
Flags:
      --open-conversation-id string    (必填)会话 openConversationId（支持群聊/单聊）
      --msg-id string                  (必填)消息 openMessageId

注意:
  - 取消钉住后消息仍保留在会话中，只是不再被标记为钉住状态
```

#### 拉取某个会话中钉住的消息列表 — 拉取指定会话中被钉住的消息列表
```
Usage:
  dws chat message list-pin-msg [flags]
Example:
  dws chat message list-pin-msg --open-conversation-id <openConversationId>
  dws chat message list-pin-msg --open-conversation-id <openConversationId> --size 50
  dws chat message list-pin-msg --open-conversation-id <openConversationId> --cursor <nextCursor> --size 20
Flags:
      --open-conversation-id string    (必填)会话 openConversationId（支持群聊/单聊）
      --cursor string   (选填)分页游标，首次不传，翻页时传上次返回的 nextCursor
      --size int        (选填)一次拉取的消息数量（默认 20，最大 100）

注意:
  - 与 `chat message list` 区别: list-pin-msg 只返回被钉住的消息；list 拉取全部消息
  - 分页: hasMore=true 时，用返回的 nextCursor 作为下次 --cursor 继续翻页
```

#### 置顶某条消息 — 将指定消息设置为置顶状态
```
Usage:
  dws chat message set-top-msg [flags]
Example:
  dws chat message set-top-msg --open-conversation-id <openConversationId> --msg-id <openMessageId>
Flags:
      --open-conversation-id string    (必填)会话 openConversationId（支持群聊/单聊）
      --msg-id string                  (必填)消息 openMessageId

注意:
  - 置顶消息后，会话成员可在会话顶部看到该条消息
  - 与 `chat set-top`（会话置顶）不同：set-top-msg 是将会话内某条消息置顶；set-top 是将整个会话在列表中置顶
```

#### 取消置顶某条消息 — 取消指定消息的置顶状态
```
Usage:
  dws chat message unset-top-msg [flags]
Example:
  dws chat message unset-top-msg --open-conversation-id <openConversationId> --msg-id <openMessageId>
Flags:
      --open-conversation-id string    (必填)会话 openConversationId（支持群聊/单聊）
      --msg-id string                  (必填)消息 openMessageId

注意:
  - 取消置顶后消息仍保留在会话中，只是不再被标记为置顶状态
```
