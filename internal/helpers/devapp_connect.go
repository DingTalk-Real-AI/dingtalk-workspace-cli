// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helpers

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cobracmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

// devapp_connect wires the channel-aware "建联" (linking) capability into the
// devapp command tree as `dws devapp robot connect`. Provisioning (建号) is NOT
// duplicated here — it reuses devapp's existing `robot create/submit/result`.
// This file holds only the linking half: detect the current agent channel, then
// start a Go-native in-process Stream that forwards @-bot messages to the local
// agent CLI. The Stream forwarder internals live in connect_stream.go.

// connectChannels is the set of supported channels: the external ones plus every
// exec-type agent in agentSpecs (kept in sync automatically).
var connectChannels = func() map[string]struct{} {
	m := map[string]struct{}{"openclaw": {}, "hermes": {}}
	for ch := range agentSpecs {
		m[ch] = struct{}{}
	}
	return m
}()

// resolveConnectChannel resolves the current agent channel using "explicit wins,
// then signal fallback". Priority: --channel flag > DWS_AGENT_CHANNEL env var >
// each agent's known runtime signal. Returns the channel name and the basis for
// the decision (detectedBy, for troubleshooting).
//
// Signals (verified on real runtimes):
//   - openclaw connector injects DINGTALK_AGENT=DING_DWS_CLAW.
//   - WorkBuddy injects WORKBUDDY_CONFIG_DIR / WORKBUDDY_APP_NAME into spawned children.
//   - QoderWork's qodercli injects QODERCLI_INTEGRATION_MODE=qoder_work (and neither QODER_CLI nor CLAUDECODE).
//   - plain Qoder injects QODER_CLI=1 (it is a Claude Code fork, so also CLAUDECODE=1).
//   - pure Claude Code injects only CLAUDECODE=1.
//   - hermes uses the official channel, marked by HERMES_AGENT / HERMES.
func resolveConnectChannel(explicit string) (channel string, detectedBy string) {
	if norm := strings.ToLower(strings.TrimSpace(explicit)); norm != "" && norm != "auto" {
		return norm, "flag:--channel"
	}
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("DWS_AGENT_CHANNEL"))); v != "" {
		return v, "env:DWS_AGENT_CHANNEL"
	}
	// Signal fallback.
	if strings.EqualFold(strings.TrimSpace(os.Getenv("DINGTALK_AGENT")), "DING_DWS_CLAW") {
		return "openclaw", "signal:DINGTALK_AGENT"
	}
	if strings.TrimSpace(os.Getenv("OPENCLAW")) != "" || strings.TrimSpace(os.Getenv("OPENCLAW_GATEWAY")) != "" {
		return "openclaw", "signal:OPENCLAW"
	}
	if strings.TrimSpace(os.Getenv("HERMES_AGENT")) != "" || strings.TrimSpace(os.Getenv("HERMES")) != "" {
		return "hermes", "signal:HERMES"
	}
	// WorkBuddy(CodeBuddy) injects WORKBUDDY_CONFIG_DIR / WORKBUDDY_APP_NAME
	// (verified, pointing at ~/.workbuddy) into the children it spawns. This is a
	// WorkBuddy-specific runtime marker that does not leak globally, so dws can
	// recognise the current host when WorkBuddy spawns it.
	if strings.TrimSpace(os.Getenv("WORKBUDDY_CONFIG_DIR")) != "" || strings.TrimSpace(os.Getenv("WORKBUDDY_APP_NAME")) != "" {
		return "workbuddy", "signal:WORKBUDDY_CONFIG_DIR"
	}
	// QoderWork's qodercli injects QODERCLI_INTEGRATION_MODE=qoder_work (verified)
	// and carries neither QODER_CLI nor CLAUDECODE. Use it to split qoderwork out
	// of the qoder family, avoiding "linking inside QoderWork but reaching Qoder".
	// Must come before the QODER_CLI / CLAUDECODE checks below.
	if strings.EqualFold(strings.TrimSpace(os.Getenv("QODERCLI_INTEGRATION_MODE")), "qoder_work") {
		return "qoderwork", "signal:QODERCLI_INTEGRATION_MODE"
	}
	if strings.TrimSpace(os.Getenv("QODER_CLI")) != "" {
		// Plain Qoder (AI coding IDE): carries QODER_CLI=1. Qoder is a Claude Code
		// fork and also carries CLAUDECODE=1, so this check must precede CLAUDECODE
		// below to avoid misdetecting it as claudecode.
		return "qoder", "signal:QODER_CLI"
	}
	if strings.TrimSpace(os.Getenv("CLAUDECODE")) != "" {
		// Pure Claude Code (not the qoder fork): only CLAUDECODE=1, no QODER_CLI.
		return "claudecode", "signal:CLAUDECODE"
	}
	return "", "undetected"
}

