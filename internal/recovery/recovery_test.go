package recovery

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/platform/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtime/transport"
)

func TestBuildContext_SanitizesArgsAndPreservesTypedFields(t *testing.T) {
	rawErr := &transport.CallError{
		Stage:      transport.CallStageHTTP,
		HTTPStatus: 429,
		RetryAfter: "3",
		TraceID:    "trace-1",
		RequestID:  "req-1",
		Cause:      errors.New("HTTP 429: too many requests"),
	}
	wrapped := apperrors.NewAPI(
		"request failed",
		apperrors.WithReason("http_429"),
		apperrors.WithRetryable(true),
		apperrors.WithCause(rawErr),
	)

	ctx := BuildContext(CaptureInput{
		CommandPath: []string{"chat", "message", "send"},
		ServerID:    "chat",
		ToolName:    "send_message_as_user",
		Args: map[string]any{
			"text":    "very secret text body",
			"token":   "secret-token",
			"payload": map[string]any{"header": "sensitive", "name": "demo"},
		},
		RawErr:     rawErr,
		WrappedErr: wrapped,
	})

	if ctx.CLIErrorCode != cliRateLimitCode {
		t.Fatalf("expected CLI error code %q, got %q", cliRateLimitCode, ctx.CLIErrorCode)
	}
	if ctx.CallStage != string(transport.CallStageHTTP) || ctx.HTTPStatus != 429 {
		t.Fatalf("expected typed call metadata, got stage=%q status=%d", ctx.CallStage, ctx.HTTPStatus)
	}
	if ctx.RetryAfter != "3" || ctx.TraceID != "trace-1" || ctx.RequestID != "req-1" {
		t.Fatalf("expected transport metadata, got %#v", ctx)
	}
	if _, ok := ctx.ArgsSummary["token"]; ok {
		t.Fatal("sensitive args must be dropped")
	}
	textSummary, ok := ctx.ArgsSummary["text"].(map[string]any)
	if !ok {
		t.Fatalf("expected text summary, got %#v", ctx.ArgsSummary["text"])
	}
	if int(textSummary["length"].(float64)) != len("very secret text body") {
		t.Fatalf("expected text summary length, got %#v", textSummary)
	}
	if ctx.Fingerprint == "" {
		t.Fatal("expected fingerprint to be computed")
	}
}

func TestBuildReplay_SummarizesContentArgsAndSanitizesNestedValues(t *testing.T) {
	replay := BuildReplay(CaptureInput{
		CommandPath: []string{"chat", "message", "send"},
		ServerID:    "chat",
		ToolName:    "send_message_as_user",
		Args: map[string]any{
			"text": "very secret text body",
			"records": []map[string]any{{
				"title": "demo record",
				"token": "secret-token",
			}},
			"payload": map[string]any{
				"header": "sensitive",
				"name":   "demo",
				"text":   "nested body",
			},
			"baseId": "base_demo",
		},
		Argv: []string{"chat", "message", "send", "--token", "secret-token"},
	})

	textSummary, ok := replay.ToolArgs["text"].(map[string]any)
	if !ok {
		t.Fatalf("expected text summary in replay, got %#v", replay.ToolArgs["text"])
	}
	if int(textSummary["length"].(float64)) != len("very secret text body") {
		t.Fatalf("expected text length summary, got %#v", textSummary)
	}

	recordsSummary, ok := replay.ToolArgs["records"].(map[string]any)
	if !ok {
		t.Fatalf("expected records summary in replay, got %#v", replay.ToolArgs["records"])
	}
	if recordsSummary["kind"] != "array" || int(recordsSummary["count"].(float64)) != 1 {
		t.Fatalf("expected records count summary, got %#v", recordsSummary)
	}

	payload, ok := replay.ToolArgs["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested payload in replay, got %#v", replay.ToolArgs["payload"])
	}
	if _, ok := payload["header"]; ok {
		t.Fatalf("expected nested sensitive field to be removed, got %#v", payload)
	}
	if payload["name"] != "demo" {
		t.Fatalf("expected non-sensitive nested field to remain, got %#v", payload)
	}
	nestedText, ok := payload["text"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested text summary, got %#v", payload["text"])
	}
	if int(nestedText["length"].(float64)) != len("nested body") {
		t.Fatalf("expected nested text length summary, got %#v", nestedText)
	}

	if replay.ToolArgs["baseId"] != "base_demo" {
		t.Fatalf("expected replayable identifier to remain, got %#v", replay.ToolArgs["baseId"])
	}
	if len(replay.RedactedArgv) != 5 || replay.RedactedArgv[4] != "<redacted>" {
		t.Fatalf("expected argv token to be redacted, got %#v", replay.RedactedArgv)
	}
}

