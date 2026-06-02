# doc permission（文档权限：add / update / list）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../doc.md`](../doc.md) — 命令路由 + 场景索引 + 意图判断 + 工作流

> **关键区分**：
> - "把**某篇文档**授权给某人" → `doc permission add`（节点级，包括「我的文档」下的文档都支持）
> - "把**某个知识库**整体授权给某人" → `wiki member add`（容器级，但**「我的文档」个人空间不支持**）

---

## doc permission add（节点级授权）

```
Usage:
  dws doc permission add [flags]
Example:
  dws doc permission add --node <DOC_ID> --user uid1 --role READER
  dws doc permission add --node <DOC_ID> --user uid1,uid2 --role EDITOR
  dws doc permission add --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --user uid1 --role MANAGER
Flags:
      --node string        目标文档/文件夹的 ID 或 URL (必填)
      --user strings       被授权的用户 userId 列表，逗号分隔 (必填，单次最多 30 个)
      --role string        授予的角色 (必填，大小写敏感，必须全大写): MANAGER (管理者) / EDITOR (可编辑) / DOWNLOADER (可下载) / READER (可阅读)
      --workspace string   所属知识库 ID (选填，仅用于辅助构造返回的 docUrl，业务实际依赖 nodeId)
```

> **重要约束**：
>
> - 仅支持 USER 类型授权。
> - 角色枚举严格大写：`MANAGER` / `EDITOR` / `DOWNLOADER` / `READER`（`OWNER` 不可通过此接口添加）。
> - 操作者需在该节点具备「可编辑（EDITOR）」及以上角色（OWNER / MANAGER / EDITOR）。
> - 授权对象是文档节点本身，不需要也不应该用 `wiki member add`（那个是知识库容器级授权）。

---

## doc permission update（修改权限）

```
Usage:
  dws doc permission update [flags]
Example:
  dws doc permission update --node <DOC_ID> --user uid1 --role EDITOR
  dws doc permission update --node <DOC_ID> --user uid1,uid2 --role READER
Flags:
      --node string        目标文档/文件夹的 ID 或 URL (必填)
      --user strings       目标用户 userId 列表，逗号分隔 (必填，单次最多 30 个)
      --role string        新角色 (必填，大小写敏感，必须全大写): MANAGER / EDITOR / DOWNLOADER / READER
      --workspace string   所属知识库 ID (选填)
```

---

## doc permission list（列出权限）

```
Usage:
  dws doc permission list [flags]
Example:
  dws doc permission list --node <DOC_ID>
  dws doc permission list --node <DOC_ID> --max-results 50
  dws doc permission list --node <DOC_ID> --filter-role EDITOR
Flags:
      --node string          目标文档/文件夹的 ID 或 URL (必填)
      --workspace string     所属知识库 ID (选填)
      --max-results int      返回数量上限，最大 200 (默认 50)
      --filter-role string   按角色过滤: MANAGER / EDITOR / DOWNLOADER / READER (选填)
```

> 接口不支持游标分页，使用 `--max-results` 一次性拉取。

## 关键说明

- 三个命令的 `--role` / `--filter-role` 都是**大小写敏感**且**必须全大写**：`MANAGER` / `EDITOR` / `DOWNLOADER` / `READER`。
- `--user` 单次最多 30 个 userId，逗号分隔。
- 需要 userId 时，使用 `dws contact user search --query "<姓名>"`（跨产品命令）取得。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| `dws contact user search` 的 `userId` | `permission add/update` 的 `--user` |
| [`./doc-search.md`](./doc-search.md) / [`./doc-list.md`](./doc-list.md) 的 `nodeId` | 三命令的 `--node` |

## 常用模板

```bash
# 查看谁有权限
dws doc permission list --node <DOC_ID> --format json

# 按角色过滤查看
dws doc permission list --node <DOC_ID> --filter-role EDITOR --format json

# 拉取更多结果（默认 50，最大 200）
dws doc permission list --node <DOC_ID> --max-results 200 --format json

# 给单人开 READER 权限
dws doc permission add --node <DOC_ID> --user <uid1> --role READER --format json

# 给多人开 EDITOR 权限（最多 30 个 uid）
dws doc permission add --node <DOC_ID> --user <uid1>,<uid2>,<uid3> --role EDITOR --format json

# 设为管理者
dws doc permission add --node <DOC_ID> --user <uid1> --role MANAGER --format json

# 修改某人角色（READER → EDITOR）
dws doc permission update --node <DOC_ID> --user <uid1> --role EDITOR --format json

# 通过 alidocs URL 直接授权
dws doc permission add --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --user <uid1> --role READER --format json
```

## 参考

- [`../doc.md` §意图判断](../doc.md#意图判断)（如何路由到本命令族）
- `dws contact user search`（取 userId 的入口，跨产品命令）
- [`wiki.md`](../../../dingtalk-wiki/references/wiki.md)（知识库容器级授权 `wiki member add`，与本节点级权限相区分）
