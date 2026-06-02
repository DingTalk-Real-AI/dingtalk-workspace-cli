# 钉钉文档创建流程

本文只处理一件事：用 `dws doc create` 创建一篇钉钉文档，并确认内容真的写进去。资料采集、汇报生成、转发通知、权限分发等都不是本文范围，应由对应 recipe 或产品参考负责。

> 改写已有文档见 [doc-update-workflow.md](./doc-update-workflow.md)。排版规范见 [doc-style-guideline.md](./doc-style-guideline.md)。

## 前置必读

> **同时读取 [doc-style-guideline.md](./doc-style-guideline.md)：**
> - **§2.0 类型判断决策表** → 锁定文档类型（决策型 / 执行型 / 说明型 / 知识沉淀型）和骨架
> - **§1 硬规则** → 全程生效（`--name` 已是 H1、不编造 URL、Markdown 草稿不写 callout 等）

### 关键词速查（用户意图 → 起稿路径）

| 用户关键词 | 文档类型 | 起稿路径 |
|-----------|---------|---------|
| 汇报 / 周报 / 月报 / 复盘 / 方案选型 / 决策 / 对比 | §2.1 决策型 | **→ JSONML 起稿** |
| 调研 / 技术方案 / 复盘报告（含对比/数据） | §2.4 知识沉淀型 | **→ JSONML 起稿** |
| SOP / Runbook / 接入指南 / 升级 / 操作手册 | §2.2 执行型 | → Markdown 起稿 |
| 接口文档 / 能力清单 / 参数说明 / 错误码 | §2.3 说明型 | → Markdown 起稿 |
| 用户原文含：颜色/高亮/美观/醒目/重点突出/像PPT | 任意类型 | **→ JSONML 起稿** |

## 适用边界

进入本文前，必须已经确认用户要创建的是钉钉文档 (`adoc`)。如果用户要的是钉钉表格、AI表格、文件上传、知识库空间管理或消息发送，不要套用本文。

本文覆盖：

- 文档标题和创建位置确认
- 正文草稿准备
- `doc create` 写入
- 写入后回读验收
- 内容缺失时的补救写入

本文不覆盖：

- 从群聊、日志、听记、表格等来源采集资料
- 生成日报、周报、月报等业务报告口径
- 文档权限分享、消息通知或待办创建
- 非钉钉文档的新建流程

## 创建前检查

创建前先锁定四个输入：

| 项目 | 要求 |
|------|------|
| 标题 | 用 `--name` 传入；正文不要再重复同名一级标题 |
| 位置 | 默认创建到我的文档；指定目录时只接受文档文件夹 `nodeId` 或 alidocs 文件夹 URL |
| 正文 | 多行、表格、代码块、特殊字符或长度 >= 2KB 时必须写入 UTF-8 临时 `.md` 文件 |
| 格式 | 按 §JSONML 起稿判定 决定起稿路径：命中 JSONML 起稿条件时**直接用 JSONML 构造**（跳过 markdown）；未命中时用 Markdown 起稿，创建后按 [doc-update-workflow.md](./doc-update-workflow.md) 精修 |

禁止把纯数字 `dentryId`、drive `parent-id` 或 spaceId 填进 `--folder`。

## JSONML 起稿判定

在正文准备之前，先判断是否直接用 JSONML 起稿。**命中以下任一条件即走 JSONML 起稿路径**（跳过 markdown 草稿阶段）：

### 文档类型触发

| 类型 | 触发条件 |
|------|---------|
| 决策型（§2.1） | **默认触发** — 汇报/方案/对比需要摘要 callout、彩色表头、决策时限标注 |
| 知识沉淀型（§2.4） | 含对比分析、多维度数据可视化、需要关键节点彩色 callout |

### 意图关键词触发

用户原文或需求描述中出现以下任一关键词：

- 颜色 / 配色 / 上色 / 高亮 / 醒目
- 字号 / 字体 / 加大 / 缩小
- 视觉效果 / 排版精美 / 好看 / 美观
- callout / 分栏 / 对比色 / 彩色表头
- "像 PPT 那样" / "有设计感" / "重点突出"

---

## JSONML 起稿（命中判定时使用）

当判定为 JSONML 起稿时，在本地临时 JSON 文件中直接构造完整的 JSONML 文档树。路径：`/tmp/<name>.json`。

