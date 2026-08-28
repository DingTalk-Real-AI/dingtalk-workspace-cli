# OA 审批核心参考

本文件只保留 OA 高频查询、角色路由和写操作闭环。已知命令直接执行；只有 leaf 参数或安全语义不确定时读取精确 compact Schema，只有 Cobra flag 不确定时读取精确 leaf Help，不要加载产品级 Catalog。

## 按需加载

| 当前任务 | 读取范围 |
|---|---|
| 查询表单、待处理、我发起、我处理、抄送、详情、任务、记录、管理员列表 | 只读本文件 |
| 同意、拒绝、转交、加签、退回、评论、追加抄送、撤销 | 本文件足够；遇到错误再按 shared 契约读取对应错误 reference |
| 发起审批、预测流程、组装表单字段 | 继续读 [oa-create.md](oa-create.md)；按其中条件加载控件和节点 reference |
| 下载链接、下载/预览授权、上传审批附件 | 继续读 [oa-attachments.md](oa-attachments.md) |

不要为普通查询预读创建、控件、节点或附件全文。

## 角色来源是对象身份的一部分

先固定用户要求的业务来源，后续 `processInstanceId` 和 `taskId` 必须继承该来源证明。

| 用户意图 | 唯一主来源 | 说明 |
|---|---|---|
| 待我处理、我要审批 | `dws oa approval list-pending` | 只有这里返回的当前任务可作为审批人操作对象 |
| 抄送我的、CC 我的 | `dws oa approval list-cc` | 不得用 pending、executed、submitted 或 admin 实例替代 |
| 我发起/提交过的审批 | `dws oa approval list-submitted` | 支持可选关键词；适合个人全景和模糊定位 |
| 已知模板和时间范围，查我发起的实例 | `dws oa approval list-initiated` | `processCode` 来自真实表单或详情 |
| 我处理过的审批 | `dws oa approval list-executed` | 不等于当前待处理，也不等于抄送关系 |
| 企业内某模板的审批、管理员统计 | `dws oa approval list-by-admin` | 仅在用户明确管理员意图且当前账号有权限时使用 |

实例可被 `detail` 访问只证明“可访问”，不证明它属于用户指定的角色关系。主来源无目标时，不得静默切换来源寻找相似对象；管理员权限失败时也不得用个人列表冒充管理员结果。

## 查询收敛与分页

1. 先按用户给出的角色、时间范围和关键词查询。
2. 若返回明确空结果，只在有业务依据时放宽一个条件，例如去掉关键词或扩大到用户允许的时间范围；说明放宽内容，不穷举同义词、页码和时间窗口。
3. 若响应给出 `hasMore`、`nextCursor`、总数或其他未读完证据，按用户要求继续分页。用户要求“全部/完整”时必须读到 endpoint exhaustion；没有分页证据时不要盲翻页。
4. 合理放宽后仍明确为空，立即交付“指定来源内无匹配”，不要切换角色来源。空结果可以是正确业务结果。
5. 候选定位阶段只保留必要字段；选中后再调用 `detail`、`tasks`、`records`，不要为每个候选读取完整详情。

这里没有固定“最多两次查询”的机械限制；每次扩展都必须有新证据或明确业务理由。

## 目标绑定与写操作闭环

任何审批决定、转交、加签、退回、评论、追加抄送或附件操作前，核对并保存：

- 角色来源和 profile；
- 流程名称/`processCode`、发起人、状态；
- `processInstanceId`、当前 `taskId`；
- 用户指定的金额、标记、附件、时间冲突等业务前提。

缺少或不匹配任何一项都停止。用户说“就是这条”只确认此前摘要中的同一实例；重新搜索到的相似对象必须重新核对。业务前提检查失败时，不得在审批意见里宣称该前提已满足。

写操作的确认要求以精确 leaf Schema 的最终 `confirmation` 为准。需要确认时先展示对象、动作和影响，得到明确同意后才追加 `--yes`。写后按真实返回和对应读取命令验证，不得仅凭退出码、对象可访问或另一种附件能力宣称成功。

## 高频读取闭环

所有结构化读取使用 `--format json`。

### 待处理审批及完整上下文

```bash
dws oa approval list-pending --start "<ISO-8601>" --end "<ISO-8601>" --query "<可选关键词>" --format json
dws oa approval detail --instance-id <processInstanceId> --format json
dws oa approval tasks --instance-id <processInstanceId> --format json
dws oa approval records --instance-id <processInstanceId> --format json
```

先从 `list-pending` 选定真实实例，再读取详情、当前任务和处理记录。需要排序时转换毫秒时间戳为当前时区后比较。

### 个人审批全景

```bash
dws oa approval list-submitted --query "<可选关键词>" --format json
dws oa approval list-executed --query "<可选关键词>" --format json
dws oa approval list-cc --query "<可选关键词>" --format json
```

三类来源可以并行读取，但必须分栏保留来源标签。用户要求每类最新一条时，只对非空类别分别读取 `detail` 和 `records`；空类别如实说明。

### 表单与我发起的实例

```bash
dws oa approval search-forms --query "<关键词>" --format json
dws oa approval list-forms --cursor 0 --limit 100 --format json
dws oa approval form-schema --process-code <processCode> --format json
dws oa approval list-initiated --process-code <processCode> --start "<ISO-8601>" --end "<ISO-8601>" --cursor 0 --limit 20 --format json
```

已知关键词优先 `search-forms`；只有用户要求枚举可见表单时使用 `list-forms` 并检查分页。多个相近模板时展示真实名称和 `processCode` 让用户消歧，不根据近似名称擅自选模板。

