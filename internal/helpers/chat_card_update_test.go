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
	"errors"
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
	t.Run("explicit evidence succeeds", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"result":{"bizId":"biz-1","updated":true}}`}}}
		err := runNativeCardUpdate(t, caller,
			"message", "update-card",
			"--biz-id", "biz-1",
			"--content", "完成",
			"--flow-status", "3",
			"--yes",
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

	t.Run("generic success is unverified", func(t *testing.T) {
		caller := &scriptedToolCaller{steps: []scriptedToolStep{{text: `{"success":true,"errorCode":null}`}}}
		err := runNativeCardUpdate(t, caller,
			"message", "update-card",
			"--biz-id", "not-a-real-card",
			"--content", "完成",
			"--flow-status", "3",
			"--yes",
		)
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Reason != "streaming_card_update_unverified" {
			t.Fatalf("error = %#v, want streaming_card_update_unverified", err)
		}
	})

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
