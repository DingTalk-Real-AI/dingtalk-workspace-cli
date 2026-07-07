# 知识库 (wiki) 命令参考

## 查询命令帮助

当你不确定某个命令的具体参数、格式或可选项时，**优先执行 `--help` 查询**，不要猜测参数名或凭记忆编造。

```bash
# 查看 wiki 下所有子命令
dws wiki --help

# 查看具体命令的完整参数说明
dws wiki space get --help
dws wiki member add --help

# 查看子命令组下的所有命令
dws wiki space --help
dws wiki node --help
dws wiki member --help
```

规则：
- 参数名不确定时 → 先 `--help`，再调用
- 报错 "unknown flag" 时 → `--help` 确认正确的 flag 名称
- 不确定某个功能是否存在时 → `dws wiki --help` 查看命令列表

## 命令总览

### 创建知识库
```
Usage:
  dws wiki space create [flags]
Example:
  dws wiki space create --name "产品文档库" --format json
  dws wiki space create --name "技术方案" --desc "团队技术方案归档" --format json
Flags:
      --name string   知识库名称 (必填，不超过 100 字符)
      --desc string   知识库描述 (选填，不超过 500 字符)
      --icon string   知识库图标标识 (选填)
```

### 删除知识库

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws wiki space delete [flags]
Example:
  dws wiki space delete --workspace <workspaceId>
  dws wiki space delete --workspace "https://alidocs.dingtalk.com/i/spaces/xxx/overview"
Flags:
      --workspace string   知识库 ID 或 URL (必填)
```

将指定知识库移入回收站。删除后知识库会进入回收站，可在回收站中恢复。

> **重要约束**：
> - 操作者必须具备知识库的 OWNER 角色。
> - 删除操作不可逆（从回收站恢复除外），请确认后再执行。

### 查看知识库详情
```
Usage:
  dws wiki space get [flags]
Example:
  dws wiki space get --workspace <workspaceId> --format json
  dws wiki space get --workspace "https://alidocs.dingtalk.com/i/spaces/xxx/overview" --format json
Flags:
      --workspace string   知识库 ID 或 URL (必填)
```

支持传入知识库 ID 或知识库 URL，系统自动识别。
知识库 URL 格式：`https://alidocs.dingtalk.com/i/spaces/{workspaceId}/overview`

### 列出空间（知识库 / 钉盘空间）
```
Usage:
  dws wiki space list [flags]
Example:
  dws wiki space list --format json                       # 组织知识库（默认）
  dws wiki space list --type myWikiSpace --format json    # 我的文档
  dws wiki space list --type orgSpace --format json       # 钉盘企业空间
  dws wiki space list --type mySpace --format json        # 钉盘「我的文件」
  dws wiki space list --type orgWikiSpace --limit 50 --format json
Flags:
      --type string    空间类型:
                         orgWikiSpace (默认) — 组织知识库
                         myWikiSpace — 我的文档个人空间
                         orgSpace — 钉盘企业空间
                         mySpace — 钉盘「我的文件」
      --limit string   每页数量 1-50 (默认 20)
      --cursor string  分页游标 (首页留空)
```

- `myWikiSpace`：返回当前用户的「我的文档」个人空间（固定 1 条，不支持分页）
- `orgWikiSpace`（默认）：返回组织内有权访问的知识库列表，支持分页
- 钉盘空间类型（`orgSpace` / `mySpace`）会自动路由到钉盘服务，等同于原 `drive list-spaces`（已 deprecated）

### 搜索知识库
```
Usage:
  dws wiki space search [flags]
Example:
  dws wiki space search --query "产品文档" --format json
  dws wiki space search --query "技术方案" --limit 20 --format json
  dws wiki space search --type myWikiSpace --format json
Flags:
      --query string   搜索关键词 (搜索组织知识库时必填，--type myWikiSpace 时可省略)
      --type string    知识库类型: myWikiSpace 时直接返回「我的文档」，省略则搜索组织知识库
      --limit string   返回数量 1-20 (默认 10)
```

当 `--type myWikiSpace` 时，忽略 `--query`，直接返回「我的文档」个人空间。

### 添加知识库成员（容器级授权）
```
Usage:
  dws wiki member add [flags]
Example:
  dws wiki member add --workspace <WS_ID> --users uid1 --role READER
  dws wiki member add --workspace <WS_ID> --users uid1,uid2 --role EDITOR
  dws wiki member add --workspace "https://alidocs.dingtalk.com/i/spaces/<WS_ID>/overview" --users uid1 --role MANAGER
Flags:
      --workspace string   目标知识库 ID 或 URL (必填)
      --users string       被加入的用户 userId 列表，逗号分隔 (必填，单次最多 30 个)
      --role string        授予的角色 (必填，大小写不敏感): MANAGER (管理者) / EDITOR (可编辑) / DOWNLOADER (可下载) / READER (可阅读)
```

