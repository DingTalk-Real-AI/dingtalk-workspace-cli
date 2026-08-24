# DWS Drive 新增命令参数幻觉增量分析

## 1. 结论摘要

本轮最初以引入新增 Drive Shortcut 的 `main@38e387bcd6fb5806f555865c81764feba43dc6f1`（2026-08-12，PR #959）冻结产品事实；复查时已在最新 `main@e7837cdc6b5e43f74ad5483eea328a6f2d6c5995` 重新生成和验证。两者之间的更新只涉及 Skill 安装升级与 release 流程，没有修改 Drive、Schema 参数或参数归一化代码。分析按照 `specs/product-cli-param-hallucination-analysis-spec.md`，对账真实 Cobra Help、运行时组装 Schema、Drive Skill、Shortcut 实现和当前正式 `internal/cli/param_concepts.json`。未使用历史 badcase、`dws-eval`、历史 Excel、固定 Catalog、用户自定义 Shortcut 或插件。

本次合入后，Drive 在运行时 Schema 中由 **41 个工具增至 63 个**，新增 **22 条 `drive +...` 路径**。这 22 个命令共有 **55 次业务参数、21 个不同 canonical flag 名**；逐命令核对 Help 与同提交 Schema，公开参数名、类型和必填关系差异为 **0**。因此问题不是 Schema 快照过期，而是新增命令没有同步进入按精确路径生效的中央参数兜底表。

本轮正式落地前，生成表对这 22 条新增路径没有独立规则。代表性问题如下：

- `drive +list --folder-id`、`--page-size`、`--next-token` 均报 unknown flag；
- `drive +download --dentry-uuid` 与 `--destination-path` 均不能进入 canonical 参数；
- `drive +recycle-restore --recycle-item-id` 不能归一到真实 `--id`；
- `drive +version-get --version-number` 不能归一到 `--version`；
- 更危险的是，`drive +delete --name`、`drive +version-revert --name` 会被通用拼写纠错当成 `--node`，分别到达确认门和后续执行链路，而不是以错误参数停止。

新增参数问题可聚合为 **7 类**：Drive 节点 ID 命名和值域边界、位置 ID 与源/目标角色、分页与排序、传输与版本工作流、宽泛名称的模糊纠错、类型/聚合开关，以及 Schema/Skill/生成器可见性漂移。

本轮实现基于原正式表增量修改，没有新增产品级 concept，而是：

- 扩展 **9 个既有 concept** 的 Drive 精确命令范围；
- 新增 **20 个精确 Drive command override**；
- 新增 **72 条审核 fixture**，其中 64 条 alias、8 条 guard；
- 隔离生成后覆盖 21 个公开新增命令，共产生 **220 条 alias、170 条 block、10 条 ambiguous**。

数量较大并不表示有 400 个独立问题：分页、空间 ID 和节点 ID 的 concept 成员及 excludes 会按命令展开。第二轮审核以既有等价命令为契约证据，补齐 `node-id` 和已验证的 ID/URL 输入，同时删除与旧命令冲突的过度 block；保留的 170 条 block 主要用于阻止数字 `dentryId → node`、`name → node`、普通文件与 Doc URL 混用、space/workspace 和源/目标角色混用。所有规则都按精确命令路径收敛，不扩散到其他产品。

当前结论是“**第二轮语义校准、正式表替换和完整测试落地均已完成**”。21 个新增公开命令均已补 complete-command 模板，64 条 alias fixture 均验证 alias 与 canonical 到达完全相同的最终 transport payload；8 条 block/ambiguous fixture 继续在 dispatch 前停止。生成器、`internal/cli`、`internal/pipeline`、Drive、完整 `internal/app`、generated drift、Schema policy 以及除未跟踪 `outputs/` 分析脚本外的全部正式 Go package 均通过。

另有一个不能由别名表解决的契约问题：`drive +publish-set` 出现在运行时 Schema 且显示 `availability=available`，但 `semantic_catalog_drive.json` 将它定义为 `public=false、availability=unavailable`，Drive Skill 也不把它列为公开命令。参数 alias 生成器按审核后的可运行叶子拒绝该路径，所以正式规则没有强行加入它。需要先统一 Shortcut 可见性、Schema 发布和 Skill，再决定是否治理它的 `--node/--permission`。

## 2. 新增命令范围

新增 22 条 Schema 路径如下：

```text
drive +cover                 drive +create-folder        drive +create-shortcut
drive +delete                drive +download             drive +inspect
drive +list                  drive +publish-get          drive +publish-set
drive +publish-unset         drive +recycle-list         drive +recycle-restore
drive +rename                drive +star-add             drive +star-list
drive +star-remove           drive +stats                drive +upload
drive +version-download      drive +version-get          drive +version-history
drive +version-revert
```

