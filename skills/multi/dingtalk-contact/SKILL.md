---
name: dingtalk-contact
description: 钉钉通讯录查询与组织管理（用户、部门、角色、花名册、离职员工、创建企业、企业账号、邀请员工）。Use when 用户说 查部门/我的信息/按 userId 查/离职员工/创建企业/企业账号/邀请员工。Distinct from dingtalk-aisearch(模糊搜人首选：找同事/查上下级/谁负责)。命令前缀：dws contact。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉通讯录 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

> **PREREQUISITE:** Read the `dws-shared` skill first for auth, global flags, product routing, URL preflight, error codes, and safety rules. The `dws` binary must be on PATH.

<!-- SAFETY_PREAMBLE_INJECT -->

> ⚠️ **命令可用性以当前 dws 二进制为准**。服务发现已下线，本文档随内置 skill 发布；如果 `dws <cmd> --help` 不存在，说明当前版本未暴露该命令。若命令存在但调用失败，请按错误中的 endpoint 或 tool 提示确认静态端点目录和后端工具注册。实际调用前可用 `dws <cmd> --help` 或 `--dry-run` 验证。


> 命令参考：[contact.md](references/contact.md)；剧本：[08-directory.md](references/08-directory.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service contact --format json` 读取参数、约束、风险和示例，并以 `dws contact <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

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
| "按部门名拉成员" | `python scripts/contact_dept_members.py --query "<部门名>"` |
| "搜部门" | `dws contact dept search --query "<关键词>"` |
| "部门成员列表" | `dws contact dept list-members --depts <deptId>` |
| "离职员工/离职名单/已离职" | `dws contact user dismission search`（可加 `--name` / `--start` + `--end` / `--depts`） |
| "花名册/员工档案/学历/银行卡/合同" | `dws contact user profile get --staff-id <STAFF_ID>`（先 `profile fields` 查字段） |
| "创建企业/新建企业/初始化企业" | `dws contact org create --org-name "<企业名>" --creator-username "<创建者名称>"` |
| "创建企业账号/专属账号/企业登录账号" | `dws contact account create --org-user-name "<姓名>" --login-id "<登录号>"` |
| "邀请员工/添加员工/新员工入职" | `dws contact user invite --org-user-name "<姓名>" --org-user-mobile "<手机号>"` |

## 评测高频硬约束

- 通讯录问题必须调用 `dws contact` 或 `dws aisearch` 获取实时结果；严禁只读 `USER.md`、环境身份或静态上下文后直接回答。
- 查自己用 `dws contact user get-self --format json`，不要把 `me/self/current` 当作 `userId` 传给 `user get`。
- 精确找人、按工号、按手机号：先用 `dws aisearch person --keyword "<完整输入>" --dimension name/jobNumber/phone --format json` 或对应 `contact user search/search-mobile`；拿到 `userId` 后必须 `dws contact user get --ids <userId> --format json` 补部门/职位/邮箱。
- 查询直属主管/上下级时，如果 `contact user get` 没返回明确主管字段，必须继续 `dws aisearch person --keyword "<完整姓名或工号>" --dimension supervisor --format json`，不要停在"可能需要进一步查询"。
- 多个同名候选时，批量 `contact user get --ids id1,id2,... --format json` 获取部门/职位后再消歧；不要默认取第一个。
- "创建企业账号"必须优先匹配长模式 `account create`，不要误路由为 `org create`；三条组织管理命令都是写操作，执行前确认当前企业与目标信息。

## 跨产品协作

- 模糊找人（姓名 / 上下级 / 谁负责 / 工号 / 手机号）→ 切到 `dingtalk-aisearch`
- 拿到 email 发邮件 → 切到 `dingtalk-mail`
- 拿到 userId 发消息 → 切到 `dingtalk-chat`
