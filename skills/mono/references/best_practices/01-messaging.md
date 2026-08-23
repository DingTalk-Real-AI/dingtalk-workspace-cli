# 消息沟通

> lite recipe 见 [SKILL.md 速查表](../../SKILL.md)。

| Recipe | 行动指南（固定路线） |
|--------|-------------------|
| query-group-chat | `dws chat +chat-messages --group "<群名>" --time "<yyyy-MM-dd HH:mm:ss>" --page-all --output ./exports/messages.json --format json`；只接受唯一解析结果，并检查 `complete/hasMore/failures`。 |
| query-private-chat | 先解析唯一用户 ID，再执行 `dws chat +chat-messages --user "<userId>" --time "<yyyy-MM-dd HH:mm:ss>" --page-all --format json`；零命中/多候选停止消歧。 |
| escalate-ding | 三级升级：<br>1. `ding message send --robot-code <robotCode> --type app --users <userId> --content "<内容>"`（必填项见 [ding.md](../products/ding.md)）<br>2. `dws chat +messages-send --as user --group <openConversationId> --text "<内容>" --format json` 群里提醒；Runtime 会在缺少确认时停止，必须先完成收件人、内容与影响确认，再以相同参数追加 `--yes`<br>3. `todo task create --title "<标题>" --executors <userId> --priority 40` 建紧急待办<br>所有稳定 ID 来自同一 profile 的真实返回，不选搜索第一项。 |
| send-by-bot | 单群用 `--group`；多群用 `dws chat +messages-send --as bot --robot-code <robotCode> --groups <cid1,cid2> --title "<标题>" --text "<内容>" --format json`。Runtime 要求先确认再追加 `--yes`，并去重、返回 `im.batch-write.v1` 逐目标 ledger；未知投递状态时停止重发。 |
| forward-message | 1. `dws chat +chat-messages --group "<源群名>" --format json` 取得唯一源会话与真实 `openMessageId`<br>2. 从真实返回或唯一解析取得目标 `openConversationId`<br>3. `dws chat +messages-forward --src-conversation-id <srcCid> --msg-id <openMessageId> --dest-conversation-id <destCid> --format json`；Runtime 会先停止并要求确认，确认后以相同参数追加 `--yes` 执行并保留幂等键。若用户只是要转述内容而非转发原消息，改走 `+dm`/`+send-to-group`。 |
| search-common-group | `chat search-common --nicks "<昵称1>,<昵称2>" --limit 20 --cursor 0`（`--match-mode AND`=全在/`OR`=任一在，翻页：`hasMore=true` 时用 `nextCursor`）<br>用户说"我和XX的共同群" → nicks 包含"我"时，需先 `contact user get-self` 取自己昵称再拼接 |
| focus-messages | **零参数一行命令**：`chat message list-focused --limit 50`（拉特别关注人发的消息聚合）<br>触发 query：`"我特别关注的人最近发了什么消息"`、`"关注的人最近聊了啥"`、`"星标联系人最近的动态"`<br>**强消歧**：query 含动词【发/说/聊/讲】或名词【消息/聊天/动态】 → **必须**走本命令，**不要**先去拉 `contact relation list-my-followings`；仅当用户终点是"人员列表"（如"我关注了谁"）才走 `relation list-my-followings`（详见 [contact.md](../products/contact.md#意图判断) 易混淆硬规则）<br>翻页：`hasMore=true` 时用 `nextCursor` 作为下次 `--cursor`<br>按人精控（可选）：先 `contact relation list-my-followings` 取 `openDingTalkId`，再 `chat message list-by-sender --sender-open-dingtalk-id <openDingTalkId> --start <ISO> --end <ISO>` |
