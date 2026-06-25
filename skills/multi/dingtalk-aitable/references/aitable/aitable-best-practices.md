# AI 表格最佳实践

## 1. 字段可写性分类

| 字段类型 | 可写 | 正确方式 |
|----------|------|----------|
| 文本/数字/日期/单选/多选/复选框/URL | ✅ | record create/update |
| 附件 | ⚠️ | 必须先走 [attachment upload 流程](./aitable-attachment.md) |
| 创建人/修改人/创建时间/修改时间 | ❌ | 系统字段，只读 |
| 公式/查找引用 | ❌ | 只读，由系统计算 |
| AI 字段 | ❌ | 只读，由 AI 自动计算 |

## 2. 查询执行契约

1. **不要拉全量后在 context 里手动统计** — 优先用 `--filters` 在服务端过滤
2. **has_more=true 时不能做全局结论** — 数据可能不完整
3. **优先用 `--filters` 在服务端过滤** — 不要拉全量后在本地 jq/grep
4. **字段名必须来自 `table get` 真实返回** — 不要猜测 fieldId
5. **减少响应体积** — 用 `--field-ids` 仅返回需要的字段

## 3. 任务选路

| 用户诉求 | 优先方案 | 不要误走 |
|---------|----------|----------|
| 查看几条数据 | `record query` | 不要用 `--all` |
| 全量拉取/统计 | `record query --all` | 不要手动循环 cursor |
| 全量导出为文件 | `export data` | 不要 `--all` 拉全量再写文件 |
| 批量写入 | `record create`（分批 100 条） | 不要一次传超过 100 条 |
| 附件/图片上传 | `attachment upload` 获取 fileToken → `record create/update` 用 fileToken 写入 | **严禁直接传图片 URL 到附件字段**（服务端同步下载会超时） |
| 文件级导入 | `import upload` + `import data` | 不要手动解析 xlsx 再逐条写入 |

## 4. 创建/修改后回读确认

执行写操作后，建议立即回读确认结果：

| 写操作 | 建议回读命令 | 确认内容 |
|--------|-------------|----------|
| `table create` | `table get --table-ids <新tableId>` | 表名、字段列表是否符合预期 |
| `field create` | `table get --table-ids <tableId>` | 新字段是否出现在字段列表中 |
| `record create/update` | `record query --record-ids <新recordId>` | 写入值是否正确 |

## 5. AI 字段注意事项

- AI 字段的 prompt **必须至少包含一个 `fieldRef` 引用**，纯文本 prompt 会被后端拒绝
- 先创建/确认被引用字段的 fieldId，再在 prompt 中引用
- `outputType` 必须与字段类型一致（如 `outputType=text` 配 `--type text`）
