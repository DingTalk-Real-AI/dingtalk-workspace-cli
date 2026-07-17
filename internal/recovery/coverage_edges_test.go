package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestCrossPlatformCoverageRecoveryModelEdges(t *testing.T) {
	typedCases := []struct {
		name string
		err  error
		raw  string
		want string
	}{
		{"validation required", apperrors.NewValidation("required value"), "", cliMissingParamCode},
		{"validation invalid", apperrors.NewValidation("bad json"), "", cliInvalidJSONCode},
		{"auth permission reason", apperrors.NewAuth("denied", apperrors.WithReason("http_403")), "", cliPermissionCode},
		{"auth forbidden", apperrors.NewAuth("forbidden"), "", cliPermissionCode},
		{"auth expired", apperrors.NewAuth("expired"), "", cliAuthExpiredCode},
		{"api rate reason", apperrors.NewAPI("limited", apperrors.WithReason("http_429")), "", cliRateLimitCode},
		{"api rate message", apperrors.NewAPI("rate limit", apperrors.WithRetryable(true)), "", cliRateLimitCode},
		{"api timeout reason", apperrors.NewAPI("failed", apperrors.WithReason("request_timeout")), "", cliTimeoutCode},
		{"api timeout message", apperrors.NewDiscovery("connection reset"), "", cliTimeoutCode},
		{"api invalid params", apperrors.NewAPI("failed", apperrors.WithReason("invalid_params")), "", cliInvalidJSONCode},
		{"api method missing", apperrors.NewAPI("failed", apperrors.WithReason("method_not_found")), "", cliResourceNotFoundCode},
		{"api 404", apperrors.NewAPI("failed", apperrors.WithReason("http_404")), "", cliResourceNotFoundCode},
		{"raw auth", nil, "user_token_illegal", cliAuthExpiredCode},
		{"raw permission", nil, "permission denied", cliPermissionCode},
		{"raw rate", nil, "too many requests", cliRateLimitCode},
		{"raw timeout", nil, "deadline exceeded", cliTimeoutCode},
		{"raw missing", nil, "base_not_found", cliResourceNotFoundCode},
		{"unknown", nil, "strange", ""},
	}
	for _, tt := range typedCases {
		t.Run(tt.name, func(t *testing.T) {
			if got := canonicalCLIErrorCode(tt.err, tt.raw); got != tt.want {
				t.Fatalf("canonicalCLIErrorCode = %q, want %q", got, tt.want)
			}
		})
	}

	for _, name := range []string{"get_x", "list_x", "search_x", "query_x", "status_x", "download_x"} {
		if InferOperationKind(name) != OperationRead {
			t.Errorf("%q should be read", name)
		}
	}
	for _, name := range []string{"create_x", "update_x", "delete_x", "send_x", "generate_image", "edit_image", "upscale", "isolate"} {
		if InferOperationKind(name) != OperationWrite {
			t.Errorf("%q should be write", name)
		}
	}
	if InferOperationKind("") != OperationUnknown || InferOperationKind("execute") != OperationUnknown {
		t.Fatal("unknown operation inference failed")
	}

	input := map[string]any{
		"short":     "value",
		"long":      strings.Repeat("x", 121),
		"text":      "secret body",
		"names":     []string{"a", "b"},
		"textItems": []string{"a"},
		"items":     []any{"a", map[string]any{"x": 1}},
		"records":   []map[string]any{{"name": "n"}},
		"objects":   []map[string]any{{"name": "n", "token": "drop"}},
		"payload":   map[string]any{"name": "n", "token": "drop"},
		"fields":    map[string]any{"z": 1, "a": 2},
		"stringMap": map[string]string{"name": "n", "token": "drop"},
		"jsonMap":   map[string]string{"z": "1", "a": "2"},
		"number":    7,
		"token":     "drop",
	}
	summary := SummarizeArgs(input)
	if len(summary) == 0 || summary["number"] != 7 {
		t.Fatalf("summary = %#v", summary)
	}
	replayed := sanitizeReplayMap(input)
	if _, ok := replayed["token"]; ok {
		t.Fatal("sensitive replay field retained")
	}
	for _, value := range []any{
		map[string]any{"safe": "x", "token": "drop"},
		map[string]string{"safe": "x", "token": "drop"},
		[]any{map[string]any{"safe": "x"}},
		[]string{"a"},
		[]map[string]any{{"safe": "x"}},
		42,
	} {
		if sanitizeReplayValue(value) == nil {
			t.Fatalf("sanitizeReplayValue(%T) returned nil", value)
		}
	}
	if sanitizeReplayMap(nil) != nil || sanitizeReplayStringMap(nil) != nil {
		t.Fatal("empty replay maps should be nil")
	}
	if got := sanitizeReplayField("data", map[string]string{"safe": "x"}); got == nil {
		t.Fatal("string map sanitization failed")
	}
	if got := sanitizeReplayField("data", []map[string]any{{"safe": "x"}}); got == nil {
		t.Fatal("map slice sanitization failed")
	}
	if got := sanitizeReplayField("data", strings.Repeat("x", 121)); got == nil {
		t.Fatal("long string sanitization failed")
	}

	argv := sanitizeArgv([]string{
		"dws", "--token=one", "--auth-code=two", "--refresh-token=three",
		"--auth-code", "four", "--refresh-token", "five", "--plain",
	})
	if len(argv) != 9 || strings.Contains(strings.Join(argv, " "), "one") {
		t.Fatalf("sanitized argv = %#v", argv)
	}
	if sanitizeArgv(nil) != nil {
		t.Fatal("nil argv should remain nil")
	}
	if value, ok := redactSensitiveArg("--plain=x"); ok || value != "" {
		t.Fatalf("plain arg redacted: %q %v", value, ok)
	}

	if keys := sortedKeys(map[string]any{"b": 1, "a": 2}); strings.Join(keys, ",") != "a,b" {
		t.Fatalf("sorted keys = %#v", keys)
	}
	if keys := sortedStringKeys(map[string]string{"b": "1", "a": "2"}); strings.Join(keys, ",") != "a,b" {
		t.Fatalf("sorted string keys = %#v", keys)
	}
	normalized := normalizeRawError(" HTTPS://example.test/x?q=secret 123456 2026-07-13T01:02:03Z abcdefghijklmnopqrstuvwxyz ")
	if strings.Contains(normalized, "secret") || strings.Contains(normalized, "123456") {
		t.Fatalf("normalizeRawError = %q", normalized)
	}

	ctx := BuildContext(CaptureInput{ToolName: "get_x"})
	if ctx.OperationKind != OperationRead || ctx.RawError != "" {
		t.Fatalf("BuildContext defaults = %#v", ctx)
	}
	replay := BuildReplay(CaptureInput{ToolName: "create_x"})
	if replay.OperationKind != OperationWrite || replay.RedactedCommand != "" {
		t.Fatalf("BuildReplay defaults = %#v", replay)
	}
}

