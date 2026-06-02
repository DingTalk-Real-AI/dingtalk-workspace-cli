# AI听记 (minutes) 命令参考

## 命令层级结构（防混淆，必须先读）

minutes 模块的命令是**两级或三级结构**，不同层级之间不能混用、跳级或遗漏：

> **默认 scope 规则**：`dws minutes list` 后**必须**跟 scope 子命令。若用户未明确指定查询范围，**一律默认补 `all`**（查询我可访问的所有听记 = 我创建的 + 他人共享给我的，覆盖面最广）。
>
> **`list` 不带 scope 的陷阱（0519 P2 Golden Case 提炼）**：
> - 裸 `dws minutes list`（不跟 mine/shared/all）虽然不会报错，但返回结果**不完整**（仅返回最近少量条目，约 913 字节）
> - `dws minutes list all` 才能返回完整列表（约 3362 字节）
> - **AI 严禁使用裸 `dws minutes list`**，必须始终带 scope 子命令
> - 如果 LLM 不确定用哪个 scope，**一律用 `all`**
>
> **scope 语义辨析（高频误判场景）**：
> - `mine` = **仅**我自己创建/发起的听记（范围最窄）
> - `shared` = **仅**他人共享给我的听记（不含我自己创建的）
> - `all` = 我可访问的**所有**听记（mine + shared 的并集，范围最广）
>
> **关键区分**：用户说"我可访问的"/"我能看到的"/"我有权限的"/"所有听记" → 选 `all`（不是 `mine`！）；用户说"我自己创建的"/"我发起的"/"我录的" → 才选 `mine`。

```
dws minutes
├── list <scope>                     # 查询听记列表（list 后必须跟 mine / shared / all）
│   ├── mine [--query] [--start] [--end] [--max] [--next-token]       # 仅查询我自己创建/发起的听记（不含他人共享给我的）
│   ├── shared [--query] [--start] [--end] [--max] [--next-token]     # 仅查询他人共享给我的听记（不含我自己创建的）
│   └── all [--query] [--start] [--end] [--max] [--next-token]        # 查询我可访问的所有听记（= mine ∪ shared，覆盖面最广，默认 scope）
├── get <subcommand>                 # 获取听记详情（get 后必须跟子命令名）
│   ├── info --id <uuid>             # 获取听记基础信息（标题、时长、参与人、状态等）
│   ├── summary --id <uuid>          # 获取听记 AI 摘要（结构化纪要）
│   ├── transcription --id <uuid>    # 获取听记语音转写原文（逐字稿，支持分页）
│   ├── keywords --id <uuid>         # 获取听记关键词
│   ├── todos --id <uuid>            # 获取听记中的待办事项（Action Items）
│   ├── audio --id <uuid>            # 获取听记音频/视频下载地址
│   └── batch --ids <uuid1,uuid2>    # 批量获取多篇听记的基础信息
├── update <subcommand>              # 修改听记内容（update 后必须跟子命令名）
│   ├── title --id <uuid> --title "..."       # 修改听记标题
│   └── summary --id <uuid> --content "..."   # 修改听记摘要内容
├── record <subcommand>              # 录音控制（record 后必须跟子命令名）
│   ├── start                        # 开始录音，发起新听记
│   ├── pause --id <uuid>            # 暂停录音
│   ├── resume --id <uuid>           # 恢复录音
│   └── stop --id <uuid>             # 停止录音，结束听记
├── speaker <subcommand>             # 发言人管理
│   ├── replace --id <uuid> --from "..." --to "..."   # 替换发言人名称（如"发言人1"→"张三"）
│   └── summary <subcommand>         # 按发言人生成/获取摘要
│       ├── create --ids <uuid1,uuid2>   # 创建按发言人维度的摘要
│       └── get --ids <uuid1,uuid2>      # 获取按发言人维度的摘要
├── mind-graph <subcommand>          # 脑图管理
│   ├── create --id <uuid>           # 根据听记内容生成脑图
│   └── status --id <uuid>           # 查询脑图生成状态
├── hot-word <subcommand>            # 个人热词管理
│   ├── add --words "..."            # 添加热词（提升语音识别准确率）
│   └── list                         # 查询我的热词列表
├── replace-text --id <uuid> --search "..." --replace "..."   # 替换转写文本中的错误文字
├── upload <subcommand>              # 音频上传（上传本地音频文件生成听记）
│   ├── create --file-name "..." --file-size <n>   # 创建上传会话，获取上传地址
│   ├── complete --session-id <sid>                # 确认上传完成
│   └── cancel --session-id <sid>                  # 取消上传会话
└── permission <subcommand>          # 权限管理（添加/移除听记成员权限）
    ├── add --ids <uuid1,uuid2> --member-uids <uid1,uid2> --policy <0-4> [--cover] [--sub-resources "..."]   # 添加成员权限
    └── remove --ids <uuid1,uuid2> --member-uids <uid1,uid2>   # 移除成员权限
```

**高频错误命令 vs 正确命令对照（真实 badcase 提炼）：**

| 错误写法 | 错在哪 | 正确写法 |
|------------|--------|------------|
| `dws minutes info --id <uuid>` | `info` 不是顶层子命令，应在 `get` 下 | `dws minutes get info --id <uuid>` |
| `dws minutes summary --id <uuid>` | `summary` 不是顶层子命令，应在 `get` 下 | `dws minutes get summary --id <uuid>` |
| `dws minutes transcription --id <uuid>` | `transcription` 不是顶层子命令，应在 `get` 下 | `dws minutes get transcription --id <uuid>` |
| `dws minutes get --uuid <uuid>` | `get` 后缺少子命令（info/summary/transcription 等） | `dws minutes get summary --id <uuid>` |
| `dws minutes get --task-uuid <uuid>` | 同上，`get` 后缺子命令。注意：`--task-uuid` 在子命令层是合法别名，但 `get` 层级不认识它 | `dws minutes get info --id <uuid>` |
| `dws minutes detail --id <uuid>` | `detail` 不是合法子命令——**不存在此命令**。LLM 高频幻觉 | `dws minutes get info --id <uuid>` |
| `dws minutes list --start "2026-04-01"` | `list` 后缺少 scope（mine/shared/all），**缺省时默认补 `all`** | `dws minutes list all --start "2026-04-01"` |
| `dws minutes list all --start-time "..."` | 参数名是 `--start` 不是 `--start-time` | `dws minutes list all --start "2026-04-01"` |
| `dws minutes list all --end-time "..."` | 参数名是 `--end` 不是 `--end-time` | `dws minutes list all --end "2026-04-30"` |
| `dws minutes list --page-size 10` | `--page-size` 不存在，分页用 `--max`；且 `list` 后缺 scope | `dws minutes list mine --max 10` |
| `dws minutes list --date-range 2026-05-04 2026-05-10` | `--date-range` 不存在，时间范围用 `--start` + `--end` 两个参数；且 `list` 后缺 scope | `dws minutes list mine --start "2026-05-04T00:00:00+08:00" --end "2026-05-10T23:59:59+08:00"` |
| `dws minutes list mine --limit 10` | `--limit` 尚未注册（跨产品规约 Primary 为 `--limit`，但 minutes CLI 当前只接受 `--max`）。**待 alias 注册后两者等价** | `dws minutes list mine --max 10` |
| `dws minutes upload create --json '{"fileName":...}'` | `--json` 不存在，cli 不接受 JSON 作为输入格式 | `dws minutes upload create --file-name "xxx.mp3" --file-size 61565431` |
| `dws minutes upload create -f json '{"fileName":...}'` | `-f json` / `--format json` 是**输出格式**控制，不是输入参数 | `dws minutes upload create --file-name "xxx.mp3" --file-size 61565431 --format json` |
| `dws minutes get transcription --id <uuid> \| head -c 2000` | Windows 沙箱无 `head` 命令，**严禁使用 shell 管道截断** | `dws minutes get transcription --id <uuid> --format json`（由 AI 在内存中截断处理） |
| `dws minutes get summary --id "https://shanji.dingtalk.com/app/transcribes/xxx"` | `--id` 只接受 taskUuid（hex 字符串），**不接受完整 URL**。AI 必须自动提取，不要让用户手动抠 | AI 自动从 URL 提取 taskUuid，再 `dws minutes get summary --id <taskUuid>`（详见「URL → taskUuid 自动提取规则」） |
| `dws minutes get transcription --id "https://shanji.dingtalk.com/meeting/minutes?taskUuid=xxx"` | 同上，`--id` 不接受 URL，且这是 `?taskUuid=` 格式的 URL | AI 自动提取 `taskUuid=` 后的值，再 `dws minutes get transcription --id <taskUuid> --format json` |
| `dws minutes get transcription --id <uuid>` 只调用一次就"总结整篇听记" | **单次调用最多返回 50 段**，不翻页就只能看到前 1/3~1/8 的内容 | **必须自动循环翻页**：检查返回中的 `nextToken`，非空则继续调用 `--next-token <token>`，直到拉完（详见「转写接口分页机制」） |
| `dws minutes get summary --id A & dws minutes get summary --id B & wait` | Windows cmd 下 `&` 是命令分隔符不是 background，`wait` 不存在 | 逐条串行调用，或让 AI 在内存中合并结果；**严禁使用 shell 并行/管道/重定向** |
| `dws minutes get transcription --id <uuid> --next-token <token>` 报 unknown flag | **不会报错**——`--next-token` 是 `get transcription` 的合法参数（见命令总览） | 确保写法正确：`dws minutes get transcription --id <uuid> --next-token <token> --format json` |
| `dws minutes get transcription --id <uuid> --page-token <token>` | `--page-token` 不存在，正确参数名是 `--next-token` | `dws minutes get transcription --id <uuid> --next-token <token>` |
| `dws minutes transcribe --url <听记url>` | `transcribe` 不是合法子命令，`--url` 参数也不存在。LLM 凭印象编造 | 先从 URL 提取 taskUuid（见「URL → taskUuid 自动提取规则」），再 `dws minutes get transcription --id <taskUuid> --format json` |
| `dws minutes get transcription --url <听记url>` | `--url` 参数不存在，`--id` 只接受纯 taskUuid | 同上：从 URL 提取 taskUuid 后用 `--id` |
| `dws minutes summary --uuid <uuid>` | `summary` 不是顶层子命令（应在 `get` 下），且顶层不识别 `--uuid`。**0519 高频错误** | `dws minutes get summary --id <uuid>` |
| `dws minutes list`（不跟 scope） | `list` 后**必须**跟 scope（mine/shared/all）。裸 `list` 返回结果不完整且行为未文档化 | `dws minutes list all`（默认 scope） |
| `dws report inbox --format json`（无时间窗口） | `inbox` 是兼容入口，等价于 `inbox list`；仍必须带 `--start` / `--end` | `dws report inbox list --start "2026-05-06T00:00:00+08:00" --end "2026-05-13T23:59:59+08:00" --format json` |
| `dws report inbox list --format json`（不带 --start） | inbox list **必须**带 `--start` / `--end` 参数，否则报 "flag --start is required" | `dws report inbox list --start "2026-05-06T00:00:00+08:00" --end "2026-05-13T23:59:59+08:00" --format json` |

> **参数别名说明**：`--id`、`--uuid`、`--task-uuid` 三者等价，均可使用。推荐统一用 `--id`，但 `--uuid` 和 `--task-uuid` 也是合法别名，不会报错。

**跨产品参数规约映射（dws-cli-param-spec 对齐）：**

> minutes 模块参数与跨产品规约 Primary 的映射关系：
>
> | 语义 | 规约 Primary | minutes 当前 CLI 实际参数 | alias 注册状态 | 说明 |
> |------|-------------|------------------------|--------------|------|
> | 单页大小 (Group 13) | `--limit` | `--max` | `--limit` 未注册 | minutes 用 `--max`，其他模块用 `--limit`。**跨模块切换时注意差异** |
> | 续页标识 (Group 14) | `--cursor` | `--next-token` | `--next-token` 在 alias 池中 | minutes 用 `--next-token`，合法 alias |
> | 搜索关键词 (Group 11) | `--query` | `--query` | 已对齐 | 无差异 |
> | 起始时间 (Group 15) | `--start` | `--start` | 已对齐 | 无差异 |
> | 结束时间 (Group 16) | `--end` | `--end` | 已对齐 | 无差异 |
>
> **跨模块参数名差异速查（防止混用）：**
> - 分页大小：minutes 用 `--max`，aitable/calendar/chat/drive 用 `--limit`
> - 续页标识：minutes 用 `--next-token`，aitable/drive/doc 用 `--cursor`
> - 起止时间：minutes/report 用 `--start`/`--end`，calendar 用 `--start-time`/`--end-time`（跨模块最高频错误）

**铁律：禁止编造 taskUuid（0512 P0 Golden Case 提炼）**

用户未直接提供 taskUuid / URL 时，**必须**先调用 `dws minutes list mine`（或 `list all`）获取列表，从返回结果中按标题/参会人/时间匹配选出正确条目，再用其 taskUuid 调用后续命令。**绝不允许凭空编造 hex 字符串作为 --id 的值。**

典型错误链路：
```
用户: "帮我看一下上次和 Tony 开会的听记摘要"
错误: LLM 直接编造: dws minutes get summary --id "7632756964..."  -- 这个 id 是捏造的
正确: 先 dws minutes list mine → 从结果按标题/参会人匹配 → 再 get summary --id <真实id>
```

**意图→命令决策表（0512 P0 Golden Case 提炼）：**

| 用户意图关键词 | 对应命令 | 说明 |
|--------------|---------|------|
| 总结、摘要、重点、概述、主要内容、按模板整理 | `get summary` | 返回结构化摘要 |
| 逐字稿、全文、原文、完整记录、转写、录音文字 | `get transcription` | 返回完整转写原文 |
| "帮我看/总结某次会议"（未给 id） | 先 `list all`（默认） | 按标题/时间匹配后再 get |
| "我可访问的听记"/"我能看到的"/"我有权限的"/"所有听记" | `list all` | **不是 `list mine`！** `all` = 我可访问的全部（mine + shared），`mine` 仅限我自己创建的 |
| "我的听记"（模糊表述） | `list all` | "我的"通常指用户可见的全部听记，默认走 `all`；仅当明确说"我创建/发起的"时才 `mine` |
| "我自己创建的听记"/"我发起的听记"/"我录的听记" | `list mine` | 明确限定创建者是自己时才走 `mine` |
| "上上周/两周前的会议" | `list all --start --end` | 需正确计算时间范围 |
| 添加参会者、批量添加参会者、添加成员、批量添加成员、加参会人 | `permission add` | 听记无独立"添加成员"接口，统一走权限接口 |

**多步流程错误传播规则（0512/0514 P0 Golden Case 提炼）：**

> 当执行 list → get summary/transcription → 套模板/写文档 等多步流程时：
> 1. 任何一步 dws 命令返回错误，**必须立即告知用户**卡在哪一步、具体错误信息
> 2. **严禁**跳过失败步骤假装成功
> 3. **严禁**用虚构数据填补失败步骤的空缺
> 4. 如果是权限问题 (NoPermission)，建议用户检查听记是否为自己创建或已被共享
> 5. 最终交付物中需注明哪些数据成功获取、哪些被权限/参数阻断
> 6. **严禁多步流程中途放弃**（0514 P0 新增）：前置步骤（如 `contact user get-self` 获取 userId）成功后，**必须**继续执行后续步骤（如 `calendar event list`、`minutes list mine`）。不能只完成前置步骤就停下来，回复"我打算帮你查…"却不实际执行

**诉求偏移防护规则（0514 P1 Golden Case 提炼）：**

> 用户明确要求"基于听记内容做 XX"（如生成会议纪要、汇报材料、周报）时：
> 1. **必须先实际调用** `dws minutes list` → `get summary/transcription` 获取真实数据
> 2. **严禁**跳过数据获取步骤直接"创建 skill"或"生成模板"——用户要的是**基于真实听记数据**的输出，不是空壳工具
> 3. 如果数据获取失败，应告知用户具体原因，而非转向其他不相关的操作
>
> **典型错误链路（0514 P1 bug 复现）：**
> ```
> 用户: "基于钉钉听记内容重新生成 Word 格式会议纪要"
> [错误] 模型创建了一个 skill 模板，但从未调用 dws minutes get summary/transcription 获取真实数据
> [正确] 先 dws minutes list mine → 从结果选匹配条目 → get summary --id <uuid> → 基于返回数据生成纪要
> ```

**跨模块组合注意事项（周报生成等场景，0512/0513/0514 P0 Golden Case 提炼）：**

> - 周报生成等场景需并行拉取 minutes / report / calendar / todo / doc 多类数据
> - **各模块参数名独立**，不可互相套用（如 doc 的参数不适用于 minutes）
> - **跨模块参数名交叉污染是 0514 最高频错误**（3/7 个 case 涉及）：切换模块时必须重新确认该模块的参数名，不要从上一个模块"顺手带过来"。典型错误对：`--start-time`(calendar) vs `--start`(minutes/report)、`--task-uuid`(无) vs `--id`(minutes)
> - **禁止**使用管道命令 `| jq`，Windows/macOS 客户端不保证 jq 可用，应由 LLM 直接解析 JSON
> - `report inbox list` **必须**带 `--start` / `--end` 参数（格式 `2026-05-04T00:00:00+08:00`），否则报错；`report outbox list` 不传时间窗口默认最近 20 天
> - `dws report inbox` 是兼容入口，仍可执行但已 deprecated；新计划必须写 `dws report inbox list --start ...`
> - `report inbox list` 不传 `--start` 会直接报错 "flag --start is required"，**不要让用户手动补**——AI 应自动推断最近 7 天范围填入
> - 旧命令 `report list / inbox / sent / created / detail / create / stats / template detail` 仍可执行但已 deprecated，新计划一律使用 `inbox list / outbox list / entry get / entry submit / entry stats / template get`
> - doc create 参数名是 `--content`（非 `--markdown`）

**钉钉听记 URL → taskUuid 自动提取规则（0513/0514 P0 Golden Case 提炼）：**

> **`--id` 参数只接受纯 taskUuid（hex 字符串），绝不接受完整 URL。** 但用户几乎总是直接粘贴完整 URL 过来，AI **必须自动完成 URL → taskUuid 的提取**，对用户完全透明——用户不需要知道"要抠出 hex 字符串"。
>
> **严禁**：
> - 直接把完整 URL 传入 `--id`（会报错）
> - 要求用户自己从 URL 中手动提取 taskUuid（用户体验极差，这是 AI 应自动完成的工作）
> - 把 URL 中的 query parameter（如 `?taskUuid=xxx`）和 path segment（如 `/transcribes/xxx`）搞混
>
> **已知的钉钉听记 URL 格式全表（AI 必须全部识别）：**
>
> | URL 格式 | taskUuid 提取方式 | 示例 |
> |---------|-----------------|------|
> | `https://shanji.dingtalk.com/app/transcribes/<taskUuid>` | 取 `/transcribes/` 后面的**整段路径** | `.../transcribes/7632756964323839...5f32` → `7632756964323839...5f32` |
> | `https://shanji.dingtalk.com/meeting/minutes?taskUuid=<taskUuid>` | 取 `taskUuid=` 后面的 query parameter 值 | `...?taskUuid=7632756964323839...5f32` → `7632756964323839...5f32` |
> | `https://shanji.dingtalk.com/app/transcribes/<taskUuid>?xxx=yyy` | 取 `/transcribes/` 和 `?` 之间的部分（忽略 query string） | `.../transcribes/763275...5f32?from=share` → `763275...5f32` |
> | 纯 hex 字符串（无 URL 前缀） | 直接使用，无需提取 | `7632756964323839...5f32` → 直接传 `--id` |
>
> **提取伪代码（AI 必须内化此逻辑）：**
> ```
> input = 用户提供的字符串（可能是 URL，也可能是纯 uuid）
>
> if input 包含 "/transcribes/":
>     taskUuid = input 中 "/transcribes/" 之后、"?" 之前（如有）的部分
> elif input 包含 "taskUuid=":
>     taskUuid = input 中 "taskUuid=" 之后、"&" 之前（如有）的部分
> elif input 匹配纯 hex 字符串模式 (只含 0-9a-f 且长度 > 20):
>     taskUuid = input  # 已经是纯 uuid
> else:
>     告知用户格式不识别，请提供听记链接或 taskUuid
>
> # 然后使用: dws minutes get summary --id <taskUuid> --format json
> ```
>
> **典型错误链路（0514 P0 bug 复现）：**
> ```
> 用户: "帮我看看这个听记 https://shanji.dingtalk.com/app/transcribes/7632756964323839..."
>
> [错误 1] dws minutes get summary --id "https://shanji.dingtalk.com/app/transcribes/763..." → 报错 invalid id
> [错误 2] "请你把 URL 中的 taskUuid 提取出来给我" → 不应该让用户手动做
> [错误 3] AI 把整个 URL decode 当作 taskUuid → 格式不对
> [正确] AI 自动提取 → dws minutes get summary --id "7632756964323839..." --format json
> ```
>
> **多链接批量场景**：用户一次性粘贴多个 URL 时，逐一提取 taskUuid，按顺序串行调用对应的 get 命令。**严禁使用 shell `&` 并行或 `|` 管道**。

