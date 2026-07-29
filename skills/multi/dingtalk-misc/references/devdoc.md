# devdoc — 开放平台文档

## 搜索开放平台文档
```
Usage:
  dws devdoc article search [flags]
Example:
  dws devdoc article search --query "OAuth2 接入" --page 1 --size 10 --format json
Flags:
      --query string     搜索关键词 (必填)
      --page string      页码 (默认 1)
      --size string      每页数量 (默认 10)
```

## 意图判断

- 用户说"开发文档 / API 文档 / 接口文档 / 调用报错" → `devdoc article search`

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|------|-------------|------|
| `devdoc article search` | 文档链接 | 直接展示给用户 |

---

## SKILL 摘要（原 dingtalk-devdoc/SKILL.md 正文）

## 意图表

| 用户说 | 命令 |
|--------|------|
| "查 OAuth2 接入文档" | `dws devdoc article search --query "OAuth2 接入"` |
| "API 调用报错怎么办" | `dws devdoc article search --query "<报错关键词>"` |
| "开放接口文档" | `dws devdoc article search --query "<接口名或场景>"` |

## 跨产品协作

- 钉钉云文档（个人 / 企业内文档）→ 切到 `dingtalk-doc`
## 局部意图与 Recipe

- [局部意图消歧](devdoc-intent-guide.md)。
