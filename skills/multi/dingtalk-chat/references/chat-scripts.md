# 自动化脚本

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [chat_export_messages.py](../scripts/chat_export_messages.py) | 导出群聊消息到 JSON 文件 | `python chat_export_messages.py --query "项目冲刺" --time "2026-03-10 00:00:00"` |
| [chat_history_with_user.py](../scripts/chat_history_with_user.py) | 查询与某人的单聊聊天记录 | `python chat_history_with_user.py --name "张三" --time "2026-03-10 00:00:00"` |
| [extract_media_id.py](../scripts/extract_media_id.py) | **旧链路** — 仅当上游已通过 `dt_media_upload` 拿到 URL 时使用，从该 URL 提取 mediaId | `python extract_media_id.py "<URL>"`（输出如 @lQLPxxx，用于 `--msg-type image --media-id`；新场景请直接用 `--msg-type file --file-path`） |
