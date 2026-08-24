# DWS Drive 新增 Shortcut 参数幻觉复查报告

## 1. 结论摘要

本轮按照 `specs/product-cli-param-hallucination-analysis-spec.md`，在 `fix/param-hallucination@0dc6735da2cbdd511272fcbe2282d0e28c54baeb` 重新构建 `dws`，复查 PR #959 新增或转为公开的 Drive Shortcut。当前远端 `main@76d54d6df6a5fffef91a5af68492f4620961616e` 相比分析提交只增加 `CHANGELOG.md`，Drive 命令、Schema 和参数别名代码没有差异，因此本轮参数结论同样适用于该最新 main。

本轮范围包含 **21 条公开新增 Shortcut**，共有 **53 次业务参数出现、20 个不同 canonical flag**。另有 `drive +publish-set` 可通过真实 Help 和运行时 Schema 查询，但不进入 `dws shortcut list --service drive` 的 28 条公开清单，也没有进入 Drive Skill 的公开路由；将它计入技术命令面后，范围为 **22 条命令、55 次参数出现、21 个不同 flag**。

核心结论：

- 21 条公开新增 Shortcut 的 Help、运行时 Schema 和 Shortcut 声明参数一致，参数名、类型和必填关系差异为 **0**；
- 当前正式 `internal/cli/param_concepts.json` 已经覆盖全部 21 条公开命令：有效结果为 **220 条 alias、170 条 block、10 条 ambiguous**；
- 当前有 **72 条审核 fixture** 覆盖这 21 条命令，其中 64 条验证 alias，8 条验证 block/ambiguous；每条公开命令至少有一条 fixture；
- 现有完整候选 `docs/parameter-hallucination/drive/param_concepts.json` 已刷新为当前正式表的完整副本，结构化 diff 为 **0**。本轮没有发现需要再次扩大 alias 的安全映射；
- 唯一新增风险是 `drive +publish-set` 的可见性契约漂移。它仍不能进入参数 alias 生成器的可治理叶子集合，向候选表增加 override 会失败；同时当前 `--name` 会被通用模糊纠错成 `--node` 并进入高风险 Shortcut 执行链路。该问题需要先统一 Cobra/Schema、Shortcut Catalog、Skill 和生成器边界，不能由现有 JSON 单独修复。

本轮未使用历史 badcase、`dws-eval`、`merged_scan.json`、历史固定 Catalog、用户自定义 Shortcut 或插件。

## 2. 分析范围

21 条公开新增 Shortcut：

```text
drive +list                 drive +inspect              drive +download
drive +upload               drive +create-folder        drive +create-shortcut
drive +rename               drive +delete               drive +stats
drive +cover                drive +recycle-list         drive +recycle-restore
drive +star-list            drive +star-add             drive +star-remove
drive +publish-get          drive +publish-unset        drive +version-history
drive +version-get          drive +version-download     drive +version-revert
```

单独审核但不计入公开范围：

```text
drive +publish-set
```

## 3. 参数问题与现有兜底结论

### 3.1 节点 ID 命名、URL 和值域边界

16 条公开命令使用 `--node`，但 Agent 容易生成 `--file-id`、`--node-id`、`--dentry-uuid`、`--document-id`、`--url`、`--folder-id` 或数字 `--dentry-id`。

这些名称不能一刀切：

- 所有 16 条命令都可安全接受值不变的 `dentry-uuid/file-id/node-id → node`；
- `+cover/+create-shortcut/+publish-get/+publish-unset/+rename/+star-add/+star-remove/+stats` 按同接口既有命令证据接受文档 ID/URL；
- `+inspect` 接受文件、文件夹或文档节点 ID，但不声明 URL 输入；
- `+delete/+rename/+inspect` 的单一目标可以是文件夹，因此 `folder/folder-id → node` 角色明确；
- `+download/+version-*` 是普通文件工作流，文档 URL、文件夹和本地路径必须拦截；
- 数字 `dentryId` 不能只改名变成 dentryUuid/fileId，必须 block。

