# 钉盘 (drive) 命令参考

钉盘 = DingTalk Drive，用于云端文件存储 / 上传 / 下载 / 目录管理。不是在线文档编辑；要编辑文档请用 [doc](./doc.md)。

## 查询命令帮助

当你不确定某个命令的具体参数、格式或可选项时，**优先执行 `--help` 查询**，不要猜测参数名或凭记忆编造。

```bash
# 查看 drive 下所有子命令
dws drive --help

# 查看具体命令的完整参数说明
dws drive list --help
dws drive search --help
dws drive upload --help
dws drive download --help
dws drive stats --help
dws drive shortcut --help
```

规则：
- 参数名不确定时 → 先 `--help`，再调用
- 报错 "unknown flag" 时 → `--help` 确认正确的 flag 名称
- 不确定某个功能是否存在时 → `dws drive --help` 查看命令列表

## 命令总览

### 获取文件/文件夹列表

```
Usage:
  dws drive list [flags]
Example:
  dws drive list --limit 20
  dws drive list --limit 20 --folder <dentryUuid> --order-by name --order asc
  dws drive list --folder <dentryUuid> --type file --start 7d
Flags:
      --limit int           每页返回数量，默认 20，最大 50 (可选)
      --cursor string       分页游标，首次不传 (可选)
      --order string        排序方向: asc|desc，默认 desc (可选，仅钉盘)
      --order-by string     排序字段: createTime|modifyTime|name (可选，仅钉盘)
      --folder string       父节点 ID (dentryUuid)，不传则列出空间根目录 (可选)
      --space-id string     钉盘空间 ID (纯数字)，不传则使用「我的文件」对应 spaceId (可选)
      --workspace string    文档空间/知识库 ID (加密 string 或 URL)，传入则路由到文档空间 (可选)
      --thumbnail           是否返回缩略图信息 (可选，仅钉盘)
      --pattern string      按名称通配过滤结果，如 "*日报*"（客户端过滤，无通配符时按子串匹配）(可选)
      --depth int           递归列出子目录层级，默认 1(仅当前层)，最大 5；与 --cursor/--limit 互斥 (可选)
      --type string         按节点类型过滤: file|folder（客户端过滤，见下节）(可选)
      --start string        按修改时间过滤·起始，如 7d / 2026-08-01 / RFC3339 (可选)
      --end string          按修改时间过滤·截止，语法同 --start (可选)
```

> 统一入口：默认列钉盘空间（`--space-id` 纯数字）；传 `--workspace` 时路由到文档空间/知识库列表。

类型/时间过滤（`--type` / `--start` / `--end`）：
- 语义：`--type` 按节点类型（file=文件 / folder=文件夹）；`--start`/`--end` 按**修改时间**圈定区间。
  注意与 `dws drive search` 的 `--modified-from/--modified-to` 区分：那两个收毫秒时间戳，这里收字符串语法；
  `--type`（节点类型）与 search 的 `--file-types`（内容类型 alidoc/image/...）也不是一回事。
- 时间语法：相对时间 `24h`/`7d`/`2w`（小时/天/周，按本机时钟换算）、RFC3339（`2026-08-01T00:00:00+08:00`）、
  无时区 ISO8601（`2026-08-01 08:00:00`，默认 Asia/Shanghai）、仅日期（`2026-08-01`）；
  不支持毫秒时间戳，不支持 `m` 单位。
- 执行方式：钉盘与知识库（--workspace）两路由统一为**客户端过滤**——全量扫描当前目录后在进程内筛选；
  与 `--depth>1` 组合时递归扫描后筛（被滤掉的条目仍占 2000 条全局上限）。
- 互斥：与 `--versions`/`--cursor`/`--order-by`/`--order`/`--limit` 不能同时使用（过滤模式为全量扫描，
  无游标与服务端排序语义）；可与 `--latest`/`--pattern`/`--depth` 组合，`--latest` 表示「符合条件的条目中最新 N 个」。
- 输出形态：带过滤时输出从单页透传变为聚合形态 `{items, maxDepth, truncated, errors}`。
- 已知代价：大目录（>2000 条）触顶截断时 `truncated=true`（退出码 0，结果每条都正确但没扫完）；
  建议用 `--folder` 指定子目录缩小扫描范围；带关键词的过滤场景改用 `dws drive search`。
- 与 `--latest` 组合时上一条不适用：排序基不完整的 Top-N 不是全局最新，故触顶截断**或**递归途中
  目录读取失败都拒绝产出并报错（`LATEST_SCAN_TRUNCATED` / `LATEST_SCAN_INCOMPLETE`），不会以
  退出码 0 交出结果；错误消息里带首个失败目录的 folder/depth/reason，以及一条复现原候选集
  （查询域 + `--folder` + `--pattern`/`--type`/`--start`/`--end`）的恢复命令。Windows 构建下，
  若原值含 shell 元字符则命令里只给占位符、原值另起一行以数据形式列出（cmd.exe 与 PowerShell
  没有共同安全的引用形式），照抄时需手动替换。

### 获取钉盘空间列表

> **Deprecated**：推荐改用 `dws wiki space list --type orgSpace` / `--type mySpace`（见 [wiki.md](./wiki.md)）。本命令仍可用，仅作兼容保留。
> 适用场景：复制/移动文件到「我的文件」或团队空间根目录时，先取 `rootFolderId`；或者枚举用户可访问的团队空间。

```
Usage:
  dws drive list-spaces [flags]
Example:
  dws wiki space list --type orgSpace     # 推荐
  dws wiki space list --type mySpace      # 推荐
  dws drive list-spaces                   # deprecated
  dws drive list-spaces --space-type orgSpace --limit 20 --cursor <TOKEN>
Flags:
      --space-type string   空间类型: orgSpace=企业空间(默认), mySpace=我的文件 (可选)
      --limit int           每页返回数量 (默认 20，最大 50)，仅 spaceType 为 orgSpace 时有效
      --cursor string       分页游标，仅企业空间支持分页 (可选)
```

spaceType 筛选规则：
- `orgSpace`（默认/不传）：返回企业空间列表，支持 `nextToken` 分页
- `mySpace`：返回用户的"我的文件"个人空间（单个，不支持分页）

返回字段说明：
- `spaceId` — 空间 ID，用于 `list`/`info`/`upload` 等命令的 `--space-id`
- `spaceName` — 空间名称（如"全员文件夹"、"我的文件"）
- `rootFolderId` — 空间根目录的 dentryUuid，可作为 `drive copy/move` 的 `--folder` 参数
- `spaceType` — 空间类型（如 `orgSpace`）
- `nextToken` — 若不为空，表示还有更多空间可查询（仅企业空间）

### 搜索钉盘文件/文件夹/空间

按关键词全局搜索文件，默认同时搜索钉盘和文档空间，合并返回结果。不同于 `list`（需要明确的 spaceId/folder 逐层遍历），`search` 用于不知道具体位置、只记得名称/关键词的场景。

```
Usage:
  dws drive search [flags]
Example:
  dws drive search --query "季度汇报"
  dws drive search --query "合同" --target file --extensions pdf,docx
  dws drive search --query "项目" --target space
  dws drive search --query "方案" --created-from 1700000000000 --created-to 1710000000000
  dws drive search --query "周报" --creator-uids 012345
  dws drive search --query "报告" --limit 30 --cursor <pageToken>
Flags:
      --query string           搜索关键词 (必填)
      --target string          搜索范围: all(默认,聚合钉盘+文档空间) | file(仅钉盘文件) | space(仅钉盘空间) (可选)
      --file-types strings     按文件内容类型过滤，逗号分隔: alidoc,document,image,video,audio,archive (仅 target=file/all 生效)
      --extensions strings     按文件扩展名过滤，不含点号，逗号分隔 (如 pdf,docx,adoc)
      --creator-uids strings   按创建者用户 ID 过滤，逗号分隔
      --created-from int       创建时间起始 (毫秒时间戳，含)
      --created-to int         创建时间截止 (毫秒时间戳，含)
      --modified-from int      修改时间起始 (毫秒时间戳，含)
      --modified-to int        修改时间截止 (毫秒时间戳，含)
      --limit int              每页返回数量（默认 10，最大 30）
      --cursor string          分页游标，从上次返回的 nextCursor 获取 (可选)
```

