---
name: dingtalk-doc
description: 钉钉在线文字文档（adoc）。Use when 用户要查找、阅读、创建、生成、撰写或编辑正文，处理块/评论/附件/导入导出/模板/版本/权限，或整理文档且未明确本地文件。默认创建在线 adoc；本地 .md 走 dingtalk-misc，文件和文档节点存储操作走 dingtalk-drive，知识库走 dingtalk-wiki。命令前缀：dws doc。
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
- 已知命令直接执行，Help 不参与选路。`unknown flag` 后仅查一次 leaf Help；`unknown command` 后仅查一次 shortcut 清单；禁止试探后缀。
- 参数/安全语义不确定时仅查一次 leaf Schema（限 `use_when,avoid_when,parameters,constraints,confirmation`）；禁用产品级/`--all` 和失败探测。
- 本地内容暂存 cwd 后用文件参数传递；stdin 只承载内容，不承载确认。
- 不猜命令、flag、字段、ID、账号或时间。后续 ID 必须来自真实返回；零命中、多候选或类型不明时停止并消歧。
- 解析、读取和执行使用同一 profile；禁止跨组织复用 userId/openDingTalkId/openConversationId。多账号组织仅用明确的 `isOrgCurrent=true` 默认账号；没有默认账号时要求指定，禁止选择第一项、最近登录或最近使用账号。
- 不输出或记录 token、refresh token、appSecret、webhook token 等凭据；宿主已注入认证时不要索要凭据。
- `not_required` 直接执行不加 `--yes`；`user_required` 须两阶段确认。首次请求只确定意图；另行说明对象、动作和影响。拒绝或未明确同意时不调用；同意后才在原参数追加 `--yes`。
- 写后按任务结果契约验证；不能仅凭退出码宣称成功。部分结果、未知投递状态和失败项必须如实保留。
- 时间戳面向用户展示时转换为带时区的可读时间；默认使用当前会话时区，必要时同时保留原值。
- 遇到认证、权限、profile、confirmation 或未知错误时，只加载 `dingtalk-shared` 中对应 reference；不要连续猜测替代命令。
<!-- DWS_RUNTIME_CONTRACT_END -->

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcut 发现（按需）

仅当下方路由和 reference 都无法定位低频能力时，才执行 `dws shortcut list --service doc --format json`；已知意图不加载完整 Catalog。
<!-- VISIBLE_SHORTCUTS_END -->

## Golden Route

选择最小入口。ID/URL 直用；只有标题时先搜索。零/多候选停止；禁止默认第一项，用户明确顺序或序号时才按其规则取真实 ID。

在线文档节点的复制、移动和重命名属于 `dingtalk-drive`；文档正文的创建、读取、追加、覆盖和 block 编辑属于 `dingtalk-doc`。

| 用户意图 | 唯一推荐入口 | 关键边界 |
|---|---|---|
| 按标题定位文档 | `dws doc +search --query <关键词>` | 检查类型和分页；零/多候选停止 |
| 读取正文或 block ID | `dws doc +fetch` | `dws doc +fetch --node <ID> --detail with-ids`；不用 `+inspect --detail` |
| 聚合元信息 | `dws doc +inspect` | 按需加 `--include-style`、`--include-permissions`、`--include-history`、`--include-media`、`--include-comments`；不存在 `--include <值>` |
| 创建正文 | `dws doc +create` | 文件：`dws doc +create --name <标题> --content @./content.md`；stdin：`dws doc +create --name <标题> --content -`；禁止 `@绝对路径`、`@../路径`、`@-` |
| 追加或覆盖正文 | `dws doc +update` | 追加：`--node <ID> --command append --content @./content.md`；覆盖把动作改为 `overwrite`；JSONML 才加 `--doc-format jsonml` |
| 插入或替换 block | `dws doc +update` | 插入：`--node <ID> --command block_insert --ref-block <BLOCK_ID> --where before\|after --content @./content.md`；替换：`--node <ID> --command block_replace --block-id <BLOCK_ID> --content @./content.md`；`--command` 只传动作名 |
| 文档空间或在线文档节点存储操作 | `dws drive` | 节点用 `dws drive +copy` / `dws drive +move` / `dws drive +rename` / `dws drive +delete`；正文才用 doc |
| 新增权限 | `dws doc +access-grant --node <ID> --to <姓名[,姓名]> --role <READER\|DOWNLOADER\|EDITOR\|MANAGER>` | 接收人参数不是 `--user`；姓名歧义时停止 |
| 变更已有权限 | `dws doc +access-change --node <ID> --to <姓名[,姓名]> --role <角色>` | 目标不是已有协作者时停止，改用 grant |
| 撤销权限 | `dws doc +access-revoke --node <ID> --to <姓名[,姓名]>` | 属于 `user_required` |
| 私信文档链接但不改权限 | `dws doc +share --to <姓名[,姓名]> --url <URL> [--note <附言>]` | 普通文本私信走 `dws chat +dm` |
| 授权后私信链接 | `dws doc +grant-and-share` | 查看权限用 `dws doc +inspect --node <ID> --include-permissions`；不存在 `+access-list`；保留逐人失败 |

