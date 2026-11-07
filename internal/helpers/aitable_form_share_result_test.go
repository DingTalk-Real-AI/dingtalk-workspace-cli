package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func validShareState() map[string]any {
	return map[string]any{"baseId": "b", "tableId": "t", "viewId": "v", "enabled": false, "status": float64(2), "cpSynced": true, "shareFormUuid": "retained", "formCover": ""}
}

func shareRequest(extra ...map[string]any) map[string]any {
	request := map[string]any{"baseId": "b", "tableId": "t", "viewId": "v"}
	for _, overrides := range extra {
		for key, value := range overrides {
			request[key] = value
		}
	}
	return request
}

// Recovery identifiers belong to local diagnostics, never to the unmodified remote response.
func TestCrossPlatformCoverageAitableShareFormPartialCoverResource(t *testing.T) {
	for _, resource := range []any{"rid-cover", "", " \t", nil, 123} {
		data := validShareState()
		data["cpSynced"] = false
		result := AitableFormShareUpdateResult(map[string]any{"data": data}, shareRequest(map[string]any{"formCoverResourceId": resource}))
		env, err := output.EnvelopeFromResult(result)
		if err != nil || result.ExitCode() != 7 || env.OK || env.Outcome != output.OutcomePartialFailure {
			t.Fatalf("partial failure changed: result=%#v err=%v", env, err)
		}
		partial := env.Data.(*output.PartialData)
		info := partial.Failed[0].Error
		id, present := info.Details["formCoverResourceId"]
		if present != (resource == "rid-cover") || (present && id != resource) {
			t.Fatalf("wrong recovery resource: input=%#v details=%#v", resource, info.Details)
		}
		if info.Retryable || !*info.ExecutionStarted || !strings.Contains(info.Hint, "不自动重放") || !reflect.DeepEqual(partial.Succeeded[0].(map[string]any)["response"], data) {
			t.Fatalf("unsafe or altered recovery evidence: %#v", partial)
		}
	}
}

func TestCrossPlatformCoverageAitableShareFormTerminalProjection(t *testing.T) {
	for _, key := range []string{"baseId", "tableId", "viewId", "enabled", "status", "cpSynced"} {
		for _, value := range []any{nil, "", []any{}} {
			data := validShareState()
			data[key] = value
			if got := AitableFormShareUpdateResult(map[string]any{"data": data}, shareRequest()); got.Outcome() != output.OutcomePartialFailure {
				t.Fatalf("%s=%#v: outcome=%s", key, value, got.Outcome())
			}
		}
		data := validShareState()
		delete(data, key)
		if AitableFormShareUpdateResult(map[string]any{"data": data}, shareRequest()).Outcome() != output.OutcomePartialFailure {
			t.Fatalf("missing %s accepted", key)
		}
	}
	for _, key := range []string{"shareFormUuid", "formCover", "formName", "formDesc"} {
		data := validShareState()
		data[key] = false
		if AitableFormShareUpdateResult(map[string]any{"data": data}, shareRequest()).Outcome() != output.OutcomePartialFailure {
			t.Fatalf("invalid optional %s accepted", key)
		}
		data[key] = nil
		if AitableFormShareUpdateResult(map[string]any{"data": data}, shareRequest()).Outcome() != output.OutcomeSuccess {
			t.Fatalf("nullable %s rejected", key)
		}
	}
	for _, tc := range []struct {
		status  any
		enabled bool
	}{{float64(0), false}, {int(1), true}, {int64(2), false}, {json.Number("2"), false}, {json.Number("-1"), false}} {
		data := validShareState()
		data["status"], data["enabled"] = tc.status, tc.enabled
		if AitableFormShareUpdateResult(map[string]any{"data": data}, shareRequest()).Outcome() != output.OutcomeSuccess {
			t.Fatalf("integer status %#v rejected", tc.status)
		}
	}
	for _, status := range []any{1.5, math.NaN(), math.Inf(1), json.Number("1.5")} {
		data := validShareState()
		data["status"] = status
		if AitableFormShareUpdateResult(map[string]any{"data": data}, shareRequest()).Outcome() != output.OutcomePartialFailure {
			t.Fatalf("non-integer status %#v accepted", status)
		}
	}
	for _, data := range []any{nil, map[string]any(nil), []any{}, "bad", map[string]any{"cpSynced": false}} {
		result := AitableFormShareUpdateResult(map[string]any{"data": data}, shareRequest())
		env, err := output.EnvelopeFromResult(result)
		if err != nil || env.OK || result.ExitCode() != 7 {
			t.Fatalf("invalid data=%#v: result=%#v err=%v", data, env, err)
		}
		partial := env.Data.(*output.PartialData)
		if !reflect.DeepEqual(partial.Succeeded[0].(map[string]any)["response"], data) {
			t.Fatal("remote response was changed")
		}
		info := partial.Failed[0].Error
		if info.ExecutionStarted == nil || !*info.ExecutionStarted || info.Retryable || info.Stage != "response_validation" {
			t.Fatalf("unsafe recovery metadata: %#v", info)
		}
	}
}