搜索范围 (`--target`) 选择规则：
- `all`（默认）：同时搜钉盘文件与文档空间，聚合返回 — 不确定目标位置时使用
- `file`：只搜钉盘文件 / 文件夹，支持 `--file-types` / `--extensions` 过滤 — 明确是找钉盘文件时使用
- `space`：只搜钉盘团队空间 — 明确知道空间名、需快速定位空间 spaceId/rootFolderId 时使用

结果中 `source` 字段区分来源：`drive` / `doc`。如果需要在某个知识库内搜索，请使用 `dws wiki node search --workspace <workspaceId>`。

> **提示**：结果按相关性排序，首页未命中时优先调整关键词 / 补充 `--file-types`/`--extensions` 缩小范围 / 加上时间范围，而非反复翻页。

### 获取最近访问/编辑的文档列表

```
Usage:
  dws drive recent [flags]
Example:
  dws drive recent
  dws drive recent --operate-type 1
  dws drive recent --creator-type 1 --limit 10
  dws drive recent --file-types 0,1 --operate-type 0
Flags:
      --file-types ints     按文档类型过滤，逗号分隔 (参考 RecentAccessType 枚举) (可选)
      --operate-type ints   按操作类型过滤: 0=最近访问(默认), 1=最近编辑; 不传默认仅返回最近访问(0) (可选)
      --creator-type int    按创建人过滤: 0=全部(默认), 1=我创建, 2=他人创建 (可选)
      --org-ids ints        按资源所属组织 ID 过滤，逗号分隔 (可选)
      --limit int           每页数量 (默认 20，最大 20) (可选)
      --cursor string       分页游标，从上次返回的 nextCursor 获取 (可选)
```

返回字段说明：
- `recentItems[]` — 最近访问/编辑的文档列表
  - `nodeId` — 文档节点 ID，可用于 `doc read/info/update` 的 `--node`
  - `name` — 文档名称
  - `contentType` — 内容类型（如 ALIDOC）
  - `extension` — 扩展名（如 adoc、axls、able）
  - `docUrl` — 文档在线访问 URL
  - `operateType` — 操作类型：LAUNCH=访问，EDIT=编辑
  - `accessTime` — 最近访问时间
  - `createTime` / `updateTime` — 创建/更新时间
- `nextCursor` — 翻页游标，传入 `--cursor` 获取下一页
- `hasMore` — 是否还有更多数据

### 获取文件元数据信息

```
Usage:
  dws drive info [flags]
Example:
  dws drive info --node <dentryUuid>
Flags:
      --node string       节点 ID (dentryUuid) (必填)
      --space-id string   节点所属空间 ID (可选)
```

### 获取节点统计信息

```text
Usage:
  dws drive stats --node <NODE_ID_OR_URL>
```

返回节点可用的阅读、编辑、评论、点赞、预览或下载等统计维度；不同文件类型返回字段可能不同。本命令只读。

### 普通文件全局评论

Drive 评论只用于 PDF、DOCX、图片、压缩包等普通文件，并且固定为文件级全局评论。在线文档（adoc）的正文/划词评论使用 `dws doc comment`，在线表格（axls）的评论使用 `dws sheet comment`。

> `drive comment list/create` 是旧评论服务的兼容入口，保留原参数与输出但已 deprecated。新任务必须使用 `list-v2/create-v2`；其返回的 `commentKey` 才能用于下面的新生命周期命令。

```text
# 查询与创建
dws drive comment list-v2 --node <NODE_ID_OR_URL> [--limit 50] [--cursor <NEXT_TOKEN>] [--resolve-status <resolved|unresolved>]
dws drive comment create-v2 --node <NODE_ID_OR_URL> --content "评论内容"

# 回复、表态与回复列表
dws drive comment reply --node <NODE_ID_OR_URL> --comment-key <COMMENT_KEY> --content "回复内容"
dws drive comment react-reply --node <NODE_ID_OR_URL> --comment-key <COMMENT_KEY> --reaction <EMOJI>
dws drive comment list-replies --node <NODE_ID_OR_URL> --comment-key <COMMENT_KEY> [--page-size 20] [--page-token <NEXT_TOKEN>]

# 更新、解决、恢复与删除
dws drive comment update --node <NODE_ID_OR_URL> --comment-key <COMMENT_KEY> --content "更新后的内容"
dws drive comment resolve --node <NODE_ID_OR_URL> --comment-key <COMMENT_KEY>
dws drive comment restore --node <NODE_ID_OR_URL> --comment-key <COMMENT_KEY>
dws drive comment delete --node <NODE_ID_OR_URL> --comment-key <COMMENT_KEY>

# 批量查询；可重复传入 comment-key
dws drive comment batch-query --node <NODE_ID_OR_URL> --comment-key <COMMENT_KEY_1> --comment-key <COMMENT_KEY_2>
```

- 完整生命周期使用 `commentKey` 作为评论标识；创建后必须保存返回的 `commentKey`。
- 新评论列表的 `--limit/--page-size` 范围为 1–50；超过上限会直接报错，不会静默截断。
- Drive 自动把评论主题固定为 `global`，不要传 `topic-id`、单元格、正文锚点或行内范围。
- `--cursor` 是不透明字符串，只能原样使用上次响应返回的 `nextToken`。
- `delete` 是破坏性操作，必须得到用户确认；其余写操作遵循统一写入安全策略。

### 创建节点快捷方式

```text
Usage:
  dws drive shortcut --node <SOURCE_NODE> [--folder <TARGET_FOLDER>] [--workspace <WORKSPACE_ID>]
Example:
  dws drive shortcut --node <SOURCE_NODE>
  dws drive shortcut --node <SOURCE_NODE> --folder <TARGET_FOLDER>
  dws drive shortcut --node <SOURCE_NODE> --workspace <WORKSPACE_ID>
```

`--folder` 和 `--workspace` 均可省略，此时由服务端选择默认位置。创建后应通过 `drive list` 回读目标位置。

### 文件内容获取路由规则

> 当用户请求"分析/查看/读取某个钉盘文件内容"时，**必须先调用 `dws drive info` 获取文件元数据**，再根据返回的 `extension` 字段选择对应链路。
> 注意：若检测到钉钉文档类型（adoc/axls/amind/adraw），会自动跟进调用 `doc info` 返回更准确的文档信息。

| extension | 文件类型 | 操作 | 命令 |
|-----------|---------|------|------|
| adoc | 在线文档 | 在线获取 Markdown 内容 | `dws doc read --node <nodeId>` |
| axls | 在线表格 | 在线读取表格数据 | `dws sheet list` → `dws sheet range read` |
| able | 多维表格 | 在线查询记录 | `dws aitable base get` → `dws aitable record query` |
| 其他（pdf/docx/txt/png 等） | 普通文件 | **不支持在线分析**，需用户主动下载后本地查看 | `dws drive download --node <nodeId> --output <path>` |

### 下载钉盘文件到本地

下载流程一步到位：获取下载 URL → HTTP GET 下载文件二进制内容到本地。

