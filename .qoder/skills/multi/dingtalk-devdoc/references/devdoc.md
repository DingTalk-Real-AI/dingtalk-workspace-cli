# 开放平台文档 (devdoc) 命令参考

搜索钉钉**开放平台**开发文档，用于回答开发者关于 OpenAPI、字段、错误码、接入指南、配额等技术问题。

## 命令总览

### 搜索开发文档
```
Usage:
  dws devdoc article search [flags]
Example:
  dws devdoc article search --query "OAuth2 接入"
  dws devdoc article search --query "消息卡片" --page 2 --size 5
  dws devdoc article search --query "机器人" --size 10
Flags:
      --query string     搜索关键词 (必填)
      --page int         分页页码 (从 1 开始，默认 1)
      --size int         分页大小 (默认 10)
```

## 意图判断

用户问开放平台 API / 字段 / 错误码 / SDK / 鉴权 / 回调 / 配额相关的技术细节:
- 走 `devdoc article search`，把用户问的关键短语作为 `--query`

关键区分:
- devdoc(钉钉**开放平台**开发者文档，面向研发) vs doc(钉钉在线文档，面向普通用户内容)
- devdoc 只做搜索，不做读取；命中条目返回标题、摘要、文档链接，由 Agent 引用链接或进一步浏览

## 核心工作流

```bash
# 开发者问"OAuth2 怎么接"
dws devdoc article search --query "OAuth2 接入" --format json

# 命中结果多时翻页
dws devdoc article search --query "消息卡片" --page 2 --size 5 --format json

# 查错误码 / 字段含义
dws devdoc article search --query "errcode 40078" --format json
```

## 注意事项

- `--query` 必填；建议传用户原话里的关键名词（API 名、错误码、能力名），不要过度改写
- 返回按相关性排序，默认 `--size 10`；要拿更多结果时先翻页，再考虑换关键词
- 命中结果里的链接是钉钉开放平台公开文档，可直接给用户做参考
- 不要把 devdoc 用来查业务数据（那是 aitable / doc / report 的事）；devdoc 只查**官方开发者文档**
