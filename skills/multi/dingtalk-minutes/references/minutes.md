# Minutes 原子命令参考

> 返回入口：[DingTalk Minutes Skill](../SKILL.md)

本文件只在必须使用原子命令、需要确认 URL/参数边界，或 Shortcut 无法表达底层控制时读取。Golden Route 已能完成任务时不要降级为手写多步原子调用。

## Reference 与脚本索引

本页同时是 Minutes 的二级导航页。根 Skill 只链接任务级入口；需要继续下钻时，从这里进入一个最精确的 Reference，不要一次预读多个文件。

| 任务 | Reference |
|---|---|
| ASR、上传恢复、异步生成、录音收尾、批量权限 | [07-minutes.md](07-minutes.md) |
| 发言人/文本替换、媒体下载、离线导出、标签等低频意图 | [intent-guide.md](intent-guide.md) |
| 总结指定发言人的内容并处理匿名发言人候选 | [10-minutes-speaker-match.md](10-minutes-speaker-match.md) |
| 用户确认对应关系后的发言人标注与替换 | [11-minutes-speaker-correct.md](11-minutes-speaker-correct.md) |
| 兼容旧调用方式的轻量 Recipe 索引 | [lite-recipes.md](lite-recipes.md) |
| 旧 Recipe 仍需遵守的分页、ID 与批量约束 | [recipes/conventions.md](recipes/conventions.md) |

### 辅助脚本

以下文件仍随 Skill 交付，所以必须可发现；但它不是 Golden Route。当前 Runtime 有对应 Shortcut 时优先 Shortcut，只有用户明确要求生成本地 Markdown 汇总文件、使用仓库脚本或兼容旧调用方时才运行脚本。

| 文件 | 定位 | 当前边界 |
|---|---|---|
| [minutes_recent_summary.py](../scripts/minutes_recent_summary.py) | 汇总最近若干条“我创建的”听记摘要 | 可直接运行；固定使用 `mine`，不能代替 `+search --scope all` 或具名目标定位 |

脚本的 `--dry-run` 只打印计划且不得调用 DWS；DWS 失败、非法 JSON 与合法空结果必须分开。脚本没有覆盖完整分页、目标消歧与跨 scope 聚合，因此不能用脚本输出声称“全部听记”。行动项读取由 `+action-items`（单条）或 `+detail --ids ... --artifacts todos`（多条）承接，不再发布重复脚本。

命令前缀统一为 `dws minutes`。结构化读取加 `--format json`；参数不确定时查询精确 leaf：

```text
dws schema --cli-path "minutes <group> <leaf>" --compact --format json
dws minutes <group> <leaf> --help
```

只在 Schema 语义/安全不确定时读第一条，只在当前 Cobra flag 不确定时读第二条；不要加载产品级全量 Schema。

## 1. ID 与 URL

`--id` 接受纯 `taskUuid`，不接受完整 URL。用户给听记链接时由 Agent 自动解析，不要求用户手动抠 ID。

| URL 形式 | 提取规则 |
|---|---|
| `https://shanji.dingtalk.com/app/transcribes/<taskUuid>` | 取 `/transcribes/` 后至 `?` 或路径结束的值 |
| `https://shanji.dingtalk.com/meeting/minutes?taskUuid=<taskUuid>` | 读取 `taskUuid` query 参数 |
| 其他包含 `minutesId` 的可信听记链接 | 读取 `minutesId` query 参数，并按 leaf 要求作为 taskUuid 使用 |
| 纯 taskUuid | 直接使用 |

解析失败时停止并说明不识别该链接格式；不把整条 URL 传给 `--id`，不把 URL 当标题关键词搜索。多个 URL 逐一解析，后续始终保持 ID 与标题/组织/时间对应关系。

## 2. 列表与定位

| 原子命令 | 范围 | 分页事实 |
|---|---|---|
| `minutes list mine` | 我创建的 | 单页接口；继续读取必须透传真实 cursor/nextToken |
| `minutes list shared` | 共享给我的 | 单页接口；继续读取必须透传真实 cursor/nextToken |
| `minutes list all` | 后端 noLimit 视图 | 不能仅凭该端点宣称等于完整 `mine + shared` |