```
Usage:
  dws drive download [flags]
Example:
  dws drive download --node <dentryUuid> --output ./report.pdf
  dws drive download --node <dentryUuid> --output ~/downloads/
  dws drive download --node <dentryUuid> --output ./big.zip --part-size 32MB --parallel 8
Flags:
      --node string       文件 ID (dentryUuid) (必填)
      --output string     本地保存路径 (必填)，可以是文件路径或目录；如果指定目录，文件名从下载 URL 中自动推断
      --space-id string   文件所属空间 ID (可选)
      --part-size string  分片下载的分片大小，支持 KB/MB/GB 单位，范围 1MB-1GB (默认 16MB)
      --parallel int      分片下载并发数，范围 1-8 (默认 4)
      --no-resume         关闭断点续传，忽略历史下载进度从头下载 (默认开启续传)
```

> **注意**：`--output` 是必填参数，不传会报错。

> **大文件分片下载**：
> - 大文件自动分片并发下载，小文件整流下载，行为对用户透明，无需任何额外操作。
> - 断点续传默认开启：下载中断后重跑同一命令会自动跳过已完成部分继续下载（`<目标文件>.dwspart` 为临时进度文件，下载完成后自动清理）；不需要续传时加 `--no-resume`。
> - 下载凭证过期会自动刷新并继续下载，已完成的部分不会重下；单个分片失败会自动重试，无需手动处理。

### 创建文件夹

```
Usage:
  dws drive mkdir [flags]
Example:
  dws drive mkdir --name "项目资料"
  dws drive mkdir --name "子目录" --folder <dentryUuid>
Flags:
      --name string       文件夹名称，最长 50 字符 (必填)
      --folder string     父节点 ID (dentryUuid)，不传则在空间根目录下创建 (可选)
      --space-id string   目标空间 ID，不传则使用「我的文件」 (可选)
```

> `mkdir` 在钉盘空间创建文件夹；要在文档空间/知识库中创建文件夹，用 `dws wiki node create --type folder --workspace <ID>`（见 [wiki.md](./wiki.md)）。

### 上传本地文件到钉盘

> **注意：** 上传文件首选 `dws drive upload` 一条命令（内部自动完成三步流程），不要手动走 `upload-info` + `curl` + `commit` 三步。

```
Usage:
  dws drive upload [flags]
Example:
  dws drive upload --file ./report.pdf
  dws drive upload --file ./slides.pptx --file-name "Q1汇报.pptx"
  dws drive upload --file ./data.xlsx --folder <dentryUuid>
  dws drive upload --file ./updated.md --node <fileId>
  dws drive upload --file ./updated.md --node <nodeId> --workspace <workspaceId> --yes
Flags:
      --file string        本地文件路径 (必填)
      --file-name string   文件显示名称 (默认使用文件名)
      --folder string      父节点 ID，不传则上传到空间根目录 (可选，与 --node 互斥)
      --node string        覆盖目标文件 ID，传入即覆盖已有文件 (可选，与 --folder 互斥)
      --space-id string    目标钉盘空间 ID，不传则使用「我的文件」 (可选)
      --workspace string   目标知识库 ID，传入时路由到文档空间上传 (可选)
      --convert            是否转换为钉钉在线文档 (仅文档空间上传时生效)
      --mime-type string   文件 MIME 类型，不传则自动推断 (可选)
```

`upload` 命令内部自动完成三步流程（获取凭证 → OSS PUT → 提交入库），无需手动分步操作。上传到知识库/文档空间时加 `--workspace` 参数。

传 `--node` 时改为覆盖指定文件：钉盘路由映射到已有 fileId，知识库路由映射到已有 nodeId。覆盖不可逆，默认要求确认；自动化场景必须先获得用户确认，再追加全局 `--yes`。可先用全局 `--dry-run` 预览操作，不会上传或提交文件。`--node` 与创建新文件所用的 `--folder` 互斥。

### 获取上传凭证 (手动三步·仅特殊场景)

> 仅当需要自定义流式上传、无法使用 `upload` 一条命令时才走手动三步：`upload-info` → HTTP PUT → `commit`。

```
Usage:
  dws drive upload-info [flags]
Example:
  dws drive upload-info --file-name "report.pdf" --file-size 102400
  dws drive upload-info --file-name "slides.pptx" --file-size 512000 --folder <dentryUuid>
Flags:
      --file-name string   文件名含后缀 (必填)
      --file-size int      文件大小，单位字节 (必填)
      --mime-type string   MIME 类型 (可选，服务端会自动推断)
      --folder string      父节点 ID (dentryUuid)，不传则上传到空间根目录 (可选)
      --space-id string    目标空间 ID，不传则使用「我的文件」 (可选)
```

### 提交上传 (手动三步·第三步)

```
Usage:
  dws drive commit [flags]
Example:
  dws drive commit --file-name "report.pdf" --file-size 102400 --upload-id <UPLOAD_ID>
Flags:
      --file-name string   文件名含后缀 (必填，须与 upload-info 一致)
      --file-size int      文件大小，单位字节 (必填，须与 upload-info 一致)
      --upload-id string   upload-info 返回的 uploadId (必填)
      --folder string      父节点 ID (dentryUuid)，须与 upload-info 一致 (可选)
      --space-id string    空间 ID，不传则使用「我的文件」 (可选)
```

### 删除文件/文件夹到回收站

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws drive delete [flags]
Example:
  dws drive delete --node <dentryUuid> --format json    # 查询 fileId: dws drive list
Flags:
      --node string   文件/文件夹 ID (dentryUuid)，即 drive list 返回的 fileId (必填)

Global Flags:
      --yes   跳过二次确认 (危险操作，建议先与用户确认)
```

> 由当前二进制静态注册（路由到 doc 服务的 `delete_document` tool）；如果当前版本未暴露，调用前可用 `dws drive delete --help` 验证。
注意：`--node` 使用的是 `drive list` 返回结果中的 `fileId` 字段（即 `dentryUuid`），**不是** `dentryId` 字段。
删除是软删除（进回收站），但仍需用户明确确认；不要在自动化脚本里默认带 `--yes`。

### 查看回收站文件列表

```
Usage:
  dws drive recycle list [flags]
Example:
  dws drive recycle list
  dws drive recycle list --space-id 12345 --limit 10
Flags:
      --space-id string    钉盘空间 ID (选填，不传则返回所有空间)
      --limit int          返回条数上限 (默认 20，最大 50)
      --cursor string      分页游标 (选填)
```

### 还原回收站文件

```
Usage:
  dws drive recycle restore [flags]
Example:
  dws drive recycle restore --id <recycleItemId>
Flags:
      --id string    回收项 ID (必填，从 recycle list 获取)
```

> **注意**：还原操作可能是异步的（返回 `async=true` 和 `taskId`）。

### 比较本地文件夹与钉盘文件夹的差异

只读命令：比较本地文件夹与钉盘文件夹的差异——本地取 `--local-folder`（**绝对路径**），钉盘取 `--remote-folder`（文件夹 dentryUuid，**必传**）指向的文件夹，按精确 MD5（默认）或快速 modified_time（`--quick`）逐文件比对。两侧各自递归遍历，`rel_path` 相对各自根目录。

```
Usage:
  dws drive status [flags]
Example:
  dws drive status --local-folder /abs/path/repo --remote-folder <dentryUuid>
  dws drive status --local-folder /abs/path/repo --remote-folder <dentryUuid> --space-id xxxx
  dws drive status --local-folder /abs/path/repo --remote-folder <dentryUuid> --quick
Flags:
      --local-folder string   本地文件夹绝对路径 (必填)
      --remote-folder string    钉盘文件夹 ID (dentryUuid) (必填)
      --space-id string         钉盘空间 ID，不传则使用「我的文件」(可选)
      --quick                   快速模式：只比较 modified_time，不计算 MD5 (可选)
