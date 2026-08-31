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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/runtimecred"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/source"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/config"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type commonConsumeOptions struct {
	EventTypes []string
	Filter     string
	Compact    bool
	FormatRaw  string
	OutputDir  string
	RoutesRaw  []string
	MaxEvents  int
	Duration   time.Duration
	Quiet      bool
	Force      bool
	DryRun     bool
	Foreground bool
}

type personalConsumeOptions struct {
	Common           commonConsumeOptions
	EventKey         string
	EventKeys        []string
	Flatten          bool
	DebugRawEvents   bool
	SubscribeID      string
	Rule             string
	Name             string
	FilterJSON       string
	QueryCSV         string
	TTL              time.Duration
	Ephemeral        bool
	UserID           string
	OpenDingTalkID   string
	GroupID          string
	RoleTypes        []string
	ControlBaseURL   string
	StreamTicketMode string
	StreamTicketURL  string
	StreamSourceID   string
	ExplicitToken    string
	ClientIDOverride string
}

type personalListOptions struct {
	Category       string
	EnabledOnly    bool
	IncludePending bool
	Format         string
}

type personalStatusOptions struct {
	EventKey         string
	Status           string
	SubscribeID      string
	Format           string
	ControlBaseURL   string
	StreamSourceID   string
	ExplicitToken    string
	ClientIDOverride string
}

type personalStopOptions struct {
	SubscribeID      string
	All              bool
	ControlBaseURL   string
	StreamSourceID   string
	ExplicitToken    string
	ClientIDOverride string
}

type personalStreamSourceOptions struct {
	ConfigDir        string
	Identity         personal.Identity
	TicketMode       string
	TicketURL        string
	ClientIDOverride string
	CredentialBroker *runtimecred.Broker
	RuntimeTokenMode bool
}

var (
	personalResolveEventIdentity        = resolvePersonalEventIdentity
	personalLookupDefinition            = personal.Lookup
	personalEnsureSubscription          = ensurePersonalSubscription
	personalGetSubscription             = (*personal.Client).GetSubscription
	personalCreateSubscription          = (*personal.Client).CreateSubscription
	personalDeleteSubscription          = (*personal.Client).DeleteSubscription
	personalListSubscriptions           = (*personal.Client).ListSubscriptions
	personalUpsertRunState              = personal.UpsertRunState
	personalRemoveRunStates             = personal.RemoveRunStates
	personalLoadRunStates               = personal.LoadRunStates
	personalConsumeRun                  = consume.Run
	personalConsumeRunMany              = consume.RunMany
	personalValidateConsumeConfig       = consume.ValidateConfig
	personalValidateNoOutputConflict    = consume.ValidateNoOutputConflict
	personalNewStreamSource             = newPersonalStreamSource
	personalBusRun                      = bus.Run
	personalFindBusByIdentity           = busctl.FindBusByIdentity
	personalQueryEntry                  = busctl.QueryEntry
	personalQueryStatus                 = busctl.QueryStatus
	personalStopBus                     = busctl.Stop
	personalStopConsumers               = busctl.StopConsumers
	personalFindProcess                 = os.FindProcess
	personalSignalProcess               = (*os.Process).Signal
	personalResolveAuxiliaryAccessToken = ResolveAuxiliaryAccessToken
	personalForceRefreshRejectedToken   = forceRefreshRejectedAccessToken
	personalLoadTokenData               = authpkg.LoadTokenData
	personalLoadProfiles                = authpkg.LoadProfiles
	personalClientID                    = authpkg.ClientID
	personalRuntimeEventClientID        = runtimePersonalEventClientID
	personalResolveAppCredentialsStrict = authpkg.ResolveAppCredentialsStrict
)

func runtimePersonalEventClientID() string {
	if clientID := strings.TrimSpace(edition.Get().AuthClientID); clientID != "" {
		return clientID
	}
	return strings.TrimSpace(os.Getenv("DWS_CLIENT_ID"))
}

func newEventSchemaCommand() *cobra.Command {
	var asIdentity string
	var formatRaw string
	var flatten bool
	cmd := &cobra.Command{
		Use:               "schema <event_key>",
		Short:             "显示事件 schema",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(c *cobra.Command, args []string) error {
			_, err := normalizeEventAs(asIdentity)
			if err != nil {
				return err
			}
			def, ok := personal.Lookup(args[0])
			if !ok {
				return fmt.Errorf("unknown personal event key %q", args[0])
			}
			if !def.Public {
				return personal.PublicAvailabilityError(args[0])
			}
			return renderPersonalSchema(c.OutOrStdout(), def, formatRaw, flatten)
		},
	}
	cmd.Flags().StringVar(&asIdentity, "as", "user", "事件身份: user")
	cmd.Flags().StringVarP(&formatRaw, "format", "f", "json", "输出格式: json")
	cmd.Flags().BoolVar(&flatten, "flatten", false, "显示 --flatten 消费模式对应的顶层业务字段 schema")
	hideEventInternalFlags(cmd, "as")
	cli.AnnotateRuntimePositionals(cmd, contract.RuntimeSchemaPositional{
		Name:        "event_key",
		Type:        "string",
		Description: "要查询 payload 字段定义的个人事件码",
		Required:    true,
		Index:       0,
	})
	helpers.DeclareLeafMetadata(cmd, helpers.LeafSpec{
		Safety: contract.SafetySpec{
			Effect: "read", Risk: "low",
			Confirmation: "not_required", Idempotency: "idempotent",
		},
		Contract: helpers.LeafContract{
			Identity: contract.ToolIdentitySpec{
				ProductID:      "event",
				Name:           "schema",
				CanonicalPath:  "event.schema",
				CLIPath:        "event schema",
				PrimaryCLIPath: "event schema",
			},
			Description: "查询指定个人事件码的输出字段结构；Agent 应查询 --flatten 模式",
			Interface: &contract.InterfaceSpec{
				Mode:         "local",
				Availability: "available",
				Reason:       "命令读取 CLI 内置的个人事件 payload 定义，不绑定 pinned MCP RPC",
			},
			Selection: contract.SelectionSpec{
				AgentSummary: "查询指定个人事件码的输出字段结构；Agent 应查询 --flatten 模式",
				UseWhen:      []string{"已知任一公开个人 IM、OA 或 Todo event_key，消费前需要理解 --flatten 输出字段或 payload 契约"},
				AvoidWhen: []string{
					"查询 CLI 命令参数契约时用顶层 dws schema",
					"要实际收事件时用 event consume",
				},
				Examples: []string{
					"dws event schema user_im_message_receive_at --flatten --format json",
					"dws event schema user_oa_approval_task_created --flatten --format json",
				},
			},
		},
	})
	return cmd
}

func runPersonalEventList(c *cobra.Command, opts personalListOptions) error {
	items := personal.Catalog(opts.Category, opts.EnabledOnly, opts.IncludePending)
	if opts.Format == "json" {
		enc := json.NewEncoder(c.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(items)
	}
	tw := tabwriter.NewWriter(c.OutOrStdout(), 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "EVENT_KEY\tRULE\tSTATUS\tDESCRIPTION")
	for _, it := range items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			it.EventKey, it.RuleType, it.Status, it.Description)
	}
	return tw.Flush()
}

func renderPersonalSchema(w io.Writer, def personal.Definition, format string, flatten bool) error {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "json"
	}
	if format != "json" {
		return fmt.Errorf("event schema only supports json output")
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(personal.BuildSchemaDocumentForMode(def, flatten))
}

func runPersonalEventConsume(c *cobra.Command, opts personalConsumeOptions) error {
	keys := dedupePersonalEventKeys(opts.EventKeys)
	if len(keys) == 0 && strings.TrimSpace(opts.EventKey) != "" {
		keys = []string{strings.TrimSpace(opts.EventKey)}
	}
	if len(keys) <= 1 {
		if len(keys) == 1 {
			opts.EventKey = keys[0]
		}
		return runPersonalEventConsumeSingle(c, opts)
	}
	opts.EventKeys = keys
	return runPersonalEventConsumeMany(c, opts)
}

