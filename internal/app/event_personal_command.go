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
	"text/tabwriter"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/source"
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
	DebugRawEvents   bool
	SubscribeID      string
	Rule             string
	Name             string
	FilterJSON       string
	QueryCSV         string
	TTL              time.Duration
	Ephemeral        bool
	UserID           string
	GroupID          string
	ControlBaseURL   string
	StreamTicketMode string
	StreamTicketURL  string
	StreamSourceID   string
}

type personalListOptions struct {
	Category       string
	EnabledOnly    bool
	IncludePending bool
	Format         string
}

type personalStatusOptions struct {
	EventKey       string
	Status         string
	SubscribeID    string
	Format         string
	ControlBaseURL string
	StreamSourceID string
}

type personalStopOptions struct {
	SubscribeID    string
	All            bool
	ControlBaseURL string
	StreamSourceID string
}

type personalStreamSourceOptions struct {
	ConfigDir        string
	Identity         personal.Identity
	TicketMode       string
	TicketURL        string
	ClientIDOverride string
}

var (
	personalResolveEventIdentity        = resolvePersonalEventIdentity
	personalEnsureSubscription          = ensurePersonalSubscription
	personalGetSubscription             = (*personal.Client).GetSubscription
	personalCreateSubscription          = (*personal.Client).CreateSubscription
	personalDeleteSubscription          = (*personal.Client).DeleteSubscription
	personalListSubscriptions           = (*personal.Client).ListSubscriptions
	personalUpsertRunState              = personal.UpsertRunState
	personalRemoveRunStates             = personal.RemoveRunStates
	personalLoadRunStates               = personal.LoadRunStates
	personalConsumeRun                  = consume.Run
	personalValidateConsumeConfig       = consume.ValidateConfig
	personalValidateNoOutputConflict    = consume.ValidateNoOutputConflict
	personalNewStreamSource             = newPersonalStreamSource
	personalBusRun                      = bus.Run
	personalFindBusByIdentity           = busctl.FindBusByIdentity
	personalQueryEntry                  = busctl.QueryEntry
	personalQueryStatus                 = busctl.QueryStatus
	personalStopBus                     = busctl.Stop
	personalFindProcess                 = os.FindProcess
	personalSignalProcess               = (*os.Process).Signal
	personalResolveAuxiliaryAccessToken = ResolveAuxiliaryAccessToken
	personalLoadTokenData               = authpkg.LoadTokenData
	personalClientID                    = authpkg.ClientID
	personalResolveAppCredentialsStrict = authpkg.ResolveAppCredentialsStrict
)

