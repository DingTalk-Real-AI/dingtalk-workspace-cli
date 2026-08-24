# Devdoc 产品 CLI 参数幻觉分析

## 结论摘要

本分析以线上 `origin/main` 冻结提交
`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df` 为唯一基线，使用该提交重新构建的
`dws`、运行时 Schema、官方 Cobra 树、实现代码、dingtalk-misc Devdoc Skill 与冻结正式
`internal/cli/param_concepts.json`。未使用固定 Catalog、历史 badcase、用户 Shortcut 或已安装
插件，也没有修改当前工作区正式别名表。

Devdoc 产品有 1 个 Agent 可见叶 `devdoc article search`，另有 1 个隐藏 hint-only 兼容节点
`devdoc search`，后者只报错并指向正式路径，不执行业务检索。相同 MCP 接口还被 `dev doc
search` 暴露为 Dev 产品叶，本轮不跨产品扩大规则。

主要风险是检索词三入口被误认为三个不同参数、页码/每页数量与 cursor/offset 混用、非法数字
被 Runtime 静默回退、对象 ID 或错误诊断字段被误当可独立搜索参数，以及 Help/Skill 的默认
输出和必填描述与真实行为不完全一致。候选扩展既有 `search_query` 与 `page_number` concept，
新增 1 个精确 override 和 17 个验证 fixture；所有自动映射均保持字符串或数字文本原值，不做
拼接、查询、数值修复或跨命令路由。

候选已通过生成器、PreParse、9 组 alias/canonical payload 比较、9 组 block/ambiguous、
非法数值与双入口行为验证、非目标结构恒等、`internal/cli`、`internal/pipeline`、generated
drift、Schema Catalog 政策与完整 `internal/app`（295.860 秒）。别名候选可以进入落地评审；
Help/Skill 漂移和 Runtime 数值/双入口行为仍需独立修复，不能把产品整体标为完全解决。

## 参数问题

### 1. 检索词存在位置参数、公开 flag 和隐藏原生兼容 flag 三个入口

真实命令接受位置 `[keyword]`、公开 `--query` 和隐藏 `--keyword`。Schema 正确发布位置
`keyword` 与参数 `query` 的 require-one-of；Skill 只展示 `--query`，容易让 Agent 把 `q`、
`keywords`、`search-word`、`search-term` 或 `query-text` 当成不存在的新字段。

候选把该命令加入既有 `search_query` concept，并在精确 override 中补
`search-term/query-text → query`。隐藏 `--keyword` 保持原生，不重复重写；位置参数也保持
原生，因为 PreParse 不能把 flag 值搬成 argv。若位置值和 `--query` 同时提供，Runtime 静默
采用 flag 值，参数表不能改成冲突错误。

### 2. 一基页码、每页数量、offset 和 cursor 属于不同分页模型

该接口只有一基 `--page` 和每页上限 `--size`。正式表已将 `limit/page-size/max-results/...`
归一到 size，并将 `page-no/current-page/page-num` 归一到 page；但常见 `page-number` 和
`results-per-page` 尚未覆盖。cursor、page-token、offset、page-index 则不是同一模型。

候选把 `page-number` 加入只作用于本命令的 `page_number` concept，并增加
`results-per-page → size`。cursor/offset/zero-based index 明确 block，不把值改写为页码。

### 3. CLI 接受字符串，Runtime 才转数字且非法值静默回退

Cobra Help/完整 Schema 的 CLI type 是 string，Contract 同时声明 MCP interface type 为
number。实现用 `strconv.Atoi`，`page < 1` 或解析失败回退 1，`size < 1` 或解析失败回退 10；
因此 `--page abc --size 0` 不会报参数错误，而会悄悄执行第一页、每页十条。

别名表只能改 flag 名，不能验证、转换或限制值。正式修复应把 page/size 声明为正整数或在
RunE 显式拒绝非法输入，并同步 Help/Schema。候选仅记录该边界，不伪造数值治理。

### 4. 开放平台文章检索不是对象读取、业务文档搜索或结构化错误诊断

`devdoc article search` 把一个完整 query 字符串传给 `search_open_platform_docs`。它不接受
doc-id、node-id、space-id 等对象标识，也没有 request-id、error-code、error-message、context
等独立字段；这些值若要搜索，需要由调用者明确组成 query。业务文档搜索应走 drive/wiki/doc。

候选 block 对象 ID、错误诊断字段和需要拼接的 context；裸 `id/type` ambiguous。它不把多个
字段合成 query，不自动切到其他产品，也不把隐藏 hint-only `devdoc search` 当真实叶。

### 5. Help、Schema 与 Skill 的公开契约存在描述漂移

冻结 Help/Skill 称默认表格输出，但全局 `--format` 默认值和 mock 实际输出均为 JSON；只有显式
`--format table` 才走 Devdoc 表格渲染。Help/Skill 又将 `--query` 标为必填，却没有完整说明
位置 keyword 与隐藏兼容 flag；Schema 则正确发布 query 可选并用 require-one-of 约束。

这属于 Help/Skill/声明来源修复，不是别名问题。第一轮落地应同步默认输出说明、检索词三入口
和正整数语义，避免 Agent 根据文档生成多余参数或误判返回形态。

## 当前别名表可以实施的方案

1. 将 `devdoc article search` 加入既有 `search_query` concept，覆盖 q、keywords、search-word。
2. 在精确 override 增加 search-term/query-text 到 query，以及 results-per-page 到 size。
3. 为一基页码 concept 增加 page-number；保留正式表已有分页 size/page 同义词。
4. 对 cursor/offset/page-index、对象 ID、错误诊断字段做安全拦截，裸 id/type 提示歧义。
5. 保持位置 keyword 与隐藏 `--keyword` 原生，不把 hint-only 或 Dev 产品兼容叶纳入本产品规则。

