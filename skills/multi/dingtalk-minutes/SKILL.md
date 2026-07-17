---
name: dingtalk-minutes
description: 钉钉 AI 听记查看、摘要、转写、待办提取与分享。Use when 用户说听记/会议录音/会议纪要/AI摘要/转写/关键字/听记标题/会后待办提取/分享听记。本地音视频转纪要/逐字稿也优先用本 skill，不要用 ffmpeg/whisper 本地转写。不做听记生成报告文档（走配方 minutes-report-to-doc）、听记补充到已有文档（走配方 minutes-to-doc）、日程（走 dingtalk-calendar）。命令前缀：dws minutes。
cli_version: ">=0.2.14"
metadata:
  category: product
  stability: experimental
  requires:
    bins:
      - dws
---

# 钉钉 AI 听记 Skill

> 🧪 **EXPERIMENTAL · 试验版 / Preview** — multi 模式当前未达 stable 标准。全部 dingtalk-* skill 已通过 dispatch verifier，但接口、命名、跨 skill 引用后续可能调整；生产 / 共享环境请优先使用 mono 模式（`dws skill setup --mode mono`）。问题请提 issue 反馈。

## 前置条件 — 执行操作前必读

> **`use_skill(dws-shared)`** — 认证、全局参数（`--format json` / `--yes`）、错误码、URL 模板、跨产品消歧、安全规则与 capability 边界。**执行任何 `dws` 命令前先读；** 单产品的清晰命令可直接用本 skill。

<!-- SAFETY_PREAMBLE_INJECT -->

> 渐进式参考：[minutes-index.md](references/minutes-index.md)；剧本：[07-minutes.md](references/07-minutes.md)。先按索引选择专题，不要一次性加载全部听记参考。

