# 钉钉文档改写流程

本文只处理一件事：用户给已有 nodeId 或 alidocs 链接，需要改写、润色、补充章节、转换段落形态时，按本文操作。从零创建新文档见 [doc-create-workflow.md](./doc-create-workflow.md)。

## 适用边界

进入本文前，必须已经确认用户要改写的是已有钉钉文档 (`adoc`)。如果用户要新建、要操作表格 / AI 表格 / 文件 / 知识库空间 / 发消息，不要套用本文。

本文覆盖：

- 已有文档的局部改写、润色、章节补充
- 段落 ↔ 列表 ↔ 表格 的形态转换
- 块级精修（callout、分栏、附件插入）
- overwrite 整篇改写的风险提示与执行
- JSONML 无损结构改写的入口

本文不覆盖：

- 新建文档（见 [doc-create-workflow.md](./doc-create-workflow.md)）
- 知识库空间管理
- 文档权限、消息分发、待办分派

---

## 一、核心原则

### 1.1 精准手术优于全量覆盖

**默认走精准手术**——只改用户指定的章节或 block，不动其他内容。具体路径见 §3 速查表。

### 1.2 保真约束

改写时必须**原样保留**以下要素，**不许**替换为纯文本/姓名/链接/占位符：

- `@人` 引用（用户、机器人、群）
- `@文档` / `@群` 等卡片引用
- 已上传的附件、图片
- 用户原话引用块
- 表格表头（除非语义错误且用户确认）

JSONML 模式下这些元素的节点结构见 [doc-jsonml-schema.md](../format/doc-jsonml-schema.md)。

### 1.3 编辑形态优先级

**改写已有文档优先 JSONML，markdown / element 只在 JSONML 不适用时兜底**：

| 优先级 | 形态 | 适用 |
|--------|------|------|
| ① 首选 | `--content-format jsonml` | 保真度最高；callout / 分栏 / 表格 / @人 / 附件 / 颜色 / 嵌套结构都能 1:1 round-trip；写入端有 normalize + validator 兜底（§4.4） |
| ② 次选 | `--content-format element`（JSON，老接口） | JSONML 不支持某个块字段时；或快速插入 callout / 分栏不想构造 JSONML 时；不保真改写正文 |
| ③ 兜底 | markdown（不带 `--content-format` 即默认）| 纯文本追加、整篇重排骨架；callout / 分栏 / 颜色 / 部分属性会被 markdown 还原过程丢失 |

实操判断：

- 用户给已有 nodeId 要「改一段、改属性、加 callout、动结构」——走 §4.4 JSONML 路径
- 用户要「在末尾追加一节纯文本 / 整篇按新骨架重写」——走 §4.2 / §4.5 markdown 路径
- 同一次任务里两类需求都有——分别走对应路径，**不要**为了省事全部 markdown overwrite

### 1.4 写入风险提示

`doc update` 在以下场景可能产生**静默失败**（返回 success=true 但实际写入不完整）：

- **overwrite 降级为 append**：大文档 overwrite 被后端静默降级，导致旧内容未清除、新内容追加在末尾
- **分块 append 内容截断**：超长文档分片写入时部分片段丢失或顺序错乱
- **编码/通道问题**：特殊终端下 UTF-8 内容传输乱码

因此 **每次 `doc update` 后必须回读校验**（见 §6）。整篇 overwrite 大文档前还要先向用户提示风险并等待确认。

---

## 二、读取策略

改写前必须先读现有内容，但要节省上下文。按粒度选读取方式（按 §1.3 优先级排序，**优先 JSONML**）：

| 用户需求 | 读取方式 | 定位方法 |
|----------|----------|----------|
| 单块精修（首选）| `doc block list --node <id> --content-format jsonml` → 拿 uuid → `doc block list --node <id> --content-format jsonml --block-id <uuid>` 读子树 | 节点结构见 [doc-jsonml-schema.md](../format/doc-jsonml-schema.md) |
| 多处保真改写 / 改 root sectPr | `doc read --node <id> --content-format jsonml --output /tmp/doc.json` | 解析 JSON，按 schema 操作；担心并发覆盖时记下 `revision` 供 update 透传 |
| 整篇按新骨架重写（纯文本场景）| `doc read --node <id>`（markdown 输出）| 直接处理 markdown 全文，定位用 `grep -n "<章节关键词>"` |
| 末尾追加纯文本章节 | 不必读全文，直接 §4.2 append | 必要时 `doc read` 看末尾衔接 |
| 老接口快速找 BLOCK_ID（无需 jsonml 时）| `doc block list --node <id>` | 默认输出 JSON；用 `grep -B2 -A2 "<关键词>"` 在 children 里定位（结构 `{"blocks":[{...,"children":[...]}]}`，jq 需 `..\|.text? // empty` 递归查文本） |

