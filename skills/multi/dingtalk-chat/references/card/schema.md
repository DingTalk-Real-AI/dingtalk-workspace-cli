# 卡片 Schema

DWS 公开两类卡片命令契约：

- `chat +messages-send-card` / `chat +messages-update-card`：仍是 `im.streaming-card.v1`
  Shortcut 工作流，只处理 streaming text。
- `chat message send-card` / `chat message update-card`：streaming 原子命令。
- `chat message send-a2ui-card` / `chat message update-a2ui-card`：A2UI 原子命令。

streaming 不是任意组件 Schema：

- target：group、direct user、direct openDingTalkId；
- content：streaming text；
- lifecycle：create 可选串联 update，后续按 `bizId` update；
- flowStatus：1–5；
- callback：不支持。

`update-card --flow-status` 的 CLI 类型为 string，但仍只接受兼容数字 1–5
（包括历史 pflag int 支持的 base-0 写法），并向 streaming RPC 发送 integer。

A2UI 原子命令规则：

- `send-a2ui-card` 调用 `im.create_and_send_a2ui_card`。
- `update-a2ui-card` 调用 `im.update_a2ui_card`。
- `--content` 必须是 JSON 字符串数组，元素为 A2UI 协议 JSON，例如
  `'["{\"version\":\"v1.0\",\"updateDataModel\":{\"surfaceId\":\"surface\",\"path\":\"/status\",\"value\":\"finished\"}}"]'`；
  CLI 解析为 `a2uiMessages`，send-card 额外生成 `summary`，值为数组元素按换行拼接
  （真实 MCP 契约无 `fallbackText` 字段）。
- 群聊目标写入顶层 `openConversationId`；单聊目标写入顶层 `receiverOpenDingTalkId`
  （传入 userId 时由 CLI 自动解析转换为 openDingTalkId）。
- A2UI wire 层 `flowStatus` 为字符串枚举；CLI `--flow-status` 接受枚举名和兼容数字 1–9：
  1 PROCESSING、2 INPUTTING、3 FINISH、4 EXECUTING、5 ERROR、6 ABORTED、
  7 TIMEOUT、8 CONFIRMING、9 CONFIRMED，由 CLI 映射为枚举字符串发送。
- A2UI `send-a2ui-card` 在 CLI 侧自动生成 `requestId`、`bizCardId`，并固定
  `protocolVersion="1.0"`；创建默认 `flowStatus=PROCESSING`。
- A2UI `update-a2ui-card` 固定附带 `a2uiAnnotations: []`。
- A2UI 命令不提供 `--at-open-dingtalk-ids` / `--at-all`；@ 仅 streaming
  `send-card` 群聊可用。

参数、required 和 confirmation 读取
`dws schema --cli-path "chat +messages-send-card" --compact -f json` 或
`dws schema --cli-path "chat message send-a2ui-card" --compact -f json`。不要把 Lark card JSON 字段翻译成
未发布的 DWS flags；A2UI 内容复用 `--content`，不要发明 `--a2ui-content`。
