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

package chat

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestPrimaryParamMigrationChatContentPayloadCompatibility(t *testing.T) {
	commands := []struct {
		command     string
		baseArgs    []string
		wantProduct string
		wantTool    string
		payloadKey  string
	}{
		{
			command:     "+messages-send-by-bot",
			baseArgs:    []string{"--robot-code", "robot", "--group", "cid", "--title", "title"},
			wantProduct: "bot",
			wantTool:    "send_robot_group_message",
			payloadKey:  "markdown",
		},
		{
			command:     "+messages-batch-send-by-bot",
			baseArgs:    []string{"--robot-code", "robot", "--users", "u1", "--title", "title"},
			wantProduct: "bot",
			wantTool:    "batch_send_robot_msg_to_users",
			payloadKey:  "markdown",
		},
		{
			command:     "+messages-send-by-webhook",
			baseArgs:    []string{"--token", "token", "--title", "title"},
			wantProduct: "bot",
			wantTool:    "send_message_by_custom_robot",
			payloadKey:  "text",
		},
	}
	spellings := []struct {
		name      string
		args      []string
		wantValue string
	}{
		{name: "old-only", args: []string{"--text", "legacy-value"}, wantValue: "legacy-value"},
		{name: "new-only", args: []string{"--content", "legacy-value"}, wantValue: "legacy-value"},
		{
			name:      "both-different-old-wins",
			args:      []string{"--text", "legacy-value", "--content", "canonical-value"},
			wantValue: "legacy-value",
		},
	}

	for _, command := range commands {
		payloads := make(map[string]map[string]any, len(spellings))
		for _, spelling := range spellings {
			t.Run(command.command+"/"+spelling.name, func(t *testing.T) {
				fake := &larkAlignmentCaller{}
				helpers.InitDeps(fake)
				root := newPlatformCoverageRoot()
				args := append([]string{"chat", command.command}, command.baseArgs...)
				args = append(args, spelling.args...)
				args = append(args, "--yes")
				root.SetArgs(args)
				if err := root.Execute(); err != nil {
					t.Fatalf("execute %v: %v", args, err)
				}
				if len(fake.calls) != 1 {
					t.Fatalf("calls = %#v, want one", fake.calls)
				}
				call := fake.calls[0]
				if call.product != command.wantProduct || call.tool != command.wantTool {
					t.Fatalf("call = %s/%s, want %s/%s", call.product, call.tool, command.wantProduct, command.wantTool)
				}
				if got := call.args[command.payloadKey]; got != spelling.wantValue {
					t.Fatalf("%s = %#v, want %q; args=%#v", command.payloadKey, got, spelling.wantValue, call.args)
				}
				payloads[spelling.name] = call.args
			})
		}
		if !reflect.DeepEqual(payloads["old-only"], payloads["new-only"]) {
			t.Errorf("%s old/new payloads differ: old=%#v new=%#v", command.command, payloads["old-only"], payloads["new-only"])
		}
		if !reflect.DeepEqual(payloads["old-only"], payloads["both-different-old-wins"]) {
			t.Errorf("%s old/both payloads differ: old=%#v both=%#v", command.command, payloads["old-only"], payloads["both-different-old-wins"])
		}
	}
}

