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
	"maps"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	outputpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtimecontext"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/agentproduct"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/configmeta"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageParseAgentVersion(t *testing.T) {
	var nilContext context.Context
	if _, ok := agentMetadataSnapshotFromContext(nilContext); ok {
		t.Fatal("nil context unexpectedly contained Agent metadata")
	}
	wantSnapshot := agentMetadataSnapshot{version: "context-version", ext: "{}"}
	if got, ok := agentMetadataSnapshotFromContext(contextWithAgentMetadataSnapshot(context.Background(), wantSnapshot)); !ok || got != wantSnapshot {
		t.Fatalf("context Agent metadata = %#v, %v; want %#v", got, ok, wantSnapshot)
	}

	valid := []struct {
		name string
		raw  string
		want string
	}{
		{name: "unset", raw: "", want: ""},
		{name: "ASCII whitespace only", raw: " \t ", want: ""},
		{name: "semantic version", raw: "1.2.3", want: "1.2.3"},
		{name: "pre-release and build", raw: " v1.2.3-rc.1+build_7 ", want: "v1.2.3-rc.1+build_7"},
		{name: "maximum length", raw: strings.Repeat("a", maxAgentVersionBytes), want: strings.Repeat("a", maxAgentVersionBytes)},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAgentVersion(tc.raw)
			if err != nil {
				t.Fatalf("parseAgentVersion() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("parseAgentVersion() = %q, want %q", got, tc.want)
			}
		})
	}

	invalid := []struct {
		name string
		raw  string
	}{
		{name: "leading punctuation", raw: "-1.2.3"},
		{name: "internal space", raw: "1.2 3"},
		{name: "slash", raw: "1.2/3"},
		{name: "line feed", raw: "1.2.3\n"},
		{name: "carriage return", raw: "1.2.3\r"},
		{name: "NUL", raw: "1.2\x003"},
		{name: "Unicode", raw: "版本1"},
		{name: "too long", raw: strings.Repeat("a", maxAgentVersionBytes+1)},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAgentVersion(tc.raw)
			if err == nil || got != "" {
				t.Fatalf("parseAgentVersion(%q) = %q, %v; want validation error", tc.raw, got, err)
			}
			assertAgentMetadataValidationError(t, err, "invalid_agent_version", tc.raw)
		})
	}
}

func TestCrossPlatformCoverageParseAgentExt(t *testing.T) {
	boundary := `{"x":"` + strings.Repeat("a", maxAgentExtensionBytes-8) + `"}`
	if len(boundary) != maxAgentExtensionBytes {
		t.Fatalf("invalid boundary fixture size: %d", len(boundary))
	}

	valid := []struct {
		name string
		raw  string
		want string
	}{
		{name: "unset", raw: "", want: ""},
		{name: "ASCII whitespace only", raw: " \t ", want: ""},
		{name: "empty object", raw: "{}", want: "{}"},
		{name: "compact generic object", raw: " \t{ \"umt\": \"masked\",\t \"nested\": { \"ok\": true }, \"unknown\": [1, 2] }\t ", want: `{"umt":"masked","nested":{"ok":true},"unknown":[1,2]}`},
		{name: "Unicode value", raw: `{"ua":"千问办公/1.0"}`, want: `{"ua":"千问办公/1.0"}`},
		{name: "escaped control remains safe", raw: `{"ua":"line\nnext"}`, want: `{"ua":"line\nnext"}`},
		{name: "maximum length", raw: boundary, want: boundary},
	}
	for _, tc := range valid {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAgentExt(tc.raw)
			if err != nil {
				t.Fatalf("parseAgentExt() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("parseAgentExt() = %q, want %q", got, tc.want)
			}
		})
	}

	invalidUTF8 := string([]byte{'{', '"', 'x', '"', ':', '"', 0xff, '"', '}'})
	invalid := []struct {
		name string
		raw  string
	}{
		{name: "too long raw input", raw: strings.Repeat(" ", maxAgentExtensionBytes+1)},
		{name: "invalid UTF-8", raw: invalidUTF8},
		{name: "array", raw: `[]`},
		{name: "string", raw: `"value"`},
		{name: "number", raw: `1`},
		{name: "boolean", raw: `true`},
		{name: "null", raw: `null`},
		{name: "malformed object", raw: `{"secret":"DO_NOT_ECHO"`},
		{name: "trailing value", raw: `{} {}`},
		{name: "line feed", raw: "{\n}"},
		{name: "carriage return", raw: "{\r}"},
		{name: "NUL", raw: "{\x00}"},
		{name: "vertical tab", raw: "{\v}"},
		{name: "form feed", raw: "{\f}"},
		{name: "DEL", raw: "{\x7f}"},
		{name: "C1 control", raw: "{\u0085}"},
	}
	for _, tc := range invalid {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseAgentExt(tc.raw)
			if err == nil || got != "" {
				t.Fatalf("parseAgentExt() = %q, %v; want validation error", got, err)
			}
			assertAgentMetadataValidationError(t, err, "invalid_agent_ext", tc.raw)
		})
	}
}

