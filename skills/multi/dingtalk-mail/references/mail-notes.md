# 注意事项

## 注意事项

- `mailbox list` 返回用户所有邮箱（含个人和企业），每条记录包含邮箱地址、账号类型、所属企业。**默认一律选择企业邮箱**（除非用户明确指定使用个人邮箱）；若有多个企业邮箱可选，优先匹配用户当前所在企业的那一个；仍无法判断时向用户确认后再操作。详见文档顶部「默认邮箱选择规则」章节
- `message search` 返回邮件 ID 和元信息（不含正文），需 `message get` 获取完整内容
- KQL 查询支持 AND/OR/NOT 组合，字段值含空格时需用双引号
- `--cc` 抄送人支持多人，逗号分隔
- 收件人邮箱获取：用户只知道同事名字时，**并发**同时执行以下三路查询，取最先返回有效邮箱的结果，无需等待其他路完成：
  1. `dws aisearch person --keyword "名字" --dimension name` → `dws contact user get --ids <userId>`，提取 `orgAuthEmail`
  2. `dws mail user search --email <发件人邮箱> --keyword "名字"`，提取 `users[].email`（仅企业邮箱账号可调用，个人 @dingtalk.com 邮箱会报权限错误可忽略；若已知工号，可改用 `--employee-no <工号>`）
  3. `dws contact user search --query "名字"`，提取用户邮箱字段
  若三路均无有效邮箱，请用户手动提供收件人邮箱，严禁臆测和假设
- `thread list --folder` 的值必须是文件夹 ID，不是文件夹显示名称；不知道文件夹 ID 时，先调用 `folder list` 查 `folders[].id`
- `thread get/update/trash/batch-update/batch-trash` 使用的是会话 ID（conversationId），不是邮件 ID；会话 ID 可来自 `thread list` 的 `conversations[].id`，也可来自 `message search` 或 `message get` 返回的 `conversationId`
- `thread update` / `thread batch-update` 仅支持 `markRead`、`markUnread`、`addTags`、`removeTags`；标签操作必须传 `--tag-ids`
- `user search` 仅支持企业邮箱（非 `@dingtalk.com` 个人邮箱），使用个人邮箱将因无权限报错；搜到的用户邮箱（`email` 字段）可直接用于 `message send` 的 `--to`/`--cc` 参数
- `folder create --folder` 的值必须是父文件夹 ID，不是文件夹显示名称；不知道父文件夹 ID 时，先调用 `folder list` 查 `folders[].id`
- `folder delete/update --id` 的值必须是目标文件夹 ID，不是文件夹显示名称；不知道目标文件夹 ID 时，先调用 `folder list` 查 `folders[].id`
