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
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/plugin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/schemacache"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageSchemaCacheBuilderChildProcess(t *testing.T) {
	expected, err := schemaCacheBuilderAssemble(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	oldCommand := schemaCacheBuilderCommand
	oldRead := schemaCacheReadResult
	t.Cleanup(func() {
		schemaCacheBuilderCommand = oldCommand
		schemaCacheReadResult = oldRead
	})
	schemaCacheBuilderCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=TestCrossPlatformCoverageSchemaCacheBuilderProtocolHelper")
	}
	schemaCacheReadResult = func(io.Reader) (cli.SchemaCacheBuildResult, error) {
		return expected, nil
	}
	result, err := buildSchemaCacheInChild(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.Identity.BuildID != expected.Identity.BuildID {
		t.Fatalf("child result identity = %x, want %x", result.Identity.BuildID, expected.Identity.BuildID)
	}
}

func TestCrossPlatformCoverageSchemaCacheBuilderChildHelper(t *testing.T) {
	if !strings.Contains(strings.Join(os.Args, " "), "-test.run=TestCrossPlatformCoverageSchemaCacheBuilderChildHelper") {
		return
	}
	_, code := RunSchemaCacheBuilder([]string{schemaCacheBuilderArgument}, os.Stdout)
	os.Exit(code)
}

func TestCrossPlatformCoverageSchemaCacheBuilderExitHelper(t *testing.T) {
	if strings.Contains(strings.Join(os.Args, " "), "-test.run=TestCrossPlatformCoverageSchemaCacheBuilderExitHelper") {
		os.Exit(7)
	}
}

func TestCrossPlatformCoverageSchemaCacheBuilderOutputHelper(t *testing.T) {
	if strings.Contains(strings.Join(os.Args, " "), "-test.run=TestCrossPlatformCoverageSchemaCacheBuilderOutputHelper") {
		_, _ = os.Stdout.Write([]byte("oversized"))
		os.Exit(0)
	}
}

func TestCrossPlatformCoverageSchemaCacheBuilderProtocolHelper(t *testing.T) {
	if strings.Contains(strings.Join(os.Args, " "), "-test.run=TestCrossPlatformCoverageSchemaCacheBuilderProtocolHelper") {
		_, _ = os.Stdout.Write([]byte("ignored"))
		os.Exit(0)
	}
}

func TestCrossPlatformCoverageSchemaCacheBuilderStderrHelper(t *testing.T) {
	if strings.Contains(strings.Join(os.Args, " "), "-test.run=TestCrossPlatformCoverageSchemaCacheBuilderStderrHelper") {
		_, _ = os.Stderr.Write([]byte("noisy child"))
		os.Exit(7)
	}
}

func TestCrossPlatformCoverageSchemaCacheBuilderChildSetupErrors(t *testing.T) {
	oldExecutable := schemaCacheBuilderExecutable
	oldEnvironment := schemaCacheBuilderEnvironment
	oldWorkingDir := schemaCacheBuilderWorkingDir
	oldCommand := schemaCacheBuilderCommand
	oldRead := schemaCacheReadResult
	t.Cleanup(func() {
		schemaCacheBuilderExecutable = oldExecutable
		schemaCacheBuilderEnvironment = oldEnvironment
		schemaCacheBuilderWorkingDir = oldWorkingDir
		schemaCacheBuilderCommand = oldCommand
		schemaCacheReadResult = oldRead
	})
	schemaCacheBuilderExecutable = func() (string, error) { return "", errors.New("executable unavailable") }
	if _, err := buildSchemaCacheInChild(context.Background()); err == nil {
		t.Fatal("missing executable unexpectedly succeeded")
	}
	schemaCacheBuilderExecutable = oldExecutable
	schemaCacheBuilderEnvironment = func() []string { return nil }
	if _, err := buildSchemaCacheInChild(context.Background()); err == nil {
		t.Fatal("empty environment unexpectedly succeeded")
	}
	schemaCacheBuilderEnvironment = oldEnvironment
	schemaCacheBuilderWorkingDir = func() string { return "" }
	schemaCacheReadResult = func(io.Reader) (cli.SchemaCacheBuildResult, error) {
		return cli.SchemaCacheBuildResult{}, nil
	}
	schemaCacheBuilderCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=TestCrossPlatformCoverageSchemaCacheBuilderOutputHelper")
	}
	if _, err := buildSchemaCacheInChild(context.Background()); err != nil {
		t.Fatalf("child with empty working directory failed: %v", err)
	}
	schemaCacheBuilderWorkingDir = oldWorkingDir
	schemaCacheBuilderCommand = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
		return exec.CommandContext(ctx, os.Args[0], "-test.run=TestCrossPlatformCoverageSchemaCacheBuilderExitHelper")
	}
	if _, err := buildSchemaCacheInChild(context.Background()); err == nil {
		t.Fatal("child failure unexpectedly succeeded")
	}
}

