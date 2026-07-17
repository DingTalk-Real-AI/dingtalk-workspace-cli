package app

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
	eventbus "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/bus"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/busctl"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/consume"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/personal"
	eventtransport "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pat"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/plugin"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/recovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/safety"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	upgradepkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/upgrade"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
)

type coverageRunner struct {
	result executor.Result
	err    error
	last   executor.Invocation
}

func (r *coverageRunner) Run(_ context.Context, inv executor.Invocation) (executor.Result, error) {
	r.last = inv
	return r.result, r.err
}

type coverageScanner struct{ report safety.Report }

func (s coverageScanner) ScanPayload(any) safety.Report { return s.report }

type coverageRawStderr string

func (e coverageRawStderr) Error() string     { return string(e) }
func (e coverageRawStderr) RawStderr() string { return string(e) }

func TestCrossPlatformCoverageToolCallerAdapterCoverage(t *testing.T) {
	runner := &coverageRunner{}
	adapter := newToolCallerAdapter(runner, nil).(*toolCallerAdapter)
	if adapter.Format() != "json" || adapter.DryRun() || adapter.Fields() != "" || adapter.JQ() != "" {
		t.Fatal("nil flag adapter defaults changed")
	}
	adapter.flags = &GlobalFlags{Format: "raw", DryRun: true, Fields: "a", JQ: ".a"}
	if adapter.Format() != "raw" || !adapter.DryRun() || adapter.Fields() != "a" || adapter.JQ() != ".a" {
		t.Fatal("adapter flags were not forwarded")
	}
	adapter.flags.DryRun = false
	if _, err := (*toolCallerAdapter)(nil).CallToolWithToken(context.Background(), "token", "doc", "get", nil); err == nil {
		t.Fatal("nil adapter token override succeeded")
	}
	authpkg.SetRuntimeProfile("saved-profile")
	adapter.flags.Token = "saved-token"
	runner.result = executor.Result{Response: map[string]any{"content": []any{map[string]any{"type": "text", "text": "ok"}}}}
	if _, err := adapter.CallToolWithToken(context.Background(), "temporary-token", "doc", "get", nil); err != nil {
		t.Fatal(err)
	}
	if adapter.flags.Token != "saved-token" || authpkg.RuntimeProfile() != "saved-profile" {
		t.Fatal("token override state was not restored")
	}
	authpkg.SetRuntimeProfile("")

	runner.err = errors.New("runner failure")
	if _, err := adapter.CallTool(context.Background(), "doc", "get", map[string]any{"x": 1}); !errors.Is(err, runner.err) {
		t.Fatalf("adapter runner error = %v", err)
	}
	runner.err = nil
	for name, response := range map[string]map[string]any{
		"nil":       nil,
		"whole":     {"value": 1},
		"list":      {"content": []any{map[string]any{"type": "text", "text": "hello"}, "skip"}},
		"map":       {"content": map[string]any{"value": 2}},
		"primitive": {"content": 3},
	} {
		t.Run(name, func(t *testing.T) {
			runner.result = executor.Result{Invocation: runner.last, Response: response}
			got, err := adapter.CallTool(context.Background(), "doc", "get", nil)
			if err != nil || got == nil {
				t.Fatalf("adapter result = %#v, %v", got, err)
			}
		})
	}
	if strVal(map[string]any{"x": 1}, "x") != "" || strVal(map[string]any{"x": "v"}, "x") != "v" {
		t.Fatal("strVal mismatch")
	}
}

func TestCrossPlatformCoverageRunnerPureCoverage(t *testing.T) {
	for _, tc := range []struct {
		content map[string]any
		want    string
	}{
		{nil, ""},
		{map[string]any{"success": true}, ""},
		{map[string]any{"success": "false"}, ""},
		{map[string]any{"success": false, "errorMsg": " message "}, "message"},
		{map[string]any{"success": false, "errorCode": " CODE "}, "business error: code CODE"},
		{map[string]any{"success": false}, "business error: success=false"},
		{map[string]any{"data": map[string]any{"success": false, "errorMsg": "nested"}}, "nested"},
		{map[string]any{"result": []any{"skip", map[string]any{"success": false, "errorMsg": "array"}}}, "array"},
	} {
		if got := detectBusinessError(tc.content); got != tc.want {
			t.Errorf("detectBusinessError(%#v) = %q, want %q", tc.content, got, tc.want)
		}
	}
	deep := map[string]any{"success": false}
	for range 10 {
		deep = map[string]any{"data": deep}
	}
	_ = detectBusinessError(deep)

	for _, tc := range []struct {
		result transport.ToolCallResult
		want   string
	}{
		{transport.ToolCallResult{Blocks: []transport.ContentBlock{{Text: " block "}}}, "block"},
		{transport.ToolCallResult{Content: map[string]any{"message": " message "}}, "message"},
		{transport.ToolCallResult{Content: map[string]any{"error": " error "}}, "error"},
		{transport.ToolCallResult{}, "MCP tool returned an error response"},
	} {
		if got := extractMCPErrorMessage(tc.result); got != tc.want {
			t.Errorf("extractMCPErrorMessage = %q, want %q", got, tc.want)
		}
	}

	if runtimeFlagEnabled("", false) || runtimeFlagEnabled("off", true) || !runtimeFlagEnabled("yes", false) {
		t.Fatal("runtime flag parsing mismatch")
	}
	if isAuthError(nil) || !isAuthError(apperrors.NewAuth("auth")) || isAuthError(errors.New("plain")) {
		t.Fatal("auth error classification mismatch")
	}
	t.Setenv("DINGTALK_MY_PRODUCT_MCP_URL", " https://override.test ")
	if got, ok := productEndpointOverride("my-product"); !ok || got != "https://override.test" {
		t.Fatalf("endpoint override = %q %v", got, ok)
	}
	if got, ok := productEndpointOverride("missing"); ok || got != "" {
		t.Fatalf("missing endpoint override = %q %v", got, ok)
	}

	r := &runtimeRunner{}
	if report, err := r.scanContent(nil); err != nil || report.Scanned {
		t.Fatalf("nil scanner = %#v %v", report, err)
	}
	r.scanner = coverageScanner{report: safety.Report{Scanned: true}}
	if report, err := r.scanContent(map[string]any{}); err != nil || !report.Scanned {
		t.Fatalf("scanner = %#v %v", report, err)
	}
	r.scanner = coverageScanner{report: safety.Report{Scanned: true, Findings: []safety.Finding{{Pattern: "bad"}}}}
	r.enforceContentScan = true
	if _, err := r.scanContent(map[string]any{}); err == nil {
		t.Fatal("enforced finding was accepted")
	}
	t.Setenv(runtimeContentScanEnv, "false")
	if newRuntimeContentScanner() != nil {
		t.Fatal("disabled scanner was created")
	}
	t.Setenv(runtimeContentScanEnv, "true")
	if newRuntimeContentScanner() == nil {
		t.Fatal("enabled scanner missing")
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	inv := executor.Invocation{CanonicalProduct: "doc", Tool: "get"}
	logBusinessError(nil, "reason", inv, nil, apperrors.ServerDiagnostics{})
	logBusinessError(logger, "reason", inv, map[string]any{"error": "e", "errorMsg": "em", "message": "m"}, apperrors.ServerDiagnostics{TraceID: "t", ServerErrorCode: "c", TechnicalDetail: "d"})

	if len(generateExecutionID()) != 16 {
		t.Fatal("execution ID length changed")
	}
	ResetRuntimeTokenCache()
}

func TestCrossPlatformCoverageDocDownloadPureCoverage(t *testing.T) {
	for _, inv := range []executor.Invocation{
		{},
		{CanonicalProduct: "DOC", Tool: docDownloadFileTool},
		{CanonicalProduct: "doc", Tool: "other"},
	} {
		_ = isDocDownloadInvocation(inv)
	}
	for _, params := range []map[string]any{
		nil,
		{"nodeId": 1},
		{"nodeId": " ", "node": " node "},
		{"dentryUuid": "uuid"},
	} {
		_ = docDownloadNodeID(params)
	}
	_ = unsupportedAXLSDownloadError()
	for _, content := range []map[string]any{
		{"result": map[string]any{"extension": " axls "}},
		{"data": map[string]any{"extension": "doc"}},
		{"extension": "sheet"},
		{"result": "bad"},
	} {
		_ = documentInfoExtension(content)
	}
	if stringAtPath(map[string]any{"x": map[string]any{"y": 1}}, "x", "y") != "" || stringAtPath("bad", "x") != "" {
		t.Fatal("stringAtPath accepted a non-string path")
	}
}

func TestCrossPlatformCoverageRecoveryPureCoverage(t *testing.T) {
	if _, err := decodeRecoveryAttempts(nil, nil, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeRecoveryAttempts(json.RawMessage("null"), nil, "", ""); err != nil {
		t.Fatal(err)
	}
	if got, err := decodeRecoveryAttempts(json.RawMessage(`[{"command_summary":"one"}]`), nil, "", ""); err != nil || len(got) != 1 {
		t.Fatalf("array attempts = %#v %v", got, err)
	}
	if _, err := decodeRecoveryAttempts(json.RawMessage("{"), nil, "", ""); err == nil {
		t.Fatal("malformed attempts succeeded")
	}
	if got, err := decodeRecoveryAttempts(json.RawMessage("2"), []string{"a"}, "ok", ""); err != nil || len(got) != 2 {
		t.Fatalf("legacy attempts = %#v %v", got, err)
	}
	if legacyRecoveryAttempts(0, nil, "", "") != nil || len(legacyRecoveryAttempts(1, nil, "", "")) != 1 {
		t.Fatal("legacy attempts edge mismatch")
	}

	for _, args := range [][]string{
		{"dws", "--debug", "doc", "get", "--node", "n"},
		{"dws", "--format=json", "doc", "--", "ignored"},
		{"dws", "--unknown", "value", "doc"},
	} {
		old := os.Args
		os.Args = args
		_ = currentCommandPath()
		os.Args = old
	}
	for _, inv := range []executor.Invocation{
		{LegacyPath: "legacy path"},
		{CanonicalProduct: "doc", Tool: "get"},
		{CanonicalProduct: "doc"},
		{},
	} {
		old := os.Args
		os.Args = []string{"dws"}
		_ = runtimeCommandPath(inv)
		os.Args = old
	}
	if cloneRecoveryArgs(nil) != nil {
		t.Fatal("empty recovery args should clone to nil")
	}
	original := map[string]any{"x": 1}
	clone := cloneRecoveryArgs(original)
	clone["x"] = 2
	if original["x"] != 1 {
		t.Fatal("recovery args were not cloned")
	}

	if got, _ := (*recoveryRuntime)(nil).Search(context.Background(), "query", recovery.RecoveryContext{}); got.DocSearch.Status != "skipped" {
		t.Fatalf("nil recovery search = %#v", got)
	}
	if got, _ := (&recoveryRuntime{}).Search(context.Background(), " ", recovery.RecoveryContext{}); got.DocSearch.Status != "skipped" {
		t.Fatalf("blank recovery search = %#v", got)
	}
	if _, err := (*recoveryRuntime)(nil).CallToolDirect(context.Background(), "x", "y", nil); err == nil {
		t.Fatal("nil recovery runtime call succeeded")
	}
	if _, err := (&recoveryRuntime{}).resolveEndpoint(context.Background(), "missing", "tool"); err == nil {
		t.Fatal("missing recovery endpoint succeeded")
	}
	loaderErr := errors.New("catalog")
	runtime := &recoveryRuntime{loader: cli.CatalogLoaderFrom(cli.Catalog{}, loaderErr)}
	if _, err := runtime.resolveEndpoint(context.Background(), "missing", "tool"); !errors.Is(err, loaderErr) {
		t.Fatalf("catalog recovery error = %v", err)
	}
	runtime.loader = cli.StaticLoader{Catalog: cli.Catalog{Products: []cli.CanonicalProduct{{ID: "empty"}, {ID: "ok", Endpoint: " https://catalog.test "}}}}
	if _, err := runtime.resolveEndpoint(context.Background(), "empty", "tool"); err == nil {
		t.Fatal("empty catalog endpoint succeeded")
	}
	if got, err := runtime.resolveEndpoint(context.Background(), "ok", "tool"); err != nil || got != "https://catalog.test" {
		t.Fatalf("catalog endpoint = %q %v", got, err)
	}
	if recoveryRuntimeToken(nil) != "" || recoveryRuntimeToken(&GlobalFlags{Token: " token "}) != "token" {
		t.Fatal("recovery token mismatch")
	}
	if toRecoveryToolResponse(nil) != nil {
		t.Fatal("nil recovery response should stay nil")
	}
	response := toRecoveryToolResponse(&transport.ToolCallResult{IsError: true, Blocks: []transport.ContentBlock{{Type: "text", Text: "body"}}})
	if response == nil || !response.IsError || len(response.Content) != 1 {
		t.Fatalf("recovery response = %#v", response)
	}

	items := []any{map[string]any{"title": "A", "url": "u", "desc": "d"}, "skip", map[string]any{}}
	for _, payload := range []map[string]any{
		nil,
		{"items": items},
		{"data": map[string]any{"items": items}},
		{"result": map[string]any{"items": items}},
	} {
		_ = parseDocSearchItemsFromMap(payload)
	}
	if toDocSearchItems("bad") != nil {
		t.Fatal("non-list doc items accepted")
	}
	result := &transport.ToolCallResult{Content: map[string]any{}, Blocks: []transport.ContentBlock{{Text: "{"}, {Text: `{"items":[{"title":"B"}]}`}}}
	if got := parseDocSearchItems(result); len(got) != 1 {
		t.Fatalf("block doc items = %#v", got)
	}
	if parseDocSearchItems(nil) != nil {
		t.Fatal("nil doc result should be nil")
	}
	searchItems := []recovery.DocSearchItem{{Title: "query", URL: "u"}, {Title: "other"}, {Title: "third"}, {Title: "fourth"}}
	if len(rerankDocSearchHits("query", recovery.RecoveryContext{ToolName: "tool", CommandPath: []string{"doc"}}, searchItems)) != 3 || rerankDocSearchHits("", recovery.RecoveryContext{}, nil) != nil {
		t.Fatal("doc search reranking mismatch")
	}
}

func TestCrossPlatformCoverageSmallAppRegistryAndRootCoverage(t *testing.T) {
	RegisterPluginAuth("coverage-registry", &PluginAuth{Token: "token"})
	t.Cleanup(func() {
		pluginAuthMu.Lock()
		delete(pluginAuthRegistry, "coverage-registry")
		pluginAuthMu.Unlock()
	})
	if got, ok := LookupPluginAuth("coverage-registry"); !ok || got.Token != "token" {
		t.Fatalf("plugin auth = %#v %v", got, ok)
	}
	if _, ok := LookupPluginAuth("missing"); ok {
		t.Fatal("missing plugin auth found")
	}
	configureOAuthProviderCompatibility(authpkg.NewOAuthProvider(t.TempDir(), nil), t.TempDir())
	configureLegacyAuthManagerCompatibility(authpkg.NewManager(t.TempDir(), nil))
	if IsAuthRetrying(nil) || !IsAuthRetrying(context.WithValue(context.Background(), authRetryingKey, true)) {
		t.Fatal("auth retry context mismatch")
	}
	if IsAuthRetrying(context.WithValue(context.Background(), authRetryingKey, "bad")) {
		t.Fatal("invalid auth retry value accepted")
	}

	oldVersion, oldBuild, oldCommit := version, buildTime, gitCommit
	t.Cleanup(func() { version, buildTime, gitCommit = oldVersion, oldBuild, oldCommit })
	version, buildTime, gitCommit = "dev", "unknown", "unknown"
	SetVersion("", "", "")
	if Version() != "dev" {
		t.Fatal("plain version mismatch")
	}
	SetVersion("1.2.3", "today", "abc")
	if RawVersion() != "1.2.3" || BuildTime() != "today" || GitCommit() != "abc" || !strings.Contains(Version(), "abc") {
		t.Fatal("version metadata mismatch")
	}

	for _, err := range []error{nil, errors.New("plain"), errors.New(`required flag(s) "email", "name" not set`), errors.New("required flag(s)  not set")} {
		_ = rewordRequiredFlagError(err)
		_ = isUnknownCommandError(err)
	}
	if !isUnknownCommandError(errors.New("unknown command x")) {
		t.Fatal("unknown command not recognized")
	}
	if resolveVerbosity(nil) != apperrors.VerbosityNormal {
		t.Fatal("nil verbosity mismatch")
	}
	cmd := &cobra.Command{Use: "cmd"}
	cmd.Flags().Bool("debug", false, "")
	cmd.Flags().Bool("verbose", false, "")
	_ = cmd.Flags().Set("verbose", "true")
	if resolveVerbosity(cmd) != apperrors.VerbosityVerbose {
		t.Fatal("verbose level mismatch")
	}
	_ = cmd.Flags().Set("debug", "true")
	if resolveVerbosity(cmd) != apperrors.VerbosityDebug {
		t.Fatal("debug level mismatch")
	}
	if wantsJSONErrors(nil) || commandRequestsJSONErrors(nil) {
		t.Fatal("nil command requested JSON")
	}
	jsonCmd := &cobra.Command{Use: "root"}
	jsonCmd.PersistentFlags().String("format", "table", "")
	child := &cobra.Command{Use: "child"}
	jsonCmd.AddCommand(child)
	_ = jsonCmd.PersistentFlags().Set("format", "json")
	if !wantsJSONErrors(child) || !commandRequestsJSONErrors(jsonCmd) {
		t.Fatal("JSON format was not recognized")
	}
	jsonBool := &cobra.Command{Use: "root"}
	jsonBool.Flags().Bool("json", false, "")
	_ = jsonBool.Flags().Set("json", "true")
	if !commandRequestsJSONErrors(jsonBool) {
		t.Fatal("JSON boolean was not recognized")
	}
	var human, raw bytes.Buffer
	if err := printExecutionError(nil, &human, &human, errors.New("plain")); err != nil {
		t.Fatal(err)
	}
	rawErr := coverageRawStderr("raw line")
	if err := printExecutionError(nil, &raw, &raw, rawErr); err != nil || !strings.Contains(raw.String(), "raw line") {
		t.Fatalf("raw stderr = %q %v", raw.String(), err)
	}
	if err := printExecutionError(jsonBool, io.Discard, io.Discard, errors.New("json")); err != nil {
		t.Fatal(err)
	}

	_ = MCPIdentityHeaders()
	if noCredentialsError() == nil {
		t.Fatal("standalone credentials error missing")
	}
	oldEdition := edition.Get()
	defer edition.Override(oldEdition)
	edition.Override(&edition.Hooks{IsEmbedded: true})
	if !strings.Contains(noCredentialsError().Error(), "认证") {
		t.Fatal("embedded credentials error mismatch")
	}
}

func TestCrossPlatformCoverageDirectRuntimeCoverage(t *testing.T) {
	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition); SetDynamicServers(nil) })
	server := mcptypes.ServerDescriptor{
		Endpoint: "https://one.test",
		CLI: mcptypes.CLIOverlay{
			ID:      "one",
			Command: "cmd",
			Aliases: []string{"alias", " "},
			Tools:   []mcptypes.CLITool{{Name: "tool"}, {Name: " "}},
			ToolOverrides: map[string]mcptypes.CLIToolOverride{
				"override": {}, "skip": {ServerOverride: "other"}, " ": {},
			},
		},
	}
	SetDynamicServers([]mcptypes.ServerDescriptor{{CLI: mcptypes.CLIOverlay{Skip: true}}, server, {CLI: mcptypes.CLIOverlay{ID: "empty"}}})
	if got, ok := directRuntimeToolEndpoint("tool"); !ok || got != "https://one.test" {
		t.Fatalf("tool endpoint = %q %v", got, ok)
	}
	if _, ok := directRuntimeToolEndpoint(" "); ok {
		t.Fatal("blank tool endpoint resolved")
	}
	for _, id := range []string{"one", "cmd", "alias"} {
		if got, ok := directRuntimeEndpoint(id, ""); !ok || got != "https://one.test" {
			t.Errorf("direct endpoint %q = %q %v", id, got, ok)
		}
	}
	if normalizeDirectRuntimeProductID("alias") != "one" || normalizeDirectRuntimeProductID("tb") != "teambition" || normalizeDirectRuntimeProductID("plain") != "plain" {
		t.Fatal("direct runtime alias mismatch")
	}
	if ids := DirectRuntimeProductIDs(); !ids["one"] || !ids[defaultPATProductID] || !ids[devappProductID] {
		t.Fatalf("direct runtime IDs = %#v", ids)
	}

	t.Setenv(cli.CatalogFixtureEnv, "fixture")
	if shouldUseDirectRuntime(executor.Invocation{Kind: "helper_invocation"}) {
		t.Fatal("fixture should disable direct runtime")
	}
	t.Setenv(cli.CatalogFixtureEnv, "")
	if !shouldUseDirectRuntime(executor.Invocation{Kind: "helper_invocation"}) || !shouldUseDirectRuntime(executor.Invocation{Kind: "compat_invocation"}) || shouldUseDirectRuntime(executor.Invocation{}) {
		t.Fatal("direct runtime kind mismatch")
	}

	edition.Override(&edition.Hooks{
		StaticServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{{ID: "", Endpoint: ""}, {ID: "static", Endpoint: " https://static.test "}}
		},
		SupplementServers: func() []edition.ServerInfo {
			return []edition.ServerInfo{{ID: "supplement", Endpoint: "https://supp.test", Prefixes: []string{"prefix"}}}
		},
	})
	for id, want := range map[string]string{"static": "https://static.test", "supplement": "https://supp.test", "prefix": "https://supp.test"} {
		if got, ok := editionServerEndpoint(id); !ok || got != want {
			t.Errorf("edition endpoint %q = %q %v", id, got, ok)
		}
	}
	if _, ok := editionServerEndpoint(""); ok {
		t.Fatal("blank edition server resolved")
	}
	if _, ok := endpointFromEditionServers("x", nil); ok {
		t.Fatal("nil edition server function resolved")
	}

	SetDynamicServers(nil)
	AppendDynamicServer(mcptypes.ServerDescriptor{CLI: mcptypes.CLIOverlay{Skip: true}})
	AppendDynamicServer(server)
	AppendDynamicServer(mcptypes.ServerDescriptor{Endpoint: "https://two.test", CLI: mcptypes.CLIOverlay{ID: "two", Command: "cmd", Tools: []mcptypes.CLITool{{Name: "tool2"}}}})
	if got, ok := directRuntimeEndpoint("cmd", ""); !ok || got != "https://one.test" {
		t.Fatalf("append preserved command endpoint = %q %v", got, ok)
	}
	if got, ok := directRuntimeEndpoint("unknown", "tool2"); !ok || got != "https://two.test" {
		t.Fatalf("tool fallback endpoint = %q %v", got, ok)
	}
	if _, ok := directRuntimeEndpoint("missing", "missing"); ok {
		t.Fatal("missing direct endpoint resolved")
	}
	_ = devappMCPEndpoint()
	_ = defaultPATServerDescriptor()
	_ = defaultPATMCPEndpoint()
}

