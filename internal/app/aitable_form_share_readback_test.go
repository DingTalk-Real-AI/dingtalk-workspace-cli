// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

type formShareReadbackCaller struct {
	response string
	tool     string
	calls    int
	upload   bool
	failure  error
}

func (c *formShareReadbackCaller) CallTool(_ context.Context, _ string, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls++
	if c.upload && tool == "get_doc_attachment_upload_info" && args["nodeId"] == "b" {
		return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: `{"result":{"uploadUrl":"https://upload.example.test/cover?signature=private-upload","resourceId":"rid-cover","resourceUrl":"/core/api/resources/rid-cover"}}`}}}, nil
	}
	if tool != c.tool || args["baseId"] != "b" || args["tableId"] != "t" || args["viewId"] != "v" {
		return nil, fmt.Errorf("unexpected form request: %s %#v", tool, args)
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.response}}}, c.failure
}

func (*formShareReadbackCaller) Format() string { return "json" }
func (*formShareReadbackCaller) DryRun() bool   { return false }
func (*formShareReadbackCaller) Fields() string { return "" }
func (*formShareReadbackCaller) JQ() string     { return "" }

type formShareUploadTransport func(*http.Request) (*http.Response, error)

func (f formShareUploadTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

// Exercise the actual MCP-error path after upload, including the root's error projection.
func TestCrossPlatformCoverageAITableShareFormCoverRecoveryErrors(t *testing.T) {
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "cover.png")
	if err := os.WriteFile(file, imageBytes.Bytes(), 0600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"aitable form share update", "aitable +form-share-update"} {
		for _, typed := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/typed=%t", path, typed), func(t *testing.T) {
				caller := &formShareReadbackCaller{tool: "update_share_form", upload: true,
					response: `{"status":"error","error":{"type":"SYSTEM_ERROR","code":"SHARE_FORM_READBACK_FAILED","message":"Final state not confirmed","retryable":true,"details":{"serverUpdated":true,"cpSynced":false}}}`}
				if typed {
					caller.failure = apperrors.NewAPI("Final state not confirmed", apperrors.WithReason("form_readback_failed"),
						apperrors.WithDetails(map[string]any{"serverUpdated": true, "cpSynced": false}),
						apperrors.WithExecutionStarted(true), apperrors.WithRetryable(true),
						apperrors.WithRetryAfterSeconds(30), apperrors.WithNextRetryAt(time.Date(2026, 10, 15, 1, 0, 0, 0, time.UTC)))
				}
				uploads := 0
				var transport http.RoundTripper = formShareUploadTransport(func(r *http.Request) (*http.Response, error) {
					uploads++
					body, _ := io.ReadAll(r.Body)
					if r.Method != "PUT" || !bytes.Equal(body, imageBytes.Bytes()) {
						t.Fatal("unexpected cover upload")
					}
					return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(""))}, nil
				})
				testseam.Swap(t, &http.DefaultTransport, transport)
				args := append(strings.Fields(path), "--base-id=b", "--table-id=t", "--view-id=v", "--form-cover-file="+file, "--yes", "--format=json")
				testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
				root := NewRootCommand()
				helpers.InitDepsForTest(t, caller)
				var stdout bytes.Buffer
				root.SetOut(&stdout)
				root.SetErr(&stdout)
				root.SetArgs(args)
				cmd, err := root.ExecuteC()
				if err == nil || uploads != 1 || caller.calls != 2 {
					t.Fatalf("expected one upload and one failed update: err=%v uploads=%d calls=%d", err, uploads, caller.calls)
				}
				info := errorInfoFromExecutionError(err)
				if info.Type != "api" || info.Details["formCoverResourceId"] != "rid-cover" || !strings.Contains(info.Hint, "先回读") {
					t.Fatalf("lost recovery evidence: %#v", info)
				}
				if typed && (info.Subtype != "form_readback_failed" || info.ExecutionStarted == nil || !*info.ExecutionStarted || info.Details["serverUpdated"] != true) {
					t.Fatalf("changed original failure classification: %#v", info)
				}
				if info.Retryable || info.RetryAfterSeconds != nil || info.NextRetryAt != "" {
					t.Fatalf("uploaded cover must not permit replaying the whole command: %#v", info)
				}
				if typed {
					original := caller.failure.(*apperrors.Error)
					if !errors.Is(err, original) || original.Details["formCoverResourceId"] != nil || !original.Retryable || original.RetryAfterSeconds == nil || original.NextRetryAt == nil {
						t.Fatal("recovery annotation changed the original error or lost its identity")
					}
				}
				code, emitErr := output.EmitResult(cmd, output.FailureWithExitCode(info, apperrors.ExitCode(err)))
				if emitErr != nil || code != 1 || strings.Contains(stdout.String(), "private-upload") {
					t.Fatalf("unsafe failure emission: code=%d err=%v stdout=%s", code, emitErr, stdout.String())
				}
			})
		}
	}
}

