# 版本管理

| 命令 | 用途 | 必填参数 |
|---|---|---|
| `doc version save` | 手动保存文档版本快照 | `--node` |
| `doc version list` | 查看文档历史版本列表 | `--node` |
| `doc version revert` | 回滚文档到指定版本（不可逆） | `--node` `--version` `--yes` |

#### 手动保存文档版本

```
Usage:
  dws doc version save [flags]
Example:
  dws doc version save --node <nodeId>
Flags:
      --node string    文档 ID 或 URL (必填)
```

> 仅支持 adoc 类型文档。保存后会创建一个 USER_SAVE 类型的版本记录。

#### 查看文档历史版本列表

```
Usage:
  dws doc version list [flags]
Example:
  dws doc version list --node <nodeId>
  dws doc version list --node <nodeId> --limit 10
Flags:
      --node string      文档 ID 或 URL (必填)
      --limit int        返回版本数量上限 (可选)
      --cursor string    分页游标 (可选)
```

#### 回滚文档到指定版本

> **CAUTION:** 不可逆操作 — 执行前必须向用户确认。

```
Usage:
  dws doc version revert [flags]
Example:
  dws doc version revert --node <nodeId> --version 3 --yes
Flags:
      --node string      文档 ID 或 URL (必填)
      --version int      目标版本号 (必填，从 version list 获取)
      --yes              跳过确认提示 (非交互终端必须传)
```
