# Mail Shortcut 下游业务能力需求规格

> 日期：2026-08-18
> Rebased executable 基线：`3fc3be37c67d14f60273a702a7a6b38f6ba32d4c`；最终 clean PR HEAD 的 live SHA 与发布复核结果记录在 PR 证据中
> 对比基线：lark-cli 1.0.87
> 范围：Shortcut only；不改 `skills/multi` 或 `skills/mono` 的路由、流程或业务逻辑。仓库 policy 强制的可见 Shortcut 自动生成块单独机械同步。
> 发布属性：仓库安全版本；不包含真实邮箱、人员、组织、邮件内容、资源 ID 或请求标识。

## 1. 执行摘要

本轮对 18 个 Mail Shortcut 完成严格 success、固定集合路径、稳定 ID、分页完整性和统一 Result 收口。8 个公开只读入口已在相同 runtime tree 逐条完成 Shortcut 与原子层的真实数据双层复核；`+unread-mail`、`+recent-mail`、`+thread-list`、`+tag-list`、`+template-list`、`+contact-list` 因缺少可控 guaranteed-zero fixture 保持 Agent-unavailable，但为守住既有 argv/Help 合同继续以 compatibility-visible 形式留在 CLI；4 个草稿/模板写入口因无法证明清理终态同样不进入公开 Catalog。

上述 6 个 compatibility-visible 入口在完整 Schema 中保留历史 `availability=available` 与既有 workflow property，仅表示旧调用仍可执行；其 Shortcut 语义状态仍为 `public=false/unavailable`，默认 Shortcut 列表与 Agent public Catalog 均不发布。底层 `folderId`、`size` 等 MCP 字段继续由 Execute 显式适配，不能在未经过版本化迁移时改写已发布 Schema property。

仍不能诚实对齐的任务集中在草稿/模板清理终态、发送终态、回复/转发草稿语义、批量修改/删除逐项结果、回执、签名、事件监听、模板附件事务和联系人创建身份回执。它们不是再包一层 Shortcut 就能解决，需要下游业务接口或安全测试 fixture 补足可验证合同。

| ID | 优先级 | 类型 | 用户任务 | 当前状态 | 下游 Owner | 解锁的 Shortcut |
|---|---|---|---|---|---|---|
| `DS-Mail-001` | P0 | contract insufficient | 发信/发送草稿并确认最终投递 | partial | Mail service / adapter | `+send`、`+draft-send` |
| `DS-Mail-002` | P0 | missing capability | 回复、回复全部、转发默认保存草稿 | partial | Mail service | `+reply`、`+reply-all`、`+forward` |
| `DS-Mail-003` | P0 | contract insufficient | 批量修改、移动、软删除邮件 | partial | Mail service / adapter | `+message-modify`、`+message-trash` |
| `DS-Mail-004` | P1 | missing capability | 处理已读回执与邮箱签名 | unavailable | Mail service | `+send-receipt`、`+decline-receipt`、`+signature` |
| `DS-Mail-005` | P1 | missing capability | 持续监听新邮件 | unavailable | Event + Mail service | `+watch` |
| `DS-Mail-006` | P1 | contract insufficient | 带附件/内联图片的模板创建更新 | partial | Mail + Drive adapters | 完整 `+template-create/update` |
| `DS-Mail-007` | P1 | adapter defect | 创建联系人并取得稳定身份 | blocked | Mail adapter | `+contact-create/update/delete` |
| `DS-Mail-008` | P1 | adapter defect | 一致的成功、空结果与分页合同 | partial | Mail adapter | 全部 list/search Shortcut |
| `DS-Mail-009` | P1 | tenant-or-fixture | 安全验证发送、回执、分享和监听 | blocked | Product QA / tenant admin | 全部高影响 Mail Shortcut |
| `DS-Mail-010` | P0 | contract insufficient | 草稿/模板可证明的清理终态 | blocked | Mail service / adapter | `+draft-create/edit`、`+template-create/update` |

## 2. 用户任务与能力缺口总览