完整检索优先用 `+search --page-all` 或 `+list-* --page-all`。这些 Shortcut 使用统一结果信封：业务集合与范围完整性位于 `data.scope/count/minutes/pages/complete`，端点耗尽与续页信息位于 `meta.pagination.endpoint_exhausted/next_token`。原子列表只有一页，必须解析真实 `itemList`，不能把未知响应形态当空数组。

定位规则：

1. 用户给真实 taskUuid/URL：直接使用。
2. 用户给标题/关键词/时间：先搜索。服务端过滤与 Agent 复核应使用同一时间范围和 profile。
3. 精确标题优先；标题包含或语义相关结果可作为候选。零命中停止，多候选、差异较大或分页未完成时消歧。
4. 锁定后所有 get/update 操作复用同一 taskUuid；某项内容为空或失败不能偷偷换对象。

## 3. 读取内容

| 原子命令 | 返回内容 | 重要边界 |
|---|---|---|
| `minutes get info --id <taskUuid>` | 标题、创建/时间、组织、链接等基础信息 | 结果必须能归属于请求 ID |
| `minutes get summary --id <taskUuid>` | AI 摘要/纪要 | 合法空摘要与调用失败分开 |
| `minutes get keywords --id <taskUuid>` | 关键词 | 不从空/未知字段编造关键词 |
| `minutes get transcription --id <taskUuid>` | 单页逐字稿 | 这是单页原子入口；存在下一页时继续传 cursor。需要完整结果优先 `+transcript` |
| `minutes get todos --id <taskUuid>` | 行动项 | 当前响应可能使用 `actions` 或 `dingtalkTodoList`；失败不能伪装成“暂无待办” |
| `minutes get audio --id <taskUuid>` | 临时媒体 URL | URL 敏感且会过期，不长期记录 |
| `minutes get batch --ids <uuid1,uuid2>` | 多条基础详情 | 批量结果逐项对应 ID，缺项不能算全成功 |

完整逐字稿优先 `+transcript`；它跨页去重，业务完整性位于 `data.complete/data.pages`，续页状态位于 `meta.pagination`，分页中断返回失败信封。`+detail` 适合一次读取多种产物；任何所选产物失败都属于 partial，不把 bundle 说成完整。

需要核对多条命中的 basic 时，不要只抽查第一条：

```text
dws minutes +detail --ids <uuid1,uuid2> --artifacts basic --format json
```

结果必须逐项覆盖请求 ID；缺项或失败项如实保留。当前 basic 投影没有逐条 `orgName` 时，应说明归属字段不可得；当前 profile 的 `corpName` 只能证明执行上下文，不能证明每条听记的创建组织或归属。

多听记、多来源或跨产品任务先建立逐来源证据台账：`requested` 记录用户要求的输入，`resolved` 记录已锁定的 `taskUuid`/来源 ID，`missing` 记录未找到的输入，`artifacts` 记录每条实际取得的内容，`status` 记录 `succeeded/partial/failed/unknown`。任一必需来源缺失时整体不能称完整；后续跨产品 Skill 只能接收有来源 ID 的真实产物，不能把已找到的子集写成全部。

内容型结论必须与逐条 artifact 对齐。只有 `title/basic` 时可以列出标题、时间等元数据，不能生成摘要、关键词、主题或内容分类；只有取得该条真实 `summary/transcript/keywords` 后，才能对相应内容做归纳。

## 4. 更新与录音控制

| 原子命令 | 效果 | 当前 confirmation |
|---|---|---|
| `minutes update title` | 修改听记标题 | `not_required`（历史兼容原子入口） |
| `minutes update summary` | 全量覆盖纪要正文 | `not_required`（历史兼容原子入口） |
| `minutes record start` | 发起实时录音 | `not_required`（历史兼容原子入口） |
| `minutes record pause` | 暂停指定 taskUuid | `not_required`（历史兼容原子入口） |
| `minutes record resume` | 恢复指定 taskUuid | `not_required`（历史兼容原子入口） |
| `minutes record stop` | 永久停止指定 taskUuid | `not_required`（历史兼容原子入口） |

标题与纪要推荐分别使用仍执行 `user_required` 门禁的 `+update`、`+summary`，因为它们包含预检/读回验证。原子入口不得作为绕过确认的降级路径。更新纪要时先读取当前完整正文，保留原有 Markdown 图片和用户未要求改变的内容，再写回完整目标内容。

