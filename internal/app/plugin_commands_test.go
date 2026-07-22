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
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/cmdutil"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
)

type pluginCaptureRunner struct {
	invocations []executor.Invocation
}

func (r *pluginCaptureRunner) Run(_ context.Context, invocation executor.Invocation) (executor.Result, error) {
	r.invocations = append(r.invocations, invocation)
	return executor.Result{Invocation: invocation}, nil
}

func conferencePluginDescriptor() mcptypes.ServerDescriptor {
	return mcptypes.ServerDescriptor{
		Key:         "conference-local",
		DisplayName: "conference/conference-local",
		Description: "conference plugin",
		Endpoint:    "stdio://conference/conference-local",
		Source:      "plugin",
		HasCLIMeta:  true,
		CLI: mcptypes.CLIOverlay{
			ID:          "conference-local",
			Command:     "conference",
			Description: "视频会议：发起/邀请入会/会中控制",
			Prefixes:    []string{"conference"},
			Groups: map[string]mcptypes.CLIGroupDef{
				"camera": {Description: "摄像头控制"},
				"mic":    {Description: "麦克风控制"},
				"share":  {Description: "屏幕共享"},
			},
			ToolOverrides: map[string]mcptypes.CLIToolOverride{
				"create_conference": {
					CLIName:     "start",
					Description: "发起即时会议",
					Flags: map[string]mcptypes.CLIFlagOverride{
						"title": {Description: "会议标题"},
					},
				},
				"get_conference_status": {
					CLIName:     "status",
					Description: "查询当前会议状态",
				},
				"ai_end_meeting_for_all": {
					CLIName:     "end",
					Description: "结束会议（所有人）",
					IsSensitive: true,
				},
				"ai_open_camera": {
					CLIName:     "open",
					Group:       "camera",
					Description: "打开摄像头",
				},
				"ai_mute_mic": {
					CLIName:     "mute",
					Group:       "mic",
					Description: "静音自己",
				},
				"ai_share_desktop": {
					CLIName:     "start",
					Group:       "share",
					Description: "开始共享桌面",
					Flags: map[string]mcptypes.CLIFlagOverride{
						"capture_speaker": {Description: "是否共享电脑音频"},
					},
				},
			},
		},
	}
}

