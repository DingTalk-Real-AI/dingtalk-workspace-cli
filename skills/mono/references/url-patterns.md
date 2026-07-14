# URL 格式与处理规范

## alidocs URL 分流决策（必须首先执行）

收到 `alidocs.dingtalk.com` URL 时，**必须按以下顺序判断，禁止跳过**：

1. URL 路径含 `/i/p/` → **分享短链**，禁止调用 `dws doc` 任何子命令 → 按下方 [分享短链处理](#分享短链处理) 执行
2. URL 路径含 `/i/nodes/` → **节点链接**，需探测类型 → 按下方 [alidocs URL 类型探测流程](#alidocs-url-类型探测流程) 执行
3. URL 路径含 `/spreadsheetv2/` → **电子表格直链**，直接路由到 `sheet`，将完整 URL 原样传给 `--node` 参数
4. URL 路径含 `/document/edit` 或 `/document/preview` 且 query 参数包含 `dentryKey` → **文档链接**，直接路由到 `doc`，将完整 URL 原样传给 `--node` 参数（URL 中不一定有 `type=d`，只需匹配路径和 `dentryKey` 参数即可）
5. 其他 alidocs URL 格式 → 告知用户当前暂不支持该链接格式

---

## 已知 URL 格式

需要自行拼接链接时，只能使用以下模板：

| 产品 | 用途 | URL 格式 | ID 来源 |
|------|------|----------|---------|
| `aitable` | AI表格 Base 链接 | `https://alidocs.dingtalk.com/i/nodes/{baseId}` | `base list/search/create/get` 返回的 `baseId` |
| `aitable` | AI表格模板预览 | `https://docs.dingtalk.com/table/template/{templateId}` | `template search` 返回的 `templateId` |
| `doc` | 文档链接 | `https://alidocs.dingtalk.com/i/nodes/{dentryUuid}` | `doc` 命令返回的 `dentryUuid` |
| `sheet` | 电子表格链接 | `https://alidocs.dingtalk.com/i/nodes/{dentryUuid}` | `sheet create` 返回的 `dentryUuid` |
| `sheet` | 电子表格直链 | `https://alidocs.dingtalk.com/spreadsheetv2/{key}/...?dentryKey={key}&type=s` | 用户提供的完整 URL，直接传给 `--node` |
| `doc` | 文档链接（edit/preview） | `https://alidocs.dingtalk.com/document/{edit\|preview}?...&dentryKey={key}` | 用户提供的完整 URL，直接传给 `--node` |
| `minutes` | 听记链接 | `https://shanji.dingtalk.com/app/transcribes/{taskUuid}` | `list mine/shared` 返回的 `taskUuid` |

不在此表中的产品，禁止自行拼接 URL。命令返回中包含完整链接时直接使用，否则告知用户无法提供。

## 分享短链处理

`alidocs.dingtalk.com/i/p/{shortKey}` 是钉钉文档的**对外分享短链**，`dws doc` 命令无法解析此格式。

### 识别规则

URL 路径中包含 `/i/p/` 即为分享短链（无论后面是否还有子路径），例如：
- `https://alidocs.dingtalk.com/i/p/Y7kmbokZp3pgGLq2`
- `https://alidocs.dingtalk.com/i/p/Y7kmbokZp3pgGLq2/docs/AY39rGpMPmeVNpXZevZm8OZkXKnaoNQ7`
- `https://alidocs.dingtalk.com/i/p/AbCdEfGh1234`
- `https://alidocs.dingtalk.com/i/p/AbCdEfGh1234/sheets/XYZ789`

> **关键**：只要 URL 中出现 `/i/p/`，无论后面跟什么子路径（`/docs/...`、`/sheets/...` 等），都属于分享短链，一律禁止调用 `dws doc`。

### 处理方式

**不要调用 `dws doc` 任何子命令**（包括 `doc info`、`doc read` 等），`dws` 无法解析此格式。

- **需要获取文档内容时**：使用 `read_url` 工具直接读取该链接
- **其他操作（如移动、复制、权限管理等）**：告知用户此链接为分享短链，无法直接执行复制、移动、权限管理等操作。如需保存该文档内容，建议用户在钉钉客户端中打开该页面，手动复制文本内容，然后可通过 `dws doc create` 创建一篇新文档并将内容写入

```
# 需要读取文档内容时（无论 /i/p/ 后面有没有子路径，都用 read_url）
read_url("https://alidocs.dingtalk.com/i/p/Y7kmbokZp3pgGLq2")
read_url("https://alidocs.dingtalk.com/i/p/Y7kmbokZp3pgGLq2/docs/AY39rGpMPmeVNpXZevZm8OZkXKnaoNQ7")

# 禁止（以下全部会失败，dws 无法解析任何含 /i/p/ 的 URL）
dws doc info --node "https://alidocs.dingtalk.com/i/p/Y7kmbokZp3pgGLq2" --format json
dws doc read --node "https://alidocs.dingtalk.com/i/p/Y7kmbokZp3pgGLq2/docs/AY39rGpMPmeVNpXZevZm8OZkXKnaoNQ7" --format json
```

### 当 `read_url` 返回内容不完整时

钉钉文档分享页是动态渲染的，`read_url` 可能只能获取到页面标题等有限信息，无法获取文档正文。此时**禁止猜测原因**（如"权限不足""文档为空""文档已删除"等），**禁止建议用户"提供 `/i/nodes/` 格式链接"**（分享短链和节点链接是不同体系，普通用户无法自行转换）。应直接告知用户：

> 这个链接是钉钉文档的分享短链，由于页面是动态渲染的，我无法通过该链接直接获取文档的完整正文内容。
>
> 你可以：
> 1. 在钉钉客户端中打开该文档，将正文内容复制粘贴给我
> 2. 如果文档已保存在你的文档空间中，可以告诉我文档名称，我通过 `dws doc search` 搜索后再读取

---

## alidocs URL 类型探测流程

`alidocs.dingtalk.com/i/nodes/{id}` 是钉钉文档空间的统一 URL，可能指向**文档、电子表格、多维表、文件、文件夹**等不同类型。**禁止仅凭 URL 就假定为文档**，必须先探测类型再路由到正确的产品。

### 探测步骤

```
Step 1 → dws doc info --node "<URL>" --format json
Step 2 → 从返回中提取 contentType、extension、nodeType 字段
Step 3 → 按下方路由规则映射到对应产品
```

### 路由映射表

| 条件 | 路由到产品 | 后续操作 |
|------|-----------|---------|
| `contentType=ALIDOC`, `extension=adoc` | `doc` | 按 [doc.md](./products/doc.md) 操作 |
| `contentType=ALIDOC`, `extension=axls` | `sheet` | 按 [sheet.md](./products/sheet.md) 操作（仅 `axls` 在线电子表格） |
| `contentType=ALIDOC`, `extension=able` | `aitable` | 将 nodeId 作为 baseId，按 [aitable.md](./products/aitable.md) 操作 |
| `contentType=DOCUMENT`, `extension=xlsx` / `xls` / `xlsm` / `csv` | `doc` | 必须用 `dws drive download` 下载到本地处理，禁止走 `sheet`（非在线表格，sheet 命令无法操作） |
| `contentType≠ALIDOC`, `nodeType=file` | `doc` | 调用 `dws drive download` 下载，返回文件下载链接 |
| `nodeType=folder` | `doc` | 调用 `dws doc list --folder <ID>` 列出指定文件夹直接子节点列表 |
| 以上均不匹配 | — | 告知用户当前暂不支持该类型 |

> axls vs xlsx 关键区分：
> - `axls`（钉钉在线电子表格，`contentType=ALIDOC`）→ 走 `sheet` 产品线（读/写/筛选/导出等服务端原子操作）
> - `xlsx` / `xls` / `xlsm` / `csv`（上传到文档空间的本地表格文件，`contentType=DOCUMENT`）→ 必须走 `dws drive download` 下载到本地后再解析处理，严禁错误路由到 `sheet` 产品线（sheet 命令只支持在线表格，调用 xlsx 节点会直接报错）
> - 用户想把在线表格导出为 xlsx 文件 → 开源 dws CLI 暂未暴露在线表格导出能力（旧动态 schema 曾包含 `submit_export_job` / `query_export_job`，但当前 cobra 未注册），需要在钉钉客户端手动导出 xlsx

### 示例

```bash
# 用户传入: https://alidocs.dingtalk.com/i/nodes/abc123
dws doc info --node "https://alidocs.dingtalk.com/i/nodes/abc123" --format json

# 返回 contentType=ALIDOC, extension=axls → 在线电子表格，路由到 sheet
dws sheet list --node "https://alidocs.dingtalk.com/i/nodes/abc123" --format json

# 返回 contentType≠ALIDOC, extension=xlsx/xls/csv → 本地表格文件，必须下载处理（禁止走 sheet）
dws drive download --node "https://alidocs.dingtalk.com/i/nodes/xlsx456"

# 返回 contentType≠ALIDOC, nodeType=file → 普通文件，下载
dws drive download --node "https://alidocs.dingtalk.com/i/nodes/def456"

# 返回 nodeType=folder → 文件夹，列出子节点
dws doc list --folder "https://alidocs.dingtalk.com/i/nodes/ghi789" --format json
```

### 何时可跳过探测

当用户指令中已明确指定产品（如"帮我读这个文档"、"看下这个表格的数据"），可结合用户意图**跳过探测**直接路由。仅在以下情况**必须执行探测**：
- 用户只粘贴 URL，无其他上下文
- 用户指令与 URL 实际类型可能不一致（如说"文档"但实际是表格）
- 用户直接粘贴的是原始 `alidocs` URL，且没有上游命令返回来确认类型

---

## alidocs URL probe 后能力矩阵

> 给 Agent 在用户问"那能不能 XXX"时使用——一眼看出该节点类型支持哪些操作。
> 标 ⚠️ 的项是当前 dws-opensource 用 transitional helper 实现（feat/align-yuyuan 分支），mse 端 toolOverride 落地后转为动态生成。

| extension / contentType | 读取 | 写入 | 删除 | 导出 | 权限 | 媒体 |
|-------------------------|------|------|------|------|------|------|
| **adoc**（在线文档） | `doc read` | `doc update` / `doc block update` | ⚠️ `doc delete` | ⚠️ `doc export` (→ docx) | ⚠️ `doc permission *` | ⚠️ `doc media download/insert` |
| **axls**（在线电子表格） | `sheet range read` / `sheet list` | `sheet range update` / `sheet append` | ⚠️ `doc delete`（节点删除） | `sheet export`（单命令一站式：提交→轮询→下载，可选 `--output` 落盘） | ⚠️ `doc permission *`（节点级，跨产品） | 不适用 |
| **able**（在线多维表） | `aitable base get` / `aitable record query` | `aitable record create/update` | ⚠️ `doc delete`（节点删除）或 `aitable base delete --yes` | `aitable export data --scope all --export-format excel --format json`（取 downloadUrl，`--output` 不落盘） | ⚠️ `doc permission *`（节点级） | `aitable attachment upload` |
| **xlsx / xls / xlsm / csv**（本地表格文件） | `drive download` → 本地用 xlsx skill 解析 | 不支持服务端写（先下载改本地再上传） | ⚠️ `doc delete`（节点删除） | 不需要（本身就是 xlsx） | ⚠️ `doc permission *` | 不适用 |
| **普通文件** (nodeType=file) | `drive download` | 不支持服务端写 | ⚠️ `doc delete` | 不需要 | ⚠️ `doc permission *` | 不适用 |
| **文件夹** (nodeType=folder) | `doc list --folder <URL>` | `doc create --folder <URL> ...` | ⚠️ `doc delete` | 不适用 | ⚠️ `doc permission *` | 不适用 |
| **分享短链** `/i/p/<short>` | `read_url` 兜底（外部工具） | 不适用 | 不适用 | 不适用 | 不适用 | 不适用 |

### 使用方式

```
Agent 流程：
  1. 用户给 URL  →  dws doc info --node <URL>           （路由起点）
  2. 拿到 extension / contentType / nodeType
  3. 在本矩阵查"能做什么 / 不能做什么"
  4. 不能做的直接告知用户（参考 capability-limits.md），不要重试
```

### 跨产品授权的关键判断

| 用户说 | 路由 | 不要混淆 |
|--------|------|---------|
| "把这个文档/表格/多维表分享给张三" | **节点级**：`doc permission add --node <URL> --user <UID> --role EDITOR` | 不是 `wiki member add` |
| "把张三加到这个知识库" | **容器级**：`wiki member add --workspace <WS> --user <UID> --role <ROLE>` | 不是 `doc permission add` |

> 区分依据：**doc permission 作用于单个 node（document / file / folder）；wiki member 作用于整个 workspace 容器**。同一用户在 workspace 是 EDITOR、在某个 node 上仍可被单独提升为 MANAGER（节点级覆盖容器级）。
