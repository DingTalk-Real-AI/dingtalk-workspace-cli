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
	"sort"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/executor"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

// devEventUpstreamResult 复刻 runDevEventTool 交给结果归一的 executor.Result：
// Implemented=true + Kind=helper_invocation 才会解包 response.content，Tool 由
// runDevEventTool 回填。少了这两步，失败投影与分页 fail-closed 都不会执行。
func devEventUpstreamResult(tool string, dryRun bool, params, content map[string]any) executor.Result {
	invocation := executor.NewHelperInvocation("dev event "+tool, devEventProduct, tool, params)
	invocation.Implemented = true
	invocation.DryRun = dryRun
	return executor.Result{
		Invocation: invocation,
		Response:   map[string]any{"content": content},
	}
}

// devEventEnvelopeOf 是断言入口：CommandResult 的 envelope 方法不导出，
// EnvelopeFromResult 是唯一被认可的观察方式。
func devEventEnvelopeOf(t *testing.T, result output.CommandResult) *output.Envelope {
	t.Helper()
	env, err := output.EnvelopeFromResult(result)
	if err != nil {
		t.Fatalf("EnvelopeFromResult() error = %v", err)
	}
	return env
}

// TestDevEventRevisionConflictIsExplicitNonRetryable 是设计方案 §7.4 的核心断言：
// 409 修订冲突映射为显式失败，Retryable 缺席（不透明重试），Hint 固定，Details
// 同时给出双方修订号，让调用方对比而不是盲目重放。
func TestDevEventRevisionConflictIsExplicitNonRetryable(t *testing.T) {
	result := devEventUpstreamResult(devEventUpdateTool, false, nil, map[string]any{
		"success":                false,
		"errorCode":              devEventRevisionConflictCode,
		"errorMsg":               "草稿修订号不一致",
		"expectedConfigRevision": 3,
		"currentConfigRevision":  5,
	})
	env := devEventEnvelopeOf(t, devEventCommandResult(result))
	if env.Outcome != output.OutcomeFailure {
		t.Fatalf("outcome = %q, want failure", env.Outcome)
	}
	info := env.Error
	if info == nil {
		t.Fatal("error = nil, want explicit failure for revision conflict")
	}
	if info.Type != "api" {
		t.Fatalf("error.type = %q, want api", info.Type)
	}
	if info.HTTPStatus != devEventRevisionConflictStatus {
		t.Fatalf("error.http_status = %d, want %d", info.HTTPStatus, devEventRevisionConflictStatus)
	}
	if info.Retryable {
		t.Fatal("error.retryable = true, want absent: 修订冲突不是网络抖动，不得透明重试")
	}
	if info.Hint != devEventRevisionConflictHint {
		t.Fatalf("error.hint = %q, want %q", info.Hint, devEventRevisionConflictHint)
	}
	if got := info.Details["expectedConfigRevision"]; got != 3 {
		t.Fatalf("details.expectedConfigRevision = %v, want 3", got)
	}
	if got := info.Details["currentConfigRevision"]; got != 5 {
		t.Fatalf("details.currentConfigRevision = %v, want 5", got)
	}
	if env.Data != nil {
		t.Fatalf("data = %#v, want absent on failure (I3: error 非空时 data 缺席)", env.Data)
	}
}

// TestDevEventRevisionConflictWithoutRevisionFacts 覆盖上游只回报冲突码、
// 不给修订号的退化场景：Hint 与不可重试语义仍必须成立，Details 不得凭空造修订号。
func TestDevEventRevisionConflictWithoutRevisionFacts(t *testing.T) {
	result := devEventUpstreamResult(devEventPublishTool, false, nil, map[string]any{
		"success":   false,
		"errorCode": devEventRevisionConflictCode,
	})
	info := devEventEnvelopeOf(t, devEventCommandResult(result)).Error
	if info == nil {
		t.Fatal("error = nil, want failure")
	}
	if info.Retryable || info.HTTPStatus != devEventRevisionConflictStatus {
		t.Fatalf("retryable = %v, http_status = %d, want false / %d",
			info.Retryable, info.HTTPStatus, devEventRevisionConflictStatus)
	}
	if _, present := info.Details["expectedConfigRevision"]; present {
		t.Fatalf("details.expectedConfigRevision = %v, want absent: CLI 不合成修订号",
			info.Details["expectedConfigRevision"])
	}
	if _, present := info.Details["currentConfigRevision"]; present {
		t.Fatalf("details.currentConfigRevision = %v, want absent", info.Details["currentConfigRevision"])
	}
}

