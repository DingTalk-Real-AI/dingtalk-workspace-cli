---
name: dingtalk-mail
description: 钉钉邮箱读写、搜索、回复与转发。Use when 用户说发邮件/查邮件/回邮件/转发邮件/未读邮件/邮件搜索/邮箱附件。一句话发邮件时先用 dingtalk-contact 解析并确认收件人，再由本 skill 发送；不做钉钉消息（走 dingtalk-chat）、紧急通知（走 dingtalk-misc）。命令前缀：dws mail。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉邮箱 Skill

## 前置条件 — 执行操作前必读

> **CRITICAL — 执行任何 `dws` 操作前，MUST 先用 Read 工具完整读取 [`dingtalk-shared`](../dingtalk-shared/SKILL.md)。**该轻量文件包含全局执行契约、安全底线及 shared references 的按需加载导航；不要预加载其全部 references。

> 命令参考：[mail.md](references/mail.md)。复杂搜索、附件、批量处理、草稿等多步邮件场景参考：[09-mail.md](references/09-mail.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 Agent leaf Schema（例如 `dws schema --cli-path "mail +<shortcut>" --compact --format json`），在当前 Cobra flags 不确定时读取 `dws mail <shortcut> --help`。只有参数映射、接口绑定或 provenance 审计才省略 `--compact`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service mail --format json` 批量发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws mail +find-mail-user` | read | 按关键词搜索邮箱联系人并投影列表（姓名/昵称/邮箱/工号等） |
| `dws mail +folder-list` | read | 列出顶层文件夹或指定父文件夹下的子文件夹 |
| `dws mail +message` | read | 读取一封邮件的完整正文与附件元数据 |
| `dws mail +messages` | read | 按请求顺序读取多封邮件并逐封验证身份 |
| `dws mail +search-mail` | read | 按 KQL 关键词搜索邮件并投影列表（主题/发件人/时间/messageId） |
| `dws mail +thread` | read | 读取完整邮件会话并精确验证 conversationId |
| `dws mail +triage` | read | 列出或筛选邮件摘要，自动解析邮箱与收件箱 |
| `dws mail +user-search` | read | 按关键词或工号搜索邮箱用户（仅企业邮箱） |
<!-- VISIBLE_SHORTCUTS_END -->

## 意图表

| 用户说 | 命令 |
|--------|------|
| "发邮件给 a@b.com" | `dws mail mailbox list --format json` → `dws mail message send --from <邮箱> --to a@b.com --subject "<标题>" --content "<正文>" --format json` |
| "回复 / 全部回复 / 转发" | `dws mail message reply` / `reply-all` / `forward` |
| "今天未读邮件" | `python scripts/mail_unread_summary.py` |
| "带抄送发送" | `python scripts/mail_send_with_cc.py --to a@b.com --cc c@d.com --subject "<标题>" --body "<正文>"` |

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 email/messageId。每条命令必须带 `--format json`。收件邮箱**必须**真实解析，**禁止**猜测。

### SOP-1 拿邮箱（get-mailbox）

**触发**：我的邮箱/发件需要邮箱/查邮件需要邮箱。

1. **执行（必须）**：`dws mail mailbox list --format json`，取自己的 `email`（默认选企业邮箱 `type:ORG`）；查**他人**邮箱用 `dws mail user search --keyword "<姓名>" --format json`，**禁止**用 `mailbox list` 查他人。

**禁止**：把 `mailbox list` 当作他人邮箱查询、猜测邮箱地址。

### SOP-2 查 / 搜邮件（search-mail）

**触发**：查邮件/搜邮件/某主题邮件/某人发的邮件。

1. **拿邮箱（必须）**：先按 SOP-1 取 `email`；用户已明确提供可跳过。
2. **执行（必须）**：浏览文件夹 `dws mail message list --email <邮箱> --limit <n> --format json`；KQL 搜索 `dws mail message search --email <邮箱> --query "<KQL>" --limit 20 --format json`（KQL 如 `subject:周报`、`from:alice@x.com`、`folderId:2`、`hasAttachments:true`，**只通过 `--query` 传**）。
3. **取正文（必须）**：`dws mail message get --email <邮箱> --id <messageId> --format json`；`messageId` 从列表/搜索结果取，**禁止**编造。

**禁止**：把 KQL 拆成多个 flag、跳过 `message list/search` 直接猜 messageId。

### SOP-3 发邮件（send-mail）

**触发**：发邮件/写邮件/群发。

1. **发件邮箱（必须）**：`dws mail mailbox list` 取自己邮箱。
2. **收件邮箱（必须）**：地址直接用；姓名按 [mail.md](references/mail.md) "查找他人邮箱地址"流程（`mail user search` 等）获取，**禁止**猜测。
3. **执行（必须）**：`dws mail message send --from <发件邮箱> --to <收件邮箱> --subject "<主题>" --content "<正文>" --format json`；按需 `--cc`/`--attachment`/`--inline-attachment`。
4. **验证（必须）**：从发送返回取真实 `internetMessageId`，执行 `dws mail message verify --email <发件邮箱> --internet-message-id <internetMessageId> --format json` 查发送状态；不要把普通 `messageId` 传给 verify。

**禁止**：猜测收件邮箱、发送后不确认状态就答复"已发送"。

### SOP-4 回复 / 转发（reply-forward）

**触发**：回复邮件/回复全部/转发。

1. **拿邮箱 + 原邮件（必须）**：SOP-1 取邮箱；用户未给 `messageId` 时**必须**先走 SOP-2 定位原邮件 `messageId`。
2. **执行（必须）**：回复 `dws mail message reply --from <邮箱> --id <messageId> --content "<正文>" --format json`；回复全部用 `reply-all`；转发 `dws mail message forward --from <邮箱> --to <收件邮箱> --id <messageId> --content "<附言>" --format json`。

**禁止**：未定位原邮件就回复/转发、编造 messageId。

## 高频硬约束

- 用户要"完整内容/看看这封邮件/正文"时，`message search` 命中后必须继续调用 `dws mail message get --email <邮箱> --id <messageId> --format json`；不要只列候选后停下。
- 搜到多封邮件时，若用户给了明确主题、附件名、发件人或时间线索，先选最匹配的一封执行 `message get`；只有同等候选无法判断时才询问用户。
- 附件链路固定三步：`message search` → `attachment list --email <邮箱> --id <messageId>` → `attachment download --email <邮箱> --message-id <messageId> --attachment-id <attachmentId> --name <文件名>`；不存在批量下载命令。
- 写入类操作（发送、回复、转发、删除、批量移动）按安全策略确认；只读查看、搜索、附件列表、下载不需要确认。
- 所有 `dws mail` 命令加 `--format json`，并复用同一封邮件的 `messageId`，不要重新搜索导致目标漂移。

## 轨迹高频直达规则

- 已被本页或 `09-mail.md` 精确覆盖的命令，直接执行；不要先探测 `contact`、`api`、`auth`、`profile`、Help 或 Schema。仅在命令真实报参数错误后，读取该命令的 leaf Help 一次。
- 多条件 KQL 必须用显式 `AND`，例如收件箱附件邮件：`hasAttachments:true AND folderId:2`；不要把相邻条件当作隐式 AND。
- 用户只要首个或任一单附件时，初始搜索后按相关性最多检查 3 个候选的 `attachment list`，下载命中附件并做一次本地存在性/大小检查后停止；若均未命中，只能说明已检查范围并询问是否继续翻页，不能断言不存在。用户要求全部/批量附件时，遍历全部匹配页和邮件，再逐个下载每个附件。
- “创建邮箱联系人”属于邮箱个人通讯录，固定走 `dws mail contact create/list`，不要切到 `dingtalk-contact`。只有“按人名解析邮件收件地址”才走下方跨产品协作。
- 用户要求用当前时间生成唯一标题且未指定显示格式时，使用紧凑格式 `date +%H%M%S`；只有用户明确要求 `HH:mm:ss` 等格式时才原样保留。
- 批量删除邮件或将整会话移入已删除前，先展示精确目标和不含确认参数的命令预览并停止；用户明确确认后，执行流程仅对同一组参数追加 `--yes`。操作成功后只做一次针对原 ID 或唯一主题的回读，结果符合预期即停止。
- 创建类返回了 `messageId`、`contactId` 或 `internetMessageId` 时，后续直接复用返回 ID；不要为了重新定位目标而做宽泛 list/search。
- 一次精确回读已经证明目标状态后结束任务；若回读冲突，报告冲突和已执行命令，不扩大到低层 API 或其他产品继续试探。

## 跨产品协作

- 收件人是人名 → 先用 `dingtalk-contact` 取 `orgAuthEmail`
- 钉钉内消息 → 切到 `dingtalk-chat`
## 局部意图与短流程

- [局部意图消歧](references/intent-guide.md)；[短流程](references/lite-recipes.md)。