// buildConnectPlan returns the linking plan that wires the bot to a channel's
// local agent. External channels (openclaw/hermes) have bespoke plans; every
// exec-type agent (agentSpecs) shares a generic Stream + headless-CLI plan. Used
// for the --dry-run preview.
func buildConnectPlan(channel, clientID, robotCode string) map[string]any {
	switch channel {
	case "openclaw":
		return map[string]any{
			"method":  "openclaw-connector",
			"summary": "走 dingtalk-openclaw-connector 官方建联，由连接器自己建号 + AI 卡片流式回复（dws 不代建机器人）",
			"steps": []string{
				"按 https://github.com/DingTalk-Real-AI/dingtalk-openclaw-connector 设备码扫码注册机器人",
				"启动 openclaw gateway，由连接器处理消息收发与卡片渲染",
			},
		}
	case "hermes":
		return map[string]any{
			"method":  "official-channel",
			"summary": "走 Hermes 官方 channel 建联，由 Hermes 自己建号 + 原生回复（dws 不代建机器人）",
			"steps": []string{
				"运行 `hermes gateway setup` → 选 DingTalk → QR Code Scan 扫码授权",
				"`hermes gateway restart`，直接在钉钉里跟新机器人对话",
				"回复打了 Done 表情却不显示/卡在'数据加载中'：钉钉按 AI 助理应答窗口渲染回复，超窗的纯文本会被丢弃；回复慢的 agent 需在 hermes 侧启用 AI 卡片（config.yaml 配 platforms.dingtalk.extra.card_template_id），由卡片先占位再流式出字",
			},
		}
	}
	if spec, ok := agentSpecs[channel]; ok {
		if channel == "codex" && codexAppServerPlanEnabled() {
			return map[string]any{
				"method":  "stream-bridge-codex-app-server",
				"summary": "Go 原生 Stream 建联，优先转发到本地 Codex app-server 的 thread/turn 协议；失败时降级 codex exec",
				"steps": []string{
					"自动定位 Codex CLI（DWS_AGENT_CMD > PATH），默认带 --skip-git-repo-check 避免空白工作目录失败",
					"用 clientId/clientSecret 起 Stream，注册 TOPIC_ROBOT 回调",
					"收到消息 → 按 conversationId 映射/恢复 Codex thread → turn/start 获取结构化增量与完成事件",
					"app-server 失败时降级为 codex exec 一次性回复",
					"经 AI 卡片或 sessionWebhook 把回复发回钉钉，并记录发送成功/失败",
				},
			}
		}
		return map[string]any{
			"method":  "stream-bridge",
			"summary": fmt.Sprintf("Go 原生 Stream 建联，转发到本地 %s 的无头 CLI（每条消息起一个新实例，可 7×24 无人值守）", spec.app),
			"steps": []string{
				"自动定位 agent CLI（DWS_AGENT_CMD > PATH > app 自带），缺包管理器装的会自动安装、装不了的提示安装",
				"用 clientId/clientSecret 起 Stream，注册 TOPIC_ROBOT 回调",
				"收到消息 → 调该 agent 的无头 CLI（如 claude -p / codex exec / codebuddy -p）→ stdout 作为回复",
				"经 sessionWebhook 把回复发回钉钉",
			},
		}
	}
	return map[string]any{"method": "unknown"}
}

// connectExternalCommand returns the connector command (argv) for channels that
// must be launched by an external process. Resolution priority: the
// DWS_CONNECT_CMD env var (space-separated, for customisation/testing, applies
// to all channels) > openclaw's built-in gateway. The stream-bridge channels
// (qoder/qoderwork/claudecode/workbuddy) use the Go-native in-process Stream
// (see connect_stream.go) and return no external command (nil). hermes uses the
// official channel and also has no built-in external command. Pure function,
// side-effect free, for easy unit testing.
func connectExternalCommand(channel string) []string {
	if v := strings.TrimSpace(os.Getenv("DWS_CONNECT_CMD")); v != "" {
		return strings.Fields(v)
	}
	switch channel {
	case "openclaw":
		// openclaw is taken over by the external connector: write credentials into
		// openclaw.json, then restart the gateway.
		return []string{"openclaw", "gateway", "restart"}
	default:
		// stream-bridge channels go Go-native; hermes etc. have no built-in command
		// and need DWS_CONNECT_CMD.
		return nil
	}
}

