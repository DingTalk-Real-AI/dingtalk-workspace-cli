package helpers

import (
	"bytes"
	"context"
	"encoding/json"
	"math"
	"reflect"
	"testing"

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

func TestCrossPlatformCoverageAitableShareFormTerminalProjection(t *testing.T) {
	for _, key := range []string{"baseId", "tableId", "viewId", "enabled", "status", "cpSynced"} {
		for _, value := range []any{nil, "", []any{}} {
			data := validShareState()
			data[key] = value
			if got := AitableFormShareUpdateResult(data, shareRequest()); got.Outcome() != output.OutcomePartialFailure {
				t.Fatalf("%s=%#v: outcome=%s", key, value, got.Outcome())
			}
		}
		data := validShareState()
		delete(data, key)
		if AitableFormShareUpdateResult(data, shareRequest()).Outcome() != output.OutcomePartialFailure {
			t.Fatalf("missing %s accepted", key)
		}
	}
	for _, key := range []string{"shareFormUuid", "formCover", "formName", "formDesc"} {
		data := validShareState()
		data[key] = false
		if AitableFormShareUpdateResult(data, shareRequest()).Outcome() != output.OutcomePartialFailure {
			t.Fatalf("invalid optional %s accepted", key)
		}
		data[key] = nil
		if AitableFormShareUpdateResult(data, shareRequest()).Outcome() != output.OutcomeSuccess {
			t.Fatalf("nullable %s rejected", key)
		}
	}
	for _, status := range []any{float64(0), int(1), int64(2), json.Number("2")} {
		data := validShareState()
		data["status"] = status
		if AitableFormShareUpdateResult(data, shareRequest()).Outcome() != output.OutcomeSuccess {
			t.Fatalf("integer status %#v rejected", status)
		}
	}
	for _, status := range []any{1.5, math.NaN(), math.Inf(1), json.Number("1.5")} {
		data := validShareState()
		data["status"] = status
		if AitableFormShareUpdateResult(data, shareRequest()).Outcome() != output.OutcomePartialFailure {
			t.Fatalf("non-integer status %#v accepted", status)
		}
	}
	for _, data := range []any{nil, map[string]any(nil), []any{}, "bad", map[string]any{"cpSynced": false}} {
		result := AitableFormShareUpdateResult(data, shareRequest())
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

func TestCrossPlatformCoverageAitableShareFormExactTarget(t *testing.T) {
	for index, key := range []string{"baseId", "tableId", "viewId"} {
		for _, expected := range []string{"", " \t", "another-form"} {
			request := shareRequest()
			request[key] = expected
			result := AitableFormShareUpdateResult(validShareState(), request)
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
			if AitableFormShareUpdateResult(data, shareRequest()).Outcome() != output.OutcomePartialFailure {
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
			result := AitableFormShareUpdateResult(data, shareRequest(map[string]any{tc.key: tc.requested}))
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
			state := shareStateFromResult(t, AitableFormShareUpdateResult(data, shareRequest(map[string]any{tc.key: tc.requested})))
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
		if AitableFormShareUpdateResult(data, shareRequest(map[string]any{key: "值"})).Outcome() != output.OutcomePartialFailure {
			t.Fatalf("requested %s missing from response accepted", key)
		}
	}
}

// The helper never echoes these properties, so success must declare them
// unverified instead of implying the whole request converged.
func TestCrossPlatformCoverageAitableShareFormUnverifiableRequestFields(t *testing.T) {
	state := shareStateFromResult(t, AitableFormShareUpdateResult(validShareState(), shareRequest()))
	if state["verified"] != true {
		t.Fatalf("target-only request not verified: %#v", state)
	}

	request := shareRequest(map[string]any{
		"anonymousSubmit": true, "submitTimesLimit": 3, "authData": "d",
		"shareUidList": "u", "enabled": false,
	})
	result := AitableFormShareUpdateResult(validShareState(), request)
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
				cmd, err := root.ExecuteContextC(ctx)
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
			cmd, err := root.ExecuteContextC(ctx)
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
			cmd, err := root.ExecuteContextC(ctx)
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