// TestDevEventCodeHintsFillSilentUpstream 遍历全部稳定业务码：上游沉默时由本模块
// 补齐可恢复动作。map 迭代顺序随机，先排序保证失败信息可复现。
func TestDevEventCodeHintsFillSilentUpstream(t *testing.T) {
	codes := make([]string, 0, len(devEventCodeHints))
	for code := range devEventCodeHints {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		t.Run(code, func(t *testing.T) {
			result := devEventUpstreamResult(devEventCreateTool, false, nil, map[string]any{
				"success":   false,
				"errorCode": code,
			})
			info := devEventEnvelopeOf(t, devEventCommandResult(result)).Error
			if info == nil {
				t.Fatal("error = nil, want failure")
			}
			if got, _ := info.UpstreamCode.(string); got != code {
				t.Fatalf("upstream_code = %v, want %q", info.UpstreamCode, code)
			}
			if info.Hint != devEventCodeHints[code] {
				t.Fatalf("hint = %q, want %q", info.Hint, devEventCodeHints[code])
			}
			if code != devEventRevisionConflictCode && info.Retryable {
				t.Fatalf("retryable = true, want absent: CLI 不自动重放写操作")
			}
		})
	}
}

// TestDevEventUpstreamHintWins 验证「上游已给出 Hint 时不覆盖」：可恢复动作的
// 权威来源是服务端，CLI 只在上游沉默时补齐。
func TestDevEventUpstreamHintWins(t *testing.T) {
	result := devEventUpstreamResult(devEventCreateTool, false, nil, map[string]any{
		"success":      false,
		"errorCode":    "DEBUG_REQUIRED",
		"friendlyHint": "服务端指定的处置动作",
	})
	info := devEventEnvelopeOf(t, devEventCommandResult(result)).Error
	if info == nil {
		t.Fatal("error = nil, want failure")
	}
	if info.Hint != "服务端指定的处置动作" {
		t.Fatalf("hint = %q, want upstream hint preserved", info.Hint)
	}
}

// TestDevEventUnknownUpstreamCodeKeepsUpstreamFacts 验证未知业务码不猜类别：
// 码原样透传，Hint 保持上游值（可能为空），不落到任何本模块映射上。
func TestDevEventUnknownUpstreamCodeKeepsUpstreamFacts(t *testing.T) {
	result := devEventUpstreamResult(devEventGetTool, false, nil, map[string]any{
		"success":   false,
		"errorCode": "SOMETHING_NEW",
	})
	info := devEventEnvelopeOf(t, devEventCommandResult(result)).Error
	if info == nil {
		t.Fatal("error = nil, want failure")
	}
	if got, _ := info.UpstreamCode.(string); got != "SOMETHING_NEW" {
		t.Fatalf("upstream_code = %v, want SOMETHING_NEW", info.UpstreamCode)
	}
	for _, hint := range devEventCodeHints {
		if info.Hint == hint {
			t.Fatalf("hint = %q, want no mapping applied for an unknown code", info.Hint)
		}
	}
}

