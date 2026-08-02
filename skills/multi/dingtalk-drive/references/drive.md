# 钉盘 (drive) 命令参考

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
      --order string        排序方向: asc|desc，默认 desc (可选)
      --order-by string     排序字段: createTime|modifyTime|name (可选)
      --folder string       父节点 ID (dentryUuid)，不传则列出空间根目录 (可选)
      --space-id string     空间 ID，不传则使用「我的文件」对应 spaceId (可选)
      --thumbnail           是否返回缩略图信息 (可选)
```

### 获取钉盘空间列表

```
Usage:
  dws drive list-spaces [flags]
Example:
  dws drive list-spaces
  dws drive list-spaces --space-type mySpace
  dws drive list-spaces --space-type orgSpace --limit 20 --cursor <TOKEN>
Flags:
      --space-type string   空间类型: orgSpace=企业空间(默认), mySpace=我的文件 (可选)
      --limit int           每页返回数量 (默认 20，最大 50)，仅 spaceType 为 orgSpace 时有效
      --cursor string   分页游标，仅企业空间支持分页 (可选)
```

spaceType 筛选规则：
- `orgSpace`（默认/不传）：返回企业空间列表，支持 `nextToken` 分页
- `mySpace`：返回用户的"我的文件"个人空间（单个，不支持分页）

返回字段说明：
- `spaceId` — 空间 ID，用于 `list`/`info`/`upload` 等命令的 `--space-id`
- `spaceName` — 空间名称（如"全员文件夹"、"我的文件"）
- `rootFolderId` — 空间根目录的 dentryUuid，可作为 `doc copy/move` 的 `--folder` 参数
- `spaceType` — 空间类型（如 `orgSpace`）
- `nextToken` — 若不为空，表示还有更多空间可查询（仅企业空间）

### 搜索文件/文件夹/空间

按关键词搜索文件、文件夹或团队空间。不同于 `list`（需要明确的 spaceId/parentId 逐层遍历），`search` 用于不知道具体位置、只记得名称/关键词的场景。

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
      --target string          搜索目标: all(默认) | file | space (可选)
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

搜索目标 (`--target`) 选择规则：
- `all`（默认）：同时搜文件与空间，返回混合结果 — 不确定目标是文件还是空间时使用
- `file`：只搜文件 / 文件夹，支持 `--file-types` / `--extensions` 过滤 — 明确是找文件时使用
- `space`：只搜团队空间 — 明确知道空间名、需快速定位空间 spaceId/rootFolderId 时使用

返回结果中 `type` 字段区分：`SPACE`（空间）、`FILE`（普通文件）、`FOLDER`（文件夹）、`ALIDOC`（钉钉在线文档）。

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
      --node string    节点 ID (dentryUuid) (必填)
      --space-id string   节点所属空间 ID (可选)
```

### 文件内容获取路由规则

> 当用户请求"分析/查看/读取某个文件内容"时，**必须先调用 `dws drive info` 获取文件元数据**，再根据返回的 `extension` 字段选择对应链路。
> 注意：若检测到钉钉文档类型（adoc/axls/amind/adraw），会自动跟进调用 `doc info` 返回更准确的文档信息。

| extension | 文件类型 | 操作 | 命令 |
|-----------|---------|------|------|
| adoc | 在线文档 | 在线获取 Markdown 内容 | `dws doc read --node <fileId>` |
| axls | 在线表格 | 在线读取表格数据 | `dws sheet list --node <ID>` → `dws sheet range read --node <ID> --sheet-id <SHEET_ID> --range <RANGE>` |
| able | 多维表格 | 在线查询记录 | `dws aitable table list --base-id <BASE_ID>` → `dws aitable record query --base-id <BASE_ID> --table-id <TABLE_ID>` |
| md | Markdown 文件 | 读内容 / 创建 / 覆盖 / 局部替换 | `dws markdown fetch` / `dws markdown create` / `dws markdown overwrite` / `dws markdown patch` |
| 其他（pdf/docx/txt/png 等） | 普通文件 | **不支持在线分析**，需用户主动下载后本地查看 | `dws drive download` |

### 下载文件到本地

下载流程一步到位：获取下载 URL → HTTP GET 下载文件二进制内容到本地。

