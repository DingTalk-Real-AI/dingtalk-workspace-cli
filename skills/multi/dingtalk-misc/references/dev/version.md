# 应用版本创建与发布

配置写入不等于上线；版本进入 `RELEASE` 才表示已发布生效。版本始终用 `unifiedAppId + versionId` 定位。

## 标准闭环

```bash
dws dev app version list --unified-app-id <id> --page-size 50 --format json
dws dev app version create --unified-app-id <id> --desc <描述> --dry-run --format json
dws dev app version check-approval --unified-app-id <id> --version-id <create返回的versionId> --format json
dws dev app version publish --unified-app-id <id> --version-id <versionId> --dry-run --format json
dws dev app version status --unified-app-id <id> --version-id <versionId> --format json
dws dev app version get --unified-app-id <id> --version-id <versionId> --format json
```

最初请求只允许执行 create/publish 的 dry-run，不能代替预检后的确认。每一步都先展示预检返回的应用 ID、动作、版本/审批参数及影响，等用户对该预览明确确认后，才把同一命令仅由 `--dry-run` 换成 `--yes`；目标或业务参数变化就重新预检并确认，确认前不得真实创建或发布。dry-run 不是业务完成。create 成功后只使用它返回的 `versionId`，不要再 list 猜“最新”；没返回 ID 就停止。默认不传 `--version`，服务端自动递增；用户明确指定时才传。

`version list` 只有游标分页：`meta.pagination.endpoint_exhausted=false` 时把原样 `next_token` 传给下一次 `--cursor`，到 true 停止；不要使用 `--page/--page-num`，版本记录在 `data.items[]`。

查询“发过哪些版本/全部版本历史”时翻到 endpoint exhaustion，再按真实状态汇总。查询“最新待发布版本”时只在完整列表的 `versionStatus=INIT` 中按 `gmtCreate` 选择最新一条；若没有 INIT，明确回答“暂无待发布版本”，不要对 RELEASE 版本调用 `check-approval`，也不要为了制造待发布项擅自 `version create`。

## 审批决策

- `requiresApproval=false && publishable=true`：可以正式 publish。
- `approvalMode=SELECT_APPROVER`：原样展示 `approvalPromptText` 或 `approvalOptions[].label`，等待用户选人；不默认第一个。
- `approvalMode=ENTERPRISE_SELF_BUILT`：不传 approver，publish 后进入企业审批。
- 高敏权限错误明确要求确认时，只改变一次参数，加 `--confirmed-sensitive` 后重试；这属于有新证据的单次修复。

## 完成态与止损

| `versionStatus` | 结论 |
|---|---|
| `INIT` | 已创建但未发布，仍未生效 |
| `AUDIT` | 已提交审核；报告待审批，不重复 publish |
| `RELEASE` | 已发布生效 |
| `GRAY` | 灰度中，不等于全量发布 |

`processStatus=UNDER_REVIEW` 同样是等待外部审批；`FAIL/WITHDRAW/CANCEL/PUBLISH_FAILED` 原样报告真实状态与错误。未列出的状态不猜。

发布失败时最多做一次 `version get/status` 获取新状态；若错误含义仍不清楚，可用错误码做一次精确 `dws devdoc article search --query`。只有新证据改变 flag 或状态时才重试一次；同一 `errorCode` 下禁止追加 debug/verbose、重复 help、反复搜索或重复 publish。无法达到 `RELEASE/AUDIT` 时明确报告阻塞和已完成步骤，仍需回答用户要求的版本列表、应用状态等其它交付项。没有实际配置变更成功时不创建空版本；`version list` 为空时明确写“暂无版本”。

已知路径不调用 group help；仅 flag 确有疑问时查一次精确 leaf compact Schema，Schema 不可用才查一次 leaf help。