func TestCrossPlatformCoveragePlannerRuleAndSearchEdges(t *testing.T) {
	rules := []struct {
		name     string
		rc       RecoveryContext
		category string
		retry    bool
	}{
		{"input", RecoveryContext{CLIErrorCode: cliInvalidJSONCode}, "input", false},
		{"resource suffix", RecoveryContext{CLIErrorCode: "THING_NOT_FOUND"}, "resource", false},
		{"permission", RecoveryContext{HTTPStatus: 403}, "permission", false},
		{"rate read", RecoveryContext{HTTPStatus: 429, OperationKind: OperationRead}, "rate_limit", true},
		{"rate write http", RecoveryContext{HTTPStatus: 429, OperationKind: OperationWrite, CallStage: "http"}, "rate_limit", true},
		{"rate write non-http", RecoveryContext{HTTPStatus: 429, OperationKind: OperationWrite, CallStage: "decode"}, "rate_limit", false},
		{"network read", RecoveryContext{HTTPStatus: 503, OperationKind: OperationRead}, "network", true},
		{"network write", RecoveryContext{RawError: "timeout", OperationKind: OperationWrite, CallStage: "http"}, "network", false},
	}
	for _, tt := range rules {
		t.Run(tt.name, func(t *testing.T) {
			plan := NewPlanner(nil).PlanWithOptions(context.Background(), tt.rc, PlanOptions{EnableDocSearch: false})
			if plan.Category != tt.category || plan.ShouldRetry != tt.retry {
				t.Fatalf("plan = %#v", plan)
			}
		})
	}
	if safeToRetry(RecoveryContext{OperationKind: OperationUnknown}, "network") {
		t.Fatal("unknown operation should not retry")
	}
	if normalizeDecisionOwner(RecoveryPlan{DecisionOwner: DecisionOwnerAgent}) != DecisionOwnerAgent ||
		normalizeDecisionOwner(RecoveryPlan{Category: "unknown"}) != DecisionOwnerAgent ||
		normalizeDecisionOwner(RecoveryPlan{Category: "auth"}) != DecisionOwnerBuiltinRule {
		t.Fatal("decision owner normalization failed")
	}

	queryRC := RecoveryContext{CLIErrorCode: "CODE", ToolName: "get_x", CommandPath: []string{"product", "get"}, RawError: "Error alpha alpha beta 123456"}
	if query := BuildFallbackQuery(queryRC); !strings.Contains(query, "CODE") || !strings.Contains(query, "alpha") {
		t.Fatalf("fallback query = %q", query)
	}
	if got := stableKeywords(""); got != nil {
		t.Fatalf("empty stable keywords = %#v", got)
	}
	many := stableKeywords("one two three four five six seven eight")
	if len(many) != 6 {
		t.Fatalf("stable keyword cap = %#v", many)
	}
	if containsAny("Hello WORLD", "none", "world") != true || containsAny("hello", "x") {
		t.Fatal("containsAny failed")
	}
	if got := normalizeSafeActions([]string{"wait_and_retry", "unsafe", "wait_and_retry"}); len(got) != 1 {
		t.Fatalf("safe actions = %#v", got)
	}

	hit := KBHit{
		Source: "docs", Title: "errors", URL: "https://docs",
		Snippet: "| httpCode | code | message | reason | action |\n| --- | --- | --- | --- | --- |\n| 403 | Forbidden | denied | permission | Request access |\n| 404 | Missing | gone | deleted | %s |\n| bad | short | row |",
	}
	retrieval := KnowledgeRetrieval{
		KBHits: []KBHit{hit, hit},
		DocSearch: DocSearch{
			Request:  &ToolCallRecord{ServerID: "docs", ToolName: "search", Arguments: map[string]any{"q": "x"}},
			Response: &ToolResponse{Content: []ToolResponseBlock{{Type: "text", Text: "ok"}}},
			Items:    []DocSearchItem{{Title: "one"}},
		},
	}
	plan := NewPlanner(fakeRetriever{retrieval: retrieval}).Plan(context.Background(), RecoveryContext{RawError: "unknown issue"})
	if plan.DocSearch.Status != "success" || len(plan.KBHits) != 1 || len(plan.DocActions) != 1 {
		t.Fatalf("retrieved plan = %#v", plan)
	}
	if !plan.DecisionHints.PermissionSensitive || !plan.DecisionHints.ResourceStateRelated {
		t.Fatalf("decision hints = %#v", plan.DecisionHints)
	}

	emptyPlan := NewPlanner(fakeRetriever{}).Plan(context.Background(), RecoveryContext{RawError: "unknown"})
	if emptyPlan.DocSearch.Status != "empty" {
		t.Fatalf("empty search = %#v", emptyPlan.DocSearch)
	}
	errPlan := NewPlanner(fakeRetriever{err: errors.New("docs down")}).Plan(context.Background(), RecoveryContext{RawError: "unknown"})
	if errPlan.DocSearch.Status != "error" || errPlan.DocSearch.Error != "docs down" {
		t.Fatalf("error search = %#v", errPlan.DocSearch)
	}
	customErr := NewPlanner(fakeRetriever{
		retrieval: KnowledgeRetrieval{DocSearch: DocSearch{Status: "error", Error: "custom"}},
		err:       errors.New("docs down"),
	}).Plan(context.Background(), RecoveryContext{RawError: "unknown"})
	if customErr.DocSearch.Error != "custom" {
		t.Fatalf("custom search error = %#v", customErr.DocSearch)
	}
	emptyErr := NewPlanner(fakeRetriever{
		retrieval: KnowledgeRetrieval{DocSearch: DocSearch{Status: "error"}},
	}).Plan(context.Background(), RecoveryContext{RawError: "unknown"})
	if emptyErr.DocSearch.Status != "error" {
		t.Fatalf("empty search error = %#v", emptyErr.DocSearch)
	}
	skipped := NewPlanner(nil).Plan(context.Background(), RecoveryContext{RawError: "unknown"})
	if skipped.DocSearch.Status != "skipped" {
		t.Fatalf("nil retriever = %#v", skipped.DocSearch)
	}
	var nilPlanner *Planner
	if got := nilPlanner.searchKnowledge(context.Background(), "query", RecoveryContext{}); got.DocSearch.Status != "skipped" {
		t.Fatalf("nil planner search = %#v", got)
	}
	if got := NewPlanner(nil).searchKnowledge(context.Background(), "", RecoveryContext{}); got.DocSearch.Status != "skipped" {
		t.Fatalf("empty query search = %#v", got)
	}

	actions := extractDocActions([]KBHit{{Snippet: ""}, hit})
	if len(actions) != 1 || len(splitTableRow("| a | b |")) != 2 {
		t.Fatalf("doc actions = %#v", actions)
	}
	if len(dedupeDocActions(append(actions, actions...))) != 1 ||
		len(dedupeKBHits([]KBHit{hit, hit})) != 1 {
		t.Fatal("dedupe failed")
	}
	if cloneDocActions(nil) != nil || cloneKBHits(nil) != nil {
		t.Fatal("nil clone should remain nil")
	}
	if got := cloneDocSearch(retrieval.DocSearch); got.Request == retrieval.DocSearch.Request || got.Response == retrieval.DocSearch.Response {
		t.Fatal("doc search clone retained pointers")
	}
	if got := stableKeywords("a ok https://example.test/path?q=x valid"); len(got) != 1 || got[0] != "valid" {
		t.Fatalf("placeholder/small keyword filtering = %#v", got)
	}
	if got := uniqueStrings([]string{" ", "one", "one"}); len(got) != 1 {
		t.Fatalf("unique strings = %#v", got)
	}
	if got := extractDocActions([]KBHit{{Snippet: "ordinary text\n| 1 | 2 | 3 | 4 | act |"}}); len(got) != 1 {
		t.Fatalf("non-table doc line handling = %#v", got)
	}
	var nilPlan *RecoveryPlan
	HydratePlanForEvent("evt", RecoveryContext{}, Replay{}, nilPlan)
}