```
Usage:
  dws drive download [flags]
Example:
  dws drive download --node <dentryUuid> --output ./report.pdf
  dws drive download --node <dentryUuid> --output ~/downloads/
Flags:
      --node string    文件 ID (dentryUuid) (必填)
      --output string     本地保存路径 (文件路径或目录，不传则保存到当前目录)；如果指定目录，文件名从下载 URL 中自动推断 (可选)
      --space-id string   文件所属空间 ID (可选)
```

> **提示**：`--output` 为可选参数，不传则保存到当前目录，文件名从下载 URL 中自动推断。

### 创建文件夹

```
Usage:
  dws drive mkdir [flags]
Example:
  dws drive mkdir --name "项目资料"
  dws drive mkdir --name "子目录" --folder <dentryUuid>
Flags:
      --name string        文件夹名称，最长 50 字符 (必填)
      --folder string   父节点 ID (dentryUuid)，不传则在空间根目录下创建 (可选)
      --space-id string    目标空间 ID，不传则使用「我的文件」 (可选)
```

### 上传本地文件到钉盘

> **注意：** 上传文件必须使用 `dws drive upload` 命令，禁止使用 `upload-info` + `curl` + `commit` 三步流程。

```
Usage:
  dws drive upload [flags]
Example:
  dws drive upload --file ./report.pdf
  dws drive upload --file ./slides.pptx --file-name "Q1汇报.pptx"
  dws drive upload --file ./data.xlsx --folder <dentryUuid>
  dws drive upload --file ./updated.pdf --node <dentryUuid> --file-name "<原文件名.pdf>"
Flags:
      --file string        本地文件路径 (必填)
      --file-name string   文件显示名称 (默认使用文件名)
      --space-id string    目标空间 ID，不传则使用「我的文件」 (可选)
      --mime-type string   文件 MIME 类型，不传则自动推断 (可选)
      --folder string   父节点 ID (dentryUuid)，不传则上传到空间根目录 (可选，与 --node 互斥)
      --node string        覆盖目标文件 ID，传入即覆盖已有文件（透明模式：钉盘路径映射 overwriteFileId，知识库路径映射 overwriteNodeId）(可选，与 --folder 互斥)
```

`upload` 命令内部自动完成三步流程（获取凭证 → OSS PUT → 提交入库），无需手动分步操作。

> **覆盖保名规则（实测）**：使用 `--node` 覆盖普通文件前，先执行
> `dws drive info --node <dentryUuid> --format json` 记录原 `name`，再把该值原样传给
> `--file-name`。省略 `--file-name` 会采用本地文件名，并同时重命名远端目标。
> `extension=md` 不走本命令，切到 `dws markdown overwrite` 保留 Markdown diff
> 预览；`adoc` / `axls` / `able` 切对应内容产品。

### 删除文件/文件夹到回收站

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws drive delete [flags]
Example:
  dws drive delete --node <dentryUuid> --format json    # 查询 fileId: dws drive list
Flags:
      --node string    文件/文件夹 ID (dentryUuid)，即 drive list 返回的 fileId (必填)
```

注意：`--node` 使用的是 `drive list` 返回结果中的 `fileId` 字段（即 `dentryUuid`），**不是** `dentryId` 字段。

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

### 收藏文档

```
Usage:
  dws drive star add [flags]
Example:
  dws drive star add --node <nodeId_or_URL>
Flags:
      --node string    文档 ID 或 URL (必填)
```

### 取消收藏文档

```
Usage:
  dws drive star remove [flags]
Example:
  dws drive star remove --node <nodeId_or_URL>
Flags:
      --node string    文档 ID 或 URL (必填)
```

### 获取收藏列表

```
Usage:
  dws drive star list [flags]
Example:
  dws drive star list
  dws drive star list --content-types doc,sheet
  dws drive star list --resource-types DENTRY --limit 10
  dws drive star list --order-by createTime --sort desc --cursor <nextCursor>
