# AISearch 低频命令参考

只在 `SKILL.md` 无法覆盖低频枚举、兼容参数或返回字段时读取。普通 `person/enterprise/behavior` 直接按入口 Golden Route 执行，不要为确认已知参数读取本文。

## `person`

```text
dws aisearch person --query <目标> [--dimension <维度>] --format json
```

- `--query` 必填；旧 `--keyword/-w`、`--name`、`--q`、`--text` 仅兼容，不应主动使用。
- `--dimension` 默认 `all`，支持逗号分隔：`all,name,department,position,duty,supervisor,subordinate,phone,jobNumber`。
- 人员结果以服务端返回的 `title/name`、`userId`、`openDingTalkId`、`url` 为证据。完整手机号精确反查不属于本命令。

| 用户线索 | query | dimension |
|---|---|---|
| “五道的上级” | 五道 | `supervisor` |
| “研发工程师职位” | 研发工程师 | `position` |
| “AI 搜问负责人” | AI 搜问 | `duty` |
| “工号 W12345” | W12345 | `jobNumber` |

正确维度的空结果不触发降级搜索；无法判断维度时首次就用 `all`。

## `enterprise`

```text
dws aisearch enterprise [--queries <主题CSV>] [--types <类型CSV>] [--time-range <时间原文>] --format json
```

- `--queries` 只含主题，不含时间、类型和语气词；汇总全部内容时可留空。
- `--types` 默认 `all`，可用 `document,im,calendar,todo,minute,report,image,link,notable,baike,mail`。
- `--time-range` 仅传用户明确给出的“今天/本周/最近/过去一月”等文本，不推测日期。

| 用户类型词 | types |
|---|---|
| 文档、资料、方案、模板 | `document` |
| 消息、聊天记录、群消息 | `im` |
| 邮件、邮箱 | `mail` |
| 日程、会议邀请 | `calendar` |
| 待办、任务 | `todo` |
| 听记、会议纪要、闪记 | `minute` |
| 日志 | `report` |
| 图片、截图 | `image` |
| 链接、URL | `link` |
| AI 表格、多维表 | `notable` |
| 企业百科 | `baike` |

搜索结果常见定位字段是 `title`、`sourceType/sourceName`、`url` 及 `meta` 中的稳定 ID。`snippet` 只是定位预览，不能证明已读取正文。

## `behavior`

```text
dws aisearch behavior [--queries <主题CSV>] [--types <类型CSV>] [--behavior-type <动作>] [--time-range <时间原文>] [--direction <方向>] [--chat-scope <群名>] --format json
```

- `--types` 与 `enterprise` 相同。
- `--behavior-type`：`all,send,receive,create,edit,share`。
- `--direction`：`我->某人`、`某人->我` 或 `我<->某人`；不要使用 `incoming/outgoing/received` 等自造枚举。
- `--chat-scope` 只和 `types=im` 组合，并保留完整群名。

多个类型共享同一动作、方向和时间时可合并为 CSV；动作或方向不同则分别调用。`enterprise` 同理按用户要求的输出组调用：组内类型合并，明确分组则组间分开。

## 结果语义与恢复

- 成功必须以 `success=true` 或等价明确成功字段为证据；`exit 0` 不能覆盖业务错误。
- `result=[]` 只证明当前查询条件下无结果。没有 `complete` 或分页完成字段时，不声称全局全量。
- 目标为精确标题时必须按完整标题消歧；只有近似结果等同未命中目标。
- `retryable=true` 可按完全相同参数重试一次；其他失败不通过换产品、扩大时间或缩词恢复。
- `unknown flag` 才读取一次精确 leaf Help；认证、权限、API 错误和空结果不读 Help。

## 产品边界

- 搜索和行为回溯由 AISearch 负责；不要用 Doc/Drive/Chat/Mail 等逐源替代。
- 已知 `userId` 后查通讯录详情执行 `dws contact user get --ids <userId> --format json`。
- 已知文档、听记、待办等稳定 ID 后读取原对象，才切对应产品。
- 只需列出搜索命中时不加载下游 Skill。