| 用户任务 / Golden Route | DWS Shortcut | Lark CLI 对应 | 当前能力 | 缺口分类 | 临时处置 |
|---|---|---|---|---|---|
| 浏览/筛选摘要 | `+triage`、`+search-mail` | `+triage` | covered | 无 | 公开，严格分页 |
| 固定未读/近期列表 | `+unread-mail`、`+recent-mail` | Lark 对应任务入口 | blocked | 固定查询/文件夹缺可控 guaranteed-zero fixture | 保持 unavailable |
| 读取一封、多封、会话 | `+message`、`+messages`、`+thread` | 同名入口 | covered | 无 | 公开，精确 ID 读回 |
| 新建/编辑草稿 | `+draft-create`、`+draft-edit` | 同名入口 | blocked | 两次 batch-delete 后同 ID 仍可读，无法证明零残留 | 保持 unavailable |
| 创建/更新基础模板 | `+template-create`、`+template-update` | 同名入口 | blocked | delete 后 get 没有 typed nonfound；from/isDraft 也不可读回 | 保持 unavailable |
| 发送新邮件/已有草稿 | 无公开 Shortcut；存在 raw send | `+send`、`+draft-send` | partial | 终态、逐项结果、幂等不足 | 保持 raw，不宣称对齐 |
| 回复/回复全部/转发 | 无公开 Shortcut；raw 路径会立即发送 | `+reply`、`+reply-all`、`+forward` | partial | 缺少默认草稿与邮件头保真合同 | 保持 raw，不宣称对齐 |
| 修改/删除邮件 | 无公开 Shortcut；存在 raw batch route | `+message-modify`、`+message-trash` | partial | 无逐项 ledger 和严格终态 | 保持 raw，不宣称对齐 |
| 发送/拒绝已读回执 | 无 | `+send-receipt`、`+decline-receipt` | unavailable | 专用业务接口与标签合同缺失 | 明确不可用 |
| 邮箱签名 | 无 | `+signature` | unavailable | 签名读取接口缺失 | 明确不可用 |
| 分享邮件到聊天 | raw 高风险入口 | `+share-to-chat` | partial | 缺安全 fixture、逐目标结果与读回 | 不公开 Shortcut |
| HTML lint | 无 | `+lint-html` | unavailable | 缺统一邮件 HTML 规则包 | 下游或本地规则能力需求 |
| 监听新邮件 | 无公开 Mail Shortcut | `+watch` | unavailable | 订阅生命周期和安全事件合同不足 | 不公开 Shortcut |
| 文件夹/标签/联系人/企业邮箱用户 | `+folder-list`、`+user-search`、`+find-mail-user` 公开；其余列表不公开 | 无同名任务入口 | partial DWS extra | 标签/模板/联系人/会话列表缺安全双态 fixture | 无双态证据的入口保持 unavailable |

## 3. 下游需求明细

### `DS-Mail-001` — 可验证的发送生命周期

- 用户任务：发送新邮件或一个/多个草稿，并知道每一封最终是成功、失败、部分成功还是状态未知。
- 当前证据：raw 发送可返回业务 success 或发送标识，但不能统一证明最终投递；批量草稿发送没有逐项 ledger、请求顺序、未知提交和安全重试合同。
- 所需接口合同：
  - 创建/发送必须返回稳定 `messageId` 与 `internetMessageId`，并明确 `accepted/pending/sent/partial_failure/failure/unknown`。
  - 提供按同一身份查询发送状态的接口；状态必须绑定请求邮件与收件人集合。
  - 批量发送返回逐项结果，任何一项失败时整体不得退出 0 冒充全成功。
  - 支持幂等键，或明确 unknown commit 不可自动重试。
  - 失败错误区分参数、权限、风控、限流、收件人拒收和提交未知。
- 验收：安全自发自收 fixture 完成 draft-create → exact get → send → 状态终态 → sent-folder exact read；批量中注入一项失败，验证 ledger 与非零整体结果；清理无测试草稿残留。

