# contact 局部意图消歧

本文件从单 Skill `intent-guide.md` 拆分而来，仅保留与本产品相关的跨产品消歧规则。

| 用户说... | 真实意图 | 应该用 | 不要用 | 理由 |
|---|---|---|---|---|
| "张三在哪个部门/张三的工号是多少" | 搜人后查通讯录详情 | `aisearch person` → `contact user get` | 直接 `contact user search` | 姓名或工号先由 aisearch 获取 userId，再由 contact 补部门、工号等详情 |
| "研发部的详细信息/部门信息" | 查部门详情 | `contact dept get-info` | `contact dept list-members` | 查部门属性（ID、名称、人数）用 get-info；查成员列表用 list-members |
| "研发部有多少人" | 查部门人数 | `contact dept get-info` | `contact dept list-members` | 问人数用 get-info（返回 memberCount）；问有哪些人用 list-members |
| "找一下张三/搜同事/找人" | 人员语义搜索 | `aisearch person` | `contact user search` | 姓名模糊搜索、工号、部门、职责和上下级走 aisearch；contact 在拿到 userId 后补详情 |
| "五道的上级是谁/谁负责XX/XX的下属有谁" | AI语义搜人 | `aisearch person` | `contact` | 涉及上下级、职责、负责人等语义维度搜索，用 aisearch |
| "222020这个工号是谁/查工号" | 按工号搜人 | `aisearch person --dimension jobNumber` | `contact` | 工号查人走 aisearch，dimension=jobNumber |
| "13800138000是谁/完整手机号反查" | 精确手机号反查 | `contact user search-mobile` | `contact user search` | 完整手机号精确匹配使用 search-mobile |
| "按手机号线索找人" | 手机号语义搜人 | `aisearch person --dimension phone` | `contact user search` | 非精确手机号匹配走 aisearch 的 phone 维度 |