## 当前能力支持不了的事项

- 把 `--query-like-flag` 的值搬成位置 argv，或反向重排 argv；
- 在位置 keyword 与 `--query` 同时出现时自动报冲突或选择另一方；
- 把多个 error/context/API 字段拼接成一个检索 query；
- 验证、转换或修复 page/size 数值，区分非法值与显式默认值；
- 把 offset/cursor/page-token 换算成一基页码；
- 根据 doc-id、URL 或业务文档意图自动切到 drive/wiki/doc；
- 把 `devdoc search` hint-only 节点升级成真实别名命令；
- 修改默认输出格式，或修复 Help/Skill/Schema 文案漂移；
- 把同接口的 `dev doc search` 跨产品规则一并扩大。

## 第一轮改造建议

第一轮建议落地 1 个 concept scope 扩展、1 个 page-number member 和 1 个 scope_strict override。
同时修改 Devdoc Help/Skill：明确真实默认 JSON、显式 table、位置 keyword/公开 query 的二选一，
以及 page/size 正整数规则。Runtime 应另加非法数值显式错误与双输入冲突校验；这些代码修复不应
由 alias 表代替。

## 候选 `param_concepts.json` 改动与审核

- `search_query.commands` 仅新增 `devdoc article search`；
- `page_number.members` 仅新增 `page-number`，该 concept 仍只作用于 Devdoc 正式叶；
- 新增 1 个 `scope_strict` command override；
- 新增 17 个 fixture：8 active、9 block/ambiguous；
- 8 active 中 7 条对应新增生成 alias，1 条复核既有 max-results；原生 hidden keyword 通过独立
  行为测试验证，不伪装成中央 alias fixture；
- 生成作用域仍为 569，Devdoc entry 从 10 alias、4 blocked、0 ambiguous 变为
  17 alias、23 blocked、2 ambiguous；fallback 无变化；
- 自动 alias 的来源均不是真实 flag，目标均为真实 canonical flag；值域、角色、基数和单位一致；
- guard 与真实可见/隐藏/全局 flags 冲突为 0；
- 删除本产品改动后，非目标 JSON 结构与冻结正式表恒等；
- `devdoc search` 是 hint-only，`dev doc search` 属于 Dev 产品，均未被误纳入候选范围。

审核结论：规则业务上合理，生成、最终 payload 与完整应用门禁均通过，可进入正式落地评审。候选位置：
`docs/parameter-hallucination/devdoc/param_concepts.json`。

## 验证结果与正式替换前置条件

| 验证项 | 结果 | 说明 |
|---|---|---|
| JSON 解析与生成器 | 通过 | 569 个命令作用域；Devdoc 17 alias、23 blocked、2 ambiguous |
| PreParse 与 alias/canonical | 通过 | 9 组最终 dry-run payload 完全一致 |
| block/ambiguous | 通过 | 9 组代表错误均在 MCP dispatch 前停止 |
| 原生入口 | 通过 | 位置 keyword、隐藏 `--keyword` 与 query payload 一致 |
| 数值边界 | 已确认缺口 | 非数字或非正 page/size 静默回退 1/10 |
| 双入口边界 | 已确认缺口 | 位置词与 query 同时出现时 query 静默覆盖 |
| 非目标回归 | 通过 | 结构恒等；generated diff 仅 Devdoc entry；fallback 无变化 |
| `internal/cli`、`internal/pipeline` | 通过 | 最终候选：CLI 81.968 秒；pipeline 0.472 秒 |
| generated drift | 通过 | 参数别名与 Schema 双次装配确定 |
| Schema Catalog 政策 | 通过 | 28 产品、1166 工具；Runtime confirmation truth 通过 |
| 完整 `internal/app` | 通过 | 295.860 秒；既有 Devdoc complete-command 模板覆盖新增 active fixture |

正式替换时应一并评审 Help/Skill 输出与必填描述；数值和双输入是否与 alias 同 PR 修复，由
Runtime 维护者决定，但不能把这些行为描述为候选已经解决。

## 分析依据

- 冻结提交：`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df`，提交时间
  2026-08-20 10:53:27 +08:00；
- 正式表 SHA-256：
  `e41e7908c26dfdcc23636d27661d705416c001d67a6d0d5d658b1ca4bcc815c1`；
- 候选 SHA-256：
  `1aa25a72e583aca9853beb262316e8e11543fcde0e0b2dc59ccd070484b78b82`；
- 命令实现：`internal/helpers/devdoc.go`、`helpers.go`；
- Skill：dingtalk-misc `references/devdoc.md` 与 `devdoc-intent-guide.md`；
- Schema：同一冻结二进制运行时声明组装的完整 leaf；interface property 和 number 类型均来自声明；
- 官方树边界：1 个 Devdoc Agent 叶、1 个隐藏 hint-only 节点；同接口 Dev 叶不属于本产品；
- 明确未使用：固定 Catalog、历史 badcase、评测工作簿、用户 Shortcut、已安装插件。

## 可复用分析流程

先把 Agent 叶、隐藏兼容 flag、位置参数、hint-only 节点和跨产品同接口叶分层；再逐项核对
CLI type、interface type、默认值、require-one-of 与 Runtime 转换；仅对可原值传递的同角色名称
开放 alias，对分页模型、对象 ID、结构化字段和数值转换 fail-closed；最后用 dry-run payload、
完整应用测试和仓库政策共同决定落地状态，并把 Help/Skill 漂移单列为来源修复。
