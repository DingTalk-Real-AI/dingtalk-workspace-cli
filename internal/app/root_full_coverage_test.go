package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/plugin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/recovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageRootExecuteAllBranchesCoverage(t *testing.T) {
	oldNormalize := rootNormalizeProcessProfileArgs
	oldExecute := rootExecuteCommand
	oldNewRoot := rootNewRootCommandWithEngine
	oldPreParse := rootRunPreParse
	oldLatest := rootLatestRecoveryCapture
	oldReset := rootResetRecoveryState
	oldStop := rootStopAllStdioClients
	oldArgs := os.Args
	t.Cleanup(func() {
		rootNormalizeProcessProfileArgs = oldNormalize
		rootExecuteCommand = oldExecute
		rootNewRootCommandWithEngine = oldNewRoot
		rootRunPreParse = oldPreParse
		rootLatestRecoveryCapture = oldLatest
		rootResetRecoveryState = oldReset
		rootStopAllStdioClients = oldStop
		os.Args = oldArgs
	})
	os.Args = []string{"dws"}
	rootNormalizeProcessProfileArgs = func() func() { return func() {} }
	rootRunPreParse = func(*cobra.Command, *pipeline.Engine) {}
	rootResetRecoveryState = func() {}
	rootStopAllStdioClients = func() {}
	rootNewRootCommandWithEngine = func(context.Context, *pipeline.Engine) *cobra.Command {
		return &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	}
	rootLatestRecoveryCapture = func() *recovery.LastError { return nil }
	rootExecuteCommand = func(cmd *cobra.Command) (*cobra.Command, error) { return cmd, nil }
	if code := Execute(); code != 0 {
		t.Fatalf("successful Execute code = %d", code)
	}

	wantErr := errors.New("unknown command missing")
	rootLatestRecoveryCapture = func() *recovery.LastError { return &recovery.LastError{EventID: "evt-test"} }
	rootExecuteCommand = func(*cobra.Command) (*cobra.Command, error) { return nil, wantErr }
	if code := Execute(); code == 0 {
		t.Fatal("failed Execute returned zero")
	}

	rootExecuteCommand = func(*cobra.Command) (*cobra.Command, error) { panic("boom") }
	if code := Execute(); code != 5 {
		t.Fatalf("panic Execute code = %d", code)
	}
}

func TestCrossPlatformCoverageRootConstructionHooksAndVersionCoverage(t *testing.T) {
	oldLoadPlugins := rootLoadPlugins
	oldEdition := edition.Get()
	oldVersion, oldBuild, oldCommit := version, buildTime, gitCommit
	t.Cleanup(func() {
		rootLoadPlugins = oldLoadPlugins
		edition.Override(oldEdition)
		version, buildTime, gitCommit = oldVersion, oldBuild, oldCommit
	})

	rootLoadPlugins = func(*cobra.Command, *pipeline.Engine, executor.Runner) []*cobra.Command {
		return []*cobra.Command{{Use: "plugin-added", Run: func(*cobra.Command, []string) {}}}
	}
	preRunCalled := false
	edition.Override(&edition.Hooks{
		AfterPersistentPreRun: func(*cobra.Command, []string) error { preRunCalled = true; return nil },
		RegisterExtraCommands: func(root *cobra.Command, _ edition.ToolCaller) {
			root.AddCommand(&cobra.Command{Use: "extra", Run: func(*cobra.Command, []string) {}})
		},
	})
	root := NewRootCommandWithEngine(context.Background(), pipeline.NewEngine())
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"version", "--client-id", "client", "--client-secret", "secret", "--debug"})
	if err := root.Execute(); err != nil || !preRunCalled {
		t.Fatalf("root version execution = %v preRun=%v", err, preRunCalled)
	}

	root = NewRootCommandWithEngine(context.Background(), nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(nil)
	if err := root.Execute(); err != nil {
		t.Fatalf("root help execution = %v", err)
	}

	standalone := newVersionCommand()
	standalone.Flags().String("format", "", "")
	standalone.SetOut(io.Discard)
	standalone.SetArgs([]string{"--format", "json"})
	version, buildTime, gitCommit = "1.2.3", "today", "commit"
	edition.Override(&edition.Hooks{})
	if err := standalone.Execute(); err != nil {
		t.Fatalf("JSON version = %v", err)
	}

	root = NewRootCommandWithEngine(context.Background(), nil)
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs([]string{"version", "--output", "bad\x00path"})
	if err := root.Execute(); err == nil {
		t.Fatal("unsafe output path succeeded")
	}
}

