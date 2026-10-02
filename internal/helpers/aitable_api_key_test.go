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

package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/aitableprotocol"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

// runAPIKeyCLI executes prepared Cobra declarations with an injected in-memory MCP caller.
func runAPIKeyCLI(t *testing.T, c *recordQueryE2ECaller, args ...string) (string, error) {
	t.Helper()
	testseam.Protect(t, &deps)
	InitDeps(c)
	old := os.Args
	t.Cleanup(func() { os.Args = old })
	os.Args = append([]string{"dws", "aitable", "api-key"}, args...)
	cmd := newAitableAPIKeyCommand()
	cmd.PersistentFlags().String("format", "json", "")
	cmd.PersistentFlags().Bool("yes", false, "")
	cmd.PersistentFlags().Bool("dry-run", false, "")
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(out)
	cmd.SetArgs(args)
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	ctx, _ := output.WithResultStore(context.Background())
	executed, err := corecmd.ExecuteContextCForTest(cmd, ctx)
	if err == nil {
		_, _, err = output.EmitStoredResult(executed)
	}
	return out.String(), err
}

func TestCrossPlatformCoverageAitableAPIKeyLifecycle(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000001"
	cases := []struct {
		name, action, tool, data string
		extra                    []string
	}{
		{"create", "create", "create_sql_sheet_api_key", `{"keyId":"` + id + `","status":"ACTIVE","createdAt":123,"apiKey":"TEST_SENTINEL_KEY"}`, []string{"--yes"}},
		{"empty list", "list", "list_sql_sheet_api_keys", `{"keys":[]}`, nil},
		{"list with creator", "list", "list_sql_sheet_api_keys", `{"keys":[{"keyId":"` + id + `","status":"ACTIVE","createdAt":123,"createdBy":{"userId":"u","corpId":"c"}}]}`, nil},
		{"list without creator", "list", "list_sql_sheet_api_keys", `{"keys":[{"keyId":"` + id + `","status":"ACTIVE","createdAt":123}]}`, nil},
		{"revoke", "revoke", "revoke_sql_sheet_api_key", `{"keyId":"` + id + `","revoked":true}`, []string{"--key-id", id, "--yes"}},
	}
	for _, errorField := range []string{"", `,"error":null`, `,"error":{}`} {
		for _, tt := range cases {
			t.Run(tt.name+errorField, func(t *testing.T) {
				c := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{result: textToolResult(`{"status":"success","data":` + tt.data + errorField + `}`)}}}
				args := append([]string{tt.action, "--base-id", " base1 "}, tt.extra...)
				out, err := runAPIKeyCLI(t, c, args...)
				if err != nil {
					t.Fatal(err)
				}
				wantArgs := map[string]any{"baseId": "base1"}
				if tt.action == "revoke" {
					wantArgs["keyId"] = id
				}
				if len(c.calls) != 1 || c.calls[0].server != "aitable" || c.calls[0].tool != tt.tool || !reflect.DeepEqual(c.calls[0].args, wantArgs) {
					t.Fatalf("incorrect call: %#v", c.calls)
				}
				var env map[string]any
				if json.Unmarshal([]byte(out), &env) != nil || env["ok"] != true {
					t.Fatalf("invalid envelope: %s", out)
				}
				var wantData map[string]any
				if err := json.Unmarshal([]byte(tt.data), &wantData); err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(env["data"], wantData) {
					t.Fatalf("data = %#v, want %#v", env["data"], wantData)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageAitableAPIKeyFailureDoesNotLeakOrReplay(t *testing.T) {
	bodies := []string{
		`{"status":"error","data":{"apiKey":"TEST_SENTINEL_KEY"},"error":{"code":"403","message":"TEST_SENTINEL_KEY"}}`,
		`{"status":"success","data":{"apiKey":"TEST_SENTINEL_KEY"}}`,
		`{"status":"success","data":{"keys":[{"apiKey":"TEST_SENTINEL_KEY"}]}}`,
		`TEST_SENTINEL_KEY`,
		`{"status":"success","error":{"code":"OTHER","message":"TEST_SENTINEL_KEY"},"data":{}}`,
	}
	for _, action := range []string{"create", "list", "revoke"} {
		for index, body := range bodies {
			t.Run(fmt.Sprintf("%s/case_%d", action, index), func(t *testing.T) {
				c := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{result: textToolResult(body)}}}
				args := []string{action, "--base-id", "base1", "--yes"}
				if action == "revoke" {
					args = append(args, "--key-id", "ed3c7fb8-03bb-4501-bb0a-6d14bc546a54")
				}
				out, err := runAPIKeyCLI(t, c, args...)
				if err == nil || strings.Contains(err.Error()+out, "TEST_SENTINEL_KEY") || len(c.calls) != 1 {
					t.Fatalf("unsafe failure: %v output=%s calls=%d", err, out, len(c.calls))
				}
			})
		}
	}
	c := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{err: errors.New("TEST_SENTINEL_KEY")}}}
	out, err := runAPIKeyCLI(t, c, "create", "--base-id", "base1", "--yes")
	if err == nil || strings.Contains(err.Error()+out, "TEST_SENTINEL_KEY") || len(c.calls) != 1 {
		t.Fatal("transport failure leaked or replayed")
	}
}

