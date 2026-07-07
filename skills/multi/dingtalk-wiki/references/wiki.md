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

`dws wiki` 有三个命令族：`space`（知识库容器）、`node`（知识库内节点：文档/文件夹/表格等）、`member`（容器级成员）。

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

### 列出知识库
```
Usage:
  dws wiki space list [flags]
Example:
  dws wiki space list --format json
  dws wiki space list --type myWikiSpace --format json
  dws wiki space list --type orgWikiSpace --limit 50 --format json
Flags:
      --type string     空间类型: orgWikiSpace(默认) / myWikiSpace / orgSpace(钉盘企业空间) / mySpace(钉盘我的文件)
      --limit string    每页数量 1-50 (默认 20)
      --cursor string   分页游标 (首页留空)
```

- `myWikiSpace`：返回当前用户的「我的文档」个人空间（固定 1 条，不支持分页）
- `orgWikiSpace`（默认）：返回组织内有权访问的知识库列表，支持分页

### 搜索知识库
```
Usage:
  dws wiki space search [flags]
Example:
  dws wiki space search --query "产品文档" --format json
  dws wiki space search --query "技术方案" --limit 20 --format json
  dws wiki space list --type myWikiSpace --format json
Flags:
      --query string   搜索关键词 (搜索组织知识库时必填)
      --type string    知识库类型: myWikiSpace 时直接返回「我的文档」，省略则搜索组织知识库
      --limit string   返回数量 1-20 (默认 10)
```

当 `--type myWikiSpace` 时，忽略 `--query`，直接返回「我的文档」个人空间。

### 删除知识库

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws wiki space delete [flags]
Example:
  dws wiki space delete --workspace <workspaceId>
Flags:
      --workspace string   知识库 ID 或 URL (必填)
```

将指定知识库移入回收站。操作者必须具备知识库的 OWNER 角色。

### 列出知识库节点
```
Usage:
  dws wiki node list [flags]
Example:
  dws wiki node list --workspace <workspaceId> --format json
  dws wiki node list --workspace <workspaceId> --folder <parentNodeId> --format json
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
      --type string        节点类型 (默认 adoc)，服务端支持: adoc(在线文档) / axls(在线表格) / able(多维表) / appt(在线演示) / adraw(白板) / amind(脑图) / folder(文件夹)
      --folder string      父节点 nodeId (选填，不传则在根目录创建)
```

> ⚠️ **type 枚举**：服务端支持 `adoc` / `axls` / `able` / `appt` / `adraw` / `amind` / `folder` 七种；**`asheet` 不支持**，传了会被后端拒绝（报「不支持的文件类型 asheet」）。创建在线表格请用 `axls`。

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
Flags:
      --workspace string   知识库 ID (必填，用于权限校验)
      --node string        节点 ID (必填)
```

将知识库中的节点移入回收站。权限要求: 对节点有「管理」权限。

### 在知识库中搜索节点
```
Usage:
  dws wiki node search [flags]
Example:
  dws wiki node search --workspace <workspaceId> --query "方案" --format json
  dws wiki node search --workspace <workspaceId> --query "设计" --extensions adoc,axls --format json
Flags:
      --workspace string    知识库 ID (必填)
      --query string        搜索关键词 (必填)
      --extensions strings  按文件扩展名过滤，逗号分隔 (如 adoc,axls,pdf) (选填)
      --limit int           每页数量 (默认 10，最大 30) (选填)
      --cursor string       分页游标 (选填)
```

在指定知识库空间内搜索节点（需 `--workspace`）。全局聚合搜索走 `dingtalk-drive` 的 `drive search`。

### 添加知识库成员（容器级授权）
```
Usage:
  dws wiki member add [flags]
Example:
  dws wiki member add --workspace <WS_ID> --users uid1 --role READER
  dws wiki member add --workspace <WS_ID> --users uid1,uid2 --role EDITOR
Flags:
      --workspace string   目标知识库 ID 或 URL (必填)
      --users string       被加入的用户 userId 列表，逗号分隔 (必填，单次最多 30 个)
      --role string        授予的角色 (必填，大小写不敏感): MANAGER / EDITOR / DOWNLOADER / READER
```

> **❗ 重要约束**：
> - 仅支持 USER 类型；角色枚举 MANAGER / EDITOR / DOWNLOADER / READER（OWNER 不可通过此接口添加，知识库创建者默认为所有者）。
> - `--role` **大小写不敏感**，`editor` 与 `EDITOR` 都能通过校验（CLI 会归一化成大写）。
> - 操作者需具备知识库的 OWNER 或 MANAGER 权限。
> - 「我的文档」(myWikiSpace) 是个人空间，**不支持容器级成员管理**；后端会直接拒绝。节点级权限授权在开源 dws 暂不支持，请在钉钉客户端文档中通过「分享」设置。

### 修改知识库成员角色
```
Usage:
  dws wiki member update [flags]
Example:
  dws wiki member update --workspace <WS_ID> --users uid1 --role EDITOR
Flags:
      --workspace string   目标知识库 ID 或 URL (必填)
      --users string       目标用户 userId 列表，逗号分隔 (必填，单次最多 30 个)
      --role string        新角色 (必填，大小写不敏感): MANAGER / EDITOR / DOWNLOADER / READER
```

### 移除知识库成员
```
Usage:
  dws wiki member remove [flags]
Example:
  dws wiki member remove --workspace <WS_ID> --users uid1
Flags:
      --workspace string   目标知识库 ID 或 URL (必填)
      --users string       被移除的用户 userId 列表，逗号分隔 (必填，单次最多 30 个)
```

