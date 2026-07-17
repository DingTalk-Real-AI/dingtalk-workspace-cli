package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/plugin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type appFailWriter struct{ err error }

func (w appFailWriter) Write([]byte) (int, error) { return 0, w.err }

type fakeAccessTokenGetter struct {
	token string
	err   error
}

func (g fakeAccessTokenGetter) GetAccessToken(context.Context) (string, error) {
	return g.token, g.err
}

type fakeLegacyTokenGetter struct {
	token string
	err   error
}

func (g fakeLegacyTokenGetter) GetToken() (string, string, error) {
	return g.token, "test", g.err
}

type fakeAppTokenGetter struct {
	token string
	err   error
}

func (g fakeAppTokenGetter) GetToken(context.Context) (string, error) { return g.token, g.err }

type fakeSkillDirEntry struct{ dir bool }

func (e fakeSkillDirEntry) Name() string               { return "entry" }
func (e fakeSkillDirEntry) IsDir() bool                { return e.dir }
func (e fakeSkillDirEntry) Type() os.FileMode          { return 0 }
func (e fakeSkillDirEntry) Info() (os.FileInfo, error) { return nil, nil }

func docPreflightServer(t *testing.T, result map[string]any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID int `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"jsonrpc": "2.0",
			"id":      request.ID,
			"result":  result,
		})
	}))
}

func TestCrossPlatformCoverageDocDownloadPreflightCoverage(t *testing.T) {
	runner := &runtimeRunner{}
	base := executor.Invocation{CanonicalProduct: "doc", Tool: "download_file", Params: map[string]any{"nodeId": " node "}}
	if err := runner.preflightDocDownload(context.Background(), transport.NewClient(nil), "", executor.Invocation{}); err != nil {
		t.Fatal(err)
	}
	if err := runner.preflightDocDownload(context.Background(), transport.NewClient(nil), "", executor.Invocation{CanonicalProduct: "DOC", Tool: "download_file"}); err != nil {
		t.Fatal(err)
	}
	if !isDocDownloadInvocation(base) || docDownloadNodeID(map[string]any{"node": " n "}) != "n" || docDownloadNodeID(map[string]any{"dentryUuid": " d "}) != "d" || docDownloadNodeID(map[string]any{"nodeId": 1}) != "" {
		t.Fatal("doc download invocation helpers returned unexpected values")
	}
	if documentInfoExtension(map[string]any{"data": map[string]any{"extension": " doc "}}) != "doc" ||
		documentInfoExtension(map[string]any{"extension": " pdf "}) != "pdf" ||
		stringAtPath(map[string]any{"x": "value"}, "x", "nested") != "" ||
		stringAtPath(map[string]any{"x": 1}, "x") != "" {
		t.Fatal("document extension helpers returned unexpected values")
	}
	if unsupportedAXLSDownloadError() == nil {
		t.Fatal("missing AXLS validation error")
	}

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	hookErr := errors.New("classified")
	for _, tc := range []struct {
		name   string
		result map[string]any
		hooks  *edition.Hooks
		want   string
	}{
		{name: "ok", result: map[string]any{"content": map[string]any{"result": map[string]any{"extension": "docx"}}}, hooks: &edition.Hooks{}},
		{name: "edition classifier", result: map[string]any{"content": map[string]any{}}, hooks: &edition.Hooks{ClassifyToolResult: func(map[string]any) error { return hookErr }}, want: "classified"},
		{name: "pat", result: map[string]any{"content": map[string]any{"errorCode": "PAT_NO_PERMISSION"}}, hooks: &edition.Hooks{}, want: "PAT_NO_PERMISSION"},
		{name: "mcp error", result: map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": "mcp failed"}}}, hooks: &edition.Hooks{}, want: "mcp failed"},
		{name: "business error", result: map[string]any{"content": map[string]any{"success": false, "errorMsg": "business failed"}}, hooks: &edition.Hooks{}, want: "business failed"},
		{name: "axls", result: map[string]any{"content": map[string]any{"data": map[string]any{"extension": "AXLS"}}}, hooks: &edition.Hooks{}, want: "extension=axls"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			edition.Override(tc.hooks)
			server := docPreflightServer(t, tc.result)
			defer server.Close()
			client := transport.NewClient(nil)
			client.TrustedDomains = []string{"127.0.0.1"}
			err := runner.preflightDocDownload(context.Background(), client, server.URL, base)
			if tc.want == "" && err != nil {
				t.Fatalf("preflight error = %v", err)
			}
			if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
				t.Fatalf("preflight error = %v, want %q", err, tc.want)
			}
		})
	}
	server := docPreflightServer(t, map[string]any{})
	endpoint := server.URL
	server.Close()
	client := transport.NewClient(nil)
	client.TrustedDomains = []string{"127.0.0.1"}
	client.MaxRetries = 0
	if err := runner.preflightDocDownload(context.Background(), client, endpoint, base); err == nil {
		t.Fatal("network preflight failure succeeded")
	}
}

func TestCrossPlatformCoverageRootHelpRemainingCoverage(t *testing.T) {
	configureRootHelp(nil)
	renderRootGlobalFlags(nil)
	if visiblePersistentFlags(nil) != nil || formatRootFlag(nil) != "" || commandShort(nil) != "" || visibleMCPRootCommands(nil) != nil || visibleUtilityRootCommands(nil) != nil {
		t.Fatal("nil root helper contract changed")
	}

	oldEdition := edition.Get()
	edition.Override(&edition.Hooks{VisibleProducts: func() []string { return []string{"service"} }})
	t.Cleanup(func() { edition.Override(oldEdition); SetDynamicServers(nil) })
	root := &cobra.Command{Use: "root", Long: "long help"}
	root.SetOut(io.Discard)
	root.PersistentFlags().StringP("value", "x", "", "value")
	root.PersistentFlags().Bool("hidden", false, "hidden")
	_ = root.PersistentFlags().MarkHidden("hidden")
	root.AddCommand(&cobra.Command{Use: "service", Short: "service"}, &cobra.Command{Use: "utility", Short: "utility"})
	configureRootHelp(root)
	if err := root.Commands()[0].Help(); err != nil {
		t.Fatal(err)
	}
	root.SetArgs([]string{"help", "missing"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	root.SetArgs([]string{"help", "utility"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if got := formatRootFlag(root.PersistentFlags().Lookup("value")); !strings.Contains(got, "-x") {
		t.Fatalf("formatted flag = %q", got)
	}
	if got := formatRootFlag(root.PersistentFlags().Lookup("hidden")); !strings.Contains(got, "--hidden") {
		t.Fatalf("formatted long flag = %q", got)
	}
	_ = commandShort(&cobra.Command{Use: "help", Short: "Help about any command"})
	renderRootGlobalFlags(&cobra.Command{Use: "no-flags"})
	edition.Override(&edition.Hooks{})
	SetDynamicServers(nil)
	_ = visibleMCPRootCommands(root)
}

func TestCrossPlatformCoverageConfigAndTokenSeamsCoverage(t *testing.T) {
	oldHome, oldExe, oldEval := userHomeDir, executablePath, evaluateSymlink
	oldEdition := edition.Get()
	t.Cleanup(func() {
		userHomeDir, executablePath, evaluateSymlink = oldHome, oldExe, oldEval
		edition.Override(oldEdition)
	})
	t.Setenv("DWS_CONFIG_DIR", "")
	edition.Override(&edition.Hooks{})
	userHomeDir = func() (string, error) { return "", errors.New("home") }
	executablePath = func() (string, error) { return "", errors.New("exe") }
	if got := defaultConfigDir(); got != ".dws" {
		t.Fatalf("fallback config dir = %q", got)
	}
	executablePath = func() (string, error) { return filepath.Join("", "tmp", "dws"), nil }
	evaluateSymlink = func(string) (string, error) { return "", errors.New("link") }
	if got := exeRelativeConfigDir(); !strings.HasSuffix(got, filepath.Join("tmp", ".dws")) {
		t.Fatalf("executable config dir = %q", got)
	}
	userHomeDir = func() (string, error) { return "/home/test", nil }
	if got := defaultConfigDir(); got != filepath.Join("/home/test", ".dws") {
		t.Fatalf("home config dir = %q", got)
	}
	edition.Override(&edition.Hooks{ConfigDir: func() string { return "/edition" }})
	if got := defaultConfigDir(); got != "/edition" {
		t.Fatalf("edition config dir = %q", got)
	}

	oldProvider, oldManager := newAccessTokenProvider, newLegacyTokenManager
	t.Cleanup(func() { newAccessTokenProvider, newLegacyTokenManager = oldProvider, oldManager })
	newAccessTokenProvider = func(string) accessTokenGetter { return fakeAccessTokenGetter{err: authpkg.ErrTokenDecryption} }
	if _, err := resolveAccessTokenFromDir(context.Background(), "unused"); !errors.Is(err, authpkg.ErrTokenDecryption) {
		t.Fatalf("decryption error = %v", err)
	}
	newAccessTokenProvider = func(string) accessTokenGetter { return fakeAccessTokenGetter{err: errors.New("missing")} }
	newLegacyTokenManager = func(string) legacyTokenGetter { return fakeLegacyTokenGetter{token: " legacy "} }
	if got, err := resolveAccessTokenFromDir(context.Background(), "unused"); err != nil || got != "legacy" {
		t.Fatalf("legacy token = %q, %v", got, err)
	}
	authpkg.SetRuntimeProfile("corp:user")
	t.Cleanup(func() { authpkg.SetRuntimeProfile("") })
	if got, err := resolveAccessTokenFromDir(context.Background(), "unused"); got != "" || err == nil || err.Error() != "missing" {
		t.Fatalf("explicit profile fallback = token %q error %v, want profile error", got, err)
	}
	authpkg.SetRuntimeProfile("")
	newAccessTokenProvider = func(string) accessTokenGetter { return fakeAccessTokenGetter{err: errors.New("load")} }
	newLegacyTokenManager = func(string) legacyTokenGetter { return fakeLegacyTokenGetter{err: errors.New("missing")} }
	edition.Override(&edition.Hooks{})
	other := filepath.Join(t.TempDir(), "other")
	if _, err := ResolveAuxiliaryAccessToken(context.Background(), other, ""); err == nil {
		t.Fatal("auxiliary provider failure succeeded")
	}
	newAccessTokenProvider = func(string) accessTokenGetter { return fakeAccessTokenGetter{err: authpkg.ErrTokenDecryption} }
	if _, err := ResolveAuxiliaryAccessToken(context.Background(), other, ""); !errors.Is(err, authpkg.ErrTokenDecryption) {
		t.Fatalf("auxiliary decryption error = %v", err)
	}
	t.Setenv("DWS_CONFIG_DIR", other)
	ResetRuntimeTokenCache()
	if _, err := ResolveAuxiliaryAccessToken(context.Background(), other, ""); err == nil {
		t.Fatal("current config without credentials succeeded")
	}
	edition.Override(&edition.Hooks{IsEmbedded: true})
	if !strings.Contains(noCredentialsError().Error(), "认证") {
		t.Fatal("embedded credentials error changed")
	}
}

func TestCrossPlatformCoverageForceRefreshAndStdioFailureCoverage(t *testing.T) {
	oldMark, oldFactory := markAccessTokenStale, newRefreshProvider
	oldStop := stopStdio
	t.Cleanup(func() {
		markAccessTokenStale, newRefreshProvider = oldMark, oldFactory
		stopStdio = oldStop
		stdioMu.Lock()
		stdioClients = make(map[string]*transport.StdioClient)
		stdioMu.Unlock()
	})
	fail := errors.New("failure")
	_ = oldFactory(t.TempDir())
	markAccessTokenStale = func(string) error { return fail }
	if _, err := ForceRefreshAccessToken(context.Background(), "config"); !errors.Is(err, fail) {
		t.Fatalf("mark stale error = %v", err)
	}
	markAccessTokenStale = func(string) error { return nil }
	for _, tc := range []struct {
		getter fakeAccessTokenGetter
		want   string
	}{
		{getter: fakeAccessTokenGetter{err: fail}, want: "failure"},
		{getter: fakeAccessTokenGetter{token: "  "}, want: "empty"},
		{getter: fakeAccessTokenGetter{token: " refreshed "}},
	} {
		newRefreshProvider = func(string) accessTokenGetter { return tc.getter }
		got, err := ForceRefreshAccessToken(context.Background(), "config")
		if tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
			t.Fatalf("refresh error = %v, want %q", err, tc.want)
		}
		if tc.want == "" && (err != nil || got != "refreshed") {
			t.Fatalf("refreshed token = %q, %v", got, err)
		}
	}

	stopStdio = func(*transport.StdioClient) error { return fail }
	RegisterStdioClient("all", transport.NewStdioClient("unused", nil, nil))
	StopAllStdioClients()
	RegisterStdioClient("one", transport.NewStdioClient("unused", nil, nil))
	if !StopStdioClient("one") {
		t.Fatal("registered stdio client not stopped")
	}
	RegisterStdioClient("plugin/server", transport.NewStdioClient("unused", nil, nil))
	if got := StopStdioClientsByPlugin("plugin"); got != 1 {
		t.Fatalf("stopped plugin clients = %d", got)
	}
}

func TestCrossPlatformCoverageOverlayRecoveryHostAndHelperRemainingCoverage(t *testing.T) {
	root := t.TempDir()
	writeOverlay := filepath.Join(root, "overlay.json")
	if err := os.WriteFile(writeOverlay, []byte(`{"toolOverrides":{"tool":{}}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []json.RawMessage{
		json.RawMessage(`"missing.json"`),
		json.RawMessage(`"unterminated`),
		json.RawMessage(`{`),
		json.RawMessage(`"overlay.json"`),
		json.RawMessage(`{"id":"","command":"","toolOverrides":{"tool":{}}}`),
	} {
		p := &plugin.Plugin{Root: root, Manifest: plugin.Manifest{Name: "plugin", Description: "description", MCPServers: map[string]*plugin.MCPServer{"server": {CLI: raw}}}}
		overlay := resolveStdioOverlay(p, plugin.StdioServerClient{Key: "server", Client: transport.NewStdioClient("unused", nil, nil)})
		if overlay.ID == "" || overlay.Command == "" {
			t.Fatalf("overlay defaults missing: %#v", overlay)
		}
	}
	p := &plugin.Plugin{Root: root, Manifest: plugin.Manifest{Name: "plugin", Description: "description", MCPServers: map[string]*plugin.MCPServer{"server": {CLI: json.RawMessage(`{}`)}}}}
	if descriptor := registerStdioServerFromManifest(p, plugin.StdioServerClient{Key: "server"}); descriptor.Endpoint == "" {
		t.Fatalf("empty overlay descriptor = %#v", descriptor)
	}
	p.Manifest.MCPServers["server"].CLI = json.RawMessage(`{"toolOverrides":{"tool":{}}}`)
	if descriptor := registerStdioServerFromManifest(p, plugin.StdioServerClient{Key: "server", Client: transport.NewStdioClient("unused", nil, nil)}); descriptor.Endpoint == "" {
		t.Fatalf("stdio overlay registration = %#v", descriptor)
	}

	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition); SetDynamicServers(nil) })
	edition.Override(&edition.Hooks{ConfigDir: func() string { return "" }})
	captureRuntimeFailure(executor.Invocation{}, nil, nil)
	captureRuntimeFailure(executor.Invocation{}, errors.New("raw"), nil)
	oldArgs := os.Args
	os.Args = []string{"dws", "doc", "download", "--node", "n"}
	if got := runtimeCommandPath(executor.Invocation{}); len(got) != 2 {
		t.Fatalf("runtime command path = %#v", got)
	}
	os.Args = oldArgs

	t.Setenv(authpkg.AgentCodeEnv, "")
	if hostControlProviderFromEnv() != "" {
		t.Fatal("host control enabled without agent code")
	}
	t.Setenv(authpkg.AgentCodeEnv, "agent")
	edition.Override(&edition.Hooks{MergeHeaders: func(headers map[string]string) map[string]string { return headers }})
	if got := hostControlProviderFromEnv(); got != edition.DefaultOSSClawType {
		t.Fatalf("default claw type = %q", got)
	}
	edition.Override(&edition.Hooks{MergeHeaders: func(map[string]string) map[string]string { return map[string]string{"claw-type": "custom"} }})
	if got := effectiveClawType(); got != "custom" {
		t.Fatalf("custom claw type = %q", got)
	}
}

