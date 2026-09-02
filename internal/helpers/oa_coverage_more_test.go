package helpers

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func executeOACommand(t *testing.T, caller *scriptedToolCaller, args ...string) error {
	t.Helper()
	previous := deps
	previousArgs := os.Args
	os.Args = []string{"dws", "oa"}
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	t.Cleanup(func() {
		deps = previous
		os.Args = previousArgs
	})

	cmd := newOaCommand()
	cmd.PersistentFlags().Bool("yes", false, "跳过确认")
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func TestCrossPlatformCoverageOARemainingTimeAndRevertBranches(t *testing.T) {
	installScriptedCaller(t, &scriptedToolCaller{dry: true})
	for _, args := range [][]string{
		{"approval", "list-pending", "--start", "bad", "--end", "2030-01-01T10:00:00+08:00"},
		{"approval", "list-pending", "--start", "2030-01-01T09:00:00+08:00", "--end", "bad"},
		{"approval", "list-pending", "--start", "2030-01-01T10:00:00+08:00", "--end", "2030-01-01T09:00:00+08:00"},
		{"approval", "list-initiated", "--process-code", "code", "--start", "bad", "--end", "2030-01-01T10:00:00+08:00"},
		{"approval", "list-initiated", "--process-code", "code", "--start", "2030-01-01T09:00:00+08:00", "--end", "bad"},
		{"approval", "list-initiated", "--process-code", "code", "--start", "2030-01-01T10:00:00+08:00", "--end", "2030-01-01T09:00:00+08:00"},
	} {
		if err := executeFilterCoverage(t, newOaCommand(), args...); err == nil {
			t.Fatalf("args=%v returned nil", args)
		}
	}

	if err := executeFilterCoverage(t, newOaCommand(),
		"approval", "list-pending",
		"--start", "2030-01-01T09:00:00+08:00", "--end", "2030-01-01T10:00:00+08:00",
		"--page", "2", "--size", "20", "--query", "travel",
	); err != nil {
		t.Fatalf("pending options: %v", err)
	}
	if err := executeFilterCoverage(t, newOaCommand(),
		"approval", "revert-task", "--instance-id", "instance", "--task-id", "12",
		"--target-activity-id", "activity", "--action", "REVERT_FOR_APPROVAL", "--remark", "retry",
	); err != nil {
		t.Fatalf("revert task: %v", err)
	}
}

func TestCrossPlatformCoverageOAApprovalCreateInstanceMapsInternalSimpleOptions(t *testing.T) {
	caller := &scriptedToolCaller{}
	err := executeOACommand(t, caller,
		"approval", "create-instance",
		"--process-code", "PROC",
		"--form-values", `{"事由":"测试"}`,
		"--originator-user-id", "originator",
		"--approvers", "approver-1,approver-2",
		"--approvers-action-type", "AND",
		"--cc-list", "cc-1,cc-2",
		"--cc-position", "FINISH",
		"--yes",
	)
	if err != nil {
		t.Fatalf("create instance: %v", err)
	}
	if caller.server != "oa" || caller.tool != "start_process_instance" {
		t.Fatalf("called %s/%s, want oa/start_process_instance", caller.server, caller.tool)
	}
	request, ok := caller.args["ProcessInstanceCreationPopRequest"].(map[string]any)
	if !ok {
		t.Fatalf("request payload = %#v", caller.args)
	}
	if got := request["originatorUserId"]; got != "originator" {
		t.Fatalf("originatorUserId = %#v", got)
	}
	approvers, ok := request["approvers"].([]map[string]any)
	if !ok || len(approvers) != 1 || approvers[0]["actionType"] != "AND" {
		t.Fatalf("approvers = %#v", request["approvers"])
	}
	if got := approvers[0]["userIds"]; len(got.([]string)) != 2 || got.([]string)[0] != "approver-1" || got.([]string)[1] != "approver-2" {
		t.Fatalf("approver userIds = %#v", got)
	}
	if got := request["ccList"]; len(got.([]string)) != 2 || got.([]string)[0] != "cc-1" || got.([]string)[1] != "cc-2" {
		t.Fatalf("ccList = %#v", got)
	}
	if got := request["ccPosition"]; got != "FINISH" {
		t.Fatalf("ccPosition = %#v", got)
	}
}

func TestCrossPlatformCoverageOAApprovalCreateInstanceRejectsMixedRequestModes(t *testing.T) {
	caller := &scriptedToolCaller{}
	err := executeOACommand(t, caller,
		"approval", "create-instance",
		"--request", `{"processCode":"PROC"}`,
		"--process-code", "PROC",
		"--yes",
	)
	if err == nil {
		t.Fatal("mixed request modes returned nil")
	}
	if caller.calls != 0 {
		t.Fatalf("unexpected MCP call count: %d", caller.calls)
	}
}

func TestCrossPlatformCoverageOAApprovalCreateInstanceRequiresExplicitYes(t *testing.T) {
	caller := &scriptedToolCaller{}
	err := executeOACommand(t, caller,
		"approval", "create-instance",
		"--request", `{"processCode":"PROC"}`,
	)
	if err == nil || !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("create instance without --yes error = %v, want explicit --yes requirement", err)
	}
	if caller.calls != 0 {
		t.Fatalf("create instance without --yes made %d MCP calls", caller.calls)
	}
}

