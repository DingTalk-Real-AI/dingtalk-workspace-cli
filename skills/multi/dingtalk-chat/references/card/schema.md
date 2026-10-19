# 卡片 Schema

按内容契约和创建记录选择卡片；A2UI 格式本身不代表需要流式。回复占位与流式、主动完整发送的边界见 [A2UI 指南](a2ui.md)。

- 用户明确要求 A2UI，需要组件布局、数据绑定，或按上述回复流程展示占位和进度时，选用 A2UI；只有正文也可使用 A2UI。发送或更新前先读取上述指南。
- 未指定 A2UI、只需现有文本卡片的正文和流转状态时，沿用[创建流式卡片](create.md)。
- 更新时沿用创建接口：`send-a2ui-card` 对应 `update-a2ui-card`；`send-card` / `+messages-send-card` 对应 `+messages-update-card`。保存真实 `bizId` 及创建命令，不根据本批是否只有文本切换接口；只有 `bizId` 时先核实创建记录。

这两个创建入口均不提供模板 ID 参数。用户指定模板 ID 和模板变量时，先核实可用的模板卡片接口，不把模板 ID 传给 `send-card`，也不根据模板 ID 是否出现判断卡片类型。A2UI 与群聊 @ 的要求同时出现时说明当前参数限制，不自动改成文本卡片。

DWS 公开两类卡片命令契约：

- `chat +messages-send-card` / `chat +messages-update-card`：仍是 `im.streaming-card.v1`
  Shortcut 工作流，只处理 streaming text。
- `chat message send-card` / `chat message update-card`：仅保留 Shortcut 未发布字段或原始响应的 streaming 原子 fallback。
- `chat message send-a2ui-card` / `chat message update-a2ui-card`：A2UI 原子命令。

streaming 不是任意组件 Schema：

- target：group、direct user、direct openDingTalkId；
- content：streaming text；
- lifecycle：create 可选串联 update，后续按 `bizId` update；
- flowStatus：1–5；
- server callback：卡片命令不提供 callback URL 注册、验签或回复；监听当前用户的互动卡片
  操作事件走 [`dingtalk-event`](../../../dingtalk-event/SKILL.md)：
  `dws event consume user_card_action_triggered --flatten -f ndjson`。

`update-card --flow-status` 的 CLI 类型为 string，但仍只接受兼容数字 1–5
（包括历史 pflag int 支持的 base-0 写法），并向 streaming RPC 发送 integer。

下方 `updateDataModel` 片段只演示已有 surface 的更新格式，完整创建示例见上述 A2UI 指南。

A2UI 原子命令规则：

- `send-a2ui-card` 调用 `im.create_and_send_a2ui_card`。
- `update-a2ui-card` 调用 `im.update_a2ui_card`。
- `--content` 必须是 JSON 字符串数组，元素为 A2UI 协议 JSON，例如
  `'["{\"version\":\"v1.0\",\"updateDataModel\":{\"surfaceId\":\"surface\",\"path\":\"/status\",\"value\":\"finished\"}}"]'`；
  CLI 解析为 `a2uiMessages`，send-a2ui-card 额外生成 `summary`，值为数组元素按换行拼接
  （真实 MCP 契约无 `fallbackText` 字段）。
- 群聊目标写入顶层 `openConversationId`；单聊目标写入顶层 `receiverOpenDingTalkId`
  （传入 userId 时由 CLI 自动解析转换为 openDingTalkId）。
- A2UI wire 层 `flowStatus` 为字符串枚举；CLI `--flow-status` 接受枚举名和兼容数字 1–9：
  1 PROCESSING、2 INPUTTING、3 FINISH、4 EXECUTING、5 ERROR、6 ABORTED、
  7 TIMEOUT、8 CONFIRMING、9 CONFIRMED，由 CLI 映射为枚举字符串发送。
- A2UI `send-a2ui-card` 在 CLI 侧自动生成 `requestId`、`bizCardId`，并固定
  `protocolVersion="1.0"`；创建默认 `flowStatus=PROCESSING`。
- 两个 A2UI 命令均支持可选 `--a2ui-annotations`，接受 JSON 对象数组并透传为
  `a2uiAnnotations`。省略时创建请求不携带该字段，更新请求保持发送 `[]`。
- A2UI 命令不提供 `--at-open-dingtalk-ids` / `--at-all`；@ 仅 streaming
  `send-card` 群聊可用。

参数、required 和 confirmation 读取
`dws schema --cli-path "chat +messages-send-card" --compact -f json` 或
`dws schema --cli-path "chat message send-a2ui-card" --compact -f json`。不要把 Lark card JSON 字段翻译成
未发布的 DWS flags；A2UI 内容复用 `--content`，不要发明 `--a2ui-content`。