**权限错误处理策略（0513 P0 Golden Case 提炼）：**

> `get info` / `get summary` / `get transcription` / `get batch` 访问他人发起的听记时，可能报以下权限错误：
>
> | 错误信息 | 含义 | AI 应如何处理 |
> |---------|------|-------------|
> | `[AUTH_PERMISSION_DENIED] Permission denied` | 当前用户对该听记无权限（不是创建者，且未被共享） | 告知用户："该听记由他人创建且未共享给你。请联系听记创建者在钉钉听记详情页将你加为共享者，然后再次尝试。" |
> | `[MCP_TOOL_ERROR] B_PERMISSION_NoPermission` | 同上，后端 dingOpenErrcode=14000011 | 同上处理 |
> | `[UNCLASSIFIED] user is not minutes creator` | batch 接口单条权限失败 | 告知用户哪几条 id 是自己的（可正常获取）、哪几条无权限，**不要整批放弃** |
>
> **关键原则**：
> - 权限报错 ≠ 命令写法有误——**不要**因为权限失败就去改参数名或换命令路径
> - batch 调用中如果部分成功部分权限失败，应**展示成功的结果** + **列出失败的 id 和原因**
> - `list shared` 返回的听记是**已被共享给我的**，这些 id 后续调用 get 不应报权限错误
> - `list all` 返回的所有 id 都是**当前有权限的**，可放心调用

**Shell 命令安全规则（0513 P0 Golden Case 强化）：**

> dws cli 运行在用户本地设备（Windows/macOS/Linux），**严禁使用任何 shell 特性**：
>
> | 禁止写法 | 原因 | 正确替代 |
> |---------|------|----------|
> | `dws ... \| head -20` | Windows cmd 无 `head` | 由 AI 在内存中截断 |
> | `dws ... \| grep "xxx"` | Windows cmd 无 `grep` | 由 AI 在返回 JSON 中筛选 |
> | `dws ... 2>&1 \| head` | 混合重定向+管道 | 直接读 dws 输出 |
> | `dws A & dws B & wait` | cmd 下 `&` 是分隔符不是 background | 逐条串行执行 |
> | `dws ... > output.txt` | 不确定用户 shell 是否支持 | 由 AI 直接处理返回结果 |
>
> **铁律：所有 dws 命令必须单条、独立、无管道地执行，AI 自行处理输出内容。**

**Upload 模块错误诊断（0513 P0 Golden Case 提炼）：**

> `dws minutes upload create` 失败时的常见原因与处理：
>
> | 错误表现 | 真实原因 | 处理方式 |
> |---------|---------|---------|
> | `business error: success=false`（无详细信息） | 默认输出不含详细错误，需检查是否有并发会话 | 告知用户："上传失败，可能原因包括：① 已有一个进行中的上传会话未完成/取消；② 文件名含特殊字符；③ 文件格式不支持。" |
> | `concurrent_limit: user already has an active upload session` | 前一次上传会话未结束（未 complete 也未 cancel） | 需要先 `dws minutes upload cancel --session-id <sid>` 取消上一个会话。若 session-id 未知，建议用户在钉钉客户端手动取消或等待超时 |
> | `missing required flag(s): --session-id` | cancel 命令必须传 session-id | 从之前 upload create 的返回中提取 session-id；若丢失则暂无法取消 |
>
> **上传流程三步走**（AI 必须按此顺序执行，不可跳步）：
> 1. `dws minutes upload create --file-name "xxx.mp3" --file-size <字节数> --format json` → 获取 `sessionId` + `uploadUrl`
> 2. 将文件通过 `uploadUrl` 直传（由客户端侧完成，AI 不介入）
> 3. `dws minutes upload complete --session-id <sid> --format json` → 确认上传完成
>
> **注意**：`--file-size` 单位是**字节**（不是 KB/MB），AI 不要帮用户估算大小，应让用户确认文件实际字节数。

## 指令树

```
dws minutes
├── list <scope>                     # 二级：list 后必须跟 mine / shared / all
│   ├── mine [--query] [--start] [--end] [--max] [--next-token]
│   ├── shared [--query] [--start] [--end] [--max] [--next-token]
│   └── all [--query] [--start] [--end] [--max] [--next-token]
├── get <subcommand>                 # 二级：get 后必须跟子命令名
│   ├── info --id <uuid>
│   ├── summary --id <uuid>
│   ├── transcription --id <uuid>
│   ├── keywords --id <uuid>
│   ├── todos --id <uuid>
│   ├── audio --id <uuid>
│   └── batch --ids <uuid1,uuid2>
├── update <subcommand>              # 二级：update 后必须跟子命令名
│   ├── title --id <uuid> --title "..."
│   └── summary --id <uuid> --content "..."
├── record <subcommand>              # 二级：record 后必须跟子命令名
│   ├── start
│   ├── pause --id <uuid>
│   ├── resume --id <uuid>
│   └── stop --id <uuid>
├── speaker <subcommand>
│   ├── replace --id <uuid> --from "..." --to "..."
│   └── summary <subcommand>
│       ├── create --ids <uuid1,uuid2>
│       └── get --ids <uuid1,uuid2>
├── mind-graph <subcommand>
│   ├── create --id <uuid>
│   └── status --id <uuid>
├── hot-word <subcommand>
│   ├── add --words "..."
│   └── list
├── replace-text --id <uuid> --search "..." --replace "..."
├── upload <subcommand>
│   ├── create --file-name "..." --file-size <n>
│   ├── complete --session-id <sid>
│   └── cancel --session-id <sid>
└── permission <subcommand>
    ├── add --ids <uuid1,uuid2> --member-uids <uid1,uid2> --policy <0-4> [--cover] [--sub-resources "..."]
    └── remove --ids <uuid1,uuid2> --member-uids <uid1,uid2>
```

## 命令总览

### 查询我创建的听记列表
```
Usage:
  dws minutes list mine [flags]
Example:
  dws minutes list mine
  dws minutes list mine --max 10
  dws minutes list mine --max 10 --next-token <nextToken>
  dws minutes list mine --query "周会"
Flags:
      --max float         查询的听记篇数 (默认 10)
      --next-token string 分页 token (首页留空，后续填写前次返回的 nextToken)
      --query string      关键字筛选 (可选)
      --start string      开始时间 ISO-8601 (可选)
      --end string        结束时间 ISO-8601 (可选)
```

查询我创建的听记列表，支持 `--max` 和 `--next-token` 分页，支持按关键字和时间范围筛选。

### 查询他人共享给我的听记列表
```
Usage:
  dws minutes list shared [flags]
Example:
  dws minutes list shared
  dws minutes list shared --max 20
  dws minutes list shared --max 5 --next-token <nextToken>
Flags:
      --max float         查询的听记篇数 (默认 10)
      --next-token string 分页 token (首页留空，后续填写前次返回的 nextToken)
      --query string      关键字筛选 (可选)
      --start string      开始时间 ISO-8601 (可选)
      --end string        结束时间 ISO-8601 (可选)
```

查询他人共享给我的听记列表，支持 `--max` 和 `--next-token` 分页，支持按关键字和时间范围筛选。

### 查询我有权限访问的所有听记列表
```
Usage:
  dws minutes list all [flags]
Example:
  dws minutes list all
  dws minutes list all --max 20
  dws minutes list all --query "周会" --max 20
  dws minutes list all --start "2026-03-01T00:00:00+08:00" --end "2026-03-20T23:59:59+08:00"
  dws minutes list all --max 10 --next-token <nextToken>
Flags:
      --end string        结束时间 ISO-8601 (可选)
      --query string      关键字筛选 (可选)
      --max float         查询的听记篇数 (默认 10)
      --next-token string 分页 token (首页留空，后续填写前次返回的 nextToken)
      --start string      开始时间 ISO-8601 (可选)
```

查询我有权限访问的所有听记列表（包括我创建的、他人共享给我的等所有有权限的听记）。支持按关键字和时间范围筛选。时间范围和关键字为可选参数，不传则返回所有有权限的听记。支持使用 `--max` 和 `--next-token` 进行分页查询。

### 获取听记基础信息
```
Usage:
  dws minutes get info [flags]
Example:
  dws minutes get info --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)，取值逻辑参考 ## 注意事项
```

返回字段: 创建人、开始时间、截止时间、听记标题、听记访问链接URL

**发言人列表输出规范（查询详情后必须执行）**：

调用 `get info` 和 `get transcription`（首页即可）后，**必须**从返回数据中提取并展示发言人列表：

1. **提取发言人**：从转写数据的 `speakerName` / `speakerId` 字段收集所有不同的发言人
2. **分类展示**：
   - **已识别**：声纹已标注真实姓名的发言人（直接显示姓名）
   - **未识别**：仅有匿名编号的发言人（显示为「发言人 1」「发言人 2」等）
3. **引导逻辑（存在未识别发言人时）**：

   展示发言人列表后，**必须**向用户提供以下选项：

   > 本次听记共有 M 位发言人，其中 N 位尚未识别身份：
   >
   > | # | 当前标注 | 发言占比 |
   > |---|---------|---------|
   > | 1 | 张三（已识别） | 35% |
   > | 2 | 发言人 1（未识别） | 30% |
   > | 3 | 发言人 2（未识别） | 20% |
   > | 4 | 李四（已识别） | 15% |
   >
   > 你可以：
   > 1. **智能匹配** — 我来帮你推断未识别的发言人身份（基于通讯录、聊天记录等）
   > 2. **提供日程/参会人** — 告诉我这次会议对应的日程链接或参会人名单，我用成员列表来匹配
   > 3. **手动设置** — 直接告诉我对应关系，如「发言人1:王五, 发言人2:赵六」
   > 4. **跳过** — 暂不处理发言人识别

   根据用户选择分别处理：
   - 选 1 → 进入 `minutes-speaker-summarize` recipe 的智能推断流程（见 [10-minutes-speaker-match.md](../best_practices/10-minutes-speaker-match.md)）
   - 选 2 → 请用户提供日程链接或参会人姓名列表：
     - 有日程链接 → 从链接提取 eventId → `dws calendar participant list --event <eventId>` 获取参与人 → 与转写发言人数量/顺序对照匹配 → 展示匹配结果请用户确认 → 确认后逐条执行 `speaker replace`
     - 有参会人名单（用户直接列出姓名）→ 与转写中匿名发言人做数量对照 → 结合发言内容/角色推断匹配 → 展示匹配结果请用户确认 → 确认后逐条执行 `speaker replace`
   - 选 3 → 用户提供映射关系（如「发言人1:张三」格式）→ 按「发言人映射模式识别」流程执行（见下方"修改说话人"章节）
   - 选 4 → 正常结束，不做发言人处理

4. **引导逻辑（所有发言人均已识别时）**：

   > 所有发言人均已识别。是否需要查看某位发言人的详细发言总结？请输入姓名。

   用户选定 → 进入 `minutes-speaker-summarize` recipe；用户不需要 → 正常结束。

> **禁止**：跳过发言人列表直接输出摘要；将匿名编号的内部格式（Speaker_X）暴露给用户；未识别发言人时不给任何引导就结束。

### 获取听记 AI 摘要
```
Usage:
  dws minutes get summary [flags]
Example:
  dws minutes get summary --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)，取值逻辑参考 ## 注意事项
```

返回 Markdown 格式摘要，涵盖会议主题、核心结论、关键讨论点等

### 获取听记关键字列表
```
Usage:
  dws minutes get keywords [flags]
Example:
  dws minutes get keywords --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)，取值逻辑参考 ## 注意事项
```

### 获取听记语音转写原文
```
Usage:
  dws minutes get transcription [flags]
Example:
  dws minutes get transcription --id <taskUuid>
  dws minutes get transcription --id <taskUuid> --direction 1
  dws minutes get transcription --id <taskUuid> --next-token <nextToken> --format json
Flags:
      --direction string   排序方向: 0=正序, 1=倒序 (默认 0)
      --id string          听记 taskUuid (必填)，取值逻辑参考 ## 注意事项
      --next-token string 下一页的token 首次查询可空 后续查询需填写前次请求返回的nextToken
```

每条记录包含: 发言人信息、转写文本、对应时间戳

**关键：转写接口每次最多返回 50 段，必须自动翻页才能拿到完整原文！**

> **这是最高频的 silent failure：** `get transcription` 单次调用**最多只返回 50 段**。一篇 30 分钟会议的转写通常有 150-400 段，如果不翻页就只能看到前 1/3 甚至 1/8 的内容。
>
> **分页机制（必须掌握）：**
> - 首次调用：`dws minutes get transcription --id <uuid> --format json`（不传 `--next-token`）
> - 检查返回 JSON 中是否包含 `nextToken` 字段（非空字符串）
> - 如果有 `nextToken`：**必须立即发起下一次调用**，传入 `--next-token <上次返回的nextToken值>`
> - 循环直到返回中**不再包含 `nextToken`**（或 `nextToken` 为空），表示所有段落已拉取完毕
> - **拼合所有页的段落**后，才算拿到了完整转写原文
>
> **自动翻页伪代码（AI 必须内化此逻辑）：**
> ```
> all_paragraphs = []
> next_token = ""           # 首次为空
> loop:
>     if next_token == "":
>         result = dws minutes get transcription --id <uuid> --format json
>     else:
>         result = dws minutes get transcription --id <uuid> --next-token <next_token> --format json
>     all_paragraphs.append(result.paragraphs)
>     next_token = result.nextToken   # 可能为空或不存在
>     if next_token 为空或不存在:
>         break   # 全部拉取完毕
> # 此时 all_paragraphs 才是完整转写原文
> ```
>
> **典型错误（导致"总结整篇听记"只看到前 50 段的 P0 bug）：**
> - [错误] 只调用一次 `get transcription`，看到返回了内容就以为拿全了 → 实际只有前 50 段
> - [错误] 看到返回 JSON 里有 `nextToken` 字段但不知道这是什么、不知道要传 `--next-token` → 默默丢弃了后续页
> - [错误] 用 `--help` 查参数但在 Windows 下输出被截断，没看到 `--next-token` 参数 → 以为不支持分页
> - [正确] **正确做法：无条件按上述循环逻辑自动翻页，直到 nextToken 为空**

**重要 — 转写原文拉取策略（AI 必须严格遵守）：**

转写原文数据量通常很大，**是否拉取、拉取多少**需要根据用户意图智能判断：

1. **用户明确要求查看/分析转写原文时 → 默认拉取全部原文**（自动翻页，不需要用户手动说"第一页"）
   - 示例："帮我看看转写原文"、"分析一下这篇听记的原文"、"把逐字稿给我"、"转写内容是什么"
   - 实现：首次调用不传 `--next-token`，如果返回中包含 `nextToken`，**自动继续调用**直到拉取完所有页，最终拼合后展示给用户
   - **字符上限保护**：在循环拉取过程中，如果已累积的转写文本总量**超过 12000 字符（1.2w）**，必须**暂停自动翻页**，向用户提示当前已处理的字符数已达到上限，并询问是否继续拉取后续分页内容。用户确认后才继续拉取，用户拒绝则停止并展示已拉取的内容

**超长会议翻页优化策略（0519 P0 Golden Case 提炼 — 70min 会议翻页 15+ 次）：**

> 当会议时长超过 45 分钟时，转写原文通常需要 **10-20 次翻页**（每次 50 段 ≈ 23k 字），每次 CLI 调用都消耗 LLM 上下文 token，极度浪费。
>
> **AI 必须遵守的翻页效率规则：**
> 1. **翻页次数预估**：首次调用返回后，如果 `nextToken` 非空，AI 应根据已拉取段落数和会议总时长估算剩余翻页次数
> 2. **超过 5 次翻页时**：自动启用"静默累积模式"——不在每次翻页时都向用户汇报进度，直接循环拉取并在内存中累积
> 3. **超过 10 次翻页时**：在第 10 次翻页完成后暂停，主动告知用户：
>    > "这篇听记较长，已拉取前 500 段（约 X 字符），预计还需 N 次翻页。是否继续拉取完整原文，还是基于已有内容先做分析？"
> 4. **严禁**：翻页中途放弃后改用 shell 重定向（如 `> tmp/file.json`）——这违反 Shell 命令安全规则
> 5. **严禁**：因翻页次数多就跳过后续页，在摘要中只基于前几页内容得出结论
>
> **用户意图为"按发言人分析"时的优化路径：**
> - 如果用户的最终目的是"区分发言人"/"看某人讲了什么"，**不一定需要拉取全部转写原文**
> - 可优先使用 `dws minutes get summary --id <uuid>` 获取结构化摘要（含发言人信息）
> - 或使用 `dws minutes speaker summary get --ids <uuid>` 直接获取按发言人维度的摘要
> - 仅当用户明确要求"逐字稿"/"完整原文"/"每句话"时，才需完整翻页拉取

2. **用户未明确要求查看原文时 → 不要主动拉取转写原文**
   - 示例："查一下和悟空相关的听记"、"帮我看看这个听记的摘要"、"这个会议讲了什么"
   - 这些场景下用户的意图是查列表、看摘要等，**不需要也不应该**把大量转写原文全部拉出来，否则会造成信息过载和不必要的性能开销
   - 如果用户问"这个会议讲了什么"，应优先使用 `get summary` 返回摘要，而非 `get transcription`

**判断原则：只有用户的意图明确指向"原文/转写/逐字稿/录音文字"时，才调用 `get transcription`；其他场景（查列表、看摘要、看待办等）严禁自动附带拉取转写原文。**

**重要 — 转写原文返回后默认按"时间线"组织各段落，AI 必须主动引导发言人聚类与关联（必须严格遵守）：**

`get transcription` 默认返回的段落是按"时间戳正序/倒序"穿插展示的（每条记录包含 `speakerNick + 时间戳 + 文本`），**对人不友好**——同一发言人的内容散落在多个时间点，难以快速看清"某个人主要讲了什么"。因此，AI **在拉取完成（含分页全部完成）后**必须主动启动以下"发言人聚类 → 关键词模糊匹配 → 引导确认 → 调用 `speaker replace`"四阶段工作流：

#### 阶段 1：拉取完成后主动询问"是否按发言人聚类"
拉取（含自动翻页）结束后，AI 必须**主动**追问用户一次（不要默认强制聚类，避免用户只想看时间线原文时被打扰）：

> "已拉取完整转写原文（共 N 段，X 个发言人）。当前默认按时间线返回。是否需要我帮你**按发言人分组聚类**，并提取每位发言人的**核心发言要点**？"

- 用户确认（"好/可以/需要/聚类一下/按发言人分一下"等）→ 进入阶段 2
- 用户拒绝 → 直接展示时间线原文，结束流程，**不再追问**

#### 阶段 2：按发言人聚类 + 提取核心内容
AI 基于已拉取的转写数据，**本地完成**聚类与摘要（无需新调用 dws 命令），输出结构如下：

```
[发言人] 发言人1（共 12 段，约 1820 字）
   核心要点：
   - 介绍 Q3 战略规划与组织调整方向
   - 强调 AI 化转型的三个关键里程碑
   - 提出对供应链效率的具体改进目标

[发言人] 发言人2（共 8 段，约 960 字）
   核心要点：
   - 汇报当前业务的财务数据与利润率
   - 分析竞品在华东市场的最新动作

[发言人] 张三（共 5 段，约 410 字）
   核心要点：
   - 同步研发团队的招聘进度
   - 提出对测试资源的支持诉求
```

**约束：**
- 聚类必须包含**全部发言人**（包括"发言人1/发言人2"占位符与已关联真实姓名的发言人，便于后续替换）
- 每位发言人**最多列出 3-5 条**核心要点，避免冗长
- 输出后必须**主动追问**用户：

> "如果你能告诉我『某某人主要讲了什么』（例如『李总主要讲了战略规划』『王经理主要负责供应链』），我可以根据关键词帮你**自动匹配**对应的发言人，并把『发言人1/发言人2』替换成真实姓名。"

#### 阶段 3：基于用户提供的关键词做模糊匹配
当用户提供形如『某某人主要讲了 XX』的输入时（例如『李总主要讲了战略规划』『拾光负责供应链效率改进』），AI 必须：

1. **提取关键词**：从用户输入中抽取核心实体/主题词（如『战略规划』『供应链效率』『AI 化转型』），可允许多关键词
2. **在阶段 2 已聚类的发言人核心要点中做模糊匹配**：
   - 优先匹配「核心要点」中包含或语义相近的发言人
   - 必要时回看该发言人的原始转写文本进行二次确认
   - 支持**同义词/近义词**容忍（如『战略』~『规划』、『供应链』~『物流』）
