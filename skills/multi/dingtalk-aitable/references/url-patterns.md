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
| `aitable` | AI表格指定数据表链接 | `https://alidocs.dingtalk.com/i/nodes/{baseId}?iframeQuery=sheetId%3D{tableId}` | `baseId` + `table create/get` 或 `base get` 返回的 `tableId` |
| `aitable` | AI表格指定数据表+视图链接 | `https://alidocs.dingtalk.com/i/nodes/{baseId}?iframeQuery=sheetId%3D{tableId}%26viewId%3D{viewId}` | `baseId` + `tableId` + `view create/get` 返回的 `viewId` |
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
> 2. 如果文档已保存在你的文档空间中，可以告诉我文档名称，我通过 `dws drive search` 搜索后再读取

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
| `contentType=ALIDOC`, `extension=adoc` | `doc` | 加载 `dingtalk-doc` 操作内容 |
| `contentType=ALIDOC`, `extension=axls` | `sheet` | 加载 `dingtalk-sheet` 操作（仅 `axls` 在线电子表格） |
| `contentType=ALIDOC`, `extension=able` | `aitable` | 将 nodeId 作为 baseId，加载 `dingtalk-aitable` 操作 |
| `contentType=DOCUMENT`, `extension=xlsx` / `xls` / `xlsm` / `csv` | `drive` | 必须用 `dws drive download` 下载到本地处理，禁止走 `sheet`（非在线表格，sheet 命令无法操作） |
| `contentType≠ALIDOC`, `nodeType=file` | `drive` | 调用 `dws drive download` 下载，返回文件下载链接 |
| `nodeType=folder` | `drive` / `wiki` | 调用 `dws drive list --workspace <WS_ID>` 或 `dws wiki node list` 列出子节点 |
| 以上均不匹配 | — | 告知用户当前暂不支持该类型 |

> axls vs xlsx 关键区分：
> - `axls`（钉钉在线电子表格，`contentType=ALIDOC`）→ 走 `sheet` 产品线（读/写/筛选/导出等服务端原子操作）
> - `xlsx` / `xls` / `xlsm` / `csv`（上传到文档空间的本地表格文件，`contentType=DOCUMENT`）→ 必须走 `dws drive download` 下载到本地后再解析处理，严禁错误路由到 `sheet` 产品线（sheet 命令只支持在线表格，调用 xlsx 节点会直接报错）
> - 用户想把在线表格导出为 xlsx 文件 → 用 `dws sheet export`（输入是 `axls`，输出是 xlsx，这是 axls → xlsx 的格式转换，不属于 xlsx 读取场景）

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
dws drive list --workspace <WS_ID> --format json
```

### 何时可跳过探测

当用户指令中已明确指定产品（如"帮我读这个文档"、"看下这个表格的数据"），可结合用户意图**跳过探测**直接路由。仅在以下情况**必须执行探测**：
- 用户只粘贴 URL，无其他上下文
- 用户指令与 URL 实际类型可能不一致（如说"文档"但实际是表格）
- 用户直接粘贴的是原始 `alidocs` URL，且没有上游命令返回来确认类型
