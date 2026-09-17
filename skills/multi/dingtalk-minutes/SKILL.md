---
name: dingtalk-minutes
description: 钉钉 AI 听记。Use when 查询或修改听记摘要、完整逐字稿、关键词、标签、行动项、录音、上传、思维导图、发言人洞察、ASR 热词/识别词配置或分享权限。写文档走 dingtalk-doc；建待办走 dingtalk-todo；日程走 dingtalk-calendar。命令前缀：dws minutes。
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

- 只用 `dws`；结构化读取加 `--format json`，按真实返回判断。
- 已知命令直调；参数/约束/安全不明查 leaf 窄 Schema。Schema 不可用才读已知 leaf Help 一次；`unknown flag` 用同 leaf Help 修正一次。`unknown command` 不查 Help：优先错误中的明确 suggestion，其次已加载 Skill/reference 中的明确兼容入口；均无则报漂移并停，禁全 Catalog。低频 reference 不默认 Help，禁 root/parent/product Help。发现后必须执行或说明阻塞。
- 不猜命令/flag/字段/ID/账号/业务事实；ID 来自真实返回。目标零命中/多候选/类型不明先消歧；仅可选时间/展示范围用契约默认，缺必需信息即停。
- 解析/读/写同一 profile，ID 不跨组织。多账号只用唯一 `isOrgCurrent=true`；否则用户指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出/记录 token、refresh token、appSecret、webhook token；已注入认证时不索要。
- 写须符合明确意图；确认以最终 Runtime gate/Schema 为准，确认后才加 `--yes`。
- 写后验证结果，不凭退出码宣称成功。退出须最终答复，区分完成、部分、阻塞、待确认、失败；保留已有数据及 `complete/hasMore/stopReason/failures`。
- 时间戳按会话时区展示，必要时保留原值。
- 认证/权限/profile/confirmation/未知错误只读 `dingtalk-shared` 对应 reference，禁连续猜替代命令。
<!-- DWS_RUNTIME_CONTRACT_END -->

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcut 发现（按需）

`minutes` 当前有 29 条公开 shortcut，完整清单保留在 Runtime Catalog 与 Schema，不在高频产品根 Skill 中重复展开。已知意图按下方路由/reference 直调；参数/约束/安全不明时读一次 leaf 窄 Schema。仅需且已发布 `result` 时查 outcomes/pagination，字段级再查 `data_schema`；缺失不以 Help/样例推断。Schema 不可用才读一次已知 leaf Help；`unknown flag` 用同 leaf Help 修正一次。`unknown command` 禁 Help：错误 suggestion → 已加载 Skill/reference 明确入口；均无则报漂移。禁全 Catalog/root/parent/product Help；低频 reference 不默认 Help。

仅当现有路由和 reference 都无法定位低频能力时，才执行 `dws shortcut list --service minutes --format json` 做最后回退；不要为已知高频意图加载完整 Shortcut Catalog 或产品级 Schema。
<!-- VISIBLE_SHORTCUTS_END -->

## Golden Route

以下是当前 Minutes Case 支持的核心路径，用于减少 Agent 选路分叉；它不等同于生产使用频率统计。已有 taskUuid 必须原样传入，禁止解码或重新编码；完整听记 URL 先提取 taskUuid，再原样使用。只有标题或时间线索时先搜索，零命中停止，多候选或候选差异较大时让用户消歧，不默认取第一条。