Flags:
      --limit int                每页条数 (默认 20，最大 20)
      --cursor string            分页游标，从上次返回的 nextCursor 获取
      --order-by string          排序字段: createTime (可选)
      --sort string              排序方向: asc|desc (可选，默认 desc)
      --resource-types strings   资源大类筛选，逗号分隔: DENTRY, TEAM, WORKSPACE
      --content-types strings    内容类型筛选，逗号分隔: doc,sheet,ppt,whiteboard,mind,notable,pdf,other,folder
```

返回字段说明：
- `starList[]` — 收藏项数组，每项包含：
  - `resourceType` — 资源类型: DENTRY / TEAM / WORKSPACE
  - `nodeId` — 文档节点 ID（仅 DENTRY）
  - `name` — 资源名称
  - `url` — 资源访问链接
  - `contentType` — 内容类型: ALIDOC / DOCUMENT / IMAGE / VIDEO / AUDIO / ARCHIVE / OTHER（仅 DENTRY）
  - `extension` — 文件扩展名（仅 DENTRY）
  - `dentryType` — 节点类型: file / folder（仅 DENTRY）
  - `starTime` — 收藏时间（RFC 3339 格式）
  - `id` — 资源唯一标识（仅 TEAM / WORKSPACE）
  - `spaceType` — 知识库类型: 1=知识库, 2=我的文档（仅 WORKSPACE）
- `hasMore` — 是否有更多数据
- `nextCursor` — 下次分页游标

## 意图判断

用户说"我的文件/钉盘/网盘/云盘" → `list`
用户说"最近访问/最近打开/最近编辑/最近文档" → `recent`（默认仅最近访问，`--operate-type 1` 仅最近编辑，`--operate-type 0,1` 全部）
用户说"钉盘空间/团队文件/有哪些空间/空间列表/团队文件列表" → `list-spaces`
用户说"搜索钉盘文件/钉盘里找个文件/查找某个钉盘文件/钉盘中搜索" → `search`
用户说"文件详情/文件信息" → `info`
用户说"下载文件" → `download`
用户说"新建文件夹/创建目录" → `mkdir`（钉盘空间）/ `folder create`（文档空间）
用户说"上传文件/传文件到钉盘" → `upload`（必须使用此命令，自动完成三步流程）
用户说"复制文件/移动文件/搬到/移到" → `copy` / `move`
用户说"创建快捷方式/创建链接/放个快捷方式/引用到其他位置" → `shortcut`
用户说"阅读量/查看次数/多少人看过/点赞数/评论数/下载次数/文档统计/数据统计" → `stats`
用户说"封面/封面图/缩略图/预览图/节点封面" → `cover`
用户说"历史版本/版本列表/文件版本/有哪些版本/版本记录" → `list --versions --node <id>`
用户说"下载历史版本/下载旧版本/恢复历史版本到本地" → `download --version <N>`
用户说"回滚版本/恢复到某个版本/还原到旧版本/版本回退" → `revert`（危险操作，需确认）
用户说"重命名/改名" → `rename`
用户说"删除文件/删除文件夹/移到回收站" → `delete`（危险操作，需确认）
用户说"回收站/查看回收站/回收站列表/回收站里有什么" → `recycle list`
用户说"恢复文件/还原删除的文件/从回收站恢复/还原回收站文件" → `recycle restore`
用户说"收藏文档/收藏这个文件/加个收藏/标星" → `star add`
用户说"取消收藏/去掉收藏/不收藏了" → `star remove`
用户说"我的收藏/收藏列表/收藏了哪些文档/看看收藏" → `star list`
用户说"给文档授权/分享权限" → `permission add`
用户说"打不开/没权限/申请权限/我要编辑权限/找谁审批/申请访问权限" → 先 `permission apply-info` 查可申请角色与审批人 → 再 `permission apply` 发起申请
用户说"公开文件/互联网公开/设置公开/让互联网所有人可访问" → `publish set`
用户说"关闭公开/取消公开/取消互联网访问" → `publish unset`
用户说"查看公开状态/是否公开/发布状态" → `publish get`
用户说"查任务状态/导出好了没/任务进度/导入状态" → 文档导出用 `dws doc export get --job-id <ID>`，导入用 `dws doc import get --task-id <ID>`

关键区分: drive(文件管理) vs doc(文档内容读写) vs wiki(空间管理)

**drive search vs wiki node search**: 用户提到"钉盘/网盘/我的文件里搜" → `drive search`；提到"知识库/文档空间/workspace 里搜" → `wiki node search`；未明确目标时优先问明。

**drive upload**: 新文件上传统一走 `drive upload`。覆盖普通文件时先 `drive info` 取原 `name`，再用 `drive upload --node <ID> --file <PATH> --file-name "<原name>" --format json`；省略 `--file-name` 会隐式重命名。上传到知识库/文档空间时加 `--workspace` 参数。

**drive permission vs wiki member**: "给某篇文档/文件授权" → `drive permission add`（节点级）；"给某个知识库整体加成员" → `wiki member add`（空间级）

**创建在线文档/表格/脑图**: drive 不支持创建文件，需走 `wiki node create --type <type>`（创建空节点）或 `doc create`（创建并写入内容）。

**导出文档/导出为Word**: 导出是内容层操作，走 `doc export`，不属于 drive。超时后的状态查询用 `dws doc export get --job-id <ID>`。

**版本管理**: 当前 CLI 仅支持钉钉在线文档（adoc）的版本管理（`dws doc version list/save/revert`）；普通文件的历史版本列出/下载/回滚暂不支持。

**.md 文件的内容操作路由**: 当 `drive info` 返回 `extension=md` 时，文件管理操作（移动/重命名/删除/下载文件）留在 `drive`，但**读取或改写原文内容必须切换到 `markdown` 产品**：
- 用户说"读取/看一下 markdown 内容/获取 .md 原文" → `dws markdown fetch --node <ID>`（非 `drive download`）
- 详见 `dingtalk-markdown` 的 [markdown.md](../../dingtalk-markdown/references/markdown.md)

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

# 6. 上传文件（必须使用 upload 命令，禁止手动分步操作）
dws drive upload --file ./报告.pdf --format json
dws drive upload --file ./报告.pdf --folder <dentryUuid> --format json

# 7. 删除文件/文件夹到回收站（危险操作：必须先向用户确认，用户同意后才加 --yes 执行）
# 正确流程：1.向用户展示"即将删除「文件名」到回收站" → 2.等用户确认 → 3.执行下面命令
dws drive delete --node <dentryUuid> --yes --format json

# 8. 查看回收站并还原文件
dws drive recycle list --format json
dws drive recycle restore --id <recycleItemId> --format json

# 9. 收藏 / 取消收藏 / 查看收藏列表
dws drive star add --node <nodeId_or_URL> --format json
dws drive star remove --node <nodeId_or_URL> --format json
dws drive star list --format json
dws drive star list --content-types doc,sheet --limit 10 --format json
```