// launchConnector wires the bot to the local agent per channel, running in the
// foreground until interrupted. Dispatch priority:
//  1. external connector (DWS_CONNECT_CMD override or openclaw gateway) → os/exec
//     child, credentials injected via CID/SEC/DWS_AGENT_CHANNEL;
//  2. stream-bridge channels (qoder/qoderwork/claudecode/workbuddy) → Go-native
//     in-process Stream + forwarder, no node/external-script dependency;
//  3. others (hermes etc.) → no built-in linking, advise DWS_CONNECT_CMD.
func launchConnector(cmd *cobra.Command, runner executor.Runner, channel, clientID, clientSecret string, opts connectAgentOptions) error {
	if argv := connectExternalCommand(channel); len(argv) > 0 {
		fmt.Fprintf(cmd.ErrOrStderr(), "[connect] channel=%s 启动外部连接器: %s\n", channel, strings.Join(argv, " "))
		proc := exec.CommandContext(cmd.Context(), argv[0], argv[1:]...)
		proc.Env = append(os.Environ(),
			"CID="+clientID,
			"SEC="+clientSecret,
			"DWS_AGENT_CHANNEL="+channel,
		)
		proc.Stdout = cmd.OutOrStdout()
		proc.Stderr = cmd.ErrOrStderr()
		return proc.Run()
	}

	if isStreamBridgeChannel(channel) {
		// Role config ("digital employee"): when supplied, validate it targets THIS
		// bot and fold its owner / persona / knowledge into the options that were
		// not set explicitly (an explicit flag always wins). Done first so the
		// merged options feed the forwarder, knowledge loading and the gate below.
		if opts.RoleConfigPath != "" {
			role, rerr := LoadRoleConfig(opts.RoleConfigPath)
			if rerr != nil {
				return rerr
			}
			if role.ClientID != clientID {
				return apperrors.NewValidation(fmt.Sprintf(
					"角色配置 %q 绑定 client_id=%s，与本次连接的机器人 %s 不一致；一个角色对应一个机器人，请核对凭证或换用对应的角色配置",
					opts.RoleConfigPath, role.ClientID, clientID))
			}
			opts = applyRoleConfig(opts, role)
			fmt.Fprintf(cmd.ErrOrStderr(), "[connect] 角色已加载：%s（主人=%s｜知识源 +%d｜权限scope=[%s]｜确认策略=%s）\n",
				role.Name, opts.OwnerUserID, len(role.KnowledgeSources), strings.Join(role.AllowedScopes, ","), role.ConfirmPolicy)
		}
		fwd, err := forwarderForChannel(channel, clientID, opts)
		if err != nil {
			return err
		}
		var cardCli *aiCardClient
		replyStyle := "text/markdown"
		if opts.ReplyCard {
			cardCli = newAICardClient(clientID, clientSecret, opts.CardTemplate)
			if cardCli.hasTemplate() {
				replyStyle = "ai-card(thinking→done, 失败回退普通消息)"
			} else {
				replyStyle = "text/markdown + thinking/done表态（配 --card-template 升级为卡片）"
			}
		}
		extras := &connectExtras{persona: opts.Persona}
		if opts.KnowledgeDir != "" {
			kb, kerr := loadKnowledgeBase(opts.KnowledgeDir)
			if kerr != nil {
				return kerr
			}
			extras.kb = kb
			fmt.Fprintf(cmd.ErrOrStderr(), "[connect] 知识库已加载：%d 个片段（%s）\n", len(kb.chunks), opts.KnowledgeDir)
		}
		if opts.KnowledgeSource != "" {
			if kb, kerr := loadConnectKnowledgeSource(cmd, runner, clientID, opts.KnowledgeSource); kerr != nil {
				return kerr
			} else if kb != nil {
				extras.kb = kb
			}
		}
		// Role-supplied knowledge sources are additive: load each and merge its
		// chunks into the same retriever (knowledgeBase is just a chunk slice), so
		// a role can carry several wiki/doc/dir sources without a flag per source.
		for _, src := range opts.KnowledgeSources {
			kb, kerr := loadConnectKnowledgeSource(cmd, runner, clientID, src)
			if kerr != nil {
				return kerr
			}
			if kb == nil {
				continue
			}
			if extras.kb == nil {
				extras.kb = kb
			} else {
				extras.kb.chunks = append(extras.kb.chunks, kb.chunks...)
			}
		}
		if len(opts.AllowedUsers) > 0 || len(opts.AllowedGroups) > 0 || opts.UserRateLimit > 0 {
			extras.gate = newConnectGate(opts.AllowedUsers, opts.AllowedGroups, opts.UserRateLimit)
			fmt.Fprintf(cmd.ErrOrStderr(), "[connect] 访问控制：用户白名单 %d、群白名单 %d、限流 %d 条/分钟/人\n",
				len(opts.AllowedUsers), len(opts.AllowedGroups), opts.UserRateLimit)
		}
		// Confirmation gate ("digital twin"): when an owner + an interactive-card
		// template are configured, action requests route to the owner for approval
		// before executing. Needs a runner to run the approved command directly.
		if opts.OwnerUserID != "" {
			if sender := newDingtalkApprovalCardSender(clientID, clientSecret, opts.ApprovalCardTemplate); sender != nil {
				gate := newApprovalGate(clientID)
				extras.approval = newApprovalOrchestrator(gate, sender, runner, opts.OwnerUserID)
				fmt.Fprintf(cmd.ErrOrStderr(), "[connect] 确认闸已开启：主人=%s（执行类请求先卡片审批）\n", opts.OwnerUserID)
			} else {
				fmt.Fprintf(cmd.ErrOrStderr(), "[connect] 已配置 --owner-user-id 但缺 --approval-card-template，确认闸未开启（无审批卡片模板无法渲染按钮）\n")
			}
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "[connect] channel=%s Go 原生 Stream 建联，转发到 %s，回复样式=%s（Ctrl-C 退出）\n", channel, fwd.label(), replyStyle)
		return runStreamConnector(cmd.Context(), channel, clientID, clientSecret, fwd, cardCli, extras)
	}

	// hermes / openclaw run their own official bot provisioning + reply logic
	// (device-flow QR registration, AI-card streaming). dws must not provision
	// a robot for them or wrap them in the Stream bridge — their bots are a
	// different type and only render through their native pipeline. Point the
	// user at the official tool instead.
	if guidance := officialChannelGuidance(channel); guidance != "" {
		fmt.Fprint(cmd.ErrOrStderr(), guidance)
		return nil
	}

	return apperrors.NewValidation(fmt.Sprintf("渠道 %q 暂无内置建联；用环境变量 DWS_CONNECT_CMD 指定要运行的连接器", channel))
}

