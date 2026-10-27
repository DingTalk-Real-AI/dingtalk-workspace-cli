package helpers

import (
	"reflect"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"strings"
	"testing"
)

func TestDevAppSuccessNextStepsScopePrecheck(t *testing.T) {
	steps := devAppSuccessNextSteps(devAppScopeSetTool, map[string]any{
		"unifiedAppId":      "app-1",
		"userVisibleScopes": []string{"1001", "1002"},
		"precheckOnly":      true,
		"publish":           false,
	}, map[string]any{"outcome": "planned"})
	if len(steps) != 1 || steps[0]["id"] != "apply_scope" {
		t.Fatalf("steps = %#v", steps)
	}
	command, _ := steps[0]["command"].(string)
	if !strings.Contains(command, `--user-visible-scopes `+ShellQuoteArg(`["1001","1002"]`)) || !strings.Contains(command, "--precheck-only=false") || strings.Contains(command, "--yes") {
		t.Fatalf("command = %q", command)
	}
}

func TestCrossPlatformCoverageDevAppScopeApplyReplayMatchesPrecheckArray(t *testing.T) {
	params := map[string]any{
		"unifiedAppId":      "app-1",
		"userVisibleScopes": []any{"group,a", "b", 2},
		"precheckOnly":      true,
	}
	steps := devAppSuccessNextSteps(devAppScopeSetTool, params, map[string]any{"outcome": "planned"})
	if len(steps) != 1 || steps[0]["id"] != "apply_scope" {
		t.Fatalf("steps = %#v", steps)
	}
	command, _ := steps[0]["command"].(string)
	jsonForm := devAppParamString(params, "userVisibleScopes")
	quoted, ok := devAppQuoteScopeValue(jsonForm)
	if !ok {
		// The platform quoter cannot safely quote the JSON array (double
		// quotes): the step must fail closed with missingParams instead of
		// fabricating a command.
		if command != "" || !strings.Contains(strings.Join(anyStrings(steps[0]["missingParams"]), ","), "userVisibleScopes") {
			t.Fatalf("fail-closed apply step = %#v", steps[0])
		}
		return
	}
	if !strings.Contains(command, "--user-visible-scopes "+quoted) {
		t.Fatalf("command %q does not carry quoted scope list %q", command, quoted)
	}
	replayed, err := transformDevAppListParam(jsonForm)
	if err != nil {
		t.Fatalf("replay transform failed: %v", err)
	}
	got, _ := replayed.([]string)
	want := []string{"group,a", "b", "2"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("replayed scopes = %#v, want %#v (command=%q)", got, want, command)
	}
}

func TestCrossPlatformCoverageDevAppWindowsNextStepValuesNeverInline(t *testing.T) {
	for _, value := range []string{"app id", "a&whoami", "a|whoami", "a<b", "a> b", "a^b", "%PATH%", `a"b`, "a'b"} {
		if got, ok := driveLatestWindowsScopeValue(value); ok || got != "" {
			t.Fatalf("Windows unsafe value %q was inlined as %q", value, got)
		}
	}
	if got, ok := driveLatestWindowsScopeValue("safe-id_1"); !ok || got != "safe-id_1" {
		t.Fatalf("Windows safe value = %q, %v", got, ok)
	}
}

