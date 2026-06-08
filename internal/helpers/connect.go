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
	"strings"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

// connect is the top-level channel-aware linking command (not nested under
// chat). A single `dws connect` does three things: (1) detect which agent
// channel is currently running; (2) provision a robot on demand (reusing
// connect bot create's server-side async provisioning); (3) emit the linking
// plan for that channel (how the bot reaches the local agent).
//
// Channel routing: every agent channel forwards to its local headless CLI (one
// shot per message, 24/7), resolved & auto-installed via agentSpecs (see
// connect_stream.go) — claudecode/codex/gemini/opencode/amp/crush/aider/cursor/
// goose, plus the desktop-app-bundled qoder/qoderwork/codebuddy/workbuddy.
// openclaw uses the external connector; hermes the official channel.
func init() {
	RegisterPublic(func() Handler {
		return connectHandler{}
	})
}

type connectHandler struct{}

func (connectHandler) Name() string {
	return "connect"
}

func (connectHandler) Command(runner executor.Runner) *cobra.Command {
	return newConnectCommand(runner)
}

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
// exec-type agent (agentSpecs) shares a generic Stream + headless-CLI plan.
func buildConnectPlan(channel, clientID, robotCode string) map[string]any {
	switch channel {
	case "openclaw":
		return map[string]any{
			"method":  "openclaw-connector",
			"summary": "通过 dingtalk-openclaw-connector 接入（plugin-sdk 契约 / OpenAI-compatible endpoint）",
			"steps": []string{
				"将 clientId/clientSecret 写入 openclaw.json 的 channels.dingtalk-connector",
				"openclaw gateway restart",
				"参考 https://github.com/DingTalk-Real-AI/dingtalk-openclaw-connector",
			},
		}
	case "hermes":
		return map[string]any{
			"method":  "official-channel",
			"summary": "通过钉钉官方 channel 渠道建联（hermes agent）",
			"steps": []string{
				"用 clientId/clientSecret 走官方 channel 订阅机器人消息",
				"将消息路由到 hermes agent 处理后回复",
			},
		}
	}
	if spec, ok := agentSpecs[channel]; ok {
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

// connectProvision reuses connect bot create's server-side async provisioning
// (submit + poll) but returns the terminal payload to the caller (instead of
// writing it out), so connect can read clientId / clientSecret / robotCode and
// continue routing. On FAIL/EXPIRED it returns an error carrying the taskId, so
// the caller can retry idempotently with --task-id.
func connectProvision(cmd *cobra.Command, runner executor.Runner, params map[string]any) (map[string]any, error) {
	submitRes, err := runRobotCreateTool(runner, cmd, robotCreateSubmitTool, params, false)
	if err != nil {
		return nil, err
	}
	submitPayload := robotCreatePayload(submitRes.Response)
	taskID := robotResultString(submitPayload, "taskId")
	if taskID == "" {
		// Server returned a terminal result inline (no taskId); pass it through.
		return submitPayload, nil
	}

	interval := robotResultDuration(submitPayload, "interval", robotCreateDefaultInterval)
	if interval < robotCreatePollMinInterval {
		interval = robotCreatePollMinInterval
	}
	if interval > robotCreatePollMaxInterval {
		interval = robotCreatePollMaxInterval
	}
	deadline := robotResultDuration(submitPayload, "expiresIn", robotCreateDefaultDeadline)

	elapsed := time.Duration(0)
	queryParams := map[string]any{"taskId": taskID}
	for {
		if err := robotCreateSleepFn(cmd.Context(), interval); err != nil {
			return nil, err
		}
		elapsed += interval

		queryRes, err := runRobotCreateTool(runner, cmd, robotCreateQueryTool, queryParams, false)
		if err != nil {
			return nil, err
		}
		queryPayload := robotCreatePayload(queryRes.Response)
		switch strings.ToUpper(robotResultString(queryPayload, "status")) {
		case "SUCCESS", "APPROVAL_REQUIRED":
			return queryPayload, nil
		case "FAIL", "EXPIRED":
			status := strings.ToUpper(robotResultString(queryPayload, "status"))
			return nil, apperrors.NewInternal(fmt.Sprintf(
				"robot creation %s (taskId=%s); retry with: dws connect ... --task-id %s",
				status, taskID, taskID))
		case "WAITING", "":
			// keep polling
		default:
			return queryPayload, nil
		}

		if elapsed >= deadline {
			return nil, apperrors.NewInternal(fmt.Sprintf(
				"robot creation still WAITING after %s (taskId=%s); retry with: dws connect ... --task-id %s",
				deadline, taskID, taskID))
		}
	}
}

// launchConnector wires the bot to the local agent per channel, running in the
// foreground until interrupted. After decoupling provisioning from linking, the
// root command's --start and the `connect start` subcommand share this. Dispatch
// priority:
//  1. external connector (DWS_CONNECT_CMD override or openclaw gateway) → os/exec
//     child, credentials injected via CID/SEC/DWS_AGENT_CHANNEL;
//  2. stream-bridge channels (qoder/qoderwork/claudecode/workbuddy) → Go-native
//     in-process Stream + forwarder, no node/external-script dependency;
//  3. others (hermes etc.) → no built-in linking, advise DWS_CONNECT_CMD.
func launchConnector(cmd *cobra.Command, channel, clientID, clientSecret string) error {
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
		fwd, err := forwarderForChannel(channel)
		if err != nil {
			return err
		}
		fmt.Fprintf(cmd.ErrOrStderr(), "[connect] channel=%s Go 原生 Stream 建联，转发到 %s（Ctrl-C 退出）\n", channel, fwd.label())
		return runStreamConnector(cmd.Context(), channel, clientID, clientSecret, fwd)
	}

	return apperrors.NewValidation(fmt.Sprintf("渠道 %q 暂无内置建联；用环境变量 DWS_CONNECT_CMD 指定要运行的连接器", channel))
}

// newConnectStartCommand implements `dws connect start`: linking only. It uses
// existing robot credentials (--client-id/--client-secret) to start the Stream
// connector per channel, and never provisions — for provisioning run
// `dws connect bot create` first.
func newConnectStartCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "start",
		Short: "建联：用现成机器人凭证起 Stream 连接器（不建号）",
		Long: "用已建好的机器人凭证把它接到当前本地 agent，不做建号。\n" +
			"必须提供 --client-id/--client-secret；渠道由 --channel 显式指定或运行时信号探测。\n" +
			"缺凭证请先用 `dws connect bot create` 建号拿 clientId/clientSecret，或用编排式 `dws connect ... --start` 一键建号+建联。",
		Example: "  dws connect start --channel workbuddy --client-id <id> --client-secret <secret>\n" +
			"  dws connect start --channel claudecode --client-id <id> --client-secret <secret>",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			channelFlag, _ := cmd.Flags().GetString("channel")
			channel, _ := resolveConnectChannel(channelFlag)
			if channel == "" {
				return apperrors.NewValidation("无法探测 agent 渠道；请用 --channel 指定 (openclaw|qoder|qoderwork|hermes|workbuddy|claudecode|codebuddy|codex|gemini|opencode|amp|cursor|goose|crush|aider) 或设置 DWS_AGENT_CHANNEL")
			}
			if _, ok := connectChannels[channel]; !ok {
				return apperrors.NewValidation(fmt.Sprintf("未知渠道 %q（支持 openclaw|qoder|qoderwork|hermes|workbuddy|claudecode|codebuddy|codex|gemini|opencode|amp|cursor|goose|crush|aider）", channel))
			}
			clientID, _ := cmd.Flags().GetString("client-id")
			clientSecret, _ := cmd.Flags().GetString("client-secret")
			if strings.TrimSpace(clientID) == "" || strings.TrimSpace(clientSecret) == "" {
				return apperrors.NewValidation("connect start 需要 --client-id/--client-secret（不建号）；建号请先跑 dws connect bot create")
			}
			return launchConnector(cmd, channel, clientID, clientSecret)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("channel", "auto", "渠道：auto(默认,自动探测)|openclaw|qoder|qoderwork|hermes|workbuddy|claudecode|codebuddy|codex|gemini|opencode|amp|cursor|goose|crush|aider")
	cmd.Flags().String("client-id", "", "现成机器人 clientId（AppKey）(必填)")
	cmd.Flags().String("client-secret", "", "现成机器人 clientSecret（AppSecret）(必填)")
	return cmd
}

