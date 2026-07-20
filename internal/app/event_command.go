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

package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/registry"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/source"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

var (
	eventRunPersonalConsume    = runPersonalEventConsume
	eventRunPersonalList       = runPersonalEventList
	eventRunPersonalStatus     = runPersonalEventStatus
	eventRunPersonalStop       = runPersonalEventStop
	eventNormalizeAs           = normalizeEventAs
	eventResolveCredentials    = resolveEventCredentials
	eventConsumeRun            = consume.Run
	eventRunForeground         = runForegroundBus
	eventNewEventSource        = newEventSource
	eventNewDingtalkSource     = source.New
	eventResolveAccessToken    = ResolveAuxiliaryAccessToken
	eventBusRun                = bus.Run
	eventReadyFDFromEnv        = busctl.ReadyFDFromEnv
	eventResolvePersonal       = resolvePersonalEventIdentity
	eventNewPersonalSource     = newPersonalStreamSource
	eventMkdirAll              = os.MkdirAll
	eventOpenFile              = os.OpenFile
	eventEnumerateBuses        = busctl.EnumerateBuses
	eventFindBus               = busctl.FindBusByClientID
	eventQueryEntry            = busctl.QueryEntry
	eventStopBus               = busctl.Stop
	eventResolveAppCredentials = authpkg.ResolveAppCredentialsStrict
)

// newEventCommand returns the `event` parent command and all its subcommands.
// Wired into root.go's utilityCommands list.
func newEventCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:               "event",
		Short:             "事件订阅 (DingTalk Stream 长连接)",
		Long:              "通过 DingTalk Stream 长连接订阅事件并以 NDJSON 输出到 stdout。详见 `dws event consume --help`。",
		Args:              cobra.NoArgs,
		TraverseChildren:  true,
		DisableAutoGenTag: true,
		RunE:              func(c *cobra.Command, _ []string) error { return c.Help() },
	}
	cmd.AddCommand(
		newEventConsumeCommand(),
		newEventListCommand(),
		newEventSchemaCommand(),
		newEventStatusCommand(),
		newEventStopCommand(),
		newEventBusCommand(),
	)
	return cmd
}

// ─────────────────────────────────────────────────────────────────────
//  event consume
// ─────────────────────────────────────────────────────────────────────