> **❗ 重要约束**：
> - 仅支持 USER 类型。
> - 角色枚举：MANAGER / EDITOR / DOWNLOADER / READER（OWNER 不可通过此接口添加，知识库创建者默认为所有者）。
> - 操作者需具备知识库的 OWNER 或 MANAGER 权限。
> - 「我的文档」(myWikiSpace) 是个人空间，**不支持容器级成员管理**；后端会直接拒绝。如果你的目标只是把某篇文档分享给别人，请改用 `dws drive permission add` 在节点级别授权。

### 移除知识库成员
```
Usage:
  dws wiki member remove [flags]
Example:
  dws wiki member remove --workspace <WS_ID> --users uid1
  dws wiki member remove --workspace <WS_ID> --users uid1,uid2
Flags:
      --workspace string   目标知识库 ID 或 URL (必填)
      --users string       被移除的用户 userId 列表，逗号分隔 (必填，单次最多 30 个)
```

> **重要约束**：
> - OWNER 角色不可通过此接口移除。
> - 操作者需具备知识库的 OWNER 或 MANAGER 权限。
> - 移除后相关用户将无法访问该知识库下的内容（除非通过节点级权限另行授权）。
> - 「我的文档」(myWikiSpace) 是个人空间，**不支持容器级成员管理**。

### 修改知识库成员角色
```
Usage:
  dws wiki member update [flags]
Example:
  dws wiki member update --workspace <WS_ID> --users uid1 --role EDITOR
  dws wiki member update --workspace <WS_ID> --users uid1,uid2 --role READER
Flags:
      --workspace string   目标知识库 ID 或 URL (必填)
      --users string       目标用户 userId 列表，逗号分隔 (必填，单次最多 30 个)
      --role string        新角色 (必填，大小写不敏感): MANAGER / EDITOR / DOWNLOADER / READER
```

### 列出知识库成员
```
Usage:
  dws wiki member list [flags]
Example:
  dws wiki member list --workspace <WS_ID>
  dws wiki member list --workspace <WS_ID> --limit 100
  dws wiki member list --workspace <WS_ID> --filter-role EDITOR
Flags:
      --workspace string     目标知识库 ID 或 URL (必填)
      --limit int            返回成员数上限，默认 30，最大 200
      --filter-role string   按角色过滤，逗号分隔: OWNER / MANAGER / EDITOR / DOWNLOADER / READER (选填)
```

> 接口不支持游标分页，使用 `--limit` 一次性拉取。

> ⚠️ **返回字段限制**：`member list` 每条只返回 `name` / `role` / `type` 三个字段，**不含 userId**（服务端不返回）。因此**无法**从 `member list` 拿到 userId 再去串联 `member update` / `member remove`。要对某人改角色 / 移除，需另行拿到其 userId（例如用 `dws contact user search --query "<姓名>"` 按姓名反查）。

### 列出知识库节点
```
Usage:
  dws wiki node list [flags]
Aliases:
  list, ls
Example:
  dws wiki node list --workspace <workspaceId> --format json
  dws wiki node list --workspace <workspaceId> --folder <parentNodeId> --format json
  dws wiki node list --workspace <workspaceId> --limit 20 --cursor <pageToken> --format json
Flags:
      --workspace string   知识库 ID (必填)
      --folder string      父节点 nodeId (选填，不传则列出根目录)
      --limit int          每页数量 (默认 50，最大 50)
      --cursor string      分页游标
```

### 在知识库中创建节点
```
Usage:
  dws wiki node create [flags]
Example:
  dws wiki node create --workspace <workspaceId> --name "新文档" --format json
  dws wiki node create --workspace <workspaceId> --name "方案目录" --type folder --format json
  dws wiki node create --workspace <workspaceId> --name "数据表" --type axls --folder <parentNodeId> --format json
Flags:
      --workspace string   知识库 ID (必填)
      --name string        节点名称 (必填)
      --type string        节点类型 (默认 adoc)，实际支持: adoc(在线文档) / axls(在线表格) / appt(演示文稿) / adraw(白板) / amind(脑图) / able(多维表) / folder(文件夹)
      --folder string      父节点 nodeId (选填，不传则在根目录创建)
```

> ⚠️ **type 枚举修正**：`--help` 中列出的 `asheet` 会被后端拒绝，**不要使用**；创建在线表格请用 `axls`。实际可用类型以上方列表为准。

