# DWS Shortcut — 方法论与进展交接文档（换会话续跑用）

> 目的：换新会话直接照此续跑。记录**方法论、已完成进展、如何继续、关键坑位、验证命令**。
> 分支：`feature/shortcut`（**改动全部未提交**，commit 由用户主动决定）。

---

## 0. 一句话现状

> 2026-07-09 **去冗余（重大修订）**：复盘发现 `internal/helpers/` 早有 ~697 个 `dws <svc> <verb>` 产品命令封装了 281 个 tool；1:1 shortcut 层有 235 个 tool 与之重复。按「tool 已被 helper 封装 且 shortcut 用 CallMCP 无投影」精确删除 **213 条纯重复 shortcut**，1:1 层 511→298、总数 579→366、服务 19→16（aisearch/live/devdoc 整包移除，其 dws 命令仍由 helper 提供）。全绿+真机复验保留命令可用。**教训：建封装层前先审已有封装。**


在 `internal/shortcut/` 下建成一套声明式 shortcut 体系：**366 条命令 = 298 条 1:1 封装 + 68 条真·智能编排**（1:1 层原 511，已删 213 条纯重复——helper 层早已封装同一批 tool），另有 **~60 条封装升级到 lark 输出投影保真度**、P2 高频自动沉淀闭环、深度对齐 lark 矩阵。**全绿**（build/gofmt/vet/shortcut 全量测试 `shortcuts=366 assembled=322 validated=44 failed=0`/app 全量回归 72s/真机抽验）。

> 2026-07-09 批33（净新增 1 条·+conflicts 的互补品）：`calendar +free-slots`（找某天工作时段内的空闲时段，list_calendar_events + 合并忙碌区间 + 工作窗口求补集，默认今天 09:00-18:00/--from/--to/--in-days）。真机验证：今日 4 段空档（09:00-09:15/15min、12:00-13:30/90min、14:00-14:30/30min、16:00-17:15/75min），正是忙碌事件的精确补集。conflicts+free-slots 构成真实排期智能。总数 578→579、smart 67→68。全绿。

> 2026-07-09 批32b（净新增 1 条·dws 原生编排，lark 也没有）：`calendar +conflicts`（检测某天日程时间冲突/双重预订，list_calendar_events + 本地两两 [start,end) 重叠检测，默认今天/--in-days）。真机验证：抓到今日 9 个日程里 2 处真实冲突（技术标评审 10:00-11:00 × AIX 共创 10:30-12:00；尖角班 14:30-15:30 × 中控项目 15:00-16:00）。证明复杂写死胡同之外，纯 MCP-tool 的本地编排仍有净新增价值。总数 577→578、smart 66→67。全绿。

> 2026-07-09 批32（review lark 复杂写类 + 架构边界结论，用户指定方向）：fresh review lark 复杂写 shortcut（mail +send/+reply/+forward、drive +import、doc +media-insert/download、base +record-upload-attachment），交叉 dws helpers。**关键结论——架构性死胡同，非没使劲**：① `mail +send` dws 已有 1:1 `+send`(send_email)+`+draft-*`，且 **contact 无 email 字段**→无法按名解析收件人（矩阵早 skip），也无 signature/template/lint 工具；② `drive +import`/`doc +media-*`/`base +record-upload-attachment` 核心是**本地文件字节 PUT/下载落盘**——dws 里由 helper 内部 `httpPutFile`/`http PUT/GET`（drive.go/doc.go）实现、已在 1:1 层覆盖（如 `drive upload --convert`），但 **shortcut 框架 `rt.CallMCP/CallMCPData` 只编排 MCP tool、结构上做不了原始文件 I/O**，故无法在 smart 层组合。**建议**：剩余复杂写要么卡此边界、要么已被 1:1 覆盖；真要补文件类能力应在 helper/1:1 层加命令，而非 shortcut 层。本批 review、无代码改动，总数仍 577。注：本轮触及 session 限额（8:20pm 重置）+ 分类器一度不可用，Bash 受限。

> 2026-07-09 批31（净新增 1 条）：`calendar +my-free`（我自己的忙闲，自动解析当前 userId、默认今天，复用 +free 的 freebusySlots 投影；无需像 +free 传别人姓名）。真机正向验证：返回今日/明日真实忙碌时段 {busy:[{start,end}],userId,free}。总数 576→577、smart 65→66。文档全量同步。全绿。