func newEventSchemaCommand() *cobra.Command {
	var asIdentity string
	var formatRaw string
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
			return renderPersonalSchema(c.OutOrStdout(), def, formatRaw)
		},
	}
	cmd.Flags().StringVar(&asIdentity, "as", "user", "事件身份: user")
	cmd.Flags().StringVarP(&formatRaw, "format", "f", "json", "输出格式: json")
	hideEventInternalFlags(cmd, "as")
	cli.AnnotateRuntimePositionals(cmd, cli.RuntimeSchemaPositional{
		Name:        "event_key",
		Type:        "string",
		Description: "要查询 payload 字段定义的个人事件码",
		Required:    true,
		Index:       0,
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

func renderPersonalSchema(w io.Writer, def personal.Definition, format string) error {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		format = "json"
	}
	if format != "json" {
		return fmt.Errorf("event schema only supports json output")
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(personal.BuildSchemaDocument(def))
}

func runPersonalEventConsume(c *cobra.Command, opts personalConsumeOptions) error {
	ctx := c.Context()
	if err := ensurePublicPersonalEvent(opts.EventKey); err != nil {
		return err
	}
	configDir := defaultConfigDir()
	identity, err := personalResolveEventIdentity(ctx, configDir, opts.StreamSourceID)
	if err != nil {
		return fmt.Errorf("event consume --as user: %w", err)
	}
	identityHash := dwsevent.IdentityHash(identity.Key())
	editionName := editionNameOrDefault()
	workDir := eventWorkDir(configDir, editionName, dwsevent.SourceKindPersonalStream, identityHash)
	ipcEndpoint := defaultIPCEndpoint(workDir, editionName, dwsevent.SourceKindPersonalStream, identityHash)

	routes, err := consume.ParseRoutes(opts.Common.RoutesRaw)
	if err != nil {
		return fmt.Errorf("event consume --as user: %w", err)
	}
	rawFormat := ""
	if f := c.Flags().Lookup("format"); f != nil && f.Changed {
		rawFormat = opts.Common.FormatRaw
	}
	normalised, fellback := consume.NormalizeFormat(rawFormat)
	if fellback && !opts.Common.Quiet {
		fmt.Fprintf(c.ErrOrStderr(), "WARN: --format %q has no meaning for event stream; using ndjson\n", rawFormat)
	}

	if opts.Common.DryRun {
		cfg := consume.Config{
			WorkDir:        workDir,
			IPCEndpoint:    ipcEndpoint,
			ClientID:       identity.ClientID,
			SpawnExtraArgs: personalBusSpawnArgs(identity, opts.StreamTicketMode, personalEventStreamTicketURL(opts.StreamTicketURL, configDir)),
			Compact:        opts.Common.Compact,
			MaxEvents:      opts.Common.MaxEvents,
			Duration:       opts.Common.Duration,
			EventKey:       opts.EventKey,
			Format:         normalised,
			OutputDir:      opts.Common.OutputDir,
			Routes:         routes,
			Stderr:         c.ErrOrStderr(),
			Quiet:          opts.Common.Quiet,
			Foreground:     opts.Common.Foreground,
			Force:          opts.Common.Force,
			DryRun:         true,
		}
		applyPersonalConsumeFilters(&cfg, opts, strings.TrimSpace(opts.SubscribeID), opts.EventKey)
		return personalConsumeRun(ctx, cfg)
	}

	client := personal.NewClient(personalEventControlBaseURL(opts.ControlBaseURL, configDir), identity)
	sub, eventKey, ruleType, err := personalEnsureSubscription(ctx, client, identity, opts)
	if err != nil {
		return fmt.Errorf("event consume --as user: %w", err)
	}
	if sub.SubscribeID == "" {
		return fmt.Errorf("event consume --as user: server returned empty subscribe_id")
	}
	if err := personalUpsertRunState(workDir, personal.RunState{
		SubscribeID:  sub.SubscribeID,
		EventKey:     eventKey,
		RuleType:     ruleType,
		ClientID:     identity.ClientID,
		SourceID:     identity.SourceID,
		IdentityHash: identityHash,
	}); err != nil {
		return fmt.Errorf("event consume --as user: save run state: %w", err)
	}
	cleanup := func() {
		_ = personalDeleteSubscription(client, context.Background(), sub.SubscribeID)
		_ = personalRemoveRunStates(workDir, []string{sub.SubscribeID})
	}
	// Ownership-based cleanup (AI-subprocess contract, aligned with
	// lark-cli): a subscription this run CREATED is unsubscribed on exit
	// (any exit — SIGTERM / stdin-EOF / limit / timeout / error), so nothing
	// leaks server-side. A subscription REUSED via --subscribe-id is left
	// intact — the caller owns its lifecycle. --ephemeral forces cleanup
	// either way.
	selfCreated := strings.TrimSpace(opts.SubscribeID) == ""
	if opts.Ephemeral || selfCreated {
		defer cleanup()
	}

	cfg := consume.Config{
		WorkDir:        workDir,
		IPCEndpoint:    ipcEndpoint,
		ClientID:       identity.ClientID,
		SpawnExtraArgs: personalBusSpawnArgs(identity, opts.StreamTicketMode, opts.StreamTicketURL),
		Compact:        opts.Common.Compact,
		MaxEvents:      opts.Common.MaxEvents,
		Duration:       opts.Common.Duration,
		EventKey:       eventKey,
		Format:         normalised,
		OutputDir:      opts.Common.OutputDir,
		Routes:         routes,
		Stdout:         c.OutOrStdout(),
		Stderr:         c.ErrOrStderr(),
		Quiet:          opts.Common.Quiet,
		Foreground:     opts.Common.Foreground,
		Force:          opts.Common.Force,
	}
	// Arm the stdin-EOF shutdown watcher only for a pipe-style, unbounded
	// run (see shouldWatchStdinEOF).
	applyEventConsumeStdin(&cfg, opts.Common.MaxEvents, opts.Common.Duration, c.InOrStdin())
	applyPersonalConsumeFilters(&cfg, opts, sub.SubscribeID, eventKey)
	if opts.DebugRawEvents && !opts.Common.Quiet {
		fmt.Fprintf(c.ErrOrStderr(), "debug raw events enabled: local event filters disabled\nworkdir: %s\nbus_log: %s\n",
			workDir, filepath.Join(workDir, "bus.log"))
	}
	if err := personalValidateConsumeConfig(cfg); err != nil {
		return err
	}
	if o := c.Flags().Lookup("output"); o != nil && o.Changed {
		if err := personalValidateNoOutputConflict(cfg, o.Value.String()); err != nil {
			return err
		}
	}
	if opts.Common.Foreground {
		src, err := personalNewStreamSource(ctx, personalStreamSourceOptions{
			ConfigDir:  configDir,
			Identity:   identity,
			TicketMode: opts.StreamTicketMode,
			TicketURL:  opts.StreamTicketURL,
		})
		if err != nil {
			if !opts.Ephemeral {
				cleanup()
			}
			return err
		}
		busCfg := bus.Config{
			WorkDir:      workDir,
			IPCEndpoint:  ipcEndpoint,
			ClientID:     identity.ClientID,
			Edition:      editionName,
			SourceKind:   dwsevent.SourceKindPersonalStream,
			IdentityHash: identityHash,
			SourceID:     identity.SourceID,
			Source:       src,
		}
		bus.ApplyEnvTuning(&busCfg)
		err = personalBusRun(ctx, busCfg)
		if err != nil && !opts.Ephemeral {
			cleanup()
		}
		return err
	}
	err = personalConsumeRun(ctx, cfg)
	if err != nil && !opts.Ephemeral {
		cleanup()
	}
	return err
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

func ensurePersonalSubscription(ctx context.Context, client *personal.Client, identity personal.Identity, opts personalConsumeOptions) (*personal.Subscription, string, string, error) {
	if strings.TrimSpace(opts.SubscribeID) != "" {
		sub, err := personalGetSubscription(client, ctx, opts.SubscribeID)
		if err != nil {
			return nil, "", "", err
		}
		eventKey := firstNonEmptyPersonalString(opts.EventKey, sub.EventKey)
		if eventKey == "" {
			return nil, "", "", fmt.Errorf("event_key is required when --subscribe-id lookup returns no event_key")
		}
		if err := ensurePublicPersonalEvent(eventKey); err != nil {
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
	if strings.TrimSpace(opts.EventKey) == "" {
		return nil, "", "", fmt.Errorf("event_key is required unless --subscribe-id is provided")
	}
	if err := ensurePublicPersonalEvent(opts.EventKey); err != nil {
		return nil, "", "", err
	}
	ruleType, ruleParam, err := personal.BuildRuleParam(opts.EventKey, personal.RuleOptions{
		RuleType: opts.Rule,
		UserID:   opts.UserID,
		GroupID:  opts.GroupID,
	})
	if err != nil {
		return nil, "", "", err
	}
	filter, filterCanonical, err := personal.BuildFilter(opts.FilterJSON, opts.QueryCSV)
	if err != nil {
		return nil, "", "", err
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
	sub, err := personalCreateSubscription(client, ctx, req)
	if err != nil {
		return nil, "", "", err
	}
	return sub, opts.EventKey, ruleType, nil
}

func runPersonalEventStatus(c *cobra.Command, opts personalStatusOptions) error {
	ctx := c.Context()
	if err := ensurePublicPersonalEvent(opts.EventKey); err != nil {
		return err
	}
	configDir := defaultConfigDir()
	identity, err := personalResolveEventIdentity(ctx, configDir, opts.StreamSourceID)
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
	subs, err := personalListSubscriptions(personal.NewClient(personalEventControlBaseURL(opts.ControlBaseURL, configDir), identity), ctx, personal.ListOptions{
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

func ensurePublicPersonalEvent(eventKey string) error {
	eventKey = strings.TrimSpace(eventKey)
	if eventKey == "" {
		return nil
	}
	if def, ok := personal.Lookup(eventKey); ok && !def.Public {
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
	identity, err := personalResolveEventIdentity(ctx, configDir, opts.StreamSourceID)
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
	client := personal.NewClient(personalEventControlBaseURL(opts.ControlBaseURL, configDir), identity)
	for _, id := range subscribeIDs {
		if err := personalDeleteSubscription(client, ctx, id); err != nil {
			return fmt.Errorf("event stop --as user: cancel subscription %s: %w", id, err)
		}
	}
	if err := personalRemoveRunStates(workDir, subscribeIDs); err != nil {
		return fmt.Errorf("event stop --as user: update local state: %w", err)
	}
	if err := interruptPersonalConsumers(ipcEndpoint, subscribeIDs); err != nil {
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
	if err := personalStopBus(busctl.StopConfig{WorkDir: workDir}); err != nil {
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

func printPersonalStopResult(w io.Writer, subscribeIDs []string, single bool, busState string) {
	if single && len(subscribeIDs) == 1 {
		fmt.Fprintf(w, "cancelled personal subscription %s; %s\n", subscribeIDs[0], busState)
		return
	}
	fmt.Fprintf(w, "cancelled %d personal subscription(s); %s\n", len(subscribeIDs), busState)
}

func resolvePersonalEventIdentity(ctx context.Context, configDir string, sourceIDOverride string) (personal.Identity, error) {
	accessToken, err := personalResolveAuxiliaryAccessToken(ctx, configDir, "")
	if err != nil {
		return personal.Identity{}, err
	}
	tokenData, _ := personalLoadTokenData(configDir)
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

func personalTokenSubject(kind, token string) string {
	token = strings.TrimSpace(token)
	if token == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(token))
	return strings.TrimSpace(kind) + ":" + hex.EncodeToString(sum[:])
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
	_ = ctx
	return source.NewPersonal(source.PersonalConfig{
		AccessToken:  opts.Identity.AccessToken,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		SourceID:     opts.Identity.SourceID,
		TicketURL:    ticketURL,
		TicketMode:   mode,
		HTTPClient:   &http.Client{Timeout: 30 * time.Second},
	})
}

func personalBusSpawnArgs(identity personal.Identity, ticketMode, ticketURL string) []string {
	args := []string{
		"--source-kind", string(dwsevent.SourceKindPersonalStream),
		"--stream-source-id", identity.SourceID,
	}
	// Forward the organization so the detached _bus child resolves
	// credentials for the SAME profile the parent used. Without this the
	// child falls back to the default profile's token slot and fails to
	// authenticate the personal stream for a non-default `--profile`
	// (symptom: "bus child reported startup failure on ready pipe", no
	// bus.log). --profile accepts a corpId; the root pre-parses it into the
	// runtime profile before the _bus handler resolves the identity.
	if cid := strings.TrimSpace(identity.CorpID); cid != "" {
		args = append(args, "--profile", cid)
	}
	if strings.TrimSpace(ticketMode) != "" {
		args = append(args, "--stream-ticket-mode", ticketMode)
	}
	if strings.TrimSpace(ticketURL) != "" {
		args = append(args, "--stream-ticket-url", ticketURL)
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
	return config.DefaultMCPBaseURL
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
