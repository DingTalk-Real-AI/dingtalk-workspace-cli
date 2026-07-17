---
name: dingtalk-aisearch
description: AI 搜问——跨源语义检索/发现（只做找到/定位，不做读写；具体读写各归 doc/report/mail/contact）。Use when 找同事/谁负责/查上下级/查工号手机号、跨文档消息邮件听记的内容检索、"我发过/收到过"行为回溯。模糊找人拿 userId 后精确查走 dingtalk-contact。命令前缀：dws aisearch。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉 AI 搜问 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

## 前置条件 — 执行操作前必读

> **`use_skill(dws-shared)`** — 认证、全局参数（`--format json` / `--yes`）、错误码、URL 模板、跨产品消歧、安全规则与 capability 边界。**执行任何 `dws` 命令前先读；** 单产品的清晰命令可直接用本 skill。

<!-- SAFETY_PREAMBLE_INJECT -->

> 命令参考：[aisearch.md](references/aisearch.md)。

## 意图表

| 用户说 | 命令 |
|--------|------|
| "找张三 / 张三是谁" | `dws aisearch person --keyword "张三" --dimension name` |
| "谁负责 XX / XX 负责人是谁" | `dws aisearch person --keyword "<XX>" --dimension duty` |
| "张三的上级 / 下级" | `dws aisearch person --keyword "张三" --dimension supervisor`（或 `subordinate`） |
| "X 部门有哪些人" | `dws aisearch person --keyword "<部门>" --dimension department` |
| "工号 12345 是谁 / 138xxxx 手机号是谁" | `dws aisearch person --keyword "<工号>" --dimension jobNumber` / `dws aisearch person --keyword "<手机号>" --dimension phone` |
| "最近 OKR 相关邮件 / 项目相关文档" | `dws aisearch enterprise --queries "<主题>" --types mail/document --time-range "<时间>"` |
| "我发过/创建过/分享过/收到过什么" | `dws aisearch behavior --queries "<主题>" --behavior-type <动作> --direction <方向>` |

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 flag/ID。每条命令必须带 `--format json`，执行后必须按"解析"步取真实字段，不得凭返回结构猜测。

### SOP-1 搜人 → 拿 userId（search-person）

**触发**：找人/谁负责/查上下级/部门成员/工号或手机号反查。

1. **定维度（必须）**：姓名→`name`、"谁负责 XX"→`duty`、部门成员→`department`、上级/下级→`supervisor`/`subordinate`、工号→`jobNumber`、手机号→`phone`；不确定→`all`。`--keyword` 必须按用户原文**完整保真**，切勿截断、改昵称、扩同音字。
2. **执行（必须）**：`dws aisearch person --keyword "<完整值>" --dimension <维度> --format json`。
3. **解析（必须）**：从 JSON 取 `userId` / `openDingTalkId`；**多候选必须输出让用户选，禁止默认取第一个、禁止编造**未返回的人员字段。
4. **衔接（必须）**：要邮箱/部门/职位/主管等详情 → 切 `dingtalk-contact` 执行 `dws contact user get --ids <userId> --format json`；发消息 → `dingtalk-chat`；发 DING → `dingtalk-ding`。
5. **失败（必须）**：未命中最多换 1 个维度重试一次（如 `name`→`department`/`jobNumber`/`phone`），仍保留完整目标值；仍无果**必须如实告知**。

**禁止**：用半截姓名扩大搜索、跳过 `--format json`、取首个候选、凭空补全人员信息。

### SOP-2 跨源搜内容（search-content）

**触发**：跨文档/邮件/消息按主题找内容。

1. **执行（必须）**：`dws aisearch enterprise --queries "<主题>" --types <document,mail,...> --time-range "<时间>" --format json`；多主题逗号分隔。
2. **衔接（必须）**：按命中来源切下游读写——文档→`dingtalk-doc`、邮件→`dingtalk-mail`、消息→`dingtalk-chat`。**aisearch 只负责"找到"，不做读写。**

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
- 拿到 userId 发 DING → 切到 `dingtalk-ding`
## 局部意图与 Recipe

- [局部意图消歧](references/intent-guide.md)；[Lite Recipe](references/lite-recipes.md)。
