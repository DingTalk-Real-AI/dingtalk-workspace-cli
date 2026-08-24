# DWS Minutes（听记）CLI 参数幻觉分析与兜底修复（2026-08-18）

## 1. 结论

本轮以任务创建时最新的 `origin/main` 提交 `c15480c452ccd607e0694a5798ce7ed37d62539b` 为冻结基线，重新盘点真实 Cobra Help、运行时组装 Schema、全部仓库内置 Minutes Shortcut（含 `public=false`）、Schema exclusions、Minutes Skill 和正式 `internal/cli/param_concepts.json`。历史 2026-08-10 产物仅用于变化对照，没有作为当前事实。

当前 Minutes 有 **62 个运行时 Schema 命令**：34 个原子命令、28 个内置 Shortcut；28 个 Shortcut 中 27 个 `public=true`，唯一 `public=false` 的是 `minutes +minutes-search`。62/62 叶子的可见业务参数在 Cobra Help 与运行时 Schema 中一致，差异为 0。

复核后的判断是：原候选方向基本正确，但还不够完整。本轮已补齐可直接安全实现的缺口：

- 增补单双值、会话、模板、标签、用户目标和异步任务角色的精确 block，防止错误参数被隐藏兼容层或近似匹配吞入。
- 为 `minutes +transcript` 声明 `--id` 与 `--keyword` 互斥；`--query` 仍可在无 `--id` 路由归一化为 `--keyword`。
- 修复原子命令隐藏 `--url`：只解析受信任的 `https://shanji.dingtalk.com` 链接，从 `taskUuid`、`minutesId` 或 `/transcribes/<taskUuid>` 提取单个 ID；冲突、多值、错误域名、错误端口、额外路径和 URL 误传 `--id` 均在 MCP 调用前以 validation/exit 3 拒绝。
- 新增候选专属 complete-command 模板和最终 payload 断言。正式表仍为 170/170；隔离安装候选后为 **190/190 个 active commands、321 个 active cases**，不再有模板缺口。

正式 `internal/cli/param_concepts.json` 未修改。交付候选是其完整副本加 Minutes 合理增量，隔离生成结果为 **59 个 Minutes command rules、274 aliases、476 blocks、3 ambiguous**。

仍不能靠 name-only alias 安全解决的事项有：`public=false` Shortcut 的框架治理、`permission ↔ policy` 值转换、身份体系转换、文件路径到原子上传元数据的 IO/单位换算，以及 Skill/reference 文档漂移。这些场景当前采用明确阻断或边界说明，不做静默猜测。

## 2. 基线、构建与事实边界

| 项目 | 事实 |
|---|---|
| 冻结基线 | `c15480c452ccd607e0694a5798ce7ed37d62539b` |
| 基线主题 | `Merge pull request #1039 from pengzhihan47-star/codex/pr1035-drive-tree-orphan-fix` |
| 基线构建顺序 | `git fetch origin` 后记录 HEAD/origin/main，再执行 `make build` |
| 基线二进制 | `./dws`，29,649,634 bytes，SHA-256 `d8da0b65aae24ff2f1c3b9db2cb41fe7775cf9977991cd4991dea85c28faa0a7` |
| 当前 origin/main | 验证期间已前进到 `c198d8577d938919629bfdb3a76de357b38a4709`；远端 diff 已复核，无 Minutes 实现或 Skill surface 改动，故不改变已冻结事实集 |
| 候选验证方式 | 在 `/private/tmp` 隔离副本中以本轮源码与候选表重新 `go generate`、build、test；正式输入不被替换 |
| Schema 来源 | `runtime-assembled`；不读取历史固定 Catalog |
| 当前全仓规模 | 27 个产品、1,152 个工具 |

纳入的正式事实源：