## 无权限引导申请工作流

当 Agent 执行文档操作（如 `doc read` / `drive info` / `drive download`）遇到权限拒绝错误时，按以下流程引导用户申请权限：

> **CAUTION（红线）：`apply` 会真实通知审批人，属于有副作用操作。Agent 严禁自行提交申请。必须先向用户逐项回显确认「资源 / 角色 / 审批人 / 理由」，得到用户明确同意后，才能执行 `apply`。用户未明确同意前，只能停在确认环节，不得调用 `apply`。**

```
1. 识别错误 → 询问用户「该文档你暂无权限，是否申请？」
2. 用户确认申请 → 查可申请角色与审批人：
   dws drive permission apply-info --node <节点ID或URL> --format json
3. 向用户展示 availableRoles / approvers，询问「申请哪个角色？发给哪位审批人？」
4. 用户选择 role + 审批人（可选填 reason）
5. Agent 回显确认「资源 / 角色 / 审批人 / 理由」→ 等待用户明确同意（这是提交前的强制关卡，不可跳过）
6. 仅在用户明确同意后，才发起申请：
   dws drive permission apply --node <节点ID或URL> --role <EDITOR|DOWNLOADER|READER> --users <审批人userId> [--reason "..."]
7. 展示 applyResult / applyResultDesc
```

要点：
- `apply-info` 的 `availableRoles[].roleId` → `apply --role`；`approvers[].userId` → `apply --users`
- **`apply` 严禁自动/静默调用**：必须由用户在看到确认信息后明确同意（如回复"确认/同意/提交"）才可执行；Agent 不得替用户决定或跳过确认
- `applyResult` 含义：NORMAL(已提交)/HAS_PERMISSION_ALREADY(已拥有)/APPLYING(审批中)/APPLYING_BUT_REAPPLY(已重新提交)

