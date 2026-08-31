---
name: dingtalk-minutes
description: 钉钉 AI 听记。Use when 查询或修改听记摘要、完整逐字稿、关键词、行动项、录音、上传、思维导图、发言人洞察或分享权限。写文档走 dingtalk-doc；建待办走 dingtalk-todo；日程走 dingtalk-calendar。命令前缀：dws minutes。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉 AI 听记 Skill

<!-- DWS_RUNTIME_CONTRACT_START -->
## 最小 DWS 执行契约

- 只通过 `dws` CLI 操作钉钉；结构化读取使用 `--format json`，按真实返回判断结果。
- 已知命令直接执行。只有 leaf 参数或安全语义不确定时读取精确 Schema，只有 Cobra flag 不确定时读取精确 leaf Help；不要加载产品级 Catalog 代替选路。
- 不猜命令、flag、字段、ID、账号或时间。后续 ID 必须来自真实返回；零命中、多候选或类型不明时停止并消歧。
- 解析目标、读取上下文和最终执行必须使用同一 profile；不得跨组织复用 userId、openDingTalkId 或 openConversationId。多账号组织只使用明确的 `isOrgCurrent=true` 默认账号；没有默认账号时要求用户指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出或记录 token、refresh token、appSecret、webhook token 等凭据；宿主已注入认证时不要索要凭据。
- 写操作必须符合用户明确意图。是否需要确认以最终 Runtime gate 和 Schema 为准；需要确认时先说明对象、动作与影响，再追加 `--yes`。
- 写后按任务结果契约验证；不能仅凭退出码宣称成功。部分结果、未知投递状态和失败项必须如实保留。
- 时间戳面向用户展示时转换为带时区的可读时间；默认使用当前会话时区，必要时同时保留原值。
- 遇到认证、权限、profile、confirmation 或未知错误时，只加载 `dingtalk-shared` 中对应 reference；不要连续猜测替代命令。
<!-- DWS_RUNTIME_CONTRACT_END -->

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcut 发现（按需）

`minutes` 当前有 29 条公开 shortcut，完整清单保留在 Runtime Catalog 与 Schema，不在高频产品根 Skill 中重复展开。已知意图直接使用下方的优先路由、意图表或任务 reference；命令已选中时直接执行，只在参数/安全语义不确定时读取 leaf Schema，在当前 Cobra flags 不确定时读取 leaf Help。

仅当现有路由和 reference 都无法定位低频能力时，才执行 `dws shortcut list --service minutes --format json` 做最后回退；不要为已知高频意图加载完整 Shortcut Catalog 或产品级 Schema。
<!-- VISIBLE_SHORTCUTS_END -->

## Golden Route

以下是当前 Minutes Case 支持的核心路径，用于减少 Agent 选路分叉；它不等同于生产使用频率统计。已有 `taskUuid` 直接使用；完整 `shanji.dingtalk.com` URL 先提取其中的真实 ID。只有标题或时间线索时先搜索，零命中停止，多候选或候选差异较大时让用户消歧，不默认取第一条。

