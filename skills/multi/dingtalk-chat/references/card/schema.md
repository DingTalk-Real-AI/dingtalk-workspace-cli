# 流式卡片 Schema

DWS 当前公开的是 `im.streaming-card.v1` 工作流契约，不是任意组件 Schema：

- target：group、direct user、direct openDingTalkId；
- content：streaming text；
- lifecycle：create 可选串联 update，后续按 `bizId` update；
- flowStatus：1–5；
- callback：不支持。

参数、required 和 confirmation 读取
`dws schema --cli-path "chat +messages-send-card" --compact -f json` 或
`dws schema --cli-path "chat +messages-update-card" --compact -f json`。不要把 Lark card JSON 字段翻译成
未发布的 DWS flags。