// officialChannelGuidance returns onboarding instructions for channels that own
// their bot lifecycle (hermes, openclaw). dws guides the user to the official
// tool rather than provisioning a robot itself, because a dws-provisioned
// "智能体" robot does not render through these channels' native reply path.
func officialChannelGuidance(channel string) string {
	switch channel {
	case "hermes":
		return "[connect] hermes 渠道走 Hermes 官方建联，dws 不代建机器人：\n" +
			"  1. 运行 `hermes gateway setup` → 选 DingTalk → QR Code Scan，用钉钉扫码授权\n" +
			"     （设备码注册，自动把 client_id/secret 写入 ~/.hermes/.env）\n" +
			"  2. `hermes gateway restart`，直接在钉钉里跟新机器人对话\n"
	case "openclaw":
		return "[connect] openclaw 渠道走 dingtalk-openclaw-connector 官方建联，dws 不代建机器人：\n" +
			"  1. 按 https://github.com/DingTalk-Real-AI/dingtalk-openclaw-connector 的设备码扫码注册机器人\n" +
			"  2. 启动 openclaw gateway，由连接器处理 AI 卡片流式回复\n"
	}
	return ""
}

// newDevAppRobotConnectCommand implements `dws devapp robot connect`: the linking
// half of provisioning. It takes an existing robot's credentials — either
// directly via --client-id/--client-secret, or by --unified-app-id (reusing
// devapp's `get_open_dev_app_credentials` to fetch them) — and starts the
// channel-aware Stream connector in the foreground. It never provisions a robot;
// for 建号 run `dws devapp robot create` (or submit/result) first.
func newDevAppRobotConnectCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "建联：把现成机器人接到当前本地 agent（起 Stream，不建号）",
		Long: "用已建好的机器人凭证把它接到当前本地 agent，不做建号。\n" +
			"凭证两种来源：① 直接传 --robot-client-id/--robot-client-secret；② 传 --unified-app-id（复用 devapp credentials get 自动取凭证）。\n" +
			"渠道由 --channel 显式指定，或运行时信号自动探测。\n" +
			"缺凭证请先用 `dws devapp robot create` 建号拿 clientId/clientSecret。",
		Example: "  dws devapp robot connect --channel workbuddy --robot-client-id <id> --robot-client-secret <secret>\n" +
			"  dws devapp robot connect --unified-app-id <ID> --channel qoderwork\n" +
			"  dws devapp robot connect --channel claudecode --robot-client-id <id> --robot-client-secret <secret>",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Daemon dispatch (see connect_daemon.go). --daemon-supervise runs the
			// restart supervisor; --daemon (parent) re-execs into a detached
			// supervisor; --daemon-worker falls through to the normal foreground
			// connector below (it IS the connector, just spawned by the supervisor).
			supervise, _ := cmd.Flags().GetBool(daemonSuperviseFlag)
			if supervise {
				return runSupervisor(cmd)
			}
			channelFlag := devAppStringFlag(cmd, "channel")
			channel, detectedBy := resolveConnectChannel(channelFlag)
			if channel == "" {
				return apperrors.NewValidation("无法探测 agent 渠道；请用 --channel 指定 (openclaw|qoder|qoderwork|hermes|workbuddy|claudecode|codebuddy|codex|gemini|opencode) 或设置 DWS_AGENT_CHANNEL")
			}
			if _, ok := connectChannels[channel]; !ok {
				return apperrors.NewValidation(fmt.Sprintf("未知渠道 %q（支持 openclaw|qoder|qoderwork|hermes|workbuddy|claudecode|codebuddy|codex|gemini|opencode）", channel))
			}

			clientID := devAppStringFlag(cmd, "robot-client-id")
			clientSecret := devAppStringFlag(cmd, "robot-client-secret")
			unifiedAppID := devAppStringFlag(cmd, "unified-app-id")
			opts := connectAgentOptionsFromCommand(cmd)

			// Credential resolution: explicit pair wins; otherwise reuse devapp's
			// credentials get against --unified-app-id.
			resolvedBy := "flag:--robot-client-id/--robot-client-secret"
			if clientID == "" || clientSecret == "" {
				if unifiedAppID == "" {
					return apperrors.NewValidation("需要 --robot-client-id/--robot-client-secret（用现成机器人凭证），或 --unified-app-id（复用 devapp credentials get 自动取凭证）")
				}
				if commandDryRun(cmd) {
					// Dry-run must not call the credentials tool; just preview routing.
					return writeCommandPayload(cmd, map[string]any{
						"channel":          channel,
						"detectedBy":       detectedBy,
						"dryRun":           true,
						"credentialSource": "unified-app-id (credentials get, skipped in dry-run)",
						"unifiedAppId":     unifiedAppID,
						"agent":            connectAgentOptionsPayload(channel, opts),
						"cli":              connectCliStatus(channel),
						"connect":          buildConnectPlan(channel, "", ""),
					})
				}
				id, secret, err := devAppFetchCredentials(runner, cmd, unifiedAppID)
				if err != nil {
					return err
				}
				if id == "" || secret == "" {
					return apperrors.NewInternal("credentials get 未返回 clientId/clientSecret；clientSecret 可能仅建号时返回一次，请改用 --robot-client-id/--robot-client-secret 直接传入")
				}
				clientID, clientSecret = id, secret
				resolvedBy = "unified-app-id:credentials get"
			}

			if commandDryRun(cmd) {
				return writeCommandPayload(cmd, map[string]any{
					"channel":          channel,
					"detectedBy":       detectedBy,
					"dryRun":           true,
					"credentialSource": resolvedBy,
					"clientId":         clientID,
					"agent":            connectAgentOptionsPayload(channel, opts),
					"cli":              connectCliStatus(channel),
					"connect":          buildConnectPlan(channel, clientID, ""),
				})
			}

			// --daemon: detach into a background supervisor that keeps the
			// connector alive 7x24. We resolve credentials/channel first (above) so
			// the parent fails fast on bad input before forking, then re-exec.
			if daemonMode, _ := cmd.Flags().GetBool(daemonFlag); daemonMode {
				return startDaemon(cmd, daemonDirKey(clientID, unifiedAppID), clientID)
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "[connect] channel=%s（%s）凭证来源=%s\n", channel, detectedBy, resolvedBy)
			return launchConnector(cmd, runner, channel, clientID, clientSecret, opts)
		},
	}
	preferLegacyLeaf(cmd)
	// Daemon mode (see connect_daemon.go). --daemon detaches the connector into a
	// self-restarting background supervisor; status/stop are sibling subcommands.
	cmd.Flags().Bool(daemonFlag, false, "守护进程模式：把连接器放到后台常驻（脱离终端、崩溃自拉起），父进程打印 pid/日志路径后退出（Windows 暂不支持）")
	// Internal re-exec mode flags, hidden from help.
	cmd.Flags().Bool(daemonSuperviseFlag, false, "internal: run the daemon supervisor (set automatically by --daemon)")
	cmd.Flags().Bool(daemonWorkerFlag, false, "internal: run a single supervised connector worker (set automatically by the supervisor)")
	_ = cmd.Flags().MarkHidden(daemonSuperviseFlag)
	_ = cmd.Flags().MarkHidden(daemonWorkerFlag)
	cmd.AddCommand(
		newDevAppRobotConnectStatusCommand(),
		newDevAppRobotConnectStopCommand(),
	)
	cmd.Flags().String("channel", "auto", "渠道：auto(默认,自动探测)|openclaw|qoder|qoderwork|hermes|workbuddy|claudecode|codebuddy|codex|gemini|opencode")
	// 用 robot-client-* 而非 client-id/client-secret：后者是全局 OAuth 客户端覆盖
	// 持久 flag（见 internal/app/flags.go），同名会 shadow 全局 flag。这里是要建联的
	// 目标机器人凭证，与 OAuth 客户端是两码事，故独立命名避免撞名。
	cmd.Flags().String("robot-client-id", "", "现成机器人 clientId（AppKey）")
	cmd.Flags().String("robot-client-secret", "", "现成机器人 clientSecret（AppSecret）")
	cmd.Flags().String("unified-app-id", "", "统一应用 ID：复用 devapp credentials get 自动取凭证（替代手填 robot-client-id/secret）")
	cmd.Flags().String("agent-model", "", "覆盖本地 agent 模型（如 claude 的 sonnet/opus；默认用渠道内置模型，求快）；env: DWS_AGENT_MODEL")
	cmd.Flags().String("agent-workdir", "", "本地 agent 的运行目录（放知识文件可给机器人上下文；默认空白临时目录，求快）；env: DWS_AGENT_WORKDIR")
	cmd.Flags().Bool("agent-memory", true, "按会话续聊：同一群/单聊共享 agent 会话上下文（claudecode/codebuddy/workbuddy 支持；--agent-memory=false 关闭）")
	cmd.Flags().Bool("reply-card", true, "用 AI 卡片回复（思考中→完成状态，同官方渠道体验）；卡片失败自动回退普通消息；--reply-card=false 关闭")
	cmd.Flags().String("card-template", "", "AI 卡片模板 ID（开发者后台·本应用·AI 卡片设置里获取；模板按应用授权，强烈建议注册自己应用的模板）；env: DWS_CARD_TEMPLATE")
	cmd.Flags().String("knowledge-dir", "", "答疑知识目录（.md/.txt）：每条消息本地检索 top-k 片段拼进 prompt，agent 仍在空目录跑、不拖慢回复；env: DWS_KNOWLEDGE_DIR")
	cmd.Flags().String("knowledge-source", "", "答疑知识源：wiki:<spaceId> / doc:<docId> 从钉钉知识库拉取并缓存为本地知识（复用 dws doc 能力）；裸值当作本地目录；与 --knowledge-dir 并存；env: DWS_KNOWLEDGE_SOURCE")
	cmd.Flags().String("allowed-users", "", "用户白名单 staffId（逗号分隔），配置后仅名单内用户可触发；env: DWS_ALLOWED_USERS")
	cmd.Flags().String("allowed-groups", "", "群白名单 openConversationId（逗号分隔），配置后仅名单内群可触发；env: DWS_ALLOWED_GROUPS")
	cmd.Flags().Int("user-rate-limit", 20, "单用户每分钟消息上限（防刷；每条消息都是一次 LLM 调用），0 关闭；env: DWS_USER_RATE_LIMIT")
	cmd.Flags().String("owner-user-id", "", "数字分身主人 staffId：开启确认闸后，执行类请求先发卡片给主人审批，同意才执行；env: DWS_OWNER_USER_ID")
	cmd.Flags().String("approval-card-template", "", "确认闸交互卡片模板 ID（带[同意][拒绝]按钮）；与 --owner-user-id 同时配置才开启确认闸；env: DWS_APPROVAL_CARD_TEMPLATE")
	cmd.Flags().String("role-config", "", "数字员工角色配置 YAML：用角色的主人/人设/知识源填充未显式给出的选项（显式 flag 优先）；role 的 client_id 必须与本机器人一致；env: DWS_ROLE_CONFIG")
	return cmd
}