func TestCrossPlatformCoverageRecoveryLoadExecutionCoverage(t *testing.T) {
	if _, err := loadRecoveryExecution(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing recovery execution succeeded")
	}
	path := filepath.Join(t.TempDir(), "execution.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRecoveryExecution(path); err == nil {
		t.Fatal("malformed recovery execution succeeded")
	}
	for name, body := range map[string]string{
		"legacy": `{"action":" one ","attempt":2,"result":" ok ","error":" bad "}`,
		"modern": `{"actions":["one"],"attempts":[{"command_summary":"one"}],"error_summary":"bad"}`,
	} {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
				t.Fatal(err)
			}
			if got, err := loadRecoveryExecution(path); err != nil || len(got.Actions) != 1 || len(got.Attempts) == 0 {
				t.Fatalf("loaded execution = %#v %v", got, err)
			}
		})
	}
}

func TestCrossPlatformCoverageRecoveryRuntimeHTTP(t *testing.T) {
	var result map[string]any = map[string]any{"content": []map[string]any{{"type": "text", "text": `{"items":[{"title":"query result","url":"u"}]}`}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID int `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result})
	}))
	defer server.Close()
	SetDynamicServers([]mcptypes.ServerDescriptor{{Endpoint: server.URL, CLI: mcptypes.CLIOverlay{ID: "devdoc", Tools: []mcptypes.CLITool{{Name: "search_open_platform_docs_rag"}}}}})
	t.Cleanup(func() { SetDynamicServers(nil) })
	runtime := &recoveryRuntime{transport: transport.NewClient(server.Client())}
	got, err := runtime.Search(context.Background(), "query", recovery.RecoveryContext{ToolName: "search"})
	if err != nil || got.DocSearch.Status != "success" || len(got.KBHits) == 0 {
		t.Fatalf("recovery search = %#v %v", got, err)
	}
	result = map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": "failed"}}}
	if _, err := runtime.CallToolDirect(context.Background(), "devdoc", "search_open_platform_docs_rag", nil); err == nil {
		t.Fatal("recovery MCP error succeeded")
	}
	server.Close()
	if _, err := runtime.CallToolDirect(context.Background(), "devdoc", "search_open_platform_docs_rag", nil); err == nil {
		t.Fatal("recovery network error succeeded")
	}
}

func TestCrossPlatformCoverageEventCommandPureCoverage(t *testing.T) {
	oldEdition := edition.Get()
	t.Cleanup(func() { edition.Override(oldEdition) })
	edition.Override(&edition.Hooks{})
	if editionNameOrDefault() != "open" {
		t.Fatal("default edition name changed")
	}
	edition.Override(&edition.Hooks{Name: "enterprise"})
	if editionNameOrDefault() != "enterprise" {
		t.Fatal("edition name not used")
	}

	for _, tc := range []struct {
		value string
		want  string
		err   bool
	}{{"", "user", false}, {" USER ", "user", false}, {"app", "", true}, {"bot", "", true}, {"bad", "", true}} {
		got, err := normalizeEventAs(tc.value)
		if got != tc.want || (err != nil) != tc.err {
			t.Errorf("normalizeEventAs(%q) = %q, %v", tc.value, got, err)
		}
	}
	if firstArg(nil) != "" || firstArg([]string{"one"}) != "one" {
		t.Fatal("firstArg mismatch")
	}
	if got := eventTypesWithDefault([]string{"one"}); len(got) != 1 || got[0] != "one" {
		t.Fatalf("explicit event types = %#v", got)
	}
	_ = eventTypesWithDefault(nil)
	if sourceKindLabel("") != string(dwsevent.SourceKindAppStream) || sourceKindLabel(dwsevent.SourceKindPersonalStream) != string(dwsevent.SourceKindPersonalStream) {
		t.Fatal("source kind labels mismatch")
	}
	if got := eventWorkDir("/tmp/config", "open", "", "hash"); !strings.Contains(got, "app_stream") {
		t.Fatalf("default event workdir = %q", got)
	}
	if got := eventWorkDir("/tmp/config", "open", dwsevent.SourceKindPersonalStream, "hash"); !strings.Contains(got, "personal_stream") {
		t.Fatalf("personal event workdir = %q", got)
	}
	_ = defaultIPCEndpoint(t.TempDir(), "open", dwsevent.SourceKindAppStream, "hash")

	for _, opts := range []eventStreamTicketOptions{
		{},
		{Mode: " NORMAL ", SourceID: " source ", TicketURL: " https://ticket.test "},
		{Mode: "websocket"},
	} {
		_ = opts.enabled()
		_ = opts.normalizedMode()
		_ = opts.usesPortalNormalMode()
		_ = opts.spawnArgs()
	}
	if got := eventStreamTicketURL(" explicit "); got != "explicit" {
		t.Fatalf("explicit ticket URL = %q", got)
	}
	if got := eventStreamTicketURL(""); !strings.HasSuffix(got, "/stream/connections/ticket") {
		t.Fatalf("default ticket URL = %q", got)
	}
	t.Setenv("DWS_STREAM_SOURCE_ID", " env-source ")
	if defaultEventStreamSourceID() != "env-source" || eventStreamSourceID("") != "env-source" || eventStreamSourceID("direct") != "direct" {
		t.Fatal("event source ID precedence mismatch")
	}
	t.Setenv("DWS_STREAM_SOURCE_ID", "")
	_ = defaultEventStreamSourceID()
	if clientID, secret, err := resolveEventCredentials(t.TempDir(), eventStreamTicketOptions{Mode: "normal", SourceID: "source"}); err != nil || secret != "" || !strings.HasPrefix(clientID, "portal-ticket-normal:") {
		t.Fatalf("portal event credentials = %q %q %v", clientID, secret, err)
	}
	if got := eventStreamBusID(eventStreamTicketOptions{SourceID: "source"}); got != "portal-ticket-normal:source" {
		t.Fatalf("event bus ID = %q", got)
	}
}

func TestCrossPlatformCoverageEventListAndStatusCoverage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", dir)
	cmd := &cobra.Command{Use: "event"}
	entries, err := collectEntries(cmd, "client", false, false)
	if err != nil || len(entries) != 1 || entries[0].Entry.State != busctl.BusStateNotRunning || entries[0].Entry.Meta == nil {
		t.Fatalf("single event entry = %#v %v", entries, err)
	}
	if all, err := collectEntries(cmd, "", true, false); err != nil || len(all) != 0 {
		t.Fatalf("all event entries = %#v %v", all, err)
	}
	if all, err := collectEntries(cmd, "", false, true); err != nil || len(all) != 0 {
		t.Fatalf("all-edition event entries = %#v %v", all, err)
	}
	if got := queryAll([]busctl.BusEntry{{State: busctl.BusStateNotRunning}}); len(got) != 1 {
		t.Fatalf("queryAll = %#v", got)
	}

	now := time.Now().Add(-time.Minute)
	live := &eventtransport.StatusResp{
		Bus:         eventtransport.StatusBus{UptimeSecs: 60},
		SourceState: eventtransport.StatusSource{State: "connected", Source: "hook", ReconnectCount: 2},
		Consumers: []eventtransport.StatusConsumer{
			{PID: 2, Received: 3, Dropped: 1},
			{PID: 3, EventTypes: []string{"event.one"}, SubscribeID: "sub", Received: 4},
		},
		PerEventTypeCounters: map[string]eventtransport.Counters{"z": {Received: 1}, "a": {Dropped: 2}},
	}
	statuses := []busctl.EntryStatus{
		{Entry: busctl.BusEntry{ClientIDHash: "hash", Edition: "open", State: busctl.BusStateNotRunning, WorkDir: dir}},
		{Entry: busctl.BusEntry{ClientIDHash: "hash", Edition: "open", State: busctl.BusStateOrphan, HolderPID: 1, WorkDir: dir, Meta: &eventbus.Meta{ClientID: "client", SourceID: "source", StartedAt: now}}},
		{Entry: busctl.BusEntry{ClientIDHash: "hash", Edition: "open", State: busctl.BusStateRunning, HolderPID: 2, WorkDir: dir, Meta: &eventbus.Meta{ClientID: "client", StartedAt: now}}},
		{Entry: busctl.BusEntry{ClientIDHash: "hash", Edition: "open", State: busctl.BusStateRunning, HolderPID: 3, WorkDir: dir}, Live: live},
	}
	var output bytes.Buffer
	if err := renderStatus(&output, statuses, "text"); err != nil || !strings.Contains(output.String(), "orphan") || !strings.Contains(output.String(), "Consumers") {
		t.Fatalf("status text = %q %v", output.String(), err)
	}
	output.Reset()
	if err := renderStatus(&output, statuses, "json"); err != nil || !strings.Contains(output.String(), "not_running") {
		t.Fatalf("status JSON = %q %v", output.String(), err)
	}

	list := make([]listEntry, 0, len(statuses))
	for _, status := range statuses {
		list = append(list, buildListEntry(status))
	}
	list = append(list, listEntry{ClientIDHash: "unknown", BusState: busctl.BusStateRunning, Consumers: live.Consumers})
	output.Reset()
	if err := renderList(&output, list, "table"); err != nil || !strings.Contains(output.String(), "catch-all") || !strings.Contains(output.String(), "unknown") {
		t.Fatalf("list table = %q %v", output.String(), err)
	}
	output.Reset()
	if err := renderList(&output, list, "json"); err != nil || !strings.Contains(output.String(), "client_id_hash") {
		t.Fatalf("list JSON = %q %v", output.String(), err)
	}
}

func TestCrossPlatformCoverageEventCommandValidationCoverage(t *testing.T) {
	execute := func(t *testing.T, cmd *cobra.Command, args ...string) error {
		t.Helper()
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetOut(io.Discard)
		cmd.SetErr(io.Discard)
		cmd.SetArgs(args)
		return cmd.Execute()
	}
	if err := execute(t, newEventCommand()); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"--as", "bad"}, {"--as", "app", "--debug-raw-events"}, {"--as", "app", "--subscribe-id", "x"}, {"--as", "app", "key"}} {
		if err := execute(t, newEventConsumeCommand(), args...); err == nil {
			t.Errorf("consume args %#v unexpectedly succeeded", args)
		}
	}
	for _, maker := range []func() *cobra.Command{newEventListCommand, newEventStatusCommand, newEventStopCommand} {
		if err := execute(t, maker(), "--as", "bad"); err == nil {
			t.Fatal("invalid event identity succeeded")
		}
	}

	flagCmd := &cobra.Command{Use: "flags"}
	flagCmd.Flags().String("one", "", "")
	flagCmd.Flags().String("two", "", "")
	hideEventInternalFlags(flagCmd, "one", "missing")
	if err := rejectChangedFlags(flagCmd, "user", "one", "missing"); err != nil {
		t.Fatalf("unchanged flags rejected: %v", err)
	}
	_ = flagCmd.Flags().Set("one", "value")
	if err := rejectChangedFlags(flagCmd, "user", "one", "missing"); err == nil {
		t.Fatal("changed app flag accepted")
	}
	if err := rejectPersonalEventUnsupportedFlags(flagCmd, "one", "missing"); err == nil {
		t.Fatal("changed personal flag accepted")
	}
}

func TestCrossPlatformCoverageVersionCacheCompletionCoverage(t *testing.T) {
	oldVersion, oldBuild, oldCommit := version, buildTime, gitCommit
	t.Cleanup(func() { version, buildTime, gitCommit = oldVersion, oldBuild, oldCommit })
	version, buildTime, gitCommit = "1.0", "unknown", "unknown"
	for _, args := range [][]string{nil, {"--format", "json"}} {
		cmd := newVersionCommand()
		cmd.Flags().String("format", "table", "")
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil || output.Len() == 0 {
			t.Fatalf("version %#v = %q %v", args, output.String(), err)
		}
	}
	version, buildTime, gitCommit = "1.0", "today", "abc"
	cmd := newVersionCommand()
	cmd.SetOut(io.Discard)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	root := &cobra.Command{Use: "dws"}
	root.PersistentFlags().String("format", "json", "")
	for _, format := range []string{"json", "pretty", "table"} {
		_ = root.PersistentFlags().Set("format", format)
		child := &cobra.Command{Use: "child"}
		root.AddCommand(child)
		child.SetOut(io.Discard)
		if err := printCacheCompatNotice(child, "status"); err != nil {
			t.Fatalf("cache %s: %v", format, err)
		}
		root.RemoveCommand(child)
	}
	cache := newCacheCommand()
	cache.SetOut(io.Discard)
	cache.SetArgs([]string{"status"})
	if err := cache.Execute(); err != nil {
		t.Fatal(err)
	}

	completionRoot := &cobra.Command{Use: "dws"}
	completionRoot.AddCommand(&cobra.Command{Use: "child"})
	for _, shell := range []string{"bash", "zsh", "fish"} {
		completion := newCompletionCommand(completionRoot)
		completion.SetOut(io.Discard)
		completion.SetArgs([]string{shell})
		if err := completion.Execute(); err != nil {
			t.Fatalf("completion %s: %v", shell, err)
		}
	}
}

func TestCrossPlatformCoverageRuntimeRunnerRoutingCoverage(t *testing.T) {
	inv := executor.Invocation{CanonicalProduct: "product", Tool: "tool", Params: map[string]any{"x": 1}}
	fallback := &coverageRunner{result: executor.Result{Response: map[string]any{"fallback": true}}}
	r := &runtimeRunner{fallback: fallback}
	if got, err := r.runSingle(context.Background(), inv, false); err != nil || got.Response["fallback"] != true {
		t.Fatalf("fallback runner = %#v %v", got, err)
	}

	r.transport = transport.NewClient(nil)
	r.loader = cli.StaticLoader{}
	r.globalFlags = &GlobalFlags{Mock: true}
	if got, err := r.runSingle(context.Background(), inv, false); err != nil || got.Response["content"] == nil {
		t.Fatalf("mock route = %#v %v", got, err)
	}
	t.Setenv("DINGTALK_PRODUCT_MCP_URL", "https://override.test")
	if got, err := r.runSingle(context.Background(), inv, false); err != nil || got.Response["endpoint"] != "https://override.test" {
		t.Fatalf("mock override route = %#v %v", got, err)
	}
	t.Setenv("DINGTALK_PRODUCT_MCP_URL", "")

	r.globalFlags = &GlobalFlags{DryRun: true}
	r.loader = cli.CatalogLoaderFrom(cli.Catalog{}, errors.New("load failure"))
	if _, err := r.runSingle(context.Background(), inv, false); err == nil || !strings.Contains(err.Error(), "load failure") {
		t.Fatalf("catalog failure = %v", err)
	}
	r.loader = cli.StaticLoader{}
	if got, err := r.runSingle(context.Background(), inv, false); err != nil || got.Response["fallback"] != true || !fallback.last.DryRun {
		t.Fatalf("dry catalog miss = %#v %v (invocation %#v)", got, err, fallback.last)
	}
	r.globalFlags = &GlobalFlags{}
	if _, err := r.runSingle(context.Background(), inv, false); err == nil {
		t.Fatal("catalog miss succeeded")
	}
	devInv := inv
	devInv.CanonicalProduct = devappProductID
	if _, err := r.handleCatalogMiss(context.Background(), devInv, "missing"); err == nil {
		t.Fatal("devapp catalog miss succeeded")
	}

	product := cli.CanonicalProduct{ID: "product", Endpoint: "https://catalog.test", Tools: []cli.ToolDescriptor{{RPCName: "tool"}}}
	r.loader = cli.StaticLoader{Catalog: cli.Catalog{Products: []cli.CanonicalProduct{product}}}
	r.globalFlags = &GlobalFlags{DryRun: true}
	if got, err := r.runSingle(context.Background(), inv, false); err != nil || got.Response["endpoint"] != "https://catalog.test" {
		t.Fatalf("catalog dry route = %#v %v", got, err)
	}
	product.Tools = nil
	r.loader = cli.StaticLoader{Catalog: cli.Catalog{Products: []cli.CanonicalProduct{product}}}
	SetDynamicServers([]mcptypes.ServerDescriptor{{Endpoint: "https://direct.test", CLI: mcptypes.CLIOverlay{ID: "product", Tools: []mcptypes.CLITool{{Name: "tool"}}}}})
	t.Cleanup(func() { SetDynamicServers(nil) })
	if got, err := r.runSingle(context.Background(), inv, false); err != nil || got.Response["endpoint"] != "https://direct.test" {
		t.Fatalf("undeclared tool direct route = %#v %v", got, err)
	}
	SetDynamicServers(nil)
	if got, err := r.runSingle(context.Background(), inv, false); err != nil || got.Response["fallback"] != true || !fallback.last.DryRun {
		t.Fatalf("undeclared tool dry miss = %#v %v (invocation %#v)", got, err, fallback.last)
	}

	SetDynamicServers([]mcptypes.ServerDescriptor{{Endpoint: "https://owner.test", CLI: mcptypes.CLIOverlay{ID: "owner", Tools: []mcptypes.CLITool{{Name: "tool"}}}}})
	product.Tools = []cli.ToolDescriptor{{RPCName: "tool"}}
	r.loader = cli.StaticLoader{Catalog: cli.Catalog{Products: []cli.CanonicalProduct{product}}}
	if got, err := r.runSingle(context.Background(), inv, false); err != nil || got.Response["endpoint"] != "https://owner.test" {
		t.Fatalf("tool owner correction = %#v %v", got, err)
	}

	for _, result := range []executor.Result{{}, {Response: map[string]any{"content": "value"}}, {Response: map[string]any{"value": 1}}} {
		_ = multiProfileResultPayload(result)
	}
	for _, err := range []error{errors.New("plain"), apperrors.NewAPI("api", apperrors.WithReason("reason"), apperrors.WithOperation("op"))} {
		payload := multiProfileErrorPayload(err)
		if payload["message"] == "" {
			t.Fatalf("multi-profile error payload = %#v", payload)
		}
	}
}

func TestCrossPlatformCoverageExecuteInvocationCoverage(t *testing.T) {
	oldEdition := edition.Get()
	edition.Override(&edition.Hooks{TokenProvider: func(context.Context, func() (string, error)) (string, error) {
		return "", nil
	}})
	ResetRuntimeTokenCache()
	t.Cleanup(func() {
		edition.Override(oldEdition)
		ResetRuntimeTokenCache()
	})
	pluginAuthMu.Lock()
	oldRegistry := pluginAuthRegistry
	pluginAuthRegistry = make(map[string]*PluginAuth)
	pluginAuthMu.Unlock()
	t.Cleanup(func() {
		pluginAuthMu.Lock()
		pluginAuthRegistry = oldRegistry
		pluginAuthMu.Unlock()
	})
	var rpcResult any = map[string]any{
		"content":           []map[string]any{{"type": "text", "text": "ok"}},
		"structuredContent": map[string]any{"value": 1},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID int `json:"id"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID, "result": rpcResult})
	}))
	defer server.Close()
	r := &runtimeRunner{
		transport:   transport.NewClient(server.Client()),
		globalFlags: &GlobalFlags{},
		fallback:    &coverageRunner{},
	}
	inv := executor.Invocation{CanonicalProduct: "product", Tool: "tool", Params: map[string]any{"x": 1}}

	inv.DryRun = true
	if got, err := r.executeInvocation(context.Background(), server.URL, inv); err != nil || got.Response["dry_run"] != true {
		t.Fatalf("HTTP dry-run = %#v %v", got, err)
	}
	inv.Params = map[string]any{"bad": func() {}}
	if _, err := r.executeInvocation(context.Background(), server.URL, inv); err != nil {
		t.Fatalf("unmarshalable dry-run = %v", err)
	}
	inv.Params = map[string]any{"x": 1}
	inv.DryRun = false
	r.globalFlags.Mock = true
	if got, err := r.executeInvocation(context.Background(), server.URL, inv); err != nil || got.Response["content"] == nil {
		t.Fatalf("HTTP mock = %#v %v", got, err)
	}
	r.globalFlags.Mock = false
	if _, err := r.executeInvocation(context.Background(), server.URL, inv); err == nil || !isAuthError(err) {
		t.Fatalf("unauthenticated execution = %v", err)
	}

	pluginAuthMu.Lock()
	pluginAuthRegistry = map[string]*PluginAuth{
		"product": {Token: "plugin-token", ExtraHeaders: map[string]string{"X-Test": "yes"}, TrustedDomains: []string{strings.TrimPrefix(server.URL, "http://")}},
	}
	pluginAuthMu.Unlock()
	r.scanner = coverageScanner{report: safety.Report{Scanned: true}}
	r.includeScanReport = true
	if got, err := r.executeInvocation(context.Background(), server.URL, inv); err != nil || got.Response["safety"] == nil || !got.Invocation.Implemented {
		t.Fatalf("successful execution = %#v %v", got, err)
	}

	rpcResult = map[string]any{"structuredContent": map[string]any{"success": false, "errorMsg": "business failed"}}
	if _, err := r.executeInvocation(context.Background(), server.URL, inv); err == nil || !strings.Contains(err.Error(), "business failed") {
		t.Fatalf("business error = %v", err)
	}
	rpcResult = map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": "tool failed"}}, "structuredContent": map[string]any{"traceId": "trace"}}
	if _, err := r.executeInvocation(context.Background(), server.URL, inv); err == nil || !strings.Contains(err.Error(), "tool failed") {
		t.Fatalf("MCP tool error = %v", err)
	}

	hookErr := errors.New("edition classification")
	edition.Override(&edition.Hooks{ClassifyToolResult: func(map[string]any) error { return hookErr }})
	rpcResult = map[string]any{"structuredContent": map[string]any{"value": 1}}
	if _, err := r.executeInvocation(context.Background(), server.URL, inv); !errors.Is(err, hookErr) {
		t.Fatalf("edition classification = %v", err)
	}
	edition.Override(&edition.Hooks{})

	r.globalFlags.Timeout = 1
	server.Close()
	if _, err := r.executeInvocation(context.Background(), server.URL, inv); err == nil {
		t.Fatal("network failure succeeded")
	}

	stdioInv := inv
	stdioInv.DryRun = true
	if got, err := r.executeInvocation(context.Background(), "stdio://plugin/server", stdioInv); err != nil || got.Response["transport"] != "stdio" {
		t.Fatalf("stdio dry-run = %#v %v", got, err)
	}
	stdioInv.DryRun = false
	if _, err := r.executeStdioInvocation(context.Background(), stdioInv); err == nil {
		t.Fatal("missing stdio client succeeded")
	}
}

