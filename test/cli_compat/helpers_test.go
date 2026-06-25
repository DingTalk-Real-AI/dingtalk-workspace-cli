package cli_compat_test

import (
	"bytes"
	"encoding/json"
	"sync"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/app"
	"github.com/spf13/cobra"
)

// ═══════════════════════════════════════════════════════════════
// Backward-compatible shim: matches the old CLI test harness API
// but uses dws NewRootCommand + EchoRunner under the hood.
// ═══════════════════════════════════════════════════════════════

// ToolResult mirrors mcp.ToolResult for test compatibility with the reference test suite.
type ToolResult struct {
	Content []ContentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

// ContentBlock mirrors mcp.ContentBlock for test compatibility.
type ContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

type capturedCall struct {
	ToolName string
	Args     map[string]any
	Endpoint string
}

type mcpCallCapture struct {
	mu      sync.Mutex
	calls   []capturedCall
	dryRun  bool
	preview bool
	confirm bool
}

func (c *mcpCallCapture) record(name string, args map[string]any, endpoint string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, capturedCall{ToolName: name, Args: args, Endpoint: endpoint})
}

func (c *mcpCallCapture) last() *capturedCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.calls) == 0 {
		return nil
	}
	return &c.calls[len(c.calls)-1]
}

func (c *mcpCallCapture) all() []capturedCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]capturedCall, len(c.calls))
	copy(out, c.calls)
	return out
}

// ── per-test capture registry ───────────────────────────────

var (
	captureMu sync.Mutex
	captures  = map[*testing.T]*mcpCallCapture{}
)

func linkCapture(t *testing.T, cap *mcpCallCapture) {
	captureMu.Lock()
	captures[t] = cap
	captureMu.Unlock()
	t.Cleanup(func() {
		captureMu.Lock()
		delete(captures, t)
		captureMu.Unlock()
	})
}

func getCapture(t *testing.T) *mcpCallCapture {
	captureMu.Lock()
	defer captureMu.Unlock()
	return captures[t]
}

// ── setup ───────────────────────────────────────────────────

func setupTestDeps(t *testing.T, _ string) *mcpCallCapture {
	t.Helper()
	cap := &mcpCallCapture{}
	linkCapture(t, cap)
	return cap
}

func setupTestDepsWithDryRun(t *testing.T, product string) *mcpCallCapture {
	t.Helper()
	cap := setupTestDeps(t, product)
	cap.dryRun = true
	return cap
}

func setupTestDepsWithPreview(t *testing.T, product string) *mcpCallCapture {
	t.Helper()
	cap := setupTestDeps(t, product)
	cap.preview = true
	return cap
}

func setupTestDepsAutoConfirm(t *testing.T, product string) *mcpCallCapture {
	t.Helper()
	cap := setupTestDeps(t, product)
	cap.confirm = true
	return cap
}

func buildRoot() *cobra.Command {
	return app.NewRootCommand()
}

// ── execution ───────────────────────────────────────────────

func execCmd(t *testing.T, root *cobra.Command, path []string, flags map[string]string) error {
	t.Helper()
	return execCmdWithArgs(t, root, path, flags, nil)
}

func execCmdWithArgs(t *testing.T, root *cobra.Command, path []string, flags map[string]string, args []string) error {
	t.Helper()

	cap := getCapture(t)

	cliArgs := []string{"-f", "json"}
	cliArgs = append(cliArgs, path...)

	// Add dry-run if capture says so
	if cap != nil && (cap.dryRun || cap.preview) {
		cliArgs = append(cliArgs, "--dry-run")
	}
	// Add --yes if auto-confirm
	if cap != nil && cap.confirm {
		cliArgs = append(cliArgs, "--yes")
	}

	for k, v := range flags {
		if v == "" {
			continue
		}
		cliArgs = append(cliArgs, "--"+k, v)
	}
	cliArgs = append(cliArgs, args...)

	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&bytes.Buffer{})
	root.SetArgs(cliArgs)

	err := root.Execute()
	if err != nil {
		return err
	}

	// Parse EchoRunner output and record into capture
	var inv struct {
		Invocation struct {
			Tool   string         `json:"tool"`
			Params map[string]any `json:"params"`
			DryRun bool           `json:"dry_run"`
		} `json:"invocation"`
	}
	var flatInv struct {
		Tool   string         `json:"tool"`
		Params map[string]any `json:"params"`
		DryRun bool           `json:"dry_run"`
	}
	if cap != nil {
		if jsonErr := json.Unmarshal(out.Bytes(), &inv); jsonErr == nil && inv.Invocation.Tool != "" {
			if !inv.Invocation.DryRun || cap.preview {
				cap.record(inv.Invocation.Tool, inv.Invocation.Params, "")
			}
			// For dry-run: don't record (matches old behavior: assertCallCount == 0)
		} else if jsonErr := json.Unmarshal(out.Bytes(), &flatInv); jsonErr == nil && flatInv.Tool != "" {
			dryRunPreview := flatInv.DryRun || cap.dryRun || cap.preview
			if !dryRunPreview || cap.preview {
				cap.record(flatInv.Tool, flatInv.Params, "")
			}
		}
	}
	return nil
}

// ── asserts (use the passed *mcpCallCapture directly) ───────

func assertToolName(t *testing.T, cap *mcpCallCapture, expected string) {
	t.Helper()
	last := cap.last()
	if last == nil {
		t.Fatalf("expected MCP call to %q, but no calls were captured", expected)
	}
	if last.ToolName != expected {
		t.Errorf("expected tool name %q, got %q", expected, last.ToolName)
	}
}

func assertToolArg(t *testing.T, cap *mcpCallCapture, key string, expected any) {
	t.Helper()
	last := cap.last()
	if last == nil {
		t.Fatalf("expected MCP call with arg %q=%v, but no calls were captured", key, expected)
	}
	got, ok := last.Args[key]
	if !ok {
		t.Errorf("expected arg %q in MCP call, but it was not present. args: %v", key, last.Args)
		return
	}
	gotJSON, _ := json.Marshal(got)
	expectedJSON, _ := json.Marshal(expected)
	if string(gotJSON) != string(expectedJSON) {
		t.Errorf("arg %q: expected %v, got %v", key, string(expectedJSON), string(gotJSON))
	}
}

func assertArgNotPresent(t *testing.T, cap *mcpCallCapture, key string) {
	t.Helper()
	last := cap.last()
	if last == nil {
		t.Fatalf("no MCP call captured")
	}
	if _, ok := last.Args[key]; ok {
		t.Errorf("expected arg %q to be absent, but it was present: %v", key, last.Args[key])
	}
}

func assertNilArgs(t *testing.T, cap *mcpCallCapture) {
	t.Helper()
	last := cap.last()
	if last == nil {
		t.Fatal("expected MCP call, but no calls were captured")
	}
	if len(last.Args) > 0 {
		t.Errorf("expected nil/empty args, got %v", last.Args)
	}
}

// assertCallCount asserts the number of captured MCP calls.
func assertCallCount(t *testing.T, capture *mcpCallCapture, expected int) {
	t.Helper()
	calls := capture.all()
	if len(calls) != expected {
		t.Errorf("expected %d MCP calls, got %d", expected, len(calls))
	}
}