## 文档空间管理命令

> 以下命令操作的是**文档空间**（知识库 / 我的文档），底层路由到 doc MCP server。
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

> **字段选择**：`drive list` 返回中有 `dentryId`（数字格式）和 `fileId`（UUID 格式），**必须使用 `fileId`（UUID 格式）**作为 `--node` 和 `--folder` 参数值。

### 获取节点统计信息

获取指定节点的统计数据（阅读人数、阅读次数、编辑次数、评论数、点赞数、预览次数、下载次数等）。

```
Usage:
  dws drive stats [flags]
Example:
  dws drive stats --node <dentryUuid>
  dws drive stats --node https://alidocs.dingtalk.com/i/nodes/<dentryUuid>
Flags:
      --node string   节点 ID (dentryUuid) 或文档 URL (必填)
```

> **统计维度因文件类型而异**：
> - 钉钉在线文档（adoc）：阅读人数、阅读次数、编辑次数、评论数、点赞数、预览次数等
> - 普通文件：仅阅读次数、下载次数
> - 点赞数（likeCount）仅在线文档（adoc）有效

### 获取节点封面地址

获取指定节点的封面图片地址。

```
Usage:
  dws drive cover [flags]
Example:
  dws drive cover --node <dentryUuid>
  dws drive cover --node https://alidocs.dingtalk.com/i/nodes/<dentryUuid>
Flags:
      --node string   节点 ID (dentryUuid) 或文档 URL (必填)
```

> **封面因文件类型而异**：
> - 钉钉在线文档（adoc）：文档首图/预览封面
> - 图片文件：图片缩略图地址
> - 其他文件：文件类型图标地址（如有）

### 文件历史版本管理

获取文件历史版本列表、下载历史版本。

> **适用范围**：仅适用于普通文件（如 pdf、docx、xlsx、png 等）。
> - 钉钉在线文档（adoc）请使用 `dws doc version list`
> - 钉钉在线表格（axls）请使用 `dws sheet version list`
> - 命令会自动检测文件类型，若为在线文档会提示使用对应服务命令

#### 获取文件历史版本列表

```
Usage:
  dws drive list --versions [flags]
Example:
  dws drive list --node <dentryUuid> --versions --format json
  dws drive list --node <dentryUuid> --versions --limit 20 --format json
Flags:
      --node string       文件 ID (dentryUuid) 或 URL (必填)
      --limit int         每页最大数量 (默认 20，最大 50)
      --cursor string     分页游标 (从上次返回结果获取，首次不传)
```

#### 下载文件历史版本

```
Usage:
  dws drive download-version [flags]
Example:
  dws drive download-version --node <dentryUuid> --version 3 --output ./report_v3.pdf --format json
  dws drive download-version --node <dentryUuid> --version 3 --output <DOWNLOAD_DIR> --format json
Flags:
      --node string       文件 ID (dentryUuid) 或 URL (必填)
      --version int       历史版本号 (必填，正整数，从 list --versions 获取)
      --output string     本地保存路径 (文件路径或目录，必填)
```

> **两步下载流程**：先调用 MCP 工具获取历史版本下载 URL 和签名头，再 HTTP GET 下载文件内容到本地。
> `--output` 指定目录时，优先从文件信息中获取原始文件名，获取不到时从下载 URL 推断。

#### 回滚文件到指定历史版本

> **CAUTION:** 危险操作 — 执行前必须向用户确认。

```
Usage:
  dws drive revert [flags]
Example:
  dws drive revert --node <dentryUuid> --version 3 --yes --format json
Flags:
      --node string       文件 ID (dentryUuid) 或 URL (必填)
      --version int       要回滚到的历史版本号 (必填，正整数)
      --yes               跳过确认提示
```

回滚成功后，系统会基于目标版本生成一份新的最新版本（内容与目标版本一致），原有历史版本不会丢失。
仅支持普通文件（Word、Excel、PDF、图片等）的历史版本回滚。在线文档的版本回滚请用 `dws doc version revert`，在线表格请用 `dws sheet version revert`。
需要当前用户对该文件具备编辑权限。