3. **匹配结果分级处理：**
   - **唯一高置信匹配**（一个发言人显著命中）→ 进入阶段 4 引导确认
   - **多个候选**（2 个及以上发言人都有部分命中）→ 列出全部候选，让用户选择，例如：
     > "根据关键词『战略规划』，我匹配到 2 个可能的候选：① 发言人1（命中『战略规划/AI 化转型』）② 发言人3（命中『战略方向』）。你说的『李总』更可能是哪一位？"
   - **无匹配** → 如实告知用户，并请其补充更具体的关键词或直接给出"发言人编号 → 真实姓名"的映射，例如：
     > "暂未在转写中匹配到与『战略规划』强相关的发言人。可以再描述得更具体一些，或者直接告诉我『发言人 X 就是李总』，我来帮你替换。"

#### 阶段 4：引导用户确认关联，并调用 `speaker replace` 完成替换
匹配到唯一候选后，AI **不要直接替换**，必须先**显式追问用户确认**：

> "我找到『发言人1』很可能就是你说的『李总』（命中关键词：战略规划、AI 化转型）。是否需要我把这篇听记里的『发言人1』全部替换为『李总』？
>  确认后我会执行：`dws minutes speaker replace --id <taskUuid> --from "发言人1" --to "李总"`"

- 用户确认 → 立即调用 `dws minutes speaker replace --id <taskUuid> --from "发言人1" --to "李总" --format json`，并在执行成功后告知用户："已将本篇听记的『发言人1』全部替换为『李总』，纪要与待办中的发言人也已同步更新。"
- 用户希望同时关联通讯录 → 引导用户提供钉钉 UID，调用时附加 `--target-uid <uid>`
- 用户拒绝 → 不替换，可询问是否还有其他发言人需要关联，否则结束流程

**严禁的行为：**
- [禁止] 拉取完转写后直接输出大段时间线原文就结束，不主动引导聚类
- [禁止] 用户提供"某某人讲了 XX"后，AI 自己默认替换而不向用户二次确认
- [禁止] 把"发言人1 → 李总"这种映射只在 AI 回复中口头说说，而**不实际调用** `speaker replace` 写回听记
- [禁止] 模糊匹配置信度不足时仍然给出唯一答案，不让用户参与挑选候选

#### 发言人识别与总结执行链路（用户指定人名查发言时必须遵循）

**触发条件**：用户同时提供了听记来源（URL/时间/关键词）和一个具体人名，目标是获取该人在会议中说了什么。例如"帮我看看这个听记里张三说了什么""李总在今天的会上提了哪些观点"。

> **【重要】** **核心铁律（必须刻在脑子里，违反任何一条都视为严重错误）：**
>
> **铁律 1：严禁在转写文本里 grep "目标人名" 字符串作为存在性判断依据**
> - 花名/真名通常**不会**出现在 TA 自己的发言里，发言里出现的"X"只意味着"`说话人 ≠ X`"或"说话人在叫 X"
> - 在转写中搜不到"灵麦"**完全不能得出"灵麦没参会"的结论**——99% 的发言人都显示为匿名编号"发言人1/2/3"
> - 正确做法是把"目标人名"作为**身份信息**去通讯录查询（Step 4 ①），而不是当作发言内容字符串去 grep
>
> **铁律 2：严禁用 AI 摘要里的"参与人/参会人"字段判断某人是否参会**
> - AI 摘要中的"参与人"字段往往**只截取最显著的 1-2 个名字**（通常是创建人或发言最多的人），**不是完整参会人列表**
> - "AI 摘要参与人只有故愚 → 灵麦没参会" 是典型的错误推理链
> - 如果一定要列出参会人，应使用 `dws minutes get info` / `get batch` 返回的 `participants` 字段，而不是摘要文本里的描述
>
> **铁律 3：通讯录查询是 Step 4 的强制起点，不依赖 Step 3 是否成功**
> - Step 2 一旦返回"未命中真名标注"，**Step 3 与 Step 4 ① 必须并发触发**，不要等 Step 3 失败再补查
> - 通讯录查询 1 个调用就能锁定"目标人物的部门 + 职级 + 上级"——这是身份推断**最低成本、最高收益**的信号源
> - 跳过通讯录直接得出"找不到 X"的结论 = 100% 失败链路
>
> **铁律 4：找不到字面匹配 ≠ 不存在**
> - "发言人识别"功能存在的本质就是**根据角色特征把匿名编号映射到真实人**——"找不到字面匹配"恰恰是这个功能要解决的问题，而不是退出的理由
> - 一旦想说"找不到 X"前，先自问：通讯录查了吗？文档查了吗？聊天记录查了吗？基于角色在转写里做模式匹配了吗？四个全是"没"则禁止给"找不到"的结论

**完整执行链路：**

```
Step 1: 定位听记并读取转写原文
    ↓
Step 2: 声纹标注检查
    ├─ [目标人名已被系统标注] → 直接跳 Step 6
    └─ [仅有匿名编号"发言人1/2/3"] ↓
Step 3: 转写原文内推断（优先，能判断就不走外部查询）
    ├─ [高置信命中] → 直接跳 Step 6
    └─ [无法确定] ↓
Step 4: 多路并发身份推断
    ↓
Step 5: 定向匹配 + 置信度分支
    ├─ 置信度 ≥ 50%  → 展示文本片段请用户确认
    └─ 置信度 < 50%  → 提供候选片段请用户辨认
    ↓
Step 6: 结构化总结输出
    ↓
Step 7: 引导用户替换发言人（调用 speaker replace 写回听记）
```

##### Step 1: 定位听记并读取转写原文

- **有 URL** → 从 URL 提取 taskUuid → `dws minutes get transcription --id <uuid> --format json`（自动翻页拉取全部）
- **有时间/关键词** → `dws minutes list all --query "关键词" --start "..." --end "..." --format json` 筛选 → 获取 taskUuid → 拉取转写
- **什么都没给** → **必须询问用户**提供听记链接、时间或关键词

##### Step 2: 声纹标注检查

检查转写原文中目标人物是否已被系统识别并标注了真实姓名（即 `speakerNick` 直接就是用户提到的人名）：
- **已标注**（某条 `speakerNick` 字面就是"木兰"）→ 直接跳到 Step 6，零确认步骤
- **仅有匿名编号**（发言人1/发言人2/发言人3）→ **必须**继续 Step 3 与 Step 4，**严禁在此处退出**

> **【重要】** **关键认知（违反则案例 7 重现）：**
> - **未命中真名标注 ≠ 目标人物没参会**——绝大多数听记的发言人都是匿名编号，"未命中"是**默认场景**，恰恰是发言人识别功能要解决的问题
> - **不要在转写文本里 grep 目标人名**作为存在性判断——花名/真名通常不会出现在 TA 自己的发言里（参见铁律 1）
> - **不要把 `dws minutes get summary` 摘要文本里写到的"参与人"当作完整参会列表**——AI 摘要里的"参与人"字段只截取最显著的 1-2 个名字，不是完整名册（参见铁律 2）
> - 唯一能下"目标人物没参会"结论的场景是：`get info` / `get batch` 返回的结构化 `participants` 字段里**完整列出参会人且不含目标**，且 `dws contact user search` 也搜不到该人 → 才允许告知用户"该人不在参会列表"

##### Step 3: 转写原文内推断（优先）

**核心原则：先基于转写原文做逻辑推断，能直接判断就不调外部接口。**

充分利用转写文本中的所有可用信息进行综合推断：
- **称呼线索**：其他人称呼"张总/李工/王老师"等
- **自我介绍**："我是 XX 部门的""我负责 XX"
- **上下文指代**：前文提到"张三你来说一下"，紧接着的发言人大概率是张三
- **发言内容特征**：用户说"李总负责战略"，而某发言人大量讨论战略方向
- **发言顺序**：主持人/领导通常先发言或总结性发言

只要能从原文**高置信度**地确定发言人，直接跳 Step 6 输出总结。如果仅凭原文无法确定，继续 Step 4。

##### Step 4: 多路并发身份推断

> **【重要】** **强制规则（违反则案例 7 重现）：**
> - **路径 ① 通讯录查询是必跑项，不依赖 Step 3 是否成功**——Step 2 一旦判定"未命中真名标注"，**Step 3 与 Step 4 ① 必须并发触发**，**严禁等 Step 3 失败再补查**
> - 通讯录查询单次调用即可拿到"部门 + 职级 + 上级 + 真名"——这是身份推断**最低成本、最高收益**的信号源
> - 路径 ②③④ 是**增量信号**，按需触发（如通讯录返回的部门是"产品设计部"这类多角色部门时，再补查 ② 文档）

**触发顺序：**

| 阶段 | 必跑 / 可选 | 触发时机 |
|------|-------------|----------|
| 路径 ① 通讯录组织架构 | **必跑** | Step 2 判定"未命中真名"后立即并发触发，与 Step 3 同时进行 |
| 路径 ② 本人创建的文档 | 可选 | ① 返回的部门是"产品设计部"这类多角色部门，需要更精确的角色信号时 |
| 路径 ③ 近期日程类型 | 可选 | ① ② 都不充分，需要补充职能边界判断时 |
| 路径 ④ 聊天记录 | 可选 | ①②③ 都不充分，需要语言风格/工作内容线索作为最后一道印证时 |

| 路径 | 命令 | 得到什么 |
|------|------|----------|
| ① 通讯录组织架构 | `dws contact user search --keyword "目标人名"` → 部门/职级/上级/真名 | 职能大类（技术/产品/设计/管理）+ 是否存在该人 |
| ② 本人创建的文档 | `dws doc search --keyword "目标人名/真名"` 至少获取 3 篇标题 | 角色精确信号（PM写PRD、研发写技术方案、设计师写视觉规范）|
| ③ 近期日程类型 | `dws calendar event list` | 职能边界（参加什么类型的会）|
| ④ 聊天记录 | `dws chat message list` 获取与目标人的近期 IM 消息 | 语言风格/工作内容/职责线索 |

**判定规则**：
- ① 命中（通讯录里搜到该人）→ 至少能拿到"部门 + 职级"信号，置信度起步 ≥ 30%
- ① + ② / ③ / ④ 任一路印证 → 置信度 ≥ 50%
- 两路以上独立信号一致 → 置信度 ≥ 70%
- ① 完全搜不到该人（且通讯录工具本身可用、未报错）→ 才允许告知用户"该人不在通讯录中，请确认花名是否正确"

> **关键约束 1**：部门名 ≠ 角色（"产品设计部"里有 PM、设计师、研究员），必须结合文档产出等信号区分。
>
> **关键约束 2**：① 通讯录查询调用一次就能锁定身份范围，**严禁省略**。常见的失败模式是：在转写文本里反复 grep 目标人名找不到 → 直接放弃 → 告诉用户"找不到"——这是 100% 错误链路（参见案例 7）。

##### Step 5: 定向匹配 + 置信度分支

基于 Step 4 推断的角色，在转写原文中寻找匹配的发言模式：

| 角色 | 典型发言特征 |
|------|----------|
| 产品经理 | 提需求、讲用户场景、定优先级 |
| 研发 | 技术约束、方案评估、排查问题 |
| 管理者 | 发言占比高、最终决策、分配任务 |
| 设计师 | 视觉方案、交互细节、体验讨论 |

**分支 A：置信度 ≥ 50%（文本确认）**

选取最具代表性的连续片段（≥ 2 句完整句子，避免"嗯/对/好"等纯语气词），展示给用户：

> "根据分析，以下发言最可能是 [人名] 的：
> 「[片段内容]」
> 确认是 TA 吗？"

- 用户确认 → Step 6
- 用户否认 → 换下一候选（最多 3 个）
- 3 个全否 → 告知无法仅通过文本确认，建议在听记详情页播放录音辅助辨认

**分支 B：置信度 < 50%（多候选展示）**

当身份推断把握不足时，列出所有候选发言人及其代表性片段，让用户挑选：

> "无法确定哪位是 [人名]。以下是几位候选发言人的代表性内容：
> ① 发言人1：「[片段]」
> ② 发言人3：「[片段]」
> 哪位更像 [人名]？或者你可以在听记详情页播放录音辅助确认。"

- 用户选定 → Step 6
- 用户无法确认 → 告知可在听记详情页点击对应段落播放原始录音来辨认，结束流程

##### Step 6: 结构化总结输出

提取已确认的该发言人的全部发言，结合会议上下文进行综合总结。根据实际内容灵活组织输出结构，例如：

```
[人名] 在本次会议中的发言总结（共 N 段，约 X 字）

核心观点：
- 观点1...
- 观点2...
- 观点3...

关键决策/结论：
- ...

提出的待办/行动项：
- ...
```

##### Step 7: 引导用户替换发言人（必须执行，不可跳过）

总结输出完成后，如果该发言人在转写中仍显示为匿名编号（如"发言人1"），**必须主动引导用户替换**：

> "目前这篇听记中 [人名] 的发言仍显示为『发言人X』。要我帮你把听记里的『发言人X』全部替换为『[人名]』吗？替换后纪要和待办中的发言人也会同步更新。
> 确认后我会执行：`dws minutes speaker replace --id <taskUuid> --from "发言人X" --to "[人名]"`"

- 用户确认 → **先主动调用通讯录模糊查询** `dws contact user search --query "[人名]" --format json` 获取该人员的 userId（长整型 dingUid）：
  - 唯一匹配 → 告知用户匹配结果，确认后执行 `dws minutes speaker replace --id <taskUuid> --from "发言人X" --to "[人名]" --target-uid <userId> --format json`
  - 多个匹配 → 列出候选（姓名+部门+userId）让用户选择后执行
  - 无匹配 → 提示用户未在通讯录中找到此人，执行不带 `--target-uid` 的替换：`dws minutes speaker replace --id <taskUuid> --from "发言人X" --to "[人名]" --format json`
- 用户拒绝 → 不替换，询问是否还有其他发言人需要处理

**追问是否还有其他人需要识别：**

> "还有其他发言人需要我帮你识别和替换吗？比如告诉我『某某人主要讲了什么内容』，我可以帮你匹配。"

**严禁的行为：**
- [禁止] 总结完就结束，不引导用户替换发言人——用户下次看听记时发言人还是"发言人1"，体验极差
- [禁止] AI 自行决定替换而不向用户确认
- [禁止] 只在回复中口头说"发言人1就是张三"，但不实际调用 `speaker replace` 写回听记

### 获取听记中提取的待办事项
```
Usage:
  dws minutes get todos [flags]
Example:
  dws minutes get todos --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)，取值逻辑参考 ## 注意事项
```

每条记录包含: 待办内容、待办唯一ID、参与人信息、待办时间

### 批量查询听记详情
```
Usage:
  dws minutes get batch [flags]
Example:
  dws minutes get batch --ids uuid1,uuid2,uuid3
Flags:
      --ids string   听记 taskUuid 列表，逗号分隔 (必填)
```

返回字段: 听记标题、时长、参与人列表、创建时间、taskUuid、听记状态

### 修改听记标题
```
Usage:
  dws minutes update title [flags]
Example:
  dws minutes update title --id <taskUuid> --title "Q2 复盘会议"
Flags:
      --id string      听记 taskUuid (必填)，取值逻辑参考 ## 注意事项
      --title string   新标题 (必填)
```

### 发起听记（开始录音）
```
Usage:
  dws minutes record start [flags]
Example:
  dws minutes record start
  dws minutes record start --session-id <sessionId>
Flags:
      --session-id string   AI 助理会话 ID (可选)
```

### 暂停听记录音
```
Usage:
  dws minutes record pause [flags]
Example:
  dws minutes record pause --id <taskUuid>
  dws minutes record pause --id <taskUuid> --session-id <sessionId>
Flags:
      --id string           听记 taskUuid (必填)
      --session-id string   AI 助理会话 ID (可选)
```

### 恢复听记录音
```
Usage:
  dws minutes record resume [flags]
Example:
  dws minutes record resume --id <taskUuid>
  dws minutes record resume --id <taskUuid> --session-id <sessionId>
Flags:
      --id string           听记 taskUuid (必填)
      --session-id string   AI 助理会话 ID (可选)
```

### 结束听记录音
```
Usage:
  dws minutes record stop [flags]
Example:
  dws minutes record stop --id <taskUuid>
  dws minutes record stop --id <taskUuid> --session-id <sessionId>
Flags:
      --id string           听记 taskUuid (必填)
      --session-id string   AI 助理会话 ID (可选)
```

### 更新纪要内容
```
Usage:
  dws minutes update summary [flags]
Example:
  dws minutes update summary --id <taskUuid> --content "新的纪要内容"
Flags:
      --id string        听记 taskUuid (必填)
      --content string   新的纪要内容 (必填)
```

用传入的摘要文本全量覆盖听记的纪要内容，不触发 AI 重新生成。适用于用户手动编辑或 AI Agent 修改纪要的场景。

**重要 — 修改纪要的完整流程（必须严格执行）：**
当用户要求"精简纪要/优化纪要/修改纪要内容"时，**必须完成以下三步，缺一不可**：
1. **读取**：先调用 `get summary --id <taskUuid>` 获取当前纪要原文
2. **修改**：AI 根据用户要求对纪要内容进行修改（如精简、重新整理、格式优化等），但必须遵守以下约束：
   - **图片必须保留**：原文中的所有 Markdown 图片（如 `![alt](url)`）必须完整保留，不得删除、漏掉、替换为纯文本或打乱语义位置
   - **仅优化文本内容**：可以调整标题层级、段落结构、列表与措辞，但不得破坏图片与对应上下文的关联关系
3. **校验**：写回前必须执行 Markdown 格式检查，确保输出结构合理、可渲染、无明显格式错误（如未闭合代码块、列表层级混乱、标题层级异常等）
4. **写回**：将修改后的完整纪要内容通过 `update summary --id <taskUuid> --content "修改后的完整纪要"` **写回听记**，确保修改持久化

**严禁只读取和修改纪要而不调用 `update summary` 写回**，否则用户看到的仍然是原始纪要，修改不会生效。

### 创建思维导图
```
Usage:
  dws minutes mind-graph create [flags]
Example:
  dws minutes mind-graph create --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)
```

触发创建听记思维导图任务。触发成功后，可通过 `mind-graph status` 轮询任务状态。状态：0=进行中，1=成功，2=失败。

**重要：当用户要求"生成思维导图/创建脑图"时，必须调用此命令（`mind-graph create`），严禁自行生成 HTML 或其他格式的思维导图。** 思维导图由服务端专业引擎生成，AI 不应尝试自己构造思维导图内容。

### 查询思维导图状态
```
Usage:
  dws minutes mind-graph status [flags]
Example:
  dws minutes mind-graph status --id <taskUuid>
Flags:
      --id string   听记 taskUuid (必填)
```

查询指定听记的思维导图生成状态。返回任务状态：0=进行中，1=成功，2=失败。如果没有返回任务状态，也视为成功。

### 替换发言人
```
Usage:
  dws minutes speaker replace [flags]
Example:
  dws minutes speaker replace --id <taskUuid> --from "张三" --to "李四"
  dws minutes speaker replace --id <taskUuid> --from "张三" --to "李四" --target-uid <uid>
Flags:
      --id string           听记 taskUuid (必填)
      --from string         源发言人昵称 (必填)
      --to string           目标发言人昵称 (必填)
      --target-uid string   目标发言人钉钉 UID (可选)
```

批量替换听记转写中指定发言人，将源发言人（speakerNick）精确匹配的所有段落替换为目标发言人。支持同时替换 nickName 和 subSpeakerNickname 两种匹配方式，并自动更新纪要、待办中的发言人信息。

**重要：**
- 此命令支持替换**任意发言人**，包括已关联通讯录信息的发言人（如"张三"、"李四"等真实姓名），不仅限于"发言人1"之类的占位符
- `--from` 填写当前听记中显示的发言人名称（无论是"发言人1"还是真实姓名），`--to` 填写要替换成的目标名称
- 如果用户希望将发言人关联到通讯录中的具体联系人，可通过 `--target-uid` 传入目标用户的钉钉 UID

### 触发创建发言人段落总结任务
```
Usage:
  dws minutes speaker summary create [flags]
Example:
  dws minutes speaker summary create --ids <uuid1,uuid2>
  dws minutes speaker summary create --task-uuids <uuid1,uuid2>
Flags:
      --ids string          听记 taskUuid 列表，逗号分隔 (必填)
      --task-uuids string   --ids 的别名，同样接受逗号分隔的 taskUuid 列表
```

触发创建发言人的段落总结任务，将听记中每位发言人的所有发言内容汇总总结。触发后需调用 `speaker summary get` 查询总结结果。

> **参数别名说明**：`--ids` 和 `--task-uuids` 等价，均可使用。推荐统一用 `--ids`。