// TestDevEventUpstreamCodeNonString 覆盖 UpstreamCode 缺席与非字符串两种退化：
// 都归一为空串，交给「上游沉默」分支处理，不得 panic。
func TestDevEventUpstreamCodeNonString(t *testing.T) {
	if got := devEventUpstreamCode(nil); got != "" {
		t.Fatalf("devEventUpstreamCode(nil) = %q, want empty", got)
	}
	if got := devEventUpstreamCode(&output.ErrorInfo{}); got != "" {
		t.Fatalf("devEventUpstreamCode(absent) = %q, want empty", got)
	}
	if got := devEventUpstreamCode(&output.ErrorInfo{UpstreamCode: 500}); got != "" {
		t.Fatalf("devEventUpstreamCode(int) = %q, want empty", got)
	}
	if got := devEventUpstreamCode(&output.ErrorInfo{UpstreamCode: "  X  "}); got != "X" {
		t.Fatalf("devEventUpstreamCode(padded) = %q, want X", got)
	}
}

// TestDevEventDebugOutcomesAreMutuallyExclusive 是方案 A 的判定表：三类结论互斥，
// 任何一类都不得假报调试通过。
func TestDevEventDebugOutcomesAreMutuallyExclusive(t *testing.T) {
	allStages := func(overrides map[string]bool) map[string]any {
		stages := map[string]any{}
		for _, stage := range devEventDebugStages {
			stages[stage] = true
		}
		for stage, value := range overrides {
			stages[stage] = value
		}
		return stages
	}
	cases := []struct {
		name        string
		content     map[string]any
		wantSubtype string
		wantParams  []string
	}{
		{
			name:        "上游无阶段事实",
			content:     map[string]any{"eventKey": "app_user_hr_employee_joined"},
			wantSubtype: "debug_not_ready",
		},
		{
			name: "阶段不全",
			content: map[string]any{
				"debugPassed": false,
				"stages":      allStages(map[string]bool{"subscriptionMatched": false, "consumerReceived": false}),
			},
			wantSubtype: "debug_not_passed",
			wantParams:  []string{"subscriptionMatched", "consumerReceived"},
		},
		{
			name: "阶段非布尔按未完成处理",
			content: map[string]any{
				"debugPassed": true,
				"stages":      map[string]any{"appAuthenticated": "yes"},
			},
			wantSubtype: "debug_not_passed",
			wantParams:  []string{"appAuthenticated", "eventAccepted", "subscriptionMatched", "consumerReceived"},
		},
		{
			name: "阶段齐全但 debugPassed 为假",
			content: map[string]any{
				"debugPassed": false,
				"stages":      allStages(nil),
			},
			wantSubtype: "debug_not_passed",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			result := devEventUpstreamResult(devEventDebugTool, false, nil, c.content)
			env := devEventEnvelopeOf(t, devEventCommandResult(result))
			if env.Outcome != output.OutcomeFailure {
				t.Fatalf("outcome = %q, want failure", env.Outcome)
			}
			if env.Error == nil || env.Error.Subtype != c.wantSubtype {
				t.Fatalf("subtype = %#v, want %q", env.Error, c.wantSubtype)
			}
			if env.Error.Type != "api" {
				t.Fatalf("type = %q, want api", env.Error.Type)
			}
			if env.Error.Hint == "" {
				t.Fatal("hint empty, want a recovery action")
			}
			if len(c.wantParams) == 0 {
				return
			}
			if len(env.Error.Params) != len(c.wantParams) {
				t.Fatalf("params = %v, want %v", env.Error.Params, c.wantParams)
			}
			for i, stage := range c.wantParams {
				if env.Error.Params[i] != stage {
					t.Fatalf("params = %v, want %v (未完成阶段按声明顺序)", env.Error.Params, c.wantParams)
				}
			}
		})
	}
}