> 2026-07-09 批30（净新增 1 条）：`contact +me`（当前用户 `get_current_user_profile` + 投影 `{name,userId,mobile,dept,org,email}`，agent 的「我是谁」；区别于 1:1 `+get-self` 吐冗长 `result[].orgEmployeeModel` raw）。真机正向验证：董鑫阳/202397/模型算法/钉钉。总数 575→576、smart 64→65。文档全量同步。全绿。

> 2026-07-09 批29（净新增便利读 3 条 + 真机抓修 1 bug）：`oa +done-approvals`（审批历史 get_done_tasks）、`mail +recent-mail`（近期收件 list_mailbox_threads + 解析绑定邮箱/收件箱 folder）、`attendance +this-month`（本月打卡 query_check_record on attendance-wukong，复用 +my-attendance）。**真机抓到并修复 bug**：`+done-approvals` 原来 --limit 不传时 pageSize=0 → 后端 business error（1:1 list-executed 有默认所以正常）；改为默认 pageSize=20，真机复验走空路径「没有已处理的审批记录」。+this-month 真机有效空、+recent-mail 正确报未绑定邮箱。总数 572→575、smart 61→64。文档全量同步。全绿。

> 2026-07-09 批28（净新增便利读 smart，多 agent 并行 + 手工）：再建 3 条只读 smart——`oa +pending`（`list_pending_approvals` 只读列待我审批，区别于会审批的 +approve-by）、`todo +due-today`（`get_user_todos_in_current_org` + `planFinishDateStart/End` 服务端过滤今天到期，区别于 +overdue 已过期）、`calendar +tomorrow`（明天日程，复用 +today/+week 投影）。真机：+tomorrow 返回真实明日日程；+pending/+due-today 空路径正确且复用已验证 helper。3 条为 dws 原生便利读、不对应 lark gap，矩阵 42/48 不变，总数 569→572、smart 58→61。文档全量同步。全绿。

> 2026-07-09 批27（写类输出扫荡收尾，确认无更多 bug）：扫 smart 里丢弃 CallMCPData 结果的 3 处——`broadcast`（per-recipient 循环、最终结构化输出，OK）、`book`（弃 add-participant 结果但最终 `get_calendar_detail` 确认 + 失败回滚，OK）、`reschedule`（弃存在性 check detail 是有意的，随后打 update 结果，OK）。**无更多 silent-success bug**。结论：**输出质量扫荡完成**，只读投影 clean、写类确认结果、honor --format。剩余仅复杂 net-new gap-buildable（mail +send/drive +import 等多步写、难安全真机验）或 commit。真机+一致性改进累计 13 条，总数 569，全绿。

> 2026-07-09 批26（写类 smart 输出一致性批量修）：扫 smart 里用 `fmt.Print*` / 无标准输出的。修 2 条：`chat +broadcast`（`fmt.Printf` 群发摘要忽略 --format → `rt.Output({sentCount,failedCount,sent,failed})`）；`wiki +wiki-new-doc`（**原创建文档后丢弃 create_file 结果、静默 return nil**，真 UX bug 拿不到新文档 id/url → 捕获并 `rt.Output({created,space,title,result})`）。写类无法真机验（会真建/发），assemble 测试确认组装正确、低风险。总数仍 569。

> 2026-07-09 批25（+next-event 输出一致性修复 + sweep 确认多数已 clean）：sweep 探 chat +conversation-list/+category-list、aitable +base-list 等——**多数 1:1 只读命令输出已 clean**（属先前 ~60 升级覆盖），保真度工作基本到位。**修复 1 条一致性**：`calendar +next-event` 原用 `fmt.Println` 打固定文本行、**忽略 --format/--jq/--fields**，改为 `rt.Output(map{event:项目投影})`（复用 +today/+week 同款投影），真机复验 `--format json` 出结构化 `{event:{title,start,end,location,eventId}}`、`--jq '.event.title'` 可用。这是 Agent 友好性修复。同时删除死代码 `shortcutNextEventSummary` + 无用 fmt import。总数仍 569。

