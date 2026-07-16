package app

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/apiclient"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageAPIAndTimingRemainingCoverage(t *testing.T) {
	oldProvider := newAppTokenProvider
	oldClientID, oldClientSecret := apiClientID, apiClientSecret
	oldMarshal, oldMkdir := timingMarshalIndent, timingMkdirAll
	oldWrite, oldRemove, oldRename := timingWriteFile, timingRemove, timingRename
	oldRead, oldHome := timingReadFile, timingUserHomeDir
	t.Cleanup(func() {
		newAppTokenProvider = oldProvider
		apiClientID, apiClientSecret = oldClientID, oldClientSecret
		timingMarshalIndent, timingMkdirAll = oldMarshal, oldMkdir
		timingWriteFile, timingRemove, timingRename = oldWrite, oldRemove, oldRename
		timingReadFile, timingUserHomeDir = oldRead, oldHome
		authpkg.SetClientID("")
		authpkg.SetClientSecret("")
	})
	fail := errors.New("failure")
	apiClientID = func() string { return "" }
	apiClientSecret = func() string { return "" }
	if _, err := resolveRawAPIToken(context.Background(), ""); err == nil {
		t.Fatal("missing raw API credentials succeeded")
	}
	apiClientID = func() string { return "<placeholder>" }
	apiClientSecret = func() string { return "secret" }
	if _, err := resolveRawAPIToken(context.Background(), ""); err == nil {
		t.Fatal("placeholder raw API credentials succeeded")
	}
	apiClientID, apiClientSecret = authpkg.ClientID, authpkg.ClientSecret
	authpkg.SetClientID("app-key")
	authpkg.SetClientSecret("app-secret")
	for _, tc := range []struct {
		getter fakeAppTokenGetter
		want   string
	}{
		{getter: fakeAppTokenGetter{err: fail}, want: "failure"},
		{getter: fakeAppTokenGetter{token: " "}, want: "为空"},
		{getter: fakeAppTokenGetter{token: " token "}},
	} {
		newAppTokenProvider = func(string, string, string) appTokenGetter { return tc.getter }
		got, err := resolveRawAPIToken(context.Background(), "")
		if tc.want != "" && (err == nil || !containsText(err.Error(), tc.want)) {
			t.Fatalf("raw token error = %v, want %q", err, tc.want)
		}
		if tc.want == "" && (err != nil || got != "token") {
			t.Fatalf("raw token = %q, %v", got, err)
		}
	}
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	endpoint := server.URL
	server.Close()
	apiclient.AllowedHosts["127.0.0.1"] = true
	t.Cleanup(func() { delete(apiclient.AllowedHosts, "127.0.0.1") })
	cmd := &cobra.Command{Use: "api"}
	cmd.SetContext(context.Background())
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	if err := runAPI(cmd, []string{"GET", "/path"}, &GlobalFlags{Token: "token", Timeout: 1}, &apiFlags{baseURL: endpoint}); err == nil {
		t.Fatal("closed raw API endpoint succeeded")
	}

	collector := NewTimingCollector()
	collector.Print(io.Discard)
	t.Setenv(PerfReportEnv, t.TempDir()+"/report.json")
	timingMarshalIndent = func(any, string, string) ([]byte, error) { return nil, fail }
	collector.WriteReportIfEnabled("v", "cmd")
	timingMarshalIndent = oldMarshal
	timingUserHomeDir = func() (string, error) { return "", fail }
	t.Setenv(PerfReportEnv, "auto")
	collector.WriteReportIfEnabled("v", "cmd")
	if defaultPerfReportPath() != "" {
		t.Fatal("home-dir failure produced a report path")
	}
	t.Setenv(PerfReportEnv, t.TempDir()+"/report.json")
	timingMkdirAll = func(string, os.FileMode) error { return fail }
	collector.WriteReportIfEnabled("v", "cmd")
	timingMkdirAll = oldMkdir
	timingWriteFile = func(string, []byte, os.FileMode) error { return fail }
	removed := false
	timingRemove = func(string) error { removed = true; return nil }
	collector.WriteReportIfEnabled("v", "cmd")
	if !removed {
		t.Fatal("failed temporary report was not removed")
	}
	timingWriteFile = oldWrite
	timingRemove = oldRemove
	renamed := false
	timingRename = func(string, string) error { renamed = true; return fail }
	collector.WriteReportIfEnabled("v", "cmd")
	if !renamed {
		t.Fatal("report rename was not attempted")
	}
	timingReadFile = func(string) ([]byte, error) { return []byte("{"), nil }
	if _, err := LoadLatestReport(); err == nil {
		t.Fatal("malformed performance report succeeded")
	}
}