// connectAgentOptionsFromCommand reads the agent tuning flags, falling back to
// the mirrored env vars so connectors run from scripts/services can be
// configured without flags.
func connectAgentOptionsFromCommand(cmd *cobra.Command) connectAgentOptions {
	model := devAppStringFlag(cmd, "agent-model")
	if model == "" {
		model = strings.TrimSpace(os.Getenv("DWS_AGENT_MODEL"))
	}
	workDir := devAppStringFlag(cmd, "agent-workdir")
	if workDir == "" {
		workDir = strings.TrimSpace(os.Getenv("DWS_AGENT_WORKDIR"))
	}
	memory, _ := cmd.Flags().GetBool("agent-memory")
	replyCard, _ := cmd.Flags().GetBool("reply-card")
	// Env kill-switch for scripted/service runs: DWS_REPLY_CARD=0 disables
	// cards regardless of the flag default.
	if v := strings.ToLower(strings.TrimSpace(os.Getenv("DWS_REPLY_CARD"))); v == "0" || v == "false" {
		replyCard = false
	}
	cardTemplate := devAppStringFlag(cmd, "card-template")
	if cardTemplate == "" {
		cardTemplate = strings.TrimSpace(os.Getenv("DWS_CARD_TEMPLATE"))
	}
	// "public" opts into the openclaw shared template explicitly — best-effort
	// only, since shared templates may not render for every app.
	if strings.EqualFold(cardTemplate, "public") {
		cardTemplate = defaultAICardTemplateID
	}
	knowledgeDir := devAppStringFlag(cmd, "knowledge-dir")
	if knowledgeDir == "" {
		knowledgeDir = strings.TrimSpace(os.Getenv("DWS_KNOWLEDGE_DIR"))
	}
	knowledgeSource := devAppStringFlag(cmd, "knowledge-source")
	if knowledgeSource == "" {
		knowledgeSource = strings.TrimSpace(os.Getenv("DWS_KNOWLEDGE_SOURCE"))
	}
	users := devAppStringFlag(cmd, "allowed-users")
	if users == "" {
		users = os.Getenv("DWS_ALLOWED_USERS")
	}
	groups := devAppStringFlag(cmd, "allowed-groups")
	if groups == "" {
		groups = os.Getenv("DWS_ALLOWED_GROUPS")
	}
	rateLimit, _ := cmd.Flags().GetInt("user-rate-limit")
	if !cmd.Flags().Changed("user-rate-limit") {
		if v := strings.TrimSpace(os.Getenv("DWS_USER_RATE_LIMIT")); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n >= 0 {
				rateLimit = n
			}
		}
	}
	owner := devAppStringFlag(cmd, "owner-user-id")
	if owner == "" {
		owner = strings.TrimSpace(os.Getenv("DWS_OWNER_USER_ID"))
	}
	approvalTemplate := devAppStringFlag(cmd, "approval-card-template")
	if approvalTemplate == "" {
		approvalTemplate = strings.TrimSpace(os.Getenv("DWS_APPROVAL_CARD_TEMPLATE"))
	}
	roleConfig := devAppStringFlag(cmd, "role-config")
	if roleConfig == "" {
		roleConfig = strings.TrimSpace(os.Getenv("DWS_ROLE_CONFIG"))
	}
	return connectAgentOptions{Model: model, WorkDir: workDir, Memory: memory,
		ReplyCard: replyCard, CardTemplate: cardTemplate,
		KnowledgeDir:    knowledgeDir,
		KnowledgeSource: knowledgeSource,
		AllowedUsers:    splitCommaList(users), AllowedGroups: splitCommaList(groups),
		UserRateLimit:        rateLimit,
		OwnerUserID:          owner,
		ApprovalCardTemplate: approvalTemplate,
		RoleConfigPath:       roleConfig}
}