- 当前命令树逐叶 `--help`，包括真实 flag 与仓库声明的隐藏兼容 flag。
- `dws schema --all -f json` 的运行时组装结果和每个 Minutes 完整叶子。
- 仓库全部内置 Minutes Shortcut，包括 `public=false` 的 `+minutes-search`。
- `internal/cli/schema_command_exclusions.go`；当前没有 Minutes exclusion。
- `skills/multi/dingtalk-minutes/SKILL.md`、`references/minutes.md` 和 mono 副本。
- 正式 `internal/cli/param_concepts.json` 与生成器的真实 runnable command 校验。

明确排除实验 badcase、历史固定 Catalog、自定义 Shortcut 和插件。完整 62 命令及逐命令参数明细放在五页 XLSX 的“参数问题明细”页，本 Markdown 不重复整张命令表。

## 3. 参数幻觉问题与本轮修复

### 3.1 taskUuid、单复数和 URL

同一听记实体跨命令出现 `id`、`task-uuid`、`uuid`、`ids`、`task-uuids` 和 `uuids`；`task-id`、`resume-id` 又分别表示异步任务和恢复句柄，不能与 taskUuid 列表混用。

候选继续用 `minutes_task_uuid` 和 `minutes_task_uuids` 表达单值/批量实体，但不把宽泛 `id` 放入全局 concept；真实 `--id` 由命令级 `bind` 声明。新增的精确保护包括：

- `permission add/remove` 阻断单值 `id/task-uuid/uuid`，并把真实 `ids` 绑定到批量 concept。
- 单听记查询、转写、待办、音频和思维导图命令阻断 `ids/task-uuids/uuids`。
- speaker replace、tag query、speaker insights、upload-and-analyze 分别阻断 `target-uids`、`tag-ids`、`task-ids`、`resume-ids`。
- 所有单 session、单 template 入口阻断对应复数形式。

原子命令的隐藏 `--url` 不再是“改名后原样透传”。九个原子叶统一走严格解析：

1. 只接受 HTTPS `shanji.dingtalk.com`（显式 443 可接受，其他端口拒绝）。
2. 只从 `taskUuid`、`minutesId` 或以 `/transcribes/<taskUuid>` 结尾的路径取值。
3. 多来源值必须一致；冲突、列表、空值、额外路径、URL 误传 `--id` 或同时提供多个 ID flag 均拒绝。
4. 合法值在 dry-run payload 中只出现提取后的 `taskUuid`，非法值不进入 MCP Runner。

### 3.2 `+transcript` 选择语义

`minutes +transcript` 有两个互斥路由：已知 `--id` 时读取指定听记；没有 ID 时用 `--keyword` 先选最新听记。候选允许 `--query → --keyword`，但原实现没有声明 `id/keyword` 互斥，导致 Agent 同时给 ID 和搜索词时语义不明确。

本轮已在 Shortcut Contract 中加入互斥约束。实测：

- `minutes +transcript --query weekly` 仍归一化为 `--keyword weekly`，代表性 Runner payload 通过。
- `minutes +transcript --id u1 --query weekly` 在 PreParse 后以 validation/exit 3 停止，不 dispatch。

### 3.3 搜索、分页和时间

当前真实表面同时存在：

- `query/keyword`；
- `limit/max/page-limit`；
- opaque string cursor 与 `audio-memo list` 的 integer cursor；
- RFC3339 `start/end` 与持续时间/轮询 timeout。

候选只做无损、角色明确的归一化，并阻断类型或角色冲突。尤其不把 `page-limit` 当页大小，不把 opaque `next-token` 送入 integer cursor，不把单个 `date-range` 拆成两个 flag。

### 3.4 权限、用户身份、文本和文件

- 原子权限命令的 `policy` 是数字值域，Shortcut 的 `permission` 是语义枚举；仅改 flag 名会制造错误 payload，因此双向 block。
- `member-uids/target-uid` 与 staffId、unionId、openDingTalkId 不等价；缺少身份解析证据时 block。
- `content`、`search/replace`、speaker `from/to`、repeatable `pair` 和 JSON 虽然都是字符串，角色不同；只开放确定 alias，不能由中央层拼装结构值。
- `+upload --file` 是本地路径，原子 `upload create` 的 `file-name/file-size` 是元数据且大小单位为 bytes；中央 alias 不做文件 IO、stat 或单位换算。