func TestCrossPlatformCoverageAitableAPIKeyLocalGuards(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000001"
	for _, tt := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{"missing base", []string{"create", "--yes"}, "base-id"},
		{"blank base", []string{"list", "--base-id", "  "}, "base-id"},
		{"create confirmation", []string{"create", "--base-id", "base1"}, "--yes"},
		{"revoke confirmation", []string{"revoke", "--base-id", "base1", "--key-id", id}, "--yes"},
		{"missing key", []string{"revoke", "--base-id", "base1", "--yes"}, "key-id"},
		{"invalid key", []string{"revoke", "--base-id", "base1", "--key-id", "NOT-UUID", "--yes"}, "小写规范 UUID"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := &recordQueryE2ECaller{}
			_, err := runAPIKeyCLI(t, c, tt.args...)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) || len(c.calls) != 0 {
				t.Fatalf("guard: error=%v want=%s calls=%d", err, tt.wantErr, len(c.calls))
			}
		})
	}
	t.Run("invalid flag classification", func(t *testing.T) {
		c := &recordQueryE2ECaller{}
		_, err := runAPIKeyCLI(t, c, "create", "--base-id", "base1", "--yes=not-bool")
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || typed.Reason != "invalid_flag" {
			t.Fatalf("expected validation/invalid_flag, got %v", err)
		}
		if code := apperrors.ExitCode(err); code != apperrors.ExitCodeValidation || len(c.calls) != 0 {
			t.Fatalf("invalid flag: exit code=%d calls=%d", code, len(c.calls))
		}
	})
	c := &recordQueryE2ECaller{dryRun: true}
	out, err := runAPIKeyCLI(t, c, "create", "--base-id", "base1", "--dry-run")
	if err != nil || len(c.calls) != 0 || !strings.Contains(out, `"executed": false`) {
		t.Fatalf("dry run: %v %s", err, out)
	}
	for _, cmd := range []*cobra.Command{newAitableAPIKeyCreateCommand(), newAitableAPIKeyListCommand(), newAitableAPIKeyRevokeCommand()} {
		f, ok := contractfinal.RuntimeContractFinal(cmd)
		if !ok || f.Result == nil || f.DryRun == nil {
			t.Fatal("missing contract")
		}
	}
}

func TestCrossPlatformCoverageAitableAPIKeySafeCallerErrors(t *testing.T) {
	for _, reason := range []string{"403", "404", "600", "INVALID_PARAM", "RATE_LIMIT_EXCEEDED", "INTERNAL_ERROR", "INVALID_SQL_SHEET_API_KEY_RESPONSE", "SQL_SHEET_API_KEY_ALREADY_EXISTS", "INVALID_SQL_SHEET_API_KEY_REQUEST", "sql_sheet_api_key_single_profile_required", "not_authenticated"} {
		t.Run(reason, func(t *testing.T) {
			remoteErr := apperrors.NewAPI("TEST_SENTINEL_KEY", apperrors.WithReason(reason))
			c := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{err: remoteErr}}}
			out, err := runAPIKeyCLI(t, c, "list", "--base-id", "base1")
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != reason || strings.Contains(out+err.Error(), "TEST_SENTINEL_KEY") {
				t.Fatalf("known failure was lost or leaked: %v", err)
			}
		})
	}
	localErr := apperrors.NewValidation("TEST_SENTINEL_KEY", apperrors.WithOrigin("client"), apperrors.WithExecutionStarted(false))
	c := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{err: localErr}}}
	_, err := runAPIKeyCLI(t, c, "list", "--base-id", "base1")
	var typed *apperrors.Error
	if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || typed.ExecutionStarted == nil || *typed.ExecutionStarted || typed.FailureStage != "invocation_setup" || strings.Contains(err.Error(), "TEST_SENTINEL_KEY") {
		t.Fatalf("local setup state was lost: %v", err)
	}
	unknown := aitableprotocol.SQLSheetKeyCallError("create_sql_sheet_api_key", errors.New("TEST_SENTINEL_KEY"))
	if !errors.As(unknown, &typed) || typed.ExecutionStarted != nil || typed.Retryable {
		t.Fatalf("unknown call outcome was classified as not executed: %v", unknown)
	}
}

