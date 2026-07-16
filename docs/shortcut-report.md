# DWS Shortcut 能力整合与对齐报告

> 版本：截至本轮 loop　|　对齐基准：larksuite/cli（lark-cli）　|　执行底座：钉钉 MCP
> 相关文档：[总规划](shortcut-plan.md) · [P2 自动沉淀设计](shortcut-p2-design.md) · [HTML 报告](shortcut-report.html) · [**逐条三方对照 HTML**](shortcut-comparison.html)（每个 shortcut：dws +命令 vs lark-cli vs 原生 MCP 组合）

---

## 1. Shortcut 整合了哪些能力

`dws` 现内建 **298 个 1:1 封装 shortcut** + **68 条 smart 智能编排命令**（§1.3）= **合计 366 条**，覆盖 **16 个钉钉服务**，以 `dws <service> +<command>` 形式提供。

> ⚠️ **重要修订（去冗余）**：1:1 层原为 511 条，**复盘发现 `internal/helpers/` 早已把大量 MCP tool 封装成 `dws <svc> <verb>` 产品命令**——其中 **213 条 1:1 shortcut 只是把已被 helper 封装过的同一个 tool 用 `+` 前缀又封了一遍、且无输出投影增量，属纯重复**，已删除。**保留的 298 条 = 233 条填 helper 空白（helper 从没封装的 tool）+ 65 条虽 tool 重复但加了干净投影**。这是对"建 1:1 层前没先摸清 helper 已封装什么"的纠偏（详见 §5 复盘）。aisearch/live/devdoc 三个服务的 shortcut 全属纯重复、已整包移除（其 `dws <svc>` 命令仍由 helper 层提供）。

### 1.1 能力清单（按服务，prune 后）

| 服务 | 1:1 shortcut 数 | 覆盖能力（摘要） |
|------|:---:|------|
| chat（群聊/消息） | 79 | 群管理、群成员、群身份角色、消息收发/撤回/转发/表情/卡片、会话置顶/免打扰、消息分组、机器人 |
| aitable（多维表 base） | 77 | 数据表/字段/记录/视图/表单/仪表盘/图表/角色/协作全生命周期 |
| attendance（考勤）★ | 33 | 打卡记录、审批、排班、班次、考勤组、统计报表、请假 |
| devapp（开放平台应用 apps） | 30 | 应用增删改查、成员、权限、版本发布、事件订阅、扩展机器人/H5 配置 |
| doc（文档） | 16 | 文档/文件夹、正文块读写、权限、附件、节点 |
| contact（通讯录） | 9 | 用户/部门搜索与详情、角色、花名册 |
| drive（钉盘） | 8 | 文件/文件夹管理、下载、复制移动、权限、最近访问 |
| calendar（日历） | 8 | 日程、参与人、会议室、忙闲、ACL、日历本 |
| minutes（AI 听记） | 7 | 妙记详情/逐字稿、录音控制、说话人 |
| oa（审批）★ | 6 | 审批实例、单据处理、模板、流程 |
| mail（邮箱） | 6 | 邮件搜索/线程、标签、联系人、收信规则（投影类保留） |
| wiki（知识库） | 5 | 知识空间、节点、成员 |
| todo（待办 task） | 5 | 待办、子任务、执行人/参与人、附件 |
| ding（DING）★ | 5 | 机器人/个人 DING 发送、撤回、接收状态 |
| sheet（钉钉表格） | 2 | 区域读写（投影类保留） |
| report（日志）★ | 2 | 日志收件箱/发件箱 |
| **合计** | **298** | **16 个服务** |

★ = 钉钉特有服务，lark-cli 无对应（详见 §3 GSB）。**注**：多数服务的 `shortcut 数` 已远小于该服务的 MCP tool 总数——因为 tool 的基础封装由 helper 层的 `dws <svc> <verb>` 命令承担，1:1 shortcut 只保留 helper 没覆盖的、或加了投影的。

### 1.2 每个 shortcut 统一具备的能力（框架注入）

不是简单命令别名，而是叠加在裸 MCP 之上的**精选薄封装**，统一获得：