仅预览标题变化时，先读取当前标题，再调用本地计划；`+update --dry-run` 本身不访问远端，所以 `before` 必须来自前一条真实 basic 读取：

```text
dws minutes +detail --id <taskUuid> --artifacts basic --format json
dws minutes +update --id <taskUuid> --title "<目标标题>" --dry-run --format json
```

最终展示 `当前标题 → 目标标题`、`executed=false` 和同一 taskUuid 后即结束。用户说“不实际写入”时不得继续索要写入确认、追加 `--yes`、真实改名或再以还原补救。

录音 start 的成功回执不一定含可控制的 taskUuid。只有响应明确提供 `taskUuid`，并由 Shortcut 返回 `controlReady=true`，才能执行 pause/resume/stop；不能通过“最新听记”或列表第一条猜测绑定。

## 5. 思维导图与发言人

| 原子命令 | 效果 | 当前 confirmation |
|---|---|---|
| `minutes mind-graph create --id <taskUuid>` | 创建思维导图异步任务 | `not_required` |
| `minutes mind-graph status --id <taskUuid>` | 查询任务状态 | `not_required` |
| `minutes speaker replace ...` | 替换逐字稿发言人昵称 | `not_required`（历史兼容原子入口） |
| `minutes speaker summary create --ids <IDs>` | 创建发言人段落总结 | `not_required` |
| `minutes speaker summary get --ids <IDs>` | 查询发言人总结 | `not_required` |

异步 create 后有界轮询；pending/timeout 保留 taskUuid/taskId，恢复只做 status/get。发言人 replace 是昵称替换，不是身份系统中的 speaker_id 重绑；写前必须完整读取逐字稿，确认源昵称存在且目标唯一。

## 6. ASR 热词与文本替换

| 原子命令 | 效果 | 当前 confirmation |
|---|---|---|
| `minutes hot-word list` | 查看个人热词 | `not_required` |
| `minutes hot-word add` | 新增热词 | `not_required` |
| `minutes hot-word delete` | 删除热词 | `not_required`，write/medium（历史兼容原子入口） |
| `minutes replace-text` | 替换一条听记中的文本 | `not_required`（历史兼容原子入口） |

普通“补充热词”优先需要确认的 `+prepare-asr`，它只新增缺失项。只有用户明确要求最终集合完全一致并接受删除多余项时使用 `+sync-asr`；旧 `+prepare-asr --sync` 保持公开以提供迁移提示，但不会调用 MCP。批量文本替换优先仍要求确认的 `+replace-batch`，保留逐项验证和失败 ledger；不得为了绕过确认改用原子 delete/replace。

## 7. Upload session

| 原子命令 | 效果 | 当前 confirmation |
|---|---|---|
| `minutes upload create` | 创建普通上传 session | `not_required` |
| `minutes upload create-and-notify` | 创建 session，并要求上传完成后发送闪记卡片 | `user_required` |
| `minutes upload complete` | 完成已知 session 并生成听记 | `not_required` |
| `minutes upload cancel` | 取消已知 session | `not_required` |

正常本地文件上传优先 `+upload` 或 `+upload-and-notify`，由 Shortcut 管理 create、PUT、complete、取消和读回。原子流程必须保存真实 sessionId/上传地址/taskUuid：

`minutes upload create --enable-message-card` 当前仍是原子命令迁移提示；需要通知时使用 `minutes upload create-and-notify`。Shortcut 层的旧 `+upload --enable-message-card` 和 `+upload-and-analyze --enable-message-card` 则继续执行原有通知语义，并遵循各自通用 Runtime confirmation；新调用优先使用 `+upload-and-notify`。真实执行 `+upload` 和 `+upload-and-analyze` 必须遵循确认门禁，不能因为不发送消息就绕过创建听记的确认。

1. create 或 create-and-notify。
2. 按服务端返回的预签名地址上传文件，不记录该地址。
3. 对同一个 session 执行 complete。
4. 上传前失败可对真实 session 执行 cancel；状态未知先读回，不重新创建。

## 8. 权限

### 8.1 当前能力边界