// A structurally valid response cannot prove a terminal state when its two
// representations of the share switch contradict each other.
func TestCrossPlatformCoverageAitableShareFormContradictoryStatus(t *testing.T) {
	for _, pair := range []struct {
		enabled bool
		status  int
	}{{true, 2}, {true, 0}, {true, 3}, {false, 1}} {
		data := validShareState()
		data["enabled"], data["status"] = pair.enabled, pair.status
		result := AitableFormShareUpdateResult(map[string]any{"data": data}, shareRequest())
		env, err := output.EnvelopeFromResult(result)
		if err != nil || env.OK || result.ExitCode() != 7 {
			t.Fatalf("contradictory state accepted: enabled=%t status=%d env=%#v err=%v", pair.enabled, pair.status, env, err)
		}
		partial := env.Data.(*output.PartialData)
		if !reflect.DeepEqual(partial.Succeeded[0].(map[string]any)["response"], data) || partial.Failed[0].Error.Retryable {
			t.Fatalf("unsafe or altered recovery evidence: %#v", partial)
		}
	}
}

func TestCrossPlatformCoverageAitableShareFormExactTarget(t *testing.T) {
	for index, key := range []string{"baseId", "tableId", "viewId"} {
		for _, expected := range []string{"", " \t", "another-form"} {
			request := shareRequest()
			request[key] = expected
			result := AitableFormShareUpdateResult(map[string]any{"data": validShareState()}, request)
			env, err := output.EnvelopeFromResult(result)
			if err != nil || env.OK || result.ExitCode() != 7 {
				t.Fatalf("request %s=%q accepted: result=%#v err=%v", key, expected, env, err)
			}
			partial := env.Data.(*output.PartialData)
			if !reflect.DeepEqual(partial.Failed[0].Error.Details["invalid_fields"], []string{key}) {
				t.Fatalf("missing mismatched field %s: %#v", key, partial)
			}
		}
		for _, returned := range []string{" \t", " " + []string{"b", "t", "v"}[index] + " "} {
			data := validShareState()
			data[key] = returned
			if AitableFormShareUpdateResult(map[string]any{"data": data}, shareRequest()).Outcome() != output.OutcomePartialFailure {
				t.Fatalf("non-exact %s=%q accepted", key, returned)
			}
		}
	}
}

func shareStateFromResult(t *testing.T, result output.CommandResult) map[string]any {
	t.Helper()
	env, err := output.EnvelopeFromResult(result)
	if err != nil || !env.OK {
		t.Fatalf("expected verified success: env=%#v err=%v", env, err)
	}
	state, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("terminal state is not an object: %#v", env.Data)
	}
	return state
}