func TestCrossPlatformCoverageDevAppNextStepsOmitUnsafeWindowsCommands(t *testing.T) {
	testseam.Swap(t, &devAppQuoteScopeValue, driveLatestWindowsScopeValue)

	readBack := devAppSuccessNextSteps(devAppUpdateTool, map[string]any{"unifiedAppId": "app&whoami"}, map[string]any{})
	if len(readBack) != 1 || readBack[0]["command"] != nil || len(readBack[0]["missingParams"].([]string)) != 1 {
		t.Fatalf("unsafe read-back step = %#v", readBack)
	}

	publish := devAppVersionCreateSuccessNextSteps(map[string]any{"unifiedAppId": "app&whoami", "versionId": "version id"})
	if len(publish) != 1 || publish[0]["command"] != nil {
		t.Fatalf("unsafe publish step = %#v", publish)
	}
	publishPending := devAppVersionPublishSuccessNextSteps(nil, map[string]any{
		"unifiedAppId": "app&whoami", "versionId": "version id", "needsPublish": true,
	})
	if len(publishPending) != 1 || publishPending[0]["command"] != nil {
		t.Fatalf("unsafe publish-pending step = %#v", publishPending)
	}
	approval := devAppVersionCheckApprovalSuccessNextSteps(map[string]any{
		"unifiedAppId": "app&whoami", "versionId": "version id",
	})
	if len(approval) != 1 || approval[0]["command"] != nil {
		t.Fatalf("unsafe approval step = %#v", approval)
	}

	scope := devAppSuccessNextSteps(devAppScopeSetTool, map[string]any{
		"unifiedAppId":      "safe-app",
		"userVisibleScopes": []string{"scope&whoami"},
		"precheckOnly":      true,
	}, map[string]any{"outcome": "planned"})
	if len(scope) != 1 || scope[0]["command"] != nil {
		t.Fatalf("unsafe scope step = %#v", scope)
	}
	scopeApp := devAppSuccessNextSteps(devAppScopeSetTool, map[string]any{
		"unifiedAppId":      "app&whoami",
		"userVisibleScopes": []string{"safe-scope"},
		"precheckOnly":      true,
	}, map[string]any{"outcome": "planned"})
	if len(scopeApp) != 1 || scopeApp[0]["command"] != nil {
		t.Fatalf("unsafe scope app step = %#v", scopeApp)
	}

	robot := devAppSuccessNextSteps(devAppRobotSubmitTool, nil, map[string]any{"taskId": "task|whoami"})
	if len(robot) != 1 || robot[0]["command"] != nil {
		t.Fatalf("unsafe robot step = %#v", robot)
	}
	if got := devAppRecoveryCommand(map[string]any{"taskId": "task|whoami"}); got != "" {
		t.Fatalf("unsafe recovery command = %q", got)
	}
	if got := devAppRecoveryCommand(map[string]any{"unifiedAppId": "app&whoami", "versionId": "version id"}); got != "" {
		t.Fatalf("unsafe version recovery command = %q", got)
	}
}

func TestCrossPlatformCoverageDevAppNextStepBranchEdges(t *testing.T) {
	if got := devAppSuccessNextSteps("unrelated", map[string]any{"precheckOnly": true}, map[string]any{}); got != nil {
		t.Fatalf("unrelated precheck steps = %#v", got)
	}
	if got := devAppSuccessNextSteps(devAppVersionPublishTool, nil, map[string]any{"approvalMode": "SELECT_APPROVER"}); got != nil {
		t.Fatalf("select approver steps = %#v", got)
	}
	if got := devAppScopeSetSuccessNextSteps(map[string]any{"precheckOnly": true}, map[string]any{"outcome": "planned"}); len(got) != 1 || got[0]["command"] != nil {
		t.Fatalf("scope missing params = %#v", got)
	}
	if got := devAppVersionPublishSuccessNextSteps(nil, map[string]any{"needsPublish": true}); got != nil {
		t.Fatalf("publish missing IDs = %#v", got)
	}
	if got := devAppVersionCheckApprovalSuccessNextSteps(map[string]any{"published": true}); got != nil {
		t.Fatalf("published approval steps = %#v", got)
	}
	if got := devAppVersionCheckApprovalSuccessNextSteps(map[string]any{}); got != nil {
		t.Fatalf("approval missing IDs = %#v", got)
	}
	if got := devAppVersionCreateSuccessNextSteps(map[string]any{}); got != nil {
		t.Fatalf("version missing IDs = %#v", got)
	}
	if devAppParamString(nil, "x") != "" || devAppParamString(map[string]any{"x": []string{"a", "b"}}, "x") != `["a","b"]` || devAppParamString(map[string]any{"x": []any{"a", "", 2}}, "x") != `["a","2"]` || devAppParamString(map[string]any{"x": 2}, "x") != "2" ||
		devAppParamString(map[string]any{"x": []string{"group,a"}}, "x") != `["group,a"]` ||
		devAppParamString(map[string]any{"x": []string{" "}}, "x") != "" || devAppParamString(map[string]any{"x": []string{"only"}}, "x") != "only" {
		t.Fatal("parameter string edge cases failed")
	}
	if !devAppNextStepIDExists([]map[string]any{{"id": "same"}}, "same") || devAppNextStepIDExists(nil, "same") || devAppNextStepIDExists(nil, "") {
		t.Fatal("next step ID edge cases failed")
	}
}

