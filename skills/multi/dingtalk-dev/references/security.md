# 安全配置

> 安全配置=应用的 IP 白名单 / 登录重定向 / 端内免登 URL；见 SKILL.md 概念地图。

`dws dev app security config` 配 IP 白名单（`--ip-whitelist`）、登录重定向（`--redirect-urls`）、端内免登（`--sso-urls`）。参数用 `dws dev app security config --help` 查询；至少给一个配置字段。

覆盖语义：未提供的字段不动；显式提供的列表是整组覆盖（传入即全量替换该项，不是追加）——要保留旧值就把旧值一起带上。

## 发现命令

调用任何方法前先查清楚再敲：

```
# 浏览命令组下的子命令与 flag
dws dev app security --help

# 查某方法的必填参数、类型、默认值
dws dev <command-path> --help
```

按 `--help` 输出构造 flag；不要凭旧 schema 名称猜参数。