func TestCrossPlatformCoverageSchemaCacheBuilderPrivateProtocol(t *testing.T) {
	var output bytes.Buffer
	handled, code := RunSchemaCacheBuilder([]string{schemaCacheBuilderArgument}, &output)
	if !handled || code != 0 {
		t.Fatalf("builder handled=%v code=%d", handled, code)
	}
	result, err := cli.ReadSchemaCacheBuildResult(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	if result.Identity.Edition != "open" || len(result.Artifacts.Registry) == 0 {
		t.Fatalf("invalid builder result: edition=%q registry=%d", result.Identity.Edition, len(result.Artifacts.Registry))
	}
}

func TestCrossPlatformCoverageSchemaCacheBuilderDispatchErrors(t *testing.T) {
	if handled, code := RunSchemaCacheBuilder(nil, &bytes.Buffer{}); handled || code != 0 {
		t.Fatalf("non-builder dispatch = handled:%v code:%d", handled, code)
	}
	oldAssemble := schemaCacheBuilderAssemble
	oldResolve := schemaCacheResolve
	oldArtifacts := schemaCacheBuildArtifacts
	oldIdentity := schemaCacheIdentity
	t.Cleanup(func() {
		schemaCacheBuilderAssemble = oldAssemble
		schemaCacheResolve = oldResolve
		schemaCacheBuildArtifacts = oldArtifacts
		schemaCacheIdentity = oldIdentity
	})
	schemaCacheBuilderAssemble = func(context.Context) (cli.SchemaCacheBuildResult, error) {
		return cli.SchemaCacheBuildResult{}, errors.New("assembly failed")
	}
	if handled, code := RunSchemaCacheBuilder([]string{schemaCacheBuilderArgument}, &bytes.Buffer{}); !handled || code != 1 {
		t.Fatalf("assembly failure dispatch = handled:%v code:%d", handled, code)
	}
	schemaCacheResolve = func(*cobra.Command) (cli.ResolvedSchemaBuild, error) {
		return cli.ResolvedSchemaBuild{}, errors.New("resolve failed")
	}
	if _, err := buildSchemaCacheResult(context.Background()); err == nil {
		t.Fatal("resolve failure unexpectedly succeeded")
	}
	schemaCacheResolve = oldResolve
	schemaCacheBuildArtifacts = func(cli.ResolvedSchemaBuild) (cli.SchemaCacheArtifacts, error) {
		return cli.SchemaCacheArtifacts{}, errors.New("artifact build failed")
	}
	if _, err := buildSchemaCacheResult(context.Background()); err == nil {
		t.Fatal("artifact failure unexpectedly succeeded")
	}
	schemaCacheBuildArtifacts = oldArtifacts
	schemaCacheIdentity = func(string, cli.SchemaCacheArtifacts) (cli.SchemaCacheIdentity, error) {
		return cli.SchemaCacheIdentity{}, errors.New("identity failed")
	}
	if _, err := buildSchemaCacheResult(context.Background()); err == nil {
		t.Fatal("identity failure unexpectedly succeeded")
	}
	schemaCacheIdentity = oldIdentity
	schemaCacheBuilderAssemble = func(context.Context) (cli.SchemaCacheBuildResult, error) {
		return cli.SchemaCacheBuildResult{}, nil
	}
	if handled, code := RunSchemaCacheBuilder([]string{schemaCacheBuilderArgument}, schemaFailWriter{}); !handled || code != 1 {
		t.Fatalf("writer failure dispatch = handled:%v code:%d", handled, code)
	}
}

type schemaFailWriter struct{}

func (schemaFailWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestCrossPlatformCoverageSchemaCacheBuilderProcessOutput(t *testing.T) {
	if err := validateSchemaCacheBuilderProcessOutput(&cappedBuffer{}, &cappedBuffer{}, nil); err != nil {
		t.Fatal(err)
	}
	truncated := &cappedBuffer{truncated: true}
	if err := validateSchemaCacheBuilderProcessOutput(truncated, &cappedBuffer{}, nil); err == nil {
		t.Fatal("truncated stdout unexpectedly accepted")
	}
	if err := validateSchemaCacheBuilderProcessOutput(&cappedBuffer{}, &cappedBuffer{}, errors.New("child failed")); err == nil {
		t.Fatal("child failure unexpectedly accepted")
	}
	stderr := &cappedBuffer{truncated: true}
	_, _ = stderr.Write([]byte("diagnostic"))
	if err := validateSchemaCacheBuilderProcessOutput(&cappedBuffer{}, stderr, errors.New("child failed")); err == nil {
		t.Fatal("truncated stderr failure unexpectedly accepted")
	}
}

func TestCrossPlatformCoverageSchemaCacheBuilderOutputLimits(t *testing.T) {
	var buffer cappedBuffer
	buffer.limit = 2
	if n, err := buffer.Write([]byte("abcd")); err != nil || n != 4 || buffer.String() != "ab" || !buffer.truncated {
		t.Fatalf("capped buffer = n:%d err:%v value:%q truncated:%v", n, err, buffer.String(), buffer.truncated)
	}
	buffer = cappedBuffer{limit: 8}
	if n, err := buffer.Write([]byte("abcd")); err != nil || n != 4 || buffer.String() != "abcd" || buffer.truncated {
		t.Fatalf("uncapped buffer = n:%d err:%v value:%q truncated:%v", n, err, buffer.String(), buffer.truncated)
	}
}

func TestCrossPlatformCoverageSchemaCacheBuilderError(t *testing.T) {
	if code := writeSchemaCacheBuilderError(&bytes.Buffer{}, errors.New("builder failed")); code != 1 {
		t.Fatalf("error exit code = %d, want 1", code)
	}
}

// A process with runtime plugins mounted must still publish and reuse the
// persisted Schema cache: plugin commands live only in the runtime command
// tree, never in the declaration-only schema source root, so the cache cannot
// misrepresent the builtin schema surface. Regression guard for the isolated
// builder path.
func TestCrossPlatformCoverageSchemaCachePublishesWithRuntimePlugins(t *testing.T) {
	if !schemacache.PersistentBackendEnabled(runtime.GOOS, runtime.GOARCH) {
		t.Skip("persistent cache backend is intentionally disabled on this target")
	}
	isolateSchemaCacheHome(t)
	t.Setenv(schemaCacheTestEnv, "1")
	cli.RegisterSchemaCacheIsolatedBuilder(func(ctx context.Context) (cli.SchemaCacheBuildResult, error) {
		resolved, err := cli.ResolveSchemaBuild(NewSchemaSourceRootCommand(ctx))
		if err != nil {
			return cli.SchemaCacheBuildResult{}, err
		}
		artifacts, err := cli.BuildSchemaCacheArtifacts(resolved)
		if err != nil {
			return cli.SchemaCacheBuildResult{}, err
		}
		identity, err := cli.IdentityFromArtifacts("open", artifacts)
		if err != nil {
			return cli.SchemaCacheBuildResult{}, err
		}
		return cli.SchemaCacheBuildResult{Artifacts: artifacts, Identity: identity}, nil
	})
	t.Cleanup(func() {
		cli.RegisterSchemaCacheIsolatedBuilder(buildSchemaCacheInChild)
		cli.ResetSchemaCacheRuntimeUncertaintyForTest()
		_ = cli.RegisterSchemaCacheOptions(cli.SchemaCacheOptions{})
	})

	testseam.Swap(t, &rootLoadPlugins, func(*cobra.Command, *pipeline.Engine, executor.Runner, string) []*cobra.Command {
		return []*cobra.Command{{
			Use:   "sample-plugin",
			Short: "plugin command mounted at runtime",
			RunE:  func(*cobra.Command, []string) error { return nil },
		}}
	})

	// Sibling tests may leave an assembled live catalog registered in-process;
	// ResolveMeta serves that catalog without touching the cache. Reset and
	// re-register so this test exercises the plugin-present publish path.
	cli.RestorePackageCLISchemaDeliveryForTest()
	cli.RegisterSchemaSourceRoot(func() *cobra.Command {
		return NewSchemaSourceRootCommand()
	})
	applyProductionSchemaCache()

	root := NewRootCommand()
	if root == nil {
		t.Fatal("NewRootCommand returned nil")
	}
	if cmd, _, err := root.Find([]string{"sample-plugin"}); err != nil || cmd == nil || cmd.Name() != "sample-plugin" {
		t.Fatalf("plugin command not mounted: cmd=%v err=%v", cmd, err)
	}

	// First schema use in the plugin-present process must generate AND publish
	// the cache instead of silently staying live-only.
	meta, ok := cli.ResolveMeta("calendar event create")
	if !ok || meta.Identity.Canonical != "calendar.create_calendar_event" {
		t.Fatalf("plugin-present ResolveMeta = %#v, %v", meta, ok)
	}
	identity, ok := cli.SchemaCacheReadableIdentityForTest()
	if !ok {
		t.Fatal("plugin-present process did not adopt a generated Schema cache identity")
	}
	cache, err := schemacache.Open(identity.Edition)
	if err != nil {
		t.Fatal(err)
	}
	cacheDir := cache.Directory()
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	assertSchemaCacheArtifactsPresent(t, cacheDir, identity)

	// A later plugin-present process reloads the published cache and serves
	// schema reads without live Cobra assembly.
	cli.RestorePackageCLISchemaDeliveryForTest()
	var factoryCalls atomic.Int64
	cli.RegisterSchemaSourceRoot(func() *cobra.Command {
		factoryCalls.Add(1)
		return NewSchemaSourceRootCommand()
	})
	applyProductionSchemaCache()
	if hit, hitOK := cli.SchemaCacheReadableIdentityForTest(); !hitOK || hit.BuildID != identity.BuildID {
		t.Fatalf("plugin-present reloaded identity = %#v ready=%v", hit, hitOK)
	}
	meta, ok = cli.ResolveMeta("calendar event create")
	if !ok || meta.Identity.Canonical != "calendar.create_calendar_event" {
		t.Fatalf("plugin-present cache-hit ResolveMeta = %#v, %v", meta, ok)
	}
	if factoryCalls.Load() != 0 {
		t.Fatalf("plugin-present cache-hit ResolveMeta invoked Cobra factory %d times", factoryCalls.Load())
	}
}

// The safety argument for publishing the Schema cache from a plugin-present
// process is that schema assembly observes none of the process-global state a
// real plugin loader mutates before returning its commands. This test performs
// those real registration side effects — dynamic endpoint descriptors, a
// registered stdio client, and a plugin auth record — and proves the assembled
// artifacts' identity (a digest over the full schema surface) is unchanged.
func TestCrossPlatformCoverageSchemaAssemblyIgnoresPluginRegistrationSideEffects(t *testing.T) {
	isolatePluginRuntime(t)

	assembleIdentity := func() (buildID [32]byte) {
		resolved, err := cli.ResolveSchemaBuild(NewSchemaSourceRootCommand())
		if err != nil {
			t.Fatal(err)
		}
		artifacts, err := cli.BuildSchemaCacheArtifacts(resolved)
		if err != nil {
			t.Fatal(err)
		}
		identity, err := cli.IdentityFromArtifacts("open", artifacts)
		if err != nil {
			t.Fatal(err)
		}
		return identity.BuildID
	}

	clean := assembleIdentity()

	// Replicate what loadPlugins completes before returning plugin commands:
	// an HTTP plugin server descriptor, a stdio overlay server (registered
	// with an unstarted client), and a plugin auth ownership record.
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()
	registerPluginHTTPServer(mcptypes.ServerDescriptor{
		Key:      "side-effect-http",
		Endpoint: server.URL,
		CLI:      mcptypes.CLIOverlay{ID: "side-effect-http", Command: "side-effect-http"},
	})

	pluginRoot := t.TempDir()
	overlay := []byte(`{
		"id":"local",
		"command":"side-effect-plugin",
		"groups":{"health":{"description":"health checks"}},
		"toolOverrides":{"ping":{"cliName":"ping","group":"health"}}
	}`)
	if err := os.WriteFile(filepath.Join(pluginRoot, "overlay.json"), overlay, 0o600); err != nil {
		t.Fatal(err)
	}
	p := &plugin.Plugin{
		Manifest: plugin.Manifest{
			Name:        "side-effect-plugin",
			Description: "side effect fixture",
			MCPServers: map[string]*plugin.MCPServer{
				"local": {Type: "stdio", Command: "unused", CLI: []byte(`"overlay.json"`)},
			},
		},
		Root: pluginRoot,
	}
	client := transport.NewStdioClient("/bin/sh", []string{"-c", ":"}, nil)
	descriptor := registerStdioServerFromManifest(p, plugin.StdioServerClient{Key: "local", Client: client})
	if descriptor.Endpoint == "" {
		t.Fatal("stdio side-effect server was not registered")
	}
	RegisterPluginAuth("side-effect-plugin", &PluginAuth{Token: "opaque"})

	if _, ok := directRuntimeEndpoint("side-effect-http", ""); !ok {
		t.Fatal("HTTP side-effect endpoint missing before reassembly")
	}
	if _, ok := LookupStdioClient("side-effect-plugin/local"); !ok {
		t.Fatal("stdio side-effect client missing before reassembly")
	}
	if _, ok := LookupPluginAuth("side-effect-plugin"); !ok {
		t.Fatal("plugin auth side-effect record missing before reassembly")
	}

	withPlugins := assembleIdentity()
	if clean != withPlugins {
		t.Fatalf("schema assembly identity changed after real plugin registration side effects: clean=%x with-plugins=%x", clean, withPlugins)
	}
}

// Strongest form of the isolation proof: drive the REAL production loader
// (rootLoadPlugins → loadPlugins) against a fully isolated plugin universe —
// an enabled stdio plugin with a CLI overlay plus pluginConfigs entries whose
// env injection (InjectPluginConfigEnv, the loader's first side effect, which
// may set any non-blacklisted variable including DWS_-prefixed ones) actually
// fires — and assert the assembled schema identity is still byte-identical.
func TestCrossPlatformCoveragePluginConfigInjectionMarksRuntimeUncertainWithoutLoadablePlugin(t *testing.T) {
	isolatePluginRuntime(t)
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	const canary = "DWS_PLUGIN_CONFIG_WITHOUT_PLUGIN"
	_ = os.Unsetenv(canary)
	t.Cleanup(func() { _ = os.Unsetenv(canary) })
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{
	"pluginConfigs": {"removed-plugin": {"`+canary+`": "injected"}}
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	rootPluginLoadHadSideEffects.Store(false)
	if commands := loadPlugins(&cobra.Command{Use: "dws"}, nil, executor.EchoRunner{}, ""); len(commands) != 0 {
		t.Fatalf("removed plugin unexpectedly produced %d commands", len(commands))
	}
	if got := os.Getenv(canary); got != "injected" {
		t.Fatalf("plugin config env injection = %q, want injected", got)
	}
	if !rootPluginLoadHadSideEffects.Load() {
		t.Fatal("plugin config environment injection did not mark runtime uncertain")
	}
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if commands := loadPlugins(&cobra.Command{Use: "dws"}, nil, executor.EchoRunner{}, ""); len(commands) != 0 {
		t.Fatalf("second plugin load unexpectedly produced %d commands", len(commands))
	}
	if !rootPluginLoadHadSideEffects.Load() {
		t.Fatal("plugin uncertainty was cleared after repeated root construction")
	}
}

func TestCrossPlatformCoverageSchemaAssemblyIgnoresRealPluginLoaderSideEffects(t *testing.T) {
	isolatePluginRuntime(t)

	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	t.Setenv("HOME", t.TempDir())

	const (
		canaryDWS   = "DWS_PLUGIN_LOADER_CANARY"
		canaryPlain = "PLUGIN_LOADER_PLAIN_CANARY"
	)
	t.Cleanup(func() {
		_ = os.Unsetenv(canaryDWS)
		_ = os.Unsetenv(canaryPlain)
	})

	pluginDir := filepath.Join(configDir, "plugins", "user", "loader-side-effect")
	if err := os.MkdirAll(filepath.Join(pluginDir, "bin"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(`{
	"name": "loader-side-effect",
	"version": "0.1.0",
	"type": "managed",
	"description": "loader side effect fixture",
	"mcpServers": {
		"local": {
			"name": "local fixture server",
			"description": "stdio fixture",
			"type": "stdio",
			"command": "${DWS_PLUGIN_ROOT}/bin/proxy",
			"args": [],
			"prefixes": ["loader-side-effect"],
			"cli": "overlay.json"
		}
	}
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "overlay.json"), []byte(`{
	"id": "local",
	"command": "loader-side-effect",
	"description": "loader side effect fixture",
	"groups": {"health": {"description": "health checks"}},
	"toolOverrides": {"ping": {"cliName": "ping", "group": "health"}}
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pluginDir, "bin", "proxy"), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	settings := `{
	"enabledPlugins": {"loader-side-effect": true},
	"pluginConfigs": {"loader-side-effect": {
		"` + canaryDWS + `": "loader-injected",
		"` + canaryPlain + `": "plain-injected"
	}}
}`
	if err := os.WriteFile(filepath.Join(configDir, "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}

	assembleIdentity := func() (buildID [32]byte) {
		resolved, err := cli.ResolveSchemaBuild(NewSchemaSourceRootCommand())
		if err != nil {
			t.Fatal(err)
		}
		artifacts, err := cli.BuildSchemaCacheArtifacts(resolved)
		if err != nil {
			t.Fatal(err)
		}
		identity, err := cli.IdentityFromArtifacts("open", artifacts)
		if err != nil {
			t.Fatal(err)
		}
		return identity.BuildID
	}

	clean := assembleIdentity()

	scratch := &cobra.Command{Use: "dws", SilenceErrors: true, SilenceUsage: true}
	pluginCmds := rootLoadPlugins(scratch, nil, executor.EchoRunner{}, "")
	if len(pluginCmds) == 0 {
		t.Fatal("real plugin loader returned no commands for the fixture plugin")
	}

	if got := os.Getenv(canaryDWS); got != "loader-injected" {
		t.Fatalf("plugin config env injection did not fire: %s=%q", canaryDWS, got)
	}
	if got := os.Getenv(canaryPlain); got != "plain-injected" {
		t.Fatalf("plugin config env injection did not fire: %s=%q", canaryPlain, got)
	}
	if _, ok := LookupStdioClient("loader-side-effect/local"); !ok {
		t.Fatal("real loader stdio registration side effect missing")
	}

	withLoader := assembleIdentity()
	if clean != withLoader {
		t.Fatalf("schema assembly identity changed after the real plugin loader ran: clean=%x with-loader=%x", clean, withLoader)
	}
}