func TestCrossPlatformCoverageAuthCommandPureCoverage(t *testing.T) {
	oldEdition := edition.Get()
	oldPrompt := authLoginManualCredentialsPrompt
	oldInteractive := authLoginInteractiveTerminal
	t.Cleanup(func() {
		edition.Override(oldEdition)
		authLoginManualCredentialsPrompt = oldPrompt
		authLoginInteractiveTerminal = oldInteractive
	})
	clearCompatCache()
	_ = authLoginHuhTheme()
	for _, value := range []time.Time{time.Time{}, time.Now().Add(-time.Hour), time.Now().Add(2 * time.Hour), time.Now().Add(48 * time.Hour)} {
		_ = timeOrEmpty(value)
		_ = authLoginFormatExpiry(value)
	}
	for _, data := range []*authpkg.TokenData{
		nil,
		{},
		{ExpiresAt: time.Now().Add(time.Hour)},
		{ExpiresAt: time.Now().Add(time.Hour), RefreshToken: "r", RefreshExpAt: time.Now().Add(48 * time.Hour)},
	} {
		_ = authLoginDisplayExpiry(data)
		_ = authStatusAuthenticated(data)
		_ = authStatusUpdatedAt(data)
	}
	for _, plan := range []*pat.LoginRecommendPlan{nil, {}, {AllGranted: true}, {Scopes: []string{"one"}}} {
		_ = authLoginRecommendPlanSkipsInteractiveAuthorization(plan)
	}
	for _, product := range []pat.LoginRecommendProduct{
		{ProductCode: "doc"},
		{ProductCode: "doc", ProductName: "doc"},
		{ProductCode: "doc", ProductName: "Document", Summary: strings.Repeat("说明", 30)},
	} {
		_ = loginRecommendProductLabel(product)
	}
	for _, limit := range []int{-1, 2, 20} {
		_ = clipRunes("abcdef", limit)
	}
	_ = authLoginStatusLine("ok")
	_ = authLoginInfoLine("key", "value")
	_ = authLoginMutedStyle()

	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("format", "table", "")
	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)
	if authLoginAllowsInteractiveDefault(nil, "table") || !authLoginAllowsInteractiveDefault(child, "table") {
		t.Fatal("interactive auth default mismatch")
	}
	_ = root.PersistentFlags().Set("format", "json")
	if authLoginAllowsInteractiveDefault(child, "json") {
		t.Fatal("explicit JSON unexpectedly allowed interactive auth")
	}
	for _, interactive := range []bool{false, true} {
		for _, recommend := range []bool{false, true} {
			_ = authLoginShouldUsePostLoginTUIModeForTerminal(child, "table", recommend, interactive)
			_ = authLoginShouldShowPostLoginTUIForTerminal(child, "table", recommend, interactive)
			_ = authLoginShouldUseHumanAuthorizationModeForTerminal(child, "table", recommend, interactive)
		}
	}

	for _, err := range []error{nil, errors.New("plain"), keychain.ErrDEKMissing, keychain.NewUnavailableError("read", errors.New("locked"))} {
		_ = authStatusDiagnosticFromError(err)
	}
	var output bytes.Buffer
	valid := &authpkg.TokenData{
		AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now().Add(time.Hour), RefreshExpAt: time.Now().Add(2 * time.Hour),
		CorpID: "corp", CorpName: "Corp", UserID: "user", UserName: "Name",
	}
	for _, tc := range []struct {
		authenticated bool
		refreshed     bool
		data          *authpkg.TokenData
		diagnostic    *authStatusDiagnostic
	}{{false, false, nil, nil}, {false, false, nil, &authStatusDiagnostic{Reason: "reason", Message: "message", Hint: "hint"}}, {true, true, valid, nil}} {
		output.Reset()
		if err := writeAuthStatusJSON(&output, tc.authenticated, tc.refreshed, tc.data, tc.diagnostic); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		data   *authpkg.TokenData
		forced bool
	}{{nil, true}, {valid, true}, {valid, false}} {
		output.Reset()
		if err := writeAuthLoginJSON(&output, tc.data, tc.forced); err != nil {
			t.Fatal(err)
		}
	}

	for _, data := range [][]byte{
		[]byte("{"),
		[]byte(`{"result":[]}`),
		[]byte(`{"result":[{"orgEmployeeModel":{}}]}`),
		[]byte(`{"result":[{"orgEmployeeModel":{"corpId":"corp","orgName":"Corp","userid":"user","name":"Name"}}]}`),
	} {
		_, _ = contactProfileIdentityFromJSON(data)
	}
	for _, result := range []*edition.ToolResult{
		nil,
		{},
		{Content: []edition.ContentBlock{{Text: " "}, {Text: `{"result":[{"orgEmployeeModel":{"corpId":"corp"}}]}`}}},
	} {
		_, _ = contactProfileIdentityFromToolResult(result)
	}
	if firstNonEmptyString(" ", " value ") != "value" || firstNonEmptyString(" ") != "" {
		t.Fatal("first non-empty string mismatch")
	}

	cmd := &cobra.Command{Use: "auth"}
	cmd.SetErr(io.Discard)
	authLoginManualCredentialsPrompt = func() (string, string, error) { return "client", "secret", nil }
	for _, action := range []authLoginGuideAction{authLoginGuideDirectCLI, authLoginGuideConfigureAgentApp, authLoginGuideManualCredentials, "unknown"} {
		err := applyAuthLoginGuideAction(cmd, t.TempDir(), action)
		if action == "unknown" && err == nil {
			t.Fatal("unknown auth guide action succeeded")
		}
	}
	authLoginManualCredentialsPrompt = func() (string, string, error) { return "", "", errors.New("prompt") }
	if err := applyAuthLoginGuideAction(cmd, t.TempDir(), authLoginGuideManualCredentials); err == nil {
		t.Fatal("manual credential prompt error succeeded")
	}

}