### `DS-Mail-002` — 回复/转发的草稿优先与 MIME 保真

- 用户任务：回复、回复全部或转发一封邮件，默认保存草稿，只有再次确认才发送。
- 当前证据：DWS raw route 会创建回复/转发草稿后立即发送，无法对齐 Lark 的默认草稿语义；上游也无法证明 `In-Reply-To`、`References`、原始引用块和收件人集合正确。
- 所需接口合同：
  - 独立 `create_reply_draft`、`create_reply_all_draft`、`create_forward_draft`，返回稳定草稿 ID，不隐式发送。
  - 服务端生成并可读回线程关系头、回复全部去重后的 To/CC、转发引用块和附件继承结果。
  - 发送必须复用 `DS-Mail-001` 的确认、终态和幂等合同。
- 验收：用隔离自发邮件分别创建三类草稿，精确 ID 读回核对父邮件、参与人集合和引用语义；未确认时远程发送调用为 0；确认发送后状态终态可验证。

### `DS-Mail-003` — 邮件修改、移动和删除的逐项终态

- 用户任务：批量标记已读/未读、增删标签、移动文件夹、软删除邮件。
- 当前证据：raw batch route 多数只给聚合 success；删除后邮件仍可能可读，无法区分“移入已删除文件夹”“永久删除”“延迟可见”或“未生效”。
- 所需接口合同：
  - 每个输入 messageId 返回 `applied/already_applied/failed/unknown` 与稳定原因码。
  - 修改/移动后详情或摘要必须可读回 `isRead/tags/folderId`；删除返回明确 tombstone 或 folder transition。
  - 软删除和永久删除使用不同操作，危险级别与确认要求可声明。
  - 任何部分失败整体 outcome 为 `partial_failure` 且进程非零。
- 验收：创建隔离邮件，执行 mark-unread/read、标签增删、移动与软删除，每步同 ID 读回；错误 ID 与合法 ID 混合时逐项 ledger 完整且整体非零。

### `DS-Mail-004` — 已读回执与签名

- 用户任务：识别邮件是否请求回执；确认后发送标准回执，或拒绝并清除提示；列出和查看默认签名。
- 当前证据：现有 Mail 接口没有稳定暴露回执请求标签、专用发送/拒绝操作或签名读取资源，上游无法安全组合普通回复替代。
- 所需接口合同：
  - 消息详情公开稳定回执请求状态和请求者身份类型。
  - 专用 send/decline receipt 操作，幂等且返回状态；正文由服务端生成，不能让上游伪造。
  - 签名列表/详情返回稳定 ID、默认发送场景、HTML/文本内容和敏感字段标注。
- 验收：预置请求回执邮件，未确认零写调用；发送/拒绝后状态读回且重复调用幂等；签名已知非空与合法空均可证明。

### `DS-Mail-005` — 新邮件监听的订阅生命周期

- 用户任务：在限定时间内监听新邮件，得到稳定、可恢复、可去重的事件流。
- 当前证据：通用事件基础设施不能证明 Mail scope、订阅状态、ready marker、断线续传和消息读取权限形成完整任务链。
- 所需接口合同：订阅/查询/退订；明确 user/bot 身份、scope 和租户开关；ready marker；事件 `eventId/messageId/mailbox/time`；断线 cursor、去重和界限参数；心跳不冒充业务事件。
- 验收：隔离邮箱订阅后注入一封测试邮件，只收到一次并能以 messageId 精确读取；超时、权限缺失、断线重连和退订后零事件均有确定结果。

### `DS-Mail-006` — 模板附件与内联图片事务

- 用户任务：创建或更新含普通附件、内联图片和 HTML 的模板，同时保留未修改 MIME 结构。
- 当前证据：本轮只对齐名称、主题、正文核心字段；现有多步上传缺少模板级事务、附件稳定 ID、失败回滚和更新时的结构保真证明。
- 所需接口合同：创建/更新草稿会话、附件上传会话、content-id 映射、提交/取消；返回逐附件 ledger；更新提供版本或 etag，避免 last-write-wins 覆盖；失败可回滚且无孤儿文件。
- 验收：普通附件和内联图片各一，创建后按模板 ID 读取附件 ID/名称/大小/content-id；更新正文不丢附件；中途失败自动取消并证明零孤儿资源。

