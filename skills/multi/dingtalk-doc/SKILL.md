---
name: dingtalk-doc
description: 钉钉在线文字文档（adoc）本体及其内容的操作：查找、创建、读取、文档信息、编辑、块、评论、附件与媒体、白板卡片、导入、导出(docx/markdown/pdf)、版本、模板、协作者权限、分享及Markdown/JSONML写入。不包括：文档空间与钉盘的文件管理（归 dingtalk-drive，doc 同名原子命令已弃用）、知识库空间与节点管理（归 dingtalk-wiki）、原生 .md 文件读写（归 dingtalk-misc）、电子表格 axls（归 dingtalk-misc）、AI 表格 able（归 dingtalk-aitable）。命令前缀：dws doc。
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

ID/URL 直用;标题先搜索,唯一命中再执行.顺序:稳定 ID → shortcut → 局部读 → 精确写;禁以产品 Schema、全文或原子命令起步.

| 用户意图 | 唯一推荐入口 | 关键边界 |
|---|---|---|
| <!-- dws-intent: doc.search.by_title -->按标题或主题定位文档 | `dws doc +search --query <精确标题>` | `complete=true,count=0,failures=[]` 即权威零命中：如实报告；禁缩词、跨产品、无 query/无端 `--page-all` |
| 最近访问或最近编辑文档 | 加载 `dingtalk-drive`，执行 `dws drive +recent [--operate-type 1] --limit <N>` | 默认最近访问，`1` 为最近编辑；不要用 `doc +search` 替代最近列表 |
| <!-- dws-intent: doc.content.read -->已知 ID/URL 读取正文或局部内容 | `dws doc +fetch --node <ID或URL>` | 术语用 `keyword`;章节 `outline` → `section`;整篇才用 `full` |
| 聚合查看信息、权限、版本、媒体或评论 | `dws doc +inspect --node <ID或URL>` | 基础元信息默认返回；样式、权限、历史、媒体、评论才用对应 `--include-*`，无 `--include-info` |
| 新建在线文字文档并写入内容 | `dws doc +create --name <标题> --content <文本\|-\|@文件> [--folder <ID>\|--workspace <ID>]` | 指定位置复用真实 ID，二者互斥;`-`=stdin,禁 `@-`;Runtime 分片回读,不拆写 |
| <!-- dws-intent: doc.content.update -->追加、覆盖或精确编辑 block | `dws doc +update --node <ID或URL> --command <动作>` | 唯一文本 `str_replace`;章节/block 局部取 ID;整篇才 overwrite |
| 重要内容更新且需要恢复点 | `dws doc +checkpoint-update` | 自动保存版本,更新并回读;检查 `steps` 和 `compensation` |
| 版本操作 | `dws doc +version-save --node` / `dws doc +version-list --node` / `dws doc +version-revert --node --version` | 快照/列表/回滚 |
| <!-- dws-intent: doc.export.format -->导出为 docx/markdown/pdf | `dws doc +export --export-format <格式>` | 格式必须显式指定;普通文件下载切 `dingtalk-drive` |
| <!-- dws-intent: doc.import.local_file -->本地文件转在线文档 | `dws doc +import --file <相对路径> [--folder <ID>\|--workspace <ID>]` | 指定位置复用真实 ID，二者互斥；未指定才由 Runtime 取默认根（唯一组织根目录）；成功须回读验证落点；仅保留原文件走 `dingtalk-drive` |
| 封面/背景 | `+resource-update/+resource-delete`；`+background-update/+background-delete` | 写后 `+inspect --include-style`；禁查 Catalog |
| 浏览模板 | `dws doc +template-list [--source MY\|PUBLIC] [--page-all]` | “我的/我这边”只查 MY；明确公开才查 PUBLIC；“有哪些/全部”加 `--page-all` 并检查 `complete` |
| 搜索模板 | `dws doc +template-search --query <名称或关键词>` | 来源可选 MY/PUBLIC；零命中停止，禁止拿无关模板替代；多候选消歧 |
| 从模板创建 | `dws doc +create-from-template --template-id <唯一ID>` | 已有唯一 templateId 才创建；不重复 list/search |
| 创建/查评论 | `dws doc +comment-create --node <ID或URL> --content <文字> [--selection <原文>]` / `+review --node <ID或URL>` | node/content 必填；划词也用 `+comment-create`；续操作复用 `commentKey` |
| <!-- dws-intent: doc.access.grant -->添加/调整/移除协作者权限 | `dws doc +access-grant/+access-change/+access-revoke` | `--to` 必填；`--role` 默认 READER（READER\|DOWNLOADER\|EDITOR\|MANAGER）；无 `--user-ids`；先读权限，歧义/profile 不一致禁写 |
| <!-- dws-intent: doc.share.link_only -->只发链接不改权限 | `dws doc +share --to <姓名[,姓名]> --url <URL> [--note <附言>]` | 内置姓名解析；仅歧义时 aisearch，禁预查人；普通私信用 chat |
| 授权后向多人分享链接 | `dws doc +grant-and-share` | 仅需改权限时用（必填 `--node`，role 默认 READER）；检查逐人账本和部分失败 |
| <!-- dws-intent: doc.media.insert -->把文件/PPT/PDF 作为正文附件 | `dws doc +media-insert --node <DOC_ID> --file <相对路径>` | 正文附件走 Doc；`drive +upload` 仅入库存储，不会插入正文 |