其中 21 条属于语义目录和 Drive Skill 认可的公开 Shortcut；`drive +publish-set` 是上述可见性漂移的例外。完整参数明细见配套工作簿“参数问题明细”。

## 3. 参数问题

### 3.1 Drive 节点 ID 命名和值域边界

17 个新增命令使用真实 `--node`，其中 16 个属于公开 Shortcut，`+publish-set` 是当前可见性漂移的例外。其值会原样进入底层 `fileId` 或 `nodeId`。第二轮复查进一步确认，不能把这 16 个命令都按同一套“仅 ID、拒绝 URL”的规则处理：应先保证通用节点 ID 名称一致，再按接口和既有等价命令收敛 URL/文档节点边界。

因此：

- 16 个公开 node 命令全部支持 `dentry-uuid/file-id/node-id → node`；
- `dentry-id` 必须 block；
- `+cover/+create-shortcut/+publish-get/+publish-unset/+rename/+star-add/+star-remove/+stats` 与已有 Drive 命令调用相同接口和字段，恢复已有命令已经验证的 `doc-id/document-id/url/document-url/id → node` 范围；
- `+inspect` 明确接收文件、文件夹或文档节点 ID，因此接受 `doc-id/document-id/folder/folder-id → node`，但不扩展 URL；
- `+delete/+rename/+inspect` 操作的单一目标可以是文件夹，`folder/folder-id → node` 没有源/目标角色歧义；
- `+download/+version-*` 仍是普通文件工作流，继续 block Doc URL、folder 和本地输入角色；
- `+upload file-id/node-id` 明确表示覆盖目标 `node`，而无 `-id` 的 `file/file-path` 仍表示本地输入；
- 其余宽泛 `id` 是否接受，以同接口既有命令证据为准；没有证据时保持 block/ambiguous。

这里仍不直接复用 `doc_node_id` concept：不同 Drive Shortcut 对 URL、普通文件、文件夹和文档节点的接受范围并不相同。新增一个包含相同 `node/file-id/dentry-uuid` 成员的 `drive_node_id` 也不可行，因为生成器禁止两个 concept 共享成员。正式实现采用精确 command override，并以同接口旧命令和当前 Shortcut 的真实参数组装共同确定每条命令的边界。

### 3.2 位置标识符和源/目标角色容易互换

`+create-folder`、`+create-shortcut`、`+list` 和 `+upload` 同时涉及父目录、源节点、覆盖节点、数字存储空间或知识库工作区：

- `--folder` 接收父/目标文件夹的 dentryUuid；
- `--space-id` 接收数字 DingDrive 存储空间 ID；
- `--workspace` 只在 `+create-shortcut` 表示目标知识库；
- `+upload --node` 是覆盖目标，不是本地输入文件；
- `+recycle-restore --id` 是 recycleItemId，不是节点 ID。

正式规则只接受角色明确的 `source-file-id`、`target-folder-id`、`target-workspace-id`、`overwrite-node-id`；`target-id`、`destination-id`、`space` 等有多个合理目标的名称保留 ambiguous。`+upload --workspace-id` 被 block，因为新 Shortcut 并没有旧 `drive upload --workspace` 的知识库路由能力。

### 3.3 分页和排序参数没有继承旧命令规则

`+list`、`+recycle-list`、`+star-list`、`+version-history` 使用 `--limit/--cursor`，但旧规则只覆盖 `drive list/recycle list/star list` 等精确路径。`+list` 还使用 `--order-by/--order`。

正式规则扩展既有 `pagination_size`、`page_cursor` 和 `drive_sort_direction`，支持值不变的 `page-size/max-results → limit`、`page-token/next-token → cursor`、`sort-direction → order`；`sort-by/order-field → order-by` 只在 `+list` 用 scoped alias。`page`、`offset` 不会转换成 cursor，防止分页模型和值发生变化。

### 3.4 传输、版本、名称和输出路径角色相近

`+download/+version-download` 的 `--output` 是本地输出路径；`+upload --file` 是本地输入路径，`--file-name` 是远端显示名称，`--node` 是覆盖目标；`+version-get/+version-download/+version-revert` 的 `--version` 是正整数历史版本号；`+rename --name` 是新显示名称。

正式规则仅做可原样传递的改名：

- `destination-path/save-path → output`；
- `source-file/local-file/file-path → file`；
- `display-name/upload-name/name → file-name`，仅限 `+upload`；
- `new-name/display-name/file-name → name`，仅限 `+rename`；
- `version-number/version-no → version`；
- `content-type → mime-type`。

它不会读取文件、转换绝对路径、推断 MIME、换算版本或把输出路径当输入路径。

### 3.5 `name → node` 的通用模糊纠错风险