func runPersonalEventConsumeSingle(c *cobra.Command, opts personalConsumeOptions) error {
	ctx := c.Context()
	if err := ensurePublicPersonalEvent(opts.EventKey); err != nil {
		return personalSubscriptionValidationError(err)
	}
	if err := validatePersonalBusinessEventOptions(opts.EventKey, opts); err != nil {
		return fmt.Errorf("event consume --as user: %w", personalSubscriptionValidationError(err))
	}
	rawFormat := ""
	if f := c.Flags().Lookup("format"); f != nil && f.Changed {
		rawFormat = opts.Common.FormatRaw
	}
	normalised, fellback := consume.NormalizeFormat(rawFormat)
	if fellback && !opts.Common.Quiet {
		fmt.Fprintf(c.ErrOrStderr(), "WARN: --format %q has no meaning for event stream; using ndjson\n", rawFormat)
	}
	if err := validatePersonalEventOutputMode([]string{opts.EventKey}, opts.Flatten, opts.DebugRawEvents, normalised); err != nil {
		return fmt.Errorf("event consume --as user: %w", personalSubscriptionValidationError(err))
	}
	projector := personalEventProjector(opts.DebugRawEvents, opts.Flatten)

	configDir := defaultConfigDir()
	identity, err := resolvePersonalEventIdentityForToken(ctx, configDir, opts.StreamSourceID, opts.ExplicitToken, opts.ClientIDOverride)
	if err != nil {
		return fmt.Errorf("event consume --as user: %w", err)
	}
	identityHash := dwsevent.IdentityHash(identity.Key())
	editionName := editionNameOrDefault()
	workDir := eventWorkDir(configDir, editionName, dwsevent.SourceKindPersonalStream, identityHash)
	ipcEndpoint := defaultIPCEndpoint(workDir, editionName, dwsevent.SourceKindPersonalStream, identityHash)
	spawnProfileSelector := ""
	if strings.TrimSpace(opts.ExplicitToken) == "" {
		spawnProfileSelector = personalBusProfileSelector(configDir, identity)
	}
	spawnArgs := personalBusSpawnArgsForToken(
		identity,
		identityHash,
		opts.StreamTicketMode,
		opts.StreamTicketURL,
		spawnProfileSelector,
		opts.ExplicitToken,
	)

	routes, err := consume.ParseRoutes(opts.Common.RoutesRaw)
	if err != nil {
		return fmt.Errorf("event consume --as user: %w", personalSubscriptionValidationError(err))
	}
	client := newPersonalEventControlClient(configDir, personalEventControlBaseURL(opts.ControlBaseURL, configDir), identity, opts.ExplicitToken)
	if opts.Common.DryRun {
		if strings.TrimSpace(opts.SubscribeID) == "" {
			if err := validatePersonalSubscriptionOptions(opts); err != nil {
				return fmt.Errorf("event consume --as user: %w", personalSubscriptionValidationError(err))
			}
		} else {
			_, eventKey, _, err := personalEnsureSubscription(ctx, client, identity, opts)
			if err != nil {
				return fmt.Errorf("event consume --as user: %w", err)
			}
			if err := validatePersonalEventOutputMode([]string{eventKey}, opts.Flatten, opts.DebugRawEvents, normalised); err != nil {
				return fmt.Errorf("event consume --as user: %w", personalSubscriptionValidationError(err))
			}
			opts.EventKey = eventKey
		}
		cfg := consume.Config{
			WorkDir:        workDir,
			IPCEndpoint:    ipcEndpoint,
			ClientID:       identity.ClientID,
			SpawnExtraArgs: personalBusSpawnArgsForToken(identity, identityHash, opts.StreamTicketMode, personalEventStreamTicketURL(opts.StreamTicketURL, configDir), spawnProfileSelector, opts.ExplicitToken),
			Compact:        opts.Common.Compact,
			MaxEvents:      opts.Common.MaxEvents,
			Duration:       opts.Common.Duration,
			EventKey:       opts.EventKey,
			Format:         normalised,
			Flatten:        opts.Flatten,
			OutputDir:      opts.Common.OutputDir,
			Routes:         routes,
			Projector:      projector,
			Stderr:         c.ErrOrStderr(),
			Quiet:          opts.Common.Quiet,
			Foreground:     opts.Common.Foreground,
			Force:          opts.Common.Force,
			DryRun:         true,
		}
		applyPersonalConsumeFilters(&cfg, opts, strings.TrimSpace(opts.SubscribeID), opts.EventKey)
		if err := personalConsumeRun(ctx, cfg); err != nil {
			return personalSubscriptionValidationError(err)
		}
		return nil
	}

	cfg := consume.Config{
		WorkDir:        workDir,
		IPCEndpoint:    ipcEndpoint,
		ClientID:       identity.ClientID,
		SpawnExtraArgs: spawnArgs,
		RuntimeToken:   strings.TrimSpace(opts.ExplicitToken),
		Compact:        opts.Common.Compact,
		MaxEvents:      opts.Common.MaxEvents,
		Duration:       opts.Common.Duration,
		EventKey:       opts.EventKey,
		Format:         normalised,
		Flatten:        opts.Flatten,
		OutputDir:      opts.Common.OutputDir,
		Routes:         routes,
		Projector:      projector,
		Stdout:         c.OutOrStdout(),
		Stderr:         c.ErrOrStderr(),
		Quiet:          opts.Common.Quiet,
		Foreground:     opts.Common.Foreground,
		Force:          opts.Common.Force,
	}
	// Complete all local validation before creating a remote subscription.
	// Otherwise an invalid output mode can repeatedly create and roll back a
	// valid subscription when an outer agent relaunches the command.
	applyEventConsumeStdin(&cfg, opts.Common.MaxEvents, opts.Common.Duration, c.InOrStdin())
	if err := personalValidateConsumeConfig(cfg); err != nil {
		return personalSubscriptionValidationError(err)
	}
	if o := c.Flags().Lookup("output"); o != nil && o.Changed {
		if err := personalValidateNoOutputConflict(cfg, o.Value.String()); err != nil {
			return personalSubscriptionValidationError(err)
		}
	}

	var (
		foregroundSource *source.PersonalSource
		foregroundBroker *runtimecred.Broker
	)
	if opts.Common.Foreground {
		explicitToken := strings.TrimSpace(opts.ExplicitToken)
		foregroundBroker = newPersonalCredentialBroker(configDir, explicitToken != "", false)
		if explicitToken != "" {
			if _, err := foregroundBroker.Update(0, explicitToken); err != nil {
				return personalSubscriptionValidationError(err)
			}
		}
		foregroundSource, err = personalNewStreamSource(ctx, personalStreamSourceOptions{
			ConfigDir:        configDir,
			Identity:         identity,
			TicketMode:       opts.StreamTicketMode,
			TicketURL:        opts.StreamTicketURL,
			CredentialBroker: foregroundBroker,
			RuntimeTokenMode: explicitToken != "",
		})
		if err != nil {
			return personalSubscriptionValidationError(err)
		}
	}

	var attempt *personalSubscriptionAttemptReservation
	if strings.TrimSpace(opts.SubscribeID) == "" {
		attempt, err = reservePersonalSubscriptionAttempts(
			workDir,
			client,
			identity,
			spawnProfileSelector,
			[]personalConsumeOptions{opts},
		)
		if err != nil {
			return fmt.Errorf("event consume --as user: %w", err)
		}
	}
	sub, eventKey, ruleType, err := personalEnsureSubscription(ctx, client, identity, opts)
	if err != nil {
		if strings.TrimSpace(opts.ExplicitToken) != "" && personalRuntimeTokenControlRejection(err) {
			err = attempt.releaseRuntimeTokenFailure()
			return fmt.Errorf("event consume --as user: %w", err)
		}
		err = attempt.completeFailure(ctx, 0, 0, err, nil)
		return fmt.Errorf("event consume --as user: %w", err)
	}
	if sub == nil {
		err = attempt.completeFailure(
			ctx,
			0,
			0,
			errors.New("personal event: server returned an empty subscription"),
			nil,
		)
		return fmt.Errorf("event consume --as user: %w", err)
	}
	if strings.TrimSpace(sub.SubscribeID) == "" {
		err = attempt.completeFailure(
			ctx,
			0,
			0,
			errors.New("personal event: server returned empty subscribe_id"),
			nil,
		)
		return fmt.Errorf("event consume --as user: %w", err)
	}
	if err := validatePersonalEventOutputMode([]string{eventKey}, opts.Flatten, opts.DebugRawEvents, normalised); err != nil {
		return fmt.Errorf("event consume --as user: %w", personalSubscriptionValidationError(err))
	}
	selfCreated := strings.TrimSpace(opts.SubscribeID) == ""
	ownsSubscription := selfCreated || opts.Ephemeral
	var cleanupOnce sync.Once
	cleanupOwnedSubscription := func(cleanupCtx context.Context) {
		if !ownsSubscription {
			return
		}
		cleanupOnce.Do(func() {
			_ = personalDeleteSubscription(client, cleanupCtx, sub.SubscribeID)
			_ = personalRemoveRunStates(workDir, []string{sub.SubscribeID})
		})
	}
	if err := personalUpsertRunState(workDir, personal.RunState{
		SubscribeID:  sub.SubscribeID,
		EventKey:     eventKey,
		RuleType:     ruleType,
		ClientID:     identity.ClientID,
		SourceID:     identity.SourceID,
		IdentityHash: identityHash,
	}); err != nil {
		wrapped := fmt.Errorf("save run state: %w", err)
		cleanupCtx := context.Background()
		if personalSubscriptionCanceled(ctx, wrapped) {
			cleanupCtx = ctx
		}
		if attempt != nil {
			classification := personalSubscriptionLocalFailure()
			wrapped = attempt.completeFailure(ctx, 0, 0, wrapped, &classification)
		}
		cleanupOwnedSubscription(cleanupCtx)
		return fmt.Errorf("event consume --as user: %w", wrapped)
	}
	if err := attempt.completeSuccess(); err != nil {
		cleanupOwnedSubscription(context.Background())
		return fmt.Errorf("event consume --as user: %w", err)
	}
	// Ownership-based cleanup: a subscription this run CREATED is
	// unsubscribed on exit
	// (any exit — SIGTERM / stdin-EOF / limit / timeout / error), so nothing
	// leaks server-side. A subscription REUSED via --subscribe-id is left
	// intact — the caller owns its lifecycle. --ephemeral forces cleanup
	// either way.
	if ownsSubscription {
		defer cleanupOwnedSubscription(context.Background())
	}

	cfg.EventKey = eventKey
	cfg.ReadySubscribeID = sub.SubscribeID
	applyPersonalConsumeFilters(&cfg, opts, sub.SubscribeID, eventKey)
	if opts.DebugRawEvents && !opts.Common.Quiet {
		fmt.Fprintf(c.ErrOrStderr(), "debug raw events enabled: local event filters disabled\nworkdir: %s\nbus_log: %s\n",
			workDir, filepath.Join(workDir, "bus.log"))
	}
	if opts.Common.Foreground {
		busCfg := bus.Config{
			WorkDir:          workDir,
			IPCEndpoint:      ipcEndpoint,
			ClientID:         identity.ClientID,
			Edition:          editionName,
			SourceKind:       dwsevent.SourceKindPersonalStream,
			IdentityHash:     identityHash,
			SourceID:         identity.SourceID,
			Source:           foregroundSource,
			CredentialBroker: foregroundBroker,
		}
		bus.ApplyEnvTuning(&busCfg)
		return personalBusRun(ctx, busCfg)
	}
	return personalConsumeRun(ctx, cfg)
}

