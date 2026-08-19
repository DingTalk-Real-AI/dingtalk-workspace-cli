---
name: dingtalk-contact
description: 钉钉通讯录精确查询。Use when 已有 userId 后查详情、部门、职位或邮箱，按完整手机号反查用户，或查询自己、部门成员及角色。姓名模糊搜索、工号、职责、上下级走 dingtalk-aisearch，拿到 userId 后用本 skill 补详情。命令前缀：dws contact。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉通讯录 Skill

## 前置条件 — 执行操作前必读

> **CRITICAL — 执行任何 `dws` 操作前，MUST 先用 Read 工具完整读取 [`dingtalk-shared`](../dingtalk-shared/SKILL.md)。**该轻量文件包含全局执行契约、安全底线及 shared references 的按需加载导航；不要预加载其全部 references。

> 命令参考：[contact.md](references/contact.md)；剧本：[08-directory.md](references/08-directory.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 Agent leaf Schema（例如 `dws schema --cli-path "contact +<shortcut>" --compact --format json`），在当前 Cobra flags 不确定时读取 `dws contact <shortcut> --help`。只有参数映射、接口绑定或 provenance 审计才省略 `--compact`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service contact --format json` 批量发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws contact +by-mobile` | read | 按手机号查询某人的完整资料（自动解析 userId 后取详情） |
| `dws contact +dept-members` | read | 按部门名列出部门成员（自动解析 deptId） |
| `dws contact +list-dept-members` | read | 查看部门成员（仅本部门，不含下级） |
| `dws contact +list-followings` | read | 获取当前用户的特别关注列表 |
| `dws contact +list-role-members` | read | 查询角色下的成员列表 |
| `dws contact +list-roles` | read | 获取企业所有角色（标签）列表 |
| `dws contact +list-sub-depts` | read | 查看指定部门的子部门 |
| `dws contact +lookup` | read | 按姓名查询某人的完整资料（自动解析 userId 后取详情） |
| `dws contact +me` | read | 查看我自己的通讯录资料（姓名/userId/手机/部门/组织，干净投影） |
| `dws contact +org` | read | 按姓名查某人所在部门的详情（自动解析 userId 与 deptId） |
| `dws contact +resolve-dept` | read | 按名称搜索部门并解析出唯一 deptId（只读） |
| `dws contact +search-mobile` | read | 按手机号搜索通讯录用户 |
| `dws contact +search-user` | read | 按关键词搜索通讯录用户 |
| `dws contact +team` | read | 按姓名列出某人所在部门的成员（自动解析 userId 与 deptId） |
<!-- VISIBLE_SHORTCUTS_END -->

## 意图表

| 用户说 | 命令 |
|--------|------|
| "查我自己的信息" | `dws contact user get-self` |
| "按 userId 查详情" | `dws contact user get --ids <userId1>,<userId2>,...`（多个并行） |
| "完整手机号反查用户" | `dws contact user search-mobile --mobile <手机号>` |
| "按部门名拉成员" | `python scripts/contact_dept_members.py --query "<部门名>"` |
| "搜部门" | `dws contact dept search --query "<关键词>"` |
| "部门成员列表" | `dws contact dept list-members --ids <deptId>` |
| "列出企业角色 / 有哪些角色" | `dws contact label list` |
| "按角色名查角色ID" | `dws contact label get --names "<角色名>"` |
| "查某角色下有哪些成员" | `dws contact label list-members --id <labelId>` |

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 userId。每条命令必须带 `--format json`。姓名模糊搜索、工号、职责与上下级走 `dingtalk-aisearch`；完整手机号精确反查走 contact；拿到 userId 后由 contact 补详情。

### SOP-1 搜人（search-person）

**触发**：按姓名/工号/部门/职责/上下级找人，或用手机号线索做语义搜索。

1. **切 aisearch（必须）**：`dws aisearch person --query "<关键词>" --dimension <维度> --format json`（姓名→`name`、工号→`jobNumber`、手机号语义线索→`phone`、负责人→`duty`、部门→`department`、上下级→`supervisor`/`subordinate`）。
2. **解析（必须）**：从结果取 `userId`、`title`；**多人同名禁止默认选第一个**，必须批量 `dws contact user get --ids <id1,id2,...> --format json` 拿部门/职位后让用户确认。
3. **补详情（必须）**：要完整部门/职位/邮箱/主管时 `dws contact user get --ids <userId> --format json`。

**禁止**：用 `contact user search` 做姓名或工号搜索、默认取首个候选、编造人员字段。完整手机号精确反查是 `search-mobile` 的唯一搜索例外。

### SOP-1A 完整手机号精确反查（search-person-by-mobile）

**触发**：用户提供完整手机号并要求确认是谁或取得 userId。

1. **执行（必须）**：`dws contact user search-mobile --mobile "<完整手机号>" --format json`。
2. **补详情（按需）**：从结果取 `userId`，需要部门、职位或邮箱时继续 `dws contact user get --ids <userId> --format json`。

**禁止**：把完整手机号精确反查改走姓名搜索，或在未返回 userId 时猜测人员。

### SOP-2 精确查人/补详情（search-user）

**触发**：已有 userId 要查完整详情，或要拿 userId 给下游（发消息/建待办/约日程）。

1. **拿 userId（必须）**：`dws aisearch person --query "<姓名>" --dimension name --format json` → `userId`；多命中必须列候选请用户确认。
2. **查详情（必须）**：`dws contact user get --ids <userId> --format json`，按返回字段（`orgEmployeeModel` 下部门/职位/邮箱）答复。

**禁止**：用模糊关键词直接调 `contact user search` 凑数、编造未返回字段。

### SOP-3 查自己（get-contact-self）

**触发**：我的信息/我的 userId/我的部门。

1. **执行（必须）**：`dws contact user get-self --format json`，取 `orgEmployeeModel.userId` / `orgUserName` / `depts[].deptName` / 主管等。

**禁止**：把自己 userId 写死或猜测。

### SOP-4 查部门 / 角色（dept-and-relation）

**触发**：部门列表/部门成员/角色/角色成员。

1. **执行（必须）**：搜部门 `dws contact dept search --query "<部门名>" --format json`；某部门下子部门 `dws contact dept list-children --dept <父部门ID> --format json`；部门成员 `dws contact dept list-members --ids <部门ID>[,<部门ID2>...] --format json`；部门详情 `dws contact dept get-info --dept <部门ID> --format json`。角色：`dws contact label list` / `dws contact label get --names "<角色名>"` / `dws contact label list-members --id <labelId>`。搜索企业根部门时服务端可能返回 `deptId=-1` 哨兵，后续 `list-children` / `list-members` / `get-info` 必须规范化为真实根部门 `deptId=1`。
2. **补详情（必须）**：拿到 userId 后用 `contact user get --ids` 补部门/职位；上下级关系优先经 `dingtalk-aisearch` 的 `supervisor`/`subordinate` 维度。

**禁止**：使用不存在的 `contact dept list`（已废弃/歧义）、编造 deptId/labelId、跳过 aisearch 维度直接猜上下级。

## 高频硬约束

- 通讯录问题必须调用 `dws contact` 或 `dws aisearch` 获取实时结果；严禁只读 `USER.md`、环境身份或静态上下文后直接回答。
- 查自己用 `dws contact user get-self --format json`，不要把 `me/self/current` 当作 `userId` 传给 `user get`。
- 姓名模糊搜索、工号反查、职责或上下级搜索走 `dws aisearch person`；完整手机号精确反查走 `dws contact user search-mobile --mobile "<手机号>" --format json`。拿到 `userId` 后按需 `dws contact user get --ids <userId> --format json` 补部门/职位/邮箱。
- 查询直属主管/上下级时，如果 `contact user get` 没返回明确主管字段，必须继续 `dws aisearch person --query "<完整姓名或工号>" --dimension supervisor --format json`，不要停在"可能需要进一步查询"。
- 多个同名候选时，批量 `contact user get --ids id1,id2,... --format json` 获取部门/职位后再消歧；不要默认取第一个。
- 用户查询企业角色、角色ID、角色成员，或“管理员/财务/HR/主管”等角色类型人员时，走 `contact label list/get/list-members`；不要用 `dept list-members` 筛字段替代。

## 跨产品协作

- 姓名模糊搜索、上下级、谁负责、工号反查、手机号语义搜索 → `dingtalk-aisearch`
- 完整手机号精确反查 → `dws contact user search-mobile`
- 拿到 email 发邮件 → 切到 `dingtalk-mail`
- 拿到 userId 发消息 → 切到 `dingtalk-chat`
## 局部意图与短流程

- [局部意图消歧](references/intent-guide.md)；[短流程](references/lite-recipes.md)。
