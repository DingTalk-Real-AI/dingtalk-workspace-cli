# AISearch 局部意图消歧

仅当“定位记录”与“操作已知对象”的边界仍不明确时读取。

| 用户目标 | 应该用 | 不要用 |
|---|---|---|
| 按姓名、工号、部门、职位、职责或上下级找人 | `aisearch person` | 用 Contact 做姓名模糊搜索 |
| 完整手机号精确反查；已知 userId 查详情 | `contact` | `aisearch person` |
| 按主题找文档、消息、邮件、待办、听记等记录 | `aisearch enterprise` | 分别加载各产品做搜索 |
| 找我/某人发送、收到、创建、编辑、分享过的记录 | `aisearch behavior` | 用 Chat/Mail/Doc recent 拼接行为结论 |
| 已有唯一稳定 ID，要求读取正文、逐字稿或详情 | 对应产品 Skill | 用 AISearch snippet 冒充原文 |
| 已有唯一稳定 ID，要求修改、发送或删除 | 对应产品 Skill | 继续用 AISearch |

判断口诀：**未知对象先定位，行为问题走 behavior；已知对象再读取或操作。**只点名一种来源不改变“按主题定位”的 AISearch 归属。