### 创建快捷方式

为指定的源节点创建快捷方式（链接），放置到目标位置。

```
Usage:
  dws drive shortcut [flags]
Example:
  dws drive shortcut --node <dentryUuid>
  dws drive shortcut --node <dentryUuid> --folder <targetFolderId>
  dws drive shortcut --node <dentryUuid> --workspace <workspaceId>
Flags:
      --node string        源节点 ID (dentryUuid) 或文档 URL (必填)
      --folder string      目标文件夹 nodeId，不传则放到知识库/我的文档根目录 (可选)
      --workspace string   目标知识库 ID (可选)
```

> **目标位置规则**：`--folder` 和 `--workspace` 都不传时默认放置到「我的文档」根目录；同时传入时以 `--folder` 为准，`--workspace` 仅用于一致性校验。
> **权限要求**：对源节点有“可查看”权限，对目标文件夹有“编辑”权限。
> **目标位置限制**：若目标位置不支持在其下创建子节点（如快捷方式节点、非文件夹），会返回错误，请改指定一个文件夹作为目标位置。

### 创建文件夹（文档空间）

```
Usage:
  dws drive folder create --name "文件夹名"
Flags:
      --name string        名称 (必填)
      --folder string      父文件夹 nodeId
      --workspace string   目标知识库 ID
```

### 权限管理（文档节点级）

```
Usage:
  dws drive permission add --node <ID> --users uid1,uid2 --role READER
  dws drive permission update --node <ID> --users uid1 --role EDITOR
  dws drive permission list --node <ID>
  dws drive permission remove --node <ID> --users uid1
  dws drive permission transfer-owner --node <ID> --new-owner <userId>
  dws drive permission transfer-owner --workspace <WS_ID> --new-owner <userId>
  dws drive permission transfer-owner --node <ID> --new-owner <userId> --reserve-role EDITOR --recursive=false --yes
Flags:
      --node string        目标节点 ID 或 URL (必填)
      --users string       用户 userId 列表，逗号分隔
      --role string        角色: MANAGER / EDITOR / DOWNLOADER / READER
      --limit int          返回成员数上限 (仅 list，默认 30，最大 200)
      --filter-role string 按角色过滤 (仅 list)
      --workspace string   目标知识库 ID 或 URL (仅 transfer-owner，与 --node 二选一)
      --new-owner string   新所有者的用户 userId (仅 transfer-owner，必填)
      --reserve-role string  转交后原所有者保留角色: MANAGER / EDITOR / DOWNLOADER / READER / NONE(移除权限) (仅 transfer-owner)
      --recursive bool     是否递归变更所有子节点的所有者 (仅 transfer-owner)
```

> **`transfer-owner` 为 [危险] 操作，执行前需要确认。**
>
> - 交互模式下会依次提示选择原所有者保留角色、是否递归变更子节点，然后显示操作摘要等待确认
> - 使用 `--yes` 跳过确认时，`--reserve-role` 和 `--recursive` 必须显式指定（禁止静默默认值）
> - `--reserve-role NONE` 表示移除原所有者的所有权限
> - 当前用户需为该节点/知识库的所有者才能执行转交

> ** Agent 行为约束（必须遵守）**
>
> `--reserve-role` 和 `--recursive` 属于高风险操作的核心决策参数，**必须由用户明确指定**，Agent 不得自行选择默认值（如 `NONE` / `false`）代替用户决策。
>
> 执行 `transfer-owner` 前，Agent **必须**先通过对话向用户询问以下两个问题，只有在用户明确回答后才构造命令执行：
>
> 1. **"转交后原所有者保留什么角色？"** 选项：
>    - `MANAGER`（管理者）
>    - `EDITOR`（编辑者）
>    - `DOWNLOADER`（下载者）
>    - `READER`（只读者）
>    - `NONE`（移除原所有者权限）
> 2. **"是否递归变更所有子节点的所有者？"** 选项：
>    - `true`（是）
>    - `false`（否）
>
> 正确流程：1.向用户询问上述两个问题 → 2.根据用户选择构造 `--reserve-role <用户选择> --recursive=<用户选择> --yes` → 3.执行命令
>
> **禁止**在未获得用户明确回答的情况下直接执行命令。

