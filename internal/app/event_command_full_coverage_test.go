package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
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

func TestCrossPlatformCoverageEventConsumeCommandAllBranchesCoverage(t *testing.T) {
	oldPersonal := eventRunPersonalConsume
	oldCreds, oldConsume, oldForeground := eventResolveCredentials, eventConsumeRun, eventRunForeground
	oldNormalize := eventNormalizeAs
	t.Cleanup(func() {
		eventRunPersonalConsume = oldPersonal
		eventResolveCredentials, eventConsumeRun, eventRunForeground = oldCreds, oldConsume, oldForeground
		eventNormalizeAs = oldNormalize
	})
	eventNormalizeAs = func(value string) (string, error) {
		if strings.EqualFold(strings.TrimSpace(value), "app") {
			return "app", nil
		}
		return normalizeEventAs(value)
	}
	fail := errors.New("failure")
	personalCalled := false
	eventRunPersonalConsume = func(*cobra.Command, personalConsumeOptions) error { personalCalled = true; return nil }
	cmd := newEventConsumeCommand()
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := cmd.RunE(cmd, []string{"event.key"}); err != nil || !personalCalled {
		t.Fatalf("personal consume = %v, called=%v", err, personalCalled)
	}

	makeApp := func() *cobra.Command {
		command := newEventConsumeCommand()
		command.SetContext(context.Background())
		command.SetOut(io.Discard)
		command.SetErr(io.Discard)
		_ = command.Flags().Set("as", "app")
		return command
	}
	invalid := newEventConsumeCommand()
	_ = invalid.Flags().Set("as", "invalid")
	if err := invalid.RunE(invalid, nil); err == nil {
		t.Fatal("invalid event identity succeeded")
	}
	debug := makeApp()
	_ = debug.Flags().Set("debug-raw-events", "true")
	if err := debug.RunE(debug, nil); err == nil {
		t.Fatal("app debug-raw-events succeeded")
	}
	userFlag := makeApp()
	_ = userFlag.Flags().Set("subscribe-id", "sub")
	if err := userFlag.RunE(userFlag, nil); err == nil {
		t.Fatal("app personal flag succeeded")
	}
	if err := makeApp().RunE(makeApp(), []string{"event.key"}); err == nil {
		t.Fatal("app event key succeeded")
	}
	eventResolveCredentials = func(string, eventStreamTicketOptions) (string, string, error) { return "", "", fail }
	if err := makeApp().RunE(makeApp(), nil); !errors.Is(err, fail) {
		t.Fatalf("event credentials error = %v", err)
	}
	eventResolveCredentials = func(string, eventStreamTicketOptions) (string, string, error) { return "client", "secret", nil }
	badRoute := makeApp()
	_ = badRoute.Flags().Set("route", "bad")
	if err := badRoute.RunE(badRoute, nil); err == nil {
		t.Fatal("invalid event route succeeded")
	}
	invalidConfig := makeApp()
	_ = invalidConfig.Flags().Set("format", "json")
	if err := invalidConfig.RunE(invalidConfig, nil); err == nil {
		t.Fatal("unbounded JSON event stream succeeded")
	}
	conflict := makeApp()
	conflict.Flags().String("output", "", "")
	_ = conflict.Flags().Set("output-dir", t.TempDir())
	_ = conflict.Flags().Set("output", filepath.Join(t.TempDir(), "out"))
	if err := conflict.RunE(conflict, nil); err == nil {
		t.Fatal("event output conflict succeeded")
	}
	eventConsumeRun = func(context.Context, consume.Config) error { return fail }
	t.Setenv(authpkg.EnvClientID, "half-set")
	t.Setenv(authpkg.EnvClientSecret, "")
	valid := makeApp()
	_ = valid.Flags().Set("format", "table")
	_ = valid.Flags().Set("max-events", "1")
	if err := valid.RunE(valid, nil); !errors.Is(err, fail) {
		t.Fatalf("event consume run error = %v", err)
	}
	eventRunForeground = func(context.Context, consume.Config, string, string, eventStreamTicketOptions) error { return fail }
	foreground := makeApp()
	_ = foreground.Flags().Set("foreground", "true")
	if err := foreground.RunE(foreground, nil); !errors.Is(err, fail) {
		t.Fatalf("foreground event error = %v", err)
	}
}

