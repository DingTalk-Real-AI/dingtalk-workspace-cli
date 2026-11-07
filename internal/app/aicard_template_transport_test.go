// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

type aicardTemplateRoundTripper func(*http.Request) (*http.Response, error)

func (f aicardTemplateRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type aicardTemplateSnapshotSpy struct{ calls int }

func (s *aicardTemplateSnapshotSpy) RecordJSONRPC(_, _ string, _, _ []byte) string {
	s.calls++
	return "snapshot-must-not-be-written"
}

func TestCrossPlatformCoverageAicardTemplateTransportProtectsBodies(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(fmt.Sprintf("remote-error-%t", failed), func(t *testing.T) {
			const marker = "private-template-body-canary"
			const file = `[{"dataModel":{"business":"` + marker + `","count":9007199254740993}}]`
			var logs bytes.Buffer
			logger := slog.New(slog.NewJSONHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
			snapshots := &aicardTemplateSnapshotSpy{}
			calls := 0
			base := transport.NewClient(&http.Client{Transport: aicardTemplateRoundTripper(func(request *http.Request) (*http.Response, error) {
				calls++
				if request.Header.Get("Authorization") != "Bearer template-test-token" {
					t.Fatal("template transport lost authentication")
				}
				var rpc struct {
					ID     any            `json:"id"`
					Method string         `json:"method"`
					Params map[string]any `json:"params"`
				}
				if err := json.NewDecoder(request.Body).Decode(&rpc); err != nil {
					t.Fatal(err)
				}
				response := map[string]any{"jsonrpc": "2.0", "id": rpc.ID}
				switch rpc.Method {
				case "tools/list":
					response["result"] = map[string]any{"tools": []any{map[string]any{
						"name": "create_aicard_template", "inputSchema": map[string]any{
							"type": "object", "required": []string{"file"}, "properties": map[string]any{"file": map[string]any{"type": "string"}},
						},
					}}}
				case "tools/call":
					arguments, _ := rpc.Params["arguments"].(map[string]any)
					if arguments["file"] != file {
						t.Fatal("diagnostic protection changed the actual request")
					}
					if failed {
						response["error"] = map[string]any{"code": -32603, "message": marker}
					} else {
						response["result"] = map[string]any{"structuredContent": map[string]any{"success": true, "templateId": "template-test", "version": "1", "file": file}}
					}
				default:
					t.Fatalf("unexpected method %s", rpc.Method)
				}
				body, err := json.Marshal(response)
				if err != nil {
					t.Fatal(err)
				}
				return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(bytes.NewReader(body))}, nil
			})})
			base.TrustedDomains = []string{"example.test"}
			base.FileLogger = logger
			base.SnapshotRecorder = snapshots
			factory := newAicardTemplateTransportFactory(&runtimeRunner{transport: base}, &GlobalFlags{Token: "template-test-token"})
			client, err := factory(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			_, err = client.InvokeValidated(t.Context(), "https://example.test/template", "create_aicard_template", map[string]any{"file": file})
			if (err != nil) != failed || calls != 2 {
				t.Fatalf("calls=%d err=%v", calls, err)
			}
			if strings.Contains(logs.String(), marker) || snapshots.calls != 0 {
				t.Fatalf("template body reached diagnostics: logLeak=%t snapshots=%d", strings.Contains(logs.String(), marker), snapshots.calls)
			}
			if !strings.Contains(logs.String(), "jsonrpc_request") {
				t.Fatal("safe request metadata was lost")
			}
			if base.FileLogger != logger || base.SnapshotRecorder != snapshots {
				t.Fatal("template invocation mutated the shared transport")
			}
		})
	}
}