### 权限申请（无权限时发起申请）

> 当你对文档/知识库暂无访问权限时，可查询可申请的角色与审批人，并向审批人发起权限申请。

```
Usage:
  dws drive permission apply-info --node <ID>
  dws drive permission apply --node <ID> --role <ROLE> --users <审批人userId...>
Example:
  dws drive permission apply-info --node <dentryUuid>
  dws drive permission apply --node <dentryUuid> --role READER --users uid1
  dws drive permission apply --node <dentryUuid> --role EDITOR --users uid1,uid2 --reason "需要编辑该文档"
Flags:
      --node string          目标节点 ID 或 URL (必填)
      --role string          申请的角色: EDITOR / DOWNLOADER / READER (仅 apply 必填)
      --users string         审批人 userId 列表，逗号分隔 (仅 apply 必填)
      --notify-mode string   通知方式: DEFAULT / MSG_ACCOUNT / SINGLE_CHAT (仅 apply 可选)
      --reason string        申请理由，最长 200 字符 (仅 apply 可选)
```

子命令说明：
- `apply-info` — 查询节点可申请的角色列表 (availableRoles) 与审批人列表 (approvers)
- `apply` — 向审批人发起权限申请

返回字段说明：
- `apply-info`：
  - `availableRoles[].roleId` — 可申请角色 (EDITOR/DOWNLOADER/READER)，用作 `apply --role`
  - `availableRoles[].name` / `desc` — 角色名称/描述
  - `approvers[].userId` — 审批人 userId，用作 `apply --users`
  - `approvers[].userName` — 审批人名称
  - `approvers[].isResourceCreator` — 是否为资源创建者
- `apply`：
  - `applyResult` — 申请结果：NORMAL(已提交申请)/HAS_PERMISSION_ALREADY(已拥有该权限)/APPLYING(已有待审批申请)/APPLYING_BUT_REAPPLY(已重新提交申请)
  - `applyResultDesc` — 结果的人类可读描述

> **重要（Agent 编排）**：`apply` 会向审批人发送通知，属副作用操作。Agent 必须在调用 `apply` 前，向用户回显「资源 / 申请角色 / 审批人 / 理由」并取得用户确认后再执行。

### 文件互联网公开发布

管理文件的互联网公开发布状态。公开后任何人通过链接即可访问，无需登录钉钉。操作者需要是文件的管理员或拥有者。

> **`publish set` 和 `publish unset` 为 [危险] 操作，执行前需要确认。必须传入 `--yes` 跳过交互式确认。**

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
- `publish get`（别名 `status`）— 查询文件当前的公开发布状态

返回字段说明：
- `published` — true=已公开，false=未公开
- `publishPermission` — 当前公开权限（READER/DOWNLOADER/EDITOR）
- `pendingApproval` — true=已提交审批待生效，false/null=无需审批或已直接生效
- `docUrl` — 文件访问链接

> **注意**：`drive export` 不存在。导出仅对自研文档 (adoc) 有意义，属于内容层操作，应使用 `doc export`。

### 目标位置参数规则

| 目标位置 | 参数传递方式 | 前置步骤 |
|---------|-----------|---------|
| 未指定目标（默认） | `--folder <rootFolderId>` | 先 `dws drive list-spaces --space-type mySpace` 获取「我的文件」的 `rootFolderId` |
| 知识库空间根目录 | `--workspace <workspaceId>` | 无需额外步骤 |
| 钉盘 space 根目录 | `--folder <rootFolderId>` | 先 `dws drive list-spaces` 获取目标 space 的 `rootFolderId` |
| 钉盘 space 下的子文件夹 | `--folder <fileId>` | 先 `dws drive list --space-id <spaceId>` 逐层浏览 |

### 工作流示例

