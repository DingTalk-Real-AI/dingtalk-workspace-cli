# 钉钉 AI 群机器人快速上手

10 分钟搭一个自己的钉钉群答疑机器人：群里 @它 提问，它用你本地的 AI（Claude Code / Codex / Qoder 等）回答，支持发文字和报错截图。

只需四步：装工具 → 建机器人 → 接上 AI → 拉进群。

## 第一步：安装 dws

一键脚本会自动下载最新版二进制 + `dingtalk-dev` skill，只需要 curl（无需 go / git）。

### macOS / Linux

打开终端，整段复制执行：

```bash
curl -fsSL https://raw.githubusercontent.com/wxianfeng/dingtalk-workspace-cli/feat/dws-devapp/scripts/install-devapp.sh | sh
```

装完按提示把 `~/.local/bin` 加进 `PATH`（脚本会在末尾提示），然后执行 `dws version` 确认。

### Windows

打开 PowerShell，整段复制执行：

```powershell
irm https://raw.githubusercontent.com/wxianfeng/dingtalk-workspace-cli/feat/dws-devapp/scripts/install-devapp.ps1 | iex
```

然后**重新打开一个 PowerShell 窗口**，执行 `dws version` 确认。

> 能打印出版本号即安装成功（脚本默认装当前最新的预览版）。脚本走 GitHub API 取最新预览版，无需手动填版本号；想钉某个版本可设环境变量 `DEVAPP_VERSION`。

### 登录钉钉

```bash
dws auth login
```

按提示扫码登录即可。

## 第二步：创建机器人

建号是异步的，两步（名字、描述可以改成你自己的）：

```bash
# 1) 提交建号任务，记下返回的 taskId
dws dev app robot submit --name 我的智能体 --robot-name 小助手 --desc "群内答疑" --yes --format json

# 2) 用上一步的 taskId 查结果，直到 status 变成 SUCCESS（还是 WAITING 就过几秒再查一次）
dws dev app robot result --task-id 上一步返回的taskId --format json
```

`status` 变成 `SUCCESS` 后，返回结果里的 `clientId` 和 `clientSecret` **保存好**，下一步要用。

## 第三步：把机器人接上你本地的 AI

```bash
dws dev connect --channel auto --robot-client-id dingxxxxxxxxxxxxxxxx --robot-client-secret yyyyyyyyyyyyyyyyyyyy
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
| `--agent-workdir ./项目目录` | 让机器人在你的项目目录里跑，能读到和终端一样的本地文件（详见下方「机器人答得不如终端准？」） |
| `--knowledge-dir ./docs` | 挂本地知识目录（.md/.txt），回答自动带上你的资料 |
| `--agent-cmd "<命令>"` | 接入内置列表之外的 AI 工具（自研的、或还没内置支持的），详见下方「想用没在列表里的 AI 工具？」 |
| `--allowed-users 工号1,工号2` | 用户白名单，名单外的人无法触发机器人 |
| `--allowed-groups 群ID` | 群白名单 |
| `--user-rate-limit 0` | 关闭限流（默认每人每分钟 20 条） |

### 想用没在列表里的 AI 工具？（自研 / 未内置支持）

`--channel auto` 只认内置的几款工具（Claude Code / Codex / Qoder / Gemini 等）。如果你用的是自研的、或还没内置支持的 AI（比如网易有道龙虾 LobsterAI），用 `--agent-cmd` 把它接进来——只要它能在命令行「一次性」跑（给一段问题、把答案打到标准输出），就能接：

```bash
dws dev connect \
  --agent-cmd "你的AI命令 一次性问答参数" \
  --robot-client-id dingxxxx --robot-client-secret yyyy
```

机器人收到群消息后，会执行 `你的AI命令 一次性问答参数 "用户的问题"`（问题作为最后一个参数追加），把它打印出来的内容当作回复发回群里。

举例：假设龙虾的命令行叫 `lobster`、一次性问答用 `-p` 参数，就写 `--agent-cmd "lobster -p"`。命令里有空格就整体用引号括起来。

## 常见问题

**执行命令报 `zsh: parse error near '\n'`？**
命令里残留了 `<...>` 尖括号占位符（旧版文档的写法），shell 会把尖括号当成重定向符。把占位符整体替换成实际值、不要保留尖括号，再执行。

**群里 @机器人 没反应？**
确认第三步的 `dev connect` 窗口还开着——关掉窗口机器人就下线了。

**第二步提示"当前用户没有开发者身份"？**
创建应用需要开放平台开发者权限。请企业管理员在钉钉开放平台（open-dev.dingtalk.com）的「权限管理」中把你的账号添加为开发者，然后重试第二步。

**提示找不到 dws 命令？**
macOS 重开一个终端窗口；Windows 重开一个 PowerShell 窗口（安装时改了 PATH，需要新窗口才生效）。

**提示本地没有装 AI 工具？**
机器人背后需要一个本地 AI CLI。推荐先装 [Claude Code](https://claude.com/claude-code) 或 Codex，装好后重新执行第三步。

**机器人回复"调用失败"？**
通常是本地 AI 工具未登录或额度用尽，单独运行一次该 AI 工具确认其本身可用。

**机器人答得不如终端准？（同样的问题，终端对、机器人不对）**
这通常不是模型问题，而是"机器人看到的上下文比终端少"：

- **工作目录不同**：默认机器人在一个空白临时目录里跑（为了启动快、回复中立），它看不到你终端所在项目里的文件。要让它和终端读到同样的资料，在第三步加 `--agent-workdir ./你的项目目录`（指到你平时在终端里跑 AI 的那个目录）。
- **知识没挂上**：如果靠的是本地文档/知识库，加 `--knowledge-dir ./docs`（或 `--knowledge-source wiki:<spaceId>`）把资料显式挂给机器人，别指望它自己去翻。
- **模型不同**：机器人默认走一个偏快的小模型；如果你终端用的是更强的模型，给机器人也指定同一个：`--agent-model <模型名>`。
- **回答"水位"上下浮动**：先确认没关 `--agent-memory`（默认开）。Codex 走 app-server thread 续聊；Qoder/Claude Code/CodeBuddy/WorkBuddy 走可恢复会话，其中 Qoder 的映射只保存在当前 DWS 进程内，重启后会重新开始；Gemini 仍是一次性调用。

一句话：让机器人和终端"看到一样的东西、用一样的模型"，差距基本就抹平了。

## 会话指令：`/new` 和 `/clear`

机器人默认记住同一个会话的上下文（多轮对话）。想重置上下文，直接在聊天里发这两个斜杠指令——整条消息就是指令时才生效（普通问题不受影响），不消耗一次 AI 调用，秒回提示：

| 指令 | 作用 |
|------|------|
| `/new`（或 `/start`、`/reset`） | **开启新会话**：之前的上下文不再带入，旧会话保留（agent 支持的话仍可回溯） |
| `/clear` | **清空当前会话**：彻底从头开始 |

两者按各渠道**真实能力**对齐：`/clear` 在 opencode 渠道会真正删除当前会话（调 opencode 的 `DELETE /session/:id`）；Codex / Qoder / Claude 系等驱动接口没有删除原语的渠道，`/clear` 退化为与 `/new` 相同的重置。

**第三步执行完，在蚂蚁钉/开放平台搜不到审批工单？**
这是正常的，不是出错。第三步 `dev connect`（把机器人接到本地 AI）只是用现成机器人的凭证起一条连接、本地转发，**不产生任何审批工单**。会产生审批工单的是第二步「建机器人」（`dev app robot submit`），由平台/管理员审批。所以第三步之后搜不到工单是预期内的。
