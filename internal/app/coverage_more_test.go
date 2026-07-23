package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/apiclient"
	authpkg "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/mcptypes"
	"github.com/spf13/cobra"
)

func appRPCServer(t *testing.T, initOK, listOK bool) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			ID     int    `json:"id"`
			Method string `json:"method"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case "initialize":
			if !initOK {
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -32601, "message": "init"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{"protocolVersion": "2025-03-26"}})
		case "tools/list":
			if !listOK {
				_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "error": map[string]any{"code": -1, "message": "list"}})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"tools": []any{map[string]any{
						"name":        "tool",
						"description": "desc",
						"inputSchema": map[string]any{
							"properties": map[string]any{"id": map[string]any{"type": "string"}},
							"required":   []any{"id", 1, ""},
						},
					}},
				},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": map[string]any{}})
		}
	}))
}

func TestCrossPlatformCoveragePluginAuthCoverage(t *testing.T) {
	registerPluginAuthFromHeaders(mcptypes.ServerDescriptor{Key: "fallback", Endpoint: "%", AuthHeaders: map[string]string{"Authorization": "token"}})
	registerPluginAuthFromHeaders(mcptypes.ServerDescriptor{Key: "server", Endpoint: "https://x.test", CLI: mcptypes.CLIOverlay{ID: "cli"}, AuthHeaders: map[string]string{"Authorization": "Bearer token", "X": "Y"}})
	registerPluginAuthFromHeaders(mcptypes.ServerDescriptor{Key: "none"})
	if got, ok := LookupPluginAuth("cli"); !ok || got == nil || got.Token != "token" {
		t.Fatalf("registered plugin auth = %#v, %v", got, ok)
	}
}

func TestCrossPlatformCoverageRawAPIAndTokenCoverage(t *testing.T) {
	oldProvider := newAccessTokenProvider
	oldManager := newLegacyTokenManager
	t.Cleanup(func() {
		newAccessTokenProvider = oldProvider
		newLegacyTokenManager = oldManager
	})
	cmd := &cobra.Command{Use: "api"}
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetContext(context.Background())

	invalid := []struct {
		args []string
		gf   GlobalFlags
		af   apiFlags
	}{
		{[]string{"GET", "/x?a=1"}, GlobalFlags{}, apiFlags{}},
		{[]string{"TRACE", "/x"}, GlobalFlags{}, apiFlags{}},
		{[]string{"GET", "../x"}, GlobalFlags{}, apiFlags{}},
		{[]string{"GET", "/x"}, GlobalFlags{}, apiFlags{params: "x\x00"}},
		{[]string{"POST", "/x"}, GlobalFlags{}, apiFlags{data: "x\x00"}},
		{[]string{"POST", "/x"}, GlobalFlags{}, apiFlags{params: "-", data: "-"}},
		{[]string{"GET", "/x"}, GlobalFlags{Output: "x"}, apiFlags{pageAll: true}},
		{[]string{"GET", "/x"}, GlobalFlags{}, apiFlags{params: "{"}},
		{[]string{"POST", "/x"}, GlobalFlags{}, apiFlags{data: "{"}},
		{[]string{"GET", "https://evil.test/x"}, GlobalFlags{Token: "t"}, apiFlags{}},
	}
	for _, tc := range invalid {
		if err := runAPI(cmd, tc.args, &tc.gf, &tc.af); err == nil {
			t.Fatalf("invalid API %#v succeeded", tc)
		}
	}
	for _, raw := range []string{"", "a=1&empty=&=x", "a=1&b=2"} {
		if got := parseQueryStringToJSON(raw); got == "" {
			t.Fatalf("query JSON %q empty", raw)
		}
	}
	if got, err := resolveRawAPIToken(context.Background(), " token "); err != nil || got != "token" {
		t.Fatalf("explicit raw token = %q, %v", got, err)
	}
	authpkg.SetClientID("")
	authpkg.SetClientSecret("")
	if _, err := resolveRawAPIToken(context.Background(), ""); err == nil {
		t.Fatal("missing app credentials succeeded")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "fail") {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":"bad"}`)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": []any{1}, "hasMore": false})
	}))
	defer server.Close()
	host := strings.TrimPrefix(server.URL, "http://")
	host = strings.Split(host, ":")[0]
	apiclient.AllowedHosts[host] = true
	t.Cleanup(func() { delete(apiclient.AllowedHosts, host) })
	gf := &GlobalFlags{Token: "token", DryRun: true, Format: "json", Timeout: 1}
	af := &apiFlags{baseURL: server.URL}
	if err := runAPI(cmd, []string{"GET", "/ok"}, gf, af); err != nil || out.Len() == 0 {
		t.Fatalf("API dry run = %q, %v", out.String(), err)
	}
	out.Reset()
	gf.DryRun = false
	if err := runAPI(cmd, []string{"GET", "/ok"}, gf, af); err != nil || out.Len() == 0 {
		t.Fatalf("API request = %q, %v", out.String(), err)
	}
	out.Reset()
	af.pageAll = true
	if err := runAPI(cmd, []string{"GET", "/ok"}, gf, af); err != nil || out.Len() == 0 {
		t.Fatalf("API pagination = %q, %v", out.String(), err)
	}
	client := apiclient.NewClient("token", server.URL)
	if err := runPaginated(context.Background(), client, apiclient.RawAPIRequest{Method: "GET", Path: "/fail"}, &apiFlags{}, apiclient.ResponseOptions{Out: io.Discard, ErrOut: io.Discard}); err == nil {
		t.Fatal("failed pagination succeeded")
	}

	if _, err := ResolveAuxiliaryAccessToken(context.Background(), "", ""); err == nil {
		t.Fatal("empty auxiliary config succeeded")
	}
	if got, err := ResolveAuxiliaryAccessToken(context.Background(), "ignored", " explicit "); err != nil || got != "explicit" {
		t.Fatalf("explicit auxiliary token = %q, %v", got, err)
	}
	dir := t.TempDir()
	newAccessTokenProvider = func(string) accessTokenGetter { return fakeAccessTokenGetter{token: "saved"} }
	newLegacyTokenManager = func(string) legacyTokenGetter { return fakeLegacyTokenGetter{} }
	if got, err := resolveAccessTokenFromDir(context.Background(), dir); err != nil || got != "saved" {
		t.Fatalf("saved access token = %q, %v", got, err)
	}
	newAccessTokenProvider = func(string) accessTokenGetter { return fakeAccessTokenGetter{} }
	missing := t.TempDir()
	if got, err := resolveAccessTokenFromDir(context.Background(), missing); got != "" || !errors.Is(err, authpkg.ErrTokenDataNotFound) {
		t.Fatalf("missing access token = %q, %v", got, err)
	}
	if _, err := ResolveAuxiliaryAccessToken(context.Background(), missing, ""); err == nil {
		t.Fatal("missing auxiliary credentials succeeded")
	}
	if _, err := ForceRefreshAccessToken(context.Background(), ""); err == nil {
		t.Fatal("empty force refresh config succeeded")
	}
	if _, err := ForceRefreshAccessToken(context.Background(), missing); err == nil {
		t.Fatal("missing force refresh token succeeded")
	}
}