func TestCrossPlatformCoverageAitableAPIKeyAmbiguousResponse(t *testing.T) {
	const metadata = `"keyId":"ed3c7fb8-03bb-4501-bb0a-6d14bc546a54","status":"ACTIVE","createdAt":123`
	bodies := []string{
		`{"status":"error","status":"success","data":{"keys":[]}}`,
		`{"Status":"success","data":{"keys":[]}}`,
		`{"status":"success","data":{"keys":[],"keys":[]}}`,
		`{"status":"success","data":{"Keys":[]}}`,
		`{"status":"success","data":{"keys":[{` + metadata + `,"KeyId":"ed3c7fb8-03bb-4501-bb0a-6d14bc546a54"}]}}`,
		`{"status":"success","data":{"keys":[{` + metadata + `,"createdBy":{"UserId":"u","corpId":"c"}}]}}`,
		`{"status":"success","data":{"keys":[{` + metadata + `,"createdBy":{"uid":"internal","corpId":"c"}}]}}`,
	}
	for _, body := range bodies {
		c := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{result: textToolResult(body)}}}
		out, err := runAPIKeyCLI(t, c, "list", "--base-id", "base1")
		if err == nil || strings.Contains(out, "internal") || len(c.calls) != 1 {
			t.Fatalf("ambiguous response accepted: %v", err)
		}
	}
}

func TestCrossPlatformCoverageAitableAPIKeyResponseBoundaries(t *testing.T) {
	for _, errorField := range []string{"", `,"error":{}`} {
		for _, data := range []string{
			`{"keys":[{"keyId":"bad","status":"ACTIVE","createdAt":1}]}`,
			`{"keys":[{"keyId":"00000000-0000-4000-8000-000000000001","status":"ACTIVE","createdAt":1,"createdBy":{"userId":"","corpId":"c"}}]}`,
			`{"keys":null}`,
			`{}`,
			`{"keys":{}}`,
			`{"keys":[{"keyId":"00000000-0000-4000-8000-000000000001","status":"ACTIVE","createdAt":1},{"keyId":"00000000-0000-4000-8000-000000000002","status":"ACTIVE","createdAt":2}]}`,
		} {
			caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{result: textToolResult(`{"status":"success","data":` + data + errorField + `}`)}}}
			if _, err := runAPIKeyCLI(t, caller, "list", "--base-id", "base1"); err == nil || len(caller.calls) != 1 {
				t.Fatalf("invalid metadata accepted: %v", err)
			}
		}
	}
	for _, result := range []*edition.ToolResult{nil, {}, {Content: []edition.ContentBlock{{Type: "image"}}}, {Content: []edition.ContentBlock{{Type: "text", Text: `{"status":"success"}`}, {Type: "text", Text: `{}`}}}} {
		caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{result: result}}}
		if _, err := runAPIKeyCLI(t, caller, "list", "--base-id", "base1"); err == nil || len(caller.calls) != 1 {
			t.Fatalf("invalid MCP content accepted: %v", err)
		}
	}
	testseam.Protect(t, &deps)
	deps = nil
	if _, err := callAitableAPIKeyResult(&cobra.Command{}, "list_sql_sheet_api_keys", nil); err == nil {
		t.Fatal("missing caller accepted")
	}
	InitDeps(&recordQueryE2ECaller{steps: []recordQueryE2EStep{{result: textToolResult(`{"status":"success","data":{}}`)}}})
	if _, err := callAitableAPIKeyResult(&cobra.Command{}, "unknown", nil); err == nil {
		t.Fatal("unknown credential operation accepted")
	}
}

func TestCrossPlatformCoverageAitableAPIKeyRejectsNonEmptyError(t *testing.T) {
	for _, detail := range []string{`{"code":""}`, `{"message":""}`, `{"retryable":false}`, `{"unknown":null}`, `[]`, `""`, `false`, `0`} {
		t.Run(detail, func(t *testing.T) {
			caller := &recordQueryE2ECaller{steps: []recordQueryE2EStep{{result: textToolResult(`{"status":"success","data":{"keys":[]},"error":` + detail + `}`)}}}
			_, err := runAPIKeyCLI(t, caller, "list", "--base-id", "base1")
			if err == nil || len(caller.calls) != 1 {
				t.Fatalf("malformed error accepted or replayed: %v", err)
			}
		})
	}
}
