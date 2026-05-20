# AI语音外呼 (outbound-call) 命令参考

## 命令总览

### create (创建外呼任务)

支持两种模式：**单个外呼**（独立参数）和 **批量外呼**（JSON 输入）。两种模式不可同时使用。

#### 单个外呼
```
Usage:
  dws outbound-call create [flags]
Example:
  dws outbound-call create --callee "张三" --prompt "提醒明天9点开会" --greeting "你好张三"
  dws outbound-call create --callee "张三" --prompt "提醒开会" --greeting "你好" --playbook "{}"
Flags:
      --callee string     被呼叫人 (必填)
      --prompt string     提示词 (必填)
      --greeting string   欢迎语 (必填)
      --playbook string   剧本 (可选)
```

#### 批量外呼
```
Usage:
  dws outbound-call create [flags]
Example:
  dws outbound-call create --json '[{"callee":"张三","prompt":"提醒开会","greeting":"你好","playbook":"{}"}]'
  dws outbound-call create --file ./callees.json
Flags:
      --json string   calleeInfos 的 JSON 数组字符串
      --file string   calleeInfos 的 JSON 数组文件路径，"-" 表示 stdin
```

### detail (查询外呼任务详情)

```
Usage:
  dws outbound-call detail [flags]
Example:
  dws outbound-call detail --task-id xxx
  dws outbound-call detail --task-id xxx -f json
Flags:
      --task-id string   外呼任务 ID (必填)
```

返回通话状态、开始/结束时间、听记 URL 等详细信息。

## 意图判断

用户说"外呼/打电话/AI通话":
- 创建/发起 → `outbound-call create`
- 查询/状态/详情 → `outbound-call detail`

关键区分: create=发起外呼任务, detail=查询已有任务状态

## 核心工作流

```bash
# 1. 单个外呼 — 创建外呼任务，返回 taskId
dws outbound-call create --callee "张三" --prompt "提醒明天9点开会" --greeting "你好张三"

# 2. 查询外呼结果 — 使用返回的 taskId
dws outbound-call detail --task-id <TASK_ID>
```

```bash
# 批量外呼流程
# 1. 准备 JSON 文件 callees.json:
# [
#   {"callee":"张三","prompt":"提醒明天9点开会","greeting":"你好张三"},
#   {"callee":"李四","prompt":"提醒明天9点开会","greeting":"你好李四"}
# ]

# 2. 批量创建外呼任务
dws outbound-call create --file ./callees.json

# 3. 查询外呼结果
dws outbound-call detail --task-id <TASK_ID>
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `outbound-call create` | `taskId` | `outbound-call detail --task-id` |

## calleeInfo 结构

批量外呼时，JSON 数组中每个元素的字段如下：

| 字段 | 类型 | 必填 | 说明 |
|------|------|------|------|
| `callee` | string | 是 | 被呼叫人 |
| `prompt` | string | 是 | 提示词 |
| `greeting` | string | 是 | 欢迎语 |
| `playbook` | string | 否 | 剧本 |

## 注意事项

- 单个外呼时 `--callee`、`--prompt`、`--greeting` 三个参数均为必填
- 单个外呼参数与批量外呼参数（`--json`/`--file`）互斥，不可混用
- 批量外呼的 calleeInfos 数组不能为空
- `--file` 支持传入 `"-"` 从 stdin 读取 JSON
- 创建外呼后务必记录返回的 `taskId`，后续查询详情时需要使用
