---
name: dingtalk-aisearch
description: AI搜问：人员语义搜索与跨源定位。Use when 按姓名/工号/部门/职责/上下级或手机号线索找人，跨文档/消息/邮件/听记检索，或回溯“我发过/收到过”。完整手机号反查走 dingtalk-contact；找到 userId 后由 contact 补详情。命令前缀：dws aisearch。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉 AI 搜问 Skill

## 前置条件 — 执行操作前必读

> **CRITICAL — 执行任何 `dws` 操作前，MUST 先用 Read 工具完整读取 [`dingtalk-shared`](../dingtalk-shared/SKILL.md)。**该轻量文件包含全局执行契约、安全底线及 shared references 的按需加载导航；不要预加载其全部 references。

> 命令参考：[aisearch.md](references/aisearch.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "找张三 / 张三是谁" | `dws aisearch person --keyword "张三" --dimension name` |
| "谁负责 XX / XX 负责人是谁" | `dws aisearch person --keyword "<XX>" --dimension duty` |
| "张三的上级 / 下级" | `dws aisearch person --keyword "张三" --dimension supervisor`（或 `subordinate`） |
| "X 部门有哪些人" | `dws aisearch person --keyword "<部门>" --dimension department` |
| "工号 12345 是谁" | `dws aisearch person --keyword "<工号>" --dimension jobNumber` |
| "按手机号线索语义搜人" | `dws aisearch person --keyword "<手机号线索>" --dimension phone` |
| "完整手机号精确反查" | `dws contact user search-mobile --mobile "<完整手机号>"` |
| "最近 OKR 相关邮件 / 项目相关文档" | `dws aisearch enterprise --queries "<主题>" --types mail/document --time-range "<时间>"` |
| "我发过/创建过/分享过/收到过什么" | `dws aisearch behavior --queries "<主题>" --behavior-type <动作> --direction <方向>` |

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 flag/ID。每条命令必须带 `--format json`，执行后必须按"解析"步取真实字段，不得凭返回结构猜测。

### SOP-1 搜人 → 拿 userId（search-person）

**触发**：姓名模糊找人/谁负责/查上下级/部门成员/工号反查/手机号线索语义搜人。

1. **定维度（必须）**：姓名→`name`、"谁负责 XX"→`duty`、部门成员→`department`、上级/下级→`supervisor`/`subordinate`、工号→`jobNumber`、手机号语义线索→`phone`；完整手机号精确反查切到 `dingtalk-contact` 的 `user search-mobile`；不确定→`all`。`--keyword` 必须按用户原文**完整保真**，切勿截断、改昵称、扩同音字。
2. **执行（必须）**：`dws aisearch person --keyword "<完整值>" --dimension <维度> --format json`。
3. **解析（必须）**：从 JSON 取 `userId` / `openDingTalkId`；**多候选必须输出让用户选，禁止默认取第一个、禁止编造**未返回的人员字段。
4. **衔接（必须）**：要邮箱/部门/职位/主管等详情 → 切 `dingtalk-contact` 执行 `dws contact user get --ids <userId> --format json`；发消息 → `dingtalk-chat`；发 DING → `dingtalk-misc`（`references/ding.md`）。
5. **失败（必须）**：未命中最多换 1 个维度重试一次（如 `name`→`department`/`jobNumber`/`phone`），仍保留完整目标值；仍无果**必须如实告知**。

**禁止**：用半截姓名扩大搜索、跳过 `--format json`、取首个候选、凭空补全人员信息。

### SOP-2 跨源搜内容（search-content）

**触发**：跨文档/邮件/消息按主题找内容。

1. **执行（必须）**：`dws aisearch enterprise --queries "<主题>" --types <document,mail,...> --time-range "<时间>" --format json`；多主题逗号分隔。
2. **衔接（必须）**：按命中来源切到对应产品 skill 读写。**aisearch 只负责"找到"，不做读写。**

**禁止**：把 aisearch 当作读写入口、跳过下游 skill 直接改数据。

### SOP-3 行为回溯（search-behavior）

**触发**："我发过/收到过/创建过/分享过什么"。

1. **执行（必须）**：`dws aisearch behavior --queries "<主题>" --behavior-type <动作> --direction <方向> --format json`。
2. **衔接（必须）**：按记录类型切对应 skill 操作；aisearch 不做读写。

**禁止**：编造行为结果、跳过 `--format json`。

## 高频硬约束

- 搜索目标必须完整保真：姓名、工号、手机号、部门名按用户原文完整传入 `--keyword`，严禁自行截断、拆字、改昵称或扩展同音字。
- 首次未命中时最多换维度重试一次（如 name → department/jobNumber/phone），仍必须保留完整目标值；不要用半截姓名扩大搜索。
- 找到候选后，如用户要邮箱、部门、职位、主管等详情，必须切到 `dingtalk-contact` 执行 `contact user get --ids <userId> --format json` 补全。
- 多候选且无法唯一判断时输出候选并询问；不要默认取第一个，也不要编造未返回的人员信息。
- 所有 `dws aisearch` 命令加 `--format json`。

## 跨产品协作

- 拿到 userId 后查详情 / 部门 → 切到 `dingtalk-contact`
- 拿到 userId 发消息 → 切到 `dingtalk-chat`
- 拿到 userId 发 DING → 切到 `dingtalk-misc`（`references/ding.md`）
## 局部意图与短流程

- [局部意图消歧](references/intent-guide.md)；[短流程](references/lite-recipes.md)。
