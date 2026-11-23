package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// sportFakeCaller 是 edition.ToolCaller 的可编程假实现：按脚本顺序返回
// CallTool 结果，用于覆盖 sportCallResult 的各条返回路径。
type sportFakeCaller struct {
	dryRun    bool
	text      string // 首个 text block 内容
	blocks    int    // 返回的 content block 数量（0=空内容）
	err       error
	calls     []string
	arguments []map[string]any
}

func (f *sportFakeCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	// 记录所有调用尝试，包括 dry-run 下的误调用；不伪造预览结果。
	f.calls = append(f.calls, productID+"/"+toolName)
	f.arguments = append(f.arguments, args)
	if f.err != nil {
		return nil, f.err
	}
	result := &edition.ToolResult{}
	for i := 0; i < f.blocks; i++ {
		result.Content = append(result.Content, edition.ContentBlock{Type: "text", Text: f.text})
	}
	return result, nil
}
func (f *sportFakeCaller) Format() string { return "json" }
func (f *sportFakeCaller) DryRun() bool   { return f.dryRun }
func (f *sportFakeCaller) Fields() string { return "" }
func (f *sportFakeCaller) JQ() string     { return "" }

// runSportCommand 以给定参数执行命令树并返回错误。
func runSportCommand(t *testing.T, args ...string) error {
	t.Helper()
	_, err := runSportCommandOutput(t, args...)
	return err
}

// runSportCommandOutput 经准备后的命令树与统一渲染器返回实际 JSON 信封。
func runSportCommandOutput(t *testing.T, args ...string) (map[string]any, error) {
	t.Helper()
	// 叶子经 RolloutUnifiedActive 把结果写入统一结果存储，必须在 root ctx
	// 预装 ResultStore（与 app 层执行边界一致）。
	ctx, _ := output.WithResultStore(context.Background())
	root := newSportCommand()
	root.SetContext(ctx)
	// --yes/--dry-run 是 app root 注入的全局 flag；子树独立执行时补注册等价
	// 的 persistent flag，使确认门能被跳过。
	root.PersistentFlags().Bool("yes", false, "")
	root.PersistentFlags().Bool("dry-run", false, "")
	root.PersistentFlags().String("format", "json", "")
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(io.Discard)
	root.SetArgs(args)
	executed, err := corecmd.ExecuteCForTest(root)
	if err != nil {
		return nil, err
	}
	code, emitted, err := output.EmitStoredResult(executed)
	if err != nil {
		return nil, err
	}
	if !emitted || code != 0 {
		t.Fatalf("统一结果未成功输出：emitted=%v code=%d", emitted, code)
	}
	var envelope map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
		t.Fatalf("解析统一结果失败：%v\n%s", err, stdout.String())
	}
	return envelope, nil
}

func TestSportCallResultWithoutCallerFails(t *testing.T) {
	InitDepsForTest(t, nil)
	_, err := sportCallResult(nil, "get_org_step_participate_status", nil)
	if err == nil || !strings.Contains(err.Error(), "caller is not initialized") {
		t.Fatalf("期望 caller 未初始化错误，实际: %v", err)
	}
}