func assertAgentMetadataValidationError(t *testing.T, err error, reason, raw string) {
	t.Helper()
	var appErr *apperrors.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("error type = %T, want *errors.Error", err)
	}
	if appErr.Category != apperrors.CategoryValidation || appErr.Reason != reason {
		t.Fatalf("error = category %q reason %q, want validation/%s", appErr.Category, appErr.Reason, reason)
	}
	if strings.Contains(raw, "DO_NOT_ECHO") && strings.Contains(err.Error(), "DO_NOT_ECHO") {
		t.Fatalf("error must not echo invalid value: %v", err)
	}
}

func TestCrossPlatformCoverageAgentMetadataConfigRegistrationAndMasking(t *testing.T) {
	items := configmeta.All()
	var versionItem, extItem *configmeta.ConfigItem
	for i := range items {
		switch items[i].Name {
		case envDWSAgentVersion:
			versionItem = &items[i]
		case envDWSAgentExt:
			extItem = &items[i]
		}
	}
	if versionItem == nil || extItem == nil {
		t.Fatalf("Agent metadata config registration missing: version=%v ext=%v", versionItem != nil, extItem != nil)
	}
	if versionItem.Category != configmeta.CategoryExternal || versionItem.Sensitive {
		t.Fatalf("version config metadata = %#v", *versionItem)
	}
	if extItem.Category != configmeta.CategoryExternal || !extItem.Sensitive {
		t.Fatalf("extension config metadata = %#v", *extItem)
	}

	const canary = `{"umt":"SENSITIVE_CANARY"}`
	t.Setenv(envDWSAgentExt, canary)
	got, ok := configmeta.Resolve(envDWSAgentExt)
	if !ok || got == "" || strings.Contains(got, "SENSITIVE_CANARY") || got == canary {
		t.Fatalf("sensitive extension was not masked: value=%q ok=%v", got, ok)
	}

	t.Setenv(envDWSAgentVersion, "9.8.7")
	command := newConfigListCommand()
	var output strings.Builder
	command.SetOut(&output)
	command.SetArgs([]string{"--category", string(configmeta.CategoryExternal), "--show-values", "--json"})
	if err := command.Execute(); err != nil {
		t.Fatalf("config list failed: %v", err)
	}
	rawOutput := output.String()
	if !json.Valid([]byte(rawOutput)) {
		t.Fatalf("config list emitted invalid JSON: %q", rawOutput)
	}
	if !strings.Contains(rawOutput, envDWSAgentVersion) || !strings.Contains(rawOutput, envDWSAgentExt) {
		t.Fatalf("config list omitted Agent metadata variables: %s", rawOutput)
	}
	if strings.Contains(rawOutput, "SENSITIVE_CANARY") || strings.Contains(rawOutput, canary) {
		t.Fatalf("config list leaked Agent extension: %s", rawOutput)
	}
}