func TestCrossPlatformCoverageRootFlagsPluginsAndOutputRemainingCoverage(t *testing.T) {
	t.Chdir(t.TempDir())
	parent := &cobra.Command{Use: "root"}
	parent.PersistentFlags().String("format", "json", "")
	child := &cobra.Command{Use: "child"}
	parent.AddCommand(child)
	if !wantsJSONErrors(child) {
		t.Fatal("root JSON format was not inherited")
	}
	localRoot := &cobra.Command{Use: "root"}
	localRoot.Flags().String("format", "json", "")
	localChild := &cobra.Command{Use: "child"}
	localRoot.AddCommand(localChild)
	if !wantsJSONErrors(localChild) {
		t.Fatal("root-local JSON format was not recognized")
	}
	falseJSON := &cobra.Command{Use: "false-json"}
	falseJSON.Flags().Bool("json", true, "")
	_ = falseJSON.Flags().Set("json", "false")
	if commandRequestsJSONErrors(falseJSON) {
		t.Fatal("explicit false JSON flag requested JSON")
	}
	brokenJSON := &cobra.Command{Use: "broken"}
	brokenJSON.Flags().String("json", "not-bool", "")
	_ = brokenJSON.Flags().Set("json", "value")
	if !commandRequestsJSONErrors(brokenJSON) {
		t.Fatal("changed non-bool json flag was not treated as JSON")
	}

	pluginRoot := &cobra.Command{Use: "root"}
	pluginRoot.AddCommand(&cobra.Command{Use: "market"})
	addPluginCommandsSafe(pluginRoot, []*cobra.Command{
		{Use: "auth"},
		{Use: "duplicate"},
		{Use: "duplicate"},
		{Use: "market"},
	})

	oldMkdir := rootMkdirAll
	oldCreate := rootCreateFile
	oldClose := rootCloseFile
	t.Cleanup(func() {
		rootMkdirAll = oldMkdir
		rootCreateFile = oldCreate
		rootCloseFile = oldClose
	})
	wantErr := errors.New("filesystem")
	newOutputCommand := func(path string) *cobra.Command {
		root := &cobra.Command{Use: "root"}
		root.PersistentFlags().String("output", path, "")
		cmd := &cobra.Command{Use: "output"}
		root.AddCommand(cmd)
		cmd.SetContext(context.Background())
		return cmd
	}
	successPath := filepath.Join("success", "out")
	successCmd := newOutputCommand(successPath)
	if err := configureOutputSink(successCmd); err != nil {
		t.Fatalf("output sink success = %v", err)
	}
	if err := closeOutputSink(successCmd); err != nil {
		t.Fatalf("output sink close = %v", err)
	}
	badTypeRoot := &cobra.Command{Use: "root"}
	badTypeRoot.PersistentFlags().Bool("output", false, "")
	badTypeChild := &cobra.Command{Use: "child"}
	badTypeRoot.AddCommand(badTypeChild)
	if err := configureOutputSink(badTypeChild); err == nil {
		t.Fatal("non-string output flag succeeded")
	}
	rootMkdirAll = func(string, os.FileMode) error { return wantErr }
	if err := configureOutputSink(newOutputCommand(filepath.Join("mkdir-failure", "out"))); err == nil {
		t.Fatal("mkdir failure succeeded")
	}
	rootMkdirAll = func(string, os.FileMode) error { return nil }
	rootCreateFile = func(string) (*os.File, error) { return nil, wantErr }
	if err := configureOutputSink(newOutputCommand(filepath.Join("create-failure", "out"))); err == nil {
		t.Fatal("create failure succeeded")
	}
	rootCreateFile = oldCreate
	file, err := os.CreateTemp(t.TempDir(), "close")
	if err != nil {
		t.Fatal(err)
	}
	cmd := &cobra.Command{Use: "close"}
	cmd.SetContext(context.WithValue(context.Background(), outputFileContextKey{}, file))
	rootCloseFile = func(*os.File) error { return wantErr }
	if err := closeOutputSink(cmd); err == nil {
		t.Fatal("close failure succeeded")
	}
	if err := file.Close(); err != nil {
		t.Fatalf("cleanup close-failure file = %v", err)
	}
	rootCloseFile = oldClose
	file, err = os.CreateTemp(t.TempDir(), "close-success")
	if err != nil {
		t.Fatal(err)
	}
	cmd.SetContext(context.WithValue(context.Background(), outputFileContextKey{}, file))
	if err := closeOutputSink(cmd); err != nil {
		t.Fatalf("close success = %v", err)
	}
	_ = file.Close()

	for _, flags := range []*GlobalFlags{nil, {Debug: true}, {Verbose: true}, {}} {
		configureLogLevel(flags)
	}
}