| 用户意图 | 唯一推荐入口 | 关键边界 |
|---|---|---|
| 按标题或时间找听记 | `dws minutes +search --scope all --query "<关键词>" --page-all` | `scope` 可选 `mine/shared/all`；至少提供 query/start/end 之一。`all --page-all` 分别追完 mine/shared 后去重，不把单个 noLimit 端点当完整全集 |
| 浏览我创建、共享给我或全部可访问听记 | `+list-mine` / `+list-shared` / `+list-all` | 默认是可续拉预览；要声称完整必须加 `--page-all` 并检查 `complete=true`。用户只说“我的听记”不等于明确 `mine`，范围不清时用 `all` |
| 看我最新创建的一条 | `dws minutes +latest [--keyword <关键词>]` | 只在用户明确说“最新”时用；不能用它替代具名目标搜索，也不能在录音 start 后拿 latest 猜新录音 ID |
| 读取基础信息、摘要或关键词 | `dws minutes +detail --id <taskUuid> --artifacts basic,summary,keywords` | 已有 taskUuid 且只读取现有产物时直接使用，不要进入上传 workflow；任一产物失败都按 partial/非零处理，不把缺失项说成空内容 |
| 读取逐字稿 | `dws minutes +transcript --id <taskUuid> [--direction 1] [--single-page]` | 已有 taskUuid 且只读取逐字稿时直接使用，不要借 `+upload-and-analyze --resume-id` 代读。默认正序并追完分页；倒序必须传 `--direction 1`，用户明确只要第一页时传 `--single-page`，不得随后自动续页。交付前检查 `data.direction/data.complete/data.pages` 与 `meta.pagination` |
| 读取行动项 | `dws minutes +action-items --id <taskUuid>` | 只有受支持字段明确返回空数组才能说“没有待办”；`unsupported_shape`、字段解析失败或工具失败都不是空结果。需要创建钉钉待办时再切 `dingtalk-todo` |
| 把摘要、关键词、完整逐字稿和行动项归档到本地 | `dws minutes +export-pack --id <taskUuid> --output <新相对目录>` | 要带媒体时加 `--include-media`；必须由 `published/path/manifest/files` 证明落盘，只有建目录、计划或文件名不能称已生成 |
| 修改或预览标题 | `dws minutes +update --id <taskUuid> --title "<新标题>"` | 真实修改按 Runtime confirmation 执行并读回；用户只要预览时先读 basic，再加 `--dry-run`，展示“当前值 → 目标值”后停止，不追加 `--yes` |
| 覆盖纪要正文 | `dws minutes +summary --id <taskUuid> --content @<相对文件>` | `content` 是完整目标正文，不是局部 patch；按 Runtime confirmation 执行，并保护图片引用、读回全文 |
| 上传音视频生成听记 | `dws minutes +upload --file <相对路径>` | 真实执行会上传文件并创建远端听记，必须按 Runtime confirmation；用户明确要闪记卡片时改用 `+upload-and-notify`，需要上传后等待分析产物时用 `+upload-and-analyze` |
| 真实开始、暂停、继续或停止录音 | `+record-start` / `+record-pause` / `+record-resume` / `+record-stop` | 这组入口会真实执行。start 返回 `accepted=true, bound=false` 或 `controlReady=false` 时，报告“已受理但未绑定”并停止：不得重试 start，也不得用 `+latest`、列表第一条或时间最近项猜 ID。结束并等待产物用 `+record-wrap-up` |
| 只预览录音请求，不实际执行 | `dws minutes record start --dry-run --format json` | 使用对应的原子 `minutes record start|pause|resume|stop` leaf；start 的 `--session-id` 可选，pause/resume/stop 必须传真实 `--id`。不要把被拒绝的 Shortcut dry-run 描述成预览成功 |
| 生成或继续思维导图 | `dws minutes +mindmap --id <taskUuid>` | 创建后有界轮询；超时或未知状态保留恢复信息，用 `--resume` 继续，不重复创建 |
| 生成或继续发言人洞察 | `dws minutes +speaker-insights --id <taskUuid>` | 有界轮询；保留 `taskId`，恢复时用 `--resume [--task-id <ID>]`，不重复创建 |
| 当前用户申请查看/下载/编辑权限 | `dws minutes +apply-permission --id <taskUuid> --permission view|download|edit` | 这是“我申请访问”，不是所有者给别人授权；按 Runtime confirmation 执行 |
| 所有者给成员授权或撤权 | `dws minutes +share ...` / `dws minutes +unshare ...` | 先用通讯录把姓名解析为同组织稳定 UID；撤权是破坏性操作。批量结果必须保留逐成员 ledger 和失败项 |

### 搜索与列表执行胶囊

用户要求“全部、所有、完整、汇总整个范围”时，首轮直接使用 `--page-all`：

```text
dws minutes +search --query "<关键词>" --scope all --page-all --format json
dws minutes +search --start "<RFC3339>" --end "<RFC3339>" --scope mine --page-all --format json
dws minutes +list-mine --page-all --format json
dws minutes +list-shared --page-all --format json
dws minutes +list-all --page-all --format json
```

只有用户明确要第一页、预览或样本时才省略 `--page-all`，并如实保留 `data.complete=false` 与 `meta.pagination.next_token`。有时间窗必须使用 `+search`，因为 `+list-*` 不接受 `--start/--end`。

## 目标与完整性

- 目标锁定优先级：用户给出的 `taskUuid`/URL > 精确标题 > 标题包含或语义相关候选。相似候选可展示，但候选差异明显或多个候选都合理时必须停下来消歧；分页未完成本身不是目标歧义。
- 用户说“先确认/核对目标”默认要求用真实 basic 字段完成证据核对，不自动变成等待用户回复的会话门禁。目标唯一、分页完整且后续均为只读时，展示标题、时间、归属等核对证据后在同一轮继续；只有用户明确要求“等我确认后再继续”或仍有多个合理候选时才暂停。
- 纯能力、规则或错误说明且没有唯一真实目标时直接解释，不猜对象；用户明确要求核对且目标唯一时必须真实读取，不能只展示命令。
- `mine` 仅表示我创建的，`shared` 仅表示共享给我的；`all` 表示 accessible 聚合目标。不得声称后端单个 noLimit 端点天然等于 `mine + shared`。
- 列表或逐字稿只有 `data.complete=true` 才能称为“全部/完整”。全量请求遇到 `meta.pagination.next_token` 时继续；只有 token 缺失、cursor 停滞/循环、达到 `page-limit` 或后页失败时才停止，并保留失败信封或不完整证据。
- 用户要求核对、汇总“这些/每条/全部”命中项时，必须覆盖完整命中集合；可用 `dws minutes +detail --ids <uuid1,uuid2> --artifacts basic --format json` 批量核对，并逐项保留失败。只检查第一条不能代表全体；响应没有逐条归属或组织字段时如实说明不可得，不能用当前 profile 的组织名代替每条听记的归属。
- 多听记、多来源或跨产品汇总按每个 `taskUuid`/来源 ID 保留 `requested/resolved/missing/artifacts/status`；缺输入或必需产物时整体按 partial，不用已找到项代表全部。
- 内容归纳必须来自每条真实 `summary/transcript/keywords`；只有 `title/basic` 时只列元数据，不生成摘要、关键词或分类。
- `partial_success`、异步 `pending`、超时和未知写入结果不是成功。按结果中的恢复句柄继续，不能重放已成功步骤。

