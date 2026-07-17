# 意图判断

## 意图判断

用户说"找文档/搜文档/最近文档":

- 全局搜索 → `dws drive search --query "<关键词>"`（聚合钉盘+文档空间）
- 空间内搜索 → `dws wiki node search --workspace <WS_ID> --query "<关键词>"`
- 遍历文件夹 → `dws drive list --workspace <WS_ID>` 或 `dws wiki node list --workspace <WS_ID>`
- 最近访问/最近编辑 → `dws drive recent`（见 [`drive.md`](../../../dingtalk-drive/references/drive.md)）

用户说"看文档/读内容/文档内容":

- 读取 → `read`（需文档 ID 或 URL）
- 元信息 → `info`

用户说"写文档/创建文档":

- 新建文字文档（adoc）→ `doc create`
- 追加内容 → `update --mode append`
- 覆盖替换 → `update --mode overwrite`
- 指定父目录时，只有明确的文档文件夹 `nodeId` / `alidocs` 文件夹 URL 才能放入 `--folder`；上一步若只返回了纯数字 `dentryId`、`spaceId` 或 drive `parent-id`，不要把它传给 doc 的 `--folder`

> 严禁把「创建表格」路由到 `doc create`：
>
> - 用户说"创建表格/新建表格/建个电子表格/在线表格" → 走 [`dws sheet create`](../../../dingtalk-sheet/references/sheet.md#创建钉钉表格文档)（axls 在线电子表格）
> - 用户说"创建多维表格/新建 AI 表格/建 base/数据库表" → 走 [`dws aitable base create`](../../../dingtalk-aitable/references/aitable.md#创建-ai-表格)（able 多维表格）

用户说"建文件夹/新建目录":

- 创建 → `dws drive folder create`

用户说"上传文件/传文件/上传到文档/上传到知识库":

- 上传 → `dws drive upload`（需本地文件路径）

用户说"下载/导出/下载到本地/导出文档/导出为Word/导出为docx/把文档导出来":

- **必须先判断目标文件类型**，再决定走 `export` 还是 `download`：
  - 在线文档 (alidocs/adoc) → **`doc export`**（导出是内容层操作，仅对 adoc 有意义）
  - 已有文件（PDF、图片、附件、视频等非在线文档） → **`dws drive download`**
- 判断方法：
  1. 如果用户明确说了"导出文档"、"导出为Word/docx" → 直接走 `doc export`
  2. 如果用户明确说了"下载PDF/图片/附件" → 直接走 `drive download`
  3. 不确定时，先用 `drive info --node <ID>` 查询节点信息，根据返回的 `contentType` 字段判断：
     - `contentType` 为 `ALIDOC` → 走 `doc export`
     - `contentType` 为 `DOCUMENT`/`IMAGE`/`VIDEO` 等 → 走 `drive download`

> **严禁将"导出文档"直接路由到 `download`**。`download` 只能下载已有文件（原样下载），`export` 是将在线文档格式转换后导出为 docx 或 markdown 或 pdf，两者完全不同。

用户说"导入文件/导入docx/导入Word/导入Excel/导入脑图/导入xmind/导入思维导图/把文件转成文档/文件转在线文档/导入到知识库/打开本地文件":

- **直接调用 `dws doc import --file <文件路径>`**，一条命令自动完成上传+转换+创建在线文档，**无需先读取文件内容再手动创建**
- 支持格式与映射：docx/doc→文档, xlsx/xls→表格, xmind/mark→脑图, md/txt→文档
- 文件大小限制：20MB
- 导入后会自动转换为钉钉在线文档，返回在线文档 URL

> **严禁先 Read 文件再 `doc create` + `doc update`**。`doc import` 是一体化命令，服务端负责格式转换，客户端无需解析文件内容。

用户说"复制文档/拷贝文件/复制到":

- 复制 → `dws drive copy`

用户说"移动文档/搬到/移到/转移文件":

- 移动 → `dws drive move`

用户说"重命名/rename/改名/改文档名/修改文档名称/修改文档标题/把这个文档叫做...":

- 重命名 → `dws drive rename --node <DOC_ID_OR_URL> --name "新名称"`
- 只有用户明确说"正文里的标题/章节标题/段落标题/H1 标题"时，才走 `block update`。

用户说"删除文档/删掉这个文件/移到回收站/丢掉这篇文档":

- 删除节点 → `dws drive delete`

用户说"插入附件/上传附件到文档/往文档里加文件/加附件":

- 插入附件 → `media insert`（需文档 ID 或 URL + 本地文件路径）

用户说"插入图片/加图片/放张图/嵌入图片/往文档里插图":

- 插入图片 → `media insert`（需文档 ID 或 URL + 本地图片文件路径）
- 注意：图片也是通过 `media insert` 作为附件块插入文档，不是通过 `block insert`

用户说"下载附件/获取附件/取出文档里的附件":

- 下载附件 → `media download`（需文档 ID 或 URL + 资源 ID）

用户说"编辑块/改段落/插入标题/删除块":

- 查看结构 → `block list`
- 插入 → `block insert`
- 修改 → `block update`
- 删除 → `block delete`

用户说"保存版本/创建版本快照/手动保存一下版本":

- 保存版本 → `version save`（需文档 ID 或 URL）

用户说"版本历史/版本列表/查看修改历史/历史版本":

- 查看版本列表 → `version list`（需文档 ID 或 URL）

用户说"回滚版本/恢复到某个版本/版本回退/回退到之前的版本":

- 回滚到指定版本 → `version revert`（需文档 ID 或 URL + 版本号，危险操作需确认）

用户说"给某人开权限/分享给某人/授权某文档/把这篇文档给 xxx 看":

- 新增权限 → `dws drive permission add`
- 修改权限 → `dws drive permission update`
- 查看谁有权限 → `dws drive permission list`
- 移除权限 → `dws drive permission remove`

> **关键区分**：
>
> - "把**某篇文档**授权给某人" → `drive permission add`（节点级，包括「我的文档」下的文档都支持）
> - "把**某个知识库**整体授权给某人" → `wiki member add`（容器级，但**「我的文档」个人空间不支持**）

> 补充：如果用户直接粘贴的是原始 `alidocs` URL，先按 [链接规范](../url-patterns.md#alidocs-url-类型探测流程) probe；只有 probe 确认是 `adoc` / `file` / `folder` 后，才继续按下列意图执行。

**用户直接粘贴文档 URL（无其他指令）**:

- 默认 → `read`（读取文档内容）
- 如 URL 明显是文件夹 → `list`（列出文件夹内容）

**用户粘贴 URL + 附加指令**:

- "帮我看看这个文档" → `read`
- "这个文档的信息" → `info`
- "往这个文档追加内容" → `update --mode append`
- "把这个文档标题改成 X" / "这个文档改名为 X" → `dws drive rename`
- "把正文里的一级标题/章节标题改成 X" → `block update`

关键区分: doc(文档编辑/阅读) vs aitable(数据表格操作) vs drive(钉盘文件管理)
