# 钉钉文档富结构更新流程

本页只处理已有文档中的富结构保真编辑。普通 append、overwrite、唯一文本替换和普通 block 编辑直接使用 `dws doc +update`，不读取本页。

执行入口优先为 `+update/+checkpoint-update`。参数、constraint 和 confirmation 以已加载的 CommandMeta 或精确 leaf Schema 为准；不要从本文推导不存在的 flag。特别禁止把原子命令的 `--revision` 或 `--fix-jsonml` 传给 `+update`，shortcut 的并发参数是 `--expected-revision`。

## 1. 选择最小更新动作

| 用户意图 | 推荐动作 |
|---|---|
| 末尾增加普通正文 | `append` |
| 整篇替换 | `overwrite` |
| 已知唯一旧文本改为新文本 | `str_replace` |
| 在真实 block 前/后插入 | `block_insert` + `ref-block/where` |
| 替换或删除真实 block | `block_replace` / `block_delete` |
| 重要整篇更新且要求恢复点 | `+checkpoint-update` |

只有已有内容确实包含颜色、callout、分栏、复杂表格、附件、@人等必须保留的结构时，才读取最小 JSONML 子树或全文。不要为了修改一段普通文字默认拉取完整 JSONML。

## 2. 最小读取

- 已知唯一旧文本：优先 `str_replace`，让 shortcut 验证唯一性。
- 已知 block URL/ID：直接使用，不重新搜索。
- 需要定位 block：用 `+fetch --detail with-ids` 或局部 scope 获取真实 ID。
- 多处富结构保真覆盖：读取 full 内容并保存真实 revision；只有这种 JSONML overwrite 才考虑 `--expected-revision`。
- 末尾追加普通正文不必预读全文；只在需要判断末尾衔接时读取末尾局部。

正文、块、版本、媒体和样式更新属于可恢复文档写入，不强制确认，也不追加 `--yes`。目标或范围不明确时仍需消歧，但不要把消歧当成安全确认。

## 3. Block ID 生命周期

| 操作 | 旧 ID 后续使用规则 |
|---|---|
| `str_replace`、不改变结构的属性更新 | 未受影响的 ID 可继续使用 |
| `block_replace` | 不复用被替换 block 的旧 ID；优先使用回执的新/受影响 ID |
| `block_delete` | 被删除 ID 立即失效 |
| `block_insert` / copy | 新块只使用回执的 `insertedBlockId/affectedBlockIds` |
| `overwrite` | 旧正文 block ID 全部视为失效 |
| `+media-insert` | 使用回执中的 `insertedBlockId/resourceId` |

下一步依赖变化后的结构且回执没有稳定 ID 时，才定点 refetch。不要每次写后全篇读取，也不要继续复用已失效 ID。

## 4. 格式与内容传输

- 普通文本编辑保持 Markdown，不因中文、篇幅或文档类型切换 JSONML。
- 只在保留已有富结构或用户明确要求富结构时使用 JSONML。
- 用户明确格式要求高于默认样式；不得主动改写未要求的章节、颜色或布局。
- 正文通过 cwd 相对 `@file` 或 stdin 传递，禁止绝对路径、`..` 和把用户正文作为 `printf` 格式字符串。
- `--expected-revision` 仅用于 `--command overwrite --doc-format jsonml`；其他动作不模拟原子 revision。

如果 JSONML validator 报错，依据具体节点错误修正一次。仍失败时简化非必要结构；简化会违反用户要求则停止并说明，不做三次以上无界重试。

## 5. 执行与验证

1. 优先消费已加载的 CommandMeta；只有参数、constraint 或 confirmation 缺失时查询一次精确 leaf Schema。
2. 按 Runtime gate 执行一个最小写动作；不要因 Help/Schema 探测失败而换同义原子命令。
3. 使用 `+update` 的稳定回执和有界回读结果判断完成状态。
4. `partial_success` 只补未完成步骤；`unknown` 或 `doc_write_verification_failed` 先读取现状，禁止重放写入。
5. 后续步骤只消费当前回执仍有效的 ID；必要时定点 refetch。

验证内容必须对应用户诉求：目标短语是否替换、标题/段落是否处于要求位置、删除对象是否消失、媒体是否返回稳定 ID。没有渲染或截图证据时，只能报告内容和结构验证，不能宣称视觉排版完全正常。
