# Drive 低频能力参考

仅在根 Skill 的 Golden Route 不足时读取本文件的一个相关章节。高频搜索、列表、检查、单文件传输和文件夹同步直接按根 Skill 执行。

## 身份与目标位置

- `--node`、`--folder`、`--remote-folder` 使用 dentryUuid/fileId；数字 dentryId 不能替代。
- 回收站恢复使用 `recycleItemId`，不能复用删除前的 nodeId。
- URL 类型不明时只执行一次 `dws drive info --node <URL> --format json`，按真实 nodeId/nodeType 分流。
- 普通文件夹目标使用 `--folder`；明确知识库 workspace 才使用 `--workspace`。零命中、多候选或类型不明时停止。

## Runtime 确认与首次执行

- 先解析唯一目标，再以精确 leaf Schema 和 Runtime gate 判断是否需要确认；不要通过一次缺少 `--yes` 的失败调用探测确认要求。
- Runtime 要求确认且当前请求已明确授权具体节点/文件夹、动作与影响时，首次正式远端写调用直接追加 `--yes`。诸如“整理一下”“处理这些文件”不能视为对删除、覆盖、移动或公开状态变更的精确授权。
- Runtime 不要求确认时不添加 `--yes`。对象、范围或影响不完整时先询问；确认后必须保持同一 profile、目标、动作、范围和关键参数，任一项变化都要重新确认。
- `--dry-run`、差异预览和只读检查不加 `--yes`；预览结果符合授权范围后，正式执行才追加。收到 `confirmation_required` 仅表示尚未通过预执行门禁，不代表业务写入成功，也不能据此盲目重放。

## 高级目录列表

普通浏览优先 `+list`。需要递归、名称模式、节点类型或修改时间过滤时使用 managed leaf：

```bash
dws drive list --folder <dentryUuid> --depth 2 --pattern "*周报*" --format json
dws drive list --folder <dentryUuid> --type file --start 7d --format json
```

- `--depth` 最大 5；递归总量上限以结果中的 `truncated/errors` 为准。
- `--type file|folder` 是节点类型；`search --file-types` 是内容类型，二者不同。
- `--start/--end` 接受 `24h/7d/2w`、日期或 RFC3339，按修改时间过滤。
- 过滤模式是客户端有界扫描；`truncated=true` 不能声称全量。需要关键词检索时改用 `+search`。
- `--latest` 遇到截断或目录读取失败会拒绝给出不完整 Top-N，按错误中的目录和恢复命令缩小范围。

## 回收站

```bash
dws drive +delete --node <dentryUuid>
dws drive +recycle-list --limit 20
dws drive +recycle-restore --id <recycleItemId>
```

删除前核对名称、类型和 ID；恢复从列表真实返回取 `id`。恢复后使用返回的新 nodeId，不沿用旧 ID。

## 普通文件历史版本

| 意图 | 入口 | 完成证据 |
|---|---|---|
| 列版本 | `+version-history` | 版本集合与分页字段 |
| 查看版本 | `+version-get` | 请求的 version |
| 下载版本 | `+version-download` | 相对路径存在且 sizeBytes > 0 |
| 回滚版本 | `+version-revert` | Runtime 确认后读回最新版本 |

这些入口只用于普通文件。adoc 版本走 Doc，axls 版本走 Sheet。

## 收藏、统计与封面

- 收藏列表：`+star-list`；收藏/取消：`+star-add` / `+star-remove`。
- 节点统计和封面优先并入 `+inspect --include-stats` / `--include-cover`；只取单项才用 `+stats` / `+cover`。
- 收藏是个人状态，不代表共享或权限变化。

## 公开状态

```bash
dws drive +publish-get --node <dentryUuid>
dws drive +publish-unset --node <dentryUuid>
```

`+publish-get` 只读；`+publish-unset` 为高风险写。Runtime 虽注册了 `+publish-set`，但当前普通文件和在线文档都没有经过验证的开启公开闭环，因此根 Skill 明确不将它开放给 Agent。用户要求开启公开时，说明当前 Agent 路由不支持并停止；不要查询或执行 `+publish-set`、`drive publish set` 或其他替代写入口。只有补齐受支持节点上的真实 set→get→unset 闭环证据并更新 Agent 路由后，才重新开放该能力。

## 权限