读取后，把改写计划告诉用户（要改哪几节、走 JSONML 还是 markdown、改成什么形态），等用户确认后再写。

---

## 三、改写路径速查

按用户请求形态查表，跳到对应详细节执行（**按 §1.3 优先级排序：JSONML 路径在前，markdown / element 兜底在后**）：

| 用户请求 | 推荐路径 | 详细节 |
|----------|----------|--------|
| 改某一章 / 某一节（首选） | block list 拿 uuid → block update --content-format jsonml | §4.4 路径 B |
| 改属性 / 改 mark / 改颜色不动文本 | block update --content-format jsonml | §4.4 路径 B |
| 插入 callout / 分栏 / 嵌套结构（首选） | block insert --content-format jsonml --element '[...]' | §4.4 路径 B |
| 多处保真改写 / 改 root sectPr | 整篇 JSONML overwrite（默认不带 --revision；并发敏感时再加） | §4.4 路径 A |
| 中间插一段纯文本 | block insert（element JSON 或 jsonml） | §4.3 / §4.4 |
| 末尾追加一节纯文本 | doc update --mode append（markdown） | §4.2 |
| 整篇按新骨架重写 | overwrite 全文（优先 JSONML；纯文本可用 markdown） | §4.5 |
| 段落转表格 / 表格转段落 | block update --content-format jsonml；或 markdown overwrite 单段 | §4.4 / §4.1 |
| 插入附件 / 图片 | doc media insert | §4.3 |
| 一次追加 >200KB 内容 | 分块 append + 用户风险确认 + 逐片记录 | §4.6 |
| 兜底：纯文本快速替换某段 | doc update --content overwrite（markdown） | §4.1 |

---

## 四、改写路径详细

> **首选 JSONML（§4.4）**——保真度最高且 normalize/validator 兜底；本节其余路径（markdown / element）仅在 §1.3 列出的"次选 / 兜底"场景下使用。

### 4.1 段落级 overwrite（markdown 兜底路径）

> 适用范围：**纯文本**改写一段或替换某节内容。若该段含 callout / 分栏 / 颜色 / @人 / 附件 / 嵌套结构，**改走 §4.4 路径 B**——markdown 还原会丢失这些元素。

```bash
dws doc update --node <nodeId> --content "<新内容>" --mode overwrite --content-format markdown
```

或写入临时文件：

```bash
dws doc update --node <nodeId> --content-file /tmp/<name>-section.md --mode overwrite --content-format markdown
```

> ⚠️ **overwrite 须用户确认**——尤其是整篇文档 overwrite。

### 4.2 追加章节（markdown）

> 适用范围：在文档末尾加 X 章 / 补充纯文本段落。追加内容若含 callout / 分栏等富结构，先用本节 append 一个占位段落，再用 §4.4 路径 B 的 `block insert --content-format jsonml` 替换/精修。

```bash
dws doc update --node <nodeId> --content-file /tmp/<name>-append.md --mode append --content-format markdown
```

按 [doc-style-guideline.md](./doc-style-guideline.md) 的元素选择规则准备追加内容。

### 4.3 块级精修（element JSON 次选路径）

> 适用范围：JSONML 不支持某个字段时，或快速插入 callout / 分栏不想构造 JSONML 时。**默认优先 §4.4 路径 B**（block update/insert `--content-format jsonml`），本节是老接口次选路径。

```bash
# 列出所有 block，定位 BLOCK_ID
dws doc block list --node <nodeId>

# 改一个 block 的文本
dws doc block update --node <nodeId> --block-id <BLOCK_ID> --text "替换后的内容" --content-format element

# 在某个 block 后插入
dws doc block insert --node <nodeId> --ref-block <BLOCK_ID> --where after --heading "补充说明" --level 2 --content-format element

# 插入复杂块（callout / 分栏）—— element 默认按 JSON 解析
dws doc block insert --node <nodeId> --ref-block <BLOCK_ID> --where after --content-format element \
  --element '{"blockType":"callout","callout":{"emoji":"⚠️","bgColor":"#FDE2E0","content":[{"text":"高风险操作，先备份"}]}}'

# 若改写过程已经在用 JSONML，整段精修也可走 jsonml 路径（uuid 必须 == --block-id）
dws doc block insert --node <nodeId> --ref-block <BLOCK_ID> --where after --content-format jsonml \
  --element '["container",{"uuid":"co_new","subType":"colorBlocks","metadata":{"bgcolor":"#FDE2E0","border":"#F5C2C7"}},["p",{"uuid":"co_new_p1"},["span",{"data-type":"text"},["span",{"data-type":"leaf"},"高风险操作，先备份"]]]]'
```