- **声明式定义**：`Shortcut{Service, Command, Product, Risk, Flags, Execute}`，一处声明、框架编译成 cobra 命令。
- **自然语言 Intent**：每条 shortcut 均带一段自然语言描述（做什么/何时用/关键输入产出，写删类点明副作用），面向用户与 AI agent 的意图匹配；`--help` 展示为长描述，`dws shortcut list` 输出 `intent` 字段。全部 366 条覆盖（`TestAllHaveIntent` 强制校验）。
- **参数收敛**：把裸 `dws mcp <svc> <tool> --json '{...}'` 的手拼 JSON，收敛成命名 flag（`--query`/`--group`…）。
- **内建校验**：required / enum 声明式校验，结构化错误提示。
- **风险确认**：read / write / high-risk-write 分级，写/删操作 `--yes` 前二次确认。
- **复用生产级 MCP 通道**：错误分类（auth/PAT/业务）、`--dry-run` 预览、`--format`/`--jq`/`--fields` 输出，全部免费继承（详见 §4）。

### 1.3 两个层次：1:1 封装层 vs 真·多步/智能层（重要澄清）

诚实区分——上面 298 条**绝大多数是 1 shortcut ≡ 1 个 MCP tool 的 1:1 封装**，本质是「给 MCP 套命名 flag + 校验 + Intent 的友好外壳」，价值在 DX 与 agent 可发现性，**不是新能力**。

真正的「shortcut 作为新能力」是 `internal/shortcut/smart/` 下的**多步/智能** shortcut——照 larksuite/cli 的实现范式（`CallAPITyped` 链式多步、按名解析 ID、Validate、DryRun 计划、失败回滚）落地。框架为此新增 `RuntimeContext.CallMCPData(product, tool, params)`（对应 lark 的 `CallAPITyped`：调用并返回 data 供下一步，跨服务）。

已落地的真·智能 shortcut（`internal/shortcut/smart/`，共 68 条，下表为代表性节选）：

| shortcut | 多步/智能逻辑 | 验证 |
|----------|--------------|------|
| `chat +dm --to <姓名> --text` | 搜人→解析唯一 userId→发单聊；多人消歧 | ✅ dry-run 真机 |
| `contact +lookup --name <姓名>` | 搜人→解析 userId→取完整资料 | ✅ **真机端到端** |
| `todo +assign --to <姓名> --task` | 解析人→建待办并把 TA 设为执行人 | ✅ dry-run 真机 |
| `chat +send-to-group --group <群名> --text` | 按群名搜群(search_groups)→消歧→发消息 | ✅ 编译/挂载 |
| `calendar +book --title --start --end [--with <姓名CSV>]` | 建日程→按名加参与者→**失败回滚删日程**（对标 lark `calendar +create`） | ✅ dry-run 真机 |
| `calendar +free --who <姓名> --start --end` | 解析人→查其时段忙闲 | ✅ **真机端到端**（解析 202397→查忙闲） |
| `chat +broadcast --to <姓名CSV> --text` | 多名逐一解析→群发单聊，失败汇总不中断 | ✅ 编译/挂载 |
| `minutes +latest-minutes` | 列妙记→取最新一条详情 | ✅ 编译/挂载 |
| `chat +group-members --group <群名>` | 按群名搜群→列群成员 | ✅ 编译/挂载 |
| `contact +org --name <姓名>` | 解析人→取详情拿 deptId→查部门详情 | ✅ **真机端到端**（3 步：董鑫阳→模型算法/16人） |
| `calendar +suggest-time --with <姓名CSV>` | 解析多人→推荐可开会时间 | ✅ 编译/挂载 |
| `calendar +invite --event <id> --with <姓名CSV>` | 解析多人→加入已有日程 | ✅ 编译/挂载 |
| `doc +share-doc --to <姓名> --url` | 解析人→把文档链接私信 TA | ✅ 编译/挂载 |
| `calendar +today` | 算出今天时间范围→列我今天的日程 | ✅ **真机端到端**（返回真实日程+参会人） |
| `calendar +next-event` | 近 7 天日程→按时间取最近一个 | ✅ 编译/挂载 |
| `contact +team --name <姓名>` | 解析人→取部门→列部门直接成员 | ✅ 编译/挂载 |
| `todo +remind --task --at` | 给自己建带截止/提醒时间的待办 | ✅ 编译/挂载 |
| `todo +todo-done --task <关键词>` | 列我的待办→按标题匹配→标记完成 | ✅ 编译/挂载 |
| `calendar +reschedule --event <id>` | 查日程详情→改时间（查→改机械多步） | ✅ 编译/挂载 |
| `wiki +wiki-new-doc --space <名>` | 按名搜知识空间→在其下建文档（跨 doc server 路由） | ✅ 编译/挂载 |
| `doc +doc-append --doc --text` | 文档末尾追加文本（update_document append 模式） | ✅ 编译/挂载 |
| `minutes +action-items` | 列妙记→取最新→取其待办事项 | ✅ 编译/挂载 |
| `minutes +detail --id <taskUuid>` | 一条命令聚合听记 basic/summary/keywords/transcript/todos，partial-failure 容错 | ✅ 全量测试 |
| `minutes +replace-batch --id --pair "原文=>替换"…` | 多组批量替换文字，去重校验+逐组结果聚合 | ✅ 全量测试 |