func TestCrossPlatformCoverageOAApprovalListByAdminMapsSimpleOptions(t *testing.T) {
	caller := &scriptedToolCaller{}
	err := executeOACommand(t, caller,
		"approval", "list-by-admin",
		"--process-code", "PROC",
		"--start", "2030-01-01T09:00:00+08:00",
		"--end", "2030-01-01T10:00:00+08:00",
		"--cursor", "5",
		"--limit", "20",
		"--user-ids", "user-1,user-2",
		"--statuses", "RUNNING,COMPLETED",
	)
	if err != nil {
		t.Fatalf("list by admin: %v", err)
	}
	if caller.server != "oa" || caller.tool != "get_process_instances_by_admin" {
		t.Fatalf("called %s/%s, want oa/get_process_instances_by_admin", caller.server, caller.tool)
	}
	request, ok := caller.args["ProcessInstanceListQueryRequest"].(map[string]any)
	if !ok {
		t.Fatalf("request payload = %#v", caller.args)
	}
	if got := request["processCode"]; got != "PROC" {
		t.Fatalf("processCode = %#v", got)
	}
	if got := request["cursor"]; got != float64(5) {
		t.Fatalf("cursor = %#v", got)
	}
	if got := request["pageSize"]; got != float64(20) {
		t.Fatalf("pageSize = %#v", got)
	}
	if got := request["startTime"]; got != "2030-01-01 09:00:00" {
		t.Fatalf("startTime = %#v", got)
	}
	if got := request["endTime"]; got != "2030-01-01 10:00:00" {
		t.Fatalf("endTime = %#v", got)
	}
	if got := request["userIds"]; len(got.([]string)) != 2 || got.([]string)[0] != "user-1" || got.([]string)[1] != "user-2" {
		t.Fatalf("userIds = %#v", got)
	}
	if got := request["statuses"]; len(got.([]string)) != 2 || got.([]string)[0] != "RUNNING" || got.([]string)[1] != "COMPLETED" {
		t.Fatalf("statuses = %#v", got)
	}
}

