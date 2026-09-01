package helpers

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

// executeAttendanceApproveCommand mirrors executeOACommand but allows capturing stdout
// (the leave/supply suite commands assert passthrough output in addition to the
// scripted MCP call recording).
func executeAttendanceApproveCommand(t *testing.T, caller *scriptedToolCaller, out io.Writer, args ...string) error {
	t.Helper()
	previous := deps
	previousArgs := os.Args
	os.Args = []string{"dws", "attendance"}
	InitDeps(caller)
	if out == nil {
		out = io.Discard
	}
	deps.Out.w = out
	deps.Out.errW = io.Discard
	t.Cleanup(func() {
		deps = previous
		os.Args = previousArgs
	})

	cmd := newAttendanceCommand()
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	return cmd.Execute()
}

// query_leave_types_with_balance：不传 --user 时查询当前用户，且响应原样透传。
func TestCrossPlatformCoverageAttendanceLeaveTypesCurrentUserPassesThrough(t *testing.T) {
	payload := `{"leaveTypes":[{"leaveCode":"annual","leaveName":"年假","leaveViewUnit":"DAY","balanceHidden":false,"balance":{"remainQuota":7.5,"quotaUnit":"day"}},{"leaveCode":"sick","leaveName":"病假","balanceHidden":true,"balance":null}]}`
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: payload}}}
	var out bytes.Buffer
	if err := executeAttendanceApproveCommand(t, caller, &out,
		"approve", "leave-types",
	); err != nil {
		t.Fatalf("leave-types: %v", err)
	}
	if caller.server != "attendance-wukong" || caller.tool != "query_leave_types_with_balance" {
		t.Fatalf("called %s/%s, want attendance-wukong/query_leave_types_with_balance", caller.server, caller.tool)
	}
	if len(caller.args) != 0 {
		t.Fatalf("args = %#v, want empty request for current user", caller.args)
	}
	for _, want := range []string{"leaveCode", "annual", "balanceHidden", "remainQuota"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("output missing %q: %s", want, out.String())
		}
	}
}

// --user 一对一映射为 MCP 的 staffId；不得套旧请求对象或改名为 userId。
func TestCrossPlatformCoverageAttendanceLeaveTypesMapsStaffID(t *testing.T) {
	caller := &scriptedToolCaller{}
	if err := executeAttendanceApproveCommand(t, caller, nil,
		"approve", "leave-types", "--user", "staff-1",
	); err != nil {
		t.Fatalf("leave-types --user: %v", err)
	}
	if got := caller.args["staffId"]; got != "staff-1" {
		t.Fatalf("staffId = %#v", got)
	}
	if _, exists := caller.args["userId"]; exists {
		t.Fatalf("unexpected userId: %#v", caller.args)
	}
	if _, exists := caller.args["McpLeaveTypeBalanceRequest"]; exists {
		t.Fatalf("unexpected nested request: %#v", caller.args)
	}
}

// U1: leave-duration 必填参数缺失 / 时间格式预检失败时不发起 MCP 调用。
func TestCrossPlatformCoverageAttendanceLeaveDurationRequiresFlags(t *testing.T) {
	cases := [][]string{
		{"approve", "leave-duration"},
		{"approve", "leave-duration", "--leave-code", "lc"},
		{"approve", "leave-duration", "--leave-code", "lc", "--start", "2026-08-13 09:00"},
		{"approve", "leave-duration", "--leave-code", "lc", "--start", "bad-date", "--end", "2026-08-14"},
		{"approve", "leave-duration", "--leave-code", "lc", "--start", "2026-08-13", "--end", "08-14"},
	}
	for _, args := range cases {
		caller := &scriptedToolCaller{}
		if err := executeAttendanceApproveCommand(t, caller, nil, args...); err == nil {
			t.Fatalf("args %v returned nil", args)
		}
		if caller.calls != 0 {
			t.Fatalf("args %v made %d MCP calls", args, caller.calls)
		}
	}
}

// U1b: 时间格式预检的错误消息携带规范的 --flag 名称（回归：曾出现 ----start 双横线）。
func TestCrossPlatformCoverageAttendanceLeaveTimePrefixMessage(t *testing.T) {
	caller := &scriptedToolCaller{}
	err := executeAttendanceApproveCommand(t, caller, nil,
		"approve", "leave-duration",
		"--leave-code", "lc",
		"--start", "bad-date",
		"--end", "2026-08-14",
	)
	if err == nil {
		t.Fatal("bad --start returned nil")
	}
	if !strings.Contains(err.Error(), "--start 时间格式不正确") || strings.Contains(err.Error(), "----start") {
		t.Fatalf("error = %q", err.Error())
	}
	if caller.calls != 0 {
		t.Fatalf("made %d MCP calls", caller.calls)
	}
}

