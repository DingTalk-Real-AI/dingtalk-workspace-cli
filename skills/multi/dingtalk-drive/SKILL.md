---
name: dingtalk-drive
description: 钉钉文件管理（存储层，覆盖钉盘与文档空间）。Use when 用户说 钉盘/文档空间/我的文档中的普通文件或文件夹、查找/上传/下载/复制/移动/重命名/删除/回收站/权限/评论/元信息，或本地与钉盘文件夹比较、拉取、推送、双向同步；也承接在线文档节点的存储管理。文档正文编辑与导出走 dingtalk-doc；明确的知识库空间及空间内节点组织走 dingtalk-wiki。命令前缀：dws drive。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉盘

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

`drive` 当前有 28 条公开 shortcut，完整清单保留在 Runtime Catalog 与 Schema，不在高频产品根 Skill 中重复展开。已知意图按下方路由。

仅当现有路由和 reference 都无法定位低频能力时，才执行 `dws shortcut list --service drive --format json` 做最后回退；不要为已知高频意图加载完整 Shortcut Catalog 或产品级 Schema。
<!-- VISIBLE_SHORTCUTS_END -->

## Golden Route

| 用户意图 | 唯一推荐入口 | 关键边界 |
|---|---|---|
| 全局按名称或关键词找文件 | `dws drive +search --query <关键词>` | 多候选停止；Drive 搜索没有 Doc 的 `--page-all`，按真实 nextCursor 翻页；在线文档正文搜索走 `doc +search` |
| 浏览根目录或已知文件夹 | `dws drive +list [--folder <dentryUuid>]` | 默认一页，处理 nextCursor |
| 发现钉盘企业空间或“我的文件”空间 | `dws wiki space list --type <orgSpace\|mySpace> --format json` | Drive 只读前置；orgSpace 按 nextToken 续页，取 spaceId/rootFolderId 后回到 Drive |
| 查看最近访问/编辑 | `dws drive +recent [--operate-type 1] --limit <N>` | 1=最近编辑；默认最近访问 |
| 查看节点类型和元数据 | `dws drive +inspect --node <dentryUuid>` | 按需加 stats/publish/cover，不为普通列表强制调用 |
| 下载普通文件 | `dws drive +download --node <dentryUuid> --output <相对路径>` | 当前 shortcut 接受 ID；在线文档用 `doc +export` |
| 上传新文件或覆盖普通文件 | `dws drive +upload --file <相对路径>` | 新建可加 folder；覆盖改加 node，二者互斥 |
| 管理普通文件全局评论 | `dws drive comment list-v2/create-v2/reply/update/delete/batch-query/list-replies/resolve/restore/react-reply` | 复用 Doc/Sheet 新评论链路；旧 `list/create` 已 deprecated；固定全文 `global`，不支持划词、单元格或 mention |
| 创建文件夹 | `dws drive +create-folder --name <名称> [--folder <ID>]` | Shortcut 已提交并读回 |
| 复制在线文档节点 | `dws drive +copy --node <ID> [--folder <目标ID>]` | 普通钉盘文件会被拒绝；Base 结构复制走 AITable `+base-copy --base-id <ID> --target-folder-id <真实ID> --only-struct` |
| 移动节点 | `dws drive +move --node <ID> --folder <目标ID>` | 破坏性变更，按 Runtime confirmation |
| 重命名节点 | `dws drive +rename --node <ID> --name <新名称>` | 写后检查最终名称 |
| 比较本地与钉盘文件夹 | `dws drive status --local-folder <绝对路径> --remote-folder <folderId>` | 只读；默认精确 MD5，不先拉取或推送 |
| 钉盘文件夹拉到本地 | `dws drive pull --local-folder <绝对路径> --remote-folder <folderId> --if-exists skip` | 安全默认不覆盖；先以相同参数 `--dry-run`，再按确认执行 |
| 本地文件夹推到钉盘 | `dws drive push --local-folder <绝对路径> --remote-folder <folderId> --if-exists skip` | 安全默认不覆盖；先 dry-run；不会删除远端多余文件 |
| 双向补齐文件夹 | `dws drive sync --local-folder <绝对路径> --remote-folder <folderId> --on-conflict skip` | 先 dry-run；冲突策略必须显式保留 |

### 低频入口

- 删除已确认节点：`dws drive +delete --node <dentryUuid>`；恢复：`+recycle-list/+recycle-restore`；版本：`+version-history/+version-get/+version-download/+version-revert`。
- 收藏：`+star-*`；公开状态：`+publish-get/+publish-unset`（`+publish-set` 不进入 Agent 路由）；统计/封面用 `+inspect`；快捷方式用 `+create-shortcut`。
- 目录树只用有界 `+list` 逐层遍历。

兼容别名不选路：`+info`→`+inspect`，`+find-file`→`+search`，`+search-docs`→`doc +search`。

## 当前最短路径

