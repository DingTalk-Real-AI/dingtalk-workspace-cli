package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/source"
	eventtransport "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoveragePersonalEventRemainingSchemaAndSubscriptionCoverage(t *testing.T) {
	for _, args := range [][]string{
		{"known", "--as", "app"},
		{"not-a-real-event"},
	} {
		cmd := newEventSchemaCommand()
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err == nil {
			t.Fatalf("schema args %#v succeeded", args)
		}
	}

	oldGet := personalGetSubscription
	oldCreate := personalCreateSubscription
	t.Cleanup(func() {
		personalGetSubscription = oldGet
		personalCreateSubscription = oldCreate
	})
	client := personal.NewClient("https://example.test", personal.Identity{})
	wantErr := errors.New("subscription")
	personalGetSubscription = func(*personal.Client, context.Context, string) (*personal.Subscription, error) { return nil, wantErr }
	if _, _, _, err := ensurePersonalSubscription(context.Background(), client, personal.Identity{}, personalConsumeOptions{SubscribeID: "sub"}); !errors.Is(err, wantErr) {
		t.Fatalf("get subscription error = %v", err)
	}
	personalGetSubscription = func(*personal.Client, context.Context, string) (*personal.Subscription, error) {
		return &personal.Subscription{}, nil
	}
	if _, _, _, err := ensurePersonalSubscription(context.Background(), client, personal.Identity{}, personalConsumeOptions{SubscribeID: "sub"}); err == nil {
		t.Fatal("empty subscription event key succeeded")
	}
	personalGetSubscription = func(*personal.Client, context.Context, string) (*personal.Subscription, error) {
		return &personal.Subscription{EventKey: personal.EventFromUser}, nil
	}
	if _, key, rule, err := ensurePersonalSubscription(context.Background(), client, personal.Identity{}, personalConsumeOptions{SubscribeID: "sub"}); err != nil || key != personal.EventFromUser || rule != "sender" {
		t.Fatalf("sender subscription = %q %q, %v", key, rule, err)
	}
	personalGetSubscription = func(*personal.Client, context.Context, string) (*personal.Subscription, error) {
		return &personal.Subscription{EventKey: personal.EventMention}, nil
	}
	if _, _, rule, err := ensurePersonalSubscription(context.Background(), client, personal.Identity{}, personalConsumeOptions{SubscribeID: "sub"}); err != nil || rule == "" {
		t.Fatalf("default subscription rule = %q, %v", rule, err)
	}
	personalCreateSubscription = func(*personal.Client, context.Context, personal.CreateSubscriptionRequest) (*personal.Subscription, error) {
		return nil, wantErr
	}
	if _, _, _, err := ensurePersonalSubscription(context.Background(), client, personal.Identity{}, personalConsumeOptions{EventKey: personal.EventMention}); !errors.Is(err, wantErr) {
		t.Fatalf("create subscription error = %v", err)
	}
}

