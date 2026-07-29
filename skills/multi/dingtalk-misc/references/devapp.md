# devapp Shortcut 参考

<!-- VISIBLE_SHORTCUTS_START -->
## Shortcuts（无专用脚本/recipe 时优先）

以下命令来自独立于 Runtime Schema 的公开 catalog。先运行 `dws shortcut list --service devapp --format json` 读取完整契约，再用 `dws devapp <shortcut> --help` 核对 flags；不要对 `+` 路径调用 `dws schema`。

| Shortcut | 风险 | 适用场景 |
|---|---|---|
| `dws devapp +create` | write | 创建开放平台企业内部应用 |
| `dws devapp +delete` | high-risk-write | 删除开放平台企业内部应用（不可逆） |
| `dws devapp +disable` | write | 停用开放平台企业内部应用 |
| `dws devapp +enable` | write | 启用开放平台企业内部应用 |
| `dws devapp +event-list` | read | 查询应用已订阅的事件列表 |
| `dws devapp +get` | read | 查询开放平台企业内部应用详情 |
| `dws devapp +list` | read | 查询开放平台企业内部应用列表 |
| `dws devapp +member-add` | write | 添加开放平台应用成员 |
| `dws devapp +member-list` | read | 查询开放平台应用成员 |
| `dws devapp +member-remove` | write | 移除开放平台应用成员 |
| `dws devapp +permission-list` | read | 查询开放平台应用权限列表 |
| `dws devapp +robot-get` | read | 查询现有应用的机器人配置 |
| `dws devapp +update` | write | 修改开放平台企业内部应用基础信息 |
| `dws devapp +version-check-approval` | read | 预检版本发布是否需要审批（不实际发布） |
| `dws devapp +version-get` | read | 查询指定版本详情 |
| `dws devapp +version-list` | read | 分页查询应用版本列表 |
| `dws devapp +version-status` | read | 查询版本发布/审批状态 |
| `dws devapp +webapp-config` | write | 配置网页应用能力 |
| `dws devapp +webapp-get` | read | 查询网页应用配置 |
<!-- VISIBLE_SHORTCUTS_END -->
