# PAT 产品 CLI 参数幻觉分析

## 结论摘要

本分析以线上 `origin/main` 冻结提交
`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df` 为唯一基线，使用该提交重新构建的
`dws`、运行时 Schema、Cobra Help、PAT 实现、dingtalk-misc PAT Skill 与冻结正式
`internal/cli/param_concepts.json`。未使用固定 Catalog、历史 badcase、用户 Shortcut 或
已安装插件，也没有修改当前工作区正式别名表。

PAT 有 2 个 Agent 可见叶：高风险远端授权 `pat chmod` 与本地浏览器策略写入
`pat browser-policy`。参数幻觉的主要风险不是普通拼写错误，而是把位置 scope 猜成 flag、
混淆 product/domain 的单复数入口、把 PAT 行为授权误当开放平台应用权限、把 force/confirm
映射成 `--yes`，以及把 `disable-browser` 错映射到正向布尔 `--enabled`。

最终候选没有新增 concept，只增加 2 个精确 command override 和 28 个验证 fixture。它已
通过生成器、PreParse、6 组 alias/canonical 输出逐字节比较、14 组代表 block/ambiguous、
非目标结构恒等、`internal/cli`、`internal/pipeline`、generated drift 与 Schema Catalog
政策。另验证出一个真实契约缺口：`browser-policy --dry-run` 仍会写本地策略；验证只发生在
隔离 HOME，候选保持真实全局 flag 原生，不用参数表掩盖该行为。完整 `internal/app` 用时
306.298 秒，失败严格收敛为候选尚未落地导致的 2 个 complete-command 模板与 14 条 active
fixture 缺口，其余应用测试完成。

## 参数问题

### 1. scope 是位置参数，不能从 `--scope/--scopes` 自动转换

`pat chmod <scope>...` 的 scope 格式为 `<product>.<entity>:<permission>`，可重复，且与
`--product/--products/--domain/--domains/--recommend` 构成 require-one-of。中央参数链只
重写 flags，不能删除 flag token 后把值搬成位置 argv。

候选因此 block `scope/scopes/permission/permissions/permission-scope(s)`，而不是伪造
alias。scope 格式、合法性、服务端 selected/skipped/pending 和 legacy scope/scopes wire
兼容均不属于参数名归一能力。

### 2. product/domain 单值重复与 CSV 入口需要保持角色和基数

`--product`、`--domain` 是可重复 StringArray；`--products`、`--domains` 是 CSV
StringSlice。实现最终都会按逗号拆分、去空和去重后汇入 productCodes，但公开参数仍明确区分
单值/复数入口。

候选只允许 `product-code → product`、`product-codes → products`、
`domain-code → domain`、`domain-codes → domains`。泛化 service/services ambiguous；
不把单值拼成列表，不在 product/domain 之间自动换角色，也不查询产品模板。

### 3. grant-type、session-id 与 agentCode 是三个独立值域

`grant-type` 枚举为 once/session/permanent；session 模式还要求 `--session-id` 或受支持的
会话环境变量。`agentCode` 只允许 `[A-Za-z0-9_-]{1,64}`，chmod 可从
`DINGTALK_DWS_AGENTCODE` 回退，browser-policy 写入则必须显式提供 agentCode 才会写 agent
级策略。

候选把 `grant-mode/authorization-mode → grant-type`，
`pat-session-id/authorization-session-id → session-id`。`agent-code → agentCode` 已由正式
kebab/camel 规则处理，不重复造 concept。裸 agent/agent-id、session、id、type/mode
ambiguous；候选不改枚举值、不读取环境变量，也不把 session 当 agentCode。

### 4. PAT 行为授权、开放平台应用权限和浏览器策略不是同一产品动作

`pat chmod` 改变 Agent 可执行的行为 scope；`dev app permission` 管理开放平台应用权限；
`pat browser-policy` 只写本地是否允许打开浏览器，不授予任何业务权限。

