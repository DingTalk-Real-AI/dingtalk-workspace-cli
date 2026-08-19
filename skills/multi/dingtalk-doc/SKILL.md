---
name: dingtalk-doc
description: 钉钉在线文字文档（adoc）本体及其内容的操作：查找、创建、读取、文档信息、编辑、块、评论、附件与媒体、白板卡片、导入、导出(docx/markdown/pdf)、版本、模板、权限、分享及Markdown/JSONML写入。不包括：文档空间与钉盘的文件管理（归 dingtalk-drive，doc 同名原子命令已弃用）、知识库空间与节点管理（归 dingtalk-wiki）、原生 .md 文件读写（归 dingtalk-misc）、电子表格 axls（归 dingtalk-misc）、AI 表格 able（归 dingtalk-aitable）。命令前缀：dws doc。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉文档 Skill

<!-- DWS_RUNTIME_CONTRACT_START -->
## 最小 DWS 执行契约

- 只通过 `dws` CLI 操作钉钉；结构化读取使用 `--format json`，按真实返回判断结果。
- 已知命令直接执行。只有 leaf 参数或安全语义不确定时读取精确 Schema，只有 Cobra flag 不确定时读取精确 leaf Help；不要加载产品级 Catalog 代替选路。
- 不猜命令、flag、字段、ID、账号或时间。后续 ID 必须来自真实返回；零命中、多候选或类型不明时停止并消歧。
- 解析目标、读取上下文和最终执行必须使用同一 profile；不得跨组织复用 userId、openDingTalkId 或 openConversationId。多账号组织只使用明确的 `isOrgCurrent=true` 默认账号；没有默认账号时要求用户指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出或记录 token、refresh token、appSecret、webhook token 等凭据；宿主已注入认证时不要索要凭据。
- 写操作必须符合用户明确意图。是否需要确认以最终 Runtime gate 和 Schema 为准；需要确认时先说明对象、动作与影响，再追加 `--yes`。
- 写后按任务结果契约验证；不能仅凭退出码宣称成功。部分结果、未知投递状态和失败项必须如实保留。
- 时间戳面向用户展示时转换为带时区的可读时间；默认使用当前会话时区，必要时同时保留原值。
- 遇到认证、权限、profile、confirmation 或未知错误时，只加载 `dingtalk-shared` 中对应 reference；不要连续猜测替代命令。
<!-- DWS_RUNTIME_CONTRACT_END -->

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcut 发现（按需）

`doc` 当前有 45 条公开 shortcut，完整清单保留在 Runtime Catalog 与 Schema，不在高频产品根 Skill 中重复展开。已知意图按下方路由。

仅当现有路由和 reference 都无法定位低频能力时，才执行 `dws shortcut list --service doc --format json` 做最后回退；不要为已知高频意图加载完整 Shortcut Catalog 或产品级 Schema。
<!-- VISIBLE_SHORTCUTS_END -->

## Golden Route

选择最小入口。ID/URL 直用；只有标题时先搜索，唯一候选后执行，禁止默认第一项。

| 用户意图 | 唯一推荐入口 | 关键边界 |
|---|---|---|
| 按标题或主题定位文档 | `dws doc +search --query <关键词>` | 检查候选类型与分页；需要正文时再用真实 `nodeId` 执行 `+fetch` |
| 已知 ID/URL 读取正文或局部内容 | `dws doc +fetch --node <ID或URL>` | `--scope keyword/section/range` 可减少无关正文；非 adoc 切换对应产品 |
| 聚合查看信息、权限、版本、媒体或评论 | `dws doc +inspect --node <ID或URL>` | 仅打开任务所需的 `--include-*`，不要默认全取 |
| 新建在线文字文档并写入内容 | `dws doc +create --name <标题> --content @<相对文件>` | 长或复杂 Markdown 使用 `@file`/stdin；根据结果验证真实 `nodeId` |
| 追加、覆盖或精确编辑 block | `dws doc +update --node <ID或URL> --command <动作>` | `--expected-revision` 仅用于 JSONML 原子覆盖；先消费 Schema confirmation |
| 重要内容更新且需要恢复点 | `dws doc +checkpoint-update` | 自动保存版本、更新并回读；检查 `steps` 和 `compensation` |
| 版本操作 | `dws doc +version-save --node` / `dws doc +version-list --node` / `dws doc +version-revert --node --version` | 快照/列表/回滚 |
| 导出为 docx/markdown/pdf | `dws doc +export --export-format <格式>` | 格式必须显式指定；普通文件下载切 `dingtalk-drive` |
| 导入本地文件为在线对象 | `dws doc +import --file <相对路径>` | folder/workspace 可选，缺省导入默认根；纯上传切 `dingtalk-drive` |
| 浏览模板 | `dws doc +template-list [--source MY\|PUBLIC]` | 没有名称/关键词时使用；不调用 search/create |
| 搜索模板 | `dws doc +template-search --query <名称或关键词>` | 有查询词才使用；零或多候选停止 |
| 从模板创建 | `dws doc +create-from-template --template-id <唯一ID>` | 已有唯一 templateId 才创建；不重复 list/search |
| 创建评论或聚合待处理评论 | `dws doc +comment-create [--selection]` / `+review` | 划词统一用 `+comment-create`；后续操作使用真实 `commentKey` |
| 添加/调整/移除协作者权限 | `dws doc +access-grant/+access-change/+access-revoke` | 先读取现有权限；姓名歧义或 profile 不一致时禁止写入 |
| 授权后向多人分享链接 | `dws doc +grant-and-share` | 返回逐人执行账本；部分失败不等于整体成功 |
| 插入或下载正文媒体 | `dws doc +media-insert/+media-download` | 本地路径必须位于工作目录；下载默认 no-clobber |