本轮落地前的正式表没有保护新增的“只有目标 `--node`、没有真实 `--name`”命令。`name` 与 `node` 编辑距离很近，实测：

```text
dws drive +delete --name fixture
  → 被当成 --node fixture
  → 到达高风险确认门

dws drive +create-shortcut --name fixture
  → 被当成 --node fixture
  → 继续进入 API/鉴权链路
```

这不是显式 alias，而是中央 semantic alias 没命中后，通用 ParamName 模糊纠错接管。正式规则在没有真实 `--name` 的新增 node 命令上精确 block `name`，使它在 dispatch 前返回 `blocked_flag`。`+rename` 保留真实 `--name`，`+upload` 则将 `name` 明确限定为远端 `--file-name`，不会一刀切拦截。

### 3.6 类型过滤和聚合开关不能凭相似名称猜测

`+star-list --content-types` 的值是 API contentTypes 列表，宽泛 `--type/--types` 也可能表达文件扩展名、节点类型或资源类型，正式规则标记 ambiguous，不自动转换。

`+inspect` 只支持 `--include-stats/--include-publish/--include-cover`。正式规则允许语义明确的 `include-statistics`、`include-public-status`、`include-thumbnail`，但 block `--include` 以及 Doc `+inspect` 的 `include-history/include-permissions/include-content`。当前链路只能改一个参数名，不能根据 `--include history,stats` 拆成多个布尔 flag。

### 3.7 Schema、Skill 和 alias 生成器的可见性漂移

`drive +publish-set` 的三个事实互相冲突：

- 运行时 Schema：路径存在，`availability=available`，发布 `--node/--permission`；
- 语义目录：`public=false`、`availability=unavailable`，原因是服务端对已验证样本返回不支持；
- Drive Skill：公开 Shortcut 清单不包含它。

在隔离候选中添加该 command override 时，生成器报：

```text
command_override "drive +publish-set" does not match any runnable Cobra leaf
```

所以它当前不能用 JSON 稳定治理。应先决定它是公开可用命令还是隐藏诊断命令，并统一声明、Schema 和 Skill；之后才能加入参数别名与最终 payload 测试。其余 21 个公开命令不受影响，均已正式落地。

同时，既有 `drive +find-file` 仍把 `dentryId/dentryUuid/fileId/nodeId/id` 多种返回候选统一投影到字段名 `dentryId`。这是输出标识符命名问题，不能由入参 alias 解决，应单独修改结果投影和 Schema result 契约。本轮未把它伪装成新增命令的 alias 问题。

## 4. 已实施的正式别名表方案

审核草稿位于同目录 `param_concepts.json`；其内容现已同步到正式 `internal/cli/param_concepts.json`，并通过生成器生成 `param_aliases_generated.go`。

本轮已经落地：

1. 为 16 个公开 node 命令完整配置 `dentry-uuid/file-id/node-id → node`，并按同接口旧命令决定是否接受文档 ID/URL；
2. 扩展 9 个既有 concept：`pagination_size`、`page_cursor`、`folder_id`、`drive_storage_space_id`、`doc_version_number`、`drive_recycle_item_id`、`drive_sort_direction`、`workspace_id`、`local_output_path`；
3. 为 `+create-folder/+create-shortcut/+list/+upload` 配置角色明确的命令级 alias，并保护 space/workspace、source/destination、folder/node；
4. 为 `+inspect`、`+rename`、`+upload` 配置只在该命令成立的 section/name/path alias；
5. 对 `name → node`、`dentryId → node`、普通文件工作流中的 Doc URL、普通 node → recycleItemId 配置 dispatch 前保护；
6. `+star-list --type/--types` 保持 ambiguous；
7. 暂不为 `+publish-set` 增加规则，先修复可见性契约。

## 5. 当前能力支持不了或不应该做的事项

- 把数字 `dentryId`、知识库 workspace、数字 storage space、Drive node 和 recycleItemId 相互查询或转换；允许 URL 的命令只做已验证的名称归一，不做 URL→ID 转换；
- 根据 `--id/--space/--target-id/--destination-id` 自动选择多个合理目标；
- 把 page/offset 换算成 cursor，或生成下一页 token；
- 把 `--include` 的值拆为多个布尔 flag；
- 把宽泛 `--types` 的值翻译成 contentTypes 枚举；
- 读取本地文件、转换路径、推断 MIME 或修改参数值；
- 修复 `+find-file` 的输出字段混名；
- 在 `+publish-set` 可见性契约统一前，为生成器不可接受的隐藏路径强行添加 override。

上述事项不阻塞其余 21 个公开新增命令的第一轮治理，但必须保持 block、ambiguous 或明确待修，不应为了覆盖率配置猜测性 alias。

## 6. 正式落地审核结论