> 2026-07-09 批24（1:1 层保真度升级续）：`contact +list-followings` 原 `rt.CallMCP` 吐 `{arguments,result:{models:[…]}}` 信封噪音，改为 `CallMCPData`+`listFollowingsProject`，真机复验干净 `{count:13, followings:[{openDingTalkId}]}`。总数仍 569。**判断**：真机验证 + 保真度升级已到深度边际收益区（本批仅拍平 ID 列表）；1:1 层多数只读命令要么需特定参数、要么后端权限受限、要么输出已可接受。**建议优先 commit 留存 24 批成果**（10 个真机改进 + 58 smart + 保真度升级 + P2 + 全套文档），再按需推进剩余 1:1 微升级。

> 2026-07-09 批23（验证驱动的 1:1 层保真度升级起步）：真机探 1:1 只读命令，`drive +recent` 原 `rt.CallMCP` 吐冗长 raw（logId/nextCursor 噪音 + 每项巨型 docUrl + hasMore），改为 `CallMCPData`+`recentListProject`：投影 `{count, hasMore, items:[{name,nodeType,contentType,accessTime,docUrl,nodeId}], nextCursor}`，去 logId 噪音、保留分页与链接，真机复验干净。`drive +list-spaces` 真机空（有 errorCode 包裹噪音但 result.items 为空，暂不动）。总数仍 569。**注**：1:1 层仍有数十个 list 命令可类似升级，但属边际收益、量大，建议按需/被动推进，优先 commit 留存已有成果。

> 2026-07-09 批22（报告综合更新，反映真机验证战役）：给 `shortcut-report.md` 新增 §2.4「真机验证战役」：记录 9 批真机验证（正向验证 20+ 条、抓修 8 个真实 bug 的表格、后端受限项、resolveUser 非 bug 澄清）；`shortcut-report.html` §③ 测试表补 2 行 + 一段说明。诚实反映「assemble 合成测试盲区 → 真机验证补齐」的价值。纯文档，无代码改动，总数仍 569。

> 2026-07-09 批21（find-record 验证 + +suggest-time 保真度升级）：`aitable +find-record` 真机正常（返回真实记录；cells 按字段 ID 键值、内含附件对象，天然复杂，clean 投影需 field-id→name 解析属更大改造，暂留）。**升级 1 条**：`calendar +suggest-time` 原 `rt.CallMCP` 吐 `result.recommendEventTimes[]` 且 `timeConflictAttendees:[null]` 噪音，改为 `CallMCPData`+`suggestTimeSlots`+`Output`：拍平 result、丢弃 null 冲突项，真机复验干净 `{suggestions:[{start,end}]}`（有真实冲突时才带 conflicts）。总数仍 569。**说明**：真机验证扫荡已进入边际收益递减区（明显 raw-verbose 的 wart 基本清完），剩余多为 minutes org-gated、写类、或输出已可接受。列出 12 条只读仍用 raw `rt.CallMCP` 的 smart（多为 minutes org-gated 或已验证 clean）。**升级 1 条**：`calendar +today` 原直吐 17 字段冗长事件（含完整 attendees 数组），改为 `CallMCPData`+复用 `+week` 的 `shortcutNextEventList/Start` 投影，真机复验干净输出 `{events:[{title,start,end,location,eventId}]}`（与 +week 一致 + location）。总数仍 569。剩余 raw-CallMCP 只读 smart：action-items/latest-minutes/transcript（minutes org-gated 无法真机验）、org/report-latest（已验 clean）、find-record/suggest-time/by-mobile/lookup/team（待验或权限受限）。

> 2026-07-09 批19（日历只读 smart 验证 + +free 保真度升级）：`calendar +next-event` 真机正常（可读摘要「下一个日程：致拓 AI FDE 经验分享…」，但**忽略 --format json 只吐文本**，已知小瑕疵未改）。**升级 1 条**：`calendar +free` 终结步原 `rt.CallMCP` 直吐冗长 `result[].scheduleItems[].{start,end}.dateTime` 嵌套，改为 `CallMCPData`+`freebusySlots`+`Output`，真机复验干净输出 `{who,userId,free,busy:[{start,end}]}`（董鑫阳 2026-07-10 忙 4 段）。总数仍 569。

