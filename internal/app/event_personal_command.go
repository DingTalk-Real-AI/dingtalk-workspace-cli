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
	"net/http"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
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
	Common             commonConsumeOptions
	EventKey           string
	SubscribeID        string
	Rule               string
	Name               string
	FilterJSON         string
	KeywordCSV         string
	TTL                time.Duration
	Ephemeral          bool
	PeerUserID         string
	PeerUnionID        string
	SenderUserID       string
	SenderUnionID      string
	OpenConversationID string
	ControlBaseURL     string
	StreamTicketMode   string
	StreamTicketURL    string
	StreamSourceID     string
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
}

type personalStopOptions struct {
	SubscribeID    string
	All            bool
	ControlBaseURL string
}

type personalStreamSourceOptions struct {
	ConfigDir        string
	Identity         personal.Identity
	TicketMode       string
	TicketURL        string
	ClientIDOverride string
}

func newEventSchemaCommand() *cobra.Command {
	var asIdentity string
	var formatRaw string
	cmd := &cobra.Command{
		Use:               "schema <event_key>",
		Short:             "显示事件 schema",
		Args:              cobra.ExactArgs(1),
		DisableAutoGenTag: true,
		RunE: func(c *cobra.Command, args []string) error {
			if normalizeEventAs(asIdentity) != "user" {
				return fmt.Errorf("event schema currently supports --as user")
			}
			def, ok := personal.Lookup(args[0])
			if !ok {
				return fmt.Errorf("unknown personal event key %q", args[0])
			}
			return renderPersonalSchema(c.OutOrStdout(), def, formatRaw)
		},
	}
	cmd.Flags().StringVar(&asIdentity, "as", "bot", "事件身份: bot|user；user 显示个人事件 schema")
	cmd.Flags().StringVarP(&formatRaw, "format", "f", "table", "输出格式: table|json")
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
	fmt.Fprintln(tw, "EVENT_KEY\tRULE\tSTATUS\tSCHEMA_IDS\tDESCRIPTION")
	for _, it := range items {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			it.EventKey, it.RuleType, it.Status, strings.Join(it.SchemaIDs, ","), it.Description)
	}
	return tw.Flush()
}

func renderPersonalSchema(w io.Writer, def personal.Definition, format string) error {
	if format == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(def)
	}
	fmt.Fprintf(w, "EventKey : %s\n", def.EventKey)
	fmt.Fprintf(w, "Rule     : %s\n", def.RuleType)
	fmt.Fprintf(w, "Status   : %s\n", def.Status)
	fmt.Fprintf(w, "Category : %s\n", def.Category)
	fmt.Fprintf(w, "Schemas  : %s\n", strings.Join(def.SchemaIDs, ","))
	if len(def.RequiredParams) > 0 {
		fmt.Fprintf(w, "Required : %s\n", strings.Join(def.RequiredParams, ", "))
	}
	fmt.Fprintf(w, "Desc     : %s\n", def.Description)
	return nil
}