// connectAgentOptionsPayload renders the effective agent tuning for the
// dry-run preview, including whether session memory actually applies to the
// chosen channel (the qoder family and codex/gemini/opencode are stateless —
// their CLIs have no addressable session ID).
func connectAgentOptionsPayload(channel string, opts connectAgentOptions) map[string]any {
	spec, ok := agentSpecs[channel]
	memory := "unsupported"
	if channel == "codex" && codexAppServerPlanEnabled() {
		if opts.Memory {
			memory = "per-conversation-app-server"
		} else {
			memory = "disabled"
		}
	} else if ok && spec.ccSessions {
		if opts.Memory {
			memory = "per-conversation"
		} else {
			memory = "disabled"
		}
	}
	model := opts.Model
	if model == "" {
		model = "(channel default)"
	}
	workDir := opts.WorkDir
	if workDir == "" {
		workDir = "(clean temp dir)"
	}
	replyStyle := "text/markdown"
	if opts.ReplyCard {
		if opts.CardTemplate != "" {
			replyStyle = "ai-card"
		} else {
			replyStyle = "text/markdown + thinking/done表态（配 --card-template 升级为卡片）"
		}
	}
	return map[string]any{"model": model, "workdir": workDir, "memory": memory, "replyStyle": replyStyle}
}

