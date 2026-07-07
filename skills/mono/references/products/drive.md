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
Flags:
      --limit int           每页返回数量，默认 20，最大 50 (可选)
      --cursor string       分页游标，首次不传 (可选)
      --order string        排序方向: asc|desc，默认 desc (可选，仅钉盘)
      --order-by string     排序字段: createTime|modifyTime|name (可选，仅钉盘)
      --folder string       父节点 ID (dentryUuid)，不传则列出空间根目录 (可选)
      --space-id string     钉盘空间 ID (纯数字)，不传则使用「我的文件」对应 spaceId (可选)
      --workspace string    文档空间/知识库 ID (加密 string 或 URL)，传入则路由到文档空间 (可选)
      --thumbnail           是否返回缩略图信息 (可选，仅钉盘)
```

> 统一入口：默认列钉盘空间（`--space-id` 纯数字）；传 `--workspace` 时路由到文档空间/知识库列表。

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
Flags:
      --node string       文件 ID (dentryUuid) (必填)
      --output string     本地保存路径 (必填)，可以是文件路径或目录；如果指定目录，文件名从下载 URL 中自动推断
      --space-id string   文件所属空间 ID (可选)
```

> **注意**：`--output` 是必填参数，不传会报错。

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
Flags:
      --file string        本地文件路径 (必填)
      --file-name string   文件显示名称 (默认使用文件名)
      --folder string      父节点 ID (dentryUuid)，不传则上传到空间根目录 (可选)
      --space-id string    目标钉盘空间 ID，不传则使用「我的文件」 (可选)
      --workspace string   目标知识库 ID，传入时路由到文档空间上传 (可选)
      --convert            是否转换为钉钉在线文档 (仅文档空间上传时生效)
      --mime-type string   文件 MIME 类型，不传则自动推断 (可选)
```

`upload` 命令内部自动完成三步流程（获取凭证 → OSS PUT → 提交入库），无需手动分步操作。上传到知识库/文档空间时加 `--workspace` 参数。

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

## 意图判断

用户说"我的文件/钉盘/网盘/云盘" → `list`
用户说"最近访问/最近打开/最近编辑/最近文档" → `recent`（默认仅最近访问，`--operate-type 1` 仅最近编辑，`--operate-type 0,1` 全部）
用户说"钉盘空间/团队文件/有哪些空间/空间列表/团队文件列表" → `wiki space list --type orgSpace`（`drive list-spaces` 已 deprecated）
用户说"搜索钉盘文件/钉盘里找个文件/查找某个钉盘文件/钉盘中搜索" → `search`
用户说"文件详情/文件信息" → `info`
用户说"下载文件" → `download` 指定 `--output` 保存到本地
用户说"新建文件夹/创建目录" → `mkdir`（钉盘空间）/ `wiki node create --type folder`（文档空间）
用户说"上传文件/传文件到钉盘" → `upload`（首选此命令，自动完成三步流程）
用户说"复制文件/移动文件/搬到/移到" → `copy` / `move`
用户说"重命名/改名" → `rename`
用户说"删除文件/删除文件夹/移到回收站" → `delete`（危险操作，需确认）
用户说"回收站/查看回收站/回收站列表/回收站里有什么" → `recycle list`
用户说"恢复文件/还原删除的文件/从回收站恢复/还原回收站文件" → `recycle restore`
用户说"给文档授权/分享权限" → `permission add`
用户说"公开文件/互联网公开/设置公开/让互联网所有人可访问" → `publish set`
用户说"关闭公开/取消公开/取消互联网访问" → `publish unset`
用户说"查看公开状态/是否公开/发布状态" → `publish get`

关键区分: drive(文件管理) vs doc(文档内容读写) vs wiki(空间管理)

**drive search vs wiki node search**: 用户提到"钉盘/网盘/我的文件里搜" → `drive search`；提到"知识库/文档空间/workspace 里搜" → `wiki node search`；未明确目标时 `drive search`（全局聚合搜索）。

**drive upload vs doc upload**: 文件上传统一走 `drive upload`。上传到知识库/文档空间时加 `--workspace` 参数。

**drive permission vs wiki member**: "给某篇文档/文件授权" → `drive permission add`（节点级）；"给某个知识库整体加成员" → `wiki member add`（空间级）

**创建在线文档/表格/脑图**: drive 不支持创建文件，需走 `wiki node create --type <type>`（创建空节点）或 `doc create`（创建并写入内容）。

**导出文档/导出为Word**: 导出是内容层操作，走 `doc export`，不属于 drive。

把图片/文件发到群里一般直接用 `chat message send --msg-type file --file-path <本地路径>`（见 [chat.md](./chat.md)），无需先经 drive 上传。

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

# 7. 删除文件/文件夹到回收站（危险操作：必须先向用户确认，用户同意后才加 --yes 执行）
# 正确流程：1.向用户展示"即将删除「文件名」到回收站" → 2.等用户确认 → 3.执行下面命令
dws drive delete --node <dentryUuid> --yes --format json

# 8. 查看回收站并还原文件
dws drive recycle list --format json
dws drive recycle restore --id <recycleItemId> --format json
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

> **rename 只传主名，不要带扩展名**：服务端会按文件原扩展名自动补一个后缀。若 `--name` 里已带扩展名（如 `报告.txt`），回读会变成双扩展名 `报告.txt.txt`。正确做法：`dws drive rename --node <ID> --name "报告"`（不含 `.txt`），系统自动补回 `报告.txt`。

权限要求：copy 需对源文档有"阅读"权限且对目标文件夹有"编辑"权限；move 需对源文档有"管理"权限且对目标文件夹有"编辑"权限；rename 需对文档有"编辑"权限。

> **字段选择**：`drive list` 返回中有 `dentryId`（数字格式）和 `fileId`（UUID 格式），**必须使用 `fileId`（UUID 格式）**作为 `--node` 和 `--folder` 参数值。

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
  dws drive permission update --node <ID> --users uid1 --role EDITOR
  dws drive permission list --node <ID>
  dws drive permission remove --node <ID> --users uid1
Flags:
      --node string        目标节点 ID 或 URL (必填)
      --users string       用户 userId 列表，逗号分隔 (add/update/remove 必填)
      --role string        角色: MANAGER / EDITOR / DOWNLOADER / READER (add/update 必填)
      --workspace string   知识库 ID (选填)
      --limit int          返回成员数上限 (仅 list，默认 30，最大 200)
      --filter-role string 按角色过滤: OWNER / MANAGER / EDITOR / DOWNLOADER / READER (仅 list)
```

