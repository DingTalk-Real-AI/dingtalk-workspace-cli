# topic：话题圈操作

> 返回入口：[DingTalk Chat Skill](../../SKILL.md)

用于创建话题圈、发布和浏览话题、向具体话题追加回复、读取回复以及转发整条话题。Topic 容器使用 `openTopicId`，容器内一条具体话题使用 `openConvThreadId`。

## 入口选择

| 用户终点 | 推荐入口 |
|---|---|
| 已有稳定成员 ID 创建话题圈 | `dws chat topic create` |
| 在话题圈中发布新话题 | `dws chat topic send` |
| 浏览话题圈中的话题主消息 | `dws chat topic list` |
| 向具体话题直接追加回复 | `dws chat topic reply` |
| 已有完整 Topic 标识，分页读取一页回复 | `dws chat topic list-replies` |
| 转发整条话题 | `dws chat topic forward` |

## 回复与读取

`topic reply` 只向指定 `openConvThreadId` 追加回复，不使用消息引用回复，也不创建新的顶层话题。

已有 `openTopicId/openConvThreadId` 且只需读取一页时，直接使用原子命令 `topic list-replies`。需要按主消息自动解析、全量翻页、排序或下载资源时，继续使用现有的 `+thread-replies` Shortcut。

## 整条转发

`topic forward` 使用源消息、源 `openTopicId/openConvThreadId` 和目标会话 `openConversationId` 转发整条话题。

## 完成与错误

- 创建结果对外使用 `openTopicId`，话题列表保留每条话题的 `openConvThreadId`。
- 发布和回复沿用异步发送结果；`openTaskId` 是任务 ID，不是话题或消息 ID。
- 回复完成后必须能从同一 `openConvThreadId` 回读；不能把引用消息产生的新顶层话题当作成功。
- 标识缺失、类型不明或不属于同一 Topic 上下文时停止，不猜 ID。
