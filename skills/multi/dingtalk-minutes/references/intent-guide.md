# Minutes 低频意图与产品边界

> 返回入口：[DingTalk Minutes Skill](../SKILL.md) · [Reference 与脚本索引](minutes.md)

本文件只承接不在根 Skill Golden Route 展开的低频能力。命令参数或 Safety 不确定时读取对应 compact leaf Schema；不要因此加载 Minutes 全量 Catalog。

## 低频能力路由

| 用户意图 | 推荐入口 | 关键边界 |
|---|---|---|
| “把发言人1改成张三” | `dws minutes +speaker-replace --id <taskUuid> --from "发言人1" --to "张三" [--target-uid <UID>]` | 这是逐字稿里的昵称替换，不是 speaker_id 与用户身份的系统级重绑；先完整预检源昵称，按 Runtime confirmation 执行并读回验证 |
| “把这篇听记里的 A/B/C 批量替换” | `dws minutes +replace-batch --id <taskUuid> --pair "旧词=>新词" --failure-policy stop --page-limit 100 --dry-run --format json` | dry-run 只预览本地规则，不检查远端命中；真实执行按 Runtime confirmation，逐项验证并保留失败项 |
| “下载这条听记的音频/视频” | `dws minutes +download --id <taskUuid> --output <相对路径>` | 媒体 URL 是短期签名地址；默认直接安全下载。只有用户明确只要链接时使用 `--url-only` |
| “把多条听记媒体下载到目录” | `dws minutes +download --ids <uuid1,uuid2> --output-dir <相对目录>` | 最多 50 个，逐项保留成功与失败；禁止目录穿越和静默覆盖 |
| “把摘要、关键词、完整逐字稿、待办归档成一包” | `dws minutes +export-pack --id <taskUuid> --output <新目录>` | 逐字稿必须完整；所有必需产物验证后才原子发布目录；目标目录已存在时拒绝覆盖 |
| “归档时也带媒体” | `dws minutes +export-pack --id <taskUuid> --output <新相对目录> --include-media --format json` | manifest 不保存签名 URL；媒体未就绪导致归档不完整时必须明确失败 |
| “按标签找听记” | `dws minutes tag list --format json`，再用真实 `tagId` 执行 `dws minutes tag query --tag-id <tagId> --limit 10 --format json` | 不按标签名猜 ID；空标签直接结束，有 `nextToken` 时续拉或明确结果不完整 |
| “查语音备忘” | `dws minutes audio-memo list --format json` | 属于独立原子查询；需要时间范围或分页参数时读取该 compact leaf Schema |

需要根据逐字稿总结指定发言人的内容时读取 [发言人匹配流程](10-minutes-speaker-match.md)；用户已经确认“匿名发言人 → 姓名”的对应关系、准备执行标注替换时读取 [发言人纠正流程](11-minutes-speaker-correct.md)。两者都不能凭职位、语言风格或列表顺序自动认定身份。

## 批量文本替换

```text
dws minutes +replace-batch --id <taskUuid> --pair "旧词1=>新词1" --pair "旧词2=>新词2" --failure-policy stop --page-limit 100 --dry-run --format json
```

- `--pair` 可以重复；也可用 `--json '<数组>'`、`--json @<相对文件>` 或 `--json -`，二者至少提供一种。每组格式固定为 `原文=>替换`，原文不能为空或重复。
- 用户没有提供完整的“原文=>替换”映射时，不查询 Help、不编造替换词，也不执行没有业务意义的空 dry-run；先按已有线索列出待补齐的 `原文=><目标词>` 模板并只询问一次，拿到完整映射后再预演。
- `--failure-policy continue` 只表示失败后继续尝试剩余规则；只要有一项失败，整体仍是 partial/非零。
- dry-run 是 `remoteReads=false` 的本地计划；返回的 `total` 是替换规则数，不是逐字稿中的命中次数。要声称命中必须有完整逐字稿或真实执行前预检证据。
- 真实执行按 Runtime confirmation；完成后以逐项 ledger 和完整逐字稿读回为准，不能让最终答复与工具结果矛盾。

## 标签查询

```text
dws minutes tag list --format json
dws minutes tag query --tag-id <tag-list真实返回的tagId> --limit 10 --format json
dws minutes tag query --tag-id <tagId> --limit 10 --cursor <nextToken> --format json
```

- 不按标签名称猜 `tagId`；`tag list` 明确返回空数组时直接交付“当前无标签”，不再查 Help 或拿其他分组补位。
- `tag query` 是单页原子查询；返回真实 `nextToken` 时继续续拉，或明确说明当前结果不完整。

## 本地归档

```text
dws minutes +export-pack --id <taskUuid> --output ./minutes-export --format json
dws minutes +export-pack --id <taskUuid> --output ./minutes-export --include-media --format json
```

- `--output` 必须是工作目录内尚不存在的安全相对目录；命令拒绝目录穿越和静默覆盖。
- 默认归档 `basic,summary,keywords,transcript,todos`；如用 `--artifacts` 缩小集合，只能声称已交付实际选择并验证通过的产物。
- `--include-media` 只决定是否附带媒体，不降低逐字稿完整性要求；manifest 不保存短期签名 URL。
- 只有响应中的 `published=true`、真实 `path/manifest/files` 和所选产物均完整，才能称归档已生成；任一产物 unknown/pending/failed 时不得宣称成功。

## 目标匹配

- 用户给了 `taskUuid` 或听记 URL：直接解析和使用真实 ID，不再按标题搜索。
- 用户给了标题：优先精确标题；没有精确命中时可以返回标题包含或语义相关候选。候选足够接近且唯一时可继续；差异明显或多个候选都合理时让用户选择。全量搜索尚未完成但有有效 `nextToken` 时继续，只有 continuation 缺失/停滞/循环、达到页数上限或后页失败时停止并说明不完整。
- 不要求用户口述标题必须与服务端字符逐字相同，但也不能把“语义相关”当成“已确认目标”。任何写操作前都要确保目标唯一。
- 用户明确说“最新一条”才使用 `+latest`；它不是通用消歧器，也不能用于录音绑定。

## 内容形态边界

| 需要的结果 | 使用 |
|---|---|
| 只在对话里查看摘要/逐字稿/待办 | Minutes 读取命令 |
| 形成可持续编辑的钉钉文档 | 先用 Minutes 读取真实内容，再切 `dingtalk-doc` 创建或编辑 |
| 把行动项变成可分派任务 | 先用 `+action-items`，再切 `dingtalk-todo` |
| 给别人发摘要文本 | 切 `dingtalk-chat`；不要把授予听记权限误当成发送消息 |
| 管理会议时间或会议室 | `dingtalk-calendar` |
| 管理普通云盘文件 | `dingtalk-drive`；听记媒体下载和听记上传仍由 Minutes 负责 |

## 写入与确认

- 发言人替换、批量文本替换都改变现有听记内容，按 Runtime confirmation 执行；dry-run 只显示计划，不写远端。
- 下载和导出写入本地工作目录，不改变远端听记，但必须使用安全相对路径、no-clobber 和原子发布语义。
- 任一批量流程只要存在失败项，就按 partial/非零交付完整 ledger；不能只汇报成功项。
