# DWS DevApp CLI 参数幻觉分析

## 1. 结论摘要

本报告以线上 `main` 提交 `fd24619437afcb92638d6a71e0bfd9254815fe06`（2026-08-11 14:08:11 +0800）为冻结基线，只使用同提交的官方 Cobra 命令树与 Help、运行时组装 Schema、DevApp Skill 及子文档、Shortcut 声明/实现、参数 alias 生成器和正式 `internal/cli/param_concepts.json`。没有使用历史 badcase、`dws-eval`、`merged_scan.json`、历史工作簿、固定 Catalog、用户 Shortcut 或插件。

DevApp 不是 `dev app` 的简单命令别名，而是一套独立的 helper-only Shortcut 产品面。它与 `dev app` 操作相同的开放平台应用领域，但命令路径、可见性和生成范围不同：官方树有 30 个参数化 `devapp +...` Shortcut，只有 19 个公开并进入 Runtime Schema，另有 11 个隐藏兼容命令。参数 alias 生成器会硬失败拒绝这 11 个隐藏路径，因此“命令可执行”不等于“可由当前中央别名表治理”。

量化结果如下：

- 官方 Cobra 树共有 30 个 runnable 路径，全部带业务参数；19 个公开、11 个隐藏，合计出现 90 次 canonical 参数，使用 41 个不同 flag 名。
- Runtime Schema 发布 19 个 DevApp 工具，合计 57 次参数、26 个不同 flag 名；逐 leaf 对账真实 Help，公开参数差异为 0。
- 11 个隐藏 Shortcut 是：`credentials-get`、`event-subscribe/unsubscribe`、`permission-add/remove`、`robot-config/enable/disable`、`security-config`、`version-create/publish`。
- 对实际 Skill 主文件及 `dev/` 子目录 13 个文档进行专门扫描，命中 21 行 `dws devapp` 命令文本；唯一机械告警是说明占位符 `dws devapp <shortcut> --help`，真实命令/参数 drift 为 0。
- 聚合得到 6 类参数问题、66 条“问题—命令”明细，覆盖全部 30 个参数化官方路径。
- 正式基线对 DevApp 的生成覆盖为 0 个命令、0 alias、0 block、0 ambiguous；候选草稿覆盖 19 个公开 Shortcut，生成 134 个 alias、122 个 block、10 个 ambiguous。
- 候选相对正式文件新增 12 个 DevApp concept，扩展 `app_id`、`page_cursor`、`pagination_size`、`search_query`、`user_ids` 5 个既有 concept 的 DevApp scope，并新增 7 个 command override；既有 override、morphological rules 和 253 条 fixture 均未变化。
- 候选已通过 17 组 alias/canonical 最终 ToolCaller 参数等价、10 组 block/ambiguous、11 个隐藏 Shortcut 边界、4 组非 DevApp 回归、三包测试和两项政策门禁。

第一轮可安全治理 19 个公开 Shortcut：统一应用 ID、appKey/版本 ID、应用与机器人名称、应用描述/图标、成员列表与角色、权限查询过滤、游标分页、搜索、应用列表筛选/排序和 WebApp 三类 URL。11 个隐藏命令的参数问题虽然已经分析，但不能写入当前候选，否则生成器会拒绝整个表；应继续使用 canonical 参数，或先独立扩展 reviewed command root。

## 2. 六类参数问题

### 2.1 unifiedAppId、appKey、版本 ID 和分组 ID 容易被统一写成 `id/client-id`

DevApp 的 ID 体系包括：

- `--unified-app-id`：应用全树主键，出现在 28 个 Shortcut；
- `--app-key`：应用列表的 appKey/clientId 过滤；
- `--app-group-id`：应用分组过滤；
- `--version-id`：已创建版本的 ID。

Skill 明确 `appKey = clientId`，但二者都不等于 `unifiedAppId`。写操作必须由用户或上游结果提供明确的 `unifiedAppId`，不能根据 appKey/clientId 或应用名自动反查并继续写。

候选将 `app_id` 扩展到 17 个公开单应用 Shortcut，并为公开 `+list` 的 appKey、三个公开版本 leaf 的 versionId 建立独立 concept。`+list` 同时存在多个 ID/filter 角色，泛化 `--id` 返回 ambiguous；appKey、versionId、groupId 与 unifiedAppId 之间互相 block。

### 2.2 应用名、机器人名、创建人、描述、简介和图标共享宽泛文本/媒体词根

公开命令中：

- `+create/+update` 的 `--name` 表示应用名；
- `+list` 同时有应用 `--name`、`--robot-name` 和 `--creator`；
- `+create/+update` 使用 `--desc` 与 `--icon-media-id`。