// The response echoes enabled/formName/formDesc, so a converged state carrying a
// different value than the one requested is not the update that was asked for.
func TestCrossPlatformCoverageAitableShareFormRequestedValuesVerified(t *testing.T) {
	for _, tc := range []struct {
		key       string
		requested any
		returned  any
	}{
		{"enabled", true, false},
		{"enabled", false, true},
		{"formName", "活动报名", "旧标题"},
		{"formDesc", "报名说明", ""},
	} {
		t.Run(tc.key, func(t *testing.T) {
			data := validShareState()
			data[tc.key] = tc.returned
			if tc.key == "enabled" && tc.returned == true {
				data["status"] = 1
			}
			result := AitableFormShareUpdateResult(map[string]any{"data": data}, shareRequest(map[string]any{tc.key: tc.requested}))
			env, err := output.EnvelopeFromResult(result)
			if err != nil || env.OK || result.ExitCode() != 7 {
				t.Fatalf("mismatched %s accepted: result=%#v err=%v", tc.key, env, err)
			}
			partial := env.Data.(*output.PartialData)
			if !reflect.DeepEqual(partial.Failed[0].Error.Details["invalid_fields"], []string{tc.key}) {
				t.Fatalf("missing mismatched %s: %#v", tc.key, partial.Failed[0].Error.Details)
			}
			if !reflect.DeepEqual(partial.Succeeded[0].(map[string]any)["response"], data) {
				t.Fatal("remote response was changed")
			}

			data[tc.key] = tc.requested
			if tc.key == "enabled" {
				data["status"] = 2
				if tc.requested == true {
					data["status"] = 1
				}
			}
			state := shareStateFromResult(t, AitableFormShareUpdateResult(map[string]any{"data": data}, shareRequest(map[string]any{tc.key: tc.requested})))
			if state["verified"] != true {
				t.Fatalf("provable %s not reported as verified: %#v", tc.key, state)
			}
			if _, exists := state["unverified"]; exists {
				t.Fatalf("empty unverified must be omitted: %#v", state)
			}
		})
	}
	// A requested value the response never echoes stays absent from the response;
	// omitting it is not proof, so it must not be silently accepted as verified.
	for _, key := range []string{"formName", "formDesc"} {
		data := validShareState()
		delete(data, key)
		if AitableFormShareUpdateResult(map[string]any{"data": data}, shareRequest(map[string]any{key: "值"})).Outcome() != output.OutcomePartialFailure {
			t.Fatalf("requested %s missing from response accepted", key)
		}
	}
}

// Older helper responses omit these properties, so success must declare them
// unverified instead of implying the whole request converged.
func TestCrossPlatformCoverageAitableShareFormUnverifiableRequestFields(t *testing.T) {
	state := shareStateFromResult(t, AitableFormShareUpdateResult(map[string]any{"data": validShareState()}, shareRequest()))
	if state["verified"] != true {
		t.Fatalf("target-only request not verified: %#v", state)
	}

	request := shareRequest(map[string]any{
		"anonymousSubmit": true, "submitTimesLimit": 3, "authData": "d",
		"shareUidList": "u", "enabled": false,
	})
	result := AitableFormShareUpdateResult(map[string]any{"data": validShareState()}, request)
	if result.Outcome() != output.OutcomeSuccess {
		t.Fatalf("unverifiable fields must not fail the write: %s", result.Outcome())
	}
	state = shareStateFromResult(t, result)
	if state["verified"] != false {
		t.Fatalf("unverifiable fields reported as verified: %#v", state)
	}
	want := []string{"anonymousSubmit", "authData", "shareUidList", "submitTimesLimit"}
	if !reflect.DeepEqual(state["unverified"], want) {
		t.Fatalf("unverified=%#v want %#v", state["unverified"], want)
	}
	for key, value := range validShareState() {
		if !reflect.DeepEqual(state[key], value) {
			t.Fatalf("server state %s was rewritten: %#v", key, state[key])
		}
	}
}

