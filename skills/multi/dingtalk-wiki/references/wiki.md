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
dws wiki member --help
```

规则：
- 参数名不确定时 → 先 `--help`，再调用
- 报错 "unknown flag" 时 → `--help` 确认正确的 flag 名称
- 不确定某个功能是否存在时 → `dws wiki --help` 查看命令列表
- `workspaceId` 是知识库空间 ID，只能用于 `wiki space/member --workspace`、`doc --workspace` 或 `doc search --workspace-ids`；不要把它传给 `doc list --folder`，也不要使用不存在的 `--space-id`

## 命令总览

### 创建知识库
```
Usage:
  dws wiki space create [flags]
Example:
  dws wiki space create --name "产品文档库" --format json
  dws wiki space create --name "技术方案" --desc "团队技术方案归档" --format json
Flags:
      --name string          知识库名称 (必填，不超过 100 字符)
      --desc string   知识库描述 (选填，不超过 500 字符)
      --icon string          知识库图标标识 (选填)
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
      --type string        知识库类型: myWikiSpace / orgWikiSpace (默认 orgWikiSpace)
      --limit string       每页数量 1-50 (默认 20)
      --cursor string  分页游标 (首页留空)
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
  dws wiki space search --type myWikiSpace --format json
Flags:
      --query string     搜索关键词 (--type myWikiSpace 时可省略)
      --type string        知识库类型: myWikiSpace 时直接返回「我的文档」，省略则搜索组织知识库
      --limit string       返回数量 1-20 (默认 10)
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
      --workspace string    目标知识库 ID 或 URL (必填)
      --users strings   被加入的用户 userId 列表，逗号分隔 (必填，单次最多 30 个)
      --role string     授予的角色 (必填，大小写敏感，必须全大写): MANAGER (管理者) / EDITOR (可编辑) / DOWNLOADER (可下载) / READER (可阅读)
```

> **❗ 重要约束**：
> - 仅支持 USER 类型。
> - 角色枚举严格大写：MANAGER / EDITOR / DOWNLOADER / READER（OWNER 不可通过此接口添加，知识库创建者默认为所有者）。
> - 操作者需具备知识库的 OWNER 或 MANAGER 权限。
> - 「我的文档」(myWikiSpace) 是个人空间，**不支持容器级成员管理**；后端会直接拒绝。如果你的目标只是把某篇文档分享给别人，请改用 `dws doc permission add` 在节点级别授权。

### 修改知识库成员角色
```
Usage:
  dws wiki member update [flags]
Example:
  dws wiki member update --workspace <WS_ID> --users uid1 --role EDITOR
  dws wiki member update --workspace <WS_ID> --users uid1,uid2 --role READER
Flags:
      --workspace string    目标知识库 ID 或 URL (必填)
      --users strings   目标用户 userId 列表，逗号分隔 (必填，单次最多 30 个)
      --role string     新角色 (必填，大小写敏感，必须全大写): MANAGER / EDITOR / DOWNLOADER / READER
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
      --workspace string         目标知识库 ID 或 URL (必填)
      --limit int             返回数量上限，最大 200 (默认 50)
      --filter-role string   按角色过滤: MANAGER / EDITOR / DOWNLOADER / READER (选填)