func TestCrossPlatformCoverageOAApprovalNewCommandValidationAndRequestModes(t *testing.T) {
	validCases := []struct {
		name string
		args []string
		tool string
	}{
		{
			name: "form schema",
			args: []string{"approval", "form-schema", "--process-code", "PROC"},
			tool: "get_process_schema",
		},
		{
			name: "forecast simple mode",
			args: []string{"approval", "forecast-process", "--process-code", "PROC", "--dept-id", "-1", "--form-values", `{"金额":"100"}`},
			tool: "forecast_process",
		},
		{
			name: "forecast request mode",
			args: []string{"approval", "forecast-process", "--request", `{"processCode":"PROC"}`},
			tool: "forecast_process",
		},
		{
			name: "create request mode",
			args: []string{"approval", "create-instance", "--request", `{"processCode":"PROC"}`, "--yes"},
			tool: "start_process_instance",
		},
		{
			name: "list-by-admin simple mode",
			args: []string{"approval", "list-by-admin", "--process-code", "PROC", "--start", "2030-01-01T09:00:00+08:00"},
			tool: "get_process_instances_by_admin",
		},
		{
			name: "list-by-admin request mode",
			args: []string{"approval", "list-by-admin", "--request", `{"processCode":"PROC","startTime":"2030-01-01 09:00:00","cursor":0,"pageSize":20}`},
			tool: "get_process_instances_by_admin",
		},
		{
			name: "list-by-admin request mode without pageSize",
			args: []string{"approval", "list-by-admin", "--request", `{"processCode":"PROC","startTime":"2030-01-01 09:00:00","cursor":0}`},
			tool: "get_process_instances_by_admin",
		},
		{
			name: "list-by-admin request mode with endTime",
			args: []string{"approval", "list-by-admin", "--request", `{"processCode":"PROC","startTime":"2030-01-01 09:00:00","endTime":"2030-01-01 23:59:59","cursor":0,"pageSize":20}`},
			tool: "get_process_instances_by_admin",
		},
	}
	for _, tc := range validCases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{}
			if err := executeOACommand(t, caller, tc.args...); err != nil {
				t.Fatalf("execute %v: %v", tc.args, err)
			}
			if caller.tool != tc.tool || caller.calls != 1 {
				t.Fatalf("called tool=%q calls=%d, want %q once", caller.tool, caller.calls, tc.tool)
			}
		})
	}

	invalidCases := [][]string{
		{"approval", "form-schema"},
		{"approval", "forecast-process"},
		{"approval", "forecast-process", "--request", `{"processCode":"PROC"}`, "--process-code", "PROC"},
		{"approval", "forecast-process", "--request", "{"},
		{"approval", "forecast-process", "--request", "null"},
		{"approval", "forecast-process", "--request", "{} {}"},
		{"approval", "forecast-process", "--process-code", "PROC", "--dept-id", "bad", "--form-values", `{"金额":"100"}`},
		{"approval", "forecast-process", "--process-code", "PROC", "--dept-id", "-1", "--form-values", "["},
		{"approval", "create-instance", "--process-code", "PROC", "--form-values", `{}`},
		{"approval", "create-instance", "--yes"},
		{"approval", "create-instance", "--request", "{", "--yes"},
		{"approval", "create-instance", "--request", "null", "--yes"},
		{"approval", "create-instance", "--request", "{} {}", "--yes"},
		{"approval", "create-instance", "--process-code", "PROC", "--form-values", "[", "--yes"},
		{"approval", "create-instance", "--process-code", "PROC", "--form-values", `{}`, "--dept-id", "bad", "--yes"},
		{"approval", "create-instance", "--process-code", "PROC", "--form-values", `{}`, "--approvers", "u", "--approvers-action-type", "bad", "--yes"},
		{"approval", "create-instance", "--process-code", "PROC", "--form-values", `{}`, "--cc-list", "u", "--cc-position", "bad", "--yes"},
		{"approval", "list-by-admin"},
		{"approval", "list-by-admin", "--process-code", "PROC"},
		{"approval", "list-by-admin", "--process-code", "PROC", "--start", "bad"},
		{"approval", "list-by-admin", "--process-code", "PROC", "--start", "2030-01-01T10:00:00+08:00", "--end", "bad"},
		{"approval", "list-by-admin", "--process-code", "PROC", "--start", "2030-01-01T10:00:00+08:00", "--end", "2030-01-01T09:00:00+08:00"},
		{"approval", "list-by-admin", "--process-code", "PROC", "--start", "2030-01-01T09:00:00+08:00", "--cursor", "bad"},
		{"approval", "list-by-admin", "--process-code", "PROC", "--start", "2030-01-01T09:00:00+08:00", "--limit", "bad"},
		{"approval", "list-by-admin", "--process-code", "PROC", "--start", "2030-01-01T09:00:00+08:00", "--limit", "21"},
		{"approval", "list-by-admin", "--process-code", "PROC", "--start", "2030-01-01T09:00:00+08:00", "--limit", "0"},
		{"approval", "list-by-admin", "--process-code", "PROC", "--start", ""},
		{"approval", "list-by-admin", "--request", "{"},
		{"approval", "list-by-admin", "--request", "null"},
		{"approval", "list-by-admin", "--request", `{"processCode":"PROC"}`, "--process-code", "PROC"},
		{"approval", "list-by-admin", "--request", `{"processCode":"PROC","startTime":"2030-01-01 09:00:00","cursor":0,"pageSize":21}`},
		{"approval", "list-by-admin", "--request", `{"processCode":"PROC","startTime":"2030-01-01 09:00:00","cursor":0,"pageSize":"20"}`},
		{"approval", "list-by-admin", "--request", `{"processCode":"PROC","startTime":1893459600000,"cursor":0,"pageSize":20}`},
		{"approval", "list-by-admin", "--request", `{"processCode":"PROC","startTime":"2030-01-01","cursor":0,"pageSize":20}`},
		{"approval", "list-by-admin", "--request", `{"processCode":"PROC","startTime":"2030-01-01 10:00:00","endTime":"2030-01-01 09:00:00","cursor":0,"pageSize":20}`},
		{"approval", "list-by-admin", "--request", `{"processCode":"PROC","startTime":"2030-01-01 09:00:00","endTime":1893463200000,"cursor":0,"pageSize":20}`},
		{"approval", "list-by-admin", "--request", `{"processCode":"PROC","endTime":"NOT-A-TIME","cursor":0,"pageSize":20}`},
		{"approval", "list-by-admin", "--request", `{"processCode":"PROC","startTime":"2030-01-01 09:00:00","endTime":"2030-01-01","cursor":0,"pageSize":20}`},
		{"approval", "list-by-admin", "--request", `{"processCode":"PROC","startTime":"2030-01-01 09:00:00","endTime":"2030-01-01 09:00:00","cursor":0,"pageSize":20}`},
		{"approval", "list-by-admin", "--request", `{"startTime":"2030-01-01 09:00:00","cursor":0,"pageSize":20}`},
		{"approval", "list-by-admin", "--request", `{"processCode":"","startTime":"2030-01-01 09:00:00","cursor":0,"pageSize":20}`},
		{"approval", "list-by-admin", "--request", `{"processCode":123,"startTime":"2030-01-01 09:00:00","cursor":0,"pageSize":20}`},
	}
	for _, args := range invalidCases {
		caller := &scriptedToolCaller{}
		if err := executeOACommand(t, caller, args...); err == nil {
			t.Fatalf("invalid args %v returned nil", args)
		}
		if caller.calls != 0 {
			t.Fatalf("invalid args %v made %d MCP calls", args, caller.calls)
		}
	}
}