## 关键结果语义

- 保留真实 `nodeId`、URL、资源类型和容器；标题不能替代稳定 ID。
- 按状态恢复：`partial_success` 只补未完成步骤；`unknown` 先回读且不重试写入；`retryable` 仅限明确未开始；权限、参数、认证失败停止。
- 仅在结果明确且关键内容回读匹配后报告写入完成。
- 搜索、列表和批量结果必须检查 `complete`、`hasMore`、continuation 和失败项；单页结果不得表述为完整集合。
- 导出/下载仅用工作目录相对路径，默认不覆盖并原子落盘。

## 参数与安全边界

- `@file` 统一协议：已有或临时文件先暂存到 cwd 后传 `@相对路径`；单次生成文本优先 `--content -` (stdin)；禁止绝对路径和 `..`。
- `doc +update` 的动作由 `--command` 指定；block 操作的 ID 必须来自 `+fetch --detail with-ids` 或真实 block 列表。
- Schema 门禁：已知只读路由且参数明确时直接执行；写命令或 selection/参数不清、准备 Help 时，本轮仅查一次 `dws schema --cli-path "doc +<leaf>" --compact --fields use_when,avoid_when,parameters,constraints,confirmation -f json`；禁用产品级/`--all`。
- 执行前消费其中的 `confirmation`：`user_required` 且用户已确认同一目标/动作/参数才加 `--yes`，否则预览/询问；禁止靠失败探测门禁。
- JSONML 顶层必须是单个非空元素；禁止 `[[...]]` 元素数组包裹。

## 按需加载

Golden Route 已给出命令且参数足够时，禁止读取 reference。其余仅在下列语义不明确时，才最多读取一个 reference：

| 触发条件 | Reference |
|---|---|
| 低频/无 shortcut 意图消歧 | [intent-guide.md](references/intent-guide.md) / [doc.md](references/doc.md) 对应章节 |
| 分页、`partial_success`、`status=unknown` 或恢复 | [contracts.md](references/contracts.md) |
| 复杂 JSONML/非默认操作 | [create](references/doc/doc-create.md) / [read](references/doc/doc-read.md) / [update](references/doc/doc-update.md) |
| block/划词评论/媒体高级参数 | [block](references/doc/doc-block.md) / [comment](references/doc/doc-comment.md) / [media](references/doc/doc-media.md) |
| 导出/导入失败恢复 | [export](references/doc/doc-export.md) / [import](references/doc/doc-import.md) |

`+create`、`+fetch`、`+update` append/overwrite、`+export`、`+import` 禁止读取 reference；禁止预加载或连读。

## 错误最短路径

1. 零命中、多候选、类型不明或分页不完整：停止后续写入，展示候选或 continuation；禁止默认第一项。
2. Help 不参与选路；按上方门禁先消费一次精确 leaf Schema。只有真实 `unknown flag`/契约漂移后才查一次 leaf Help；`unknown command` 只查一次 shortcut 清单，禁止试探后缀和 `dws doc --help | grep/head`。
3. `REVISION_CONFLICT`：重新读取当前 revision，展示差异；未经用户确认不得改成无 revision 覆盖。
4. `partial_success`：保留已完成步骤，优先执行补偿或从 checkpoint 继续，不重放成功步骤。
5. `doc_write_commit_unknown`：先回读文档确认状态；禁止自动重试创建或追加。
6. 认证、权限或 profile 错误：只读取 `dingtalk-shared` 对应 reference；不要尝试同义底层命令绕过。
7. 导出或媒体失败：保留 `jobId/resourceId/nodeId` 后停止；禁止 `curl`、安装依赖、Python 文档库或手写 HTTP 兜底。

## 跨产品边界

- 普通文件、目录、纯上传下载、节点存储权限 → `dingtalk-drive`
- 知识库空间、节点层级和成员管理 → `dingtalk-wiki`
- 原生 `.md` 文件读取和编辑 → `dingtalk-misc`
- `axls` / `able` → 对应电子表格或多维表 Skill
- 持续监听文档事件 → `dingtalk-misc`
