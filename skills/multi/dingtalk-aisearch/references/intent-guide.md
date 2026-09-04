# AISearch 局部意图消歧

仅当“定位记录”与“操作已知对象”的边界仍不明确时读取。

| 用户目标 | 应该用 | 不要用 |
|---|---|---|
| 按姓名、工号、部门、职位、职责或上下级找人 | `aisearch person` | 用 Contact 做姓名模糊搜索 |
| 完整手机号精确反查；已知 userId 查详情 | `contact` | `aisearch person` |
| 按主题跨文档、消息、邮件、待办、听记等来源找记录 | `aisearch enterprise` | 分别加载各产品做跨源搜索 |
| 找以我为关系端点的发送/接收记录，或我创建、编辑、分享过的记录 | `aisearch behavior` | 用 Chat/Mail/Doc recent 拼接行为结论 |
| <!-- dws-intent: chat.search.filtered -->资源只限 IM，答案是逐条消息并带发送者、会话、时间、reaction 等消息谓词 | `dws chat +search-msg` | 用行为方向替代消息集合过滤 |
| 已有唯一稳定 ID，要求读取正文、逐字稿或详情 | 对应产品 Skill | 用 AISearch snippet 冒充原文 |
| 已有唯一稳定 ID，要求修改、发送或删除 | 对应产品 Skill | 继续用 AISearch |

判断顺序：**资源范围 → 答案形态 → 原生谓词。**跨来源主题走 enterprise，当前用户行为轨迹走 behavior，仅 IM 的逐条消息过滤走 Chat；已知对象再读取或操作。
