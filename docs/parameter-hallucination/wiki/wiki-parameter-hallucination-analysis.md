# Wiki 参数幻觉系统重审（最新 main 修订版）

> 状态：候选草稿，未提交、未推送、未合并，也未替换仓库正式 `internal/cli/param_concepts.json`。

## 一、给老板的结论

本次在刷新后的最新 `origin/main` 上重新完成 Wiki 参数兜底审计，并修复了上一版候选的三个确定问题：

1. 候选已从最新正式 `internal/cli/param_concepts.json` 重建，不再覆盖或回退 Calendar、Chat 等非 Wiki 规则。
2. `wiki +node-copy`、`wiki +move`、`wiki +move-to-drive`、`wiki +node-delete` 已补齐安全的 `--node-id → --node`。
3. `wiki +member-add/update/remove` 已将 `--user-id/--uid` 安全归一化到 Shortcut 自身真实、同类型、可重复的 `--user`。

当前候选覆盖 Wiki **36 个公开可执行叶子**（16 个原生命令、20 个内建 Shortcut），复用 6 个正式 concept，并新增 36 条精确 Wiki command override。临时应用候选后生成：

| 项目 | 数量 |
|---|---:|
| Wiki 有效命令规则 | 36 |
| alias | 292 |
| block | 426 |
| ambiguous | 45 |
| Wiki validation fixture | 10 |

候选的参数实体和值域边界总体合理：知识库 `workspaceId`、Drive 存储 `spaceId`、Wiki `nodeId`、目标文件夹 ID、成员 `userId`、`openDingTalkId` 继续严格区分；同时保留原生与 Shortcut 在分页、枚举、参数类型和确认策略上的真实差异。

仍有两类不属于中央参数表的命令拥有层问题：原生 Wiki 的通用隐藏分页别名，以及原生成员隐藏 user 别名。它们已经是 Cobra 真实 flag，中央 PreParse 必须避让，只能在命令实现或通用注册器中另行修复。

## 二、精确基线与事实来源

| 项目 | 本次事实 |
|---|---|
| 最新基线 commit | `379f625ca96f74acf7d5464c29a1332af1b83e96` |
| 基线核验 | 独立 Worktree `HEAD == origin/main` |
| 动态 Schema | 28 products、1166 tools |
| Registry hash | `sha256:a117cb3cd08f89b4ce6041f3287a98ba9bb43aa5f87d3371598f298c38a90035` |
| Source hash | `sha256:aeeae985d24c464b0c8bb84cc34724f97ace4d6a9b2da6295286d88eee2db7e2` |
| Wiki 审计范围 | 36 个公开叶子；另核验隐藏 proxy、Cobra command alias 和通用 hidden flag 注册 |
| 事实优先级 | 当前 Cobra/运行时 Contract > 当前动态 Schema > 当前 Skill |

历史 Wiki 结果只用于定位需要重新核验的风险，未作为当前事实来源。候选构建以最新正式 `internal/cli/param_concepts.json` 为底稿，Wiki 之外的 concept 元数据、命令作用域、override 和 fixture 均逐字段保持一致。

## 三、最新正式表与候选的差异

| 指标 | 最新正式表 | 当前候选 |
|---|---:|---:|
| concepts | 52 | 52 |
| command overrides | 241 | 277 |
| validation fixtures | 533 | 543 |
| Wiki overrides | 0 | 36 |
| Wiki fixtures | 0 | 10 |

候选 SHA-256：

```text
02896ed0bd596d082aef110e2e47fa141d40cb7113b0c27d3ab78023670cb47e
```

复用的正式 concept：`workspace_id`、`search_query`、`pagination_size`、`page_cursor`、`plain_description`、`user_ids`。

非 Wiki 深比较结果：

- 不缺少最新正式 concept；
- 不修改 concept 的非 `commands` 元数据；
- concept 只新增 Wiki command scope；
- 不修改或删除任何最新正式 override；
- 只新增 36 条 Wiki override；
- 保留全部 533 条正式 fixture，只新增 10 条 Wiki fixture；
- 未带回旧候选中过时的 Chat override，也未丢失最新 Calendar 内容。

## 四、本次修复内容

### 4.1 最新 main 重建

旧候选基于 `c15480c`，如果整体应用到最新 main，会回退 7 个 Calendar concept、33 个 Calendar override 和大量最新 fixture。当前候选改为：

1. 从最新正式参数表完整读取；
2. 只合并旧候选中以 `wiki ` 开头的 concept scope；
3. 只合并 36 条 Wiki override；
4. 再应用本次人工复核的 Shortcut 修正；
5. 对非 Wiki JSON 做逐字段等价检查。

### 4.2 Shortcut `--node-id`

以下 Shortcut 的 `--node` 都明确表示源节点或删除目标节点，`--node-id` 仅改变参数拼写，不改变值、类型或实体角色：

| Shortcut | 修复前 | 修复后 |
|---|---|---|
| `wiki +node-copy` | `--node-id` 被 block | `--node-id → --node` |
| `wiki +move` | `--node-id` 被 block | `--node-id → --node` |
| `wiki +move-to-drive` | 普通 unknown flag，并产生错误粘连提示 | `--node-id → --node` |
| `wiki +node-delete` | `--node-id` 被 block | `--node-id → --node` |

多 ID 角色命令中的通用 `--id` 仍为 ambiguous；`workspace-id`、`folder-id`、`space-id` 不会被误写为 node；`+node-get` 原有的 `node-id/document-id/url → node` 保持不变。

### 4.3 Shortcut `--user-id/--uid`

`wiki +member-add/update/remove` 自身声明：

```text
--users  StringSlice，必填
--user   StringSlice，--users 的公开可重复别名
```

因此当前候选增加：