// U2: leave-duration 参数映射（--leave-code/--start/--end/--user → leaveCode/fromDate/toDate/staffId）。
func TestCrossPlatformCoverageAttendanceLeaveDurationMapsRequest(t *testing.T) {
	caller := &scriptedToolCaller{}
	if err := executeAttendanceApproveCommand(t, caller, nil,
		"approve", "leave-duration",
		"--leave-code", "lc-1",
		"--start", "2026-08-13 12:08",
		"--end", "2026-08-14 18:08",
		"--user", "staff-1",
	); err != nil {
		t.Fatalf("leave-duration: %v", err)
	}
	if caller.server != "attendance-wukong" || caller.tool != "get_leave_time" {
		t.Fatalf("called %s/%s, want attendance-wukong/get_leave_time", caller.server, caller.tool)
	}
	if got := caller.args["leaveCode"]; got != "lc-1" {
		t.Fatalf("leaveCode = %#v", got)
	}
	if got := caller.args["fromDate"]; got != "2026-08-13 12:08" {
		t.Fatalf("fromDate = %#v", got)
	}
	if got := caller.args["toDate"]; got != "2026-08-14 18:08" {
		t.Fatalf("toDate = %#v", got)
	}
	if got := caller.args["staffId"]; got != "staff-1" {
		t.Fatalf("staffId = %#v", got)
	}
}

// U3: leave-duration 响应原样透传（无裁剪无改名，含 corpId 回显）。
func TestCrossPlatformCoverageAttendanceLeaveDurationPassesResponseThrough(t *testing.T) {
	payload := `{"durationInHour":14.87,"durationInDay":1.65,"detailList":[{"workDate":"2026-08-13"}],"compressedValue":"1f8b...","corpId":"ding-xxx","unit":"HOUR"}`
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: payload}}}
	var out bytes.Buffer
	if err := executeAttendanceApproveCommand(t, caller, &out,
		"approve", "leave-duration",
		"--leave-code", "lc-1",
		"--start", "2026-08-13 12:08",
		"--end", "2026-08-14 18:08",
	); err != nil {
		t.Fatalf("leave-duration passthrough: %v", err)
	}
	got := out.String()
	for _, want := range []string{"durationInHour", "14.87", "detailList", "compressedValue", "corpId"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %s", want, got)
		}
	}
}

// U4: leave-duration MCP 业务错误透传为非零退出。
func TestCrossPlatformCoverageAttendanceLeaveDurationBusinessError(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("leaveCode 无效")}}}
	if err := executeAttendanceApproveCommand(t, caller, nil,
		"approve", "leave-duration",
		"--leave-code", "bad",
		"--start", "2026-08-13",
		"--end", "2026-08-14",
	); err == nil {
		t.Fatal("business error returned nil")
	}
	if caller.calls != 1 {
		t.Fatalf("calls = %d", caller.calls)
	}
}

// U6: leave-check 必填参数缺失（含 number flags 的 Changed 检查）时不发起 MCP 调用。
func TestCrossPlatformCoverageAttendanceLeaveCheckRequiresFlags(t *testing.T) {
	cases := [][]string{
		{"approve", "leave-check"},
		{"approve", "leave-check", "--leave-code", "lc"},
		{"approve", "leave-check", "--leave-code", "lc", "--process-code", "P"},
		{"approve", "leave-check", "--leave-code", "lc", "--process-code", "P", "--start", "2026-08-13"},
		{"approve", "leave-check", "--leave-code", "lc", "--process-code", "P", "--start", "2026-08-13", "--end", "bad"},
		{"approve", "leave-check", "--leave-code", "lc", "--process-code", "P", "--start", "2026-08-13", "--end", "2026-08-14"},
	}
	for _, args := range cases {
		caller := &scriptedToolCaller{}
		if err := executeAttendanceApproveCommand(t, caller, nil, args...); err == nil {
			t.Fatalf("args %v returned nil", args)
		}
		if caller.calls != 0 {
			t.Fatalf("args %v made %d MCP calls", args, caller.calls)
		}
	}
}