> **MUST READ**：动手写 JSONML 前，必须先用 Read 工具读取 [doc-jsonml-cookbook.md](../format/doc-jsonml-cookbook.md) — 其中 §决策型文档骨架范例 有可直接复制修改的完整模板。
> 节点类型和属性的权威定义见 [doc-jsonml-schema.md](../format/doc-jsonml-schema.md)。

### ⚠️ JSONML 结构严格约束（生成时必须遵守）

每个节点是一个 JSON 数组：`[tagName, attributes?, ...children]`

- **第一个元素**是字符串，表示标签名（如 `"p"`, `"h1"`, `"span"`, `"container"`）
- **第二个元素**（可选）是一个 JSON 对象，表示属性（如 `{"uuid": "abc"}`）。如果无属性，可以直接进入子节点
- **随后的元素**是子节点，可以是纯字符串（仅限 leaf span 内），也可以是另一个 JSONML 数组
- **所有 `[` 必须有对应 `]`，所有 `{` 必须有对应 `}`，数组元素之间用 `,` 分隔，最后一个元素后不加 `,`**

常见 LLM 生成错误（务必避免）：

| 错误类型 | 示例 | 后果 |
|---------|------|------|
| 缺少闭合 `]` | `["p", {}, ["span", ...]` | JSON 解析失败 |
| 多余逗号 | `["p", {},]` | JSON 解析失败 |
| 缺少逗号 | `["p", {} ["span"]]` | JSON 解析失败 |
| 引号不匹配 | `["p", {"uuid": "abc}]` | JSON 解析失败 |

### 基础结构

文件内容是一个裸 JSONML 数组，根节点为 `"root"`：

```json
["root", {},
  ["h2", {}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "章节标题"]]],
  ["p", {}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "正文段落"]]]
]
```

- 根节点固定 `"root"`（不是 `"body"`）
- `--name` 已是 H1，JSONML 从 `h2` 开始
- 表格结构是 `table → tr → tc`（无 `th`/`td`）
- uuid 可省略（CLI normalize 自动补）

### 视觉设计要点

构造时主动使用这些属性实现视觉效果：
- **文字着色**：leaf 上 `"color": "#hex"`、`"highlight": "#hex"`
- **字号**：leaf 上 `"sz": 14, "szUnit": "pt"`
- **callout**：`["container", {"subType": "colorBlocks", "metadata": {"bgcolor": "#E8F5E9", "border": "left"}}, ...blocks]`
- **表格单元格底色**：tc 上 `"fill": "#hex"`

### 写入

```bash
dws doc create --name "<文档名>" --content-file /tmp/<name>.json --content-format jsonml
```

### 回读验收

```bash
dws doc read --node <nodeId> --content-format jsonml --output /tmp/<name>-readback.json
```

---

## 正文准备（未命中 JSONML 判定时）

正文草稿先在本地临时 Markdown 文件中完成，推荐路径形如 `/tmp/<name>.md`。

准备规则：

- 只使用用户已提供或对话中已确认的正文素材。
- 如果正文素材不足，先补齐文档目标、受众、章节和缺口；不要在本文中临时扩展跨产品采集流程。
- **先按 [doc-style-guideline.md §2.0 类型判断决策表](./doc-style-guideline.md) 确定文档类型，再用对应类型的骨架样板（§2.1 决策型 / §2.2 执行型 / §2.3 说明型 / §2.4 知识沉淀型）**。不要套通用三段式。
- **`--name` 已是 H1，正文从 `##` 开始**；正文内不要再写 `#` 一级标题（除非确实需要正文内再造一级 H1 并说明动机）。
- 摘要、bullet、引用块、callout 等元素的使用边界以 style-guideline §3-§7 为准。
- 同类信息保持一致：风险、状态、行动项各用一种元素 + 一种视觉语义（style-guideline §1.2 / §5）。
- 临时文件必须保留真实换行，不能把换行写成字面量 `\n`。
- Markdown 草稿阶段**不要**写 callout / 分栏 / 附件——这些留到「创建后的精修」用 `doc block insert` 操作（style-guideline §1.3）。
- **图片素材闭环（硬规则）**：正文需求含图片/截图/图文并茂时，**禁止**在 Markdown 中写 `![](...)` 图片语法（包括真实存在的 alidocs URL）。正确做法：Markdown 只写文本骨架和图片占位说明（如 `📌 此处插入：xxx 产品截图`），创建文档后逐个执行 `dws doc media insert --node <nodeId> --file <本地图片路径>` 插入，最后用 `dws doc block list --node <nodeId>` 验证图片块存在。图片来源如果是钉盘文件，必须先 `dws doc download --node <图片nodeId> --output /tmp/xxx.png` 下载到本地再 insert。

