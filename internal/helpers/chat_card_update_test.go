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
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func runNativeCardUpdate(t *testing.T, caller *scriptedToolCaller, args ...string) error {
	t.Helper()
	installScriptedCaller(t, caller)
	root := newChatCommand()
	root.SilenceErrors = true
	root.SilenceUsage = true
	if root.PersistentFlags().Lookup("dry-run") == nil {
		root.PersistentFlags().Bool("dry-run", false, "preview without executing")
	}
	if root.PersistentFlags().Lookup("yes") == nil {
		root.PersistentFlags().Bool("yes", false, "skip confirmation")
	}
	root.SetArgs(args)
	return root.Execute()
}

func TestCrossPlatformCoverageNativeMessageUpdateCardVerifiesWrite(t *testing.T) {
	t.Run("atomic command preserves no-extra-confirmation contract", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":{"bizId":"biz-confirm","updated":true}}`}}}
		if err := runNativeCardUpdate(t, caller,
			"message", "update-card",
			"--biz-id", "biz-confirm",
			"--content", "原子更新",
			"--flow-status", "3",
		); err != nil {
			t.Fatal(err)
		}
		wantArgs := map[string]any{
			"bizId":      "biz-confirm",
			"msgContent": "原子更新",
			"flowStatus": 3,
		}
		if caller.calls != 1 || caller.server != "im" || caller.tool != "update_streaming_card" || !reflect.DeepEqual(caller.args, wantArgs) {
			t.Fatalf("atomic call = count:%d server:%q tool:%q args:%#v", caller.calls, caller.server, caller.tool, caller.args)
		}
	})

	t.Run("explicit evidence succeeds", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":{"bizId":"biz-1","updated":true}}`}}}
		err := runNativeCardUpdate(t, caller,
			"message", "update-card",
			"--biz-id", "biz-1",
			"--content", "完成",
			"--flow-status", "3",
		)
		if err != nil {
			t.Fatal(err)
		}
		if caller.calls != 1 || caller.server != "im" || caller.tool != "update_streaming_card" {
			t.Fatalf("call = count:%d server:%q tool:%q", caller.calls, caller.server, caller.tool)
		}
		if caller.args["bizId"] != "biz-1" {
			t.Fatalf("args = %#v", caller.args)
		}
	})

	t.Run("success acknowledgement is verified", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true,"errorCode":null}`}}}
		err := runNativeCardUpdate(t, caller,
			"message", "update-card",
			"--biz-id", "not-a-real-card",
			"--content", "完成",
			"--flow-status", "3",
		)
		if err != nil {
			t.Fatal(err)
		}
		if caller.calls != 1 || caller.tool != "update_streaming_card" {
			t.Fatalf("call = count:%d tool:%q", caller.calls, caller.tool)
		}
	})

	t.Run("lower write error is preserved", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{err: errors.New("write unavailable")}}}
		err := runNativeCardUpdate(t, caller,
			"message", "update-card",
			"--biz-id", "biz-1",
			"--content", "完成",
			"--flow-status", "3",
		)
		if err == nil {
			t.Fatal("lower write error was ignored")
		}
	})

	for _, test := range []struct {
		name       string
		response   string
		wantReason string
	}{
		{name: "empty response", response: "", wantReason: "streaming_card_update_unverified"},
		{name: "invalid response", response: "{", wantReason: "streaming_card_update_response_invalid"},
		{name: "not applied", response: `{"result":{"updated":false}}`, wantReason: "streaming_card_update_not_applied"},
		{name: "biz id drift", response: `{"result":{"bizId":"biz-other","updated":true}}`, wantReason: "streaming_card_update_biz_id_mismatch"},
	} {
		t.Run(test.name, func(t *testing.T) {
			caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: test.response}}}
			err := runNativeCardUpdate(t, caller,
				"message", "update-card",
				"--biz-id", "biz-1",
				"--content", "完成",
				"--flow-status", "3",
			)
			var typed *apperrors.Error
			if !errors.As(err, &typed) || typed.Reason != test.wantReason {
				t.Fatalf("error = %#v, want reason %q", err, test.wantReason)
			}
		})
	}

	t.Run("invalid arguments make no call", func(t *testing.T) {
		for _, args := range [][]string{
			{"message", "update-card", "--biz-id", "<bizId>", "--content", "完成", "--flow-status", "3"},
			{"message", "update-card", "--biz-id", "biz-1", "--content", "完成", "--flow-status", "6"},
		} {
			caller := &scriptedToolCaller{}
			if err := runNativeCardUpdate(t, caller, args...); err == nil {
				t.Fatalf("args %v unexpectedly succeeded", args)
			}
			if caller.calls != 0 {
				t.Fatalf("args %v made %d calls", args, caller.calls)
			}
		}
	})

	t.Run("dry run publishes unverified plan without write", func(t *testing.T) {
		caller := &scriptedToolCaller{}
		err := runNativeCardUpdate(t, caller,
			"message", "update-card",
			"--biz-id", "biz-preview",
			"--content", "完成",
			"--flow-status", "3",
			"--dry-run",
		)
		if err != nil {
			t.Fatal(err)
		}
		if caller.calls != 0 {
			t.Fatalf("dry-run made %d calls", caller.calls)
		}
	})
}

func TestCrossPlatformCoverageNativeMessageUpdateCardA2UIEngine(t *testing.T) {
	t.Run("a2ui payload uses a2ui update tool", func(t *testing.T) {
		caller := &scriptedToolCaller{}
		err := runNativeCardUpdate(t, caller,
			"message", "update-a2ui-card",
			"--biz-id", "biz-1",
			"--content", "[\"message1\",\"message2\"]",
			"--flow-status", "FINISH",
		)
		if err != nil {
			t.Fatal(err)
		}
		messages, ok := caller.args["a2uiMessages"].([]string)
		wantArgs := map[string]any{
			"bizId":        "biz-1",
			"flowStatus":   "FINISH",
			"a2uiMessages": []string{"message1", "message2"},
		}
		if caller.calls != 1 || caller.server != "im" || caller.tool != "update_a2ui_card" || !ok || !reflect.DeepEqual(messages, wantArgs["a2uiMessages"]) {
			t.Fatalf("a2ui call = count:%d server:%q tool:%q args:%#v", caller.calls, caller.server, caller.tool, caller.args)
		}
		if caller.args["bizId"] != wantArgs["bizId"] || caller.args["flowStatus"] != wantArgs["flowStatus"] || caller.args["requestId"] == "" {
			t.Fatalf("a2ui args = %#v", caller.args)
		}
		if annotations, ok := caller.args["a2uiAnnotations"].([]any); !ok || len(annotations) != 0 {
			t.Fatalf("a2uiAnnotations = %#v", caller.args["a2uiAnnotations"])
		}
	})

	t.Run("a2ui maps numeric flow status to enum", func(t *testing.T) {
		for status, want := range map[int]string{
			1: "PROCESSING", 2: "INPUTTING", 3: "FINISH", 4: "EXECUTING", 5: "ERROR",
			6: "ABORTED", 7: "TIMEOUT", 8: "CONFIRMING", 9: "CONFIRMED",
		} {
			caller := &scriptedToolCaller{}
			err := runNativeCardUpdate(t, caller,
				"message", "update-a2ui-card",
				"--biz-id", "biz-1",
				"--content", "[\"message\"]",
				"--flow-status", fmt.Sprintf("%d", status),
			)
			if err != nil {
				t.Fatal(err)
			}
			if caller.calls != 1 || caller.tool != "update_a2ui_card" || caller.args["flowStatus"] != want {
				t.Fatalf("status %d: call = count:%d tool:%q flowStatus:%#v", status, caller.calls, caller.tool, caller.args["flowStatus"])
			}
		}
	})

	t.Run("streaming keeps old status range", func(t *testing.T) {
		caller := &scriptedToolCaller{}
		err := runNativeCardUpdate(t, caller,
			"message", "update-card",
			"--biz-id", "biz-1",
			"--content", "完成",
			"--flow-status", "6",
		)
		if err == nil || !strings.Contains(err.Error(), "1-5") {
			t.Fatalf("error = %v, want streaming status range", err)
		}
		if caller.calls != 0 {
			t.Fatalf("invalid streaming status made %d calls", caller.calls)
		}
	})

	t.Run("streaming preserves native int spellings", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":{"bizId":"biz-1","updated":true}}`}}}
		err := runNativeCardUpdate(t, caller,
			"message", "update-card",
			"--biz-id", "biz-1",
			"--content", "完成",
			"--flow-status", "0x3",
		)
		if err != nil {
			t.Fatal(err)
		}
		if caller.calls != 1 || caller.tool != "update_streaming_card" || caller.args["flowStatus"] != 3 {
			t.Fatalf("call = count:%d tool:%q flowStatus:%#v", caller.calls, caller.tool, caller.args["flowStatus"])
		}
	})

	t.Run("a2ui validates content and status before call", func(t *testing.T) {
		tests := [][]string{
			{"--content", "plain", "--flow-status", "1"},
			{"--content", "[\"message\"]", "--flow-status", "0"},
			{"--content", "[\"message\"]", "--flow-status", "10"},
			{"--content", "[1]", "--flow-status", "1"},
			{"--content", "[]", "--flow-status", "1"},
		}
		for _, extra := range tests {
			caller := &scriptedToolCaller{}
			args := []string{"message", "update-a2ui-card", "--biz-id", "biz-1"}
			args = append(args, extra...)
			err := runNativeCardUpdate(t, caller, args...)
			if err == nil {
				t.Fatalf("args %v unexpectedly succeeded", args)
			}
			if caller.calls != 0 {
				t.Fatalf("args %v made %d calls", args, caller.calls)
			}
		}
		for _, args := range [][]string{
			{"message", "update-a2ui-card", "--biz-id", "biz-1", "--content", "[\"message\"]"},
			{"message", "update-a2ui-card", "--biz-id", "<bizId>", "--content", "[\"message\"]", "--flow-status", "1"},
		} {
			caller := &scriptedToolCaller{}
			if err := runNativeCardUpdate(t, caller, args...); err == nil {
				t.Fatalf("args %v unexpectedly succeeded", args)
			}
			if caller.calls != 0 {
				t.Fatalf("args %v made %d calls", args, caller.calls)
			}
		}
		if _, err := normalizeA2UIUpdateFlowStatus(""); err == nil || !strings.Contains(err.Error(), "PROCESSING") {
			t.Fatalf("empty a2ui status error = %v, want enum list", err)
		}
		if _, err := normalizeA2UIUpdateFlowStatus("BAD_STATUS"); err == nil || !strings.Contains(err.Error(), "PROCESSING") {
			t.Fatalf("bad a2ui status error = %v, want enum list", err)
		}
	})
}

