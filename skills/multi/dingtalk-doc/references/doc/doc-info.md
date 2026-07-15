# doc info（获取文档元信息 + URL 解析）

> **前置条件（MUST READ）：** 执行本命令前，必须先用 Read 工具读取以下文件：
> 1. [`../doc.md`](../doc.md) — 命令路由 + 场景索引 + 意图判断 + 工作流
> 2. [`../../url-patterns.md`](../../url-patterns.md) — 仅当用户原始 `alidocs` URL 需要 probe 时
>
> **同任务常配合**：`dws drive search` / `dws wiki node search`（先定位 nodeId）/ [`doc-read.md`](./doc-read.md)（确认是 ALIDOC 后读正文）

## 命令格式

```
Usage:
  dws doc info [flags]
Example:
  dws doc info --node <DOC_ID>
  dws doc info --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>"
  dws doc info --node "https://alidocs.dingtalk.com/document/edit?dentryKey=<DENTRY_KEY>"
  dws doc info --node "https://alidocs.dingtalk.com/document/preview?dentryKey=<DENTRY_KEY>"
Flags:
      --node string   文档 ID 或 URL (必填)
```

## 文件内容获取路由规则

> 当用户请求"分析/查看/读取某个文件内容"时，**必须先调用 `dws doc info` 获取文件元数据**，再根据返回的 `contentType` 和 `extension` 字段选择对应链路：

| contentType | extension | 操作 | 命令 |
|-------------|-----------|------|------|
| ALIDOC | adoc | 在线获取 Markdown 内容 | `dws doc read --node <ID>` |
| ALIDOC | axls | 在线读取表格数据 | `dws sheet list` → `dws sheet range read` |
| ALIDOC | able | 在线查询多维表格记录 | `dws aitable table list` → `dws aitable record query` |
| 非 ALIDOC | — | **不支持在线分析** | 告知用户需下载到本地后查看 |

**关键规则**：非 ALIDOC 类型文件（PDF/Word/图片/视频等）不支持在线分析，用户可以选择下载后本地查看。

## URL 识别与 DOC_ID 提取

当用户输入包含钉钉文档 URL 时，**必须先识别并提取 DOC_ID**，再判断意图。