func TestCreateInstanceDenialMessage(t *testing.T) {
	if msg := createInstanceDenialMessage(nil); msg != "" {
		t.Fatalf("nil error → %q, want empty", msg)
	}
	if msg := createInstanceDenialMessage(errors.New("其他服务端错误")); msg != "" {
		t.Fatalf("unmatched error → %q, want empty", msg)
	}
	denial := &CLIError{
		Code:    CodeUnclassified,
		Message: "the supply check point has already been bound to an approval. Please do not submit again",
	}
	const want = "该卡点已补卡完成或有正在进行中的审批流程，请勿重复提交"
	if msg := createInstanceDenialMessage(denial); msg != want {
		t.Fatalf("matched error → %q, want %q", msg, want)
	}
}

func TestCrossPlatformCoverageOAApprovalCreateInstanceTranslatesSupplyPointDenial(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{
		err: &CLIError{
			Code:    CodeUnclassified,
			Message: "the supply check point has already been bound to an approval. Please do not submit again",
		},
	}}}
	err := executeOACommand(t, caller,
		"approval", "create-instance",
		"--process-code", "PROC",
		"--form-values", `{"事由":"测试"}`,
		"--yes",
	)
	if err == nil {
		t.Fatalf("create-instance should fail with the denial error")
	}
	const want = "该卡点已补卡完成或有正在进行中的审批流程，请勿重复提交"
	if err.Error() != want {
		t.Fatalf("error = %q, want %q", err.Error(), want)
	}
	if caller.calls != 1 {
		t.Fatalf("calls = %d, want 1", caller.calls)
	}
}

// U-gate: create-instance 未确认（无 --yes）时拒绝执行且不发起 MCP 调用；
// 用户确认后（--yes）才产生唯一的 start_process_instance 精确调用。
func TestCrossPlatformCoverageOAApprovalCreateInstanceConfirmationGate(t *testing.T) {
	unconfirmed := &scriptedToolCaller{}
	err := executeOACommand(t, unconfirmed,
		"approval", "create-instance",
		"--process-code", "PROC-1",
		"--form-values", `{"单行输入框":"测试"}`,
	)
	if err == nil {
		t.Fatal("create-instance without --yes returned nil")
	}
	if !strings.Contains(err.Error(), "--yes") {
		t.Fatalf("error = %q, want confirmation guidance mentioning --yes", err.Error())
	}
	if unconfirmed.calls != 0 {
		t.Fatalf("unconfirmed create-instance made %d MCP calls", unconfirmed.calls)
	}

	confirmed := &scriptedToolCaller{}
	if err := executeOACommand(t, confirmed,
		"approval", "create-instance",
		"--process-code", "PROC-1",
		"--form-values", `{"单行输入框":"测试"}`,
		"--yes",
	); err != nil {
		t.Fatalf("confirmed create-instance: %v", err)
	}
	if confirmed.server != "oa" || confirmed.tool != "start_process_instance" {
		t.Fatalf("called %s/%s, want oa/start_process_instance", confirmed.server, confirmed.tool)
	}
	if confirmed.calls != 1 {
		t.Fatalf("calls = %d, want exactly one confirmed call", confirmed.calls)
	}
	if len(confirmed.args) != 1 {
		t.Fatalf("args = %#v, want only ProcessInstanceCreationPopRequest", confirmed.args)
	}
	inner, ok := confirmed.args["ProcessInstanceCreationPopRequest"].(map[string]any)
	if !ok {
		t.Fatalf("args = %#v, want ProcessInstanceCreationPopRequest object", confirmed.args)
	}
	if inner["processCode"] != "PROC-1" {
		t.Fatalf("processCode = %#v", inner["processCode"])
	}
	values, ok := inner["formComponentValues"].([]map[string]string)
	if !ok || len(values) != 1 || values[0]["name"] != "单行输入框" || values[0]["value"] != "测试" {
		t.Fatalf("formComponentValues = %#v", inner["formComponentValues"])
	}
}