### 3.5 Help/Schema/Skill 漂移

当前 Help 与 Schema 已经 62/62 对齐，漂移主要留在 Skill/reference：

- 仍有把 `max/next-token` 写成主参数的旧表述；当前主参数是 `limit/cursor`。
- 泛化声称所有需要 ID 的命令都支持 `id/uuid/task-uuid/url`，实际只有部分原子命令有隐藏兼容。
- 对 `--url` 的描述前后冲突；运行时现在只在声明该隐藏 flag 的原子叶上严格解析。
- 原子命令树遗漏 `audio-memo list`、`hot-word delete`，并误述裸 `minutes list` 的行为。

CLI 兜底可以降低错误 argv 风险，但不能替代 Skill 文档修订。

## 4. 相对 2026-08-10 的 Shortcut 变化

相对历史盘点新增 15 个 Shortcut：`+search`、`+download`、`+upload`、`+update`、`+apply-permission`、`+summary`、`+speaker-replace`、`+record-wrap-up`、`+upload-and-analyze`、`+mindmap`、`+speaker-insights`、`+prepare-asr`、`+export-pack`、`+share`、`+unshare`。

这些命令全部进入当前 Schema，本轮全部纳入实体、基数、值域、角色、类型、单位和 complete-command 验证。历史 `+latest-minutes` 当前是 `minutes +latest` 的 CLI path alias，不是新的主路径。

## 5. 候选 param_concepts.json

候选是当前正式表的完整副本加本产品合理增量：

| 指标 | 正式表 | 候选 | 增量 |
|---|---:|---:|---:|
| concepts | 45 | 49 | +4 |
| command overrides | 208 | 257 | +49 |
| validation fixtures | 426 | 487 | +61 |

新增 4 个 Minutes concept：`minutes_task_uuid`、`minutes_task_uuids`、`minutes_hot_words`、`minutes_upload_session_id`；同时只在 Minutes 命令 scope 内扩展 `search_query`、`pagination_size`、`page_cursor`、`content_text`、`time_start`、`time_end` 和 `user_ids`。

隔离生成统计：59 个 Minutes command rules、274 aliases、476 blocks、3 ambiguous。生成文件逐 entry 对比确认：候选改变了 59 个 Minutes entry，**非 Minutes entry 变化为 0**。

候选专属测试数据与正式测试数据分开注册：正式表继续要求 170 个 active command 与 170 个模板精确相等；只有候选临时安装时才激活额外 20 个 Minutes 模板和 5 个代表性最终 payload 断言，候选覆盖为 190/190。

## 6. `public=false` Shortcut 生成器实测

旧结论“所有 `public=false` Shortcut 都无法中央治理”过于宽泛；应按当前框架和具体命令实测。对当前唯一目标 `minutes +minutes-search`，结论仍成立。

在隔离候选中把它加入 `search_query.commands` 后执行生成器，得到精确错误：

```text
concept "search_query" command scope "minutes +minutes-search" does not match any runnable Cobra command
```

因此候选不伪造该 scope。当前安全做法是继续使用真实 `--query`；如果传 `--keyword`，只得到普通 `unknown_flag`，没有中央 did-you-mean。正式解决需要统一 Shortcut registry 与生成器 runnable command 集合，或增加精确、非通配、可审查的 exclusion 机制。

## 7. 验证结果

所有写命令验证只使用 `--dry-run`、PreParse/validation 截断或注入 Runner；没有真实写入。