func TestCrossPlatformCoverageAuthLoginTokenCommandCoverage(t *testing.T) {
	oldInteractive := authLoginInteractiveTerminal
	authLoginInteractiveTerminal = func() bool { return false }
	t.Cleanup(func() { authLoginInteractiveTerminal = oldInteractive; authpkg.SetRuntimeProfile("") })
	for _, format := range []string{"table", "json"} {
		t.Run(format, func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			root := &cobra.Command{Use: "dws"}
			root.PersistentFlags().String("format", "table", "")
			root.PersistentFlags().Bool("yes", false, "")
			root.PersistentFlags().String("profile", "", "")
			cmd := newAuthLoginCommand(nil)
			root.AddCommand(cmd)
			var output bytes.Buffer
			root.SetOut(&output)
			root.SetErr(io.Discard)
			args := []string{"login", "--token", "manual-token", "--yes"}
			if format == "json" {
				args = append(args, "--format", "json")
			}
			root.SetArgs(args)
			if err := root.Execute(); err != nil || output.Len() == 0 {
				t.Fatalf("token login = %q %v", output.String(), err)
			}
		})
	}

	for _, hidden := range []bool{false, true} {
		old := edition.Get()
		edition.Override(&edition.Hooks{HideAuthLogin: hidden})
		cmd := buildAuthCommand(nil)
		found, _, err := cmd.Find([]string{"login"})
		hasLogin := err == nil && found != nil && found.Name() == "login"
		if hasLogin == hidden {
			t.Fatalf("HideAuthLogin=%v command tree mismatch", hidden)
		}
		edition.Override(old)
	}
}