// TestDevEventDebugPassedAppendsNextCommand 验证调试通过的唯一路径：四阶段全真且
// debugPassed 为真 → success，并按本次真实参数补可直接执行的 publish 命令
// （调试不递增修订号，故复用同一 expectedConfigRevision）。
func TestDevEventDebugPassedAppendsNextCommand(t *testing.T) {
	content := map[string]any{
		"debugPassed": true,
		"stages": map[string]any{
			"appAuthenticated":    true,
			"eventAccepted":       true,
			"subscriptionMatched": true,
			"consumerReceived":    true,
		},
		"testedRevision": 4,
	}
	params := map[string]any{
		"unifiedAppId":           "uapp_self_hr_A",
		"eventKey":               "app_user_hr_employee_joined",
		"expectedConfigRevision": 4,
	}
	env := devEventEnvelopeOf(t, devEventCommandResult(devEventUpstreamResult(devEventDebugTool, false, params, content)))
	if env.Outcome != output.OutcomeSuccess {
		t.Fatalf("outcome = %q, want success (error = %#v)", env.Outcome, env.Error)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data = %#v, want object", env.Data)
	}
	want := "dws dev event publish --unified-app-id uapp_self_hr_A --event-key app_user_hr_employee_joined --expected-revision 4 --yes"
	if data["nextCommand"] != want {
		t.Fatalf("nextCommand = %v, want %q", data["nextCommand"], want)
	}
}

// TestDevEventDebugDryRunNeverReportsStageFacts 验证 dry-run 不做阶段判定：
// 本地 invocation 预览没有阶段事实，既不得报 debug_not_ready，也不得出现
// debugPassed:true 这类假通过。
func TestDevEventDebugDryRunNeverReportsStageFacts(t *testing.T) {
	env := devEventEnvelopeOf(t, devEventCommandResult(devEventUpstreamResult(devEventDebugTool, true, nil, map[string]any{})))
	if env.Outcome != output.OutcomeSuccess {
		t.Fatalf("outcome = %q, want success for a dry-run preview", env.Outcome)
	}
	if !env.DryRun {
		t.Fatal("dry_run = false, want true: dry-run 是已完成的本地预演，不是异步未终态")
	}
	data, _ := env.Data.(map[string]any)
	if data == nil {
		t.Fatalf("data = %#v, want the preview payload", env.Data)
	}
	if passed, present := data["debugPassed"]; present && passed == true {
		t.Fatal("dry-run payload carries debugPassed:true, want no stage facts invented")
	}
}

// TestDevEventDebugErrorInfoCarriesTestedRevisionAndCodeHint 验证 debug 失败骨架：
// 上游的 testedRevision 原样透传，稳定业务码命中映射时覆盖本模块默认 Hint。
func TestDevEventDebugErrorInfoCarriesTestedRevisionAndCodeHint(t *testing.T) {
	info := devEventDebugErrorInfo(map[string]any{
		"testedRevision": 4,
		"errorCode":      "DEBUG_TIMEOUT",
	}, "debug_not_passed", "本地默认信息", "本地默认动作")
	if info.Type != "api" || info.Subtype != "debug_not_passed" {
		t.Fatalf("type/subtype = %q/%q, want api/debug_not_passed", info.Type, info.Subtype)
	}
	if info.Message != "本地默认信息" {
		t.Fatalf("message = %q, want the caller-supplied message", info.Message)
	}
	if info.Details["testedRevision"] != 4 {
		t.Fatalf("details.testedRevision = %v, want 4", info.Details["testedRevision"])
	}
	if got, _ := info.UpstreamCode.(string); got != "DEBUG_TIMEOUT" {
		t.Fatalf("upstream_code = %v, want DEBUG_TIMEOUT", info.UpstreamCode)
	}
	if info.Hint != devEventCodeHints["DEBUG_TIMEOUT"] {
		t.Fatalf("hint = %q, want %q", info.Hint, devEventCodeHints["DEBUG_TIMEOUT"])
	}
}

// TestDevEventDebugErrorInfoWithoutUpstreamFacts 覆盖上游沉默时的退化：不合成
// testedRevision，保留调用方给出的 Hint。
func TestDevEventDebugErrorInfoWithoutUpstreamFacts(t *testing.T) {
	info := devEventDebugErrorInfo(map[string]any{}, "debug_not_ready", "信息", "动作")
	if info.Details != nil {
		t.Fatalf("details = %#v, want nil: CLI 不合成调试修订号", info.Details)
	}
	if info.UpstreamCode != nil {
		t.Fatalf("upstream_code = %v, want nil", info.UpstreamCode)
	}
	if info.Hint != "动作" {
		t.Fatalf("hint = %q, want 动作", info.Hint)
	}
}