func TestCrossPlatformCoverageResolveMCPRequestHeadersScopesAndFinalizesAgentMetadata(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(envDWSAgentHost, "")
	t.Setenv(agentproduct.EnvName, "")
	t.Setenv(envDWSAgentVersion, " 1.2.3-rc.1 ")
	t.Setenv(envDWSAgentExt, " { \"umt\": \"masked\", \"unknown\": true } ")

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	edition.Override(&edition.Hooks{
		MergeHeaders: func(headers map[string]string) map[string]string {
			headers["X-Dws-Agent-Ver"] = "merge-must-not-win"
			headers["X-Dws-Agent-Ext"] = `{"source":"merge"}`
			return headers
		},
		EnterpriseCredentialHeaders: func(headers map[string]string) map[string]string {
			headers[transport.HeaderAgentVersion] = "credential-must-not-win"
			headers[transport.HeaderAgentExt] = `{"source":"credential"}`
			return headers
		},
	})

	for name, headers := range map[string]map[string]string{
		"shared identity": resolveIdentityHeaders(),
		"A2A export":      MCPIdentityHeaders(),
	} {
		if hasHeaderFold(headers, transport.HeaderAgentVersion) || hasHeaderFold(headers, transport.HeaderAgentExt) {
			t.Fatalf("%s leaked MCP-only metadata: %#v", name, headers)
		}
	}

	headers := resolveMCPRequestHeaders()
	if got := headers[transport.HeaderAgentVersion]; got != "1.2.3-rc.1" {
		t.Fatalf("%s = %q, want 1.2.3-rc.1", transport.HeaderAgentVersion, got)
	}
	if got := headers[transport.HeaderAgentExt]; got != `{"umt":"masked","unknown":true}` {
		t.Fatalf("%s = %q", transport.HeaderAgentExt, got)
	}
	if got := headers[transport.HeaderVersion]; got != version {
		t.Fatalf("%s = %q, want CLI version %q", transport.HeaderVersion, got, version)
	}
	if _, ok := headers["User-Agent"]; ok {
		t.Fatal("Agent extension must not create or replace the standard User-Agent header")
	}
	for _, key := range []string{"umt", "miniwua", "ua", "x-dws-agent-umt", "x-dws-agent-miniwua", "x-dws-agent-ua"} {
		if hasHeaderFold(headers, key) {
			t.Fatalf("Agent extension was split into an extra header %q: %#v", key, headers)
		}
	}

	// Library paths are best-effort: one invalid value is omitted without
	// suppressing the other valid field or preserving hook-injected values.
	t.Setenv(envDWSAgentExt, `{"secret":"DO_NOT_ECHO"`)
	headers = resolveMCPRequestHeaders()
	if got := headers[transport.HeaderAgentVersion]; got != "1.2.3-rc.1" {
		t.Fatalf("valid version was suppressed: %q", got)
	}
	if hasHeaderFold(headers, transport.HeaderAgentExt) {
		t.Fatalf("invalid extension or hook value leaked: %#v", headers)
	}

	// Exercise the nil-map and empty-input library paths. An absent environment
	// must not allocate a map, while an EXT-only value must allocate one and
	// remain a single compact Header.
	t.Setenv(envDWSAgentVersion, "")
	t.Setenv(envDWSAgentExt, "")
	if got := applyAgentMetadataHeaders(nil); got != nil {
		t.Fatalf("empty metadata allocated headers: %#v", got)
	}
	t.Setenv(envDWSAgentExt, " { } ")
	headers = applyAgentMetadataHeaders(nil)
	if got := headers[transport.HeaderAgentExt]; got != "{}" {
		t.Fatalf("EXT-only metadata = %q, want {}", got)
	}
}

