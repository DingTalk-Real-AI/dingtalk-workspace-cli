# 钉钉 AI 群机器人快速上手

10 分钟搭一个自己的钉钉群答疑机器人：群里 @它 提问，它用你本地的 AI（Claude Code / Codex / Qoder 等）回答，支持发文字和报错截图。

只需四步：装工具 → 建机器人 → 接上 AI → 拉进群。

## 第一步：安装 dws

### macOS

打开终端，整段复制执行（Intel / Apple 芯片都适用）：

```bash
ARCH=$(uname -m | sed 's/x86_64/amd64/')
mkdir -p ~/.local/bin
curl -fsSL -o /tmp/dws.tar.gz "https://github.com/PeterGuy326/dingtalk-workspace-cli/releases/download/v1.0.53-dws-devapp/dws-darwin-${ARCH}.tar.gz"
tar xzf /tmp/dws.tar.gz -C ~/.local/bin dws
export PATH="$HOME/.local/bin:$PATH"
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
dws version
```

### Windows

打开 PowerShell，整段复制执行：

```powershell
New-Item -ItemType Directory -Force "$env:USERPROFILE\dws" | Out-Null
Invoke-WebRequest -Uri "https://github.com/PeterGuy326/dingtalk-workspace-cli/releases/download/v1.0.53-dws-devapp/dws-windows-amd64.zip" -OutFile "$env:TEMP\dws.zip"
Expand-Archive -Path "$env:TEMP\dws.zip" -DestinationPath "$env:USERPROFILE\dws" -Force
[Environment]::SetEnvironmentVariable("Path", "$env:Path;$env:USERPROFILE\dws", "User")
```

然后**重新打开一个 PowerShell 窗口**，执行 `dws version` 确认。

> 两个平台装好后都应显示 `dws version v1.0.53-dws-devapp`。其他平台（Linux / Windows ARM）的安装包在 [Releases 页面](https://github.com/PeterGuy326/dingtalk-workspace-cli/releases/tag/v1.0.53-dws-devapp)下载。

### 登录钉钉

```bash
dws auth login
```

按提示扫码登录即可。

## 第二步：创建机器人

一条命令建号（名字、描述可以改成你自己的）：

```bash
dws devapp robot create --app-name 我的智能体 --robot-name 小助手 --desc "群内答疑" --yes --format json
```

返回结果里的 `clientId` 和 `clientSecret` **保存好**，下一步要用。

## 第三步：把机器人接上你本地的 AI

```bash
dws devapp robot connect --channel auto --robot-client-id dingxxxxxxxxxxxxxxxx --robot-client-secret yyyyyyyyyyyyyyyyyyyy
```

- 把 `dingxxxxxxxxxxxxxxxx` 和 `yyyyyyyyyyyyyyyyyyyy` 换成第二步返回的 `clientId` 和 `clientSecret` 的实际值
- `--channel auto` 自动识别你电脑上装的 AI 工具（Claude Code / Codex / Qoder / Gemini 等）
- 这个命令是前台运行的：窗口开着机器人在线，关掉窗口机器人下线

## 第四步：拉进群聊

在钉钉里打开目标群：

**群设置 → 机器人 → 添加机器人 → 在企业机器人里搜"小助手"（你起的名字）→ 添加**

完成。现在在群里 @小助手 提问试试，发文字、发报错截图都能答。

## 进阶配置（可选）

按需加在第三步的命令后面：

| 参数 | 作用 |
|------|------|
| `--knowledge-dir ./docs` | 挂本地知识目录（.md/.txt），回答自动带上你的资料 |
| `--allowed-users 工号1,工号2` | 用户白名单，名单外的人无法触发机器人 |
| `--allowed-groups 群ID` | 群白名单 |
| `--user-rate-limit 0` | 关闭限流（默认每人每分钟 20 条） |

## 常见问题

**执行命令报 `zsh: parse error near '\n'`？**
命令里残留了 `<...>` 尖括号占位符（旧版文档的写法），shell 会把尖括号当成重定向符。把占位符整体替换成实际值、不要保留尖括号，再执行。

**群里 @机器人 没反应？**
确认第三步的 `robot connect` 窗口还开着——关掉窗口机器人就下线了。

**第二步提示"当前用户没有开发者身份"？**
创建应用需要开放平台开发者权限。请企业管理员在钉钉开放平台（open-dev.dingtalk.com）的「权限管理」中把你的账号添加为开发者，然后重试第二步。

**提示找不到 dws 命令？**
macOS 重开一个终端窗口；Windows 重开一个 PowerShell 窗口（安装时改了 PATH，需要新窗口才生效）。

**提示本地没有装 AI 工具？**
机器人背后需要一个本地 AI CLI。推荐先装 [Claude Code](https://claude.com/claude-code) 或 Codex，装好后重新执行第三步。

**机器人回复"调用失败"？**
通常是本地 AI 工具未登录或额度用尽，单独运行一次该 AI 工具确认其本身可用。