## 创建写入

优先用 `--content-file` 一次创建并写入：

```bash
dws doc create --name "<文档名>" --content-file /tmp/<name>.md --content-format markdown
```

创建到指定文件夹：

```bash
dws doc create --name "<文档名>" --content-file /tmp/<name>.md --folder <DOC_FOLDER_NODE_ID> --content-format markdown
```

创建到知识库：

```bash
dws doc create --name "<文档名>" --content-file /tmp/<name>.md --workspace <WS_ID> --content-format markdown
```

短纯文本才允许直接传 `--content`：

```bash
dws doc create --name "<文档名>" --content "短内容" --content-format markdown
```

返回后立即记录：

| 字段 | 用法 |
|------|------|
| `nodeId` | 后续 `doc read`、`doc update`、`doc block`、`doc media` 的目标 |
| `docUrl` | 最终交付给用户的链接；缺失时用 `doc info` 补查 |
| `chunksWritten` | 判断是否触发自动分片；大于 1 时重点检查章节顺序 |

## 回读验收

创建命令返回成功不等于正文完整。每次创建后都必须回读：

```bash
dws doc read --node <nodeId>
```

验收要点：

- 开头摘要、关键章节、表格表头、末尾章节都存在。
- 回读文本顺序和临时 Markdown 一致。
- 没有把字面量 `\n` 渲染成一整行。
- 如果返回 `chunksWritten > 1`，检查分片边界没有破坏表格、代码块或列表。
- 最终回复必须给用户 `docUrl`；如果只拿到 `nodeId`，说明链接字段未返回，并报告已尝试 `doc info`。

## 缺失补救

DWS 写入管道会自动处理长内容分片。只有出现以下情况才手工补片：

- 返回 `CONTENT_TRUNCATED`
- 命令超时或只写入部分分片
- 回读发现后半段缺失、章节乱序或表格损坏

补救流程：

1. 用 `doc read` 确认已经写到哪个章节。
2. 从原始临时 Markdown 中截取缺失部分，写入 `/tmp/<name>-resume.md`。
3. 追加缺失内容：

```bash
dws doc update --node <nodeId> --content-file /tmp/<name>-resume.md --mode append --content-format markdown
```

4. 再次 `doc read`，确认缺失章节已补齐。

## 创建后的精修

创建流程本身优先完成整篇正文。只有需要局部补充、插入附件、加 callout / 分栏、或无损结构调整时，才进入精修——**精修路径统一走 [doc-update-workflow.md](./doc-update-workflow.md)**。

精修常见入口（**按 [doc-update-workflow.md §1.3](./doc-update-workflow.md) 优先级排序：JSONML 首选**）：

- 单 block JSONML 精修（首选）：`doc block list --node <id> --content-format jsonml --block-id <uuid>` 取子树 → `doc block update --node <id> --block-id <uuid> --content-format jsonml --element '[...]'` 写回（uuid 必须 == --block-id；写入端默认 normalize + validate，详见 [doc-update-workflow.md §4.4](./doc-update-workflow.md)）
- 整篇 JSONML 无损：`doc update --content-format jsonml --mode overwrite`（默认直接覆盖，适合一次改多处或改 root sectPr；担心并发覆盖时加 `--revision <N>` 触发并发检查）
- 插入附件 / 图片：`doc media insert`（无 JSONML 形态，直接走 element）
- element JSON 次选：`doc block insert` / `doc block update` 不带 `--content-format jsonml` 时按老接口 JSON 解析；仅在 JSONML 不支持某字段时使用
- markdown 兜底：`doc update --mode append`（末尾追加纯文本段落，无富结构需保留时）

字段结构以 [`doc.md`](../../doc.md) 为准；何时用何种精修路径见 [doc-update-workflow.md §3「改写路径速查」](./doc-update-workflow.md)。

## 交付口径

只报告已经验证过的信息：

- 文档标题
- `docUrl` 或 `nodeId`
- 已写入的正文范围
- 回读验收结果
- 如有缺失，说明缺失位置和补救状态

未回读前，不要说内容完整或任务完成。
