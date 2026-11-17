---
category: Added
---

- **指令裁剪策略（企业定制）** — 企业发行版可通过 `edition.Hooks.Restrict` 把一组规则编译进二进制，限制 Agent 能执行和能发现的命令。规则支持路径 `Allow`/`Deny`（glob）、`MaxEffect`、`MaxRisk`、`AllowUnannotated` 和 `AllowChannels`；规则内各条件须同时满足，多条规则任一条通过即放行。被拒绝的命令返回 `command_denied`（validation，退出码 3），且不出现在 `dws schema`、帮助和 Shell 补全中。规则可用 `DenyMessage` 配置拒绝指引（如找谁开通），它会附加到拒绝错误的 `hint` 和 `details.deny_message`。未配置 `Restrict` 时行为与此前完全一致。
- **通道入口须显式点名** — `api`、`mcp published invoke`、`mcp url get`、`upgrade` 和 `plugin` 管理命令不会被路径通配或风险上限放行，必须在规则的 `AllowChannels` 中按精确路径点名。
- **受限版不加载本机扩展** — 策略启用后不加载运行时插件、插件 Hook 和用户自定义 shortcut。
- **新增诊断命令 `dws config policy show`** — 显示策略状态、规则、被拒绝的命令、通道入口状态和诊断提示；只读取本机状态，返回结构已发布到 Schema（`config.policy_show`）。策略生效时 Schema 的目录级输出额外带 `policy_view_hash`，标识当前规则集下的可见面。详见 `docs/command-restriction-policy.md`。