// Exercise actual public MCP field representations, including JSON numbers,
// missing legacy fields and contradictory read-back values.
func TestCrossPlatformCoverageAitableShareFormExpandedReadback(t *testing.T) {
	for _, tc := range []struct {
		key       string
		requested any
		returned  any
		wrong     any
	}{
		{"authTypeCode", 6, float64(6), float64(1)},
		{"anonymousSubmit", true, true, false},
		{"loadLastSubmit", false, false, true},
		{"submitRecordViewMode", 1, float64(1), float64(0)},
		{"replyNotice", true, true, false},
		{"formStartTime", int64(1790038800000), float64(1790038800000), float64(1790038700000)},
		{"formEndTime", int64(0), float64(0), float64(1790038800000)},
		{"submitTimesUserLimit", 2, json.Number("2"), float64(1)},
		{"formCover", "https://example.test/new.png", "https://example.test/new.png", "https://example.test/old.png"},
		{"shareUidList", " user-a, user-b,user-a ", "user-b,user-a", "user-a"},
		{"shareUidList", "", "", "user-a"},
		{"authData", "user-a, user-b,user-a", `{"members":[{"type":"user","userId":"user-b","corpId":"corp"},{"type":"user","userId":"user-a","corpId":"corp"}]}`, `{"members":[{"type":"user","userId":"other","corpId":"corp"}]}`},
	} {
		t.Run(tc.key, func(t *testing.T) {
			request := shareRequest(map[string]any{tc.key: tc.requested})
			for _, returned := range []any{tc.returned, tc.wrong, []any{}} {
				data := validShareState()
				data[tc.key] = returned
				result := AitableFormShareUpdateResult(map[string]any{"data": data}, request)
				if reflect.DeepEqual(returned, tc.returned) {
					state := shareStateFromResult(t, result)
					if state["verified"] != true || state["unverified"] != nil || !reflect.DeepEqual(state[tc.key], returned) {
						t.Fatalf("matching readback was changed or left unverified: %#v", state)
					}
				} else if result.Outcome() != output.OutcomePartialFailure || result.ExitCode() != 7 {
					t.Fatalf("contradictory %s readback accepted: %#v", tc.key, result.Data())
				}
			}
			// Older servers can omit newly echoed fields. Keep uncertainty explicit.
			if tc.key != "formCover" {
				for _, missing := range []bool{true, false} {
					data := validShareState()
					if !missing {
						data[tc.key] = nil
					}
					state := shareStateFromResult(t, AitableFormShareUpdateResult(map[string]any{"data": data}, request))
					if state["verified"] != false || !reflect.DeepEqual(state["unverified"], []string{tc.key}) {
						t.Fatalf("unavailable field claimed as verified: %#v", state)
					}
				}
			}
		})
	}
}

func TestCrossPlatformCoverageAitableShareFormReadbackNormalization(t *testing.T) {
	for _, tc := range []struct {
		key       string
		requested any
		returned  any
		verified  bool
		partial   bool
	}{
		{"submitTimesLimit", 1000000, float64(10000), true, false},
		{"submitTimesLimit", -1, float64(0), true, false},
		{"submitTimesLimit", 0, float64(0), true, false},
		{"submitTimesLimit", 10, float64(0), false, true},
		{"submitTimesLimit", 10, float64(11), false, true},
		{"submitTimesLimit", 10, float64(-1), false, true},
		{"authTypeCode", -1, float64(2), true, false},
		{"submitTimesUserLimit", -1, float64(1), true, false},
		{"formStartTime", int64(123), 123.5, false, true},
		{"formStartTime", int64(123), math.Inf(1), false, true},
		{"formStartTime", int64(123), -float64(math.MinInt64), false, true},
		{"formStartTime", int64(123), json.Number("bad"), false, true},
		{"formStartTime", "123", float64(123), false, true},
		{"shareUidList", "user-a", "user-a,", false, true},
		{"authData", "user-a", `{"members":[{"type":"staff","userId":"user-a","corpId":"corp"}]}`, true, false},
		{"authData", "user-a", `{"members":[{"type":"user","id":"internal-uid"}]}`, false, false},
		{"authData", "user-a", `{"members":[{"type":"user","userId":"user-a"}]}`, false, false},
		{"authData", "user-a", `{"members":[]}`, false, true},
		{"authData", "user-a", `{"members":[{"type":"dept","id":"dept"}]}`, false, true},
		{"authData", "user-a,user-b", `{"members":[{"type":"user","userId":"user-a","corpId":"a"},{"type":"user","userId":"user-b","corpId":"b"}]}`, false, true},
		{"authData", "user-a", `{"members":false}`, false, true},
		{"authData", "user-a", `null`, false, true},
		{"authData", "", "", true, false},
		{"authData", "", `{"members":[]}`, false, true},
		{"authData", "user-a", `bad json`, false, true},
		{"authData", "user-a,", `{"members":[]}`, false, true},
		{"formCoverResourceId", "rid", nil, false, false},
	} {
		data := validShareState()
		data[tc.key] = tc.returned
		result := AitableFormShareUpdateResult(map[string]any{"data": data}, shareRequest(map[string]any{tc.key: tc.requested}))
		if tc.partial {
			if result.Outcome() != output.OutcomePartialFailure {
				t.Fatalf("%s=%#v accepted: %#v", tc.key, tc.returned, result.Data())
			}
			continue
		}
		state := shareStateFromResult(t, result)
		if state["verified"] != tc.verified {
			t.Fatalf("%s=%#v verification changed: %#v", tc.key, tc.returned, state)
		}
	}
}