补充：如果这是用户直接提供的原始 `alidocs` URL，必须先按 [链接规范](../../url-patterns.md#alidocs-url-类型探测流程) probe 一次确认真实类型，再判断是否继续走 `doc`。

### 支持的 URL 格式

| 格式 | 示例 | DOC_ID 提取方式 |
|------|------|----------------|
| `alidocs.dingtalk.com/i/nodes/{id}` | `https://alidocs.dingtalk.com/i/nodes/9E05BDRVQePjzLkZt2p2vE7kV63zgkYA` | 取 URL 路径最后一段：`9E05BDRVQePjzLkZt2p2vE7kV63zgkYA` |
| `alidocs.dingtalk.com/i/nodes/{id}?queryParams` | `https://alidocs.dingtalk.com/i/nodes/abc123?doc_type=wiki_doc` | 忽略 query 参数，取路径最后一段：`abc123` |

### 提取规则

1. 匹配 URL 中 `alidocs.dingtalk.com` 域名
2. 取 URL path 的最后一段作为 DOC_ID（去掉 query string 和 fragment）
3. 提取出的 DOC_ID 可直接用于所有 `--node` 参数，也可将完整 URL 传给 `--node`（CLI 会自动解析）
4. 对用户直接提供的原始 `alidocs` URL，先按 [链接规范](../../url-patterns.md#alidocs-url-类型探测流程) 执行 probe；只有 probe 确认是 `adoc` / `file` / `folder` 时，才继续走 `doc`

## ID 边界与参数映射

- `nodeId` 是 `doc` 命令的统一节点标识；文档、文件夹、文件都通过 `nodeId` 或完整 `alidocs` URL 传给 `--node` / `--folder`。
- `dentryUuid` 是 `alidocs` URL `/i/nodes/{dentryUuid}` 的最后一段，在 `doc` 场景中等价于可传入 CLI 的 `nodeId`；不要把它改写成数字 ID。
- `dentryId` 通常是纯数字，**不是** `doc` 的 `nodeId`，也不是 `doc --folder` 的目标文件夹 ID；不要把数字 `dentryId` 当作 `--node`、`--folder` 或 `--parent-id` 使用。
- `parentId` / `--parent-id` 不是 `doc` 命令参数；`doc` 里目标父文件夹统一使用 `--folder <folderNodeId或folderUrl>`，目标知识库使用 `--workspace <workspaceId或workspaceUrl>`。
- 如果上下文只有数字 `dentryId`，但用户要读、改、移动、复制、重命名文档，先通过 `dws drive search` / `dws drive list` / 用户提供的 `alidocs` URL 获取 `nodeId` / `dentryUuid`，不要用数字 `dentryId` 重试为父目录参数。

## 处理流程

```
用户输入含 alidocs.dingtalk.com URL
  → 若是用户直接提供的原始 URL，先按链接规范做 probe
  → 提取 DOC_ID（URL 路径最后一段）
  → 结合用户意图选择命令（doc 默认 read，folder 默认 list，file 默认 download）
  → 将 DOC_ID 传给 --node 参数
```

## 获取 nodeId 的三种方式（按场景选择，**无需全部执行**）

执行 `read` / `info` / `update` / `copy` / `move` / `rename` / `delete` / `block` 等所有需要 `--node` 的命令前，按以下顺序确定 nodeId 来源，**命中即停**，不要走多余的 search/list 浪费一次调用：

| 方式 | 触发条件 | 操作 |
|------|----------|------|
| **A** | 用户**直接提供文档 URL 或 nodeId** | **直接传给 `--node`**，无需额外查询；优先使用此方式 |
| **B** | 用户给出关键字 / 文档名 | `dws drive search --query "<关键字>" --format json` 或 `dws wiki node search --workspace <WS_ID> --keyword "<关键字>"` 从返回中提取 nodeId |
| **C** | 用户指向某个文件夹下的文档 | `dws drive list --workspace <WS_ID> --format json` 或 `dws wiki node list --workspace <WS_ID>` 从返回中提取 |

> **关键节省**：方式 A 命中时，禁止再调 search/list "确认一下" —— 用户提供的 URL/nodeId 本身就是权威输入。同理，`--folder` 也支持 alidocs 文件夹 URL 直传，不要先 search 把 URL 解析成纯数字 ID 再传。

## nodeId 多格式说明

所有 `--node` 参数同时支持以下格式，系统自动识别：

- **文档 ID**: 字母数字字符串，如 `9E05BDRVQePjzLkZt2p2vE7kV63zgkYA`
- **文档 URL**: `https://alidocs.dingtalk.com/i/nodes/{dentryUuid}`，如 `https://alidocs.dingtalk.com/i/nodes/9E05BDRVQePjzLkZt2p2vE7kV63zgkYA`
- **文档链接（edit/preview）**: `https://alidocs.dingtalk.com/document/{edit|preview}?...&dentryKey={key}`（必须传入完整 URL，不要提取其中的 query 参数单独使用）

以下命令效果相同：

```bash
dws doc read --node 9E05BDRVQePjzLkZt2p2vE7kV63zgkYA
dws doc read --node "https://alidocs.dingtalk.com/i/nodes/9E05BDRVQePjzLkZt2p2vE7kV63zgkYA"
dws doc read --node "https://alidocs.dingtalk.com/document/edit?dentryKey=wo1g3x54FzVEJ5yE"
dws doc read --node "https://alidocs.dingtalk.com/document/preview?cid=74993670680&type=d&docKey=Pd6l2Z7V8ZWydl7M&dentryKey=rBGBr2r1HmwanAGW"
```

> **注意**：`document/edit` 和 `document/preview` 格式 URL 中的 `dentryKey` 参数值不是合法的独立 nodeId，禁止提取后单独使用，必须传入完整 URL。URL 中可能包含 `utm_source`、`chInfo` 等追踪参数，无需手动去除，直接传入完整 URL 即可。

`--folder` 参数同样支持 alidocs 文件夹 URL 或文档文件夹 nodeId。

不要把纯数字 `dentryId` 当成这里的 ID。需要父文件夹时，使用文件夹的 `nodeId` / `dentryUuid` / URL 传给 `--folder`；不能改用 `--parent-id`。如果上一步只拿到了 drive/chat 链路里的纯数字 `dentryId`、`spaceId` 或 `parent-id`，说明还没有拿到 doc 文件夹，应该省略 `--folder` 使用默认文档根目录，或先通过 `dws drive search` / `dws drive list` 找到文档文件夹 nodeId。

## 上下文传递

| 从返回中提取 | 用于 |
|-------------|------|
| `contentType` + `extension` | 选择 [`./doc-read.md`](./doc-read.md) / `dws sheet ...` / `dws aitable ...` / `dws drive download`（非 ALIDOC 走存储层下载） |
| `nodeId` / `docUrl` | 后续所有 `--node` 入参 |

## 常用模板

```bash
# 标准用法（nodeId 直传）
dws doc info --node <DOC_ID> --format json

# alidocs URL 直传（CLI 自动解析）
dws doc info --node "https://alidocs.dingtalk.com/i/nodes/<DOC_UUID>" --format json

# document/edit URL 直传（必须完整 URL，禁止单独提取 dentryKey）
dws doc info --node "https://alidocs.dingtalk.com/document/edit?dentryKey=<DENTRY_KEY>" --format json

# document/preview URL 直传
dws doc info --node "https://alidocs.dingtalk.com/document/preview?dentryKey=<DENTRY_KEY>" --format json
```

**反例**（这些 ID **不能**直接用于 `--node`）：

```bash
# ❌ 纯数字 dentryId（drive 链路）
dws doc info --node 1234567890123   # 错误

# ❌ 单独的 dentryKey（必须带完整 URL）
dws doc info --node "wo1g3x54FzVEJ5yE"   # 错误

# ❌ workspaceId（应作为 --workspace 用）
dws doc info --node <WS_ID>   # 错误
```

## 参考

- [`../doc.md` §意图判断](../doc.md#意图判断)（如何路由到本命令）
- `dws drive search` / `dws wiki node search`（前置：定位 nodeId 的搜索入口，详见 [`../drive.md`](../drive.md) / [`../wiki.md`](../wiki.md)）
- [`./doc-read.md`](./doc-read.md)（contentType=ALIDOC + extension=adoc 的后续命令）
- [`../../url-patterns.md`](../../url-patterns.md)（用户原始 alidocs URL 的 probe 流程）