func newConnectCommand(runner executor.Runner) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "渠道感知建联：探测 agent 渠道 → 建号 → 按渠道把机器人接到本地 agent",
		Long: "一句话把钉钉机器人和当前本地 agent 建联。\n" +
			"① 探测渠道（--channel 显式 > DWS_AGENT_CHANNEL > 运行时信号兜底）；\n" +
			"② 建号（缺凭证时复用服务端异步 provisioning，返回 clientId/clientSecret/robotCode，clientSecret 仅一次）；\n" +
			"③ 输出该渠道的建联方案：openclaw→连接器 / qoder|qoderwork→Stream 桥接到 qodercli / hermes→官方 channel / workbuddy→当前 WorkBuddy 会话 / claudecode→桥接到 claude -p。\n" +
			"已有机器人用 --client-id/--client-secret 直接建联；新建机器人传 --app-name/--robot-name/--desc。",
		Example: "  dws connect --channel auto --app-name \"销售助手\" --robot-name \"销售助手机器人\" --desc \"销售线索查询\"\n" +
			"  dws connect --channel qoderwork --client-id <id> --client-secret <secret>",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			channelFlag, _ := cmd.Flags().GetString("channel")
			channel, detectedBy := resolveConnectChannel(channelFlag)
			if channel == "" {
				return apperrors.NewValidation("无法探测 agent 渠道；请用 --channel 指定 (openclaw|qoder|qoderwork|hermes|workbuddy|claudecode|codebuddy|codex|gemini|opencode|amp|cursor|goose|crush|aider) 或设置 DWS_AGENT_CHANNEL")
			}
			if _, ok := connectChannels[channel]; !ok {
				return apperrors.NewValidation(fmt.Sprintf("未知渠道 %q（支持 openclaw|qoder|qoderwork|hermes|workbuddy|claudecode|codebuddy|codex|gemini|opencode|amp|cursor|goose|crush|aider）", channel))
			}

			clientID, _ := cmd.Flags().GetString("client-id")
			clientSecret, _ := cmd.Flags().GetString("client-secret")
			robotCode, _ := cmd.Flags().GetString("robot-code")
			var status string
			provisioned := false

			if strings.TrimSpace(clientID) == "" || strings.TrimSpace(clientSecret) == "" {
				appName, _ := cmd.Flags().GetString("app-name")
				robotName, _ := cmd.Flags().GetString("robot-name")
				desc, _ := cmd.Flags().GetString("desc")
				if strings.TrimSpace(appName) == "" || strings.TrimSpace(robotName) == "" || strings.TrimSpace(desc) == "" {
					return apperrors.NewValidation("需要 --client-id/--client-secret（用现成机器人），或 --app-name/--robot-name/--desc（新建机器人）")
				}
				params := map[string]any{"appName": appName, "robotName": robotName, "desc": desc}
				if v, _ := cmd.Flags().GetString("task-id"); strings.TrimSpace(v) != "" {
					params["taskId"] = strings.TrimSpace(v)
				}
				if commandDryRun(cmd) {
					return writeCommandPayload(cmd, map[string]any{
						"channel": channel, "detectedBy": detectedBy, "dryRun": true,
						"wouldProvision": params, "connect": buildConnectPlan(channel, "", ""),
					})
				}
				payload, err := connectProvision(cmd, runner, params)
				if err != nil {
					return err
				}
				clientID = robotResultString(payload, "clientId")
				clientSecret = robotResultString(payload, "clientSecret")
				robotCode = robotResultString(payload, "robotCode")
				status = robotResultString(payload, "status")
				provisioned = true
			}

			out := map[string]any{
				"channel":     channel,
				"detectedBy":  detectedBy,
				"provisioned": provisioned,
				"clientId":    clientID,
				"robotCode":   robotCode,
				"connect":     buildConnectPlan(channel, clientID, robotCode),
			}
			if status != "" {
				out["status"] = status
				if strings.EqualFold(status, "APPROVAL_REQUIRED") {
					out["approvalNotice"] = "应用需企业管理员后台审批通过后，钉钉才会把消息路由进来"
				}
			}
			if provisioned {
				out["clientSecret"] = clientSecret
				out["clientSecretNotice"] = "clientSecret 仅返回一次，请立即安全保存"
			}

			// --start: actually launch the connector for this channel (foreground, until interrupted).
			if start, _ := cmd.Flags().GetBool("start"); start {
				return launchConnector(cmd, channel, clientID, clientSecret)
			}

			return writeCommandPayload(cmd, out)
		},
	}
	preferLegacyLeaf(cmd)
	cmd.Flags().String("channel", "auto", "渠道：auto(默认,自动探测)|openclaw|qoder|qoderwork|hermes|workbuddy|claudecode|codebuddy|codex|gemini|opencode|amp|cursor|goose|crush|aider")
	cmd.Flags().String("app-name", "", "新建机器人：智能体应用名称，2~20 字，企业内唯一")
	cmd.Flags().String("robot-name", "", "新建机器人：承载机器人名称，2~20 字")
	cmd.Flags().String("desc", "", "新建机器人：功能描述，≤200 字")
	cmd.Flags().String("task-id", "", "建号重试用：上次返回的 taskId，避免重复建号")
	cmd.Flags().String("client-id", "", "用现成机器人建联：clientId（AppKey）")
	cmd.Flags().String("client-secret", "", "用现成机器人建联：clientSecret（AppSecret）")
	cmd.Flags().String("robot-code", "", "用现成机器人建联：robotCode（可选）")
	cmd.Flags().Bool("start", false, "建联后实际拉起该渠道的连接器（前台运行；可用 DWS_CONNECT_CMD 覆盖启动命令）")

	// Subcommands: provisioning and linking are each independent, single-purpose;
	// bare `connect` acts as the orchestration convenience shell.
	//   dws connect bot create  provision (returns clientId/secret/robotCode)
	//   dws connect start        link (consume existing credentials, start the Stream connector, no provisioning)
	bot := &cobra.Command{
		Use:               "bot",
		Short:             "建联机器人管理（建号）",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	bot.AddCommand(newConnectBotCreateCommand(runner))
	cmd.AddCommand(bot, newConnectStartCommand())
	return cmd
}
