# Whiteboard CLI 参数幻觉复核报告

## 1. 结论

冻结基线为 `origin/main@f4474b57eb1db23b1638b9574be2f5dca368a360`。本轮覆盖 2 个此前未进入正式参数兜底范围的公开 Shortcut；识别出 0 个可直接自动兜底、0 个只能部分兜底、2 个只应增加保护的命令，其中高风险写入或敏感读取 1 个。

本报告中的修订候选已完成正式落地复核；Whiteboard 因官方别名生成源树仍未覆盖 runnable leaf，正式 `internal/cli/param_concepts.json` 继续保持零条 Whiteboard 规则。其余产品规则已重放到最新 main。

## 2. 参数问题明细

| # | 命令 | 真实参数焦点 | 幻觉风险 | 候选处理 | 落地能力 | 风险 |
|---:|---|---|---|---|---|---|
| 1 | `whiteboard +query` | `node,part-id` | 运行时 Help/Schema 可见，但 go generate 使用的官方参数别名源树未收录该 runnable leaf | 现有 param_concepts 链路无法安全落地；保持候选不变并登记框架缺口 | 仅保护 | 中 |
| 2 | `whiteboard +update` | `node,part-id,source` | 除源树缺口外，source 的裸 file-path 还需要补 @，不满足原值传递 | 不写入不可达 override；不得把 file-path 映射到 source | 仅保护 | 高 |

## 3. 可直接解决的方案

### 3.1 Concept 变更

| Concept | 处理 | 命令范围 |
|---|---|---|
| — | 无新增或扩展 | — |

### 3.2 命令级 Override

| 命令 | 处理原则 |
|---|---|
| — | 本产品当前不具备可达的参数别名落地点 |

Whiteboard 正式规则仍新增 0 条 PreParse fixture；完整命令模板保留在 `internal/app/param_alias_payload_equivalence_test.go`，用于确认不可达边界没有被误放开。

## 4. 当前不能自动解决

- `whiteboard +query`：运行时 Help/Schema 可见，但 go generate 使用的官方参数别名源树未收录该 runnable leaf。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。
- `whiteboard +update`：除源树缺口外，source 的裸 file-path 还需要补 @，不满足原值传递。候选只处理“值不变且角色明确”的别名；其余通过 block/ambiguous 停止，不做查找、转换、枚举翻译或单复数扩展。

## 5. 第一轮推荐

1. 先合入值域明确、值不变的 concept/命令级别名。
2. 对 ID 域、单复数、两种真实兼容 flag 并存、文件传输编码和多角色参数优先保留 block/ambiguous。
3. 不通过别名表实现名称查 ID、URL 拼装、裸文件路径补 `@`、枚举值翻译或分页模型转换。
4. 写操作的确认语义由真实命令负责，候选 fixture 不注入 `--yes`；测试模板另行验证移除 `--yes` 后仍停在确认门禁。

## 6. 候选表差异与实体必要性审计

- 变更 concept：无。
- 新增 command override：0 条。
- 新增 fixture：0 条。
- 新 concept 仅在同一实体跨多个命令重复出现时建立；单命令或单角色字段全部下沉到 command override。
- 没有新增 role-free 的 `id`、`name`、`status`、`config`、`content` concept。

完整候选见同目录 `param_concepts.json`。

## 7. 事实依据与验证门禁

事实优先级：同提交 Help/Cobra > 同提交运行时 Schema > 产品 Skill > 当前正式别名表。

- Help/Cobra：冻结提交重新构建的 `./dws <command> --help`。
- Runtime Schema：同一二进制 `./dws schema --cli-path <path> --compact -f json`。
- Skill：`skills/multi` 下对应产品 reference。
- 正式表：评审候选基于冻结基线完整复制；当前分支正式落地时继续保留 Whiteboard 零变更结论。
- 已通过：JSON/Schema、`go generate ./internal/cli`、全部既有与新增 PreParse fixture、alias/canonical、block/ambiguous、candidate-only 完整命令模板、移除 `--yes` 的确认不可绕过测试、非目标别名回归、`check-generated-drift.sh`、`check-schema-catalog.sh`。
- 评审阶段在隔离副本验证；正式落地阶段重新生成并确认 Whiteboard 仍不可达、无 stale override。

## 8. 可复用流程

1. 冻结 commit、构建二进制并导出命令/Help/Schema 证据。
2. 对每个参数标注实体、值域、角色、基数、真实/兼容/不可用状态。
3. 先判断是否已有可复用 concept；只在跨命令重复实体时新增 concept。
4. 角色局部差异下沉到 command override；需要查找/转换/扩展时拒绝 alias。
5. 为正向 alias、canonical 不变、block、ambiguous 与非目标命令补 fixture 和完整命令模板。
6. 在隔离副本验证候选；评审通过后合入其余四产品规则、重新生成并合并最新 main。