func TestCrossPlatformCoverageAitableShareFormWarningProjection(t *testing.T) {
	meta := map[string]any{"suggestions": []any{
		map[string]any{"reason": " historical member unavailable ", "action": " do not clear members ", "priority": "medium", "internal": "must not leak"},
		false, map[string]any{"reason": false}, map[string]any{"priority": "high"},
		map[string]any{"action": "inspect settings"},
	}}
	data := validShareState()
	data["authData"] = nil
	want := []formShareWarning{{Reason: "historical member unavailable", Action: "do not clear members", Priority: "medium"}, {Action: "inspect settings"}}
	response := map[string]any{"data": data, "meta": meta}
	read, err := AitableFormShareGetResult(response)
	if err != nil {
		t.Fatal(err)
	}
	state := shareStateFromResult(t, read)
	if !reflect.DeepEqual(state["warnings"], want) || state["authData"] != nil {
		t.Fatalf("read warning or original state lost: %#v", state)
	}
	write := AitableFormShareUpdateResult(response, shareRequest(map[string]any{"authData": "user-a"}))
	state = shareStateFromResult(t, write)
	if state["verified"] != false || !reflect.DeepEqual(state["warnings"], want) {
		t.Fatalf("write warning or uncertainty lost: %#v", state)
	}
	data["cpSynced"] = false
	partial := AitableFormShareUpdateResult(response, shareRequest())
	env, _ := output.EnvelopeFromResult(partial)
	details := env.Data.(*output.PartialData).Failed[0].Error.Details
	if !reflect.DeepEqual(details["warnings"], want) || data["warnings"] != nil {
		t.Fatalf("partial warning lost or upstream response mutated: %#v", details)
	}
	for _, badMeta := range []any{nil, false, map[string]any{"suggestions": false}} {
		read, err := AitableFormShareGetResult(map[string]any{"data": data, "meta": badMeta})
		if err != nil || shareStateFromResult(t, read)["warnings"] != nil {
			t.Fatalf("invalid suggestions manufactured a warning: %v", err)
		}
	}
	for _, badData := range []any{nil, false, []any{}, map[string]any(nil)} {
		if _, err := AitableFormShareGetResult(map[string]any{"data": badData}); err == nil {
			t.Fatalf("malformed read data accepted: %#v", badData)
		}
	}
}

