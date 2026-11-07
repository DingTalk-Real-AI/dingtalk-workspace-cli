# DevApp CLI 参数幻觉复核报告

## 1. 结论

冻结基线为 `origin/main@f4474b57eb1db23b1638b9574be2f5dca368a360`。本轮覆盖 6 个此前未进入正式参数兜底范围的公开 Shortcut；识别出 0 个可直接自动兜底、6 个只能部分兜底、0 个只应增加保护的命令，其中高风险写入或敏感读取 6 个。

本报告中的修订候选已合入正式 `internal/cli/param_concepts.json`，并重放到最新 main。所有映射均满足：源参数不是该命令真实 flag、目标是该命令真实 flag、实体/角色/值域/基数一致、值原样传递、不绕过确认。

## 2. 参数问题明细

| # | 命令 | 真实参数焦点 | 幻觉风险 | 候选处理 | 落地能力 | 风险 |
|---:|---|---|---|---|---|---|
| 1 | `devapp +credentials-get` | `unified-app-id` | unifiedAppId 易与 appKey/agentId/robotCode 混用 | 复用 app_id；其他 ID 域阻断 | 部分兜底 | 高 |
| 2 | `devapp +robot-config` | `unified-app-id,name,brief,desc,icon-media-id,outgoing-url,event-callback-url,mode,skills` | 两个回调 URL、机器人名称、简介/描述、mediaId 与技能 ID 列表角色并存 | 复用必要 concept；`skill-list` 保留 ID 列表原值，`skill-names` 保持歧义；仅角色完整 URL 别名 | 部分兜底 | 高 |
| 3 | `devapp +robot-enable` | `unified-app-id` | 应用 ID 易被写成机器人 Code 或 appKey | 复用 app_id；机器人/凭证 ID 阻断 | 部分兜底 | 高 |
| 4 | `devapp +robot-disable` | `unified-app-id` | 停用写操作不能容忍错误 ID 域 | 复用 app_id；机器人/凭证 ID 阻断 | 部分兜底 | 高 |
| 5 | `devapp +event-subscribe` | `unified-app-id,event-codes` | 事件 code 列表与单值、权限 scope 列表易混 | 复用 dev_event_codes；阻断单值与其他列表域 | 部分兜底 | 高 |
| 6 | `devapp +version-create` | `unified-app-id,version,desc` | 新版本标签不是 versionId/taskId | 版本角色别名映射到 version；ID 形态阻断 | 部分兜底 | 高 |

## 3. 可直接解决的方案

### 3.1 Concept 变更

| Concept | 处理 | 命令范围 |
|---|---|---|
| `plain_description` | 扩展既有 concept 命令范围 | `devapp +robot-config`、`devapp +version-create` |
| `app_id` | 扩展既有 concept 命令范围 | `devapp +credentials-get`、`devapp +robot-config`、`devapp +robot-enable`、`devapp +robot-disable`、`devapp +event-subscribe`、`devapp +version-create` |
| `dev_event_codes` | 扩展既有 concept 命令范围 | `devapp +event-subscribe` |
| `dev_robot_name` | 扩展既有 concept 命令范围 | `devapp +robot-config` |

### 3.2 命令级 Override

| 命令 | 处理原则 |
|---|---|
| `devapp +credentials-get` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `devapp +robot-config` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `devapp +robot-enable` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `devapp +robot-disable` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `devapp +event-subscribe` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |
| `devapp +version-create` | bind/scoped_aliases/block/ambiguous 按命令事实独立评审 |

正式规则新增 24 条 PreParse 验证 fixture，覆盖 alias/canonical、block 与 ambiguous 三类路径。完整命令模板位于 `internal/app/param_alias_payload_equivalence_test.go`，其中 `devapp +robot-config` 显式携带 canonical `--skills`。

## 4. 当前不能自动解决

- `devapp +credentials-get`：unifiedAppId 易与 appKey/agentId/robotCode 混用。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `devapp +robot-config`：两个回调 URL、机器人名称、简介/描述、mediaId 与技能 ID 列表角色并存。`skill-list` 只保留原始 ID 列表，`skill-names` 不推断名称到 ID；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `devapp +robot-enable`：应用 ID 易被写成机器人 Code 或 appKey。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `devapp +robot-disable`：停用写操作不能容忍错误 ID 域。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `devapp +event-subscribe`：事件 code 列表与单值、权限 scope 列表易混。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `devapp +version-create`：新版本标签不是 versionId/taskId。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。

## 5. 第一轮推荐

1. 先合入值域明确、值不变的 concept/命令级别名。
2. 对 ID 域、单复数、两种真实兼容 flag 并存、文件传输编码和多角色参数优先保留 block/ambiguous。
3. 不通过别名表实现名称查 ID、URL 拼装、裸文件路径补 `@`、枚举值翻译或分页模型转换。
4. 写操作的确认语义由真实命令负责，候选 fixture 不注入 `--yes`；测试模板另行验证移除 `--yes` 后仍停在确认门禁。

## 6. 候选表差异与实体必要性审计

- 变更 concept：`plain_description`、`app_id`、`dev_event_codes`、`dev_robot_name`。
- 新增 command override：6 条。
- 新增 fixture：24 条。
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
