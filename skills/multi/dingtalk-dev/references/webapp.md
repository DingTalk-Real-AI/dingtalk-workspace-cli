# 网页应用配置

> 网页应用=应用的能力扩展之一，钉钉内打开的 H5；见 SKILL.md 概念地图。

`dws dev app webapp get` 查配置，`webapp config` 配移动端/PC 首页和管理后台地址。参数用 `dws dev app webapp get --help` 和 `dws dev app webapp config --help` 查询；config 至少给一个配置字段。

- 未配置网页应用前，`get` 返回空对象 `{}`。拿到 `{}` 就是还没配过，走 `webapp config` 首次配置。
- `h5PageType` 未显式传入时不要假设固定默认值；配置后以 `webapp get` 回读为准。

## 发现命令

调用任何方法前先查清楚再敲：

```
# 浏览命令组下的子命令与 flag
dws dev app webapp --help

# 查某方法的必填参数、类型、默认值
dws dev <command-path> --help
```

按 `--help` 输出构造 flag；不要凭旧 schema 名称猜参数。