### 查询发言人段落总结结果
```
Usage:
  dws minutes speaker summary get [flags]
Example:
  dws minutes speaker summary get --ids <uuid1,uuid2>
  dws minutes speaker summary get --task-uuids <uuid1,uuid2>
Flags:
      --ids string          听记 taskUuid 列表，逗号分隔 (必填)
      --task-uuids string   --ids 的别名，同样接受逗号分隔的 taskUuid 列表
```

查询发言人段落总结任务的结果，返回每位发言人的发言汇总。需先调用 `speaker summary create` 触发任务。

> **参数别名说明**：`--ids` 和 `--task-uuids` 等价，均可使用。推荐统一用 `--ids`。

**speaker summary 异步轮询策略（必须严格遵守）：**

`speaker summary create` 只是触发后端异步任务，**不会立即返回总结结果**。AI 必须按以下策略轮询 `speaker summary get`：

1. 调用 `speaker summary create --ids <uuids>` 触发任务
2. **等待至少 5 秒**后，首次调用 `speaker summary get --ids <uuids>` 查询结果
3. 如果返回结果为空（无内容），**继续等待 5 秒**后重试
4. **最大轮询次数不超过 20 次**（即最长等待约 100 秒）
5. 如果 20 次轮询后仍然为空，视为**任务未完成或无内容**，告知用户："发言人段落总结任务可能仍在处理中，请稍后再试。"

```
伪代码：
speaker summary create --ids <uuids>
wait 5s
for i in 1..20:
    result = speaker summary get --ids <uuids>
    if result 不为空:
        break  # 拿到结果，继续后续流程
    wait 5s
if 仍为空:
    告知用户任务可能仍在处理中
```

**严禁**：
- 调用 `create` 后立即调用 `get`（不等待）——后端异步任务需要时间生成
- 只轮询 1-2 次就放弃——正常任务可能需要 10-30 秒
- 轮询超过 20 次——避免无限等待

**speaker summary 典型应用场景 — 通过发言主题匹配发言人：**

当用户只知道某个人的发言主题或关联内容（如"讲战略规划的那个人是谁"），但不知道对应的发言人编号时，可以通过 speaker summary 获取每位发言人的段落总结，再用关键词匹配确认身份。

**完整链路（发言人段落总结 → 关键词匹配 → 确认 → 替换）：**

1. **确定听记 taskUuid**：从 `list mine/all` 或用户提供的 URL 中获取
2. **触发发言人段落总结**：`dws minutes speaker summary create --ids <taskUuid> --format json`
3. **延迟轮询获取结果**：等待 5s 后调用 `dws minutes speaker summary get --ids <taskUuid> --format json`，最多轮询 20 次
4. **AI 按发言人分组展示总结**：将每位发言人的段落总结以结构化方式呈现给用户
5. **关键词匹配**：从用户描述中抽取关键词（如"战略规划"、"供应链"），在各发言人的段落总结中做模糊匹配
6. **引导用户确认**：
   - 唯一高置信命中 → "『发言人1』的段落总结涉及『战略规划、AI化转型』，很可能就是你说的『李总』，确认替换吗？"
   - 多候选 → 列出所有命中的发言人编号 + 匹配的总结关键词，让用户挑选
   - 无匹配 → 请用户补充更具体的关键词
7. **用户确认后执行替换**：`dws minutes speaker replace --id <taskUuid> --from "发言人1" --to "李总"`

**自然语言触发示例：**

| Query | AI 处理策略 |
|-------|-----------|
| "帮我看看这个会议里每个人主要说了什么" | speaker summary create → 轮询 get → 按发言人分组展示 |
| "讲战略规划的那个人是谁" | speaker summary create → 轮询 get → 关键词匹配 → 引导确认 |
| "帮我把每个发言人的核心观点总结一下" | speaker summary create → 轮询 get → 结构化输出 |
| "我想知道会上谁讲了供应链相关内容" | speaker summary create → 轮询 get → "供应链"关键词匹配 |
| "那个讲招聘进度的人应该是张三" | speaker summary create → 轮询 get → "招聘进度"匹配 → 确认 → speaker replace |

> **与 `get transcription` 聚类方案的区别**：`speaker summary` 是后端 AI 生成的结构化总结，信息密度更高、匹配更精准；而 `get transcription` 聚类是 AI 在本地按 speakerNick 分组原文再提取要点，适合需要看原文细节的场景。两者可配合使用：先用 speaker summary 快速定位，再用 transcription 看具体原文。

### 添加个人热词
```
Usage:
  dws minutes hot-word add [flags]
Example:
  dws minutes hot-word add --words "钉钉"
  dws minutes hot-word add --words "OKR,钉钉,Copilot"
Flags:
      --words string   要添加的热词，多个用逗号分隔 (必填)
```

添加听记个人热词，用于优化语音识别中专有名词、人名等的识别准确率。支持一次添加多个热词（逗号分隔），每个热词长度不超过 10 个汉字或 5 个英文单词。

### 查询我的热词列表
```
Usage:
  dws minutes hot-word list
Example:
  dws minutes hot-word list
```

查询当前用户配置的所有听记热词列表。无需传入额外参数，系统自动识别当前用户身份。返回用户已添加的全部热词，适用于查看已有热词、去重检查等场景。

### 查找替换听记文字
```
Usage:
  dws minutes replace-text [flags]
Example:
  dws minutes replace-text --id <taskUuid> --search "旧文字" --replace "新文字"
Flags:
      --id string        听记 taskUuid (必填)
      --search string    要查找的文字 (必填)
      --replace string   替换为的新文字 (必填)
```

把听记中所有出现的原文字替换为目标文字，包括转写段落和纪要摘要中出现的原文字都会被替换。区分大小写，精确匹配。

**重要 — 执行前必须检查特殊字符并提示用户确认：**
用户输入的原始文本（`--search`）或目标文本（`--replace`）中可能携带特殊字符（如引号 `"` `'` `"` `"`、书名号 `《》`、括号 `【】()（）`、星号 `*`、反斜杠 `\`、换行符、Markdown 格式符号等）。这些特殊字符在转写原文中**通常不存在**，会导致精确匹配失败（替换 0 处）。

AI **必须在执行 `replace-text` 之前**检查用户输入，若发现特殊字符，**先向用户确认**：

> "我注意到你输入的文本中包含特殊字符（如 `…`），转写原文中通常不会包含这些字符，直接匹配可能找不到。建议去掉特殊字符后再替换：
>
> - 原始文本：`XXX` → 建议改为：`YYY`
> - 目标文本：`AAA` → 建议改为：`BBB`
>
> 使用去掉特殊字符的版本执行替换？[是] [否，使用原始输入]"

- 用户确认 → 使用清理后的文本执行替换
- 用户选择原始输入 → 按原样执行，尊重用户意图
- **严禁不提示就自行去掉特殊字符**——用户可能确实需要匹配带特殊字符的文本

**重要 — 执行后必须主动引导用户添加热词（避免长期反复识别错）：**
`replace-text` 仅修正**当前这一篇听记**的文字，**不会影响后续新听记的语音识别结果**。如果用户替换的是一个**长期容易被识别错的专有名词、人名、产品名**（如把"付工"改成"悟空"、把"非书"改成"飞书"），AI **必须在 `replace-text` 成功后主动追问用户**：

> "我已经把这篇听记里的『旧文字』替换为『新文字』。如果这个词以后也容易被识别错，建议把它加到个人热词里，后续新听记就不会再识别错了。要我现在帮你执行 `dws minutes hot-word add --words "新文字"` 吗？"

用户确认后立即调用 `hot-word add`。**严禁只做替换不引导**——这会让用户每次都要手动改一次，体验非常差。

### 创建文件上传会话或者文件转听记或者链接转听记
```
Usage:
  dws minutes upload create [flags]
Example:
  dws minutes upload create --file-name "meeting.mp4" --file-size 102400
  dws minutes upload create --file-name "meeting.mp4" --file-size 102400 --title "周会录音"
  dws minutes upload create --file-name "meeting.mp4" --file-size 102400 --input-language "zh" --enable-message-card
Flags:
      --file-name string        文件名（含后缀），如 meeting.mp4 (必填)
      --file-size int           文件大小（字节）(必填，正整数)
      --title string            听记标题，不传时默认使用文件名去掉后缀 (可选)
      --template-id string      纪要生成使用的模板 ID (可选)
      --input-language string   ASR 识别的源语言 (可选)
      --enable-message-card     是否推送闪记卡片消息 (可选，默认 false)
```

创建文件上传会话，获取预签名上传 URL。调用方拿到 URL 后，直接用 HTTP PUT 将文件上传到该 URL。必须与 `upload complete` 配合使用：
1. 调用 `upload create` 获取预签名上传 URL 和 sessionId
2. HTTP PUT 预签名上传 URL 上传文件（不带 HEADER）
3. 调用 `upload complete` 传入 sessionId 完成创建

### 完成文件上传并创建听记
```
Usage:
  dws minutes upload complete [flags]
Example:
  dws minutes upload complete --session-id <sessionId>
Flags:
      --session-id string   上传会话 ID，来自 upload create 返回的 sessionId (必填)
```

文件上传完成后，调用此命令创建听记。必须在 `upload create` 之后、预签名 URL 上传完成后调用。幂等：同一 sessionId 重复调用直接返回已有任务，不会重复创建。

### 取消文件上传会话
```
Usage:
  dws minutes upload cancel [flags]
Example:
  dws minutes upload cancel --session-id <sessionId>
Flags:
      --session-id string   要取消的会话 sessionId (必填)
```

取消 `upload create` 创建的上传会话，释放服务端资源。用于在上传前或上传失败后取消会话。

### 批量添加听记成员并设置权限
```
Usage:
  dws minutes permission add [flags]
Example:
  dws minutes permission add --ids <uuid1,uuid2> --member-uids 123456,789012 --policy 3
  dws minutes permission add --ids <uuid> --member-uids 123456 --policy 2 --cover
  dws minutes permission add --ids <uuid> --member-uids 123456 --policy 3 --sub-resources "OrigContent,Summary"
  dws minutes permission add --uuids <uuid1,uuid2> --member-uids 123456 --policy 3
  dws minutes permission add --task-uuids <uuid1,uuid2> --member-uids 123456 --policy 3
Flags:
      --ids string            听记 taskUuid 列表，逗号分隔 (必填)
      --uuids string          --ids 的别名
      --task-uuids string     --ids 的别名
      --member-uids string    成员钉钉 UID 列表，逗号分隔 (必填)
      --policy int            权限类型: 0=管理员, 1=所有者, 2=可编辑, 3=可查看/下载, 4=仅查看 (必填)
      --cover                 是否覆盖已有权限 (可选，默认 false)
      --sub-resources string  权限子模块，逗号分隔: OrigContent/Summary/Analysis/Note (可选)
```

批量给多个听记增加成员，并设置成员的权限。

**权限类型说明：**

| --policy 值 | 含义 | 说明 |
|------------|------|------|
| 0 | 管理员 | 可管理听记的所有设置和成员权限 |
| 1 | 所有者 | 听记的所有者，拥有最高权限 |
| 2 | 可编辑 | 可编辑听记的纪要、待办等内容 |
| 3 | 可查看/下载 | 可查看和下载听记内容，不可编辑 |
| 4 | 仅查看 | 仅可查看听记内容，不可下载 |

**权限子模块说明（--sub-resources，可选）：**

| 子模块 | 含义 |
|--------|------|
| OrigContent | 原始内容（转写原文） |
| Summary | 纪要（AI 摘要） |
| Analysis | 分析 |
| Note | 笔记 |

不传 `--sub-resources` 时，默认对所有子模块生效。传入后仅对指定的子模块授权。

**典型使用场景：**
- 会议结束后将听记共享给未参会的同事查看
- 给团队成员批量授权某几篇听记的编辑权限
- 限制只共享纪要而不共享原始转写内容

### 批量移除听记成员权限
```
Usage:
  dws minutes permission remove [flags]
Example:
  dws minutes permission remove --ids <uuid1,uuid2> --member-uids 123456,789012
  dws minutes permission remove --ids <uuid> --member-uids 123456
  dws minutes permission remove --uuids <uuid1,uuid2> --member-uids 123456
  dws minutes permission remove --task-uuids <uuid1,uuid2> --member-uids 123456
Flags:
      --ids string            听记 taskUuid 列表，逗号分隔 (必填)
      --uuids string          --ids 的别名
      --task-uuids string     --ids 的别名
      --member-uids string    成员钉钉 UID 列表，逗号分隔 (必填)
```

批量移除多个听记的成员权限。移除后，对应成员将失去对这些听记的访问权限。

**注意事项：**
- 移除权限后，该成员将无法再访问对应的听记内容
- 如果成员是听记的创建者（所有者），无法通过此命令移除其权限
- 建议在移除前先确认成员的当前权限，避免误操作

## 意图判断

### 发起听记
用户说"开始听记/开始录音/发起一个听记/启动听记/开始听记录这次会议/我要开始听记了/录音开始/我要听记" → `record start`

**自然语言示例：**
- "开始听记开启听记"
- "开始录音"
- "发起一个听记"
- "启动听记"
- "开始听记录这次会议"
- "我要开始听记了"
- "录音开始"

### 暂停/继续录制
用户说"暂停一下/暂停录音" → `record pause`
用户说"继续/继续录音" → `record resume`

**自然语言示例：**
- "暂停一下"
- "继续"

### 结束录制
用户说"结束听记/结束录音/停止录音/结束" → `record stop`

**自然语言示例：**
- "结束听记"
- "结束录音"
- "停止"

### 文件转听记
用户说"把文件转成听记/上传音频文件/上传录音/把这个 mp3 转成听记/帮我把会议录音转写一下/把附件里的音频文件做转写" → `upload create` → HTTP PUT → `upload complete`

**自然语言示例：**
- "把这个文件转写生成纪要"
- "帮我把这段录音转成文字"
- "上传一个音频文件，帮我转写"
- "我有一个录音文件，帮我生成听记"
- "把这个 mp3 转成听记"
- "帮我把会议录音转写一下"
- "这是昨天的录音，帮我整理成纪要"
- "把附件里的音频文件做转写"

用户说"取消上传" → `upload cancel`

### 查询我的热词列表
用户说"看看我的热词/我加了哪些热词/热词列表/查热词" → `hot-word list`

**自然语言示例：**
- "我之前加过哪些热词"
- "看看我的热词列表"
- "查一下我配置的热词"
- "查一下我听记的热词"

### 添加热词
用户说"加热词/添加热词/设置热词/把某个词加到热词里/这个词总是识别错" → `hot-word add`

**自然语言示例：**
- "转写不准，应该是'悟空'而不是'付工'"
- "帮我把'钉钉'加到热词里"
- "这个词总是识别错，加一下热词"
- "'飞书'总被识别成'非书'，加个热词"
- "帮我设置一下热词，'悟空'经常识别不对"
- "我们公司有个产品叫'魔镜'，加一下热词避免识别错"
- "把'AI听记'加进热词库"
- "这个专有名词总转写错，帮我加到热词"

### 查找替换
用户说"把所有的A替换成B/查找替换/批量替换文字" → `replace-text`，**执行成功后必须主动引导用户："要不要把『新文字』加到热词里，避免后续新听记再识别错？" 用户确认后再调用 `hot-word add`**

**自然语言示例：**
- "把所有的A，替换成B"
- "把听记里的'付工'都改成'悟空'"
- "这一篇里把'非书'替换成'飞书'"

**正确的对话节奏（必须遵守）：**
1. **检查特殊字符**：检查用户提供的原始文本和目标文本是否包含特殊字符（引号 `"` `'` `"` `"`、书名号 `《》`、括号 `【】()（）`、星号 `*`、反斜杠 `\`、Markdown 格式符号等）。若包含，先提示用户确认：
   > "你输入的文本中包含特殊字符 `…`，转写原文中通常不包含这些字符，直接匹配可能替换不到。建议去掉特殊字符后替换：原始文本 `XXX` → 建议 `YYY`。使用去掉特殊字符的版本？[是] [否，使用原始输入]"
   - 用户确认 → 使用清理后文本
   - 用户选择原始输入 → 按原样执行
   - **严禁不提示就自行去掉特殊字符**
2. 执行 `replace-text` 完成本篇替换
3. 立即追问："如果这个词以后也容易被识别错，建议加个热词，后续新听记就不会再错了。要现在加吗？"
4. 用户确认 → 调用 `hot-word add --words "<新文字>"`
5. 用户拒绝 → 直接结束，不再追问

### 修改说话人
用户说"替换发言人/修改发言人/换发言人名字/把说话人改成某某/发言人标注错了" → `speaker replace`

**关键：支持替换任意发言人，包括已关联通讯录信息的真实姓名（如"张三"、"拾光"等），不限于"发言人1"等占位符。** 用户提到"把 XX 改成 YY"时，XX 即为 `--from`，YY 即为 `--to`。

**默认引导通讯录查询获取 dingUid（推荐流程）：**

替换发言人时，`--target-uid` 参数可以将发言人关联到钉钉通讯录真实身份。**AI 应默认引导用户通过通讯录模糊查询获取目标人员的 dingUid**，而非让用户自行提供。流程如下：

1. 用户提供目标姓名（如"张三"）后，**AI 主动调用通讯录模糊查询**：
   ```
   dws contact user search --query "张三" --format json
   ```
2. 从返回结果中提取匹配的人员列表，每条包含 `userId`（长整型数字，即 dingUid）和姓名：
   - **唯一匹配** → 直接向用户确认："通讯录中找到『张三（userId: 123456789）』，确认将发言人替换为此人并关联通讯录身份？"
   - **多个匹配** → 列出候选让用户选择：
     > 通讯录中找到多个匹配，请选择目标人员：
     >
     > | # | 姓名 | 部门 | userId |
     > |---|------|------|--------|
     > | 1 | 张三 | 产品部 | 123456789 |
     > | 2 | 张三丰 | 技术部 | 987654321 |
     >
     > 请输入序号选择。
   - **无匹配** → 提示用户："通讯录中未找到『张三』，可以直接替换发言人名称（不关联通讯录身份），或提供更精确的姓名重新查询。直接替换？[是] [重新查询]"
3. 获取到 userId 后，在 `speaker replace` 中附加 `--target-uid <userId>`，实现发言人关联通讯录身份

> **注意**：`dws contact user search` 返回的 `userId` 是**长整型数字**（如 `123456789`），这就是 `--target-uid` 需要的值。不要与 `staffId`、`unionId` 等其他 ID 混淆。

**发言人映射模式识别（高频场景）：**

用户经常使用如下格式批量告知发言人对应关系：
- `发言人1:张三`
- `发言人1：张三`（中文冒号）
- `发言人1->张三`
- `发言人1 -> 张三`
- `发言人1 = 张三`
- 多条换行或逗号分隔：`发言人1:张三, 发言人2:李四, 发言人3:王五`

**AI 识别到上述模式时，必须执行以下流程：**

1. **解析映射关系**：提取所有 `源发言人 → 目标姓名` 的配对
2. **批量通讯录查询**：对每个目标姓名调用 `dws contact user search --query "<姓名>" --format json`，获取 userId（dingUid）。将查询结果汇总后一并向用户确认
3. **向用户确认**：展示解析结果（含通讯录匹配）并请求确认，格式如下：

> 我识别到以下发言人对应关系，并已从通讯录中匹配到对应人员：
>
> | 当前标注 | 替换为 | 通讯录匹配 | userId |
> |---------|--------|-----------|--------|
> | 发言人1 | 张三 | 张三（产品部） | 123456789 |
> | 发言人2 | 李四 | 李四（技术部） | 987654321 |
>
> 替换后，发言人将关联到通讯录真实身份，该听记中所有对应的发言段落标注、纪要和待办中的发言人信息都会同步更新。
>
> 确认执行？[确认] [取消]

4. **用户确认后**，逐条执行 `speaker replace`（附带 `--target-uid`）：
   ```
   dws minutes speaker replace --id <taskUuid> --from "发言人1" --to "张三" --target-uid 123456789 --format json
   dws minutes speaker replace --id <taskUuid> --from "发言人2" --to "李四" --target-uid 987654321 --format json
   ```
   如果某个姓名未在通讯录中找到匹配，则该条不带 `--target-uid`：
   ```
   dws minutes speaker replace --id <taskUuid> --from "发言人3" --to "王五" --format json
   ```
5. **全部完成后**，主动追问是否需要同时执行文本替换（`replace-text`），因为纪要正文中可能也残留了"发言人1"等文字：

> 发言人标注已全部替换完成。纪要正文中如果还有残留的「发言人1」「发言人2」等文字，是否也一并替换？[是] [否]

6. 用户确认后，对每条映射执行 `replace-text`：
   ```
   dws minutes replace-text --id <taskUuid> --search "发言人1" --replace "张三" --format json
   ```

**禁止行为：**
- 禁止不确认就直接执行替换（发言人替换不可逆）
- 禁止只执行 `speaker replace` 不引导 `replace-text`（纪要正文可能有残留）
- 禁止只执行 `replace-text` 不执行 `speaker replace`（转写段落的发言人标注不会被 replace-text 修改）
- 禁止跳过通讯录查询直接要求用户提供 dingUid——应由 AI 主动查询并引导选择