func TestCrossPlatformCoverageEventSourcesAndForegroundCoverage(t *testing.T) {
	oldNew, oldToken, oldEventSource, oldBus := eventNewDingtalkSource, eventResolveAccessToken, eventNewEventSource, eventBusRun
	oldEdition := edition.Get()
	t.Cleanup(func() {
		eventNewDingtalkSource, eventResolveAccessToken = oldNew, oldToken
		eventNewEventSource, eventBusRun = oldEventSource, oldBus
		edition.Override(oldEdition)
	})
	fail := errors.New("failure")
	eventNewDingtalkSource = func(source.Config, ...source.SourceOption) (*source.DingtalkSource, error) { return nil, fail }
	if _, err := newEventSource(context.Background(), "config", "client", "secret", eventStreamTicketOptions{}); !errors.Is(err, fail) {
		t.Fatalf("SDK event source error = %v", err)
	}
	eventNewDingtalkSource = func(source.Config, ...source.SourceOption) (*source.DingtalkSource, error) {
		return &source.DingtalkSource{}, nil
	}
	if _, err := newEventSource(context.Background(), "config", "client", "secret", eventStreamTicketOptions{}); err != nil {
		t.Fatal(err)
	}
	stream := eventStreamTicketOptions{Mode: "custom"}
	var captured source.Config
	eventNewDingtalkSource = func(cfg source.Config, _ ...source.SourceOption) (*source.DingtalkSource, error) {
		captured = cfg
		return &source.DingtalkSource{}, nil
	}
	eventResolveAccessToken = func(context.Context, string, string) (string, error) { return "", fail }
	if _, err := newEventSource(context.Background(), "config", "client", "secret", stream); err != nil {
		t.Fatalf("stream source construction = %v", err)
	}
	if _, err := captured.PortalTicket.AccessTokenProvider(context.Background()); !errors.Is(err, fail) {
		t.Fatalf("stream token provider error = %v", err)
	}
	eventResolveAccessToken = func(context.Context, string, string) (string, error) { return "token", nil }
	for _, mode := range []string{"custom", "normal"} {
		if _, err := newEventSource(context.Background(), "config", "client", "secret", eventStreamTicketOptions{Mode: mode}); err != nil {
			t.Fatalf("stream source %s = %v", mode, err)
		}
	}

	edition.Override(&edition.Hooks{Name: "test"})
	eventNewEventSource = func(context.Context, string, string, string, eventStreamTicketOptions) (*source.DingtalkSource, error) {
		return nil, fail
	}
	if err := runForegroundBus(context.Background(), consume.Config{}, "config", "secret", eventStreamTicketOptions{}); !errors.Is(err, fail) {
		t.Fatalf("foreground source error = %v", err)
	}
	eventNewEventSource = func(context.Context, string, string, string, eventStreamTicketOptions) (*source.DingtalkSource, error) {
		return &source.DingtalkSource{}, nil
	}
	eventBusRun = func(context.Context, bus.Config) error { return fail }
	cfg := consume.Config{WorkDir: t.TempDir(), IPCEndpoint: filepath.Join(t.TempDir(), "bus.sock"), ClientID: "client"}
	if err := runForegroundBus(context.Background(), cfg, "config", "secret", eventStreamTicketOptions{}); !errors.Is(err, fail) {
		t.Fatalf("foreground bus error = %v", err)
	}

	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte("https://pre-mcp.example.test"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DWS_STREAM_SOURCE_ID", "")
	if got := defaultEventStreamSourceID(); got != "pre_open_source" {
		t.Fatalf("pre stream source ID = %q", got)
	}
}

func TestCrossPlatformCoverageEventBusCommandAllBranchesCoverage(t *testing.T) {
	oldReady, oldPersonal, oldPersonalSource := eventReadyFDFromEnv, eventResolvePersonal, eventNewPersonalSource
	oldCreds, oldSource, oldRun := eventResolveCredentials, eventNewEventSource, eventBusRun
	oldMkdir, oldOpen := eventMkdirAll, eventOpenFile
	t.Cleanup(func() {
		eventReadyFDFromEnv, eventResolvePersonal, eventNewPersonalSource = oldReady, oldPersonal, oldPersonalSource
		eventResolveCredentials, eventNewEventSource, eventBusRun = oldCreds, oldSource, oldRun
		eventMkdirAll, eventOpenFile = oldMkdir, oldOpen
	})
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	fail := errors.New("failure")
	makeBus := func(kind string) *cobra.Command {
		cmd := newEventBusCommand()
		cmd.SetContext(context.Background())
		_ = cmd.Flags().Set("source-kind", kind)
		return cmd
	}
	read, write, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	eventReadyFDFromEnv = func() *os.File { return write }
	eventResolvePersonal = func(context.Context, string, string) (personal.Identity, error) { return personal.Identity{}, fail }
	if err := makeBus(string(dwsevent.SourceKindPersonalStream)).RunE(makeBus(string(dwsevent.SourceKindPersonalStream)), nil); !errors.Is(err, fail) {
		t.Fatalf("personal identity error = %v", err)
	}
	marker := make([]byte, 1)
	_, _ = read.Read(marker)
	_ = read.Close()
	if marker[0] != 'E' {
		t.Fatalf("ready failure marker = %q", marker)
	}
	eventReadyFDFromEnv = func() *os.File { return nil }
	eventResolvePersonal = func(context.Context, string, string) (personal.Identity, error) {
		return personal.Identity{AccessToken: "token", ClientID: "client", SourceID: "open"}, nil
	}
	eventNewPersonalSource = func(context.Context, personalStreamSourceOptions) (*source.PersonalSource, error) { return nil, fail }
	personalCmd := makeBus(string(dwsevent.SourceKindPersonalStream))
	_ = personalCmd.Flags().Set("client-id", "override")
	if err := personalCmd.RunE(personalCmd, nil); !errors.Is(err, fail) {
		t.Fatalf("personal stream source error = %v", err)
	}
	eventNewPersonalSource = func(context.Context, personalStreamSourceOptions) (*source.PersonalSource, error) {
		return &source.PersonalSource{}, nil
	}
	eventMkdirAll = func(string, os.FileMode) error { return nil }
	eventOpenFile = func(string, int, os.FileMode) (*os.File, error) { return os.CreateTemp(t.TempDir(), "bus-log") }
	eventBusRun = func(context.Context, bus.Config) error { return fail }
	if err := personalCmd.RunE(personalCmd, nil); !errors.Is(err, fail) {
		t.Fatalf("personal bus run error = %v", err)
	}

	eventResolveCredentials = func(string, eventStreamTicketOptions) (string, string, error) { return "", "", fail }
	if err := makeBus(string(dwsevent.SourceKindAppStream)).RunE(makeBus(string(dwsevent.SourceKindAppStream)), nil); !errors.Is(err, fail) {
		t.Fatalf("app bus credentials error = %v", err)
	}
	eventResolveCredentials = func(string, eventStreamTicketOptions) (string, string, error) { return "client", "secret", nil }
	eventNewEventSource = func(context.Context, string, string, string, eventStreamTicketOptions) (*source.DingtalkSource, error) {
		return nil, fail
	}
	appCmd := makeBus("")
	_ = appCmd.Flags().Set("client-id", "override")
	if err := appCmd.RunE(appCmd, nil); !errors.Is(err, fail) {
		t.Fatalf("app event source error = %v", err)
	}
	eventNewEventSource = func(context.Context, string, string, string, eventStreamTicketOptions) (*source.DingtalkSource, error) {
		return &source.DingtalkSource{}, nil
	}
	if err := appCmd.RunE(appCmd, nil); !errors.Is(err, fail) {
		t.Fatalf("app bus run error = %v", err)
	}
	eventMkdirAll = func(string, os.FileMode) error { return fail }
	if err := appCmd.RunE(appCmd, nil); !errors.Is(err, fail) {
		t.Fatalf("app bus run after log mkdir failure = %v", err)
	}
}

func TestCrossPlatformCoverageEventListStatusCollectAndStopCoverage(t *testing.T) {
	oldList, oldStatus, oldStopPersonal := eventRunPersonalList, eventRunPersonalStatus, eventRunPersonalStop
	oldEnum, oldFind, oldQuery, oldStop := eventEnumerateBuses, eventFindBus, eventQueryEntry, eventStopBus
	oldCreds := eventResolveAppCredentials
	oldNormalize := eventNormalizeAs
	t.Cleanup(func() {
		eventRunPersonalList, eventRunPersonalStatus, eventRunPersonalStop = oldList, oldStatus, oldStopPersonal
		eventEnumerateBuses, eventFindBus, eventQueryEntry, eventStopBus = oldEnum, oldFind, oldQuery, oldStop
		eventResolveAppCredentials = oldCreds
		eventNormalizeAs = oldNormalize
	})
	eventNormalizeAs = func(value string) (string, error) {
		if strings.EqualFold(strings.TrimSpace(value), "app") {
			return "app", nil
		}
		return normalizeEventAs(value)
	}
	fail := errors.New("failure")
	eventRunPersonalList = func(*cobra.Command, personalListOptions) error { return fail }
	list := newEventListCommand()
	list.SetOut(io.Discard)
	if err := list.RunE(list, nil); !errors.Is(err, fail) {
		t.Fatalf("personal list error = %v", err)
	}
	eventRunPersonalStatus = func(*cobra.Command, personalStatusOptions) error { return fail }
	status := newEventStatusCommand()
	status.SetOut(io.Discard)
	if err := status.RunE(status, nil); !errors.Is(err, fail) {
		t.Fatalf("personal status error = %v", err)
	}
	eventRunPersonalStop = func(*cobra.Command, personalStopOptions) error { return fail }
	stop := newEventStopCommand()
	stop.SetOut(io.Discard)
	stop.Flags().Bool("yes", false, "")
	_ = stop.Flags().Set("yes", "true")
	if err := stop.RunE(stop, []string{"sub"}); !errors.Is(err, fail) {
		t.Fatalf("personal stop error = %v", err)
	}

	eventEnumerateBuses = func(string, string) ([]busctl.BusEntry, error) { return nil, fail }
	if _, err := collectEntries(&cobra.Command{}, "client", false, true); !errors.Is(err, fail) {
		t.Fatalf("all-editions collect error = %v", err)
	}
	if _, err := collectEntries(&cobra.Command{}, "client", true, false); !errors.Is(err, fail) {
		t.Fatalf("all collect error = %v", err)
	}
	eventEnumerateBuses = func(string, string) ([]busctl.BusEntry, error) {
		return []busctl.BusEntry{{ClientIDHash: "hash"}}, nil
	}
	eventQueryEntry = func(entry busctl.BusEntry) busctl.EntryStatus { return busctl.EntryStatus{Entry: entry} }
	if got, err := collectEntries(&cobra.Command{}, "client", false, true); err != nil || len(got) != 1 {
		t.Fatalf("all-editions entries = %#v, %v", got, err)
	}
	if got, err := collectEntries(&cobra.Command{}, "client", true, false); err != nil || len(got) != 1 {
		t.Fatalf("all entries = %#v, %v", got, err)
	}
	eventResolveAppCredentials = func(string) (string, string, authpkg.CredentialSource, authpkg.CredentialSource, error) {
		return "", "", authpkg.CredentialSourceUnknown, authpkg.CredentialSourceUnknown, fail
	}
	if _, err := collectEntries(&cobra.Command{}, "", false, false); !errors.Is(err, fail) {
		t.Fatalf("single credentials error = %v", err)
	}
	eventResolveAppCredentials = func(string) (string, string, authpkg.CredentialSource, authpkg.CredentialSource, error) {
		return "client", "secret", authpkg.CredentialSourceEnv, authpkg.CredentialSourceEnv, nil
	}
	eventFindBus = func(string, string, string) *busctl.BusEntry { return nil }
	if got, err := collectEntries(&cobra.Command{}, "", false, false); err != nil || len(got) != 1 || got[0].Entry.State != busctl.BusStateNotRunning {
		t.Fatalf("missing bus entry = %#v, %v", got, err)
	}
	eventFindBus = func(string, string, string) *busctl.BusEntry { return &busctl.BusEntry{} }
	if got, err := collectEntries(&cobra.Command{}, "client", false, false); err != nil || len(got) != 1 || got[0].Entry.Meta == nil {
		t.Fatalf("found bus entry = %#v, %v", got, err)
	}

	appList := newEventListCommand()
	appList.SetOut(io.Discard)
	_ = appList.Flags().Set("as", "app")
	_ = appList.Flags().Set("client-id", "client")
	if err := appList.RunE(appList, nil); err != nil {
		t.Fatal(err)
	}
	rejectedList := newEventListCommand()
	_ = rejectedList.Flags().Set("as", "app")
	_ = rejectedList.Flags().Set("category", "chat")
	if err := rejectedList.RunE(rejectedList, nil); err == nil {
		t.Fatal("app list personal flag succeeded")
	}
	eventFindBus = func(string, string, string) *busctl.BusEntry {
		return &busctl.BusEntry{State: busctl.BusStateOrphan, Meta: &bus.Meta{ClientID: "client"}}
	}
	eventQueryEntry = func(entry busctl.BusEntry) busctl.EntryStatus { return busctl.EntryStatus{Entry: entry} }
	appStatus := newEventStatusCommand()
	appStatus.SetOut(io.Discard)
	_ = appStatus.Flags().Set("as", "app")
	_ = appStatus.Flags().Set("client-id", "client")
	_ = appStatus.Flags().Set("fail-on-orphan", "true")
	if err := appStatus.RunE(appStatus, nil); err == nil {
		t.Fatal("orphan event status succeeded")
	}
	rejectedStatus := newEventStatusCommand()
	_ = rejectedStatus.Flags().Set("as", "app")
	_ = rejectedStatus.Flags().Set("event", "key")
	if err := rejectedStatus.RunE(rejectedStatus, nil); err == nil {
		t.Fatal("app status personal flag succeeded")
	}

	appStop := func() *cobra.Command {
		cmd := newEventStopCommand()
		cmd.SetOut(io.Discard)
		cmd.Flags().Bool("yes", true, "")
		_ = cmd.Flags().Set("as", "app")
		return cmd
	}
	changedStop := appStop()
	_ = changedStop.Flags().Set("all", "true")
	if err := changedStop.RunE(changedStop, nil); err == nil {
		t.Fatal("app stop personal flag succeeded")
	}
	if err := appStop().RunE(appStop(), []string{"sub"}); err == nil {
		t.Fatal("app subscribe ID succeeded")
	}
	eventResolveAppCredentials = func(string) (string, string, authpkg.CredentialSource, authpkg.CredentialSource, error) {
		return "", "", authpkg.CredentialSourceUnknown, authpkg.CredentialSourceUnknown, fail
	}
	if err := appStop().RunE(appStop(), nil); !errors.Is(err, fail) {
		t.Fatalf("app stop credentials error = %v", err)
	}
	eventResolveAppCredentials = func(string) (string, string, authpkg.CredentialSource, authpkg.CredentialSource, error) {
		return "client", "secret", authpkg.CredentialSourceEnv, authpkg.CredentialSourceEnv, nil
	}
	eventStopBus = func(busctl.StopConfig) error { return busctl.ErrNotRunning }
	if err := appStop().RunE(appStop(), nil); err != nil {
		t.Fatalf("already stopped bus = %v", err)
	}
	eventStopBus = func(busctl.StopConfig) error { return fail }
	if err := appStop().RunE(appStop(), nil); !errors.Is(err, fail) {
		t.Fatalf("stop bus error = %v", err)
	}
	eventStopBus = func(busctl.StopConfig) error { return nil }
	if err := appStop().RunE(appStop(), nil); err != nil {
		t.Fatalf("stop bus success = %v", err)
	}
}

func TestCrossPlatformCoverageEventCommandParentCoverage(t *testing.T) {
	cmd := newEventCommand()
	cmd.SetOut(io.Discard)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(cmd.Use, "event") {
		t.Fatal("event command use changed")
	}
}

func TestCrossPlatformCoverageEventCommandPureAndRenderBranchesCoverage(t *testing.T) {
	t.Setenv(authpkg.EnvClientID, "client")
	t.Setenv(authpkg.EnvClientSecret, "secret")
	if got := (eventStreamTicketOptions{Mode: "custom", SourceID: " source ", TicketURL: " https://ticket "}).spawnArgs(); len(got) != 6 {
		t.Fatalf("stream spawn args = %#v", got)
	}
	if id, secret, err := resolveEventCredentials(t.TempDir(), eventStreamTicketOptions{}); err != nil || id != "client" || secret != "secret" {
		t.Fatalf("app credentials = %q, %q, %v", id, secret, err)
	}
	if id, secret, err := resolveEventCredentials(t.TempDir(), eventStreamTicketOptions{Mode: "normal", SourceID: "source"}); err != nil || id != "portal-ticket-normal:source" || secret != "" {
		t.Fatalf("portal credentials = %q, %q, %v", id, secret, err)
	}
	if eventStreamTicketURL(" https://ticket ") != "https://ticket" || eventStreamSourceID(" source ") != "source" {
		t.Fatal("explicit event stream routing changed")
	}
	t.Setenv("DWS_STREAM_SOURCE_ID", "environment")
	if defaultEventStreamSourceID() != "environment" {
		t.Fatal("environment stream source ID ignored")
	}

	oldEdition := edition.Get()
	edition.Override(&edition.Hooks{})
	t.Cleanup(func() { edition.Override(oldEdition) })
	if editionNameOrDefault() != "open" || sourceKindLabel("") != string(dwsevent.SourceKindAppStream) {
		t.Fatal("default event labels changed")
	}
	if !strings.Contains(eventWorkDir("config", "open", "", "hash"), string(dwsevent.SourceKindAppStream)) {
		t.Fatal("default event workdir changed")
	}
	if _, err := normalizeEventAs("bot"); err == nil {
		t.Fatal("bot events became public unexpectedly")
	}
	flags := &cobra.Command{}
	flags.Flags().Bool("all", false, "")
	if err := rejectPersonalEventUnsupportedFlags(flags, "all"); err != nil {
		t.Fatal(err)
	}
	_ = flags.Flags().Set("all", "true")
	if err := rejectPersonalEventUnsupportedFlags(flags, "all"); err == nil {
		t.Fatal("personal unsupported flag accepted")
	}
	if firstArg(nil) != "" || firstArg([]string{"value"}) != "value" || len(eventTypesWithDefault([]string{"type"})) != 1 {
		t.Fatal("event helper defaults changed")
	}
	_ = eventTypesWithDefault(nil)

	live := &eventtransport.StatusResp{
		Bus:         eventtransport.StatusBus{UptimeSecs: 12},
		SourceState: eventtransport.StatusSource{State: "connected", Source: "hook", ReconnectCount: 1},
		Consumers: []eventtransport.StatusConsumer{
			{PID: 1, Received: 2, Dropped: 3},
			{PID: 2, EventTypes: []string{"chat"}, SubscribeID: "sub"},
		},
		PerEventTypeCounters: map[string]eventtransport.Counters{"chat": {Received: 2, Dropped: 1}},
	}
	entries := []busctl.EntryStatus{
		{Entry: busctl.BusEntry{State: busctl.BusStateNotRunning, ClientIDHash: "not-running"}},
		{Entry: busctl.BusEntry{State: busctl.BusStateOrphan, HolderPID: 3, Meta: &bus.Meta{ClientID: "orphan", SourceID: "source", StartedAt: time.Now()}}},
		{Entry: busctl.BusEntry{State: busctl.BusStateRunning, HolderPID: 4, Meta: &bus.Meta{ClientID: "offline", StartedAt: time.Now().Add(-time.Minute)}}},
		{Entry: busctl.BusEntry{State: busctl.BusStateRunning, HolderPID: 5, Meta: &bus.Meta{ClientID: "live"}}, Live: live},
	}
	if err := renderStatus(io.Discard, entries, "text"); err != nil {
		t.Fatal(err)
	}
	if err := renderStatus(io.Discard, entries, "json"); err != nil {
		t.Fatal(err)
	}
	if err := renderStatus(appFailWriter{err: errors.New("write")}, entries, "json"); err == nil {
		t.Fatal("status JSON write failure succeeded")
	}
	listEntries := []listEntry{
		{ClientIDHash: "hash", BusState: busctl.BusStateNotRunning},
		{ClientID: "client", SourceKind: dwsevent.SourceKindPersonalStream, BusState: busctl.BusStateRunning, BusPID: 2, Consumers: live.Consumers},
	}
	if err := renderList(io.Discard, listEntries, "table"); err != nil {
		t.Fatal(err)
	}
	if err := renderList(io.Discard, listEntries, "json"); err != nil {
		t.Fatal(err)
	}
	if err := renderList(appFailWriter{err: errors.New("write")}, listEntries, "json"); err == nil {
		t.Fatal("list JSON write failure succeeded")
	}
	if got := buildListEntry(busctl.EntryStatus{Entry: busctl.BusEntry{Meta: &bus.Meta{ClientID: "client"}}, Live: live}); len(got.Consumers) != 2 {
		t.Fatalf("live list entry = %#v", got)
	}
}

func TestCrossPlatformCoverageEventCommandClosureErrorBranchesCoverage(t *testing.T) {
	oldNormalize := eventNormalizeAs
	oldList, oldStatus := eventRunPersonalList, eventRunPersonalStatus
	oldCreds, oldFind, oldQuery := eventResolveAppCredentials, eventFindBus, eventQueryEntry
	t.Cleanup(func() {
		eventNormalizeAs = oldNormalize
		eventRunPersonalList, eventRunPersonalStatus = oldList, oldStatus
		eventResolveAppCredentials, eventFindBus, eventQueryEntry = oldCreds, oldFind, oldQuery
	})
	eventNormalizeAs = func(value string) (string, error) {
		if strings.TrimSpace(value) == "app" {
			return "app", nil
		}
		return normalizeEventAs(value)
	}
	personalList := newEventListCommand()
	_ = personalList.Flags().Set("all", "true")
	if err := personalList.RunE(personalList, nil); err == nil {
		t.Fatal("personal list app flag succeeded")
	}
	personalStatus := newEventStatusCommand()
	_ = personalStatus.Flags().Set("fail-on-orphan", "true")
	if err := personalStatus.RunE(personalStatus, nil); err == nil {
		t.Fatal("personal status app flag succeeded")
	}
	fail := errors.New("failure")
	eventResolveAppCredentials = func(string) (string, string, authpkg.CredentialSource, authpkg.CredentialSource, error) {
		return "", "", authpkg.CredentialSourceUnknown, authpkg.CredentialSourceUnknown, fail
	}
	appList := newEventListCommand()
	_ = appList.Flags().Set("as", "app")
	if err := appList.RunE(appList, nil); !errors.Is(err, fail) {
		t.Fatalf("app list collect error = %v", err)
	}
	appStatus := newEventStatusCommand()
	_ = appStatus.Flags().Set("as", "app")
	if err := appStatus.RunE(appStatus, nil); !errors.Is(err, fail) {
		t.Fatalf("app status collect error = %v", err)
	}
	eventResolveAppCredentials = func(string) (string, string, authpkg.CredentialSource, authpkg.CredentialSource, error) {
		return "client", "secret", authpkg.CredentialSourceEnv, authpkg.CredentialSourceEnv, nil
	}
	eventFindBus = func(string, string, string) *busctl.BusEntry {
		return &busctl.BusEntry{State: busctl.BusStateRunning, Meta: &bus.Meta{ClientID: "client"}}
	}
	eventQueryEntry = func(entry busctl.BusEntry) busctl.EntryStatus { return busctl.EntryStatus{Entry: entry} }
	appStatus = newEventStatusCommand()
	appStatus.SetOut(io.Discard)
	_ = appStatus.Flags().Set("as", "app")
	_ = appStatus.Flags().Set("fail-on-orphan", "true")
	if err := appStatus.RunE(appStatus, nil); err != nil {
		t.Fatalf("healthy status with orphan gate = %v", err)
	}
	appStatus = newEventStatusCommand()
	appStatus.SetOut(appFailWriter{err: errors.New("write")})
	_ = appStatus.Flags().Set("as", "app")
	_ = appStatus.Flags().Set("format", "json")
	if err := appStatus.RunE(appStatus, nil); err == nil {
		t.Fatal("status render failure should propagate")
	}
}
