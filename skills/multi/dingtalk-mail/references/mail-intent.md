# 意图判断

## 意图判断

用户说"我的邮箱/邮箱地址" → `mailbox list`（**仅限查询自己的邮箱，不能查他人**）
用户说"获取/查找/得到 某人的邮箱地址" → **不是 `mailbox list`**，走三路并发查询流程（见「查找他人邮箱地址」章节）
用户说"找邮件/搜邮件/查邮件" → `message search`
用户说"看邮件/打开邮件/邮件内容" → 先 `message search` 获取 messageId，再 `message get`
用户说"发邮件/写邮件" → 先 `mailbox list` 获取发件地址，再 `message send`
用户说“给(某人名字)发邮件” / “查询某人发给我的邮件” / “查询发给某人的邮件” / 任何涉及按人名查找邮箱的场景 →
  **第一步**：并发同时发起以下三路查询，取最先返回有效邮箱的结果；若三路均无有效邮箱，请用户提供，禁止臆测：
    1. `aisearch person --keyword <姓名>` → `contact user get --ids <userId>`，提取 `orgAuthEmail`
    2. `mail user search --email <当前邮箱> --keyword <姓名>`，提取 `users[].email`（仅企业邮箱可用；若已知工号，可改用 `--employee-no <工号>`）
    3. `contact user search --query <姓名>`，提取用户邮箱字段
  **第二步**：用获得的目标邮箱拼入 KQL（如 `from:<email>` 或 `to:<email>`）执行 `message search`，或用于 `message send`
用户说"发带附件的邮件/发邮件附件" → 先 `mailbox list` 获取发件地址，再 `message send --attachment <文件路径>`
用户说"给(某人名字)发邮件" → 先 `aisearch person` 获取 userId，再 `contact user get` 获取收件人邮箱，再 `message send`
用户说"查看附件/邮件附件/有什么附件" → 先 `message search` 获取 messageId，再 `attachment list`
用户说"下载附件/保存附件/把附件存到本地/把所有附件下载到..." → 先 `message search` 获取 messageId，再 `attachment list` 获取 attachmentId 和 name，最后逐个 `attachment download`（**不支持批量下载，不存在 download_batch/download_all 命令，必须逐个下载**）
用户说"把XX邮件的所有附件都下载" / "批量下载附件" / "下载4月所有发票邮件的附件" → **不存在批量下载命令**，必须按以下流程循环执行：1) `message search` 搜索匹配邮件获取 messageId 列表 → 2) 对每封邮件 `attachment list` 获取 attachmentId + name → 3) 对每个附件逐个调用 `attachment download`。不要编造 `download_batch` / `download_all` / `batch_download` 等不存在的命令
用户说"创建邮件文件夹/新建邮箱文件夹/新建邮件目录" → `folder create`
用户说"在某个邮件文件夹下创建子文件夹" → 先 `folder list` 找到父文件夹 ID，再 `folder create --folder <folderId>`；禁止把父文件夹名称直接填给 `--folder`
用户说"删除邮件文件夹/删除邮箱文件夹/删除邮件目录" → 先确认要删除的文件夹 ID；如果用户只给名称，先 `folder list` 找到 `folders[].id`，再 `folder delete --id <folderId>`
用户说"重命名邮件文件夹/修改邮箱文件夹名称/更新邮件目录名称" → 先确认要更新的文件夹 ID；如果用户只给原名称，先 `folder list` 找到 `folders[].id`，再 `folder update --id <folderId> --name <新名称>`
用户说"查看邮件标签/列出邮箱标签/查看邮箱 label" → `tag list`
用户说"创建邮件标签/新建邮箱标签/新增 label" → `tag create`
用户说"在某个邮件标签下创建子标签" → 先 `tag list` 找到父标签 ID，再 `tag create --parent-id <tagId>`；禁止把父标签名称直接填给 `--parent-id`
用户说"删除邮件标签/删除邮箱标签/删除 label" → 先确认要删除的标签 ID；如果用户只给名称，先 `tag list` 找到 `tags[].id`，再 `tag delete --id <tagId>`
用户说"重命名邮件标签/修改邮箱标签名称/更新 label 名称" → 先确认要更新的标签 ID；如果用户只给原名称，先 `tag list` 找到 `tags[].id`，再 `tag update --id <tagId> --name <新名称>`
用户说"列出邮件会话/查看会话列表/查看某个文件夹里的邮件会话" → 先确认邮箱地址和文件夹 ID；如果只有文件夹名称，先 `folder list` 找到 `folders[].id`，再 `thread list --folder <folderId>`
用户说"查看会话/获取会话/看这封邮件的会话详情" → 如果已有会话 ID，直接 `thread get`；如果只有邮件线索，先 `message search` 或 `message get` 获取 `conversationId`，再 `thread get`
用户说"标记会话已读/未读/给会话加标签/移除会话标签" → 用 `thread update`；如果是多条会话，用 `thread batch-update`；标签操作必须先有标签 ID
用户说"删除会话/把会话放入已删除/批量删除会话" → 单条用 `thread trash`，多条用 `thread batch-trash`；传入的是会话 ID，不是邮件 ID
用户说"搜索/查找/联系 邮箱用户/联系人/某人的邮箱地址" → `user search`（搜索通讯录人员，不是搜邮件内容）
用户说"发送草稿/把草稿发出去/发这封草稿" → 先 `message search --query "folderId:5"` 找到草稿 messageId，再 `draft send`
用户说"邮件发出去了吗/查邮件发送状态/确认邮件是否发送成功/邮件投递结果" → 用发送类命令返回的 `internetMessageId`，调用 `message verify` 查询 `sendStatus`
用户说"翻页继续搜索联系人/通讯录" → `user search --cursor <nextCursor>`（注意：不是 `message search`）

**`user search` vs `message search` 关键区别：**
- `user search`：搜索的是**人**（通讯录联系人），入参是 `--keyword 姓名` 或 `--employee-no 工号`，返回用户信息
- `message search`：搜索的是**邮件**（邮件内容），入参是 `--query KQL表达式`，返回邮件列表
