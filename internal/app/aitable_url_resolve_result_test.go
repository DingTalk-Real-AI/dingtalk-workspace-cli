// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/pipeline"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
	"github.com/spf13/cobra"
)

type aitableURLResolveCaller struct {
	aitableOutputCompatibilityCaller
	body          string
	err           error
	calls         int
	tool, product string
	args          map[string]any
}

func (c *aitableURLResolveCaller) CallTool(_ context.Context, product, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls++
	wantProduct, wantTool, wantArgs := c.product, c.tool, c.args
	if wantTool == "" {
		wantProduct, wantTool, wantArgs = "aitable", "get_fields", map[string]any{"baseId": "b", "tableId": "s", "fieldIds": []string{"f"}}
	}
	if product != wantProduct || tool != wantTool || !reflect.DeepEqual(args, wantArgs) {
		return nil, fmt.Errorf("unexpected URL verification: %s/%s %#v", product, tool, args)
	}
	if c.err != nil {
		return nil, c.err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: c.body}}}, nil
}

// Keep the real CLI tree, shortcut, process error projection and renderer.
// Replace only the remote caller and capture the final process streams.
func TestCrossPlatformCoverageAitableURLResolveResultAndExitCode(t *testing.T) {
	for _, tc := range []struct {
		name, url, body, errorType, reason string
		err                                error
		verify                             bool
		code, calls                        int
		product, tool, targetKey, targetID string
		params                             map[string]any
	}{
		{name: "local field", url: "https://alidocs.dingtalk.com/i/nodes/b?sheetId=s&fieldId=f"},
		{name: "verified field", url: "https://alidocs.dingtalk.com/i/nodes/b?sheetId=s&fieldId=f", verify: true, body: `{"fields":[{"fieldId":"f","tableId":"s"}]}`, calls: 1},
		{name: "invalid URL", url: "https://example.com/i/nodes/b", code: 3, errorType: "validation", reason: "invalid_aitable_url"},
		{name: "retired locator", url: "https://alidocs.dingtalk.com/i/nodes/b?targetSheetId=s&targetFieldId=f", verify: true, code: 3, errorType: "validation", reason: "invalid_aitable_url"},
		{name: "retired nested locator", url: "https://alidocs.dingtalk.com/i/nodes/b?sheetId=s&iframeQuery=targetViewId%3Dv", verify: true, code: 3, errorType: "validation", reason: "invalid_aitable_url"},
		{name: "missing parent", url: "https://alidocs.dingtalk.com/i/nodes/b?fieldId=f", verify: true, code: 3, errorType: "validation", reason: "invalid_aitable_url"},
		{name: "App has no read-only verification", url: "https://alidocs.dingtalk.com/i/nodes/b?appId=a", verify: true, code: 3, errorType: "validation", reason: "target_verification_unsupported"},
		{name: "unproven object", url: "https://alidocs.dingtalk.com/i/nodes/b?sheetId=s&fieldId=f", verify: true, body: `{"fields":[]}`, calls: 1, code: 1, errorType: "api", reason: "target_verification_failed"},
		{name: "conflicting envelope parent", url: "https://alidocs.dingtalk.com/i/nodes/b?sheetId=s&fieldId=f", verify: true, body: `{"baseId":"other","data":{"fields":[{"fieldId":"f"}]}}`, calls: 1, code: 1, errorType: "api", reason: "target_verification_failed"},
		{name: "upstream API failure", url: "https://alidocs.dingtalk.com/i/nodes/b?sheetId=s&fieldId=f", verify: true, err: apperrors.NewAPI("Field not found", apperrors.WithReason("field_not_found")), calls: 1, code: 1, errorType: "api", reason: "field_not_found"},
		{name: "upstream auth failure", url: "https://alidocs.dingtalk.com/i/nodes/b?sheetId=s&fieldId=f", verify: true, err: apperrors.NewAuth("Read not authorized", apperrors.WithReason("read_not_authorized")), calls: 1, code: 2, errorType: "auth", reason: "read_not_authorized"},
		{name: "document navigation node", url: "https://alidocs.dingtalk.com/i/nodes/b?applicationId=document-node", verify: true, body: `{"data":{"items":[{"nodeId":"document-node","nodeType":"document"}],"total":1}}`, product: "aitable-helper", tool: "list_nsheet_nodes", params: map[string]any{"baseId": "b"}, targetKey: "applicationId", targetID: "document-node", calls: 1},
		{name: "document UUID is not navigation node", url: "https://alidocs.dingtalk.com/i/nodes/b?applicationId=document-dentry-uuid", verify: true, body: `{"data":{"items":[{"nodeId":"document-node","nodeType":"document"}],"total":1}}`, product: "aitable-helper", tool: "list_nsheet_nodes", params: map[string]any{"baseId": "b"}, calls: 1, code: 1, errorType: "api", reason: "target_verification_failed"},
		{name: "automation background view", url: "https://alidocs.dingtalk.com/i/nodes/b?entrance=automation&sheetId=s&viewId=stale", verify: true, body: `{"baseId":"b"}`, product: "aitable", tool: "get_base", params: map[string]any{"baseId": "b"}, targetKey: "baseId", targetID: "b", calls: 1},
		{name: "workflow with background view", url: "https://alidocs.dingtalk.com/i/nodes/b?entrance=automation&sheetId=s&viewId=stale&flowId=f", verify: true, body: `{"flowId":"f"}`, product: "aitable-helper", tool: "get_workflow", params: map[string]any{"baseId": "b", "workflowId": "f"}, targetKey: "flowId", targetID: "f", calls: 1},
		{name: "wrong workflow with background view", url: "https://alidocs.dingtalk.com/i/nodes/b?entrance=automation&sheetId=s&viewId=stale&flowId=f", verify: true, body: `{"flowId":"other"}`, product: "aitable-helper", tool: "get_workflow", params: map[string]any{"baseId": "b", "workflowId": "f"}, calls: 1, code: 1, errorType: "api", reason: "target_verification_failed"},
		{name: "fractional workflow identity", url: "https://alidocs.dingtalk.com/i/nodes/b?flowId=1.5", verify: true, body: `{"flowId":1.5}`, product: "aitable-helper", tool: "get_workflow", params: map[string]any{"baseId": "b", "workflowId": "1.5"}, calls: 1, code: 1, errorType: "api", reason: "target_verification_failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("DWS_CONFIG_DIR", t.TempDir())
			args := []string{"aitable", "+url-resolve", "--url", tc.url, "--format=json"}
			if tc.verify {
				args = append(args, "--verify")
			}
			testseam.Swap(t, &os.Args, append([]string{"dws"}, args...))
			caller := &aitableURLResolveCaller{body: tc.body, err: tc.err, product: tc.product, tool: tc.tool, args: tc.params}
			var stdout, stderr bytes.Buffer
			testseam.Swap(t, &rootNewRootCommandWithEngine, func(ctx context.Context, engine *pipeline.Engine) *cobra.Command {
				root := NewRootCommandWithEngine(ctx, engine)
				helpers.InitDepsForTest(t, caller)
				root.SetOut(&stdout)
				root.SetErr(&stderr)
				return root
			})
			if code := Execute(); code != tc.code {
				t.Fatalf("exit=%d, want %d; stdout=%s stderr=%s", code, tc.code, &stdout, &stderr)
			}
			if caller.calls != tc.calls {
				t.Fatalf("remote calls=%d, want %d", caller.calls, tc.calls)
			}
			var result struct {
				OK      bool              `json:"ok"`
				Outcome string            `json:"outcome"`
				Data    map[string]any    `json:"data"`
				Error   *output.ErrorInfo `json:"error"`
			}
			if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
				t.Fatalf("expected one result: %v; stdout=%s stderr=%s", err, &stdout, &stderr)
			}
			if tc.code == 0 {
				target, _ := result.Data["target"].(map[string]any)
				key, id := tc.targetKey, tc.targetID
				if key == "" {
					key, id = "fieldId", "f"
				}
				if !result.OK || result.Outcome != "success" || result.Error != nil || target[key] != id || result.Data["verified"] != tc.verify {
					t.Fatalf("incorrect resolution result: %s", &stdout)
				}
			} else if result.OK || result.Outcome != "failure" || result.Data != nil || result.Error == nil || result.Error.Type != tc.errorType || result.Error.Subtype != tc.reason || result.Error.ExitCode != tc.code {
				t.Fatalf("failure classification or exit code lost: %s", &stdout)
			}
		})
	}
}

func TestCrossPlatformCoverageAitableURLResolvePublishedResult(t *testing.T) {
	full := executeShortcutSchemaQuery(t, "--cli-path", "aitable +url-resolve")
	compact := executeShortcutSchemaQuery(t, "--cli-path", "aitable +url-resolve", "--compact")
	if full["result"] == nil || !schemaContractJSONEqual(full["result"], compact["result"]) {
		t.Fatalf("missing or divergent URL resolution Result: full=%v compact=%v", full["result"], compact["result"])
	}
}