- 已知 dentryUuid：直接执行 inspect/download/list/move/rename，禁止先 search；仅确认是受支持的在线文档节点后才执行 copy。
- 目标 Drive 空间未知：先明确企业空间 `orgSpace` 或“我的文件”`mySpace`，用 `dws wiki space list --type <类型> --format json` 发现空间；`orgSpace` 在 `nextToken` 非空时以 `--cursor <nextToken>` 续页，`mySpace` 固定单条且不分页。按后续命令取真实 spaceId 或 rootFolderId 后立即回到 Drive；已知这些 ID 时不做空间发现。
- 只有名称：`+search` → 唯一候选的 nodeId → 目标命令；不得自动选择第一项。
- 只有文件夹层级：从最近的已知 folder ID 开始 `+list`，不要从根目录无界递归。
- 上传新文件：单条 `+upload`；不要退回 upload-info + 手写 HTTP + commit。
- 导出后上传：`doc +export` 首次就指定最终本地文件名，直接复用回执 `localPath`，首次正式 `drive +upload` 带已获授权的 `--yes`；禁止上传后再 rename。
- copy/move/rename/create-folder 已内置写后读取时，不再由 Agent重复执行 `+inspect`。
- 已知 nodeId 的重命名直接 `+rename`，不先 Catalog、Help 或 search；ALIDOC 的逻辑标题由 shortcut 内部文档读回验证。
- 文件夹方向已明确时直接 `status/pull/push/sync`，不先 status；写操作先用完全相同参数 dry-run，再正式执行。
- 搜索结果 `type=able` 后按业务动词重路由：结构复制/删除/Base 内操作走 AITable。结构复制按当前 leaf 提供源 Base ID 和真实 `--target-folder-id`；缺少目标 ID 时停止，不猜根 ID或发明 `--target-root`。
- `+inspect/+download/+list` 只保证 dentryUuid；只有 URL 时先用 `dws drive info --node <URL> --format json` 解析并核对 nodeId。

最短路径不省略类型检查、确认、传输验证或写后校验。

## 关键结果语义

- `+list/+search/+recent` 检查集合、hasMore 和 nextCursor；缺少集合不能当空结果，多候选禁止默认第一项。
- `+download` 验证相对路径存在且 sizeBytes > 0；`+upload` 检查最终 nodeId、名称、类型和大小。只有源端与结果都提供可比哈希时才核对 checksum；缺失时保留现有证据，不虚构端到端校验和。
- copy/move/rename/create-folder 检查 `ok/outcome` 和读回；`partial_success` 不是完成。
- status 检查分类集合；pull/push/sync 检查 summary 和逐项结果，failed/unknown 必须保留。
- 分页未结束时返回 continuation；目录树或大列表必须有最大深度、页数和条目数。
- 未知写入效果先 inspect/list 回读，不盲目重放写操作。

## 参数与安全边界

- `--node`、`--folder` 使用 dentryUuid/fileId，不使用数字型 dentryId；回收站 restore 使用 recycleItemId。
- `+list --limit` 最大 50，`+search --limit` 最大 30；超过时分页，不以非法参数反复试错。
- 写操作只按精确 leaf Runtime 判定确认；已明确授权具体对象、动作与影响时，首次正式执行直接带 `--yes`，否则先确认。预览不带，参数变化重新确认；禁止用缺少 `--yes` 的失败探测。
- 普通文件覆盖前确认真实类型和原名称；adoc/axls/able 不按普通文件覆盖。
- 单文件 Shortcut 的本地输入输出使用 cwd 相对路径且禁止 `..`；文件夹 `status/pull/push/sync` 按 leaf 契约使用绝对 `--local-folder`。
- 文件夹同步默认精确 MD5；仅在用户接受时间戳近似时用 `--quick`，且不会删除任一侧多余文件。
- 参数不确定时只查一次精确 leaf Schema；禁止产品级 Schema。

## 按需加载

Golden Route 参数足够时禁止读取 reference。其余最多读取一个精确 reference：

| 触发条件 | Reference |
|---|---|
| URL、文件类型或跨产品边界 | [intent-guide](references/intent-guide.md) |
| 文件夹比较、拉取、推送或双向同步 | [folder-sync](references/folder-sync.md) |
| 低频权限、版本、回收站、公开状态 | [drive reference](references/drive.md) 的对应章节 |
| 文档查询、导入和模板保形流程 | [lite-recipes](references/lite-recipes.md) |

## 错误最短路径

1. 零/多候选、类型不明或分页不完整：停止写入，返回候选或 continuation。
2. `unknown flag`：只查一次当前 leaf Help；`unknown command`：只查一次 Drive shortcut 清单。
3. 普通下载遇到在线文档类型：切 `doc +export`，不重复尝试 Drive download。
4. 传输中断：保留本地临时状态或 checkpoint；先判断能否续传。
5. 写入效果未知：按 nodeId 回读；无法证明时报告 unknown。
6. 普通文件 `+copy` 被拒绝时不要重试或伪装成功；独立副本改走经用户授权的 download→upload。AITable 结构复制缺少或无法验证目标文件夹时停止，不猜 ID或创建测试文件夹。

## 跨产品边界

- 普通文件/文件夹及在线文档节点的存储管理 → Drive；把文件作为附件放进某篇文档正文走 Doc `+media-insert`，其他正文/内容分别走 Doc、Sheet、AITable。
- able 外层移动/重命名走 Drive；结构复制、Base 删除（`+base-delete`）及 Base 内操作走 AITable。
- 明确知识库 workspace 层级 → Wiki；泛称“文档空间/我的文档”仍走 Drive。
- 钉盘存储空间发现例外地复用 managed `dws wiki space list --type orgSpace|mySpace`；只取真实 spaceId/rootFolderId 后回到 Drive。spaceId 用于空间参数，rootFolderId 才可作为空间根目录 folder；`orgWikiSpace/myWikiSpace` 返回 workspaceId，不能混入 Drive 参数。
- Word/Markdown/Text 转在线文档用 `doc +import`；Drive upload 只保留原文件。