```

输出五类差异（`rel_path` 始终以 `/` 分隔、相对各自根目录）：

| 字段 | 含义 |
|------|------|
| `new_local` | 仅本地存在 |
| `new_remote` | 仅钉盘存在 |
| `modified` | 两侧都存在且本次检测判定为已变更（exact 比 MD5，quick 比 modified_time） |
| `unchanged` | 两侧都存在且本次检测判定为未变更 |
| `unknown` | 两侧都存在，但 exact 模式下**远端未返回可靠 MD5**、无法核对内容——既不判 unchanged 也不判 modified，如实归入此类（quick 模式不会产生 unknown） |

输出 schema：

```json
{
  "detection": "exact",
  "new_local":  [{"rel_path": "..."}],
  "new_remote": [{"rel_path": "..."}],
  "modified":   [{"rel_path": "..."}],
  "unchanged":  [{"rel_path": "..."}],
  "unknown":    [{"rel_path": "..."}]
}
```

注意事项：

- 默认 `detection=exact`（比较 MD5）；传 `--quick` 后 `detection=quick`（只比较 modified_time，best-effort）。
- exact 模式**只在能拿到远端 MD5 时才判定 unchanged/modified**；远端缺失 MD5 的文件一律进入 `unknown`，绝不会因大小 / mtime 恰好相同而被误报为 unchanged。当前 `list_files` 通常不返回 MD5，因此这类文件多会落在 `unknown`——请据此决定是否用 `pull`/`push` 强制对齐。
- 本地 hash 仅在文件双端都存在、远端有 MD5、且非 `--quick` 模式时才按需计算。
- 远端文件或文件夹名称若无法安全、无歧义地映射到本地路径（如 `..`、路径分隔符、盘符或目标平台保留名），命令会中止整棵远端树并返回失败；不会静默跳过后继续报告不完整结果。
- 只比对钉盘 `type=file` 的二进制文件；在线文档（docx/sheet/bitable/mindnote/slides）与快捷方式（shortcut）会被跳过。本地只比对常规文件（符号链接、设备文件忽略）。
- `--local-folder` 必须是绝对路径（相对路径会被直接拒绝）；`--remote-folder` 必传，是钉盘侧待比对文件夹的 dentryUuid（可用 `dws drive list` 查到）。

### 把钉盘文件夹拉取（镜像）到本地

只写本地命令：把 `--remote-folder` 指向的钉盘文件夹**单向、文件级**镜像到本地 `--local-folder`（Drive → 本地）。递归下载所有 `type=file` 的文件，子目录自动创建。**执行前必须获得用户确认；非交互环境先用 `--dry-run` 预览，确认后再加 `--yes`。**

```
Usage:
  dws drive pull [flags]
Example:
  dws drive pull --local-folder /abs/path/repo --remote-folder <dentryUuid>
  dws drive pull --local-folder /abs/path/repo --remote-folder <dentryUuid> --if-exists smart
  dws drive pull --local-folder /abs/path/repo --remote-folder <dentryUuid> --space-id xxxx
Flags:
      --local-folder string   本地文件夹绝对路径 (必填)
      --remote-folder string    钉盘文件夹 ID (dentryUuid) (必填)
      --space-id string         钉盘空间 ID，不传则使用「我的文件」(可选)
      --if-exists string        本地文件已存在时的策略: skip|smart|overwrite (默认 skip；命令写本地，执行需确认)
```

`--if-exists` 策略：

| 值 | 行为 |
|----|------|
| `skip`（默认） | 本地已存在则保持不动，只新增 |
| `smart`（推荐增量同步） | 本地 `modified_time` 已 ≥ 远端 `modified_time` 则跳过；时间戳缺失/非法时退回安全路径继续下载 |
| `overwrite` | 总是下载覆盖（Drive 作为权威源） |

输出 schema：

```json
{
  "summary": {"downloaded": 0, "skipped": 0, "failed": 0},
  "items": [
    {"rel_path": "sub/a.txt", "action": "downloaded"},
    {"rel_path": "b.txt", "action": "skipped"},
    {"rel_path": "c.bin", "action": "failed", "error": "..."}
  ]
}
```

注意事项：

- 只下载钉盘 `type=file` 的二进制文件；在线文档与快捷方式会被跳过。`rel_path` 始终以 `/` 分隔。
- 下载目标始终被约束在 `--local-folder` 之内：远端名称含 `..`、路径分隔符、盘符或目标平台保留名等不可安全映射成分时，命令会在下载前中止整棵远端树；拼接后仍逃逸出根目录的路径记为 `failed`、不会落盘。
- 镜像采用跨平台一致的路径等价规则：远端树中若出现 `A/a`、Unicode NFC/NFD 异写，或等价目录前缀下的不同子树，会在任何下载前整批失败，避免不同文件系统得到不一致结果。
- 下载成功后本地文件 mtime 会对齐到远端 `modified_time`，便于后续 `--if-exists smart` 增量同步跳过。
- `summary.failed > 0` 时命令以**非零退出码**退出；结构化 `summary + items` 仍打印在 stdout 上，stderr 只保留简短失败说明。脚本/agent 直接看 exit code 即可判断成败。

### 把本地文件夹推送（镜像）到钉盘

只写远端命令：把本地 `--local-folder` **单向、文件级**镜像到钉盘 `--remote-folder` 文件夹（本地 → Drive）。递归遍历本地文件与子目录（含空目录），缺失的远端目录按需创建（已存在则复用、不重建），文件按 `--if-exists` 新建/覆盖/跳过。**执行前必须获得用户确认；非交互环境先用 `--dry-run` 预览，确认后再加 `--yes`。只新增/覆盖，不删除远端多余文件。**

```
Usage:
  dws drive push [flags]
Example:
  dws drive push --local-folder /abs/path/repo --remote-folder <dentryUuid>
  dws drive push --local-folder /abs/path/repo --remote-folder <dentryUuid> --if-exists smart
  dws drive push --local-folder /abs/path/repo --remote-folder <dentryUuid> --if-exists overwrite
Flags:
      --local-folder string   本地文件夹绝对路径 (必填)
      --remote-folder string    钉盘目标文件夹 ID (dentryUuid) (必填)
      --space-id string         钉盘空间 ID，不传则使用「我的文件」(可选)
      --if-exists string        远端文件已存在时的策略: skip|smart|overwrite (默认 skip；命令写钉盘，执行需确认)
```

`--if-exists` 策略（与 pull 一样默认 `skip`，避免未显式选择时覆盖既有文件）：

| 值 | 行为 |
|----|------|
| `skip`（默认） | 远端已存在则保持不动，只新增 |
| `smart` | 增量同步：远端 `modified_time` 已 ≥ 本地则跳过，否则走覆盖路径 |
| `overwrite` | 覆盖远端同名文件（原地覆盖，保留 fileId，不产生重名副本） |

输出 schema（`action`：`uploaded` / `overwritten` / `skipped` / `folder_created` / `failed`）：

```json
{
  "summary": {"uploaded": 0, "skipped": 0, "failed": 0, "aborted": false},
  "items": [
    {"rel_path": "sub", "action": "folder_created"},
    {"rel_path": "a.txt", "action": "uploaded", "size_bytes": 11},
    {"rel_path": "b.txt", "action": "overwritten", "size_bytes": 8},
    {"rel_path": "c.txt", "action": "skipped", "size_bytes": 5},
    {"rel_path": "d.bin", "action": "failed", "size_bytes": 0, "error": "..."}
  ]
}
```

注意事项：

- 只上传/覆盖 `type=file`；`summary.uploaded` 同时统计新建与覆盖，**不含目录**。
- `overwrite` / `smart` 命中覆盖分支时走**覆盖上传**（`get_upload_info` 与 `commit_upload` 两阶段都携带远端 `overwriteFileId`、不传 `parentId`），在原文件上原地覆盖、保留 fileId，不会在同目录新建重名副本。
- 本地子目录（含空目录）整体镜像：缺失的按需 `create_folder`（以 `folder_created` 留痕），已存在的远端目录复用其 fileId、不重建、不出现在 `items[]`。
- 本地名称若含反斜杠、控制字符等无法安全映射到钉盘的成分，或双端存在 `A/a`、Unicode NFC/NFD、等价祖先前缀或文件/目录类型歧义，命令会在任何创建或上传前整批失败；不会只跳过冲突项后继续写入。
- `summary.failed > 0` 时命令以**非零退出码**退出；结构化 `summary + items` 仍打印在 stdout 上，脚本/agent 直接看 exit code 判断成败。

### 本地文件夹与钉盘文件夹双向同步

读写命令：把本地 `--local-folder` 与钉盘 `--remote-folder` 做**文件级双向同步**。**这是写操作，非交互环境下必须显式加 `--yes`；先用 `--dry-run` 看清将发生什么。**先按 `status` 同源逻辑算出五类差异，再分别处理：`new_remote` 下载到本地、`new_local` 上传到钉盘、两侧都变更的 `modified` 按 `--on-conflict` 策略消解；`unchanged` 与 `unknown` 一律跳过、不动。**只新增/覆盖，两侧都不删除多余文件。**

```
Usage:
  dws drive sync [flags]