**自然语言示例：**
- "把这一段内容的说话人全部改成张三"
- "帮我把发言人1改成李总"
- "这个说话人标注错了，改成王伟"
- "把所有'发言人2'替换成'小明'"
- "说话人识别错了，帮我修改一下"
- "把这段的说话人改成'拾贝'"
- "发言人1是我，帮我把名字改过来"
- "把'未知说话人'全都改成张总"
- "把这里面的发言人1，替换成拾光"
- "张三其实是李四，帮我改一下"
- "发言人1:张三 发言人2:李四 发言人3:王五"
- "发言人1->张三, 发言人2->李四"
- "发言人1是张三，发言人2是李四"

### 查某人在听记中说了什么（发言人识别与总结）

**触发条件**（必须同时满足）：
1. 用户提供了听记来源（URL / 时间 / 关键词可定位到具体听记）
2. 用户指定了一个具体人名（花名/真名/关系称谓均可）
3. 用户的目标是获取该人在会议中说了什么

**不触发**（走其他链路）：
- 用户只要整体纪要、未指定特定人物 → `get summary`
- 用户直接说"把发言人1改成张三" → `speaker replace`（无需走推断链路）
- 用户想提取待办 → `get todos`

用户说"帮我看看某人说了什么/某人在会上提了哪些观点/某人有什么发言" → 走**发言人识别与总结**完整链路（详见 [获取听记语音转写原文](#获取听记语音转写原文) 章节中的"发言人识别与总结执行链路"）

**自然语言示例：**

| Query | 关键线索 |
|-------|----------|
| "帮我看看这个听记里张三说了什么" + URL | 听记URL + 人名 |
| "李总在今天的会上提了哪些观点" | 时间(今天) + 人名 + 观点 |
| "上周产品评审里王五有什么发言" | 时间(上周) + 关键词(产品评审) + 人名 |
| "这个会议里我老板说了啥" | 听记上下文 + 关系称谓(需解析为人名) |
| "帮我找一下设计师小陈在访谈里的观点" | 角色提示(设计师) + 人名 + 场景 |

- "帮我看看这个听记里张三说了什么"
- "李总在今天的会上提了哪些观点"
- "上周产品评审里王五有什么发言"
- "这个会议里我老板说了啥"
- "帮我找一下设计师小陈在访谈里的观点"
- "拾光在这个会议里讲了什么"
- "帮我总结一下这次会议里李总的核心发言"
- "王经理今天开会说了啥重点"

**关键约束（必须遵守）：**
- 这是一个**完整的识别 → 推断 → 确认 → 总结 → 引导替换**链路，不要只做总结就结束，**必须引导用户将「发言人1」等占位符替换为真实姓名**
- 完整执行链路见 [获取听记语音转写原文](#获取听记语音转写原文) 章节中的"发言人识别与总结执行链路"

### 通过关键词模糊匹配确认发言人（与 `get transcription` 联动）
当用户在拉取完转写原文（且已按发言人聚类）之后，提供形如『某某人主要讲了 XX』『某某人负责 YY』『某某人这次的核心是 ZZ』的描述时，**不要直接做替换**，而是按下面的链路推进：

1. **从用户输入中抽取关键词**（如『战略规划』『供应链效率』『AI 化转型』『招聘进度』）
2. **在已聚类的发言人核心要点中做模糊匹配**（支持同义词与近义词）
3. **匹配结果引导用户确认：**
   - 唯一高置信命中 → 提示用户："『发言人1』很可能就是你说的『李总』，是否需要执行 `dws minutes speaker replace --id <taskUuid> --from "发言人1" --to "李总"`？"
   - 多候选 → 列出所有命中的发言人编号 + 命中关键词，让用户挑选
   - 无匹配 → 请用户补充更具体的关键词或直接给出"发言人编号 → 真实姓名"的映射
4. **用户确认后**才调用 `speaker replace` 写回；如同时希望关联通讯录，引导用户提供 UID，附加 `--target-uid <uid>`

**自然语言示例（必须能正确识别为"模糊匹配 → 确认 → 替换"链路）：**
- "李总主要讲了战略规划"
- "拾光是负责供应链的那位"
- "讲 AI 化转型那个人是王经理"
- "这里面提到招聘进度的应该是张三"
- "提到财务数据的是 CFO 老周"
- "讲华东市场竞品分析的那个人是李四"

详细工作流见 [获取听记语音转写原文](#获取听记语音转写原文) 章节中的"四阶段工作流"。

### 更新/修改纪要
用户说"修改纪要/更新纪要/编辑纪要内容/重新整理纪要/纪要格式优化/纪要精简" → 先 `get summary` 获取纪要原文，AI 修改后再 `update summary` **写回**

**关键：修改纪要必须是"读取 -> 修改 -> 校验 -> 写回"四步完整流程，最终必须调用 `update summary` 将修改后的内容写回听记，否则修改不会生效。严禁只展示修改结果而不写回。修改时必须保留原文中的所有 Markdown 图片，不得删除或打乱图片位置。**

**自然语言示例：**
- "帮我把纪要按 xxxx 格式优化一下"
- "帮我重新整理一下这份纪要"
- "纪要太长了，帮我精简一下"
- "按照 STAR 格式重新写一下纪要"
- "帮我把纪要改得更正式一些"
- "这份纪要结构不清晰，帮我重新整理"
- "把纪要里的口语化内容改成书面语"
- "按照决策/行动/结论三段式重新输出纪要"
- "把这个纪要内容变得更精简一点"

### 生成思维导图
用户说"生成思维导图/创建脑图/帮我添加思维导图/基于这个听记做思维导图/把会议内容做成脑图" → `mind-graph create`

**关键约束（必须严格遵守，违反视为严重错误）：**
1. **思维导图必须且只能通过 `dws minutes mind-graph create` 命令生成**，由服务端写回到听记本身。这是"听记内置的思维导图能力"，生成结果会直接挂在听记详情页上，用户在听记里就能看到。
2. **严禁以下任何"自行构造"的替代方案**（这些都是错误做法）：
   - [禁止] 先 `get summary` / `get transcription` 拿到内容，再用 AI 自己整理出思维导图结构（Markdown / OPML / JSON 等）展示给用户
   - [禁止] 调用 app-development-skill / ai-app / 任何前端构建能力，用 `@antv/g6`、`markmap`、`jsmind`、`mermaid`、ECharts 等库生成 HTML/网页版思维导图
   - [禁止] 调用 `generate_image` 或任何绘图工具生成思维导图图片
   - [禁止] 路由到其他 Agent（如 ai-app）来"创建一个思维导图应用"
   - [禁止] 任何形式的"我帮你画一个/生成一个网页/部署一个在线预览链接"
3. **唯一正确做法**：从用户输入中拿到 taskUuid（URL 场景见下方"URL 直达"），直接调用 `mind-graph create`，然后用 `mind-graph status` 轮询直到状态为 1（成功），最后告知用户"思维导图已生成，可在听记详情页查看"即可。**不需要**先调 `get summary` 等任何读取命令——服务端会基于听记自身内容生成。

**URL 直达场景：**
当用户输入形如 `https://shanji.dingtalk.com/app/transcribes/{taskUuid}` 的链接 + "创建思维导图/生成脑图"等诉求时：
1. 直接从 URL 路径末段提取 taskUuid
2. **直接** `dws minutes mind-graph create --id <taskUuid> --format json`
3. 用 `mind-graph status` 轮询
4. 严禁中间插入 `get summary` / `get transcription` 等任何"先读取内容"的步骤，更严禁基于读到的内容自行构造思维导图

**自然语言示例：**
- "帮我添加思维导图"
- "生成思维导图"
- "帮我创建一个脑图"
- "把这个听记做成思维导图"
- "生成这个会议的思维导图"
- "基于这个听记创建思维导图"
- "把会议内容整理成脑图"
- "<听记URL> 创建思维导图"
- "<听记URL> 帮我生成脑图"

用户说"思维导图状态/脑图进度/思维导图好了吗" → `mind-graph status`

### 查询听记列表
用户说"我创建的听记/我发起的听记/我自己录的听记" → `list mine`（**仅**返回我自己创建的，不含他人共享给我的）
用户说"别人给我的听记/共享听记/共享给我的" → `list shared`（**仅**返回他人共享给我的，不含我创建的）
用户说"我可访问的听记/我能看到的听记/我有权限的听记/所有听记/全部听记" → `list all`（返回我可访问的**所有**听记 = mine + shared）
用户说"我的听记" → `list all`（**注意**："我的听记"是模糊表述，用户通常期望看到所有可访问的听记，而不仅是自己创建的，默认走 `all`；仅当上下文明确表示"我自己创建/发起的"时才走 `mine`）
用户说"某时间段内的听记/按时间查听记/按关键词查听记" → 根据所属范围选择 `list mine`/`list shared`/`list all`，附加 `--start`、`--end`、`--query` 参数

**自然语言示例：**
- "查一下我最近的听记"
- "帮我找一下上周的听记"
- "有没有关于'周会'的听记"
- "看看别人共享给我的听记"
- "我这个月的听记有哪些"
- "帮我从今天听记里找需求评审"
- "上月听记里有没有提到OKR"
- "从我的听记里搜一下技术方案"
- "这周有没有关于复盘的会"
- "帮我找上周关于项目排期的听记"

> **路由提示**：
> - 用户说"从我创建的/我发起的听记里找XX" → `list mine --query`
> - 用户说"我的听记里找XX"/"我可访问的听记"/"帮我找关于XX的听记" → `list all --query`（"我的"是模糊表述，默认走 `all` 覆盖最广）
> - **仅当**用户明确强调"我自己创建/录制/发起的"时才走 `mine`
> - 含时间词（今天/上周/上月/这周/本月）时，必须同时附加 `--start` + `--end` 参数

### 获取听记内容
用户说"听记详情/听记信息" → `get info`
用户说"摘要/总结/会议纪要/纪要" → `get summary`
用户说"关键字/关键词" → `get keywords`
用户说"原文/转写/录音文字/逐字稿" → `get transcription`（**默认拉取全部原文**，自动翻页直到拉完，**拉完后必须主动询问用户是否按发言人聚类**，详见 [获取听记语音转写原文](#获取听记语音转写原文) 的"四阶段工作流"）
用户说"会议待办/听记待办/待办事项" → `get todos`
用户说"批量查询/查多个听记" → `get batch`
用户说"音频地址/音频链接/录音文件/下载录音/音频下载/视频文件/媒体地址" → `get audio`

**转写原文拉取时机判断（必须遵守）：**
- **明确要看原文** → 调用 `get transcription`，自动翻页拉取所有原文；**累积超过 12000 字符时暂停，询问用户是否继续**
  - 示例："帮我看看转写原文"、"分析一下这篇听记的原文"、"把逐字稿发给我"
- **未明确要看原文** → **不要**调用 `get transcription`，用其他更合适的命令响应
  - 示例："查一下和悟空相关的听记" → 应走 `list`，不需要拉原文
  - 示例："这个会议讲了什么" → 应走 `get summary`，不需要拉原文
  - 示例："这个听记有哪些待办" → 应走 `get todos`，不需要拉原文

**自然语言示例：**
- "帮我看看这个听记的摘要"
- "这个会议讲了什么"
- "帮我看看会议的转写原文"
- "分析一下这篇听记的转写原文" → **默认拉全部**
- "这个听记有哪些待办"
- "帮我提取一下会议的关键词"
- "帮我看看这个听记的基本信息"
- "把这个听记的录音文件给我"
- "帮我下载这个听记的音频"
- "这个听记的音频地址是什么"
- "我想下载这次会议的录音"
- "帮我拿一下这个听记的视频文件"
- "给我这个听记的媒体下载链接"

### 添加/管理听记成员权限
用户说"把某人加到这个听记中/共享听记给某人/给某人添加听记权限/把听记分享给同事/让某人也能看这个听记" → `permission add`
用户说"添加参会者/批量添加参会者/添加成员/批量添加成员/加参会人/加入成员" → `permission add`（**注意：听记模块没有独立的"添加参会者/成员"命令，统一走权限接口**）
用户说"移除某人的权限/取消共享/不让某人看这个听记了" → `permission remove`

**自然语言示例：**
- "把张三加到这个听记中"
- "帮我把这个听记共享给李四"
- "让小王也能看这个听记"
- "给团队的同事们添加这个听记的查看权限"
- "把这几个听记共享给 UID 123456 的同事，设置为可编辑"
- "帮我把这个听记分享给参会的人"
- "给这个听记加个协作者"
- "把听记的编辑权限给我同事"
- "添加张三为这个听记的参会者"
- "帮我批量添加参会者：张三、李四、王五"
- "给这个听记添加几个成员"
- "批量添加成员到这个听记"
- "把张三加为参会人"
- "移除张三对这个听记的权限"
- "取消小王对这个听记的访问"
- "不让某人看这个听记了"

**路由规则：**
- 用户提到"加/添加/共享/分享/让...看/让...访问"等增加权限语义 → `permission add`
- 用户提到"添加参会者/加参会者/批量添加参会者/添加成员/加成员/批量添加成员/加参会人/添加参会人"等成员语义 → `permission add`（听记没有独立的"添加成员"接口，**统一走权限接口**）
- 用户提到"移除/取消/删除/不让...看"等移除权限语义 → `permission remove`
- 如果用户未指定权限类型，默认使用 `--policy 4`（可查看/下载/编辑）
- 如果用户未提供 member-uids，需要先引导用户提供目标成员的钉钉 UID（可通过 `dws contact user search --query "姓名"` 查询）

**典型执行链路：**
1. 用户说"把张三加到这个听记中" → AI 需获取张三的 UID
2. 调用 `dws contact user search --query "张三" --format json` 获取 UID
3. 调用 `dws minutes permission add --ids <taskUuid> --member-uids <uid> --policy 4 --format json`

### 修改听记标题
用户说"改听记标题/重命名听记/修改标题" → `update title`

**自然语言示例：**
- "把这个听记的标题改成'Q2 复盘会议'"
- "帮我重命名一下这个听记"

### URL 识别
- **格式**: `https://shanji.dingtalk.com/app/transcribes/{taskUuid}`
- **示例**: `https://shanji.dingtalk.com/app/transcribes/76327569643231383535353939365f3436383537393431335f32`
用户传入听记 URL（如 `https://shanji.dingtalk.com/app/transcribes/xxx`），从 URL 提取 taskUuid，再执行对应的 get/update 操作

## 核心工作流

```bash
# 0. 发起听记（开始录音）
dws minutes record start --format json

# 1. 查看我的听记列表 — 提取 taskUuid
dws minutes list mine --format json
dws minutes list mine --max 10 --next-token <nextToken> --format json
dws minutes list mine --query "周会" --format json

# 1b. 查看共享给我的听记
dws minutes list shared --max 20 --format json
dws minutes list shared --query "日报" --format json

# 1c. 查看我有权限访问的所有听记（支持关键字和时间范围筛选）
dws minutes list all --format json
dws minutes list all --query "周会" --start "2026-03-01T00:00:00+08:00" --end "2026-03-20T23:59:59+08:00" --format json

# 2. 获取 AI 摘要
dws minutes get summary --id <taskUuid> --format json

# 3. 查看完整转写原文（拉完后默认按时间线返回，AI 必须主动追问"是否按发言人聚类"）
dws minutes get transcription --id <taskUuid> --format json
# 3a. 用户确认聚类 → AI 在本地按 speakerNick 分组并提取核心要点（无需新调用 dws）
# 3b. 用户提供"某某人讲了 XX" → AI 模糊匹配关键词后，引导确认替换发言人
dws minutes speaker replace --id <taskUuid> --from "发言人1" --to "李总" --format json

# 4. 提取待办事项
dws minutes get todos --id <taskUuid> --format json

# 4b. 获取音频/视频地址（用于下载或播放原始媒体文件）
dws minutes get audio --id <taskUuid> --format json

# 5. 修改标题
dws minutes update title --id <taskUuid> --title "新标题" --format json

# 6. 更新纪要内容
dws minutes update summary --id <taskUuid> --content "新的纪要内容" --format json

# 7. 录音控制（基于 start 返回的 taskUuid）
dws minutes record pause --id <taskUuid> --format json
dws minutes record resume --id <taskUuid> --format json
dws minutes record stop --id <taskUuid> --format json

# 8. 思维导图
dws minutes mind-graph create --id <taskUuid> --format json
dws minutes mind-graph status --id <taskUuid> --format json

# 9. 替换发言人
dws minutes speaker replace --id <taskUuid> --from "张三" --to "李四" --format json

# 9b. 发言人段落总结（异步：create 触发 → 延迟 5s → 轮询 get，最多 20 次）
dws minutes speaker summary create --ids <taskUuid1,taskUuid2> --format json
# 等待至少 5 秒...
dws minutes speaker summary get --ids <taskUuid1,taskUuid2> --format json
# 若返回为空，继续等待 5s 后重试，最多 20 次

# 10. 添加个人热词
dws minutes hot-word add --words "OKR,钉钉,Copilot" --format json

# 11. 查找替换听记文字
dws minutes replace-text --id <taskUuid> --search "旧文字" --replace "新文字" --format json

# 12. 文件上传转听记（三步流程）
# 12a. 创建上传会话，获取预签名 URL 和 sessionId
dws minutes upload create --file-name "meeting.mp4" --file-size 102400 --format json
# 12b. 用 HTTP PUT 上传文件到预签名 URL（不带 HEADER）
curl -X PUT "<presignedUrl>" -T "/path/to/meeting.mp4"
# 12c. 通知服务端上传完成，创建听记
dws minutes upload complete --session-id <sessionId> --format json
# 12d.（可选）取消上传会话
dws minutes upload cancel --session-id <sessionId> --format json
```

## 上下文传递表

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `list mine` | `taskUuid`、`nextToken` | get/update 的 --id；翻页时 --next-token |
| `list shared` | `taskUuid`、`nextToken` | get/update 的 --id；翻页时 --next-token |
| `list all` | `taskUuid`、`nextToken` | get/update 的 --id；翻页时 --next-token |
| `get batch` | 各听记 `taskUuid` | 进一步查询详情 |
| `get audio` | 音频/视频 OSS 地址 | 用 HTTP GET 下载录音文件 / 在浏览器播放 |
| `record start` | `taskUuid`/`uuid` | record pause/resume/stop 的 --id |
| `upload create` | `sessionId`、`presignedUrl` | HTTP PUT 上传文件；upload complete/cancel 的 --session-id |
| `mind-graph create` | 任务状态 | mind-graph status 轮询 |

## 错误响应诊断指南

当 `get info` / `get summary` / `get transcription` / `get todos` / `get keywords` / `get audio` 返回异常时，按以下决策表快速判断原因并决定下一步动作，避免盲目重试浪费轮次：

### 错误快速决策表

| 错误现象 | 可能原因 | 正确处理 | 禁止动作 |
|----------|----------|----------|----------|
| `dingOpenErrcode=300` 且 `error_msg` 含 "taskUuid is invalid" | taskUuid 格式错误、不存在、或从上下文猜测/拼凑而来 | **立即停止当前 uuid，切换到 list 策略**：调用 `dws minutes list mine --max 10 --format json` 获取真实 uuid 列表，让用户选择或自动匹配最相关的一条。**严禁用同一个无效 uuid 重试**——真实案例中模型用同一个错 uuid 重试了 20 次全部失败 | 禁止用同一个无效 ID 重试哪怕 1 次；禁止从历史对话/文档/链接中猜测 uuid；禁止从十六进制编码字符串里截取数字拼凑新 uuid |
| stdout 完全为空，error_msg 也为空 | 鉴权过期 / 服务端临时不可用 | 最多重试 1 次；仍为空则告知用户「服务暂时不可用，请稍后再试」 | 禁止连续重试超过 2 次 |
| `dingOpenErrcode=403` 或含 "permission" / "forbidden" | 无权限访问该听记 | 告知用户无权限，建议联系听记创建者共享权限 | 禁止重试（权限问题不会因重试改变） |
| `dingOpenErrcode=404` 或含 "not found" | 听记已被删除或不存在 | 告知用户该听记不存在或已被删除 | 禁止重试 |
| 返回 JSON 但关键字段（如 `result`）为 null | 听记处理中（ASR/AI 摘要尚未完成） | 告知用户听记内容正在生成中，建议等待 1-2 分钟后再查询 | 禁止立即重试（处理需要时间） |
| 网络超时 / 连接错误 | 网络不稳定 | 最多重试 1 次 | 禁止连续重试超过 2 次 |

### 重试约束（必须遵守）

- 同一个 taskUuid + 同一个命令，最多重试 **1 次**（总计最多调用 2 次）
- 重试前必须检查：错误是否属于「可重试」类别（仅 stdout 为空 / 网络超时 属于可重试）
- `dingOpenErrcode=300/403/404` 均为「不可重试」错误，立即停止并给出明确诊断
- 如果第一次调用返回了结构化错误（含 errcode），禁止换一个别名 flag（如 --task-uuid 换 --id）重试——问题不在 flag 名称

### Flag 别名兼容说明

所有需要传入 taskUuid 的子命令（`get info/summary/keywords/transcription/todos/audio`、`mind-graph create/status` 等）均支持以下 flag 名称，自动降级到 `--id`：

| Flag 名称 | 状态 | 说明 |
|-----------|------|------|
| `--id` | 推荐 | 唯一的正式 flag 名称，文档和示例中统一使用 |
| `--url` | 隐藏别名 | 兼容听记 URL 传入场景 |
| `--task-uuid` | 隐藏别名 | 兼容钉钉 OpenAPI 原生字段名 taskUuid |
| `--uuid` | 隐藏别名 | 兼容通用 UUID 命名习惯 |

传入 `--task-uuid` / `--uuid` / `--url` 时，CLI 会自动降级到 `--id` 处理，无需报错后用 `--id` 重试。

## 关键词搜索与筛选最佳实践

### 服务端筛选优先原则（必须遵守）

`list mine` / `list shared` / `list all` 均支持 `--query`（关键词）、`--start`（开始时间）、`--end`（结束时间）三个筛选参数，筛选在服务端完成，效率远高于全量拉取后本地过滤。

**禁止全量拉取后本地过滤**：当用户提供了搜索关键词或时间范围时，必须使用 `--query` / `--start` / `--end` 参数在服务端筛选，严禁先 `list all` 拉全量再在 prompt 中用 Python/grep 做本地关键词匹配。

### 自然语言到参数映射表

| 用户表述 | 映射参数 | 示例命令 |
|----------|----------|----------|
| "今天的听记" | `--start` 今日 00:00 `--end` 当前时间 | `dws minutes list all --start "2026-05-11T00:00:00+08:00" --end "2026-05-11T23:59:59+08:00"` |
| "上周的听记" | `--start` 上周一 00:00 `--end` 上周日 23:59 | `dws minutes list all --start "2026-05-04T00:00:00+08:00" --end "2026-05-10T23:59:59+08:00"` |
| "上月的听记" | `--start` 上月 1 日 `--end` 上月最后一天 | `dws minutes list all --start "2026-04-01T00:00:00+08:00" --end "2026-04-30T23:59:59+08:00"` |
| "最近的听记" | 不传时间参数，使用默认排序 | `dws minutes list mine --max 10` |
| "关于XX的听记" | `--query "XX"` | `dws minutes list all --query "双叶汽车"` |
| "找XX相关的听记" | `--query "XX"` | `dws minutes list all --query "安利"` |
| "本月关于XX的听记" | `--query` + `--start` + `--end` | `dws minutes list all --query "ROI" --start "2026-05-01T00:00:00+08:00" --end "2026-05-31T23:59:59+08:00"` |
| "帮我从今天听记里找XX" | `--query` + `--start` + `--end`（今日范围） | `dws minutes list mine --query "需求评审" --start "2026-05-11T00:00:00+08:00" --end "2026-05-11T23:59:59+08:00"` |
| "上月听记里有没有提到XX" | `--query` + `--start` + `--end`（上月范围） | `dws minutes list mine --query "OKR" --start "2026-04-01T00:00:00+08:00" --end "2026-04-30T23:59:59+08:00"` |
| "从我的听记里搜XX" | `--query "XX"`（走 `list mine`） | `dws minutes list mine --query "技术方案"` |
| "这周有没有关于XX的会" | `--query` + `--start` + `--end`（本周范围） | `dws minutes list mine --query "复盘" --start "2026-05-05T00:00:00+08:00" --end "2026-05-11T23:59:59+08:00"` |

### 组合筛选示例

```bash
# 按关键词搜索
dws minutes list all --query "神威数智需求变更沟通会议" --format json

# 按时间范围搜索
dws minutes list all --start "2026-04-01T00:00:00+08:00" --end "2026-04-30T23:59:59+08:00" --format json

# 关键词 + 时间范围组合
dws minutes list all --query "精益生产" --start "2026-04-01T00:00:00+08:00" --end "2026-04-30T23:59:59+08:00" --format json

# 按关键词搜索共享听记
dws minutes list shared --query "安利" --max 20 --format json

# 从我的听记里按关键词 + 今日时间范围搜索（"帮我从今天听记里找需求评审"）
dws minutes list mine --query "需求评审" --start "2026-05-11T00:00:00+08:00" --end "2026-05-11T23:59:59+08:00" --format json

# 从我的听记里按关键词 + 上月时间范围搜索（"上月听记里有没有提到OKR"）
dws minutes list mine --query "OKR" --start "2026-04-01T00:00:00+08:00" --end "2026-04-30T23:59:59+08:00" --format json

# 从我的听记里仅按关键词搜索（"从我的听记里搜技术方案"）
dws minutes list mine --query "技术方案" --format json
```

### 搜索无结果时的 Fallback 策略

1. `--query` 无结果 --> 尝试缩短关键词（如「神威数智需求变更沟通会议」缩短为「神威数智」）
2. 仍无结果 --> 扩大搜索范围（`list mine` 换为 `list all`）
3. 仍无结果 --> 放宽时间范围（扩大 `--start` / `--end`）
4. 最终无结果 --> 如实告知用户未找到匹配的听记，建议用户确认关键词或时间范围

## 转写翻页异常处理

### 翻页空响应防御规则（必须遵守）

`get transcription` 使用 `--next-token` 进行分页查询。在自动翻页过程中，可能遇到以下异常情况：

| 异常现象 | 含义 | 处理方式 |
|----------|------|----------|
| 返回 JSON 中不包含 `nextToken` 字段 | 已到达最后一页，无更多数据 | 正常终止翻页，拼合已拉取内容 |
| `nextToken` 字段值为空字符串 `""` | 等同于无 nextToken，已到达最后一页 | 正常终止翻页 |
| 使用同一个 `next-token` 值连续 2 次返回 stdout 为空 | 服务端临时异常或该 token 已失效 | 立即终止翻页，不再重试；基于已拉取的内容进行分析 |
| 返回 JSON 但 `paragraphList` 为空数组 `[]` | 当前页无内容（可能是中间空页） | 如果有 nextToken 则继续翻页；如果无 nextToken 则终止 |

### 翻页流程伪代码

```
已累积文本 = ""
当前token = ""（首页不传）
连续空响应计数 = 0
上一次token = null

loop:
  调用 get transcription --id <uuid> [--next-token 当前token]
  
  if stdout 为空 or 返回无效:
    if 当前token == 上一次token:
      连续空响应计数 += 1
      if 连续空响应计数 >= 2:
        终止翻页，输出已累积内容
        break
    else:
      连续空响应计数 = 1
    上一次token = 当前token
    continue
  
  连续空响应计数 = 0
  拼合本页转写文本到已累积文本
  
  if len(已累积文本) > 12000:
    暂停翻页
    询问用户："当前已拉取约 N 字符，已达上限，是否继续？"
    if 用户拒绝: break
  
  if 返回中无 nextToken 或 nextToken 为空:
    break（最后一页）
  
  上一次token = 当前token
  当前token = 返回的 nextToken
```

### 严禁的翻页行为

- [禁止] 同一个 next-token 值连续重试超过 2 次
- [禁止] 翻页失败后换用不同的 flag 名（如 --next-token 换成 --nextToken）重试
- [禁止] 累积超过 12000 字符后不暂停，继续拉取所有页
- [禁止] 拉到空页就立即放弃全部已累积内容，应基于已有内容进行分析

## 注意事项
- `taskUuid` 是听记的唯一标识，所有 get/update 操作均以此为入参
- `record start` 对应 MCP 工具 `execute_listening_note_command` 的 `cmd=create`，通常会返回可继续控制录音的 `taskUuid/uuid`
- `record pause` / `record resume` / `record stop` 对应 `cmd=pause/resume/end`，需要传入 `--id`（映射 MCP 入参 `uuid`）
- 如果用户传入听记 URL（格式: `https://shanji.dingtalk.com/app/transcribes/<taskUuid>`），直接从路径末段提取 taskUuid 作为 `--id` 参数，无需再调用 list 查询
- `list mine`、`list shared`、`list all` 统一走 `list_by_keyword_and_time_range` 链路，通过 `belongingConditionId` 区分（`created` / `shared` / `noLimit`）
- 三个 list 命令均支持 `--max`、`--next-token` 分页及 `--query`、`--start`、`--end` 筛选
- `list mine`、`list shared` 默认每页 20 条，`list all` 默认每页 10 条
- `get summary` 返回 AI 生成的结构化 Markdown 摘要
- `get transcription` 的 `--direction` 控制时间排序: 0=正序(默认), 1=倒序；当用户明确要求查看/分析转写原文时，默认自动翻页拉取全部原文（不需要用户手动说"拉第一页"），如果用户意图不是专门看原文（如查列表、看摘要），则不应主动调用此命令
- `get transcription` 默认按"时间线"返回各段落，**拉完后 AI 必须主动追问用户"是否需要按发言人分组聚类并提取核心内容"**；用户确认后 AI 在本地完成聚类与摘要，并进一步引导用户通过关键词模糊匹配（如"李总主要讲了战略规划"）确认"发言人编号 ↔ 真实姓名"的映射，最终调用 `speaker replace` 写回。完整工作流见对应命令章节的"四阶段工作流"
- `get batch` 支持一次查询多个听记，用逗号分隔 taskUuid
- `get audio` 返回听记原始音频/视频文件的 OSS 地址，操作人需拥有该听记"读"权限及以上；以下场景不返回地址：听记已被删除、A1 无痕模式听记、临存过期的听记（媒体未准备好或临时存储已过期）
- `update summary` 全量覆盖纪要内容，不触发 AI 重新生成；适用于手动编辑或 AI Agent 修改纪要
- `mind-graph create` 触发异步任务，需通过 `mind-graph status` 轮询状态（0=进行中，1=成功，2=失败）
- `speaker replace` 精确匹配源发言人昵称，替换所有段落并自动更新纪要和待办中的发言人信息
- `hot-word add` 支持逗号分隔批量添加，每个热词不超过 10 个汉字或 5 个英文单词
- `replace-text` 区分大小写精确匹配，同时替换转写段落和纪要摘要中的文字
- 文件上传流程为三步：`upload create` → HTTP PUT 上传 → `upload complete`；`upload complete` 幂等，同一 sessionId 重复调用不会重复创建
- `upload create` 返回的 `presignedUrl` 用于 HTTP PUT 上传文件，上传时不需要带任何 HEADER
- 所有需要 taskUuid 的子命令均支持 --task-uuid / --uuid / --url 作为 --id 的隐藏别名，传入后自动降级，无需报错重试
- 同一个 taskUuid + 同一个命令，最多重试 1 次（总计最多调用 2 次），dingOpenErrcode=300/403/404 为不可重试错误
- 当用户提供关键词或时间范围时，必须使用 --query / --start / --end 在服务端筛选，严禁全量拉取后本地过滤。**严禁在 dws 命令后拼接 shell 管道做本地过滤**（如 `| grep "关键词"` / `| head -50` / `python -c "import json;..."` / `> /tmp/xxx.txt && grep ...`），这些写法在 Windows 沙箱里 100% 失败（`head`/`grep`/`wait` 不是 Windows 内置命令），即使在 macOS 上也会因 list 本身失败导致整个管道链路断掉。正确做法是始终使用 `--query` / `--start` / `--end` / `--max` 等 cli 内置参数在服务端完成筛选
- `dws minutes list` 后面**必须跟 `mine` / `shared` / `all` 子命令**，不能直接 `dws minutes list --start ... --end ...`（会报 `unknown flag: --start`）。正确写法：`dws minutes list mine --start ... --end ...` 或 `dws minutes list all --query "关键词" --start ... --end ...`
- get transcription 翻页时，同一个 next-token 连续返回空 2 次即终止翻页，不再重试
- **跨平台兼容性**：悟空运行环境可能是 Windows cmd / PowerShell / macOS bash，**严禁在 dws 命令中使用任何依赖特定 shell 的工具或管道**，包括但不限于：`| head`、`| grep`、`| tail`、`| wc`、`& wait`、`> %TEMP%\xxx`、`timeout /t 5`、`Start-Sleep`、`python -c "..."` 等。所有数据筛选、截断、过滤都必须通过 dws cli 自身的参数完成（`--query`/`--start`/`--end`/`--max`/`--next-token`）

### 错误恢复策略（AI 遇到以下错误时必须自动执行对应恢复动作）

| 错误类型 | 错误信息特征 | 恢复动作 | 严禁行为 |
|----------|-------------|----------|----------|
| 命令结构错误 | `unknown command "info"` / `unknown command "get"` (后无子命令) / `error[validation]: unknown flag: --start` (在 `list` 而非 `list mine/shared/all` 上) | **立即参照本文档顶部"命令层级结构"纠正**。常见错误：① `dws minutes info` → 应为 `dws minutes get info`；② `dws minutes get --id` → `get` 后缺子命令，应为 `get info/summary/transcription` 等；③ `dws minutes list --start` → `list` 后缺 scope，应为 `list mine/shared/all --start` | 严禁在命令结构报错后只换参数名不修正命令层级；严禁把 `get` 当作独立命令使用（后面必须跟 info/summary/transcription/keywords/todos/audio/batch） |
| 参数名错误 | `unknown flag: --task-uuid` / `unknown flag: --uuid` / `unknown flag: --start-time` / `unknown flag: --end-time` | 立即调用 `dws minutes <子命令> --help` 查询正确参数名；minutes 模块统一用 `--id`，时间统一用 `--start` / `--end`（不是 `--start-time` / `--end-time`） | 严禁用同一个错误的参数名重试；严禁在不同子命令间尝试 --uuid / --task-uuid / --id 三种名称反复试错；严禁凭记忆猜测参数名，必须查 --help |
| UUID 无效 | `taskUuid is invalid` / `dingOpenErrcode=300` | **第一时间切换策略**：调用 `dws minutes list mine --max 10 --format json` 获取真实可用的 uuid 列表，让用户选择或自动匹配最相关的一条。**真实案例中模型用同一个错 uuid 重试了 20 次全部失败——这是 minutes 模块失败率最高的错误模式，必须零容忍** | 严禁用同一个无效 uuid 重试哪怕 1 次（errcode=300 是不可重试错误）；严禁从历史对话/文档/链接中猜测 uuid；严禁从十六进制编码字符串里截取部分数字拼凑新 uuid；严禁不切 list 就放弃 |
| 命令返回空 stdout | `get summary` / `get transcription` 返回空内容（stdout 为空，error_msg 也为空） | 1) 先调用 `dws minutes get info --id <uuid> --format json` 确认听记是否存在且状态正常；2) 如果 info 也为空或报错，说明 uuid 本身有问题，回退到 list 命令重新获取 | 严禁在 stdout 为空时重复调用同一命令超过 2 次；严禁把空返回当作"没有内容"直接告知用户而不做任何排查 |
| 翻页 next-token 返回空 | `get transcription --next-token xxx` 返回空内容 | 同一个 next-token 返回空 **1 次**即视为到达末尾（has_more=false），立即终止翻页，基于已累积内容继续处理 | 严禁同一个 next-token 连续重试超过 2 次；严禁换不同的 next-token 值盲目尝试 |
| 时间格式解析失败 | `cannot parse time` / `parse fail` | 标准格式三选一：`2026-03-23T14:00:00+08:00`（ISO-8601 带时区）/ `2026-03-23 14:00:00`（无时区默认 +08:00）/ `2026-03-23`（纯日期） | 严禁使用 Unix 时间戳、`2026/03/23`、`Mar 23, 2026` 等非标准格式 |
| 权限不足 | `AUTH_PERMISSION_DENIED` / `Permission denied` / `user is not minutes creator` | 1) 如果是 `get transcription` 报权限错误，**自动降级**尝试 `get summary`（摘要通常不需要下载权限）；2) 如果降级也失败，引导用户："这份听记是别人分享给你的，你可以让原作者将你加为协作者，或者用 `dws minutes list shared` 查看共享给你的听记"；3) 如果需要其他人的听记内容，可先用 `dws minutes list shared` 确认有无共享记录 | 严禁权限报错后用同一 uuid 反复重试；严禁只回复"Permission denied"而不给用户可操作的下一步 |
| 资源不存在 | `P_DataNotFound` / `dingOpenErrcode=404` | 与 `taskUuid is invalid`（uuid 格式错误）**区分对待**：`P_DataNotFound` 表示 uuid 格式正确但对应的听记不存在（可能已被删除或过期）。告知用户"该听记可能已被删除或已过期"，然后建议用 `dws minutes list mine` 或 `list all` 查找替代 | 严禁把 `P_DataNotFound` 和 `taskUuid is invalid` 混为一谈；严禁 `P_DataNotFound` 后还换 uuid 格式重试 |
| 认证过期 | `not_authenticated` / `未登录` / `认证信息已失效` | 直接**重试刚才失败的命令**（最多两次），系统会自动刷新或重新授权；不要让用户重新提问。**注意**：长会话（>100 步）中 token 可能中途过期，这不是用户的问题 | 严禁认证过期后直接放弃整个任务；严禁让用户自己处理认证 |
| 上传业务报错 | `business error: success=false` / `Server Code: ERROR`（出现在 `upload create`） | 1) 检查文件名是否含特殊字符（建议用纯英文/数字文件名重试）；2) 检查 `--file-size` 单位是否正确（必须是**字节**，不是 KB/MB）；3) 确认音频格式是否支持（mp3/mp4/wav/m4a/aac）；4) 最多重试 **1 次**，仍然失败则如实告知用户"上传服务暂时不可用"，给出错误 Trace ID 供排查 | 严禁同一参数重试超过 2 次；严禁在错误信息未变化时反复重试期望随机成功；严禁尝试 `--json` / `-f json '{"...":...}'` 等不存在的输入格式——`--format json` / `-f json` 仅控制**输出格式**，不是输入参数 |