type personalMultiSubscription struct {
	Sub      *personal.Subscription
	EventKey string
	RuleType string
}

func runPersonalEventConsumeMany(c *cobra.Command, opts personalConsumeOptions) error {
	plans, err := preparePersonalMultiOptions(opts)
	if err != nil {
		return fmt.Errorf("event consume --as user: %w", personalSubscriptionValidationError(err))
	}
	rawFormat := ""
	if f := c.Flags().Lookup("format"); f != nil && f.Changed {
		rawFormat = opts.Common.FormatRaw
	}
	normalised, fellback := consume.NormalizeFormat(rawFormat)
	if fellback && !opts.Common.Quiet {
		fmt.Fprintf(c.ErrOrStderr(), "WARN: --format %q has no meaning for event stream; using ndjson\n", rawFormat)
	}
	if err := validatePersonalEventOutputMode(opts.EventKeys, opts.Flatten, opts.DebugRawEvents, normalised); err != nil {
		return fmt.Errorf("event consume --as user: %w", personalSubscriptionValidationError(err))
	}
	projector := personalEventProjector(false, opts.Flatten)

	ctx := c.Context()
	configDir := defaultConfigDir()
	identity, err := resolvePersonalEventIdentityForToken(ctx, configDir, opts.StreamSourceID, opts.ExplicitToken, opts.ClientIDOverride)
	if err != nil {
		return fmt.Errorf("event consume --as user: %w", err)
	}
	identityHash := dwsevent.IdentityHash(identity.Key())
	editionName := editionNameOrDefault()
	workDir := eventWorkDir(configDir, editionName, dwsevent.SourceKindPersonalStream, identityHash)
	ipcEndpoint := defaultIPCEndpoint(workDir, editionName, dwsevent.SourceKindPersonalStream, identityHash)
	spawnProfileSelector := ""
	if strings.TrimSpace(opts.ExplicitToken) == "" {
		spawnProfileSelector = personalBusProfileSelector(configDir, identity)
	}
	routes, err := consume.ParseRoutes(opts.Common.RoutesRaw)
	if err != nil {
		return fmt.Errorf("event consume --as user: %w", personalSubscriptionValidationError(err))
	}
	baseCfg := consume.Config{
		WorkDir:        workDir,
		IPCEndpoint:    ipcEndpoint,
		ClientID:       identity.ClientID,
		SpawnExtraArgs: personalBusSpawnArgsForToken(identity, identityHash, opts.StreamTicketMode, personalEventStreamTicketURL(opts.StreamTicketURL, configDir), spawnProfileSelector, opts.ExplicitToken),
		Compact:        opts.Common.Compact,
		MaxEvents:      opts.Common.MaxEvents,
		Duration:       opts.Common.Duration,
		Format:         normalised,
		Flatten:        opts.Flatten,
		OutputDir:      opts.Common.OutputDir,
		Routes:         routes,
		Projector:      projector,
		Stdout:         c.OutOrStdout(),
		Stderr:         c.ErrOrStderr(),
		Quiet:          opts.Common.Quiet,
	}
	applyEventConsumeStdin(&baseCfg, opts.Common.MaxEvents, opts.Common.Duration, c.InOrStdin())
	if err := personalValidateConsumeConfig(baseCfg); err != nil {
		return personalSubscriptionValidationError(err)
	}
	if o := c.Flags().Lookup("output"); o != nil && o.Changed {
		if err := personalValidateNoOutputConflict(baseCfg, o.Value.String()); err != nil {
			return personalSubscriptionValidationError(err)
		}
	}
	if opts.Common.DryRun {
		printPersonalMultiDryRun(c.ErrOrStderr(), baseCfg, plans)
		return nil
	}
	baseCfg.RuntimeToken = strings.TrimSpace(opts.ExplicitToken)

	client := newPersonalEventControlClient(configDir, personalEventControlBaseURL(opts.ControlBaseURL, configDir), identity, opts.ExplicitToken)
	attempt, err := reservePersonalSubscriptionAttempts(
		workDir,
		client,
		identity,
		spawnProfileSelector,
		plans,
	)
	if err != nil {
		return fmt.Errorf("event consume --as user: %w", err)
	}
	created := make([]personalMultiSubscription, 0, len(plans))
	cleanup := func(cleanupCtx context.Context) {
		ids := make([]string, 0, len(created))
		for i := len(created) - 1; i >= 0; i-- {
			id := strings.TrimSpace(created[i].Sub.SubscribeID)
			ids = append(ids, id)
			if err := personalDeleteSubscription(client, cleanupCtx, id); err != nil {
				fmt.Fprintf(c.ErrOrStderr(), "WARN: failed to clean personal subscription %s: %v\n", id, err)
			}
		}
		if len(ids) > 0 {
			if err := personalRemoveRunStates(workDir, ids); err != nil {
				fmt.Fprintf(c.ErrOrStderr(), "WARN: failed to clean personal event run state: %v\n", err)
			}
		}
	}
	failAndCleanup := func(
		failedIndex int,
		succeededCount int,
		cause error,
		override *personalSubscriptionFailureClass,
	) error {
		cleanupCtx := context.Background()
		if personalSubscriptionCanceled(ctx, cause) {
			cleanupCtx = ctx
		}
		if strings.TrimSpace(opts.ExplicitToken) != "" && personalRuntimeTokenControlRejection(cause) {
			cleanup(cleanupCtx)
			return attempt.releaseRuntimeTokenFailure()
		}
		completed := attempt.completeFailure(ctx, failedIndex, succeededCount, cause, override)
		// Persist the hold (or release a canceled claim) before any potentially
		// slow remote rollback. Otherwise the attempt lease can expire while
		// deleting earlier subscriptions and admit a duplicate create batch.
		cleanup(cleanupCtx)
		return completed
	}
	seenSubscribeIDs := make(map[string]struct{}, len(plans))
	for i, plan := range plans {
		sub, eventKey, ruleType, err := personalEnsureSubscription(ctx, client, identity, plan)
		if err != nil {
			err = failAndCleanup(i, len(created), err, nil)
			return fmt.Errorf("event consume --as user: create subscription for %s: %w", plan.EventKey, err)
		}
		if sub == nil {
			cause := fmt.Errorf("personal event: server returned an empty subscription for %s", plan.EventKey)
			cause = failAndCleanup(i, len(created), cause, nil)
			return fmt.Errorf("event consume --as user: %w", cause)
		}
		id := strings.TrimSpace(sub.SubscribeID)
		if id == "" {
			cause := fmt.Errorf("personal event: server returned empty subscribe_id for %s", plan.EventKey)
			cause = failAndCleanup(i, len(created), cause, nil)
			return fmt.Errorf("event consume --as user: %w", cause)
		}
		if _, exists := seenSubscribeIDs[id]; exists {
			cause := fmt.Errorf("personal event: server returned duplicate subscribe_id %s", id)
			cause = failAndCleanup(i, len(created), cause, nil)
			return fmt.Errorf("event consume --as user: %w", cause)
		}
		seenSubscribeIDs[id] = struct{}{}
		item := personalMultiSubscription{Sub: sub, EventKey: eventKey, RuleType: ruleType}
		created = append(created, item)
		if err := personalUpsertRunState(workDir, personal.RunState{
			SubscribeID:  id,
			EventKey:     eventKey,
			RuleType:     ruleType,
			ClientID:     identity.ClientID,
			SourceID:     identity.SourceID,
			IdentityHash: identityHash,
		}); err != nil {
			cause := fmt.Errorf("save run state for %s: %w", eventKey, err)
			classification := personalSubscriptionLocalFailure()
			cause = failAndCleanup(i, len(created)-1, cause, &classification)
			return fmt.Errorf("event consume --as user: %w", cause)
		}
	}
	if err := attempt.completeSuccess(); err != nil {
		cleanup(context.Background())
		return fmt.Errorf("event consume --as user: %w", err)
	}
	defer cleanup(context.Background())

	specs := make([]consume.ConsumerSpec, 0, len(created))
	for _, item := range created {
		specs = append(specs, consume.ConsumerSpec{
			EventKey:         item.EventKey,
			EventTypes:       []string{item.EventKey},
			SubscribeID:      item.Sub.SubscribeID,
			ReadySubscribeID: item.Sub.SubscribeID,
		})
	}
	if err := personalConsumeRunMany(ctx, baseCfg, specs); err != nil {
		return err
	}
	return nil
}