func TestCrossPlatformCoverageDirectRuntimeRemainingCoverage(t *testing.T) {
	oldEdition := edition.Get()
	t.Cleanup(func() {
		edition.Override(oldEdition)
		SetDynamicServers(nil)
	})
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	for _, raw := range []string{
		"not a url",
		"https://mcp.dingtalk.com/path?q=1#fragment",
		"https://pre-mcp.example.test:8443/path/",
		"https://mcp.example.test/path/",
	} {
		if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(raw), 0o600); err != nil {
			t.Fatal(err)
		}
		if got := defaultPATGatewayBaseURL(); got == "" {
			t.Fatalf("gateway for %q is blank", raw)
		}
	}

	endpoints := map[string]string{}
	products := map[string]bool{}
	aliases := map[string]string{}
	tools := map[string]string{}
	registerDynamicServer(mcptypes.ServerDescriptor{CLI: mcptypes.CLIOverlay{Skip: true}}, endpoints, products, aliases, tools)
	registerDynamicServer(mcptypes.ServerDescriptor{
		Endpoint: "https://server.test",
		CLI: mcptypes.CLIOverlay{
			ID: "id", Command: "command", Aliases: []string{"alias", " "},
			Tools:         []mcptypes.CLITool{{Name: "tool"}, {Name: " "}},
			ToolOverrides: map[string]mcptypes.CLIToolOverride{"override": {}, " ": {}},
		},
	}, endpoints, products, aliases, tools)
	if endpoints["command"] == "" || aliases["alias"] != "id" || tools["override"] == "" {
		t.Fatalf("registered dynamic server = %#v %#v %#v", endpoints, aliases, tools)
	}

	SetDynamicServers(nil)
	dynamicMu.Lock()
	dynamicEndpoints = map[string]string{}
	dynamicProducts = map[string]bool{}
	dynamicAliases = map[string]string{}
	dynamicToolEndpoints = map[string]string{}
	dynamicMu.Unlock()
	if got, ok := directRuntimeEndpoint(defaultPATProductID, ""); !ok || got == "" {
		t.Fatal("cold-start PAT fallback did not resolve")
	}
	SetDynamicServers(nil)
	if _, ok := directRuntimeEndpoint(" ", " "); ok {
		t.Fatal("blank runtime endpoint resolved")
	}
	t.Setenv("DINGTALK_CUSTOM_MCP_URL", "https://override.test")
	if got, ok := directRuntimeEndpoint("custom", ""); !ok || got != "https://override.test" {
		t.Fatalf("environment runtime endpoint = %q, %v", got, ok)
	}
	if got, ok := directRuntimeEndpoint(devappProductID, ""); !ok || got == "" {
		t.Fatal("devapp fallback did not resolve")
	}
	if got, ok := directRuntimeEndpoint(defaultPATProductID, ""); !ok || got == "" {
		t.Fatal("PAT fallback did not resolve")
	}
	edition.Override(&edition.Hooks{
		StaticServers: func() []edition.ServerInfo { return []edition.ServerInfo{{ID: "other", Endpoint: ""}} },
		SupplementServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{{ID: "other", Endpoint: "https://other.test", Prefixes: []string{" ", "wanted"}}}
		},
	})
	if got, ok := directRuntimeEndpoint("wanted", ""); !ok || got != "https://other.test" {
		t.Fatalf("edition runtime endpoint = %q, %v", got, ok)
	}

	dynamicMu.Lock()
	dynamicEndpoints, dynamicProducts, dynamicAliases, dynamicToolEndpoints = nil, nil, nil, nil
	dynamicMu.Unlock()
	AppendDynamicServer(mcptypes.ServerDescriptor{
		Endpoint: "https://append.test",
		CLI: mcptypes.CLIOverlay{
			ID: "append", Command: "append-command", Aliases: []string{"append-alias"},
			Tools: []mcptypes.CLITool{{Name: "append-tool"}},
			ToolOverrides: map[string]mcptypes.CLIToolOverride{
				"append-override": {}, "skip": {ServerOverride: "other"}, " ": {},
			},
		},
	})
	if got, ok := directRuntimeToolEndpoint("append-override"); !ok || got != "https://append.test" {
		t.Fatalf("append override endpoint = %q, %v", got, ok)
	}
}