// TestDevEventAppendDebugNextCommandRespectsUpstreamAndParams 覆盖三种情况：
// 上游已给 nextCommand 时不覆盖；参数不全时不补；补出的命令用真实入参。
func TestDevEventAppendDebugNextCommandRespectsUpstreamAndParams(t *testing.T) {
	upstream := map[string]any{"nextCommand": "  dws dev event get --unified-app-id X  "}
	devEventAppendDebugNextCommand(upstream, map[string]any{
		"unifiedAppId": "uapp_self_hr_A", "eventKey": "app_user_a_b_c", "expectedConfigRevision": 1,
	})
	if upstream["nextCommand"] != "  dws dev event get --unified-app-id X  " {
		t.Fatalf("nextCommand = %v, want upstream value preserved verbatim", upstream["nextCommand"])
	}

	for _, params := range []map[string]any{
		{},
		{"eventKey": "app_user_a_b_c", "expectedConfigRevision": 1},
		{"unifiedAppId": "uapp_self_hr_A", "expectedConfigRevision": 1},
		{"unifiedAppId": "uapp_self_hr_A", "eventKey": "app_user_a_b_c"},
		{"unifiedAppId": "uapp_self_hr_A", "eventKey": "app_user_a_b_c", "expectedConfigRevision": "4"},
	} {
		content := map[string]any{}
		devEventAppendDebugNextCommand(content, params)
		if _, present := content["nextCommand"]; present {
			t.Fatalf("nextCommand = %v with params %#v, want absent", content["nextCommand"], params)
		}
	}

	content := map[string]any{}
	devEventAppendDebugNextCommand(content, map[string]any{
		"unifiedAppId": "  uapp_self_hr_A ", "eventKey": " app_user_a_b_c ", "expectedConfigRevision": 7,
	})
	want := "dws dev event publish --unified-app-id uapp_self_hr_A --event-key app_user_a_b_c --expected-revision 7 --yes"
	if content["nextCommand"] != want {
		t.Fatalf("nextCommand = %v, want %q", content["nextCommand"], want)
	}
}

// TestDevEventListFailsClosedWithoutPaginationFacts 验证分页 fail-closed：
// 只有 list 声明了游标分页，缺 hasMore/nextCursor 时必须失败，不能把不完整
// 的一页当作全量结果；dry-run 预览不受此约束。
func TestDevEventListFailsClosedWithoutPaginationFacts(t *testing.T) {
	env := devEventEnvelopeOf(t, devEventCommandResult(
		devEventUpstreamResult(devEventListTool, false, nil, map[string]any{"items": []any{}})))
	if env.Outcome != output.OutcomeFailure {
		t.Fatalf("outcome = %q, want failure", env.Outcome)
	}
	if env.Error == nil || env.Error.Subtype != "pagination_inconsistent" {
		t.Fatalf("subtype = %#v, want pagination_inconsistent", env.Error)
	}
	if env.Error.Hint == "" {
		t.Fatal("hint empty, want 停止翻页的处置动作")
	}

	dryRun := devEventEnvelopeOf(t, devEventCommandResult(
		devEventUpstreamResult(devEventListTool, true, nil, map[string]any{"items": []any{}})))
	if dryRun.Outcome != output.OutcomeSuccess || !dryRun.DryRun {
		t.Fatalf("dry-run outcome/dry_run = %q/%v, want success/true", dryRun.Outcome, dryRun.DryRun)
	}

	for _, tool := range []string{devEventGetTool, devEventValidateTool, devEventOfflineTool} {
		if devEventToolRequiresPagination(tool) {
			t.Fatalf("devEventToolRequiresPagination(%q) = true, want false", tool)
		}
		single := devEventEnvelopeOf(t, devEventCommandResult(
			devEventUpstreamResult(tool, false, nil, map[string]any{"eventKey": "app_user_a_b_c"})))
		if single.Outcome != output.OutcomeSuccess {
			t.Fatalf("%s outcome = %q, want success without pagination facts", tool, single.Outcome)
		}
	}
	if !devEventToolRequiresPagination("  " + devEventListTool + "  ") {
		t.Fatal("devEventToolRequiresPagination(padded list tool) = false, want true")
	}
}