type fakeRecoveryInvoker struct {
	result *transport.ToolCallResult
	err    error
}

func (f fakeRecoveryInvoker) CallToolDirect(context.Context, string, string, map[string]any) (*transport.ToolCallResult, error) {
	return f.result, f.err
}

func TestCrossPlatformCoverageExecutorAndProbeEdges(t *testing.T) {
	last := LastError{
		EventID: "evt-1",
		Context: RecoveryContext{
			CommandPath:   []string{"aitable", "base", "get"},
			ServerID:      "aitable",
			ToolName:      "get_base",
			OperationKind: OperationRead,
			ArgsSummary:   map[string]any{"baseId": "base_1"},
			RawError:      "unknown",
		},
		Replay: Replay{
			ServerID:        "aitable",
			ToolName:        "get_base",
			OperationKind:   OperationRead,
			ToolArgs:        map[string]any{"baseId": "base_1"},
			RedactedCommand: "dws aitable base get",
		},
	}
	if got := (*Executor)(nil).Execute(context.Background(), last); got.Status != "analysis_failed" {
		t.Fatalf("nil executor = %#v", got)
	}
	if got := (&Executor{}).Execute(context.Background(), last); got.Status != "analysis_failed" {
		t.Fatalf("nil planner executor = %#v", got)
	}

	result := &transport.ToolCallResult{Blocks: []transport.ContentBlock{{Type: "text", Text: "{\"bases\":[1]}"}}}
	executor := NewExecutor(NewPlanner(fakeRetriever{}), fakeRecoveryInvoker{result: result})
	bundle := executor.Execute(context.Background(), last)
	if bundle.Status != "needs_agent_action" || len(bundle.ProbeResults) != 2 ||
		bundle.Plan.AgentRoute.Payload == nil || len(bundle.Plan.AgentRoute.Payload.ProbeResults) != 2 {
		t.Fatalf("successful bundle = %#v", bundle)
	}
	if bundle.FinalizeHint.Command == "" || len(bundle.AgentTask.MustReadRefs) != 4 {
		t.Fatalf("agent handoff metadata = %#v %#v", bundle.FinalizeHint, bundle.AgentTask)
	}

	failing := NewExecutor(NewPlanner(fakeRetriever{err: errors.New("docs")}), fakeRecoveryInvoker{err: errors.New("probe")})
	failedBundle := failing.Execute(context.Background(), LastError{EventID: "evt-2", Context: last.Context})
	if failedBundle.Status != "analysis_failed" ||
		!strings.Contains(failedBundle.AnalysisError, "replay data missing") ||
		!strings.Contains(failedBundle.AnalysisError, "doc search failed") {
		t.Fatalf("failed bundle = %#v", failedBundle)
	}

	executor.probes = nil
	if got := executor.runProbes(context.Background(), last.Context, last.Replay, RecoveryPlan{}); got != nil {
		t.Fatalf("empty probes = %#v", got)
	}
	executor.probes = []Probe{
		func(context.Context, ToolInvoker, RecoveryContext, Replay, RecoveryPlan) *ProbeResult { return nil },
		func(context.Context, ToolInvoker, RecoveryContext, Replay, RecoveryPlan) *ProbeResult {
			return &ProbeResult{Name: "custom", Status: "success"}
		},
	}
	if got := executor.runProbes(context.Background(), last.Context, last.Replay, RecoveryPlan{}); len(got) != 1 {
		t.Fatalf("custom probes = %#v", got)
	}

	if got := probeUnknownContextAudit(context.Background(), nil, RecoveryContext{}, Replay{}, RecoveryPlan{Category: "auth"}); got != nil {
		t.Fatalf("built-in plan audit = %#v", got)
	}
	audit := probeUnknownContextAudit(context.Background(), nil, RecoveryContext{}, Replay{}, RecoveryPlan{Category: "unknown"})
	if audit == nil || !strings.Contains(audit.Summary, "no identifier") {
		t.Fatalf("empty audit = %#v", audit)
	}
	fullAudit := probeUnknownContextAudit(
		context.Background(),
		nil,
		RecoveryContext{
			CommandPath: []string{"x"},
			ArgsSummary: map[string]any{"empty_id": "", "name": "n", "record_id": "r1"},
		},
		Replay{ToolArgs: map[string]any{"uuid": "u1"}, RedactedCommand: "dws x"},
		RecoveryPlan{
			Category:   "unknown",
			DocSearch:  DocSearch{Status: "success"},
			KBHits:     []KBHit{{Title: "hit"}},
			DocActions: []DocAction{{Action: "act"}},
		},
	)
	if fullAudit == nil || !strings.Contains(fullAudit.Summary, "found 2") {
		t.Fatalf("full audit = %#v", fullAudit)
	}
	if got := probeAITableBaseCatalog(context.Background(), nil, RecoveryContext{}, Replay{}, RecoveryPlan{}); got != nil {
		t.Fatalf("unrelated probe = %#v", got)
	}
	if got := probeAITableBaseCatalog(context.Background(), nil, RecoveryContext{CommandPath: []string{"aitable", "base", "list"}}, Replay{}, RecoveryPlan{}); got.Status != "skipped" {
		t.Fatalf("unsupported aitable probe = %#v", got)
	}
	if got := probeAITableBaseCatalog(context.Background(), nil, last.Context, last.Replay, RecoveryPlan{}); got.Status != "skipped" {
		t.Fatalf("nil invoker probe = %#v", got)
	}

	outputs := []struct {
		result  *transport.ToolCallResult
		summary string
	}{
		{nil, "no output"},
		{&transport.ToolCallResult{Blocks: []transport.ContentBlock{{Type: "image"}, {Type: "text", Text: " plain "}}}, "text payload"},
		{&transport.ToolCallResult{Content: map[string]any{"x": 1}}, "content payload"},
		{&transport.ToolCallResult{StructuredContent: map[string]any{"x": 1}}, "non-text payload"},
	}
	for _, tt := range outputs {
		_, summary := summarizeProbeOutput(tt.result)
		if !strings.Contains(summary, tt.summary) {
			t.Errorf("summarizeProbeOutput = %q, want %q", summary, tt.summary)
		}
	}
	if len(buildMustReadRefs(nil)) != 3 || productReferenceFor("unknown") != "" {
		t.Fatal("must-read references failed")
	}
	for _, product := range []string{"aitable", "attendance", "calendar", "chat", "contact", "devdoc", "ding", "report", "todo", "workbench", "approval"} {
		if productReferenceFor(product) == "" {
			t.Errorf("missing product reference for %q", product)
		}
	}
	if cloneProbeResults(nil) != nil {
		t.Fatal("nil probe clone should remain nil")
	}
}