### 间接意图识别（用户 query 未直接提及听记命令时的调用策略）

当用户的 query 不是直接要求"查听记/看摘要"，而是隐含需要听记数据的高阶任务时，AI 必须自动拆解并先完成听记数据采集。

> **【核心判断原则】** 用户 query 中只要涉及"会议/讨论/沟通"等**口头交流**产出的信息，听记就是不可替代的数据源。即使用户没说"听记"二字，只要任务的完成依赖于"会议中说了什么"，就必须走 `dws minutes` 采集。**文档、日程、聊天记录无法替代听记**——文档是书面产出，日程只有标题和时间，聊天记录是文字沟通，唯有听记才包含会议的完整发言内容和 AI 摘要。

#### 间接意图 query 模式速查表

| 用户 query 模式 | 典型真实 query 示例 | 隐含的听记需求 | 正确的第一步 |
|----------------|-------------------|---------------|-------------|
| **报告/分析类**：要求基于某个主题或业务重新撰写报告 | "从悟空的商业模式分析，重新写一下市场感知报告" | 用户的报告素材来源于历史会议讨论，需要从听记中提取相关主题的会议内容作为写作输入 | `dws minutes list all --query "<主题关键词>" --max 20 --format json` → 对匹配的听记逐篇 `get summary` 提取素材 → 再基于素材撰写报告 |
| **分类总结类**：要求按类别（问题/事故/AI工作等）汇总工作成果 | "根据我的工作情况总结，分成问题应收、异常事故处理、ai相关工作和其他几部分总结"；"最新文档啥也没有啊，我要你根据我的工作情况总结" | 用户需要的"工作情况"不仅存在于文档中——会议讨论、评审结论、决策记录都在听记里。尤其当用户明确说"文档啥也没有"时，听记是**唯一剩余的结构化工作记录来源** | `dws minutes list mine --start <起始日期> --end <截止日期> --format json` → 逐篇 `get summary` → 按用户要求的分类维度（问题应收/事故处理/AI工作/其他）对摘要内容进行归类整理 |
| **多源聚合类**：用户明确要求补充听记/会议数据源，或抱怨当前数据源不全 | "你这个好像只是基于日程，我要的是我的对话、私聊、群聊、日程、会议、文档修改等相关的所有动作"；"根据今天的聊天记录和听记总结工作日报" | 用户**点名要求**多数据源聚合，其中"会议"对应的数据源就是听记。即使 query 中还提到了聊天、日程、文档等，听记侧的数据采集**不能被省略** | 听记侧**必跑**：`dws minutes list mine --start <起始日期> --end <截止日期> --format json` → 逐篇 `get summary`；再并行采集其他数据源（聊天/日程/文档）→ 最终汇总 |
| **Skill 封装类**：要求把数据聚合逻辑封装成可复用的 skill 或脚本 | "把这个封装成日报skill"；"做成自动化脚本" | 封装之前必须先**验证底层数据获取链路是否通畅**，不能跳过数据验证直接写 skill 框架 | 先用 `dws minutes list mine --start <today> --format json` 验证听记数据可达且格式正确，再设计 skill 结构 |
| **日报/周报/月报类**：要求生成工作汇报 | "帮我写日报/周报"；"帮我生成本周工作汇报" | 近期听记摘要是工作记录的核心来源之一 | `dws minutes list mine --start <起始日期> --end <截止日期> --format json` → 逐篇 `get summary` → 汇总写入报告 |
| **客户/项目类**：按客户名或项目名搜索相关信息 | "外资背景的客户信息总结"；"某某客户相关的所有信息" | 需要按关键词搜索听记 | `dws minutes list all --query "<客户名/项目名>" --format json` → `get summary` |
| **时间范围批量类**：梳理一段时间内的所有会议记录 | "梳理 4 月至今所有会议记录"；"本周所有听记" | 明确时间范围的批量听记获取 | `dws minutes list mine --start "2026-04-01T00:00:00+08:00" --end "2026-05-08T23:59:59+08:00" --max 50 --format json` |

