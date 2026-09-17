package helpers

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageOAApprovalListResponseEnvelope(t *testing.T) {
	for _, command := range []struct{ name, tool string }{
		{"list-pending", "get_todo_tasks"},
		{"list-executed", "get_done_tasks"},
		{"list-submitted", "get_submitted_instances"},
		{"list-cc", "get_noticed_instances"},
	} {
		t.Run(command.name, func(t *testing.T) {

			for _, format := range []string{"json", "raw"} {
				for _, tc := range []struct {
					name, response string
					wantError      bool
				}{
					{"string success", `{"success":"true","error_code":"0","result":{"id":9007199254740993,"success":"false","error_code":"007"}}`, false},
					{"no error code", `{"success":"true","result":{"id":9007199254740993,"success":"false","error_code":"007"}}`, false},
					{"typed success", `{"success":true,"errorCode":0,"result":{"id":9007199254740993,"success":"false","error_code":"007"}}`, false},
					{"string failure", `{"success":"false","error_code":"0"}`, true},
					{"typed failure", `{"success":false,"errorCode":0}`, true},
					{"nonzero code", `{"success":"true","error_code":"123"}`, true},
					{"invalid success", `{"success":"unknown","error_code":"0"}`, true},
					{"symbolic failure", `{"success":"false","error_code":"INVALID_ARGUMENT","error_message":"参数错误"}`, true},
				} {
					t.Run(format+"/"+tc.name, func(t *testing.T) {
						caller := &scriptedToolCaller{format: format, steps: []scriptedToolStep{{text: tc.response}}}
						installScriptedCaller(t, caller)
						testseam.Swap(t, &os.Args, []string{"dws", "oa"})
						var out bytes.Buffer
						deps.Out.w = &out
						cmd := newOaCommand()
						cmd.SilenceErrors, cmd.SilenceUsage = true, true
						cmd.SetArgs([]string{"approval", command.name, "--page", "1", "--limit", "20"})
						err := cmd.Execute()
						if caller.server != "oa" || caller.tool != command.tool || caller.calls != 1 {
							t.Fatalf("want one oa/%s call, got %s/%s calls=%d", command.tool, caller.server, caller.tool, caller.calls)
						}
						if tc.wantError {
							if err == nil || out.Len() != 0 {
								t.Fatalf("want error without success output, got err=%v output=%s", err, &out)
							}
							if tc.name == "symbolic failure" {
								var cliErr *CLIError
								if !errors.As(err, &cliErr) || cliErr.Message != tc.response {
									t.Fatalf("symbolic diagnostic was masked: %v", err)
								}
							}
							return
						}
						if err != nil {
							t.Fatal(err)
						}
						var body map[string]json.RawMessage
						if err := json.Unmarshal(out.Bytes(), &body); err != nil {
							t.Fatal(err)
						}
						if tc.name == "no error code" {
							if body["errorCode"] != nil {
								t.Fatalf("invented errorCode: %s", &out)
							}
						} else if string(body["errorCode"]) != "0" {
							t.Fatalf("errorCode is not numeric zero: %s", &out)
						}
						if string(body["success"]) != "true" || body["error_code"] != nil {
							t.Fatalf("unexpected envelope: %s", &out)
						}
						var result map[string]json.RawMessage
						if err := json.Unmarshal(body["result"], &result); err != nil {
							t.Fatal(err)
						}
						if string(result["id"]) != "9007199254740993" || string(result["success"]) != `"false"` || string(result["error_code"]) != `"007"` {
							t.Fatalf("business data changed: %s", body["result"])
						}
					})
				}
			}

		})
	}
}

func TestCrossPlatformCoverageOAApprovalListFailureEnvelopeTypes(t *testing.T) {
	for _, command := range []struct{ name, tool string }{
		{"list-pending", "get_todo_tasks"},
		{"list-executed", "get_done_tasks"},
		{"list-submitted", "get_submitted_instances"},
		{"list-cc", "get_noticed_instances"},
	} {
		t.Run(command.name, func(t *testing.T) {

			for _, format := range []string{"json", "raw"} {
				t.Run(format, func(t *testing.T) {
					caller := &scriptedToolCaller{format: format, steps: []scriptedToolStep{{text: `{"error_code":"400002","error_message":"参数错误","result":{"values":null},"success":"false","trace_id":"2104940317890289991958640e05b4"}`}}}
					installScriptedCaller(t, caller)
					testseam.Swap(t, &os.Args, []string{"dws", "oa"})
					var out bytes.Buffer
					deps.Out.w = &out
					cmd := newOaCommand()
					cmd.SilenceErrors, cmd.SilenceUsage = true, true
					cmd.SetArgs([]string{"approval", command.name, "--page", "1", "--limit", "100"})
					err := cmd.Execute()
					if caller.server != "oa" || caller.tool != command.tool || caller.calls != 1 {
						t.Fatalf("want one oa/%s call, got %s/%s calls=%d", command.tool, caller.server, caller.tool, caller.calls)
					}
					var cliErr *CLIError
					if !errors.As(err, &cliErr) || cliErr.Code != CodeMCPToolError {
						t.Fatalf("want MCP error, got %v", err)
					}
					if out.Len() != 0 {
						t.Fatalf("failure emitted success output: %s", &out)
					}
					var body map[string]any
					if err := json.Unmarshal([]byte(cliErr.Message), &body); err != nil {
						t.Fatalf("invalid error payload: %s", cliErr.Message)
					}
					if body["success"] != false || body["errorCode"] != float64(400002) || body["error_code"] != nil {
						t.Fatalf("unexpected error field types: %s", cliErr.Message)
					}
					if body["error_message"] != "参数错误" || body["trace_id"] != "2104940317890289991958640e05b4" {
						t.Fatalf("lost diagnostics: %s", cliErr.Message)
					}
					result, ok := body["result"].(map[string]any)
					if !ok {
						t.Fatalf("lost result: %s", cliErr.Message)
					}
					if v, exists := result["values"]; !exists || v != nil {
						t.Fatalf("changed values: %v", result)
					}
				})
			}

		})
	}
}

func TestCrossPlatformCoverageOAApprovalListInvalidResponse(t *testing.T) {
	for _, raw := range []string{`null`, `[]`, `{`, `{"success":1e1000}`} {
		t.Run(raw, func(t *testing.T) {
			installScriptedCaller(t, &scriptedToolCaller{format: "json"})
			var out bytes.Buffer
			deps.Out.w = &out
			if err := renderOAApprovalListResponse(raw); err == nil {
				t.Fatal("invalid response accepted")
			}
			if out.Len() != 0 {
				t.Fatalf("invalid response leaked output: %s", &out)
			}
		})
	}
}

func TestCrossPlatformCoverageOAApprovalListUnrelatedResponses(t *testing.T) {
	const response = `{"success":"true","error_code":"0"}`
	for _, tc := range []struct{ server, tool string }{
		{"other", "get_todo_tasks"},
		{"oa", "list_user_visible_process"},
	} {
		t.Run(tc.server+"/"+tc.tool, func(t *testing.T) {
			caller := &scriptedToolCaller{format: "json", steps: []scriptedToolStep{{text: response}}}
			installScriptedCaller(t, caller)
			var out bytes.Buffer
			deps.Out.w = &out
			if err := callMCPToolOnServer(tc.server, tc.tool, nil); err != nil {
				t.Fatal(err)
			}
			var body map[string]any
			if err := json.Unmarshal(out.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body["success"] != "true" || body["error_code"] != "0" || body["errorCode"] != nil {
				t.Fatalf("unrelated response was normalized: %s", &out)
			}
		})
	}
}
