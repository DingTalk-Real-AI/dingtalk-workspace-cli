# Field 兼容规则终点

这是保留给 Mono/Multi 历史内容映射的兼容文件，根 Skill 不主动路由到这里。普通字段操作可直接使用下列事实；AI、关联、lookup、filterUp 或复杂 config 应由根 Skill 直接选择唯一 `references/aitable/aitable-field.md`，不要从本文件继续跳转。

| 用户意图 | 正确命令/边界 |
|---|---|
| 读取字段目录 | `dws aitable field list --base-id <B> --table-id <T>`；只取 fieldId/name/type |
| 读取完整字段配置 | `dws aitable +field-get --base-id <B> --table-id <T>`；需要 config/options 时使用 |
| 创建普通字段 | `dws aitable field create --base-id <B> --table-id <T> --name <N> --type <T> [--config <JSON>]` |
| 批量创建字段 | `field create ... --fields '<JSON_ARRAY>'`；与 `--name/--type/--config/--ai-config` 互斥 |
| 重命名/改配置 | `field update ... --field-id <F> --name <N>` 或 `--config <JSON>`；不能改字段类型 |
| 删除字段 | `dws aitable +field-delete --base-id <B> --table-id <T> --field-id <F>`；主字段和最后一个字段不可删除 |
| 调整列顺序 | 使用 `view update visible-fields`；没有 `field reorder` / `field move` |

创建 Table 时第一个字段自动成为主字段，且必须是 text。singleSelect/multipleSelect 创建时在 `config.options` 中声明选项；记录写入使用已存在的选项名称。公式、引用、创建/修改人和创建/修改时间等只读字段不得写入。

本文件是终点：不读取 cell-value、field-properties 或 Help。只有 Runtime 实际返回 `unknown command` / `unknown flag` 时才读取精确 leaf Help，并最多修正一次。
