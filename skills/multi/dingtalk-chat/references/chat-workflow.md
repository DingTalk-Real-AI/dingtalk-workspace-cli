# 核心工作流

## 核心工作流

```bash
# 1. 搜索群 — 提取 openconversation_id
dws chat search --query "项目冲刺" --format json

# 2. 拉取群消息
dws chat message list --group <openconversation_id> --time "2025-03-01 00:00:00" --format json

# 2b. 拉取未读会话列表
dws chat message list-unread-conversations --count 20 --format json

# 3. 以个人身份发送群消息
dws chat message send --group <openconversation_id> --title "周报提醒" "请大家本周五前提交周报" --format json

# 4. 以个人身份单聊（通过 userId）
dws chat message send --user <userId> "你好" --format json

# 4b. 以个人身份单聊（通过 openDingTalkId，三方应用等无法获取 userId 时使用）
dws chat message send --open-dingtalk-id <openDingTalkId> "你好" --format json

# 5. 机器人发群消息（Markdown）
dws chat message send-by-bot --robot-code <robot-code> \
  --group <openconversation_id> --title "日报" --text "## 今日完成..." --format json

# 6. 机器人单聊发消息
dws chat message send-by-bot --robot-code <robot-code> \
  --users userId1,userId2 --title "提醒" --text "请提交周报" --format json

# 7. Webhook 发告警
dws chat message send-by-webhook --token <webhook-token> \
  --title "告警" --text "CPU 超 90% @10" --at-all --format json
```