候选在两个 PAT 叶上 block `app-id/app-key/app-secret/oauth-scope/api-scope`；chmod 上 block
browser policy 拼写，browser-policy 上 block scope/product/domain/grant/session 拼写。它只
提供清晰路由错误，不尝试跨命令或跨产品执行第二步。

### 5. 推荐集合与“全部授权”不能混为一个布尔开关

`--recommend` 让服务端计算推荐 scope；不存在 `--all/--all-scopes`。计划结果可能包含
selected/skipped/pending，真正批量执行还需用户确认。`all` 既可能表示所有产品，也可能表示
所有 scope，自动扩张会造成过度授权。

候选仅允许正向拼写 `recommended → recommend`；`all/all-scopes` block。它不能把推荐计划
转成所有权限，也不能从自然语言推导产品、域或 scopes。

### 6. `--yes` 是授权安全门，不能由 force/confirm 生成

Schema 发布 `chmod confirmation=user_required`。批量 scope、产品/域计划与推荐授权的
Runtime 还显式要求 `--yes`；`--dry-run` 用于先检查计划。把 `force/confirm/confirmed` 映射
为 `yes` 会把普通拼写纠错升级成用户授权。

候选 block 这些非真实确认拼写，完全不生成 `--yes`。参数表也不替用户选择 once/session/
permanent、有效期或 scope。

### 7. browser-policy 的正向布尔与反向动词不能自动互换

`--enabled` 是必填 boolean；`--enabled=false` 禁止浏览器。`open-browser/browser-enabled/
allow-browser` 与 enabled 同为正向布尔，值可原样传递；`disable-browser/deny-browser/
browser-disabled` 则需要逻辑取反。

候选只映射三个正向名称，反向名称 block，泛化 policy/browser/state/value/mode ambiguous。
它不会因省略值猜 false，也不会把 agentCode 环境回退误用于策略写入。

### 8. browser-policy 继承真实 `--dry-run`，但不会预览

全局 Help 展示 `--dry-run`，但 browser-policy RunE 不读取它，会直接更新本地
`pat_policy.json`。隔离验证确认加 `--dry-run` 仍退出 0 并产生策略文件。这不是 alias 问题，
且生成器禁止 block 真实 flag。

正式修复应让命令尊重 dry-run，或通过声明/Help 明确不支持；在此之前，Agent 不应把
browser-policy 的 dry-run 当成预览。候选只能记录风险，不能改变 Runtime。

## 当前别名表可以实施的方案

1. 为 `pat chmod` 建立 product/domain、grant/session、recommend 的精确原值别名。
2. 为 `pat browser-policy` 建立三个正向 enabled 别名。
3. 复用既有 kebab/camel 规则处理两个命令的 `agent-code → agentCode`。
4. 对位置 scope、all-scope、确认绕过、开放平台权限、跨叶参数和反向布尔做保护。
5. 补齐 2 个 complete-command payload 模板后再正式替换。

## 当前能力支持不了的事项

- 把 `--scope/--scopes` 转成一个或多个位置参数；
- 解析、补全或验证 `<product>.<entity>:<permission>`；
- 查询产品/域模板并把自然语言权限变成 scopes；
- 把单值拆成列表、把多个值合并成 CSV，或改变 product/domain 角色；
- 把中文/同义授权类型转换为 once/session/permanent；
- 自动获取、验证或生成 session-id/agentCode；
- 把推荐集合扩张成 all scopes，或自动处理 pending flow；
- 把 force/confirm 自动改成 `--yes` 或替用户确认；
- 把 disable/deny 反向动词转换成 `enabled=false`；
- 用参数表修复 browser-policy 的 dry-run 行为；
- 把 PAT 行为授权切换为开放平台应用权限命令；
- 在没有 complete-command 模板时直接替换正式表。

## 第一轮改造建议

