package helpers

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// U1: supply-plans 必填参数缺失 / 时间格式预检失败时不发起 MCP 调用。
func TestCrossPlatformCoverageAttendanceSupplyPlansRequiresFlags(t *testing.T) {
	cases := [][]string{
		{"approve", "supply-plans"},
		{"approve", "supply-plans", "--time", "bad-date"},
		{"approve", "supply-plans", "--time", "2026-08-05"},
		{"approve", "supply-plans", "--time", "2026-08-05 04"},
		{"approve", "supply-plans", "--time", "08-05 04:00"},
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

// U2: 时间格式预检的错误消息携带规范的 --flag 名称与目标格式。
func TestCrossPlatformCoverageAttendanceSupplyPlansTimeFormatMessage(t *testing.T) {
	caller := &scriptedToolCaller{}
	err := executeAttendanceApproveCommand(t, caller, nil,
		"approve", "supply-plans",
		"--time", "2026-08-05",
	)
	if err == nil {
		t.Fatal("date-only --time returned nil")
	}
	if !strings.Contains(err.Error(), "--time 时间格式不正确，应为 yyyy-MM-dd HH:mm") || strings.Contains(err.Error(), "----time") {
		t.Fatalf("error = %q", err.Error())
	}
	if caller.calls != 0 {
		t.Fatalf("made %d MCP calls", caller.calls)
	}
}

// U1b: supply-plans 参数映射（--time/--user → supplyTimestampMs/userId）与响应原样透传。
func TestCrossPlatformCoverageAttendanceSupplyPlansMapsRequestAndPassesThrough(t *testing.T) {
	payload := `{"plans":[{"planId":"PLAN-1","planTip":"周二 ( 08.04 下班) 补卡","planText":"2026-08-05 10:39","workDate":1785772800000,"supplyDate":1785873600000}]}`
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: payload}}}
	var out bytes.Buffer
	if err := executeAttendanceApproveCommand(t, caller, &out,
		"approve", "supply-plans",
		"--time", "2026-08-05 04:00",
		"--user", "staff-1",
	); err != nil {
		t.Fatalf("supply-plans: %v", err)
	}
	if caller.server != "attendance-wukong" || caller.tool != "match_plans_for_supply" {
		t.Fatalf("called %s/%s, want attendance-wukong/match_plans_for_supply", caller.server, caller.tool)
	}
	if got, ok := caller.args["supplyTimestampMs"].(int64); !ok || got != 1785873600000 {
		t.Fatalf("supplyTimestampMs = %#v (type %T)", caller.args["supplyTimestampMs"], caller.args["supplyTimestampMs"])
	}
	if got := caller.args["userId"]; got != "staff-1" {
		t.Fatalf("userId = %#v", got)
	}
	got := out.String()
	for _, want := range []string{"plans", "planTip", "supplyDate", "1785873600000"} {
		if !strings.Contains(got, want) {
			t.Fatalf("output missing %q: %s", want, got)
		}
	}
}

