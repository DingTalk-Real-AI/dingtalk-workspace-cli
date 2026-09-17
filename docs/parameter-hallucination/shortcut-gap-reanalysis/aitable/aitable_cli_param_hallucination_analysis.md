# AITable CLI 参数幻觉复核报告

## 1. 结论

冻结基线为 `origin/main@f4474b57eb1db23b1638b9574be2f5dca368a360`。本轮覆盖 11 个此前未进入正式参数兜底范围的公开 Shortcut；识别出 0 个可直接自动兜底、11 个只能部分兜底、0 个只应增加保护的命令，其中高风险写入或敏感读取 5 个。

本报告中的修订候选已合入正式 `internal/cli/param_concepts.json`，并重放到最新 main。所有映射均满足：源参数不是该命令真实 flag、目标是该命令真实 flag、实体/角色/值域/基数一致、值原样传递、不绕过确认。

## 2. 参数问题明细

| # | 命令 | 真实参数焦点 | 幻觉风险 | 候选处理 | 落地能力 | 风险 |
|---:|---|---|---|---|---|---|
| 1 | `aitable +base-bootstrap` | `name,folder-id,template-id,tables` | 创建目标与已有 Base/Table ID 易混；tables 是定义 JSON | 仅角色完整的名称/文件夹/模板/JSON 别名；已有资源 ID 阻断 | 部分兜底 | 高 |
| 2 | `aitable +table-bootstrap` | `base-id,name,fields` | fields 定义 JSON 与 field-ids 列表不是同一实体 | 复用 base_id；定义 JSON 仅命令级别名；field-ids 阻断 | 部分兜底 | 高 |
| 3 | `aitable +url-resolve` | `url,verify` | URL 与拆出的 base/table/view/record ID 不能互换 | URL 角色别名可映射；独立 ID 阻断 | 部分兜底 | 中 |
| 4 | `aitable +resolve-base` | `name,fuzzy` | 名称唯一解析与关键词搜索是不同执行语义 | base-name 可映射；query/keyword/base-id 阻断 | 部分兜底 | 中 |
| 5 | `aitable +datasource-create` | `base-id,datasource-type,source-config,auto,field-ids,auto-sync-setting` | sourceConfig、自动同步配置、字段 ID 列表三种结构并存 | 仅明确 sourceConfig 拼写进入 concept；宽泛 datasource-config 保持歧义，generic config/json 和单字段 ID 阻断 | 部分兜底 | 高 |
| 6 | `aitable +datasource-update` | `base-id,table-id,source-config,auto,field-ids,auto-sync-setting` | 两个 ID + 两类 JSON + 字段列表角色冲突 | 按实体 concept 绑定；宽泛 datasource-config 无法选择 JSON 角色，保持歧义 | 部分兜底 | 高 |
| 7 | `aitable +datasource-sync` | `base-id,table-ids` | 同步要求列表，不能把单个 table-id 自动扩成列表 | 仅角色完整复数别名；单数阻断 | 部分兜底 | 高 |
| 8 | `aitable +datasource-sync-status` | `base-id,table-id,task-ids` | 单表 ID 与任务 ID 列表并存 | 复用 base/table concept；任务列表仅角色完整别名；单任务阻断 | 部分兜底 | 中 |
| 9 | `aitable +datasource-get-config` | `base-id,table-id` | 返回的配置容易被误当成请求参数 | 仅绑定两个 ID；source-config/data/json 阻断 | 部分兜底 | 中 |
| 10 | `aitable +datasource-list-sources` | `base-id,datasource-type` | 目录 source 与 sourceConfig 容易混淆 | data-source-type 可映射；source-config 阻断 | 部分兜底 | 中 |
| 11 | `aitable +datasource-get-fields` | `base-id,datasource-type,source-config` | field-ids 是输出，不是请求字段；config 角色不完整 | 仅明确 sourceConfig 拼写可绑定；field-ids 阻断，宽泛 datasource-config 保持歧义 | 部分兜底 | 中 |

## 3. 可直接解决的方案

### 3.1 Concept 变更

| Concept | 处理 | 命令范围 |
|---|---|---|
| `base_id` | 扩展既有 concept 命令范围 | `aitable +table-bootstrap`、`aitable +datasource-create`、`aitable +datasource-update`、`aitable +datasource-sync`、`aitable +datasource-sync-status`、`aitable +datasource-get-config`、`aitable +datasource-list-sources`、`aitable +datasource-get-fields` |
| `table_id` | 扩展既有 concept 命令范围 | `aitable +datasource-update`、`aitable +datasource-sync-status`、`aitable +datasource-get-config` |
| `aitable_field_ids` | 新增（跨命令复用） | `aitable +field-get`、`aitable +record-query`、`aitable +datasource-create`、`aitable +datasource-update` |
| `aitable_datasource_config_json` | 新增（跨命令复用；仅 `source-config` / `source-config-json`） | `aitable +datasource-create`、`aitable +datasource-update`、`aitable +datasource-get-fields` |

### 3.2 命令级 Override

| 命令 | 处理原则 |
|---|---|
| `aitable +base-bootstrap` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `aitable +table-bootstrap` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `aitable +url-resolve` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `aitable +resolve-base` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `aitable +datasource-create` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `aitable +datasource-update` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `aitable +datasource-sync` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `aitable +datasource-sync-status` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `aitable +datasource-get-config` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `aitable +datasource-list-sources` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `aitable +datasource-get-fields` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |

正式规则新增 46 条 PreParse 验证 fixture，覆盖 alias/canonical、block 与 ambiguous 三类路径。完整命令模板位于 `internal/app/param_alias_payload_equivalence_test.go`。

## 4. 当前不能自动解决

- `aitable +base-bootstrap`：创建目标与已有 Base/Table ID 易混；tables 是定义 JSON。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `aitable +table-bootstrap`：fields 定义 JSON 与 field-ids 列表不是同一实体。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `aitable +url-resolve`：URL 与拆出的 base/table/view/record ID 不能互换。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `aitable +resolve-base`：名称唯一解析与关键词搜索是不同执行语义。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `aitable +datasource-create`：sourceConfig、自动同步配置、字段 ID 列表三种结构并存。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `aitable +datasource-update`：两个 ID + 两类 JSON + 字段列表角色冲突。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `aitable +datasource-sync`：同步要求列表，不能把单个 table-id 自动扩成列表。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `aitable +datasource-sync-status`：单表 ID 与任务 ID 列表并存。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `aitable +datasource-get-config`：返回的配置容易被误当成请求参数。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `aitable +datasource-list-sources`：目录 source 与 sourceConfig 容易混淆。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `aitable +datasource-get-fields`：field-ids 是输出，不是请求字段；config 角色不完整。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。

## 5. 第一轮推荐

1. 先合入值域明确、值不变的 concept/命令级别名。
2. 对 ID 域、单复数、两种真实兼容 flag 并存、文件传输编码和多角色参数优先保留 block/ambiguous。
3. 不通过别名表实现名称查 ID、URL 拼装、裸文件路径补 `@`、枚举值翻译或分页模型转换。
4. 写操作的确认语义由真实命令负责，候选 fixture 不注入 `--yes`；测试模板另行验证移除 `--yes` 后仍停在确认门禁。

## 6. 候选表差异与实体必要性审计

- 变更 concept：`base_id`、`table_id`、`aitable_field_ids`、`aitable_datasource_config_json`。
- 新增 command override：11 条。
- 新增 fixture：46 条。
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
