---
category: Fixed
---

- **AI 表格建表及仪表盘指引**：建表前拒绝不适用的 formatter，bootstrap 返回字段 ID 并容忍建表后短暂不可见；补齐图表模板说明兼容、均值配置与布局尺寸诊断。更新 mono/multi Skill 的单条执行、独立核验及关联记录格式指引。
- **AI 表格链接交付**：保留 Base 列表和搜索返回的逐对象导航字段；过程展示或宿主卡片不替代最终正文中的有效具名完整链接。按对象身份和用途选择入口，原样保留 URL 编码，缺少填写入口时不向填写者交付管理页。详见 [过程链接与最终交付](../docs/aitable-process-link-display.md)。
- **分页完整性**：聊天及智能读取在分页未知时不再宣称 `paginationKnown=true`；DING 列表允许递减但不重复的续页游标，避免误判停滞。
- **考勤组成员更新**：`attendance group update-members` 必须显式传入 `--member-join-policy SPECIFIC_USERS`，并同步参数声明、校验、示例与 Skill。
- **文档图片预览**：上传回执带资源 ID 时，内联图片写入 `extraData.resourceId`；兼容不含资源 ID 的旧内联图片回执，其他依赖资源 ID 的写入仍严格校验。