### `DS-Mail-007` — 联系人写操作的稳定身份

- 用户任务：创建、更新、删除个人邮件联系人并验证精确对象。
- 当前证据：真实 create 返回 `success=true` 但没有 contactId；上游只能用随机显示名再扫列表定位，无法用于一般用户输入，因为名称/邮箱可能重复。
- 所需接口合同：create 返回稳定 contactId；get-by-id；update/delete 返回同 ID 与版本；列表支持 exact email 或 ID filter；重复联系人规则明确。
- 验收：创建回执直接得到 ID，get-by-id 精确核对，更新同 ID，删除后 not-found/tombstone；重复邮箱和同名联系人有稳定结果而非猜测。

### `DS-Mail-008` — 统一成功、空结果与分页协议

- 用户任务：可靠地区分“确实没有结果”“还有下一页”“服务异常或响应漂移”。
- 当前证据：同一产品的 success 同时出现布尔和字符串；hasMore 也出现两种编码；搜索终页用 `$`，部分列表用空串；零命中邮件会返回 `total=0` 加一个只有空收件人字段的占位对象。当前租户又没有空邮箱或空邮件文件夹，不能为无筛选列表证明 guaranteed-zero。
- 所需接口合同：
  - success 与 hasMore 统一为布尔；所有列表显式数组，合法空只返回 `[]`。
  - 统一 `nextCursor` 与 `endpointExhausted`；终页不使用业务哨兵对象或魔法值。
  - 每项稳定 ID 必填；total 使用整数；服务错误必须 `success=false` 和稳定错误码。
  - 保留兼容期，但提供 capability/version 让上游安全切换。
- 验收：每个列表/搜索执行已知非空、保证零命中、坏 item、缺集合、错型、hasMore 无游标、重复游标；只有显式合法空成功。

### `DS-Mail-009` — 安全租户与真实 E2E fixture

- 用户任务：在不触达真实业务收件人和内容的前提下验证所有高影响 Mail Shortcut。
- 所需 fixture：隔离自发自收邮箱、可控第二收件人、回执请求邮件、可分享的测试聊天、安全事件订阅、测试签名、可回收附件；所有资源用随机无业务含义标记并有自动清理。
- 权限：最小 Mail read/write/event、Drive attachment、IM share scopes 分离；可测试 user/bot 差异和缺权限错误。
- 验收：stdout 只输出 PASS 标签与聚合计数；原始 JSON 只在临时目录；finally 清理；远端零测试草稿/模板/联系人/邮件/订阅残留；仓库和历史扫描无身份数据。

### `DS-Mail-010` — 草稿/模板可证明的清理终态

- 用户任务：用可回收 fixture 验证草稿与模板写 Shortcut，不留下无法确认的远端测试对象。
- 当前证据：草稿创建/更新回执和 exact-ID 读回成功，但同一 ID 连续两次 batch-delete 后仍可读；模板 delete 返回成功后，get 仅为未分类失败，既非 typed nonfound 也不能证明 tombstone。
- 所需接口合同：分离软删除与永久删除；返回稳定 ID、终态和幂等证据；get-by-id 对已永久删除对象返回稳定 `not_found/deleted` 错误或已审核 tombstone，不得空 body、通用失败或继续返回对象。
- 验收：create/update → exact-ID readback → permanent delete → exact Shortcut + raw get 双层 typed absence；有界轮询后仍可读或终态未知时整体非零，且不得发布 Shortcut。
- 临时处置：四个写 Shortcut 保持 `public=false` / `unavailable`，直到安全 fixture 与 typed absence 同时可证明。

## 4. Lark 对齐与平台差异