func TestCrossPlatformCoverageRootUtilityAndTimingCoverage(t *testing.T) {
	_ = resolveVerbosity(nil)
	for _, flags := range []struct {
		debug   bool
		verbose bool
		format  string
		json    bool
	}{{}, {verbose: true}, {debug: true}, {format: "json"}, {format: "table"}, {json: true}} {
		flagCmd := &cobra.Command{Use: "flags"}
		flagCmd.Flags().Bool("debug", flags.debug, "")
		flagCmd.Flags().Bool("verbose", flags.verbose, "")
		flagCmd.Flags().String("format", flags.format, "")
		flagCmd.Flags().Bool("json", false, "")
		if flags.json {
			_ = flagCmd.Flags().Set("json", "true")
		}
		_ = resolveVerbosity(flagCmd)
		_ = commandRequestsJSONErrors(flagCmd)
		_ = wantsJSONErrors(flagCmd)
	}
	_ = commandRequestsJSONErrors(nil)
	_ = wantsJSONErrors(nil)
	if got, changed := normalizeProfileFlagArgs([]string{"--profile", "a,", "b"}); !changed || len(got) == 0 {
		t.Fatalf("profile args = %#v, %v", got, changed)
	}
	if _, changed := normalizeProfileFlagArgs([]string{"--profile"}); changed {
		t.Fatal("incomplete profile flag changed")
	}
	if preparseProfileFlag([]string{"--profile=a"}) != "a" || preparseProfileFlag([]string{"--profile", "b"}) != "b" || preparseProfileFlag(nil) != "" {
		t.Fatal("profile preparse mismatch")
	}
	if !argsChanged([]string{"a"}, []string{"b"}) || argsChanged([]string{"a"}, []string{"a"}) {
		t.Fatal("argsChanged mismatch")
	}
	cmd := &cobra.Command{Use: "root"}
	cmd.SetContext(context.Background())
	cmd.Flags().String("output", "", "")
	if err := configureOutputSink(cmd); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "nested", "out.txt")
	_ = cmd.Flags().Set("output", path)
	if err := configureOutputSink(cmd); err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(cmd.OutOrStdout(), "data")
	if err := closeOutputSink(cmd); err != nil {
		t.Fatal(err)
	}
	local := &cobra.Command{Use: "local"}
	local.SetContext(context.Background())
	local.SetOut(io.Discard)
	local.Flags().String("output", "", "")
	if err := configureOutputSink(local); err != nil {
		t.Fatal(err)
	}
	if err := validateOptionalPath("--x", ""); err != nil {
		t.Fatal(err)
	}
	if err := validateOptionalPath("--x", "bad\x00path"); err == nil {
		t.Fatal("unsafe path succeeded")
	}

	root := &cobra.Command{Use: "root", Short: "root"}
	bindPersistentFlags(root, &GlobalFlags{})
	root.AddCommand(&cobra.Command{Use: "alpha", Short: "alpha"}, &cobra.Command{Use: "hidden", Hidden: true})
	configureRootHelp(root)
	var help bytes.Buffer
	root.SetOut(&help)
	_ = root.Help()
	renderRootGlobalFlags(root)
	_ = visiblePersistentFlags(root)
	for _, command := range root.Commands() {
		_ = commandShort(command)
	}
	_ = visibleMCPRootCommands(root)
	_ = visibleUtilityRootCommands(root)

	tc := NewTimingCollector()
	tc.Record("a", time.Microsecond)
	tc.Record("b", 2*time.Second)
	for _, d := range []time.Duration{time.Nanosecond, time.Microsecond, time.Millisecond, time.Second} {
		_ = formatDuration(d)
	}
	for _, debug := range []bool{false, true} {
		if debug {
			t.Setenv(PerfDebugEnv, "1")
		} else {
			t.Setenv(PerfDebugEnv, "")
		}
		tc.PrintIfEnabled()
	}
	var timingOut bytes.Buffer
	tc.Print(&timingOut)
	if timingOut.Len() == 0 {
		t.Fatal("timing output empty")
	}
	t.Setenv("HOME", t.TempDir())
	if defaultPerfReportPath() == "" {
		t.Fatal("default perf path empty")
	}
	t.Setenv(PerfReportEnv, "auto")
	tc.WriteReportIfEnabled("v", "cmd")
	if _, err := LoadLatestReport(); err != nil {
		t.Fatal(err)
	}
	t.Setenv(PerfReportEnv, "")
	tc.WriteReportIfEnabled("v", "cmd")
	_ = exeRelativeConfigDir()

	merged := mergeTopLevelCommands([]*cobra.Command{{Use: "a"}, {Use: "a"}, {Use: "b"}, nil})
	if len(merged) != 2 {
		t.Fatalf("merged commands = %#v", merged)
	}
	dedupRoot := &cobra.Command{Use: "root"}
	dedupRoot.AddCommand(&cobra.Command{Use: "same"}, &cobra.Command{Use: "same"})
	deduplicateCommands(dedupRoot)
	addPluginCommandsSafe(dedupRoot, []*cobra.Command{{Use: "same"}, {Use: "new"}})
	_ = newCompletionCommand(dedupRoot)
	_ = newCatalogCommand(nil)
	_ = newConfigCommand()
	_ = newCacheCommand()
	_ = newVersionCommand()
	_ = newRecoveryCommand(context.Background(), nil, &GlobalFlags{})
	_ = newAPICommand(&GlobalFlags{})
	_ = NewRootCommand(context.Background())
}
