# 文档知识

> lite recipe 见 [SKILL.md 速查表](../SKILL.md)。

| Recipe | 行动指南（固定路线） |
|--------|-------------------|
| import-file | 1. **直接执行** `dws doc import --file <本地文件路径> --format json`（一条命令完成上传+转换+创建）<br>2. 从返回中提取 `documentUrl` 并告知用户<br>3. **禁止先 Read 文件内容再 `doc create` + `doc update`**——`doc import` 是服务端格式转换，客户端无需解析文件内容<br>4. 可选参数：`--folder <文件夹ID>` 指定目标文件夹、`--workspace <知识库ID>` 指定目标知识库、`--name "文档名"` 自定义名称<br>5. 格式映射：docx/doc→文档, xlsx/xls→表格, xmind/mark→脑图, md/txt→文档<br>6. 超时或中断时 CLI 返回 `taskId`，用 `dws doc import get --task-id <taskId>` 手动查询<br>详见 [doc-import.md](./doc/doc-import.md) |
| write-doc | 1. 普通线性正文直接写入 UTF-8 `.md`，执行 `doc create --name <标题> --content-file <tmp.md> --content-format markdown --format json`<br>2. 仅当用户要求复杂版式且确实选择 JSONML 时，读取 [doc-create-workflow.md](./doc/style/doc-create-workflow.md) 对应章节，不预读整套 style/reference<br>3. 大内容依赖 DWS 自动分片；仅在 `CONTENT_TRUNCATED`、中断或回读缺失时恢复<br>4. 取 create 返回的 `nodeId` 执行 `doc read --node <nodeId> --format json`，核对明确要求的标题、段落和结构 |
| search-docs-and-share | 1. `dws drive search --query "<关键词>" --format json` → 取候选 `nodeId` + 标题建索引（不读全文）<br>2. 对追问选中的候选执行 `dws drive info --node <nodeId> --format json`<br>3. 仅 `extension=adoc` 使用 `dws doc read --node <nodeId> --format json`（最多 2 篇）；`md` / `axls` / `able` / 普通文件分别切到 markdown / sheet / aitable / drive，禁止固定执行 `doc read` |
| create-knowledge-base | 1. 创建知识库空间取 `WS_ID`<br>2. `wiki node create --workspace <WS_ID> --name "<文档名>"` → 取 `nodeId`<br>3. `wiki node list --workspace <WS_ID>` 确认 |
| migrate-doc | 1. `doc read --node <源nodeId>` → 取正文并写入临时文件 `<tmp>.md`<br>2. `doc create --name "<文档名>" --folder <DOC_FOLDER_NODE_ID> --content-file <tmp>.md` → 取新 `nodeId`；所有长度都先走这一条原生命令，由 CLI 自动分片（`--folder` 只传文档文件夹 nodeId / alidocs 文件夹 URL，不传数字 dentryId）<br>3. **回读校验**：`doc read --node <nodeId>` 校验内容完整性；仅在 `CONTENT_TRUNCATED`、中断或回读缺失时，从真实断点补写缺失部分 |
| update-doc-section | 1. `dws drive search --query "<关键词>" --format json` → 取 `nodeId`<br>2. `dws drive info --node <nodeId> --format json`，仅 `extension=adoc` 继续；其他类型切到对应 skill/reference<br>3. **形态选择（按 [doc-update-workflow.md §1.3](./doc/style/doc-update-workflow.md) 优先级）**：目标段落含 callout / 分栏 / 颜色 / @人 / 附件 / 嵌套结构 → 走 `jsonml-node-edit`；纯文本替换且确认无富结构 → 继续本 recipe<br>4. `dws doc read --node <nodeId> --format json` 定位目标章节<br>5. `dws doc update --node <nodeId> --content "<替换内容>" --mode overwrite --yes --format json`<br>6. **回读校验**：`dws doc read --node <nodeId> --format json` 确认 overwrite 未被降级为 append、内容完整无截断<br>**overwrite 须用户确认**；完整改写流程见 [doc-update-workflow.md](./doc/style/doc-update-workflow.md) |
| rewrite-doc | 1. 阅读并执行 [doc-update-workflow.md](./doc/style/doc-update-workflow.md)：先看 §1.3 编辑形态优先级（**JSONML 首选**），再按 §3 速查表选路径，跳 §4 对应小节执行<br>2. 单块改写 / 含富结构 → §4.4 路径 B；多处保真改写或改 root → §4.4 路径 A；纯文本骨架重写 → §4.5 markdown<br>3. 整篇 overwrite 前必须按 workflow §4.5 向用户提示风险并等待确认<br>4. **回读校验（必须）**：按 workflow §6 的校验要点逐项核查；@人、附件、图片等保真要素必须原样保留<br>**适用场景**：用户提供已有 nodeId/链接，需要改写、润色、章节补充、段落形态转换、整篇重写 |
| doc-to-message | 1. `doc read --node <nodeId>` → 取正文（大文档只摘要+链接）<br>2. `aisearch person --keyword "<姓名>" --dimension name` → 取 `openDingTalkId`（推荐）；或 `chat search --query "<群名>"` → 取 `openConversationId`<br>3. `chat message send --open-dingtalk-id <openDingTalkId> --text "<内容>"`（推荐）或 `--group <openConversationId> --text "<内容>"` 发送。仅当无法获取 openDingTalkId 时才用 `--user <userId>`（备选） |
| lossless-doc-edit | 1. `doc read --node <nodeId> --content-format jsonml --output /tmp/doc.json` → 获取完整 JSONML 结构（输出含 `revision`，并发敏感时记下来；默认改写不需要）<br>2. 解析 JSON 文件，修改 `jsonml` 数组中的目标节点（节点结构参见 [doc-jsonml-schema.md](./doc/format/doc-jsonml-schema.md)）<br>3. 将修改后的内容写回临时文件 `/tmp/doc_modified.json`，格式为 `{"jsonml": [...]}`<br>4. `doc update --node <nodeId> --content-file /tmp/doc_modified.json --content-format jsonml --mode overwrite`（默认不做并发检查；担心多 agent 同时改时加 `--revision <第 1 步拿到的 N>` 触发并发检查，版本不一致返回 `VersionConflict` 时回到第 1 步重读重写）<br>5. **回读校验**：`doc read --node <nodeId> --content-format jsonml` 确认写入成功<br>**适用场景**：保留样式、精准插入特定节点类型、改属性不动文本；普通文本编辑仍优先用 markdown 模式。完整 JSONML 改写流程见 [doc-update-workflow.md §4.4](./doc/style/doc-update-workflow.md) |
| jsonml-node-edit | 1. `doc block list --node <nodeId> --content-format jsonml` → 获取 JSONML 节点列表（含 uuid）<br>2. 根据 uuid 定位目标节点<br>3. `doc block list --node <nodeId> --content-format jsonml --block-id <uuid>` → 读取完整子树<br>4. 修改 JSONML 节点内容（节点结构参见 [doc-jsonml-schema.md](./doc/format/doc-jsonml-schema.md)，可复制范例见 [doc-jsonml-cookbook.md](./doc/format/doc-jsonml-cookbook.md)）<br>5. `doc block update --node <nodeId> --block-id <uuid> --content-format jsonml --element '<修改后的 JSONML>'` → 写回<br>**适用场景**：只改一个 block 的结构/样式，无需全文回写；写入端默认 normalize（自动补 uuid、裸字符串包成 canonical 文本），可用 `--no-fix-jsonml` 关闭全部修复；`--fix-jsonml` 额外启用 JSON 语法修复（推荐 agent 调用） |

