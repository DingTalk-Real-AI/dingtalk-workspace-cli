package helpers

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

type attendanceFailWriter struct{}

func (attendanceFailWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

type attendanceRecordedCall struct {
	server string
	tool   string
	args   map[string]any
}

type attendanceRecordingCaller struct {
	text  string
	err   error
	calls []attendanceRecordedCall
}

func (c *attendanceRecordingCaller) CallTool(_ context.Context, server, tool string, args map[string]any) (*edition.ToolResult, error) {
	c.calls = append(c.calls, attendanceRecordedCall{server: server, tool: tool, args: args})
	if c.err != nil {
		return nil, c.err
	}
	return textToolResult(c.text), nil
}

func (*attendanceRecordingCaller) Format() string { return "" }
func (*attendanceRecordingCaller) DryRun() bool   { return false }
func (*attendanceRecordingCaller) Fields() string { return "" }
func (*attendanceRecordingCaller) JQ() string     { return "" }

func runAttendanceCoverageCommand(t *testing.T, caller edition.ToolCaller, args ...string) error {
	t.Helper()
	InitDeps(caller)
	deps.Out.w = io.Discard
	deps.Out.errW = io.Discard
	root := newAttendanceCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	return root.ExecuteContext(context.Background())
}

func runAttendanceRecordingCommand(t *testing.T, caller *attendanceRecordingCaller, args ...string) (string, error) {
	t.Helper()
	InitDeps(caller)
	var output bytes.Buffer
	deps.Out.w = &output
	deps.Out.errW = io.Discard
	root := newAttendanceCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors = true
	root.SilenceUsage = true
	root.SetOut(io.Discard)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return output.String(), err
}

func assertAttendanceToolCall(t *testing.T, caller *attendanceRecordingCaller, wantTool string, wantArgs map[string]any) {
	t.Helper()
	if len(caller.calls) != 1 {
		t.Fatalf("tool calls = %d, want 1", len(caller.calls))
	}
	call := caller.calls[0]
	if call.server != "attendance-wukong" || call.tool != wantTool {
		t.Fatalf("tool call = %s/%s, want attendance-wukong/%s", call.server, call.tool, wantTool)
	}
	if !reflect.DeepEqual(call.args, wantArgs) {
		t.Fatalf("tool args = %#v, want %#v", call.args, wantArgs)
	}
}

func runAttendanceCoverageDirect(t *testing.T, path []string, flags map[string]string) error {
	t.Helper()
	command, _, err := newAttendanceCommand().Find(path)
	if err != nil {
		return err
	}
	for name, value := range flags {
		if err := command.Flags().Set(name, value); err != nil {
			return err
		}
	}
	return command.RunE(command, nil)
}

func attendanceInput(t *testing.T, text string) *os.File {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stdin")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file
}

func TestCrossPlatformCoverageAttendanceCommandEdgeValidation(t *testing.T) {
	previousDeps, previousArgs, previousStdin := deps, os.Args, os.Stdin
	os.Args = []string{"dws", "attendance"}
	InitDeps(&productExampleCaller{})
	deps.Out.w, deps.Out.errW = io.Discard, io.Discard
	t.Cleanup(func() { deps, os.Args, os.Stdin = previousDeps, previousArgs, previousStdin })

	users := strings.TrimSuffix(strings.Repeat("u,", 101), ",")
	commands := [][]string{
		{"check", "result", "--users=" + users, "--start=2026-01-01", "--end=2026-01-02"},
		{"check", "result", "--users=u", "--start=2026-01-01", "--end=2026-01-02", "--offset=-1"},
		{"check", "result", "--users=u", "--start=2026-01-01", "--end=2026-01-02", "--limit=0"},
		{"report", "query-data", "--users=u", "--columns=1,,2", "--start=2026-01-01 00:00:00", "--end=2026-01-02 00:00:00"},
		{"schedule", "get", "--users=u", "--start=2026-01-01", "--end=bad"},
		{"schedule", "get", "--users=u", "--start=2026-01-01", "--end=2026-01-02"},
		{"checkin", "records", "--operator-corp-id=c", "--operator-staff-id=o", "--staff-ids=,", "--start=2026-01-01 00:00:00", "--end=2026-01-02 00:00:00"},
		{"checkin", "records", "--operator-corp-id=c", "--operator-staff-id=o", "--staff-ids=u", "--start=2026-01-02 00:00:00", "--end=2026-01-01 00:00:00"},
	}
	for _, args := range commands {
		_ = runAttendanceCoverageCommand(t, &productExampleCaller{}, args...)
	}
	for _, tc := range []struct {
		path  []string
		flags map[string]string
	}{
		{[]string{"approve", "list"}, map[string]string{"users": "u", "types": "leave", "start": "2026-01-01"}},
		{[]string{"group", "create"}, map[string]string{"type": "TURN"}},
	} {
		_ = runAttendanceCoverageDirect(t, tc.path, tc.flags)
	}
}

func TestCrossPlatformCoverageAttendanceConfirmedMutationEdges(t *testing.T) {
	previousDeps, previousArgs, previousStdin := deps, os.Args, os.Stdin
	os.Args = []string{"dws", "attendance"}
	t.Cleanup(func() { deps, os.Args, os.Stdin = previousDeps, previousArgs, previousStdin })

	validClass := `{"sections":[]}`
	validFixed := `{"workDayClassList":[1],"defaultClassId":1}`
	mutations := [][]string{
		{"class", "create", "--name=shift", "--class-vo=" + validClass},
		{"class", "update", "--class-id=1", "--name=shift"},
		{"group", "update-members", "--group-id=1", "--add-users=u"},
		{"group", "create", "--name=group", "--type=TURN", "--owner=u"},
		{"group", "update", "--group-id=1", "--name=group", "--enable-outside-check=true", "--classIds=[1]"},
		{"selfsetting", "save", "--setting-scene=checkResultNotify", "--user=u", "--check-result-msg=1"},
		{"globalsetting", "save", "--setting-scene=checkRemind", "--scope=企业", "--check-remind-corp=true"},
		{"vacation", "update-type", "--leave-code=leave", "--name=name"},
		{"vacation", "save-balance", "--target=u", "--leave-code=leave", "--num=1", "--reason=reason"},
		{"schedule", "import", "--groupId=1", `--scheduleVOS=[{"userId":"u","workDate":"2026-01-01","classId":1,"isRest":false}]`},
		{"boss-check", "--result-id=1", "--time=2026-01-01 09:00", "--result=Normal", "--absent-min=1", "--remark=ok"},
	}
	for _, args := range mutations {
		for _, input := range []string{"", "no\n"} {
			os.Stdin = attendanceInput(t, input)
			_ = runAttendanceCoverageCommand(t, &productExampleCaller{}, args...)
		}
	}

	for _, args := range [][]string{
		{"group", "create", "--name=group", "--type=FIXED", "--group-vo={}"},
		{"group", "create", "--name=group", "--type=FIXED", "--group-vo={\"workDayClassList\":[]}"},
		{"group", "create", "--name=group", "--type=FIXED", "--group-vo={\"workDayClassList\":[1]}"},
		{"group", "create", "--name=group", "--type=FIXED", "--group-vo=" + validFixed, "--yes"},
		{"globalsetting", "save", "--setting-scene=checkRemind", "--scope=企业", "--fast-check-corp=true", "--yes"},
		{"group", "update", "--group-id=1", "--enable-outside-check=true", "--classIds=[1]", "--yes"},
	} {
		_ = runAttendanceCoverageCommand(t, &productExampleCaller{}, args...)
	}
	_ = runAttendanceCoverageCommand(t, &productExampleCaller{dry: true}, "group", "create", "--name=group", "--type=TURN", "--yes")

	InitDeps(&productExampleCaller{})
	deps.Out.w, deps.Out.errW = attendanceFailWriter{}, io.Discard
	root := newAttendanceCommand()
	installExampleGlobalFlags(root)
	root.SilenceErrors, root.SilenceUsage = true, true
	root.SetArgs([]string{"group", "create", "--name=group", "--type=TURN", "--yes"})
	_ = root.Execute()
}

func TestCrossPlatformCoverageAttendanceConfirmedMutationSuccessCoverage(t *testing.T) {
	previousDeps, previousArgs := deps, os.Args
	os.Args = []string{"dws", "attendance"}
	t.Cleanup(func() { deps, os.Args = previousDeps, previousArgs })
	startTime := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.Local).UnixMilli()
	endTime := time.Date(2026, time.December, 31, 23, 59, 59, 0, time.Local).UnixMilli()

	for _, tc := range []struct {
		name       string
		response   string
		args       []string
		wantTool   string
		wantArgs   map[string]any
		wantOutput string
	}{
		{
			name:       "class create",
			response:   `{}`,
			args:       []string{"class", "create", "--name=shift", `--class-vo={"sections":[]}`, "--yes"},
			wantTool:   "create_class_setting",
			wantArgs:   map[string]any{"TopAtClassVO": map[string]any{"name": "shift", "sections": []any{}}},
			wantOutput: "{}\n",
		},
		{
			name:       "group create raw response",
			response:   "created",
			args:       []string{"group", "create", "--name=group", "--type=TURN", "--yes"},
			wantTool:   "create_group_setting",
			wantArgs:   map[string]any{"groupVO": map[string]any{"name": "group", "type": "TURN"}},
			wantOutput: "created\n",
		},
		{
			name:       "self setting save",
			response:   `{}`,
			args:       []string{"selfsetting", "save", "--setting-scene=checkResultNotify", "--user=u", "--check-result-msg=1", "--yes"},
			wantTool:   "save_self_setting",
			wantArgs:   map[string]any{"RuleMcpSaveSelfSettingRequest": map[string]any{"settingScene": "checkResultNotify", "userId": "u", "checkResultMsg": 1}},
			wantOutput: "{}\n",
		},
		{
			name:       "vacation balance",
			response:   `{}`,
			args:       []string{"vacation", "save-balance", "--target=u", "--leave-code=leave", "--num=7.5", "--reason=grant", "--yes"},
			wantTool:   "update_leave_balance",
			wantArgs:   map[string]any{"McpUpdateLeaveBalanceRequest": map[string]any{"targetUserId": "u", "leaveCode": "leave", "quotaNum": "750", "reason": "grant"}},
			wantOutput: "{}\n",
		},
		{
			name:       "vacation balance dates",
			response:   `{}`,
			args:       []string{"vacation", "save-balance", "--target=u", "--leave-code=leave", "--num=8", "--reason=grant", "--start=2026-01-01", "--end=2026-12-31", "--yes"},
			wantTool:   "update_leave_balance",
			wantArgs:   map[string]any{"McpUpdateLeaveBalanceRequest": map[string]any{"targetUserId": "u", "leaveCode": "leave", "quotaNum": "800", "reason": "grant", "startTime": startTime, "endTime": endTime}},
			wantOutput: "{}\n",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			caller := &attendanceRecordingCaller{text: tc.response}
			output, err := runAttendanceRecordingCommand(t, caller, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			assertAttendanceToolCall(t, caller, tc.wantTool, tc.wantArgs)
			if output != tc.wantOutput {
				t.Fatalf("output = %q, want %q", output, tc.wantOutput)
			}
		})
	}

	boom := errors.New("create group failed")
	groupCaller := &attendanceRecordingCaller{err: boom}
	if _, err := runAttendanceRecordingCommand(t, groupCaller, "group", "create", "--name=group", "--type=TURN", "--yes"); !errors.Is(err, boom) {
		t.Fatalf("group create error = %v, want %v", err, boom)
	}
	assertAttendanceToolCall(t, groupCaller, "create_group_setting", map[string]any{"groupVO": map[string]any{"name": "group", "type": "TURN"}})

	for _, tc := range []struct {
		name      string
		flags     []string
		wantError string
	}{
		{name: "invalid start", flags: []string{"--start=invalid"}, wantError: "invalid --startTime format"},
		{name: "invalid end", flags: []string{"--start=2026-01-01", "--end=invalid"}, wantError: "invalid --endTime format"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			args := []string{"vacation", "save-balance", "--target=u", "--leave-code=leave", "--num=8", "--reason=grant", "--yes"}
			args = append(args, tc.flags...)
			caller := &attendanceRecordingCaller{}
			_, err := runAttendanceRecordingCommand(t, caller, args...)
			if err == nil || !strings.Contains(err.Error(), tc.wantError) {
				t.Fatalf("vacation balance error = %v, want %q", err, tc.wantError)
			}
			if len(caller.calls) != 0 {
				t.Fatalf("tool calls = %d, want 0", len(caller.calls))
			}
		})
	}
}

