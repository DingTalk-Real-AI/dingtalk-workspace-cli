# 脑图 OpenNodes mind-v1

[返回脑图操作入口](../mind.md)。仅在携带内容创建或需要理解主题结构时读取本页。

## 开放结构与传输

创建 source 包含 `schemaVersion="1.0"`、`catalogVersion="mind-v1"` 和 pages 数组。每页包含 id、title、nodes；每个主题包含 id、type="topic"、text，可选 parentId。这是脑图协议，不能使用白板的 shape、坐标、连接线、样式或其 OpenNodes 示例。

| 字段/边界 | 约束 |
|---|---|
| source | UTF-8，最多 2 MiB，1..20 页，总计最多 10000 个主题 |
| 页面 id、主题 id | 客户端临时 ID，匹配 `[A-Za-z0-9][A-Za-z0-9._:-]{0,127}`；页面 ID 不重复，主题 ID 在全部页面中不重复 |
| 页面 title | 1..255 个 Unicode 字符；与创建显示名称的 128 个 UTF-16 单元限制不同 |
| 主题 text | 必须显式提供字符串，最多 10000 个 Unicode 字符；允许空字符串，空格和换行原样保留 |
| parentId | 根主题必须省略，不能用 null 或空字符串代替；其他主题引用同页父主题 |
| 树结构 | 每页恰好一个根，无环、无孤儿、无跨页父引用，最多 64 层（根为第 1 层）；父主题可出现在数组后面，同父兄弟顺序按输入相对顺序 |
| 字段 | 拒绝未知字段、重复 JSON key、错误类型或缺失必填字段，不传内部 checkpoint/模型协议 |

CLI 的 `--source` 接受文件或内联 JSON，`--topic` 接受仅含 id/type/text 的 JSON 对象或文件（最多 128 KiB），`--text` 接受原始文本。CLI 前置校验发生在远端调用前，dry-run 也校验；非法输入不会提交给 MCP。

MCP 将嵌套内容定义为 JSON string，CLI 负责一次编码：

| CLI 输入 | MCP 业务参数 |
|---|---|
| `--source ./mind.json` | source：文件内容对应的 JSON 字符串 |
| `--topic '{"id":"temp","type":"topic","text":"任务"}'` | node：JSON 字符串，不是嵌套对象 |
| `--text ''` | patch：字符串 `{"text":""}`，不是 null 或缺失 text |

不要把整个 source 再编码成带外层引号的 JSON 字符串文件，也不要手写 MindCreateRequest、uid 或组织包装。回参的数组和 ID 映射仍为原生数组/对象，不转字符串。

## 一页十节点示例

下面内容保存为 `mind.json`：一个根主题加九个子主题，总计十个；n9 特意保留空文本。

```json
{
  "schemaVersion": "1.0",
  "catalogVersion": "mind-v1",
  "pages": [
    {
      "id": "p1",
      "title": "项目计划",
      "nodes": [
        {"id": "root", "type": "topic", "text": "项目计划"},
        {"id": "n1", "type": "topic", "text": "目标", "parentId": "root"},
        {"id": "n2", "type": "topic", "text": "需求", "parentId": "root"},
        {"id": "n3", "type": "topic", "text": "方案", "parentId": "root"},
        {"id": "n4", "type": "topic", "text": "开发", "parentId": "root"},
        {"id": "n5", "type": "topic", "text": "测试", "parentId": "root"},
        {"id": "n6", "type": "topic", "text": "发布", "parentId": "root"},
        {"id": "n7", "type": "topic", "text": "风险", "parentId": "root"},
        {"id": "n8", "type": "topic", "text": "跟进", "parentId": "root"},
        {"id": "n9", "type": "topic", "text": "", "parentId": "root"}
      ]
    }
  ]
}
```

本地预览：

```bash
dws mind create --name '项目计划' --source ./mind.json --request-id mind-create-001 --dry-run --format json
```

预览应返回 pageCount=1、topicCount=10、executed=false；这不是远端创建证明。正式创建位置、确认、返回 ID 与回读方式见操作入口。创建后使用 pageIdMap/nodeIdMap 或查询结果中的真实 ID，不能直接用 p1、root、n1 等临时值编辑文件。
