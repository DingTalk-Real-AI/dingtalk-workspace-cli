package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

func TestCrossPlatformCoverageWhiteboardTrace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("DWS_WHITEBOARD_TRACE_DIR", dir)
	runner := &countingErrorRunner{}
	caller := newToolCallerAdapter(runner, &GlobalFlags{DryRun: true})
	_, err := caller.CallTool(context.Background(), "whiteboard", "save_personal_whiteboard_tpl", map[string]any{"dryRun": true})
	if err != nil || runner.calls.Load() != 0 {
		t.Fatalf("dry-run contacted runner: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.json"))
	if len(files) != 1 {
		t.Fatalf("trace files: %v", files)
	}
	b, _ := os.ReadFile(files[0])
	var record map[string]any
	if err := json.Unmarshal(b, &record); err != nil {
		t.Fatal(err)
	}
	if record["stage"] != "local_dry_run" || record["remoteCallIssued"] != false {
		t.Fatalf("wrong origin: %s", b)
	}
	info, _ := os.Stat(files[0])
	if info.Mode().Perm()&0077 != 0 {
		t.Fatal("trace readable by other users")
	}

	raw := `{"content":[{"type":"text","text":"{\"executed\":false}"}],"structuredContent":{"scope":"personal"},"extra":{"nested":123}}`
	var result transport.ToolCallResult
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		t.Fatal(err)
	}
	inv := executor.NewHelperInvocation("test", "whiteboard", "save_personal_whiteboard_tpl", nil)
	traceWhiteboardTransportResponse(inv, result, nil)
	files, _ = filepath.Glob(filepath.Join(dir, "*.json"))
	if len(files) != 2 {
		t.Fatal(files)
	}
	found := false
	for _, file := range files {
		b, _ := os.ReadFile(file)
		if err := json.Unmarshal(b, &record); err != nil {
			t.Fatal(err)
		}
		if record["rawMcpResult"] == raw {
			found = true
		}
	}
	if !found {
		t.Fatal("raw MCP result was changed or lost")
	}
	inv.CanonicalProduct = "mail"
	traceWhiteboardTransportResponse(inv, result, nil)
	t.Setenv("DWS_WHITEBOARD_TRACE_DIR", "")
	inv.CanonicalProduct = "whiteboard"
	traceWhiteboardTransportResponse(inv, result, nil)
	files, _ = filepath.Glob(filepath.Join(dir, "*.json"))
	if len(files) != 2 {
		t.Fatal("logging enabled without opt-in or for other product")
	}
}