func containsText(value, substring string) bool {
	for i := 0; i+len(substring) <= len(value); i++ {
		if value[i:i+len(substring)] == substring {
			return true
		}
	}
	return false
}

func TestCrossPlatformCoverageEmbeddedSkillAndTinyCommandsRemainingCoverage(t *testing.T) {
	oldStat, oldTemp, oldRemove := embeddedSkillStat, embeddedSkillMkdirTemp, embeddedSkillRemoveAll
	oldWalk, oldRead := embeddedSkillWalkDir, embeddedSkillReadFile
	oldMkdir, oldWrite := embeddedSkillMkdirAll, embeddedSkillWriteFile
	t.Cleanup(func() {
		embeddedSkillStat, embeddedSkillMkdirTemp, embeddedSkillRemoveAll = oldStat, oldTemp, oldRemove
		embeddedSkillWalkDir, embeddedSkillReadFile = oldWalk, oldRead
		embeddedSkillMkdirAll, embeddedSkillWriteFile = oldMkdir, oldWrite
	})
	fail := errors.New("failure")
	embeddedSkillStat = func(string) (os.FileInfo, error) { return nil, nil }
	embeddedSkillMkdirTemp = func(string, string) (string, error) { return "", fail }
	if _, _, err := materializeEmbeddedSkillSource("codex"); !errors.Is(err, fail) {
		t.Fatalf("embedded mkdir error = %v", err)
	}
	embeddedSkillMkdirTemp = func(string, string) (string, error) { return t.TempDir(), nil }
	removed := false
	embeddedSkillRemoveAll = func(string) error { removed = true; return nil }
	embeddedSkillWalkDir = func(_ string, fn fs.WalkDirFunc) error {
		return fn("entry", nil, fail)
	}
	if _, _, err := materializeEmbeddedSkillSource("codex"); !errors.Is(err, fail) || !removed {
		t.Fatalf("embedded walk error = %v, removed=%v", err, removed)
	}
	embeddedSkillWalkDir = func(_ string, fn fs.WalkDirFunc) error {
		return fn("skills/codex/file", fakeSkillDirEntry{}, nil)
	}
	embeddedSkillReadFile = func(string) ([]byte, error) { return nil, fail }
	if _, _, err := materializeEmbeddedSkillSource("codex"); !errors.Is(err, fail) {
		t.Fatalf("embedded read error = %v", err)
	}
	embeddedSkillReadFile = func(string) ([]byte, error) { return []byte("skill"), nil }
	embeddedSkillMkdirAll = func(string, os.FileMode) error { return fail }
	if _, _, err := materializeEmbeddedSkillSource("codex"); !errors.Is(err, fail) {
		t.Fatalf("embedded nested mkdir error = %v", err)
	}
	embeddedSkillWalkDir = func(_ string, fn fs.WalkDirFunc) error {
		return fn("skills/codex/dir", fakeSkillDirEntry{dir: true}, nil)
	}
	if _, _, err := materializeEmbeddedSkillSource("codex"); !errors.Is(err, fail) {
		t.Fatalf("embedded directory mkdir error = %v", err)
	}
	embeddedSkillWalkDir = func(_ string, fn fs.WalkDirFunc) error {
		return fn("skills/codex/file", fakeSkillDirEntry{}, nil)
	}
	embeddedSkillMkdirAll = func(string, os.FileMode) error { return nil }
	embeddedSkillWriteFile = func(string, []byte, os.FileMode) error { return fail }
	if _, _, err := materializeEmbeddedSkillSource("codex"); !errors.Is(err, fail) {
		t.Fatalf("embedded write error = %v", err)
	}

	merged := mergeTopLevelCommands([]*cobra.Command{nil, {}})
	if len(merged) != 0 {
		t.Fatalf("empty legacy commands = %#v", merged)
	}
	root := &cobra.Command{Use: "root"}
	completion := newCompletionCommand(root)
	if err := completion.RunE(completion, []string{"other"}); err != nil {
		t.Fatal(err)
	}
	catalog := newCatalogCommand(nil)
	catalog.SetOut(io.Discard)
	if err := catalog.RunE(catalog, nil); err != nil {
		t.Fatal(err)
	}
}