```text
--user-id → --user
--uid     → --user
```

这不是单数转复数、CSV 转数组或实体转换，而是归一化到 Shortcut 已有的同类型 flag。`open-dingtalk-id`、`open-dingtalk-ids` 和 `staff-id` 继续 block，避免把不同 ID 值域混入 `userId`。

该修复只作用于 Shortcut，不掩盖原生 `wiki member add/update/remove` 的隐藏 flag 接线缺陷。

## 五、原生与 Shortcut 的真实差异

| 业务 | Shortcut | 原生命令 | 关键差异 |
|---|---|---|---|
| 空间列表 | `wiki +space-list` | `wiki space list` | Shortcut 只有 Wiki 类型并支持自动分页；原生还可发现 Drive 存储空间，且只请求单页 |
| 节点列表 | `wiki +node-list` | `wiki node list` | Shortcut 有自动分页控制；原生是单页 |
| 动态列表 | `wiki +feed-list` | `wiki feed list` | Shortcut 有自动分页控制；原生是单页 |
| 成员写入 | `wiki +member-*` | `wiki member *` | Shortcut 是 StringSlice/可重复输入；原生 `--users` 是 CSV string |
| 成员列表 | `wiki +member-list` | `wiki member list` | Shortcut `filter-role` 为 StringSlice；原生为 CSV string；后端均没有 cursor |
| 节点写入 | `wiki +node-copy/+move/+node-delete` | 对应 `wiki node ...` | Shortcut 有更严格的确认和读回/终态验证 |
| 空间搜索 | `wiki +space-search` | `wiki space search` | 原生可通过 `--type myWikiSpace` 路由“我的文档”；Shortcut 不暴露该类型 |

参数表只处理已经解析出准确 command path 后的 flag，不能补漏写的 `+`，也不能替换原生与 Shortcut 的能力差异。

## 六、仍待单独修复的命令拥有层问题

### 6.1 原生成员隐藏 user flags

`wiki member add/update/remove` 公开 `--users`，手工隐藏注册 `--user`。通用注册器又创建真实隐藏 flag：

```text
--user-id
--uid
--user-ids
--user-list
--user-id-list
```

但 `collectUserIDs` 只读取 `--users/--user`。因此这些 flag：

- Cobra 接受，不报 unknown flag；
- 中央 PreParse 看到真实 flag 后必须避让；
- 命令处理层没有值，最终报 `--users is required`；
- transport 调用数为 0。

### 6.2 原生隐藏分页 flags

通用注册器创建的部分分页 flag 同样没有进入 payload：

| 命令 | 未消费的真实隐藏 flag |
|---|---|
| `wiki space list` | `page-size`, `next-token` |
| `wiki space search` | `page-size` |
| `wiki member list` | `page-size` |
| `wiki node list` | `page-size`, `next-token` |
| `wiki node search` | `page-size`, `next-token` |
| `wiki feed list` | `page-size`, `next-token` |

这些问题必须在 `RegisterCrossProductAliases` 的所有权设计或各命令 fallback 读取逻辑中处理，不能通过 `param_concepts.json` 强行覆盖真实 Cobra flags。

## 七、保留的审慎边界

- `workspaceId` 不等于 Drive 数字 `spaceId`。
- Wiki `nodeId` 不等于父/目标 `folderId`，也不等于 Drive 数字 `dentryId`。
- 成员 `userId` 不等于 `openDingTalkId` 或 `staffId`。
- `limit` 是单页大小；带 `max-items` 的 Shortcut 另有聚合上限。因此 `max-results/size/top/take` 在同时存在两种上限时继续 ambiguous。
- 名称搜索串不能被本地改写成 workspace ID。
- CSV string 与 StringSlice/重复 flag 只有在命令已声明同类型兼容入口时才归一化。

还有一项低优先级一致性决策未改动：原生 `wiki feed list --id` 会映射到 `workspace`，而 Shortcut `wiki +feed-list --id` 目前 block。两者都只有一个 workspace 范围参数，正式落地前建议统一政策或补充差异理由。

## 八、验证记录

候选只在独立 Worktree 中临时替换正式输入；所有正式输入与生成文件均通过退出 trap 恢复。

| 验证 | 结果 |
|---|---|
| JSON Schema/结构解析 | 通过 |
| 非 Wiki 深比较 | 通过：最新正式内容逐字段保留 |
| `go generate ./internal/cli` | 通过：605 commands，34 command-path fallbacks |
| Wiki 生成规则统计 | 36 entries / 292 aliases / 426 blocked / 45 ambiguous |
| `go test ./internal/cli ./internal/pipeline ./internal/shortcut/wiki -count=1` | 通过 |
| `make build` | 通过 |
| `+node-copy --node-id` 与 `--node` dry-run payload 等价 | 通过 |
| 3 个 `+member-*` 的 `--user-id/--uid` 与 `--user` payload 等价 | 6/6 通过 |
| `+move/+move-to-drive/+node-delete --node-id` PreParse/Help 解析 | 3/3 通过 |
| `make generate-schema` | 通过；两次组装 hash 一致 |
| `check-generated-drift.sh` | 通过 |
| `check-schema-catalog.sh` | 通过：28 products / 1166 tools |

Schema catalog 首次在受限沙箱内执行时，仅因 `httptest` 无权绑定 `[::1]:0` 失败；同一候选在允许本机 loopback 测试端口的环境重跑后通过，产品断言没有失败。

## 九、交付与工作区状态

- 当前候选：`docs/parameter-hallucination/wiki/param_concepts.json`。
- 当前中文分析：本文件。
- 正式 `internal/cli/param_concepts.json` 已恢复到最新 main 原状。
- 正式生成文件已恢复到最新 main 原状。
- 未提交 commit、未 push、未创建 PR、未合并。