func TestCrossPlatformCoveragePersonalEventRemainingConsumeCoverage(t *testing.T) {
	oldIdentity := personalResolveEventIdentity
	oldEnsure := personalEnsureSubscription
	oldUpsert := personalUpsertRunState
	oldDelete := personalDeleteSubscription
	oldRemove := personalRemoveRunStates
	oldConsume := personalConsumeRun
	oldValidate := personalValidateConsumeConfig
	oldConflict := personalValidateNoOutputConflict
	oldNewSource := personalNewStreamSource
	oldBusRun := personalBusRun
	t.Cleanup(func() {
		personalResolveEventIdentity = oldIdentity
		personalEnsureSubscription = oldEnsure
		personalUpsertRunState = oldUpsert
		personalDeleteSubscription = oldDelete
		personalRemoveRunStates = oldRemove
		personalConsumeRun = oldConsume
		personalValidateConsumeConfig = oldValidate
		personalValidateNoOutputConflict = oldConflict
		personalNewStreamSource = oldNewSource
		personalBusRun = oldBusRun
	})

	wantErr := errors.New("consume")
	cmd := newPersonalCoverageCommand()
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) { return personal.Identity{}, wantErr }
	if err := runPersonalEventConsume(cmd, personalConsumeOptions{EventKey: personal.EventMention}); !errors.Is(err, wantErr) {
		t.Fatalf("identity error = %v", err)
	}
	identity := personal.Identity{AccessToken: "token", CorpID: "corp", UserID: "user", ClientID: "client", SourceID: "source"}
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) { return identity, nil }
	if err := runPersonalEventConsume(cmd, personalConsumeOptions{EventKey: personal.EventMention, Common: commonConsumeOptions{RoutesRaw: []string{"bad-route"}}}); err == nil {
		t.Fatal("invalid route succeeded")
	}
	personalConsumeRun = func(context.Context, consume.Config) error { return wantErr }
	_ = cmd.Flags().Set("format", "table")
	if err := runPersonalEventConsume(cmd, personalConsumeOptions{EventKey: personal.EventMention, Common: commonConsumeOptions{DryRun: true, FormatRaw: "bogus"}}); !errors.Is(err, wantErr) {
		t.Fatalf("dry-run consume error = %v", err)
	}

	personalEnsureSubscription = func(context.Context, *personal.Client, personal.Identity, personalConsumeOptions) (*personal.Subscription, string, string, error) {
		return nil, "", "", wantErr
	}
	if err := runPersonalEventConsume(cmd, personalConsumeOptions{EventKey: personal.EventMention}); !errors.Is(err, wantErr) {
		t.Fatalf("ensure subscription error = %v", err)
	}
	personalEnsureSubscription = func(context.Context, *personal.Client, personal.Identity, personalConsumeOptions) (*personal.Subscription, string, string, error) {
		return &personal.Subscription{}, personal.EventMention, "at", nil
	}
	if err := runPersonalEventConsume(cmd, personalConsumeOptions{EventKey: personal.EventMention}); err == nil || !strings.Contains(err.Error(), "empty subscribe_id") {
		t.Fatalf("empty subscription = %v", err)
	}
	personalEnsureSubscription = func(context.Context, *personal.Client, personal.Identity, personalConsumeOptions) (*personal.Subscription, string, string, error) {
		return &personal.Subscription{SubscribeID: "sub"}, personal.EventMention, "at", nil
	}
	personalUpsertRunState = func(string, personal.RunState) error { return wantErr }
	if err := runPersonalEventConsume(cmd, personalConsumeOptions{EventKey: personal.EventMention}); !errors.Is(err, wantErr) {
		t.Fatalf("state upsert error = %v", err)
	}

	deletes := 0
	personalUpsertRunState = func(string, personal.RunState) error { return nil }
	personalDeleteSubscription = func(*personal.Client, context.Context, string) error { deletes++; return nil }
	personalRemoveRunStates = func(string, []string) error { return nil }
	personalValidateConsumeConfig = func(consume.Config) error { return wantErr }
	if err := runPersonalEventConsume(cmd, personalConsumeOptions{EventKey: personal.EventMention, DebugRawEvents: true}); !errors.Is(err, wantErr) {
		t.Fatalf("validate error = %v", err)
	}
	personalValidateConsumeConfig = func(consume.Config) error { return nil }
	_ = cmd.Flags().Set("output", "file")
	personalValidateNoOutputConflict = func(consume.Config, string) error { return wantErr }
	if err := runPersonalEventConsume(cmd, personalConsumeOptions{EventKey: personal.EventMention}); !errors.Is(err, wantErr) {
		t.Fatalf("output conflict = %v", err)
	}
	personalValidateNoOutputConflict = func(consume.Config, string) error { return nil }

	personalNewStreamSource = func(context.Context, personalStreamSourceOptions) (*source.PersonalSource, error) {
		return nil, wantErr
	}
	if err := runPersonalEventConsume(cmd, personalConsumeOptions{EventKey: personal.EventMention, Common: commonConsumeOptions{Foreground: true}}); !errors.Is(err, wantErr) || deletes == 0 {
		t.Fatalf("foreground source error = %v deletes=%d", err, deletes)
	}
	before := deletes
	if err := runPersonalEventConsume(cmd, personalConsumeOptions{EventKey: personal.EventMention, Ephemeral: true, Common: commonConsumeOptions{Foreground: true}}); !errors.Is(err, wantErr) || deletes == before {
		t.Fatalf("ephemeral source error = %v deletes=%d", err, deletes)
	}
	personalNewStreamSource = func(context.Context, personalStreamSourceOptions) (*source.PersonalSource, error) { return nil, nil }
	personalBusRun = func(context.Context, bus.Config) error { return wantErr }
	if err := runPersonalEventConsume(cmd, personalConsumeOptions{EventKey: personal.EventMention, Common: commonConsumeOptions{Foreground: true}}); !errors.Is(err, wantErr) {
		t.Fatalf("bus run error = %v", err)
	}
	personalConsumeRun = func(context.Context, consume.Config) error { return wantErr }
	if err := runPersonalEventConsume(cmd, personalConsumeOptions{EventKey: personal.EventMention}); !errors.Is(err, wantErr) {
		t.Fatalf("background consume error = %v", err)
	}
}