| `oa +approve-by --keyword` ★ | 列待审批→匹配→取 taskId→通过（钉钉原生，lark 无） | ✅ 编译/挂载 |
| `attendance +my-attendance` ★ | 当前用户→算今天→查我打卡（路由 attendance-wukong server） | ✅ 编译/挂载 |
| `todo +overdue` | 列我待办→本地过滤过期→投影输出 | ✅ **真机端到端** |
| `report +report-latest` ★ | 列我日志→取最新→取详情 | ✅ 编译/挂载 |
| `aitable +find-record --base --table` | 表内按关键词查记录 | ✅ 编译/挂载 |

另有 gap-buildable 补齐（批6）：`chat +my-groups`（列群+类型过滤+投影）、`calendar +find-room`（时段找可用会议室）、`minutes +minutes-search`（关键词搜妙记）、`mail +search-mail`（搜邮件+自动解析绑定邮箱）、`drive +find-file`（搜钉盘文件+投影）。

批7-8 续补（10 条）：`chat +at-me`（近期@我）、`calendar +cancel-event`（查→删，高危二次确认）、`todo +assign-multi`（多人指派）、`contact +dept-members`（搜部门→列成员）、`minutes +transcript`（最新妙记逐字稿）、`calendar +week`（本周日程）、`contact +by-mobile`（手机号→资料）、`todo +created-todos`（我创建的）、`chat +unread-chats`（未读会话）、`mail +unread-mail`（未读邮件）。

批9 续补（3 条·手工，对齐 gap-buildable）：`minutes +detail`（单命令聚合一条听记的 basic/summary/keywords/transcript/todos，partial-failure 容错）、`minutes +replace-batch`（多组 `原文=>替换` 批量替换 + 去重校验 + 逐组结果聚合，补齐一次一组的 1:1 `+word-replace`）、`aitable +record-share-links`（>20 条记录分享链接：去重+分片(≤20/批)+跨 `aitable-helper` server fanout+合并，补齐单批 20 条上限）。

批10 续补（3 条·多 agent 并行，对齐 gap-buildable）：`chat +thread-replies`（拉某条话题消息的全部回复 list_topic_replies + sender/text/time 投影）、`todo +related-tasks`（creator+executor+participant 三角色并集「与我相关的待办」+ taskId 去重 + 投影）、`doc +find-doc`（按关键词搜云文档 search_documents + title/url/type/token 投影）。

批11 续补（3 条·多 agent 并行）：`aitable +resolve-base`（按名搜 Base 解析 baseId，0/1/多候选消歧 search_bases）、`chat +chat-messages`（群/单聊会话消息列表，list_conversation_message_v2 / list_individual_chat_message 互斥+投影）、`mail +find-mail-user`（按名/邮箱搜企业邮箱联系人 search_mail_users + 投影）。

批12 续补（3 条·多 agent 并行，dws 原生 resolver 层，按名解析 ID）：`wiki +resolve-space`（search_wikiSpaces 名→spaceId）、`aitable +resolve-table`（get_tables 在 Base 内名→tableId，本地匹配）、`contact +resolve-dept`（search_dept_by_keyword 名→deptId，含数值 ID 兼容）。均 0/1/多候选消歧，对标 `resolveUser` 的各资源版。

批28 续补（3 条·净新增便利读，dws 原生）：`oa +pending`（**只读**列待我审批，区别于会审批的 +approve-by）、`todo +due-today`（今天到期待办，planFinishDate 服务端过滤，区别于 +overdue 已过期）、`calendar +tomorrow`（明天日程，复用 +today/+week 投影）。均只读、真机验证（+tomorrow 返回真实明日日程；+pending/+due-today 空路径正确且复用已验证 helper）。

