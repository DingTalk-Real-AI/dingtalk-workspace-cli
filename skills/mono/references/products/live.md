# 直播 (live) 命令参考

查询当前用户的钉钉直播列表 / 信息。当前能力仅查询，不含开播/控制。

## 命令总览

### 查看我的直播列表
```
Usage:
  dws live stream list [flags]
Example:
  dws live stream list
  dws live stream list --format json
```
无业务 flag，仅全局 flag。唯一命令是 `dws live stream list`；`dws live list` 不是可用别名（会返回 validation error: use dws live stream list），不要使用。

## 意图判断

- 用户说"我的直播/直播列表/有哪些直播/查直播" → `live stream list`

## 核心工作流

```bash
# 列出我的直播
dws live stream list --format json
```

## 注意事项

- 该产品当前只提供直播列表查询，不支持创建/开播/会中控制。
- 唯一命令是 `dws live stream list`；`dws live list` 不可用（返回 validation error）。
