package app

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/auth"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageAitableChartLocalReasonIsPersisted(t *testing.T) {
	t.Setenv("DWS_CONFIG_DIR", t.TempDir())
	t.Setenv("DINGTALK_AITABLE_MCP_URL", "https://mock-mcp-aitable.dingtalk.com")
	previousProfile := auth.RuntimeProfile()
	auth.SetRuntimeProfile("")
	t.Cleanup(func() { auth.SetRuntimeProfile(previousProfile) })
	resetAuditIdentityCache()
	t.Cleanup(resetAuditIdentityCache)
	testseam.Swap(t, &loadTokenForProfile, func(string, string) (*auth.TokenData, error) {
		return &auth.TokenData{UserID: "review-user", CorpID: "review-org"}, nil
	})

	dir := t.TempDir()
	writer, err := audit.NewDateRotatingWriter(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	sink := audit.NewFileSink(writer, audit.NewChain(dir), nil)
	t.Cleanup(func() { _ = sink.Close() })
	flags := &GlobalFlags{Mock: true, Format: "json"}
	runner := newCommandRunnerWithFlags(flags).(*runtimeRunner)
	runner.auditSink = sink
	caller := newToolCallerAdapter(runner, flags)
	params := map[string]any{"baseId": "b", "dashboardId": "d", "chartId": "c", "confirm": true}
	wantParams := map[string]any{"baseId": "b", "dashboardId": "d", "chartId": "c", "confirm": true}
	reason := "清理重复图表\n保留主图"
	for _, ctx := range []context.Context{audit.WithLocalReason(t.Context(), reason), t.Context()} {
		if _, err := caller.CallTool(ctx, "aitable", "delete_chart", params); err != nil {
			t.Fatal(err)
		}
	}
	// Error events retain the same local remark as successful invocations.
	emitAudit(audit.WithLocalReason(t.Context(), reason), sink, "failed-delete", time.Now(),
		executor.Invocation{CanonicalProduct: "aitable", Tool: "delete_chart", Params: params},
		"https://mock-mcp-aitable.dingtalk.com", errors.New("permission denied"), "test")
	if !reflect.DeepEqual(params, wantParams) {
		t.Fatalf("audit mutated MCP arguments: %v", params)
	}
	if err := sink.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := audit.LatestAuditFile(dir)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("audit lines=%d, want 3", len(lines))
	}
	for i, line := range lines {
		var event audit.Event
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatal(err)
		}
		var summary map[string]any
		if err := json.Unmarshal([]byte(event.ParamsSummary), &summary); err != nil {
			t.Fatal(err)
		}
		if i == 1 {
			if _, exists := summary["_local"]; exists {
				t.Fatalf("remark leaked into following invocation: %v", summary)
			}
		} else {
			local, ok := summary["_local"].(map[string]any)
			if !ok || local["reason"] != reason {
				t.Fatalf("audit lost local remark: %v", summary)
			}
		}
		wantResult := "success"
		if i == 2 {
			wantResult = "error"
		}
		if event.Command != "delete_chart" || event.Result != wantResult {
			t.Fatalf("unexpected audit outcome: %+v", event)
		}
		for _, level := range []audit.RedactLevel{audit.RedactHashed, audit.RedactMinimal} {
			if got := audit.RedactEvent(event, level); got.ParamsSummary != "" {
				t.Fatalf("%s forwarding retained local metadata: %q", level, got.ParamsSummary)
			}
		}
	}
	if valid, brokenAt, err := audit.VerifyFile(file); err != nil || !valid {
		t.Fatalf("audit chain: valid=%v brokenAt=%d err=%v", valid, brokenAt, err)
	}
}