func TestCrossPlatformCoveragePluginCommandLifecycleCoverage(t *testing.T) {
	home := t.TempDir()
	work := t.TempDir()
	t.Setenv("HOME", home)
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(work); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldWD) })

	run := func(t *testing.T, cmd *cobra.Command, args ...string) (string, error) {
		t.Helper()
		var output bytes.Buffer
		cmd.SetOut(&output)
		cmd.SetErr(io.Discard)
		cmd.SilenceErrors = true
		cmd.SilenceUsage = true
		cmd.SetArgs(args)
		err := cmd.Execute()
		return output.String(), err
	}

	if _, err := run(t, newPluginCreateCommand(), "Bad_Name"); err == nil {
		t.Fatal("invalid plugin scaffold succeeded")
	}
	if output, err := run(t, newPluginCreateCommand(), "demo-plugin", "--description", "Demo"); err != nil || !strings.Contains(output, "Created plugin") {
		t.Fatalf("plugin create = %q %v", output, err)
	}
	if _, err := run(t, newPluginCreateCommand(), "demo-plugin"); err == nil {
		t.Fatal("duplicate plugin scaffold succeeded")
	}
	if _, err := run(t, newPluginValidateCommand(), "missing"); err == nil {
		t.Fatal("missing plugin validation succeeded")
	}
	if output, err := run(t, newPluginValidateCommand(), "demo-plugin"); err != nil || !strings.Contains(output, "Valid") {
		t.Fatalf("plugin validate = %q %v", output, err)
	}
	createdManifest, err := plugin.ParseManifest(filepath.Join("demo-plugin", "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	createdManifest.Build = nil
	manifestData, err := json.Marshal(createdManifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join("demo-plugin", "plugin.json"), manifestData, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := run(t, newPluginInstallCommand()); err == nil {
		t.Fatal("source-less plugin install succeeded")
	}
	if _, err := run(t, newPluginInstallCommand(), "--dir", "missing"); err == nil {
		t.Fatal("missing plugin install succeeded")
	}
	if output, err := run(t, newPluginInstallCommand(), "--dir", "demo-plugin"); err != nil || !strings.Contains(output, "Installed") {
		t.Fatalf("plugin install = %q %v", output, err)
	}

	if output, err := run(t, newPluginListCommand()); err != nil || !strings.Contains(output, "demo-plugin") {
		t.Fatalf("plugin list = %q %v", output, err)
	}
	if output, err := run(t, newPluginListCommand(), "--json"); err != nil || !strings.Contains(output, "demo-plugin") {
		t.Fatalf("plugin list JSON = %q %v", output, err)
	}
	if output, err := run(t, newPluginInfoCommand(), "demo-plugin"); err != nil || !strings.Contains(output, "Description") {
		t.Fatalf("plugin info = %q %v", output, err)
	}
	if _, err := run(t, newPluginInfoCommand(), "missing"); err == nil {
		t.Fatal("missing plugin info succeeded")
	}
	if _, err := run(t, newPluginEnableCommand(), "missing"); err == nil {
		t.Fatal("missing plugin enable succeeded")
	}
	if output, err := run(t, newPluginDisableCommand(), "demo-plugin"); err != nil || !strings.Contains(output, "disabled") {
		t.Fatalf("plugin disable = %q %v", output, err)
	}
	if output, err := run(t, newPluginEnableCommand(), "demo-plugin"); err != nil || !strings.Contains(output, "enabled") {
		t.Fatalf("plugin enable = %q %v", output, err)
	}

	if _, err := run(t, newPluginConfigSetCommand(), "missing", "KEY", "value"); err == nil {
		t.Fatal("missing plugin config set succeeded")
	}
	if output, err := run(t, newPluginConfigSetCommand(), "demo-plugin", "API_KEY", "abcdefghijk"); err != nil || !strings.Contains(output, "saved") {
		t.Fatalf("plugin config set = %q %v", output, err)
	}
	if output, err := run(t, newPluginConfigGetCommand(), "demo-plugin", "API_KEY"); err != nil || !strings.Contains(output, "abcdefghijk") {
		t.Fatalf("plugin config get = %q %v", output, err)
	}
	if _, err := run(t, newPluginConfigGetCommand(), "demo-plugin", "missing"); err == nil {
		t.Fatal("missing plugin config get succeeded")
	}
	for _, args := range [][]string{{"demo-plugin"}, {"demo-plugin", "--json"}, {"missing"}} {
		if _, err := run(t, newPluginConfigListCommand(), args...); err != nil {
			t.Fatalf("plugin config list %#v: %v", args, err)
		}
	}
	if _, err := run(t, newPluginConfigUnsetCommand(), "demo-plugin", "missing"); err == nil {
		t.Fatal("missing plugin config unset succeeded")
	}
	if output, err := run(t, newPluginConfigUnsetCommand(), "demo-plugin", "API_KEY"); err != nil || !strings.Contains(output, "removed") {
		t.Fatalf("plugin config unset = %q %v", output, err)
	}

	if _, err := run(t, newPluginDevCommand(), "missing"); err == nil {
		t.Fatal("missing dev plugin registration succeeded")
	}
	if output, err := run(t, newPluginDevCommand(), "demo-plugin"); err != nil || !strings.Contains(output, "registered") {
		t.Fatalf("dev plugin register = %q %v", output, err)
	}
	if output, err := run(t, newPluginDevCommand(), "demo-plugin", "--off"); err != nil || !strings.Contains(output, "unregistered") {
		t.Fatalf("dev plugin unregister = %q %v", output, err)
	}
	if _, err := run(t, newPluginDevCommand(), "missing", "--off"); err == nil {
		t.Fatal("missing dev plugin unregister succeeded")
	}

	if _, err := run(t, newPluginBuildCommand(), "missing"); err == nil {
		t.Fatal("missing plugin build succeeded")
	}
	if _, err := run(t, newPluginBuildCommand(), "demo-plugin"); err == nil {
		t.Fatal("scaffold placeholder build unexpectedly succeeded")
	}
	buildDir := filepath.Join(work, "build-plugin")
	if err := os.MkdirAll(buildDir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `{"name":"build-plugin","version":"0.1.0","type":"user","build":{"command":"mkdir -p bin && touch bin/server","output":"bin/server"}}`
	if err := os.WriteFile(filepath.Join(buildDir, "plugin.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := run(t, newPluginBuildCommand(), buildDir); err != nil || !strings.Contains(output, "succeeded") {
		t.Fatalf("plugin build = %q %v", output, err)
	}

	loader := plugin.NewLoader(RawVersion())
	if got := loadDeclaredUserConfig(loader, "missing"); got != nil {
		t.Fatalf("missing declared config = %#v", got)
	}
	for _, value := range []string{"", "short", "abcdefghijk"} {
		_ = maskSensitiveValue(value)
	}
	if statusStr(true) != "enabled" || statusStr(false) != "disabled" {
		t.Fatal("plugin status labels mismatch")
	}
	_ = newPluginCommand()
	_ = newPluginConfigCommand()

	if output, err := run(t, newPluginRemoveCommand(), "demo-plugin", "--keep-data"); err != nil || !strings.Contains(output, "removed") {
		t.Fatalf("plugin remove = %q %v", output, err)
	}
	if _, err := run(t, newPluginRemoveCommand(), "missing"); err == nil {
		t.Fatal("missing plugin remove succeeded")
	}
	if output, err := run(t, newPluginListCommand()); err != nil || !strings.Contains(output, "No plugins") {
		t.Fatalf("empty plugin list = %q %v", output, err)
	}
}

func TestCrossPlatformCoverageUpgradeCommandHTTPAndDryRunCoverage(t *testing.T) {
	oldEdition := edition.Get()
	oldVersion := version
	t.Cleanup(func() { edition.Override(oldEdition); version = oldVersion })
	t.Setenv("HOME", t.TempDir())
	assetName := "dws-" + runtime.GOOS + "-" + runtime.GOARCH + ".tar.gz"
	if runtime.GOOS == "windows" {
		assetName = "dws-" + runtime.GOOS + "-" + runtime.GOARCH + ".zip"
	}
	releases := []upgradepkg.GitHubRelease{
		{TagName: "v9.9.9-beta.1", Prerelease: true, PublishedAt: "2026-01-02T03:04:05Z", Body: "* abcdef1 - beta change", Assets: []upgradepkg.GitHubAsset{{Name: assetName}}},
		{TagName: "v9.9.9", PublishedAt: "2026-01-01T03:04:05Z", Body: "* abcdef1 - stable change", HTMLURL: "https://release.test", Assets: []upgradepkg.GitHubAsset{{Name: assetName}, {Name: "dws-skills.zip"}}},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/releases/tags/") {
			if len(releases) == 0 {
				http.NotFound(w, r)
				return
			}
			_ = json.NewEncoder(w).Encode(releases[len(releases)-1])
			return
		}
		_ = json.NewEncoder(w).Encode(releases)
	}))
	defer server.Close()
	t.Setenv("DWS_UPGRADE_URL", server.URL)
	t.Setenv("DWS_UPGRADE_REPOSITORY", "owner/repo")

	cmd := &cobra.Command{Use: "upgrade"}
	var output bytes.Buffer
	cmd.SetOut(&output)
	for _, format := range []string{"json", "table"} {
		for _, track := range []upgradepkg.ReleaseTrack{upgradepkg.ReleaseTrackRelease, upgradepkg.ReleaseTrackBeta} {
			output.Reset()
			version = "1.0.0"
			if err := runUpgradeCheck(cmd, format, track); err != nil {
				t.Fatalf("upgrade check %s/%s: %v", format, track, err)
			}
			if err := runUpgradeList(cmd, format, 1, track); err != nil {
				t.Fatalf("upgrade list %s/%s: %v", format, track, err)
			}
			if err := runUpgradeList(cmd, format, 0, track); err != nil {
				t.Fatalf("upgrade list all %s/%s: %v", format, track, err)
			}
		}
	}
	version = "9.9.9"
	if err := runUpgradeCheck(cmd, "table", upgradepkg.ReleaseTrackRelease); err != nil {
		t.Fatal(err)
	}
	releases = nil
	if err := runUpgradeList(cmd, "table", 10, upgradepkg.ReleaseTrackRelease); err != nil {
		t.Fatal(err)
	}
	releases = []upgradepkg.GitHubRelease{{TagName: "v9.9.9", Assets: []upgradepkg.GitHubAsset{{Name: assetName}, {Name: "dws-skills.zip"}}}}
	version = "1.0.0"
	if err := runUpgrade(context.Background(), upgradeOptions{dryRun: true, track: upgradepkg.ReleaseTrackRelease}); err != nil {
		t.Fatalf("upgrade dry-run: %v", err)
	}
	if err := runUpgrade(context.Background(), upgradeOptions{dryRun: true, targetVersion: "v9.9.9", skipSkills: true}); err != nil {
		t.Fatalf("target upgrade dry-run: %v", err)
	}
	releases[0].Assets = nil
	if err := runUpgrade(context.Background(), upgradeOptions{dryRun: true, force: true}); err == nil {
		t.Fatal("upgrade dry-run without binary succeeded")
	}
	server.Close()
	if err := runUpgradeCheck(cmd, "json", upgradepkg.ReleaseTrackRelease); err == nil {
		t.Fatal("upgrade network failure succeeded")
	}
	if err := runUpgradeList(cmd, "json", 1, upgradepkg.ReleaseTrackRelease); err == nil {
		t.Fatal("upgrade list network failure succeeded")
	}
	if err := runUpgrade(context.Background(), upgradeOptions{dryRun: true}); err == nil {
		t.Fatal("upgrade network failure succeeded")
	}
	if err := runUpgradeRollback(true); err == nil {
		t.Fatal("rollback without backup succeeded")
	}

	edition.Override(&edition.Hooks{IsEmbedded: true, Name: "host"})
	upgradeCmd := newUpgradeCommand()
	upgradeCmd.SetArgs(nil)
	if err := upgradeCmd.Execute(); err == nil || !strings.Contains(err.Error(), "嵌入") {
		t.Fatalf("embedded upgrade = %v", err)
	}
	edition.Override(&edition.Hooks{})
	upgradeCmd = newUpgradeCommand()
	upgradeCmd.SetArgs([]string{"--beta", "--version", "v1.0.0"})
	if err := upgradeCmd.Execute(); err == nil {
		t.Fatal("conflicting upgrade track succeeded")
	}
}

func TestCrossPlatformCoverageUpgradeRemainingPureCoverage(t *testing.T) {
	originalCommandOutput := upgradeCommandOutput
	t.Cleanup(func() { upgradeCommandOutput = originalCommandOutput })
	for _, beta := range []bool{false, true} {
		track := upgradeTrack(beta)
		_ = upgradeTrackSuffix(track)
		_ = upgradeTrackVersionName(track)
		_ = upgradeHintForTrack(track)
	}
	var output bytes.Buffer
	writeDryRunPlan(&output, "1.0.0", "binary.tar.gz", false)
	writeDryRunPlan(&output, "1.0.0", "binary.tar.gz", true)
	if output.Len() == 0 {
		t.Fatal("empty upgrade dry-run plan")
	}
	root := &cobra.Command{Use: "root"}
	root.PersistentFlags().String("format", "table", "")
	child := &cobra.Command{Use: "child"}
	root.AddCommand(child)
	if resolveUpgradeFormat(child) != "table" {
		t.Fatal("default upgrade format mismatch")
	}
	_ = root.PersistentFlags().Set("format", " JSON ")
	if resolveUpgradeFormat(child) != "json" {
		t.Fatal("JSON upgrade format mismatch")
	}
	if err := writeJSON(io.Discard, map[string]any{"ok": true}); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	archive := filepath.Join(dir, "archive.tar.gz")
	file, err := os.Create(archive)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	body := []byte("data")
	_ = tw.WriteHeader(&tar.Header{Name: "nested/file", Mode: 0o600, Size: int64(len(body))})
	_, _ = tw.Write(body)
	_ = tw.Close()
	_ = gz.Close()
	_ = file.Close()
	dest := filepath.Join(dir, "dest")
	if runtime.GOOS == "windows" {
		upgradeCommandOutput = func(_ string, args ...string) ([]byte, error) {
			for _, arg := range args {
				if strings.Contains(arg, "missing") {
					return nil, os.ErrNotExist
				}
			}
			return nil, nil
		}
	}
	if err := extractTarGz(archive, dest); err != nil {
		t.Fatalf("extract tar: %v", err)
	}
	if err := extractTarGz(filepath.Join(dir, "missing"), filepath.Join(dir, "bad")); err == nil {
		t.Fatal("missing tar extraction succeeded")
	}
	_ = shortenHome(filepath.Join(os.Getenv("HOME"), "path"))
}

func TestCrossPlatformCoveragePersonalEventPureCoverage(t *testing.T) {
	cmd := &cobra.Command{Use: "personal"}
	var out bytes.Buffer
	cmd.SetOut(&out)
	for _, opts := range []personalListOptions{
		{Format: "json", IncludePending: true},
		{Format: "table", Category: "im", EnabledOnly: true},
	} {
		out.Reset()
		if err := runPersonalEventList(cmd, opts); err != nil || out.Len() == 0 {
			t.Fatalf("personal list %#v = %q, %v", opts, out.String(), err)
		}
	}
	def, ok := personal.Lookup(personal.EventMention)
	if !ok {
		t.Fatal("mention definition missing")
	}
	if err := renderPersonalSchema(io.Discard, def, ""); err != nil {
		t.Fatal(err)
	}
	if err := renderPersonalSchema(io.Discard, def, "yaml"); err == nil {
		t.Fatal("unsupported schema format succeeded")
	}
	for _, key := range []string{"", "unknown", personal.EventMention, personal.EventFromUser} {
		if err := ensurePublicPersonalEvent(key); err != nil {
			t.Fatalf("public event %q: %v", key, err)
		}
	}

	var cfg consume.Config
	applyPersonalConsumeFilters(nil, personalConsumeOptions{}, "", "")
	applyPersonalConsumeFilters(&cfg, personalConsumeOptions{
		Common: personalCommonOptions([]string{"explicit"}, "filter"),
	}, " sub ", personal.EventMention)
	if len(cfg.EventTypes) != 1 || cfg.SubscribeID != "sub" || cfg.Filter != "filter" {
		t.Fatalf("personal filters = %#v", cfg)
	}
	applyPersonalConsumeFilters(&cfg, personalConsumeOptions{DebugRawEvents: true}, "sub", personal.EventMention)
	if cfg.EventTypes != nil || cfg.SubscribeID != "" || cfg.Filter != "" {
		t.Fatalf("debug filters = %#v", cfg)
	}

	for _, tc := range []struct {
		explicit []string
		key      string
		wantNil  bool
	}{
		{nil, "", true},
		{nil, personal.EventMention, false},
		{[]string{"x"}, personal.EventMention, false},
	} {
		got := personalEventTypes(tc.key, tc.explicit)
		if (got == nil) != tc.wantNil {
			t.Errorf("personalEventTypes(%q, %#v) = %#v", tc.key, tc.explicit, got)
		}
	}
	identity := personal.Identity{CorpID: "corp", UserID: "user", ClientID: "client", SourceID: "source"}
	if len(redactedPersonalIdentity(identity, "hash")) != 5 || displayIdentityPart(" ") != "unknown" || displayIdentityPart(" value ") != "value" {
		t.Fatal("personal identity display mismatch")
	}
	if firstNonEmptyPersonalString(" ", " x ") != "x" || firstNonEmptyPersonalString(" ") != "" {
		t.Fatal("first nonempty mismatch")
	}
	if personalTokenSubject("access", "") != "" || !strings.HasPrefix(personalTokenSubject(" access ", " token "), "access:") {
		t.Fatal("personal token subject mismatch")
	}
	if displayPersonalStatusValue(" ") != "-" || displayPersonalStatusValue(" x ") != "x" {
		t.Fatal("personal status display mismatch")
	}

	args := personalBusSpawnArgs(identity, "custom", "https://ticket")
	if len(args) != 10 || len(personalBusSpawnArgs(identity, "", "")) != 6 {
		t.Fatalf("personal bus args = %#v", args)
	}
	if personalEventControlBaseURL(" https://control/ ", "") != "https://control" || personalEventStreamTicketURL(" https://ticket/ ", "") != "https://ticket" {
		t.Fatal("personal explicit URL mismatch")
	}
	if personalEventStreamSourceID(" explicit ") != "explicit" {
		t.Fatal("personal explicit source mismatch")
	}
	emptyDir := t.TempDir()
	if configuredMCPBaseURL(emptyDir) != "" {
		t.Fatal("missing MCP URL unexpectedly resolved")
	}
	if err := os.WriteFile(filepath.Join(emptyDir, "mcp_url"), []byte(" https://mcp.test/ \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if configuredMCPBaseURL(emptyDir) != "https://mcp.test/" || personalEventMCPBaseURL(emptyDir) != "https://mcp.test" {
		t.Fatal("configured MCP URL mismatch")
	}

	var rendered bytes.Buffer
	printPersonalStopResult(&rendered, []string{"sub"}, true, "stopped")
	printPersonalStopResult(&rendered, []string{"a", "b"}, false, "running")
	qs := busctl.EntryStatus{Entry: busctl.BusEntry{WorkDir: "work", State: busctl.BusStateRunning, HolderPID: 42}, Live: &eventtransport.StatusResp{}}
	renderPersonalStatusText(&rendered, identity, "hash", []personal.Subscription{{SubscribeID: "sub", EventKey: personal.EventMention, Status: "active"}}, qs)
	if !strings.Contains(rendered.String(), "SUBSCRIBE_ID") || !strings.Contains(rendered.String(), "cancelled") {
		t.Fatalf("personal rendering = %q", rendered.String())
	}
	renderPersonalConsumers(io.Discard, busctl.EntryStatus{Entry: busctl.BusEntry{State: busctl.BusStateRunning}, Live: &eventtransport.StatusResp{Consumers: []eventtransport.StatusConsumer{{PID: 7}}}})

	if got, err := personalStopTargets(t.TempDir(), " sub ", false); err != nil || len(got) != 1 || got[0] != "sub" {
		t.Fatalf("explicit stop targets = %#v, %v", got, err)
	}
	if _, err := personalStopTargets(t.TempDir(), "sub", true); err == nil {
		t.Fatal("conflicting stop target succeeded")
	}
	if _, err := personalStopTargets(t.TempDir(), "", false); err == nil {
		t.Fatal("missing stop target succeeded")
	}
	workDir := t.TempDir()
	for _, id := range []string{"b", "a"} {
		if err := personal.UpsertRunState(workDir, personal.RunState{SubscribeID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := personalStopTargets(workDir, "", true); err != nil || strings.Join(got, ",") != "a,b" {
		t.Fatalf("all stop targets = %#v, %v", got, err)
	}
	if err := interruptPersonalConsumers("", []string{"sub"}); err != nil {
		t.Fatal(err)
	}
	if err := interruptPersonalConsumers("bad-endpoint", nil); err != nil {
		t.Fatal(err)
	}
	if err := interruptPersonalConsumers("bad-endpoint", []string{"sub"}); err != nil {
		t.Fatal(err)
	}
}

func personalCommonOptions(eventTypes []string, filter string) commonConsumeOptions {
	return commonConsumeOptions{EventTypes: eventTypes, Filter: filter}
}

func TestCrossPlatformCoveragePersonalSubscriptionAndSourceCoverage(t *testing.T) {
	identity := personal.Identity{AccessToken: "token", CorpID: "corp", UserID: "user", ClientID: "client", SourceID: "source"}
	var createCount, cancelCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/event/sublist":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{{"subId": "existing", "eventKey": personal.EventMention, "ruleType": "at", "status": "active"}}, "total": 1})
		case "/subscription/user":
			createCount++
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []string{"created"}})
		case "/subscription/cancel":
			cancelCount++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := personal.NewClient(server.URL, identity)

	for _, tc := range []struct {
		name string
		opts personalConsumeOptions
		ok   bool
	}{
		{"lookup", personalConsumeOptions{SubscribeID: "existing"}, true},
		{"create", personalConsumeOptions{EventKey: personal.EventMention, Name: "name", FilterJSON: `{"field":"content","op":"eq","value":"x"}`, QueryCSV: "a,b", TTL: time.Minute}, true},
		{"missing", personalConsumeOptions{}, false},
		{"sender", personalConsumeOptions{EventKey: personal.EventFromUser, UserID: "u"}, true},
		{"bad-rule", personalConsumeOptions{EventKey: personal.EventMention, Rule: "other"}, false},
		{"bad-filter", personalConsumeOptions{EventKey: personal.EventMention, FilterJSON: "{"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sub, key, rule, err := ensurePersonalSubscription(context.Background(), client, identity, tc.opts)
			if tc.ok && (err != nil || sub == nil || key == "" || rule == "") {
				t.Fatalf("subscription = %#v %q %q, %v", sub, key, rule, err)
			}
			if !tc.ok && err == nil {
				t.Fatal("invalid subscription succeeded")
			}
		})
	}
	if createCount != 2 {
		t.Fatalf("create count = %d", createCount)
	}

	dir := t.TempDir()
	if _, err := newPersonalStreamSource(context.Background(), personalStreamSourceOptions{ConfigDir: dir, Identity: identity, TicketMode: "bad"}); err == nil {
		t.Fatal("invalid ticket mode succeeded")
	}
	if src, err := newPersonalStreamSource(context.Background(), personalStreamSourceOptions{ConfigDir: dir, Identity: identity}); err != nil || src == nil {
		t.Fatalf("normal source = %#v, %v", src, err)
	}
	t.Setenv(authpkg.EnvClientID, "resolved-client")
	t.Setenv(authpkg.EnvClientSecret, "secret")
	if src, err := newPersonalStreamSource(context.Background(), personalStreamSourceOptions{ConfigDir: dir, Identity: personal.Identity{AccessToken: "token", SourceID: "source"}, TicketMode: "custom", ClientIDOverride: "override"}); err != nil || src == nil {
		t.Fatalf("custom source = %#v, %v", src, err)
	}
	if cancelCount != 0 {
		t.Fatalf("unexpected cancel count = %d", cancelCount)
	}
}

func TestCrossPlatformCoveragePersonalEventCommandRuntimeCoverage(t *testing.T) {
	configDir := setupPersonalIdentityToken(t, &authpkg.TokenData{
		AccessToken: "access", RefreshToken: "refresh", ExpiresAt: time.Now().Add(time.Hour),
		CorpID: "corp", UserID: "user", ClientID: "client",
	})
	t.Setenv("DWS_CONFIG_DIR", configDir)
	var cancelCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/event/sublist":
			_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{{"subId": "sub", "eventKey": personal.EventMention, "ruleType": "at", "status": "active", "sourceId": "open"}}, "total": 1})
		case "/subscription/user":
			_ = json.NewEncoder(w).Encode(map[string]any{"success": true, "result": []string{"created"}})
		case "/subscription/cancel":
			cancelCount++
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	newCmd := func() (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
		cmd := &cobra.Command{Use: "event"}
		cmd.SetContext(context.Background())
		out, errOut := &bytes.Buffer{}, &bytes.Buffer{}
		cmd.SetOut(out)
		cmd.SetErr(errOut)
		cmd.Flags().String("format", "table", "")
		cmd.Flags().String("output", "", "")
		return cmd, out, errOut
	}
	for _, format := range []string{"json", "table"} {
		cmd, out, _ := newCmd()
		if err := runPersonalEventStatus(cmd, personalStatusOptions{Format: format, ControlBaseURL: server.URL}); err != nil || out.Len() == 0 {
			t.Fatalf("personal status %s = %q, %v", format, out.String(), err)
		}
	}
	cmd, _, _ := newCmd()
	if err := runPersonalEventConsume(cmd, personalConsumeOptions{Common: commonConsumeOptions{DryRun: true}, EventKey: personal.EventMention, ControlBaseURL: server.URL}); err != nil {
		t.Fatalf("personal dry-run consume: %v", err)
	}
	cmd, _, _ = newCmd()
	if err := runPersonalEventConsume(cmd, personalConsumeOptions{Common: commonConsumeOptions{Foreground: true}, EventKey: personal.EventMention, ControlBaseURL: server.URL, StreamTicketMode: "invalid"}); err == nil {
		t.Fatal("invalid foreground consume succeeded")
	}
	if cancelCount == 0 {
		t.Fatal("failed foreground consume did not clean up subscription")
	}

	if err := runPersonalEventStop(cmd, personalStopOptions{SubscribeID: "sub", All: true, ControlBaseURL: server.URL}); err == nil {
		t.Fatal("conflicting stop succeeded")
	}
	if err := runPersonalEventStop(cmd, personalStopOptions{ControlBaseURL: server.URL}); err == nil {
		t.Fatal("missing stop target succeeded")
	}
	if err := runPersonalEventStop(cmd, personalStopOptions{SubscribeID: "sub", ControlBaseURL: server.URL}); err != nil {
		t.Fatalf("single stop: %v", err)
	}
}

func TestCrossPlatformCoverageDoctorCommandCoverage(t *testing.T) {
	configDir := setupPersonalIdentityToken(t, &authpkg.TokenData{
		AccessToken: "access", ExpiresAt: time.Now().Add(time.Hour), ClientID: "client",
	})
	t.Setenv("DWS_CONFIG_DIR", configDir)
	home := t.TempDir()
	oldVersion := version
	oldDiagnose := doctorKeychainDiagnose
	oldTimingHome := timingUserHomeDir
	timingUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() {
		version = oldVersion
		doctorKeychainDiagnose = oldDiagnose
		timingUserHomeDir = oldTimingHome
	})
	version = "1.0.0"
	doctorKeychainDiagnose = func() keychain.Diagnostic {
		return keychain.Diagnostic{OK: true, Message: "available"}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/releases/latest") {
			_ = json.NewEncoder(w).Encode(map[string]any{"tag_name": "v1.0.0"})
			return
		}
		_, _ = io.WriteString(w, "ok")
	}))
	defer server.Close()
	if err := os.WriteFile(filepath.Join(configDir, "mcp_url"), []byte(server.URL), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DWS_UPGRADE_URL", server.URL)
	t.Setenv("DWS_UPGRADE_REPOSITORY", "owner/repo")
	t.Setenv(PerfReportEnv, "auto")
	tc := NewTimingCollector()
	tc.Record("phase", 2*time.Millisecond)
	tc.WriteReportIfEnabled("1.0.0", "doctor-test")

	for _, args := range [][]string{{"--json", "--perf", "--timeout", "1"}, {"--timeout", "1"}} {
		cmd := newDoctorCommand()
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		var out bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetArgs(args)
		if err := cmd.Execute(); err != nil || out.Len() == 0 {
			t.Fatalf("doctor %#v = %q, %v", args, out.String(), err)
		}
	}

	for _, jsonOut := range []bool{false, true} {
		if got := doctorCheckCache(io.Discard, jsonOut); got.Status != statusPass {
			t.Fatal("cache check failed")
		}
		if got := doctorCheckPerf(io.Discard, jsonOut); got.Status != statusPass {
			t.Fatalf("perf check = %#v", got)
		}
	}
	if p, w, f := countResults([]checkResult{{Status: statusPass}, {Status: statusWarn}, {Status: statusFail}, {Status: "other"}}); p != 1 || w != 1 || f != 1 {
		t.Fatalf("doctor counts = %d/%d/%d", p, w, f)
	}
	for _, status := range []checkStatus{statusPass, statusWarn, statusFail, "other"} {
		_ = statusIcon(status)
		printCheckResult(io.Discard, checkResult{Status: status, Message: "message", Hint: "hint"})
	}
	printPerfReportSummary(io.Discard, &PerfReport{Command: "cmd", Timestamp: time.Now(), Phases: []PerfPhase{{Name: "slow", DurationMs: 3}}, Slowest: "slow", TotalMs: 4, OverheadMs: 1})

	doctorKeychainDiagnose = func() keychain.Diagnostic {
		return keychain.Diagnostic{OK: false, Message: "bad", Hint: "fix", Reason: "reason"}
	}
	if got := doctorCheckKeychain(io.Discard, false); got.Status != statusFail {
		t.Fatal("keychain failure not reported")
	}
	doctorKeychainDiagnose = func() keychain.Diagnostic {
		return keychain.Diagnostic{OK: false, Message: "bad", Reason: "reason", Detail: map[string]string{"backend": "test"}}
	}
	_ = doctorCheckKeychain(io.Discard, true)

	badConfig := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", badConfig)
	if err := os.WriteFile(filepath.Join(badConfig, "mcp_url"), []byte("://bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := doctorCheckNetwork(context.Background(), io.Discard, false, time.Second); got.Status != statusFail {
		t.Fatalf("bad network check = %#v", got)
	}
	t.Setenv("DWS_UPGRADE_URL", "http://127.0.0.1:1")
	if got := doctorCheckVersion(io.Discard, false, time.Millisecond); got.Status != statusFail {
		t.Fatalf("bad version check = %#v", got)
	}
	missingHome := t.TempDir()
	timingUserHomeDir = func() (string, error) { return missingHome, nil }
	if got := doctorCheckPerf(io.Discard, false); got.Status != statusWarn {
		t.Fatalf("missing perf check = %#v", got)
	}
}

func coverageSkillZip(t *testing.T) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	dir, err := zw.Create("demo/")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = dir.Write(nil)
	f, err := zw.Create("demo/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(f, "# demo")
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestCrossPlatformCoverageSkillCommandHTTPCoverage(t *testing.T) {
	configDir := setupPersonalIdentityToken(t, &authpkg.TokenData{AccessToken: "access", ExpiresAt: time.Now().Add(time.Hour), ClientID: "client"})
	t.Setenv("DWS_CONFIG_DIR", configDir)
	home := t.TempDir()
	oldSkillHome := skillUserHomeDir
	skillUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { skillUserHomeDir = oldSkillHome })
	zipData := coverageSkillZip(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cli/find-skills":
			switch r.URL.Query().Get("keyword") {
			case "empty":
				_ = json.NewEncoder(w).Encode(findSkillsResponse{Success: true})
			case "business":
				_ = json.NewEncoder(w).Encode(findSkillsResponse{ErrorCode: "E"})
			case "malformed":
				_, _ = io.WriteString(w, "{")
			default:
				_ = json.NewEncoder(w).Encode(findSkillsResponse{Success: true, Result: []CliSkillDTO{{SkillID: "id", Name: "name", Desc: "desc"}}})
			}
		case "/cli/install", "/package.zip":
			w.Header().Set("Content-Disposition", `attachment; filename="demo.zip"`)
			_, _ = w.Write(zipData)
		case "/info":
			_ = json.NewEncoder(w).Encode(downloadSkillResponse{Success: true, Result: &downloadSkillResult{DownloadURL: serverURL(r) + "/package.zip", FileName: "demo.zip"}})
		case "/unauthorized":
			w.WriteHeader(http.StatusUnauthorized)
		case "/bad-request":
			w.WriteHeader(http.StatusBadRequest)
		case "/not-found":
			w.WriteHeader(http.StatusNotFound)
		case "/failure":
			w.WriteHeader(http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Setenv("DWS_SKILL_API_HOST", server.URL+"/")
	oldEndpoint := skillDownloadEndpoint
	skillDownloadEndpoint = server.URL + "/info"
	t.Cleanup(func() { skillDownloadEndpoint = oldEndpoint })

	run := func(cmd *cobra.Command, args ...string) (string, error) {
		var out bytes.Buffer
		cmd.SilenceUsage = true
		cmd.SilenceErrors = true
		cmd.SetOut(&out)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return out.String(), err
	}
	for _, args := range [][]string{{"--query", "demo", "--source", "DingtalkMarket"}, {"--query", "empty"}} {
		if output, err := run(newSkillSearchCommand(), args...); err != nil || output == "" {
			t.Fatalf("skill search %#v = %q, %v", args, output, err)
		}
	}
	for _, query := range []string{"business", "malformed"} {
		if _, err := run(newSkillSearchCommand(), "--query", query); err == nil {
			t.Fatalf("skill search %s succeeded", query)
		}
	}
	if output, err := run(newSkillGetCommand(), "--skill-id", "id"); err != nil || !strings.Contains(output, "dws-skill-") {
		t.Fatalf("skill get = %q, %v", output, err)
	}
	if output, err := run(newSkillInstallCommand(), "id", "agents"); err != nil || !strings.Contains(output, "技能安装成功") {
		t.Fatalf("skill install = %q, %v", output, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "demo", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	for _, cmd := range []*cobra.Command{buildSkillCommand(), newSkillFindHintCommand(), newSkillAddHintCommand()} {
		_, _ = run(cmd)
	}
	if _, err := run(newSkillInstallCommand(), "", "agents"); err == nil {
		t.Fatal("empty skill ID succeeded")
	}
	if _, err := run(newSkillInstallCommand(), "id", "missing"); err == nil {
		t.Fatal("bad target succeeded")
	}
}

func serverURL(r *http.Request) string {
	return "http://" + r.Host
}

func TestCrossPlatformCoverageSkillHelpersAndErrorsCoverage(t *testing.T) {
	oldEmbedded := edition.Get()
	t.Cleanup(func() { edition.Override(oldEmbedded) })
	for _, embedded := range []bool{false, true} {
		edition.Override(&edition.Hooks{IsEmbedded: embedded})
		_ = skillAuthError()
	}
	t.Setenv("DWS_SKILL_API_HOST", " https://host.test/ ")
	if skillAPIHost() != "https://host.test" || supportedTargets() == "" || longestAgentTargetName() == 0 || formatAgentSkillPathsForHelp() == "" {
		t.Fatal("skill helper mismatch")
	}
	if _, err := resolveSkillTargetPath(""); err == nil {
		t.Fatal("empty target succeeded")
	}
	if _, err := resolveSkillTargetPath("missing"); err == nil {
		t.Fatal("unknown target succeeded")
	}
	if got, err := resolveSkillTargetPath("."); err != nil || got == "" {
		t.Fatalf("dot target = %q, %v", got, err)
	}
	if filenameFromDisposition(`attachment; filename="named.zip"`) != "named.zip" || filenameFromDisposition("bad;") != "skill.zip" || filenameFromDisposition("") != "skill.zip" {
		t.Fatal("content disposition parsing mismatch")
	}
	for _, status := range []int{http.StatusUnauthorized, http.StatusBadRequest, http.StatusNotFound, http.StatusInternalServerError} {
		_ = parseLegacySkillAPIError(&http.Response{StatusCode: status})
	}
	cleanupTempFile("")
	missing := filepath.Join(t.TempDir(), "missing.zip")
	cleanupTempFile(missing)
	if err := extractSkillZip(missing, t.TempDir()); err == nil {
		t.Fatal("missing skill zip succeeded")
	}
	zipPath := filepath.Join(t.TempDir(), "bad.zip")
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, _ := zw.Create("../escape")
	_, _ = io.WriteString(f, "bad")
	_ = zw.Close()
	if err := os.WriteFile(zipPath, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := extractSkillZip(zipPath, t.TempDir()); err == nil {
		t.Fatal("zip slip succeeded")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = io.WriteString(w, "data")
		case "/unauthorized":
			w.WriteHeader(http.StatusUnauthorized)
		case "/bad":
			w.WriteHeader(http.StatusBadRequest)
		case "/missing":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	if p, err := downloadSkillFile(context.Background(), server.URL+"/ok", ""); err != nil {
		t.Fatal(err)
	} else {
		cleanupTempFile(p)
	}
	if _, err := downloadSkillFile(context.Background(), server.URL+"/bad", "x"); err == nil {
		t.Fatal("bad download succeeded")
	}
	for _, path := range []string{"unauthorized", "bad", "missing", "failure"} {
		if _, err := downloadSkillToTmpDir(context.Background(), server.URL+"/"+path, "token"); err == nil {
			t.Fatalf("legacy download %s succeeded", path)
		}
	}
	if _, err := downloadSkillToTmpDir(context.Background(), "://bad", "token"); err == nil {
		t.Fatal("invalid legacy URL succeeded")
	}
	if _, err := downloadSkillFile(context.Background(), "://bad", "x"); err == nil {
		t.Fatal("invalid download URL succeeded")
	}
}

func TestCrossPlatformCoverageProfileCommandAndModelCoverage(t *testing.T) {
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	now := time.Now().UTC()
	cfg := &authpkg.ProfilesConfig{
		Version: 1, PrimaryProfile: "corp-a", CurrentProfile: "corp-a", PreviousProfile: "corp-b",
		Profiles: []authpkg.Profile{
			{Name: "alpha", CorpID: "corp-a", CorpName: "Alpha 组织", UserID: "u-a", UserName: "Alice", LastLoginAt: now.Add(-time.Hour).Format(time.RFC3339)},
			{Name: "beta", CorpID: "corp-b", UserID: "u-b", UpdatedAt: now.Format(time.RFC3339), Status: authpkg.ProfileStatusExpired},
			{Name: "gamma", CorpID: "corp-c", LastUsedAt: "bad-time"},
		},
	}
	if err := authpkg.SaveProfiles(configDir, cfg); err != nil {
		t.Fatal(err)
	}
	for _, profile := range cfg.Profiles {
		data := &authpkg.TokenData{
			AccessToken: "token-" + profile.CorpID,
			CorpID:      profile.CorpID,
			UserID:      profile.UserID,
		}
		var err error
		if profile.UserID == "" {
			err = authpkg.SaveTokenDataKeychainForCorpID(profile.CorpID, data)
		} else {
			err = authpkg.SaveTokenDataKeychainForIdentity(profile.CorpID, profile.UserID, data)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, profile := range cfg.Profiles {
			if profile.UserID == "" {
				_ = authpkg.DeleteTokenDataKeychainForCorpID(profile.CorpID)
			} else {
				_ = authpkg.DeleteTokenDataKeychainForIdentity(profile.CorpID, profile.UserID)
			}
		}
	})
	oldInteractive := profileSwitchInteractiveTerminal
	oldSelector := profileSwitchSelector
	t.Cleanup(func() {
		profileSwitchInteractiveTerminal = oldInteractive
		profileSwitchSelector = oldSelector
	})
	profileSwitchInteractiveTerminal = func() bool { return false }

	run := func(cmd *cobra.Command, args ...string) (string, error) {
		root := &cobra.Command{Use: "root"}
		root.PersistentFlags().String("format", "table", "")
		root.AddCommand(cmd)
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(io.Discard)
		root.SilenceUsage = true
		root.SilenceErrors = true
		root.SetArgs(append([]string{cmd.Name()}, args...))
		err := root.Execute()
		return out.String(), err
	}
	if output, err := run(newProfileListCommand()); err != nil || !strings.Contains(output, "corp-a") {
		t.Fatalf("profile list = %q, %v", output, err)
	}
	rootJSON := &cobra.Command{Use: "root"}
	rootJSON.PersistentFlags().String("format", "json", "")
	listJSON := newProfileListCommand()
	rootJSON.AddCommand(listJSON)
	var out bytes.Buffer
	rootJSON.SetOut(&out)
	rootJSON.SetArgs([]string{"list"})
	if err := rootJSON.Execute(); err != nil || !strings.Contains(out.String(), `"profiles"`) {
		t.Fatalf("profile JSON = %q, %v", out.String(), err)
	}
	if output, err := run(newProfileSwitchCommand(), "corp-b"); err != nil || !strings.Contains(output, "corp-b") {
		t.Fatalf("profile switch = %q, %v", output, err)
	}
	if output, err := run(newProfileUseCommand(), "-"); err != nil || !strings.Contains(output, "corp-a") {
		t.Fatalf("profile previous = %q, %v", output, err)
	}
	if _, err := run(newProfileSwitchCommand()); err == nil {
		t.Fatal("noninteractive empty profile switch succeeded")
	}
	profileSwitchSelector = func(*cobra.Command, string) (string, error) { return "corp-c", nil }
	if output, err := run(newProfileSwitchCommand()); err != nil || !strings.Contains(output, "corp-c") {
		t.Fatalf("profile injected selection = %q, %v", output, err)
	}
	if _, err := run(newProfileSwitchCommand(), "missing"); err == nil {
		t.Fatal("missing profile switch succeeded")
	}
	_, _ = run(newProfileCommand())

	selectorCmd := newProfileSwitchCommand()
	if got, err := profileSwitchSelectorFromCommand(selectorCmd, nil); err != nil || got != "" {
		t.Fatalf("empty selector = %q, %v", got, err)
	}
	_ = selectorCmd.Flags().Set("corpId", "corp-a")
	if got, err := profileSwitchSelectorFromCommand(selectorCmd, []string{"corp-a"}); err != nil || got != "corp-a" {
		t.Fatalf("matching selectors = %q, %v", got, err)
	}
	_ = selectorCmd.Flags().Set("name", "beta")
	if _, err := profileSwitchSelectorFromCommand(selectorCmd, []string{"corp-a"}); err == nil {
		t.Fatal("conflicting profile selectors succeeded")
	}
	if _, changed := changedStringFlag(nil, "x"); changed {
		t.Fatal("nil command flag changed")
	}

	model := newProfileSwitchTUIModel(cfg, "corp-a")
	_ = model.Init()
	if model.View() == "" || model.tableView() == "" || model.selectedCorpID() == "" {
		t.Fatal("profile model rendering empty")
	}
	for _, key := range []tea.KeyType{tea.KeyDown, tea.KeyUp, tea.KeyEnter, tea.KeyEsc} {
		updated, _ := model.Update(tea.KeyMsg{Type: key})
		model = updated.(profileSwitchTUIModel)
	}
	updated, _ := model.Update(struct{}{})
	model = updated.(profileSwitchTUIModel)
	for range 10 {
		model.profiles = append(model.profiles, authpkg.Profile{CorpID: fmt.Sprintf("corp-%d", len(model.profiles))})
	}
	model.selected = 99
	model.offset = -1
	model.ensureSelectedVisible()
	model.selected = -1
	model.ensureSelectedVisible()
	emptyModel := newProfileSwitchTUIModel(nil, "")
	emptyModel.ensureSelectedVisible()
	if emptyModel.selectedCorpID() != "" {
		t.Fatal("empty profile model selected a profile")
	}

	_ = profileSwitchSortedProfiles(cfg.Profiles)
	for _, p := range cfg.Profiles {
		_ = profileSwitchOptionLabel(p, cfg)
		_, _ = profileSwitchProfileCells(p, cfg)
	}
	_ = profileSwitchProfileIndex(cfg.Profiles, "missing", cfg)
	_ = profileSwitchBorder("a", "b", "c")
	_ = profileSwitchTableLine("org", "status")
	_ = profileSwitchStyledTableLine("org", "status", profileSwitchNormalRowStyle())
	_ = profileSwitchTableSeparator()
	_ = profileSwitchTableCell("organization", 4)
	_ = padProfileDisplayCell("long", 1)
	_ = profileSwitchCellWidth(3)
	_ = profileSwitchSelectedRowStyle()
	_ = profileSwitchHeaderStyle()
	_ = profileSwitchBorderStyle()
	_ = profileSwitchTitleStyle()
	_ = profileSwitchMutedStyle()

	writeProfileListTable(io.Discard, "", nil)
	writeProfileListTable(io.Discard, "", cfg)
	if err := writeProfileListJSON(io.Discard, "", cfg); err != nil || writeProfileUseJSON(io.Discard, nil, nil) != nil || writeProfileUseJSON(io.Discard, &cfg.Profiles[0], cfg) != nil {
		t.Fatal("profile JSON write failed")
	}
	_ = profileUseMessage(nil)
	for _, p := range []authpkg.Profile{{CorpID: "corp"}, {Name: "name", CorpID: "corp"}, {CorpName: "org", CorpID: "corp"}} {
		_ = profileUseMessage(&p)
		_ = profileOrgName(p)
	}
	_ = profileViews("", nil)
	_ = profileViews("", cfg)
	for _, limit := range []int{0, 2, 5, 40} {
		_ = clipProfileCell("abcdefgh", limit)
		_ = clipProfileDisplayCell("中文abcdefgh", limit)
	}
}

func TestCrossPlatformCoverageSkillSetupRuntimeCoverage(t *testing.T) {
	home := t.TempDir()
	oldSetupHome := skillSetupUserHomeDir
	skillSetupUserHomeDir = func() (string, error) { return home, nil }
	t.Cleanup(func() { skillSetupUserHomeDir = oldSetupHome })
	mono := filepath.Join(t.TempDir(), "mono")
	if err := os.MkdirAll(filepath.Join(mono, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mono, "SKILL.md"), []byte("# mono"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mono, "references", "ref.md"), []byte("ref"), 0o600); err != nil {
		t.Fatal(err)
	}
	_ = os.Symlink(filepath.Join(mono, "SKILL.md"), filepath.Join(mono, "linked.md"))
	multi := filepath.Join(t.TempDir(), "multi")
	for _, name := range []string{"dws-shared", "dingtalk-a", "dingtalk-b"} {
		dir := filepath.Join(multi, name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("# "+name), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	run := func(args ...string) (string, string, error) {
		cmd := newSkillSetupCommand()
		cmd.Flags().Bool("dry-run", false, "")
		var out, errOut bytes.Buffer
		cmd.SetOut(&out)
		cmd.SetErr(&errOut)
		cmd.SetArgs(args)
		err := cmd.Execute()
		return out.String(), errOut.String(), err
	}
	if output, _, err := run("--mode", "mono", "--source", mono, "--target", "agents", "--yes"); err != nil || !strings.Contains(output, "installed=1") {
		t.Fatalf("mono setup = %q, %v", output, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "dws", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if output, warnings, err := run("--mode", "multi", "--source", multi, "--target", "agents", "--yes", "--skill", "a"); err != nil || !strings.Contains(output, "installed=2") || warnings == "" {
		t.Fatalf("multi setup = %q / %q, %v", output, warnings, err)
	}
	if _, err := os.Stat(filepath.Join(home, ".agents", "skills", "dws-shared", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
	if output, _, err := run("--mode", "multi", "--source", multi, "--target", "agents", "--yes", "--dry-run", "--exclude", "b"); err != nil || !strings.Contains(output, "DRY-RUN") {
		t.Fatalf("multi dry run = %q, %v", output, err)
	}
	for _, args := range [][]string{
		{"--mode", "bad", "--source", mono, "--yes"},
		{"--mode", "mono", "--source", mono, "--yes", "--skill", "a"},
		{"--mode", "mono", "--source", filepath.Join(home, "missing"), "--yes"},
		{"--mode", "mono", "--source", mono, "--target", "missing", "--yes"},
		{"--mode", "multi", "--source", multi, "--yes", "--skill", "missing"},
		{"--mode", "multi", "--source", multi, "--yes", "--skill", "a", "--exclude", "b"},
	} {
		if _, _, err := run(args...); err == nil {
			t.Fatalf("invalid setup %#v succeeded", args)
		}
	}
	if _, _, err := run("--source", mono, "--target", "agents", "--yes", "--dry-run"); err != nil {
		t.Fatalf("default mono setup: %v", err)
	}
}

func TestCrossPlatformCoverageSkillSetupPureCoverage(t *testing.T) {
	all := []string{"dingtalk-a", "dingtalk-b", "dws-shared"}
	for _, tc := range []struct {
		include []string
		exclude []string
		ok      bool
	}{
		{nil, nil, true},
		{[]string{"a", "DINGTALK-A", ""}, nil, true},
		{nil, []string{"b"}, true},
		{[]string{"missing"}, nil, false},
		{nil, []string{"missing"}, false},
		{[]string{"a"}, []string{"b"}, false},
		{nil, all, false},
	} {
		if _, err := filterMultiSkillNames(all, tc.include, tc.exclude); (err == nil) != tc.ok {
			t.Errorf("filter %#v/%#v = %v", tc.include, tc.exclude, err)
		}
	}
	for _, selected := range [][]string{nil, {"dws-shared"}, {"dingtalk-a"}} {
		_ = ensureMandatorySharedSkill(selected, all)
	}
	_ = ensureMandatorySharedSkill([]string{"dingtalk-a"}, []string{"dingtalk-a"})
	for _, name := range []string{"", " A ", "DINGTALK-B"} {
		_ = normalizeMultiSkillName(name)
	}
	if _, err := listMultiSkillNames(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing multi source succeeded")
	}
	if mode, err := resolveSkillSetupMode("", true, io.Discard); err != nil || mode != skillSetupModeMono {
		t.Fatalf("default setup mode = %q, %v", mode, err)
	}
	if _, err := resolveSkillSetupMode("bad", true, io.Discard); err == nil {
		t.Fatal("bad setup mode succeeded")
	}
	for _, mode := range []string{skillSetupModeMono, skillSetupModeMulti} {
		if got, err := resolveSkillSetupMode(mode, false, io.Discard); err != nil || got != mode {
			t.Fatalf("setup mode %s = %q, %v", mode, got, err)
		}
	}

	root := t.TempDir()
	mono := filepath.Join(root, "skills", "mono")
	multi := filepath.Join(root, "skills", "multi", "dingtalk-a")
	if err := os.MkdirAll(multi, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(mono, 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(mono, "SKILL.md"), []byte("x"), 0o600)
	_ = os.WriteFile(filepath.Join(multi, "SKILL.md"), []byte("x"), 0o600)
	for _, tc := range []struct{ explicit, mode string }{{root, skillSetupModeMono}, {root, skillSetupModeMulti}, {mono, skillSetupModeMono}} {
		if got, err := resolveSkillSetupSource(tc.explicit, tc.mode); err != nil || got == "" {
			t.Fatalf("skill source %#v = %q, %v", tc, got, err)
		}
	}
	if _, err := resolveSkillSetupSource(filepath.Join(root, "missing"), skillSetupModeMono); err == nil {
		t.Fatal("bad explicit source succeeded")
	}
	_ = skillSourceCandidates(root, skillSetupModeMono)
	for _, tc := range []struct{ path, mode string }{{"", skillSetupModeMono}, {mono, skillSetupModeMono}, {filepath.Dir(multi), skillSetupModeMulti}, {root, "bad"}} {
		_ = isSkillSourceRoot(tc.path, tc.mode)
	}
	t.Setenv("HOME", t.TempDir())
	for _, tc := range []struct{ target, mode string }{{"agents", skillSetupModeMono}, {"agents", skillSetupModeMulti}, {"all", skillSetupModeMono}, {"missing", skillSetupModeMono}} {
		_, _ = resolveSkillSetupTargets(tc.target, tc.mode)
	}
	_ = agentHomeForMode("base", skillSetupModeMono)
	_ = agentHomeForMode("base", skillSetupModeMulti)
	_ = detectExistingAgentHomes(t.TempDir(), skillSetupModeMono)
	for _, mode := range []string{skillSetupModeMono, skillSetupModeMulti, "bad"} {
		_, _ = confirmSkillSetup(io.Discard, mode, root, []string{root}, all)
		_ = mutualExclusionVictims(root, mode)
	}
	if isCharDevice(nil) || isInteractiveTerminal() {
		t.Fatal("test process unexpectedly interactive")
	}

	monoDest := filepath.Join(t.TempDir(), "agent", "dws")
	_ = os.MkdirAll(filepath.Join(filepath.Dir(monoDest), "dingtalk-old"), 0o755)
	_ = mutualExclusionVictims(monoDest, skillSetupModeMono)
	multiDest := filepath.Join(t.TempDir(), "agent")
	_ = os.MkdirAll(filepath.Join(multiDest, "dws"), 0o755)
	_ = mutualExclusionVictims(multiDest, skillSetupModeMulti)
	cleanupMutualExclusion(monoDest, skillSetupModeMono, io.Discard, io.Discard)
	cleanupMutualExclusion(multiDest, skillSetupModeMulti, io.Discard, io.Discard)

	badParent := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(badParent, []byte("x"), 0o600)
	_, _, _ = installSkillToHomes(root, []string{filepath.Join(badParent, "dest")}, io.Discard, io.Discard)
	_, _, _ = installMultiSkillToHomes(root, []string{"missing"}, []string{filepath.Join(badParent, "dest")}, io.Discard, io.Discard)
	if err := copyDir(filepath.Join(root, "missing"), t.TempDir()); err == nil {
		t.Fatal("copy missing directory succeeded")
	}
	if err := copyFileContent(filepath.Join(root, "missing"), filepath.Join(t.TempDir(), "out"), 0o600); err == nil {
		t.Fatal("copy missing file succeeded")
	}

	for _, mode := range []string{skillSetupModeMono, skillSetupModeMulti} {
		src, cleanup, err := materializeEmbeddedSkillSource(mode)
		if err != nil || src == "" {
			t.Fatalf("embedded %s = %q, %v", mode, src, err)
		}
		cleanup()
	}
	if _, _, err := materializeEmbeddedSkillSource("missing"); err == nil {
		t.Fatal("missing embedded setup source succeeded")
	}
	if src, cleanup, err := resolveSkillSetupSourceOrEmbedded(root, skillSetupModeMono); err != nil || src == "" {
		t.Fatalf("explicit embedded wrapper = %q, %v", src, err)
	} else {
		cleanup()
	}
}
