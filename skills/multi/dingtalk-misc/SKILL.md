---
name: dingtalk-misc
description: 长尾产品集合技能，覆盖低频钉钉产品：OA审批/考勤/直播/DING紧急消息/开放平台应用管理/Agoal目标管理/日志日报周报/电子表格/开放平台文档搜索/文档内嵌白板。Use when 用户提到上述任一产品，或审批/打卡/排班/OKR/日报周报/单元格读写/白板节点读写等相关操作。命中后由本 skill 的「产品索引表」定位具体子产品和命令前缀，再按对应子产品说明执行。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 长尾产品集合 Skill（dingtalk-misc）

## 前置条件 — 执行操作前必读

> **CRITICAL — 执行任何 `dws` 操作前，MUST 先用 Read 工具完整读取 [`dws-shared`](../dws-shared/SKILL.md)。**该轻量文件包含全局执行契约、安全底线及 shared references 的按需加载导航；不要预加载其全部 references。

本 skill 打包低频长尾产品，命令前缀各不相同。**不要**把这份 SKILL.md 当作命令细节来源——它只负责路由；命中某个产品后，务必读取该产品的 `references/<product>.md` 获取 Usage / Example / Flags / 危险操作确认 / 跨产品协作等完整信息。

## 产品索引表

| 触发关键词 | 一句话范围 | 命令前缀 | 详细参考 |
|---|---|---|---|
| OA / 审批 / 待处理审批 / 同意 / 拒绝 / 撤销 / 已发起审批 | OA 审批：待处理/详情/同意/拒绝/撤销/已发起/批量审批 | `dws oa` | [oa.md](references/oa.md) |
| 考勤 / 打卡记录 / 排班 / 班次 / 考勤报表 / 考勤组 | 考勤记录、打卡查询、排班、考勤组、报表导出 | `dws attendance` | [attendance.md](references/attendance.md) |
| 直播 / 我的直播 / 直播列表 | 直播列表与直播记录查询 | `dws live` | [live.md](references/live.md) |
| DING / 紧急通知 / 电话DING / 短信DING / 必达消息 | DING 紧急消息（应用内/短信/电话），个人DING | `dws ding` | [ding.md](references/ding.md) |
| 开放平台应用 / 企业内部应用 / 应用成员 / 应用权限 / 应用版本 | 开放平台企业内部应用的查询、创建、修改、成员权限和版本管理 | `dws devapp` | [devapp.md](references/devapp.md) |
| 目标管理 / 战略解码 / 经营合约 / 计分卡 / OKR / 周月报统计 | Agoal 目标管理与经营目标跟进 | `dws agoal` | [agoal.md](references/agoal.md) |
| 日报 / 周报 / 月报 / 写日志 / 收件箱日志 / 发件箱日志 | 日志（日报/周报/月报）查询与按模版提交 | `dws report`（别名 `dws log`） | [report.md](references/report.md) |
| 电子表格 / 工作表 / 单元格读写 / 公式 / 超链接 / 浮动图片 | 电子表格创建/读写/公式/超链接/浮动图片/导出 | `dws sheet` | [sheet.md](references/sheet.md) |
| 开放平台文档 / API文档 / 接口文档 / 接口报错 | 开放平台开发文档搜索 | `dws devdoc` | [devdoc.md](references/devdoc.md) |
| 白板 / 画布 / OpenNodes / 白板节点 | 读取和更新钉钉文档中的内嵌白板 | `dws whiteboard` | [whiteboard.md](references/whiteboard.md) |

## 说明

- 每个产品的意图表、危险操作确认、硬约束、跨产品协作和短流程均在其 `references/<product>.md` 内，命中产品后必须读取，不要只凭本表推测命令。
- 产品自己的局部意图消歧文档命名为 `references/<product>-intent-guide.md`，不是共享的 `references/intent-guide.md`。
- 各产品之间跨产品协作若指向本包内的其它产品，已在对应 `references/<product>.md` 里写成"见本包 references/X.md"，无需切换 skill；若指向 top10 独立产品（如 `chat`/`aisearch`/`doc`），仍按 `dingtalk-<product>` 切换 skill。
