# Markdown 产品 CLI 参数幻觉分析

## 结论摘要

本分析以线上 `origin/main` 冻结提交
`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df` 为唯一基线，使用该提交重新构建的
`dws`、运行时 Schema、Cobra Help、Markdown 命令实现、dingtalk-misc 的 Markdown
reference 和冻结正式 `internal/cli/param_concepts.json`。未使用固定 Catalog、历史
badcase、用户 Shortcut 或已安装插件，也没有修改当前工作区的正式别名表。

Markdown 产品有 5 个 Agent 可见且可执行叶：`fetch`、`create`、`diff`、`overwrite`、
`patch`。冻结正式别名表对 Markdown 没有 concept 命令范围、command override 或
fixture。主要风险不是 Markdown 语法，而是把五组角色混成泛化参数：远端 node、文档
workspace、数字钉盘 space、本地输入/输出路径、正文/文件名，以及 diff 的左右版本和
patch 的匹配式/替换值。破坏性 `overwrite` 和需确认的 `patch` 还要求保持预览与确认
边界，不能让 `id/path/text/space` 静默落到一个目标。

候选已通过真实生成器、PreParse、5 组 alias/canonical dry-run 逐字节比较、11 组
block/ambiguous、非目标结构恒等、`internal/cli`、`internal/pipeline`、generated drift
和 Schema Catalog 政策。完整 `internal/app` 运行 241.398 秒后未通过；聚焦门禁证明
产品相关缺口是 5 个 Markdown 命令没有 complete-command E2E 模板，24 条 active alias
fixture 因此未进入最终 payload 等价验证。正式状态为“规则与链路已验证，补齐 5 个模板
后方可落地”。

## 参数问题

### 1. 远端 node、workspace 与钉盘 space 是三个不同值域

- `--node` 是一个远端原生 `.md` 文件节点；
- `--workspace` 是文档空间/知识库 ID 或 URL；
- `--space-id` 是数字钉盘存储空间 ID；
- `create --folder` 是对应域下的父文件夹 ID。

`fetch/overwrite/patch` 以 node 为操作目标，`create` 则创建新节点且没有 node 参数。
候选只把 `node-id/doc-id/file-id/document-id/dentry-uuid` 在精确叶上归到 node；
`workspace-id/knowledge-base-id` 归到 workspace，`drive/storage-space-id` 归到
space-id。泛化 `id/space/path` 标为 ambiguous；在 create 上出现 node，在 fetch 上出现
folder 都会 block。任何 ID/URL 转换或自动探测仍由 Runtime 完成，别名层不改值。

### 2. `--content`、`--file`、`--name` 与 `--output` 的传输角色不能互换

`create/overwrite --content` 接受字面值、`@file` 或 `-`，`--file` 是本地 `.md` 路径，
两者必须二选一。`--name` 是远端展示文件名；`fetch --output` 是本地保存文件或已有目录。
同一个“文件”词可能因此表示内容来源、远端名称、目标 node 或本地输出。

候选扩展 `content_text`，新增本地 Markdown 文件路径和 Markdown 文件名 concept，并把
`body/text/markdown-content`、`file-path/local-file`、`filename/file-name` 分别限制到
真实角色。`content-file`、泛化 `path/destination` 需要消歧；不会把本地路径包装成
`@file`，不会自动读取文件，也不会猜测 create 应使用 content 还是 file。

### 3. `diff` 的 node、本地 file、version/version2 与 context 组成模式契约

`markdown diff` 有两种模式：远端版本对远端版本，或远端版本对本地 `.md`。`--version`
是左侧正整数版本号，`--version2` 是右侧正整数；`--file` 是本地右侧并与 version2
互斥；`--context` 是非负上下文行数。至少要有一个历史版本或本地文件。

候选把 `version-no/version-number` 归到 version，使用命令级
`from/left/base-version → version`、`to/right/target-version → version2`，新增 diff context
concept。`version-id/revision` block，泛化 `left/right/path` ambiguous。候选只改 flag 名，
不决定模式、不交换左右侧、不把 revision ID 转成版本号，也不绕过 10 MB/超时/正整数
Runtime 校验。