批29 续补（3 条·净新增便利读，dws 原生）：`oa +done-approvals`（我已处理的审批历史 get_done_tasks；真机抓到并修复 pageSize=0 → 默认 20 的后端报错 bug）、`mail +recent-mail`（近期收件箱会话 list_mailbox_threads + 解析绑定邮箱/收件箱）、`attendance +this-month`（本月打卡 query_check_record on attendance-wukong，复用 +my-attendance 自身解析）。真机：+this-month 返回有效空、+done-approvals 修后走空路径、+recent-mail 正确报未绑定邮箱。

批30 续补（1 条·净新增便利读）：`contact +me`（当前用户 get_current_user_profile + 投影 {name,userId,mobile,dept,org,email}，agent 的「我是谁」；区别于 1:1 +get-self 吐冗长 raw，真机验证 董鑫阳/202397/模型算法）。

批31 续补（1 条·净新增便利读）：`calendar +my-free`（我自己的忙闲，自动解析当前 userId，默认今天，复用 +free 的 freebusySlots 投影；无需像 +free 传别人姓名，真机验证返回今日忙碌时段）。

批32 续补（1 条·净新增 dws 原生编排，lark 也没有）：`calendar +conflicts`（检测某天日程时间冲突/双重预订，list_calendar_events + 本地两两重叠检测，默认今天/--in-days；真机验证抓到今日 2 处真实冲突）。这类纯 MCP-tool 的本地编排是复杂写死胡同之外仍有价值的方向。

批33 续补（1 条·净新增 dws 原生编排，+conflicts 的互补品）：`calendar +free-slots`（找某天工作时段内的空闲时段"什么时候能安排会"，list_calendar_events + 合并忙碌区间 + 工作窗口内求补集，默认今天 09:00-18:00/--from/--to/--in-days；真机验证今日 4 段空档）。

共 **68 条真·智能 shortcut**（多批多 agent 工作流并行生成 + 手工续补）。★=钉钉原生编排，lark 完全没有。

**真机验证（登录态抽样，返回真实数据）**：`calendar +today/+week`（真实日程+投影）、`contact +org`（3 步→部门详情）、`contact +lookup/+free`、`todo +overdue`、`attendance +my-attendance` 等端到端可用。

### 深度对齐矩阵（逐条分析 lark 361 条 shortcut）

见 [`shortcut-lark-alignment.md`](shortcut-lark-alignment.md)——12 agent 逐条深读 lark 每个 shortcut 的智能实现（Validate/DryRun/ID解析/投影/多步/分页），映射钉钉：

| dws_status | 数量 | 含义 |
|---|:---:|---|
| covered-1to1 | 144 (40%) | lark 组合在钉钉塌缩成 1:1，封装层已覆盖 |
| no-dingtalk-tool | 127 (35%) | 钉钉无对应工具，客观不可对齐 |
| **gap-buildable** | **42 (12%)** | 钉钉有工具、值得补成智能 shortcut（建设目标） |
| covered-smart | 48 (13%) | 已建智能 shortcut / 部分覆盖 |

**框架系统性能力已对齐 lark**：`resolveUser`（名→ID）· `CallMCPData`（多步取数）· `rt.Output`（输出投影）· `rt.MutuallyExclusive/AtLeastOne/ExactlyOne/RangeInt/RequireAll`（跨字段校验）。

### 保真度升级（对齐 lark 96% 的输出投影）

lark 96% 的 shortcut 都做**输出投影**（把原始 API 返回精简为干净字段列表）。已给 **~60 条列表/读类封装**升级到此保真度——从 `rt.CallMCP`（打印原始 MCP 返回）改为 `rt.CallMCPData` + 防御式投影 + `rt.Output`（自动吃 `--format/--jq/--fields`）：

`contact +search-user/+search-mobile/+list-roles/+list-sub-depts` · `todo +get-my-tasks/+list-sub` · `calendar +book-list/+attendee-list` · `drive +list` · `wiki +node-list` · `chat +conversation-list/+category-list/+messages-list-unread-conversations/+messages-list-pin` · `doc +search/+list` · `mail +tag-list/+contact-list` · `aitable +base-list/+base-search` · `oa …`