func TestCrossPlatformCoverageDevAppApprovalNextStepBranches(t *testing.T) {
	missing := devAppSuccessNextSteps(devAppVersionPublishTool, map[string]any{"precheckOnly": true}, map[string]any{
		"approvalMode": "SELECT_APPROVER", "unifiedAppId": "app", "versionId": "version",
	})
	if len(missing) != 1 || missing[0]["command"] != nil {
		t.Fatalf("missing approver step = %#v", missing)
	}
	selected := devAppSuccessNextSteps(devAppVersionPublishTool, map[string]any{"precheckOnly": true}, map[string]any{
		"approvalMode": "SELECT_APPROVER", "approverUserId": "user-1", "unifiedAppId": "app", "versionId": "version",
	})
	if len(selected) != 1 || !strings.Contains(devAppContentString(selected[0], "command"), "--approver-user-id user-1") {
		t.Fatalf("selected approver step = %#v", selected)
	}
	testseam.Swap(t, &devAppQuoteScopeValue, driveLatestWindowsScopeValue)
	unsafe := devAppVersionCheckApprovalSuccessNextSteps(map[string]any{
		"approvalMode": "SELECT_APPROVER", "approverUserId": "user&bad", "unifiedAppId": "app", "versionId": "version",
	})
	if len(unsafe) != 1 || unsafe[0]["command"] != nil {
		t.Fatalf("unsafe approver step = %#v", unsafe)
	}
	missingPublish := devAppSuccessNextSteps(devAppUpdateTool, map[string]any{"precheckOnly": false}, map[string]any{
		"needsPublish": true, "approvalMode": "SELECT_APPROVER", "unifiedAppId": "app", "versionId": "version",
	})
	if len(missingPublish) != 1 || missingPublish[0]["command"] != nil {
		t.Fatalf("missing publish approver step = %#v", missingPublish)
	}
	selectedPublish := devAppSuccessNextSteps(devAppUpdateTool, map[string]any{"precheckOnly": false}, map[string]any{
		"needsPublish": true, "approvalMode": "SELECT_APPROVER", "approverUserId": "user-1", "unifiedAppId": "app", "versionId": "version",
	})
	if len(selectedPublish) != 1 || !strings.Contains(devAppContentString(selectedPublish[0], "command"), "--approver-user-id user-1") {
		t.Fatalf("selected publish approver step = %#v", selectedPublish)
	}
	unsafePublish := devAppSuccessNextSteps(devAppUpdateTool, map[string]any{"precheckOnly": false}, map[string]any{
		"needsPublish": true, "approvalMode": "SELECT_APPROVER", "approverUserId": "user&bad", "unifiedAppId": "app", "versionId": "version",
	})
	if len(unsafePublish) != 1 || unsafePublish[0]["command"] != nil {
		t.Fatalf("unsafe publish approver step = %#v", unsafePublish)
	}
}

func TestDevAppSuccessNextStepsScopeApplyQuotesAllDynamicValues(t *testing.T) {
	steps := devAppSuccessNextSteps(devAppScopeSetTool, map[string]any{
		"unifiedAppId":      "app; echo bad",
		"userVisibleScopes": []string{"user one", "user&two"},
		"precheckOnly":      true,
	}, map[string]any{"outcome": "planned"})
	if len(steps) != 1 {
		t.Fatalf("steps = %#v", steps)
	}
	command := devAppContentString(steps[0], "command")
	for _, want := range []string{
		"--unified-app-id " + ShellQuoteArg("app; echo bad"),
		"--user-visible-scopes " + ShellQuoteArg(`["user one","user&two"]`),
	} {
		if !strings.Contains(command, want) {
			t.Fatalf("command %q does not contain %q", command, want)
		}
	}
}

