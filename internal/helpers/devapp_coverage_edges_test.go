package helpers

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/spf13/cobra"
)

type devAppErrorRunner struct{ err error }

func (r devAppErrorRunner) Run(context.Context, executor.Invocation) (executor.Result, error) {
	return executor.Result{}, r.err
}

func TestCrossPlatformCoverageDevAppNormalizationEdges(t *testing.T) {
	for _, response := range []map[string]any{
		{},
		{"content": "plain"},
		{"content": map[string]any{}},
		{"content": map[string]any{"success": false}},
		{"content": map[string]any{"success": true}},
		{"content": map[string]any{"success": true, "result": nil}},
	} {
		result := executor.Result{Response: response}
		_ = normalizeDevAppServiceResult(result)
	}
	result := executor.Result{Response: map[string]any{
		"content": map[string]any{"success": true, "result": "unwrapped"},
	}}
	if got := normalizeDevAppServiceResult(result).Response["content"]; got != "unwrapped" {
		t.Fatalf("content = %#v, want unwrapped", got)
	}

	plain := executor.Result{Response: map[string]any{"content": "plain"}}
	if got := normalizeDevAppToolResult(devAppDisableTool, plain).Response["content"]; got != "plain" {
		t.Fatalf("non-map content = %#v, want unchanged", got)
	}
	for tool, key := range map[string]string{devAppDisableTool: "disabled", devAppEnableTool: "enabled"} {
		content := map[string]any{key: false}
		normalizeDevAppToolResult(tool, executor.Result{Response: map[string]any{"content": content}})
		if content[key] != false {
			t.Fatalf("%s existing value = %#v, want false", key, content[key])
		}
	}
}

func TestCrossPlatformCoverageDevAppApprovalNormalizationEdges(t *testing.T) {
	for _, content := range []map[string]any{
		{},
		{"approvalCandidates": "invalid"},
		{"approvalCandidates": []any{}},
		{"approvalCandidates": []any{"invalid"}},
	} {
		normalizeDevAppVersionApproval(content)
	}

	nonSelect := map[string]any{
		"approvalMode": "automatic",
		"approvalCandidates": []any{
			map[string]any{},
		},
	}
	normalizeDevAppVersionApproval(nonSelect)
	options, ok := nonSelect["approvalOptions"].([]map[string]any)
	if !ok || len(options) != 1 || options[0]["label"] != "候选审批人 1" {
		t.Fatalf("approvalOptions = %#v, want fallback candidate", nonSelect["approvalOptions"])
	}

	selectMode := map[string]any{
		"approvalMode": " select_approver ",
		"approvalCandidates": []any{
			map[string]any{"staffId": "u-1", "nickName": " User ", "mainAdmin": "TRUE"},
		},
	}
	normalizeDevAppVersionApproval(selectMode)
	steps, ok := selectMode["nextSteps"].([]map[string]any)
	if !ok || len(steps) != 2 {
		t.Fatalf("nextSteps = %#v, want selection and publish", selectMode["nextSteps"])
	}
	command, _ := steps[1]["command"].(string)
	if !strings.Contains(command, "--unified-app-id <unifiedAppId>") || !strings.Contains(command, "--version-id <versionId>") {
		t.Fatalf("publish command = %q, want locator placeholders", command)
	}

	labels := []struct {
		name, userID string
		admin        bool
		want         string
	}{
		{" Name ", "", false, "Name"},
		{"", "u-1", false, "userId: u-1"},
		{"Name", "u-1", true, "Name（userId: u-1）（主管理员）"},
		{" ", "", true, ""},
	}
	for _, tc := range labels {
		if got := devAppApprovalCandidateLabel(tc.name, tc.userID, tc.admin); got != tc.want {
			t.Errorf("candidate label = %q, want %q", got, tc.want)
		}
	}
	if devAppOptionKey(-1) != "0" || devAppOptionKey(0) != "A" || devAppOptionKey(26) != "27" {
		t.Fatalf("unexpected option keys: %q %q %q", devAppOptionKey(-1), devAppOptionKey(0), devAppOptionKey(26))
	}
}

func TestCrossPlatformCoverageDevAppRobotAndStepEdges(t *testing.T) {
	empty := map[string]any{}
	normalizeDevAppRobotResult(empty)
	if _, ok := empty["lifecycle"]; ok {
		t.Fatal("blank status should not create lifecycle")
	}

	unknown := map[string]any{"status": " future "}
	normalizeDevAppRobotResult(unknown)
	lifecycle := unknown["lifecycle"].(map[string]any)
	if lifecycle["phase"] != "unknown" {
		t.Fatalf("unknown lifecycle = %#v", lifecycle)
	}
	if _, ok := unknown["nextSteps"]; ok {
		t.Fatal("unknown status should not create next steps")
	}

	waiting := map[string]any{"status": "WAITING"}
	normalizeDevAppRobotResult(waiting)
	waitSteps := waiting["nextSteps"].([]map[string]any)
	if command := waitSteps[0]["command"].(string); !strings.Contains(command, "--task-id <taskId>") {
		t.Fatalf("poll command = %q, want task placeholder", command)
	}

	failed := map[string]any{"status": "FAIL"}
	normalizeDevAppRobotResult(failed)
	failSteps := failed["nextSteps"].([]map[string]any)
	if command := failSteps[0]["command"].(string); !strings.Contains(command, "--task-id <taskId>") {
		t.Fatalf("retry command = %q, want task placeholder", command)
	}

	success := map[string]any{"status": "SUCCESS", "unifiedAppId": "u-1", "clientId": "client-only"}
	normalizeDevAppRobotResult(success)
	if got := success["nextSteps"].([]map[string]any); len(got) != 4 {
		t.Fatalf("steps without complete credentials = %d, want 4", len(got))
	}

	connect := devAppRobotConnectStep("", "")
	if command := connect["command"].(string); !strings.Contains(command, "--robot-client-id <clientId>") {
		t.Fatalf("connect command = %q, want client placeholder", command)
	}

	minimal := devAppNextStep(devAppStep{ID: "minimal"})
	if _, ok := minimal["command"]; ok {
		t.Fatalf("minimal step unexpectedly contains command: %#v", minimal)
	}
	if _, ok := minimal["dryRunCommand"]; ok {
		t.Fatalf("minimal step unexpectedly contains dry run command: %#v", minimal)
	}
}