func TestPrimaryParamMigrationMessagesReplyPayloadCompatibility(t *testing.T) {
	spellings := []struct {
		name             string
		args             []string
		wantConversation string
		wantContent      string
	}{
		{
			name:             "old-only",
			args:             []string{"--conversation-id", "legacy-cid", "--text", "legacy-body"},
			wantConversation: "legacy-cid",
			wantContent:      "legacy-body",
		},
		{
			name:             "new-only",
			args:             []string{"--group", "legacy-cid", "--content", "legacy-body"},
			wantConversation: "legacy-cid",
			wantContent:      "legacy-body",
		},
		{
			name: "both-different-old-wins",
			args: []string{
				"--conversation-id", "legacy-cid", "--group", "canonical-cid",
				"--text", "legacy-body", "--content", "canonical-body",
			},
			wantConversation: "legacy-cid",
			wantContent:      "legacy-body",
		},
	}

	payloads := make(map[string]map[string]any, len(spellings))
	for _, spelling := range spellings {
		t.Run(spelling.name, func(t *testing.T) {
			fake := &larkAlignmentCaller{responses: map[string]string{
				"chat/send_personal_message": `{"result":{"openMessageId":"new-msg"}}`,
			}}
			helpers.InitDeps(fake)
			root := newPlatformCoverageRoot()
			var output bytes.Buffer
			root.SetOut(&output)
			args := []string{
				"chat", "+messages-reply", "--message-id", "msg",
				"--ref-sender", fixtureCurrentDOpenID,
			}
			args = append(args, spelling.args...)
			args = append(args, "--yes")
			root.SetArgs(args)
			if err := root.Execute(); err != nil {
				t.Fatalf("execute %v: %v", args, err)
			}
			if len(fake.calls) != 1 {
				t.Fatalf("calls = %#v, want one", fake.calls)
			}
			call := fake.calls[0]
			if call.product != "chat" || call.tool != "send_personal_message" {
				t.Fatalf("reply call = %#v", call)
			}
			assertReplyPrimaryPayload(t, call.args, spelling.wantConversation, spelling.wantContent)
			payloads[spelling.name] = call.args

			var payload map[string]any
			if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
				t.Fatalf("decode reply output %q: %v", output.String(), err)
			}
			if got := payload["conversationId"]; got != spelling.wantConversation {
				t.Fatalf("result conversationId = %#v, want %q; payload=%#v", got, spelling.wantConversation, payload)
			}
		})
	}
	if !reflect.DeepEqual(payloads["old-only"], payloads["new-only"]) {
		t.Errorf("reply old/new payloads differ: old=%#v new=%#v", payloads["old-only"], payloads["new-only"])
	}
	if !reflect.DeepEqual(payloads["old-only"], payloads["both-different-old-wins"]) {
		t.Errorf("reply old/both payloads differ: old=%#v both=%#v", payloads["old-only"], payloads["both-different-old-wins"])
	}

	t.Run("dry-run-both-different-old-wins", func(t *testing.T) {
		fake := &larkAlignmentCaller{}
		helpers.InitDeps(fake)
		root := newPlatformCoverageRoot()
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetArgs([]string{
			"chat", "+messages-reply", "--message-id", "msg", "--ref-sender", fixtureCurrentDOpenID,
			"--conversation-id", "legacy-cid", "--group", "canonical-cid",
			"--text", "legacy-body", "--content", "canonical-body",
			"--dry-run", "--yes",
		})
		if err := root.Execute(); err != nil {
			t.Fatal(err)
		}
		if len(fake.calls) != 0 {
			t.Fatalf("dry-run reached transport: %#v", fake.calls)
		}
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatalf("decode dry-run output %q: %v", output.String(), err)
		}
		if got := payload["conversationId"]; got != "legacy-cid" {
			t.Fatalf("dry-run conversationId = %#v, want legacy-cid", got)
		}
		arguments, _ := payload["arguments"].(map[string]any)
		assertReplyPrimaryPayload(t, arguments, "legacy-cid", "legacy-body")
	})
}

func assertReplyPrimaryPayload(t *testing.T, args map[string]any, wantConversation, wantContent string) {
	t.Helper()
	if got := args["openConversationId"]; got != wantConversation {
		t.Fatalf("openConversationId = %#v, want %q; args=%#v", got, wantConversation, args)
	}
	contentText, _ := args["content"].(string)
	var content map[string]string
	if err := json.Unmarshal([]byte(contentText), &content); err != nil {
		t.Fatalf("decode reply content %q: %v", contentText, err)
	}
	if got := content["content"]; got != wantContent {
		t.Fatalf("reply content = %q, want %q; payload=%#v", got, wantContent, content)
	}
}