第一轮建议落地 2 个 scope_strict override，不新增全局 concept。落地 PR 必须为
`pat chmod` 与 `pat browser-policy` 补 complete-command E2E 模板，覆盖 14 条 active
fixture；browser-policy 模板必须使用隔离 `DWS_CONFIG_DIR`。同时另开 Runtime 修复，让
browser-policy 尊重 `--dry-run` 或从该命令公开契约中消除误导。

## 候选 `param_concepts.json` 改动与审核

- 新增 concept 0；新增 2 个 command override；
- 新增 28 个 fixture，其中 14 个 active、14 个 block/ambiguous；
- active 中 12 条来自 override，2 条验证既有 kebab/camel morphology；
- `go generate ./internal/cli` 从 569 个命令作用域变为 571 个；
- 生成 12 个 alias、42 个 blocked、17 个 ambiguous token；fallback 无变化；
- 非目标 concept、override、fixture 结构恒等，guard 与真实 flag 冲突为 0；
- 6 组 alias/canonical 退出码、stdout、stderr 逐字节相同；
- 14 组保护检查全部以 blocked_flag 或 ambiguous_flag 在派发前退出；
- browser-policy dry-run 写入只在隔离 HOME 验证，未触碰用户正式策略。

候选位置：`docs/parameter-hallucination/pat/param_concepts.json`。

## 验证结果与正式替换前置条件

| 验证项 | 结果 | 说明 |
|---|---|---|
| JSON 解析与生成器 | 通过 | 571 个命令作用域 |
| PreParse 与 alias/canonical | 通过 | product、domains、direct、recommend、browser、browser-agent 六组一致 |
| block/ambiguous | 通过 | 14 组代表错误均在派发前停止 |
| 原生 morphology | 通过 | agent-code → agentCode，无重复 concept |
| 安全行为 | 部分通过 | chmod dry-run 不写授权；browser-policy dry-run 仍写隔离本地策略 |
| 非目标回归 | 通过 | JSON 结构恒等；生成 diff 仅 2 个 PAT entry；fallback 无变化 |
| `internal/cli`、`internal/pipeline` | 通过 | CLI 82.088 秒 |
| generated drift | 通过 | alias 与 Schema 双次装配确定 |
| Schema Catalog 政策 | 通过 | 28 产品、1166 工具；Runtime confirmation truth 通过 |
| 完整 `internal/app` | 条件未通过 | 306.298 秒；仅缺 2 个 PAT 模板与对应 14 条 active fixture |
| complete-command payload 门禁 | 未通过 | 200/202 个活跃命令有模板；缺 2 个命令、14 条 active fixture；393 active cases |

正式替换前必须补齐 2 个模板并重跑完整 `internal/app` 与政策门禁。browser-policy dry-run
属于独立 Runtime 契约缺口；是否作为同一落地 PR 阻塞项，应由安全评审决定，候选本身不能修复。

## 分析依据

- 冻结提交：`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df`，提交时间
  2026-08-20 10:53:27 +08:00。
- 正式表 SHA-256：
  `e41e7908c26dfdcc23636d27661d705416c001d67a6d0d5d658b1ca4bcc815c1`。
- 候选 SHA-256：
  `d09712be8b70294c08a67c7977d9e8f028be51777b6f9563ac51e2d87d37ea03`。
- 命令实现：`internal/pat/chmod.go`、`browser_policy.go`、`pat.go`。
- Skill：dingtalk-misc PAT reference；与 devapp 开放平台权限边界明确。
- Schema 来源：同一冻结二进制运行时声明组装；未使用历史或固定 Catalog。

## 可复用分析流程

先识别位置参数与 flags 的链路边界；再按角色、值域、单复数、布尔极性和安全授权逐项审核；
对环境回退与显式 flag 的优先级单独核对；所有写操作只用 dry-run/mock/隔离配置目录；用生成器
捕获真实 flag 冲突，用 PreParse 验证保护先于派发；最后以完整 payload 模板和仓库政策决定
落地状态，Runtime 契约问题单列而不伪装成 alias 规则。