### 4. `patch` 的 pattern、regex 与 replacement 是三个独立角色

`--pattern` 是字面匹配式或 RE2 表达式；`--regex` 只切换 pattern 解释方式；`--content`
是始终按字面量处理的替换值，`$1/$2` 不展开。0 命中不写入，替换为空会中止。

候选新增 pattern 与 regex concept，允许 `find-text/old-text/search-pattern → pattern`、
`regexp/use-regex → regex`，并以命令级别名把 `replacement/replace-with/new-text → content`。
泛化 `text/body/value` 无法判断是旧文本还是新文本，统一 ambiguous；`file/replacement-file`
block，避免引入候选表不具备的文件读取和内容包装能力。

### 5. 全局 dry-run 与命令级 dry-run 的行为不同

根命令全局 `--dry-run` 只做无网络计划预览；`overwrite/patch` 的叶级 `--dry-run` 会读取
远端当前内容并输出真实差异。二者同名但生命周期和网络行为不同，且叶 flag 会 shadow
根 persistent flag。

候选仅在 overwrite/patch 内把 `show-diff/preview-only/no-write` 映射到叶级 dry-run，
不把它推广到其他产品或声明为“完全离线”。代表 alias/canonical 的逐字节比较使用根级
dry-run，保证没有业务写；叶级预览映射由生成器/PreParse fixture 验证，真实执行仍要在
明确 node 和可接受远端读取的前提下进行。

### 6. Skill 与真实命令面存在 `diff` 漏项

dingtalk-misc 的 Markdown reference 只列 `fetch/create/overwrite/patch` 四个命令，冻结
Cobra/Schema 还正式发布 `markdown diff`。同时 Help/Schema 隐藏了
`RegisterCrossProductAliases` 注册的 node、folder、workspace、file、content 兼容 flag；
根框架还存在通用 output sink，生成器因此禁止候选 block 某些真实 flag。

这些是真实命令声明、Help/Schema 与 Skill 的公开契约差异，不应伪装成中央别名能力。
候选保持原生 hidden flag 行为，不重复重写 `node-id/doc-id/file-id/parent-id/file-path` 等
已注册兼容面；正式落地应同步补 Skill 的 diff 章节，并单独评审 hidden compatibility
是否应进入正式 Help/Schema。

## 当前别名表可以实施的方案

1. 扩展 `workspace_id`、`drive_storage_space_id`、`local_output_path`、`folder_id`、
   `content_text`、`doc_version_number` 到精确 Markdown 叶。
2. 新增本地 Markdown 文件路径、Markdown 文件名、diff 上下文行数、patch pattern、
   patch regex 五个专用 concept。
3. 为 5 个叶声明 node、内容传输、路由、左右版本、匹配/替换、预览语义的 scoped alias、
   block 和 ambiguous，并启用 `scope_strict`。
4. 保持所有 alias 值和类型原样传递；不读取文件、不包装 `@file`、不转换 ID/URL、
   不交换 diff 左右侧、不改正则或替换文本。
5. 保持已存在 hidden compatibility flag 的原生行为；补齐全部 5 个 complete-command
   payload 模板后再评审正式替换。

## 当前能力支持不了的事项

- 把 URL、nodeId、workspaceId、数字 spaceId 和 folderId 自动互转；
- 从泛化 `--id/--space/--path` 推断远端目标、路由域或本地输入/输出角色；
- 自动选择 `--content` 与 `--file`，或把普通路径包装成 `@file`/stdin；
- 自动补 `.md` 后缀、读取文件内容、判断路径是文件还是目录；
- 把 revision ID、标签或时间点转换成正整数历史版本；
- 自动决定 diff 模式、左右版本顺序或 file/version2 互斥关系；
- 猜测 patch 的 `text` 是 pattern 还是 replacement，或把捕获组扩展语义写进替换值；
- 把全局无网络 dry-run 与叶级远端差异预览视为同一能力；
- 用别名表补 Skill 漏掉的 `diff`，或消除 hidden flag 的 Help/Schema 漂移；
- 在没有 complete-command 模板时直接替换正式表。

