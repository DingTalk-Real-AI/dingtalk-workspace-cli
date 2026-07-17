# 查询命令帮助

## 查询命令帮助

当你不确定某个命令的具体参数、格式或可选项时，**优先执行 `--help` 查询**，不要猜测参数名或凭记忆编造。

```bash
# 查看 doc 下所有子命令
dws doc --help

# 查看具体命令的完整参数说明
dws doc read --help
dws doc create --help
dws doc block insert --help

# 查看子命令组下的所有命令
dws doc block --help
dws doc media --help
```

规则：

- 参数名不确定时 → 先 `--help`，再调用
- 报错 "unknown flag" 时 → `--help` 确认正确的 flag 名称
- 不确定某个功能是否存在时 → `dws doc --help` 查看命令列表