### 管理员模板实例

```bash
dws oa approval list-by-admin --process-code <processCode> --start "<ISO-8601>" --cursor 0 --limit 20 --statuses RUNNING,COMPLETED --format json
```

权限错误立即作为管理员查询失败交付，不尝试个人来源替代。`processInstanceId` 可继续用于 `detail`、`records` 和 `tasks`。

## 审批处理闭环

| 意图 | 前置 | 写命令 | 写后验证 |
|---|---|---|---|
| 同意 | `list-pending → detail/tasks/records` | `dws oa approval approve --instance-id <id> --task-id <taskId> --remark "<意见>" --format json` | `detail` + `records` + 必要时 `list-executed` |
| 拒绝 | 同上 | `dws oa approval reject --instance-id <id> --task-id <taskId> --remark "<原因>" --format json` | `detail` + `records` + 必要时 `list-executed` |
| 转交 | 当前 `taskId` + 真实目标 userId | `dws oa approval redirect-task --task-id <taskId> --to-actioner-id <userId> --remark "<说明>" --format json` | `tasks` + `records` |
| 加签 | 当前实例/任务 + 真实被加签人 userId | `dws oa approval append-task --instance-id <id> --task-id <taskId> --type <before|after|Parallel> --appender-user-ids <ids> --activate-type <ALL|ONE_BY_ONE> --agree-all <true|false> --format json` | `tasks` + `records` |
| 退回 | 先查可退节点 | `dws oa approval revert-task --instance-id <id> --task-id <taskId> --target-activity-id <activityId> --action <revertAction> --remark "<说明>" --format json` | `tasks` + `records` |
| 撤销我发起的审批 | 从 submitted/initiated 核对为本人发起 | `dws oa approval revoke --instance-id <id> --remark "<说明>" --format json` | `detail` + `records` |
| 评论 | 核对实例及来源 | `dws oa approval oa-comments --instance-id <id> --content "<评论>" --format json` | 用能展示评论的真实返回/读取结果验证；无法独立回读时明确说明 |
| 追加抄送 | 核对实例 + 真实 userId | `dws oa approval oa-cc-noticer --instance-id <id> --users <userId1,userId2> --format json` | `detail` 或对应真实返回验证 |

姓名先用 `dws aisearch person --query "<姓名>" --dimension name --format json` 解析并消歧，禁止把姓名直接当 userId。

### 退回的强制前置

```bash
dws oa approval revert-activities --task-id <taskId> --format json
```

从真实返回选择 `activityId` 和 `revertAction`。无可退节点时立即停止；不得编造 `target-activity-id` 或把固定的 `sid-startevent` 用于非“退回发起人”场景。

## 发起审批路由

用户要创建、提交或发起审批时，继续读取 [oa-create.md](oa-create.md)。不要凭本文件直接组装 `form-values`，也不要用真实 `create-instance` 试探 payload。

## 附件路由

处理附件时继续读取 [oa-attachments.md](oa-attachments.md)。核心能力边界如下，保留这些语义用于快速选路：

| 目标 | 命令 | 关键边界 |
|---|---|---|
| 获取单个附件临时下载链接 | `dws oa approval attachment download-url` | 只返回链接，不保存文件 |
| 为当前用户授权下载 | `dws oa approval attachment authorize-download` | `fileInfos` 最多 10 项；不返回下载链接 |
| 授权审批内预览 | `dws oa approval attachment authorize-preview` | `fileIds` 最多 20 项；不等于下载授权 |
| 上传本地审批附件 | `dws oa approval attachment upload` | 返回附件元数据，供创建审批字段使用 |

附件实例也必须来自用户指定的角色来源。生成下载链接成功不能证明下载授权成功，反之亦然。

## 催办

先从当前审批任务获取被催办人，再按 DING 规则发送，禁止猜 userId：

```bash
dws oa approval ding-info --task-id <taskId> --format json
dws ding message send --robot-code <robotCode> --users <userIds> --content "<催办内容>" --format json
```

短信或电话 DING 有成本，按 DING Skill 和最终 Schema 的确认规则执行。

## 错误与最终交付

- 空结果不是错误，按“查询收敛与分页”处理。
- 参数或命令错误只在 Help/Schema 给出明确修正依据时纠正一次。
- 业务错误先读 JSON 中的 `retryable`、`retry_after_seconds`、`next_retry_at`、`hint` 和 `actions`；只有明确可重试才重试，参数变体不重置预算。
- 权限错误、不可重试错误或重复业务错误立即停止，不切换角色来源或相邻能力碰运气。
- 预留最终答复时间；没有成功字段和写后证据时明确说“未完成”。

## 自动化脚本

| 场景 | 用法 |
|---|---|
| 查看 7 天待审批并逐条显示详情 | `python scripts/oa_pending_review.py --days 7` |
| 批量同意/拒绝 | `python scripts/oa_batch_approve.py --action approve --days 7` |

批量决策仍需逐项核对对象、业务前提和最终 Schema 的确认要求；不要因脚本批量执行而跳过验证。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 Agent leaf Schema（例如 `dws schema --cli-path "oa +<shortcut>" --compact --format json`），在当前 Cobra flags 不确定时读取 `dws oa <shortcut> --help`。只有参数映射、接口绑定或 provenance 审计才省略 `--compact`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service oa --format json` 批量发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws oa +search-forms` | read | 按关键字模糊搜索当前用户可见的审批表单 |
<!-- VISIBLE_SHORTCUTS_END -->
