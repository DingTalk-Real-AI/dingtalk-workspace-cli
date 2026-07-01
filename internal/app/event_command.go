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
	"runtime"
	"sort"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
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
		Long: `订阅 DingTalk Stream 事件并将每条事件以 NDJSON 输出到 stdout。

输出格式（事件流默认 ndjson；显式 -f json/pretty/raw 可覆盖；-f table/csv 对
事件流无意义会 fallback 到 ndjson）：
  ndjson  (默认)  一行一对象，适合 jq / 管道处理
  json            每事件多行美化 JSON（必须配 --max-events 或 --duration）
  pretty          同 json，未来加颜色
  raw             仅 SDK 原始 payload，无外层封装
  compact         扁平化 + 解析嵌套 + 抽取语义字段（Agent 友好）

bus 上游永远全订阅 (开放平台后台勾选的所有事件类型)；--event-types/--filter 只
影响 bus → consume 这一段投递。开放平台后台未勾选的事件类型即使设置 --event-types
也收不到。

凭证：默认 bot/app 模式优先从 DWS_CLIENT_ID + DWS_CLIENT_SECRET 环境变量读取
(成组覆盖)，否则走 dws config init 配置的 keychain。--as user 使用当前
OAuth 登录态自动创建/复用个人订阅并建立个人长连接。

应用事件流：默认走 SDK app credential；指定 --stream-ticket-mode normal/custom
后可走 portal 取票接口。normal 使用 portal 托管 DWS 凭证；custom 使用当前
DWS clientId/clientSecret 透传给 portal 建立用户 Stream 连接。`,
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE: func(c *cobra.Command, args []string) error {
			if normalizeEventAs(asIdentity) == "user" {
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
				return runPersonalEventConsume(c, personalOpts)
			}
			if len(args) > 0 {
				return fmt.Errorf("event consume: event_key is only supported with --as user")
			}
			ctx, cancel := signal.NotifyContext(c.Context(), os.Interrupt, syscall.SIGTERM)
			defer cancel()

			// Step 1: resolve credentials (strict).
			configDir := defaultConfigDir()
			clientID, clientSecret, _, _, err := authpkg.ResolveAppCredentialsStrict(configDir)
			if err != nil {
				return fmt.Errorf("event consume: %w", err)
			}
			if authpkg.EnvHalfSet() {
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
				return runForegroundBus(ctx, cfg, configDir, clientSecret, streamOpts)
			}
			return consume.Run(ctx, cfg)
		},
	}

	f := cmd.Flags()
	f.StringVar(&asIdentity, "as", "bot", "事件身份: bot|user；user 使用个人事件订阅")
	f.StringSliceVar(&eventTypes, "event-types", nil,
		"逗号分隔事件类型（开放平台 event_type 值）；省略 = catch-all")
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
		"不 fork daemon，当前进程跑 bus (systemd/k8s/launchd 友好)")
	f.StringVar(&personalOpts.SubscribeID, "subscribe-id", "",
		"个人事件订阅 ID；传入后复用已有订阅")
	f.StringVar(&personalOpts.Rule, "rule", "",
		"个人事件规则类型；默认根据 event_key 推断")
	f.StringVar(&personalOpts.Name, "name", "",
		"个人事件订阅名称")
	f.StringVar(&personalOpts.FilterJSON, "filter-json", "",
		"个人事件 Filter DSL JSON")
	f.StringVar(&personalOpts.KeywordCSV, "keyword", "",
		"按消息文本关键词过滤，逗号分隔")
	f.DurationVar(&personalOpts.TTL, "ttl", 0,
		"个人订阅 TTL (Go duration，如 24h；0 表示不过期)")
	f.BoolVar(&personalOpts.Ephemeral, "ephemeral", false,
		"consume 退出时自动取消个人订阅")
	f.StringVar(&personalOpts.PeerUserID, "peer-user-id", "",
		"singleChat 规则：对端 userId")
	f.StringVar(&personalOpts.PeerUnionID, "peer-union-id", "",
		"singleChat 规则：对端 unionId")
	f.StringVar(&personalOpts.SenderUserID, "sender-user-id", "",
		"sender 规则：发送人 userId (P1)")
	f.StringVar(&personalOpts.SenderUnionID, "sender-union-id", "",
		"sender 规则：发送人 unionId (P1)")
	f.StringVar(&personalOpts.OpenConversationID, "open-conversation-id", "",
		"group 规则：openConversationId (P1)")
	f.StringVar(&personalOpts.ControlBaseURL, "personal-event-base-url", "",
		"个人事件控制面 base URL；默认当前 MCP base + /v1/personal-events")
	f.StringVar(&streamOpts.Mode, "stream-ticket-mode", strings.TrimSpace(os.Getenv("DWS_STREAM_TICKET_MODE")),
		"Stream 建联模式：app 默认空=SDK app credential；user 默认 normal；normal=portal 托管凭证；custom=传当前 clientId/clientSecret")
	f.StringVar(&streamOpts.SourceID, "stream-source-id", strings.TrimSpace(os.Getenv("DWS_STREAM_SOURCE_ID")),
		"Stream sourceId；app portal 默认 open/pre_open_source，user 默认 edition.PersonalEventSourceID")
	f.StringVar(&streamOpts.TicketURL, "stream-ticket-url", strings.TrimSpace(os.Getenv("DWS_STREAM_TICKET_URL")),
		"Stream 取票 URL；默认 <MCP_BASE>/stream/connections/ticket")
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
	src, err := newEventSource(ctx, configDir, cfg.ClientID, clientSecret, streamOpts)
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
	return bus.Run(ctx, busCfg)
}