func TestSportOrgAdminResults(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		want       any
	}{
		{"管理员", `{"success":true,"result":true,"errorCode":null,"errorMsg":null}`, true},
		{"非管理员", `{"success":true,"result":false,"errorCode":null,"errorMsg":null}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &sportFakeCaller{text: tc.text, blocks: 1}
			InitDepsForTest(t, fake)
			envelope, err := runSportCommandOutput(t, "org", "admin")
			if err != nil {
				t.Fatal(err)
			}
			if len(fake.calls) != 1 || fake.calls[0] != sportServerID+"/is_org_admin" {
				t.Fatalf("应调用 is_org_admin：%v", fake.calls)
			}
			if !reflect.DeepEqual(fake.arguments[0], map[string]any{}) {
				t.Fatalf("无业务入参工具应发送空对象：%v", fake.arguments[0])
			}
			data, _ := envelope["data"].(map[string]any)
			if data["result"] != tc.want || envelope["outcome"] != "success" {
				t.Fatalf("三态结果错误：%v", envelope)
			}
		})
	}

	t.Run("组织服务不可用不得当作非管理员", func(t *testing.T) {
		fake := &sportFakeCaller{text: `{"success":false,"errorCode":"QW_ADMIN_CHECK_FAILED","errorMsg":"组织管理员服务查询不可用"}`, blocks: 1}
		InitDepsForTest(t, fake)
		err := runSportCommand(t, "org", "admin")
		// success=false 由统一通道按业务错误体分类，携带 errorMsg 作为 API 错误返回，不会产出 result=false 的结果。
		if err == nil || apperrors.ExitCode(err) != ExitAPI || !strings.Contains(err.Error(), "组织管理员服务查询不可用") {
			t.Fatalf("success=false 应作为组织服务不可用错误返回：%v", err)
		}
	})

	t.Run("缺 success 视为无效回执", func(t *testing.T) {
		fake := &sportFakeCaller{text: `{"result":false}`, blocks: 1}
		InitDepsForTest(t, fake)
		err := runSportCommand(t, "org", "admin")
		if err == nil || apperrors.ExitCode(err) != ExitAPI || !strings.Contains(err.Error(), "不能当作非管理员") {
			t.Fatalf("缺 success 应报组织服务不可用而非 false：%v", err)
		}
	})

	t.Run("result 非布尔视为无效回执", func(t *testing.T) {
		fake := &sportFakeCaller{text: `{"success":true,"result":"yes"}`, blocks: 1}
		InitDepsForTest(t, fake)
		err := runSportCommand(t, "org", "admin")
		if err == nil || apperrors.ExitCode(err) != ExitAPI || !strings.Contains(err.Error(), "缺少布尔 result") {
			t.Fatalf("result 类型不符应报错：%v", err)
		}
	})

	t.Run("dry-run 不调用 MCP", func(t *testing.T) {
		fake := &sportFakeCaller{dryRun: true, err: errors.New("预览不得调用 MCP")}
		InitDepsForTest(t, fake)
		envelope, err := runSportCommandOutput(t, "org", "admin")
		if err != nil {
			t.Fatal(err)
		}
		if envelope["dry_run"] != true || len(fake.calls) != 0 {
			t.Fatalf("预览不得调用工具：%v", envelope)
		}
	})
}

func TestSportCallerDryRunSkipsToolCall(t *testing.T) {
	fake := &sportFakeCaller{dryRun: true, err: errors.New("预览不得调用 MCP")}
	InitDepsForTest(t, fake)
	envelope, err := runSportCommandOutput(t, "org", "status")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"tool": "get_org_step_participate_status", "arguments": map[string]any{}, "executed": false}
	if envelope["dry_run"] != true || !reflect.DeepEqual(envelope["data"], want) || len(fake.calls) != 0 {
		t.Fatalf("caller 预览不应执行工具：envelope=%v calls=%v", envelope, fake.calls)
	}
}

func TestSportCallResultParsesJSONText(t *testing.T) {
	fake := &sportFakeCaller{text: `{"participate":true}`, blocks: 1}
	InitDepsForTest(t, fake)
	envelope, err := runSportCommandOutput(t, "org", "status")
	if err != nil {
		t.Fatalf("JSON 结果不应报错: %v", err)
	}
	if !reflect.DeepEqual(envelope["data"], map[string]any{"participate": true}) || envelope["dry_run"] == true {
		t.Fatalf("真实调用必须保留原业务结果：%v", envelope)
	}
	if len(fake.calls) != 1 || fake.calls[0] != sportServerID+"/get_org_step_participate_status" {
		t.Fatalf("工具调用记录不符合预期: %v", fake.calls)
	}
}

// TestSportCallResultRejectsBusinessErrorBody 验证 isError=false 的业务错误
// 体经统一分类后不会伪装成 success（评审阻塞项 1 的回归用例）。
func TestSportCallResultRejectsBusinessErrorBody(t *testing.T) {
	fake := &sportFakeCaller{text: `{"code":"K001","success":false,"message":"活动不存在"}`, blocks: 1}
	InitDepsForTest(t, fake)
	err := runSportCommand(t, "org", "status")
	var classified *CLIError
	if !errors.As(err, &classified) || classified.Code != CodeMCPToolError || classified.ServerCode != "K001" || classified.ExitCode() != ExitAPI {
		t.Fatalf("必须保留统一 MCP 业务错误分类：%v", err)
	}
	if !strings.Contains(classified.Message, "活动不存在") {
		t.Fatalf("错误信息应保留服务端语义，实际: %v", err)
	}
}

func TestSportCallResultPreservesNullAndEmptyContent(t *testing.T) {
	for _, tc := range []struct {
		name string
		fake *sportFakeCaller
		want any
	}{
		{"配置不存在", &sportFakeCaller{text: "null", blocks: 1}, nil},
		{"空内容", &sportFakeCaller{}, map[string]any{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			InitDepsForTest(t, tc.fake)
			envelope, err := runSportCommandOutput(t, "competition", "config", "--rule-code", "fixture-rule")
			if err != nil {
				t.Fatal(err)
			}
			if envelope["ok"] != true || !reflect.DeepEqual(envelope["data"], tc.want) {
				t.Fatalf("业务空值语义被改写：got=%v want=%v", envelope["data"], tc.want)
			}
		})
	}
}

func TestSportCreateEndToEndUsesNestedPayload(t *testing.T) {
	fake := &sportFakeCaller{text: `{"ruleCode":"R1","status":1}`, blocks: 1}
	InitDepsForTest(t, fake)
	err := runSportCommand(t, "competition", "create",
		"--user-id", "0123456789", "--name", "走路挑战赛", "--sport-type", "WALK",
		"--competition-type", "FIXED", "--start-date", "20261111", "--end-date", "20261130",
		"--goal-value", "8000", "--target-day-value", "18", "--group-id", "123456",
		"--need-signup", "false", "--allow-free-join", "false",
		"--cover-image-url", "https://gw.alicdn.com/cover.png",
		"--daily-limit", "9000", "--open-conversation-id", "cidXXXX", "--yes")
	if err != nil {
		t.Fatalf("create 合法参数不应报错: %v", err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != sportServerID+"/create_competition" {
		t.Fatalf("create 应调用 create_competition: %v", fake.calls)
	}
	want := map[string]any{
		"userId": "0123456789",
		"CompetitionCreateMcpRequest": map[string]any{
			"competitionName": "走路挑战赛", "sportType": "WALK", "competitionType": "FIXED",
			"firstPeriodStartDate": "20261111", "firstPeriodEndDate": "20261130",
			"goalValue": "8000", "targetDayValue": "18", "groupId": "123456",
			"needSignup": "false", "allowFreeJoin": "false", "coverImageUrl": "https://gw.alicdn.com/cover.png",
			"dailyLimit": "9000", "openConversationId": "cidXXXX",
		},
	}
	if !reflect.DeepEqual(fake.arguments[0], want) {
		t.Fatalf("实际调用必须使用嵌套参数：got=%v want=%v", fake.arguments[0], want)
	}
}

func TestSportCreateRejectsTargetBeyondActualPeriod(t *testing.T) {
	fake := &sportFakeCaller{}
	InitDepsForTest(t, fake)
	// 必填参数完整且确认已满足，避免缺参或确认门使日期回归假通过。
	err := runSportCommand(t, "competition", "create", "--user-id", "0123456789", "--name", "走路挑战赛", "--sport-type", "WALK",
		"--competition-type", "FIXED", "--start-date", "20260430", "--end-date", "20260501",
		"--goal-value", "8000", "--target-day-value", "3", "--group-id", "123456",
		"--need-signup", "false", "--allow-free-join", "false", "--cover-image-url", "https://gw.alicdn.com/cover.png", "--yes")
	if err == nil || !strings.Contains(err.Error(), "单期实际天数（2") || apperrors.ExitCode(err) != ExitValidation {
		t.Fatalf("应因跨月周期仅两天返回 validation：%v", err)
	}
	if len(fake.calls) != 0 {
		t.Fatalf("日期校验失败不得调用工具：%v", fake.calls)
	}
}

func TestSportCreateAcceptsOpenConversationIdOnly(t *testing.T) {
	fake := &sportFakeCaller{text: `{"ruleCode":"R1","status":1}`, blocks: 1}
	InitDepsForTest(t, fake)
	err := runSportCommand(t, "competition", "create",
		"--user-id", "0123456789", "--name", "走路挑战赛", "--sport-type", "WALK",
		"--competition-type", "FIXED", "--start-date", "20261111", "--end-date", "20261130",
		"--goal-value", "8000", "--target-day-value", "18",
		"--open-conversation-id", "cidXXXXXXXX==",
		"--need-signup", "false", "--allow-free-join", "false",
		"--cover-image-url", "https://gw.alicdn.com/cover.png", "--yes")
	if err != nil {
		t.Fatalf("仅凭加密会话 id 创建不应报错: %v", err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != sportServerID+"/create_competition" {
		t.Fatalf("create 应调用 create_competition: %v", fake.calls)
	}
	want := map[string]any{
		"userId": "0123456789",
		"CompetitionCreateMcpRequest": map[string]any{
			"competitionName": "走路挑战赛", "sportType": "WALK", "competitionType": "FIXED",
			"firstPeriodStartDate": "20261111", "firstPeriodEndDate": "20261130",
			"goalValue": "8000", "targetDayValue": "18",
			"needSignup": "false", "allowFreeJoin": "false", "coverImageUrl": "https://gw.alicdn.com/cover.png",
			"openConversationId": "cidXXXXXXXX==",
		},
	}
	if !reflect.DeepEqual(fake.arguments[0], want) {
		t.Fatalf("仅会话 id 时应只发 openConversationId，不发 groupId：got=%v want=%v", fake.arguments[0], want)
	}
}

const sportUpdateFixture = `{"ruleCode":"RULE123","competitionName":"步行挑战","coverImageUrl":"https://example.com/cover.png","competitionType":"FIXED","firstPeriodStartDate":"20261201","firstPeriodEndDate":"20261231","periodType":null,"cycleCount":null,"goalValue":6000,"targetDayValue":20,"dailyLimit":null,"extra":null}`

func TestSportNewToolsArgumentsAndConfirmation(t *testing.T) {
	request, err := sportParseUpdateRequest(sportUpdateFixture)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		tool string
		args []string
		want map[string]any
		text string
	}{
		{"send_competition_invite_msg", []string{"competition", "invite", "--rule-code", "RULE123", "--receiver-user-id", "00123", "--target-group-id", "000456"}, map[string]any{"ruleCode": "RULE123", "receiverUserId": "00123", "targetGroupId": "000456"}, ""},
		{"delete_competition", []string{"competition", "delete", "--rule-code", "RULE123"}, map[string]any{"ruleCode": "RULE123"}, ""},
		{"audit_competition", []string{"competition", "audit", "--rule-code", "RULE123", "--approved=false"}, map[string]any{"ruleCode": "RULE123", "approved": false}, ""},
		{"audit_competition", []string{"competition", "audit", "--rule-code", "RULE123", "--approved=true"}, map[string]any{"ruleCode": "RULE123", "approved": true}, ""},
		{"update_competition", []string{"competition", "update", "--request", sportUpdateFixture}, map[string]any{"request": request}, ""},
		{"submit_competition_export", []string{"competition", "export", "--competition-code", "COMP456"}, map[string]any{"competitionCode": "COMP456", "exportScope": "ALL"}, `{"success":true,"result":12345}`},
		{"submit_competition_export", []string{"competition", "export", "--competition-code", "COMP456", "--export-scope", "STAFF", "--staff-ids", "00123,00456"}, map[string]any{"competitionCode": "COMP456", "exportScope": "STAFF", "staffIds": "00123,00456"}, `{"success":true,"result":12345}`},
		{"submit_step_report_export", []string{"step", "export", "--stat-date", "20261001", "--end-date", "20261031", "--staff-ids", "00123"}, map[string]any{"statDate": "20261001", "endDate": "20261031", "staffIds": "00123"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.tool+strings.Join(tc.args, " "), func(t *testing.T) {
			text := tc.text
			if text == "" {
				text = `{"success":true,"result":true,"errorCode":null,"errorMsg":null}`
			}
			fake := &sportFakeCaller{text: text, blocks: 1}
			InitDepsForTest(t, fake)
			if err := runSportCommand(t, tc.args...); err == nil || !strings.Contains(err.Error(), "确认") && !strings.Contains(err.Error(), "confirmation") {
				t.Fatalf("写操作必须先确认：%v", err)
			}
			if len(fake.calls) != 0 {
				t.Fatal("未确认不得调用工具")
			}
			result, err := runSportCommandOutput(t, append(append([]string{}, tc.args...), "--yes")...)
			if err != nil {
				t.Fatal(err)
			}
			if len(fake.calls) != 1 || fake.calls[0] != sportServerID+"/"+tc.tool || !reflect.DeepEqual(fake.arguments[0], tc.want) {
				t.Fatalf("实调用参数/类型不符：%v %v，期望 %v", fake.calls, fake.arguments, tc.want)
			}
			if tc.tool == "submit_step_report_export" {
				data, _ := result["data"].(map[string]any)
				if result["outcome"] != "success" || data["result"] != true || data["taskId"] != nil || result["meta"] != nil {
					t.Fatalf("步数报表应保留受理回执，不编造任务ID或可查询状态：%v", result)
				}
			}
		})
	}
	file := filepath.Join(t.TempDir(), "request.json")
	if err := os.WriteFile(file, []byte(sportUpdateFixture), 0600); err != nil {
		t.Fatal(err)
	}
	fake := &sportFakeCaller{err: errors.New("dry-run 不得调用")}
	InitDepsForTest(t, fake)
	if err := runSportCommand(t, "competition", "update", "--request", "@"+file, "--dry-run"); err != nil || len(fake.calls) != 0 {
		t.Fatalf("文件对象预览失败：%v %v", err, fake.calls)
	}
}

func TestSportNewToolsValidation(t *testing.T) {
	testseam.Swap(t, &sportExportNow, func() time.Time { return time.Date(2026, 11, 16, 0, 0, 0, 0, time.UTC) })
	cases := [][]string{
		{"competition", "invite", "--rule-code", "RULE123"},
		{"competition", "invite", "--rule-code", "RULE123", "--receiver-user-id", "  "},
		{"competition", "create", "--user-id", "0123456789", "--name", "走路挑战赛", "--sport-type", "WALK", "--competition-type", "FIXED", "--start-date", "20261111", "--end-date", "20261130", "--goal-value", "8000", "--target-day-value", "18", "--need-signup", "false", "--allow-free-join", "false", "--cover-image-url", "https://gw.alicdn.com/cover.png"},
		{"competition", "create", "--user-id", "0123456789", "--name", "走路挑战赛", "--sport-type", "WALK", "--competition-type", "FIXED", "--start-date", "20261111", "--end-date", "20261130", "--goal-value", "8000", "--target-day-value", "18", "--open-conversation-id", "  ", "--need-signup", "false", "--allow-free-join", "false", "--cover-image-url", "https://gw.alicdn.com/cover.png"},
		{"competition", "audit", "--rule-code", "RULE123"},
		{"competition", "audit", "--rule-code", "RULE123", "--approved=maybe"},
		{"competition", "export", "--competition-code", "COMP456", "--export-scope", "STAFF"},
		{"competition", "export", "--competition-code", "COMP456", "--export-scope", "WRONG"},
		{"competition", "export", "--competition-code", "COMP456", "--staff-ids", " , "},
		{"step", "export", "--stat-date", "20261001", "--end-date", "20261101"},
		{"step", "export", "--stat-date", "20261010", "--end-date", "20261001"},
		{"step", "export", "--stat-date", "20261115", "--end-date", "20261116"},
		{"step", "export", "--stat-date", "20260230", "--end-date", "20260301"},
		{"step", "export", "--stat-date", "20261001", "--end-date", "20261301"},
		{"step", "export", "--stat-date", "20261001", "--end-date", "20261002", "--staff-ids", " , "},
	}
	for _, id := range []string{"0", "-1", "1.2", "x", "9223372036854775808"} {
		cases = append(cases, []string{"competition", "export-task", "--task-id", id})
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			fake := &sportFakeCaller{}
			InitDepsForTest(t, fake)
			err := runSportCommand(t, append(args, "--yes")...)
			if err == nil || apperrors.ExitCode(err) != ExitValidation || len(fake.calls) != 0 {
				t.Fatalf("非法参数应本地拒绝且零调用：%v %v", err, fake.calls)
			}
		})
	}
}

func TestSportUpdateRequestValidation(t *testing.T) {
	for _, raw := range []string{"x", "[]", "null", "{} {}", "{}"} {
		if _, err := sportParseUpdateRequest(raw); err == nil {
			t.Fatalf("非法对象应拒绝：%s", raw)
		}
	}
	cases := []struct {
		key   string
		value any
	}{
		{"sportType", "RUN"}, {"ruleCode", nil}, {"competitionName", " "},
		{"competitionName", strings.Repeat("名", 129)}, {"coverImageUrl", strings.Repeat("x", 513)},
		{"extra", 1}, {"goalValue", "6000"}, {"targetDayValue", nil}, {"targetDayValue", 1.5},
		{"goalValue", -1}, {"goalValue", json.Number("1e999")}, {"dailyLimit", 0}, {"cycleCount", 13},
		{"firstPeriodStartDate", "20260230"}, {"firstPeriodEndDate", "20261301"}, {"firstPeriodEndDate", "20261101"},
		{"firstPeriodEndDate", nil}, {"competitionType", "BAD"}, {"periodType", "WEEK"},
		{"competitionType", "RECURRING"}, {"targetDayValue", 32},
	}
	for _, tc := range cases {
		request, err := sportDecodeObject(sportUpdateFixture)
		if err != nil {
			t.Fatal(err)
		}
		request[tc.key] = tc.value
		raw, err := json.Marshal(request)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sportParseUpdateRequest(string(raw)); err == nil {
			t.Errorf("%s=%v 应被拒绝", tc.key, tc.value)
		}
	}
	for _, period := range []string{"WEEK", "MONTH", "CUSTOM"} {
		request, err := sportDecodeObject(sportUpdateFixture)
		if err != nil {
			t.Fatal(err)
		}
		request["competitionType"], request["periodType"], request["cycleCount"] = "RECURRING", period, 3
		if period != "CUSTOM" {
			request["firstPeriodEndDate"] = nil
		}
		raw, _ := json.Marshal(request)
		if _, err := sportParseUpdateRequest(string(raw)); err != nil {
			t.Fatalf("合法%s：%v", period, err)
		}
		request["firstPeriodStartDate"], request["firstPeriodEndDate"] = "20261229", nil
		raw, _ = json.Marshal(request)
		_, err = sportParseUpdateRequest(string(raw))
		if period != "WEEK" && err == nil {
			t.Fatalf("%s 边界应拒绝", period)
		}
	}
}

func TestSportServiceResultsAndExportStates(t *testing.T) {
	const taskID = "9007199254740993"
	cases := []struct {
		tool, text, outcome, wantErr string
		args                         map[string]any
	}{
		{"submit_competition_export", `{"success":true,"result":` + taskID + `}`, "pending", "", nil},
		{"get_competition_export_task", `{"success":true,"result":{"taskId":` + taskID + `,"status":"PROCESSING"}}`, "pending", "", map[string]any{"taskId": int64(9007199254740993)}},
		{"get_competition_export_task", `{"success":true,"result":{"taskId":` + taskID + `,"status":"SUCCESS","ossUrl":"https://example.com/a?x=1%2F2","dailyOssUrls":null}}`, "success", "", map[string]any{"taskId": int64(9007199254740993)}},
		{"get_competition_export_task", `{"success":true,"result":{"taskId":1,"status":"FAILED","errorMsg":"生成失败"}}`, "", "生成失败", map[string]any{"taskId": int64(1)}},
		{"get_competition_export_task", `{"success":true,"result":{"taskId":1,"status":"UNKNOWN"}}`, "", "状态", map[string]any{"taskId": int64(1)}},
		{"get_competition_export_task", `{"success":true,"result":{"taskId":1,"status":"SUCCESS"}}`, "", "下载链接", map[string]any{"taskId": int64(1)}},
		{"get_competition_export_task", `{"success":true,"result":{"taskId":2,"status":"PROCESSING"}}`, "", "taskId", map[string]any{"taskId": int64(1)}},
		{"get_competition_export_task", `{"success":true,"result":null}`, "", "taskId", nil},
		{"submit_competition_export", `{"success":true,"result":"12"}`, "", "taskId", nil},
		{"submit_competition_export", `{"success":true,"result":0}`, "", "taskId", nil},
		{"delete_competition", `{"result":true}`, "", "success", nil},
		{"delete_competition", `{"success":true,"result":false}`, "", "result", nil},
		{"delete_competition", `not-json`, "", "ServiceResult", nil},
		{"delete_competition", `{"success":false,"errorCode":"QW_COMPETITION_DELETE_FAILED","errorMsg":"活动已删除"}`, "", "活动已删除", nil},
	}
	for _, tc := range cases {
		t.Run(tc.tool+tc.text, func(t *testing.T) {
			fake := &sportFakeCaller{text: tc.text, blocks: 1}
			InitDepsForTest(t, fake)
			cmd := newSportCommand()
			cmd.SetContext(context.Background())
			cmd.PersistentFlags().String("profile", "fixture:operator", "")
			result, err := sportServiceCallResult(cmd, tc.tool, tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("期望%s，实际%v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			env, err := output.EnvelopeFromResult(result)
			if err != nil {
				t.Fatal(err)
			}
			if string(env.Outcome) != tc.outcome {
				t.Fatalf("结果态：%v", env)
			}
			wire, err := json.Marshal(env)
			if err != nil || !strings.Contains(string(wire), taskID) {
				t.Fatalf("Long精度丢失：%s %v", wire, err)
			}
			if tc.outcome == "pending" && (env.Meta.Operation.ID != taskID || !strings.Contains(env.Meta.Operation.NextCommand, "export-task --task-id "+taskID) || !strings.Contains(env.Meta.Operation.NextCommand, "--profile 'fixture:operator'")) {
				t.Fatalf("任务恢复指引错误：%v", env.Meta)
			}
			if len(fake.calls) != 1 {
				t.Fatal("不得重复调用或下载链接")
			}
		})
	}
	InitDepsForTest(t, nil)
	if _, err := sportServiceCallResult(newSportCommand(), "delete_competition", nil); err == nil {
		t.Fatal("caller缺失应报错")
	}
	fake := &sportFakeCaller{dryRun: true}
	InitDepsForTest(t, fake)
	if result, err := sportServiceCallResult(newSportCommand(), "delete_competition", nil); err != nil || result == nil || len(fake.calls) != 0 {
		t.Fatalf("caller预览失败：%v", err)
	}
}

func TestSportNumericHelpersEdgeCases(t *testing.T) {
	for _, v := range []string{"", "abc", "-1", "1.2.3", "00"} {
		if sportIsPositiveNumber(v) {
			t.Fatalf("sportIsPositiveNumber(%q) 应为 false", v)
		}
	}
	for _, v := range []string{"1", "0.5", "8000.00"} {
		if !sportIsPositiveNumber(v) {
			t.Fatalf("sportIsPositiveNumber(%q) 应为 true", v)
		}
	}
	for _, v := range []string{"", "x", "0", "1.5", "1000001"} {
		if _, ok := sportParsePositiveInt(v); ok {
			t.Fatalf("sportParsePositiveInt(%q) 应失败", v)
		}
	}
	if n, ok := sportParsePositiveInt("1000000"); !ok || n != 1_000_000 {
		t.Fatalf("sportParsePositiveInt 上限边界不符合预期: %d %v", n, ok)
	}
	// 评测次要项 4：超过 6 位小数不得截断失真。
	if got := sportCompareNumbers("2", "1.0000001"); got != 1 {
		t.Fatalf("sportCompareNumbers(2, 1.0000001) 应为 1，实际 %d", got)
	}
	if got := sportCompareNumbers("01.20", "1.2"); got != 0 {
		t.Fatalf("sportCompareNumbers(01.20, 1.2) 应为 0，实际 %d", got)
	}
	if got := sportCompareNumbers("100", "99.999999999"); got != 1 {
		t.Fatalf("sportCompareNumbers(100, 99.999999999) 应为 1，实际 %d", got)
	}
	for _, tc := range []struct {
		a, b string
		want int
	}{
		{"5", "10", -1}, {"1.2", "1.25", -1}, {"1.25", "1.2", 1}, {".5", ".50", 0},
		{"8000", "8000.000000", 0}, {"9000", "8000", 1},
	} {
		if got := sportCompareNumbers(tc.a, tc.b); got != tc.want {
			t.Fatalf("sportCompareNumbers(%s, %s) 应为 %d，实际 %d", tc.a, tc.b, tc.want, got)
		}
	}
}

func TestSportReadToolsArgumentsAndResults(t *testing.T) {
	const page = `{"success":true,"result":{"list":[{"code":"RULE123"}],"hasMore":false,"nextCursor":null}}`
	cases := []struct {
		tool, text string
		args       []string
		want       map[string]any
	}{
		{"list_competition_configs", page, []string{"competition", "list"}, map[string]any{"cursor": int64(0), "size": 20}},
		{"list_my_competition_rules", page, []string{"competition", "my-list"}, map[string]any{"listScope": "JOINED", "cursor": nil, "size": 20}},
		{"list_my_competition_rules", page, []string{"competition", "my-list", "--list-scope", "CREATED", "--cursor", "9007199254740993", "--size", "50"}, map[string]any{"listScope": "CREATED", "cursor": int64(9007199254740993), "size": 50}},
		{"list_my_participated_comp", page, []string{"competition", "my-participated", "--cursor", "null"}, map[string]any{"cursor": nil, "pageSize": 20}},
		{"list_competition_rank", page, []string{"competition", "ranking", "--competition-code", "COMP456"}, map[string]any{"competitionCode": "COMP456", "cursor": nil, "size": 20}},
		{"get_competition_rank_top_n", `{"success":true,"result":[]}`, []string{"competition", "rank-top", "--rule-code", "RULE123", "--rank-type", "TOTAL", "--sort-by-reached-times=false"}, map[string]any{"ruleCode": "RULE123", "rankType": "TOTAL", "sortByReachedTimes": false, "topN": 10}},
		{"count_my_can_join_competitions", `{"success":true,"result":0}`, []string{"competition", "my-can-join-count"}, map[string]any{}},
		{"count_my_participated_comp", `{"success":true,"result":3}`, []string{"competition", "my-participated-count"}, map[string]any{}},
		{"get_competition_activity_detail", `{"success":true,"result":{"ruleCode":"RULE123","periodList":null}}`, []string{"competition", "activity-detail", "--rule-code", "RULE123", "--competition-code", "COMP456"}, map[string]any{"ruleCode": "RULE123", "competitionCode": "COMP456"}},
		{"get_personal_achievement", `{"success":true,"result":{"userId":"00123","rankNo":-1}}`, []string{"competition", "my-achievement", "--competition-code", "COMP456"}, map[string]any{"competitionCode": "COMP456"}},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			fake := &sportFakeCaller{text: tc.text, blocks: 1}
			InitDepsForTest(t, fake)
			// 只读命令不得要求 --yes，且不得上送操作人身份。
			envelope, err := runSportCommandOutput(t, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			if len(fake.calls) != 1 || fake.calls[0] != sportServerID+"/"+tc.tool || len(fake.arguments[0]) != len(tc.want) || (len(tc.want) > 0 && !reflect.DeepEqual(fake.arguments[0], tc.want)) {
				t.Fatalf("实调用参数不符：%v %v，期望 %v", fake.calls, fake.arguments, tc.want)
			}
			if envelope["outcome"] != "success" {
				t.Fatalf("只读结果应成功：%v", envelope)
			}
			if tc.text == page {
				data, _ := envelope["data"].(map[string]any)
				meta, _ := envelope["meta"].(map[string]any)
				pagination, _ := meta["pagination"].(map[string]any)
				if !reflect.DeepEqual(data["result"], map[string]any{"list": []any{map[string]any{"code": "RULE123"}}}) || pagination["endpoint_exhausted"] != true || pagination["next_token"] != nil {
					t.Fatalf("分页控制信息必须只在 meta：%v", envelope)
				}
			}
		})
	}
}

func TestSportReadPaginationAndInvalidResults(t *testing.T) {
	const next = "9007199254740993"
	fake := &sportFakeCaller{text: `{"success":true,"result":{"list":[],"hasMore":true,"nextCursor":` + next + `}}`, blocks: 1}
	InitDepsForTest(t, fake)
	envelope, err := runSportCommandOutput(t, "competition", "ranking", "--competition-code", "COMP456", "--cursor", "12")
	if err != nil {
		t.Fatal(err)
	}
	meta, _ := envelope["meta"].(map[string]any)
	pagination, _ := meta["pagination"].(map[string]any)
	if pagination["endpoint_exhausted"] != false || pagination["next_token"] != next {
		t.Fatalf("空页仍有下一页时必须保留精确 Long 游标：%v", envelope)
	}

	cases := []struct {
		tool, text, wantErr string
		args                map[string]any
	}{
		{"list_competition_rank", `{"success":true,"result":{"list":[],"hasMore":true}}`, "nextCursor", nil},
		{"list_competition_rank", `{"success":true,"result":{"list":[],"hasMore":true,"nextCursor":5}}`, "未前进", map[string]any{"cursor": int64(5)}},
		{"list_competition_configs", `{"success":true,"result":{"hasMore":false}}`, "list", nil},
		{"list_my_competition_rules", `{"success":true,"result":{"list":[],"hasMore":"false"}}`, "hasMore", nil},
		{"list_my_participated_comp", `{"result":{"list":[],"hasMore":false}}`, "success", nil},
		{"get_competition_rank_top_n", `not-json`, "ServiceResult", nil},
		{"get_competition_rank_top_n", `{"success":true,"result":{}}`, "数组", nil},
		{"count_my_can_join_competitions", `{"success":true,"result":-1}`, "计数", nil},
		{"count_my_participated_comp", `{"success":true,"result":"3"}`, "计数", nil},
		{"count_my_participated_comp", `{"success":true,"result":1.5}`, "计数", nil},
		{"get_competition_activity_detail", `{"success":true,"result":null}`, "详情", nil},
		{"get_competition_activity_detail", `{"success":false,"errorCode":"QW_COMPETITION_QUERY_FAILED","errorMsg":"活动不存在"}`, "活动不存在", nil},
	}
	for _, tc := range cases {
		t.Run(tc.tool+tc.text, func(t *testing.T) {
			fake := &sportFakeCaller{text: tc.text, blocks: 1}
			InitDepsForTest(t, fake)
			cmd := newSportCommand()
			cmd.SetContext(context.Background())
			if _, err := sportReadServiceCallResult(cmd, tc.tool, tc.args); err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("异常回执不得当作空结果，期望%s，实际%v", tc.wantErr, err)
			}
			if len(fake.calls) != 1 {
				t.Fatalf("只读查询不得自动重试：%v", fake.calls)
			}
		})
	}
	InitDepsForTest(t, nil)
	if _, err := sportReadServiceCallResult(newSportCommand(), "list_competition_rank", nil); err == nil {
		t.Fatal("caller缺失应报错")
	}
}

func TestSportReadToolsValidation(t *testing.T) {
	cases := [][]string{
		{"competition", "list", "--size", "101"},
		{"competition", "list", "--size", "0"},
		{"competition", "list", "--size", "x"},
		{"competition", "my-list", "--size", "51"},
		{"competition", "my-list", "--list-scope", "ALL"},
		{"competition", "my-list", "--cursor", "1.5"},
		{"competition", "my-participated", "--page-size", "0"},
		{"competition", "ranking"},
		{"competition", "ranking", "--competition-code", "COMP456", "--size", "51"},
		{"competition", "ranking", "--competition-code", "COMP456", "--cursor", "9223372036854775808"},
		{"competition", "rank-top", "--rank-type", "TOTAL"},
		{"competition", "rank-top", "--rule-code", "RULE123", "--rank-type", "YEAR"},
		{"competition", "rank-top", "--rule-code", "RULE123", "--rank-type", "TOTAL", "--top-n", "51"},
		{"competition", "activity-detail"},
		{"competition", "my-achievement"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			fake := &sportFakeCaller{}
			InitDepsForTest(t, fake)
			err := runSportCommand(t, args...)
			if err == nil || apperrors.ExitCode(err) != ExitValidation || len(fake.calls) != 0 {
				t.Fatalf("非法参数应本地拒绝且零调用：%v %v", err, fake.calls)
			}
		})
	}
}
