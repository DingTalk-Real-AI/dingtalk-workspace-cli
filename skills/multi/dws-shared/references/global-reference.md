# 全局参考

## 认证

认证由系统自动管理，无需手动登录。

```bash
# 查看当前认证状态
dws auth status

# 退出登录
dws auth logout
```

系统自动管理 token 刷新，日常使用无需干预。

| Token | 有效期 | 说明 |
|-------|--------|------|
| Access Token | 2 小时 | 调用 API 的凭证，过期自动刷新 |
| Refresh Token | 30 天 | 换新 Access Token，使用后轮转 |

30 天内使用一次即自动续期。

### 认证失败处理
- 命令返回 `AUTH_TOKEN_EXPIRED` / `USER_TOKEN_ILLEGAL` / "Token验证失败" → 请重新执行上一条命令（最多重试两次）

### Headless 环境 (CI/CD)

```bash
# 桌面: 导出凭证
dws auth export > credentials.json

# 服务器: 设置环境变量
export DINGTALK_CREDENTIALS_FILE=~/.config/dingtalk/credentials.json
```
refresh_token 单设备独占，远程刷新后源设备凭证失效。


## 全局标志

| 标志 | 短名 | 说明 | 默认 |
|------|:---:|------|------|
| `--format` | `-f` | 输出格式: json / table / raw | table |
| `--verbose` | `-v` | 详细日志 | false |
| `--debug` | | 调试日志 | false |
| `--yes` | `-y` | 跳过确认提示 | false |
| `--dry-run` | | 预览操作不执行 | false |
| `--timeout` | | HTTP 超时 (秒) | 30 |
| `--token` | | API Token (覆盖配置) | 无 |
| `--mock` | | Mock 数据 (开发用) | false |

## 输出格式

### --format json (机器可读, Agent 必须使用)

```json
{"success": true, "body": {...}}
```

### --format table (人类可读, 默认)

```
已创建 AI 表格 "项目管理" (UUID: abc123)

下一步:
  dws aitable base get --base-id abc123
```

## 环境变量

| 变量 | 说明 |
|------|------|
| `DINGTALK_TOKEN` | API Token (最高优先级) |
| `DINGTALK_CREDENTIALS_FILE` | 导出的凭证文件路径 |
| `DINGTALK_MCP_URL` | MCP Server URL (覆盖内置) |

凭证优先级: `--token` > `DINGTALK_TOKEN` > `DINGTALK_CREDENTIALS_FILE` > OAuth token.json

## 命令自省

不确定某个命令的参数时，用 `--help` 查看：

```bash
dws calendar event create --help
dws chat message send --help
dws aitable record query --help
```

`--help` 会显示所有 flag、类型、默认值和必填标记，比凭记忆猜参数更可靠。