| 用户意图 | 唯一推荐入口 | 关键边界 |
|---|---|---|
| 按标题或时间找听记 | `dws minutes +search --scope all --query "<关键词>" --page-all` | `scope` 可选 `mine/shared/all`；至少提供 query/start/end 之一。`all --page-all` 分别追完 mine/shared 后去重，不把单个 noLimit 端点当完整全集 |
| 浏览我创建、共享给我或全部可访问听记 | `+list-mine` / `+list-shared` / `+list-all` | 默认是可续拉预览；要声称完整必须加 `--page-all` 并检查 `complete=true`。用户只说“我的听记”不等于明确 `mine`，范围不清时用 `all` |
| 按真实标签找听记 | `dws minutes tag list --format json`，再 `tag query --tag-id <真实tagId>` | 标签不等于关键词；空标签即结束。按标签查找的分页细节见局部意图 |
| 看我最新创建的一条 | `dws minutes +latest [--keyword <关键词>]` | 只在用户明确说“最新”时用；不能用它替代具名目标搜索，也不能在录音 start 后拿 latest 猜新录音 ID |
| 读取基础信息、摘要或关键词 | `dws minutes +detail --id <taskUuid> --artifacts basic,summary,keywords` | 已有 taskUuid 且只读取现有产物时直接使用，不要进入上传 workflow；任一产物失败都按 partial/非零处理，不把缺失项说成空内容 |
| 读取逐字稿 | `dws minutes +transcript --id <taskUuid> [--direction 1] [--single-page]` | 已有 taskUuid 且只读取逐字稿时直接使用，不要借 `+upload-and-analyze --resume-id` 代读。默认正序并追完分页；倒序必须传 `--direction 1`，用户明确只要第一页时传 `--single-page`，不得随后自动续页。交付前检查 `data.direction/data.complete/data.pages` 与 `meta.pagination` |
| 读取行动项 | 单条 `+action-items --id <taskUuid>`；批量 `+detail --ids <uuid1,uuid2> --artifacts todos` | 只在 `state=known_empty` 时说“没有待办”；`ready` 才能读取 `items`。`unsupported_shape/failed` 必须非零并保留脱敏 shape 证据，不能翻译为空结果。需要创建钉钉待办时再切 `dingtalk-todo` |
| 把摘要、关键词、完整逐字稿和行动项归档到本地 | `dws minutes +export-pack --id <taskUuid> --output <新相对目录>` | 要带媒体时加 `--include-media`；必须由 `published/path/manifest/files` 证明落盘，只有建目录、计划或文件名不能称已生成 |
| 修改或预览标题 | `dws minutes +update --id <taskUuid> --title "<新标题>"` | 真实修改按 Runtime confirmation 执行并读回；用户只要预览时先读 basic，再加 `--dry-run`，展示“当前值 → 目标值”后停止，不追加 `--yes` |
| 覆盖纪要正文 | `dws minutes +summary --id <taskUuid> --content @<相对文件>` | `content` 是完整目标正文，不是局部 patch；按 Runtime confirmation 执行，并保护图片引用、读回全文 |
| 上传音视频生成听记 | `dws minutes +upload --file <相对路径>` | 真实执行会上传文件并创建远端听记，必须按 Runtime confirmation；用户明确要闪记卡片时改用 `+upload-and-notify`，需要上传后等待分析产物时用 `+upload-and-analyze` |
| 真实开始、暂停、继续或停止录音 | `+record-start` / `+record-pause` / `+record-resume` / `+record-stop` | 这组入口会真实执行。start 返回 `accepted=true, bound=false` 或 `controlReady=false` 时，报告“已受理但未绑定”并停止：不得重试 start，也不得用 `+latest`、列表第一条或时间最近项猜 ID。结束并等待产物用 `+record-wrap-up` |
| 只预览录音请求，不实际执行 | `dws minutes record start --dry-run --format json` | 使用对应的原子 `minutes record start|pause|resume|stop` leaf；start 的 `--session-id` 可选，pause/resume/stop 必须传真实 `--id`。不要把被拒绝的 Shortcut dry-run 描述成预览成功 |
| 生成或继续思维导图 | `dws minutes +mindmap --id <taskUuid>` | 创建后有界轮询；超时或未知状态保留恢复信息，用 `--resume` 继续，不重复创建 |
| 生成或继续发言人洞察 | `dws minutes +speaker-insights --id <taskUuid>` | 只有 `state=ready/complete=true` 才完成；pending 保留 `taskId` 并同账号 `--resume [--task-id <ID>]`，不重复创建；用户给定等待时长传 `--timeout` |
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

