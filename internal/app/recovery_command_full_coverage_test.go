package app

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/cli"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/recovery"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func recoveryCoverageRun(cmdArgs ...string) (string, error) {
	cmd := newRecoveryCommand(context.Background(), cli.StaticLoader{}, &GlobalFlags{})
	out := &strings.Builder{}
	cmd.SetOut(out)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(cmdArgs)
	err := cmd.Execute()
	return out.String(), err
}

func TestCrossPlatformCoverageRecoveryCommandRemainingCoverage(t *testing.T) {
	oldSavePlan, oldSaveAnalysis := recoverySavePlan, recoverySaveAnalysis
	t.Cleanup(func() {
		recoverySavePlan, recoverySaveAnalysis = oldSavePlan, oldSaveAnalysis
	})
	configDir := t.TempDir()
	t.Setenv("DWS_CONFIG_DIR", configDir)
	store := recovery.NewStore(configDir)
	last, err := store.Capture(recovery.RecoveryContext{ServerID: "doc", ToolName: "get"})
	if err != nil {
		t.Fatal(err)
	}
	recoverySavePlan = func(*recovery.Store, string, recovery.RecoveryPlan) error { return errors.New("save plan") }
	if _, err := recoveryCoverageRun("plan", "--last"); err == nil {
		t.Fatal("injected plan save failure succeeded")
	}
	recoverySavePlan = oldSavePlan
	recoverySaveAnalysis = func(*recovery.Store, string, recovery.RecoveryPlan, recovery.RecoveryBundle) error {
		return errors.New("save analysis")
	}
	if _, err := recoveryCoverageRun("execute", "--last"); err == nil {
		t.Fatal("injected analysis save failure succeeded")
	}
	recoverySaveAnalysis = oldSaveAnalysis

	parent := newRecoveryCommand(context.Background(), nil, nil)
	parent.SetOut(io.Discard)
	if err := parent.RunE(parent, nil); err != nil {
		t.Fatal(err)
	}
	if out, err := recoveryCoverageRun("plan", "--last"); err != nil || !strings.Contains(out, last.EventID) {
		t.Fatalf("recovery plan = %q, %v", out, err)
	}
	if out, err := recoveryCoverageRun("execute", "--event-id", last.EventID); err != nil || out == "" {
		t.Fatalf("recovery execute = %q, %v", out, err)
	}

	for _, args := range [][]string{
		{"finalize"},
		{"finalize", "--event-id", last.EventID},
		{"finalize", "--event-id", last.EventID, "--outcome", "unknown"},
		{"finalize", "--event-id", last.EventID, "--outcome", "recovered", "--execution-file", "missing"},
	} {
		if _, err := recoveryCoverageRun(args...); err == nil {
			t.Fatalf("recovery finalize %#v should fail", args)
		}
	}
	executionPath := filepath.Join(t.TempDir(), "execution.json")
	if err := os.WriteFile(executionPath, []byte(`{"action":"retry","attempt":1,"result":"ok"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if out, err := recoveryCoverageRun("finalize", "--event-id", last.EventID, "--outcome", "handoff", "--execution-file", executionPath); err != nil || !strings.Contains(out, "execution_recorded") {
		t.Fatalf("recovery finalize = %q, %v", out, err)
	}
	if _, err := recoveryCoverageRun("finalize", "--event-id", last.EventID, "--outcome", "failed"); err != nil {
		t.Fatal(err)
	}

	if _, err := loadRecoverySnapshot(store, true, last.EventID); err == nil {
		t.Fatal("conflicting snapshot selectors should fail")
	}
	if _, err := loadRecoverySnapshot(store, false, "missing"); err == nil {
		t.Fatal("missing event snapshot should fail")
	}
	if _, err := loadRecoverySnapshot(store, false, ""); err == nil {
		t.Fatal("empty snapshot selector should fail")
	}
	missingStore := recovery.NewStore(t.TempDir())
	if _, err := loadRecoverySnapshot(missingStore, true, ""); err == nil {
		t.Fatal("missing latest snapshot should fail")
	}

	eventsPath := filepath.Join(configDir, "recovery", "recovery_events.jsonl")
	if err := os.Remove(eventsPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(eventsPath, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := recoveryCoverageRun("plan", "--last"); err == nil {
		t.Fatal("recovery plan save should fail")
	}
	if _, err := recoveryCoverageRun("execute", "--last"); err == nil {
		t.Fatal("recovery analysis save should fail")
	}
	if _, err := recoveryCoverageRun("finalize", "--event-id", last.EventID, "--outcome", "recovered"); err == nil {
		t.Fatal("recovery finalization save should fail")
	}
}

func TestCrossPlatformCoverageRecoveryExecutionAndRuntimeRemainingCoverage(t *testing.T) {
	t.Setenv("DINGTALK_DEVDOC_MCP_URL", "http://127.0.0.1:1")
	path := filepath.Join(t.TempDir(), "execution.json")
	if err := os.WriteFile(path, []byte(`{"attempts":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadRecoveryExecution(path); err == nil {
		t.Fatal("invalid attempts should fail")
	}
	if _, err := decodeRecoveryAttempts([]byte(`[{}`), nil, "", ""); err == nil {
		t.Fatal("invalid attempt array should fail")
	}

	fail := errors.New("catalog")
	SetDynamicServers(nil)
	runtime := &recoveryRuntime{
		loader:    cli.CatalogLoaderFrom(cli.Catalog{}, fail),
		transport: transport.NewClient(nil),
	}
	if _, err := runtime.CallToolDirect(context.Background(), "missing", "tool", nil); !errors.Is(err, fail) {
		t.Fatalf("direct resolution error = %v", err)
	}
	if got, err := runtime.Search(context.Background(), "query", recovery.RecoveryContext{}); err == nil || got.DocSearch.Status != "error" {
		t.Fatalf("search error = %#v, %v", got, err)
	}
	if got := parseDocSearchItems(&transport.ToolCallResult{Content: map[string]any{}, Blocks: []transport.ContentBlock{{Text: "not-json"}}}); got != nil {
		t.Fatalf("empty doc search items = %#v", got)
	}
	for _, payload := range []map[string]any{
		{"data": map[string]any{}},
		{"result": map[string]any{}},
	} {
		if got := parseDocSearchItemsFromMap(payload); got != nil {
			t.Fatalf("empty nested doc search items = %#v", got)
		}
	}
}