```

> 接口不支持游标分页，使用 `--limit` 一次性拉取。

## 意图判断

- 用户说"创建知识库/新建知识库" → `space create`
- 用户说"查看知识库/知识库详情" → `space get`
- 用户说"我的知识库/知识库列表/有哪些知识库" → `space list`
- 用户说"搜索知识库/找知识库" → `space search`
- 用户说"我的文档/个人空间" → `space search --type myWikiSpace` 或 `space list --type myWikiSpace`
- 用户说"把知识库分享给某人/给某人加入知识库/邀请进知识库" → `member add`（需 `--workspace` + `--users` + `--role`）
- 用户说"修改某人在知识库的权限/调整成员角色" → `member update`
- 用户说"知识库有哪些成员/查看知识库成员" → `member list`

> ** 跨产品路由（重要）**：`dws wiki` 只管知识库容器（space/member），**不提供查看知识库文件/文档的能力**。以下意图必须走 `dws doc`，不要在 wiki 下尝试 `node`/`file`/`list` 等子命令：
>- 用户说"知识库下的文件/知识库里有哪些文档/浏览知识库内容" → 先用 `dws wiki space list` 或 `space search` 拿到 `workspaceId`，再走 **`dws doc list --workspace <workspaceId>`**
>- 用户说"读某个知识库里的某篇文档" → 先 `dws wiki space list/search` 拿 `workspaceId`，再 `dws doc search --query "<文档名>" --workspace-ids <workspaceId> --format json` 找 `nodeId`，最后 **`dws doc read --node <nodeId> --format json`**
>- 用户说"在知识库里搜文档" → 走 **`dws doc search --workspace-ids <workspaceId>`**
>- 用户说"在知识库里创建文档" → 走 **`dws doc create --workspace <workspaceId>`**

> **禁止反模式**：
>- `dws doc list --space-id <workspaceId>`：`doc list` 没有 `--space-id`
>- `dws doc list --folder <workspaceId>`：`--folder` 只接受文件夹 `nodeId` / 文件夹 URL，不接受知识库 `workspaceId`
>- `doc get --node <nodeId>`：读取正文使用 `dws doc read --node <nodeId> --format json`
>- 多个知识库同名时，不要默认取第一个；用 `doc list --workspace` / `doc search --workspace-ids` / `doc read` 验证哪个空间包含目标文档或目标文件夹

关键区分：
- wiki(知识库空间级管理：创建/查询/列出/搜索/成员管理) vs doc(文档内容级操作：搜索/读写/编辑/节点级权限)
- wiki space(知识库容器) vs drive(钉盘文件存储/上传/下载)
- **wiki member**（容器级，授权整个知识库）vs **doc permission**（节点级，授权单篇文档）
  - 「我的文档」**只能用** `doc permission`，不能用 `wiki member`

## 核心工作流

```bash
# 列出我有权访问的组织知识库
dws wiki space list --format json

# 获取「我的文档」个人空间
dws wiki space list --type myWikiSpace --format json

# 搜索知识库
dws wiki space search --query "产品" --format json

# 搜索「我的文档」
dws wiki space search --type myWikiSpace --format json

# 创建知识库
dws wiki space create --name "新项目文档" --desc "项目相关文档归档" --format json

# 查看知识库详情
dws wiki space get --workspace <workspaceId> --format json

# ── 工作流: 读取某个知识库里的指定文档 ──

# 1. 找知识库空间，取 workspaceId
dws wiki space search --query "评测记录" --format json

# 2. 在该知识库内搜索文档，取 nodeId
dws doc search --query "MinHash 学习笔记" --workspace-ids <workspaceId> --format json

# 3. 读取正文
dws doc read --node <nodeId> --format json

# ── 工作流: 给知识库加成员 ──

# 1. 先确认知识库 ID（避免授权到「我的文档」）
dws wiki space list --format json   # 注意：不要 --type myWikiSpace

# 2. 添加成员
dws wiki member add --workspace <WS_ID> --users <UID> --role EDITOR --format json

# 3. 查看当前成员
dws wiki member list --workspace <WS_ID> --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `space create` | `workspaceId` | space get 的 --workspace / member add 的 --workspace |
| `space list` | `workspaceId` | space get 的 --workspace / member add 的 --workspace |
| `space search` | `workspaceId` | space get 的 --workspace / member add 的 --workspace |
| `space get` | `spaceUrl` | 分享给用户 |
| `member list` | `userId` | member update 的 --users |

## 相关产品

- [doc](./doc.md) — 文档内容级操作（搜索/读写/编辑文档、知识库内文档管理）
- [drive](./drive.md) — 钉盘文件存储/上传/下载