type eventStreamTicketOptions struct {
	Mode      string
	SourceID  string
	TicketURL string
}

func (o eventStreamTicketOptions) enabled() bool {
	return strings.TrimSpace(o.Mode) != ""
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

func newEventSource(ctx context.Context, configDir, clientID, clientSecret string, streamOpts eventStreamTicketOptions) (*source.DingtalkSource, error) {
	if !streamOpts.enabled() {
		return source.New(source.Config{
			ClientID:     clientID,
			ClientSecret: clientSecret,
		})
	}

	token, err := ResolveAuxiliaryAccessToken(ctx, configDir, "")
	if err != nil {
		return nil, fmt.Errorf("event stream ticket: resolve user token: %w", err)
	}
	if strings.TrimSpace(token) == "" {
		return nil, errors.New("event stream ticket: empty user token")
	}

	return source.New(source.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		PortalTicket: &source.PortalTicketConfig{
			TicketURL:    eventStreamTicketURL(streamOpts.TicketURL),
			AccessToken:  token,
			SourceID:     eventStreamSourceID(streamOpts.SourceID),
			Mode:         streamOpts.Mode,
			ClientID:     clientID,
			ClientSecret: clientSecret,
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
			readyPipe := busctl.ReadyFDFromEnv()
			failEarly := func(err error) error {
				if readyPipe != nil {
					_, _ = readyPipe.Write([]byte{'E'})
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
				identity, err := resolvePersonalEventIdentity(ctx, configDir, streamOpts.SourceID)
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
				src, err := newPersonalStreamSource(ctx, personalStreamSourceOptions{
					ConfigDir:        configDir,
					Identity:         identity,
					TicketMode:       streamOpts.Mode,
					TicketURL:        streamOpts.TicketURL,
					ClientIDOverride: clientIDOverride,
				})
				if err != nil {
					return failEarly(err)
				}
				if err := os.MkdirAll(workDir, config.DirPerm); err == nil {
					if lf, ferr := os.OpenFile(filepath.Join(workDir, "bus.log"),
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
				return bus.Run(ctx, busCfg)
			}

			resolvedID, secret, _, _, err := authpkg.ResolveAppCredentialsStrict(configDir)
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

			src, err := newEventSource(ctx, configDir, clientID, secret, streamOpts)
			if err != nil {
				return failEarly(err)
			}

			// busLogger writes to bus.log inside WorkDir so the daemon's
			// own log lines never pollute stdout/stderr (which busctl/Spawn
			// detached). Best-effort: if mkdir / open fails we fall back
			// to slog.Default (stderr) so we at least see startup errors.
			if err := os.MkdirAll(workDir, config.DirPerm); err == nil {
				if lf, ferr := os.OpenFile(filepath.Join(workDir, "bus.log"),
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
			return bus.Run(ctx, busCfg)
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
		"portal 用户 Stream sourceId")
	cmd.Flags().StringVar(&streamOpts.TicketURL, "stream-ticket-url", strings.TrimSpace(os.Getenv("DWS_STREAM_TICKET_URL")),
		"portal 用户 Stream 取票 URL")
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
		Short:             "列出当前 edition 下所有 event 消费者",
		Long:              "列出 bus 守护进程下挂载的消费者。默认只显示当前 ClientID；--all 列出当前 edition 所有；--all-editions 跨 edition。",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(c *cobra.Command, _ []string) error {
			if normalizeEventAs(asIdentity) == "user" {
				return runPersonalEventList(c, personalListOptions{
					Category:       category,
					EnabledOnly:    enabledOnly,
					IncludePending: includePending,
					Format:         formatRaw,
				})
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
	cmd.Flags().StringVar(&asIdentity, "as", "bot", "事件身份: bot|user；user 展示个人事件目录")
	cmd.Flags().StringVar(&category, "category", "", "个人事件目录分类")
	cmd.Flags().BoolVar(&enabledOnly, "enabled-only", false, "个人事件目录只显示 enabled")
	cmd.Flags().BoolVar(&includePending, "include-pending", false, "个人事件目录包含 pending 项")
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
		Short:             "显示 bus 守护进程健康状态",
		Long:              "显示 bus 进程的连接状态、消费者数量、per-event-type 计数。--fail-on-orphan 在检测到 orphan 时退出码 2。",
		Args:              cobra.NoArgs,
		DisableAutoGenTag: true,
		RunE: func(c *cobra.Command, _ []string) error {
			if normalizeEventAs(asIdentity) == "user" {
				personalOpts.Format = formatRaw
				return runPersonalEventStatus(c, personalOpts)
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
	cmd.Flags().StringVar(&asIdentity, "as", "bot", "事件身份: bot|user；user 显示个人事件订阅状态")
	cmd.Flags().StringVar(&personalOpts.EventKey, "event", "", "个人事件 event_key 过滤")
	cmd.Flags().StringVar(&personalOpts.Status, "status", "active", "个人订阅状态过滤: active|paused|error|deleted|all")
	cmd.Flags().StringVar(&personalOpts.SubscribeID, "subscribe-id", "", "个人订阅 ID 过滤")
	cmd.Flags().StringVar(&personalOpts.ControlBaseURL, "personal-event-base-url", "", "个人事件控制面 base URL")
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
		entries, err := busctl.EnumerateBuses(configDir, "")
		if err != nil {
			return nil, err
		}
		return queryAll(entries), nil
	}
	if all {
		entries, err := busctl.EnumerateBuses(configDir, editionName)
		if err != nil {
			return nil, err
		}
		return queryAll(entries), nil
	}

	// Single ClientID path. If --client-id passed use it directly,
	// otherwise resolve via strict resolver.
	clientID := clientIDOver
	if clientID == "" {
		resolved, _, _, _, err := authpkg.ResolveAppCredentialsStrict(configDir)
		if err != nil {
			return nil, fmt.Errorf("event status: resolve credentials: %w (or pass --client-id)", err)
		}
		clientID = resolved
	}
	hash := dwsevent.ClientIDHash(clientID)
	entry := busctl.FindBusByClientID(configDir, editionName, hash)
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
	return []busctl.EntryStatus{busctl.QueryEntry(*entry)}, nil
}

func queryAll(entries []busctl.BusEntry) []busctl.EntryStatus {
	out := make([]busctl.EntryStatus, 0, len(entries))
	for _, e := range entries {
		out = append(out, busctl.QueryEntry(e))
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

// renderStatus prints a multi-line block per bus, matching lark-cli's
// status output shape. JSON mode dumps the raw EntryStatus slice.
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
		Use:               "stop",
		Short:             "优雅停止 bus 守护进程",
		Args:              cobra.MaximumNArgs(1),
		DisableAutoGenTag: true,
		RunE: func(c *cobra.Command, args []string) error {
			if normalizeEventAs(asIdentity) == "user" {
				opts.SubscribeID = firstArg(args)
				return runPersonalEventStop(c, opts)
			}
			if len(args) > 0 {
				return fmt.Errorf("event stop: subscribe_id is only supported with --as user")
			}
			configDir := defaultConfigDir()
			clientID, _, _, _, err := authpkg.ResolveAppCredentialsStrict(configDir)
			if err != nil {
				return fmt.Errorf("event stop: %w", err)
			}
			editionName := editionNameOrDefault()
			clientIDHash := dwsevent.ClientIDHash(clientID)
			workDir := eventWorkDir(configDir, editionName, dwsevent.SourceKindAppStream, clientIDHash)
			if err := busctl.Stop(busctl.StopConfig{WorkDir: workDir}); err != nil {
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
	cmd.Flags().StringVar(&asIdentity, "as", "bot", "事件身份: bot|user；user 取消个人事件订阅并断开长链接")
	cmd.Flags().StringVar(&opts.ControlBaseURL, "personal-event-base-url", "", "个人事件控制面 base URL")
	cmd.Flags().BoolVar(&opts.All, "all", false, "取消当前身份下本地记录的所有个人订阅")
	return cmd
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
// name for the bus. Encapsulates the platform-specific shape so callers
// don't sprinkle GOOS checks throughout the cobra layer.
func defaultIPCEndpoint(workDir, editionName string, sourceKind dwsevent.SourceKind, identityHash string) string {
	if runtime.GOOS == "windows" {
		return `\\.\pipe\dws-event-` + editionName + "-" + sourceKindLabel(sourceKind) + "-" + identityHash
	}
	return filepath.Join(workDir, "bus.sock")
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

func normalizeEventAs(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "", "bot", "app":
		return "bot"
	case "user":
		return "user"
	default:
		return v
	}
}

func firstArg(args []string) string {
	if len(args) == 0 {
		return ""
	}
	return args[0]
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