func TestCrossPlatformCoverageAitableShareFormAtomicTerminalEnvelope(t *testing.T) {
	for _, key := range []string{"cpSynced", "baseId", "tableId", "viewId", "enabled", "status"} {
		for _, missing := range []bool{true, false} {
			t.Run(key+map[bool]string{true: "/missing", false: "/invalid"}[missing], func(t *testing.T) {
				data := validShareState()
				if missing {
					delete(data, key)
				} else {
					data[key] = false
					if key == "enabled" {
						data[key] = "false"
					}
				}
				raw, _ := json.Marshal(map[string]any{"success": true, "data": data})
				caller := &aitableTestCaller{responses: []string{string(raw)}}
				InitDepsForTest(t, caller)
				root := newAitableCommand()
				installExampleGlobalFlags(root)
				stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
				root.SetOut(stdout)
				root.SetErr(stderr)
				root.SetArgs([]string{"form", "share", "update", "--base-id=b", "--table-id=t", "--view-id=v", "--enabled=true", "--format=json"})
				ctx, _ := output.WithResultStore(context.Background())
				cmd, err := corecmd.ExecuteContextCForTest(root, ctx)
				if err != nil {
					t.Fatal(err)
				}
				code, emitted, err := output.EmitStoredResult(cmd)
				if err != nil || !emitted || code != 7 || len(caller.calls) != 1 || stderr.Len() != 0 {
					t.Fatalf("code=%d emitted=%v err=%v calls=%v stderr=%s", code, emitted, err, caller.calls, stderr)
				}
				var envelope map[string]any
				if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
					t.Fatal(err)
				}
				if envelope["ok"] != false || envelope["outcome"] != "partial_failure" || envelope["error"] != nil {
					t.Fatalf("bad envelope: %s", stdout)
				}
				partial := envelope["data"].(map[string]any)
				info := partial["failed"].([]any)[0].(map[string]any)["error"].(map[string]any)
				if info["execution_started"] != true {
					t.Fatalf("missing execution evidence: %s", stdout)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageAitableShareFormAtomicRequestTarget(t *testing.T) {
	for _, key := range []string{"", "baseId", "tableId", "viewId"} {
		t.Run("mismatch/"+key, func(t *testing.T) {
			data := validShareState()
			if key != "" {
				data[key] = "another-form"
			}
			raw, _ := json.Marshal(map[string]any{"success": true, "data": data})
			caller := &aitableTestCaller{responses: []string{string(raw)}}
			InitDepsForTest(t, caller)
			root := newAitableCommand()
			installExampleGlobalFlags(root)
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			root.SetOut(stdout)
			root.SetErr(stderr)
			root.SetArgs([]string{"form", "share", "update", "--base-id=b", "--table-id=t", "--view-id=v", "--enabled=false", "--format=json"})
			ctx, _ := output.WithResultStore(context.Background())
			cmd, err := corecmd.ExecuteContextCForTest(root, ctx)
			if err != nil {
				t.Fatal(err)
			}
			code, emitted, err := output.EmitStoredResult(cmd)
			if err != nil || !emitted || len(caller.calls) != 1 || stderr.Len() != 0 {
				t.Fatalf("code=%d emitted=%v err=%v calls=%v stderr=%s", code, emitted, err, caller.calls, stderr)
			}
			call := caller.calls[0]
			wantArgs := map[string]any{"baseId": "b", "tableId": "t", "viewId": "v", "enabled": false}
			if call.server != "aitable-helper" || call.tool != "update_share_form" || !reflect.DeepEqual(call.args, wantArgs) {
				t.Fatalf("request target changed: %#v", call)
			}
			var envelope map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if key == "" {
				want := validShareState()
				want["verified"] = true
				if code != 0 || envelope["ok"] != true || envelope["outcome"] != "success" || !reflect.DeepEqual(envelope["data"], want) {
					t.Fatalf("matching target rejected: %s", stdout)
				}
				return
			}
			if code != 7 || envelope["ok"] != false || envelope["outcome"] != "partial_failure" || envelope["error"] != nil {
				t.Fatalf("wrong target accepted: code=%d envelope=%s", code, stdout)
			}
			partial := envelope["data"].(map[string]any)
			info := partial["failed"].([]any)[0].(map[string]any)["error"].(map[string]any)
			response := partial["succeeded"].([]any)[0].(map[string]any)["response"]
			invalid := info["details"].(map[string]any)["invalid_fields"]
			if info["execution_started"] != true || info["stage"] != "response_validation" || !reflect.DeepEqual(response, data) || !reflect.DeepEqual(invalid, []any{key}) {
				t.Fatalf("wrong target evidence lost: %s", stdout)
			}
		})
	}
}

// A response for the requested target that contradicts an explicitly requested
// value is not a verified update, and a requested property the response never
// echoes must be surfaced as unverified rather than claimed as converged.
func TestCrossPlatformCoverageAitableShareFormAtomicRequestedValue(t *testing.T) {
	for _, tc := range []struct {
		name       string
		flag       string
		returned   map[string]any
		invalid    []any
		unverified []any
	}{
		{name: "enabled/mismatch", flag: "--enabled=true", returned: map[string]any{"enabled": false}, invalid: []any{"enabled"}},
		{name: "formName/mismatch", flag: "--form-name=活动报名", returned: map[string]any{"formName": "旧标题"}, invalid: []any{"formName"}},
		{name: "formDesc/missing", flag: "--form-desc=报名说明", returned: map[string]any{"formDesc": nil}, invalid: []any{"formDesc"}},
		{name: "anonymousSubmit/unverifiable", flag: "--anonymous-submit=true", unverified: []any{"anonymousSubmit"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data := validShareState()
			for key, value := range tc.returned {
				data[key] = value
			}
			raw, _ := json.Marshal(map[string]any{"success": true, "data": data})
			caller := &aitableTestCaller{responses: []string{string(raw)}}
			InitDepsForTest(t, caller)
			root := newAitableCommand()
			installExampleGlobalFlags(root)
			stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
			root.SetOut(stdout)
			root.SetErr(stderr)
			root.SetArgs([]string{"form", "share", "update", "--base-id=b", "--table-id=t", "--view-id=v", tc.flag, "--format=json"})
			ctx, _ := output.WithResultStore(context.Background())
			cmd, err := corecmd.ExecuteContextCForTest(root, ctx)
			if err != nil {
				t.Fatal(err)
			}
			code, emitted, err := output.EmitStoredResult(cmd)
			if err != nil || !emitted || len(caller.calls) != 1 || stderr.Len() != 0 {
				t.Fatalf("code=%d emitted=%v err=%v calls=%v stderr=%s", code, emitted, err, caller.calls, stderr)
			}
			var envelope map[string]any
			if err := json.Unmarshal(stdout.Bytes(), &envelope); err != nil {
				t.Fatal(err)
			}
			if tc.unverified != nil {
				state := envelope["data"].(map[string]any)
				if code != 0 || envelope["outcome"] != "success" || state["verified"] != false {
					t.Fatalf("unverifiable request field not declared: code=%d envelope=%s", code, stdout)
				}
				if !reflect.DeepEqual(state["unverified"], tc.unverified) {
					t.Fatalf("unverified=%#v want %#v", state["unverified"], tc.unverified)
				}
				return
			}
			if code != 7 || envelope["ok"] != false || envelope["outcome"] != "partial_failure" || envelope["error"] != nil {
				t.Fatalf("contradicted request value accepted: code=%d envelope=%s", code, stdout)
			}
			partial := envelope["data"].(map[string]any)
			info := partial["failed"].([]any)[0].(map[string]any)["error"].(map[string]any)
			response := partial["succeeded"].([]any)[0].(map[string]any)["response"]
			if !reflect.DeepEqual(info["details"].(map[string]any)["invalid_fields"], tc.invalid) {
				t.Fatalf("missing mismatched field evidence: %s", stdout)
			}
			if info["execution_started"] != true || info["stage"] != "response_validation" || !reflect.DeepEqual(response, data) {
				t.Fatalf("recovery evidence lost: %s", stdout)
			}
		})
	}
}