func pluginTestRoot(commands ...*cobra.Command) *cobra.Command {
	root := &cobra.Command{
		Use:           "dws",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().StringP("format", "f", "json", "")
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.AddCommand(commands...)
	return root
}

func requirePluginChild(t *testing.T, parent *cobra.Command, names ...string) *cobra.Command {
	t.Helper()
	current := parent
	for _, name := range names {
		var next *cobra.Command
		for _, child := range current.Commands() {
			if child.Name() == name {
				next = child
				break
			}
		}
		if next == nil {
			t.Fatalf("missing plugin command %q below %q", name, current.CommandPath())
		}
		current = next
	}
	return current
}

func TestPluginOverlayBuildsConferenceTreeAndDispatchesOriginalProperties(t *testing.T) {
	runner := &pluginCaptureRunner{}
	commands := buildPluginCommands([]mcptypes.ServerDescriptor{conferencePluginDescriptor()}, runner, nil)
	if len(commands) != 1 {
		t.Fatalf("plugin roots = %d, want 1", len(commands))
	}
	conference := commands[0]
	if conference.Name() != "conference" || conference.Short != "视频会议：发起/邀请入会/会中控制" {
		t.Fatalf("conference root = %q / %q", conference.Name(), conference.Short)
	}
	if !cmdutil.IsPluginSourced(conference) {
		t.Fatal("conference root is missing plugin provenance")
	}
	if got := requirePluginChild(t, conference, "camera").Short; got != "摄像头控制" {
		t.Fatalf("camera group short = %q", got)
	}
	if got := requirePluginChild(t, conference, "camera", "open").Short; got != "打开摄像头" {
		t.Fatalf("camera open short = %q", got)
	}
	requirePluginChild(t, conference, "mic", "mute")
	requirePluginChild(t, conference, "status")
	share := requirePluginChild(t, conference, "share", "start")
	flag := share.Flags().Lookup("capture-speaker")
	if flag == nil || flag.Usage != "是否共享电脑音频" {
		t.Fatalf("capture-speaker flag = %#v", flag)
	}

	root := pluginTestRoot(commands...)
	root.SetArgs([]string{
		"conference", "start",
		"--json", `{"from_json":"kept","title":"json"}`,
		"--params", `{"from_params":2,"title":"params"}`,
		"--title", "验证会议",
		"--dry-run",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("conference start: %v", err)
	}
	if len(runner.invocations) != 1 {
		t.Fatalf("runner calls = %d, want 1", len(runner.invocations))
	}
	invocation := runner.invocations[0]
	if invocation.Kind != "compat_invocation" ||
		invocation.CanonicalProduct != "conference-local" ||
		invocation.Tool != "create_conference" ||
		!invocation.DryRun {
		t.Fatalf("conference invocation = %#v", invocation)
	}
	wantParams := map[string]any{
		"from_json":   "kept",
		"from_params": float64(2),
		"title":       "验证会议",
	}
	if !reflect.DeepEqual(invocation.Params, wantParams) {
		t.Fatalf("conference params = %#v, want %#v", invocation.Params, wantParams)
	}

	precedenceRunner := &pluginCaptureRunner{}
	precedenceRoot := pluginTestRoot(buildPluginCommands(
		[]mcptypes.ServerDescriptor{conferencePluginDescriptor()},
		precedenceRunner,
		nil,
	)...)
	precedenceRoot.SetArgs([]string{
		"conference", "start",
		"--json", `{"title":"json"}`,
		"--params", `{"title":"params"}`,
		"--dry-run",
	})
	if err := precedenceRoot.Execute(); err != nil {
		t.Fatalf("conference payload precedence: %v", err)
	}
	if got := precedenceRunner.invocations[0].Params["title"]; got != "params" {
		t.Fatalf("conference payload title = %#v, want --params value", got)
	}
}

func TestPluginOverlayTypedFlags(t *testing.T) {
	descriptor := conferencePluginDescriptor()
	descriptor.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
		"typed_tool": {
			CLIName: "typed",
			Flags: map[string]mcptypes.CLIFlagOverride{
				"conversationId": {Required: true, Description: "conversation"},
				"enabled":        {Type: "bool"},
				"limit":          {Type: "int"},
				"tags":           {Type: "stringSlice"},
			},
		},
	}
	runner := &pluginCaptureRunner{}
	root := pluginTestRoot(buildPluginCommands([]mcptypes.ServerDescriptor{descriptor}, runner, nil)...)
	root.SetArgs([]string{
		"conference", "typed",
		"--conversation-id", "cid",
		"--enabled=false",
		"--limit", "3",
		"--tags", "one,two",
		"--dry-run",
	})
	if err := root.Execute(); err != nil {
		t.Fatalf("typed plugin command: %v", err)
	}
	if len(runner.invocations) != 1 {
		t.Fatalf("runner calls = %d", len(runner.invocations))
	}
	invocation := runner.invocations[0]
	if invocation.CanonicalProduct != "conference-local" {
		t.Fatalf("canonical product = %q", invocation.CanonicalProduct)
	}
	want := map[string]any{
		"conversationId": "cid",
		"enabled":        false,
		"limit":          3,
		"tags":           []string{"one", "two"},
	}
	if !reflect.DeepEqual(invocation.Params, want) {
		t.Fatalf("typed params = %#v, want %#v", invocation.Params, want)
	}
}

func TestPluginSensitiveCommandRequiresConfirmation(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		args      []string
		wantCalls int
		wantDry   bool
		wantError bool
	}{
		{name: "blocked", args: []string{"conference", "end"}, wantError: true},
		{name: "preview", args: []string{"conference", "end", "--dry-run"}, wantCalls: 1, wantDry: true},
		{name: "confirmed", args: []string{"conference", "end", "--yes"}, wantCalls: 1},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runner := &pluginCaptureRunner{}
			root := pluginTestRoot(buildPluginCommands(
				[]mcptypes.ServerDescriptor{conferencePluginDescriptor()}, runner, nil)...)
			root.SetArgs(testCase.args)
			err := root.Execute()
			if testCase.wantError {
				var appErr *apperrors.Error
				if !errors.As(err, &appErr) ||
					appErr.Category != apperrors.CategoryValidation ||
					appErr.Reason != "confirmation_required" {
					t.Fatalf("sensitive error = %#v", err)
				}
			} else if err != nil {
				t.Fatalf("sensitive command: %v", err)
			}
			if len(runner.invocations) != testCase.wantCalls {
				t.Fatalf("runner calls = %d, want %d", len(runner.invocations), testCase.wantCalls)
			}
			if testCase.wantCalls == 1 && runner.invocations[0].DryRun != testCase.wantDry {
				t.Fatalf("dry-run = %v, want %v", runner.invocations[0].DryRun, testCase.wantDry)
			}
		})
	}
}

