# 技能管理命令参考

## 搜索技能

```
Usage:
  dws skill search [flags]
Example:
  dws skill search --query "周报"
  dws skill search --query "日报" --source OrgInternal
Flags:
      --query string    搜索关键词 (必填)
      --source string   查询范围：DingtalkMarket / OrgInternal；空格分隔
```

从返回中提取真实 `skillId`、名称、版本、来源与 `securityStatus`。兼容入口 `skill find` 只会提示改用 `search`。

## 下载技能包

```
Usage:
  dws skill get --skill-id <skillId>
Flags:
      --skill-id string   技能 ID (必填)
```

成功后返回本地临时目录路径，供检查或后续安装使用。

## 安装市场技能

```
Usage:
  dws skill install <skillId> <target>
Example:
  dws skill install skill-123 claude
  dws skill install skill-123 qoder
  dws skill install skill-123 .
```

`skillId` 来自搜索结果；`target` 使用 `skill install --help` 列出的 Agent 名称，或用 `.` 安装到当前目录。两个值均为位置参数。

## 部署 DWS 内置技能

```
Usage:
  dws skill setup [flags]
Example:
  dws skill setup --mode mono --yes
  dws skill setup --mode multi --target qoder --yes
  dws skill setup --mode multi -s aitable -s calendar --target qoder --yes
  dws skill setup --mode multi -x live -x devdoc --target qoder --yes
Flags:
      --mode string       mono | multi
      --target string     目标 Agent，默认 all
      --source string     显式 skill 源目录
  -s, --skill strings     multi 模式只安装指定子 skill
  -x, --exclude strings   multi 模式排除指定子 skill
      --yes               跳过确认
```

`--skill` 与 `--exclude` 互斥。未指定 `--source` 时使用当前二进制内置的 skill 版本。

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|---|---|---|
| `skill search` | `skillId`、版本、来源、安全状态 | 下载或安装 |
| `skill get` | 临时目录 | 本地检查 |
| `skill install` | 安装目标与结果 | 确认指定 Agent 已安装 |
| `skill setup` | 已安装/保留/跳过的 skill 列表 | 验证 mono/multi 部署 |