### 低频精确入口

仅在意图命中时使用，不为这些已知命令查询 Help/Catalog：

| 用户意图 | 入口 |
|---|---|
| 恢复点/版本 | 重要更新用 `dws doc +checkpoint-update`；版本用 `dws doc +version-save --node` / `dws doc +version-list --node` / `dws doc +version-revert --node --version`，使用真实版本 ID |
| 导入/导出 | `dws doc +import --file <相对路径> [--folder <ID> \| --workspace <ID>]`，目标最多提供一个，省略时用默认个人文档根目录；导出用 `+export --export-format <格式>` |
| 模板 | 无查询词用 `dws doc +template-list [--source MY\|PUBLIC]`；有查询词用 `dws doc +template-search --query <名称或关键词>`；唯一 ID 用 `dws doc +create-from-template --template-id <唯一ID>` |
| 评论/媒体/样式 | 评论用 `+comment-create/+review`；媒体用 `dws doc +media-insert/+media-download` 和 cwd 相对路径；封面用 `dws doc +cover-set/+cover-download/+cover-clear`，封面不是正文图片 |

## 关键结果语义

## 参数与安全边界

- 生成文档的已有或临时文件先暂存到 cwd，再传 `@./相对路径`；也可用 `--content -`。禁绝对路径、`..` 和把正文作为 `printf` 格式串。
- 执行预算：准备 Help 时，本轮仅查一次 leaf；`unknown command` 后只查一次 shortcut 清单。Schema 使用 `--fields use_when,avoid_when,parameters,constraints,confirmation`；禁止靠失败探测门禁。
- block ID 必须来自真实回执或 `+fetch --detail with-ids`。Block ID 生命周期：replace/delete/overwrite 后不复用受影响 ID；insert/copy 使用回执新 ID；下步依赖新结构且回执无稳定 ID 时定点 refetch。
- 媒体插入回执中的 `insertedBlockId` 是图片/附件容器本身，不是“图片后的空块”。只有 `position.followingBlockExists=true` 时才存在独立后继块，并且只能用 `position.followingBlockId` 清理它；`false` 时禁止删除媒体容器或重传媒体来伪造空块清理。
- 安全模型以 leaf Schema 为准，并遵循上方两阶段确认协议。
- 参数不确定时仅查一次精确 leaf Schema；禁产品级/`--all`。
- **空列表必须使用 JSONML**：用户要求保留无文本的列表项/列表占位时，不得用 Markdown 的裸 `-`、`*` 或 `1.`，因为它可能被规范化为普通空行或丢弃。
- 最小空列表 JSONML：`["bulletList",{"listId":"list-1","isOrdered":false},["listItem",{},["paragraph",{},["text",{"data-type":"leaf"},""]]]]`。
- JSONML 顶层必须是单个非空元素；禁止 `[[...]]` 元素数组包裹。

## 按需加载

Golden Route 已给出命令且参数足够时，禁止读取 reference；其余仅在语义不明确时，才最多读取一个 reference：

| 触发条件 | Reference |
|---|---|
| 低频意图消歧 | [intent-guide](references/intent-guide.md) / [doc](references/doc.md) |
| 分页、`partial_success`、`status=unknown` 或恢复 | [contracts.md](references/contracts.md) |
| create/read/update 非默认操作 | [create](references/doc/doc-create.md) / [read](references/doc/doc-read.md) / [update](references/doc/doc-update.md) |
| 空列表、颜色、callout、分栏等复杂 JSONML | [create workflow](references/doc/style/doc-create-workflow.md) |
| block/划词评论/媒体高级参数 | [block](references/doc/doc-block.md) / [comment](references/doc/doc-comment.md) / [media](references/doc/doc-media.md) |
| 导出/导入失败恢复 | [export](references/doc/doc-export.md) / [import](references/doc/doc-import.md) |

普通纪要、周报、方案用 Markdown；空列表或富结构才用 JSONML。`+create`、`+fetch`、`+update` append/overwrite、`+export`、`+import` 禁止读取 reference；不连读链接。

## 错误最短路径

1. 零/多候选、类型不明或分页不完整：停止写入，展示候选/continuation。
2. 真实 `unknown flag` 后仅查一次 leaf Help；`unknown command` 仅查一次 shortcut 清单，禁止猜命令。
3. `REVISION_CONFLICT` 重读 revision；`partial_success` 不重放成功步骤；`doc_write_commit_unknown` 先回读且不重试写入。
4. 认证/权限/profile 错误只读 shared 对应 reference；导出/媒体失败保留真实 ID，禁 `curl`、安装依赖或手写 HTTP 兜底。

## 跨产品边界

- 文件/目录/上传下载及文档节点复制、移动、重命名、删除 → `dingtalk-drive`；正文读写 → `dingtalk-doc`。
- 明确知识库/wiki/命名团队空间 → `dingtalk-wiki`；明确本地 `.md` 或持续监听 → `dingtalk-misc`。
- 正文内嵌表格、AI 表格、听记、`axls`、`able` → 保留真实 ID/URL 后切对应 Skill。