// TestDevEventListProjectsPaginationMeta 验证分页字段挂 meta 层且从 data 剥离，
// hasMore 是权威的耗尽信号。
func TestDevEventListProjectsPaginationMeta(t *testing.T) {
	cases := []struct {
		name      string
		content   map[string]any
		exhausted bool
		nextToken string
	}{
		{"可续跑", map[string]any{"hasMore": true, "nextCursor": " c2 "}, false, "c2"},
		{"已耗尽", map[string]any{"hasMore": false, "nextCursor": "terminal"}, true, ""},
		{"只有游标", map[string]any{"nextCursor": "c3"}, false, "c3"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := devEventEnvelopeOf(t, devEventCommandResult(
				devEventUpstreamResult(devEventListTool, false, nil, c.content)))
			if env.Outcome != output.OutcomeSuccess {
				t.Fatalf("outcome = %q, want success (error = %#v)", env.Outcome, env.Error)
			}
			if env.Meta == nil || env.Meta.Pagination == nil {
				t.Fatalf("meta.pagination = %#v, want projected", env.Meta)
			}
			pg := env.Meta.Pagination
			if pg.EndpointExhausted != c.exhausted || pg.NextToken != c.nextToken {
				t.Fatalf("pagination = %+v, want exhausted=%v token=%q", pg, c.exhausted, c.nextToken)
			}
			data, ok := env.Data.(map[string]any)
			if !ok {
				t.Fatalf("data = %#v, want object", env.Data)
			}
			for _, key := range []string{"hasMore", "nextCursor"} {
				if _, present := data[key]; present {
					t.Fatalf("data[%q] = %v, want stripped into meta.pagination", key, data[key])
				}
			}
		})
	}
}

// TestDevEventPaginationInconsistentUpstream 覆盖上游自相矛盾的两种载荷：
// hasMore=true 却无 cursor，以及 hasMore 非布尔。都必须失败而不是静默降级。
func TestDevEventPaginationInconsistentUpstream(t *testing.T) {
	cases := []struct {
		name    string
		content map[string]any
	}{
		{"hasMore 为真但缺 nextCursor", map[string]any{"hasMore": true}},
		{"hasMore 非布尔", map[string]any{"hasMore": "yes"}},
		{"nextCursor 非字符串", map[string]any{"nextCursor": 7}},
		{"nextCursor 为空且无耗尽信号", map[string]any{"nextCursor": "  "}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			env := devEventEnvelopeOf(t, devEventCommandResult(
				devEventUpstreamResult(devEventListTool, false, nil, c.content)))
			if env.Outcome != output.OutcomeFailure {
				t.Fatalf("outcome = %q, want failure", env.Outcome)
			}
			if env.Error == nil || env.Error.Subtype != "pagination_inconsistent" {
				t.Fatalf("subtype = %#v, want pagination_inconsistent", env.Error)
			}
		})
	}
}

// TestDevEventNonObjectPayloadStaysSuccess 覆盖上游返回数组等非对象载荷：
// 结果归一只在对象上做失败与阶段判定，其余载荷原样成功。
func TestDevEventNonObjectPayloadStaysSuccess(t *testing.T) {
	for _, payload := range []any{[]any{"app_user_a_b_c"}, "app_user_a_b_c", nil} {
		invocation := executor.NewHelperInvocation("dev event get", devEventProduct, devEventGetTool, nil)
		invocation.Implemented = true
		result := executor.Result{Invocation: invocation, Response: map[string]any{"content": payload}}
		env := devEventEnvelopeOf(t, devEventCommandResult(result))
		if env.Outcome != output.OutcomeSuccess {
			t.Fatalf("payload %#v outcome = %q, want success (error = %#v)", payload, env.Outcome, env.Error)
		}
	}
}
