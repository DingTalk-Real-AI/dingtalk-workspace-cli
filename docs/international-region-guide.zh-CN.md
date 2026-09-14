# DWS 国际版（DingTalk `.io`）使用手册

本手册适用于使用钉钉国际版账号登录并调用国际站服务的用户。

## 核心规则

- `dws auth login --intl` 创建或刷新国际版登录，使用 `*.dingtalk.io` 登录、鉴权和 MCP 服务。
- 不传 `--intl` 时仍使用国内钉钉 `*.dingtalk.com`，原有链路保持不变。
- `--intl` 只用于登录命令。登录完成后，`contact`、`calendar`、`doc` 等业务命令不需要再传该参数。
- 每个 Token 会记录登录区域。执行业务命令时，DWS 根据当前或 `--profile` 指定的账号自动选择 `.com` 或 `.io` 网关。
- `--international` 是 `--intl` 的兼容别名；新脚本推荐使用较短的 `--intl`。

## 确认当前版本支持国际版

运行：

```bash
dws auth login --help
```

帮助中应包含：

```text
--intl
--international
```

从源码分支验证时，先在仓库根目录构建，并始终使用本次构建的 `./dws`，避免误用系统中已安装的旧版本：

```bash
make build
./dws auth login --help
```

## 国际版登录

### 浏览器登录

```bash
dws auth login --intl
```

DWS 会打开国际版登录页面。完成扫码或账号授权后，登录结果会保存为本机 profile。

### 设备码登录

适用于 SSH、容器或没有可用浏览器的环境：

```bash
dws auth login --intl --device
```

按照终端提示，在另一台可打开浏览器的设备上完成授权。

### 使用自有应用凭证完成用户 OAuth

```bash
dws auth login --intl \
  --client-id <APP_KEY> \
  --client-secret <APP_SECRET>
```

该模式仍然需要用户在浏览器中完成 OAuth 授权，不是无用户授权的 `client_credentials` 登录。应用必须在国际版开放平台正确配置回调地址和所需权限。不要在命令历史、日志或 PR 中提交真实的 AppSecret。

## 验证登录和业务调用

查看当前登录状态：

```bash
dws auth status --format json
```

列出本机全部账号并找到当前 profile：

```bash
dws profile list --format json
```

执行一个只读命令验证国际链路，例如：

```bash
dws contact user get-self
```

登录状态正常但业务命令提示组织未开通 CLI 时，需要由国际版组织管理员在国际版开发者平台开启 CLI 访问或完成授权审批。

## 国内版和国际版账号并存

可以在同一台机器上分别登录国内版和国际版账号：

```bash
# 国内版（.com）
dws auth login

# 国际版（.io）
dws auth login --intl

# 查看稳定的 profile 选择器
dws profile list --format json
```

持久切换账号：

```bash
dws profile switch <corpId>:<userId>
```

切回上一个账号：

```bash
dws profile switch -
```

只为单次命令指定账号，不修改默认账号：

```bash
dws --profile <corpId>:<userId> contact user get-self
```

DWS 会按照选中 profile 的 Token 区域自动选择 `.com` 或 `.io`，不需要在业务命令上追加 `--intl`。

## 使用独立配置目录进行验证

如果不希望测试登录影响日常使用的 `~/.dws`，可以指定独立配置目录：

```bash
DWS_CONFIG_DIR=/tmp/dws-intl-smoke ./dws auth login --intl
DWS_CONFIG_DIR=/tmp/dws-intl-smoke ./dws auth status --format json
DWS_CONFIG_DIR=/tmp/dws-intl-smoke ./dws contact user get-self
```

请在三条命令中使用同一个 `DWS_CONFIG_DIR`。验证源码分支时使用 `./dws`；验证已安装版本时可改为 `dws`。

## 预发参数（仅维护者）

普通国际版用户只需要 `--intl`，不要配置 `--pre-url` 或 `--mcp-url`。

维护者验证预发登录/MCP 链路时可以使用：

```bash
dws auth login --intl --pre-url https://pre-login.dingtalk.io
```

也可以传入对应的 `pre-mcp.*` 地址；DWS 会推导配套的 `pre-login.*` / `pre-mcp.*` 地址。`--mcp-url` 用于显式覆盖本次登录的 MCP base URL。

预发环境可能只对内网或特定测试账号开放。`--pre-url` 主要服务于 MCP 托管凭证登录流程；除非预发 API 契约已经明确支持，否则不要把它与自有 `--client-id/--client-secret` 直连模式组合使用。

## 常见问题

### 仍然打开 `.com` 登录页面

1. 运行 `dws auth login --help`，确认当前二进制包含 `--intl`。
2. 从源码验证时使用 `./dws`，不要误用 PATH 中的旧版本。
3. 确认实际执行的是 `dws auth login --intl`，而不是普通 `dws auth login`。

### 业务命令似乎使用了错误区域

先检查当前账号：

```bash
dws profile list --format json
```

然后使用精确的 `<corpId>:<userId>` 切换或通过全局 `--profile` 单次指定。对于在区域字段引入前生成的历史 Token，建议使用正确的登录方式重新授权：国际账号执行 `dws auth login --intl`，国内账号执行 `dws auth login`。

### 登录成功但提示没有权限

这通常是组织 CLI 准入或应用授权问题，不代表区域路由失败。请确认目标组织已开启 CLI 访问，并且当前应用拥有命令所需权限。

### 是否需要手工修改 `~/.dws/mcp_url`

不需要。正常使用应通过 `dws auth login` 或 `dws auth login --intl` 建立登录态；业务命令会根据选中的 Token/profile 自动路由。手工修改配置只适用于明确了解目标环境的维护者调试场景。

## 命令速查

| 场景 | 命令 |
|---|---|
| 国内版浏览器登录 | `dws auth login` |
| 国际版浏览器登录 | `dws auth login --intl` |
| 国际版设备码登录 | `dws auth login --intl --device` |
| 查看登录状态 | `dws auth status --format json` |
| 查看所有账号 | `dws profile list --format json` |
| 持久切换账号 | `dws profile switch <corpId>:<userId>` |
| 切回上一个账号 | `dws profile switch -` |
| 单次指定账号 | `dws --profile <corpId>:<userId> <command>` |