## 安全边界

- 是否确认以 leaf Schema 与 Runtime gate 为准，不根据“看起来像写操作”自行推断。推荐 Golden Route 中的 `+update`、`+summary`、`+record-*`、`+share/+unshare/+apply-permission`、`+speaker-replace`、`+replace-batch` 等当前要求确认。
- 为兼容既有公开 Contract，对应的底层原子命令保留历史 `not_required`；它们只用于 Shortcut 无法表达的明确底层控制，不得为了绕过 Golden Route 的确认门禁而降级调用。
- `+upload` 与 `+upload-and-analyze` 即使不发送消息，仍会上传本地媒体并创建远端听记，真实执行必须按 Runtime confirmation；`--dry-run` 仍可在零远端调用下预览。上传并发送闪记卡片、精确同步并删除热词、撤权等副作用更大的入口继续单独处理。
- `+mindmap`、`+speaker-insights` 和 `+prepare-asr` 当前要求确认；`--resume` 仍沿用所在 Shortcut 的命令级门禁。旧 `+upload --enable-message-card` 与 `+upload-and-analyze --enable-message-card` 继续作为可执行兼容入口，并遵循各自 Shortcut 的确认门禁；新调用仍推荐 `+upload-and-notify`。原子 `minutes upload create --enable-message-card` 与旧 `--sync` 只保留为公开迁移提示，不得当作可执行 Golden Route。
- `--dry-run` 必须返回明确的 dry-run/request 证据且不调用远端；录音预览按上方原子入口执行。任何入口若拒绝 dry-run，必须报告“不支持预览”，不得把拦截或普通执行称为预演成功。
- 用户明确要求“仅预览/不实际写入”时，任务在真实现状读取、dry-run 计划和差异交付后结束；不得继续请求写入确认，也不得为了验证预览而执行真实写入后再还原。
- 分享/撤权使用稳定成员 UID，不能把姓名、手机号或跨组织 ID 直接当 UID；同一目标解析、读取、写入和验证必须使用同一 profile。

## 按需加载

Golden Route 参数足够时直接执行，不预读 Reference。每个 Case 最多先读取一个最精确的文件；参数事实优先读取 compact leaf Schema，不读取产品级全量 Catalog。

| 触发条件 | Reference |
|---|---|
| ASR 热词、上传会话恢复、复杂异步轮询、批量权限 workflow | [复杂流程](references/07-minutes.md) |
| 发言人替换、批量文本替换、下载媒体、离线导出等低频意图 | [局部意图](references/intent-guide.md) |
| 必须落到原子命令，或需要 URL/参数/确认事实 | [原子命令](references/minutes.md) |

## 错误最短路径

1. 零命中、多候选或 ID 类型不明：停止并返回候选证据。全量请求分页未完成但有有效 continuation 时继续；只有 token 缺失/停滞/循环、达到页数上限或后页失败时停止并返回 `data.complete`、`meta.pagination` 或失败信封等证据。
2. 认证、权限、profile 或 confirmation 错误：按 `dingtalk-shared` 对应错误 Reference 处理；不更换 scope、账号或写命令碰运气。
3. 异步超时或部分成功：保留 `taskUuid/taskId/sessionId/checkpoint`，只恢复未完成阶段；未知写入先读回，不能自动重试。

## 跨产品边界

- 把听记摘要或逐字稿写成文档 → 读取真实内容后切 `dingtalk-doc`。
- 把听记行动项创建为任务 → 读取行动项后切 `dingtalk-todo`，并按其身份解析规则处理执行人。
- 把摘要发给同事 → 切 `dingtalk-chat`；`+share` 只管理听记权限，不发送摘要消息。
- 创建或修改日程、会议室 → `dingtalk-calendar`；Minutes 只处理听记产物和录音控制。
