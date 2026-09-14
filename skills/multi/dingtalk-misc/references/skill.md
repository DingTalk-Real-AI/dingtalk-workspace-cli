# skill — DWS 技能管理

> 这是元能力：只管理 dws 平台上的技能资源。Distinct from `dingtalk-shared`（钉钉产品路由入口）、其他 `dingtalk-*` 产品 skill（执行具体业务能力）、本地 Codex skill 开发。命令前缀：`dws skill`。

## 意图表

| 用户说 | 命令 |
|---|---|
| "搜索技能 / 找技能" | `dws skill search --query "<关键词>" [--source DingtalkMarket\|OrgInternal]` |
| "下载技能包" | `dws skill get --skill-id <skillId>` |
| "安装市场技能" | `dws skill install <skillId> <target>` |
| "安装 DWS mono/multi skills" | `dws skill setup --mode <mono\|multi> --target <target>` |

## 约束

- `skillId` 必须来自 `skill search` 返回，不能用名称代替。
- `skill install` 的 `skillId` 与 `target` 是位置参数，不是 `--skill-id` flag。
- `skill setup --mode multi` 可用 `--skill/-s` 只装指定产品，或用 `--exclude/-x` 排除产品，两者不能同时使用。
- 搜索结果中的 `securityStatus` 需要如实展示；状态异常时不要把安装描述为已通过安全检测。
- 开源 CLI 不提供技能发布/上传命令；发布需求应转到对应的技能市场发布流程。

## 兼容提示

- `dws skill find` → `dws skill search --query <关键词>`
- `dws skill add` → `dws skill install <skillId> <target>`

---

## 命令参考

### 搜索技能

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

### 下载技能包

```
Usage:
  dws skill get --skill-id <skillId>
Flags:
      --skill-id string   技能 ID (必填)
```

成功后返回本地临时目录路径，供检查或后续安装使用。

### 安装市场技能

```
Usage:
  dws skill install <skillId> <target>
Example:
  dws skill install skill-123 claude
  dws skill install skill-123 qoder
  dws skill install skill-123 .
```

`skillId` 来自搜索结果；`target` 使用 `skill install --help` 列出的 Agent 名称，或用 `.` 安装到当前目录。两个值均为位置参数。

### 部署 DWS 内置技能

```
Usage:
  dws skill setup [flags]
Example:
  dws skill setup --mode mono
  dws skill setup --mode multi --target qoder
  dws skill setup --mode multi -s aitable -s calendar --target qoder
  dws skill setup --mode multi -x live -x devdoc --target qoder
Flags:
      --mode string       mono | multi
      --target string     目标 Agent，默认 all
      --source string     显式 skill 源目录
  -s, --skill strings     multi 模式只安装指定子 skill
  -x, --exclude strings   multi 模式排除指定子 skill
      --yes               仅脚本使用：跳过确认（删除仍先备份）
```

`--skill` 与 `--exclude` 互斥。未指定 `--source` 时使用当前二进制内置的 skill 版本。setup 会清理对面模式残留与不在 bundle 内的过期 skill；这些目录在确认前逐条列出，删除前先备份到 `~/.dws/skill-backups/`，备份失败的目录保留原样。代用户执行时不要附加 `--yes` 绕过确认。

## 上下文传递

| 操作 | 从返回中提取 | 用于 |
|---|---|---|
| `skill search` | `skillId`、版本、来源、安全状态 | 下载或安装 |
| `skill get` | 临时目录 | 本地检查 |
| `skill install` | 安装目标与结果 | 确认指定 Agent 已安装 |
| `skill setup` | 已安装/保留/跳过的 skill 列表 | 验证 mono/multi 部署 |