示例：`contact +search-user` 由原始 MCP 返回 → 干净 `{count, users:[{name,userId,flowerName,openDingTalkId,title}]}`（真机验证）。这是把封装层往 lark 高保真水平系统性拉升的开始。另有 `mail +to`（按名发邮件）被 agent **正确 skip**——钉钉 contact 无 email 字段、mail `send_email` 需发件人邮箱 `from` 无法解析，宁缺勿错不编造。关键复用：「按名解析人」抽成共享 helper `resolveUser`（对标 lark `ResolveOpenIDsTyped`，带 0/多人消歧，不瞎猜）；多步靠 `CallMCPData`（对标 lark `CallAPITyped`）。其中 5 条由多 agent 工作流并行生成——各自以 helper 为 ground truth 研究参数、`send-to-group` 的 agent 还主动纠正了「按群名搜群」的正确工具（`search_groups` 而非按成员昵称的 `search_common_groups`）。

> 定位：1:1 层是「MCP 友好外壳」，smart 层才是「真 shortcut」。二者不混淆。

---

## 2. 测试验证报告

**验证理念**：写/删命令不能真跑（会发消息、解散群、删数据），故用**假 Caller 拦截**——让每条命令（含写/删）真实走完「解析→校验→确认→组装 MCP 调用」全流程，捕获组装出的 `(product, tool, params)`，只是不真发网络。以此对**全部 366 条**做零副作用验证。

### 2.1 结果总览（全绿）

| 验证项 | 范围 | 结果 |
|------|------|------|
| `go build ./...` | 全仓 | ✅ 通过 |
| `gofmt -l` / `go vet` | shortcut 全包 | ✅ 0 未格式化 / 0 告警 |
| 框架单元测试（5） | 类型/挂载/校验/分组 | ✅ 全通过 |
| **TestAllShortcutsAssemble** | **全部 366 条** | ✅ 322 组装真实 MCP · 44 自校验拦截 · **0 失败 · 0 panic** |
| **TestAllToolLiteralsAreReal** | 全部 tool 字面量（逐条） | ✅ **0 编造**（tool 名逐一比对 helper ground truth） |
| TestNoDuplicateCommands | 全部 366 条 | ✅ 无重复、命名规范（均 `+` 前缀） |
| **TestAllHaveIntent** | 全部 366 条 | ✅ 每条均有自然语言 Intent 描述（无一遗漏） |
| usage 包单测（4） | 埋点/脱敏/聚合/开关 | ✅ 全通过 |
| app 包全量回归 | `internal/app` | ✅ 72.2s 通过（接线未破坏任何现有命令） |
| 只读命令真机验证 | ~25 条（登录态，见 §2.4） | ✅ `contact +me`/`+org`/`+lookup`、`calendar +today/+week/+free/+conflicts/+free-slots`、`doc +find-doc`、`aitable +resolve-base/+resolve-table`、`drive +find-file`、`oa +my-initiated` 等端到端返回真实数据、投影核对 |

### 2.2 关键指标解读

- **322 「组装真实 MCP」**：喂合成参数后成功组装出 MCP 调用，且 tool 名经 helper ground truth 核验真实、非编造。
- **44 「自校验拦截」**：这些命令有结构化/JSON/互斥输入（如多维表建记录需 JSON、DING 三选一接收人），dummy 值被其**自身校验正确拒绝**——证明校验链路健全。其 tool 名由静态测试 `TestAllToolLiteralsAreReal` 单独覆盖，无遗漏。
- **0 编造 / 0 panic / 0 失败**：无幻觉工具名，无运行时崩溃，无死命令。

> ⚠️ **验证强度分层（诚实口径，勿把"全绿"读成"真机全对"）**：`TestAllShortcutsAssemble` 的"0 失败"只证明**能正确组装 MCP 调用、零副作用**——它用**合成响应**，**验证不了防御式投影是否匹配真实响应结构**（真机验证正是靠这个抓到过 deptList 容器、pageSize=0 等 assemble 盖不住的 bug，见 §2.4）。按真机验证强度分三层：**(A) 真机正向验证** ~25 条只读/解析类（返回真实数据、投影核对）；**(B) 仅 assemble + 复用已验证 helper**（如 +due-today/+this-month 等，逻辑同构于已验证命令，但该条本身未在真机跑出正样本）；**(C) 未对真实后端跑过**——17 条写类 smart（不宜真跑，会发消息/建数据）、6 条 minutes smart（该 org 未开 CLI 数据访问）、mail +recent-mail（无绑定邮箱）。(C) 类**很可能仍有 assemble 盖不住的投影/参数 bug**，不应因"全绿"就当作"真机可用"。

### 2.3 防幻觉机制（工作流生成时）