func TestComputeFingerprint_NormalizesVolatileSegments(t *testing.T) {
	ctxA := RecoveryContext{
		ServerID:     "aitable",
		ToolName:     "get_base",
		CLIErrorCode: cliResourceNotFoundCode,
		HTTPStatus:   404,
		RawError:     "resource base_abcd1234 not found at 2026-03-19T10:10:10Z trace=550e8400-e29b-41d4-a716-446655440000",
	}
	ctxB := RecoveryContext{
		ServerID:     "aitable",
		ToolName:     "get_base",
		CLIErrorCode: cliResourceNotFoundCode,
		HTTPStatus:   404,
		RawError:     "resource base_zzzz9999 not found at 2026-03-20T10:10:10Z trace=660e8400-e29b-41d4-a716-446655440000",
	}
	if gotA, gotB := ComputeFingerprint(ctxA), ComputeFingerprint(ctxB); gotA != gotB {
		t.Fatalf("expected normalized fingerprint to stay stable, got %q vs %q", gotA, gotB)
	}
}

func TestPlanner_ExactRulesAndFallback(t *testing.T) {
	planner := NewPlanner(fakeRetriever{retrieval: KnowledgeRetrieval{
		KBHits: []KBHit{{Source: "docs", Title: "doc"}},
		DocSearch: DocSearch{
			Provider: "open_platform_docs",
			Query:    "get_approval_instance approval approval get weird upstream issue",
			Status:   "success",
			Items: []DocSearchItem{{
				Title: "doc",
				URL:   "https://open.dingtalk.com/doc",
				Desc:  "审批接口文档",
			}},
		},
	}})

	authPlan := planner.Plan(context.Background(), RecoveryContext{
		CLIErrorCode:  cliAuthExpiredCode,
		OperationKind: OperationRead,
	})
	if authPlan.Category != "auth" || !authPlan.ShouldRetry {
		t.Fatalf("expected auth plan with retry, got %#v", authPlan)
	}
	if len(authPlan.SafeActions) != 1 || authPlan.SafeActions[0] != "rerun_original_command" {
		t.Fatalf("expected safe action alias for auth plan, got %#v", authPlan.SafeActions)
	}
	if !authPlan.DecisionHints.AuthRelated || !authPlan.DecisionHints.Retryable {
		t.Fatalf("expected auth decision hints, got %#v", authPlan.DecisionHints)
	}
	if authPlan.AgentRoute.Required {
		t.Fatalf("expected auth retryable plan to stay on rule path, got %#v", authPlan.AgentRoute)
	}

	unknownPlan := planner.Plan(context.Background(), RecoveryContext{
		CommandPath:   []string{"approval", "approval", "get"},
		ToolName:      "get_approval_instance",
		OperationKind: OperationUnknown,
		RawError:      "weird upstream issue",
	})
	if unknownPlan.Category != "unknown" {
		t.Fatalf("expected unknown category, got %s", unknownPlan.Category)
	}
	if unknownPlan.DecisionOwner != DecisionOwnerAgent {
		t.Fatalf("expected unknown plan to be agent-owned, got %#v", unknownPlan.DecisionOwner)
	}
	if len(unknownPlan.KBHits) == 0 {
		t.Fatal("expected fallback retriever hits")
	}
	if !strings.Contains(strings.Join(unknownPlan.Evidence, " "), "fallback_query=") {
		t.Fatalf("expected fallback query evidence, got %#v", unknownPlan.Evidence)
	}
	if unknownPlan.ShouldRetry || unknownPlan.ShouldStop {
		t.Fatalf("expected unknown plan to stay undecided, got %#v", unknownPlan)
	}
	if len(unknownPlan.HumanActions) != 0 {
		t.Fatalf("expected unknown plan to avoid built-in human actions, got %#v", unknownPlan.HumanActions)
	}
	if !unknownPlan.AgentRoute.Required {
		t.Fatalf("expected unknown plan to force agent route, got %#v", unknownPlan.AgentRoute)
	}
	if len(unknownPlan.AgentRoute.Reasons) != 1 || unknownPlan.AgentRoute.Reasons[0] != "unknown_category" {
		t.Fatalf("expected unknown_category route reason, got %#v", unknownPlan.AgentRoute.Reasons)
	}
	if unknownPlan.AgentRoute.Payload == nil || unknownPlan.AgentRoute.Payload.DecisionOwner != DecisionOwnerAgent {
		t.Fatalf("expected unknown payload to include agent decision owner, got %#v", unknownPlan.AgentRoute.Payload)
	}
	if unknownPlan.DocSearch.Status != "success" || len(unknownPlan.DocSearch.Items) != 1 {
		t.Fatalf("expected doc search payload to be preserved, got %#v", unknownPlan.DocSearch)
	}
	if unknownPlan.RuleHints.DecisionOwner != DecisionOwnerAgent || unknownPlan.RuleHints.Confidence != unknownPlan.Confidence {
		t.Fatalf("expected rule hints to mirror final unknown plan state, got %#v", unknownPlan.RuleHints)
	}
}