> 旧路径兼容入口：[minutes.md](references/minutes.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 来自独立于 Runtime Schema 的公开 catalog。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。用 `dws shortcut list --service minutes --format json` 读取参数、约束、风险和示例，并以 `dws minutes <shortcut> --help` 核对当前 Cobra flags；不要对 `+` 路径调用 `dws schema`。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws minutes +detail` | read | 一条命令聚合取一条妙记（听记）的多项产物（基础信息/摘要/关键词/逐字稿/待办） |
| `dws minutes +list-all` | read | 查询我有权限访问的所有听记列表 |
| `dws minutes +list-mine` | read | 查询我创建的听记列表 |
| `dws minutes +list-shared` | read | 查询他人共享给我的听记列表 |
| `dws minutes +record-start` | write | 发起听记（开始录音） |
| `dws minutes +replace-batch` | write | 对一条妙记（听记）批量执行多组文字替换（原文=>替换） |
<!-- VISIBLE_SHORTCUTS_END -->

## 意图表

| 用户说 | 命令 |
|--------|------|
| "我的听记列表" | `dws minutes list mine [--query "<关键词>"] [--start "<ISO>"] [--end "<ISO>"]` |
| "查某段时间/最近/本周/上月的听记" | `dws minutes list all --start "<ISO>" --end "<ISO>" [--query "<关键词>"]` |
| "看一篇听记摘要" | `dws minutes get summary --id <taskUuid>` |
| "看转写 / 原文" | `dws minutes get transcription --id <taskUuid>` |
| "下载/导出最近一周听记原文/逐字稿" | `dws minutes list all --start "<ISO>" --end "<ISO>"` → 逐条 `dws minutes get transcription --id <taskUuid>` |
| "近期听记摘要合并" | `python scripts/minutes_recent_summary.py --max 5` |
| "提取会议待办" | `python scripts/minutes_extract_todos.py --id <taskUuid>` |
| "改听记标题" | `dws minutes update title --id <taskUuid> --title "<新标题>"` |

## 标准 SOP（必遵流程）

> 命中以下意图**必须**按对应 SOP 顺序执行；**禁止**跳步、替换命令、编造 taskUuid。每条命令必须带 `--format json`。锁定目标**必须**按 `taskUuid + title + organizationName + 时间` 精准匹配，**禁止**凭标题相似度乱选。

### SOP-1 查听记列表（query-minutes）

**触发**：查听记/会议记录/某会议的纪要/按关键词或时间找听记。

1. **选 scope（必须·铁律）**：`mine`=我创建/发起的；`shared`=他人共享给我的；`all`=我可访问的全部（含 mine+shared）。用户说"我能访问/可见/所有/我的听记"等覆盖语义**一律 `all`**；只有明确"我创建的/我发起的/我录的"才用 `mine`。
2. **执行（必须）**：`dws minutes list all|mine|shared --format json`；按关键词加 `--query "<关键词>"`，按时间加 `--start "<ISO>" --end "<ISO>"`，限制条数用 canonical 参数 `--limit <n>`，翻页用 `--cursor <token>`。
3. **解析（必须）**：从 `itemList[]` 取真实 `taskUuid` + `title` + 时间；多候选必须让用户确认，**禁止**默认取第一条。

**禁止**：把 `mine` 当全量、凭标题相似度锁定、跳过 `--format json`。

### SOP-2 取听记详情（get-minute-detail）

**触发**：要摘要/转写原文/关键词/待办/基础信息/音频。

1. **前置（必须）**：先按 SOP-1 拿到目标 `taskUuid`。
2. **执行（必须·按需选一）**：摘要 `dws minutes get summary --id <taskUuid>`；转写原文 `dws minutes get transcription --id <taskUuid>`（返回 `cursor`/`nextToken` 时**必须**继续 `--cursor` 翻页拉全）；关键词 `get keywords`；待办 `get todos`；基础信息 `get info`；音频地址 `get audio`；批量 `get batch --ids <uuid1,uuid2>`。全部带 `--format json`。
3. **解析（必须）**：`--id`/`--uuid`/`--task-uuid` 三者等价，**推荐统一 `--id`**；转写/摘要按用户需要抽取，**禁止**无差别全文贴出。

**禁止**：编造 taskUuid、转写有分页不翻完就总结、跳过 SOP-1 直接猜 ID。

### SOP-3 听记转文档 / 待办回写（handoff）

**触发**：把会议纪要转成文档/把听记待办建到待办系统。

1. **转文档（必须）**：取 `minutes get summary`/`transcription` 内容 → 切 `dingtalk-doc` 用 `dws doc create --content-file <tmp.md>` 落盘（或用 `dingtalk-products-skills/minutes-to-doc` recipe skill）。
2. **待办回写（必须）**：取 `minutes get todos` 的待办项 → 切 `dingtalk-todo` 按 SOP-1 解析执行者 userId 后 `todo task create`。

**禁止**：在 minutes 内直接写文档/建待办（应切对应 skill）。

## 高频硬约束

- `shanji.dingtalk.com` URL 必须走 `dws minutes`，禁止用浏览器或 `read_file` 打开链接。自动提取 taskUuid 后调用 `get info/summary/transcription/todos`。
- 用户给了时间线索（今天、本周、上周、上月、最近 N 天、某日期范围）时，必须自行计算 `--start` / `--end`，格式用 ISO-8601，如 `2026-05-11T00:00:00+08:00`。不要反问用户时间范围。
- 未指定 mine/shared 时，检索型任务默认 `list all`；如果只查"我创建的"才用 `list mine`。
- 不要全量拉取后本地过滤时间。时间范围和关键词能服务端过滤时必须放进同一条 `list all --start --end --query`。
- 列表为空时按顺序兜底：同范围 `list all` → 去掉关键词但保留时间范围 → 明确告知无数据。禁止用模板或虚构听记内容继续生成纪要/周报。
- 生成纪要、文档、待办、周报前，必须先完成 `list` → 选定真实 `taskUuid` → `get summary`；需要原文或行动项时继续 `get transcription` / `get todos`。前置数据没拿到就停止并说明卡点。
- 用户明确说"下载/导出/文件"且对象是"听记原文/逐字稿/转写/完整记录"时，不能降级为摘要汇总：必须 `list all --start --end` 找到范围内听记；列表为空则明确无数据；列表非空则对每条听记逐一 `get transcription` 并翻页到结束，交付内容必须包含真实转写段落。可附标题、时间等元数据，但不能只给 summary，也不能要求用户再指定某一条。
- 所有 dws 命令带 `--format json`，不要用 shell 管道、重定向、`head`、`grep`、`jq`。

## 跨产品协作

- 提取的待办批量建任务 → 切到 `dingtalk-todo` 的 batch-create-todo recipe；批量脚本仅在 `dingtalk-todo` sub-skill 内可用，未切换前不要在当前 skill 运行。
- 摘要发给同事 → 切到 `dingtalk-chat`
- 日程 / 会议室 → 切到 `dingtalk-calendar`
## 局部意图与 Recipe

- [局部意图消歧](references/intent-guide.md)；[Lite Recipe](references/lite-recipes.md)。
