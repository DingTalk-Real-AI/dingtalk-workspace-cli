---
category: Added
---

- **审批图片上传** — 新增 `oa approval image upload`，校验本地图片并自动压缩超出 2MB 的图片，返回 `mediaIdV2` 供审批图片控件使用；同步修正联系人控件取值格式与部门 ID 核验指引。
- **群公告选项** — 创建和编辑公告支持 `--no-card` 静默发布及 `--must-read` 新人必看；显式传入 `false` 可关闭对应选项。
- **联系人身份查询** — 新增 `contact user get-open-dingtalk-id`（别名 `get-open-id`），按手机号或钉钉号查询 `openDingTalkId`，提供统一结果与 Schema；服务端灰度与频次限制仍适用。
- **登录审批来源** — CLI 授权状态、超级管理员查询及授权申请请求在识别到 Agent 宿主时携带来源标识。