Example:
  dws drive sync --local-folder /abs/path/repo --remote-folder <dentryUuid>
  dws drive sync --local-folder /abs/path/repo --remote-folder <dentryUuid> --on-conflict local-wins
  dws drive sync --local-folder /abs/path/repo --remote-folder <dentryUuid> --on-conflict keep-both
  dws drive sync --local-folder /abs/path/repo --remote-folder <dentryUuid> --quick
Flags:
      --local-folder string    本地文件夹绝对路径 (必填)
      --remote-folder string   钉盘文件夹 ID (dentryUuid) (必填)
      --space-id string        钉盘空间 ID，不传则使用「我的文件」(可选)
      --on-conflict string     两侧都变更时的策略: skip|remote-wins|local-wins|keep-both|ask (默认 skip；命令写双端，执行需确认)
      --quick                  快速模式：只比较 modified_time，不计算 MD5 (可选)
```

`--on-conflict` 仅作用于 `modified`（两侧都存在且都变更）的文件：

| 值 | 行为 |
|----|------|
| `skip`（默认） | 两侧都不动，两边内容都保留，计入 `skipped` |
| `remote-wins` | 下载远端覆盖本地（需 `--yes`） |
| `local-wins` | 覆盖上传本地到远端（原地覆盖、保留 fileId；需 `--yes`） |
| `keep-both` | 先在同一目录以不覆盖的原子硬链接保留本地副本（`名.conflict-<fileId 末 8 位>.扩展名`），再把远端拉到原名；拉取失败时原文件与候选副本都保留并报告失败，不做可能误伤并发文件的回滚 |
| `ask` | 逐个交互询问；`--dry-run` 或非交互环境下等价于跳过 |

输出 schema（`action`：`downloaded` / `uploaded` / `overwritten` / `folder_created` / `renamed_local` / `skipped` / `failed`；其中 `renamed_local` 是兼容动作名，表示已成功保留本地冲突副本；`direction`：`pull` / `push` / `conflict`）：

```json
{
  "detection": "exact",
  "diff": {
    "new_local":  [{"rel_path": "a.txt"}],
    "new_remote": [{"rel_path": "b.txt"}],
    "modified":   [{"rel_path": "c.txt"}],
    "unchanged":  [],
    "unknown":    []
  },
  "summary": {"pulled": 1, "pushed": 1, "skipped": 0, "failed": 0},
  "items": [
    {"rel_path": "b.txt", "action": "downloaded", "direction": "pull"},
    {"rel_path": "a.txt", "action": "uploaded", "direction": "push"},
    {"rel_path": "c.txt", "action": "overwritten", "direction": "conflict"}
  ]
}
```

注意事项：

- 复用 `status`/`pull`/`push` 的全部安全约束：只处理 `type=file`（在线文档、快捷方式跳过）；远端名称含 `..`、路径分隔符、盘符或目标平台保留名等不可安全映射成分时会在任何同步写入前中止整棵远端树，拼接后逃逸出 `--local-folder` 的路径记为 `failed` 不落盘；下载走「先写临时文件、成功才原子 rename」，失败绝不破坏本地原文件。
- `--dry-run` 只算差异并输出独立 JSON 预览对象，不触发任何下载/上传/改名/落盘；差异位于顶层预览对象的 `plan.diff`（同时包含 `dry_run=true`、`executed=false` 与 `preview_kind=plan`）。
- 双端存在 `A/a`、Unicode NFC/NFD、等价祖先前缀或文件/目录类型歧义时，`sync` 会在任何一侧写入前整批失败；本地无法安全映射到钉盘的名称同样 fail-closed。
- `unknown`（exact 模式远端无可靠 MD5）一律计入 `skipped`、不做任何写操作；需要强制对齐时改用单向的 `pull`/`push`。
- `summary.failed > 0` 时命令以**非零退出码**退出，结构化结果仍打印在 stdout 上。

## 意图判断

用户说"我的文件/钉盘/网盘/云盘" → `list`
用户说"最近访问/最近打开/最近编辑/最近文档" → `recent`（默认仅最近访问，`--operate-type 1` 仅最近编辑，`--operate-type 0,1` 全部）
用户说"钉盘空间/团队文件/有哪些空间/空间列表/团队文件列表" → `wiki space list --type orgSpace`（`drive list-spaces` 已 deprecated）
用户说"搜索钉盘文件/钉盘里找个文件/查找某个钉盘文件/钉盘中搜索" → `search`
用户说"文件详情/文件信息" → `info`
用户说"文件阅读量/编辑量/评论数/下载数/节点统计" → `stats`
用户说"给这个 PDF/附件/普通文件评论、回复评论、解决评论、恢复评论、删除评论" → `comment list-v2/create-v2/reply/update/delete/batch-query/list-replies/resolve/restore/react-reply`（仅在用户明确要求旧评论兼容行为时使用 deprecated 的 `list/create`）
用户说"给文件创建快捷方式/放一个链接到目标文件夹" → `shortcut`
用户说"下载文件" → `download` 指定 `--output` 保存到本地
用户说"新建文件夹/创建目录" → `mkdir`（钉盘空间）/ `wiki node create --type folder`（文档空间）
用户说"上传文件/传文件到钉盘" → `upload`（首选此命令，自动完成三步流程）
用户说"覆盖/替换钉盘或知识库中的已有文件" → `upload --node <fileId>`（不可逆，先 `--dry-run`，确认后再加 `--yes`）
用户说"复制文件/移动文件/搬到/移到" → `copy` / `move`
用户说"重命名/改名" → `rename`
用户说"删除文件/删除文件夹/移到回收站" → `delete`（危险操作，需确认）
用户说"回收站/查看回收站/回收站列表/回收站里有什么" → `recycle list`
用户说"恢复文件/还原删除的文件/从回收站恢复/还原回收站文件" → `recycle restore`
用户说"给文档授权/分享权限" → `permission add`（协作者级授权；链接公开的访问密码/有效期走 `publish set`）
用户说"授权并通知对方/加权限后告知他/通知一下被授权的人" → `permission add --members ... --notify`（未提通知需求时不传 `--notify`）
用户说"权限设置/权限模式/分享范围/水印等策略配置" → `permission get-setting`
用户说"公开文件/互联网公开/设置公开/让互联网所有人可访问/设置访问密码/公开有效期/分享链接密码" → `publish set`
用户说"关闭公开/取消公开/取消互联网访问" → `publish unset`
用户说"查看公开状态/是否公开/发布状态" → `publish get`
用户说"比较本地和云盘/看哪些文件变了/同步差异/diff" → `status`
用户说"把钉盘文件夹拉到本地/下载整个文件夹/镜像/同步到本地/pull" → `pull`
用户说"把本地文件夹传到钉盘/推送整个文件夹/上传目录/同步到云端/push" → `push`
用户说"双向同步/两边同步/本地和云盘互相同步/让两边一致/sync" → `sync`（默认两侧都变更时跳过；要覆盖须显式给 `--on-conflict` 并加 `--yes`）
用户说"存储容量/企业盘用量/剩余空间/用了多少空间" → `quota`（默认企业级；应用列表用 `quota apps`），完整规则见 [`drive-storage.md`](./drive/drive-storage.md)
用户说"异步任务/任务状态/任务查询/导出结果查询" → `task get`（统一入口，`--type export|import|copy|move`），完整规则见 [`drive-task.md`](./drive/drive-task.md)
用户说"导出为 xlsx/pptx"或不确定文档类型 → `export`（通用导出入口，自动识别类型），完整规则见 [`drive-export.md`](./drive/drive-export.md)

关键区分: drive(文件管理) vs doc(文档内容读写) vs wiki(空间管理)

**drive search vs wiki node search**: 用户提到"钉盘/网盘/我的文件里搜" → `drive search`；提到"知识库/文档空间/workspace 里搜" → `wiki node search`；未明确目标时 `drive search`（全局聚合搜索）。

**drive upload vs doc upload**: 文件上传统一走 `drive upload`。上传到知识库/文档空间时加 `--workspace` 参数。

**drive permission vs wiki member**: "给某篇文档/文件授权" → `drive permission add`（节点级）；"给某个知识库整体加成员" → `wiki member add`（空间级）

**通知意图 → `--notify`**（默认不通知，省略时 CLI 不向服务端发送该字段）：
- 用户明确要求“通知 / 告知 / 提醒对方 / 让他知道” → 追加 `--notify`
- 用户明确要求“不要通知 / 别提醒 / 悄悄加 / 不要打扰” → 追加 `--notify=false`
- 用户没提通知需求 → **不传该 flag**，保持不通知；不要自行补上 `--notify`
- `--notify` 仅在 `--members` 新格式下生效；旧格式 `--users` 下传了也不会生效，有通知需求必须改用 `--members`
- 仅 USER 和 CONVERSATION 类型成员会收到通知；被授权对象是 DEPT / TAG 时通知不会送达，**需主动向用户说明这一点**，不要默不作声

**创建在线文档/表格/脑图**: drive 不支持创建文件，需走 `wiki node create --type <type>`（创建空节点）或 `doc create`（创建并写入内容）。

**导出文档/导出为Word**: 钉盘在线文档（存储在钉盘里的文档）的导出走 `drive export`；文档内容层操作走 `doc export`。

把图片/文件发到群里一般直接用 `chat message send --msg-type file --file <本地路径>`（见 [chat.md](./chat.md)），无需先经 drive 上传。

## 核心工作流

```bash
# 1. 浏览「我的文件」根目录
dws drive list --limit 20 --format json