func TestCrossPlatformCoverageRootRejectsInvalidAgentMetadataBeforeEditionHook(t *testing.T) {
	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })

	tests := []struct {
		name   string
		env    string
		value  string
		reason string
	}{
		{name: "version", env: envDWSAgentVersion, value: "DO_NOT ECHO", reason: "invalid_agent_version"},
		{name: "extension", env: envDWSAgentExt, value: `{"secret":"DO_NOT_ECHO"`, reason: "invalid_agent_ext"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			t.Setenv(envDWSAgentHost, "")
			t.Setenv(agentproduct.EnvName, "")
			t.Setenv(envDWSAgentVersion, "")
			t.Setenv(envDWSAgentExt, "")
			t.Setenv(tc.env, tc.value)

			headerHookCalled := false
			afterHookCalled := false
			edition.Override(&edition.Hooks{
				MergeHeaders: func(headers map[string]string) map[string]string {
					headerHookCalled = true
					return headers
				},
				EnterpriseCredentialHeaders: func(headers map[string]string) map[string]string {
					headerHookCalled = true
					return headers
				},
				AfterPersistentPreRun: func(_ *cobra.Command, _ []string) error {
					afterHookCalled = true
					return nil
				},
			})

			root := NewRootCommand()
			root.SetOut(io.Discard)
			root.SetErr(io.Discard)
			root.SetArgs([]string{"version"})
			err := root.Execute()
			if err == nil {
				t.Fatalf("root command accepted invalid %s", tc.env)
			}
			if headerHookCalled || afterHookCalled {
				t.Fatalf("edition hook ran before %s validation", tc.env)
			}
			assertAgentMetadataValidationError(t, err, tc.reason, tc.value)
		})
	}
}