| Lark 用户任务 | 可精确对齐 | 平台差异 | DWS 推荐结论 |
|---|---|---|---|
| `+message` / `+messages` / `+thread` / `+triage` | yes | DWS 额外自动解析邮箱和收件箱，并严格发布完整性 | 已公开 |
| `+draft-create` / `+draft-edit` | blocked | 核心写回可证，但删除后同 ID 仍可读，无安全清理终态 | 不公开，保持 unavailable |
| `+template-create` / `+template-update` | blocked | 核心字段可读回，但 from/isDraft 不可验且删除后缺 typed nonfound | 不公开，保持 unavailable |
| `+send` / `+draft-send` | no | DWS raw 偏立即发送且缺统一终态/逐项 ledger | 暂不公开 Shortcut |
| `+reply` / `+reply-all` / `+forward` | no | DWS raw 会立即发送，Lark 默认保存草稿 | 暂不公开 Shortcut |
| `+message-modify` / `+message-trash` | no | 聚合 success 不足以证明逐项终态 | 暂不公开 Shortcut |
| `+send-receipt` / `+decline-receipt` | no | 缺专用接口和可验证标签 | platform unavailable |
| `+signature` | no | 缺签名读取资源 | platform unavailable |
| `+watch` | no | 缺完整订阅生命周期与安全 fixture | fixture + capability blocked |
| `+share-to-chat` | partial | raw 可调用但缺逐目标验证和安全 fixture | 保持 raw |
| `+lint-html` | no | DWS 未提供统一规则包 | downstream/local capability needed |

## 5. 超越 Lark 的产品机会

| 产品原生能力 | 可形成的 DWS Shortcut | 安全/验证要求 | 优先级 |
|---|---|---|---|
| 文件夹、标签与联系人目录 | `+organize`：规则化移动、标记与标签组合 | 逐项 ledger、写后读回、补偿恢复 | P1 |
| 收信规则、白名单、黑名单、自动回复 | `+inbox-policy-audit` | 只读汇总优先；写操作强确认和版本化 | P2 |
| 邮箱日历 | `+mail-calendar-conflicts` | 与主 Calendar 的 ownership boundary 明确，禁止双写 | P2 |
| 发送状态与召回 | `+delivery-audit` | 终态、收件人粒度、召回结果和不可逆提示 | P1 |
| 附件导出与分享 | `+archive-message` | 精确 messageId、原子本地写入、敏感路径与清理 | P2 |

## 6. 无需下游变更的上游修复

| Shortcut | 上游根因 | 已完成修复 | 回归证据 |
|---|---|---|---|
| 全部 list/search | 容忍式探测任意 result/data/list/items，坏元素静默丢弃 | 固定已观测路径、严格 success/数组/item/ID；无双态 fixture 的 leaf 不发布 | deterministic 响应矩阵；live 证据逐 leaf 记录，不作泛化 |
| `+search-mail` / `+triage` | `$` 终止游标被误作下一页；零命中占位对象被当邮件 | 明确 `$` 终页；仅窄规则归一化已观测哨兵 | 各完成 known-nonempty 20；3 个 fresh 零命中 raw 均为 `total=0` + 无稳定 ID/正文且收件字段全空的 reviewed sentinel + terminal cursor，exact 才归一化为显式 `[]`；不把该下游特例描述成 raw 空数组 |
| `+search-mail` / `+triage` 自动邮箱解析 | 严格化时只接受顶层对象数组，会拒绝历史已观测的字符串数组和 `result/data.emailAccounts` 包装 | 仅接受三个审核路径 `emailAccounts` / `result.emailAccounts` / `data.emailAccounts`，每项可为非空邮箱字符串或含非空 `email` 的对象；缺集合、错型、坏项或多路径冲突全部 fail-closed；空发件人也不再投影为空字符串成功 | top/result/data × string/object、blank/wrong/multiple-path 与 sender missing/null/wrong-type 回归覆盖；最终 live 未传 `--email` 执行 `+search-mail`/`+triage`，owning 响应为顶层 object-item 形态并成功解析 |
| `+unread-mail` / `+recent-mail` / `+thread-list` | 固定条件或文件夹不能保证零命中 | 严格响应代码已完成，但没有空邮箱/空文件夹证据时关闭发布 | BLOCKED fixture；不得修改真实邮件状态造空 |
| `+user-search` / `+find-mail-user` | `hasMore`/`nextCursor` 未交付；零命中被误报 validation error | 发布 complete/nextCursor；合法空成功 | 各完成 known-nonempty 20 + fresh raw 显式空；stable identity set 与 raw pagination/meta 精确一致；`+user-search` 同轮实跑历史 string `--limit` |
| `+tag-list` / `+template-list` / `+contact-list` | 无 query 的列表容易把末页/删除后列表误作合法空 | 严格响应代码已完成；无专用空邮箱和 typed cleanup 时关闭发布 | BLOCKED fixture；不把临时资源从列表消失记为零态 PASS |
| `+message(s)` / `+thread` | 缺任务层完整读取和身份绑定 | 自动邮箱解析、精确请求 ID 读回、保序多读 | `+message`/`+thread` 与同稳定 ID raw 完整对象一致；`+messages` 用两个不同 ID 验证输入顺序与逐对象一致 |
| 草稿/模板写 | 仅写回执会产生假成功 | 稳定 ID + exact get + 请求字段核对；清理无法证明时保持 unavailable | deterministic 回执/读回矩阵 PASS；live cleanup BLOCKED |