// Run both atomic and shortcut leaves through the real root output boundary.
// Only the remote transport is replaced; every scenario must make exactly one call.
func TestCrossPlatformCoverageAITableShareFormReadbackDelivery(t *testing.T) {
	for _, path := range []string{
		"aitable form share get", "aitable +form-share-get",
		"aitable form share update", "aitable +form-share-update",
	} {
		write := strings.Contains(path, "update")
		var schemas []*jsonschema.Schema
		for _, compact := range []bool{false, true} {
			query := []string{"--cli-path", path}
			if compact {
				query = append(query, "--compact")
			}
			leaf := executeShortcutSchemaQuery(t, query...)
			decl := aitableShareSchemaObject(t, leaf["result"], "result")
			compiler := jsonschema.NewCompiler()
			const resource = "https://dws.test/form-readback.json"
			if err := compiler.AddResource(resource, decl["data_schema"]); err != nil {
				t.Fatal(err)
			}
			schema, err := compiler.Compile(resource)
			if err != nil {
				t.Fatal(err)
			}
			schemas = append(schemas, schema)
		}
		for _, scenario := range []string{"matching", "unavailable", "mismatch", "cp-partial", "contradictory-status", "cleared-auth", "trusted-cover"} {
			if !write && (scenario == "mismatch" || scenario == "cp-partial" || scenario == "contradictory-status") {
				continue
			}
			t.Run(path+"/"+scenario, func(t *testing.T) {
				data := map[string]any{
					"baseId": "b", "tableId": "t", "viewId": "v", "enabled": true, "status": 1,
					"shareFormUuid": "share", "shareFormUrl": "https://alidocs.dingtalk.com/notable/share/form/share",
					"formCover": "https://example.test/cover.png", "formName": "报名表", "formDesc": "说明",
					"authTypeCode": 2, "authData": `{"members":[{"type":"user","userId":"user-b","corpId":"corp"},{"type":"user","userId":"user-a","corpId":"corp"}]}`,
					"submitTimesLimit": 10000, "submitTimesUserLimit": 0,
					"formStartTime": int64(1790038800000), "formEndTime": 0,
					"anonymousSubmit": true, "loadLastSubmit": true, "submitRecordViewMode": 1,
					"replyNotice": true, "shareUidList": "user-b,user-a",
				}
				tool := "get_share_form_config"
				authType, authData := "2", "user-a,user-b"
				if scenario == "cleared-auth" {
					data["authTypeCode"], data["authData"] = 0, ""
					authType, authData = "0", ""
				}
				args := append(strings.Fields(path), "--base-id=b", "--table-id=t", "--view-id=v", "--format=json")
				if write {
					tool = "update_share_form"
					data["cpSynced"] = true
					args = append(args, "--yes", "--auth-type-code="+authType, "--auth-data="+authData, "--submit-record-view-mode=1", "--anonymous-submit=true", "--reply-notice=true", "--share-uid-list=user-a,user-b", "--form-start-at=2026-09-22T09:00:00+08:00", "--submit-times-limit=1000000")
					if scenario == "trusted-cover" {
						data["formCover"] = "https://img.alicdn.com/cover%28a%29.png"
						args = append(args, "--form-cover="+data["formCover"].(string))
					}
				} else {
					data["punished"] = false
				}
				response := map[string]any{"status": "success", "data": data}
				if scenario == "unavailable" || scenario == "cp-partial" {
					data["authData"], data["shareUidList"] = nil, nil
					response["meta"] = map[string]any{"suggestions": []any{map[string]any{
						"action": "Inspect settings; do not clear members", "reason": "Historical identity unavailable", "priority": "medium", "private": "must-not-leak",
					}}, "private": "must-not-leak"}
				}
				if scenario == "mismatch" {
					data["submitRecordViewMode"] = 0
				}
				if scenario == "cp-partial" {
					data["cpSynced"] = false
				}
				if scenario == "contradictory-status" {
					data["status"] = 2
				}
				raw, _ := json.Marshal(response)
				caller := &formShareReadbackCaller{response: string(raw), tool: tool}
				testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
				root := NewRootCommand()
				helpers.InitDepsForTest(t, caller)
				var stdout, stderr bytes.Buffer
				root.SetOut(&stdout)
				root.SetErr(&stderr)
				root.SetArgs(args)
				if err := root.Execute(); err != nil {
					t.Fatalf("execute: %v; stderr=%s", err, stderr.String())
				}
				if caller.calls != 1 || stderr.Len() != 0 || strings.Contains(stdout.String(), "must-not-leak") {
					t.Fatalf("unexpected calls/diagnostics: calls=%d stdout=%s stderr=%s", caller.calls, stdout.String(), stderr.String())
				}
				var envelope map[string]any
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				for _, schema := range schemas {
					if err := schema.Validate(envelope["data"]); err != nil {
						t.Fatalf("real command result violates delivered schema: %v; %s", err, stdout.String())
					}
				}
				state := envelope["data"].(map[string]any)
				wantWarnings := []any{map[string]any{"action": "Inspect settings; do not clear members", "reason": "Historical identity unavailable", "priority": "medium"}}
				if scenario == "mismatch" || scenario == "cp-partial" || scenario == "contradictory-status" {
					if envelope["outcome"] != "partial_failure" || envelope["ok"] != false {
						t.Fatalf("invalid terminal state accepted: %s", stdout.String())
					}
					details := state["failed"].([]any)[0].(map[string]any)["error"].(map[string]any)["details"].(map[string]any)
					field := "submitRecordViewMode"
					if scenario == "cp-partial" {
						field = "cpSynced"
						if !reflect.DeepEqual(details["warnings"], wantWarnings) {
							t.Fatalf("partial result lost identity warning: %s", stdout.String())
						}
					}
					wantInvalid := []any{field}
					if scenario == "contradictory-status" {
						wantInvalid = []any{"enabled", "status"}
					}
					if !reflect.DeepEqual(details["invalid_fields"], wantInvalid) {
						t.Fatalf("wrong terminal diagnosis: %#v", details)
					}
					return
				}
				if envelope["outcome"] != "success" || envelope["ok"] != true {
					t.Fatalf("valid state rejected: %s", stdout.String())
				}
				if write && state["verified"] != (scenario != "unavailable") {
					t.Fatalf("incorrect verification: %s", stdout.String())
				}
				if scenario == "unavailable" {
					if state["authData"] != nil || state["shareUidList"] != nil || !reflect.DeepEqual(state["warnings"], wantWarnings) {
						t.Fatalf("identity warning or null state lost: %s", stdout.String())
					}
					if write && !reflect.DeepEqual(state["unverified"], []any{"authData", "shareUidList"}) {
						t.Fatalf("unavailable identities were verified: %s", stdout.String())
					}
				} else if state["warnings"] != nil {
					t.Fatalf("manufactured warning: %s", stdout.String())
				}
				for _, schema := range schemas {
					state["submitRecordViewMode"] = 2
					if schema.Validate(state) == nil {
						t.Fatal("published schema accepts an invalid view mode")
					}
					state["submitRecordViewMode"] = float64(1)
					state["replyNotice"] = "true"
					if schema.Validate(state) == nil {
						t.Fatal("published schema accepts a string notification flag")
					}
					state["replyNotice"] = true
				}
			})
		}
	}
}