func TestCrossPlatformCoveragePersonalEventRemainingStatusStopAndInterruptCoverage(t *testing.T) {
	oldIdentity := personalResolveEventIdentity
	oldFindBus := personalFindBusByIdentity
	oldQueryEntry := personalQueryEntry
	oldList := personalListSubscriptions
	oldDelete := personalDeleteSubscription
	oldRemove := personalRemoveRunStates
	oldLoad := personalLoadRunStates
	oldStop := personalStopBus
	oldQueryStatus := personalQueryStatus
	oldFindProcess := personalFindProcess
	oldSignal := personalSignalProcess
	t.Cleanup(func() {
		personalResolveEventIdentity = oldIdentity
		personalFindBusByIdentity = oldFindBus
		personalQueryEntry = oldQueryEntry
		personalListSubscriptions = oldList
		personalDeleteSubscription = oldDelete
		personalRemoveRunStates = oldRemove
		personalLoadRunStates = oldLoad
		personalStopBus = oldStop
		personalQueryStatus = oldQueryStatus
		personalFindProcess = oldFindProcess
		personalSignalProcess = oldSignal
	})

	wantErr := errors.New("status-stop")
	cmd := newPersonalCoverageCommand()
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) { return personal.Identity{}, wantErr }
	if err := runPersonalEventStatus(cmd, personalStatusOptions{}); !errors.Is(err, wantErr) {
		t.Fatalf("status identity error = %v", err)
	}
	if err := runPersonalEventStop(cmd, personalStopOptions{SubscribeID: "sub"}); !errors.Is(err, wantErr) {
		t.Fatalf("stop identity error = %v", err)
	}
	identity := personal.Identity{ClientID: "client", SourceID: "source"}
	personalResolveEventIdentity = func(context.Context, string, string) (personal.Identity, error) { return identity, nil }
	entry := &busctl.BusEntry{State: busctl.BusStateRunning}
	personalFindBusByIdentity = func(string, string, dwsevent.SourceKind, string) *busctl.BusEntry { return entry }
	personalQueryEntry = func(busctl.BusEntry) busctl.EntryStatus { return busctl.EntryStatus{Entry: *entry} }
	personalListSubscriptions = func(*personal.Client, context.Context, personal.ListOptions) ([]personal.Subscription, error) {
		return nil, wantErr
	}
	if err := runPersonalEventStatus(cmd, personalStatusOptions{}); !errors.Is(err, wantErr) {
		t.Fatalf("status list error = %v", err)
	}
	personalListSubscriptions = func(*personal.Client, context.Context, personal.ListOptions) ([]personal.Subscription, error) {
		return nil, nil
	}
	if err := runPersonalEventStatus(cmd, personalStatusOptions{Status: "all", Format: "json"}); err != nil {
		t.Fatalf("status entry JSON = %v", err)
	}

	personalLoadRunStates = func(string) ([]personal.RunState, error) { return nil, wantErr }
	if err := runPersonalEventStop(cmd, personalStopOptions{All: true}); !errors.Is(err, wantErr) {
		t.Fatalf("stop targets error = %v", err)
	}
	personalDeleteSubscription = func(*personal.Client, context.Context, string) error { return wantErr }
	if err := runPersonalEventStop(cmd, personalStopOptions{SubscribeID: "sub"}); !errors.Is(err, wantErr) {
		t.Fatalf("delete error = %v", err)
	}
	personalDeleteSubscription = func(*personal.Client, context.Context, string) error { return nil }
	personalRemoveRunStates = func(string, []string) error { return wantErr }
	if err := runPersonalEventStop(cmd, personalStopOptions{SubscribeID: "sub"}); !errors.Is(err, wantErr) {
		t.Fatalf("remove error = %v", err)
	}
	personalRemoveRunStates = func(string, []string) error { return nil }
	personalQueryStatus = func(string) (*eventtransport.StatusResp, error) { return nil, wantErr }
	personalLoadRunStates = func(string) ([]personal.RunState, error) { return nil, wantErr }
	if err := runPersonalEventStop(cmd, personalStopOptions{SubscribeID: "sub"}); !errors.Is(err, wantErr) {
		t.Fatalf("remaining state error = %v", err)
	}
	personalQueryStatus = func(string) (*eventtransport.StatusResp, error) {
		return &eventtransport.StatusResp{Consumers: []eventtransport.StatusConsumer{{SubscribeID: "sub", PID: 123}}}, nil
	}
	personalFindProcess = func(int) (*os.Process, error) { return nil, wantErr }
	personalLoadRunStates = func(string) ([]personal.RunState, error) { return []personal.RunState{{SubscribeID: "other"}}, nil }
	if err := runPersonalEventStop(cmd, personalStopOptions{SubscribeID: "sub"}); err != nil {
		t.Fatalf("interrupt warning stop = %v", err)
	}
	personalLoadRunStates = func(string) ([]personal.RunState, error) { return []personal.RunState{{SubscribeID: "other"}}, nil }
	if err := runPersonalEventStop(cmd, personalStopOptions{SubscribeID: "sub"}); err != nil {
		t.Fatalf("remaining bus stop = %v", err)
	}
	personalLoadRunStates = func(string) ([]personal.RunState, error) { return nil, nil }
	personalStopBus = func(busctl.StopConfig) error { return busctl.ErrNotRunning }
	if err := runPersonalEventStop(cmd, personalStopOptions{SubscribeID: "sub"}); err != nil {
		t.Fatalf("not running stop = %v", err)
	}
	personalStopBus = func(busctl.StopConfig) error { return wantErr }
	if err := runPersonalEventStop(cmd, personalStopOptions{SubscribeID: "sub"}); !errors.Is(err, wantErr) {
		t.Fatalf("bus stop error = %v", err)
	}

	personalLoadRunStates = func(string) ([]personal.RunState, error) {
		return []personal.RunState{{}, {SubscribeID: "b"}, {SubscribeID: "a"}}, nil
	}
	if got, err := personalStopTargets("", "", true); err != nil || strings.Join(got, ",") != "a,b" {
		t.Fatalf("stop target filtering = %#v, %v", got, err)
	}
	status := &eventtransport.StatusResp{Consumers: []eventtransport.StatusConsumer{
		{SubscribeID: "other", PID: 1},
		{SubscribeID: "sub", PID: 0},
		{SubscribeID: "sub", PID: os.Getpid()},
		{SubscribeID: "sub", PID: 123},
		{SubscribeID: "sub", PID: 123},
	}}
	personalQueryStatus = func(string) (*eventtransport.StatusResp, error) { return status, nil }
	personalFindProcess = func(int) (*os.Process, error) { return nil, wantErr }
	if err := interruptPersonalConsumers("ipc", []string{" sub ", ""}); !errors.Is(err, wantErr) {
		t.Fatalf("find process error = %v", err)
	}
	proc := &os.Process{}
	personalFindProcess = func(int) (*os.Process, error) { return proc, nil }
	personalSignalProcess = func(*os.Process, os.Signal) error { return wantErr }
	if err := interruptPersonalConsumers("ipc", []string{"sub"}); !errors.Is(err, wantErr) {
		t.Fatalf("signal process error = %v", err)
	}
	personalSignalProcess = func(*os.Process, os.Signal) error { return os.ErrProcessDone }
	if err := interruptPersonalConsumers("ipc", []string{"sub"}); err != nil {
		t.Fatalf("completed process signal = %v", err)
	}
}

