---
category: Added
---

- `dws sport` 新增 7 个工具，保持 helpers 手写注册与隐藏的 misc 长尾入口：`competition invite/update/delete/audit/export/export-task`、`step export`；不接入 `get_user_org_list`。
- 比赛邀请支持员工与群至少一个目标，工号保持字符串；编辑使用完整 `--request` JSON 对象（支持 `@文件`），保留数值与显式 null，拒绝遗漏字段导致的误清空；删除不可恢复、审批同意会补发广播，均需用户确认且不自动重放。
- 比赛导出提交后返回 pending 与真实任务 ID；点查 PROCESSING/SUCCESS/FAILED 状态并保留 Long 精度、profile 及钉盘链接。步数报表只报告受理成功，无任务 ID、不自动等待或下载，文件由钉钉异步投递。
- 新工具操作人及企业身份由平台注入；同步 Help、Agent Schema、mono/misc 命令参考，明确灰度、权限、频控、编辑状态差异与“受理不等于完成”的边界。旧 create 的 FIXED 限制和既有参数保持不变。