| 用户要做的事 | 当前公开入口 | 能否完成 | 边界 |
|---|---|---:|---|
| 当前登录用户为自己申请访问 | `minutes permission apply` / `+apply-permission` | 是 | 原子入口保留历史 `not_required`；推荐 `+apply-permission`，要求确认且只支持编辑、查看下载、仅查看 |
| 所有者/管理员给稳定 member UID 授权 | `minutes permission add` / `+share` | 是 | 原子入口保留历史 `not_required` 且支持 policy `0..4`；推荐 `+share`，要求确认且只支持 `edit/download/view` |
| 所有者/管理员撤销稳定 member UID 权限 | `minutes permission remove` / `+unshare` | 是 | 原子入口为历史 write/medium、`not_required`；推荐 `+unshare` 为 write/medium、`user_required` |
| 列出、读取或检查一条听记当前的成员权限 | 无公开 `permission list/get/inspect` 命令 | 否 | 不得用基本信息、写入回执或 dry-run 冒充权限读回 |
| 删除整条听记 | 无公开 Minutes delete 命令 | 否 | `permission remove` 只撤权；`hot-word delete` 只删除个人热词，都不能替代删除听记 |

### 8.2 policy 映射

| `--policy` | 权限 | `permission add` | `permission apply` | Shortcut 语义值 |
|---:|---|---:|---:|---|
| `0` | 管理员 | 支持 | 不支持 | 无；需要时使用原子命令 |
| `1` | 所有者 | 支持 | 不支持 | 无；需要时使用原子命令 |
| `2` | 可编辑 | 支持 | 支持 | `edit` |
| `3` | 可查看/下载 | 支持 | 支持 | `download` |
| `4` | 仅查看 | 支持 | 支持 | `view` |

`permission add` 的 `--policy` 是必填参数，没有默认值；`permission apply --policy` 也必须显式传入，只接受 `2..4`。用户未指定权限类型时先询问，不得把空值解释成 `0`，也不得擅自选择“仅查看”；即使确认选择“仅查看”，命令中仍必须显式传入 `--policy 4`。示例：

```text
dws minutes permission add --ids <uuid1,uuid2> --member-uids <uid1,uid2> --policy 3
dws minutes permission remove --ids <uuid1,uuid2> --member-uids <uid1,uid2>
dws minutes permission apply --id <taskUuid> --policy 4
```

根路径优先用 `+apply-permission`、`+share`、`+unshare` 的语义化参数；完整参数见 [07-minutes.md](07-minutes.md#4-权限-workflow)。只有姓名时先用 Contact/AI Search 解析为同组织稳定 UID，不能把姓名、手机号、userId、openId 或跨组织 UID 直接当作 member UID。

当前没有公开权限读取接口，因此权限写入成功最多证明服务端接受了这次写调用。交付边界按 `verification.mode=write_ack_only`、`verified=false` 表述；这两个值描述证据等级，不代表 Runtime 一定已经返回同名字段。`+share/+unshare` 的逐成员 `complete` ledger 仍只是写入回执，不能声称已读回最终权限状态。

## 9. 标签与语音备忘

| 原子命令 | 用途 |
|---|---|
| `minutes tag list` | 列出当前用户的听记标签，取得真实 tagId |
| `minutes tag query --tag-id <tagId>` | 查询标签下的听记并按真实 token 翻页 |
| `minutes audio-memo list` | 查询语音备忘；具体范围和分页参数读取 leaf Schema |

这些是长尾只读入口，不在根 Skill 展开。tagId 必须来自真实 `tag list`，不能按标签名猜 ID。

## 10. 结果与错误底线

- 成功必须由结构化业务结果与结果契约共同证明，不只看退出码或 `success=true`。
- 列表、逐字稿、批量读取必须保留分页与完整性事实；页数上限、cursor 循环、缺 nextToken 或某页失败都返回不完整/非零。
- 有公开读取接口的写操作完成后按稳定 ID 读回；权限写入当前没有公开读回入口，只能报告 `write_ack_only`、`verified=false`，未知结果不得重放整个请求。
- 权限、上传、异步任务和批量替换必须保留部分成功 ledger 与恢复句柄。
- Runtime confirmation 与 compact Schema 是最终权威；若本文与当前 leaf Schema 冲突，使用更安全解释并报告 contract drift。
