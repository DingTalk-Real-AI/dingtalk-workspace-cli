package errors

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"strings"
	"testing"
)

func TestError_Unwrap_NilCause(t *testing.T) {
	t.Parallel()
	e := &Error{Category: CategoryInternal, Message: "test"}
	if e.Unwrap() != nil {
		t.Fatal("expected nil cause")
	}
}

func TestError_Unwrap_WithCause(t *testing.T) {
	t.Parallel()
	cause := fmt.Errorf("root cause")
	e := &Error{Category: CategoryInternal, Message: "wrapper", Cause: cause}
	if !stderrors.Is(e, cause) {
		t.Fatalf("expected cause, got %v", e.Unwrap())
	}
}

func TestWithCause_Option(t *testing.T) {
	t.Parallel()
	cause := fmt.Errorf("underlying")
	e := NewAPI("api error", WithCause(cause))
	var appErr *Error
	if !stderrors.As(e, &appErr) {
		t.Fatal("expected *Error type")
	}
	if !stderrors.Is(appErr.Cause, cause) {
		t.Fatal("missing cause")
	}
}

func TestErrorsIs_ChainTraversal(t *testing.T) {
	t.Parallel()
	sentinel := fmt.Errorf("sentinel")
	e := &Error{Category: CategoryAuth, Message: "auth failed", Cause: sentinel}
	if !stderrors.Is(e, sentinel) {
		t.Fatal("errors.Is should find sentinel through Unwrap")
	}
}

func TestPrintJSON_WithCause(t *testing.T) {
	t.Parallel()
	cause := fmt.Errorf("db connection failed")
	e := &Error{Category: CategoryInternal, Message: "save failed", Cause: cause}
	var buf bytes.Buffer
	if err := PrintJSON(&buf, e); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	if !strings.Contains(buf.String(), "db connection failed") {
		t.Fatalf("expected cause in JSON output: %s", buf.String())
	}
}

func TestPrintJSON_WithoutCause(t *testing.T) {
	t.Parallel()
	e := &Error{Category: CategoryAuth, Message: "unauthorized"}
	var buf bytes.Buffer
	if err := PrintJSON(&buf, e); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	if strings.Contains(buf.String(), `"cause"`) {
		t.Fatalf("should not contain cause key: %s", buf.String())
	}
}

func TestPrintJSON_AllFields(t *testing.T) {
	t.Parallel()
	rpcData := json.RawMessage(`{"detail":"oops"}`)
	e := NewAPI("full error",
		WithOperation("tools/call"),
		WithReason("timeout"),
		WithServerKey("doc"),
		WithHint("retry later"),
		WithRetryable(true),
		WithActions("action1", "action2"),
		WithSnapshot("/tmp/snap.json"),
		WithRPCCode(-32600),
		WithRPCData(rpcData),
		WithCause(fmt.Errorf("root")),
	)
	var buf bytes.Buffer
	if err := PrintJSON(&buf, e); err != nil {
		t.Fatalf("PrintJSON() error = %v", err)
	}
	out := buf.String()
	for _, expected := range []string{"timeout", "tools/call", "doc", "retry later", "action1", "snap.json", "-32600", "oops", "root"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("missing %q in output: %s", expected, out)
		}
	}
}

func TestPrintHuman_WithCause(t *testing.T) {
	t.Parallel()
	cause := fmt.Errorf("timeout")
	e := &Error{Category: CategoryDiscovery, Message: "discovery failed", Cause: cause}
	var buf bytes.Buffer
	PrintHuman(&buf, e)
	if !strings.Contains(buf.String(), "timeout") {
		t.Fatalf("expected cause in human output: %s", buf.String())
	}
}

func TestPrintHuman_AllFields(t *testing.T) {
	t.Parallel()
	e := NewAPI("api error",
		WithOperation("initialize"),
		WithReason("connection_refused"),
		WithServerKey("doc"),
		WithHint("check network"),
		WithRetryable(true),
		WithActions("run again"),
		WithSnapshot("/tmp/snap"),
		WithRPCCode(-32601),
		WithRPCData(json.RawMessage(`{"x":1}`)),
		WithCause(fmt.Errorf("network")),
	)
	var buf bytes.Buffer
	PrintHuman(&buf, e)
	out := buf.String()
	for _, expected := range []string{"API", "initialize", "connection_refused", "doc", "check network", "run again", "snap", "-32601", "network", "Retryable"} {
		if !strings.Contains(out, expected) {
			t.Fatalf("missing %q in human output: %s", expected, out)
		}
	}
}

func TestPrintHuman_NilError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := PrintHuman(&buf, nil); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if buf.Len() != 0 {
		t.Fatalf("expected empty output, got: %s", buf.String())
	}
}

func TestPrintHuman_PlainError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	PrintHuman(&buf, fmt.Errorf("plain error"))
	if !strings.Contains(buf.String(), "plain error") {
		t.Fatalf("expected plain error: %s", buf.String())
	}
}

func TestPrintJSON_PlainError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	PrintJSON(&buf, fmt.Errorf("plain error"))
	if !strings.Contains(buf.String(), "plain error") {
		t.Fatalf("expected plain error: %s", buf.String())
	}
}

func TestExitCode_AllCategories(t *testing.T) {
	t.Parallel()
	tests := []struct {
		cat  Category
		want int
	}{
		{CategoryAPI, 1},
		{CategoryAuth, 2},
		{CategoryValidation, 3},
		{CategoryDiscovery, 4},
		{CategoryInternal, 5},
	}
	for _, tt := range tests {
		e := &Error{Category: tt.cat}
		if got := e.ExitCode(); got != tt.want {
			t.Errorf("ExitCode(%s) = %d, want %d", tt.cat, got, tt.want)
		}
	}
}

func TestExitCode_NonTyped(t *testing.T) {
	t.Parallel()
	if got := ExitCode(fmt.Errorf("plain")); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
}

func TestWithRPCData_Empty(t *testing.T) {
	t.Parallel()
	opt := WithRPCData(json.RawMessage(""))
	if opt == nil {
		t.Fatal("expected non-nil no-op option for empty RPC data")
	}
	// Should not panic when applied
	e := &Error{}
	opt(e)
	if len(e.RPCData) != 0 {
		t.Fatal("empty RPC data option should be a no-op")
	}
}

func TestWithActions_Empty(t *testing.T) {
	t.Parallel()
	e := NewAPI("test", WithActions("", "", ""))
	var appErr *Error
	stderrors.As(e, &appErr)
	if len(appErr.Actions) != 0 {
		t.Fatalf("expected no actions, got %v", appErr.Actions)
	}
}

func TestSafePath_Valid(t *testing.T) {
	t.Parallel()
	if err := SafePath("/tmp/test.json"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSafePath_Traversal(t *testing.T) {
	t.Parallel()
	if err := SafePath("../../etc/passwd"); err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestSafePath_Empty(t *testing.T) {
	t.Parallel()
	if err := SafePath(""); err == nil {
		t.Fatal("expected error for empty path")
	}
}
