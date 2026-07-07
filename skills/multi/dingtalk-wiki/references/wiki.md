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

## 命令总览

### 创建知识库
```
Usage:
  dws wiki space create [flags]
Example:
  dws wiki space create --name "产品文档库" --format json
  dws wiki space create --name "技术方案" --description "团队技术方案归档" --format json
Flags:
      --name string          知识库名称 (必填，不超过 100 字符)
      --description string   知识库描述 (选填，不超过 500 字符)
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
      --page-token string  分页游标 (首页留空)
```

- `myWikiSpace`：返回当前用户的「我的文档」个人空间（固定 1 条，不支持分页）
- `orgWikiSpace`（默认）：返回组织内有权访问的知识库列表，支持分页

### 搜索知识库
```
Usage:
  dws wiki space search [flags]
Example:
  dws wiki space search --keyword "产品文档" --format json
  dws wiki space search --keyword "技术方案" --limit 20 --format json
  dws wiki space list --type myWikiSpace --format json
Flags:
      --keyword string     搜索关键词 (--type myWikiSpace 时可省略)
      --type string        知识库类型: myWikiSpace 时直接返回「我的文档」，省略则搜索组织知识库
      --limit string       返回数量 1-20 (默认 10)
```

当 `--type myWikiSpace` 时，忽略 `--keyword`，直接返回「我的文档」个人空间。

### 添加知识库成员（容器级授权）
```
Usage:
  dws wiki member add [flags]
Example:
  dws wiki member add --workspace <WS_ID> --user uid1 --role READER
  dws wiki member add --workspace <WS_ID> --user uid1,uid2 --role EDITOR
  dws wiki member add --workspace "https://alidocs.dingtalk.com/i/spaces/<WS_ID>/overview" --user uid1 --role MANAGER
Flags:
      --workspace string 目标知识库 ID 或 URL (必填)
      --user string     被加入的用户 userId 列表，逗号分隔 (必填，单次最多 30 个)
      --role string     授予的角色 (必填，大小写敏感，必须全大写): MANAGER (管理者) / EDITOR (可编辑) / DOWNLOADER (可下载) / READER (可阅读)
```

> **❗ 重要约束**：
> - 仅支持 USER 类型。
> - 角色枚举严格大写：MANAGER / EDITOR / DOWNLOADER / READER（OWNER 不可通过此接口添加，知识库创建者默认为所有者）。
> - 操作者需具备知识库的 OWNER 或 MANAGER 权限。
> - 「我的文档」(myWikiSpace) 是个人空间，**不支持容器级成员管理**；后端会直接拒绝。如果你的目标只是把某篇文档分享给别人，节点级权限授权在开源 dws v1.0.30 暂不支持，请在钉钉客户端文档中通过「分享」设置。

### 修改知识库成员角色
```
Usage:
  dws wiki member update [flags]
Example:
  dws wiki member update --workspace <WS_ID> --user uid1 --role EDITOR
  dws wiki member update --workspace <WS_ID> --user uid1,uid2 --role READER
Flags:
      --workspace string 目标知识库 ID 或 URL (必填)
      --user string     目标用户 userId 列表，逗号分隔 (必填，单次最多 30 个)
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
      --workspace string     目标知识库 ID 或 URL (必填)
      --limit int            返回成员数上限
      --filter-role string   按角色过滤，逗号分隔: MANAGER / EDITOR / DOWNLOADER / READER (选填)
```

> 接口不支持游标分页，使用 `--limit` 一次性拉取。

## 意图判断

- 用户说"创建知识库/新建知识库" → `space create`
- 用户说"查看知识库/知识库详情" → `space get`
- 用户说"我的知识库/知识库列表/有哪些知识库" → `space list`
- 用户说"搜索知识库/找知识库" → `space search`
- 用户说"我的文档/个人空间" → `space list --type myWikiSpace`
- 用户说"把知识库分享给某人/给某人加入知识库/邀请进知识库" → `member add`（需 `--workspace` + `--user` + `--role`）
- 用户说"修改某人在知识库的权限/调整成员角色" → `member update`
- 用户说"知识库有哪些成员/查看知识库成员" → `member list`

> **跨产品路由（重要）**：`dws wiki` 只管知识库容器（space/member），**不提供查看知识库文件/文档的能力**。以下意图必须走 `dws doc`，不要在 wiki 下尝试 `node`/`file`/`list` 等子命令：
> - 用户说"知识库下的文件/知识库里有哪些文档/浏览知识库内容" → 先用 `dws wiki space list` 或 `space search` 拿到 `workspaceId`，再走 **`dws doc list --workspace <workspaceId>`**
> - 用户说"读某个知识库里的某篇文档" → 先通过上面的方式找到 nodeId，再走 **`dws doc read --node <nodeId>`**
> - 用户说"在知识库里搜文档" → 走 **`dws doc search --workspace-ids <workspaceId>`**
> - 用户说"在知识库里创建文档" → 走 **`dws doc create --workspace <workspaceId>`**

关键区分：
- wiki(知识库空间级管理：创建/查询/列出/搜索/成员管理) vs doc(文档内容级操作：搜索/读写/编辑/节点级权限)
- wiki space(知识库容器) vs drive(钉盘文件存储/上传/下载)
- **wiki member**（容器级，授权整个知识库）；节点级 `doc permission` 在开源 dws v1.0.30 不可用
  - 「我的文档」当前开源版本不能通过 CLI 改权限；在钉钉客户端用「分享」设置

## 核心工作流

```bash
# 列出我有权访问的组织知识库
dws wiki space list --format json

# 获取「我的文档」个人空间
dws wiki space list --type myWikiSpace --format json

# 搜索知识库
dws wiki space search --keyword "产品" --format json

# 搜索「我的文档」
dws wiki space list --type myWikiSpace --format json

# 创建知识库
dws wiki space create --name "新项目文档" --description "项目相关文档归档" --format json

# 查看知识库详情
dws wiki space get --workspace <workspaceId> --format json

# ── 工作流: 给知识库加成员 ──

# 1. 先确认知识库 ID（避免授权到「我的文档」）
dws wiki space list --format json   # 注意：不要 --type myWikiSpace

# 2. 添加成员
dws wiki member add --workspace <WS_ID> --user <UID> --role EDITOR --format json

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
| `member list` | `userId` | member update 的 --user |

## 相关产品

- [doc](./doc.md) — 文档内容级操作（搜索/读写/编辑文档、知识库内文档管理）
- [drive](./drive.md) — 钉盘文件存储/上传/下载