| 验证 | 结果 | 说明 |
|---|---|---|
| 基线 `make build` 与哈希记录 | 通过 | 构建来源与冻结提交已记录 |
| 62 叶 Cobra Help / Schema 对账 | 通过 | 可见业务参数差异 0 |
| 候选 `go generate ./internal/cli` | 通过 | 全仓 commands=594；Minutes rules=59 |
| alias/block/ambiguous fixture | 通过 | 含新增单双值、session、template、target/tag/task/resume 保护 |
| 合法/非法 URL | 通过 | 合法 dry-run 只传提取 ID；非法输入 validation/exit 3 且不 dispatch |
| `+transcript` alias/互斥 | 通过 | no-id query payload 通过；id+query 提前拒绝 |
| complete-command 模板 | 通过 | 正式 170/170；候选 190/190，321 active cases |
| `go test ./internal/cli ./internal/pipeline ./internal/app` | 通过 | `internal/app` 在允许 httptest 本地端口的环境执行 |
| `check-generated-drift.sh` | 通过 | 候选输入与生成结果一致、Schema 组装确定 |
| `check-schema-catalog.sh` | 通过 | Catalog、同源性和覆盖门禁通过 |
| `make policy` | 通过 | 参数、Schema、示例、安全与 policy 门禁通过 |
| 非目标产品回归 | 通过 | 非 Minutes 生成 entry 变化 0；Catalog 仍为 27/1,152 |
| public=false exact generator test | 预期未通过 | 证明当前框架确实不接受 `+minutes-search` scope |

## 8. 正式落地前仍需处理

1. 评审并合入候选 `param_concepts.json` 的 Minutes 增量；合入时再运行同一组生成、完整模板、非目标产品与 policy 门禁。
2. 决定 `public=false` Shortcut 的中央治理模型；不能通过伪造 public path 或通配 exclusion 绕过。
3. 修订 Minutes Skill/reference，以真实 `limit/cursor/query`、当前命令树和严格 URL/ID 边界为准。
4. 如果要支持 `permission ↔ policy`、跨身份 ID 或路径到上传元数据，必须先增加显式转换器、来源证明、歧义处理和最终 payload 测试。
5. 保留 URL 解析的 fail-closed 回归：多 ID flag、复数值、冲突 query/path、错误域名/端口和额外路径均不得到达 MCP。

## 9. 可复用分析流程

1. `git fetch origin`，记录任务创建时最新 `origin/main` 的完整 commit，再从该提交构建并记录二进制哈希。
2. 从真实 Cobra tree、运行时组装 Schema、全部内置 Shortcut、exclusions 和 Skill 建立事实集；历史产物只做差异提示。
3. 按“实体、基数、角色、值域、类型/单位、生命周期”对每个 flag 分类，先判断能否无损改名，再决定 alias、block 或 ambiguous。
4. 对隐藏兼容 flag 检查最终 payload，不能只看 Cobra 接受；涉及 URL、单位、身份、枚举或 IO 时必须进入显式代码转换/验证层。
5. 候选基于正式参数表做完整副本，只加入目标产品增量；隔离替换正式输入并重新生成。
6. 为每个 active command 提供完整可执行模板；为代表 alias 比较 canonical/alias 最终 Runner payload，写操作只 dry-run 或注入 Runner。
7. 实测 public/private Shortcut 的生成器边界；不把 generator error 当成抽象猜测。
8. 运行 focused tests、目标包回归、generated drift、Schema catalog、policy 和非目标产品生成差异检查。
9. 最后生成 Markdown 与五页 XLSX，逐页渲染检查；在正式落地前明确列出可完成、不可完成及所需代码/测试。

## 10. 最终判定

本轮直接可实现的 Minutes 参数兜底已经补齐，并有运行时、PreParse、Runner payload、完整模板和政策门禁证据。候选可以进入正式评审，但本次仍只保留为 `docs/parameter-hallucination/minutes/param_concepts.json` 草稿，不直接修改正式 `internal/cli/param_concepts.json`，也不提交、不推送、不提 PR。