相对本轮修改前的正式 `internal/cli/param_concepts.json`：

- 新增 concept：0；
- 修改既有 concept：9，仅增加新增 Drive 命令范围；其中 `doc_version_number.denotes` 文案扩为“Doc 或 Drive 普通文件历史版本”，members/excludes 不变；
- 新增 command override：20，全部为本轮 Drive `+` 命令；
- 新增 validation fixture：72，其中 64 条 alias、8 条 block/ambiguous guard，全部可追溯到本文问题；
- 非 Drive command override、concept 命令范围和 fixture 改动：0；
- `drive +publish-set` 经审核后从草稿移除，原因是生成器与 Schema 可见性冲突；
- 曾尝试新增 `drive_node_id`，因与 `doc_node_id` 共享成员会被生成器拒绝，审核后移除，改为精确 command override；
- 自动 alias 均满足同一实体、角色、值域、单位和 cardinality，值原样传递；新增的 URL 别名只覆盖同接口旧命令已接受 URL 的精确路径，不声称进行 URL 解析或值转换；不满足条件的名称均为 block/ambiguous/暂不支持。

正式生成结果覆盖 21 个公开新增命令，展开为 220 alias、170 block、10 ambiguous。规则规模主要由通用 concept 成员、excludes 和精确 command override 展开，未扩大到插件、用户 Shortcut 或其他产品。

## 7. 正式验证结果

正式表替换并重新生成后，验证结果如下：

- `jq` 解析与 JSON Schema/生成器读取：通过；
- `go generate ./internal/cli`：通过，两次生成结果确定；
- `go test ./internal/cli ./internal/pipeline ./internal/generator/cmd_param_aliases`：通过；
- 全量 reviewed guard 到真实 runtime contract：通过；
- fixture 经过最终嵌入交付路径：通过；
- `check-generated-drift.sh`：通过；
- `check-schema-catalog.sh`：通过；
- 21 个新增公开命令 complete-command E2E 模板：全部存在并满足真实 required/constraint；
- 64 条 Drive alias/canonical 最终 transport payload 等价测试：全部通过；
- 8 条 Drive block/ambiguous guard：全部通过并在 dispatch 前停止；
- 完整 `internal/app`：通过；
- 除未跟踪 `outputs/` 分析脚本外的全部正式 Go package：通过；
- 代表性 alias：`+list`、`+download`、`+recycle-restore`、`+version-get` 以及第二轮补充的 `+cover --node-id/--url`、`+create-shortcut --document-id`、`+delete/+inspect/+rename --folder-id`、`+star-add --url`、`+stats --document-url`、`+upload --file-id` 均不再报 unknown flag，进入 canonical 对应的 mock、确认或本地校验链路；
- 代表性保护：`+create-shortcut --name`、`+delete --name` 返回 `blocked_flag`，`+star-list --types` 返回 `ambiguous_flag`，`+upload --workspace-id` 返回 `blocked_flag`，均在 dispatch 前停止；
- 未发起真实业务写操作：最终 payload 测试统一注入 capture caller/runner；下载使用不受信任 URL 的稳定校验边界，上传停在受控凭证响应校验边界，alias 与 canonical 的调用序列和错误均一致。

本轮没有剩余的参数别名落地门禁。仍待单独处理的是 `+publish-set` 可见性契约和 `+find-file` 输出字段命名，它们不属于 `param_concepts.json` 能解决的入参别名问题。

## 8. 后续事项

1. 先修复 `drive +publish-set` 的 Hidden/availability/Schema/Skill 一致性，明确它是否进入公开产品面；
2. 后续修改这些 Drive 参数规则时，保持 21 个 complete-command 模板和 64 条 payload 等价测试同步更新；
3. 把 `name → node` 和 `dentryId → node` 作为 P0 保护，不只增加收益 alias；
4. 对 `+create-shortcut`、`+upload`、`+recycle-restore`、`+version-revert` 等写命令使用注入 Runner 或确认门验证，禁止真实写调用；
5. 单独修复 `+find-file` 输出把多种 ID 命名为 `dentryId` 的契约问题，不放入本次入参 alias 改造；
6. 后续若扩展 `content-types`、公开权限 enum 或普通文件版本值域，应由 Drive owner 重新复核。

## 9. 可复用流程

后续继续使用：冻结最新 main 并从同提交构建 → 比较新增前后官方 Schema 路径 → 对账每条新增命令的 Help、完整 Schema、Skill 和实现 → 按实体/值域/角色/cardinality 聚合问题 → 从正式表生成完整候选 → 独立生成审计并移除冲突 concept/隐藏路径 → 验证 alias/canonical 最终 payload、block/ambiguous 和非目标回归 → 测试模板全绿后再替换正式表。
