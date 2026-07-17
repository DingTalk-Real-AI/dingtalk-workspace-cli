# 文档知识

## 模板保形生成

已有 alidocs 文档需要“按模板生成、复刻或生成同形态变体”时，优先在原生文档层复制，避免 `doc read` 转成 Markdown 后重建导致样式丢失：

1. `dws drive copy --node <SOURCE_NODE_ID> --folder <FOLDER_NODE_ID> --format json` 复制原文档。
2. 从返回中取得副本 `nodeId`，再用 `dws drive rename --node <COPY_NODE_ID> --name "<新名称>" --format json` 重命名。
3. 用 `dws doc block list --node <COPY_NODE_ID> --format json` 定位需要替换的块，仅在副本上执行 `doc block update`。
4. 回读副本的关键块，确认文字已替换且原布局、表格与样式仍保留。

不要用 `doc read + doc create` 重建要求保形的模板文档。

> lite recipe 见 [SKILL.md 速查表](../SKILL.md)。

| Recipe | 行动指南（固定路线） |
|--------|-------------------|
| import-file | 1. **直接执行** `dws doc import --file <本地文件路径> --format json`（一条命令完成上传+转换+创建）<br>2. 从返回中提取 `documentUrl` 并告知用户<br>3. **禁止先 Read 文件内容再 `doc create` + `doc update`**——`doc import` 是服务端格式转换，客户端无需解析文件内容<br>4. 可选参数：`--folder <文件夹ID>` 指定目标文件夹、`--workspace <知识库ID>` 指定目标知识库、`--name "文档名"` 自定义名称<br>5. 格式映射：docx/doc→文档, xlsx/xls→表格, xmind/mark→脑图, md/txt→文档<br>6. 超时或中断时 CLI 返回 `taskId`，用 `dws doc import get --task-id <taskId>` 手动查询<br>详见 [doc-import.md](./doc/doc-import.md) |
| write-doc | 0. 阅读 [doc-create-workflow.md](./doc/style/doc-create-workflow.md) 的 §前置必读 + §关键词速查表，锁定文档类型和起稿路径：**决策型/含对比的知识沉淀型/用户要求美观 → JSONML 起稿**（`.json`）；执行型/说明型 → Markdown 起稿（`.md`）<br>1. 按选定路径执行 doc-create-workflow.md（JSONML 路径有骨架范例可直接复制修改）<br>2. `doc create --content-file /tmp/<name>.json --content-format jsonml`（或 `.md` + `--content-format markdown`）<br>3. 大内容默认依赖 DWS 自动分片；只有 `CONTENT_TRUNCATED`、部分写入失败或回读发现缺失时，才按 workflow 的恢复流程手工补片<br>4. **回读校验（必须）**：所有写入完成后，执行 `doc read --node <nodeId>`，校验关键标题/段落/表格是否完整写入 |
| search-docs-and-share | 1. `drive search --query "<关键词>"` → 取 `nodeId` + 标题建索引（不读全文）<br>2. `doc read --node <nodeId>`（追问按需，最多 2 篇） |
| create-knowledge-base | 1. 创建知识库空间取 `WS_ID`<br>2. `wiki node create --workspace <WS_ID> --name "<文档名>"` → 取 `nodeId`<br>3. `wiki node list --workspace <WS_ID>` 确认 |
| migrate-doc | 1. `doc read --node <源nodeId>` → 取正文并写入临时文件 `<tmp>.md`<br>2. `doc create --name "<文档名>" --folder <DOC_FOLDER_NODE_ID> --content-file <tmp>.md` → 取新 `nodeId`（`--folder` 只传文档文件夹 nodeId / alidocs 文件夹 URL，不传数字 dentryId；正文 <200KB 单步到位）<br>2a. 若正文 >200KB：**必须先向用户提示截断风险**（详见下方「分块 append 截断风险提示」），用户确认后再执行：`doc create --name "<文档名>" --folder <DOC_FOLDER_NODE_ID>` → `nodeId` → 按段落切片 → 每片 `doc update --node <nodeId> --content-file <part> --mode append`<br>3. **回读校验**：`doc read --node <nodeId>` 校验内容完整性 |
| update-doc-section | 1. `drive search --query "<关键词>"` → 取 `nodeId`<br>2. **形态选择（按 [doc-update-workflow.md §1.3](./doc/style/doc-update-workflow.md) 优先级）**：目标段落含 callout / 分栏 / 颜色 / @人 / 附件 / 嵌套结构 → 走 `jsonml-node-edit`；纯文本替换且确认无富结构 → 继续本 recipe<br>3. `doc read --node <nodeId>` 定位目标章节<br>4. `doc update --node <nodeId> --content "<替换内容>" --mode overwrite`<br>5. **回读校验**：`doc read --node <nodeId>` 确认 overwrite 未被降级为 append、内容完整无截断<br>**overwrite 须用户确认**；完整改写流程见 [doc-update-workflow.md](./doc/style/doc-update-workflow.md) |
| rewrite-doc | 1. 阅读并执行 [doc-update-workflow.md](./doc/style/doc-update-workflow.md)：先看 §1.3 编辑形态优先级（**JSONML 首选**），再按 §3 速查表选路径，跳 §4 对应小节执行<br>2. 单块改写 / 含富结构 → §4.4 路径 B；多处保真改写或改 root → §4.4 路径 A；纯文本骨架重写 → §4.5 markdown<br>3. 整篇 overwrite 前必须按 workflow §4.5 向用户提示风险并等待确认<br>4. **回读校验（必须）**：按 workflow §6 的校验要点逐项核查；@人、附件、图片等保真要素必须原样保留<br>**适用场景**：用户提供已有 nodeId/链接，需要改写、润色、章节补充、段落形态转换、整篇重写 |
| doc-to-message | 1. `doc read --node <nodeId>` → 取正文（大文档只摘要+链接）<br>2. `contact user search --query "<姓名>"` → 取 `openDingTalkId`（推荐）；或 `chat search --query "<群名>"` → 取 `openConversationId`<br>3. `chat message send --open-dingtalk-id <openDingTalkId> --text "<内容>"`（推荐）或 `--group <openConversationId> --text "<内容>"` 发送。仅当无法获取 openDingTalkId 时才用 `--user <userId>`（备选） |
| lossless-doc-edit | 1. `doc read --node <nodeId> --content-format jsonml --output /tmp/doc.json` → 获取完整 JSONML 结构（输出含 `revision`，并发敏感时记下来；默认改写不需要）<br>2. 解析 JSON 文件，修改 `jsonml` 数组中的目标节点（节点结构参见 [doc-jsonml-schema.md](./doc/format/doc-jsonml-schema.md)）<br>3. 将修改后的内容写回临时文件 `/tmp/doc_modified.json`，格式为 `{"jsonml": [...]}`<br>4. `doc update --node <nodeId> --content-file /tmp/doc_modified.json --content-format jsonml --mode overwrite`（默认不做并发检查；担心多 agent 同时改时加 `--revision <第 1 步拿到的 N>` 触发并发检查，版本不一致返回 `VersionConflict` 时回到第 1 步重读重写）<br>5. **回读校验**：`doc read --node <nodeId> --content-format jsonml` 确认写入成功<br>**适用场景**：保留样式、精准插入特定节点类型、改属性不动文本；普通文本编辑仍优先用 markdown 模式。完整 JSONML 改写流程见 [doc-update-workflow.md §4.4](./doc/style/doc-update-workflow.md) |
| jsonml-node-edit | 1. `doc block list --node <nodeId> --content-format jsonml` → 获取 JSONML 节点列表（含 uuid）<br>2. 根据 uuid 定位目标节点<br>3. `doc block list --node <nodeId> --content-format jsonml --block-id <uuid>` → 读取完整子树<br>4. 修改 JSONML 节点内容（节点结构参见 [doc-jsonml-schema.md](./doc/format/doc-jsonml-schema.md)，可复制范例见 [doc-jsonml-cookbook.md](./doc/format/doc-jsonml-cookbook.md)）<br>5. `doc block update --node <nodeId> --block-id <uuid> --content-format jsonml --element '<修改后的 JSONML>'` → 写回<br>**适用场景**：只改一个 block 的结构/样式，无需全文回写；写入端默认 normalize（自动补 uuid、裸字符串包成 canonical 文本），可用 `--no-fix-jsonml` 关闭全部修复；`--fix-jsonml` 额外启用 JSON 语法修复（推荐 agent 调用） |