func preparePersonalMultiOptions(opts personalConsumeOptions) ([]personalConsumeOptions, error) {
	if strings.TrimSpace(opts.SubscribeID) != "" {
		return nil, errors.New("--subscribe-id is not supported when consuming multiple events")
	}
	if strings.TrimSpace(opts.Rule) != "" {
		return nil, errors.New("--rule is not supported when consuming multiple events")
	}
	if len(opts.Common.EventTypes) > 0 {
		return nil, errors.New("--event-types is not supported when consuming multiple events; use event_key positionals")
	}
	if strings.TrimSpace(opts.Common.Filter) != "" {
		return nil, errors.New("--filter is not supported when consuming multiple events; use event_key positionals")
	}
	if opts.Common.Foreground || opts.Common.Force {
		return nil, errors.New("--foreground/--force are not supported when consuming multiple events")
	}
	if opts.DebugRawEvents {
		return nil, errors.New("--debug-raw-events is not supported when consuming multiple events")
	}

	keys := dedupePersonalEventKeys(opts.EventKeys)
	if len(keys) < 2 {
		return nil, errors.New("multiple event keys are required")
	}
	hasUserScope := false
	hasGroupScope := false
	for _, eventKey := range keys {
		def, ok := personalLookupDefinition(eventKey)
		if !ok {
			return nil, fmt.Errorf("unknown personal event key %q", eventKey)
		}
		if !def.Public {
			return nil, personal.PublicAvailabilityError(eventKey)
		}
		if err := validatePersonalBusinessEventOptions(eventKey, opts); err != nil {
			return nil, err
		}
		switch def.RuleType {
		case "singleChat", "sender":
			hasUserScope = true
		case "group":
			hasGroupScope = true
		}
		if (strings.TrimSpace(opts.QueryCSV) != "" || strings.TrimSpace(opts.FilterJSON) != "") && !personal.SupportsMessageFilter(eventKey) {
			return nil, fmt.Errorf("--query/--filter-json require all selected events to be message receive events; %s is not", eventKey)
		}
	}
	if hasUserScope && hasGroupScope {
		return nil, errors.New("user-scoped and group-scoped events cannot be consumed in one command")
	}
	userID := strings.TrimSpace(opts.UserID)
	openID := strings.TrimSpace(opts.OpenDingTalkID)
	groupID := strings.TrimSpace(opts.GroupID)
	if userID != "" && openID != "" {
		return nil, errors.New("--user and --open-dingtalk-id are mutually exclusive")
	}
	switch {
	case hasUserScope:
		if groupID != "" {
			return nil, errors.New("--group cannot be used with user-scoped events")
		}
		if userID == "" && openID == "" {
			return nil, errors.New("one of --user or --open-dingtalk-id is required for the selected events")
		}
	case hasGroupScope:
		if userID != "" || openID != "" {
			return nil, errors.New("--user/--open-dingtalk-id cannot be used with group-scoped events")
		}
		if groupID == "" {
			return nil, errors.New("--group is required for the selected events")
		}
	default:
		if userID != "" || openID != "" || groupID != "" {
			return nil, errors.New("the selected events do not use --user, --open-dingtalk-id, or --group")
		}
	}

	plans := make([]personalConsumeOptions, 0, len(keys))
	for _, eventKey := range keys {
		def, _ := personalLookupDefinition(eventKey)
		plan := opts
		plan.EventKey = eventKey
		plan.EventKeys = nil
		switch def.RuleType {
		case "at", "all":
			plan.UserID = ""
			plan.OpenDingTalkID = ""
			plan.GroupID = ""
		case "singleChat", "sender":
			plan.GroupID = ""
		case "group":
			plan.UserID = ""
			plan.OpenDingTalkID = ""
		}
		if err := validatePersonalSubscriptionOptions(plan); err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	return plans, nil
}

func printPersonalMultiDryRun(w io.Writer, cfg consume.Config, plans []personalConsumeOptions) {
	preview := cfg
	preview.EventTypes = make([]string, 0, len(plans))
	for _, plan := range plans {
		preview.EventTypes = append(preview.EventTypes, plan.EventKey)
	}
	consume.PrintDryRun(w, preview)
	for i, plan := range plans {
		ruleType, ruleParam, _ := personal.BuildRuleParam(plan.EventKey, personal.RuleOptions{
			UserID:         plan.UserID,
			OpenDingTalkID: plan.OpenDingTalkID,
			GroupID:        plan.GroupID,
			RoleTypes:      plan.RoleTypes,
		})
		_, filter, _ := personal.BuildFilter(plan.FilterJSON, plan.QueryCSV)
		ruleJSON, _ := personal.CanonicalJSON(ruleParam)
		fmt.Fprintf(w, "  subscription[%d]  : event_key=%s rule_type=%s rule_param=%s",
			i, plan.EventKey, ruleType, ruleJSON)
		if filter != "" {
			fmt.Fprintf(w, " filter=%s", filter)
		}
		fmt.Fprintln(w)
	}
}

func personalEventProjector(debugRawEvents, flatten bool) consume.Projector {
	if debugRawEvents {
		return func(ev transport.Event) (any, error) { return ev, nil }
	}
	if flatten {
		return personal.ProjectOutput
	}
	return personal.ProjectTransportOutput
}

func validatePersonalEventOutputMode(eventKeys []string, flatten, debugRawEvents bool, format consume.Format) error {
	if flatten && debugRawEvents {
		return fmt.Errorf("--flatten and --debug-raw-events are mutually exclusive")
	}
	if flatten && format == consume.FormatRaw {
		return fmt.Errorf("--flatten and --format raw are mutually exclusive")
	}
	if format == consume.FormatRaw && !debugRawEvents {
		for _, eventKey := range eventKeys {
			if strings.TrimSpace(eventKey) == personal.EventVoIPCallReceiveInvite {
				return fmt.Errorf("--format raw for VoIP events requires explicit --debug-raw-events")
			}
		}
	}
	return nil
}

func applyPersonalConsumeFilters(cfg *consume.Config, opts personalConsumeOptions, subscribeID, eventKey string) {
	if cfg == nil {
		return
	}
	if opts.DebugRawEvents {
		cfg.EventTypes = nil
		cfg.Filter = ""
		cfg.SubscribeID = ""
		return
	}
	cfg.EventTypes = personalEventTypes(eventKey, opts.Common.EventTypes)
	cfg.Filter = opts.Common.Filter
	cfg.SubscribeID = strings.TrimSpace(subscribeID)
}

func validatePersonalSubscriptionOptions(opts personalConsumeOptions) error {
	if err := validatePersonalBusinessEventOptions(opts.EventKey, opts); err != nil {
		return err
	}
	if _, _, err := personal.BuildRuleParam(opts.EventKey, personal.RuleOptions{
		RuleType:       opts.Rule,
		UserID:         opts.UserID,
		OpenDingTalkID: opts.OpenDingTalkID,
		GroupID:        opts.GroupID,
		RoleTypes:      opts.RoleTypes,
	}); err != nil {
		return err
	}
	_, _, err := personal.BuildFilter(opts.FilterJSON, opts.QueryCSV)
	return err
}

func validatePersonalBusinessEventOptions(eventKey string, opts personalConsumeOptions) error {
	if err := validatePersonalOAOrVoIPOptions(eventKey, opts); err != nil {
		return err
	}
	return validatePersonalTodoOptions(eventKey, opts)
}

func validatePersonalOAOrVoIPOptions(eventKey string, opts personalConsumeOptions) error {
	changed := personalUnsupportedOptionNames(opts)
	if len(changed) == 0 {
		return nil
	}
	def, ok := personalLookupDefinition(strings.TrimSpace(eventKey))
	if !ok {
		return nil
	}
	categoryName := map[string]string{
		"oa":   "OA",
		"voip": "VoIP",
	}[def.Category]
	if categoryName == "" {
		return nil
	}
	return fmt.Errorf("%s not supported for %s event %s", strings.Join(changed, ", "), categoryName, eventKey)
}

func personalUnsupportedOptionNames(opts personalConsumeOptions) []string {
	var changed []string
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: "--user", value: opts.UserID},
		{name: "--open-dingtalk-id", value: opts.OpenDingTalkID},
		{name: "--group", value: opts.GroupID},
		{name: "--query", value: opts.QueryCSV},
		{name: "--filter-json", value: opts.FilterJSON},
	} {
		if strings.TrimSpace(item.value) != "" {
			changed = append(changed, item.name)
		}
	}
	if len(opts.RoleTypes) > 0 {
		changed = append(changed, "--role-types")
	}
	return changed
}

func validatePersonalTodoOptions(eventKey string, opts personalConsumeOptions) error {
	def, ok := personalLookupDefinition(strings.TrimSpace(eventKey))
	if !ok || def.Category != "todo" {
		return nil
	}
	changed := personalTodoUnsupportedOptionNames(opts)
	if len(changed) == 0 {
		return nil
	}
	return fmt.Errorf("%s not supported for Todo event %s", strings.Join(changed, ", "), eventKey)
}

func personalTodoUnsupportedOptionNames(opts personalConsumeOptions) []string {
	var changed []string
	for _, item := range []struct {
		name  string
		value string
	}{
		{name: "--user", value: opts.UserID},
		{name: "--open-dingtalk-id", value: opts.OpenDingTalkID},
		{name: "--group", value: opts.GroupID},
		{name: "--query", value: opts.QueryCSV},
		{name: "--filter-json", value: opts.FilterJSON},
	} {
		if strings.TrimSpace(item.value) != "" {
			changed = append(changed, item.name)
		}
	}
	return changed
}