> 2026-07-09 批18（真机验证续 + +group-members 保真度升级）：**验证正常**：`contact +org`（董鑫阳→模型算法/17人，3步链）、`drive +find-file`（干净投影 {dentryId,fileSize,name,type}）。**后端受限（非 bug）**：`contact +team`（列部门成员 `PAT_MEDIUM_RISK_NO_PERMISSION`）、`chat +search-msg`（org 未开 CLI 数据访问 `TOKEN_VERIFIED_FAILED`）。**升级 1 条**：`chat +group-members` 终结步原用 `rt.CallMCP`（直吐原始冗长 `result.list[]` + memberAvatarMediaId + arguments/errorCode 噪音），改为 `CallMCPData`+`groupMemberProject`+`Output`，真机复验干净输出 `{count, members:[{name,nick,role,openDingtalkId}]}`（刘力/怒龙/群主…）。总数仍 569。

> 2026-07-09 批17（修复批16 发现的 +at-me 投影）：`chat +at-me` 原来因 `atMeMessageItems` 不认识真实两层嵌套 `result.conversationMessagesList[].messages[]` → 命中 fallback、直接吐原始结构。真机 dump 出真实结构（group 有 title/openConversationId/messages；message 有 sender/content/createTime/openConversationId），新增 `atMeFlattenGroups` 把各会话组拍平成单一消息列表、并把组的会话 title 下沉到每条消息。真机复验：43 条消息干净投影为 `{conversation,sender,text,time}`（如 conversation:"AI全栈"、sender:"龙衔"）。总数仍 569。

> 2026-07-09 批16（只读 smart 真机验证扫荡 + 质量修复）：真机跑一批时间/自身类只读 smart。**验证正常**：`calendar +today`（真实日程+参会人）、`calendar +week`（干净投影）、`todo +overdue`（空）、`attendance +my-attendance`（空）、`report +report-latest`（"暂无日志"空路径）、`oa +my-initiated`（真实审批数据）。**修复 1 个输出 wart**：`chat +unread-chats` 每行都吐 `unread: null`——因 `unread_message_conversation_list` 根本不返回每会话未读数（在列表里即代表未读），改为「仅当 gateway 真返回未读数时才带 unread 字段」，真机复验输出已干净 `{conversationId,name}`。**已知待优化（未改）**：`chat +at-me` 返回 `result.conversationMessagesList[].messages[]` 冗长嵌套原始结构、未拍平成干净消息列表（功能正常，投影可再优化）。总数仍 569。

> 2026-07-09 批15（质量修复 + resolveUser 排查，真机）：**修复** `contact +dept-members` 消歧消息 `<red>` 标记泄漏——复用 `stripHighlightTags`（resolve_dept.go）在 name 提取处剥离，真机复验消息已干净（"开放平台(666202009)、技术平台-开放平台研发(1085781688)…"）。注：`dept_members.go` 本身容器解析(含 deptList)+数值 deptId 早已健壮，仅 name markup 未剥。**排查澄清（非 bug）**：`resolveUser` 对 `董鑫阳` 真机端到端正常（userId 202397、部门 模型算法）；但对 `秋画` 这类联系人 `search_contact_by_key_word` 返回 name/userId 全 null（仅 openDingTalkId），resolveUser 正确报「没找到」而非瞎猜——这是钉钉数据模型现实（外部/受限联系人无 userId），非代码 bug。**已知限制**：按名解析仅对「搜索能返回 userId 的组织内成员」有效。总数仍 569。

> 2026-07-09 批14（真机验证续，需具体 ID 的只读 shortcut）：**正向验证过**：`chat +my-groups`（98 真实群+投影）、`aitable +base-list`/`+list-tables`（真实 base/table）、`aitable +resolve-table`（单命中 通用→99dV75A、多候选消歧，容器 key `tables` 正确，无 deptList-class bug）。**后端权限受限、无法正向验证（非代码 bug）**：`chat +chat-messages`（`PAT_MEDIUM_RISK_NO_PERMISSION`，读会话消息需更高权限）、`minutes +*`（该 org 未开启 CLI 数据访问 `TOKEN_VERIFIED_FAILED`）。结论：可验证的 read/resolve shortcut 全部投影正确，仅批13 的 resolve-dept 有真 bug 已修。

