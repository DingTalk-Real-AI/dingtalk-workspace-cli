package sport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

// sportFakeCaller 是 edition.ToolCaller 的可编程假实现：按脚本顺序返回
// CallTool 结果，用于覆盖 sportCallResult 的各条返回路径。
type sportFakeCaller struct {
	dryRun       bool
	text         string // 首个 text block 内容
	blocks       int    // 返回的 content block 数量（0=空内容）
	leadingEmpty bool   // 在首个有效 text 前插入一个空 text block
	err          error
	calls        []string
	arguments    []map[string]any
}

func (f *sportFakeCaller) CallTool(_ context.Context, productID, toolName string, args map[string]any) (*edition.ToolResult, error) {
	// 记录所有调用尝试，包括 dry-run 下的误调用；不伪造预览结果。
	f.calls = append(f.calls, productID+"/"+toolName)
	f.arguments = append(f.arguments, args)
	if f.err != nil {
		return nil, f.err
	}
	result := &edition.ToolResult{}
	if f.leadingEmpty {
		result.Content = append(result.Content, edition.ContentBlock{Type: "text", Text: "  "})
	}
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
	root := NewCommand()
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

// sportRunStatus 执行 sport org status（最简单的只读叶子）。
func sportRunStatus(t *testing.T) error {
	t.Helper()
	return runSportCommand(t, "org", "status")
}

func TestSportCallResultWithoutCallerFails(t *testing.T) {
	helpers.InitDepsForTest(t, nil)
	err := sportRunStatus(t)
	if err == nil || !strings.Contains(err.Error(), "caller is not initialized") {
		t.Fatalf("期望 caller 未初始化错误，实际: %v", err)
	}
}

func TestSportCallResultDryRunSkipsToolCall(t *testing.T) {
	for _, tc := range []struct {
		name      string
		callerDry bool
		args      []string
	}{
		{"caller预览", true, []string{"org", "status"}},
		{"根flag预览", false, []string{"--dry-run", "org", "status"}},
		{"叶子后flag预览", false, []string{"org", "status", "--dry-run"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &sportFakeCaller{dryRun: tc.callerDry, err: errors.New("预览不得调用 MCP")}
			helpers.InitDepsForTest(t, fake)
			envelope, err := runSportCommandOutput(t, tc.args...)
			if err != nil {
				t.Fatalf("dry-run 不应报错: %v", err)
			}
			if len(fake.calls) != 0 {
				t.Fatalf("dry-run 不应调用工具: %v", fake.calls)
			}
			if envelope["dry_run"] != true || envelope["ok"] != true {
				t.Fatalf("顶层信封必须标记 dry_run=true：%v", envelope)
			}
			want := map[string]any{"tool": "get_org_step_participate_status", "arguments": map[string]any{}, "executed": false}
			if !reflect.DeepEqual(envelope["data"], want) {
				t.Fatalf("预览载荷不符：got=%v want=%v", envelope["data"], want)
			}
		})
	}
}

func TestSportCallResultParsesJSONText(t *testing.T) {
	fake := &sportFakeCaller{text: `{"participate":true}`, blocks: 1}
	helpers.InitDepsForTest(t, fake)
	if err := sportRunStatus(t); err != nil {
		t.Fatalf("JSON 结果不应报错: %v", err)
	}
	if len(fake.calls) != 1 || fake.calls[0] != sportServerID+"/get_org_step_participate_status" {
		t.Fatalf("工具调用记录不符合预期: %v", fake.calls)
	}
}

func TestSportCallResultRejectsNonJSONText(t *testing.T) {
	fake := &sportFakeCaller{text: "不是 JSON", blocks: 1}
	helpers.InitDepsForTest(t, fake)
	err := sportRunStatus(t)
	if err == nil || !strings.Contains(err.Error(), "解析") || !strings.Contains(err.Error(), "返回失败") {
		t.Fatalf("期望统一通道的 JSON 解析错误，实际: %v", err)
	}
}

// TestSportCallResultRejectsBusinessErrorBody 验证 isError=false 的业务错误
// 体经统一分类后不会伪装成 success（评审阻塞项 1 的回归用例）。
func TestSportCallResultRejectsBusinessErrorBody(t *testing.T) {
	fake := &sportFakeCaller{text: `{"code":"K001","success":false,"message":"活动不存在"}`, blocks: 1}
	helpers.InitDepsForTest(t, fake)
	err := sportRunStatus(t)
	if err == nil {
		t.Fatalf("业务错误体不应被当作 success")
	}
	if !strings.Contains(err.Error(), "活动不存在") {
		t.Fatalf("错误信息应保留服务端语义，实际: %v", err)
	}
}

func TestSportCallResultHandlesNullAndEmptyContent(t *testing.T) {
	for name, fake := range map[string]*sportFakeCaller{
		"null文本": {text: "null", blocks: 1},
		"空内容":    {},
		"跳过空文本块": {text: `null`, blocks: 1, leadingEmpty: true},
	} {
		helpers.InitDepsForTest(t, fake)
		if err := sportRunStatus(t); err != nil {
			t.Fatalf("%s 不应报错: %v", name, err)
		}
	}
}

func TestSportCallResultWrapsToolError(t *testing.T) {
	fake := &sportFakeCaller{err: errors.New("SERVICE_NOT_FOUND")}
	helpers.InitDepsForTest(t, fake)
	err := sportRunStatus(t)
	if err == nil || !strings.Contains(err.Error(), "SERVICE_NOT_FOUND") {
		t.Fatalf("期望包装后的工具错误，实际: %v", err)
	}
}

func TestSportCreateEndToEndUsesNestedPayload(t *testing.T) {
	fake := &sportFakeCaller{text: `{"ruleCode":"R1","status":1}`, blocks: 1}
	helpers.InitDepsForTest(t, fake)
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

func TestSportCreateValidationErrorsShortCircuitBeforeCall(t *testing.T) {
	base := []string{
		"competition", "create",
		"--user-id", "0123456789", "--name", "走路挑战赛", "--sport-type", "WALK",
		"--start-date", "20261111", "--end-date", "20261130",
		"--goal-value", "8000", "--target-day-value", "18", "--group-id", "123456",
		"--need-signup", "false", "--allow-free-join", "false",
		"--cover-image-url", "https://gw.alicdn.com/cover.png",
	}
	cases := [][]string{
		append(append([]string{}, base...), "--competition-type", "RECURRING"),
		append(append([]string{}, base...), "--competition-type", "FIXED", "--period-type", "WEEK"),
		append(append([]string{}, base...), "--competition-type", "FIXED", "--goal-value", "0"),
		append(append([]string{}, base...), "--competition-type", "FIXED", "--target-day-value", "abc"),
		append(append([]string{}, base...), "--competition-type", "FIXED", "--daily-limit", "1.2.3"),
		// 评测 V4 复现：20260430→20260501 实际仅 2 天，target 3 必须被拒绝。
		{"competition", "create", "--user-id", "0123456789", "--name", "走路挑战赛", "--sport-type", "WALK",
			"--competition-type", "FIXED", "--start-date", "20260430", "--end-date", "20260501",
			"--goal-value", "8000", "--target-day-value", "3", "--group-id", "123456",
			"--need-signup", "false", "--allow-free-join", "false"},
	}
	for i, args := range cases {
		fake := &sportFakeCaller{}
		helpers.InitDepsForTest(t, fake)
		if err := runSportCommand(t, args...); err == nil {
			t.Fatalf("case %d 期望校验报错", i)
		}
		if len(fake.calls) != 0 {
			t.Fatalf("case %d 校验失败不应调用工具", i)
		}
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
	for _, v := range []string{"", "x", "0", "1000001"} {
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
	} {
		if got := sportCompareNumbers(tc.a, tc.b); got != tc.want {
			t.Fatalf("sportCompareNumbers(%s, %s) 应为 %d，实际 %d", tc.a, tc.b, tc.want, got)
		}
	}
}