生成阶段每个服务由独立 agent 负责，硬性规则：tool 名与参数 key **只能逐字取自 dws helper 的真实调用点**，无法确定参数的 tool 主动跳过并记录原因（如嵌套对象、时间戳转换、本地文件分片上传）。测试阶段再用 ground truth 二次核验，双重保险。

### 2.4 真机验证战役（登录态打真实钉钉后端）

assemble 测试用**合成响应**，能验证「调用是否组装正确」，但验证不了「防御式投影解析是否匹配真实响应结构」。为此做了 9 批真机验证（登录态 corp「钉钉」，token 有效期内），把只读/解析类 smart shortcut 打真实后端、逐条核对投影输出。

**正向验证 20+ 条**（返回真实数据、投影正确）：`doc +find-doc`、`aitable +resolve-base`/`+resolve-table`/`+list-tables`/`+base-list`/`+find-record`、`mail +find-mail-user`、`chat +my-groups`/`+group-members`/`+at-me`、`contact +org`/`+lookup`、`calendar +today`/`+week`/`+next-event`/`+free`/`+suggest-time`、`todo +overdue`/`+related-tasks`、`attendance +my-attendance`、`report +report-latest`、`oa +my-initiated`、`drive +find-file`、`wiki +resolve-space` 等。

**真机抓到并修复 8 个真实问题**（assemble 测试抓不到，只有真机能抓）：

| shortcut | 真机发现的问题 | 修复 |
|---|---|---|
| `contact +resolve-dept` | 对任何真实部门名都「未找到」——真实响应容器 key 是 `deptList`（防御探测清单漏了），deptName 带 `<red>` 高亮、deptId 是数值 | 加 `deptList` 探测 + `stripHighlightTags` + 数值 coerce |
| `contact +dept-members` | 消歧消息泄漏 `<red>` 标记 | 复用 `stripHighlightTags` |
| `chat +unread-chats` | 每行吐 `unread: null`（底层不返回每会话未读数） | 仅当有值才带该字段 |
| `chat +at-me` | 直吐原始两层嵌套、未拍平 | 新增 `atMeFlattenGroups`，拍平 43 条为 `{conversation,sender,text,time}` |
| `chat +group-members` | 终结步 raw `CallMCP` 吐冗长 raw（含 avatar 媒体 ID + errorCode 噪音） | 升级 `CallMCPData`+投影 `{name,nick,role,openDingtalkId}` |
| `calendar +free` | 吐冗长 `result[].scheduleItems[].{start,end}.dateTime` 嵌套 | 投影为 `{who,userId,free,busy:[{start,end}]}` |
| `calendar +today` | 吐 17 字段冗长事件（含完整 attendees 数组），与 `+week` 不一致 | 投影为 `{title,start,end,location,eventId}`，对齐 `+week` |
| `calendar +suggest-time` | `timeConflictAttendees:[null]` 噪音 + result 包裹 | 拍平 + 丢 null 冲突 → `{suggestions:[{start,end}]}` |

**后端受限、无法真机正向验证的（非代码问题）**：`chat +chat-messages`/`+search-msg`（读会话消息需更高 PAT 权限 / org 未开 CLI 数据访问）、`minutes +*`（org 未开 CLI 数据访问 `TOKEN_VERIFIED_FAILED`）、`contact +team`（列部门成员 medium-risk 权限墙）。这些命令的**组装链路**经 assemble 测试验证正确，仅无法在本环境跑通后端。

**一个已澄清的非 bug**：`resolveUser` 对组织内成员（如董鑫阳→userId 202397）真机端到端正常；对外部/资料受限联系人 `search_contact_by_key_word` 只返回 openDingTalkId（name/userId 全 null），此时正确报「没找到」而非瞎猜——钉钉数据模型现实，非代码缺陷。

> **结论**：真机验证证明「防御式多候选 key 投影」在真实响应上整体成立，并纠正了 8 处「合成测试盖不住」的解析/投影偏差。这是把可用性从「组装正确」提升到「真机输出正确」的关键一环。

---

## 3. 效果评估 GSB（vs lark-cli）

以 lark-cli 各服务领域为基准，评估 dws shortcut 的相对表现。**G**ood=优于/更全，**S**ame=持平，**B**ad=弱于/缺失。

> ⚠️ 重要前提：两边是**不同 API**（飞书 vs 钉钉），数量不能机械 1:1；覆盖度受钉钉实际能力约束。
>
> **口径（prune 后重做）**：不再用 shortcut 数（会因去冗余失真），改用**「dws 该服务实际暴露的 distinct MCP tool 数」= helper 命令 ∪ shortcut 覆盖的 tool 合集**——这才代表真实能力，与 helper/shortcut 怎么分层无关。对比 lark 的 shortcut 数（不同 API，只作量级参考）。

