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
	"strings"
	"testing"
)

func TestCrossPlatformCoverageChatMessageUpdateTextEmotionMapsReplacementTemplate(t *testing.T) {
	for _, locator := range []string{"conversation-id", "group", "id", "chat"} {
		t.Run(locator, func(t *testing.T) {
			caller := &wukongWeeklySyncCaller{}
			_, _, err := executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand,
				"message", "update-text-emotion",
				"--"+locator, "cid",
				"--msg-id", "mid",
				"--old-emotion-id", "old-emotion",
				"--emotion-id", "new-emotion",
				"--emotion-name", "赞",
				"--text", "nice",
				"--background-id", "im_bg_5",
			)
			if err != nil {
				t.Fatalf("update-text-emotion returned error: %v", err)
			}
			requireWukongWeeklySyncCall(t, caller, wukongWeeklySyncCall{
				server: "im",
				tool:   "update_text_emotion",
				args: map[string]any{
					"openConversationId": "cid",
					"openMsgId":          "mid",
					"oldEmotionId":       "old-emotion",
					"emotionId":          "new-emotion",
					"emotionName":        "赞",
					"text":               "nice",
					"backgroundId":       "im_bg_5",
				},
			})
		})
	}
}

func TestCrossPlatformCoverageChatMessageUpdateTextEmotionRequiresEveryBusinessParameter(t *testing.T) {
	required := []struct {
		name string
		args []string
	}{
		{name: "conversation-id", args: []string{"--group", "cid"}},
		{name: "msg-id", args: []string{"--msg-id", "mid"}},
		{name: "old-emotion-id", args: []string{"--old-emotion-id", "old-emotion"}},
		{name: "emotion-id", args: []string{"--emotion-id", "new-emotion"}},
		{name: "emotion-name", args: []string{"--emotion-name", "赞"}},
		{name: "text", args: []string{"--text", "nice"}},
		{name: "background-id", args: []string{"--background-id", "im_bg_5"}},
	}

	for omitted := range required {
		t.Run("missing-"+required[omitted].name, func(t *testing.T) {
			args := []string{"message", "update-text-emotion"}
			for i, parameter := range required {
				if i != omitted {
					args = append(args, parameter.args...)
				}
			}

			caller := &wukongWeeklySyncCaller{}
			_, _, err := executeWukongWeeklySyncCommand(t, "chat", caller, newChatCommand, args...)
			if err == nil || !strings.Contains(err.Error(), required[omitted].name) {
				t.Fatalf("missing %s error = %v", required[omitted].name, err)
			}
			requireWukongWeeklySyncNoCalls(t, caller)
		})
	}
}