func TestCrossPlatformCoverageA2UIAnnotations(t *testing.T) {
	for _, target := range []struct {
		name string
		args []string
		tool string
	}{
		{"group", []string{"message", "send-a2ui-card", "--conversation-id", "group-1"}, "create_and_send_a2ui_card"},
		{"direct", []string{"message", "send-a2ui-card", "--open-dingtalk-id", "DAAAAAAAAAAAiE"}, "create_and_send_a2ui_card"},
		{"update", []string{"message", "update-a2ui-card", "--biz-id", "biz-1", "--flow-status", "INPUTTING"}, "update_a2ui_card"},
	} {
		for _, tc := range []struct {
			name    string
			raw     string
			invalid bool
		}{
			{name: "omitted"},
			{name: "empty array", raw: "[]"},
			{name: "artifact", raw: `[{"componentId":"answer","surfaceId":"example-card","type":"artifact"}]`},
			{name: "objects", raw: `[{"id":9007199254740993,"nested":{"values":[true,null,"中文"]}},{}]`},
			{name: "blank", raw: " ", invalid: true},
			{name: "null", raw: "null", invalid: true},
			{name: "object", raw: "{}", invalid: true},
			{name: "string element", raw: `["annotation"]`, invalid: true},
			{name: "null element", raw: `[null]`, invalid: true},
			{name: "number element", raw: `[1]`, invalid: true},
			{name: "malformed", raw: `[`, invalid: true},
			{name: "trailing value", raw: `[] {}`, invalid: true},
		} {
			t.Run(target.name+"/"+tc.name, func(t *testing.T) {
				caller := &scriptedToolCaller{}
				args := append(slices.Clone(target.args), "--content", `["message"]`)
				if tc.name != "omitted" {
					args = append(args, "--a2ui-annotations", tc.raw)
				}
				err := runNativeCardUpdate(t, caller, args...)
				if tc.invalid {
					if err == nil || !strings.Contains(err.Error(), "--a2ui-annotations") || caller.calls != 0 {
						t.Fatalf("err=%v calls=%d", err, caller.calls)
					}
					return
				}
				if err != nil || caller.calls != 1 || caller.server != "im" || caller.tool != target.tool {
					t.Fatalf("err=%v caller=%#v", err, caller)
				}
				value, exists := caller.args["a2uiAnnotations"]
				if tc.name == "omitted" && target.name != "update" {
					if exists {
						t.Fatal("creation must preserve omitted annotations")
					}
					return
				}
				want := tc.raw
				if tc.name == "omitted" {
					want = "[]"
				}
				got, err := json.Marshal(value)
				if err != nil || string(got) != want {
					t.Fatalf("annotations=%s want=%s err=%v", got, want, err)
				}
			})
		}
	}
}