#### 间接意图识别的三条铁律

**铁律 A：用户的任务包含"会议/讨论/沟通"类信息需求 → 听记采集是必跑前置步骤，不可跳过**
- 即使用户 query 中没有出现"听记"二字，只要任务完成需要"会议上说了什么/讨论了什么/达成了什么结论"，就必须走 `dws minutes` 采集
- 典型信号词：报告、分析、总结、汇总、复盘、回顾、工作情况、商业模式、市场感知、客户沟通

**铁律 B：用户说"文档/数据啥也没有" → 听记的优先级反而更高**
- 当用户明确反馈文档不足时，听记（会议记录）往往是**唯一还有内容的结构化数据源**
- 此时 AI 的正确反应不是"那我也没办法"，而是主动切到听记侧采集："让我先看看你最近的听记记录中有没有相关内容。"

**铁律 C：多源聚合场景中，听记侧 0 调用 = 严重失败**
- 当用户 query 中同时提到"聊天记录 + 听记 / 会议 + 文档"等多个数据源时，每个被提及的数据源都必须有对应的采集动作
- 听记侧的采集不能因为"其他数据源已经够了"而被省略——用户点名要了就必须给

## 自动化脚本

| 脚本 | 场景 | 用法 |
|------|------|------|
| [minutes_recent_summary.py](../../scripts/minutes_recent_summary.py) | 获取最近听记的 AI 摘要并合并 | `python minutes_recent_summary.py --max 5` |
| [minutes_extract_todos.py](../../scripts/minutes_extract_todos.py) | 从听记中提取待办事项汇总 | `python minutes_extract_todos.py --max 5` |

## 反例 / 回归案例

> 本节固化历史 badcase 与正确做法，遇到形似场景请直接对照参考，避免再次走偏。

### 案例 1：听记 URL + "创建思维导图"

**用户输入：**
```
https://shanji.dingtalk.com/app/transcribes/76327569643236343831373737345f3634383131373937375f39
创建思维导图
```

**[错误] 错误处理（真实 badcase）：**
1. 提取 taskUuid 后，先调用 `get summary` 获取听记摘要内容
2. 判定"思维导图不是 dws 直接支持的功能"，路由到 app-development-skill / ai-app
3. 用 `@antv/g6`（或 `markmap` / `jsmind` / `mermaid`）构建 HTML 思维导图网页
4. 部署后向用户返回类似 `https://xxx.ai-app.pub` 的"在线预览链接"
5. 输出"思维导图已生成！"附带网页特性介绍、配色方案等