type personalPreparedSubscription struct {
	EventKey string
	RuleType string
	Request  personal.CreateSubscriptionRequest
}

func preparePersonalSubscription(identity personal.Identity, opts personalConsumeOptions) (personalPreparedSubscription, error) {
	if strings.TrimSpace(opts.EventKey) == "" {
		return personalPreparedSubscription{}, fmt.Errorf("event_key is required unless --subscribe-id is provided")
	}
	if err := ensurePublicPersonalEvent(opts.EventKey); err != nil {
		return personalPreparedSubscription{}, err
	}
	if err := validatePersonalBusinessEventOptions(opts.EventKey, opts); err != nil {
		return personalPreparedSubscription{}, err
	}
	ruleType, ruleParam, err := personal.BuildRuleParam(opts.EventKey, personal.RuleOptions{
		RuleType:       opts.Rule,
		UserID:         opts.UserID,
		OpenDingTalkID: opts.OpenDingTalkID,
		GroupID:        opts.GroupID,
		RoleTypes:      opts.RoleTypes,
	})
	if err != nil {
		return personalPreparedSubscription{}, err
	}
	filter, filterCanonical, err := personal.BuildFilter(opts.FilterJSON, opts.QueryCSV)
	if err != nil {
		return personalPreparedSubscription{}, err
	}
	req := personal.CreateSubscriptionRequest{
		EventKey:       opts.EventKey,
		RuleType:       ruleType,
		Name:           opts.Name,
		RuleParam:      ruleParam,
		Filter:         filter,
		Delivery:       map[string]any{"mode": "stream"},
		IdempotencyKey: personal.IdempotencyKey(identity, opts.EventKey, ruleType, ruleParam, filterCanonical),
	}
	if opts.TTL > 0 {
		req.TTLSeconds = int64(opts.TTL.Seconds())
	}
	return personalPreparedSubscription{
		EventKey: opts.EventKey,
		RuleType: ruleType,
		Request:  req,
	}, nil
}

func createPreparedPersonalSubscription(ctx context.Context, client *personal.Client, plan personalPreparedSubscription) (*personal.Subscription, string, string, error) {
	sub, err := personalCreateSubscription(client, ctx, plan.Request)
	if err != nil {
		return nil, "", "", err
	}
	return sub, plan.EventKey, plan.RuleType, nil
}

func ensurePersonalSubscription(ctx context.Context, client *personal.Client, identity personal.Identity, opts personalConsumeOptions) (*personal.Subscription, string, string, error) {
	if strings.TrimSpace(opts.SubscribeID) != "" {
		sub, err := personalGetSubscription(client, ctx, opts.SubscribeID)
		if err != nil {
			return nil, "", "", err
		}
		if sub == nil {
			return nil, "", "", errors.New("personal event: server returned an empty subscription")
		}
		requestedEventKey := strings.TrimSpace(opts.EventKey)
		actualEventKey := strings.TrimSpace(sub.EventKey)
		if requestedEventKey != "" && actualEventKey != "" && requestedEventKey != actualEventKey {
			return nil, "", "", fmt.Errorf(
				"event_key %q does not match reused subscription %q event_key %q",
				requestedEventKey,
				strings.TrimSpace(opts.SubscribeID),
				actualEventKey,
			)
		}
		eventKey := actualEventKey
		if eventKey == "" {
			eventKey = requestedEventKey
		}
		if eventKey == "" {
			return nil, "", "", fmt.Errorf("event_key is required when --subscribe-id lookup returns no event_key")
		}
		if err := ensurePublicPersonalEvent(eventKey); err != nil {
			return nil, "", "", err
		}
		if len(opts.RoleTypes) > 0 {
			return nil, "", "", fmt.Errorf("--role-types is not supported when reusing --subscribe-id")
		}
		if err := validatePersonalBusinessEventOptions(eventKey, opts); err != nil {
			return nil, "", "", err
		}
		ruleType := firstNonEmptyPersonalString(sub.RuleType, opts.Rule)
		if ruleType == "" {
			if def, ok := personal.Lookup(eventKey); ok {
				ruleType = def.RuleType
			}
		}
		sub.SubscribeID = strings.TrimSpace(opts.SubscribeID)
		return sub, eventKey, ruleType, nil
	}
	plan, err := preparePersonalSubscription(identity, opts)
	if err != nil {
		return nil, "", "", err
	}
	return createPreparedPersonalSubscription(ctx, client, plan)
}

func runPersonalEventStatus(c *cobra.Command, opts personalStatusOptions) error {
	ctx := c.Context()
	if err := ensurePublicPersonalEvent(opts.EventKey); err != nil {
		return err
	}
	configDir := defaultConfigDir()
	identity, err := resolvePersonalEventIdentityForToken(ctx, configDir, opts.StreamSourceID, opts.ExplicitToken, opts.ClientIDOverride)
	if err != nil {
		return fmt.Errorf("event status --as user: %w", err)
	}
	identityHash := dwsevent.IdentityHash(identity.Key())
	editionName := editionNameOrDefault()
	workDir := eventWorkDir(configDir, editionName, dwsevent.SourceKindPersonalStream, identityHash)
	entry := personalFindBusByIdentity(configDir, editionName, dwsevent.SourceKindPersonalStream, identityHash)
	var qs busctl.EntryStatus
	if entry != nil {
		qs = personalQueryEntry(*entry)
	} else {
		qs = busctl.EntryStatus{Entry: busctl.BusEntry{
			WorkDir:      workDir,
			Edition:      editionName,
			SourceKind:   dwsevent.SourceKindPersonalStream,
			ClientIDHash: identityHash,
			IdentityHash: identityHash,
			State:        busctl.BusStateNotRunning,
			Meta: &bus.Meta{
				ClientID:     identity.ClientID,
				Edition:      editionName,
				SourceKind:   dwsevent.SourceKindPersonalStream,
				IdentityHash: identityHash,
				SourceID:     identity.SourceID,
			},
		}}
	}
	status := opts.Status
	if status == "" || status == "all" {
		status = ""
	}
	subs, err := personalListSubscriptions(newPersonalEventControlClient(configDir, personalEventControlBaseURL(opts.ControlBaseURL, configDir), identity, opts.ExplicitToken), ctx, personal.ListOptions{
		Status:      status,
		EventKey:    opts.EventKey,
		SubscribeID: opts.SubscribeID,
	})
	if err != nil {
		return fmt.Errorf("event status --as user: %w", err)
	}
	if opts.Format == "json" {
		enc := json.NewEncoder(c.OutOrStdout())
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"identity":      redactedPersonalIdentity(identity, identityHash),
			"subscriptions": subs,
			"bus":           qs,
		})
	}
	renderPersonalStatusText(c.OutOrStdout(), identity, identityHash, subs, qs)
	return nil
}

func personalRuntimeTokenControlRejection(err error) bool {
	var apiErr *personal.APIError
	if !errors.As(err, &apiErr) || apiErr == nil {
		return false
	}
	return apiErr.HTTPStatus == http.StatusUnauthorized ||
		strings.EqualFold(strings.TrimSpace(apiErr.Code), "RUNTIME_TOKEN_REJECTED")
}

func ensurePublicPersonalEvent(eventKey string) error {
	eventKey = strings.TrimSpace(eventKey)
	if eventKey == "" {
		return nil
	}
	if def, ok := personalLookupDefinition(eventKey); ok && !def.Public {
		return personal.PublicAvailabilityError(eventKey)
	}
	return nil
}

func renderPersonalStatusText(w io.Writer, identity personal.Identity, identityHash string, subs []personal.Subscription, qs busctl.EntryStatus) {
	fmt.Fprintf(w, "Personal identity: corp=%s user=%s client=%s source=%s hash=%s\n",
		displayIdentityPart(identity.CorpID), displayIdentityPart(identity.UserID), identity.ClientID, identity.SourceID, identityHash)
	fmt.Fprintf(w, "Bus: %s", qs.Entry.State)
	if qs.Entry.HolderPID > 0 {
		fmt.Fprintf(w, " pid=%d", qs.Entry.HolderPID)
	}
	fmt.Fprintf(w, "\nWorkdir: %s\n", qs.Entry.WorkDir)
	if len(subs) == 0 {
		fmt.Fprintln(w, "Subscriptions: none")
	} else {
		tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "SUBSCRIBE_ID\tEVENT_KEY\tRULE\tSTATUS\tSOURCE")
		for _, sub := range subs {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
				sub.SubscribeID, sub.EventKey, sub.RuleType, sub.Status, sub.SourceID)
		}
		_ = tw.Flush()
	}
	renderPersonalConsumers(w, qs)
}