func TestCrossPlatformCoverageStoreRuntimeAndPruningEdges(t *testing.T) {
	ResetRuntimeState()
	if LatestCapture() != nil {
		t.Fatal("latest capture should start nil")
	}
	setLatestCapture(nil)
	last := &LastError{
		EventID: "evt",
		Context: RecoveryContext{
			CommandPath: []string{"x"},
			ArgsSummary: map[string]any{"nested": map[string]any{"x": 1}},
		},
		Replay: Replay{
			ToolArgs:     map[string]any{"slice": []any{map[string]any{"x": 1}}, "strings": []string{"x"}},
			RedactedArgv: []string{"dws"},
		},
	}
	setLatestCapture(last)
	got := LatestCapture()
	if got == last || got.Context.ArgsSummary == nil || got.Replay.ToolArgs == nil {
		t.Fatalf("LatestCapture clone = %#v", got)
	}
	ResetRuntimeState()
	if cloneMap(nil) != nil || cloneSlice(nil) != nil {
		t.Fatal("nil clones should remain nil")
	}
	if cloneValue(7) != 7 {
		t.Fatal("scalar clone changed")
	}
	if !isEmptyReplay(Replay{}) || isEmptyReplay(Replay{ToolName: "x"}) {
		t.Fatal("empty replay detection failed")
	}

	var disabled *Store
	if disabled.Enabled() || NewStore("").Enabled() {
		t.Fatal("disabled store reported enabled")
	}
	if captured, err := disabled.Capture(RecoveryContext{}); err != nil || captured != nil {
		t.Fatalf("disabled capture = %#v, %v", captured, err)
	}
	for name, call := range map[string]func() error{
		"load":   func() error { _, err := disabled.LoadLastError(); return err },
		"event":  func() error { _, err := disabled.LoadErrorByEvent("evt"); return err },
		"ensure": disabled.ensureDir,
	} {
		if err := call(); err == nil {
			t.Errorf("disabled %s unexpectedly succeeded", name)
		}
	}
	if err := disabled.SavePlan("evt", RecoveryPlan{}); err != nil {
		t.Fatal(err)
	}
	if err := disabled.SaveAnalysis("evt", RecoveryPlan{}, RecoveryBundle{}); err != nil {
		t.Fatal(err)
	}
	if err := disabled.Finalize("evt", "failed", nil); err != nil {
		t.Fatal(err)
	}
	if err := disabled.pruneExpiredArtifacts(time.Now()); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	store := NewStore(dir)
	if _, err := store.LoadErrorByEvent(""); err == nil {
		t.Fatal("empty event id succeeded")
	}
	if _, err := store.LoadErrorByEvent("missing"); err == nil {
		t.Fatal("missing event id succeeded")
	}
	if _, err := store.LoadLastError(); err == nil {
		t.Fatal("missing last error succeeded")
	}
	badLast := store.lastErrorPath()
	if err := os.MkdirAll(store.dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(badLast, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := store.LoadLastError(); err == nil {
		t.Fatal("malformed last error succeeded")
	}

	cutoff := time.Now().UTC().Add(-recoveryMaxAge)
	old := cutoff.Add(-time.Hour).Format(time.RFC3339Nano)
	recent := cutoff.Add(time.Hour).Format(time.RFC3339Nano)
	if _, ok := parseRecordedAt(""); ok {
		t.Fatal("empty timestamp parsed")
	}
	if _, ok := parseRecordedAt("invalid"); ok {
		t.Fatal("invalid timestamp parsed")
	}
	if _, ok := parseRecordedAt(time.Now().UTC().Format(time.RFC3339)); !ok {
		t.Fatal("RFC3339 timestamp did not parse")
	}

	if err := os.WriteFile(badLast, []byte("{\"recorded_at\":\""+old+"\"}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.pruneLastError(cutoff); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(badLast); !os.IsNotExist(err) {
		t.Fatalf("old last error remains: %v", err)
	}
	if err := os.WriteFile(badLast, []byte("{\"recorded_at\":\""+recent+"\"}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.pruneLastError(cutoff); err != nil {
		t.Fatal(err)
	}

	events := strings.Join([]string{
		"",
		"{",
		"{\"event_id\":\"old\",\"recorded_at\":\"" + old + "\"}",
		"{\"event_id\":\"recent\",\"recorded_at\":\"" + recent + "\"}",
	}, "\n") + "\n"
	if err := os.WriteFile(store.eventsPath(), []byte(events), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.pruneEvents(cutoff); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(store.eventsPath())
	if err != nil || strings.Contains(string(data), "old") || !strings.Contains(string(data), "recent") {
		t.Fatalf("pruned events = %q, %v", data, err)
	}
	if err := os.WriteFile(store.eventsPath(), []byte("{\"event_id\":\"old\",\"recorded_at\":\""+old+"\"}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.pruneEvents(cutoff); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(store.eventsPath()); !os.IsNotExist(err) {
		t.Fatalf("empty events file remains: %v", err)
	}

	oldArtifact := filepath.Join(store.dir(), "old-artifact")
	recentArtifact := filepath.Join(store.dir(), "recent-artifact")
	if err := os.MkdirAll(oldArtifact, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recentArtifact, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := cutoff.Add(-time.Hour)
	if err := os.Chtimes(oldArtifact, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := store.pruneOtherArtifacts(cutoff); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldArtifact); !os.IsNotExist(err) {
		t.Fatalf("old artifact remains: %v", err)
	}
	if _, err := os.Stat(recentArtifact); err != nil {
		t.Fatalf("recent artifact removed: %v", err)
	}

	emptyReplay, err := store.Capture(RecoveryContext{RawError: "x"})
	if err != nil || emptyReplay == nil {
		t.Fatalf("empty replay capture = %#v, %v", emptyReplay, err)
	}
	if _, err := store.LoadErrorByEvent(emptyReplay.EventID); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageStoreMarshalErrors(t *testing.T) {
	store := NewStore(t.TempDir())
	if err := store.writeJSON(filepath.Join(store.dir(), "bad.json"), map[string]any{"bad": func() {}}); err == nil {
		t.Fatal("expected writeJSON marshal error")
	}
	event := RecoveryEvent{
		EventID: "evt",
		Bundle: &RecoveryBundle{
			ProbeResults: []ProbeResult{{Output: func() {}}},
		},
	}
	if err := store.appendEvent(event); err == nil {
		t.Fatal("expected appendEvent marshal error")
	}
}

func TestCrossPlatformCoverageLoadErrorByEventFallback(t *testing.T) {
	store := NewStore(t.TempDir())
	last := LastError{EventID: "fallback", RecordedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	data, err := json.Marshal(last)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(store.dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.lastErrorPath(), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := store.LoadErrorByEvent("fallback"); err != nil || got.EventID != "fallback" {
		t.Fatalf("fallback event = %#v, %v", got, err)
	}
	otherEvent := RecoveryEvent{
		EventID:    "other",
		Phase:      "planned",
		RecordedAt: time.Now().UTC().Format(time.RFC3339Nano),
	}
	eventData, err := json.Marshal(otherEvent)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.eventsPath(), append(eventData, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := store.LoadErrorByEvent("fallback"); err != nil || got.EventID != "fallback" {
		t.Fatalf("EOF fallback event = %#v, %v", got, err)
	}
}

func TestCrossPlatformCoverageStoreSystemCallEdges(t *testing.T) {
	oldReadFile := recoveryReadFile
	oldWriteFile := recoveryWriteFile
	oldOpen := recoveryOpen
	oldOpenFile := recoveryOpenFile
	oldMkdirAll := recoveryMkdirAll
	oldReadDir := recoveryReadDir
	oldStat := recoveryStat
	oldRemove := recoveryRemove
	oldRemoveAll := recoveryRemoveAll
	oldFileWrite := recoveryFileWrite
	t.Cleanup(func() {
		recoveryReadFile = oldReadFile
		recoveryWriteFile = oldWriteFile
		recoveryOpen = oldOpen
		recoveryOpenFile = oldOpenFile
		recoveryMkdirAll = oldMkdirAll
		recoveryReadDir = oldReadDir
		recoveryStat = oldStat
		recoveryRemove = oldRemove
		recoveryRemoveAll = oldRemoveAll
		recoveryFileWrite = oldFileWrite
	})
	reset := func() {
		recoveryReadFile = oldReadFile
		recoveryWriteFile = oldWriteFile
		recoveryOpen = oldOpen
		recoveryOpenFile = oldOpenFile
		recoveryMkdirAll = oldMkdirAll
		recoveryReadDir = oldReadDir
		recoveryStat = oldStat
		recoveryRemove = oldRemove
		recoveryRemoveAll = oldRemoveAll
		recoveryFileWrite = oldFileWrite
	}

	store := NewStore(t.TempDir())
	recoveryWriteFile = func(string, []byte, os.FileMode) error { return errors.New("write") }
	if _, err := store.Capture(RecoveryContext{}); err == nil {
		t.Fatal("expected capture snapshot write error")
	}
	reset()

	recoveryOpenFile = func(string, int, os.FileMode) (*os.File, error) { return nil, errors.New("open event") }
	if _, err := store.Capture(RecoveryContext{}); err == nil {
		t.Fatal("expected capture event append error")
	}
	reset()

	recoveryReadFile = func(string) ([]byte, error) { return nil, errors.New("prune") }
	if _, err := store.LoadLastError(); err == nil {
		t.Fatal("expected LoadLastError prune error")
	}
	if _, err := store.LoadErrorByEvent("evt"); err == nil {
		t.Fatal("expected LoadErrorByEvent prune error")
	}
	if err := store.pruneExpiredArtifacts(time.Now()); err == nil {
		t.Fatal("expected last-error prune failure")
	}
	reset()

	recoveryReadFile = func(path string) ([]byte, error) {
		if path == store.lastErrorPath() {
			return nil, os.ErrNotExist
		}
		if path == store.eventsPath() {
			return nil, errors.New("events")
		}
		return oldReadFile(path)
	}
	if err := store.pruneExpiredArtifacts(time.Now()); err == nil {
		t.Fatal("expected event prune failure")
	}
	reset()

	recoveryOpen = func(string) (*os.File, error) { return nil, errors.New("open") }
	if _, err := store.LoadErrorByEvent("evt"); err == nil {
		t.Fatal("expected event open error")
	}
	reset()

	if err := os.MkdirAll(store.dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(store.eventsPath(), 0o755); err != nil {
		t.Fatal(err)
	}
	recoveryReadFile = func(path string) ([]byte, error) {
		if path == store.lastErrorPath() || path == store.eventsPath() {
			return nil, os.ErrNotExist
		}
		return oldReadFile(path)
	}
	if _, err := store.LoadErrorByEvent("evt"); err == nil || os.IsNotExist(err) {
		t.Fatalf("expected non-EOF reader error, got %v", err)
	}
	if err := os.RemoveAll(store.eventsPath()); err != nil {
		t.Fatal(err)
	}
	reset()

	recoveryMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir") }
	if err := store.writeJSON(store.lastErrorPath(), map[string]any{"x": 1}); err == nil {
		t.Fatal("expected writeJSON ensure error")
	}
	if err := store.appendEvent(RecoveryEvent{}); err == nil {
		t.Fatal("expected appendEvent ensure error")
	}
	if err := store.ensureDir(); err == nil {
		t.Fatal("expected ensureDir mkdir error")
	}
	reset()

	recoveryOpenFile = func(string, int, os.FileMode) (*os.File, error) { return nil, errors.New("open file") }
	if err := store.appendEvent(RecoveryEvent{}); err == nil {
		t.Fatal("expected appendEvent open error")
	}
	reset()

	recoveryFileWrite = func(*os.File, []byte) (int, error) { return 0, errors.New("write event") }
	if err := store.appendEvent(RecoveryEvent{}); err == nil {
		t.Fatal("expected appendEvent write error")
	}
	reset()

	cutoff := time.Now().UTC().Add(-recoveryMaxAge)
	if err := os.MkdirAll(store.dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.lastErrorPath(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	oldTime := cutoff.Add(-time.Hour)
	if err := os.Chtimes(store.lastErrorPath(), oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	if err := store.pruneLastError(cutoff); err != nil {
		t.Fatal(err)
	}
	reset()

	if err := os.WriteFile(store.lastErrorPath(), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	recoveryStat = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
	if err := store.pruneLastError(cutoff); err != nil {
		t.Fatal(err)
	}
	recoveryStat = func(string) (os.FileInfo, error) { return nil, errors.New("stat") }
	if err := store.pruneLastError(cutoff); err == nil {
		t.Fatal("expected last-error stat error")
	}
	reset()

	recoveryReadDir = func(string) ([]os.DirEntry, error) { return nil, errors.New("read dir") }
	if err := store.pruneOtherArtifacts(cutoff); err == nil {
		t.Fatal("expected artifact read error")
	}
	reset()

	artifact := filepath.Join(store.dir(), "artifact")
	if err := os.WriteFile(artifact, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	recoveryStat = func(path string) (os.FileInfo, error) {
		if path == artifact {
			return nil, os.ErrNotExist
		}
		return oldStat(path)
	}
	if err := store.pruneOtherArtifacts(cutoff); err != nil {
		t.Fatal(err)
	}
	recoveryStat = func(path string) (os.FileInfo, error) {
		if path == artifact {
			return nil, errors.New("stat")
		}
		return oldStat(path)
	}
	if err := store.pruneOtherArtifacts(cutoff); err == nil {
		t.Fatal("expected artifact stat error")
	}
	reset()

	if err := os.Chtimes(artifact, oldTime, oldTime); err != nil {
		t.Fatal(err)
	}
	recoveryRemoveAll = func(string) error { return errors.New("remove") }
	if err := store.pruneOtherArtifacts(cutoff); err == nil {
		t.Fatal("expected artifact removal error")
	}
}
