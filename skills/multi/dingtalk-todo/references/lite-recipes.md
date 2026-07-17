# todo Lite Recipe

## create-todo

1. 确定执行者：指定姓名 → `aisearch person --keyword "<姓名>" --dimension name` → userId；未指定 → `contact user get-self` → userId。
2. 多人时逐个解析 userId，以英文逗号拼接。
3. 创建：`todo task create --title "<标题>" --executors <userId>[,<userId2>...] --priority <10|20|30|40> [--due "<截止ISO>"]`。
4. 从返回中保存 `todoTaskId`，用于详情、完成或重开。

## todo-query-ops

- 查询：`todo task list [--status false|true]`
- 详情：`todo task get --task-id <id>`
- 完成/重开：`todo task done --task-id <id> --status <true|false>`

## minutes-to-todo

1. 切到 `dingtalk-minutes`，定位真实 taskUuid。
2. 执行 `minutes get todos --id <taskUuid> --format json`。
3. 对每条行动项解析执行人 userId；原听记未给执行人或截止时间时先请用户确认。
4. 使用 `todo task create` 创建任务；批量任务按返回逐条核对成功与失败项。

不要在 todo Skill 中复制听记的命令参考；听记查询、scope、分页与对象匹配规则以 `dingtalk-minutes` 为准。