| lark 服务 | lark 数 | dws 对应 | dws tool 覆盖 | (其中 shortcut 补) | GSB | 说明 |
|-----------|:---:|---------|:---:|:---:|:---:|------|
| im | 21 | chat | **95** | +3 | 🟢 G | 群/消息/机器人能力更全 |
| mail | 21 | mail | **43** | +0 | 🟢 G | 覆盖更广（几乎全由 helper 提供，shortcut 曾重复、已 prune） |
| doc | 14 | doc | **34** | +4 | 🟢 G | 块级读写更细 |
| minutes | 14 | minutes | **25** | +0 | 🟢 G | 录音控制/说话人更全 |
| calendar | 12 | calendar | **24** | +5 | 🟢 G | 会议室/ACL/忙闲；shortcut 另补排期智能 |
| contact | 2 | contact | **15** | +0 | 🟢 G | 部门/角色/花名册更全 |
| wiki | 12 | wiki | **16** | +0 | 🟢 G | 略优 |
| base | 87 | aitable | **79** | **+63** | 🟡 S | 基本持平；**helper 仅 16，shortcut 层补齐了绝大部分**（真·gap-fill） |
| task | 18 | todo | **20** | +1 | 🟡 S | 持平 |
| drive | 26 | drive | **25** | +4 | 🟡 S | 基本持平 |
| apps | 63 | devapp | **25** | **+25** | 🔴 B | **helper 无 devapp 命令、25 个全由 shortcut 提供**（纯 gap-fill），但仍少于 lark |
| sheets | 84 | sheet | **60** | +1 | 🔴 B | 钉钉表格 MCP 较少；helper 已覆盖，shortcut 曾重复、已 prune |
| vc | 18 | — | 0 | — | 🔴 B | 钉钉 conference 无干净 MCP tool |
| okr | 13 | — | 0 | — | 🔴 B | 钉钉无对应能力，**不可对齐** |
| slides / markdown / whiteboard / note / event | 17 | — | 0 | — | 🔴 B | 钉钉无对应能力，**不可对齐** |

**dws 独有（lark 无对应服务）**：oa 审批(~20 tool) · attendance 考勤(~38) · report 日志(~7) · ding(~8) · aisearch/live/devdoc（helper 层提供）——钉钉工作流核心，构成差异化。

### GSB 汇总

- 🟢 **G（7 领域）**：im/mail/doc/minutes/calendar/contact/wiki——dws tool 覆盖更全。
- 🟡 **S（3 领域）**：base/task/drive 量级持平（base 的持平**几乎全靠 shortcut 层 gap-fill**，helper 只有 16）。
- 🔴 **B（受限 2 + 不可对齐 6）**：apps/sheets 少于 lark（sheets 受钉钉 API 限，apps 全由 shortcut 补但仍少）；vc/okr/slides/markdown/whiteboard/note/event 客观不可对齐（非遗漏）。
- **注**：这张表也印证了 1:1 层的真实价值分布——**base/apps 靠 shortcut 补了大量 helper 没有的 tool（gap-fill），而 mail/sheets 的 shortcut 基本是重复 helper（已 prune）**。

---

## 4. dws 差异化于 lark 的优势

### 4.1 执行底座：复用生产级 MCP 通道（架构性优势）
lark-cli 每个 shortcut 直连飞书 SDK，错误处理/输出各自实现。dws shortcut 的 `CallMCP` **委托统一的 MCP 调用路径**，一步继承：
- **错误分类**：auth 过期 / 未登录 / PAT / 业务错误，自动给出可执行提示；
- **`--dry-run` 预览**：不发网络，输出将执行的 tool + 参数；
- **`--format`/`--jq`/`--fields`**：机器可解析输出，Agent 友好。

lark 需在每个命令重复实现这些；dws 由框架统一注入，一致性与维护成本双赢。

### 4.2 覆盖更全的高频协作能力
按 **dws tool 覆盖**（helper∪shortcut，§3 口径）：chat 95 / mail 43 / doc 34 / minutes 25 / calendar 24 等即时协作场景，多为 lark 对应服务的 2–4 倍。

### 4.3 钉钉原生特有能力（lark 完全没有）
审批 oa、日志 report、考勤 attendance、DING、企业智能搜索 aisearch —— 这些是钉钉工作流的核心，构成差异化护城河。