### 6.1 clean executable HEAD 双层证据

| 公开入口 | exact Shortcut + owning raw 证据 | 状态 |
|---|---|---|
| `+search-mail`, `+triage` | 各 20 条 known-nonempty；3 个独立 fresh 零命中由 raw `total=0`、无稳定 ID/正文的单 sentinel 与 terminal cursor 共同证明，exact 严格归一化为显式空；稳定 message ID 集合和分页状态一致 | `PASS_WITH_REVIEWED_ZERO_ENCODING`；最终 SHA 见 PR 证据 |
| `+user-search`, `+find-mail-user` | 各 20 条 known-nonempty 与 raw 显式 fresh zero；条件身份集合和分页状态一致 | `PASS`；最终 SHA 见 PR 证据 |
| `+folder-list` | 顶层 5 条 nonempty；本轮先由 raw 验证同一父文件夹确实为空，再由 Shortcut 返回显式空；ID 集合一致 | `PASS`；最终 SHA 见 PR 证据 |
| `+message`, `+messages`, `+thread` | 单邮件/会话同稳定 ID 完整对象一致；批量用两个不同 ID 验证请求顺序和逐对象一致 | `PASS`；最终 SHA 见 PR 证据 |

8 个公开入口均在最终 clean runtime tree 从零重跑；其中 6 个使用标准 raw 显式空或精确对象证据，2 个邮件搜索使用上述审核过的下游零命中 sentinel 编码。最终可执行 SHA 写入 PR 证据，本文只保留脱敏业务断言。

## 7. 安全与脱敏声明

- 本文不含用户、组织、租户、profile、邮箱、人员姓名、邮件/会话/模板/联系人/聊天真实 ID。
- 本文不含邮件主题正文、收发件人、trace/request ID、token、签名 URL、电话或真实业务时间。
- 真实 E2E 原始响应仅在仓库外临时目录解析；普通输出只保留能力标签、计数和布尔断言。
- 临时草稿虽已执行两次 batch-delete 但仍可按同 ID 读取；临时模板删除后也未获得 typed nonfound。两者都不记为清理 PASS，四个写 Shortcut 因此保持 unavailable。
- 当前邮箱没有已验证的空邮件文件夹或专用空邮箱；因此 `+unread-mail`、`+recent-mail`、`+thread-list`、`+tag-list`、`+template-list`、`+contact-list` 不记 live 双态 PASS，并保持 unavailable。
- 最终提交前仍需扫描最终树、未跟踪文件和 `origin/main..HEAD` 全部历史。