> OWNER 角色不可通过此接口移除；操作者需具备 OWNER 或 MANAGER 权限。

### 列出知识库成员
```
Usage:
  dws wiki member list [flags]
Example:
  dws wiki member list --workspace <WS_ID>
  dws wiki member list --workspace <WS_ID> --filter-role EDITOR
Flags:
      --workspace string     目标知识库 ID 或 URL (必填)
      --limit int            返回成员数上限，默认 30，最大 200
      --filter-role string   按角色过滤，逗号分隔: OWNER / MANAGER / EDITOR / DOWNLOADER / READER (选填)
```

> 接口不支持游标分页，使用 `--limit` 一次性拉取。

> ⚠️ **返回字段限制**：`member list` 每条只返回 `name` / `role` / `type` 三个字段，**不含 userId**（服务端不返回）。因此**无法**从 `member list` 拿到 userId 再去串联 `member update` / `member remove`。要对某人改角色 / 移除，需另行拿到其 userId（例如用 `dws contact user search --query "<姓名>"` 按姓名反查）。

## 意图判断

- 用户说"创建知识库/新建知识库" → `space create`
- 用户说"查看知识库/知识库详情" → `space get`
- 用户说"我的知识库/知识库列表/有哪些知识库" → `space list`
- 用户说"搜索知识库/找知识库" → `space search`
- 用户说"我的文档/个人空间" → `space list --type myWikiSpace`
- 用户说"删除知识库/移除知识库" → `space delete`（需 `--workspace`）
- 用户说"知识库下的文件/知识库里有哪些文档/浏览知识库内容" → `node list`（需 `--workspace`）
- 用户说"在知识库里搜文档/空间内搜索" → `node search`（需 `--workspace` + `--query`）
- 用户说"在知识库里创建文档/新建文件夹" → `node create`（需 `--workspace` + `--name`）
- 用户说"复制/移动知识库里的文档" → `node copy` / `node move`（需 `--workspace` + `--node`）
- 用户说"删除知识库里的文档/节点" → `node delete`（需 `--workspace` + `--node`）
- 用户说"把知识库分享给某人/邀请进知识库" → `member add`（需 `--workspace` + `--users` + `--role`）
- 用户说"修改某人在知识库的权限/调整成员角色" → `member update`
- 用户说"移除知识库成员/把某人从知识库移除" → `member remove`（需 `--workspace` + `--users`）
- 用户说"知识库有哪些成员/查看知识库成员" → `member list`

> **跨产品路由**：`dws wiki node` 负责知识库内节点的**列出/创建/复制/移动/删除/搜索**（空间管理层）；节点的**内容读写**（读取/编辑/块级操作）切到 `dingtalk-doc`：
> - 用户说"读某个知识库里的某篇文档" → 先用 `node list` / `node search` 拿到 `nodeId`，再切到 `dingtalk-doc` 用 `dws doc read --node <nodeId>`
> - 用户说"搜文件"（不指定空间） → 切到 `dingtalk-drive` 用 `dws drive search`（全局聚合搜索）

关键区分：
- **wiki node**（空间管理层：节点列出/创建/复制/移动/删除/搜索）vs **doc**（内容层：读写/编辑/块级/评论/导出）vs **drive**（存储层：上传/下载/全局搜索/权限）
- **wiki node search**（空间内搜索，需 `--workspace`）vs **drive search**（全局聚合搜索）
- **wiki member**（容器级，授权整个知识库）；「我的文档」不支持容器级成员管理

## 核心工作流

```bash
# 列出我有权访问的组织知识库
dws wiki space list --format json

# 搜索知识库
dws wiki space search --query "产品" --format json

# 创建知识库
dws wiki space create --name "新项目文档" --desc "项目相关文档归档" --format json

# ── 工作流: 浏览知识库内容 ──
dws wiki space list --format json                                  # 1. 取 workspaceId
dws wiki node list --workspace <workspaceId> --format json         # 2. 列根目录节点
dws wiki node list --workspace <workspaceId> --folder <nodeId> --format json  # 3. 进子目录
# 4. 读取文档内容 → 切到 dingtalk-doc: dws doc read --node <nodeId> --format json

# ── 工作流: 在知识库中创建文档/文件夹 ──
dws wiki node create --workspace <workspaceId> --name "新方案" --format json
dws wiki node create --workspace <workspaceId> --name "方案归档" --type folder --format json

# ── 工作流: 在知识库中搜索 ──
dws wiki node search --workspace <workspaceId> --query "方案" --format json

# ── 工作流: 给知识库加成员 ──
dws wiki space list --format json                                  # 1. 确认 workspaceId（不要 --type myWikiSpace）
dws wiki member add --workspace <WS_ID> --users <UID> --role EDITOR --format json  # 2. 加成员
dws wiki member list --workspace <WS_ID> --format json             # 3. 查看当前成员

# ── 工作流: 移除知识库成员 ──
dws wiki member list --workspace <WS_ID> --format json             # 1. 查看成员（只返回 name/role/type，无 userId）
dws contact user search --query "<姓名>" --format json             # 2. 按姓名反查 userId
dws wiki member remove --workspace <WS_ID> --users <UID> --format json  # 3. 移除
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

- [doc](../../dingtalk-doc/references/doc.md) — 内容层：文档读写/编辑/块级操作/评论/导出
- [drive](../../dingtalk-drive/references/drive.md) — 存储层：文件上传/下载/全局搜索/权限
