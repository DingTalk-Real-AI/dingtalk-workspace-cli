# contact 局部意图消歧

本文件从单 Skill `intent-guide.md` 拆分而来，仅保留与本产品相关的跨产品消歧规则。

| 用户说... | 真实意图 | 应该用 | 不要用 | 理由 |
|---|---|---|---|---|
| "张三在哪个部门/查一下同事工号" | 通讯录精确查询 | #8 `contact` | #5 汇报 / #4 文档 | 需要 userId、手机号、部门 ID 等精确信息时用 contact |
| "研发部的详细信息/部门信息" | 查部门详情 | `contact dept get-info` | `contact dept list-members` | 查部门属性（ID、名称、人数）用 get-info；查成员列表用 list-members |
| "研发部有多少人" | 查部门人数 | `contact dept get-info` | `contact dept list-members` | 问人数用 get-info（返回 memberCount）；问有哪些人用 list-members |
| "找一下张三/搜同事/找人" | AI搜人(首选) | `aisearch person` | `contact user search` | 搜人首选 aisearch，支持姓名/部门/职责/上下级维度；精确查 userId/手机号用 contact |
| "五道的上级是谁/谁负责XX/XX的下属有谁" | AI语义搜人 | `aisearch person` | `contact` | 涉及上下级、职责、负责人等语义维度搜索，用 aisearch |
| "222020这个工号是谁/查工号" | 按工号搜人 | `aisearch person --dimension jobNumber` | `contact` | 工号查人走 aisearch，dimension=jobNumber |
| "13800138000是谁/查手机号" | 按手机号搜人 | `aisearch person --dimension phone` | `contact` | 手机号查人走 aisearch，dimension=phone |
