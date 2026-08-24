# 全 DWS 产品 CLI 参数兜底复核与落地状态（更新至 2026-08-24）

## 最终结论

原始产品审计以当时线上 `main` `11934eed057267d97e7442ddd420c711ee1802dc` 为冻结基线；2026-08-24
又将当前分支快进并语义合并到最新 `origin/main` `da6f867d`。中央参数兜底的
产品审计范围已从 28 个补齐为 30 个：Agoal 新增必要规则，Profile 经审计确认应为零新增；此前缺口
Attendance、Contact、Dev、DevApp、Mail、OA、Sheet、Recruit 均已重建独立 latest-main 候选。

联合候选为 [param_concepts.json](param_concepts.json)，SHA-256
`1778e66f11431a930c8c2732b95537c241e5c3a509f9b491b30672ba59fcb409`，包含 105 concepts、
600 command overrides、1689 fixtures。线上冻结基线正式表为 62 / 349 / 675，SHA-256
`d2efc9507b1455c7a41e2af16f45c7531f47aff63bbfe58401e96911af0ae440`；2026-08-24 已将联合候选
同步到当前工作区正式 `internal/cli/param_concepts.json`；合入最新 main 的 Wiki 分页与 OA shortcut
降级规则后，工作区正式表 SHA-256 为
`1778e66f11431a930c8c2732b95537c241e5c3a509f9b491b30672ba59fcb409`。该状态表示工作区已同步，
不表示已经合入或上线。

## 本轮补充结果

| 产品 | 有效命令 | 新 alias | 新 block | 新 ambiguous | 结论 |
|---|---:|---:|---:|---:|---|
| Agoal | 13 | 14 | 4 | 25 | 新发现漏项，已补 |
| Attendance | 52 | 62 | 312 | 21 | 已补，删除无效推测词 |
| Contact | 25 | 28 | 116 | 7 | 已补，保持人/部门/手机号边界 |
| Dev | 36 | 39 | 161 | 26 | 已补，与 DevApp 同域去重 |
| DevApp | 19 | 22 | 101 | 10 | 已补，与 Dev 共用必要概念 |
| Mail | 57 | 53 | 288 | 1 | 已补，禁止 subject→query |
| OA | 28 | 45 | 124 | 23 | 已补，实例/任务/流程分域 |
| Profile | 0 | 0 | 0 | 0 | 已审；原生别名与位置选择器足够 |
| Sheet | 89 | 106 | 1876 | 19 | 已补，修复 space/workspace 混域 |
| Recruit | 3 | 3 | 4 | 8 | 已补，文件/JSON/身份字段分层 |

## 覆盖边界

30 个产品根为：agoal、aisearch、aitable、attendance、audit、calendar、chat、contact、dev、devapp、
devdoc、ding、doc、drive、event、hrbrain、live、mail、markdown、mcp、minutes、oa、pat、profile、recruit、
report、sheet、todo、whiteboard、wiki。Auth、config、completion、schema、plugin、skill、upgrade 等是框架/管理命令，
不作为钉钉业务产品重复建设业务参数 concept。

“已覆盖”不等于给每个 flag 都造别名：只加入可证明同义、值域相同、基数相同且 Runtime 可无损处理的
归一；原生 flag、位置参数、枚举翻译、时间格式、JSON 结构转换、跨字段依赖与身份选择保留给 Cobra、
Schema、Runtime 或 agent 工作流。

## 验证与落地状态

- 10 份新增/更新独立候选均 fresh generate；其嵌入式 PreParse fixture 均通过。
- 中央 concept 必要性复核后，将 8 个仅服务单一逻辑端点或局部 flag 角色的实体下沉为精确命令
  override：`attendance_adjustment_rule_id`、`attendance_overtime_rule_id`、
  `attendance_pagination_offset`、`attendance_report_column_ids`、`contact_account_nickname`、
  `contact_manager_user_id`、`oa_form_values_json`、`oa_request_json`。
- 下沉后的联合候选生成结果与下沉前 `param_aliases_generated.go` 逐字节一致；必要 alias/guard 未减少，
  只是从中央实体层回到拥有该语义的命令层。
- Attendance 二次语义复核后，check-in 的通用 `user/user-id/userid/uid/staff-id` 不再被猜成
  `operator-staff-id`，仅保留 `operator-user-id/operator-id` 等角色明确的别名；单用户
  `attendance record get --user` 不再接受复数 `--users`。这些边界以 ambiguous/block 失败关闭。
- `creator_user_ids` 的定义已从 Doc/Drive 专属表述修正为“资源搜索或列表的创建人 userId 列表”，
  与 Recruit 的实际使用范围一致。
- 清理 8 个被 bind、scoped alias 或 ambiguous 完全覆盖、不会形成最终 block 的冗余 exclude：
  `ding_id/id`、`calendar_event_id/id`、`contact_org_user_name/employee-name`、
  `contact_org_user_name/staff-name`、`mail_template_id/id`、`mail_rule_id/id`、
  `mail_tag_id/id`、`oa_instance_id/id`；清理前后生成别名表逐字节一致。
- 合并时误丢的 15 条冻结基线 fixture 已恢复；唯一继续删除的是经产品复核确认不安全的
  `mail message search --subject → --query`。
- 联合候选完成结构审计：目标命令、bind、scoped alias、native-shadow、fixture 重复错误为 0。
- 联合候选已在冻结提交隔离副本通过参数政策、pipeline、runtime guard、1689 条嵌入式 fixture、
  fresh generate、生成 drift、Schema Catalog 政策与构建验证；完整 payload 模板门禁单独列为落地前置，
  不与参数候选正确性混写。