## 关键结果语义

- 保留真实 `nodeId`/URL/类型/容器；复用 ID，禁标题/钉盘重搜。
- 用完整回执；Runtime 回读后不复读；仅局部验收、`partial_success`/commit-unknown 再 `+fetch`。
- 恢复：`partial_success` 只补未完成；`unknown` 先回读、禁重写；`retryable` 仅限明确未开始；权限/参数/认证失败即停。
- 结果明确且回读匹配才报完成。
- 搜索/列表检查 `complete`/`hasMore`/cursor/失败项；“全部”翻完页，前 N 条须声明范围。
- `+import` 检查 `success=true`、`verified=true`、`taskId/nodeId/documentUrl`；复用返回 ID，禁 Drive 重找；中断查原任务，禁重导。
- 导出/下载用 cwd 相对路径；`+export` 有 `localPath` 且 `sizeBytes>0` 即终态，禁 `ls/stat`。

## 参数与安全边界

- `@file`：已有或临时文件先暂存到 cwd；传 `@相对路径`，生成文本优先 `--content -`；禁绝对路径和 `..`。
- `doc +update` 用 `--command` 指定动作；block ID 必须来自 `+fetch --detail with-ids` 或真实列表。
- Schema 门禁：不确定时仅查一次精确 leaf：`--fields use_when,avoid_when,parameters,constraints,confirmation`；禁用产品级/`--all`。准备 Help 时，本轮仅查一次。
- 消费本页或精确 Schema 的 `confirmation`：`user_required` 且原请求/预授权已确认目标、动作、参数时，首调即加 `--yes`；否则预览/询问；禁止靠失败探测门禁。
- JSONML 顶层必须是单个非空元素；禁止 `[[...]]` 元素数组包裹。

## 按需加载

Golden Route 已给出命令且参数足够时，禁止读取 reference；其余仅遇下表语义时才最多读取一个 reference：

| 触发条件 | Reference |
|---|---|
| 低频/无 shortcut 意图消歧 | [intent-guide.md](references/intent-guide.md) / [doc.md](references/doc.md) 对应章节 |
| 分页、`partial_success`、`status=unknown` 或恢复 | [contracts.md](references/contracts.md) |
| 复杂 JSONML、长文或局部精准读写 | [create](references/doc/doc-create.md) / [read](references/doc/doc-read.md) / [update](references/doc/doc-update.md) |
| block/划词评论/媒体/封面/背景高级参数 | [block](references/doc/doc-block.md) / [comment](references/doc/doc-comment.md) / [media](references/doc/doc-media.md) |
| 导出/导入失败恢复 | [export](references/doc/doc-export.md) / [import](references/doc/doc-import.md) |

常规 `+create`、`+fetch`、`+update` append/overwrite、`+export`、`+import` 禁止读取 reference；禁预加载/连读。

## 错误最短路径

1. 零命中、多候选、类型不明或分页不完整：停止写入，展示候选或 continuation；禁止默认第一项。
2. Help 不参与选路；先读一次精确 leaf Schema。仅真实 `unknown flag`/契约漂移查一次 leaf Help；`unknown command` 只查一次 shortcut 清单，禁止试探后缀和 `dws doc --help | grep/head`。
3. `REVISION_CONFLICT`：重新读取当前 revision，展示差异；未经用户确认不得改成无 revision 覆盖。
4. `doc_write_commit_unknown`：先回读；禁止自动重试创建或追加。
5. 认证、权限或 profile 错误：只读 `dingtalk-shared` 对应 reference，禁底层命令绕过。
6. 导出/媒体失败：保留稳定 ID 后停止；禁网络请求/安装依赖/本地文档库兜底。

## 产品边界

- 姓名/工号/部门/职责找人或解析 userId → `dingtalk-aisearch`；已有完整 userId 补详情才用 `dingtalk-contact`
- 普通文件/目录存储与上传下载 → `dingtalk-drive`；“放进/附到这篇文档”是正文附件 → `doc +media-insert`；在线转换 → `doc +import`
- 文档节点复制、移动、模板另存 → `dingtalk-drive +copy/+move`；doc 同名命令仅兼容
- 知识库空间、节点层级和成员管理 → `dingtalk-wiki`
- 原生 `.md` 文件读取和编辑 → `dingtalk-misc`
- `axls` / `able` → 对应电子表格或多维表 Skill
- 持续监听文档事件 → `dingtalk-misc`