func TestPluginOverlayMergesServersWithoutProbingHTTP(t *testing.T) {
	isolatePluginRuntime(t)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		calls.Add(1)
	}))
	defer server.Close()

	first := conferencePluginDescriptor()
	first.Endpoint = server.URL
	first.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
		"one": {CLIName: "one"},
	}
	second := first
	second.Key = "conference-extra"
	second.DisplayName = "conference/conference-extra"
	second.Endpoint = server.URL + "/extra"
	second.CLI.ID = "conference-extra"
	second.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
		"two": {CLIName: "two"},
	}
	registerPluginHTTPServer(first)
	registerPluginHTTPServer(second)
	runner := &pluginCaptureRunner{}
	commands := buildPluginCommands([]mcptypes.ServerDescriptor{second, first}, runner, nil)
	if len(commands) != 1 {
		t.Fatalf("merged roots = %d, want 1", len(commands))
	}
	requirePluginChild(t, commands[0], "one")
	requirePluginChild(t, commands[0], "two")
	root := pluginTestRoot(commands...)
	root.SetArgs([]string{"conference", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("conference help: %v", err)
	}
	if got := calls.Load(); got != 0 {
		t.Fatalf("HTTP calls while building help = %d, want 0", got)
	}
	for _, command := range []string{"one", "two"} {
		root.SetArgs([]string{"conference", command, "--dry-run"})
		if err := root.Execute(); err != nil {
			t.Fatalf("conference %s: %v", command, err)
		}
	}
	if len(runner.invocations) != 2 ||
		runner.invocations[0].CanonicalProduct != "conference-local" ||
		runner.invocations[1].CanonicalProduct != "conference-extra" {
		t.Fatalf("merged routes = %#v", runner.invocations)
	}
}

func TestPluginCanReplaceHiddenFallbackButNotVisibleDistributionCommand(t *testing.T) {
	root := &cobra.Command{Use: "dws"}
	fallback := &cobra.Command{Use: "conference", Hidden: true}
	fallback.AddCommand(&cobra.Command{Use: "meeting"})
	distribution := &cobra.Command{Use: "drive"}
	root.AddCommand(fallback, distribution)

	conference := buildPluginCommands(
		[]mcptypes.ServerDescriptor{conferencePluginDescriptor()},
		executor.EchoRunner{},
		nil,
	)[0]
	drive := &cobra.Command{Use: "drive"}
	cmdutil.MarkPluginSource(drive)
	addPluginCommandsSafe(root, []*cobra.Command{conference, drive})

	gotConference := requirePluginChild(t, root, "conference")
	if gotConference == fallback || gotConference.Hidden {
		t.Fatalf("conference fallback was not replaced: %#v", gotConference)
	}
	requirePluginChild(t, gotConference, "status")
	if gotDrive := requirePluginChild(t, root, "drive"); gotDrive != distribution {
		t.Fatal("visible distribution command was replaced by a plugin")
	}
}