### 复制知识库节点
```
Usage:
  dws wiki node copy [flags]
Example:
  dws wiki node copy --workspace <workspaceId> --node <nodeId> --format json
  dws wiki node copy --workspace <workspaceId> --node <nodeId> --folder <targetFolderId> --format json
Flags:
      --workspace string   知识库 ID (必填)
      --node string        源节点 ID (必填)
      --folder string      目标文件夹 nodeId (选填)
```

### 移动知识库节点
```
Usage:
  dws wiki node move [flags]
Example:
  dws wiki node move --workspace <workspaceId> --node <nodeId> --folder <targetFolderId> --format json
  dws wiki node move --workspace <workspaceId> --node <nodeId> --format json
Flags:
      --workspace string   知识库 ID (必填)
      --node string        源节点 ID (必填)
      --folder string      目标文件夹 nodeId (选填)
```

### 删除知识库节点

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws wiki node delete [flags]
Example:
  dws wiki node delete --workspace <workspaceId> --node <nodeId>
  dws wiki node delete --workspace <workspaceId> --node <nodeId> --yes
Flags:
      --workspace string   知识库 ID (必填，用于权限校验)
      --node string        节点 ID (必填)
```

将知识库中的节点移入回收站。权限要求: 对节点有"管理"权限。

### 在知识库中搜索节点
```
Usage:
  dws wiki node search [flags]
Example:
  dws wiki node search --workspace <workspaceId> --query "方案" --format json
  dws wiki node search --workspace <workspaceId> --query "周报" --limit 10 --format json
  dws wiki node search --workspace <workspaceId> --query "设计" --extensions adoc,axls --format json
Flags:
      --workspace string    知识库 ID (必填)
      --query string        搜索关键词 (必填)
      --extensions strings  按文件扩展名过滤，逗号分隔 (如 adoc,axls,pdf) (选填)
      --limit int           每页数量 (默认 10，最大 30) (选填)
      --cursor string       分页游标 (选填)
```

在指定知识库空间内搜索节点。与 `drive search` 的区别：
- `wiki node search` — 限定在某个知识库空间内搜索（需要 `--workspace`）
- `drive search` — 全局搜索，聚合钉盘 + 文档空间结果

## 意图判断

- 用户说"创建知识库/新建知识库" → `space create`
- 用户说"查看知识库/知识库详情" → `space get`
- 用户说"我的知识库/知识库列表/有哪些知识库" → `space list`
- 用户说"列出钉盘空间/钉盘团队空间" → `space list --type orgSpace`
- 用户说"搜索知识库/找知识库" → `space search`
- 用户说"我的文档/个人空间" → `space list --type myWikiSpace`
- 用户说"知识库下的文件/知识库里有哪些文档/浏览知识库内容" → `node list`（需 `--workspace`）
- 用户说"在知识库里搜文档/空间内搜索" → `node search`（需 `--workspace` + `--query`）
- 用户说"在知识库里创建文档/新建文件夹" → `node create`（需 `--workspace` + `--name`）
- 用户说"复制知识库里的文档" → `node copy`（需 `--workspace` + `--node`）
- 用户说"移动知识库里的文档" → `node move`（需 `--workspace` + `--node`）
- 用户说"删除知识库里的文档/节点" → `node delete`（需 `--workspace` + `--node`）
- 用户说"把知识库分享给某人/给某人加入知识库/邀请进知识库" → `member add`（需 `--workspace` + `--users` + `--role`）
- 用户说"修改某人在知识库的权限/调整成员角色" → `member update`
- 用户说"移除知识库成员/把某人从知识库移除/删除知识库成员" → `member remove`（需 `--workspace` + `--users`）
- 用户说"知识库有哪些成员/查看知识库成员" → `member list`
- 用户说"删除知识库/移除知识库/把知识库删了" → `space delete`（需 `--workspace`）

> **跨产品路由说明**：知识库节点的**内容操作**（读取/编辑/块级操作）仍由 `dws doc` 承担：
>- 用户说"读某个知识库里的某篇文档" → 先 `node list` 拿到 nodeId，再走 **`dws doc read --node <nodeId>`**
>- 用户说"搜文件"（不指定空间） → 走 **`dws drive search`**（全局聚合搜索）

关键区分（两层模型）：
- **wiki node**（空间管理层：节点的列出/创建/复制/移动/删除/搜索）vs **doc**（内容层：读写/编辑/块级/评论/导出）vs **drive**（存储层：文件上传/下载/搜索/权限，不关心格式）
- **wiki node search**（空间内搜索，需 `--workspace`）vs **drive search**（全局搜索，聚合钉盘+文档空间）
- **wiki node create**（在空间中创建空文件实体）vs **doc create**（创建文档并写入内容）
- **wiki member**（容器级，授权整个知识库）vs **drive permission**（节点级，授权单篇文档）
  - 「我的文档」**只能用** `drive permission`，不能用 `wiki member`
- **wiki space list --type orgSpace/mySpace**（列出钉盘空间）vs **wiki space list**（默认列出知识库）

## 核心工作流

```bash
# 列出我有权访问的组织知识库
dws wiki space list --format json