### 4.4 高频自动沉淀（P2，lark 无此设计）
基于用户高频使用**主动把常用操作沉淀为自定义 shortcut**（`~/.dws/shortcuts/*.yaml` 运行时加载），让 CLI 越用越顺手。埋点**默认关（opt-in，`DWS_USAGE_TRACKING=1` 开启）**+开启后首跑告知，隐私优先（记形状不记值）。详见 [P2 设计](shortcut-p2-design.md)。**lark-cli 无任何等价能力。**

### 4.5 AI Agent 友好
`--yes` 跳过确认、结构化错误、`--dry-run` 预览、`--print-schema`（规划中）——为 Agent 自动化调用而设计。

### 4.6 工程质量：全量自动化测试
假 Caller 拦截，对全部 366 条（含写/删）做零副作用验证 + tool 名 ground truth 核验，可复跑、可回归。

---

## 5. 复盘与后续（loop 持续项）

### 5.0 关键复盘：1:1 层去冗余（511 → 298）
**问题**：建 1:1 shortcut 层之前，**没有先摸清 `internal/helpers/` 已经把哪些 MCP tool 封装成了 `dws <svc> <verb>` 产品命令**。结果对 **213 个 tool 重复封装**——同一个 tool，helper 有 `dws contact user search`、我又造了 `dws contact +search-user`，且这批无投影增量，纯重复。
**纠偏**：按「tool 已被 helper 封装 且 shortcut 用 CallMCP 无投影」精确删除 213 条，1:1 层 511 → **298**（保留 233 填空白 + 65 有投影），总数 579 → **366**，服务 19 → **16**（aisearch/live/devdoc 整包移除）。全绿、真机复验保留命令仍可用。
**教训**：**做封装层前先审已有封装**。对齐 lark「每一条 shortcut」时，应先问「dws 这边是不是已经有等价命令了」，而不是无脑对齐。这是本项目最大的方法论盲点。

1. ~~补 apps（↔devapp）~~ ✅ 已完成：新增 30 个 devapp shortcut。
2. **P2 落地**（差异化能力，lark 无）：
   - ✅ **P2-1 usage 埋点**：装饰 MCP 调用唯一必经点记录 `~/.dws/usage.jsonl`（**记形状不记值**，敏感/自由文本字段脱敏；**默认关/opt-in**，`DWS_USAGE_TRACKING=1` 开启、开启后首跑告知）。端到端验证通过。
   - ✅ **stats/list**：`dws shortcut list [--service]`、`dws shortcut stats [--top N] [--purge]`（按 `(product,tool,arg_keys)` 聚合、识别固定值 fixed_args）。
   - ✅ **P2-2 suggest**：`dws shortcut suggest [--min N]` 把高频分组转成「建议沉淀的 +command」候选（含固定/可变参数拆分）。
   - ✅ **P2-3 YAML 自定义 shortcut 沉淀闭环**：`dws shortcut add` 写 `~/.dws/shortcuts/*.yaml` → 下次运行 `userdef.Load()` 编译成 Shortcut 注册（复用同一 runner；`${flag}` 绑定+常量；与内建冲突自动跳过）。**端到端验证通过**：add→重载→`dws <svc> +<cmd>` 可用，dry-run 组装正确。
   - ⏳ **P2-4 nudge**：命中高频候选时主动提示（TTY、频控、可永久关闭）。

   > 至此，**「高频使用 → 建议 → 一键沉淀 → 自定义 shortcut 运行时生效」完整闭环已打通**——这是 lark-cli 完全没有的差异化能力。
3. **深化 sheets**：随钉钉表格 MCP 能力增强补齐。
4. **实机全读回归**：登录态下对全部只读 shortcut 做真实调用回归（需有效 token + 真实资源 ID）。
5. **不可对齐项归档**：okr/slides/whiteboard/note/vc/event 明确标注为钉钉无能力，避免误解为遗漏。

---

## 附：一句话结论

> dws 已把钉钉侧**能对齐的主要服务全部对齐**（16 服务 / **366 shortcut** = 298 封装 + 68 智能编排），在即时协作能力上显著优于 lark，并拥有审批/考勤/日志/DING 等钉钉原生差异化能力与「高频自动沉淀」独有设计；受限项均为钉钉客观无对应能力，非工程遗漏。全部 366 条通过零副作用全量自动化验证。
