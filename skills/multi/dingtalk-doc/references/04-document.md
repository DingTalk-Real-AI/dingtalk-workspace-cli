# 文档知识

> 通用规范见 [_common/conventions.md](_common/conventions.md)。

| Recipe | 行动指南（固定路线） |
|--------|-------------------|
| write-doc | 1. 按[「多源并行采集」](_common/conventions.md#多源并行采集公共模式)执行<br>2. **先把内容写入临时文件**（Linux/Mac `/tmp/<name>.md`，Windows `%TEMP%\<name>.md`）—— 含多行/表格/长文本必须走文件，不要把 markdown 直接作为命令行字符串<br>3. **单步创建**（< 200KB）：`doc create --name "<文档名>" --content-file <tmp> [--folder <DOC_FOLDER_NODE_ID>] [--workspace <WS_ID>]`（`--folder` 只传文档文件夹 nodeId / alidocs 文件夹 URL，不传数字 dentryId）<br>4. **超长兜底**（> 200KB）：**必须先向用户提示截断风险**（详见下方「分块 append 截断风险提示」），用户确认后再执行：`doc create --name "<文档名>" [--folder/--workspace]` → `nodeId` → 按段落切 ≤200KB 片段（不断表格） → 每片 `doc update --node <nodeId> --content-file <part> --mode append`<br>5. **回读校验**（必须）：所有写入完成后，执行 `doc read --node <nodeId>` 回读文档，校验关键标题/段落是否完整写入（详见下方「doc update 回读校验规范」）<br>备选（仅短内容 <2KB 且无换行/表格）：`doc create --name "..." --content "..."` |
| search-docs-and-share | 1. `doc search --query "<关键词>"` → 取 `nodeId` + 标题建索引（不读全文）<br>2. `doc read --node <nodeId>`（追问按需，最多 2 篇） |
| create-knowledge-base | 1. 创建知识库空间取 `WS_ID`<br>2. `doc create --name "<文档名>" --workspace <WS_ID>` → 取 `nodeId`<br>3. `doc list --workspace <WS_ID>` 确认 |
| migrate-doc | 1. `doc read --node <源nodeId>` → 取正文并写入临时文件 `<tmp>.md`<br>2. `doc create --name "<文档名>" --folder <DOC_FOLDER_NODE_ID> --content-file <tmp>.md` → 取新 `nodeId`（`--folder` 只传文档文件夹 nodeId / alidocs 文件夹 URL，不传数字 dentryId；正文 <200KB 单步到位）<br>2a. 若正文 >200KB：**必须先向用户提示截断风险**（详见下方「分块 append 截断风险提示」），用户确认后再执行：`doc create --name "<文档名>" --folder <DOC_FOLDER_NODE_ID>` → `nodeId` → 按段落切片 → 每片 `doc update --node <nodeId> --content-file <part> --mode append`<br>3. **回读校验**：`doc read --node <nodeId>` 校验内容完整性 |
| update-doc-section | **优先块级精修（保形）**：1. `doc search --query "<关键词>"` → 取 `nodeId`<br>2. `doc block list --node <nodeId> --format json` 定位目标块取 `blockId`<br>3. `doc block update --node <nodeId> --block-id <blockId> --text "<替换内容>" --format json` 局部修改（不破坏富格式）<br>4. **回读校验**：`doc read --node <nodeId>` 确认目标章节已更新<br>**兜底**：仅当无法定位块且用户确认整篇替换时，先 `doc update --node <nodeId> --content-file <tmp>.md --mode overwrite --dry-run` 预览，再加 `--yes` 执行；overwrite 会丢富格式（颜色/字号/合并单元格等），详见下方 template-based-generation §"为什么避免 overwrite" |
| template-based-generation | **触发**：用户给已有 alidocs URL + "参照/按模板/复刻/同样模板生成 X 月份的" → **保形复制，禁止重写**。固定走下方 [template-based-generation](#template-based-generation) 章节：`doc copy` 保形复制 → `doc rename` → `doc block list/update` 局部改副本。禁止 `doc read + doc create` 重写链。 |
| doc-to-message | 1. `doc read --node <nodeId>` → 取正文（大文档只摘要+链接）<br>2. `contact user search --query "<姓名>"` → 取 `openDingTalkId`（推荐）；或 `chat search --query "<群名>"` → 取 `openConversationId`<br>3. `chat message send --open-dingtalk-id <openDingTalkId> --text "<内容>"`（推荐）或 `--group <openConversationId> --text "<内容>"` 发送。仅当无法获取 openDingTalkId 时才用 `--user <userId>`（备选） |
| delete-old-doc | 1. `doc search --query "<关键词>"` 或 `doc list --folder <FOLDER>` 取 `nodeId`<br>2. **必须先向用户展示要删除的文档标题/路径**，等用户确认<br>3. `doc delete --node <nodeId> --yes`（注意：是 `doc delete`，不是 `doc block delete`；前者删整篇，后者删块）<br>4. 验证：`doc info --node <nodeId>` 应返回 not-found |
| export-doc-as-docx | 1. `doc search --query "<关键词>"` → 取 `nodeId`<br>2. `doc export --node <nodeId> --output /tmp/<name>.docx --timeout-sec 600`（CLI 内置渐进式退避轮询，**不要自己拼 GET downloadUrl**）<br>3. 落盘失败兜底：`doc export get --job-id <ID> --output /tmp/<name>.docx` 续等<br>4. （可选）`doc-to-message` 把 docx 路径作为附件发给用户 |
| grant-doc-access | 1. `doc search --query "<关键词>"` → 取 `nodeId`<br>2. `contact user search --query "<姓名>"` → 取 `userId`（**注意**: doc permission 用 `userId`，不是 `openDingTalkId`）<br>3. **节点级**授权：`doc permission add --node <nodeId> --user <UID1,UID2> --role EDITOR`<br>　　role 取值: MANAGER / EDITOR / DOWNLOADER / READER（**不要传 OWNER**）<br>　　单次最多 30 个 userId<br>4. 查权限：`doc permission list --node <nodeId>` 确认<br>5. **不要混淆**: 给"知识库"加成员用 `wiki member add --workspace`（容器级）；给"单个文档"加权限用 `doc permission add --node`（节点级）|
| insert-image-to-doc | 1. `doc search --query "<关键词>"` → 取 `nodeId`<br>2. `doc media insert --node <nodeId> --file ./image.png`（3 步一体化：获取凭证 → PUT OSS → 插入块；图片 ≤20MB 自动作为内联图片，其他作为附件块）<br>3. （可选）`doc read --node <nodeId>` 回读确认<br>**禁止**: 自己拿 uploadUrl 写 PUT —— 90%+ 会因 Content-Type 未清空触发 SignatureDoesNotMatch |
| download-doc-attachment | 1. `doc block list --node <nodeId>` → 找到 `blockType=attachment` 的块取 `resourceId`<br>2. `doc media download --node <nodeId> --resource-id <RID>` → 返回 OSS 临时 URL（**注意 expirationSeconds**）<br>3. 调用方自行 GET 该 URL 落盘 |

---

## template-based-generation

### 触发条件

用户给已有 alidocs URL，并要求：
- "参照这个生成同样的"
- "按模板生成"
- "复刻这个"
- "同样的模板，X 月份的"
- "仿照这个文档"

### 核心原则

**保形优于重写**。adoc 富格式（行高、单元格背景色、字号、渐变文字色、表格列宽、合并单元格）在 markdown 层无表达，`doc read → create_file → doc create` 会做两次 lossy projection（先 adoc→markdown，再 markdown→adoc），必然丢失这些属性。

正确做法：**在 adoc 层保形复制（`doc copy`）**，再局部改写需要替换的块（`doc block update`），保留所有富格式。

### 固定路线

```bash
# 1. probe 模板节点，确认是 ALIDOC（adoc 富文档）
dws doc info --node <模板NODE或URL> --format json

# 2. 在 adoc 层保形复制（不经过 markdown 投影）
dws doc copy --node <模板NODE或URL> --folder <目标文件夹NODE或URL> --format json
# → 取返回的副本 nodeId

# 3. 重命名副本
dws doc rename --node <副本nodeId> --name "<新文档名>" --format json

# 4. 列出所有块定位要改的内容
dws doc block list --node <副本nodeId> --format json
# → 记录 blockId 和现有文本

# 5. 逐块局部改写（只改月份/日期/姓名/指标等局部变量，不动结构）
dws doc block update --node <副本nodeId> --block-id <blockId> --text "<新内容>" --format json

# 6. 回读确认
dws doc read --node <副本nodeId>
```

### 为什么避免 overwrite

| 场景 | overwrite 后果 |
|------|---------------|
| 模板带表格背景色 | 全部变白 |
| 标题字号 24pt 加粗渐变 | 退化为普通 H1 |
| 合并单元格的表格 | 拆成独立单元格 |
| 嵌入图片 | 链接保留但样式重置 |

**用 copy+block update 全部可保留**。

### 禁止反模式

- ❌ doc read + doc create 重写模板文档（两次 lossy projection）
- ❌ 先 `dws doc copy`，再删除副本退回 create 重写链路
- ❌ 用 `dws doc update --mode overwrite --yes` 把含富格式的 adoc 整篇覆盖成 markdown

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

### 为什么需要回读校验

`doc update` 在以下场景可能产生**静默失败**，返回 `success=true` 但实际写入不完整：

- **overwrite 降级为 append**：大文档 overwrite 模式被后端静默降级，导致旧内容未清除、新内容追加在末尾
- **分块 append 内容截断**：超长文档分片写入时，部分片段丢失或顺序错乱
- **编码/通道问题**：PowerShell 等特殊终端下 UTF-8 内容传输乱码

### 校验流程

在 **每次 `doc update` 完成后**（无论 overwrite 还是 append 模式），必须执行以下步骤：

1. **回读文档**：`doc read --node <nodeId>`
2. **关键内容校验**：检查回读结果是否包含预期的关键标题、段落首句、表格标记等
3. **异常处理**：
   - 若内容缺失或截断 → 向用户报告具体差异，建议重试或手动修复
   - 若 overwrite 后仍残留旧内容 → 提示用户 overwrite 可能被降级，建议先清空再 append
   - **禁止**在未回读的情况下直接向用户报告"已完成"

### 示例

```
# 更新文档
doc update --node abc123 --content-file /tmp/new-content.md --mode overwrite

# 回读校验
doc read --node abc123
# → 检查输出是否包含预期内容的关键标题和段落
```