func newEventConsumeCommand() *cobra.Command {
	var (
		eventTypes   []string
		filter       string
		compact      bool
		formatRaw    string
		outputDir    string
		routesRaw    []string
		maxEvents    int
		duration     time.Duration
		quiet        bool
		force        bool
		dryRun       bool
		foreground   bool
		asIdentity   string
		personalOpts personalConsumeOptions
		streamOpts   eventStreamTicketOptions
	)

	cmd := &cobra.Command{
		Use:   "consume [event_key]",
		Short: "订阅事件流并输出到 stdout",
		Long: `订阅 DingTalk 个人事件并将每条事件以 NDJSON 输出到 stdout。

输出格式（事件流默认 ndjson；显式 -f json/pretty/raw 可覆盖；-f table/csv 对
事件流无意义会 fallback 到 ndjson）：
  ndjson  (默认)  一行一对象，适合 jq / 管道处理
  json            每事件多行美化 JSON（必须配 --max-events 或 --duration）
  pretty          同 json，未来加颜色
  raw             仅 SDK 原始 payload，无外层封装
  compact         扁平化 + 解析嵌套 + 抽取语义字段（Agent 友好）

默认使用当前 OAuth 登录态自动创建/复用个人订阅并建立个人长连接；非默认组织加
--profile。连上后 stderr 打就绪行 [event] ready，等它出现再读 stdout；停机用
SIGTERM、关 stdin，或先用 dws event stop <subscribe_id> --dry-run 预览、确认后加
--yes，绝不要 kill -9。
--event-types/--filter 只影响本地 bus → consume 这一段投递；普通个人事件消费
通常不需要设置。`,
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE: func(c *cobra.Command, args []string) error {
			as, err := eventNormalizeAs(asIdentity)
			if err != nil {
				return err
			}
			if as == "user" {
				personalOpts.EventKey = firstArg(args)
				personalOpts.Common = commonConsumeOptions{
					EventTypes: eventTypes,
					Filter:     filter,
					Compact:    compact,
					FormatRaw:  formatRaw,
					OutputDir:  outputDir,
					RoutesRaw:  routesRaw,
					MaxEvents:  maxEvents,
					Duration:   duration,
					Quiet:      quiet,
					Force:      force,
					DryRun:     dryRun,
					Foreground: foreground,
				}
				personalOpts.StreamTicketMode = streamOpts.Mode
				personalOpts.StreamTicketURL = streamOpts.TicketURL
				personalOpts.StreamSourceID = streamOpts.SourceID
				return eventRunPersonalConsume(c, personalOpts)
			}
			if personalOpts.DebugRawEvents {
				return fmt.Errorf("event consume: --debug-raw-events is only supported with --as user")
			}
			if err := rejectChangedFlags(c, "user",
				"subscribe-id",
				"rule",
				"name",
				"filter-json",
				"query",
				"ttl",
				"ephemeral",
				"user",
				"open-dingtalk-id",
				"group",
				"personal-event-base-url",
			); err != nil {
				return fmt.Errorf("event consume: %w", err)
			}
			if len(args) > 0 {
				return fmt.Errorf("event consume: event_key is only supported with --as user")
			}
			ctx, cancel := signal.NotifyContext(c.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			// Step 1: resolve credentials.
			// Portal ticket normal mode uses portal-managed app credentials, so
			// local ClientSecret is intentionally not required there.
			configDir := defaultConfigDir()
			clientID, clientSecret, err := eventResolveCredentials(configDir, streamOpts)
			if err != nil {
				return fmt.Errorf("event consume: %w", err)
			}
			if !streamOpts.usesPortalNormalMode() && authpkg.EnvHalfSet() {
				fmt.Fprintln(c.ErrOrStderr(),
					"WARN: only one of DWS_CLIENT_ID/DWS_CLIENT_SECRET is set; env fallback disabled, using keychain/app config")
			}

			// Step 2: derive bus working directory + IPC endpoint.
			editionName := editionNameOrDefault()
			clientIDHash := dwsevent.ClientIDHash(clientID)
			workDir := eventWorkDir(configDir, editionName, dwsevent.SourceKindAppStream, clientIDHash)
			ipcEndpoint := defaultIPCEndpoint(workDir, editionName, dwsevent.SourceKindAppStream, clientIDHash)

			// Step 3: parse routes.
			routes, err := consume.ParseRoutes(routesRaw)
			if err != nil {
				return fmt.Errorf("event consume: %w", err)
			}

			// Step 4: resolve format. Event command's default is ndjson;
			// we check Changed on the inherited global -f/--format flag so
			// the user can still override (-f json etc.). table/csv fall
			// back to ndjson with a stderr WARN. See plan §3.1 输出约束.
			rawFormat := ""
			if f := c.Flags().Lookup("format"); f != nil && f.Changed {
				rawFormat = formatRaw
			}
			normalised, fellback := consume.NormalizeFormat(rawFormat)
			if fellback && !quiet {
				fmt.Fprintf(c.ErrOrStderr(),
					"WARN: --format %q has no meaning for event stream; using ndjson\n", rawFormat)
			}

			cfg := consume.Config{
				WorkDir:        workDir,
				IPCEndpoint:    ipcEndpoint,
				ClientID:       clientID,
				EventTypes:     eventTypesWithDefault(eventTypes),
				Filter:         filter,
				Compact:        compact,
				MaxEvents:      maxEvents,
				Duration:       duration,
				Format:         normalised,
				OutputDir:      outputDir,
				Routes:         routes,
				Stdout:         c.OutOrStdout(),
				Stderr:         c.ErrOrStderr(),
				Quiet:          quiet,
				Foreground:     foreground,
				Force:          force,
				DryRun:         dryRun,
				SpawnExtraArgs: streamOpts.spawnArgs(),
			}
			// Arm the stdin-EOF shutdown watcher only for a pipe-style,
			// unbounded run (see shouldWatchStdinEOF).
			applyEventConsumeStdin(&cfg, maxEvents, duration, c.InOrStdin())

			// Step 5: validation (flag-only rules).
			if err := consume.ValidateConfig(cfg); err != nil {
				return err
			}
			// --output-dir/--route vs global hidden -o/--output.
			if o := c.Flags().Lookup("output"); o != nil && o.Changed {
				if err := consume.ValidateNoOutputConflict(cfg, o.Value.String()); err != nil {
					return err
				}
			}

			// Step 6: foreground mode runs the bus in-process. Otherwise
			// consume.Run discovers / forks the bus and dials it.
			if foreground {
				return eventRunForeground(ctx, cfg, configDir, clientSecret, streamOpts)
			}
			return eventConsumeRun(ctx, cfg)
		},
	}

	f := cmd.Flags()
	f.StringVar(&asIdentity, "as", "user", "事件身份: user")
	f.StringSliceVar(&eventTypes, "event-types", nil,
		"逗号分隔事件类型；省略时按 event_key 过滤")
	f.StringVar(&filter, "filter", "",
		"客户端正则过滤事件类型 (下推到 bus 减少 IPC 投递量)")
	f.BoolVar(&compact, "compact", false,
		"提示 bus 客户端期望 compact 渲染（语义透传，bus 仍按原 payload 投递）")
	f.StringVarP(&formatRaw, "format", "f", "ndjson",
		"输出格式 (ndjson/json/pretty/raw/compact)；事件流默认 ndjson")
	f.StringVar(&outputDir, "output-dir", "",
		"每事件写一个文件到该目录 ({type}_{id}_{ts}.json)；与 stdout 互斥")
	f.StringArrayVar(&routesRaw, "route", nil,
		"按 regex 路由事件到目录：'<regex>=dir:<path>'，可重复；未命中走 stdout/--output-dir")
	f.IntVar(&maxEvents, "max-events", 0,
		"收到 N 条后退出 (0 = 不限)")
	f.DurationVar(&duration, "duration", 0,
		"运行时长上限 (Go duration 如 30s/5m)；事件流专用，不复用全局 --timeout")
	f.BoolVar(&quiet, "quiet", false,
		"抑制 stderr 状态信息")
	f.BoolVar(&force, "force", false,
		"仅 --foreground 模式生效：跳过单实例锁 (慎用：会让云事件被随机切分)")
	f.BoolVar(&dryRun, "dry-run", false,
		"仅打印解析后的配置，不连接 bus / 云端")
	f.BoolVar(&foreground, "foreground", false,
		"当前进程直接跑 bus 服务、不 fork、不打印事件（给 systemd/k8s 托管用）；读事件不要用它")
	f.StringVar(&personalOpts.SubscribeID, "subscribe-id", "",
		"个人事件订阅 ID；传入后复用已有订阅")
	f.StringVar(&personalOpts.Rule, "rule", "",
		"个人事件规则类型；默认根据 event_key 推断")
	f.StringVar(&personalOpts.Name, "name", "",
		"个人事件订阅名称")
	f.StringVar(&personalOpts.FilterJSON, "filter-json", "",
		"个人事件 Filter DSL JSON")
	f.StringVar(&personalOpts.QueryCSV, "query", "",
		"按消息文本关键词过滤，逗号分隔")
	f.DurationVar(&personalOpts.TTL, "ttl", 0,
		"个人订阅 TTL (Go duration，如 24h；0 表示不过期)")
	f.BoolVar(&personalOpts.Ephemeral, "ephemeral", false,
		"强制退出时取消个人订阅。默认已按归属清理：本次新建的订阅退出即取消，"+
			"用 --subscribe-id 复用的订阅保留。优雅停可用 SIGTERM、关闭 stdin，"+
			"或从外部先用 dws event stop <subscribe_id> --dry-run 预览、确认后加 --yes（会一并退订）；"+
			"请勿 kill -9（会跳过退订、泄漏服务端订阅）")
	f.StringVar(&personalOpts.UserID, "user", "",
		"单聊对端或指定发送人的 userId（与 --open-dingtalk-id 二选一）")
	f.StringVar(&personalOpts.OpenDingTalkID, "open-dingtalk-id", "",
		"单聊对端或指定发送人的 openDingtalkId（与 --user 二选一）")
	f.StringVar(&personalOpts.GroupID, "group", "",
		"group 规则：openConversationId")
	f.StringVar(&personalOpts.ControlBaseURL, "personal-event-base-url", "",
		"个人事件控制面 base URL；当前默认预发 https://pre-mcp.dingtalk.com/dws")
	f.BoolVar(&personalOpts.DebugRawEvents, "debug-raw-events", false,
		"个人事件联调：绕过本地 event type/subscribe_id 过滤，输出当前 personal stream bus 收到的所有事件")
	f.StringVar(&streamOpts.Mode, "stream-ticket-mode", strings.TrimSpace(os.Getenv("DWS_STREAM_TICKET_MODE")),
		"个人 Stream 建联模式；默认 normal")
	f.StringVar(&streamOpts.SourceID, "stream-source-id", strings.TrimSpace(os.Getenv("DWS_STREAM_SOURCE_ID")),
		"个人 Stream sourceId；开源版默认 open，可由 edition 覆盖")
	f.StringVar(&streamOpts.TicketURL, "stream-ticket-url", strings.TrimSpace(os.Getenv("DWS_STREAM_TICKET_URL")),
		"个人 Stream 取票 URL；当前默认预发 https://pre-mcp.dingtalk.com/stream/connections/ticket")
	hideEventInternalFlags(cmd, "as")
	cli.AnnotateRuntimePositionals(cmd, cli.RuntimeSchemaPositional{
		Name:        "event_key",
		Type:        "string",
		Description: "要消费的个人事件码；省略时仅适用于显式配置其它事件来源的兼容模式",
		Required:    false,
		Index:       0,
	})
	return cmd
}

// runForegroundBus implements --foreground: instead of dialing an existing
// bus or forking a new one, the current process IS the bus. We construct a
// source.DingtalkSource with the same credentials, then bus.Run in this
// goroutine with a ready pipe wired to stderr-only.
//
// Implementation note: a true --foreground run does NOT spawn a second
// process to consume — the consumer's NDJSON output would then have no
// stdout. For v1 we keep it simple: --foreground runs bus only; the user
// can run `dws event consume` from another shell to consume the events.
// v2 may add a "foreground + in-process consumer" combined mode.
func runForegroundBus(ctx context.Context, cfg consume.Config, configDir, clientSecret string, streamOpts eventStreamTicketOptions) error {
	src, err := eventNewEventSource(ctx, configDir, cfg.ClientID, clientSecret, streamOpts)
	if err != nil {
		return err
	}
	busCfg := bus.Config{
		WorkDir:      cfg.WorkDir,
		IPCEndpoint:  cfg.IPCEndpoint,
		ClientID:     cfg.ClientID,
		Edition:      editionNameOrDefault(),
		SourceKind:   dwsevent.SourceKindAppStream,
		IdentityHash: dwsevent.ClientIDHash(cfg.ClientID),
		Source:       src,
		Logger:       slog.Default(),
	}
	bus.ApplyEnvTuning(&busCfg)
	return eventBusRun(ctx, busCfg)
}

type eventStreamTicketOptions struct {
	Mode      string
	SourceID  string
	TicketURL string
}

func (o eventStreamTicketOptions) enabled() bool {
	return strings.TrimSpace(o.Mode) != ""
}

func (o eventStreamTicketOptions) normalizedMode() string {
	return strings.ToLower(strings.TrimSpace(o.Mode))
}

func (o eventStreamTicketOptions) usesPortalNormalMode() bool {
	return o.enabled() && o.normalizedMode() == source.PortalTicketModeNormal
}

func (o eventStreamTicketOptions) spawnArgs() []string {
	if !o.enabled() {
		return nil
	}
	args := []string{"--stream-ticket-mode", strings.TrimSpace(o.Mode)}
	if sourceID := strings.TrimSpace(o.SourceID); sourceID != "" {
		args = append(args, "--stream-source-id", sourceID)
	}
	if ticketURL := strings.TrimSpace(o.TicketURL); ticketURL != "" {
		args = append(args, "--stream-ticket-url", ticketURL)
	}
	return args
}

func resolveEventCredentials(configDir string, streamOpts eventStreamTicketOptions) (clientID, clientSecret string, err error) {
	if !streamOpts.usesPortalNormalMode() {
		clientID, clientSecret, _, _, err = authpkg.ResolveAppCredentialsStrict(configDir)
		return clientID, clientSecret, err
	}
	return eventStreamBusID(streamOpts), "", nil
}

func eventStreamBusID(streamOpts eventStreamTicketOptions) string {
	sourceID := eventStreamSourceID(streamOpts.SourceID)
	return "portal-ticket-normal:" + sourceID
}

func newEventSource(ctx context.Context, configDir, clientID, clientSecret string, streamOpts eventStreamTicketOptions) (*source.DingtalkSource, error) {
	if !streamOpts.enabled() {
		return eventNewDingtalkSource(source.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
		})
	}

	token, err := eventResolveAccessToken(ctx, configDir, "")
	if err != nil {
		return nil, fmt.Errorf("event stream ticket: resolve user token: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("event stream ticket: empty user token")
	}

	portalClientID := clientID
	portalClientSecret := clientSecret
	if streamOpts.usesPortalNormalMode() {
		portalClientID = ""
		portalClientSecret = ""
	}

	return eventNewDingtalkSource(source.Config{
		ClientID:     portalClientID,
		ClientSecret: portalClientSecret,
		PortalTicket: &source.PortalTicketConfig{
			TicketURL:    eventStreamTicketURL(streamOpts.TicketURL),
			AccessToken:  token,
			SourceID:     eventStreamSourceID(streamOpts.SourceID),
			Mode:         streamOpts.Mode,
			ClientID:     portalClientID,
			ClientSecret: portalClientSecret,
			UserAgent:    "dws-event-consume",
		},
	})
}

func eventStreamTicketURL(raw string) string {
	if v := strings.TrimSpace(raw); v != "" {
		return v
	}
	return strings.TrimRight(config.GetMCPBaseURL(), "/") + "/stream/connections/ticket"
}

func defaultEventStreamSourceID() string {
	if v := strings.TrimSpace(os.Getenv("DWS_STREAM_SOURCE_ID")); v != "" {
		return v
	}
	base := strings.ToLower(config.GetMCPBaseURL())
	if strings.Contains(base, "pre-mcp") {
		return "pre_open_source"
	}
	return "open"
}

func eventStreamSourceID(raw string) string {
	if v := strings.TrimSpace(raw); v != "" {
		return v
	}
	return defaultEventStreamSourceID()
}

// ─────────────────────────────────────────────────────────────────────
//  event _bus  (hidden — auto-forked by consume)
// ─────────────────────────────────────────────────────────────────────

func newEventBusCommand() *cobra.Command {
	var (
		clientIDOverride string
		idleTimeout      time.Duration
		sourceKindRaw    string
		streamOpts       eventStreamTicketOptions
	)
	cmd := &cobra.Command{
		Use:               "_bus",
		Short:             "Internal event bus daemon (do not call directly)",
		Long:              "Hidden subcommand auto-spawned by `dws event consume`. Do not invoke directly.",
		Hidden:            true,
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(c *cobra.Command, _ []string) error {
			ctx, cancel := signal.NotifyContext(c.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			// Acquire ReadyPipe early so pre-bus.Run failures can signal
			// 'E' to the parent process instead of silently dying.
			readyPipe := eventReadyFDFromEnv()
			failEarly := func(err error) error {
				if readyPipe != nil {
					// 'E' signals failure; the trailing text lets the parent
					// (busctl.waitReady) surface the real startup error to the
					// user instead of an opaque "startup failure on ready pipe".
					_, _ = readyPipe.Write([]byte{'E'})
					if err != nil {
						_, _ = io.WriteString(readyPipe, err.Error())
					}
					_ = readyPipe.Close()
				}
				return err
			}

			configDir := defaultConfigDir()
			sourceKind := dwsevent.SourceKind(strings.TrimSpace(sourceKindRaw))
			if sourceKind == "" {
				sourceKind = dwsevent.SourceKindAppStream
			}
			if sourceKind == dwsevent.SourceKindPersonalStream {
				identity, err := eventResolvePersonal(ctx, configDir, streamOpts.SourceID)
				if err != nil {
					return failEarly(fmt.Errorf("event _bus: %w", err))
				}
				if clientIDOverride != "" {
					identity.ClientID = clientIDOverride
				}
				identityHash := dwsevent.IdentityHash(identity.Key())
				editionName := editionNameOrDefault()
				workDir := eventWorkDir(configDir, editionName, dwsevent.SourceKindPersonalStream, identityHash)
				endpoint := defaultIPCEndpoint(workDir, editionName, dwsevent.SourceKindPersonalStream, identityHash)
				src, err := eventNewPersonalSource(ctx, personalStreamSourceOptions{
					ConfigDir:        configDir,
					Identity:         identity,
					TicketMode:       streamOpts.Mode,
					TicketURL:        streamOpts.TicketURL,
					ClientIDOverride: clientIDOverride,
				})
				if err != nil {
					return failEarly(err)
				}
				if err := eventMkdirAll(workDir, config.DirPerm); err == nil {
					if lf, ferr := eventOpenFile(filepath.Join(workDir, "bus.log"),
						os.O_CREATE|os.O_WRONLY|os.O_APPEND, config.FilePerm); ferr == nil {
						defer lf.Close()
						slog.SetDefault(slog.New(slog.NewTextHandler(lf, &slog.HandlerOptions{Level: slog.LevelInfo})))
					}
				}
				busCfg := bus.Config{
					WorkDir:      workDir,
					IPCEndpoint:  endpoint,
					ClientID:     identity.ClientID,
					Edition:      editionName,
					SourceKind:   dwsevent.SourceKindPersonalStream,
					IdentityHash: identityHash,
					SourceID:     identity.SourceID,
					Source:       src,
					IdleTimeout:  idleTimeout,
					ReadyPipe:    readyPipe,
					Logger:       slog.Default(),
				}
				bus.ApplyEnvTuning(&busCfg)
				return eventBusRun(ctx, busCfg)
			}

			resolvedID, secret, err := eventResolveCredentials(configDir, streamOpts)
			if err != nil {
				return failEarly(fmt.Errorf("event _bus: %w", err))
			}
			clientID := resolvedID
			if clientIDOverride != "" {
				clientID = clientIDOverride
			}
			editionName := editionNameOrDefault()
			clientIDHash := dwsevent.ClientIDHash(clientID)
			workDir := eventWorkDir(configDir, editionName, dwsevent.SourceKindAppStream, clientIDHash)
			endpoint := defaultIPCEndpoint(workDir, editionName, dwsevent.SourceKindAppStream, clientIDHash)

			src, err := eventNewEventSource(ctx, configDir, clientID, secret, streamOpts)
			if err != nil {
				return failEarly(err)
			}

			// busLogger writes to bus.log inside WorkDir so the daemon's
			// own log lines never pollute stdout/stderr (which busctl/Spawn
			// detached). Best-effort: if mkdir / open fails we fall back
			// to slog.Default (stderr) so we at least see startup errors.
			if err := eventMkdirAll(workDir, config.DirPerm); err == nil {
				if lf, ferr := eventOpenFile(filepath.Join(workDir, "bus.log"),
					os.O_CREATE|os.O_WRONLY|os.O_APPEND, config.FilePerm); ferr == nil {
					defer lf.Close()
					slog.SetDefault(slog.New(slog.NewTextHandler(lf, &slog.HandlerOptions{Level: slog.LevelInfo})))
				}
			}

			busCfg := bus.Config{
				WorkDir:      workDir,
				IPCEndpoint:  endpoint,
				ClientID:     clientID,
				Edition:      editionName,
				SourceKind:   dwsevent.SourceKindAppStream,
				IdentityHash: clientIDHash,
				Source:       src,
				IdleTimeout:  idleTimeout,
				ReadyPipe:    readyPipe,
				Logger:       slog.Default(),
			}
			// env-var tuning (only fills in fields left at zero; explicit
			// flags above keep precedence).
			bus.ApplyEnvTuning(&busCfg)
			return eventBusRun(ctx, busCfg)
		},
	}
	cmd.Flags().StringVar(&clientIDOverride, "client-id", "",
		"override clientID resolved from app config / env (used by busctl/Spawn)")
	cmd.Flags().DurationVar(&idleTimeout, "idle-timeout", 5*time.Minute,
		"exit after this long with zero consumers (0 = disabled)")
	cmd.Flags().StringVar(&sourceKindRaw, "source-kind", string(dwsevent.SourceKindAppStream),
		"event source kind: app_stream|personal_stream")
	cmd.Flags().StringVar(&streamOpts.Mode, "stream-ticket-mode", strings.TrimSpace(os.Getenv("DWS_STREAM_TICKET_MODE")),
		"用户 Stream 建联模式：空=SDK app credential；normal/custom=portal 取票")
	cmd.Flags().StringVar(&streamOpts.SourceID, "stream-source-id", strings.TrimSpace(os.Getenv("DWS_STREAM_SOURCE_ID")),
		"用户 Stream sourceId；personal_stream 开源版默认 open")
	cmd.Flags().StringVar(&streamOpts.TicketURL, "stream-ticket-url", strings.TrimSpace(os.Getenv("DWS_STREAM_TICKET_URL")),
		"用户 Stream 取票 URL；personal_stream 默认由 MCP base URL 派生")
	return cmd
}

// ─────────────────────────────────────────────────────────────────────
//  event list
// ─────────────────────────────────────────────────────────────────────

// listEntry is one row in `dws event list` output. Combines on-disk bus
// metadata with the live status RPC results so consumers from multiple
// buses can be rendered as a single flat table.
type listEntry struct {
	ClientID     string                     `json:"client_id"`
	ClientIDHash string                     `json:"client_id_hash"`
	SourceKind   dwsevent.SourceKind        `json:"source_kind,omitempty"`
	SourceID     string                     `json:"source_id,omitempty"`
	Edition      string                     `json:"edition"`
	BusPID       int                        `json:"bus_pid"`
	BusState     busctl.BusEntryState       `json:"bus_state"`
	WorkDir      string                     `json:"workdir"`
	Consumers    []transport.StatusConsumer `json:"consumers,omitempty"`
	// PerType is included in JSON only — text mode renders it in `status`.
	PerType map[string]transport.Counters `json:"per_event_type,omitempty"`
}

func newEventListCommand() *cobra.Command {
	var (
		all            bool
		allEditions    bool
		formatRaw      string
		clientIDOver   string
		asIdentity     string
		category       string
		enabledOnly    bool
		includePending bool
	)
	cmd := &cobra.Command{
		Use:               "list",
		Short:             "列出个人事件目录",
		Long:              "列出当前支持的个人事件目录。",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(c *cobra.Command, _ []string) error {
			as, err := eventNormalizeAs(asIdentity)
			if err != nil {
				return err
			}
			if as == "user" {
				if err := rejectPersonalEventUnsupportedFlags(c, "all", "all-editions", "client-id"); err != nil {
					return fmt.Errorf("event list: %w", err)
				}
				return eventRunPersonalList(c, personalListOptions{
					Category:       category,
					EnabledOnly:    enabledOnly,
					IncludePending: includePending,
					Format:         formatRaw,
				})
			}
			if err := rejectChangedFlags(c, "user", "category", "enabled-only", "include-pending"); err != nil {
				return fmt.Errorf("event list: %w", err)
			}
			entries, err := collectEntries(c, clientIDOver, all, allEditions)
			if err != nil {
				return err
			}
			rendered := make([]listEntry, 0, len(entries))
			for _, qs := range entries {
				rendered = append(rendered, buildListEntry(qs))
			}
			return renderList(c.OutOrStdout(), rendered, formatRaw)
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "列出当前 edition 下所有 ClientID 的消费者")
	cmd.Flags().BoolVar(&allEditions, "all-editions", false, "跨 edition 列出（罕用，调试用）")
	cmd.Flags().StringVar(&clientIDOver, "client-id", "", "指定具体 ClientID（覆盖凭证解析）")
	cmd.Flags().StringVarP(&formatRaw, "format", "f", "table", "输出格式: table|json")
	cmd.Flags().StringVar(&asIdentity, "as", "user", "事件身份: user")
	cmd.Flags().StringVar(&category, "category", "", "个人事件目录分类")
	cmd.Flags().BoolVar(&enabledOnly, "enabled-only", false, "个人事件目录只显示 enabled")
	cmd.Flags().BoolVar(&includePending, "include-pending", false, "个人事件目录包含 pending 项")
	hideEventInternalFlags(cmd, "as", "all", "all-editions", "client-id")
	return cmd
}

// ─────────────────────────────────────────────────────────────────────
//  event status
// ─────────────────────────────────────────────────────────────────────

func newEventStatusCommand() *cobra.Command {
	var (
		all          bool
		allEditions  bool
		formatRaw    string
		clientIDOver string
		failOnOrphan bool
		asIdentity   string
		personalOpts personalStatusOptions
	)
	cmd := &cobra.Command{
		Use:               "status",
		Short:             "显示个人事件订阅和本地消费状态",
		Long:              "显示当前用户个人事件订阅、personal bus 状态和本地 consume 进程。",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(c *cobra.Command, _ []string) error {
			as, err := eventNormalizeAs(asIdentity)
			if err != nil {
				return err
			}
			if as == "user" {
				if err := rejectPersonalEventUnsupportedFlags(c, "all", "all-editions", "client-id", "fail-on-orphan"); err != nil {
					return fmt.Errorf("event status: %w", err)
				}
				personalOpts.Format = formatRaw
				return eventRunPersonalStatus(c, personalOpts)
			}
			if err := rejectChangedFlags(c, "user", "event", "status", "subscribe-id", "personal-event-base-url", "stream-source-id"); err != nil {
				return fmt.Errorf("event status: %w", err)
			}
			entries, err := collectEntries(c, clientIDOver, all, allEditions)
			if err != nil {
				return err
			}
			if err := renderStatus(c.OutOrStdout(), entries, formatRaw); err != nil {
				return err
			}
			if failOnOrphan {
				for _, qs := range entries {
					if qs.Entry.State == busctl.BusStateOrphan {
						return &consume.ValidationError{Msg: "orphan bus detected (--fail-on-orphan set)"}
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "当前 edition 下所有 ClientID")
	cmd.Flags().BoolVar(&allEditions, "all-editions", false, "跨 edition")
	cmd.Flags().StringVar(&clientIDOver, "client-id", "", "指定具体 ClientID")
	cmd.Flags().StringVarP(&formatRaw, "format", "f", "text", "输出格式: text|json")
	cmd.Flags().BoolVar(&failOnOrphan, "fail-on-orphan", false, "检测到 orphan 时退出码 2")
	cmd.Flags().StringVar(&asIdentity, "as", "user", "事件身份: user")
	cmd.Flags().StringVar(&personalOpts.EventKey, "event", "", "个人事件 event_key 过滤")
	cmd.Flags().StringVar(&personalOpts.Status, "status", "active", "个人订阅状态过滤: active|paused|error|deleted|all")
	cmd.Flags().StringVar(&personalOpts.SubscribeID, "subscribe-id", "", "个人订阅 ID 过滤")
	cmd.Flags().StringVar(&personalOpts.ControlBaseURL, "personal-event-base-url", "", "个人事件控制面 base URL；当前默认预发 https://pre-mcp.dingtalk.com/dws")
	cmd.Flags().StringVar(&personalOpts.StreamSourceID, "stream-source-id", strings.TrimSpace(os.Getenv("DWS_STREAM_SOURCE_ID")),
		"个人事件 sourceId；开源版默认 open，可由 edition 覆盖")
	hideEventInternalFlags(cmd, "as", "all", "all-editions", "client-id", "fail-on-orphan")
	return cmd
}

// ─────────────────────────────────────────────────────────────────────
//  list/status 共享 helpers
// ─────────────────────────────────────────────────────────────────────

// collectEntries resolves which BusEntries the command should operate on
// and queries each one's live status. Returns at most one entry when
// neither --all nor --all-editions is set.
func collectEntries(c *cobra.Command, clientIDOver string, all, allEditions bool) ([]busctl.EntryStatus, error) {
	configDir := defaultConfigDir()
	editionName := editionNameOrDefault()

	// --all-editions trumps --all (scan whole tree)
	if allEditions {
		entries, err := eventEnumerateBuses(configDir, "")
		if err != nil {
			return nil, err
		}
		return queryAll(entries), nil
	}
	if all {
		entries, err := eventEnumerateBuses(configDir, editionName)
		if err != nil {
			return nil, err
		}
		return queryAll(entries), nil
	}

	// Single ClientID path. If --client-id passed use it directly,
	// otherwise resolve via strict resolver.
	clientID := clientIDOver
	if clientID == "" {
		resolved, _, _, _, err := eventResolveAppCredentials(configDir)
		if err != nil {
			return nil, fmt.Errorf("event status: resolve credentials: %w (or pass --client-id)", err)
		}
		clientID = resolved
	}
	hash := dwsevent.ClientIDHash(clientID)
	entry := eventFindBus(configDir, editionName, hash)
	if entry == nil {
		// No directory at all — render an empty "not running" so the user
		// sees a useful answer instead of an error.
		return []busctl.EntryStatus{
			{Entry: busctl.BusEntry{
				WorkDir:      eventWorkDir(configDir, editionName, dwsevent.SourceKindAppStream, hash),
				Edition:      editionName,
				SourceKind:   dwsevent.SourceKindAppStream,
				ClientIDHash: hash,
				IdentityHash: hash,
				State:        busctl.BusStateNotRunning,
				Meta: &bus.Meta{
					ClientID:     clientID,
					Edition:      editionName,
					SourceKind:   dwsevent.SourceKindAppStream,
					IdentityHash: hash,
				},
			}},
		}, nil
	}
	if entry.Meta == nil {
		entry.Meta = &bus.Meta{ClientID: clientID, Edition: editionName}
	}
	return []busctl.EntryStatus{eventQueryEntry(*entry)}, nil
}

func queryAll(entries []busctl.BusEntry) []busctl.EntryStatus {
	out := make([]busctl.EntryStatus, 0, len(entries))
	for _, e := range entries {
		out = append(out, eventQueryEntry(e))
	}
	return out
}

func buildListEntry(qs busctl.EntryStatus) listEntry {
	le := listEntry{
		ClientIDHash: qs.Entry.ClientIDHash,
		SourceKind:   qs.Entry.SourceKind,
		Edition:      qs.Entry.Edition,
		BusPID:       qs.Entry.HolderPID,
		BusState:     qs.Entry.State,
		WorkDir:      qs.Entry.WorkDir,
	}
	if qs.Entry.Meta != nil {
		le.ClientID = qs.Entry.Meta.ClientID
		le.SourceID = qs.Entry.Meta.SourceID
	}
	if qs.Live != nil {
		le.Consumers = qs.Live.Consumers
		le.PerType = qs.Live.PerEventTypeCounters
	}
	return le
}

// renderList prints either a table or a JSON array.
func renderList(w io.Writer, entries []listEntry, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}
	// Table: one line per consumer, prefixed by client_id. Buses with no
	// consumers still get one row (so users see they exist).
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SOURCE\tCLIENT_ID\tBUS\tCONSUMER PID\tEVENT KEYS\tSUBSCRIBE_ID\tRECEIVED\tDROPPED")
	for _, le := range entries {
		busDisplay := fmt.Sprintf("%s pid=%d", le.BusState, le.BusPID)
		if le.BusState == busctl.BusStateNotRunning {
			busDisplay = string(le.BusState)
		}
		clientLabel := le.ClientID
		if clientLabel == "" {
			clientLabel = "(unknown — hash=" + le.ClientIDHash + ")"
		}
		if len(le.Consumers) == 0 {
			fmt.Fprintf(tw, "%s\t%s\t%s\t-\t-\t-\t-\t-\n", sourceKindLabel(le.SourceKind), clientLabel, busDisplay)
			continue
		}
		for _, cs := range le.Consumers {
			keys := strings.Join(cs.EventTypes, ",")
			if keys == "" {
				keys = "(catch-all)"
			}
			subID := cs.SubscribeID
			if subID == "" {
				subID = "-"
			}
			fmt.Fprintf(tw, "%s\t%s\t%s\t%d\t%s\t%s\t%d\t%d\n",
				sourceKindLabel(le.SourceKind), clientLabel, busDisplay, cs.PID, keys, subID, cs.Received, cs.Dropped)
		}
	}
	return tw.Flush()
}

// renderStatus prints a multi-line block per bus. JSON mode dumps the raw
// EntryStatus slice.
func renderStatus(w io.Writer, entries []busctl.EntryStatus, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(entries)
	}
	for i, qs := range entries {
		if i > 0 {
			fmt.Fprintln(w)
		}
		renderStatusBlock(w, qs)
	}
	return nil
}

func renderStatusBlock(w io.Writer, qs busctl.EntryStatus) {
	clientLabel := qs.Entry.ClientIDHash
	if qs.Entry.Meta != nil && qs.Entry.Meta.ClientID != "" {
		clientLabel = qs.Entry.Meta.ClientID
	}
	fmt.Fprintf(w, "ClientID: %s\n", clientLabel)
	fmt.Fprintf(w, "  Edition  : %s\n", qs.Entry.Edition)
	fmt.Fprintf(w, "  Source   : %s\n", sourceKindLabel(qs.Entry.SourceKind))
	if qs.Entry.Meta != nil && qs.Entry.Meta.SourceID != "" {
		fmt.Fprintf(w, "  SourceID : %s\n", qs.Entry.Meta.SourceID)
	}
	fmt.Fprintf(w, "  Workdir  : %s\n", qs.Entry.WorkDir)

	switch qs.Entry.State {
	case busctl.BusStateNotRunning:
		fmt.Fprintln(w, "  Bus      : not_running")
		return
	case busctl.BusStateOrphan:
		fmt.Fprintf(w, "  Bus      : orphan  (last_pid=%d not alive)\n", qs.Entry.HolderPID)
		if qs.Entry.Meta != nil && !qs.Entry.Meta.StartedAt.IsZero() {
			fmt.Fprintf(w, "  Started  : %s (from bus.meta)\n", qs.Entry.Meta.StartedAt.Format(time.RFC3339))
		}
		fmt.Fprintln(w, "  Action   : run `dws event consume` to force-restart, or rm -rf the workdir")
		return
	}

	// Running: include live RPC results when present.
	uptime := ""
	if qs.Live != nil {
		uptime = fmt.Sprintf("uptime=%ds", qs.Live.Bus.UptimeSecs)
	} else if qs.Entry.Meta != nil && !qs.Entry.Meta.StartedAt.IsZero() {
		uptime = fmt.Sprintf("uptime=%s (from bus.meta)", time.Since(qs.Entry.Meta.StartedAt).Round(time.Second))
	}
	fmt.Fprintf(w, "  Bus      : running  pid=%d  %s\n", qs.Entry.HolderPID, uptime)

	if qs.Live == nil {
		fmt.Fprintln(w, "  (status RPC failed — bus may be shutting down)")
		return
	}
	live := qs.Live
	fmt.Fprintf(w, "  Source   : state=%s  state_source=%s  reconnects=%d\n",
		live.SourceState.State, live.SourceState.Source, live.SourceState.ReconnectCount)
	fmt.Fprintf(w, "  Consumers: %d active\n", len(live.Consumers))

	if len(live.PerEventTypeCounters) > 0 {
		fmt.Fprintln(w, "  Per-event-type counters (since bus start):")
		keys := make([]string, 0, len(live.PerEventTypeCounters))
		for k := range live.PerEventTypeCounters {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			c := live.PerEventTypeCounters[k]
			fmt.Fprintf(w, "    %s  received=%d  dropped=%d\n", k, c.Received, c.Dropped)
		}
	}
	if len(live.Consumers) > 0 {
		fmt.Fprintln(w, "  Consumers:")
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "    PID\tEVENT KEYS\tSUBSCRIBE_ID\tRECEIVED\tDROPPED")
		for _, cs := range live.Consumers {
			keys := strings.Join(cs.EventTypes, ",")
			if keys == "" {
				keys = "(catch-all)"
			}
			subID := cs.SubscribeID
			if subID == "" {
				subID = "-"
			}
			fmt.Fprintf(tw, "    %d\t%s\t%s\t%d\t%d\n", cs.PID, keys, subID, cs.Received, cs.Dropped)
		}
		_ = tw.Flush()
	}
}

func newEventStopCommand() *cobra.Command {
	var asIdentity string
	var opts personalStopOptions
	cmd := &cobra.Command{
		Use:   "stop [subscribe_id]",
		Short: "取消个人事件订阅并停止本地消费",
		Long: `取消个人事件订阅并停止本地消费，清理对应本地消费状态，并尝试停止对应前台 consume。

传 subscribe_id 时取消该个人订阅；传 --all 时取消当前身份下本地记录的全部个人订阅。
取消后如果没有剩余本地个人订阅，会停止 personal bus。`,
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE: func(c *cobra.Command, args []string) error {
			as, err := eventNormalizeAs(asIdentity)
			if err != nil {
				return err
			}
			if as == "user" {
				opts.SubscribeID = firstArg(args)
				hasSubscribeID := strings.TrimSpace(opts.SubscribeID) != ""
				if hasSubscribeID && opts.All {
					return fmt.Errorf("event stop --as user: subscribe_id and --all are mutually exclusive")
				}
				if !hasSubscribeID && !opts.All {
					return fmt.Errorf("event stop --as user: subscribe_id is required unless --all is set")
				}
				if eventStopDryRun(c) {
					return writeEventStopDryRun(c, as, opts)
				}
				if !eventStopConfirmed(c) {
					return eventStopConfirmationRequired("event stop 会取消个人事件订阅并停止本地消费")
				}
				return eventRunPersonalStop(c, opts)
			}
			if err := rejectChangedFlags(c, "user", "all", "personal-event-base-url", "stream-source-id"); err != nil {
				return fmt.Errorf("event stop: %w", err)
			}
			if len(args) > 0 {
				return fmt.Errorf("event stop: subscribe_id is only supported with --as user")
			}
			if eventStopDryRun(c) {
				return writeEventStopDryRun(c, as, opts)
			}
			if !eventStopConfirmed(c) {
				return eventStopConfirmationRequired("event stop 会停止事件消费")
			}
			configDir := defaultConfigDir()
			clientID, _, _, _, err := eventResolveAppCredentials(configDir)
			if err != nil {
				return fmt.Errorf("event stop: %w", err)
			}
			editionName := editionNameOrDefault()
			clientIDHash := dwsevent.ClientIDHash(clientID)
			workDir := eventWorkDir(configDir, editionName, dwsevent.SourceKindAppStream, clientIDHash)
			if err := eventStopBus(busctl.StopConfig{WorkDir: workDir}); err != nil {
				if errors.Is(err, busctl.ErrNotRunning) {
					fmt.Fprintln(c.OutOrStdout(), "bus is not running")
					return nil
				}
				return err
			}
			fmt.Fprintln(c.OutOrStdout(), "bus stopped")
			return nil
		},
	}
	cmd.Flags().StringVar(&asIdentity, "as", "user", "事件身份: user")
	cmd.Flags().StringVar(&opts.ControlBaseURL, "personal-event-base-url", "", "个人事件控制面 base URL；当前默认预发 https://pre-mcp.dingtalk.com/dws")
	cmd.Flags().StringVar(&opts.StreamSourceID, "stream-source-id", strings.TrimSpace(os.Getenv("DWS_STREAM_SOURCE_ID")),
		"个人事件 sourceId；开源版默认 open，可由 edition 覆盖")
	cmd.Flags().BoolVar(&opts.All, "all", false, "取消当前身份下本地记录的所有个人订阅")
	hideEventInternalFlags(cmd, "as")
	cli.AnnotateRuntimePositionals(cmd, cli.RuntimeSchemaPositional{
		Name:        "subscribe_id",
		Type:        "string",
		Description: "要取消的个人事件订阅 ID；与 --all 二选一",
		Required:    false,
		Index:       0,
	})
	return cmd
}

func eventStopDryRun(cmd *cobra.Command) bool {
	value, _ := cmd.Flags().GetBool("dry-run")
	return value
}

func eventStopConfirmed(cmd *cobra.Command) bool {
	value, _ := cmd.Flags().GetBool("yes")
	return value
}

func eventStopConfirmationRequired(action string) error {
	return apperrors.NewValidation(
		action+"；请先使用 --dry-run 预览，确认后加 --yes 执行",
		apperrors.WithReason("confirmation_required"),
		apperrors.WithHint("先以相同参数加 --dry-run 预览；获得用户确认后改用 --yes 执行"),
		apperrors.WithActions("使用 --dry-run 生成预览", "获得用户确认后使用 --yes 执行"),
	)
}

func writeEventStopDryRun(cmd *cobra.Command, identity string, opts personalStopOptions) error {
	payload := map[string]any{
		"dry_run":  true,
		"action":   "event.stop",
		"identity": strings.TrimSpace(identity),
		"all":      opts.All,
	}
	if subscribeID := strings.TrimSpace(opts.SubscribeID); subscribeID != "" {
		payload["subscribe_id"] = subscribeID
	}
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(payload)
}

// ─────────────────────────────────────────────────────────────────────
//  helpers
// ─────────────────────────────────────────────────────────────────────

// editionNameOrDefault returns edition.Get().Name with "open" fallback.
// Centralised so every event subcommand agrees on the path prefix.
func editionNameOrDefault() string {
	name := edition.Get().Name
	if name == "" {
		return "open"
	}
	return name
}

// defaultIPCEndpoint returns the canonical Unix socket path / Windows pipe
// name for the bus. Thin wrapper over dwsevent.IPCEndpoint, which owns the
// platform shape and the too-long-socket-path fallback.
func defaultIPCEndpoint(workDir, editionName string, sourceKind dwsevent.SourceKind, identityHash string) string {
	return dwsevent.IPCEndpoint(workDir, editionName, sourceKind, identityHash)
}

func eventWorkDir(configDir, editionName string, sourceKind dwsevent.SourceKind, identityHash string) string {
	if sourceKind == "" {
		sourceKind = dwsevent.SourceKindAppStream
	}
	return filepath.Join(configDir, "events", editionName, string(sourceKind), identityHash)
}

func sourceKindLabel(kind dwsevent.SourceKind) string {
	if kind == "" {
		return string(dwsevent.SourceKindAppStream)
	}
	return string(kind)
}

func normalizeEventAs(v string) (string, error) {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "", "user":
		return "user", nil
	case "app", "bot":
		// App stream implementation is intentionally retained for future use,
		// while the public event command currently exposes only user events.
		return "", fmt.Errorf("app event is not publicly available yet")
	default:
		return "", fmt.Errorf("--as only supports user")
	}
}

func hideEventInternalFlags(cmd *cobra.Command, names ...string) {
	for _, name := range names {
		_ = cmd.Flags().MarkHidden(name)
	}
}

func rejectPersonalEventUnsupportedFlags(c *cobra.Command, names ...string) error {
	changed := make([]string, 0, len(names))
	for _, name := range names {
		if f := c.Flags().Lookup(name); f != nil && f.Changed {
			changed = append(changed, "--"+name)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	return fmt.Errorf("%s are not supported for personal events", strings.Join(changed, ", "))
}

func rejectChangedFlags(c *cobra.Command, supportedAs string, names ...string) error {
	changed := make([]string, 0, len(names))
	for _, name := range names {
		if f := c.Flags().Lookup(name); f != nil && f.Changed {
			changed = append(changed, "--"+name)
		}
	}
	if len(changed) == 0 {
		return nil
	}
	return fmt.Errorf("%s are only supported with --as %s", strings.Join(changed, ", "), supportedAs)
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
}

// shouldWatchStdinEOF gates the stdin-EOF shutdown watcher (AI-subprocess
// contract). It arms only for a parent-controlled, pipe-style stdin on an
// unbounded run:
//   - bounded runs (--max-events / --duration) already have their own
//     lifecycle, so stdin is irrelevant;
//   - char devices (an interactive TTY, or /dev/null) are excluded, so a
//     terminal Ctrl-D and the common `< /dev/null` launch do NOT trigger a
//     surprise shutdown. Only a pipe / regular file — an stdin a parent
//     holds and can close to stop us — arms the watcher.
func shouldWatchStdinEOF(maxEvents int, duration time.Duration) bool {
	if maxEvents > 0 || duration > 0 {
		return false
	}
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice == 0
}

func applyEventConsumeStdin(cfg *consume.Config, maxEvents int, duration time.Duration, stdin io.Reader) {
	if cfg != nil && shouldWatchStdinEOF(maxEvents, duration) {
		cfg.Stdin = stdin
	}
}

// eventTypesWithDefault picks the catch-all list from registry when the
// user did not pass --event-types.
func eventTypesWithDefault(types []string) []string {
	if len(types) > 0 {
		return types
	}
	return registry.CatchAllEventTypes()
}

// compile-time guard: avoid "imported and not used" if any of these
// indirect imports become unused after future refactors.
var _ = io.Discard