> 2026-07-09 批13（真机验证 + bug 修复，登录态 corp「钉钉」）：用登录态把批9-12 只读 shortcut 打真实后端。**正向验证过**：`doc +find-doc`（10 真实文档、投影干净）、`aitable +resolve-base`（多候选真实 baseId）、`mail +find-mail-user`（命中真实用户+邮箱）、`contact +resolve-dept`（修复后返回真实候选）。**真机抓到并修复 1 个真 bug**：`contact +resolve-dept` 原来对任何真实部门名都返回「未找到」——真实 `search_dept_by_keyword` 响应容器 key 是 **`deptList`**（agent 的探测清单漏了），且 `deptName` 带 `<red>…</red>` 高亮标记、`deptId` 是数值。已修：容器加 `deptList`、`stripHighlightTags` 去标记、deptId 数值 coerce 成串（`resolve_dept.go`），真机复验通过（开放平台→666202009、财务→846624121，名称干净）。**已知遗留（未改）**：`contact +dept-members` 的消歧提示消息里 `<red>` 标记未剥离（仅 cosmetic，功能正常）。总数仍 569，本批未加新命令。

> 2026-07-09 批12（多 agent 并行，dws 原生 resolver 层）：再建 3 条「按名解析 ID」智能 shortcut——`wiki +resolve-space`（search_wikiSpaces 名→spaceId）、`aitable +resolve-table`（get_tables 在 Base 内本地名→tableId）、`contact +resolve-dept`（search_dept_by_keyword 名→deptId，**已修数值 ID 兼容**：deptId 为 JSON number 时 coerce 成串，非 string-only）。均 0/1/多候选消歧，对标 resolveUser 各资源版。这 3 条不对应具体 lark gap（是 dws 原生便利层），故 gap-buildable/covered-smart 矩阵计数不变(42/48)，仅总数 566→569、smart 55→58。文档全量同步。全绿。

> 2026-07-09 批11（多 agent 并行）：再建 3 条智能 shortcut——`aitable +resolve-base`（search_bases 按名解析 baseId + 0/1/多候选消歧）、`chat +chat-messages`（群/单聊会话消息 list_conversation_message_v2 / list_individual_chat_message，ExactlyOne 互斥 + 投影）、`mail +find-mail-user`（search_mail_users 按名搜企业邮箱联系人 + 投影）。文档全量同步 566/505/55（gap-buildable 49→42、covered-smart→48）。全绿。注：本批 app 回归首跑因并发负载 flaky FAIL 一次（80s），连跑 2 次稳定 PASS（71s）——非本次改动导致。

> 2026-07-09 批10（多 agent 并行）：再建 3 条智能 shortcut——`chat +thread-replies`（list_topic_replies 拉话题回复 + sender/text/time 投影）、`todo +related-tasks`（get_user_todos_in_current_org 三角色 creator+executor+participant 并集 + taskId 去重 + 投影）、`doc +find-doc`（search_documents 关键词搜文档 + title/url/type/token 投影）。均以 helper 为 ground truth、0 编造。文档全量同步到 563/503/52（report.md/html、lark-alignment.md gap-buildable 49→44、covered-smart→46、comparison.html 重生成）。全量测试 + app 回归全绿。

> 2026-07-09 续跑增量（批9·手工）：新建 3 条智能 shortcut——`minutes +detail`（单命令聚合一条听记 basic/summary/keywords/transcript/todos、partial-failure 容错）、`minutes +replace-batch`（多组 `原文=>替换` 批量替换、去重校验+逐组聚合）、`aitable +record-share-links`（>20 条记录分享链接：去重+分片≤20/批+跨 `aitable-helper` server fanout+合并）。均以 helper 为 ground truth。**同步刷新全部文档到 560/501/49**：`shortcut-report.md`、`shortcut-report.html`、`shortcut-lark-alignment.md`（gap-buildable 49→46、covered-smart 41→44）、重生成 `shortcut-comparison.html`。全量测试 + app 回归全绿。

---

## 1. 背景与目标

- 对齐基准：`/Users/dennis/Projects/larksuite/cli`（lark-cli 的 `shortcuts/` 框架，飞书 REST API）。
- dws 执行底座：**钉钉 MCP**（粗粒度：一个 tool = 一个完整操作）。
- 目标：把 lark 的 shortcut 能力**深度对齐每一个**到 dws，并补钉钉侧系统性能力。
- 关键认知：lark 的"组合性"多源于飞书 API 细粒度（先查 token→id→再操作）；钉钉 MCP 粗粒度，**lark 的多步在钉钉大量塌缩成 1:1（已被封装层覆盖）**。真正需要"编排"的是「按名解析 ID + 多工具串联 + 跨服务」——这些做成了 smart 层。