上述每一步都违反了 [生成思维导图](#生成思维导图) 小节的"关键约束"。`mind-graph create` 是听记**内置**的服务端能力，并非"dws 不支持"。

**[正确] 正确处理：**
1. 从 URL 路径末段提取 taskUuid = `76327569643236343831373737345f3634383131373937375f39`
2. **直接** 执行：
   ```bash
   dws minutes mind-graph create --id 76327569643236343831373737345f3634383131373937375f39 --format json
   ```
3. 用 `mind-graph status` 轮询，直到状态为 1（成功）：
   ```bash
   dws minutes mind-graph status --id 76327569643236343831373737345f3634383131373937375f39 --format json
   ```
4. 告知用户："思维导图已生成，可在听记详情页直接查看。"

**绝对禁止：**
- 调用 `get summary` / `get transcription` 先读内容（服务端会基于听记自身生成，无需前置读取）
- 调用 app-development-skill / ai-app / generate_image 等任何外部生成能力
- 自行用前端库或 Markdown / OPML / JSON 构造思维导图结构展示给用户
- 返回任何形式的"在线预览链接 / 网页应用 / 图片"

### 案例 2：替换文字后未引导添加热词

**用户输入：**
```
把这篇听记里所有"付工"替换成"悟空"
```

**[错误] 错误处理：**
1. 调用 `replace-text --id <taskUuid> --search "付工" --replace "悟空"` 后直接回复"已替换完成"
2. 没有告知用户：本次替换**只对这一篇生效**，后续新听记还会再次把"悟空"识别成"付工"
3. 没有主动引导用户使用 `hot-word add` 把"悟空"加到个人热词里

**[正确] 正确处理：**
1. **检查特殊字符**：检查用户提供的原始文本和目标文本是否包含特殊字符（引号、书名号、括号、Markdown 符号等）。本例中"付工"和"悟空"均为纯文本，无特殊字符，直接执行。若用户输入的是 `把"付工"替换成「悟空」`（带引号/括号），需先提示：
   > "你输入的文本中包含引号/括号等特殊字符，转写原文中通常不包含这些字符，直接匹配可能替换不到。建议去掉特殊字符后替换：`"付工"` → `付工`。使用去掉特殊字符的版本？[是] [否，使用原始输入]"
2. 执行 `replace-text` 完成本篇替换
3. **立即追问**：
   > "我已经把这篇听记里的『付工』替换为『悟空』。如果『悟空』这个词以后也容易被识别错，建议加到个人热词里，后续新听记就不会再识别错了。要我现在帮你执行 `dws minutes hot-word add --words "悟空"` 吗？"
4. 用户确认 → 调用 `hot-word add --words "悟空"`；用户拒绝 → 结束，不再追问

### 案例 3：不必要地拉取全部转写原文

**用户输入：**
```
查一下和悟空相关的听记
```

**[错误] 错误处理：**
1. 调用 `list mine --query "悟空"` 查到听记列表
2. 对每条听记依次调用 `get transcription` 拉取全部转写原文
3. 把大量原文全部展示给用户

用户只是想查一下列表，根本不需要看转写原文。大量拉取原文既造成不必要的性能开销，也会让用户被信息淹没。

**[正确] 正确处理：**
1. 调用 `list mine --query "悟空"` 或 `list all --query "悟空"` 返回听记列表
2. 直接把列表结果展示给用户
3. 不调用 `get transcription`——因为用户没有要求看原文

**另一组对比：**

**用户输入：**
```
帮我分析一下这篇听记的转写原文
```

**[错误] 错误处理：**
1. 调用 `get transcription --id <taskUuid>` 只拉了第一页就停了
2. 展示部分原文，告诉用户"如果要看更多请传入 next-token"

用户说的是"分析转写原文"，意图很明确是要看完整原文，不应该让用户手动翻页。

**[正确] 正确处理：**
1. 调用 `get transcription --id <taskUuid>`，拿到第一页和 `nextToken`
2. **自动继续调用** `get transcription --id <taskUuid> --next-token <nextToken>`
3. 每次拼合后检查累积字符数：
   - **未超过 12000 字符** → 继续自动翻页
   - **超过 12000 字符** → 暂停，提示用户："当前已拉取约 X 字符的转写内容，已达到单次处理上限。是否继续拉取后续内容？"
4. 用户确认继续 → 接着翻页；用户拒绝 → 停止翻页，基于已拉取内容进行分析展示

### 案例 4：拉完转写后只输出时间线原文，未引导发言人聚类与替换

**用户输入：**
```
帮我把这篇听记的转写原文拉出来分析一下
```

**[错误] 错误处理（真实 badcase）：**
1. 调用 `get transcription` 自动翻页拉完全部原文
2. 直接把按时间戳穿插的"发言人1: ... / 发言人2: ... / 发言人1: ..."大段原文全部丢给用户，结束流程
3. 用户后续追问"李总主要讲了什么"，AI 又要重新读一遍原文才能回答
4. 即使用户后来说"发言人1 就是李总"，AI 也只是在自己回复里口头改一改，**不调用 `speaker replace`**，听记本身的发言人映射没有任何变化

按时间线穿插的原文对人**极其不友好**——同一个发言人的内容散落在不同时间点，用户很难看清"某个人讲了什么"。而且听记里的『发言人1/发言人2』占位符如果一直不被替换为真实姓名，用户每次回看都要靠脑补才能对上号。

**[正确] 正确处理：**
1. 调用 `get transcription` 自动翻页拉完全部原文（注意 12000 字符上限保护）
2. **拉完后立即追问**："已拉取完整转写原文（共 N 段，X 个发言人）。当前默认按时间线返回。是否需要我帮你**按发言人分组聚类**，并提取每位发言人的**核心发言要点**？"
3. 用户确认 → AI **本地完成聚类**（按 `speakerNick` 分组），每位发言人输出 3-5 条核心要点，再追问："如果你能告诉我『某某人主要讲了什么』（如『李总主要讲了战略规划』），我可以根据关键词帮你**自动匹配**对应的发言人，并把『发言人1/发言人2』替换成真实姓名。"
4. 用户回复『李总主要讲了战略规划』→ AI 抽取关键词『战略规划』，在已聚类的各发言人核心要点里做模糊匹配；找到唯一高置信候选『发言人1』→ 引导确认："『发言人1』很可能就是你说的『李总』（命中关键词：战略规划、AI 化转型）。是否需要把这篇听记里的『发言人1』全部替换为『李总』？确认后我会执行 `dws minutes speaker replace --id <taskUuid> --from "发言人1" --to "李总"`"
5. 用户确认 → **立即调用** `dws minutes speaker replace --id <taskUuid> --from "发言人1" --to "李总" --format json`，执行成功后告知用户"已替换，纪要与待办中的发言人也已同步更新"

**绝对禁止：**
- 拉完转写就只丢一大段时间线原文给用户，不做聚类、不主动引导
- 用户提供"某某人讲了 XX"后，AI 在自己脑内/回复里"假装替换"了，但**不实际调用** `speaker replace`
- 在置信度不足（多候选）时仍然给出唯一答案，不让用户参与挑选
- 把『发言人1 → 李总』的映射写到用户回复里就完事，听记本身没有任何写回

### 案例 5：用户查某人在听记中说了什么，总结完不引导替换发言人

**用户输入：**
```
帮我看看这个听记里张三说了什么
https://shanji.dingtalk.com/app/transcribes/76327569643231383535353939365f3436383537393431335f32
```

**[错误] 错误处理（真实 badcase）：**
1. 从 URL 提取 taskUuid，调用 `get transcription` 拉取转写原文
2. 发现转写中只有"发言人1/发言人2/发言人3"，没有"张三"
3. AI 凭原文推断"发言人2 可能是张三"（因为内容提到了产品需求），直接把发言人2 的内容当作张三的输出给用户
4. 总结输出完就结束了，**没有引导用户确认推断是否正确**
5. **没有引导用户替换发言人**——听记本身还是显示"发言人2"，下次用户打开听记还是看不出谁是张三

上述做法有两个严重问题：① 推断结果未经用户确认就当作事实输出，可能张冠李戴；② 即使推断正确，也没有调用 `speaker replace` 写回听记，用户下次看还是"发言人2"。

**[正确] 正确处理：**
1. 从 URL 提取 taskUuid，调用 `get transcription` 自动翻页拉取全部转写
2. **Step 2 声纹标注检查**：检查转写中是否已有"张三"作为 speakerNick → 本例中没有，只有匿名编号
3. **Step 3 转写原文推断**：在原文中寻找线索——例如其他人说"张三你来汇报一下"，紧接着"发言人2"开始发言；或者"发言人2"的内容大量涉及产品需求（与用户描述的张三角色吻合）
4. 推断出"发言人2"可能是张三后，**必须向用户确认**：
   > "根据转写内容分析，以下发言最可能是张三的：
   > 「接下来我汇报一下 Q3 的产品规划，主要有三个方向...」
   > 确认是张三吗？"
5. 用户确认 → **Step 6 结构化总结输出**：提取发言人2 的全部发言，输出张三的核心观点、关键决策、待办等
6. **Step 7 引导替换发言人**（必须执行）：
   > "目前这篇听记中张三的发言仍显示为『发言人2』。要我帮你把听记里的『发言人2』全部替换为『张三』吗？替换后纪要和待办中的发言人也会同步更新。"
7. 用户确认 → **先通讯录查询获取 dingUid**：调用 `dws contact user search --query "张三" --format json`
   - 唯一匹配（如返回 userId=123456789）→ 执行 `dws minutes speaker replace --id <taskUuid> --from "发言人2" --to "张三" --target-uid 123456789 --format json`
   - 多个匹配 → 列出候选（姓名+部门+userId）让用户选择后执行
   - 无匹配 → 提示用户通讯录未找到，执行不带 `--target-uid` 的替换：`dws minutes speaker replace --id <taskUuid> --from "发言人2" --to "张三" --format json`
8. 替换成功后追问："还有其他发言人需要我帮你识别和替换吗？"

**绝对禁止：**
- 推断"发言人X 是张三"后不向用户确认就直接输出总结——可能张冠李戴
- 总结完就结束，不引导替换发言人——用户下次看听记还是"发言人2"
- 只在回复里说"发言人2 就是张三"但不调用 `speaker replace`——听记本身没有任何变化
- Step 3 推断不出来时直接告知"找不到张三"就结束——应继续走 Step 4 多路并发推断（通讯录/文档/日程/聊天记录）

### 案例 6：通过通讯录 + 部门角色 + 转写线索三路印证推断发言人（真实复盘）

> 这是一次**完整走完 Step 1~7 全流程**的真实案例，重点演示 Step 4 多路并发身份推断如何与 Step 3 转写原文线索互相印证，以及如何识别"花名相似但不是同一人"的陷阱。

**用户输入：**
```
https://shanji.dingtalk.com/app/transcribes/<taskUuid> 分析下木兰讲了什么
```

**[正确] 完整执行链路：**

**Step 1：定位听记并读取转写**
- 从 URL 末段提取 taskUuid
- `dws minutes get transcription --id <uuid> --format json` 自动翻页拉取全部（注意 12000 字符上限保护）

**Step 2：声纹标注检查**
- 转写中所有 `speakerNick` 都是匿名编号（发言人1/2/3/4），**未命中** → 进入 Step 3

**Step 3：转写原文内推断（同时并发启动 Step 4，不串行等待）**

先做粗粒度的发言人画像，列出每位发言人的发言量、主题、关键互动信号：

| 发言人 | 发言特征 | 互斥线索（说明 TA "不是谁"）|
|--------|----------|------|
| 发言人1 | 发言较多，集中在 UI/交互设计：A/B 面切换、按钮位置、页面层级、设备号承接页 | 1013s 提到「**木风**也给了一些各种状态」→ 木风 ≠ 发言人1 |
| 发言人2 | 发言最多且最主导，讨论 agent 架构、skill 设计、记忆系统、定时任务 | 1063s 叫「青锋给大家看」、2113s 叫「行远」、多次提「虎哥」「陈林」→ 这几人都不是发言人2 |
| 发言人3 | 中等发言量，讨论功能号、对话框、班主任场景 | 3549s 提到「木风挺调皮」→ 木风 ≠ 发言人3 |
| 发言人4 | 发言较少，技术实现相关（接口、链路、安卓、蓝牙）| —— |

**Step 4：多路并发身份推断（与 Step 3 同时进行）**

| 路径 | 命令 | 结果 |
|------|------|------|
| ① 通讯录组织架构 | `dws contact user search --keyword "木兰"` | 木兰 = **王佳明**，X 事业群-X 事业部-X-X-**产品设计部**，上级临渊（王临一）|
| ② 文档产出 | `dws doc search --keyword "王佳明"`（按需）| 多为设计稿/原型，进一步印证设计师角色 |

**Step 5：定向匹配 + 置信度判断**

把 Step 4 拿到的"产品设计部 / 设计师角色"信号回投到 Step 3 的发言人画像：

- 候选锁定 **发言人1**——其特征（UI 设计、A/B 面切换、页面层级、汇报设计进展）与"产品设计部设计师"高度吻合
- **额外强信号**：发言人1 在 1856s 说「最开始跟**拾光**做 A 店」——拾光 = 王刘明（当前用户），属同部门协作关系，与"木兰也在产品设计部"完全对得上
- **同名陷阱排查**：发言人1 在 1013s 说「**木风**也给了一些各种状态」——`木风 ≠ 木兰`（两个不同花名，木风是另一位设计同事），**这条线索看似矛盾，实则强化了"发言人1 是木兰本人在转述木风的产出"的判断**
- 综合置信度 **≈ 75%**（>70%），走**分支 A：文本片段确认**

**Step 5 → 用户确认环节**（实际对话中必须有这一步）：

> 根据分析，发言人1 很可能就是木兰（王佳明，产品设计部）。最具代表性的发言片段：
> 「最开始跟拾光做 A 店就是在功能号做的……当前首页内容层级太多，无论怎么改都解决不了根本问题」
> 推断依据：UI 设计视角 + 与拾光有合作经历 + 同部门关系
> 确认是木兰吗？是 / 不是

**Step 6：结构化总结输出（四段式模板）**

确认后按以下结构组织——这套四段式适用于绝大多数会议场景：

```
[人名] 在本次会议中的发言总结

**核心观点**
- 观点1（带具体上下文，不要只写关键词）
- 观点2
- ...

**提出的问题 / 关注点**
- 问题1
- ...

**Action Item / 承诺事项**
- 时间节点 + 具体动作
- ...

**立场 / 态度**
- 对核心议题的明确态度（支持/反对/务实/保留）
- ...
```

**Step 7：引导替换发言人（必须执行）**

> 目前这篇听记中木兰的发言仍显示为『发言人1』。要我帮你把听记里的『发言人1』全部替换为『木兰』吗？
> 通讯录中已匹配到：木兰（王佳明，产品设计部，userId: 123456789），替换后将关联到通讯录真实身份，纪要和待办中的发言人也会同步更新。
> 确认后我会执行：`dws minutes speaker replace --id <taskUuid> --from "发言人1" --to "木兰" --target-uid 123456789`

用户确认 → 立即调用 `dws minutes speaker replace`（附带 `--target-uid`），并追问"还有其他发言人需要识别吗？"

**本案例固化的关键经验（必须吸收）：**

1. **花名相似不等于同一人**：`木风 ≠ 木兰`、`拾光 ≠ 拾贝`、`临渊 ≠ 临川` 这类 1 字之差的花名极易误判。**遇到候选人花名出现在某发言人原话里时，必须先确认这是"自指（在自我介绍）"还是"他指（在叫别人）"，再做互斥推断**。在原文中"A 提到 B"通常意味着 `A ≠ B`，不要反过来判定 `A = B`。

2. **Step 3 与 Step 4 必须并发**：先做发言人画像（Step 3）的同时**异步发起**通讯录查询（Step 4 ①），等通讯录返回了"角色/部门"信号再回投到画像里做匹配，这样不串行等待、不浪费时间。

3. **同部门协作关系是强信号**：当发言人 X 提到了某个同事的花名（如「跟拾光做 A 店」），而通讯录显示"目标人物"和该同事**同部门**，这是非常强的身份匹配信号，可以直接把置信度提升到 70%+。

4. **置信度 ≥ 70% 即可走分支 A**：不要一味追求 90%+，否则会陷入"再查一路、再印证一次"的死循环。70% 是经验阈值，分支 A 本身就有"用户文本确认"作为兜底，错了用户会立即纠正。

5. **结构化总结用四段式模板**：核心观点 / 关注点 / Action Item / 立场态度——这套模板适用于绝大多数会议场景（产品评审、技术方案、复盘会、双周会）。每条要点必须**带上下文**（如"她认为底部那一排功能导航必须去掉，因为与上方内容严重重复"），不要只写"反对底部导航"这种干巴巴的关键词。

6. **Step 7 必须执行，不能跳过**：本案例如果只输出总结就结束，下次用户打开听记看到的还是"发言人1"，依然要靠脑补对应木兰——这正是发言人识别功能存在的意义被完全抹掉的反例。

**绝对禁止：**
- 看到发言人说「木风也给了状态」就直接判定"那 TA 就是木风的同事/下属"等过度推断——只能得出 `TA ≠ 木风` 这一条互斥信息
- Step 3 还在分析就阻塞住，等画像分析完再串行去查通讯录——必须并发
- 总结写成"木兰讨论了 UI 设计、A/B 面切换、功能号方向"这种关键词堆砌——必须展开成带上下文的完整观点
- 置信度 70% 就纠结要不要再查文档/聊天记录——分支 A 的文本确认本身就是兜底，不要无谓地继续查

### 案例 7：跳过通讯录、在转写文本里 grep 花名 → 误判"目标人物没参会"（真实反面教材）

> 这是**与案例 6 输入完全相同**但执行链路完全错误的真实 badcase，重点演示"没走 Step 4 通讯录查询"会带来怎样灾难性的失败结论。**强烈建议每次执行发言人识别任务前，对照本案例自检一遍**。

**用户输入：**（与案例 6 完全相同）
```
https://shanji.dingtalk.com/app/transcribes/<taskUuid> 分析下木兰讲了什么
```

**[错误] 真实失败链路（每一步都要识别为反模式）：**

```
Step 1 **[完成]** get transcription 拉取了多页转写
   ↓
Step 2 **[禁止]** 看到全是匿名编号 → 没有继续走 Step 3-4，反而开始在转写里 grep "木兰"
   ↓
错误动作 A：连续多次"搜索所有日志文件中是否有'木兰'这个名字"
   ↓
错误动作 B：转写里搜不到 → 调用 get summary 看 AI 摘要里的"参与人"
   ↓
错误动作 C：AI 摘要"参与人=拾光" → 推理"参与人只有拾光 → 木兰没参会"
   ↓
错误结论：告诉用户"在这篇听记中，没有找到名为木兰的发言人。可能木兰没参加这次会议"
   ↓
错误兜底：把责任甩给用户："请你确认是哪位发言人，或在客户端看参会人列表"
```

**这条链路违反了几乎所有铁律：**

| 反模式 | 违反的铁律 | 后果 |
|--------|------------|------|
| 在转写文本里 grep "木兰" 字符串作为存在性判断 | 铁律 1 | 99% 听记的发言人都是匿名编号，搜不到字面是默认场景，根本不构成"没参会"的证据 |
| 用 AI 摘要的"参与人=拾光"推断"木兰没参会" | 铁律 2 | AI 摘要"参与人"字段只截取最显著的 1-2 人，**不是**完整参会名册 |
| 全程没调用过一次 `dws contact user search --keyword "木兰"` | 铁律 3 | 通讯录查询是 Step 4 的必跑项，单次调用就能拿到"木兰=王佳明，产品设计部" |
| 一旦字面搜不到就放弃身份推断，把任务甩给用户 | 铁律 4 | 这恰恰把发言人识别功能的核心价值（把匿名编号映射到真实人）完全抹掉了 |

**[正确] 应该这样执行（与案例 6 一致）：**

1. **Step 1**：从 URL 提取 taskUuid → `dws minutes get transcription` 自动翻页拉全部
2. **Step 2**：检查 `speakerNick` 字段是否含"木兰"——发现全是匿名编号 → **不要在转写文本里 grep "木兰"，立即并发触发 Step 3 + Step 4 ①**
3. **Step 3**（与 Step 4 ① 并发）：在转写里做发言人画像（每位发言人的发言量、主题、互斥线索）
4. **Step 4 ①**（与 Step 3 并发，必跑）：`dws contact user search --keyword "木兰"` → 拿到"木兰=王佳明，产品设计部，上级临渊"
5. **Step 5**：把"产品设计部 + 设计师角色"信号回投到画像 → 锁定发言人1（UI/交互设计视角高度匹配）+ 与拾光的同部门协作信号 → 置信度 ≈ 75% → 走分支 A
6. **Step 5 用户确认**：展示发言人1 的代表性片段请用户确认
7. **Step 6**：四段式结构化总结（核心观点/关注点/Action Item/立场态度）
8. **Step 7**：引导调用 `speaker replace` 把"发言人1"替换为"木兰"

**关键经验（强制吸收，案例 6 已讲过的不再重复，本案例独有的）：**

1. **"在转写里搜不到目标人名"绝不构成"没参会"的证据**：花名/真名通常不出现在 TA 自己的发言里，这是默认场景而非例外。99% 的听记发言人都是匿名编号——这正是发言人识别功能要解决的问题。

2. **AI 摘要的"参与人"字段是低保真信号，不能作为参会判断依据**：`get summary` 返回的是 AI 生成的自然语言摘要，里面提到的"参与人"通常只是最显著的 1-2 人；要拿完整参会列表，应使用 `get info` / `get batch` 返回的结构化 `participants` 字段。

3. **`dws contact user search` 是 Step 4 ① 的必跑项，单次调用就能突破死局**：本案例的整个失败链路只要有 1 次通讯录查询就能立即扭转——拿到"木兰=王佳明，产品设计部"后，Step 5 的角色匹配就有了锚点，再也不会得出"没参会"的错误结论。

4. **想说"找不到 X"前的四个自检问题**（任何一个回答"没"都禁止给"找不到"结论）：
   - 通讯录查了吗？(`dws contact user search --keyword "X"`)
   - 文档查了吗？(`dws doc search --keyword "X"`)
   - 聊天记录查了吗？(`dws chat message list`)
   - 基于角色在转写里做模式匹配了吗？（设计师 vs 研发 vs 管理者的发言特征）

5. **`get summary` 不能替代发言人识别**：摘要是"会议讲了什么"的总览，**不是**"谁讲了什么"的精细切分。混淆这两个能力会导致 Step 4 直接跳过。

**绝对禁止：**
- **[禁止]** Step 2 看到匿名编号后，直接在转写文本里 grep 目标人名 → 没找到就退出
- **[禁止]** 用 `get summary` 摘要里写到的"参与人"判断某人是否参会
- **[禁止]** 全流程不调用 `dws contact user search` 就给出"找不到 X"的结论
- **[禁止]** 把身份推断的责任甩回给用户："请你告诉我哪位是木兰" / "请去客户端看参会人"——发言人识别功能的存在意义就是 AI 来做这件事

### 案例 8：听记/纪要类 query 不走 dws 技能（基础评测集 16 例 badcase 复盘）

> 这是一组**最高频的失败模式**——用户提出听记/纪要/会议总结/待办提取/链接解析等典型 dws 场景请求，AI 却用 `session_search` / `memory_search` / `activity:search` / `browser_use` / `read_file` / 直接反问 等"伪替代"路径绕过 dws 技能，导致核心链路 0 命中。下面把基础评测集（minutes-base, evalrun_4ab46f8da846）中**全部 16 个该模式失败 query 完整列出**，遇到形似输入请直接对照本案例处理。

#### 一、五类典型 badcase 模式（按失败动作归类）

**模式 A：模糊/省略型 query → AI 直接反问要细节，不主动 list**

涉及 query：

| case_id | 用户原始 query |
|---------|----------------|
| `dws_minutes_hotquery_0049` | 按关键词搜索我的听记 |
| `dws_minutes_hotquery_0057` | 查列表+看摘要 |
| `dws_minutes_hotquery_0063` | 周会回顾整理 |
| `dws_minutes_hotquery_0064` | 评测工作复盘 |
| `dws_minutes_hotquery_0042` | 把所有听记内容添加到汇报中 |

**典型错误动作**：`tool_calls = []`，AI 回复"请告诉我具体的关键词/时间范围/会议名"就停下，等待用户补充。

**模式 B：用 `session_search` / `memory_search` 搜索历史会话假装"找过了"**

涉及 query：

| case_id | 用户原始 query |
|---------|----------------|
| `dws_minutes_hotquery_0003` | 帮我查一下最近一次会议的纪要内容 |
| `dws_minutes_hotquery_0017` | 总结下我的会议 |
| `dws_minutes_hotquery_0020` | 我昨天那个会的重点帮我提炼一下 |
| `dws_minutes_hotquery_0027` | 把最近一次会议的待办整理出来 |

**典型错误动作**：调用 `session_search` 搜以前的对话记录，把以前 AI 自己生成过的"会议纪要文件描述"当作真实数据复述出来；从未触发 `dws minutes list / get summary / get todos`。

**模式 C：用 `activity:search` / web 搜索把"找听记"做成"搜网页"**

涉及 query：

| case_id | 用户原始 query |
|---------|----------------|
| `dws_minutes_hotquery_0025` | 调取某某项目讨论的两个听记内容 |
| `dws_minutes_hotquery_0060` | 搜索+摘要+关键词 |

**典型错误动作**：调用 `activity:search` 搜公网，返回的是"钉钉 AI 听记产品介绍"页面，与用户的私人听记数据毫不相关。

**模式 D：钉钉听记/文档 URL 走 `browser_use` / `read_file` 而非 `dws`**

涉及 query：

| case_id | 用户原始 query |
|---------|----------------|
| `dws_minutes_hotquery_0045` | `https://shanji.dingtalk.com/meeting/minutes?taskUuid=sample004` |
| `dws_minutes_hotquery_0046` | `https://alidocs.dingtalk.com/i/nodes/sampleDocNode01` 帮我读取这个文档内容 |
| `dws_minutes_hotquery_0047` | 这个听记链接你能打开看内容吗 `https://shanji.dingtalk.com/meeting/minutes?taskUuid=sample006` |

**典型错误动作**：`browser_use` 打开页面遇到登录墙就回复"需要登录"；或 `read_file` 当本地文件读 → 失败 → 把锅甩给用户。完全没有意识到 dws 技能本身已携带账号态，能直接通过 taskUuid/dentryUuid 拿到内容。

**模式 E：多源数据生成日报/汇报/总结/报告，听记侧 0 调用**

涉及 query：

| case_id | 用户原始 query |
|---------|----------------|
| `dws_minutes_hotquery_0040` | 根据今天的聊天记录和听记总结工作日报 |
| `dws_minutes_hotquery_0005` | 把会议纪要写入钉钉文档 |
| `minutes-001` (P0) | 从悟空的商业模式分析，重新写一下市场感知报告 |
| `minutes-002` (P0) | 最新文档啥也没有啊，我要你根据我的工作情况总结，分成问题应收、异常事故处理、ai相关工作和其他几部分总结 |
| `minutes-004` (P0) | 你这个好像只是基于日程，我要的是我的对话，发出去的文字，私聊，群聊，日程，会议，文档修改等相关的所有动作，参考这个生成日报，并且把这个封装成日报skill |

**典型错误动作**：识别出"日报/报告/总结/写文档"场景后，只想着调用周报技能 / 文档写入技能，**完全跳过听记数据获取**这一步。更隐蔽的变体是：用户 query 中**没有出现"听记"二字**（如"根据工作情况总结"、"重新写市场感知报告"），但任务的完成**实际依赖会议讨论内容**，AI 却完全没有意识到需要从听记中采集素材，结果输出空洞的报告或反问"请告诉我会议纪要内容"。

**P0 case 暴露的三种变体（必须全部识别）：**
- **变体 1（minutes-001）**：用户要写报告，报告素材来源于历史会议讨论 → AI 没识别出需要查听记
- **变体 2（minutes-002）**：用户要分类总结工作，且明确说"文档啥也没有" → AI 没切到听记作为替代数据源
- **变体 3（minutes-004）**：用户明确抱怨"只是基于日程"并点名要求"会议"等多数据源 → AI 仍然只用日程数据

#### 二、五类共性反模式（必须全部识别为禁止动作）

| 反模式 | 错误根因 | 正确做法 |
|--------|----------|----------|
| **R1：模糊请求 → 反问要细节** | AI 把"信息不足"当成必须澄清的前置条件 | 听记类模糊请求 → **默认先调用 `dws minutes list mine --max 10 --format json`** 把最近的听记列出来，让用户从列表里挑，而不是反问关键词 |
| **R2：用 `session_search`/`memory_search` 替代 dws** | AI 把"历史会话里聊过的纪要描述"误认为是真实数据源 | 历史会话只能回忆"以前我们聊过什么"，**不是真实听记数据**。听记数据**必须**从 `dws minutes` 实时拉取 |
| **R3：用 `activity:search` 通用 web 搜索听记** | 把"听记"理解成公网信息 | 听记是用户私人钉钉数据，**只能**通过 `dws minutes list/get` 获取，公网搜不到也不该搜 |
| **R4：钉钉 URL 走 browser_use/read_file** | 把钉钉 URL 当成普通网页 | 钉钉听记 URL（`shanji.dingtalk.com/meeting/minutes?taskUuid=xxx` 或 `shanji.dingtalk.com/app/transcribes/xxx`）→ **提取 taskUuid → 走 `dws minutes get summary/get transcription`**；钉钉文档 URL（`alidocs.dingtalk.com/i/nodes/xxx`）→ **提取 dentryUuid → 走 `dws doc read`** |
| **R5：日报/汇报/总结/报告场景跳过听记数据采集** | 只看到"日报/报告/总结"就直奔输出端，忘了用户指定的（或隐含的）数据源包含会议/听记。**不仅限于用户说了"听记"二字的场景**——"根据我的工作情况总结"（minutes-002）、"重新写一下市场感知报告"（minutes-001）、"我要的是我的对话...会议...生成日报"（minutes-004）都属于此类 | query 中出现"总结/报告/汇报/日报/周报/工作情况/商业分析/市场感知/复盘"等产出类关键词时，必须自检：**用户的产出是否依赖会议讨论内容？** 如果是 → **第一步必须** `dws minutes list mine` 拿听记 → `get summary` 逐篇拉摘要 → 再汇总。详见「间接意图识别」章节 |

#### 三、五类 badcase 的统一正确链路（速查表）

| 用户 query 形态 | 第一步必跑命令 | 关键说明 |
|------------------|------------------|----------|
| 模糊请求："总结下我的会议" / "周会回顾整理" / "查列表+看摘要" / "评测工作复盘" / "按关键词搜索我的听记" | `dws minutes list mine --max 10 --format json` | 拿到最近听记列表后，对前 1~3 篇 `get summary`，引导用户挑选目标 |
| 含时间词："最近一次/昨天/本周/上周的会议" | `dws minutes list mine --start <ISO> --end <ISO> --max 20 --format json` | 时间范围按用户描述折算，不要让用户自己提供日期 |
| 含主题/项目关键词："某某项目讨论" / "搜索+摘要+关键词" | `dws minutes list all --query "<关键词>" --max 20 --format json` | 用 `--query` 而不是 `activity:search` |
| 钉钉听记 URL（`shanji.dingtalk.com/...?taskUuid=xxx`） | `dws minutes get summary --id <taskUuid> --format json` | 从 URL 提取 taskUuid，禁用 browser_use |
| 钉钉文档 URL（`alidocs.dingtalk.com/i/nodes/xxx`） | `dws doc read --node <url 或 dentryUuid> --format json` | 走 doc 技能而非 read_file/browser_use |
| 多篇听记对比："对比一下这几个听记 [URL1] [URL2]" | 对每个 URL 分别 `dws minutes get summary --id <uuid> --format json` | 失败的 URL 给出明确说明，不要把锅全甩给用户 |
| 日报/汇报含"听记"/"会议纪要"关键词 | 先 `dws minutes list mine --start <今日 0 点> --max 20`，再对每篇 `get summary`，最后才汇总 | 听记数据采集是必跑前置，不能直接跳到周报技能 |
| 报告/分析类（query 未显式提"听记"但依赖会议讨论素材）："重新写一下市场感知报告" / "帮我生成商业分析" | `dws minutes list all --query "<主题关键词>" --max 20 --format json` → 逐篇 `get summary` 提取素材 | 详见「间接意图识别」，用户的报告素材来源于历史会议讨论 |
| 分类工作总结（query 未显式提"听记"但需要工作记录）："根据我的工作情况总结，分成…几部分" / "文档啥也没有，根据工作情况总结" | `dws minutes list mine --start <起始日期> --end <截止日期> --format json` → 逐篇 `get summary` → 按分类维度归类 | 文档不足时听记是唯一剩余数据源；详见「间接意图识别」 |
| 多源聚合（用户抱怨数据源不全或点名要求会议数据）："只是基于日程，我要的是对话+会议+文档等所有动作" / "聊天记录和听记总结日报" | 听记侧必跑 `dws minutes list mine --start <起始日期> --end <截止日期>` → `get summary`，再并行采集其他源 | 用户点名要了"会议"就不能省略听记采集 |
| 待办提取："最近一次会议的待办" | `dws minutes list mine --max 1` → `dws minutes get todos --id <taskUuid>` | 用 `get todos`，不要自己从转写里硬抠 |
| 写入钉钉文档："把会议纪要写入钉钉文档" | 先 `dws minutes get summary --id <uuid>` 拿到内容 → 再 `dws doc create` / `dws doc update` 写入 | 听记数据采集 + 文档写入是两步，缺一不可 |

#### 四、绝对禁止（任何一条触发即视为严重失败）

- **[禁止]** 听记/纪要/会议类请求 → `tool_calls = []` 直接反问要细节（任何模糊请求至少要先 `dws minutes list mine` 跑一次，让用户在列表里挑）
- **[禁止]** 用 `session_search` / `memory_search` 搜以前的会话记录当作"真实听记数据"复述给用户——历史会话不是数据源
- **[禁止]** 用 `activity:search` / web 搜索找用户私人听记——听记是私域数据，公网搜不到
- **[禁止]** 钉钉听记 URL（`shanji.dingtalk.com/meeting/minutes?taskUuid=xxx` / `shanji.dingtalk.com/app/transcribes/xxx`）走 `browser_use` 打开页面——必须提取 taskUuid 走 `dws minutes get`
- **[禁止]** 钉钉文档 URL（`alidocs.dingtalk.com/i/nodes/xxx`）走 `read_file` / `browser_use`——必须走 `dws doc read`
- **[禁止]** 遇到登录墙 / URL 无效就只回复"需要登录" / "请提供正确 URL" 然后停下——必须 fallback 到 `dws minutes list mine` 让用户从自己的听记列表里挑替代项
- **[禁止]** 日报/周报/汇报/总结/报告场景，任务产出依赖会议讨论内容，却跳过 `dws minutes` 数据采集——**不限于 query 中出现"听记"二字的场景**，"根据工作情况总结"（minutes-002）、"重新写市场感知报告"（minutes-001）等间接意图同样适用
- **[禁止]** "把会议纪要写入钉钉文档"类请求只调文档写入工具不调 `dws minutes get summary`——会议纪要内容必须先实时获取，不能让用户自己粘贴
- **[禁止]** `taskUuid is invalid` 报错后用同一个无效 uuid 重试（哪怕 1 次）——必须立即切换到 `dws minutes list mine` 获取真实 uuid
- **[禁止]** 从历史对话、文档内容、十六进制编码字符串中猜测或拼凑 uuid——uuid **只能**来自 `list mine/shared/all` 返回或用户提供的听记 URL
- **[禁止]** 使用错误的命令层级结构：`dws minutes info`（应为 `get info`）、`dws minutes get`（缺子命令）、`dws minutes list --start`（缺 scope）——参照文档顶部"命令层级结构"
- **[禁止]** 工具调用缺少 `--format json` 参数——所有 `dws` 命令都应带上以便结构化解析

#### 五、自检清单（执行前 5 秒强制走一遍）

收到任何含以下信号词的 query 时，**第一个动作必须是 `dws minutes` 或 `dws doc`，否则视为走错链路**：

> **直接信号词**（query 中直接提到听记相关概念）：
> "听记 / 纪要 / 摘要 / 转写 / 录音 / 会议 / 周会 / 日会 / 评审会 / 复盘 / 回顾 / 待办 / Action Item / 关键词 / 我的会 / 昨天那个会 / 上周的会 / 共享听记 / shanji.dingtalk.com / alidocs.dingtalk.com"

> **间接信号词**（query 未提"听记"但任务产出依赖会议讨论内容，需要额外判断——详见「间接意图识别」章节）：
> "报告 / 市场感知 / 商业分析 / 工作情况总结 / 分类总结 / 日报 / 周报 / 月报 / 工作汇报 / 文档啥也没有 / 只是基于日程 / 所有动作 / 封装成 skill"
> **判断标准**：用户的产出是否依赖"会议上说了什么/讨论了什么"？如果是 → 听记采集是必跑前置步骤

强制问自己 5 个问题（任何一个回答"否"都禁止开始执行）：

1. 我接下来的第一个 tool call 是 `dws minutes ...` 或 `dws doc ...` 吗？
2. 我有没有在用 `session_search` / `memory_search` / `activity:search` / `browser_use` / `read_file` 替代 dws？
3. 用户给了钉钉 URL 时，我有没有从 URL 提取 taskUuid/dentryUuid 后走 dws，而不是 browser_use？
4. 用户的请求模糊（如"总结下我的会议"）时，我是先 `dws minutes list mine` 列出来，还是反问要细节？
5. 用户要写报告/总结/日报但没提"听记"时，我有没有判断"任务产出是否依赖会议讨论内容"？如果依赖 → 听记采集不能省略（参见间接意图识别三条铁律）