func TestCrossPlatformCoverageA2UISupportForward(t *testing.T) {
	for _, target := range [][]string{
		{"--conversation-id", "group-1"},
		{"--open-dingtalk-id", "DAAAAAAAAAAAiE"},
	} {
		for _, tc := range []struct {
			name    string
			flags   []string
			want    bool
			invalid bool
		}{
			{name: "default"},
			{name: "enabled", flags: []string{"--support-forward"}, want: true},
			{name: "disabled", flags: []string{"--support-forward=false"}},
			{name: "invalid", flags: []string{"--support-forward=invalid"}, invalid: true},
		} {
			t.Run(target[0]+"/"+tc.name, func(t *testing.T) {
				caller := &scriptedToolCaller{}
				args := append([]string{"message", "send-a2ui-card", "--content", `["message"]`}, target...)
				args = append(args, tc.flags...)
				err := runNativeCardUpdate(t, caller, args...)
				if tc.invalid {
					if err == nil || caller.calls != 0 {
						t.Fatalf("err=%v calls=%d", err, caller.calls)
					}
					return
				}
				if err != nil || caller.calls != 1 || caller.tool != "create_and_send_a2ui_card" || caller.args["supportForward"] != tc.want {
					t.Fatalf("err=%v caller=%#v", err, caller)
				}
				if _, exists := caller.args["cardProperty"]; exists {
					t.Fatal("MCP accepts supportForward directly, not the downstream cardProperty envelope")
				}
			})
		}
	}
}