// U1c: supply-plans 空班次列表属正常业务结果，原样透传不报错。
func TestCrossPlatformCoverageAttendanceSupplyPlansEmptyListPassesThrough(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"plans":[]}`}}}
	var out bytes.Buffer
	if err := executeAttendanceApproveCommand(t, caller, &out,
		"approve", "supply-plans",
		"--time", "2026-08-05 04:00",
	); err != nil {
		t.Fatalf("empty plans should pass through: %v", err)
	}
	if !strings.Contains(out.String(), "plans") {
		t.Fatalf("output = %s", out.String())
	}
}

// U1d: supply-plans MCP 业务错误透传为非零退出。
func TestCrossPlatformCoverageAttendanceSupplyPlansBusinessError(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("考勤服务不可用")}}}
	if err := executeAttendanceApproveCommand(t, caller, nil,
		"approve", "supply-plans",
		"--time", "2026-08-05 04:00",
	); err == nil {
		t.Fatal("business error returned nil")
	}
	if caller.calls != 1 {
		t.Fatalf("calls = %d", caller.calls)
	}
}

// U1e: supply-plans 正则预检通过但日历日期非法（如 2 月 30 日）时，
// parseSupplyTimeMillis 仍拒绝且不发起 MCP 调用。
func TestCrossPlatformCoverageAttendanceSupplyPlansInvalidCalendarDate(t *testing.T) {
	caller := &scriptedToolCaller{}
	err := executeAttendanceApproveCommand(t, caller, nil,
		"approve", "supply-plans",
		"--time", "2026-02-30 10:00",
	)
	if err == nil {
		t.Fatal("invalid calendar date returned nil")
	}
	if !strings.Contains(err.Error(), "--time 时间格式不正确，应为 yyyy-MM-dd HH:mm") {
		t.Fatalf("error = %q", err.Error())
	}
	if caller.calls != 0 {
		t.Fatalf("made %d MCP calls", caller.calls)
	}
}

// U3/U5: supply-check 必填参数缺失时不发起 MCP 调用。
func TestCrossPlatformCoverageAttendanceSupplyCheckRequiresFlags(t *testing.T) {
	caller := &scriptedToolCaller{}
	if err := executeAttendanceApproveCommand(t, caller, nil, "approve", "supply-check"); err == nil {
		t.Fatal("missing --timestamp returned nil")
	}
	if caller.calls != 0 {
		t.Fatalf("made %d MCP calls", caller.calls)
	}
}

// U3b: supply-check qualify=true 原样输出 + 参数映射（--timestamp int64 / --user）。
func TestCrossPlatformCoverageAttendanceSupplyCheckQualifyTrueAndMapping(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"qualify":true}`}}}
	var out bytes.Buffer
	if err := executeAttendanceApproveCommand(t, caller, &out,
		"approve", "supply-check",
		"--timestamp", "1785873600000",
		"--user", "staff-1",
	); err != nil {
		t.Fatalf("supply-check: %v", err)
	}
	if !strings.Contains(out.String(), "qualify") {
		t.Fatalf("output = %s", out.String())
	}
	if caller.tool != "check_supply_qualification" || caller.server != "attendance-wukong" {
		t.Fatalf("called %s/%s", caller.server, caller.tool)
	}
	if got, ok := caller.args["supplyTimestampMs"].(int64); !ok || got != 1785873600000 {
		t.Fatalf("supplyTimestampMs = %#v (type %T)", caller.args["supplyTimestampMs"], caller.args["supplyTimestampMs"])
	}
	if got := caller.args["userId"]; got != "staff-1" {
		t.Fatalf("userId = %#v", got)
	}
}

// U4: supply-check qualify=false → 非零退出 + title/desc 原样转告
// （顶层与 result 嵌套两种形态；title/desc 组合缺一亦可）。
func TestCrossPlatformCoverageAttendanceSupplyCheckQualifyFalsePath(t *testing.T) {
	cases := []struct {
		name    string
		payload string
		wantMsg string
	}{
		{
			name:    "flat title and desc",
			payload: `{"qualify":false,"title":"无法补卡","desc":"已超过允许补卡的期限（30 天）"}`,
			wantMsg: "无法补卡: 已超过允许补卡的期限（30 天）",
		},
		{
			name:    "result-wrapped desc only",
			payload: `{"result":{"qualify":false,"desc":"本月补卡次数已用完"}}`,
			wantMsg: "本月补卡次数已用完",
		},
		{
			name:    "flat no reason",
			payload: `{"qualify":false}`,
			wantMsg: "补卡资格校验未通过（服务端未返回具体原因）",
		},
		{
			name:    "flat title only",
			payload: `{"qualify":false,"title":"无法补卡"}`,
			wantMsg: "无法补卡",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: tc.payload}}}
			err := executeAttendanceApproveCommand(t, caller, nil,
				"approve", "supply-check",
				"--timestamp", "1785873600000",
			)
			if err == nil {
				t.Fatal("qualify=false returned nil")
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

// dry-run 预览：supply-check 不实际调用 MCP。
func TestCrossPlatformCoverageAttendanceSupplyCheckDryRunPreview(t *testing.T) {
	caller := &scriptedToolCaller{dry: true}
	if err := executeAttendanceApproveCommand(t, caller, nil,
		"approve", "supply-check",
		"--timestamp", "1785873600000",
	); err != nil {
		t.Fatalf("dry-run preview: %v", err)
	}
	if caller.calls != 0 {
		t.Fatalf("dry-run made %d MCP calls", caller.calls)
	}
}

// U6b: supply-check 响应携带 success=false 时会被统一错误分类拦截为
// MCP 业务错误；此处须从拦截后的错误中还原 errorMsg 原样返回（不包装）。
func TestCrossPlatformCoverageAttendanceSupplyCheckBusinessErrorRestoresErrorMsg(t *testing.T) {
	caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":false,"errorCode":"MOCK_SUPPLY","errorMsg":"服务端拒绝补卡"}`}}}
	err := executeAttendanceApproveCommand(t, caller, nil,
		"approve", "supply-check",
		"--timestamp", "1785873600000",
	)
	if err == nil {
		t.Fatal("business error returned nil")
	}
	if err.Error() != "服务端拒绝补卡" {
		t.Fatalf("error = %q, want 服务端拒绝补卡", err.Error())
	}
	if caller.calls != 1 {
		t.Fatalf("calls = %d", caller.calls)
	}
}