func TestCrossPlatformCoverageDevAppSuccessNextStepsScopePrecheckPreservesFlags(t *testing.T) {
	for _, tc := range []struct {
		name   string
		params map[string]any
		want   string
	}{
		{name: "hidden true", params: map[string]any{"unifiedAppId": "app-1", "isHidden": true, "precheckOnly": true}, want: "--is-hidden=true"},
		{name: "hidden false", params: map[string]any{"unifiedAppId": "app-1", "isHidden": false, "precheckOnly": true}, want: "--is-hidden=false"},
		{name: "publish true", params: map[string]any{"unifiedAppId": "app-1", "isHidden": false, "precheckOnly": true, "publish": true}, want: "--publish=true"},
		{name: "publish false", params: map[string]any{"unifiedAppId": "app-1", "isHidden": false, "precheckOnly": true, "publish": false}, want: "--publish=false"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			steps := devAppSuccessNextSteps(devAppScopeSetTool, tc.params, map[string]any{"outcome": "planned"})
			if len(steps) != 1 {
				t.Fatalf("steps = %#v", steps)
			}
			command := devAppContentString(steps[0], "command")
			if !strings.Contains(command, tc.want) || strings.Contains(command, "--yes") {
				t.Fatalf("command = %q", command)
			}
			if _, ok := steps[0]["missingParams"]; ok {
				t.Fatalf("unexpected missingParams = %#v", steps[0]["missingParams"])
			}
		})
	}
}

func TestCrossPlatformCoverageDevAppSuccessNextStepsScopeMissingParams(t *testing.T) {
	steps := devAppSuccessNextSteps(devAppScopeSetTool, map[string]any{
		"unifiedAppId": "app-1",
		"precheckOnly": true,
	}, map[string]any{"outcome": "planned"})
	if len(steps) != 1 {
		t.Fatalf("steps = %#v", steps)
	}
	if _, ok := steps[0]["command"]; ok {
		t.Fatalf("missing parameters must not produce command: %#v", steps[0])
	}
	missing, _ := steps[0]["missingParams"].([]string)
	if len(missing) != 1 || missing[0] != "至少一个可见范围参数" {
		t.Fatalf("missingParams = %#v", steps[0]["missingParams"])
	}
}

func TestCrossPlatformCoverageDevAppSuccessNextStepsVersionStatus(t *testing.T) {
	steps := devAppSuccessNextSteps(devAppVersionCreateTool, nil, map[string]any{
		"unifiedAppId": "app-1",
		"versionId":    "version-1",
	})
	if len(steps) != 1 || steps[0]["id"] != "check_version_status" {
		t.Fatalf("steps = %#v", steps)
	}
	if got := steps[0]["command"].(string); !strings.Contains(got, "--version-id version-1") {
		t.Fatalf("command = %q", got)
	}
}

func TestCrossPlatformCoverageDevAppSuccessNextStepsVersionPublish(t *testing.T) {
	steps := devAppSuccessNextSteps(devAppRobotConfigUpsertTool, map[string]any{"unifiedAppId": "app-1"}, map[string]any{
		"needsPublish": true,
		"published":    false,
		"versionId":    "version-1",
	})
	if len(steps) != 1 || steps[0]["id"] != "publish_version" {
		t.Fatalf("steps = %#v", steps)
	}
	if got := steps[0]["command"].(string); !strings.Contains(got, "dev app version publish") || !strings.Contains(got, "--confirmed-sensitive=false") {
		t.Fatalf("command = %q", got)
	}
}

func TestCrossPlatformCoverageDevAppSuccessNextStepsDoesNotDuplicateApprovalPublish(t *testing.T) {
	steps := devAppSuccessNextSteps(devAppVersionPublishTool, map[string]any{"precheckOnly": true}, map[string]any{
		"requiresApproval": true,
		"approvalMode":     "SELECT_APPROVER",
		"approvalOptions":  []map[string]any{{"key": "A"}},
		"unifiedAppId":     "app-1",
		"versionId":        "version-1",
	})
	if len(steps) != 1 || steps[0]["command"] != nil {
		t.Fatalf("steps = %#v", steps)
	}
	missing, _ := steps[0]["missingParams"].([]string)
	if len(missing) != 1 || missing[0] != "approver-user-id" {
		t.Fatalf("missing approver = %#v", steps[0]["missingParams"])
	}
}