# 2. 进入子目录 — 提取 dentryUuid 作为 folder
dws drive list --limit 20 --folder <dentryUuid> --format json

# 3. 查看文件元数据
dws drive info --node <dentryUuid> --format json

# 4. 下载文件到本地
dws drive download --node <dentryUuid> --output /tmp/ --format json

# 5. 创建文件夹
dws drive mkdir --name "项目资料" --format json

# 6. 上传文件（首选 upload 命令，自动完成三步流程）
dws drive upload --file ./报告.pdf --format json
dws drive upload --file ./报告.pdf --folder <dentryUuid> --format json

# 6b. 覆盖已有文件（先预览；用户确认后再执行）
dws drive upload --file ./更新版.pdf --node <fileId> --dry-run --format json
dws drive upload --file ./更新版.pdf --node <fileId> --yes --format json

# 7. 删除文件/文件夹到回收站（危险操作：必须先向用户确认，用户同意后才加 --yes 执行）
# 正确流程：1.向用户展示"即将删除「文件名」到回收站" → 2.等用户确认 → 3.执行下面命令
dws drive delete --node <dentryUuid> --yes --format json

# 8. 查看回收站并还原文件
dws drive recycle list --format json
dws drive recycle restore --id <recycleItemId> --format json

# 9. 比较本地文件夹与钉盘文件夹的差异（只读；remote-folder 必传，用 list 查 dentryUuid）
dws drive status --local-folder /abs/path/repo --remote-folder <dentryUuid> --format json
dws drive status --local-folder /abs/path/repo --remote-folder <dentryUuid> --space-id <spaceId> --format json
dws drive status --local-folder /abs/path/repo --remote-folder <dentryUuid> --quick --format json

# 10. 把钉盘文件夹镜像到本地（Drive → 本地；smart 为推荐的增量同步）
dws drive pull --local-folder /abs/path/repo --remote-folder <dentryUuid> --format json
dws drive pull --local-folder /abs/path/repo --remote-folder <dentryUuid> --if-exists smart --format json

# 11. 把本地文件夹镜像到钉盘（本地 → Drive；默认 skip 只新增，smart 增量，overwrite 覆盖）
dws drive push --local-folder /abs/path/repo --remote-folder <dentryUuid> --format json
dws drive push --local-folder /abs/path/repo --remote-folder <dentryUuid> --if-exists smart --format json

# 12. 本地与钉盘双向同步（默认 --on-conflict=skip 两侧都不动；要覆盖须显式选策略并加 --yes）
dws drive sync --local-folder /abs/path/repo --remote-folder <dentryUuid> --format json
dws drive sync --local-folder /abs/path/repo --remote-folder <dentryUuid> --on-conflict keep-both --format json
```

## 文档空间管理命令

> 以下命令操作的是**文档空间**（知识库 / 我的文档），底层路由到 doc 服务。
> 与钉盘命令（list / mkdir / upload 等）的区别：钉盘命令操作钉盘空间（spaceId 纯数字），文档空间命令操作知识库/我的文档（workspaceId 加密 string）。

### 复制/移动/重命名文件

```
Usage:
  dws drive copy --node <ID> [--folder <TARGET>] [--workspace <WS>]
  dws drive move --node <ID> [--folder <TARGET>] [--workspace <WS>]
  dws drive rename --node <ID> --name "新名称"
Flags:
      --node string        文档/文件 ID 或 URL (必填)
      --folder string      目标文件夹 nodeId
      --workspace string   目标知识库 ID
      --name string        新名称 (仅 rename 必填)
