package aitable

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
)

func TestCrossPlatformCoverageShareFormShortcutTerminalEnvelope(t *testing.T) {
	for _, key := range []string{"cpSynced", "baseId", "tableId", "viewId", "enabled", "status"} {
		for _, missing := range []bool{true, false} {
			t.Run(key+map[bool]string{true: "/missing", false: "/invalid"}[missing], func(t *testing.T) {
				data := map[string]any{"baseId": "b", "tableId": "t", "viewId": "v", "enabled": true, "status": float64(1), "cpSynced": true, "shareFormUuid": "share", "formCover": ""}
				if missing {
					delete(data, key)
				} else {
					data[key] = false
					if key == "enabled" {
						data[key] = "false"
					}
				}
				raw, _ := json.Marshal(map[string]any{"success": true, "data": data})
				caller := &platformCoverageCaller{response: string(raw)}
				helpers.InitDepsForTest(t, caller)
				root := newPlatformCoverageRoot()
				stdout, stderr := &bytes.Buffer{}, &bytes.Buffer{}
				root.SetOut(stdout)
				root.SetErr(stderr)
				root.SetArgs([]string{"aitable", "+form-share-update", "--base-id=b", "--table-id=t", "--view-id=v", "--enabled=true", "--yes", "--format=json"})
				cmd, err := root.ExecuteC()
				if err != nil {
					t.Fatal(err)
				}
				code, emitted, err := output.EmitStoredResult(cmd)
				if err != nil || !emitted || code != 7 || caller.callCount != 1 || stderr.Len() != 0 {
					t.Fatalf("code=%d emitted=%v err=%v calls=%d stderr=%s", code, emitted, err, caller.callCount, stderr)
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
				response := partial["succeeded"].([]any)[0].(map[string]any)["response"]
				if info["execution_started"] != true || !reflect.DeepEqual(response, data) {
					t.Fatalf("missing execution/response evidence: %s", stdout)
				}
			})
		}
	}
}
