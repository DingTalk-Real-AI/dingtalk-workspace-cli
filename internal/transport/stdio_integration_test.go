// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package transport

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestStdioClientEndToEnd tests the full stdio MCP lifecycle:
// Start → Initialize → ListTools → CallTool → Stop.
//
// It compiles a minimal MCP server helper from testdata and runs it as
// a subprocess, exercising the real JSON-RPC protocol over stdin/stdout.
func TestStdioClientEndToEnd(t *testing.T) {
	// Build the test helper server.
	helperBin := buildTestHelper(t)

	client := NewStdioClient(helperBin, nil, nil)

	// Use background context for Start so subprocess lives for the test duration.
	if err := client.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer client.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Initialize
	_, err := client.Initialize(ctx)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// ListTools
	toolsResult, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(toolsResult.Tools) == 0 {
		t.Fatal("ListTools: no tools returned")
	}

	// Find the test_echo tool
	found := false
	for _, tool := range toolsResult.Tools {
		if tool.Name == "test_echo" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("ListTools: test_echo tool not found, got tools: %v", toolNames(toolsResult.Tools))
	}

	// CallTool
	callResult, err := client.CallTool(ctx, "test_echo", map[string]any{
		"message": "hello world",
	})
	if err != nil {
		t.Fatalf("CallTool: %v", err)
	}
	if callResult.IsError {
		t.Fatalf("CallTool returned isError=true")
	}

	// Verify response content
	if len(callResult.Blocks) == 0 {
		t.Fatal("CallTool: no content blocks")
	}
	if callResult.Blocks[0].Text != "Echo: hello world" {
		t.Errorf("CallTool text = %q, want %q", callResult.Blocks[0].Text, "Echo: hello world")
	}

	// CallTool with unknown tool should return RPC error
	_, err = client.CallTool(ctx, "nonexistent", nil)
	if err == nil {
		t.Error("CallTool with unknown tool should return error")
	}

	// Stop — after kill, Stop should return nil (non-zero exit is suppressed)
	if err := client.Stop(); err != nil {
		t.Errorf("Stop after kill should return nil, got %v", err)
	}
}

func TestStdioClientStartFailsWithBadCommand(t *testing.T) {
	client := NewStdioClient("/nonexistent/binary", nil, nil)
	err := client.Start(context.Background())
	if err == nil {
		t.Error("expected error when starting with nonexistent binary")
	}
}

func TestStdioClientCallBeforeStart(t *testing.T) {
	client := NewStdioClient("echo", nil, nil)
	_, err := client.CallTool(context.Background(), "test", nil)
	if err == nil {
		t.Error("expected error when calling before Start")
	}
}

// buildTestHelper compiles testdata/stdio_test_server.go into a temporary binary.
func buildTestHelper(t *testing.T) string {
	t.Helper()
	serverSrc := filepath.Join("testdata", "stdio_test_server.go")
	if _, err := os.Stat(serverSrc); err != nil {
		t.Skipf("testdata/stdio_test_server.go not found: %v", err)
	}

	binPath := filepath.Join(t.TempDir(), "stdio-test-server")
	cmd := exec.Command("go", "build", "-o", binPath, serverSrc)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("failed to build test helper: %v\n%s", err, out)
	}
	return binPath
}

func toolNames(tools []ToolDescriptor) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = t.Name
	}
	return names
}

// TestStdioProtocolNewlineDelimited verifies that the protocol is correctly
// newline-delimited (one JSON object per line).
func TestStdioProtocolNewlineDelimited(t *testing.T) {
	helperBin := buildTestHelper(t)

	cmd := exec.Command(helperBin)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()

	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer cmd.Process.Kill()

	scanner := bufio.NewScanner(stdout)

	// Send initialize
	req := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n"
	fmt.Fprint(stdin, req)

	if !scanner.Scan() {
		t.Fatal("no response from server")
	}
	var resp struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      int             `json:"id"`
		Result  json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp.JSONRPC != "2.0" {
		t.Errorf("jsonrpc = %q, want 2.0", resp.JSONRPC)
	}
	if resp.ID != 1 {
		t.Errorf("id = %d, want 1", resp.ID)
	}

	stdin.Close()
	cmd.Wait()
}