隐藏 `+robot-config` 还使用机器人 `name/brief/desc/icon/skills`，隐藏 `+version-create` 使用版本说明 `desc`。因此 `name/description/text/media-id` 不是跨产品面全局同义参数。

候选只在 19 个公开 Shortcut 内建立 app name、robot name、description、icon concept；`+list --query` 无法判断应用名、机器人名或创建人，返回 ambiguous；`--media-id` 不会被猜成图标媒体 ID。

### 2.3 成员、权限、事件和版本审批参数混合角色、单复数与安全含义

代表性参数包括：

- 公开成员写入：`--user-ids`、`--member-type`；
- 公开权限查询：`--scope-value`、`--scope-type`、`--api-status`、`--auth-status`；
- 隐藏权限写入：`--scope-values`；
- 隐藏事件订阅：`--event-codes`；
- 隐藏版本发布：`--approver-user-id`、`--confirmed-sensitive`。

它们不能因为都是 ID、列表或状态就合并。候选对公开成员 leaf 复用 `user_ids` 并新增 member type concept，明确 block 单值 `user-id`；对公开 `permission-list` 分离 scope filter/type 与两种 status，泛化 `scope/status/type` 返回 ambiguous。隐藏权限、事件和发布命令不进入候选。

### 2.4 游标分页、搜索、筛选、排序和版本标签协议名称不统一

`+list/+event-list/+permission-list/+version-list` 使用 `cursor + page-size`；event/permission 搜索使用 `keyword`；应用列表还有 group、creator、develop-type、filter-cool-app、sort-order、sort-type。隐藏 `+version-create --version` 则是人工填写的版本标签，不是服务端 `version-id`。

候选在同协议内扩展 `page_cursor`、`pagination_size` 和 `search_query`，并为 `+list` 增加角色明确的 scoped alias。它不把 page 换算成 cursor，不翻译枚举值，也不把 version 标签和 versionId 互换。

### 2.5 机器人回调、Web 首页、安全 URL/IP 与模式参数属于不同端点角色

公开 `+webapp-config` 同时具有：

- `--homepage-url`：移动/H5 首页；
- `--pc-homepage-url`：PC 首页；
- `--omp-url`：OMP 地址；
- `--h5-page-type`：页面类型。

隐藏 `+robot-config` 还有 event/outgoing URL、mode、SSL；隐藏 `+security-config` 还有 redirect URL、SSO URL 与 IP whitelist。候选只治理公开 WebApp 三类 URL，并把 generic `url/home-url` 设为 ambiguous。URL/IP 列表、协议校验和值转换不属于参数别名层。

### 2.6 19 个公开与 11 个隐藏 Shortcut 的生成、Schema 和安全生命周期边界

本次最重要的边界是：

```text
官方 Cobra 可执行：30
  ├─ 公开并进入 Runtime Schema：19
  │    └─ 当前 alias generator 可治理：19
  └─ 隐藏兼容：11
       └─ 当前 alias generator 硬失败拒绝：11
```

最初将 11 个隐藏命令加入候选时，`go generate ./internal/cli` 明确报告每个 path “does not match any runnable Cobra leaf/command”。候选因此主动收敛到 19 个公开 Shortcut。这不是漏做，而是当前生成入口与声明树的真实能力边界。

此外，参数名正确也不等于业务完成：删除、停用、成员移除仍受 Runtime confirmation；版本发布的审批人必须用户选择；permission/robot/webapp 配置后仍需版本进入 `RELEASE/AUDIT/UNDER_REVIEW` 才能判断上线状态。参数别名不拥有这些门禁。

## 3. 当前别名表可以直接实施的方案

候选完整文件位于同目录 `param_concepts.json`，主要改动如下：

1. 新增 12 个 DevApp concept：appKey、应用/机器人名称、description、icon、member type、permission scope filter/type、version ID、三类 WebApp URL。
2. 扩展 `app_id`、`page_cursor`、`pagination_size`、`search_query`、`user_ids` 5 个既有 concept 的 DevApp 精确命令范围。
3. 新增 7 个公开 command override：`+create/+update/+list/+member-add/+member-remove/+permission-list/+webapp-config`。
4. 19 个公开 Shortcut 全部进入生成结果；11 个隐藏 Shortcut 不进入候选。

生成影响面由正式基线的 0/0/0/0 扩展为 19/134/122/10（命令/alias/block/ambiguous）。独立审核确认：

- 19 个生成 path 全部是冻结提交中公开、可执行、进入 Schema 的 Shortcut；
- 134 个 alias 的目标全部是对应 leaf 的真实 canonical flag；
- alias 来源没有与该 leaf 的真实 flag 冲突；
- 122 个 block 和 10 个 ambiguous 没有拦截真实 flag；
- 11 个隐藏 path 全部不在 generated table；
- 既有非 DevApp override、morphological rules 和 253 条 fixture 完全不变。