---

## 2. 架构与关键文件

```
internal/shortcut/
  types.go        # Shortcut / Flag / Risk 声明结构
  runner.go       # RuntimeContext + mount(编译成cobra) + CallMCP/CallMCPData/Output + 校验/dry-run/风险确认
  validate.go     # 跨字段校验 helper：MutuallyExclusive/AtLeastOne/ExactlyOne/RangeInt/RequireAll
  register.go     # Register() / Commands() / All()
  shortcut_test.go# 框架单测
  builtin/
    builtin.go       # blank-import 所有服务包 + smart 包；Commands() 汇总
    coverage_test.go # ★全量测试：TestAllShortcutsAssemble / TestAllToolLiteralsAreReal / TestAllHaveIntent / TestNoDuplicateCommands
  <service>/         # 19 个服务包：contact/chat/calendar/todo/doc/drive/mail/wiki/minutes/oa/report/attendance/aitable/sheet/devapp/ding/aisearch/live/devdoc
    <service>.go     # 该服务的 1:1 封装 shortcut（var + init(){shortcut.Register(...)}）
  smart/             # ★真·智能层（多步/编排/按名解析/跨服务）
    resolve.go       # resolveUser(rt,name) 名→userId+消歧；contactUser{userID,name}；extractUsers/userLabels
    dm.go lookup.go assign.go book.go free.go ... # 每条一个文件
  usage/          # P2 埋点：recorder.go(记形状不记值) stats.go command.go(dws shortcut list/stats/suggest/add)
  userdef/        # P2 自定义 shortcut YAML 运行时加载 loader.go

internal/app/legacy.go   # 接线点：newLegacyPublicCommands 里 append builtin.Commands() + userdef.Load()
internal/app/root.go     # 装配 recordingToolCaller(埋点) + dws shortcut 命令
internal/helpers/*.go     # ★Ground truth：钉钉真实 MCP tool 名 + 参数（callMCPTool("tool",{...})）

docs/
  shortcut-plan.md            # 总规划
  shortcut-p2-design.md       # P2 自动沉淀设计
  shortcut-report.md / .html  # 综合报告 + GSB
  shortcut-comparison.html    # 逐条三方对照(dws vs lark vs 原生MCP)
  shortcut-lark-alignment.md  # ★深度对齐矩阵(lark 361条逐条分析, 49 gap-buildable)
  shortcut-handoff.md         # 本文件
scripts/gen_shortcut_comparison.py  # 生成三方对照 HTML
```

---

## 3. 框架契约（写新 shortcut 必读）

一个 shortcut = 包级 `var X = shortcut.Shortcut{...}` + `func init(){ shortcut.Register(X) }`。

```go
var SearchUser = shortcut.Shortcut{
    Service: "contact",        // 顶层命令
    Command: "+search-user",   // + 前缀，kebab-case
    Product: "contact",        // MCP server id（默认=Service；注意跨 server，见坑位）
    Description: "...",         // 一行
    Intent: "自然语言：做什么/何时用/副作用",  // 每条必填(TestAllHaveIntent 强制)
    Risk: shortcut.RiskRead,   // Read / Write / HighWrite(删除等，框架二次确认)
    Flags: []shortcut.Flag{{Name:"query", Type:shortcut.FlagString, Required:true, Desc:"...", Enum:[]string{...}}},
    Validate: func(rt *shortcut.RuntimeContext) error { return rt.RequireAll("query") }, // 可选
    Execute: func(rt *shortcut.RuntimeContext) error { ... },
}
```

RuntimeContext 方法（`internal/shortcut/runner.go`/`validate.go`）：
- 读参数：`rt.Str/Bool/Int/StrSlice(name)`、`rt.Changed(name)`
- **调 MCP 并打印**（终结步，1:1 封装用）：`rt.CallMCP(tool, params) error`（用自身 Product）
- **调 MCP 拿数据**（多步/投影用，不打印，可跨 server）：`rt.CallMCPData(product, tool, params) (map[string]any, error)`
- **投影输出**：`rt.Output(payload) error`（吃 --format/--jq/--fields）
- 校验：`rt.MutuallyExclusive/AtLeastOne/ExactlyOne(flags...)`、`rt.RangeInt(flag,min,max)`、`rt.RequireAll(flags...)`
- smart 复用：`resolveUser(rt, name) (contactUser, error)`（名→userId+消歧，在 smart/resolve.go）

