# DWS Contact 产品 CLI 参数幻觉分析

> 状态说明（2026-08-21）：本文件保留 2026-08-11 的原始事实盘点和问题证据；其中 concept 数量、
> concept 名称及落位方案已被同目录 `contact_cli_param_hallucination_review_20260821.md` 与最新候选
> `param_concepts.json` 取代。最新候选基线为线上 main `11934eed057267d97e7442ddd420c711ee1802dc`，
> 直属主管 userId 与账号昵称均按局部命令角色治理，不是中央 concept。

## 1. 结论摘要

本轮以线上 `main` 提交 `fd24619437afcb92638d6a71e0bfd9254815fe06`（2026-08-11 14:08:11 +0800）为冻结基线，从该提交重新构建二进制，并使用同一份官方 `NewSchemaSourceRootCommand()` 命令树，对 Contact 的真实 Help、运行时组装 Schema、仓库内置 Skill、审核排除项、隐藏兼容命令、命令实现和正式参数概念表进行对账。分析未使用历史 badcase、`dws-eval`、历史工作簿、固定 Catalog、用户自定义 Shortcut 或插件。

Contact 官方命令树共有 **72 个 runnable 路径**：其中 **46 个可见路径、26 个隐藏路径**。46 个可见路径由 38 个公开叶子和 8 个可执行父命令组成；38 个公开叶子中，35 个进入运行时 Agent Schema，`contact label list/get/list-members` 3 个公开叶子被 `schema_command_exclusions.go` 精确审核排除。产品共有 **16 个仓库内置 Shortcut 路径**，其中 14 个进入 Schema，`contact +get-roster` 与 `contact +list-roster-fields` 为隐藏内置 Shortcut。

官方树中有 **40 个路径带 canonical 业务参数**，共出现 **76 次 canonical 参数**、形成 **32 个不同 canonical flag 名**；其中 31 个是可见参数化叶子，9 个是隐藏参数化兼容路径。命令树另外注册了 **74 次隐藏 alias flag**，涉及 15 个隐藏名称。对 35 个 Schema 工具逐命令核对，真实 Help 与运行时 Schema 的公开业务 flag 差异为 **0**；Contact Skill 的 16 条机械扫描告警经人工复核均为命令别名、审核排除命令或正文路径提示，真实参数问题为 **0 条**。

本轮聚合为 **7 类参数问题**，Excel 中形成 **87 条“问题—命令”明细，覆盖全部 40 个参数化官方路径**，同时单独记录公开排除项和隐藏 Shortcut 边界。最需要优先处理的是：

- `--user-id`、`--staff-id`、`--ids` 和 `--master-user-id` 都是人员标识符，但分别代表目标员工、花名册员工、用户列表和直属主管，单复数及操作角色不能互换；
- `--dept`、`--parent`、`--depts` 和 `--dept-ids` 同时混合目标部门、父部门、CSV 列表与 JSON 部门成员数组，尤其同名 `--depts` 的值协议并不统一；
- `--query`、`--name`、`--org-user-name`、`--org-name`、`--creator-username`、`--nick` 和 `--ownness-text` 都可能被模型概括成“名称”，但目标实体和写入角色不同；
- 角色/标签命令使用宽泛 `--id`、`--label-id` 和名称列表 `--names`，必须把 role ID、用户 ID、部门 ID 与角色名列表分开；
- `--fields` 在 root 是输出字段筛选，在 `contact user profile get` 叶子内却是花名册 fieldCode 列表，当前作用域依赖参数位置；
- Schema、公开可执行面、隐藏 compatibility 和中央 alias 是不同层；解析成功不能证明中央规则生效，也不能证明最终业务参数被读取。

冻结正式别名表对 Contact 生成 **7 个命令条目、32 条 alias、29 条 block、0 条 ambiguous**。候选草稿扩展到 **31 个 Contact 命令条目、193 条 alias、159 条 block、7 条 ambiguous**，恰好覆盖全部 31 个公开参数化叶子。相对冻结正式表新增 15 个 Contact concept、扩展 8 个既有 concept 的 Contact 命令范围、新增 17 个 Contact override，并修改 1 个既有 Contact override；候选共有 21 个 Contact override。正式 253 条 validation fixture 保持不变，非 Contact concept、override 和保护规则均未修改。

