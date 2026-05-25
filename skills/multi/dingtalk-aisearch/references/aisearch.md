# aisearch — AI 搜问

> `aisearch` 模块**规范子命令是 `person`**，所有调用请使用 `dws aisearch person --keyword "..."`。
>
>  **容错说明**（无需主动使用）：CLI 兼容下列 alias 兜底，模型偶尔写 `search` / `find` / `query` / `contact` / `people` 等也能跑通——但**输出和文档以 `person` 为准**。
>
>  keyword flag 同样兼容 `--name` / `--q` / `--query` / `--text` 兜底，规范用法是 `--keyword`。

## 企业人员搜索

通过关键词搜索企业内人员信息，支持按维度筛选。

```
Usage:
  dws aisearch person [flags]
Example:
  dws aisearch person --keyword "张三" --dimension name --format json
  dws aisearch person --keyword "产品部" --dimension department --format json
  dws aisearch person --keyword "五道" --dimension supervisor --format json
  dws aisearch person --keyword "AI搜问" --dimension duty --format json
  dws aisearch person --keyword "李四" --dimension name,department --format json
  dws aisearch person --keyword "13800138000" --dimension phone --format json
  dws aisearch person --keyword "W12345" --dimension jobNumber --format json
Flags:
      --keyword string     搜索关键词 (必填，如人名、技能关键词等)
      --dimension string   查询维度，多个用逗号分隔 (默认 "all")
```

### dimension 可选值

| 值 | 含义 | 触发词 |
|----|------|--------|
| `all` | 全部维度（默认） | — |
| `name` | 姓名 | "叫什么"、"是谁" |
| `department` | 部门 | "部门"、"团队"、"哪个部门" |
| `position` | 职位 | "职位"、"岗位"、"职级" |
| `duty` | 职责/技能 | "负责什么"、"职责"、"技能"、"负责人" |
| `supervisor` | 上级 | "上级"、"领导"、"主管" |
| `subordinate` | 下级 | "下级"、"下属"、"团队成员" |
| `phone` | 手机号 | "手机号是多少"、"电话"、"联系方式" |
| `jobNumber` | 工号 | "工号"、"工号是多少"、"员工编号" |

### keyword 提取规则

仅填入实际的搜索目标（人名、技能关键词等），不包含查询维度词。维度词必须映射到 `--dimension`：

| 用户说 | keyword | dimension |
|--------|---------|-----------|
| "五道的上级是谁" | 五道 | supervisor |
| "张三负责什么" | 张三 | duty |
| "AI搜问的负责人是谁" | AI搜问 | duty |
| "产品部有谁" | 产品部 | department |
| "李四是哪个部门的" | 李四 | department |
| "13800138000是谁" | 13800138000 | phone |
| "工号W12345是谁" | W12345 | jobNumber |

---

## 意图判断

- 用户说"搜人/找人/谁负责/上级是谁/哪个部门的人" → `aisearch person`
- 用户说"搜同事/查部门/查通讯录" → `contact`（通讯录）

**关键区分**：`aisearch person`（AI 语义搜人，支持按维度筛选：职责/手机号/上级/下级等）vs `contact`（通讯录精确查询：userId/部门成员列表）

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `aisearch person` | `userId`（用户ID）、`title`（姓名） | 展示搜索结果、后续操作（发消息/建待办等） |

## 重名消歧

> **CAUTION:** 多人同名时禁止默认选第一个 — 须追加 `contact user get --ids userId1,userId2,...` 获取部门/职位后请用户确认。详见 `dingtalk-contact/references/08-directory.md`「多命中」。