- Whiteboard/Recruit 已加入当前工作区正式静态产品可见性声明，正式表可直接 fresh generate。
- 清理 36 个 `confirmation=not_required` 命令模板及 3 个命令变体中多余的 `--yes`，并新增通用确认门禁：
  Schema 要求确认的模板必须且只能带一个产品真实确认参数（`--yes` 或 Attendance 原生
  `--user-say-yes`），Schema 不要求确认的模板不得带确认参数；显式 `--dry-run` 模板也不得携带确认参数。
  所有仍带确认参数的不同模板都必须在删除该参数后、零 transport call 前返回结构化
  `confirmation_required`。
- 完整 payload 模板门禁已闭环：正式表有 604 个 active 命令、985 个 active case，当前仓库已有
  604 个命令模板，缺模板命令为 0；每个 active fixture 的 canonical 参数都能在完整模板或精确互斥
  route 变体中找到。
- 本次 12 个产品新增的 45 个缺失命令模板已逐条补齐（Report 7、Ding 6、Event 6、Markdown 5、
  AISearch 4、Whiteboard 2、Audit 3、PAT 2、HRBrain 10；DevDoc/Live/MCP 原本无缺口），包含互斥
  route 变体、必填 JSON、枚举、文件输入和确认参数；目标产品范围再次筛选后缺模板命令为 0。
- Markdown 的 `--dry-run` 变体明确不携带 `--yes`；非 dry-run 的 `markdown patch` 新增平台门禁测试，
  证明删除 `--yes` 后返回 `confirmation_required` 且 transport call 为 0。Whiteboard 使用合法、受控的
  OpenNodes V1 testdata，避免非法输入在确认门禁之前短路测试。
- 已继续补齐本次范围外的 Agoal 11、Recruit 3、Contact 23、Dev 35、DevApp 19，共 91 个历史
  命令模板；其中 Recruit 创建使用合法职位 JSON testdata，Contact/Dev/DevApp 所有
  `confirmation=user_required` 模板均通过删除 `--yes` 后的零 transport-call 验证。
- 已补齐 OA 27 和 Mail 49，共 76 个模板；门禁发现并修复 OA `create-instance`、Mail 邮件撤回、
  单会话删除和批量会话删除仍返回普通字符串确认错误的问题，现均发布结构化
  `confirmation_required`，并在 transport call 前阻断。
- 已补齐 Attendance 50 个模板；确认门禁扩展为同时覆盖 `--yes` 与产品原生
  `--user-say-yes`。FIXED 考勤组模板因缺少 `workDayClassList` 会在确认前失败，已改为业务有效的
  `NONE` 自由工时创建场景。
- 已修复门禁中 5 个“模板存在但没有覆盖目标 canonical 参数”的问题：`contact user profile get`
  补 `--fields`，`dev app get` 补互斥的 `--app-key` 变体，3 个 Mail 分页模板补 `--cursor`；
  复跑后不再有 canonical 缺席错误。
- 已逐条补齐最后 89 个 Sheet 命令模板，覆盖必填 JSON、范围、维度、筛选视图、图表、透视表、
  本地文件和版本参数；其中 16 个 `confirmation=user_required` 命令全部通过“删除确认参数后返回
  `confirmation_required` 且 transport call 为 0”的门禁。逐模板首个 transport 门禁还发现并修正四处
  业务无效输入：筛选视图 `--column` 应为 0-based 整数偏移、版本回退 `--version` 应为整数版本号，
  透视表 create/update 的字段应使用 `field` / `summarize_by`，不能套用另一套 RPC 字段名。
- 当前工作区复核结果：runtime confirmation truth、参数别名嵌入交付整组测试、`internal/app` 全量测试、
  `internal/helpers` 全量测试及 `make build` 均通过；初次语义合并时 `internal/cli` 全量测试通过，最终增加
  `oa +list-executed/count` 精确 block 后又以 fresh generate 和 1689 条 `internal/app` 嵌入式 fixture
  完整复核。fresh 参数别名生成产生 1074 条命令记录，并与正式 `param_aliases_generated.go` 逐字节一致；合入最新 main 后生成物
  SHA-256 为 `c763ef42be37f10f43d7494c7a79c7b290f761d6fac56f8c25da07f40b0f5e6c`。重建后的运行时 Schema 为
  28 个 Agent 产品、1200 个工具，selection 缺失和示例 `--yes` 均为 0。
- 当前桌面执行环境会在独立启动政策脚本临时构建的 Go 生成器时，于进入 `main` 前由系统以 SIGKILL
  终止（测得 CPU 0、RSS 32 KiB）；因此本轮用同一生成函数的 Go 测试进程完成 fresh byte compare，
  并以 `internal/cli` 全量装配测试和重建 `dws schema --all` 复核最终 Catalog。这是本机临时二进制
  启动限制，不是生成差异或测试断言失败；CI/正常 shell 仍应执行原政策脚本作为最终合入门禁。

因此，参数名兜底的**产品审计、工作区正式表同步、模板补齐和确认边界修复均已完成**。当前状态可
表述为“工作区实现与可运行验证完成，等待正常 CI/评审合入”，不能表述为“已合入”或“已上线”。

## 正式模板门禁状态（2026-08-24）

| 产品 | 缺模板命令 | 待验证 user_required | 待验证 not_required | Schema 未发布 |
|---|---:|---:|---:|---:|
| sheet | 0 | 0 | 0 | 0 |
| **合计** | **0** | **0** | **0** | **0** |

“Schema 未发布”表示命令属于已审核兼容/快捷命令但不在 Agent Schema surface；它仍需要真实 Cobra
参数和最终 payload 模板，不能因为 Schema 缺席而跳过。本轮所有 active 命令均已有审核模板；后续
新增命令仍必须逐命令审核必填、互斥、枚举、JSON 结构和确认边界，不能用自动填充默认值绕过门禁。