func TestStore_CaptureLoadAndFinalizeLifecycle(t *testing.T) {
	dir := t.TempDir()
	store := NewStore(dir)

	ctx := RecoveryContext{
		CommandPath:   []string{"aitable", "base", "get"},
		ServerID:      "aitable",
		ToolName:      "get_base",
		OperationKind: OperationRead,
		CLIErrorCode:  cliResourceNotFoundCode,
		RawError:      "base_not_found",
		Fingerprint:   "fp-1",
	}
	replay := Replay{
		ServerID:      "aitable",
		ToolName:      "get_base",
		OperationKind: OperationRead,
		ToolArgs:      map[string]any{"baseId": "base_1"},
	}

	last, err := store.Capture(ctx, replay)
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if last == nil || last.EventID == "" {
		t.Fatalf("Capture() = %#v, want event id", last)
	}

	loaded, err := store.LoadLastError()
	if err != nil {
		t.Fatalf("LoadLastError() error = %v", err)
	}
	if loaded.EventID != last.EventID {
		t.Fatalf("LoadLastError().EventID = %q, want %q", loaded.EventID, last.EventID)
	}

	byEvent, err := store.LoadErrorByEvent(last.EventID)
	if err != nil {
		t.Fatalf("LoadErrorByEvent() error = %v", err)
	}
	if byEvent.EventID != last.EventID {
		t.Fatalf("LoadErrorByEvent().EventID = %q, want %q", byEvent.EventID, last.EventID)
	}

	plan := RecoveryPlan{Category: "resource", Confidence: 0.92}
	if err := store.SavePlan(last.EventID, plan); err != nil {
		t.Fatalf("SavePlan() error = %v", err)
	}
	bundle := RecoveryBundle{EventID: last.EventID, Plan: plan, Status: "needs_agent_action"}
	if err := store.SaveAnalysis(last.EventID, plan, bundle); err != nil {
		t.Fatalf("SaveAnalysis() error = %v", err)
	}
	exec := &RecoveryExecution{
		Actions: []string{"inspect_bundle"},
		Result:  "handoff",
	}
	if err := store.Finalize(last.EventID, "handoff", exec); err != nil {
		t.Fatalf("Finalize() error = %v", err)
	}

	eventsPath := filepath.Join(dir, "recovery", "recovery_events.jsonl")
	data, err := os.ReadFile(eventsPath)
	if err != nil {
		t.Fatalf("ReadFile(events) error = %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("expected 4 recovery event lines, got %d", len(lines))
	}
	var lastEvent RecoveryEvent
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &lastEvent); err != nil {
		t.Fatalf("json.Unmarshal(final event) error = %v", err)
	}
	if lastEvent.Phase != "finalized" || lastEvent.Outcome != "handoff" {
		t.Fatalf("unexpected final event %#v", lastEvent)
	}
}

type fakeRetriever struct {
	retrieval KnowledgeRetrieval
	err       error
}

func (f fakeRetriever) Search(ctx context.Context, query string, rc RecoveryContext) (KnowledgeRetrieval, error) {
	return f.retrieval, f.err
}