当前正式表已通过精确 command override 实现上述边界，并对 14 条没有真实名称参数的 node 命令保护 `name → node` 模糊纠错。`+rename` 保留真实 `--name`，`+upload` 则把 `name` 精确归入远端 `--file-name`，没有误拦截。

### 3.2 存储空间、知识库、父目录和源/目标角色

`+list/+create-folder/+create-shortcut/+upload/+recycle-restore` 同时出现 `space-id`、`workspace`、`folder`、`node` 或宽泛 `id`：

- `--space-id` 是数字 DingDrive 存储空间；
- `--workspace` 是知识库/文档空间；
- `--folder` 是父目录或目标目录；
- `+upload --node` 是覆盖目标，不是本地输入文件；
- `+recycle-restore --id` 是 recycleItemId，不是普通节点 ID。

现有表使用 concept、bind、scoped alias、block 和 ambiguous 分离这些角色。`target-folder-id`、`target-workspace-id`、`overwrite-node-id` 等角色明确的名称可归一；`target-id`、`destination-id`、`space` 等存在多个目标的名称继续提示歧义。

### 3.3 分页与排序命名不一致

`+list/+recycle-list/+star-list/+version-history` 使用 `limit/cursor`，`+list` 还同时使用 `order-by/order`。模型容易沿用 `page-size/max-results/page-token/next-token/sort-by/sort-direction`。

现有 `pagination_size`、`page_cursor`、`drive_sort_direction` 和 `+list` 精确 alias 已覆盖值不变的名称；`page/offset` 仍被拦截，因为当前链路不能把页码或偏移换算成 cursor。

### 3.4 本地文件、输出路径、远端名称和版本号角色相近

`+download/+upload/+rename/+version-get/+version-download/+version-revert` 同时出现：

- 本地输入 `file`；
- 本地输出 `output`；
- 远端显示名称 `file-name/name`；
- MIME `mime-type`；
- 覆盖目标 `node`；
- 正整数历史版本 `version`。

当前表只做值可原样传递的改名，例如 `destination-path/save-path → output`、`source-file/file-path → file`、`content-type → mime-type`、`version-number/version-no → version`。它不会读取文件、转换绝对路径、推断 MIME、换算版本或把输入路径和输出路径互换。

### 3.5 聚合开关和类型过滤不能使用宽泛名称

`+inspect` 只支持 `include-stats/include-publish/include-cover`；现有规则允许 `include-statistics/include-public-status/include-thumbnail`，但拦截需要值拆分的泛化 `--include` 以及 Doc `+inspect` 的其他 section 名称。

`+star-list --content-types` 的值域可能与文件扩展名、节点类型或资源类型混淆，因此 `--type/--types` 保持 ambiguous。`+list --thumbnail` 在稳定命令和 Shortcut 中命名一致，无需新增 alias。

### 3.6 `drive +publish-set` 可见性和治理边界不一致

当前事实同时存在：

1. `dws drive +publish-set --help` 可找到真实叶子；
2. `dws schema --cli-path "drive +publish-set"` 发布 `availability=available`、`--node/--permission` 和高风险确认；
3. `dws shortcut list --service drive` 不公开它，Drive Skill 也明确不推荐；
4. 参数 alias 生成器拒绝该路径：`command_override "drive +publish-set" does not match any runnable Cobra leaf`；
5. 当前 `--name fixture` 不会报 unknown flag，而是被模糊纠错成 `--node fixture` 并进入 Shortcut 执行链路；仍有 `--yes`/确认和后端校验，但参数名保护缺失。

因此本轮没有把 `+publish-set` override 强行留在候选表。安全顺序应是：先明确它是否公开可用；若不公开，应同步从可执行/Schema 面隐藏；若公开，应先让生成器和 Catalog 边界一致，再增加 `name/dentry-id` 保护、审核后的 node/permission alias 和最终 payload/确认回归。

## 4. 当前正式别名表覆盖情况

