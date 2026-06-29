# 行命名规则枚举键（recordNameKey）映射

`dws aitable table update --record-name-key <枚举键>` 用于设置数据表的"行命名规则"——卡片/详情页里"行"的展示别名。**取值是固定枚举，不是字段 ID**；传非法值服务端返回 `INVALID_RECORD_NAME_KEY`。

## 中文 → 枚举键（按 UI 下拉顺序）

| 用户说 | --record-name-key | 用户说 | --record-name-key |
|---|---|---|---|
| 记录 | `ji_lu`（默认） | 项目 | `project` |
| 任务 | `task` | 事件 | `event` |
| 请求 | `request` | 活动 | `campaign` |
| 目标 | `objective` | 交付物 | `deliverable` |
| 资产 | `asset` | 客户 | `customer` |
| 订单 | `order` | 联系人 | `contact` |
| 物料/物品 | `item` | 问题 | `question` 或 `issue` |
| 工单 | `ticket` | 候选人 | `candidate` |
| 商机/机会 | `opportunity` | 会议 | `meeting` |
| 成员 | `member` | OKR | `okr` |

## 其他常用键（按场景分组）

- **业务流程**：`approval` / `application` / `case` / `decision` / `delivery` / `payment` / `purchase_order` / `quote` / `release`
- **HR / 财务**：`employee` / `expense` / `budget` / `invoice`
- **产品 / 研发**：`feature` / `feedback` / `idea` / `bug` / `requirement` / `risk` / `sprint` / `story` / `subtask` / `epic`
- **CRM**：`account` / `lead` / `prospect` / `deal`
- **运营 / 支持**：`note` / `report` / `topic` / `session` / `service`
- **资源 / 通用**：`file` / `document` / `product` / `team` / `user` / `vendor` / `key_result` / `metric`

完整集合较大（共 273 个），服务端校验；以上未列出的合法键也可直接传（如 `goal` / `okr` / `pillar` / `phase` / `milestone` 等）。

## 使用示例

```bash
# 用户说"把这张表的行叫'任务'吧" → 传 task
dws aitable table update --base-id BASE --table-id TBL --record-name-key task

# 用户说"换成项目" → 传 project
dws aitable table update --base-id BASE --table-id TBL --record-name-key project

# 用户说"恢复成默认（记录）" → 传 ji_lu
dws aitable table update --base-id BASE --table-id TBL --record-name-key ji_lu
```

## 注意

- recordNameKey **不会在 `table get` 响应里回显**（`get_tables` DTO 设计上不暴露该字段）；写入是否成功以 `table update` 的 set response 是否回填 `recordNameKey` 字段为准。
- 中文别名是 server 内置 i18n，UI 显示用户对应的国际化文案，CLI 必须传英文枚举键。