## template-based-generation

当用户提供已有 alidocs 文档并要求“按模板生成、复刻、生成同形态的新版本”时，必须走保形复制链路：

1. `dws drive info --node <源文档ID或URL> --format json`，确认源节点存在并记录类型。
2. `dws drive copy --node <源文档ID或URL> [--folder <目标文件夹>] [--workspace <目标知识库>] --format json`，从返回中提取副本 `nodeId`。
3. 如需改名，只对副本执行 `dws drive rename --node <副本nodeId> --name "<新名称>" --format json`。
4. `dws doc block list --node <副本nodeId> --content-format jsonml` 定位需要替换的块；仅对副本执行 `dws doc block update`。涉及多个块或富结构时，按 [doc-update-workflow.md](./doc/style/doc-update-workflow.md) 的 JSONML 路径操作。
5. 再次读取副本的 `drive info` 和目标块，确认名称、内容与结构；禁止修改源文档。

**禁止**用 `doc read` → `doc create` 重建模板。Markdown 是有损投影，会丢失行高、单元格背景色、字号和部分富结构。

---

## 自动分片失败恢复

`doc create` / `doc update` 的 Markdown 写入管道会从 10,000 个 Unicode 字符开始自动分片，超时后降到 5,000 字符重试。不要在调用前手工复制分片逻辑。

只有返回 `CONTENT_TRUNCATED`、写入被中断或回读发现缺失时，才执行恢复：

1. 保存命令返回的真实 `nodeId` 和 `chunksWritten`，不要重新创建文档。
2. `doc read --node <nodeId>` 定位最后一个完整标题/段落。
3. 从原始输入中只提取缺失后缀，写入一个新的 UTF-8 文件。
4. 向用户报告已完成范围与待补范围；得到继续指示后，用一次 `doc update --mode append --content-file <missing.md>` 补写。
5. 再次回读并核对关键标题、表格、列表和代码块；仍缺失则停止，不循环追加。

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

## 显式工作流

- 用户点名的 `create → list → insert/append/update` 是可观察命令链，必须保持顺序逐项执行；create 只承载明确的初始正文。有序列表块必须验证回读结构中的 `list.isOrdered=true`。
- 新建资源返回 ID 后，同一请求的指代默认绑定该新资源；禁止搜索同名旧资源替换绑定。
