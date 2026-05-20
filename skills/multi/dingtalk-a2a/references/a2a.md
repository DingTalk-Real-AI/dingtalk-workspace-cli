# A2A Agent 协作 (a2a) 命令参考

通过 A2A 协议发现 Agent 并进行协作通信。协议为 A2A v1.0（JSON-RPC over HTTP，流式为 SSE）。与 `chat` 的区别：`chat` 面向钉钉会话/群聊；`a2a` 面向 Agent 发现与协作通信，二者场景不同。

## 前置条件

- **登录**：认证由系统自动管理，无需手动登录。请求会携带 `x-user-access-token`。
- **默认地址**：`https://mcp-gw.dingtalk.com`；可通过环境变量 **`DWS_A2A_GATEWAY`** 覆盖（例如本地联调 `http://127.0.0.1:18080`）。

## 命令总览

```
dws a2a
  agents   list | info
  send     （同步默认 | --stream）
```

---

## agents（Agent 发现）

Agent 列表由服务端 **`a2a/agents`** 路径提供。`agents list` 始终从服务端获取当前列表，无 `--refresh` 标志。`agents info --agent <name>` 可查看指定 Agent 详情（`--agent` 为 Agent **名称**，与 `list` 中「名称」列一致）。

### 列出可用 Agent

```
Usage:
  dws a2a agents list [flags]
Example:
  dws a2a agents list
```

### 查看指定 Agent 详情

```
Usage:
  dws a2a agents info [flags]
Example:
  dws a2a agents info --agent ai-search
Flags:
      --agent string   Agent 名称 (必填)
```

---

## send（向 Agent 发消息）

仅两种模式：**同步（默认）**或 **流式 `--stream`**。`--text` 与 `--data` 互斥。

```
Usage:
  dws a2a send [flags]
Example:
  dws a2a send --agent ai-search --text "搜索钉钉开放平台文档"
  dws a2a send --agent ai-search --text "分析代码" --stream
  dws a2a send --agent ai-search --data '{"query":"test"}'
Flags:
      --agent string        目标 Agent 名称（必填，对应 agents 列表中的名称）
      --text string         纯文本消息（与 --data 二选一）
      --data string         结构化 JSON 字符串，作为 data Part（与 --text 二选一）
      --stream              SSE 流式：实时输出进度与产物片段
      --context-id string   会话上下文 ID（多轮续接）
```

注意：

- **同步**：阻塞直到返回完整结果（适合短请求）。
- **`--stream`**：长连接 SSE；人类可读时进度在 **stderr**（`[进度] ...`），正文片段在 **stdout**。长请求请适当加大全局超时，例如 `--timeout 120`（秒）。需要机器可读事件可加 `--format json`（每行/每条为 JSON 事件）。

---

## 输出格式与全局参数

- 与各产品一致，支持 **`-f json` / `--format json`** 输出可解析 JSON（`agents list`、`agents info`、`send` 等均适用）。
- **`dws a2a send ... --stream`**：默认人类可读流式输出；若加 `--format json`，输出为 **SSE 事件级 JSON**（便于脚本解析），与「表格 human 输出」场景不同。
- 全局 **`--timeout`**（秒）作用于非 SSE 的 HTTP 请求；**流式**连接主要由客户端与上下文控制，长流式务必显式加大超时。

更多全局标志见 `dws-shared/references/global-reference.md`。