func TestCrossPlatformCoverageAttendanceVacationAndScheduleEdges(t *testing.T) {
	previousDeps, previousArgs, previousStdin := deps, os.Args, os.Stdin
	os.Args = []string{"dws", "attendance"}
	t.Cleanup(func() { deps, os.Args, os.Stdin = previousDeps, previousArgs, previousStdin })

	for _, rules := range []string{
		`[]`,
		`[{},{"type":"dept","visible":[]}]`,
		`[{"type":"other","visible":["x"]},{"type":"dept","visible":[]}]`,
		`[{"visible":["x"]},{"type":"dept","visible":["1"]}]`,
		`[{"type":"dept","visible":["-1"]},{"type":"dept","visible":["2"]}]`,
	} {
		_ = runAttendanceCoverageCommand(t, &productExampleCaller{}, "vacation", "update-type", "--leave-code=leave", "--visibility-rules="+rules, "--paid=false", "--unit=day", "--user-say-yes")
	}

	for _, schedules := range []string{
		`[]`,
		`[{}]`,
		`[{"userId":"u","workDate":"bad","classId":1,"isRest":false}]`,
		`[{"userId":"u","workDate":"2026-01-01","classId":1,"isRest":false}]`,
	} {
		_ = runAttendanceCoverageCommand(t, &productExampleCaller{}, "schedule", "import", "--groupId=bad", "--scheduleVOS="+schedules, "--user-say-yes")
	}
	_ = runAttendanceCoverageCommand(t, &productExampleCaller{}, "schedule", "import", "--groupId=1", `--scheduleVOS=[{"userId":"u","workDate":"2026-01-01","classId":1,"isRest":false}]`, "--user-say-yes")
	for _, input := range []string{"", "no\n"} {
		os.Stdin = attendanceInput(t, input)
		_ = runAttendanceCoverageCommand(t, &productExampleCaller{}, "vacation", "update-type", "--leave-code=leave", `--visibility-rules=[{"type":"dept","visible":["-1"]}]`, "--paid=false")
	}
}