这些情况应停止并提示真实参数和产品边界，不得为了继续写文件而猜测目标或内容。

## 第一轮改造建议

第一轮建议落地远端 node、workspace/space、内容传输、文件名、diff 左右版本/context、
patch pattern/replacement/regex 和叶级预览的低风险别名与保护。落地 PR 必须同步为
`markdown fetch`、`markdown create`、`markdown diff`、`markdown overwrite`、
`markdown patch` 补 complete-command E2E 模板，覆盖 24 条 active fixture；另行补齐 Skill
中的 diff Usage/Flags/Examples，并审查通用 hidden alias 与 Help/Schema 的同源关系。

## 候选 `param_concepts.json` 改动与审核

候选文件是冻结提交正式表的完整副本，不是增量片段。相对冻结正式文件：

- 修改 6 个既有 concept 的精确 Markdown 命令范围；
- 新增 5 个 Markdown 专用 concept；
- 新增 5 个 Markdown command override；
- 新增 35 个审核 fixture，其中 24 个是 active alias fixture、11 个是保护 fixture；
- `go generate ./internal/cli` 从 569 个命令作用域变为 574 个；
- 还原 Markdown 改动后，非目标 concept、override、fixture 与正式表结构完全相同；
- 生成 Go 差异只新增 5 个 Markdown 条目，command path fallback 无变化；
- 5 组代表 alias/canonical 命令退出码与 stdout/stderr 逐字节相同；
- 11 组错误输入稳定返回 `blocked_flag` 或 `ambiguous_flag`；
- 生成器审核后移除了对真实 hidden flag 的重复 rewrite/block，保留原生兼容路径。

候选位置：`docs/parameter-hallucination/markdown/param_concepts.json`。

## 验证结果与正式替换前置条件

| 验证项 | 结果 | 说明 |
|---|---|---|
| JSON 解析与真实生成器 | 通过 | `go generate ./internal/cli`，574 个命令作用域 |
| PreParse 与 alias/canonical 输出 | 通过 | fetch、create、diff、overwrite、patch 五组逐字节一致 |
| block/ambiguous | 通过 | 11 组值域、路径、版本、内容角色错误均在业务派发前停止 |
| 原生参数 | 通过 | canonical flags 不变；hidden compatibility 不重复重写 |
| 非目标回归 | 通过 | 非目标 JSON 结构恒等；生成 diff 仅 5 个 Markdown 条目；fallback 无变化 |
| `internal/cli`、`internal/pipeline` | 通过 | 隔离冻结副本执行，CLI 80.201 秒 |
| generated drift | 通过 | 双次 alias 与 Schema 装配 hash 一致 |
| Schema Catalog 政策 | 通过 | 28 产品、1166 工具；Runtime confirmation truth 通过 |
| 完整 `internal/app` | 未通过 | 241.398 秒；聚焦门禁定位为 complete-command 模板缺口 |
| complete-command payload 门禁 | 未通过 | 200/205 个活跃命令已有模板；Markdown 缺 5 个命令、24 个 active fixture 模板；403 active cases |

正式替换前必须补齐 5 个模板、补 Skill 的 diff 章节并重跑完整 `internal/app` 和政策门禁；
未完成前，本候选只作为完整待审核草稿。

## 分析依据

- 冻结提交：`aa4ae9a90323aa97e5cebdb5045b129f0f14e0df`，提交时间
  2026-08-20 10:53:27 +08:00。
- 正式表 SHA-256：
  `e41e7908c26dfdcc23636d27661d705416c001d67a6d0d5d658b1ca4bcc815c1`。
- 候选 SHA-256：
  `7df963c21af2d92c24b2a65e9110f990d97fd575eeded01962f31a3fc0458c6f`。
- 命令实现：`internal/helpers/markdown.go`、`internal/helpers/markdown_diff.go`、
  `internal/helpers/cross_product_aliases.go`。
- Skill：dingtalk-misc 根 Skill 与 `references/markdown.md`；该 reference 漏列 diff。
- Schema 来源：同一冻结二进制运行时声明组装；未使用历史或固定 Schema Catalog。