候选已在隔离副本完成生成、构建、21 组 alias/canonical 最终 payload 等价、9 组 block/ambiguous、6 组原生兼容、4 组非目标产品回归以及 `internal/cli`、`internal/pipeline`、`internal/app`、生成漂移和 Schema 政策门禁。所有写命令测试均使用 `--dry-run`，没有真实业务写调用。候选可进入产品评审，但仍是完整待审核草稿，不会直接替换当前工作区正式 `internal/cli/param_concepts.json`。

## 2. 参数问题

### 2.1 用户标识符的名称、单复数和操作角色不统一

Contact 使用四类主要用户参数：目标员工 `--user-id`、花名册查询员工 `--staff-id`、批量用户 `--ids`、直属主管 `--master-user-id`。稳定命令和隐藏兼容入口还接受 `id`、`userid`、`user-id`、`user-ids` 等真实 alias。

这些值都可能长得像 userId，但业务角色不同。例如 `contact user update --user-id` 是被修改员工，`--master-user-id` 是该员工的直属主管；`contact user get --ids` 是列表，不能把单个 `--staff-id` 静默包装成列表。候选扩展 `user_id`/`user_ids`；主管 userId 仅在拥有 `--master-user-id` 的精确命令中按局部角色治理，并对单值/列表和目标/主管配置 block 或 ambiguous。

### 2.2 部门参数同时混用目标部门、父部门、CSV 列表和 JSON 数组

部门参数的复杂度来自三个维度同时存在：

- `--dept` 表示单个部门 ID，但 `contact +dept-members --dept` 又表示部门名称关键词；
- `--parent` 表示创建或移动后的父部门；
- `--depts` 在部门成员查询和离职查询中是 CSV 部门 ID 列表；
- `--depts` 在员工邀请/更新中是 JSON 部门成员数组；
- `--dept-ids` 在账号创建中是 CSV 列表。

候选只对同角色、同 cardinality、同值协议的名称做归一。CSV 列表复用 `dept_ids`；JSON 数组使用独立 concept，并只接受显式带 JSON 语义的来源名。`contact dept update/create` 使用 `target-dept-id`、`parent-dept-id` 等角色明确的 scoped alias；跨角色、跨单复数和 CSV/JSON 之间全部保护。

### 2.3 查询词、人员姓名、组织内姓名和对象名称共用 name/query 词根

`contact user/dept search` 的 `--query` 是检索词；`contact +lookup/+org/+team --name` 是人员姓名/花名查询；`--org-user-name` 是写入企业账号的员工姓名；`contact org create` 同时存在组织名 `--org-name` 和创建者名 `--creator-username`；`--nick` 与 `--ownness-text` 又分别是昵称和个人状态。

候选把这些角色拆成 search query、person-name query、org-user-name、organization name、creator name、nickname 和 status text。`employee-name`/`staff-name` 因为可表示“查询某人”或“写入组织内姓名”，没有放入全局 concept，而是在精确命令 override 中映射。`contact org create --name` 因有两个合理目标，明确返回 ambiguous。

### 2.4 角色标签的宽泛 id 与名称列表容易混淆

`contact +list-role-members` 和 `contact label list-members` 的真实 `--id` 表示单个 role/label ID；`contact label get --names` 表示逗号分隔的精确角色名称列表。隐藏兼容路径还可能出现 `--label-id`、`--role-id`、`--name`、`--query` 和 `--keyword`。

候选新增 role ID 与 role names 两个 concept，在精确命令中绑定宽泛真实 `--id`/`--names`，同时阻断部门 ID、用户 ID、ID 列表和名称列表之间的错误互换。角色名模糊搜索或名称转 ID 需要业务查询，不属于参数改名能力。

### 2.5 账号标识、联系方式和资料字段处于相邻工作流但值域不同