对标 lark：`CallMCPData`≈`CallAPITyped`；`resolveUser`≈`ResolveOpenIDsTyped`；`rt.Output`≈`OutFormat`；`Validate helper`≈lark 的 MutuallyExclusive/AtLeastOne。

---

## 4. 方法论（怎么高效批量建，屡试不爽）

**核心：多 agent workflow 并行 + helper 为 ground truth + 严格 skip + build/test 门禁。**

1. **每个 shortcut/服务一个 agent**，并行（`parallel(...)`）。
2. **Ground truth 铁律**：tool 名和参数 key **只能逐字取自 `internal/helpers/<svc>.go` 的真实 `callMCPTool("tool",{params})` 调用点**，严禁编造。agent 必须先 Read+grep helper。
3. **宁缺勿错**：拿不准的 tool/参数/结构 → **skip 并说明**，不瞎写（已多次证明 agent 会正确 skip，如 mail 无 email 字段）。
4. **响应字段防御式解析**：返回结构无契约保证 → 多候选 key 探测（result/data/list/items + 字段别名），不硬编码。
5. **不同 agent 写不同文件**（服务包 vs smart 包，或不同 service 文件）→ 无写冲突；**禁止 agent 改 builtin.go**（我事后统一维护 blank import）。
6. **落地后统一**：`gofmt -w` → `go build ./...` → shortcut 全量测试 → 命名冲突用 rename 修（如 smart 的 `+approve` 撞 1:1 层 → 改 `+approve-by`）。
7. **周期性 app 全量回归**（改多个服务文件后）：`go test ./internal/app/...`（~73s）验证接线。

workflow 脚本模板见任意 `~/.claude/.../workflows/scripts/build-smart-*.js` 或 `upgrade-fidelity-*.js`（每次 Workflow 调用都存了盘，可 `{scriptPath}` 复用/改）。

---

## 5. 已完成进展

### 5.1 覆盖层（511 条 1:1 封装 / 19 服务）
chat89 aitable86 mail43 attendance36 doc33 minutes31 devapp30 sheet29 calendar24 drive24 oa20 todo18 wiki15 contact14 report7 ding7 aisearch3 live1 devdoc1。每条带自然语言 Intent。

### 5.2 智能层（~46 条 `internal/shortcut/smart/`）
按名操作人/群、多步编排+失败回滚、时间/自身智能、跨服务、钉钉原生编排。已建（举例）：
`chat +dm/+send-to-group/+broadcast/+group-members/+at-me/+search-msg/+unread-chats`、`contact +lookup/+org/+team/+by-mobile/+dept-members`、`calendar +book(回滚)/+free/+today/+week/+next-event/+invite/+suggest-time/+reschedule/+cancel-event/+respond-event/+find-room`、`todo +assign/+assign-multi/+overdue/+todo-done/+remind/+created-todos`、`minutes +latest-minutes/+action-items/+transcript/+minutes-search/+detail/+replace-batch`、`oa +approve-by/+my-initiated`、`attendance +my-attendance`、`report +report-latest`、`aitable +find-record/+list-tables`、`doc +share-doc/+doc-append`、`wiki +wiki-new-doc`、`drive +find-file`、`mail +search-mail/+unread-mail`。

### 5.3 框架系统性能力（对齐 lark）
`resolveUser`、`CallMCPData`、`rt.Output`、`Validate×5`。

### 5.4 保真度升级（~64 条封装：CallMCP→CallMCPData+投影+Output，对齐 lark 96% 输出投影）
覆盖 contact/chat/calendar/todo/doc/drive/mail/wiki/aitable/oa/devapp/attendance/minutes/report/sheet 等服务的列表类命令。

### 5.5 P2 高频自动沉淀（差异化，lark 无）
埋点(记形状不记值,默认关/opt-in DWS_USAGE_TRACKING=1) → `dws shortcut stats/suggest` → `dws shortcut add` 写 `~/.dws/shortcuts/*.yaml` → 运行时 `userdef.Load()` 编译注册。**闭环端到端跑通。**