// devAppFetchCredentials reuses devapp's get_open_dev_app_credentials tool to
// resolve a unified app's clientId/clientSecret, so `robot connect
// --unified-app-id` need not have the caller paste raw credentials. Note the
// open platform only returns clientSecret once (at provisioning); if the tool
// omits it the caller is told to fall back to explicit flags.
//
// TODO(verify): the clientId/clientSecret (and appKey/appSecret fallback) field
// names below are NOT yet confirmed against the real get_open_dev_app_credentials
// response — no fixture exists in-repo. Verify against the pre-prod gateway and
// pin the exact field names before relying on --unified-app-id auto-fetch. The
// path degrades safely today: an unrecognised shape yields empty strings and the
// caller is told to use --robot-client-id/--robot-client-secret instead.
func devAppFetchCredentials(runner executor.Runner, cmd *cobra.Command, unifiedAppID string) (clientID, clientSecret string, err error) {
	invocation := executor.NewHelperInvocation(
		cobracmd.LegacyCommandPath(cmd),
		devAppProduct,
		"get_open_dev_app_credentials",
		map[string]any{"unifiedAppId": unifiedAppID},
	)
	res, err := runner.Run(cmd.Context(), invocation)
	if err != nil {
		return "", "", err
	}
	payload := devAppConnectUnwrap(res.Response)
	clientID = devAppConnectFirst(payload, "clientId", "appKey", "clientID")
	clientSecret = devAppConnectFirst(payload, "clientSecret", "appSecret")
	return clientID, clientSecret, nil
}

// devAppConnectUnwrap descends the executor/MCP envelope
// (Response{"content":{...,"result":{...}}}) to the innermost object, tolerating
// either wrapper being absent.
func devAppConnectUnwrap(resp map[string]any) map[string]any {
	cur := resp
	if cur == nil {
		return nil
	}
	if inner, ok := cur["content"].(map[string]any); ok {
		cur = inner
	}
	if inner, ok := cur["result"].(map[string]any); ok {
		cur = inner
	}
	return cur
}

// devAppConnectFirst returns the first non-empty string value among the given
// keys, tolerating nil maps and non-string scalars.
func devAppConnectFirst(resp map[string]any, keys ...string) string {
	if resp == nil {
		return ""
	}
	for _, key := range keys {
		if v, ok := resp[key].(string); ok {
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		}
	}
	return ""
}