字段结构以 [doc-block.md](../doc-block.md) 为准，不要猜。callout 字段名不确定时，先用 `doc block list --node <id> --block-type callout` 抓现有 callout 实例看真实字段。整段 JSONML 形态与可复制范例见 §4.4 与 [doc-jsonml-cookbook.md](../format/doc-jsonml-cookbook.md)。

### 4.4 JSONML 无损改写（**首选路径**）

> 改写已有文档**默认走本节**——保真度最高，callout / 分栏 / 表格 / @人 / 附件 / 颜色 / 嵌套都能 1:1 round-trip；写入端有 normalize + validator 兜底。其他路径（§4.1/4.2/4.3/4.5 markdown）仅在 §1.3 列出的"次选 / 兜底"场景下使用。

两条子路径：

**路径 B：单 block JSONML 精修（最常用——只动一个 block 时的默认选择）**

```bash
# 1. 列出所有 block 拿到 uuid
dws doc block list --node <nodeId> --content-format jsonml

# 2. 读单个 block 完整子树
dws doc block list --node <nodeId> --content-format jsonml --block-id <BLOCK_UUID>

# 3. 改完后写回（uuid 必须 == --block-id）
dws doc block update --node <nodeId> --block-id <BLOCK_UUID> --content-format jsonml \
  --element '["p", {"uuid": "<BLOCK_UUID>"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "新内容"]]]'

# 在某个 block 前/后插入新 block
dws doc block insert --node <nodeId> --ref-block <BLOCK_UUID> --where after --content-format jsonml \
  --element '["container", {"uuid": "co1", "subType": "colorBlocks", "metadata": {"bgcolor": "#E8F2FE", "border": "#B3D4FC"}}, ["p", {"uuid": "co1p1"}, ["span", {"data-type": "text"}, ["span", {"data-type": "leaf"}, "提示内容"]]]]'
```

**路径 A：整篇 JSONML overwrite（一次改多处、改 root 级 sectPr 才用）**

```bash
# 1. 读出完整 JSONML 结构（输出含 revision，普通改写场景下不需要）
dws doc read --node <nodeId> --content-format jsonml --output /tmp/doc.json

# 2. 解析 JSON，修改 jsonml 数组中的目标节点
#    节点结构见 doc-jsonml-schema.md，可复制范例见 doc-jsonml-cookbook.md

# 3. 写回临时文件 /tmp/doc_modified.json，格式 {"jsonml": [...]}

# 4. 提交修改（默认直接覆盖，不做并发检查）
dws doc update --node <nodeId> --content-file /tmp/doc_modified.json \
  --content-format jsonml --mode overwrite
```

> **并发安全模式（担心被并发覆盖时使用）**：如果担心多 agent 同时改这篇文档，可以把第 1 步 read 返回的 `revision` 通过 `--revision <N>` 透传给第 4 步：服务端会做并发检查，版本不一致返回 `VersionConflict`，此时回到第 1 步重读重写即可。普通单 agent 改写场景默认不传 `--revision`。

#### JSONML 写入端的 normalize 与 validator

写入命令（`doc create/update` + `doc block insert/update`）默认按 **normalize → validate** 两步处理 JSONML：

| 行为 | 缺省 | `--fix-jsonml` | `--no-fix-jsonml` |
|------|------|----------------|-------------------|
| JSON 语法修复（括号/逗号补全） | ✗ | ✓（打印 `[FIX]`） | ✗ |
| 注入缺失的 block `uuid` | ✓ | ✓ | ✗ |
| 裸字符串 → 包成 `["span",{"data-type":"text"},["span",{"data-type":"leaf"},"..."]]` | ✓（stderr 打印 `[FIX]`） | ✓ | ✗ |
| validator 阻断（HasErrors → 拒发） | ✓ | ✓ | ✓ |

报错格式（agent 友好）：

```
$[2][2]: paragraph child must be span wrapper, got raw string.
Suggestion: ["span",{"data-type":"text"},["span",{"data-type":"leaf"},"<your text>"]]
```

三态设计：