账号创建/更新和人员查询同时使用 `mobile`/`org-user-mobile`、`login-id`、`email`、`avatar-file-id`、`nick` 与 `org-user-name`。候选允许 phone/mobile-number 等在手机号角色内原样传递，但不把手机号转成 loginId/userId；email、头像文件 ID、昵称和组织内姓名分别使用独立 concept，并对 file/node/user/name 等跨值域名称做保护。

### 2.6 时间、分页和布尔控制缺少统一但不能进行值协议转换

`contact user dismission search` 使用 `start/end`、一基 `page` 和 `limit`；另有 `hide-retirement`、`hide-partner`。部门创建要求显式 `create-dept-group`，账号创建有 `send-pwd-via-sms`。

候选扩展 `time_start`、`time_end`、`page_number` 和 `pagination_size`，只在离职查询中接受同格式名称变体并原样传值；cursor/offset/page-token 等不同分页模型被保护。布尔 flag 保持 Cobra 原生解析和默认值，候选不转换其业务含义。

### 2.7 公开 Schema、审核排除项、隐藏兼容路径和同名 fields 的可发现性边界

Contact 的 72 个 runnable 路径并不都进入 Agent Schema。`contact label list/get/list-members` 是公开可执行叶子，但被精确审核排除；26 个隐藏路径用于兼容、别名或导航。有效隐藏 alias 保持原生，不在中央候选重复实现。

生成器审核还发现 `contact +get-roster` 虽是仓库内置隐藏 Shortcut，但当前参数 alias reducer 不把它识别为可治理的 runnable leaf，因此候选不能为它增加中央 alias，只能保留 canonical `--staff-id`/`--fields`。此外，`contact user profile get --fields` 在叶子后表示花名册 fieldCode，而 root `--fields` 表示输出筛选；别名表无法重命名真实同名 flag 或消除参数位置差异。

## 3. 当前别名表可以实施的方案

候选草稿位于同目录 `param_concepts.json`，是冻结正式表的完整副本加 Contact 改动，不是增量片段。

第一轮建议落地：

1. 扩展 `search_query`、`user_id`、`user_ids`、`dept_ids`、`time_start`、`time_end`、`page_number` 和 `pagination_size` 的 Contact 精确命令范围；
2. 新增手机号、按姓名查人、组织内员工姓名、JSON 部门数组、主管 userId、角色 ID/名称、花名册字段、头像文件 ID、状态文本、组织名、创建者名、登录号、邮箱和昵称等 15 个 concept；
3. 为 17 个新命令配置 override，并完善 `contact user profile get` 的既有 override；
4. 对用户单值/列表、目标员工/主管、部门单值/列表、CSV/JSON、目标部门/父部门、role/user/dept ID 和多种 name 角色配置 block/ambiguous；
5. 保留真实隐藏 flag 和兼容路径，不把原生行为重复包装成中央 alias；
6. 隐藏 `+get-roster`、Schema exclusions 和 root/leaf `fields` 冲突明确留在能力边界，不为了覆盖率强行配置。

候选生成后的影响面为 31 个 Contact 命令、193 条 alias、159 条 block、7 条 ambiguous。规则数量较大的主要原因是同一产品同时包含用户、部门、角色、账号、检索和资料字段，并需要对跨实体/角色/单复数做成对保护；所有新增 concept 和 override 均已由代表性最终 payload 或 guard 用例覆盖。

## 4. 当前能力支持不了或不应该做的事项

- 把姓名、花名或手机号自动查询并解析成唯一 userId；
- 把 CSV 部门 ID 列表包装成 JSON 部门成员数组，或反向转换；
- 把单个用户/部门 ID 自动变成列表，或从列表中选择一个；
- 根据宽泛 `name`、`department`、`manager` 在同一命令的多个目标间自动选择；
- 消除 root `--fields` 与花名册 leaf `--fields` 的真实同名冲突；
- 为当前 reducer 不识别的隐藏 `contact +get-roster` 增加中央 alias；
- 通过 `param_concepts.json` 让审核排除的 label 命令进入 Agent Schema；
- 对角色名称做模糊匹配并解析为 role ID；
- 在日期、page、cursor、offset、布尔默认值之间做值协议转换。