### 5.6 深度对齐矩阵
`docs/shortcut-lark-alignment.md`：逐条分析 lark 361 条 → covered-1to1 144 / no-dingtalk-tool 127 / **gap-buildable 49** / covered-smart 41。

---

## 6. 如何继续（下一步 backlog）

1. **保真度升级剩余列表命令**（还有部分服务的 list 命令仍是裸 CallMCP）：起 `upgrade-fidelity-N` workflow，每服务 agent 挑 1-2 个未升级(`grep 'rt.CallMCP('`)的列表读命令，改成 CallMCPData+投影+Output。范式见 `contact.go` 的 `searchUserProject`/`listRolesProject`。
2. **补剩余 gap-buildable smart shortcut**（矩阵里 49 个，已建 ~20+）：起 `build-smart-N` workflow（smart 包，不碰服务包）。剩余偏复杂（sheets/base 操作、消息富化、分片下载），谨慎、允许 skip。
3. **每批**：gofmt→build→shortcut 测试→（改多文件后）app 回归→更新 `docs/shortcut-report.md`。
4. **收尾**：把 `docs/shortcut-report.md/html` 的计数刷新到最终（部分 §2 测试数字可能还停在旧值 511/456，实际 ~557/~500），重跑 `python3 scripts/gen_shortcut_comparison.py`。

---

## 7. 关键坑位（务必注意）

- **跨 server 路由**：有些 tool 不在本服务 server。已知：contact 花名册 tool 走 `hrmregister`；chat 部分 tool 走 `im`/`bot`；`query_check_record` 走 `attendance-wukong`（不是 attendance）；wiki `create_file` 走 `doc` server。→ 这类必须用 `rt.CallMCPData("<真实server>", ...)`，不能用 `rt.CallMCP`（它按 shortcut.Product 路由会打错 server）。判断依据：helper 里是 `callMCPToolOnServer("<server>", ...)`。
- **命名冲突**：smart 命令别撞 1:1 层（如 `+approve`→用 `+approve-by`，`+freebusy`→用 `+free`）。`TestNoDuplicateCommands` 会抓。
- **参数别名**：aitable 查询关键词是 `keyword`（不是 query）；todo 建待办用嵌套 `PersonalTodoCreateVO`；日程 create/update 时间是 ISO 字符串，而 list/busy 是毫秒——一切以 helper 调用点为准。
- **中文 const tool 名**：minutes 录音 tool 是中文 `"执行听记指令-发起AI听记录音"`（const `listeningNoteCmdTool`）——真实，别当编造。
- **测试真机验证**：token 有时效（`dws auth status` 看 expires），过期会报错非代码问题。真机跑命令时第一次有 catalog 发现 banner（stderr），结果 JSON 在后面，别用 `head` 截断。
- **工具标签格式**（给 AI：本会话我多次误用错标签导致工具调用失败——务必用正确的 function-call 格式）。

---

## 8. 验证命令速查

```bash
cd /Users/dennis/Projects/dingtalk-workspace/dingtalk-workspace-cli
go build ./...                                    # 编译
gofmt -l internal/shortcut/                        # 格式(应空)
go test ./internal/shortcut/...                    # shortcut 全量(含 assemble/tool-real/intent/no-dup)
go test ./internal/app/...                         # app 回归(~73s，改服务文件后必跑)
go test ./internal/shortcut/builtin/ -run TestAllShortcutsAssemble -v 2>&1 | grep shortcuts=  # 看总数

# 真机(需登录 dws auth login)
DWS_USAGE_TRACKING=0 dws contact +lookup --name <真实姓名>   # 智能多步示范
DWS_USAGE_TRACKING=0 dws calendar +today                     # 时间智能
DWS_USAGE_TRACKING=0 dws contact +search-user --query <名>   # 投影输出示范
```

测试口径：`TestAllShortcutsAssemble` = 假 Caller 拦截，给每条命令(含写/删)喂合成参数、走完解析→校验→组装 MCP 调用、断言 tool 真实无 panic（零副作用全量验证）。`TestAllToolLiteralsAreReal` = 所有 CallMCP tool 名比对 helper ground truth 防编造。

---

## 9. 未提交提醒

所有工作在 `feature/shortcut`，**未 commit**。建议尽快分语义化 commit 留存（框架 / 511封装 / smart层 / 保真度升级 / P2 / 文档）。