func runPersonalEventConsume(c *cobra.Command, opts personalConsumeOptions) error {
	ctx := c.Context()
	configDir := defaultConfigDir()
	identity, err := resolvePersonalEventIdentity(ctx, configDir, opts.StreamSourceID)
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
			WorkDir:     workDir,
			IPCEndpoint: ipcEndpoint,
			ClientID:    identity.ClientID,
			EventTypes:  personalEventTypes(opts.EventKey, opts.Common.EventTypes),
			Filter:      opts.Common.Filter,
			SubscribeID: strings.TrimSpace(opts.SubscribeID),
			Compact:     opts.Common.Compact,
			MaxEvents:   opts.Common.MaxEvents,
			Duration:    opts.Common.Duration,
			Format:      normalised,
			OutputDir:   opts.Common.OutputDir,
			Routes:      routes,
			Stderr:      c.ErrOrStderr(),
			Quiet:       opts.Common.Quiet,
			Foreground:  opts.Common.Foreground,
			Force:       opts.Common.Force,
			DryRun:      true,
		}
		return consume.Run(ctx, cfg)
	}

	client := personal.NewClient(opts.ControlBaseURL, identity)
	sub, eventKey, ruleType, err := ensurePersonalSubscription(ctx, client, identity, opts)
	if err != nil {
		return fmt.Errorf("event consume --as user: %w", err)
	}
	if sub.SubscribeID == "" {
		return fmt.Errorf("event consume --as user: server returned empty subscribe_id")
	}
	if err := personal.UpsertRunState(workDir, personal.RunState{
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
		_ = client.DeleteSubscription(context.Background(), sub.SubscribeID)
		_ = personal.RemoveRunStates(workDir, []string{sub.SubscribeID})
	}
	if opts.Ephemeral {
		defer cleanup()
	}

	cfg := consume.Config{
		WorkDir:        workDir,
		IPCEndpoint:    ipcEndpoint,
		ClientID:       identity.ClientID,
		SpawnExtraArgs: personalBusSpawnArgs(identity, opts.StreamTicketMode, opts.StreamTicketURL),
		EventTypes:     personalEventTypes(eventKey, opts.Common.EventTypes),
		Filter:         opts.Common.Filter,
		SubscribeID:    sub.SubscribeID,
		Compact:        opts.Common.Compact,
		MaxEvents:      opts.Common.MaxEvents,
		Duration:       opts.Common.Duration,
		Format:         normalised,
		OutputDir:      opts.Common.OutputDir,
		Routes:         routes,
		Stdout:         c.OutOrStdout(),
		Stderr:         c.ErrOrStderr(),
		Quiet:          opts.Common.Quiet,
		Foreground:     opts.Common.Foreground,
		Force:          opts.Common.Force,
	}
	if err := consume.ValidateConfig(cfg); err != nil {
		return err
	}
	if o := c.Flags().Lookup("output"); o != nil && o.Changed {
		if err := consume.ValidateNoOutputConflict(cfg, o.Value.String()); err != nil {
			return err
		}
	}
	if opts.Common.Foreground {
		src, err := newPersonalStreamSource(ctx, personalStreamSourceOptions{
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
		err = bus.Run(ctx, busCfg)
		if err != nil && !opts.Ephemeral {
			cleanup()
		}
		return err
	}
	err = consume.Run(ctx, cfg)
	if err != nil && !opts.Ephemeral {
		cleanup()
	}
	return err
}

func ensurePersonalSubscription(ctx context.Context, client *personal.Client, identity personal.Identity, opts personalConsumeOptions) (*personal.Subscription, string, string, error) {
	if strings.TrimSpace(opts.SubscribeID) != "" {
		sub, err := client.GetSubscription(ctx, opts.SubscribeID)
		if err != nil {
			return nil, "", "", err
		}
		eventKey := firstNonEmptyString(opts.EventKey, sub.EventKey)
		if eventKey == "" {
			return nil, "", "", fmt.Errorf("event_key is required when --subscribe-id lookup returns no event_key")
		}
		ruleType := firstNonEmptyString(sub.RuleType, opts.Rule)
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
	ruleType, ruleParam, err := personal.BuildRuleParam(opts.EventKey, personal.RuleOptions{
		RuleType:           opts.Rule,
		PeerUserID:         opts.PeerUserID,
		PeerUnionID:        opts.PeerUnionID,
		SenderUserID:       opts.SenderUserID,
		SenderUnionID:      opts.SenderUnionID,
		OpenConversationID: opts.OpenConversationID,
	})
	if err != nil {
		return nil, "", "", err
	}
	filter, filterCanonical, err := personal.BuildFilter(opts.FilterJSON, opts.KeywordCSV)
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
	sub, err := client.CreateSubscription(ctx, req)
	if err != nil {
		return nil, "", "", err
	}
	return sub, opts.EventKey, ruleType, nil
}

func runPersonalEventStatus(c *cobra.Command, opts personalStatusOptions) error {
	ctx := c.Context()
	configDir := defaultConfigDir()
	identity, err := resolvePersonalEventIdentity(ctx, configDir, "")
	if err != nil {
		return fmt.Errorf("event status --as user: %w", err)
	}
	identityHash := dwsevent.IdentityHash(identity.Key())
	editionName := editionNameOrDefault()
	workDir := eventWorkDir(configDir, editionName, dwsevent.SourceKindPersonalStream, identityHash)
	entry := busctl.FindBusByIdentity(configDir, editionName, dwsevent.SourceKindPersonalStream, identityHash)
	var qs busctl.EntryStatus
	if entry != nil {
		qs = busctl.QueryEntry(*entry)
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
	subs, err := personal.NewClient(opts.ControlBaseURL, identity).ListSubscriptions(ctx, personal.ListOptions{
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

func renderPersonalStatusText(w io.Writer, identity personal.Identity, identityHash string, subs []personal.Subscription, qs busctl.EntryStatus) {
	fmt.Fprintf(w, "Personal identity: corp=%s user=%s client=%s source=%s hash=%s\n",
		identity.CorpID, identity.UserID, identity.ClientID, identity.SourceID, identityHash)
	fmt.Fprintf(w, "Bus: %s", qs.Entry.State)
	if qs.Entry.HolderPID > 0 {
		fmt.Fprintf(w, " pid=%d", qs.Entry.HolderPID)
	}
	fmt.Fprintf(w, "\nWorkdir: %s\n", qs.Entry.WorkDir)
	if len(subs) == 0 {
		fmt.Fprintln(w, "Subscriptions: none")
		return
	}
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SUBSCRIBE_ID\tEVENT_KEY\tRULE\tSTATUS\tSOURCE")
	for _, sub := range subs {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			sub.SubscribeID, sub.EventKey, sub.RuleType, sub.Status, sub.SourceID)
	}
	_ = tw.Flush()
}

func runPersonalEventStop(c *cobra.Command, opts personalStopOptions) error {
	ctx := c.Context()
	configDir := defaultConfigDir()
	identity, err := resolvePersonalEventIdentity(ctx, configDir, "")
	if err != nil {
		return fmt.Errorf("event stop --as user: %w", err)
	}
	identityHash := dwsevent.IdentityHash(identity.Key())
	workDir := eventWorkDir(configDir, editionNameOrDefault(), dwsevent.SourceKindPersonalStream, identityHash)
	subscribeIDs, err := personalStopTargets(workDir, opts.SubscribeID)
	if err != nil {
		return fmt.Errorf("event stop --as user: %w", err)
	}
	client := personal.NewClient(opts.ControlBaseURL, identity)
	for _, id := range subscribeIDs {
		if err := client.DeleteSubscription(ctx, id); err != nil {
			return fmt.Errorf("event stop --as user: cancel subscription %s: %w", id, err)
		}
	}
	if err := personal.RemoveRunStates(workDir, subscribeIDs); err != nil {
		return fmt.Errorf("event stop --as user: update local state: %w", err)
	}
	if err := busctl.Stop(busctl.StopConfig{WorkDir: workDir}); err != nil {
		if errors.Is(err, busctl.ErrNotRunning) {
			if len(subscribeIDs) == 0 {
				fmt.Fprintln(c.OutOrStdout(), "personal bus is not running")
			} else {
				fmt.Fprintf(c.OutOrStdout(), "cancelled %d personal subscription(s); personal bus is not running\n", len(subscribeIDs))
			}
			return nil
		}
		return err
	}
	if len(subscribeIDs) == 0 {
		fmt.Fprintln(c.OutOrStdout(), "personal bus stopped")
		return nil
	}
	fmt.Fprintf(c.OutOrStdout(), "cancelled %d personal subscription(s); personal bus stopped\n", len(subscribeIDs))
	return nil
}

func personalStopTargets(workDir, explicit string) ([]string, error) {
	explicit = strings.TrimSpace(explicit)
	if explicit != "" {
		return []string{explicit}, nil
	}
	states, err := personal.LoadRunStates(workDir)
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

func resolvePersonalEventIdentity(ctx context.Context, configDir string, sourceIDOverride string) (personal.Identity, error) {
	accessToken, err := ResolveAuxiliaryAccessToken(ctx, configDir, "")
	if err != nil {
		return personal.Identity{}, err
	}
	tokenData, _ := authpkg.LoadTokenData(configDir)
	var corpID, userID, clientID string
	if tokenData != nil {
		corpID = tokenData.CorpID
		userID = tokenData.UserID
		clientID = tokenData.ClientID
	}
	if corpID == "" {
		corpID = resolveRuntimeDefault(ctx, "$corpId")
	}
	if userID == "" {
		userID = resolveRuntimeDefault(ctx, "$currentUserId")
	}
	if clientID == "" {
		clientID = authpkg.ClientID()
	}
	if clientID == "" {
		if id, _, _, _, err := authpkg.ResolveAppCredentialsStrict(configDir); err == nil {
			clientID = id
		}
	}
	if corpID == "" || userID == "" {
		return personal.Identity{}, fmt.Errorf("current OAuth token is missing corp_id/user_id; run dws auth login again")
	}
	if clientID == "" {
		return personal.Identity{}, fmt.Errorf("cannot resolve OAuth client_id for personal events")
	}
	sourceID := strings.TrimSpace(sourceIDOverride)
	if sourceID == "" {
		sourceID = edition.PersonalEventSourceID()
	}
	return personal.Identity{
		AccessToken: accessToken,
		CorpID:      corpID,
		UserID:      userID,
		ClientID:    clientID,
		SourceID:    sourceID,
	}, nil
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
		ticketURL = strings.TrimRight(config.GetMCPBaseURL(), "/") + "/stream/connections/ticket"
	}
	clientID := opts.Identity.ClientID
	clientSecret := ""
	if mode == "custom" {
		resolvedID, secret, _, _, err := authpkg.ResolveAppCredentialsStrict(opts.ConfigDir)
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
		"--source-id", identity.SourceID,
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
		"corp_id":       identity.CorpID,
		"user_id":       identity.UserID,
		"client_id":     identity.ClientID,
		"source_id":     identity.SourceID,
		"identity_hash": identityHash,
	}
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
