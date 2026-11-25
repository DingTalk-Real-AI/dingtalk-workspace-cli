---
category: Added
---
- 新增 sport org admin：判断当前用户是否企业管理员（is_org_admin），只读无业务入参、身份平台注入，三态语义（管理员/非管理员/组织服务不可用报错），供导出删除审批等受限操作前权限预判与话术分流
- competition create 绑定群改为 --group-id 与 --open-conversation-id 至少一个（可同传，群 id 优先），仅加密会话 id 也能创建比赛