---

## 分块 append 截断风险提示

### 触发条件

当内容总大小 **超过 200KB**，需要拆分为多片通过 `doc update --mode append` 分块写入时，**必须在执行前向用户发出截断风险提示**，等待用户确认后再继续。

### 提示话术（参考模板）

> 注意: 内容较长（约 {size}），需要分 {n} 片写入。分块 append 存在以下风险：
> - 部分片段可能写入失败但返回 success，导致文档**内容截断或缺失**
> - 片段之间的表格、代码块等跨块元素可能**被截断破坏**
> - 写入顺序异常可能导致**段落错乱**
>
> 建议：写入完成后我会回读校验文档完整性。是否继续？

### 执行规范

1. **提示时机**：在执行第一片 append **之前**提示，而非写入过程中
2. **分片原则**：按段落/标题边界切分，**禁止**在表格、代码块、列表内部截断
3. **逐片校验**（推荐）：每写入一片后记录已写入的最后一个标题/段落标记，供最终回读时比对
4. **最终回读**（必须）：所有片段写入完成后，执行 `doc read --node <nodeId>` 回读全文，逐片比对关键标记是否完整（详见下方「doc update 回读校验规范」）
5. **失败处理**：若回读发现缺失片段，向用户报告具体缺失位置，建议针对缺失部分单独重试 append

---

## doc update 回读校验规范

**所有 `doc update`（含 overwrite 和 append）执行后都必须回读校验**——返回 `success=true` 不等于内容真的写入完整。

完整规范（静默失败场景、校验流程、异常处理路径、"先清空再重建"命令样板）见 [doc-update-workflow.md §6「回读验收」](./doc/style/doc-update-workflow.md)。

最小流程：

```bash
dws doc update --node <nodeId> --content-file /tmp/new-content.md --mode overwrite
dws doc read --node <nodeId>   # 校验关键标题、段落首句、表格、@人、附件
```

**禁止**在未回读的情况下向用户报告「已完成」。
