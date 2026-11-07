# Minutes 兼容 Recipe 索引

> 返回入口：[DingTalk Minutes Skill](../SKILL.md) · [Reference 与脚本索引](minutes.md) · [Recipe 通用约束](recipes/conventions.md)

本文件保留旧版 Recipe 名称到当前 Golden Route 的映射，供兼容调用方定位；它不是另一套命令权威。当前路由、安全、Result 和 Pagination 以根 Skill、精确 Reference、leaf Schema 与 Runtime 为准。

| 旧 Recipe 意图 | 当前推荐入口 | 继续阅读 |
|---|---|---|
| `minutes-query`：查询、列表、详情 | `+search`、`+list-*`、`+latest`、`+detail` | [原子与完整性边界](minutes.md) |
| 完整逐字稿 | `+transcript` | [读取内容](minutes.md#3-读取内容) |
| 行动项 | `+action-items` | [读取内容](minutes.md#3-读取内容) |
| `minutes-edit`：标题、纪要 | `+update`、`+summary` | [更新与录音控制](minutes.md#4-更新与录音控制) |
| 发言人总结/替换 | `+transcript` → 用户确认 → `+speaker-replace` | [发言人内容匹配](10-minutes-speaker-match.md) / [标注与替换](11-minutes-speaker-correct.md) |
| `minutes-tag`：标签与语音备忘 | 原子 `tag` / `audio-memo` 精确 leaf | [标签与语音备忘](minutes.md#9-标签与语音备忘) |
| `minutes-permission` | `+apply-permission`、`+share`、`+unshare` | [复杂流程](07-minutes.md#4-权限-workflow) |
| `minutes-upload` | `+upload`、`+upload-and-notify`、`+upload-and-analyze` | [上传、通知与恢复](07-minutes.md#2-上传通知与恢复) |
| ASR 热词 | `+prepare-asr` 或明确 destructive 的 `+sync-asr` | [ASR 热词](07-minutes.md#1-asr-热词) |

## 兼容边界

- “我的听记”不自动等于 `mine`；只有用户明确说“我创建/发起的”才缩窄。完整 accessible 读取走当前有界聚合，并检查 `complete=true`。
- 原子 `get transcription` 是单页接口；完整逐字稿只推荐 `+transcript`。
- 旧 flag、alias 或脚本只有在当前 leaf Help/脚本说明仍存在时才可使用；不能根据旧 Recipe 猜参数。
- 仓库辅助脚本的定位和限制见 [Reference 与脚本索引](minutes.md#辅助脚本)。它们不覆盖完整分页、跨 scope 聚合或通用目标消歧。
- 任何写操作的确认以当前 leaf Schema 和 Runtime gate 为准；旧 Recipe 中出现过的示例不能构成执行授权。
