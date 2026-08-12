---
name: dingtalk-misc
description: 长尾产品集合技能，覆盖低频钉钉产品：OA审批查询与处理/考勤/直播/DING紧急消息/开放平台应用管理/Agoal目标管理/日志日报周报/电子表格/开放平台文档搜索/文档内嵌白板/DWS技能市场安装/组织大脑Hrbrain/原生Markdown/PAT行为授权/多组织profile。Use when 用户提到上述任一产品，或查待审批/同意拒绝转交撤销审批/打卡/排班/OKR/日报周报/单元格读写/白板节点读写/搜索安装技能/开发者后台应用/人才池/员工档案/职业历程/绩效/原生.md文件/PAT授权/切换组织/跨组织/profile 等相关操作。未来审批任务或实例变化的实时监听不属于本 skill，应使用 dingtalk-event。命中后由本 skill 的「产品索引表」定位具体子产品和命令前缀，再按对应子产品说明执行。
metadata:
  cli_version: ">=0.2.14"
  category: product
  requires:
    bins:
      - dws
---

# 长尾产品集合 Skill（dingtalk-misc）

## 前置条件 — 执行操作前必读

> **CRITICAL — 执行任何 `dws` 操作前，MUST 先用 Read 工具完整读取 [`dingtalk-shared`](../dingtalk-shared/SKILL.md)。**该轻量文件包含全局执行契约、安全底线及 shared references 的按需加载导航；不要预加载其全部 references。

本 skill 打包低频长尾产品，命令前缀各不相同。**不要**把这份 SKILL.md 当作命令细节来源——它只负责路由；命中某个产品后，务必读取该产品的 `references/<product>.md` 获取 Usage / Example / Flags / 危险操作确认 / 跨产品协作等完整信息。

## 产品索引表

| 触发关键词 | 一句话范围 | 命令前缀 | 详细参考 |
|---|---|---|---|
| OA / 审批 / 待处理审批 / 同意 / 拒绝 / 撤销 / 已发起审批 | OA 审批：待处理/详情/同意/拒绝/撤销/已发起/批量审批 | `dws oa` | [oa.md](references/oa.md) |
| 考勤 / 打卡记录 / 排班 / 班次 / 考勤报表 / 考勤组 | 考勤记录、打卡查询、排班、考勤组、报表导出 | `dws attendance` | [attendance.md](references/attendance.md) |
| 直播 / 我的直播 / 直播列表 | 直播列表与直播记录查询 | `dws live` | [live.md](references/live.md) |
| DING / 紧急通知 / 电话DING / 短信DING / 必达消息 | DING 紧急消息（应用内/短信/电话），个人DING | `dws ding` | [ding.md](references/ding.md) |
| 开放平台应用 / 企业内部应用 / 应用成员 / 应用权限 / 应用版本 / agentId / clientId / 机器人配置 / 版本发布 / connect | 开放平台企业内部应用的查询、创建、修改、成员权限、机器人与版本管理 | `dws dev` / `dws devapp` | [devapp.md](references/devapp.md) |
| 目标管理 / 战略解码 / 经营合约 / 计分卡 / OKR / 周月报统计 | Agoal 目标管理与经营目标跟进 | `dws agoal` | [agoal.md](references/agoal.md) |
| 日报 / 周报 / 月报 / 写日志 / 收件箱日志 / 发件箱日志 | 日志（日报/周报/月报）查询与按模版提交 | `dws report`（别名 `dws log`） | [report.md](references/report.md) |
| 电子表格 / 工作表 / 单元格读写 / 公式 / 超链接 / 浮动图片 | 电子表格创建/读写/公式/超链接/浮动图片/导出 | `dws sheet` | [sheet.md](references/sheet.md) |
| 开放平台文档 / API文档 / 接口文档 / 接口报错 | 开放平台开发文档搜索 | `dws devdoc` | [devdoc.md](references/devdoc.md) |
| 白板 / 画布 / OpenNodes / 白板节点 | 读取和更新钉钉文档中的内嵌白板 | `dws whiteboard` | [whiteboard.md](references/whiteboard.md) |
| 搜索技能 / 找技能 / 安装技能 / 技能市场 / 安装 DWS mono 或 multi skill | DWS 技能市场搜索、下载、安装与内置技能部署 | `dws skill` | [skill.md](references/skill.md) |
| 人才池 / 储备干部池 / 员工档案 / 职业历程 / 绩效记录 / 员工标签 / 组织大脑 / 人才搜索 | 组织大脑：人才池、员工档案专项模块与结构化人才搜索 | `dws hrbrain` | [hrbrain.md](references/hrbrain.md) |
| 原生 Markdown / `.md` 原文 / 覆盖 Markdown / 局部替换 Markdown | 原生 `.md` 文件读取、创建、全量覆盖与局部替换 | `dws markdown` | [markdown.md](references/markdown.md) |
| PAT 授权 / 行为权限 / scope 授权 / 一次性授权 / 会话授权 / 永久授权 / 授权浏览器策略 | PAT 行为授权与本地浏览器策略 | `dws pat` | [pat.md](references/pat.md) |
| 切换组织 / 换组织 / 跨组织 / 多组织 / profile / 看登录了哪些组织 | 多组织 / profile 管理与跨组织取数 | `dws profile` / `dws auth` / `--profile` | [profile.md](references/profile.md) |
| 宜搭 / AI应用脚本 / 财务辅助脚本（未产品化） | **无**稳定命令面；仅仓库内辅助脚本 | （非默认路由） | [unsupported-scripts.md](references/unsupported-scripts.md) |

## 说明

- 每个产品的意图表、危险操作确认、硬约束、跨产品协作和短流程均在其 `references/<product>.md` 内，命中产品后必须读取，不要只凭本表推测命令。
- 产品自己的局部意图消歧文档命名为 `references/<product>-intent-guide.md`，不是共享的 `references/intent-guide.md`。
- 各产品之间跨产品协作若指向本包内的其它产品，已在对应 `references/<product>.md` 里写成"见本包 references/X.md"，无需切换 skill；若指向 top10 独立产品（如 `chat`/`aisearch`/`doc`），仍按 `dingtalk-<product>` 切换 skill。
- `scripts/` 下 yida / finance / `aiapp_create_and_poll.py` 等见 [unsupported-scripts.md](references/unsupported-scripts.md)；默认不要当正式能力调用。
- 开放平台应用的命令组细文档在 [references/dev/](references/dev/)；命中后先读 [devapp.md](references/devapp.md)，再按需加载对应子文件。
- 查询、同意、拒绝、转交或撤销审批走 [oa.md](references/oa.md)；要求未来审批任务或实例发生变化时实时通知，切换独立的 [`dingtalk-event`](../dingtalk-event/SKILL.md)。开放平台应用事件配置仍属于 DevApp，按 [dev/event.md](references/dev/event.md) 执行，不要与个人实时事件混淆。
- 原生 `.md` 与在线富文本 `adoc`、通用文件存储的边界见 [markdown.md](references/markdown.md)；跨组织 / profile 规则见 [profile.md](references/profile.md)。
- PAT 行为授权不是开放平台应用权限；后者见 [devapp.md](references/devapp.md)。