func TestPrimaryParamMigrationChatSurfaceAndContract(t *testing.T) {
	tests := []struct {
		command string
		params  []chatPrimaryParamExpectation
	}{
		{
			command: "+messages-send-by-bot",
			params:  []chatPrimaryParamExpectation{{name: "content", legacy: "text", property: "text"}},
		},
		{
			command: "+messages-batch-send-by-bot",
			params:  []chatPrimaryParamExpectation{{name: "content", legacy: "text", property: "text"}},
		},
		{
			command: "+messages-send-by-webhook",
			params:  []chatPrimaryParamExpectation{{name: "content", legacy: "text", property: "text"}},
		},
		{
			command: "+messages-reply",
			params: []chatPrimaryParamExpectation{
				{name: "group", legacy: "conversation-id", property: "conversationId"},
				{name: "content", legacy: "text", property: "text"},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.command, func(t *testing.T) {
			root := newPlatformCoverageRoot()
			cmd, _, err := root.Find([]string{"chat", test.command})
			if err != nil || cmd == nil {
				t.Fatalf("find command: %v", err)
			}
			for _, param := range test.params {
				canonical := cmd.Flags().Lookup(param.name)
				if canonical == nil || canonical.Hidden {
					t.Fatalf("canonical --%s = %#v, want visible", param.name, canonical)
				}
				legacy := cmd.Flags().Lookup(param.legacy)
				if legacy == nil || !legacy.Hidden {
					t.Fatalf("legacy --%s = %#v, want hidden", param.legacy, legacy)
				}
				if got := legacy.Annotations[runtimeannotate.AnnotationFlagAliasOf]; len(got) != 1 || got[0] != param.name {
					t.Fatalf("--%s alias_of = %#v, want %s", param.legacy, got, param.name)
				}
				if got := legacy.Annotations[runtimeannotate.AnnotationFlagAliasOrigin]; len(got) != 1 || got[0] != runtimeannotate.FlagAliasOriginCorecmdV1 {
					t.Fatalf("--%s alias origin = %#v", param.legacy, got)
				}
			}

			var help bytes.Buffer
			cmd.SetOut(&help)
			if err := cmd.Help(); err != nil {
				t.Fatal(err)
			}
			for _, param := range test.params {
				if !strings.Contains(help.String(), "--"+param.name) || strings.Contains(help.String(), "--"+param.legacy) {
					t.Fatalf("help does not expose only --%s (legacy --%s):\n%s", param.name, param.legacy, help.String())
				}
			}

			spec := chatRegisteredShortcut(t, test.command)
			if len(spec.Contract.Parameters) != len(test.params) {
				t.Fatalf("ParamDecls = %#v, want %d", spec.Contract.Parameters, len(test.params))
			}
			for i, want := range test.params {
				param := spec.Contract.Parameters[i]
				if param.Name != want.name || param.Property != want.property ||
					param.Required == nil || !*param.Required || param.InterfaceType != "" {
					t.Fatalf("ParamDecl[%d] = %#v, want %#v", i, param, want)
				}
			}
			for _, example := range append(append([]string(nil), spec.Tips...), spec.Contract.Selection.Examples...) {
				for _, param := range test.params {
					if strings.Contains(example, "--"+param.legacy) || !strings.Contains(example, "--"+param.name) {
						t.Fatalf("example does not recommend only --%s: %q", param.name, example)
					}
				}
			}
		})
	}
}

type chatPrimaryParamExpectation struct {
	name     string
	legacy   string
	property string
}

func chatRegisteredShortcut(t *testing.T, command string) shortcut.Shortcut {
	t.Helper()
	for _, spec := range shortcut.All() {
		if spec.Service == "chat" && spec.Command == command {
			return spec
		}
	}
	t.Fatalf("registered shortcut chat %s not found", command)
	return shortcut.Shortcut{}
}