# 获取「我的文档」个人空间
dws wiki space list --type myWikiSpace --format json

# 搜索知识库
dws wiki space search --query "产品" --format json

# 创建知识库
dws wiki space create --name "新项目文档" --desc "项目相关文档归档" --format json

# 查看知识库详情
dws wiki space get --workspace <workspaceId> --format json

# ── 工作流: 浏览知识库内容 ──

# 1. 获取知识库 ID
dws wiki space list --format json

# 2. 列出根目录节点
dws wiki node list --workspace <workspaceId> --format json

# 3. 进入子目录
dws wiki node list --workspace <workspaceId> --folder <parentNodeId> --format json

# 4. 读取文档内容（跨到 doc）
dws doc read --node <nodeId> --format json

# ── 工作流: 在知识库中创建文档 ──

# 1. 创建文档节点
dws wiki node create --workspace <workspaceId> --name "新方案" --format json

# 2. 创建文件夹
dws wiki node create --workspace <workspaceId> --name "方案归档" --type folder --format json

# 3. 在指定文件夹下创建
dws wiki node create --workspace <workspaceId> --name "子文档" --folder <parentNodeId> --format json

# ── 工作流: 在知识库中搜索 ──

# 在指定知识库内搜索
dws wiki node search --workspace <workspaceId> --query "方案" --format json

# 按文件类型过滤
dws wiki node search --workspace <workspaceId> --query "周报" --extensions adoc --format json

# ── 工作流: 列出钉盘空间 ──

# 列出钉盘企业空间
dws wiki space list --type orgSpace --format json

# 获取钉盘「我的文件」
dws wiki space list --type mySpace --format json

# ── 工作流: 复制/移动节点 ──

# 复制节点到另一个文件夹
dws wiki node copy --workspace <workspaceId> --node <nodeId> --folder <targetFolderId> --format json

# 移动节点到另一个文件夹
dws wiki node move --workspace <workspaceId> --node <nodeId> --folder <targetFolderId> --format json

# ── 工作流: 删除知识库节点 ──

# 删除节点（会要求确认）
dws wiki node delete --workspace <workspaceId> --node <nodeId>

# ── 工作流: 给知识库加成员 ──

# 1. 先确认知识库 ID（避免授权到「我的文档」）
dws wiki space list --format json   # 注意：不要 --type myWikiSpace

# 2. 添加成员
dws wiki member add --workspace <WS_ID> --users <UID> --role EDITOR --format json

# 3. 查看当前成员
dws wiki member list --workspace <WS_ID> --format json

# ── 工作流: 移除知识库成员 ──

# 1. 查看当前成员（只返回 name/role/type，拿不到 userId）
dws wiki member list --workspace <WS_ID> --format json

# 2. 另行按姓名反查目标成员的 userId（member list 不返回 userId）
dws contact user search --query "<姓名>" --format json

# 3. 移除成员
dws wiki member remove --workspace <WS_ID> --users <UID> --format json

# ── 工作流: 删除知识库 ──

# 1. 确认知识库信息
dws wiki space get --workspace <workspaceId> --format json

# 2. 删除知识库
dws wiki space delete --workspace <workspaceId> --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `space create` | `workspaceId` | node list / member add 的 --workspace |
| `space list` | `workspaceId` | node list / member add 的 --workspace |
| `space search` | `workspaceId` | node list / member add 的 --workspace |
| `space get` | `spaceUrl` | 分享给用户 |
| `node list` | `nodeId` | node copy/move/delete 的 --node / `dws doc read` 的 --node |
| `node search` | `nodeId` | node copy/move/delete 的 --node / `dws doc read` 的 --node |
| `node create` | `nodeId` | node copy/move/delete 的 --node / `dws doc read` 的 --node |
| `member list` | `name` / `role` / `type`（**不含 userId**）| 仅用于查看成员名单；**无法**从这里取 userId 去串联 member update/remove，需另行按姓名反查 userId（如 `dws contact user search --query "<姓名>"`）|

## 相关产品

- [doc](./doc.md) — 内容层：文档读写/编辑/块级操作/评论/导出（仅对自研文档有意义）
- [drive](./drive.md) — 存储层：文件列出/搜索/上传/下载/复制/移动/重命名/删除/权限（不关心文件格式）