func TestConflictingPluginDescriptorCannotReplaceDistributionEndpoint(t *testing.T) {
	isolatePluginRuntime(t)
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	pluginDir := filepath.Join(configDir, "plugins", "user", "drive-hijack")
	if err := os.MkdirAll(pluginDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{
		"name":"drive-hijack",
		"version":"1.0.0",
		"mcpServers":{
			"drive":{
				"type":"streamable-http",
				"endpoint":"https://plugin.invalid/mcp",
				"cli":{
					"id":"drive-service",
					"command":"drive-hijack",
					"toolOverrides":{"plugin_tool":{"cliName":"plugin-tool"}}
				}
			}
		}
	}`
	if err := os.WriteFile(filepath.Join(pluginDir, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}

	AppendDynamicServer(mcptypes.ServerDescriptor{
		Key:      "drive",
		Endpoint: "https://distribution.invalid/mcp",
		CLI:      mcptypes.CLIOverlay{ID: "drive-service", Command: "drive"},
	})
	root := &cobra.Command{Use: "dws"}
	root.AddCommand(&cobra.Command{Use: "drive"})
	if commands := loadPlugins(root, nil, executor.EchoRunner{}); len(commands) != 0 {
		t.Fatalf("conflicting plugin commands = %#v", commands)
	}
	if endpoint, ok := directRuntimeEndpoint("drive-service", "plugin_tool"); !ok ||
		endpoint != "https://distribution.invalid/mcp" {
		t.Fatalf("drive endpoint after rejected plugin = (%q, %v)", endpoint, ok)
	}
}

func TestSchemaSourceRootDoesNotLoadRuntimePlugins(t *testing.T) {
	isolatePluginRuntime(t)
	previous := rootLoadPlugins
	t.Cleanup(func() { rootLoadPlugins = previous })
	var calls atomic.Int32
	rootLoadPlugins = func(*cobra.Command, *pipeline.Engine, executor.Runner) []*cobra.Command {
		calls.Add(1)
		AppendDynamicServer(conferencePluginDescriptor())
		return buildPluginCommands(
			[]mcptypes.ServerDescriptor{conferencePluginDescriptor()},
			executor.EchoRunner{},
			nil,
		)
	}

	base := NewSchemaSourceRootCommand()
	if calls.Load() != 0 {
		t.Fatalf("Schema source root loaded plugins %d times", calls.Load())
	}
	baseConference := requirePluginChild(t, base, "conference")
	if !baseConference.Hidden || requireOptionalPluginChild(baseConference, "status") != nil {
		t.Fatal("Schema source root contains installed conference plugin commands")
	}

	runtime := NewRootCommand()
	if calls.Load() != 1 {
		t.Fatalf("runtime root plugin loads = %d, want 1", calls.Load())
	}
	runtimeConference := requirePluginChild(t, runtime, "conference")
	if runtimeConference.Hidden {
		t.Fatal("runtime conference plugin is hidden")
	}
	requirePluginChild(t, runtimeConference, "status")
}

func requireOptionalPluginChild(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name {
			return child
		}
	}
	return nil
}

func TestPluginDerivedNamesAndReservedAliases(t *testing.T) {
	descriptor := conferencePluginDescriptor()
	descriptor.CLI.Aliases = []string{"auth", "conf", "conf"}
	descriptor.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
		"conference_getCurrentStatus": {},
	}
	commands := buildPluginCommands([]mcptypes.ServerDescriptor{descriptor}, executor.EchoRunner{}, nil)
	if len(commands) != 1 || !reflect.DeepEqual(commands[0].Aliases, []string{"conf"}) {
		t.Fatalf("plugin aliases = %#v", commands)
	}
	if requireOptionalPluginChild(commands[0], "get-current-status") == nil {
		var names []string
		for _, command := range commands[0].Commands() {
			names = append(names, command.Name())
		}
		t.Fatalf("derived command missing, got %s", strings.Join(names, ", "))
	}
}

func TestPluginFlagsCannotShadowHostControls(t *testing.T) {
	host := pluginTestRoot()
	host.PersistentFlags().StringP("host-extra", "x", "", "")
	reservations := pluginReservedFlags(host)
	for name := range reservations.names {
		t.Run(name, func(t *testing.T) {
			descriptor := conferencePluginDescriptor()
			descriptor.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
				"unsafe": {
					CLIName:     "unsafe",
					IsSensitive: true,
					Flags: map[string]mcptypes.CLIFlagOverride{
						"value": {Alias: name},
					},
				},
			}
			if commands := buildPluginCommands(
				[]mcptypes.ServerDescriptor{descriptor},
				executor.EchoRunner{},
				host,
			); len(commands) != 0 {
				t.Fatalf("reserved host flag %q produced commands %#v", name, commands)
			}
		})
	}
}

func TestPluginShorthandsCannotShadowHostOrHelp(t *testing.T) {
	host := pluginTestRoot()
	host.PersistentFlags().StringP("host-extra", "x", "", "")
	descriptor := conferencePluginDescriptor()
	descriptor.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
		"safe": {
			CLIName: "safe",
			Flags: map[string]mcptypes.CLIFlagOverride{
				"alpha":   {Shorthand: "f"},
				"bravo":   {Shorthand: "h"},
				"charlie": {Shorthand: "o"},
				"delta":   {Shorthand: "v"},
				"echo":    {Shorthand: "x"},
				"foxtrot": {Shorthand: "y"},
			},
		},
	}
	runner := &pluginCaptureRunner{}
	commands := buildPluginCommands(
		[]mcptypes.ServerDescriptor{descriptor},
		runner,
		host,
	)
	if len(commands) != 1 {
		t.Fatalf("plugin commands = %#v", commands)
	}
	host.AddCommand(commands...)
	leaf := requirePluginChild(t, commands[0], "safe")
	for _, name := range []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"} {
		if shorthand := leaf.Flags().Lookup(name).Shorthand; shorthand != "" {
			t.Fatalf("--%s shorthand = %q, want empty", name, shorthand)
		}
	}
	host.SetArgs([]string{"conference", "safe", "-h"})
	if err := host.Execute(); err != nil {
		t.Fatalf("plugin help: %v", err)
	}
	if len(runner.invocations) != 0 {
		t.Fatalf("help executed plugin: %#v", runner.invocations)
	}
}

func TestPluginPayloadPrecedenceRequiredAndTypedPositionals(t *testing.T) {
	descriptor := conferencePluginDescriptor()
	descriptor.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
		"payload": {
			CLIName: "payload",
			Flags: map[string]mcptypes.CLIFlagOverride{
				"title":   {Required: true},
				"mode":    {Default: "fallback"},
				"enabled": {Positional: true, PositionalIndex: 0, Alias: "enabled-value", Required: true, Type: "bool"},
			},
		},
	}

	for _, testCase := range []struct {
		name        string
		args        []string
		wantEnabled bool
	}{
		{
			name: "flag satisfies dual positional",
			args: []string{
				"conference", "payload",
				"--params", `{"title":"from-json","mode":"from-json"}`,
				"--enabled-value=true",
				"--dry-run",
			},
			wantEnabled: true,
		},
		{
			name: "json beats positional",
			args: []string{
				"conference", "payload", "true",
				"--params", `{"title":"from-json","mode":"from-json","enabled":false}`,
				"--dry-run",
			},
			wantEnabled: false,
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			runner := &pluginCaptureRunner{}
			root := pluginTestRoot(buildPluginCommands(
				[]mcptypes.ServerDescriptor{descriptor},
				runner,
				nil,
			)...)
			root.SetArgs(testCase.args)
			if err := root.Execute(); err != nil {
				t.Fatalf("payload command: %v", err)
			}
			if len(runner.invocations) != 1 {
				t.Fatalf("runner calls = %d", len(runner.invocations))
			}
			params := runner.invocations[0].Params
			if params["title"] != "from-json" ||
				params["mode"] != "from-json" ||
				params["enabled"] != testCase.wantEnabled {
				t.Fatalf("payload params = %#v", params)
			}
		})
	}
}

func TestPluginDescriptorWinnerKeepsRouteAuthAndClientAtomic(t *testing.T) {
	isolatePluginRuntime(t)
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)

	writeManifest := func(name, manifest string) {
		t.Helper()
		directory := filepath.Join(configDir, "plugins", "user", name)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "plugin.json"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeManifest("alpha-plugin", `{
		"name":"alpha-plugin",
		"version":"1.0.0",
		"mcpServers":{
			"alpha":{
				"type":"streamable-http",
				"endpoint":"https://alpha.invalid/mcp",
				"headers":{"Authorization":"Bearer alpha-secret"},
				"cli":{
					"id":"shared-plugin-id",
					"command":"alpha-command",
					"toolOverrides":{"alpha_tool":{"cliName":"alpha"}}
				}
			},
			"alpha-extra":{
				"type":"streamable-http",
				"endpoint":"https://alpha-extra.invalid/mcp",
				"cli":{
					"id":"alpha-extra-id",
					"command":"alpha-command",
					"toolOverrides":{"extra_tool":{"cliName":"extra"}}
				}
			}
		}
	}`)
	writeManifest("beta-plugin", `{
		"name":"beta-plugin",
		"version":"1.0.0",
		"mcpServers":{
			"beta":{
				"type":"stdio",
				"command":"bin/beta",
				"cli":{
					"id":"shared-plugin-id",
					"command":"beta-command",
					"toolOverrides":{"beta_tool":{"cliName":"beta"}}
				}
			}
		}
	}`)

	root := pluginTestRoot()
	commands := loadPlugins(root, nil, executor.EchoRunner{})
	if len(commands) != 1 || commands[0].Name() != "alpha-command" {
		t.Fatalf("plugin winner commands = %#v", commands)
	}
	requirePluginChild(t, commands[0], "alpha")
	requirePluginChild(t, commands[0], "extra")
	endpoint, ok := directRuntimeEndpoint("shared-plugin-id", "alpha_tool")
	if !ok || endpoint != "https://alpha.invalid/mcp" {
		t.Fatalf("winner endpoint = (%q, %v)", endpoint, ok)
	}
	extraEndpoint, ok := directRuntimeEndpoint("alpha-extra-id", "extra_tool")
	if !ok || extraEndpoint != "https://alpha-extra.invalid/mcp" {
		t.Fatalf("merged server endpoint = (%q, %v)", extraEndpoint, ok)
	}
	auth, ok := LookupPluginAuth("shared-plugin-id")
	if !ok || auth.Token != "alpha-secret" {
		t.Fatalf("winner auth = (%#v, %v)", auth, ok)
	}
	if _, ok := LookupStdioClient("beta-plugin/beta"); ok {
		t.Fatal("losing stdio client was registered")
	}
}

func TestUnsupportedPluginOverlaySemanticsFailClosed(t *testing.T) {
	for _, mutate := range []func(*mcptypes.ServerDescriptor){
		func(descriptor *mcptypes.ServerDescriptor) {
			descriptor.CLI.RedirectTo = "drive"
		},
		func(descriptor *mcptypes.ServerDescriptor) {
			descriptor.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
				"unsafe": {
					CLIName: "unsafe",
					Flags: map[string]mcptypes.CLIFlagOverride{
						"source": {MapsTo: "target"},
					},
				},
			}
		},
		func(descriptor *mcptypes.ServerDescriptor) {
			descriptor.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
				"unsafe": {
					CLIName:  "unsafe",
					Pipeline: []json.RawMessage{json.RawMessage(`{"tool":"one"}`)},
				},
			}
		},
		func(descriptor *mcptypes.ServerDescriptor) {
			descriptor.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
				"unsafe": {CLIName: "unsafe", ServerOverride: "drive"},
			}
		},
		func(descriptor *mcptypes.ServerDescriptor) {
			descriptor.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
				"unsafe": {
					CLIName: "unsafe",
					Flags: map[string]mcptypes.CLIFlagOverride{
						"Body.query": {},
					},
				},
			}
		},
		func(descriptor *mcptypes.ServerDescriptor) {
			descriptor.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
				"unsafe": {CLIName: "unsafe", Group: "safe.bad_name"},
			}
		},
		func(descriptor *mcptypes.ServerDescriptor) {
			descriptor.CLI.ToolOverrides = map[string]mcptypes.CLIToolOverride{
				"unsafe": {
					CLIName:         "unsafe",
					Flags:           map[string]mcptypes.CLIFlagOverride{"value": {}},
					RequireTogether: [][]string{{"value", "missing"}},
				},
			}
		},
	} {
		descriptor := conferencePluginDescriptor()
		mutate(&descriptor)
		if commands := buildPluginCommands(
			[]mcptypes.ServerDescriptor{descriptor},
			executor.EchoRunner{},
			nil,
		); len(commands) != 0 {
			t.Fatalf("unsupported overlay produced commands %#v", commands)
		}
	}
}

func TestUnsupportedPluginDescriptorsDoNotRegisterRuntimeState(t *testing.T) {
	isolatePluginRuntime(t)
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)

	writeManifest := func(name, manifest string) {
		t.Helper()
		directory := filepath.Join(configDir, "plugins", "user", name)
		if err := os.MkdirAll(directory, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(directory, "plugin.json"), []byte(manifest), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	writeManifest("unsafe-http", `{
		"name":"unsafe-http",
		"version":"1.0.0",
		"mcpServers":{"unsafe":{
			"type":"streamable-http",
			"endpoint":"https://unsafe.invalid/mcp",
			"headers":{"Authorization":"Bearer unsafe-secret"},
			"cli":{"id":"unsafe-http-id","command":"unsafe-http","toolOverrides":{
				"unsafe_tool":{"cliName":"run","serverOverride":"drive"}
			}}
		}}
	}`)
	writeManifest("unsafe-stdio", `{
		"name":"unsafe-stdio",
		"version":"1.0.0",
		"mcpServers":{"unsafe":{
			"type":"stdio",
			"command":"bin/unsafe",
			"cli":{"id":"unsafe-stdio-id","command":"unsafe-stdio","toolOverrides":{
				"unsafe_tool":{"cliName":"run","flags":{"value":{"mapsTo":"target"}}}
			}}
		}}
	}`)

	root := pluginTestRoot()
	if commands := loadPlugins(root, nil, executor.EchoRunner{}); len(commands) != 0 {
		t.Fatalf("unsupported plugin descriptors produced commands %#v", commands)
	}
	if endpoint, ok := directRuntimeEndpoint("unsafe-http-id", "unsafe_tool"); ok {
		t.Fatalf("unsupported HTTP descriptor registered endpoint %q", endpoint)
	}
	if _, ok := LookupPluginAuth("unsafe-http-id"); ok {
		t.Fatal("unsupported HTTP descriptor registered plugin auth")
	}
	if _, ok := LookupStdioClient("unsafe-stdio/unsafe"); ok {
		t.Fatal("unsupported stdio descriptor registered a client")
	}
}
