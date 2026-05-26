# 通讯录（组织架构）

> **SKILL.md** 中 #8 仅内联 2 条 **lite**：`get-contact-self`、`search-user`。下列 recipe、专用规则与消歧请在命中 #8 且**超出**上述 lite 时阅读本文。  
> 产品命令见 [contact.md](../products/contact.md)。通用批量/并行见 [conventions.md](_common/conventions.md)。

## 专用规则（#8 非 lite 步骤必守）

- **角色类查人优先 label**：用户说"角色为XX的员工/XX角色的员工/XX角色的人员""所有主管/主管理员/财务/HR/总经理"等角色类型人员时，**优先** `contact label list` 获取全部角色 → 匹配目标角色 → `contact label list-members --id <labelId>`；若用户明确指定了角色名称（如"角色为总经理"），则先用 `contact label get --names <XX>` 精确匹配，**若精确匹配无结果，降级 `label list` 模糊匹配**（如用户说"管理员"可匹配到"主管理员"和"子管理员"）。
- **脚本优先**：按部门拉成员**优先** `python scripts/contact_dept_members.py --query "<部门名>"`（`--dry-run` / `--format json`）；失败再 `dept search` → `dept list-members --depts`。
- **详情链路**：用户要子部门、职位、联系方式、汇报关系等，在 `user search` 之后**必须**再 `contact user get --ids <userId>`；禁止仅用 search 的浅表字段交差。
- **`user get` 后部门仍空**：不得过早结束或只建议用户去 App；须在 CLI 能力内尝试 **用户点名的部门** `dept search` + `dept list-members` 等与 `userId` 交叉核对，再结构化汇总「返回中有哪些字段 / 哪些为空及可能原因」。
- **多命中**：`user search` 或 `dept search` 多条时须列候选（姓名、title、部门线索）请用户确认，禁止默认猜一人。具体消歧流程：
  1. 从搜索结果中提取所有同名/多命中用户的 `userId`
  2. 调用 `contact user get --ids userId1,userId2,...` 获取每人详情（含 `depts` 部门列表、职位等）
  3. 将「姓名 + 部门 + 职位」列表展示给用户，请用户确认选择哪一位
  4. 使用用户确认的 `userId` 继续后续操作
  > **根因**：`user search` 和 `aisearch person` 均不返回部门信息，无法仅凭搜索结果区分同名用户。**必须**追加 `contact user get` 获取部门信息才能消歧。
- **批量**：多个 `userId` 用 `contact user get --ids id1,id2,...`；多部门成员列表按需并行，遵守单次批量上限与 [conventions.md](_common/conventions.md)。
- **子部门枚举**：已知父部门 `deptId` 时**优先** `contact dept list-children --dept <父deptId>` 直接拿到完整子部门列表；只知道部门名时先 `contact dept search --query "<父部门名>"` 取 `deptId` 再 `list-children`；用户明示的子部门名（无需枚举）可直接 `dept search` 命中。多子部门展开见 `explore-subdepts-and-members`。

## 与其他场景消歧

- **按角色/职位类型查人（主管/管理员/财务等）** → 优先 `contact label list` + `label list-members`；label 精确命中角色维度，返回完整名单；aisearch 是语义模糊搜索不保证完整性。
- **搜人/找人/找同事/查工号/查手机号** → 首选 **`aisearch person`**（AI 语义搜索，支持姓名/部门/职责/上下级/手机号/工号维度），见 `aisearch`（开源版未引入，悟空内部产品）。
- **需要 userId 做后续操作 / 按手机号查 / 按 userId 查详情** → `contact`（精确查询）。
- **纯查部门与子部门成员 / 验证归属 / 组织关系** → `contact`。
- **终点是发消息、待办、日程** → 先用 `search-person` 或 `search-user` 取 `userId`，再进入 #1 / #2 / #3。
- **联系客户 + 发邮件** → 先用 `contact` 取 `orgAuthEmail`，再走 [mail.md](../products/mail.md)。

## Recipe 速查（本表步骤，非 SKILL lite）

| Recipe | 步骤 |
|--------|------|
| `lookup-label-members` | 1. `contact label list` → 浏览全部角色，匹配目标角色的 labelId<br>2. `contact label list-members --id <labelId>` → 该角色下的成员列表 |
| `search-user-by-mobile` | 1. `contact user search-mobile --mobile "<手机号>"` → 按需 `contact user get --ids <userId>` |
| `lookup-dept-id` | 1. `contact dept search --query "<部门关键词>"` → 回显 `deptId`（多命中须消歧） |
| `list-subdepts` | 1. 已有父 `deptId` → `contact dept list-children --dept <父deptId>` 直接取直属子部门列表<br>2. 只有部门名 → 先 `lookup-dept-id` 取 `deptId`，再 `list-children` |
| `list-dept-members` | 1. **优先** `python scripts/contact_dept_members.py --query "<部门名>"`<br>2. 备选：`lookup-dept-id` → `contact dept list-members --depts <deptId>`<br>3. 若要每人档案字段：对 `userId` 批量 `contact user get --ids …` |
| `list-multi-dept-members` | 1. 对每个部门名 `contact dept search --query "<名>"` → 各 `deptId`<br>2. `contact dept list-members --depts <id1>,<id2>,...`（多部门并行/批量见 conventions）<br>3. 需要档案再 `contact user get --ids …` |
| `verify-user-dept` | 1. `contact dept search --query "<部门名>"` → `deptId`<br>2. `contact dept list-members --depts <deptId>` 中匹配姓名；或先 `search-user` lite 再 `user get` 核对部门字段 |

## Full / 多步组合

| Recipe | 行动指南（固定路线） |
|--------|---------------------|
| explore-subdepts-and-members | 1. 取父部门 `deptId`：用户给了 ID 直接用；只给名字则 `contact dept search --query "<父部门名>"` → 父 `deptId`（多命中先消歧）<br>2. **优先** `contact dept list-children --dept <父deptId>` 拿到全部直属子 `deptId` 列表；若用户只点名了部分子部门，则改为对每个子部门名 `contact dept search --query "<子部门名>"`<br>3. 对子 `deptId`：`contact dept list-members --depts <id1>,<id2>,...`（多部门按 conventions **并行/批量**）<br>4. 若还要成员详情：汇总 `userId` → `contact user get --ids …`（≤30 条/批，超出分批 + 用户确认） |
| verify-user-in-dept | 同速查表 `verify-user-dept`；多轮对话中用户追加「是否在某部门」时叠加本路线 |
| cross-level-dept-members | 1. `contact dept search --query "<父部门关键词>"` → 父 `deptId`<br>2. **优先** `contact dept list-children --dept <父deptId>` 枚举全部直属子 `deptId`；用户已点名的子部门则用 `dept search` 精确命中<br>3. 需要逐层下钻时，对上一步拿到的子 `deptId` 继续 `dept list-children` 递归（注意控制深度，避免一次拉太多）<br>4. `contact dept list-members --depts <id1,id2,…>` → 按需 `user get` |
| user-detail-organization | 1. `contact user search --query "<关键词>"` → `userId`（多结果先消歧）<br>2. **必须** `contact user get --ids <userId>`<br>3. 若用户同时给出部门语境：叠加 `verify-user-dept` |
| batch-users-by-keyword | 1. `contact user search --query "<职位或技能关键词>"` → 多条<br>2. 提取 `userId`（≤30）→ `contact user get --ids …`<br>3. 结果过多时汇总或请用户收窄 |