// U7: leave-check success=true 原样输出 + 参数映射（number 时长、可选 procInstId/staffId）。
func TestCrossPlatformCoverageAttendanceLeaveCheckSuccessAndMapping(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true}`}}}
	var out bytes.Buffer
	if err := executeAttendanceApproveCommand(t, caller, &out,
		"approve", "leave-check",
		"--leave-code", "lc-1",
		"--process-code", "PROC-1",
		"--start", "2026-08-13 12:08",
		"--end", "2026-08-14 18:08",
		"--duration-day", "1.65",
		"--duration-hour", "14.87",
		"--user", "staff-1",
		"--proc-inst-id", "inst-1",
	); err != nil {
		t.Fatalf("leave-check: %v", err)
	}
	if !strings.Contains(out.String(), "success") {
		t.Fatalf("output = %s", out.String())
	}
	if caller.tool != "can_leave_check" || caller.server != "attendance-wukong" {
		t.Fatalf("called %s/%s", caller.server, caller.tool)
	}
	if got := caller.args["durationInDay"]; got != 1.65 {
		t.Fatalf("durationInDay = %#v (type %T)", got, got)
	}
	if got := caller.args["durationInHour"]; got != 14.87 {
		t.Fatalf("durationInHour = %#v", got)
	}
	if got := caller.args["procInstId"]; got != "inst-1" {
		t.Fatalf("procInstId = %#v", got)
	}
	if got := caller.args["staffId"]; got != "staff-1" {
		t.Fatalf("staffId = %#v", got)
	}
}

// U8: leave-check success=false → 非零退出 + errorMsg 原样输出（顶层与 result 嵌套两种形态）。
func TestCrossPlatformCoverageAttendanceLeaveCheckFailurePath(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantMsg string
	}{
		{
			name:    "flat success false",
			payload: `{"success":false,"errorCode":"MOCK_CONFLICT","errorMsg":"mock：时间段冲突"}`,
			wantMsg: "mock：时间段冲突",
		},
		{
			name:    "result-wrapped success false",
			payload: `{"result":{"success":false,"errorCode":"QUOTA","errorMsg":"额度不足"}}`,
			wantMsg: "额度不足",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: tc.payload}}}
			err := executeAttendanceApproveCommand(t, caller, nil,
				"approve", "leave-check",
				"--leave-code", "lc-1",
				"--process-code", "PROC-1",
				"--start", "2026-08-13 12:08",
				"--end", "2026-08-14 18:08",
				"--duration-day", "1.65",
				"--duration-hour", "14.87",
			)
			if err == nil {
				t.Fatal("success=false returned nil")
			}
			if err.Error() != tc.wantMsg {
				t.Fatalf("error = %q, want %q", err.Error(), tc.wantMsg)
			}
			if caller.calls != 1 {
				t.Fatalf("calls = %d", caller.calls)
			}
		})
	}
}

// U8b: leave-check success=false 但服务端未返回具体原因时，
// 返回标准兜底文案而非空错误。
func TestCrossPlatformCoverageAttendanceLeaveCheckFailureNoReason(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":false}`}}}
	err := executeAttendanceApproveCommand(t, caller, nil,
		"approve", "leave-check",
		"--leave-code", "lc-1",
		"--process-code", "PROC-1",
		"--start", "2026-08-13 12:08",
		"--end", "2026-08-14 18:08",
		"--duration-day", "1.65",
		"--duration-hour", "14.87",
	)
	if err == nil {
		t.Fatal("success=false without reason returned nil")
	}
	if err.Error() != "请假校验未通过（服务端未返回具体原因）" {
		t.Fatalf("error = %q, want 请假校验未通过（服务端未返回具体原因）", err.Error())
	}
	if caller.calls != 1 {
		t.Fatalf("calls = %d", caller.calls)
	}
}

// U8c: leaveCheckErrorMessage 只还原 CodeMCPToolError 分类且消息体
// 可解析出失败语义的错误；其它错误一律返回空串交由原样返回。
func TestCrossPlatformCoverageAttendanceLeaveCheckErrorMessageClassifiesOnlyMCPToolError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "plain error",
			err:  errors.New("network unreachable"),
			want: "",
		},
		{
			name: "non-MCP tool CLIError",
			err:  &CLIError{Code: CodeAuthNotConfigured, Message: `{"success":false,"errorMsg":"x"}`},
			want: "",
		},
		{
			name: "MCP tool CLIError with non-JSON message",
			err:  &CLIError{Code: CodeMCPToolError, Message: "not-json"},
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := leaveCheckErrorMessage(tc.err); got != tc.want {
				t.Fatalf("leaveCheckErrorMessage(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// dry-run 预览：leave-check 不实际调用 MCP。
func TestCrossPlatformCoverageAttendanceLeaveCheckDryRunPreview(t *testing.T) {
	caller := &scriptedToolCaller{dry: true}
	if err := executeAttendanceApproveCommand(t, caller, nil,
		"approve", "leave-check",
		"--leave-code", "lc-1",
		"--process-code", "PROC-1",
		"--start", "2026-08-13 12:08",
		"--end", "2026-08-14 18:08",
		"--duration-day", "1.65",
		"--duration-hour", "14.87",
	); err != nil {
		t.Fatalf("dry-run preview: %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("dry-run made %d MCP calls", caller.calls)
	}
}