func TestCrossPlatformCoverageRootLoadPluginsRemainingCoverage(t *testing.T) {
	oldInject := rootPluginInjectConfigEnv
	oldUser := rootPluginLoadUser
	oldDev := rootPluginLoadDev
	oldDescriptors := rootPluginDescriptors
	oldStdioClients := rootPluginStdioClients
	oldHTTP := rootRegisterPluginHTTPServer
	oldStdioDescriptor := rootPluginStdioDescriptor
	oldStdioRegister := rootRegisterResolvedStdioServer
	oldHooks := rootPluginLoadHooks
	oldSync := rootPluginSyncSkills
	oldToken := rootAuthLoadTokenData
	t.Cleanup(func() {
		rootPluginInjectConfigEnv = oldInject
		rootPluginLoadUser = oldUser
		rootPluginLoadDev = oldDev
		rootPluginDescriptors = oldDescriptors
		rootPluginStdioClients = oldStdioClients
		rootRegisterPluginHTTPServer = oldHTTP
		rootPluginStdioDescriptor = oldStdioDescriptor
		rootRegisterResolvedStdioServer = oldStdioRegister
		rootPluginLoadHooks = oldHooks
		rootPluginSyncSkills = oldSync
		rootAuthLoadTokenData = oldToken
	})

	p1 := &plugin.Plugin{Manifest: plugin.Manifest{Name: "one"}}
	p2 := &plugin.Plugin{Manifest: plugin.Manifest{Name: "two"}}
	p3 := &plugin.Plugin{Manifest: plugin.Manifest{Name: "three"}}
	rootPluginInjectConfigEnv = func(*plugin.Loader) {}
	rootPluginLoadUser = func(*plugin.Loader) []*plugin.Plugin { return []*plugin.Plugin{p1, p2} }
	rootPluginLoadDev = func(*plugin.Loader) []*plugin.Plugin { return []*plugin.Plugin{p3} }
	rootAuthLoadTokenData = func(string) (*authpkg.TokenData, error) {
		return &authpkg.TokenData{UserID: "user", CorpID: "corp"}, nil
	}
	rootPluginDescriptors = func(p *plugin.Plugin) []mcptypes.ServerDescriptor {
		if p == p1 {
			return []mcptypes.ServerDescriptor{{
				Key: "http", Endpoint: "https://example.test",
				CLI: mcptypes.CLIOverlay{
					ID: "http", Command: "one-http",
					ToolOverrides: map[string]mcptypes.CLIToolOverride{
						"ping": {CLIName: "ping"},
					},
				},
			}}
		}
		return []mcptypes.ServerDescriptor{{Key: p.Manifest.Name + "-no-cli", Endpoint: "https://example.test"}}
	}
	client := transport.NewStdioClient("ignored", nil, nil)
	rootPluginStdioClients = func(p *plugin.Plugin, uc *plugin.UserContext) []plugin.StdioServerClient {
		if p == p1 && uc != nil && uc.UserID == "user" {
			return []plugin.StdioServerClient{{Key: "local", Client: client}}
		}
		return nil
	}
	httpCount := 0
	stdioCount := 0
	rootRegisterPluginHTTPServer = func(mcptypes.ServerDescriptor) { httpCount++ }
	rootPluginStdioDescriptor = func(*plugin.Plugin, plugin.StdioServerClient) (mcptypes.ServerDescriptor, bool) {
		return mcptypes.ServerDescriptor{
			Key: "local",
			CLI: mcptypes.CLIOverlay{
				ID: "local", Command: "one-stdio",
				ToolOverrides: map[string]mcptypes.CLIToolOverride{
					"pong": {CLIName: "pong"},
				},
			},
		}, true
	}
	rootRegisterResolvedStdioServer = func(
		*plugin.Plugin,
		plugin.StdioServerClient,
		mcptypes.ServerDescriptor,
	) {
		stdioCount++
	}
	rootPluginLoadHooks = func(p *plugin.Plugin) (*plugin.HooksConfig, error) {
		switch p {
		case p1:
			return nil, errors.New("hooks")
		case p2:
			return nil, nil
		default:
			return &plugin.HooksConfig{Hooks: []plugin.HookEntry{{Phase: "pre-request", Command: "true"}}}, nil
		}
	}
	synced := false
	rootPluginSyncSkills = func([]*plugin.Plugin) { synced = true }
	got := loadPlugins(nil, pipeline.NewEngine(), runnerCoverageFallback{})
	if len(got) != 2 || got[0].Name() != "one-http" || got[1].Name() != "one-stdio" {
		t.Fatalf("loaded plugin commands = %#v", got)
	}
	if httpCount != 3 || stdioCount != 1 || !synced {
		t.Fatalf("registered http=%d stdio=%d synced=%v", httpCount, stdioCount, synced)
	}
}
