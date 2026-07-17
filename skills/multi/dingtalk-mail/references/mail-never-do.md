# 严格禁止 (NEVER DO)

## 严格禁止 (NEVER DO)
- 明确禁止猜测、假设、推断发件人和收件人邮箱
- 无法获取邮箱时，请用户提供并确认，不要通过假设或其他方式继续执行
- **严禁在用户未明确指定使用个人邮箱时，默认选择 `@dingtalk.com` 个人邮箱作为 `--email` / `--from`**；默认必须从 `mailbox list` 中挑选企业邮箱
- **涉及带附件的邮件操作时，严禁上传到钉钉媒体存储（media upload）**；必须使用对应命令的 `--attachment` / `--inline-attachment` 参数，由 CLI 内部完成附件处理
- **严禁编造不存在的批量下载命令**（如 `attachment download_batch`、`attachment download_all`、`attachment batch_download` 等）。下载附件只有 `attachment download` 一条命令，每次只能下载一个附件；需要批量下载时必须循环调用
- **严禁把文件夹名称当作 `folder create --folder` 的值**；`--folder` 只能填父文件夹 ID，父文件夹 ID 必须来自 `folder list` 的 `folders[].id`
- **严禁把文件夹名称当作 `folder delete/update --id` 的值**；`--id` 只能填要操作的文件夹 ID，文件夹 ID 必须来自 `folder list` 的 `folders[].id` 或 `folder create` 的 `result.folder.id`
- **严禁把标签名称当作 `tag create --parent-id` 的值**；`--parent-id` 只能填父标签 ID，父标签 ID 必须来自 `tag list` 的 `tags[].id`
- **严禁把标签名称当作 `tag delete/update --id` 的值**；`--id` 只能填要操作的标签 ID，标签 ID 必须来自 `tag list` 的 `tags[].id` 或 `tag create` 的 `result.tag.id`
- **严禁更新或删除系统标签**；`tag update/delete` 只适用于用户自定义标签
- **严禁把文件夹名称当作 `thread list --folder` 的值**；`--folder` 只能填文件夹 ID，文件夹 ID 必须来自 `folder list` 的 `folders[].id`
- **严禁把邮件 ID 当作 `thread get/update/trash/batch-update/batch-trash` 的会话 ID**；这些命令需要 conversationId，可来自 `thread list` 的 `conversations[].id` 或邮件结果中的 `conversationId`
