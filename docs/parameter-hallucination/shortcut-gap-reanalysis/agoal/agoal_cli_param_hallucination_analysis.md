# Agoal CLI 参数幻觉复核报告

## 1. 结论

冻结基线为 `origin/main@f4474b57eb1db23b1638b9574be2f5dca368a360`。本轮覆盖 5 个此前未进入正式参数兜底范围的公开 Shortcut；识别出 2 个可直接自动兜底、3 个只能部分兜底、0 个只应增加保护的命令，其中高风险写入或敏感读取 0 个。

本报告中的修订候选已合入正式 `internal/cli/param_concepts.json`，并重放到最新 main。所有映射均满足：源参数不是该命令真实 flag、目标是该命令真实 flag、实体/角色/值域/基数一致、值原样传递、不绕过确认。

## 2. 参数问题明细

| # | 命令 | 真实参数焦点 | 幻觉风险 | 候选处理 | 落地能力 | 风险 |
|---:|---|---|---|---|---|---|
| 1 | `agoal +report-statistics-list` | `keyword` | 查询词常被写成 query/q；name/title 不是同义字段 | 扩展 search_query；阻断 name/title/subject | 可自动兜底 | 低 |
| 2 | `agoal +obj-template-list` | `keyword,page,page-size` | 搜索和页码命名不一致；cursor 与页码模型冲突 | 复用 search_query/page_number/pagination_size；阻断 cursor/offset | 可自动兜底 | 低 |
| 3 | `agoal +contract-fields` | `keyword` | 模糊查询词容易被误写为精确 field-id 或结构化 type/code | 仅 query/q 归一到 keyword；精确 ID 阻断，role-free 名称设歧义 | 部分兜底 | 中 |
| 4 | `agoal +user-rules` | `user-id,rule-id` | userId 与 openId/unionId/ruleId 值域冲突 | 复用 user_id；阻断其他身份域；id 保持歧义 | 部分兜底 | 中 |
| 5 | `agoal +report-submit-detail` | `template-id,submit-state,query-date,keyword,page,page-size` | 模板 ID、单日、状态和编号分页角色易混淆 | 角色完整别名 + 现有 concept；阻断日期范围和 cursor | 部分兜底 | 中 |

## 3. 可直接解决的方案

### 3.1 Concept 变更

| Concept | 处理 | 命令范围 |
|---|---|---|
| `search_query` | 扩展既有 concept 命令范围 | `agoal +report-statistics-list`、`agoal +obj-template-list`、`agoal +contract-fields`、`agoal +report-submit-detail` |
| `pagination_size` | 扩展既有 concept 命令范围 | `agoal +obj-template-list`、`agoal +report-submit-detail` |
| `page_number` | 扩展既有 concept 命令范围 | `agoal +obj-template-list`、`agoal +report-submit-detail` |
| `user_id` | 扩展既有 concept 命令范围 | `agoal +user-rules` |

### 3.2 命令级 Override

| 命令 | 处理原则 |
|---|---|
| `agoal +report-statistics-list` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `agoal +obj-template-list` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `agoal +contract-fields` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `agoal +user-rules` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `agoal +report-submit-detail` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |

正式规则新增 17 条 PreParse 验证 fixture，覆盖 alias/canonical、block 与 ambiguous 三类路径。完整命令模板位于 `internal/app/param_alias_payload_equivalence_test.go`。

## 4. 当前不能自动解决

- `agoal +contract-fields`：模糊查询词容易被误写为精确 field-id 或结构化 type/code。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `agoal +user-rules`：userId 与 openId/unionId/ruleId 值域冲突。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `agoal +report-submit-detail`：模板 ID、单日、状态和编号分页角色易混淆。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。

## 5. 第一轮推荐

1. 先合入值域明确、值不变的 concept/命令级别名。
2. 对 ID 域、单复数、两种真实兼容 flag 并存、文件传输编码和多角色参数优先保留 block/ambiguous。
3. 不通过别名表实现名称查 ID、URL 拼装、裸文件路径补 `@`、枚举值翻译或分页模型转换。
4. 写操作的确认语义由真实命令负责，候选 fixture 不注入 `--yes`；测试模板另行验证移除 `--yes` 后仍停在确认门禁。

## 6. 候选表差异与实体必要性审计

- 变更 concept：`search_query`、`pagination_size`、`page_number`、`user_id`。
- 新增 command override：5 条。
- 新增 fixture：17 条。
- 新 concept 仅在同一实体跨多个命令重复出现时建立；单命令或单角色字段全部下沉到 command override。
- 没有新增 role-free 的 `id`、`name`、`status`、`config`、`content` concept。

完整候选见同目录 `param_concepts.json`。

## 7. 事实依据与验证门禁

事实优先级：同提交 Help/Cobra > 同提交运行时 Schema > 产品 Skill > 当前正式别名表。

- Help/Cobra：冻结提交重新构建的 `./dws <command> --help`。
- Runtime Schema：同一二进制 `./dws schema --cli-path <path> --compact -f json`。
- Skill：`skills/multi` 下对应产品 reference。
- 正式表：评审候选基于冻结基线完整复制；当前分支已将修订候选合入 `internal/cli/param_concepts.json`。
- 已通过：JSON/Schema、`go generate ./internal/cli`、全部既有与新增 PreParse fixture、alias/canonical、block/ambiguous、candidate-only 完整命令模板、移除 `--yes` 的确认不可绕过测试、非目标别名回归、`check-generated-drift.sh`、`check-schema-catalog.sh`。
- 评审阶段在隔离副本验证；正式落地阶段重新生成并通过参数概念与 Schema 门禁。

## 8. 可复用流程

1. 冻结 commit、构建二进制并导出命令/Help/Schema 证据。
2. 对每个参数标注实体、值域、角色、基数、真实/兼容/不可用状态。
3. 先判断是否已有可复用 concept；只在跨命令重复实体时新增 concept。
4. 角色局部差异下沉到 command override；需要查找/转换/扩展时拒绝 alias。
5. 为正向 alias、canonical 不变、block、ambiguous 与非目标命令补 fixture 和完整命令模板。
6. 在隔离副本验证候选；评审通过后合入正式表、重新生成并合并最新 main。