### 文件互联网公开发布

管理文件的互联网公开发布状态。公开后任何人通过链接即可访问，无需登录钉钉。操作者需要是文件的管理员或拥有者。

> **`publish set` 和 `publish unset` 为 [危险] 操作，执行前需要向用户确认。确认后传入 `--yes` 跳过交互式确认。**

```
Usage:
  dws drive publish set --node <fileId> [--permission READER|DOWNLOADER|EDITOR]
  dws drive publish unset --node <fileId>
  dws drive publish get --node <fileId>
Example:
  dws drive publish set --node <dentryUuid>
  dws drive publish set --node <dentryUuid> --permission READER
  dws drive publish get --node <dentryUuid>
  dws drive publish unset --node <dentryUuid>
Flags:
      --node string         目标文件 ID (dentryUuid) 或 URL (必填)
      --permission string   公开后的权限: READER(仅可查看) / DOWNLOADER(可查看和下载，默认) / EDITOR(可编辑)，仅 set 有效
```

子命令说明：
- `publish set` — [危险] 设置文件为互联网公开，可选指定公开权限
- `publish unset` — [危险] 关闭文件互联网公开
- `publish get` — 查询文件当前的公开发布状态

返回字段说明：
- `published` — true=已公开，false=未公开
- `publishPermission` — 当前公开权限（READER/DOWNLOADER/EDITOR）
- `pendingApproval` — true=已提交审批待生效，false/null=无需审批或已直接生效
- `docUrl` — 文件访问链接

> **注意**：`drive export` 不存在。导出仅对自研文档 (adoc) 有意义，属于内容层操作，应使用 `doc export`。

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
| `list` | **`fileId`**（UUID 格式，注意：不是 `dentryId`） | info / download / delete 的 --node；list / mkdir 的 --folder；`drive copy/move` 的 --node 或 --folder |
| `list` | `spaceId` | info / download / mkdir / upload 的 --space-id |
| `list` | `nextCursor` | 下次 list 的 --cursor |
| `list-spaces` / `wiki space list` | `rootFolderId` | `drive copy/move` 的 --folder（复制/移动到钉盘 space 根目录时） |
| `list-spaces` / `wiki space list` | `spaceId` | list / info / download / mkdir / upload 的 --space-id |
| `search` | **`fileId`**（文件/文件夹结果） | info / download / delete 的 --node；list 的 --folder |
| `search` | `spaceId` / `rootFolderId`（空间结果） | list 的 --space-id；`drive copy/move` 的 --folder |
| `search` | `nextCursor` | search 的 --cursor（翻页） |
| `mkdir` | `fileId`（UUID 格式） | list / upload 的 --folder |
| `upload` | `dentryUuid` | download / info 的 --node |
| `recycle list` | `id`（回收项 ID） | recycle restore 的 --id |
| `recycle list` | `name`（原始文件名） | 供用户确认还原目标 |
| `recent` | `recentItems[].nodeId` / `docUrl` | doc read / info / update / block 操作的 --node |
| `recent` | `nextCursor` | recent 的 --cursor（翻页） |

> **重要**：`drive list` 返回结果中同时包含 `dentryId` 和 `fileId` 两个字段。所有需要传 `--node` 的命令（info / download / delete）必须使用 `fileId`（即 dentryUuid），**不要使用** `dentryId`。

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

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [drive_tree_list.py](../../scripts/drive_tree_list.py) | 递归列出钉盘目录树结构 | `python drive_tree_list.py --depth 2` |

## 相关产品

- [doc](./doc.md) — 文档内容读写（Markdown/块级编辑/导出），不是文件存储
- [wiki](./wiki.md) — 知识库/空间管理层（空间列表、节点创建、空间内搜索、成员管理）
- [chat](./chat.md) — 发送图片/文件消息用 `chat message send --msg-type file --file-path`