func renderPersonalConsumers(w io.Writer, qs busctl.EntryStatus) {
	if qs.Entry.State != busctl.BusStateRunning {
		fmt.Fprintln(w, "Consumers: none")
		return
	}
	if qs.Live == nil {
		fmt.Fprintln(w, "Consumers: unavailable (status RPC failed)")
		return
	}
	if len(qs.Live.Consumers) == 0 {
		fmt.Fprintln(w, "Consumers: none")
		return
	}
	fmt.Fprintln(w, "Consumers:")
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "PID\tEVENT_KEYS\tSUBSCRIBE_ID\tFILTER\tRECEIVED\tDROPPED")
	for _, cs := range qs.Live.Consumers {
		eventKeys := strings.Join(cs.EventTypes, ",")
		if eventKeys == "" {
			eventKeys = "(catch-all)"
		}
		subscribeID := displayPersonalStatusValue(cs.SubscribeID)
		filter := displayPersonalStatusValue(cs.Filter)
		fmt.Fprintf(tw, "%d\t%s\t%s\t%s\t%d\t%d\n",
			cs.PID, eventKeys, subscribeID, filter, cs.Received, cs.Dropped)
	}
	_ = tw.Flush()
}

func displayPersonalStatusValue(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "-"
	}
	return v
}

func runPersonalEventStop(c *cobra.Command, opts personalStopOptions) error {
	ctx := c.Context()
	explicitSubscribeID := strings.TrimSpace(opts.SubscribeID)
	isSingleTarget := explicitSubscribeID != ""
	if explicitSubscribeID != "" && opts.All {
		return fmt.Errorf("event stop --as user: subscribe_id and --all are mutually exclusive")
	}
	if explicitSubscribeID == "" && !opts.All {
		return fmt.Errorf("event stop --as user: subscribe_id is required unless --all is set")
	}

	configDir := defaultConfigDir()
	identity, err := resolvePersonalEventIdentityForToken(ctx, configDir, opts.StreamSourceID, opts.ExplicitToken, opts.ClientIDOverride)
	if err != nil {
		return fmt.Errorf("event stop --as user: %w", err)
	}
	identityHash := dwsevent.IdentityHash(identity.Key())
	editionName := editionNameOrDefault()
	workDir := eventWorkDir(configDir, editionName, dwsevent.SourceKindPersonalStream, identityHash)
	ipcEndpoint := defaultIPCEndpoint(workDir, editionName, dwsevent.SourceKindPersonalStream, identityHash)
	subscribeIDs, err := personalStopTargets(workDir, explicitSubscribeID, opts.All)
	if err != nil {
		return fmt.Errorf("event stop --as user: %w", err)
	}
	client := newPersonalEventControlClient(configDir, personalEventControlBaseURL(opts.ControlBaseURL, configDir), identity, opts.ExplicitToken)
	for _, id := range subscribeIDs {
		if err := personalDeleteSubscription(client, ctx, id); err != nil {
			return fmt.Errorf("event stop --as user: cancel subscription %s: %w", id, err)
		}
	}
	if err := personalRemoveRunStates(workDir, subscribeIDs); err != nil {
		return fmt.Errorf("event stop --as user: update local state: %w", err)
	}
	if err := stopPersonalConsumers(c.ErrOrStderr(), ipcEndpoint, subscribeIDs); err != nil {
		fmt.Fprintf(c.ErrOrStderr(), "WARN: failed to stop matching local consume process: %v\n", err)
	}

	remaining, err := personalLoadRunStates(workDir)
	if err != nil {
		return fmt.Errorf("event stop --as user: load remaining local state: %w", err)
	}
	if len(remaining) > 0 {
		printPersonalStopResult(c.OutOrStdout(), subscribeIDs, isSingleTarget, "personal bus still running")
		return nil
	}

	busState := "personal bus stopped"
	if err := personalStopBus(busctl.StopConfig{WorkDir: workDir, IPCEndpoint: ipcEndpoint}); err != nil {
		if errors.Is(err, busctl.ErrNotRunning) {
			busState = "personal bus is not running"
		} else {
			return err
		}
	}
	printPersonalStopResult(c.OutOrStdout(), subscribeIDs, isSingleTarget, busState)
	return nil
}

func personalStopTargets(workDir, explicit string, all bool) ([]string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" && all {
		return nil, fmt.Errorf("subscribe_id and --all are mutually exclusive")
	}
	if explicit != "" {
		return []string{explicit}, nil
	}
	if !all {
		return nil, fmt.Errorf("subscribe_id is required unless --all is set")
	}
	states, err := personalLoadRunStates(workDir)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(states))
	for _, st := range states {
		if st.SubscribeID != "" {
			ids = append(ids, st.SubscribeID)
		}
	}
	sort.Strings(ids)
	return ids, nil
}

func interruptPersonalConsumers(ipcEndpoint string, subscribeIDs []string) error {
	targets := make(map[string]struct{}, len(subscribeIDs))
	for _, id := range subscribeIDs {
		id = strings.TrimSpace(id)
		if id != "" {
			targets[id] = struct{}{}
		}
	}
	if ipcEndpoint == "" || len(targets) == 0 {
		return nil
	}
	status, err := personalQueryStatus(ipcEndpoint)
	if err != nil {
		return nil
	}
	signalled := make(map[int]struct{})
	for _, consumer := range status.Consumers {
		if _, ok := targets[strings.TrimSpace(consumer.SubscribeID)]; !ok {
			continue
		}
		if consumer.PID <= 0 || consumer.PID == os.Getpid() {
			continue
		}
		if _, ok := signalled[consumer.PID]; ok {
			continue
		}
		proc, err := personalFindProcess(consumer.PID)
		if err != nil {
			return fmt.Errorf("find consume pid=%d: %w", consumer.PID, err)
		}
		if err := personalSignalProcess(proc, os.Interrupt); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("signal consume pid=%d: %w", consumer.PID, err)
		}
		signalled[consumer.PID] = struct{}{}
	}
	return nil
}

func stopPersonalConsumers(w io.Writer, ipcEndpoint string, subscribeIDs []string) error {
	hasTarget := false
	for _, id := range subscribeIDs {
		if strings.TrimSpace(id) != "" {
			hasTarget = true
			break
		}
	}
	if !hasTarget {
		return nil
	}

	if _, err := personalStopConsumers(ipcEndpoint, subscribeIDs); err == nil {
		return nil
	} else if !errors.Is(err, busctl.ErrConsumerStopUnsupported) {
		return err
	} else {
		fmt.Fprintf(w, "WARN: running bus does not support targeted consumer stop; falling back to process signal: %v\n", err)
	}
	return interruptPersonalConsumers(ipcEndpoint, subscribeIDs)
}

func printPersonalStopResult(w io.Writer, subscribeIDs []string, single bool, busState string) {
	if single && len(subscribeIDs) == 1 {
		fmt.Fprintf(w, "cancelled personal subscription %s; %s\n", subscribeIDs[0], busState)
		return
	}
	fmt.Fprintf(w, "cancelled %d personal subscription(s); %s\n", len(subscribeIDs), busState)
}

func resolvePersonalEventIdentityForToken(ctx context.Context, configDir, sourceIDOverride, explicitToken string, clientIDOverrides ...string) (personal.Identity, error) {
	explicitToken = strings.TrimSpace(explicitToken)
	if explicitToken == "" {
		return personalResolveEventIdentity(ctx, configDir, sourceIDOverride)
	}
	clientIDOverride := ""
	if len(clientIDOverrides) > 0 {
		clientIDOverride = strings.TrimSpace(clientIDOverrides[0])
	}
	return resolvePersonalEventIdentityWithToken(ctx, configDir, sourceIDOverride, explicitToken, clientIDOverride)
}

// resolvePersonalEventIdentityWithToken resolves only non-sensitive identity
// metadata around a caller-supplied bearer token. It intentionally does not
// call LoadTokenData or any refresh-capable token resolver: an explicit root
// --token must never be replaced with, persisted into, or used to refresh a
// local OAuth profile.
func resolvePersonalEventIdentityWithToken(ctx context.Context, configDir, sourceIDOverride, explicitToken string, clientIDOverrides ...string) (personal.Identity, error) {
	explicitToken = strings.TrimSpace(explicitToken)
	if explicitToken == "" {
		return resolvePersonalEventIdentity(ctx, configDir, sourceIDOverride)
	}
	if strings.Contains(strings.TrimSpace(authpkg.RuntimeProfile()), ",") {
		return personal.Identity{}, fmt.Errorf("personal events require exactly one --profile")
	}

	corpID := resolveRuntimeDefault(ctx, "$corpId")
	userID := resolveRuntimeDefault(ctx, "$currentUserId")
	clientID := ""
	if len(clientIDOverrides) > 0 {
		clientID = strings.TrimSpace(clientIDOverrides[0])
	}
	if clientID == "" {
		// An edition hook or explicit environment value is runtime identity,
		// not persisted app state. Resolve it before profiles.json so a complete
		// host context never depends on local OAuth metadata health.
		clientID = strings.TrimSpace(personalRuntimeEventClientID())
	}
	explicitProfile := strings.TrimSpace(authpkg.RuntimeProfile()) != ""
	if explicitProfile || corpID == "" || userID == "" || clientID == "" {
		profile, err := personalEventProfileMetadata(configDir)
		if err != nil {
			// A user-selected --profile remains a strict contract. Without an
			// explicit selector, profiles.json is optional metadata for a
			// host-managed bearer: malformed or stale persisted state must not
			// override complete runtime defaults or prevent the later global
			// client-id fallback.
			if explicitProfile {
				return personal.Identity{}, fmt.Errorf("load OAuth identity metadata: %w", err)
			}
			profile = nil
		}
		if profile != nil {
			if corpID == "" {
				corpID = strings.TrimSpace(profile.CorpID)
			}
			if userID == "" {
				userID = strings.TrimSpace(profile.UserID)
			}
			if clientID == "" {
				clientID = strings.TrimSpace(profile.ClientID)
			}
		}
	}
	if clientID == "" {
		// Persisted/global app credentials are only a fallback after the
		// selected profile, so an old app config cannot override profile.ClientID.
		clientID = strings.TrimSpace(personalClientID())
	}
	if clientID == "" {
		if id, _, _, _, resolveErr := personalResolveAppCredentialsStrict(configDir); resolveErr == nil {
			clientID = strings.TrimSpace(id)
		}
	}
	if clientID == "" {
		return personal.Identity{}, fmt.Errorf("cannot resolve OAuth client_id for personal events")
	}

	sourceID := strings.TrimSpace(sourceIDOverride)
	if sourceID == "" {
		sourceID = personalEventStreamSourceID("")
	}
	localSubject := ""
	if corpID == "" || userID == "" {
		localSubject = personalTokenSubject("access", explicitToken)
	}
	return personal.Identity{
		LocalSubject: localSubject,
		CorpID:       corpID,
		UserID:       userID,
		ClientID:     clientID,
		SourceID:     sourceID,
	}, nil
}