- 不传：结构修复 ON + JSON 语法修复 OFF + 校验 ON（推荐人工调用）
- `--fix-jsonml`：全部修复 ON（含 JSON 语法修复，推荐 agent 调用）
- `--no-fix-jsonml`：全部修复 OFF，校验仍 ON；用于排查原始错误

**何时不走本节、改用 markdown**：纯文本追加章节（§4.2）、整篇按全新骨架重写（§4.5，且无富结构需要保留时）、只在乎"加一段文字"且确认目标段落无 callout / 分栏 / 颜色 / @人 / 附件。其余场景默认本节。

字段细节见 [doc-jsonml-schema.md](../format/doc-jsonml-schema.md)；可复制范例见 [doc-jsonml-cookbook.md](../format/doc-jsonml-cookbook.md)。

### 4.5 整篇 overwrite

适合「按新风格重写整篇」「按新骨架重组结构」。

**形态选择（按 §1.3 优先级）**：

- 若原文档含 callout / 分栏 / 颜色 / @人 / 附件 / 嵌套结构且需要保留——**走 §4.4 路径 A**（整篇 JSONML overwrite；默认不带 `--revision`，担心并发时再加）
- 若是纯文本骨架重写、原文档没有富结构需要保真——走本节 markdown overwrite

执行前必须先向用户**显式提示**：

> 注意：本次操作将覆盖整篇文档内容（约 {size}）。可能存在以下风险：
> - 大文档 overwrite 可能被后端静默降级为 append，导致**旧内容残留 + 新内容追加在末尾**
> - markdown overwrite 会丢失原文档的 callout / 分栏 / 颜色等富结构；如需保真改走 §4.4 路径 A
> - 写入完成后我会回读校验，发现异常会主动报告
>
> 是否继续？

得到确认后执行（markdown 兜底路径）：

```bash
dws doc update --node <nodeId> --content-file /tmp/<name>-full.md --mode overwrite --content-format markdown
```

**写入后必须回读**（§6）。如果发现旧内容残留，按 §6 的修复路径处理。

### 4.6 超长内容追加（分块 append）

当一次性追加内容 **超过 200KB** 时，必须拆分为多片 `--mode append`，并在执行第一片**之前**向用户发出截断风险提示等待确认。

完整规范（提示话术模板、触发条件、失败处理）见 [04-document.md «分块 append 截断风险提示»](../../../best_practices/04-document.md)。

update 场景下的额外约束：

1. 按段落/标题边界切分，**禁止**在表格、代码块、列表内部截断
2. 每写一片记录已写入的最后一个标题/段落标记，供 §6 回读比对
3. 与既有内容衔接位置不能产生悬空标题或断列表

---

## 五、改写时的样式约束

按 [doc-style-guideline.md](./doc-style-guideline.md) 处理：

- **文档类型保持不变**；用户明确要求转型时除外（如从「执行型 SOP」改成「说明型接口文档」）
- **同类信息保持一致**：改写时不要把原本统一的元素改为多种表达
- **颜色/emoji 语义**：改写后仍满足 style guideline §5「颜色与视觉语义」的一致性
- **不删除附件/图片**；用户明确要求时除外

---

## 六、回读验收

**所有 `doc update` 完成后都必须回读**，无论 overwrite 还是 append：

```bash
dws doc read --node <nodeId> --content-format jsonml
```

校验要点：

- 改写章节的关键标题、段落首句、表格表头是否符合预期
- overwrite 后旧内容是否真的被清除
- append 后新内容是否在期望位置
- 表格、代码块、列表跨块元素是否完整
- @人、附件、图片等保真要素是否原样保留

### 异常处理

| 现象 | 可能原因 | 处理 |
|------|----------|------|
| overwrite 后旧内容残留 + 新内容追加在末尾 | overwrite 被静默降级为 append | 告知用户 overwrite 降级，按下方「先清空再重建」路径修复 |
| append 后部分片段缺失 | 分块写入丢失 | 定位缺失片段，针对该段单独再 append 一次 |
| @人 / 附件被替换为纯文本 | 改写时未走保真约束 | 用 JSONML 无损编辑修复（§4.4）|
| 整篇内容乱序 | 写入顺序异常 | 报告给用户；若可重做，按下方「先清空再重建」路径修复 |

**禁止**在未回读的情况下向用户报告"已完成"。

---

## 七、交付口径

只报告已经验证过的信息：

- 改写涉及的章节范围
- 改写后的 nodeId 与 docUrl
- 回读验收结果（哪些章节确认改写成功、保真要素是否完整）
- 如有缺失或异常，说明具体位置和已采取的修复动作

未回读前，不要说「内容完整」「改写完成」。
