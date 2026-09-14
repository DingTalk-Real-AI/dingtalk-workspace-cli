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

package smart

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/runtimeannotate"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/helpers"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	"github.com/spf13/cobra"
)

func TestPrimaryParamMigrationSmartPayloadCompatibility(t *testing.T) {
	commands := []struct {
		name        string
		baseArgs    []string
		wantProduct string
		wantTool    string
		payloadKey  string
	}{
		{
			name:        "broadcast",
			baseArgs:    []string{"chat", "+broadcast", "--to", "张三"},
			wantProduct: "chat",
			wantTool:    "send_personal_message",
			payloadKey:  "content",
		},
		{
			name:        "dm",
			baseArgs:    []string{"chat", "+dm", "--to", "张三"},
			wantProduct: "chat",
			wantTool:    "send_personal_message",
			payloadKey:  "content",
		},
		{
			name:        "send-to-group",
			baseArgs:    []string{"chat", "+send-to-group", "--group", "项目冲刺"},
			wantProduct: "chat",
			wantTool:    "send_personal_message",
			payloadKey:  "content",
		},
		{
			name:        "doc-append",
			baseArgs:    []string{"doc", "+doc-append", "--doc", "doc-1"},
			wantProduct: "doc",
			wantTool:    "update_document",
			payloadKey:  "markdown",
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
			t.Run(command.name+"/"+spelling.name, func(t *testing.T) {
				fake := &platformCoverageCaller{}
				helpers.InitDeps(fake)
				root := newPlatformCoverageRoot()
				args := append([]string(nil), command.baseArgs...)
				args = append(args, spelling.args...)
				args = append(args, "--yes")
				root.SetArgs(args)
				if err := root.Execute(); err != nil {
					t.Fatalf("execute %v: %v", args, err)
				}
				if len(fake.calls) == 0 {
					t.Fatal("shortcut made no MCP call")
				}
				call := fake.calls[len(fake.calls)-1]
				if call.product != command.wantProduct || call.tool != command.wantTool {
					t.Fatalf("last call = %s/%s, want %s/%s", call.product, call.tool, command.wantProduct, command.wantTool)
				}
				if got := smartPrimaryPayloadValue(t, call.args, command.payloadKey); got != spelling.wantValue {
					t.Fatalf("payload value = %q, want %q; args=%#v", got, spelling.wantValue, call.args)
				}
				payloads[spelling.name] = call.args
			})
		}
		if !reflect.DeepEqual(payloads["old-only"], payloads["new-only"]) {
			t.Errorf("%s old/new payloads differ: old=%#v new=%#v", command.name, payloads["old-only"], payloads["new-only"])
		}
		if !reflect.DeepEqual(payloads["old-only"], payloads["both-different-old-wins"]) {
			t.Errorf("%s old/both payloads differ: old=%#v both=%#v", command.name, payloads["old-only"], payloads["both-different-old-wins"])
		}
	}
}

func smartPrimaryPayloadValue(t *testing.T, args map[string]any, key string) string {
	t.Helper()
	value, _ := args[key].(string)
	if key == "markdown" {
		return value
	}
	var content map[string]string
	if err := json.Unmarshal([]byte(value), &content); err != nil {
		t.Fatalf("decode markdown content %q: %v", value, err)
	}
	return content["text"]
}

func TestPrimaryParamMigrationSmartSurfaceAndContract(t *testing.T) {
	tests := []struct {
		service string
		command string
	}{
		{service: "chat", command: "+broadcast"},
		{service: "chat", command: "+dm"},
		{service: "chat", command: "+send-to-group"},
		{service: "doc", command: "+doc-append"},
	}

	root := newPlatformCoverageRoot()
	for _, test := range tests {
		t.Run(test.service+"/"+test.command, func(t *testing.T) {
			cmd, _, err := root.Find([]string{test.service, test.command})
			if err != nil || cmd == nil {
				t.Fatalf("find command: %v", err)
			}
			canonical := cmd.Flags().Lookup("content")
			if canonical == nil || canonical.Hidden {
				t.Fatalf("canonical --content = %#v, want visible", canonical)
			}
			legacy := cmd.Flags().Lookup("text")
			if legacy == nil || !legacy.Hidden {
				t.Fatalf("legacy --text = %#v, want hidden", legacy)
			}
			if got := legacy.Annotations[runtimeannotate.AnnotationFlagAliasOf]; len(got) != 1 || got[0] != "content" {
				t.Fatalf("--text alias_of = %#v, want content", got)
			}
			if got := legacy.Annotations[runtimeannotate.AnnotationFlagAliasOrigin]; len(got) != 1 || got[0] != runtimeannotate.FlagAliasOriginCorecmdV1 {
				t.Fatalf("--text alias origin = %#v", got)
			}

			var help bytes.Buffer
			cmd.SetOut(&help)
			if err := cmd.Help(); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(help.String(), "--content") || strings.Contains(help.String(), "--text") {
				t.Fatalf("help does not expose only the new Primary:\n%s", help.String())
			}

			spec := smartRegisteredShortcut(t, test.service, test.command)
			if len(spec.Contract.Parameters) != 1 {
				t.Fatalf("ParamDecls = %#v, want one", spec.Contract.Parameters)
			}
			param := spec.Contract.Parameters[0]
			if param.Name != "content" || param.Property != "text" ||
				param.Required == nil || !*param.Required || param.InterfaceType != "" {
				t.Fatalf("content ParamDecl = %#v", param)
			}
			for _, example := range append(append([]string(nil), spec.Tips...), spec.Contract.Selection.Examples...) {
				if strings.Contains(example, "--text") || !strings.Contains(example, "--content") {
					t.Fatalf("example does not recommend only --content: %q", example)
				}
			}
		})
	}
}

func TestPrimaryParamMigrationDocAppendRejectsBlankContent(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("doc", "", "")
	cmd.Flags().String("content", "", "")
	cmd.Flags().String("text", "", "")
	if err := cmd.Flags().Set("doc", "doc-1"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("content", "   "); err != nil {
		t.Fatal(err)
	}
	err := DocAppend.Execute(shortcut.RuntimeContextForTest(cmd, DocAppend))
	if err == nil || !strings.Contains(err.Error(), "--content 不能为空") {
		t.Fatalf("blank --content error = %v", err)
	}
}

func smartRegisteredShortcut(t *testing.T, service, command string) shortcut.Shortcut {
	t.Helper()
	for _, spec := range shortcut.All() {
		if spec.Service == service && spec.Command == command {
			return spec
		}
	}
	t.Fatalf("registered shortcut %s %s not found", service, command)
	return shortcut.Shortcut{}
}
