# AITable 局部意图消歧

| 用户表达 | 归属 | 理由 |
|---|---|---|
| AI 表格、多维表、Base、Table、字段、记录、视图、表单、仪表盘、自动化 | AITable | 操作 AITable 的业务数据与配置 |
| 搜索 Base 候选、按关键词找 Base、检查某 Base 是否存在 | AITable | 直接使用 `+base-search --query`；即使关键词像人名，只要对象是 Base，也不得改走 `aisearch person` |
| 表格链接，需要读取记录 | AITable | 先用 `+url-resolve` 取稳定 ID，再用 `+record-query` |
| 只有 Base/Table 名称，需要读取记录 | AITable | 先用 `+resolve-base` / `+resolve-table` 唯一解析，再查询记录 |
| 只复制 Base 结构、删除整个 Base | AITable | 复制到已知文档文件夹用 `+base-copy --target-folder-id ... --only-struct`；删除用真实 baseId。不要 Drive 完整复制后逐表删数据 |
| Base 整体移动到普通文件夹、外层存储重命名 | Drive | 这是 Base 作为单个存储节点的外层位置/名称动作 |
| Base 内 Table、Dashboard、Section 的复制/移动/重命名/删除 | AITable | 这些是 Base 内 nsheet/业务结构，不是独立 Drive dentry |
| Base 角色、高级权限 | AITable | `+role-*` / `+advperm-*`；仅普通文件 ACL 才走 Drive |
| 记录主键文档正文 | Doc | AITable 只取/建关联，正文由 Doc 处理 |
| Excel 式单元格、区域、工作表、公式 | Sheet（dingtalk-misc） | 二维电子表格，不是多维表记录模型 |
| CSV/JSON 数据进入现有 AI 表格 | AITable import 或 record create | 需要保留导入任务语义时用 import；已映射字段时直接写记录 |

若链接类型不明确，先做 URL 类型预检；不要仅凭 URL 文本猜产品。明确是 AI 表格后才加载本 Skill。
