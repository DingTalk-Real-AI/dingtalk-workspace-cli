//go:build !windows

package mock_mcp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sync/atomic"
	"testing"
)

func TestMockMCPSmoke_MailBatchCancellationPreservesSignalExit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), mockMCPCLIExecutionTimeout)
	defer cancel()
	ready := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			ID json.RawMessage `json:"id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Error(err)
			return
		}
		var reply string
		switch calls.Add(1) {
		case 1:
			reply = `{"success":false,"errorCode":"SYSTEM_ERROR","errorMsg":"orgEmails[1]必须是完整有效的企业邮箱地址"}`
		case 2:
			reply = `{"success":true,"result":{"uid":123,"orgEmail":"a@example.com"}}`
		case 3:
			close(ready)
			select {
			case <-r.Context().Done():
			case <-ctx.Done():
			}
			return
		default:
			t.Error("mail lookup continued after cancellation")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": request.ID,
			"result": map[string]any{"content": []map[string]string{{"type": "text", "text": reply}}}})
	}))
	defer server.Close()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestCLIHelperProcess$", "--", "--token", "ci-smoke-token", "--format", "json", "mail", "user", "batch-get", "--org-emails", "a@example.com,invalid,missing@example.com")
	cmd.Env = isolatedCLIEnv(t, map[string]string{"DINGTALK_MAIL_MCP_URL": server.URL + "/mcp/mail"})
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill(); _ = cmd.Wait() }()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatal("CLI did not reach the fallback request")
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		t.Fatal(err)
	}
	err := cmd.Wait()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 130 || calls.Load() != 3 {
		t.Fatalf("cancellation: err=%v calls=%d stdout=%s stderr=%s", err, calls.Load(), stdout.String(), stderr.String())
	}
	var result struct {
		Outcome string `json:"outcome"`
		Error   struct {
			Subtype  string `json:"subtype"`
			ExitCode int    `json:"exit_code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil || result.Outcome != "failure" || result.Error.Subtype != "cancelled_by_user" || result.Error.ExitCode != 130 {
		t.Fatalf("signal was downgraded to an item failure: %s (%v)", stdout.String(), err)
	}
}
