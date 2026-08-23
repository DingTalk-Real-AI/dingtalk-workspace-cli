---
name: dingtalk-minutes
description: 钉钉 AI 听记。Use when 查询听记摘要、转写、关键词、待办或分享。写文档走 dingtalk-doc；日程走 dingtalk-calendar。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 钉钉 AI 听记 Skill

## 前置条件 — 执行操作前必读

> **CRITICAL — 执行任何 `dws` 操作前，MUST 先用 Read 工具完整读取 [`dingtalk-shared`](../dingtalk-shared/SKILL.md)。**该轻量文件包含全局执行契约、安全底线及 shared references 的按需加载导航；不要预加载其全部 references。

> 命令参考：[minutes.md](references/minutes.md)；剧本：[07-minutes.md](references/07-minutes.md)。

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下 shortcut 同时进入公开 catalog 与 Runtime Schema。先按本 skill 的意图表、脚本和 recipe 路由：存在精确覆盖该场景的专用脚本/recipe 时按其执行；否则用户意图命中时，shortcut 优先于手写原子命令。命令已选中时直接执行；只在参数或安全语义不确定时读取 Agent leaf Schema（例如 `dws schema --cli-path "minutes +<shortcut>" --compact --format json`），在当前 Cobra flags 不确定时读取 `dws minutes <shortcut> --help`。只有参数映射、接口绑定或 provenance 审计才省略 `--compact`。仅当现有路由和 reference 都无法定位低频能力时，才用 `dws shortcut list --service minutes --format json` 批量发现。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws minutes +action-items` | read | 读取指定或我最新一条听记中已抽取的行动项 |
| `dws minutes +apply-permission` | write | 为当前用户申请语义化的听记访问权限 |
| `dws minutes +detail` | read | 批量聚合听记基础信息、摘要、关键词、完整逐字稿和行动项，支持安全文件输出 |
| `dws minutes +download` | read | 批量取得听记音视频地址并安全下载到本地 |
| `dws minutes +export-pack` | read | 把完整听记产物写入受控目录并生成不含签名 URL 的 manifest |
| `dws minutes +latest` | read | 取我最新的一条妙记（听记）详情 |
| `dws minutes +list-all` | read | 查询我有权限访问的所有听记列表 |
| `dws minutes +list-mine` | read | 查询我创建的听记列表 |
| `dws minutes +list-shared` | read | 查询他人共享给我的听记列表 |
| `dws minutes +mindmap` | write | 创建听记思维导图并轮询到明确成功、失败或超时 |
| `dws minutes +prepare-asr` | write | 读取个人热词、计算差异、按需新增并可显式同步删除后读回验证 |
| `dws minutes +record-pause` | write | 暂停听记录音 |
| `dws minutes +record-resume` | write | 恢复听记录音 |
| `dws minutes +record-start` | write | 发起听记（开始录音） |
| `dws minutes +record-stop` | write | 结束听记录音 |
| `dws minutes +record-wrap-up` | write | 停止实时录音并有界等待听记产物，失败时保留恢复句柄 |
| `dws minutes +replace-batch` | write | 预检并批量执行多组听记文字替换，逐项验证且失败必定非零 |
| `dws minutes +search` | read | 按范围、标题关键词和时间搜索听记，支持安全全量翻页 |
| `dws minutes +share` | write | 按成员逐项授予一个或多个听记权限，输出可审计的部分写入 ledger |
| `dws minutes +speaker-insights` | write | 创建发言人段落总结并轮询结果，保留异步任务恢复句柄 |
| `dws minutes +speaker-replace` | write | 预检逐字稿中的发言人昵称，替换后重新读回验证 |
| `dws minutes +summary` | write | 读取当前纪要、校验图片引用、全量覆盖并读回验证 |
| `dws minutes +transcript` | read | 读取指定或我最新一条听记的完整逐字稿，并交付分页完整性证据 |
| `dws minutes +unshare` | write | 按成员逐项移除一个或多个听记权限，输出可审计的部分写入 ledger |
| `dws minutes +update` | write | 读取现状、预览差异、更新听记标题并读回验证 |
| `dws minutes +upload` | write | 把本地音视频完整上传并创建听记，失败时取消会话 |
| `dws minutes +upload-and-analyze` | write | 本地音视频直传听记并等待分析产物，可选思维导图和发言人洞察 |
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
2. **执行（必须）**：`dws minutes list all|mine|shared --format json`；按关键词加 `--query "<关键词>"`，按时间加 `--start "<ISO>" --end "<ISO>"`，限制条数 `--limit <n>`（`--max` 兼容别名），翻页用 `--cursor <token>`（`--next-token` 兼容别名）。
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

1. **转文档（必须）**：取 `minutes get summary`/`transcription` 内容 → 切 `dingtalk-doc` 用 `dws doc create --content-file <tmp.md>` 落盘。
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

- 提取的待办批量建任务 → 切到 `dingtalk-todo`，按其批量创建 SOP 执行；批量脚本仅在 `dingtalk-todo` sub-skill 内可用，未切换前不要在当前 skill 运行。
- 摘要发给同事 → 切到 `dingtalk-chat`
- 日程 / 会议室 → 切到 `dingtalk-calendar`
## 局部意图与短流程

- [局部意图消歧](references/intent-guide.md)；[短流程](references/lite-recipes.md)。