func TestCrossPlatformCoveragePersonalEventRemainingIdentityAndSourceCoverage(t *testing.T) {
	oldAux := personalResolveAuxiliaryAccessToken
	oldLoad := personalLoadTokenData
	oldClientID := personalClientID
	oldCredentials := personalResolveAppCredentialsStrict
	oldEdition := edition.Get()
	t.Cleanup(func() {
		personalResolveAuxiliaryAccessToken = oldAux
		personalLoadTokenData = oldLoad
		personalClientID = oldClientID
		personalResolveAppCredentialsStrict = oldCredentials
		edition.Override(oldEdition)
	})
	wantErr := errors.New("identity")
	personalResolveAuxiliaryAccessToken = func(context.Context, string, string) (string, error) { return "", wantErr }
	if _, err := resolvePersonalEventIdentity(context.Background(), "", ""); !errors.Is(err, wantErr) {
		t.Fatalf("aux token error = %v", err)
	}
	personalResolveAuxiliaryAccessToken = func(context.Context, string, string) (string, error) { return "access", nil }
	personalLoadTokenData = func(string) (*authpkg.TokenData, error) { return nil, nil }
	personalClientID = func() string { return "" }
	personalResolveAppCredentialsStrict = func(string) (string, string, authpkg.CredentialSource, authpkg.CredentialSource, error) {
		return "resolved", "secret", "", "", nil
	}
	edition.Override(&edition.Hooks{RuntimeDefaults: func() map[string]edition.RuntimeDefaultFn {
		return map[string]edition.RuntimeDefaultFn{
			"$corpId":        func(context.Context) (string, bool) { return " corp ", true },
			"$currentUserId": func(context.Context) (string, bool) { return " user ", true },
		}
	}})
	if got, err := resolvePersonalEventIdentity(context.Background(), "", "source"); err != nil || got.ClientID != "resolved" || got.CorpID != "corp" {
		t.Fatalf("resolved identity = %#v, %v", got, err)
	}
	personalResolveAppCredentialsStrict = func(string) (string, string, authpkg.CredentialSource, authpkg.CredentialSource, error) {
		return "", "", "", "", wantErr
	}
	edition.Override(&edition.Hooks{})
	if _, err := resolvePersonalEventIdentity(context.Background(), "", ""); err == nil {
		t.Fatal("missing client ID succeeded")
	}
	if got := resolveRuntimeDefault(context.Background(), "missing"); got != "" {
		t.Fatalf("missing runtime default = %q", got)
	}

	if _, err := newPersonalStreamSource(context.Background(), personalStreamSourceOptions{ConfigDir: "", Identity: personal.Identity{}, TicketMode: "custom"}); !errors.Is(err, wantErr) {
		t.Fatalf("custom credential error = %v", err)
	}
	personalResolveAppCredentialsStrict = func(string) (string, string, authpkg.CredentialSource, authpkg.CredentialSource, error) {
		return "resolved", "secret", "", "", nil
	}
	if src, err := newPersonalStreamSource(context.Background(), personalStreamSourceOptions{Identity: personal.Identity{AccessToken: "token", SourceID: "source"}, TicketMode: "custom"}); err != nil || src == nil {
		t.Fatalf("custom resolved source = %#v, %v", src, err)
	}
	if got := personalEventStreamSourceID(""); got != "open" {
		t.Fatalf("default stream source = %q", got)
	}
	if got := configuredMCPBaseURL(""); got != "" {
		t.Fatalf("default missing configured MCP = %q", got)
	}
}

func newPersonalCoverageCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "event"}
	cmd.SetContext(context.Background())
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.Flags().String("format", "table", "")
	cmd.Flags().String("output", "", "")
	return cmd
}

var _ = time.Second