对 21 条公开新增 Shortcut，当前正式表有效使用：

- 15 个已有 concept；
- 20 个精确 command override，`+recycle-list` 仅依靠已有 concept 即可完成；
- 220 条生成 alias；
- 170 条生成 block；
- 10 条生成 ambiguous；
- 72 条审核 fixture：64 条 alias、8 条 guard；
- 21 条 complete-command 模板和最终 transport payload/确认保护测试。

这些数字是 concept 成员、excludes 和 command override 按真实 flag 交集后的展开结果，不代表 400 个独立问题。规则均收敛到审核过的精确路径，没有扩散到其他产品、用户 Shortcut 或插件。

## 5. 当前能力支持不了或不应该做的事项

- 把数字 dentryId、dentryUuid/fileId、知识库 workspace、数字 storage space 和 recycleItemId 相互查询或转换；
- 根据 `--target-id/--destination-id/--space` 自动选择多个合理 canonical 目标；
- 把 page/offset 换算成 cursor，或生成下一页 token；
- 把一个 `--include` 值拆成多个布尔 flag；
- 翻译 `--types` 的枚举值或在不同类型值域之间转换；
- 读取本地文件、转换路径、推断 MIME 或修改参数值；
- 在 `+publish-set` 被生成器排除时，仅靠 JSON 为它增加 block/ambiguous。

这些限制不阻塞 21 条公开新增 Shortcut；`+publish-set` 需要单独修复命令可见性契约。

## 6. 第一轮改造建议

1. **公开 21 条 Shortcut 不再修改正式别名表**：现有规则覆盖完整，继续扩大 alias 反而会增加值域误判；
2. **修复 `+publish-set` 可见性契约**：由 Drive owner 决定隐藏还是公开，并统一 Cobra、Schema、semantic catalog、Skill 和 alias 生成器；
3. **保留现有安全保护**：`name → node`、`dentryId → node`、space/workspace、source/destination 和 recycleItem/node 边界不可收缩；
4. **后续变更必须同步测试**：维持 72 条 fixture、21 条完整模板、64 条 alias/canonical payload 等价和高风险确认保护。

## 7. 候选别名表审核结论

`docs/parameter-hallucination/drive/param_concepts.json` 已从当前正式 `internal/cli/param_concepts.json` 重新生成完整副本。

结构化审核结果：

- 新增 concept：0；
- 修改 concept：0；
- 新增 command override：0；
- 修改 command override：0；
- fixture 差异：0；
- 非 Drive 规则差异：0；
- 候选与正式文件字节一致。

曾在隔离 worktree 尝试给 `+publish-set` 增加“只保护、不增加 alias”的 override，但生成器拒绝该隐藏路径，因此审核后移除并转入当前不支持清单。候选表当前状态是“**已审核、无需替换正式表**”。

## 8. 验证结果

在独立临时 worktree 中把候选文件作为正式输入后：

- `jq empty`：通过；
- `go generate ./internal/cli`：通过，生成 340 条命令级 alias entry；
- 生成结果与当前提交一致：通过；
- `go test ./internal/cli ./internal/pipeline ./internal/generator/cmd_param_aliases ./internal/app -count=1`：通过；
- 21 条 Drive Shortcut payload 等价、确认保护和最终嵌入 fixture 定向测试：通过；
- `check-generated-drift.sh`：通过；
- `check-schema-catalog.sh`：通过，27 个产品、1098 个工具；
- 未发起真实业务写调用：写操作测试使用注入调用器/Runner 或确认保护。

## 9. 可复用流程

冻结当前提交并重新构建 → 以 Shortcut Catalog 确认公开范围 → 用 Help/完整 Schema/Skill/实现对账 flags → 按实体、角色、值域和 cardinality 聚合问题 → 对照正式别名表的有效生成结果 → 只为安全且值可原样传递的缺口生成候选 → 独立 worktree 验证生成、payload、保护和非目标回归 → 可见性或值转换问题转入当前不支持，不强行写入 JSON。