func TestCrossPlatformCoverageDevAppScalarAndScopeEdges(t *testing.T) {
	content := map[string]any{
		"nil":    nil,
		"text":   " value ",
		"number": 42,
		"yes":    true,
		"no":     false,
		"truth":  " TRUE ",
		"other":  1,
	}
	if devAppContentString(content, "missing") != "" || devAppContentString(content, "nil") != "" {
		t.Fatal("missing and nil strings should be empty")
	}
	if devAppContentString(content, "text") != "value" || devAppContentString(content, "number") != "42" {
		t.Fatal("string conversion did not trim or stringify")
	}
	if !devAppContentBool(content, "yes") || devAppContentBool(content, "no") || !devAppContentBool(content, "truth") || devAppContentBool(content, "other") {
		t.Fatal("boolean conversion returned unexpected values")
	}
	if devAppContentBool(content, "missing") || devAppContentBool(content, "nil") {
		t.Fatal("missing and nil booleans should be false")
	}

	valid := map[string]any{"scopes": []any{"scope-a", map[string]any{"scopeValue": "scope-b"}}}
	normalizeDevAppScopeValueArray(valid, "scopes")
	if want := []string{"scope-a", "scope-b"}; !reflect.DeepEqual(valid["scopes"], want) {
		t.Fatalf("scopes = %#v, want %#v", valid["scopes"], want)
	}
	empty := map[string]any{"scopes": []any{}}
	normalizeDevAppScopeValueArray(empty, "scopes")
	if got, ok := empty["scopes"].([]string); !ok || len(got) != 0 {
		t.Fatalf("empty scopes = %#v, want []string", empty["scopes"])
	}
	invalidValues := []any{"", map[string]any{}, 42}
	invalid := map[string]any{"scopes": invalidValues, "other": "unchanged"}
	normalizeDevAppScopeValueArray(invalid, "missing")
	normalizeDevAppScopeValueArray(invalid, "scopes")
	if !reflect.DeepEqual(invalid["scopes"], invalidValues) {
		t.Fatalf("invalid scopes = %#v, want unchanged", invalid["scopes"])
	}
}

func TestCrossPlatformCoverageDevAppCommandUtilityEdges(t *testing.T) {
	annotated := annotateDevAppTool(&cobra.Command{Use: "leaf"}, "tool")
	if annotated.Annotations["mcp-tool"] != "tool" || annotated.Annotations["mcp-source"] != "op-app" {
		t.Fatalf("annotations = %#v", annotated.Annotations)
	}

	cmd := &cobra.Command{Use: "leaf"}
	cmd.Flags().String("primary", "", "")
	cmd.Flags().String("fallback", "fallback-value", "")
	cmd.Flags().Int("count", 7, "")
	if got := devAppFlagOrFallback(cmd, "primary", "fallback"); got != "fallback-value" {
		t.Fatalf("fallback = %q", got)
	}
	if err := cmd.Flags().Set("primary", "primary-value"); err != nil {
		t.Fatal(err)
	}
	if got := devAppFlagOrFallback(cmd, "primary", "fallback"); got != "primary-value" {
		t.Fatalf("primary = %q", got)
	}
	if devAppIntFlag(cmd, "count") != 7 {
		t.Fatal("int flag not read")
	}
	params := map[string]any{}
	devAppPutString(params, "empty", "")
	devAppPutInt(params, "zero", 0)
	devAppPutInt(params, "count", 7)
	if len(params) != 1 || params["count"] != 7 {
		t.Fatalf("params = %#v", params)
	}

	wantErr := errors.New("runner failure")
	if err := runDevAppTool(devAppErrorRunner{err: wantErr}, cmd, "tool", nil); !errors.Is(err, wantErr) {
		t.Fatalf("runDevAppTool error = %v, want %v", err, wantErr)
	}

	for _, response := range []map[string]any{
		{"name": "top"},
		{"content": map[string]any{"name": "content"}},
		{"content": map[string]any{"result": map[string]any{"name": "result"}}},
		{"content": "invalid"},
		{"content": map[string]any{"result": "invalid"}},
	} {
		_ = devAppExtractString(response, "name")
	}
}