func TestCrossPlatformCoverageDevAppSuccessNextStepsIgnoresNonSuccessStates(t *testing.T) {
	if got := devAppSuccessNextSteps(devAppScopeSetTool, map[string]any{"precheckOnly": false}, map[string]any{"outcome": "pending"}); len(got) != 0 {
		t.Fatalf("pending steps = %#v", got)
	}
}

func TestCrossPlatformCoverageAppendDevAppSuccessNextStepsDeduplicates(t *testing.T) {
	content := map[string]any{"nextSteps": []any{map[string]any{"id": "apply_scope"}}}
	appendDevAppSuccessNextSteps(content, map[string]any{"id": "apply_scope", "command": "dws duplicate"})
	if got := len(normalizeDevAppNextSteps(content["nextSteps"])); got != 1 {
		t.Fatalf("step count = %d", got)
	}
}

func TestCrossPlatformCoverageDevAppSuccessNextStepsRemainingWrites(t *testing.T) {
	for tool, id := range map[string]string{
		devAppCreateTool: "check_app", devAppUpdateTool: "check_app", devAppEnableTool: "check_app", devAppDisableTool: "check_app", devAppDeleteTool: "check_app_list",
		devAppScopeSetTool: "check_scope", devAppWebappSetTool: "check_webapp",
		devAppPermissionAddTool: "check_permissions", devAppPermissionRmTool: "check_permissions", devAppMemberAddTool: "check_members", devAppMemberRemoveTool: "check_members", devAppSecurityConfigTool: "verify_security_config",
		devAppEventSubscribeTool: "check_events", devAppEventUnsubscribeTool: "check_events", devAppRobotConfigUpsertTool: "check_robot", devAppRobotEnableTool: "check_robot", devAppRobotOfflineTool: "check_robot", devAppRobotSubmitTool: "check_robot_result", devAppVersionPublishTool: "check_version_status",
	} {
		t.Run(tool, func(t *testing.T) {
			params := map[string]any{"unifiedAppId": "app-1", "versionId": "v1", "clientSecret": "secret-must-not-leak"}
			content := map[string]any{"taskId": "t1", "unifiedAppId": "app-1"}
			steps := devAppSuccessNextSteps(tool, params, content)
			if len(steps) != 1 || steps[0]["id"] != id {
				t.Fatalf("steps=%#v", steps)
			}
			if strings.Contains(devAppContentString(steps[0], "command"), "secret-must-not-leak") {
				t.Fatal("secret leaked")
			}
			for _, state := range []string{"pending", "failure", "unknown"} {
				content["outcome"] = state
				if got := devAppSuccessNextSteps(tool, params, content); len(got) != 0 {
					t.Fatalf("%s: %#v", state, got)
				}
			}
		})
	}
}

func TestCrossPlatformCoverageDevAppSuccessNextStepsReadTerminals(t *testing.T) {
	for _, tool := range []string{devAppListTool, devAppGetTool, devAppCredentialsGetTool, devAppScopeGetTool, devAppWebappGetTool, devAppPermissionListTool, devAppMemberListTool, devAppEventListTool, devAppRobotConfigGetTool, devAppRobotResultTool, devAppVersionListTool, devAppVersionDetailTool, devAppVersionStatusTool} {
		if steps := devAppSuccessNextSteps(tool, map[string]any{"unifiedAppId": "a"}, map[string]any{"versionId": "v"}); len(steps) != 0 {
			t.Fatalf("%s: %#v", tool, steps)
		}
	}
}

func TestCrossPlatformCoverageDevAppSuccessNextStepsResultPaths(t *testing.T) {
	for _, dryRun := range []bool{false, true} {
		result := devAppCommandResult(executor.Result{Invocation: executor.Invocation{Implemented: true, Kind: "helper_invocation", Tool: devAppUpdateTool, Params: map[string]any{"unifiedAppId": "a"}, DryRun: dryRun}, Response: map[string]any{"content": map[string]any{"hasMore": false, "items": []any{}}}})
		env, err := output.EnvelopeFromResult(result)
		if err != nil {
			t.Fatal(err)
		}
		if env.Outcome != output.OutcomeSuccess {
			t.Fatalf("env=%+v", env)
		}
		data, _ := env.Data.(map[string]any)
		steps := normalizeDevAppNextSteps(data["nextSteps"])
		if dryRun && len(steps) != 0 || !dryRun && len(steps) != 1 {
			t.Fatalf("dryRun=%v data=%#v", dryRun, data)
		}
	}
}