正式合入时还应与 Dev 产品候选做一次跨产品 concept 合并评审。两套命令面参数高度相似，长期更适合让共享 `app_id/pagination/user_ids` concept 同时列出 `dev app` 和 `devapp +` path，并对角色相同的新 concept 去重，而不是保留两套互相独立的同义词定义。本报告的候选必须单独可用，因此仅基于正式基线生成，没有依赖尚未合入的 Dev 草稿。

## 4. 当前能力支持不了或不应该自动处理的事项

以下事项不能通过本次 `param_concepts.json` 候选解决：

- 为 11 个隐藏 Shortcut 配置中央 alias；
- unifiedAppId 与 appKey/clientId 自动互转；
- 应用名或机器人名自动解析成稳定应用 ID；
- 单个 userId 与 user-ids 自动扩展或收缩；
- `scope-value` 查询过滤与 `scope-values` 写入列表互换；
- 翻译 develop/filter/status/type/mode 等枚举值；
- 拆分或合并 URL、IP、skills、event-codes 等列表；
- 主动读取、展示或放宽 appSecret/clientSecret 脱敏；
- 用 alias 代替 `--yes`、审批人选择或 `confirmed-sensitive`；
- 用参数名归一保证配置已经上线或版本已经 `RELEASE`。

其中第一项是当前框架的硬能力缺口，其余是值转换、实体解析、安全或业务生命周期问题。它们不阻塞 19 个公开 Shortcut 的第一轮安全治理。

## 5. 候选草稿验证结果

候选仅在冻结快照 `/private/tmp/dws-main-param-analysis.HcTfUP` 中临时替换正式输入并重新生成、构建和测试；当前工作区正式 `internal/cli/param_concepts.json` 与 `internal/cli/param_aliases_generated.go` 未修改。

验证结果：

- JSON 结构、候选 scope 和 generated 全量规则审核通过；
- `go generate ./internal/cli` 稳定生成 300 个全局命令条目，其中 DevApp 19 个；
- 17 组 alias/canonical 通过注入 Runner 的最终 ToolCaller 调用序列与参数等价；读类 Shortcut 未使用“同样鉴权失败”作为等价证据；
- 10 组 block/ambiguous 在 PreParse 中命中预期保护；
- 11 个隐藏 Shortcut 全部确认可执行且不在 generated table；`+credentials-get --app-id` 保持 unknown flag，而 canonical `--unified-app-id` 仍可解析；
- 4 组 Calendar、Doc、Drive、Mail 非目标 alias 最终 payload 未变化；
- `internal/cli`（127.945s）、`internal/pipeline`（1.312s）、`internal/app`（201.296s）全部通过；
- `check-generated-drift.sh` 通过，参数 alias 与 Schema assembly 两次结果一致；
- `check-schema-catalog.sh` 通过，最终仍为 27 个产品、1018 个工具；
- runtime confirmation truth、flag/help/schema homology 和 Catalog assembly determinism 均通过。

## 6. 第一轮改造建议

1. 先落地 12 个 DevApp concept、5 个既有 concept 的精确 scope 和 7 个公开 override，不修改真实 CLI flag。
2. DevApp owner 重点审核 `+list`、`+permission-list`、`+member-add/remove`、`+webapp-config` 的 block/ambiguous。
3. 保持 11 个隐藏 Shortcut canonical；若确需参数治理，先独立设计 alias generator 对隐藏 reviewed command 的收录契约与安全测试。
4. 与 Dev 候选合并前做 concept 去重，确保相同实体/角色使用共享 concept，路径仍精确列举。
5. 将 17/10/11/4 行为集转为长期仓库测试，尤其固定“隐藏命令 alias 仍不命中”的能力边界。

## 7. 可复用到其他产品的流程

1. 冻结线上 main，并在隔离副本重建二进制；
2. 同时遍历公开和隐藏 runnable Cobra 路径，不能只看 Runtime Schema；
3. 对账公开 Help/Schema，针对实际 Skill 目录补充扫描，不把“文件未发现”当作无漂移；
4. 按实体、角色、cardinality、值域、结构和安全语义聚合问题；
5. 基于正式别名表生成完整候选，并实际运行 generator 验证命令是否属于 reviewed root；
6. generator 硬失败的 path 列为能力边界，不能通过删除检查或伪造 path 绕过；
7. 对读类 Shortcut 使用注入 Runner 验证最终参数，不能拿相同鉴权错误当等价；
8. 验证保护、隐藏边界、非目标回归、包测试、生成确定性和 Schema policy；
9. 在跨产品命令面高度同构时，正式合并前做 concept 去重与共享，而每个产品候选仍应可独立复现。
