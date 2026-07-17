# contact Lite Recipe

本文件从单 Skill `lite-recipes.md` 拆分而来，仅保留与本产品相关的轻量流程。

## #8 通讯录

### get-contact-self

`contact user get-self` → 当前用户 userId、部门、主管等

### search-person

**搜人首选入口**。凡是“找人/搜人/找同事/谁负责/上级/下级/负责人/团队成员”均优先用 `aisearch person`：

1. 从用户问题中提取 keyword（人名/业务关键词）和 dimension（维度），规则见 [aisearch.md](../../dingtalk-aisearch/references/aisearch.md)。
2. `aisearch person --keyword "<关键词>" --dimension <维度>`
3. 结果中提取 `userId` 和 `title`（姓名）展示给用户。
4. 若需要 userId 做后续操作（发消息/建待办），可直接使用结果中的 `userId`。
5. **重名消歧**：多人同名时禁止默认选第一个，须追加 `contact user get --ids` 获取部门/职位后请用户确认，详见 [08-directory.md](../../dingtalk-contact/references/08-directory.md)「多命中」。

### search-user

仅在以下**精确查询**场景使用，搜人请优先用 `search-person`：

- 需要获取 userId 给其他产品使用（发消息/建待办/约日程）
- 已有 userId 需查完整详情（`contact user get --ids`）

1. `aisearch person --keyword "<姓名>" --dimension name` → `userId`；**多命中须列出候选请用户确认**。
2. **重名消歧**：多人同名时禁止默认选第一个，须追加 `contact user get --ids` 获取部门/职位后请用户确认，详见 [08-directory.md](../../dingtalk-contact/references/08-directory.md)「多命中」。
3. 需详情时：`contact user get --ids <userId>`（多人可 `--ids id1,id2,...`）