func personalEventProfileMetadata(configDir string) (*authpkg.Profile, error) {
	cfg, err := personalLoadProfiles(configDir)
	if err != nil {
		return nil, err
	}
	selector := strings.TrimSpace(authpkg.RuntimeProfile())
	explicitSelector := selector != ""
	if strings.Contains(selector, ",") {
		return nil, fmt.Errorf("personal events require exactly one --profile")
	}
	if cfg == nil || len(cfg.Profiles) == 0 {
		if explicitSelector {
			return nil, fmt.Errorf("profile %q not found", selector)
		}
		return nil, nil
	}
	if selector == "" {
		selector = strings.TrimSpace(cfg.CurrentProfile)
	}
	if selector == "" {
		return nil, nil
	}
	profile, err := selectPersonalEventProfileMetadata(cfg, selector, make(map[string]struct{}))
	if err != nil && !explicitSelector {
		// A stale persisted CurrentProfile must not make a host-provided bearer
		// unusable. Runtime defaults and the one-way local subject are sufficient
		// to isolate the event bus without consulting local OAuth credentials.
		return nil, nil
	}
	return profile, err
}

func selectPersonalEventProfileMetadata(cfg *authpkg.ProfilesConfig, selector string, visited map[string]struct{}) (*authpkg.Profile, error) {
	_ = visited // retained for the focused compatibility seam used by app tests.
	return authpkg.ResolveProfileMetadata(cfg, strings.TrimSpace(selector))
}

func resolvePersonalEventIdentity(ctx context.Context, configDir string, sourceIDOverride string) (personal.Identity, error) {
	accessToken, err := personalResolveAuxiliaryAccessToken(ctx, configDir, "")
	if err != nil {
		return personal.Identity{}, err
	}
	tokenData, err := personalLoadTokenData(configDir)
	if err != nil && !errors.Is(err, authpkg.ErrTokenDataNotFound) {
		return personal.Identity{}, fmt.Errorf("load OAuth identity metadata: %w", err)
	}
	var corpID, userID, clientID, refreshToken string
	if tokenData != nil {
		corpID = tokenData.CorpID
		userID = tokenData.UserID
		clientID = tokenData.ClientID
		refreshToken = tokenData.RefreshToken
	}
	if corpID == "" {
		corpID = resolveRuntimeDefault(ctx, "$corpId")
	}
	if userID == "" {
		userID = resolveRuntimeDefault(ctx, "$currentUserId")
	}
	if clientID == "" {
		clientID = personalClientID()
	}
	if clientID == "" {
		if id, _, _, _, err := personalResolveAppCredentialsStrict(configDir); err == nil {
			clientID = id
		}
	}
	if clientID == "" {
		return personal.Identity{}, fmt.Errorf("cannot resolve OAuth client_id for personal events")
	}
	sourceID := strings.TrimSpace(sourceIDOverride)
	if sourceID == "" {
		sourceID = personalEventStreamSourceID("")
	}
	localSubject := ""
	if strings.TrimSpace(corpID) == "" || strings.TrimSpace(userID) == "" {
		localSubject = personalTokenSubject("refresh", refreshToken)
		if localSubject == "" {
			localSubject = personalTokenSubject("access", accessToken)
		}
	}
	return personal.Identity{
		AccessToken:  accessToken,
		LocalSubject: localSubject,
		CorpID:       corpID,
		UserID:       userID,
		ClientID:     clientID,
		SourceID:     sourceID,
	}, nil
}

func newPersonalEventControlClient(configDir, baseURL string, identity personal.Identity, explicitTokens ...string) *personal.Client {
	explicitToken := ""
	if len(explicitTokens) > 0 {
		explicitToken = strings.TrimSpace(explicitTokens[0])
	}
	identity.AccessToken = ""
	client := personal.NewClient(baseURL, identity)
	version := strings.TrimSpace(RawVersion())
	if version == "" {
		version = "unknown"
	}
	client.ClientVersion = version
	client.UserAgent = "dws-cli/" + version
	if explicitToken != "" {
		client.AccessTokenProvider = func(context.Context) (string, error) { return explicitToken, nil }
		client.HTTPClient.Transport = runtimeTokenControlTransport{base: http.DefaultTransport, token: explicitToken}
		client.HTTPClient.CheckRedirect = runtimeTokenRedirectPolicy
	} else {
		client.AccessTokenProvider = func(ctx context.Context) (string, error) {
			return personalResolveAuxiliaryAccessToken(ctx, configDir, "")
		}
	}
	return client
}

// runtimeTokenRedirectPolicy prevents Go's redirect machinery from copying
// DWS's custom x-user-access-token header to another authority. Returning
// ErrUseLastResponse keeps the 3xx response available to the caller without a
// url.Error that could echo an attacker-controlled Location value.
func runtimeTokenRedirectPolicy(req *http.Request, via []*http.Request) error {
	if len(via) == 0 || req == nil || req.URL == nil || via[0] == nil || via[0].URL == nil {
		return http.ErrUseLastResponse
	}
	origin := via[0].URL
	if !strings.EqualFold(strings.TrimSpace(req.URL.Host), strings.TrimSpace(origin.Host)) {
		return http.ErrUseLastResponse
	}
	if strings.EqualFold(origin.Scheme, "https") && !strings.EqualFold(req.URL.Scheme, "https") {
		return http.ErrUseLastResponse
	}
	return nil
}

const runtimeTokenControlErrorBody = `{"code":"RUNTIME_TOKEN_REJECTED","message":"event runtime token was rejected; retry with a fresh host credential"}`

// runtimeTokenControlTransport scrubs an explicit bearer from every response
// body and diagnostic header before the control client decodes or logs it. A
// 401 is replaced with a fixed rejection envelope so untrusted response text
// can never escape through stderr or debug logs.
type runtimeTokenControlTransport struct {
	base  http.RoundTripper
	token string
}

func (t runtimeTokenControlTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	resp, err := base.RoundTrip(req)
	if err != nil {
		if token := strings.TrimSpace(t.token); token != "" && strings.Contains(err.Error(), token) {
			return nil, errors.New("personal event: runtime-token control request failed")
		}
		return nil, err
	}
	if resp == nil {
		return resp, err
	}
	token := strings.TrimSpace(t.token)
	for key, values := range resp.Header {
		for i := range values {
			if token != "" {
				values[i] = strings.ReplaceAll(values[i], token, "<redacted-runtime-token>")
			}
		}
		resp.Header[key] = values
	}
	var responseBody []byte
	if resp.Body != nil {
		responseBody, err = io.ReadAll(io.LimitReader(resp.Body, config.MaxResponseBodySize))
		_ = resp.Body.Close()
		if err != nil {
			return nil, errors.New("personal event: read runtime-token control response")
		}
	}
	if resp.StatusCode == http.StatusUnauthorized {
		responseBody = []byte(runtimeTokenControlErrorBody)
	} else if token != "" {
		responseBody = redactRuntimeTokenResponseBody(responseBody, token)
	}
	resp.Body = io.NopCloser(bytes.NewReader(responseBody))
	resp.ContentLength = int64(len(responseBody))
	if resp.Header == nil {
		resp.Header = make(http.Header)
	}
	resp.Header.Set("Content-Type", "application/json")
	resp.Header.Set("Content-Length", fmt.Sprintf("%d", len(responseBody)))
	return resp, nil
}