func TestCrossPlatformCoverageConfigAndCacheCommandRemainingCoverage(t *testing.T) {
	for _, command := range []*cobra.Command{newConfigCommand(), newCacheCommand()} {
		command.SetOut(io.Discard)
		if err := command.RunE(command, nil); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("DWS_CONFIG_DIR", "configured")
	configCmd := &cobra.Command{Use: "config"}
	var configOut bytes.Buffer
	configCmd.SetOut(&configOut)
	if err := writeConfigJSON(configCmd, filterVisible(nil), true); err != nil {
		t.Fatal(err)
	}
	list := newConfigListCommand()
	list.SetOut(io.Discard)
	_ = list.Flags().Set("category", "core")
	_ = list.Flags().Set("show-values", "true")
	_ = list.Flags().Set("show-hidden", "true")
	_ = list.Flags().Set("json", "true")
	if err := runConfigList(list, nil); err != nil {
		t.Fatal(err)
	}

	cacheRoot := &cobra.Command{Use: "root"}
	cacheRoot.PersistentFlags().String("format", "", "")
	cacheCmd := &cobra.Command{Use: "cache"}
	cacheRoot.AddCommand(cacheCmd)
	for _, format := range []string{"json", "pretty", "table"} {
		_ = cacheRoot.PersistentFlags().Set("format", format)
		cacheCmd.SetOut(io.Discard)
		if err := printCacheCompatNotice(cacheCmd, "status"); err != nil {
			t.Fatal(err)
		}
	}
	fail := errors.New("write")
	cacheCmd.SetOut(appFailWriter{err: fail})
	_ = cacheRoot.PersistentFlags().Set("format", "pretty")
	if err := printCacheCompatNotice(cacheCmd, "status"); !errors.Is(err, fail) {
		t.Fatalf("pretty write error = %v", err)
	}
	_ = cacheRoot.PersistentFlags().Set("format", "table")
	if err := printCacheCompatNotice(cacheCmd, "status"); !errors.Is(err, fail) {
		t.Fatalf("table write error = %v", err)
	}
}