func TestCrossPlatformCoverageDevAppSuccessNextStepsApprovalAndPublishGuards(t *testing.T) {
	for _, options := range []any{[]any{map[string]any{"key": "A"}}, []map[string]any{{"key": "A"}}, nil} {
		steps := devAppSuccessNextSteps(devAppVersionPublishTool, map[string]any{"precheckOnly": true}, map[string]any{"approvalMode": "SELECT_APPROVER", "approvalOptions": options, "unifiedAppId": "a", "versionId": "v"})
		if len(steps) != 1 || steps[0]["command"] != nil {
			t.Fatalf("steps=%#v", steps)
		}
		missing, _ := steps[0]["missingParams"].([]string)
		if len(missing) != 1 || missing[0] != "approver-user-id" {
			t.Fatalf("missing approver=%#v", steps[0]["missingParams"])
		}
	}
	steps := devAppSuccessNextSteps(devAppVersionPublishTool, map[string]any{"precheckOnly": true}, map[string]any{
		"approvalMode": "SELECT_APPROVER", "approverUserId": "user-1", "unifiedAppId": "a", "versionId": "v",
	})
	if len(steps) != 1 || !strings.Contains(devAppContentString(steps[0], "command"), "--approver-user-id user-1") {
		t.Fatalf("selected approver steps=%#v", steps)
	}
	steps = devAppSuccessNextSteps(devAppVersionPublishTool, map[string]any{"precheckOnly": true, "unifiedAppId": "a", "versionId": "v"}, map[string]any{"requiresApproval": false})
	if len(steps) != 1 || steps[0]["id"] != "publish_version" ||
		!strings.Contains(devAppContentString(steps[0], "command"), "--confirmed-sensitive=false") {
		t.Fatalf("steps=%#v", steps)
	}
	steps = devAppSuccessNextSteps(devAppVersionPublishTool, map[string]any{"precheckOnly": true}, map[string]any{
		"requiresApproval": true, "unifiedAppId": "a", "versionId": "v",
	})
	if len(steps) != 1 {
		t.Fatalf("missing approval mode steps=%#v", steps)
	}
	if _, ok := steps[0]["command"]; ok {
		t.Fatalf("missing approval mode must not produce publish command: %#v", steps[0])
	}
	missing, _ := steps[0]["missingParams"].([]string)
	if len(missing) != 2 || missing[0] != "approvalMode" || missing[1] != "approver-user-id" {
		t.Fatalf("missing approval params=%#v", steps[0]["missingParams"])
	}
	for _, tool := range []string{devAppScopeSetTool, devAppEventSubscribeTool, devAppEventUnsubscribeTool, devAppUpdateTool} {
		steps := devAppSuccessNextSteps(tool, map[string]any{"unifiedAppId": "a"}, map[string]any{"needsPublish": true, "versionId": "v"})
		if len(steps) != 1 || steps[0]["id"] != "publish_version" {
			t.Fatalf("%s: %#v", tool, steps)
		}
	}
	if got := devAppReadBackStep(nil, nil, "get", "check_app", "check"); len(got) != 0 {
		t.Fatalf("missing ID: %#v", got)
	}
	steps = devAppReadBackStep(map[string]any{"unifiedAppId": "a; echo bad"}, nil, "get", "check_app", "check")
	if quoted, ok := devAppQuoteScopeValue("a; echo bad"); ok {
		if !strings.Contains(devAppContentString(steps[0], "command"), quoted) {
			t.Fatal("unquoted ID")
		}
	} else if steps[0]["command"] != nil || !reflect.DeepEqual(steps[0]["missingParams"], []string{"unifiedAppId"}) {
		t.Fatalf("unsafe ID must require input instead of producing a command: %#v", steps[0])
	}
}
