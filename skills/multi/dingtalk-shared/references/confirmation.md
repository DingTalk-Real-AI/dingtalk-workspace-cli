# 确认门禁协议

写操作与高风险操作必须以最终 Runtime gate / leaf Schema 的 `confirmation` 为准。
本文件是 multi 布局下的**全局协议**；各产品危险表示例索引见文末，细节仍以产品
reference 为准。

## 确认流程

```text
Step 1 → 展示操作摘要（操作类型 + 目标对象 + 影响范围）
Step 2 → 用户明确回复确认（如「确认」/「好的」）
Step 3 → 在原始命令末尾追加 --yes 执行（不改动业务参数）
```

## `confirmation_required` 识别与重试

非交互环境（Agent/CI，stdin 非 TTY）下，写命令不带 `--yes` 时 CLI **不打印交互提示语**，
直接失败并输出结构化错误。识别方式：

- `--format json` 输出（或 stderr）中 `error.reason == "confirmation_required"`
- 错误信息通常含「当前环境无法交互确认」

遇到 `confirmation_required` 时：

1. **不要当普通错误放弃**：把命令、风险等级与关键参数展示给用户，明确这是写/高风险操作
2. 用户显式同意 → 在**原始命令**末尾追加 `--yes` 重试（不改动任何业务参数）
3. 用户拒绝 → 终止，不得改写参数绕过门禁
4. 想先让用户 review 具体请求：加 `--dry-run` 重试——它**不触发确认门禁**，会输出完整调用预览（`invocation.params`），用户确认预览后再换 `--yes` 执行

**禁止**：

- 看到 `confirmation_required` 就未经用户同意自动追加 `--yes` 静默重试
- 把 `confirmation_required` 当网络/权限错误处理或重试
- 用 `echo yes | dws ...` 等管道方式喂答案代替 `--yes`（技术上可能被接受，但违背显式知悉意图）
- 换成确认更弱的底层命令来「绕过」门禁

## 与 Schema 的关系

- leaf Schema / Shortcut 中 `confirmation=user_required` → 必须先向用户确认再加 `--yes`
- 不要根据 `effect` 或 `risk` 的值自行改写最终 confirmation winner
- 字段解读见 [schema-usage.md](./schema-usage.md)

`error-codes.md` 中对 `reason=confirmation_required` 的一行提示指向本协议；完整步骤以本文为准。

## 高影响操作索引（入口）

完整命令与边界在对应产品 skill；此处只给冷启动索引：

| 产品面 | multi skill | 危险/确认细节落点 |
|---|---|---|
| AI 表格删除类 | `dingtalk-aitable` | 产品 `SKILL.md` / aitable references |
| 日历删除/取消 | `dingtalk-calendar` | calendar references |
| 群成员移除 / 机器人撤回 | `dingtalk-chat` | chat references |
| 文档删除 / 块删除 / 权限降权 | `dingtalk-doc` | doc references |
| 待办删除 | `dingtalk-todo` | todo references |
| 听记全文替换 | `dingtalk-minutes` | minutes references |
| DING 撤回 / OA 拒绝撤销等 | `dingtalk-misc` | `ding` / `oa` 等 references |

不确定时：先 `dws schema --cli-path "<path>" --compact --format json` 看 `confirmation`，再按本协议执行。
