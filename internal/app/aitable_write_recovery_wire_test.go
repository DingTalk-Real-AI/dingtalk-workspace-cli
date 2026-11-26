// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/audit"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// This caller replaces only endpoint discovery; real JSON-RPC encoding reaches the local HTTP server.
type aitableRecoveryWireCaller struct {
	client   *transport.Client
	endpoint string
}

func (c *aitableRecoveryWireCaller) CallTool(ctx context.Context, _, tool string, args map[string]any) (*edition.ToolResult, error) {
	result, err := c.client.CallTool(ctx, c.endpoint, tool, args)
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(result.Content)
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: string(raw)}}}, err
}

func (*aitableRecoveryWireCaller) Format() string { return "json" }

func (*aitableRecoveryWireCaller) DryRun() bool { return false }

func (*aitableRecoveryWireCaller) Fields() string { return "" }

func (*aitableRecoveryWireCaller) JQ() string { return "" }

func executeAitableRecoveryWire(t *testing.T, args []string, body string) (map[string]any, error) {
	t.Helper()
	requests := make(chan map[string]any, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var wire map[string]any
		if err := json.NewDecoder(r.Body).Decode(&wire); err != nil {
			t.Error(err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		requests <- wire
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"jsonrpc":"2.0","id":3,"result":{"content":%s}}`, body)
	}))
	defer server.Close()
	argv := append([]string{"aitable"}, args...)
	argv = append(argv, "--yes", "--format=json")
	testseam.Swap(t, &os.Args, append([]string{"dws"}, argv...))
	root := NewRootCommand()
	helpers.InitDepsForTest(t, &aitableRecoveryWireCaller{client: transport.NewClient(server.Client()), endpoint: server.URL})
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(argv)
	err := root.Execute()
	if len(requests) > 1 {
		t.Fatalf("write was replayed: %d requests", len(requests))
	}
	select {
	case request := <-requests:
		return request, err
	default:
		return nil, err
	}
}

func TestCrossPlatformCoverageAitableAttachmentWire(t *testing.T) {
	for _, leaf := range []string{"record create", "record update"} {
		t.Run(leaf, func(t *testing.T) {
			records := `[{"recordId":"r","cells":{"attachments":[{"url":"https://alidocs.dingtalk.com/i/nodes/source?x=1&y=2"},{"filename":"来源","type":"adoc","url":"https://alidocs.dingtalk.com/i/nodes/linked-document","isLink":true}],"checked":false}}]`
			wire, err := executeAitableRecoveryWire(t, append(strings.Fields(leaf), "--base-id=b", "--table-id=t", "--records="+records), `{"status":"success","data":{"newRecordIds":["r"],"recordIds":["r"],"attachmentVerification":"verified"}}`)
			if err != nil {
				t.Fatal(err)
			}
			var want any
			_ = json.Unmarshal([]byte(records), &want)
			got := wire["params"].(map[string]any)["arguments"].(map[string]any)["records"]
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("attachment wire changed: %#v", got)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableAttachmentUpstreamFailure(t *testing.T) {
	for _, tc := range []struct {
		args []string
		body string
		code int
	}{
		// Existing upstream MCP tool failures retain exit code 1; local argument failures above use 3.
		{[]string{"record", "update", "--base-id=b", "--table-id=t", `--records=[{"recordId":"r","cells":{"attachment":[{"url":"https://alidocs.dingtalk.com/i/nodes/source"}]}}]`}, `{"status":"error","data":{"recordIds":["r"],"attachmentVerification":"unconfirmed"},"error":{"type":"SYSTEM_ERROR","code":"ATTACHMENT_WRITE_UNCONFIRMED","message":"Online attachment unconfirmed; do not replay","retryable":false,"details":{"recordIds":["r"],"attachmentVerification":"unconfirmed"}}}`, 1},
	} {
		wire, err := executeAitableRecoveryWire(t, tc.args, tc.body)
		if apperrors.ExitCode(err) != tc.code || wire == nil {
			t.Fatalf("upstream failure masked: error=%v code=%d wire=%v", err, apperrors.ExitCode(err), wire)
		}
		if strings.Contains(tc.body, "ATTACHMENT_WRITE_UNCONFIRMED") {
			var receipt *apperrors.Error
			if !errors.As(err, &receipt) || receipt.Details["attachmentVerification"] != "unconfirmed" ||
				!reflect.DeepEqual(receipt.Details["recordIds"], []string{"r"}) || receipt.ExecutionStarted == nil || !*receipt.ExecutionStarted ||
				!receipt.RetryableSet || receipt.Retryable {
				t.Fatalf("attachment reconciliation evidence lost: %#v", err)
			}
		}
	}
}

func TestCrossPlatformCoverageAitableUpsertUnconfirmedAttachmentWire(t *testing.T) {
	body := `{"status":"error","error":{"type":"SYSTEM_ERROR","code":"ATTACHMENT_WRITE_UNCONFIRMED","message":"Query original records without replay","retryable":false,"details":{"attachmentVerification":"unconfirmed","recordIds":["created"],"createdRecordIds":["created"],"updatedRecordIds":["updated"],"clientToken":"original-token"}}}`
	wire, err := executeAitableRecoveryWire(t, []string{"record", "upsert", "--base-id=b", "--table-id=t", `--records=[{"cells":{"F":"new"}},{"recordId":"updated","cells":{"F":"old"}}]`}, body)
	var receipt *apperrors.Error
	if !errors.As(err, &receipt) || wire == nil || apperrors.ExitCode(err) != 1 {
		t.Fatalf("upsert reconciliation error lost: %v", err)
	}
	if !reflect.DeepEqual(receipt.Details["createdRecordIds"], []string{"created"}) ||
		!reflect.DeepEqual(receipt.Details["updatedRecordIds"], []string{"updated"}) ||
		receipt.Details["clientToken"] != "original-token" || receipt.Retryable {
		t.Fatalf("upsert recovery fields lost: %#v", receipt)
	}
}

func TestCrossPlatformCoverageAitableUnknownCreateKeepsUpdateAttachmentState(t *testing.T) {
	for _, code := range []string{"CREATE_RECORDS_OUTCOME_UNKNOWN", "DUPLICATE_CLIENT_TOKEN"} {
		body := fmt.Sprintf(`{"status":"error","error":{"type":"SYSTEM_ERROR","code":%q,"message":"Query creation and attachments without replay","retryable":false,"details":{"attachmentVerification":"unconfirmed","recordIds":["updated"],"fieldIds":["attachment"],"updatedRecordIds":["updated"],"clientToken":"original-token","reconcileTool":"get_record_write_result"}}}`, code)
		wire, err := executeAitableRecoveryWire(t, []string{"record", "upsert", "--base-id=b", "--table-id=t", `--records=[{"cells":{"F":"new"}},{"recordId":"updated","cells":{"F":"old"}}]`}, body)
		var receipt *apperrors.Error
		if !errors.As(err, &receipt) || wire == nil || apperrors.ExitCode(err) != 1 ||
			receipt.Reason != code || receipt.Retryable ||
			receipt.Details["attachmentVerification"] != "unconfirmed" ||
			receipt.Details["clientToken"] != "original-token" ||
			receipt.Details["reconcileTool"] != "get_record_write_result" ||
			!reflect.DeepEqual(receipt.Details["fieldIds"], []string{"attachment"}) ||
			!reflect.DeepEqual(receipt.Details["updatedRecordIds"], []string{"updated"}) {
			t.Fatalf("mixed recovery state lost: code=%s error=%#v", code, err)
		}
	}
}

func TestCrossPlatformCoverageAitableRuntimeRecoveryDropsCredentials(t *testing.T) {
	previous := edition.Get()
	t.Cleanup(func() { edition.Override(previous) })
	edition.Override(&edition.Hooks{})
	for _, target := range []struct{ product, tool string }{
		{"aitable", "create_records"},
		{"aitable", "update_records"},
		{"aitable", "record_upsert"},
		{"aitable-helper", "record_upsert"},
	} {
		for _, isError := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%s/isError=%t", target.product, target.tool, isError), func(t *testing.T) {
				t.Setenv("DWS_CONFIG_DIR", t.TempDir())
				calls := 0
				testseam.Swap(t, &runnerCallTool, func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
					calls++
					body := map[string]any{
						"status": "error", "summary": "private-summary-canary", "logId": "private-log-canary",
						"requestId": "private-request-canary", "technicalDetail": "private-detail-canary", "message": "private-message-canary",
						"error": map[string]any{"code": "ATTACHMENT_WRITE_UNCONFIRMED", "message": "fileToken=private-upload-canary", "retryable": false,
							"details": map[string]any{"attachmentVerification": "unconfirmed", "recordIds": []any{"r"}}},
					}
					block, marshalErr := json.Marshal(body)
					if marshalErr != nil {
						t.Fatal(marshalErr)
					}
					return transport.ToolCallResult{IsError: isError, Content: body,
						Blocks: []transport.ContentBlock{{Type: "text", Text: string(block)}},
					}, nil
				})
				var logs bytes.Buffer
				client := transport.NewClient(nil)
				client.FileLogger = slog.New(slog.NewJSONHandler(&logs, nil))
				r := &runtimeRunner{transport: client, globalFlags: &GlobalFlags{Token: "fixture-only"}, auditSink: aitableRecoveryAuditSink(t, target.product, target.tool)}
				_, err := r.executeInvocation(context.Background(), "https://fixture.test", executor.Invocation{CanonicalProduct: target.product, Tool: target.tool})
				var typed *apperrors.Error
				if calls != 1 || !errors.As(err, &typed) || typed.Reason != "ATTACHMENT_WRITE_UNCONFIRMED" || typed.Retryable || !reflect.DeepEqual(typed.Details["recordIds"], []string{"r"}) {
					t.Fatal("runtime lost the reviewed recovery receipt or repeated the call")
				}
				wire, _ := json.Marshal(errorInfoFromExecutionError(err))
				if bytes.Contains(wire, []byte("private-")) {
					t.Fatal("runtime recovery retained raw upstream credentials")
				}
				if bytes.Contains(logs.Bytes(), []byte("private-")) || !strings.Contains(logs.String(), `"msg":"business_error"`) ||
					!strings.Contains(logs.String(), `"reason":"ATTACHMENT_WRITE_UNCONFIRMED"`) ||
					!strings.Contains(logs.String(), `"product":"`+target.product+`"`) || !strings.Contains(logs.String(), `"tool":"`+target.tool+`"`) {
					t.Fatal("runtime recovery must log only safe product/tool/reason diagnostics")
				}
			})
		}
	}
	for _, tc := range []struct{ product, tool, status, reason string }{
		{"drive", "update_records", "error", "mcp_tool_error"},
		{"aitable", "query_records", "error", "mcp_tool_error"},
		{"aitable", "update_records", "success", ""},
		{"aitable", "update_records", "error", "mcp_tool_error"},
	} {
		t.Run(tc.product+"/"+tc.tool+"/"+tc.status, func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			testseam.Swap(t, &runnerCallTool, func(*transport.Client, context.Context, string, string, map[string]any) (transport.ToolCallResult, error) {
				return transport.ToolCallResult{IsError: tc.status == "error", Content: map[string]any{"status": tc.status,
					"error": map[string]any{"code": "FORBIDDEN", "message": "Rejected", "details": map[string]any{"writeOutcome": "REJECTED", "mutationSubmitted": false}},
				}}, nil
			})
			r := &runtimeRunner{transport: transport.NewClient(nil), globalFlags: &GlobalFlags{Token: "fixture-only"}, auditSink: aitableRecoveryAuditSink(t, tc.product, tc.tool)}
			_, err := r.executeInvocation(context.Background(), "https://fixture.test", executor.Invocation{CanonicalProduct: tc.product, Tool: tc.tool})
			if tc.reason == "" {
				if err != nil {
					t.Fatal("successful record write entered recovery")
				}
				return
			}
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != tc.reason {
				t.Fatal("unrelated product/tool or definite rejection entered record recovery")
			}
		})
	}
}

// Each recovery invocation owns its real audit writer; unrelated Cobra tests
// may already have closed the process-wide sink before this test runs.
func aitableRecoveryAuditSink(t *testing.T, product, tool string) audit.Sink {
	t.Helper()
	dir := t.TempDir()
	writer, err := audit.NewDateRotatingWriter(dir, 0)
	if err != nil {
		t.Fatal(err)
	}
	sink := audit.NewFileSink(writer, audit.NewChain(dir), nil)
	t.Cleanup(func() {
		if err := sink.Close(); err != nil {
			t.Error(err)
		}
		path, err := audit.LatestAuditFile(dir)
		if err != nil {
			t.Error(err)
			return
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Error(err)
			return
		}
		var event audit.Event
		if json.Unmarshal(bytes.TrimSpace(raw), &event) != nil || event.Product != product || event.Command != tool {
			t.Error("recovery invocation must persist exactly one matching audit event")
		}
		if bytes.Contains(raw, []byte("private-")) {
			t.Error("recovery audit retained raw upstream credentials")
		}
		if valid, _, err := audit.VerifyFile(path); err != nil || !valid {
			t.Errorf("recovery audit chain invalid: %v", err)
		}
	})
	return sink
}