```bash
# ── 场景 默认: 复制/移动到「我的文件」根目录 ──
dws drive list --space-id <SPACE_ID> --format json
dws drive list-spaces --space-type mySpace --format json
dws drive copy --node <源文件dentryUuid> --folder <我的文件rootFolderId> --format json

# ── 场景 A: 复制到知识库空间根目录 ──
dws drive copy --node <源文件dentryUuid> --workspace <TARGET_WS_ID> --format json

# ── 场景 B: 移动到另一个钉盘 space 根目录 ──
dws drive list-spaces --format json
dws drive move --node <源文件dentryUuid> --folder <目标space的rootFolderId> --format json

# ── 场景 C: 复制到钉盘子文件夹 ──
dws drive list --space-id <TARGET_SPACE_ID> --format json
dws drive copy --node <源文件dentryUuid> --folder <目标文件夹fileId> --format json
```

## 上下文传递表

| 操作            | 从返回中提取                       | 用于                                                       |
| ------------- | ---------------------------- | -------------------------------------------------------- |
| `list`        | **`fileId`**（UUID 格式，注意：不是 `dentryId`） | info / download / mkdir / delete / stats / shortcut / cover 的 --node 或 --folder；`drive copy/move` 的 --node 或 --folder |
| `list`        | `spaceId`                    | info / download / mkdir / commit 的 --space-id            |
| `list-spaces` | `rootFolderId`               | `drive copy/move` 的 --folder（复制/移动到钉盘 space 根目录时） |
| `list-spaces` | `spaceId`                    | list / info / download / mkdir / upload 的 --space-id     |
| `search`      | **`fileId`**（文件/文件夹结果） | info / download / delete 的 --node；list 的 --folder         |
| `search`      | `spaceId` / `rootFolderId`（空间结果） | list 的 --space-id；`drive copy/move` 的 --folder        |
| `search`      | `nextCursor`                 | search 的 --cursor（翻页）                                  |
| `mkdir`       | `fileId`（UUID 格式）            | list 的 --folder                                          |
| `recycle list` | `id`（回收项 ID）               | recycle restore 的 --id                                    |
| `recycle list` | `name`（原始文件名）             | 供用户确认还原目标                                          |
| `recent`      | `recentItems[].nodeId` / `docUrl` | doc read / info / update / block 操作的 --node |
| `recent`      | `nextCursor`                 | recent 的 --cursor（翻页）                                  |
| `permission apply-info` | `availableRoles[].roleId` | `permission apply` 的 --role                        |
| `permission apply-info` | `approvers[].userId`      | `permission apply` 的 --users                       |
| `star list`   | `starList[].nodeId`             | star remove 的 --node；info / download 的 --node             |
| `star list`   | `nextCursor`                    | star list 的 --cursor（翻页）                                  |

> **重要**：`drive list` 返回结果中同时包含 `dentryId` 和 `fileId` 两个字段。所有需要传 `--node` 的命令（info / download / delete）必须使用 `fileId`（即 dentryUuid），**不要使用** `dentryId`。

## 注意事项

- 不传 `--space-id` 时默认使用「我的文件」空间
- 不传 `--folder` 时默认操作空间根目录
- `--folder` 只能使用父文件夹的 `dentryUuid`。不要把 `drive info` 返回的数字型 `dentryId` 当作父目录；`dentryId` 只用于 `chat message send --dentry-id`
- **`--limit` 最大值为 50**，禁止传入超过 50 的值（如 `--limit 100`）。用户要求超过 50 条时，应使用 `--limit 50` 配合 `--cursor` 分页查询，不要直接传大于 50 的值
- `--order-by` 支持: `createTime`、`modifyTime`、`name`
- **上传文件必须使用 `dws drive upload` 命令**，禁止使用 `upload-info` + `curl` + `commit` 三步手动流程
- `--file-name` 必须包含扩展名（如 `report.pdf`）

## 自动化脚本

| 脚本                                                     | 场景          | 用法                                    |
| ------------------------------------------------------ | ----------- | ------------------------------------- |
| [drive_tree_list.py](../scripts/drive_tree_list.py) | 递归列出钉盘目录树结构 | `python drive_tree_list.py --depth 2` |

## 相关产品

- [doc](../../dingtalk-doc/references/doc.md) — 文档内容读写/知识库空间，不是文件存储
- [markdown](../../dingtalk-misc/references/markdown.md) — `.md` 文件的内容读取（fetch），非文件管理
- [chat](../../dingtalk-chat/references/chat.md) — 上传文件到 drive 后可通过 Markdown 语法发送图片/文件消息