func redactRuntimeTokenResponseBody(data []byte, token string) []byte {
	token = strings.TrimSpace(token)
	if len(data) == 0 || token == "" {
		return data
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err == nil {
		var trailing any
		if trailingErr := decoder.Decode(&trailing); errors.Is(trailingErr, io.EOF) {
			if redacted, changed := redactRuntimeTokenJSONValue(decoded, token); changed {
				if encoded, marshalErr := json.Marshal(redacted); marshalErr == nil {
					return encoded
				}
			}
		}
	}
	return bytes.ReplaceAll(data, []byte(token), []byte("<redacted-runtime-token>"))
}

func redactRuntimeTokenJSONValue(value any, token string) (any, bool) {
	switch typed := value.(type) {
	case string:
		redacted := strings.ReplaceAll(typed, token, "<redacted-runtime-token>")
		return redacted, redacted != typed
	case []any:
		changed := false
		for i := range typed {
			var itemChanged bool
			typed[i], itemChanged = redactRuntimeTokenJSONValue(typed[i], token)
			changed = changed || itemChanged
		}
		return typed, changed
	case map[string]any:
		changed := false
		redactedMap := make(map[string]any, len(typed))
		for key, item := range typed {
			redactedKey := strings.ReplaceAll(key, token, "<redacted-runtime-token>")
			redacted, itemChanged := redactRuntimeTokenJSONValue(item, token)
			redactedMap[redactedKey] = redacted
			changed = changed || itemChanged || redactedKey != key
		}
		if !changed {
			return typed, false
		}
		return redactedMap, true
	default:
		return value, false
	}
}

func personalTokenSubject(kind, token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return strings.TrimSpace(kind) + ":" + hex.EncodeToString(sum[:])
}

func validPersonalIdentityHash(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != 16 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func resolveRuntimeDefault(ctx context.Context, key string) string {
	if fnMap := edition.Get().RuntimeDefaults; fnMap != nil {
		if fn := fnMap()[key]; fn != nil {
			if v, ok := fn(ctx); ok {
				return strings.TrimSpace(v)
			}
		}
	}
	return ""
}

func newPersonalStreamSource(ctx context.Context, opts personalStreamSourceOptions) (*source.PersonalSource, error) {
	mode := strings.TrimSpace(opts.TicketMode)
	if mode == "" {
		mode = "normal"
	}
	if mode != "normal" && mode != "custom" {
		return nil, fmt.Errorf("stream ticket mode must be normal or custom")
	}
	ticketURL := strings.TrimSpace(opts.TicketURL)
	if ticketURL == "" {
		ticketURL = personalEventStreamTicketURL("", opts.ConfigDir)
	}
	clientID := opts.Identity.ClientID
	clientSecret := ""
	if mode == "custom" {
		resolvedID, secret, _, _, err := personalResolveAppCredentialsStrict(opts.ConfigDir)
		if err != nil {
			return nil, err
		}
		if opts.ClientIDOverride != "" {
			clientID = opts.ClientIDOverride
		} else if clientID == "" {
			clientID = resolvedID
		}
		clientSecret = secret
	}
	credentialBroker := opts.CredentialBroker
	if credentialBroker == nil {
		credentialBroker = newPersonalCredentialBroker(opts.ConfigDir, false, false)
	}
	httpClient := &http.Client{Timeout: 30 * time.Second}
	if opts.RuntimeTokenMode {
		httpClient.CheckRedirect = runtimeTokenRedirectPolicy
	}
	_ = ctx
	return source.NewPersonal(source.PersonalConfig{
		AccessTokenProvider: func(ctx context.Context) (string, error) {
			return credentialBroker.Resolve(ctx)
		},
		ForceRefreshToken: func(ctx context.Context, rejectedToken string) (string, error) {
			return credentialBroker.RefreshRejected(ctx, rejectedToken)
		},
		ClassifyRetryReject: credentialBroker.ClassifyRejectedAfterRetry,
		ClientID:            clientID,
		ClientSecret:        clientSecret,
		SourceID:            opts.Identity.SourceID,
		TicketURL:           ticketURL,
		TicketMode:          mode,
		HTTPClient:          httpClient,
	})
}

func newPersonalCredentialBroker(configDir string, requireSeed, requireActivation bool) *runtimecred.Broker {
	return runtimecred.New(runtimecred.Config{
		RequireSeed:       requireSeed,
		RequireActivation: requireActivation,
		LocalResolve: func(ctx context.Context) (string, error) {
			return personalResolveAuxiliaryAccessToken(ctx, configDir, "")
		},
		LocalRefresh: func(ctx context.Context, rejectedToken string) (string, error) {
			return personalForceRefreshRejectedToken(ctx, configDir, rejectedToken)
		},
	})
}

func personalBusProfileSelector(configDir string, identity personal.Identity) string {
	// The parent already resolved and loaded this selector. Preserve it before
	// consulting identity metadata: personal event discovery can fill an empty
	// token userId from runtime defaults, and that inferred value must not turn a
	// historical unresolved account into a different exact same-corp account in
	// the detached child.
	if selector := strings.TrimSpace(authpkg.RuntimeProfile()); selector != "" {
		return selector
	}
	if cfg, err := authpkg.LoadProfiles(configDir); err == nil && cfg != nil {
		// With no explicit process-local override, LoadTokenData selected the
		// persisted current profile. Prefer that selection over the enriched
		// identity: $currentUserId may describe an exact same-corp account even
		// though the token came from the historical unresolved profile.
		currentSelector := strings.TrimSpace(cfg.CurrentProfile)
		for i := range cfg.Profiles {
			profile := cfg.Profiles[i]
			selector := authpkg.ProfileSelectionSelector(profile, cfg)
			if selector == currentSelector &&
				(strings.TrimSpace(identity.CorpID) == "" || strings.TrimSpace(profile.CorpID) == strings.TrimSpace(identity.CorpID)) {
				return selector
			}
		}
		for i := range cfg.Profiles {
			profile := cfg.Profiles[i]
			if strings.TrimSpace(profile.CorpID) == strings.TrimSpace(identity.CorpID) &&
				strings.TrimSpace(profile.UserID) == strings.TrimSpace(identity.UserID) {
				return authpkg.ProfileSelectionSelector(profile, cfg)
			}
		}
	}
	return authpkg.ProfileSelector(authpkg.Profile{
		CorpID: identity.CorpID,
		UserID: identity.UserID,
	})
}

func personalBusSpawnArgs(identity personal.Identity, ticketMode, ticketURL string, profileSelectors ...string) []string {
	args := []string{
		"--source-kind", string(dwsevent.SourceKindPersonalStream),
		"--stream-source-id", identity.SourceID,
	}
	// Forward the exact account so the detached _bus child resolves the same
	// credentials as the parent, including when one organization has multiple
	// logged-in users.
	if cid := strings.TrimSpace(identity.CorpID); cid != "" {
		profileSelector := authpkg.ProfileSelector(authpkg.Profile{CorpID: identity.CorpID, UserID: identity.UserID})
		if len(profileSelectors) > 0 && strings.TrimSpace(profileSelectors[0]) != "" {
			profileSelector = strings.TrimSpace(profileSelectors[0])
		}
		args = append(args, "--profile", profileSelector)
	}
	if strings.TrimSpace(ticketMode) != "" {
		args = append(args, "--stream-ticket-mode", ticketMode)
	}
	if strings.TrimSpace(ticketURL) != "" {
		args = append(args, "--stream-ticket-url", ticketURL)
	}
	return args
}

func personalBusSpawnArgsForToken(identity personal.Identity, identityHash, ticketMode, ticketURL, profileSelector, explicitToken string) []string {
	if strings.TrimSpace(explicitToken) == "" {
		return personalBusSpawnArgs(identity, ticketMode, ticketURL, profileSelector)
	}
	args := []string{
		"--source-kind", string(dwsevent.SourceKindPersonalStream),
		"--runtime-token-mode",
		"--identity-hash", strings.TrimSpace(identityHash),
		"--stream-source-id", strings.TrimSpace(identity.SourceID),
	}
	if strings.TrimSpace(ticketMode) != "" {
		args = append(args, "--stream-ticket-mode", strings.TrimSpace(ticketMode))
	}
	if strings.TrimSpace(ticketURL) != "" {
		args = append(args, "--stream-ticket-url", strings.TrimSpace(ticketURL))
	}
	return args
}

func personalEventTypes(eventKey string, explicit []string) []string {
	if len(explicit) > 0 {
		return explicit
	}
	if strings.TrimSpace(eventKey) == "" {
		return nil
	}
	return []string{eventKey}
}

func redactedPersonalIdentity(identity personal.Identity, identityHash string) map[string]string {
	return map[string]string{
		"corp_id":       displayIdentityPart(identity.CorpID),
		"user_id":       displayIdentityPart(identity.UserID),
		"client_id":     identity.ClientID,
		"source_id":     identity.SourceID,
		"identity_hash": identityHash,
	}
}

func displayIdentityPart(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return "unknown"
	}
	return v
}

func firstNonEmptyPersonalString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func dedupePersonalEventKeys(values []string) []string {
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func personalEventControlBaseURL(raw, configDir string) string {
	if v := strings.TrimSpace(raw); v != "" {
		return strings.TrimRight(v, "/")
	}
	return personalEventMCPBaseURL(configDir) + personal.DefaultBasePath
}

func personalEventStreamTicketURL(raw, configDir string) string {
	if v := strings.TrimSpace(raw); v != "" {
		return strings.TrimRight(v, "/")
	}
	return personalEventMCPBaseURL(configDir) + "/stream/connections/ticket"
}

func personalEventStreamSourceID(raw string) string {
	if v := strings.TrimSpace(raw); v != "" {
		return v
	}
	return strings.TrimSpace(edition.PersonalEventSourceID())
}

func personalEventMCPBaseURL(configDir string) string {
	if v := configuredMCPBaseURL(configDir); v != "" {
		return strings.TrimRight(v, "/")
	}
	return strings.TrimRight(config.DefaultMCPBaseURL, "/")
}

func configuredMCPBaseURL(configDir string) string {
	if strings.TrimSpace(configDir) == "" {
		configDir = defaultConfigDir()
	}
	data, err := os.ReadFile(filepath.Join(configDir, "mcp_url"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