只有用户明确要第一页、预览、样本或限定页数时才省略 `--page-all`，如实保留完整性与续页证据。每页 N 条须首轮及续页均传 `--limit N`，不能取默认数量后只展示 N 条冒充；限定页数用同一 leaf 的 `--cursor <真实 next_token>` 续拉，不扩大为全量。`+list-all` 的 `--cursor` 与 `--page-all` 互斥。有时间窗用 `+search`，因为 `+list-*` 不接受 `--start/--end`。

## 目标与完整性

- 时间展示与筛选：日期、时区偏移、星期及“本周”等边界必须由日期工具计算，不能心算或从标题日期推断。时间戳使用本地脚本 [format_timestamp.py](scripts/format_timestamp.py)：`python3 <本Skill目录>/scripts/format_timestamp.py --unit ms --timezone Asia/Shanghai <时间戳1> [时间戳2 ...]`；示例单位/时区须替换为已确认值（优先用户指定时区，否则已知会话时区）。最终直接复用输出，不补算星期或二次换算；已有明确时区的可读返回可直接引用，单位/时区不明时保留原值。相对周边界另用日期库计算，该脚本只转换时间戳。`duration` 等时长不传给该脚本；仅按已确认单位用本地计算换算，单位未知保留原值，不凭时长判断内容无效或录音状态。
- 目标锁定优先级：用户给出的 `taskUuid`/URL > 精确标题 > 标题包含或语义相关候选。相似候选可展示，但候选差异明显或多个候选都合理时必须停下来消歧；分页未完成本身不是目标歧义。
- 用户说“先确认/核对目标”默认要求用真实基础信息字段完成证据核对，不自动变成等待用户回复的会话门禁。目标唯一、分页完整且后续均为只读时，展示标题、时间、归属等核对证据后在同一轮完成所需交付；只有用户明确要求“等我确认后再继续”或仍有多个合理候选时才暂停。
- 纯能力、规则或错误说明且没有唯一真实目标时直接解释，不猜对象；用户明确要求核对且目标唯一时必须真实读取，不能只展示命令。
- `mine` 仅表示我创建的，`shared` 仅表示共享给我的；`all` 表示 accessible 聚合目标。不得声称后端单个 noLimit 端点天然等于 `mine + shared`。
- 列表或逐字稿只有 `data.complete=true` 才能称为“全部/完整”。全量请求遇到 `meta.pagination.next_token` 时继续；只有 token 缺失、cursor 停滞/循环、达到 `page-limit` 或后页失败时才停止，并保留失败信封或不完整证据。
- 用户要求核对、汇总“这些/每条/全部”命中项时，必须覆盖完整命中集合；可用 `dws minutes +detail --ids <uuid1,uuid2> --artifacts basic --format json` 批量核对，并逐项保留失败。只检查第一条不能代表全体；响应没有逐条归属或组织字段时如实说明不可得，不能用当前 profile 的组织名代替每条听记的归属。
- 多听记、多来源或跨产品汇总按每个 `taskUuid`/来源 ID 保留 `requested/resolved/missing/artifacts/status`；缺输入或必需产物时整体按 partial，仍交付成功内容支持的独立结果并列缺项；不缩减原请求或把子集说成全部，依赖缺失来源的结论暂不生成。
- 内容归纳必须来自每条真实 `summary/transcript/keywords`；只有 `title/basic` 时只列元数据；其他未满足用户要求的内容依据也只列元数据，分类任务列“待分类/证据不足”，不凭标题或时长判无效。按 taskUuid 去重后的已分类数＋待分类数须等于目标总数。
- `partial_success`、异步 `pending`、超时和未知写入结果不是成功。按结果中的恢复句柄继续，不能重放已成功步骤。

## 安全边界

- `+export-pack` 会递归清理全部文本产物中的已识别签名链接/凭据，签名目标替换为 `[signed-url-removed]`；扫描失败不发布。交付时说明 `sanitized/redactionCount/sanitizationScope`，不要把 `complete=true` 解释为摘要图片已离线保存；当前 `offlineImagesComplete=false`。二进制媒体内容不属于文本扫描范围。
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
