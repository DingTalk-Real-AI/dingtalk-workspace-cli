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

func TestCrossPlatformCoverageAitableShareFormTerminalProjection(t *testing.T) {
	for _, key := range []string{"baseId", "tableId", "viewId", "enabled", "status", "cpSynced"} {
		for _, value := range []any{nil, "", []any{}} {
			data := validShareState()
			data[key] = value
			if got := AitableFormShareUpdateResult(data); got.Outcome() != output.OutcomePartialFailure {
				t.Fatalf("%s=%#v: outcome=%s", key, value, got.Outcome())
			}
		}
		data := validShareState()
		delete(data, key)
		if AitableFormShareUpdateResult(data).Outcome() != output.OutcomePartialFailure {
			t.Fatalf("missing %s accepted", key)
		}
	}
	for _, key := range []string{"shareFormUuid", "formCover", "formName", "formDesc"} {
		data := validShareState()
		data[key] = false
		if AitableFormShareUpdateResult(data).Outcome() != output.OutcomePartialFailure {
			t.Fatalf("invalid optional %s accepted", key)
		}
		data[key] = nil
		if AitableFormShareUpdateResult(data).Outcome() != output.OutcomeSuccess {
			t.Fatalf("nullable %s rejected", key)
		}
	}
	for _, status := range []any{float64(0), int(1), int64(2), json.Number("2")} {
		data := validShareState()
		data["status"] = status
		if AitableFormShareUpdateResult(data).Outcome() != output.OutcomeSuccess {
			t.Fatalf("integer status %#v rejected", status)
		}
	}
	for _, status := range []any{1.5, math.NaN(), math.Inf(1), json.Number("1.5")} {
		data := validShareState()
		data["status"] = status
		if AitableFormShareUpdateResult(data).Outcome() != output.OutcomePartialFailure {
			t.Fatalf("non-integer status %#v accepted", status)
		}
	}
	for _, data := range []any{nil, map[string]any(nil), []any{}, "bad", map[string]any{"cpSynced": false}} {
		result := AitableFormShareUpdateResult(data)
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