```

> **rename 会按真实节点元数据处理扩展名**：实际执行前先读取节点类型和当前扩展名。文件的新名称若以当前扩展名结尾，只去掉完全匹配的一层再交给会保留原后缀的服务端；文件夹（包括 `release.v2` 这类带点名称）保持原样，也不再依赖扩展名白名单。`--dry-run` 不读取远端元数据，因此预览保留输入名称。

权限要求：copy 需对源文档有"阅读"权限且对目标文件夹有"编辑"权限；move 需对源文档有"管理"权限且对目标文件夹有"编辑"权限；rename 需对文档有"编辑"权限。

> **字段选择**：`drive list` 返回中有 `dentryId`（数字格式）和 `fileId`（UUID 格式），**必须使用 `fileId`（UUID 格式）**作为 `--node` 和 `--folder` 参数值。

> **异步任务自动轮询**：服务端返回 `taskId` 时，copy/move 会自动轮询直至终态（渐进式退避：2s×5 → 5s×5 → 10s×10 → 15s×10，上限 30 次约 5 分钟）。轮询可随时 Ctrl-C 中断，服务端任务不会中止；超时或中断后用 `dws drive task get --type copy|move --id <taskId>` 查询兜底，任务状态枚举与查询入口区分详见 [`drive/drive-task.md`](./drive/drive-task.md)。`PARTIAL_FAILED` 时同样可用该命令查明细。

### 创建文件夹（文档空间）

drive 没有独立的文档空间建文件夹命令，在知识库/文档空间中创建文件夹走：

```bash
dws wiki node create --type folder --name "文件夹名" --workspace <WORKSPACE_ID> --format json
```

详见 [wiki.md](./wiki.md)。

### 权限管理（文档节点级）

> 仅适用于文档空间节点，不适用于钉盘文件。

```
Usage:
  dws drive permission add --node <ID> --users uid1,uid2 --role READER
  dws drive permission add --node <ID> --members '[{"type":"USER","id":"uid1","roleId":"READER","corpId":"xxx"},{"type":"TAG","id":"tagId1","roleId":"EDITOR","corpId":"xxx"}]' --notify
  dws drive permission update --node <ID> --users uid1 --role EDITOR
  dws drive permission update --node <ID> --members '[{"type":"USER","id":"uid1","roleId":"EDITOR","corpId":"xxx"}]' --notify=false
  dws drive permission update --node <ID> --members '[{"type":"CONVERSATION","id":"cidXXX","roleId":"READER"}]'
  dws drive permission list --node <ID>
  dws drive permission list --node <ID> --limit 50 --next-token <上次返回的 nextToken>
  dws drive permission get-setting --node <ID>
  dws drive permission remove --node <ID> --users uid1
  dws drive permission remove --node <ID> --members '[{"type":"USER","id":"uid1","corpId":"xxx"},{"type":"DEPT","id":"deptId1","corpId":"xxx"}]'
Flags:
      --node string          目标节点 ID 或 URL (必填)
      --users string         用户 userId 列表，逗号分隔 (旧格式)
      --role string          角色: MANAGER / EDITOR / DOWNLOADER / READER (旧格式必填)
      --members string       成员列表 JSON 数组（新格式），支持 USER/DEPT/CONVERSATION/TAG 类型（TAG=角色组），与 --users 互斥
      --notify bool          是否通知被添加/变更的成员 (仅 --members 新格式时生效，add / update 均默认 false)
      --limit int            返回成员数上限 (仅 list，默认 30，最大 50)
      --filter-role string   按角色过滤 (仅 list)
      --next-token string    分页游标，首次不传，后续传入上一次返回的 nextToken (仅 list)
      --workspace string     知识库 ID (选填)
```

> **add / update / remove 支持两种传参方式（互斥）**：
> - 旧格式：`--users` 传入逗号分隔的 userId 列表 + `--role` 指定统一角色（仅 USER 类型）
> - 新格式：`--members` 传入 JSON 数组，支持 USER/DEPT/CONVERSATION/TAG 四种成员类型，每个 member 携带独立 `roleId`（remove 只需 type 和 id，但 USER/DEPT/TAG 仍需 corpId）
>
> **成员类型说明**：
> - `USER` — 用户，id 为用户 userId，需携带 `corpId`（标识用户所属组织）
> - `DEPT` — 部门，id 为部门 ID，需携带 `corpId`（标识部门所属组织）
> - `CONVERSATION` — 群聊，id 为群聊 conversationId（cid 开头），无需 `corpId`
> - `TAG` — 角色标签（也称角色组），id 为角色标签 ID，需携带 `corpId`。当用户要求"添加角色组"或"添加角色标签"时使用此类型
>
> **重要约束**：
> - `--notify` 仅在新格式时生效，仅对 USER 和 CONVERSATION 类型成员发送通知（DEPT 和 TAG 不通知），add / update 均默认 false；省略时不会向服务端发送该字段，需要通知请显式传 `--notify`
> - 操作者须满足该节点配置的权限管理最低角色要求（默认 MANAGER，可配置为 EDITOR 等），权限不足返回 `forbidden.accessDenied`
> - 单次请求最多 30 个成员，超出请分批调用
> - list 命令底层一次性返回全量成员后在内存中按 pageSize 分页，当 `hasMore` 为 true 时，传入 `--next-token` 即可获取下一页

`get-setting` 返回节点权限配置（不是成员清单）：`permissionMode`（INHERITED 继承上级 / INDEPENDENT 独立管理）、`shareScope`（可见范围与链接分享设置）、`policies`（水印、组织外分享、添加成员门槛等策略列表）。查询协作者清单仍用 `permission list`。

get-setting 返回字段说明：
- `permissionMode` — INHERITED（继承上级）/ INDEPENDENT（独立管理），未知时为 null
- `shareScope` — `visibility`（PRIVATE/ORGANIZATION/PUBLIC）；`partnerIncluded`、`defaultRole`、`canSearch`、`canRecommend` 仅 ORGANIZATION 有意义；`linkShare`（仅开启链接分享时返回）：`requirePassword`（密码明文不返回）、`expireAt`/`expireDays`（未设置为 null）、`forCurrentNode`
- `policies[]` — 每项含 `code`（策略码）、`name`/`description`（中文名与值语义说明，随行必带）、`value`（当前值）、`disabledValues`（不可设置取值列表）、`allowedValues`（可设置值域，与 disabledValues 互斥）；未下发或不支持的策略不返回；`node_spread_scope` 仅文件夹类节点返回
- `disabledValues[]` — 每项含 `value`（被禁档位取值，与 value 同一值域）与 `reason`（服务端按请求语言返回的禁用原因文案，仅供展示理解，可为 null）；恒返回，无被禁档位时为空数组；示例：`{"value": "READER_AND_ABOVE", "reason": "企业安全策略要求不可低于可下载角色"}`
- `value` 按策略分型：开关型（external_share、external_share_manager_only、member_invite_org_only、permission_apply、external_permission_apply、watermark、node_move_forbidden）为 ENABLED/DISABLED；member_invite、comment 为 READER_AND_ABOVE/DOWNLOADER_AND_ABOVE/EDITOR_AND_ABOVE/MANAGER_AND_ABOVE（无 NOBODY）；node_spread、online_content_copy 为 DOWNLOADER_AND_ABOVE/EDITOR_AND_ABOVE/MANAGER_AND_ABOVE 或 NOBODY（无 READER_AND_ABOVE）；node_spread_scope 为 ALL_NODES（限制对所有文档生效）/ PREVIEWABLE_ONLY（仅对可预览的文档生效）
- `name`/`description` 示例（文案与产品权限设置页一致）：external_share「添加企业外协作者」：是否允许添加企业外的人为协作者（ENABLED=允许，DISABLED=禁止）；node_spread「谁可以下载、创建副本、打印」：允许哪些角色及以上的用户下载、创建副本、打印；NOBODY=所有人禁止下载、创建副本、打印；node_move_forbidden「禁止移动」：是否禁止移动到其他知识库或团队共享文件夹（ENABLED=禁止移动，DISABLED=允许移动）
- 方向语义：NOBODY=该操作对所有人禁止；XXX_AND_ABOVE=不低于该角色才允许

### 文件互联网公开发布

管理文件的互联网公开发布状态。公开后任何人通过链接即可访问，无需登录钉钉。操作者需要是文件的管理员或拥有者。

> **`publish set` 和 `publish unset` 为 [危险] 操作，执行前需要向用户确认。确认后传入 `--yes` 跳过交互式确认。**

```
Usage:
  dws drive publish set --node <fileId> [--permission READER|DOWNLOADER|EDITOR] [--password Ab12] [--expire-days N]
  dws drive publish unset --node <fileId>
  dws drive publish get --node <fileId>