| 意图 | managed leaf |
|---|---|
| 查看成员权限 | `drive permission list` |
| 查询节点权限设置（权限模式/分享范围/策略） | `permission get-setting` |
| 添加、修改、移除成员 | `permission add` / `update` / `remove` |
| 转移所有者 | `permission transfer-owner` |
| 查看可申请权限和审批人 | `permission apply-info` |
| 发起权限申请 | `permission apply` |

只在意图命中时读取一个精确 leaf Schema。成员变更、转移所有者、发起申请和公开状态变更必须明确节点、用户、角色与影响范围。转移所有者时，在构造最终命令前必须让用户分别明确决定 `--reserve-role <MANAGER|EDITOR|DOWNLOADER|READER|NONE>` 和 `--recursive=<true|false>`；Agent 不得根据默认值、对象类型或便捷性自行选择任一项。两项决策与目标、新所有者均明确后，才按 Runtime confirmation 构造首次正式调用。

`permission get-setting` 返回 `permissionMode`（INHERITED/INDEPENDENT，未知时为 null）、`shareScope`（可见范围与链接分享，密码明文不返回；`partnerIncluded`、`defaultRole` 等仅 ORGANIZATION 有意义，`linkShare` 仅开启链接分享时返回）和 `policies[]`（code/name/description/value/disabledValues/allowedValues；name/description 为中文名与值语义说明，随行必带；未下发的策略不返回，`node_spread_scope` 仅文件夹）。`disabledValues` 为不可设置取值列表（恒返回，无被禁档位时为空数组），每项含 `value`（被禁档位取值，与 value 同一值域）与 `reason`（服务端按请求语言返回的禁用原因文案，仅供展示理解，可为 null），与 allowedValues 互斥；示例：`{"value": "READER_AND_ABOVE", "reason": "企业安全策略要求不可低于可下载角色"}`。`value` 按策略分型：开关型为 ENABLED/DISABLED；member_invite、comment 为 READER_AND_ABOVE/DOWNLOADER_AND_ABOVE/EDITOR_AND_ABOVE/MANAGER_AND_ABOVE；node_spread、online_content_copy 为 DOWNLOADER_AND_ABOVE/EDITOR_AND_ABOVE/MANAGER_AND_ABOVE 或 NOBODY；node_spread_scope 为 ALL_NODES（限制对所有文档生效）/ PREVIEWABLE_ONLY（仅对可预览的文档生效）。NOBODY=该操作对所有人禁止；XXX_AND_ABOVE=不低于该角色才允许。name/description 示例（文案与产品权限设置页一致）：external_share「添加企业外协作者」：是否允许添加企业外的人为协作者（ENABLED=允许，DISABLED=禁止）；node_spread「谁可以下载、创建副本、打印」：允许哪些角色及以上的用户下载、创建副本、打印；NOBODY=所有人禁止下载、创建副本、打印；node_move_forbidden「禁止移动」：是否禁止移动到其他知识库或团队共享文件夹（ENABLED=禁止移动，DISABLED=允许移动）。

发起权限申请先只读执行 `permission apply-info`。正式 `permission apply` 会通知审批人；调用前必须向用户逐项回显并确认资源、申请角色、审批人和理由。Agent 不得默认选择第一位审批人、最高/最低角色或代写申请理由；用户未明确同意完整申请内容时停在确认环节。

## 快捷方式节点

`+create-shortcut --node <源ID> [--folder <目标ID>|--workspace <知识库ID>]` 创建链接。在线文档节点需要保留版式的独立副本时使用 `+copy`；普通钉盘文件的独立副本必须经用户授权后走 download→upload，因为当前 `+copy` 会拒绝普通文件。源/目标类型不兼容时停止，不把快捷方式当普通文件继续覆盖。

## 错误恢复

1. `unknown flag` 只查当前 leaf Help；`unknown command` 只查一次 Drive Shortcut 清单。
2. 分页、递归或过滤未完成时保留 cursor/truncated/errors，不包装成全量成功。
3. 写入超时或响应丢失先按 nodeId、名称、路径和大小回读，不能盲目重放。
4. 在线文档误入普通下载/覆盖时切 Doc、Sheet 或 AITable；不要用 Drive 重试改变内容。

## 辅助脚本

- [drive_tree_list.py](../scripts/drive_tree_list.py)：递归列出钉盘目录树；普通浏览仍优先使用 `dws drive +list`。