func TestCrossPlatformCoverageAgentMetadataProcessEntryValidationPrecedesRootConstruction(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want bool
	}{
		{name: "default JSON", args: []string{"version"}, want: true},
		{name: "long JSON", args: []string{"version", "--format", "JSON"}, want: true},
		{name: "long table", args: []string{"--format=table", "version"}, want: false},
		{name: "short attached JSON", args: []string{"version", "-fjson"}, want: true},
		{name: "short table", args: []string{"version", "-f", "table"}, want: false},
		{name: "last wins", args: []string{"--format", "table", "version", "-f=json"}, want: true},
		{name: "terminator", args: []string{"version", "--format", "table", "--", "--format", "json"}, want: false},
		{name: "missing value", args: []string{"version", "--format"}, want: false},
	} {
		t.Run("presentation/"+tc.name, func(t *testing.T) {
			if got := processArgsRequestJSON(tc.args); got != tc.want {
				t.Fatalf("processArgsRequestJSON(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}

	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(envDWSAgentHost, "")
	t.Setenv(agentproduct.EnvName, "")
	t.Setenv(envDWSAgentVersion, "")
	sensitiveRaw := "{\"umt\":\"must-not-leak\"}\n"
	t.Setenv(envDWSAgentExt, sensitiveRaw)
	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	extensionHookCalls := 0
	edition.Override(&edition.Hooks{
		Name: "presentation-test",
		RegisterExtraCommands: func(*cobra.Command, edition.ToolCaller) {
			extensionHookCalls++
		},
		VisibleProducts: func() []string {
			extensionHookCalls++
			return nil
		},
		StaticServers: func() []edition.ServerInfo {
			extensionHookCalls++
			return nil
		},
	})

	oldArgs := os.Args
	os.Args = []string{"dws", "version"}
	t.Cleanup(func() { os.Args = oldArgs })

	rootConstructed := false
	preParseCalled := false
	testseam.Swap(t, &rootNewRootCommandWithEngine, func(context.Context, *pipeline.Engine) *cobra.Command {
		rootConstructed = true
		return &cobra.Command{Use: "dws"}
	})
	testseam.Swap(t, &rootRunPreParse, func(*cobra.Command, *pipeline.Engine) error {
		preParseCalled = true
		return nil
	})

	stderrFile, err := os.CreateTemp(t.TempDir(), "agent-metadata-stderr-*")
	if err != nil {
		t.Fatalf("create stderr capture: %v", err)
	}
	oldStderr := os.Stderr
	os.Stderr = stderrFile
	t.Cleanup(func() {
		os.Stderr = oldStderr
		_ = stderrFile.Close()
	})

	if code := Execute(); code == 0 {
		t.Fatal("process entry accepted invalid Agent metadata")
	}
	if rootConstructed || preParseCalled {
		t.Fatalf("invalid Agent metadata reached root hooks: constructed=%v preParse=%v", rootConstructed, preParseCalled)
	}
	if extensionHookCalls != 0 {
		t.Fatalf("invalid Agent metadata executed %d extension hooks", extensionHookCalls)
	}
	if err := stderrFile.Sync(); err != nil {
		t.Fatalf("sync stderr capture: %v", err)
	}
	stderrOutput, err := os.ReadFile(stderrFile.Name())
	if err != nil {
		t.Fatalf("read stderr capture: %v", err)
	}
	if strings.Contains(string(stderrOutput), "must-not-leak") || strings.Contains(string(stderrOutput), sensitiveRaw) {
		t.Fatalf("process validation error leaked raw EXT: %q", stderrOutput)
	}
	if !json.Valid(stderrOutput) || !strings.Contains(string(stderrOutput), `"reason": "invalid_agent_ext"`) {
		t.Fatalf("default JSON error presentation = %q", stderrOutput)
	}

	stdoutFile, err := os.CreateTemp(t.TempDir(), "agent-metadata-stdout-*")
	if err != nil {
		t.Fatalf("create stdout capture: %v", err)
	}
	oldStdout := os.Stdout
	os.Stdout = stdoutFile
	t.Cleanup(func() {
		os.Stdout = oldStdout
		_ = stdoutFile.Close()
	})
	emitEarlyAgentMetadataValidationError(invalidAgentExtError(), []string{"drive", "+list", "--format", "json"})
	if err := stdoutFile.Sync(); err != nil {
		t.Fatalf("sync stdout capture: %v", err)
	}
	unifiedOutput, err := os.ReadFile(stdoutFile.Name())
	if err != nil {
		t.Fatalf("read stdout capture: %v", err)
	}
	if !json.Valid(unifiedOutput) || !strings.Contains(string(unifiedOutput), `"outcome": "failure"`) ||
		!strings.Contains(string(unifiedOutput), `"subtype": "invalid_agent_ext"`) {
		t.Fatalf("unified JSON error presentation = %q", unifiedOutput)
	}
	if extensionHookCalls != 0 {
		t.Fatalf("presentation-only root executed %d extension hooks", extensionHookCalls)
	}

	if err := stderrFile.Truncate(0); err != nil {
		t.Fatalf("truncate fallback stderr capture: %v", err)
	}
	if _, err := stderrFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("rewind fallback stderr capture: %v", err)
	}
	testseam.Swap(t, &rootEmitResult, func(*cobra.Command, outputpkg.CommandResult) (int, error) {
		return 0, errors.New("injected result emission failure")
	})
	emitEarlyAgentMetadataValidationError(invalidAgentExtError(), []string{"drive", "+list", "--format", "json"})
	if err := stderrFile.Sync(); err != nil {
		t.Fatalf("sync fallback stderr capture: %v", err)
	}
	fallbackOutput, err := os.ReadFile(stderrFile.Name())
	if err != nil {
		t.Fatalf("read fallback stderr capture: %v", err)
	}
	if !json.Valid(fallbackOutput) || !strings.Contains(string(fallbackOutput), `"reason": "invalid_agent_ext"`) ||
		strings.Contains(string(fallbackOutput), "must-not-leak") {
		t.Fatalf("fallback validation error presentation = %q", fallbackOutput)
	}

	if err := stderrFile.Truncate(0); err != nil {
		t.Fatalf("truncate stderr capture: %v", err)
	}
	if _, err := stderrFile.Seek(0, io.SeekStart); err != nil {
		t.Fatalf("rewind stderr capture: %v", err)
	}
	emitEarlyAgentMetadataValidationError(invalidAgentExtError(), []string{"version", "--format", "table"})
	if err := stderrFile.Sync(); err != nil {
		t.Fatalf("sync human stderr capture: %v", err)
	}
	humanOutput, err := os.ReadFile(stderrFile.Name())
	if err != nil {
		t.Fatalf("read human stderr capture: %v", err)
	}
	if json.Valid(humanOutput) || !strings.Contains(string(humanOutput), "DWS_AGENT_EXT") ||
		strings.Contains(string(humanOutput), "must-not-leak") {
		t.Fatalf("human validation error presentation = %q", humanOutput)
	}

	var capturedRunner *runtimeRunner
	testseam.Swap(t, &rootNewCommandRunnerWithFlags, func(flags *GlobalFlags) executor.Runner {
		capturedRunner = newCommandRunnerWithFlags(flags).(*runtimeRunner)
		return capturedRunner
	})
	cachedSnapshot := agentMetadataSnapshot{version: "9.8.7", ext: `{"ua":"cached"}`}
	_ = newRootCommandWithMode(
		contextWithAgentMetadataSnapshot(context.Background(), cachedSnapshot),
		nil,
		false,
		true,
		true,
	)
	if capturedRunner == nil || capturedRunner.agentMetadata == nil || *capturedRunner.agentMetadata != cachedSnapshot {
		t.Fatalf("root runner Agent metadata = %#v, want %#v", capturedRunner, cachedSnapshot)
	}
}

func TestCrossPlatformCoverageAgentMetadataExcludedFromServiceDiscovery(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(envDWSAgentHost, "")
	t.Setenv(agentproduct.EnvName, "")
	t.Setenv(envDWSAgentVersion, "3.0.0")
	t.Setenv(envDWSAgentExt, `{"umt":"test-value"}`)

	headers := resolveMCPRequestHeadersForInvocation(executor.Invocation{
		CanonicalProduct: mcpMetaServerID,
		Tool:             mcpMetaURLTool,
	})
	if hasHeaderFold(headers, transport.HeaderAgentVersion) || hasHeaderFold(headers, transport.HeaderAgentExt) {
		t.Fatalf("service-discovery request leaked Agent metadata: %#v", headers)
	}

	headers = resolveMCPRequestHeadersForInvocation(executor.Invocation{CanonicalProduct: "doc", Tool: "read"})
	if headers[transport.HeaderAgentVersion] != "3.0.0" || headers[transport.HeaderAgentExt] == "" {
		t.Fatalf("ordinary MCP request omitted Agent metadata: %#v", headers)
	}
	cached := agentMetadataSnapshot{version: "3.1.0", ext: "{}"}
	headers = resolveMCPRequestHeadersForInvocation(executor.Invocation{CanonicalProduct: "doc", Tool: "read"}, cached)
	if headers[transport.HeaderAgentVersion] != "3.1.0" || headers[transport.HeaderAgentExt] != "{}" {
		t.Fatalf("ordinary MCP request ignored its validated snapshot: %#v", headers)
	}
}

func TestCrossPlatformCoverageAgentMetadataMCPAndPluginScoping(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv(envDWSAgentHost, "")
	t.Setenv(agentproduct.EnvName, "")
	t.Setenv(envDWSAgentVersion, "2.0.0")
	t.Setenv(envDWSAgentExt, `{"ua":"test-agent/2.0"}`)
	testseam.Swap(t, &runtimeContextResolve, func() runtimecontext.Result {
		return runtimecontext.ReadyResultForTest("runtime-context-value")
	})

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	edition.Override(&edition.Hooks{})

	pluginAuthMu.Lock()
	oldPluginRegistry := pluginAuthRegistry
	pluginAuthRegistry = make(map[string]*PluginAuth)
	pluginAuthMu.Unlock()
	t.Cleanup(func() {
		pluginAuthMu.Lock()
		pluginAuthRegistry = oldPluginRegistry
		pluginAuthMu.Unlock()
	})
	dynamicMu.Lock()
	oldDynamicEndpoints := dynamicEndpoints
	oldDynamicProducts := dynamicProducts
	oldDynamicAliases := dynamicAliases
	oldDynamicToolEndpoints := dynamicToolEndpoints
	dynamicEndpoints = nil
	dynamicProducts = nil
	dynamicAliases = nil
	dynamicToolEndpoints = nil
	dynamicMu.Unlock()
	t.Cleanup(func() {
		dynamicMu.Lock()
		dynamicEndpoints = oldDynamicEndpoints
		dynamicProducts = oldDynamicProducts
		dynamicAliases = oldDynamicAliases
		dynamicToolEndpoints = oldDynamicToolEndpoints
		dynamicMu.Unlock()
	})

	testseam.Swap(t, &runnerPreflightDocDownload, func(*runtimeRunner, context.Context, *transport.Client, string, executor.Invocation) error {
		return nil
	})
	type capturedRequest struct {
		headers map[string]string
		token   string
	}
	var captured []capturedRequest
	testseam.Swap(t, &runnerCallTool, func(client *transport.Client, _ context.Context, _, _ string, _ map[string]any) (transport.ToolCallResult, error) {
		copyHeaders := make(map[string]string, len(client.ExtraHeaders))
		for key, value := range client.ExtraHeaders {
			copyHeaders[key] = value
		}
		captured = append(captured, capturedRequest{headers: copyHeaders, token: client.AuthToken})
		return transport.ToolCallResult{Content: map[string]any{"value": "ok"}}, nil
	})

	created := newCommandRunnerWithFlags(&GlobalFlags{}).(*runtimeRunner)
	if hasHeaderFold(created.transport.ExtraHeaders, transport.HeaderAgentVersion) ||
		hasHeaderFold(created.transport.ExtraHeaders, transport.HeaderAgentExt) {
		t.Fatalf("new runner resolved Agent metadata before invocation validation: %#v", created.transport.ExtraHeaders)
	}

	// runSingle must not cache ambient MCP metadata on the shared base transport.
	// Use mock mode to exercise the path without authentication or network I/O.
	t.Setenv(envDWSAgentVersion, "2.0.1")
	refreshRunner := &runtimeRunner{
		transport:   transport.NewClient(nil),
		globalFlags: &GlobalFlags{Mock: true},
		auditSink:   audit.NopSink{},
	}
	refreshInvocation := executor.Invocation{CanonicalProduct: "refresh", Tool: "tool", Params: map[string]any{}}
	if _, err := refreshRunner.runSingle(context.Background(), refreshInvocation, false); err != nil {
		t.Fatalf("mock runSingle failed: %v", err)
	}
	if hasHeaderFold(refreshRunner.transport.ExtraHeaders, transport.HeaderAgentVersion) {
		t.Fatalf("runSingle mutated the shared transport Header map: %#v", refreshRunner.transport.ExtraHeaders)
	}
	t.Setenv(envDWSAgentVersion, "2.0.0")

	r := &runtimeRunner{
		transport:   transport.NewClient(nil),
		globalFlags: &GlobalFlags{Token: "test-token"},
		auditSink:   audit.NopSink{},
		agentMetadata: &agentMetadataSnapshot{
			version: "2.0.0",
			ext:     `{"ua":"test-agent/2.0"}`,
		},
	}
	builtIn := executor.Invocation{CanonicalProduct: "built-in", Tool: "tool", Params: map[string]any{}}
	if _, err := r.executeInvocation(context.Background(), "https://example.test", builtIn); err != nil {
		t.Fatalf("built-in invocation failed: %v", err)
	}

	pluginDescriptor := mcptypes.ServerDescriptor{
		Key:      "third-party",
		Endpoint: "https://plugin.example.test",
		CLI:      mcptypes.CLIOverlay{ID: "third-party"},
		AuthHeaders: map[string]string{
			"X-Plugin":        "yes",
			"X-Dws-Agent-Ver": "plugin-must-not-forge-version",
			"X-Dws-Agent-Ext": `{"source":"plugin"}`,
			"X-DingTalk-Ext":  `{"umid":"plugin-must-not-forge-runtime"}`,
		},
	}
	registerPluginHTTPServer(pluginDescriptor)
	registeredPlugin, pluginOwned := LookupPluginAuth("third-party")
	if !pluginOwned || registeredPlugin == nil || registeredPlugin.Token != "" {
		t.Fatalf("anonymous HTTP plugin ownership = %#v, %v", registeredPlugin, pluginOwned)
	}
	registerPluginHTTPServer(mcptypes.ServerDescriptor{
		Key:      "anonymous-empty",
		Endpoint: "https://anonymous.example.test",
		CLI:      mcptypes.CLIOverlay{ID: "anonymous-empty"},
	})
	if emptyPlugin, owned := LookupPluginAuth("anonymous-empty"); !owned || emptyPlugin == nil || emptyPlugin.Token != "" || len(emptyPlugin.ExtraHeaders) != 0 {
		t.Fatalf("headerless HTTP plugin ownership = %#v, %v", emptyPlugin, owned)
	}
	originalPluginHeaders := maps.Clone(registeredPlugin.ExtraHeaders)
	pluginInvocation := executor.Invocation{CanonicalProduct: "third-party", Tool: "tool", Params: map[string]any{}}
	if _, err := r.executeInvocation(context.Background(), "https://plugin.example.test", pluginInvocation); err != nil {
		t.Fatalf("plugin invocation failed: %v", err)
	}

	if len(captured) != 2 {
		t.Fatalf("captured %d calls, want 2", len(captured))
	}
	if captured[0].headers[transport.HeaderAgentVersion] != "2.0.0" || captured[0].headers[transport.HeaderAgentExt] != `{"ua":"test-agent/2.0"}` {
		t.Fatalf("built-in MCP metadata = %#v", captured[0].headers)
	}
	if captured[0].headers[runtimecontext.HeaderName] != `{"umid":"runtime-context-value"}` {
		t.Fatalf("built-in runtime context = %#v", captured[0].headers)
	}
	if hasHeaderFold(captured[1].headers, transport.HeaderAgentVersion) || hasHeaderFold(captured[1].headers, transport.HeaderAgentExt) {
		t.Fatalf("plugin request leaked Agent metadata: %#v", captured[1].headers)
	}
	if hasHeaderFold(captured[1].headers, runtimecontext.HeaderName) {
		t.Fatalf("plugin request leaked runtime context: %#v", captured[1].headers)
	}
	if got := captured[1].headers["X-Plugin"]; got != "yes" {
		t.Fatalf("plugin-owned header = %q, want yes", got)
	}
	if captured[1].token != "" {
		t.Fatalf("anonymous plugin unexpectedly received default OAuth token")
	}
	if !maps.Equal(registeredPlugin.ExtraHeaders, originalPluginHeaders) {
		t.Fatalf("plugin Header sanitization mutated registry state: got %#v want %#v", registeredPlugin.ExtraHeaders, originalPluginHeaders)
	}
	if got := pluginRequestHeaders(nil); got != nil {
		t.Fatalf("nil plugin auth produced Headers: %#v", got)
	}
	if got := pluginRequestHeaders(&PluginAuth{ExtraHeaders: map[string]string{
		"X-DWS-AGENT-VER": "forged",
		"X-DWS-AGENT-EXT": `{"forged":true}`,
		"X-DINGTALK-EXT":  `{"umid":"forged"}`,
	}}); got != nil {
		t.Fatalf("reserved-only plugin Headers survived sanitization: %#v", got)
	}

	// Keep the execution-boundary auth guard independently testable: even if a
	// future token provider returns an empty token without an error, built-in MCP
	// calls must fail before preflight or transport while anonymous plugins remain
	// valid above.
	resolveCalled := false
	testseam.Swap(t, &runnerResolveAuthSnapshot, func(*runtimeRunner, context.Context) (AccessTokenSnapshot, error) {
		resolveCalled = true
		return AccessTokenSnapshot{}, nil
	})
	callsBefore := len(captured)
	unauthenticated := &runtimeRunner{
		transport:   transport.NewClient(nil),
		globalFlags: &GlobalFlags{},
		auditSink:   audit.NopSink{},
	}
	if _, err := unauthenticated.executeInvocation(context.Background(), "https://example.test", executor.Invocation{CanonicalProduct: "built-in-unauthenticated", Tool: "tool"}); err == nil || !isAuthError(err) {
		t.Fatalf("unauthenticated built-in request = %v, want auth error", err)
	}
	if !resolveCalled {
		t.Fatal("unauthenticated request did not exercise the token resolver")
	}
	if len(captured) != callsBefore {
		t.Fatalf("unauthenticated built-in request reached transport: calls %d -> %d", callsBefore, len(captured))
	}
}

func hasHeaderFold(headers map[string]string, want string) bool {
	for key := range headers {
		if strings.EqualFold(key, want) {
			return true
		}
	}
	return false
}