Example:
  dws drive publish set --node <dentryUuid>
  dws drive publish set --node <dentryUuid> --permission READER
  dws drive publish set --node <dentryUuid> --password Ab12 --expire-days 7
  dws drive publish get --node <dentryUuid>
  dws drive publish unset --node <dentryUuid>
Flags:
      --node string         目标文件 ID (dentryUuid) 或 URL (必填)
      --permission string   公开后的权限: READER(仅可查看) / DOWNLOADER(可查看和下载，默认) / EDITOR(可编辑)，仅 set 有效
      --password string     公开访问密码: 4 位字母或数字 (如 Ab12)，仅 set 有效；显式传空串清除密码保护
      --expire-days int     公开有效期天数: 正整数=N 天后过期，0=永久有效，仅 set 有效
```

子命令说明：
- `publish set` — [危险] 设置文件为互联网公开，可选指定公开权限、访问密码与有效期
- `publish unset` — [危险] 关闭文件互联网公开
- `publish get` — 查询文件当前的公开发布状态

返回字段说明：
- `published` — true=已公开，false=未公开
- `publishPermission` — 当前公开权限（READER/DOWNLOADER/EDITOR）
- `pendingApproval` — true=已提交审批待生效，false/null=无需审批或已直接生效
- `docUrl` — 文件访问链接

> **注意**：导出钉盘在线文档到本地可使用 `dws drive export`（通用导出，支持 docx/xlsx/pptx/pdf/markdown），完整规则见 [`drive/drive-export.md`](./drive/drive-export.md)；`doc export` 与 `sheet export` 是分别针对在线文档与在线表格的产品级入口。
> 导出/复制/移动的自动轮询过程可随时用 Ctrl-C 中断；已提交的服务端任务不会中止，之后可用 `dws drive task get` 查询任务状态。

### 目标位置参数规则

| 目标位置 | 参数传递方式 | 前置步骤 |
|---------|-----------|---------|
| 未指定目标（默认） | `--folder <rootFolderId>` | 先 `dws wiki space list --type mySpace` 获取「我的文件」的 `rootFolderId` |
| 知识库空间根目录 | `--workspace <workspaceId>` | 无需额外步骤，直接传入 workspaceId |
| 钉盘 space 根目录 | `--folder <rootFolderId>` | 先 `dws wiki space list --type orgSpace` 获取目标 space 的 `rootFolderId` |
| 钉盘 space 下的子文件夹 | `--folder <fileId>` | 先 `dws drive list --space-id <spaceId>` 逐层浏览，获取目标文件夹的 `fileId`（dentryUuid 格式） |

### 工作流示例

```bash
# ── 场景默认: 用户未指定目标位置 → 复制/移动到「我的文件」根目录 ──
dws drive list --space-id <SPACE_ID> --format json                       # 获取源文件 dentryUuid
dws wiki space list --type mySpace --format json                         # 获取「我的文件」rootFolderId
dws drive copy --node <源文件dentryUuid> --folder <我的文件rootFolderId> --format json

# ── 场景 A: 复制钉盘文件到知识库空间根目录 ──
dws drive copy --node <源文件dentryUuid> --workspace <TARGET_WS_ID> --format json

# ── 场景 B: 移动钉盘文件到另一个钉盘 space 根目录 ──
dws wiki space list --type orgSpace --format json
dws drive move --node <源文件dentryUuid> --folder <目标space的rootFolderId> --format json
# 注意：移动到其他 space 时只传 --folder，不传 --workspace

# ── 场景 C: 复制钉盘文件到钉盘 space 下的子文件夹 ──
dws drive list --space-id <TARGET_SPACE_ID> --format json
dws drive copy --node <源文件dentryUuid> --folder <目标文件夹fileId> --format json

```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list` | **`fileId`**（UUID 格式，注意：不是 `dentryId`） | info / stats / shortcut / download / delete 的 --node；list / mkdir 的 --folder；`drive copy/move/shortcut` 的 --node 或 --folder |
| `list` | `spaceId` | info / download / mkdir / upload 的 --space-id |
| `list` | `nextCursor` | 下次 list 的 --cursor |
| `list-spaces` / `wiki space list` | `rootFolderId` | `drive copy/move` 的 --folder（复制/移动到钉盘 space 根目录时） |
| `list-spaces` / `wiki space list` | `spaceId` | list / info / download / mkdir / upload 的 --space-id |
| `search` | **`fileId`**（文件/文件夹结果） | info / download / delete 的 --node；list 的 --folder |
| `search` | `spaceId` / `rootFolderId`（空间结果） | list 的 --space-id；`drive copy/move` 的 --folder |
| `search` | `nextCursor` | search 的 --cursor（翻页） |
| `mkdir` | `fileId`（UUID 格式） | list / upload 的 --folder |
| `upload` | `dentryUuid` / `nodeId` | download / info / 后续 `upload --node` 覆盖 |
| `recycle list` | `id`（回收项 ID） | recycle restore 的 --id |
| `recycle list` | `name`（原始文件名） | 供用户确认还原目标 |
| `recent` | `recentItems[].nodeId` / `docUrl` | doc read / info / update / block 操作的 --node |
| `recent` | `nextCursor` | recent 的 --cursor（翻页） |

> **重要**：`drive list` 返回结果中同时包含 `dentryId` 和 `fileId` 两个字段。所有需要传 `--node` 的命令（info / download / delete）必须使用 `fileId`（即 dentryUuid），**不要使用** `dentryId`。

- `upload --node` 会覆盖已有文件且不可逆；`--node` 与 `--folder` 互斥。先 dry-run，得到用户明确确认后再加 `--yes`。

## 注意事项

- 不传 `--space-id` 时默认使用「我的文件」空间
- 不传 `--folder` 时默认操作空间根目录
- `--folder` 只能使用父文件夹的 `dentryUuid`。不要把 `drive info` 返回的数字型 `dentryId` 当作父目录
- **`--limit` 有效上限为 50**：CLI 不做本地校验，传 `--limit 100` 不会报错，但服务端每页最多返回 50 条，超出部分无效。用户要求超过 50 条时，应使用 `--limit 50` 配合 `--cursor` 分页查询
- `--order-by` 支持: `createTime`、`modifyTime`、`name`
- **上传文件首选 `dws drive upload` 命令**；手动三步（`upload-info` → HTTP PUT → `commit`）仅用于自定义流式上传等特殊场景。手动三步时 HTTP PUT 必须把 upload-info 返回的 `headers` 全部回传，`Content-Type` 通常要留空；PUT 返回 200 后才能调 `commit`；`uploadId` 有过期时间，过期需重新 `upload-info`；`--folder` 在 upload-info / commit 中要保持一致
- `--file-name` 必须包含扩展名（如 `report.pdf`）
- `download` 需要指定 `--output`，CLI 会把文件保存到本地路径或目录
- 文件名规则：头尾不能有空格；不能含 `*`、`"`、`<`、`>`、`|`、制表符；不能以 `.` 结尾
- `shortcut` 会创建新节点，执行后必须通过 `drive list` 回读确认目标位置；`stats` 为只读命令

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [drive_tree_list.py](../../scripts/drive_tree_list.py) | 递归列出钉盘目录树结构 | `python drive_tree_list.py --depth 2` |

## 相关产品

- [doc](./doc.md) — 文档内容读写（Markdown/块级编辑/导出），不是文件存储
- [markdown](./markdown.md) — 钉盘或文档空间中原生 `.md` 文件的读取、创建、覆盖与局部替换
- [wiki](./wiki.md) — 知识库/空间管理层（空间列表、节点创建、空间内搜索、成员管理）
- [chat](./chat.md) — 发送图片/文件消息用 `chat message send --msg-type file --file`