上述情况应继续使用真实 Help/Schema 的 canonical 参数；多目标场景由候选在 dispatch 前停止，查询/转换场景应先调用独立命令得到稳定值。

## 5. 候选草稿审核结论

候选相对冻结正式文件的结构化审核结论：

- 新增 15 个 concept，全部只包含 `contact ...` 命令；
- 修改 8 个既有 concept，只增加 Contact 精确命令范围，成员、排除项、含义和风险没有变化；
- 新增 17 个 override，修改 1 个既有 Contact override；候选共有 21 个 Contact override；
- 非 Contact concept、override、保护规则和 253 条 validation fixture 变化为 0；
- 所有可治理命令路径都来自同提交官方命令树；
- `employee-name`/`staff-name`、target/parent department 等存在角色冲突的来源已从全局 concept 移到精确 override；
- 与真实隐藏 flag 重复的来源由生成器自动保留为原生行为，没有重复 alias；
- `contact +get-roster` 因 reducer 边界从候选移除，并转入当前无法解决；
- 所有自动 alias 均满足同实体、同角色、同值域、同单位、同 cardinality 且值可原样传递；
- 无法确认的映射均转为 block、ambiguous、原生行为或当前不支持。

因此候选在语义、作用域和仓库契约上合理，可进入 Contact owner 评审；它没有直接修改正式工作区别名表。

## 6. 验证结果

候选在隔离副本中临时替换正式输入并重新生成、构建和测试，验证包括：

- `jq` 解析、真实生成器读取和二次生成确定性；
- 21 组 alias/canonical 最终等价，其中 17 组真实 dry-run payload、4 组 Shortcut mock 结果/错误等价；
- 9 组 block/ambiguous 在 dispatch 前停止；
- 6 组原生兼容，其中 dept-id/id/user-id/keyword/role-id 最终等价，隐藏 `+get-roster` canonical mock 可执行；
- 4 组 Calendar、Doc、Drive、Mail 非目标 alias/canonical 最终 payload 等价；
- `internal/cli` 81.404 秒、`internal/pipeline` 0.482 秒、`internal/app` 242.021 秒全部通过；
- `check-generated-drift.sh` 通过，两次生成确定；
- `check-schema-catalog.sh` 通过，最终仍为 27 个产品、1018 个工具。

本产品不需要修改 validation fixture 或 complete-command payload 模板。写操作全部使用 `--dry-run`，未发起真实业务写调用。当前工作区正式 `internal/cli/param_concepts.json` 和 `internal/cli/param_aliases_generated.go` 在验证前后均未修改。

## 7. 第一轮改造建议

1. 优先落地用户角色、部门值协议、role ID/name 和多目标 name 保护，避免写命令只增加 alias 而没有风险约束；
2. 同步落地 21 组 payload 等价、9 组 guard 和 6 组原生兼容回归，防止后续命令新增时作用域静默扩大；
3. 单独评审 `contact label ...` 的 Schema exclusions 是否仍应保留，这属于 Agent 可发现性治理，不混入别名表改动；
4. 评估花名册业务 `--fields` 是否需要长期重命名，并为 root/leaf 参数位置给出明确文档；
5. 如果未来希望治理隐藏 `+get-roster`，先完善 reducer 对官方隐藏内置 Shortcut 的审核入口，再新增规则。

## 8. 可复用分析流程

后续产品继续使用同一流程：冻结提交并构建官方二进制 → 合并 runtime Schema、真实 Help、Skill、官方树、审核排除项和内置 Shortcut → 按实体、值域、角色、cardinality、单位和值协议归并问题 → 基于冻结正式表生成完整候选 → 审核真实 flag 冲突和原生 compatibility → 在隔离副本执行生成、PreParse、payload、保护、非目标回归、包测试和政策门禁 → 只交付产品 Markdown、五页中文 Excel 和候选草稿，不直接改正式别名表。
