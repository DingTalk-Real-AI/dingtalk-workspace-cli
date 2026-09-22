package helpers

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd"
)

func TestCrossPlatformCoverageDeapAgentCreateRequiresExplicitType(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags []string
	}{
		{name: "omitted"},
		{name: "empty", flags: []string{"--type", ""}},
		{name: "whitespace", flags: []string{"--type", " \t "}},
		{name: "unsupported", flags: []string{"--type", "a2a"}},
	} {
		for _, dryRun := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/dry_run=%t", tc.name, dryRun), func(t *testing.T) {
				caller, _ := newDeapAgentTestTree(t, dryRun)
				root := deapHandler{}.Command(&captureRunner{})
				root.PersistentFlags().Bool("yes", false, "test confirmation")
				root.PersistentFlags().Bool("dry-run", false, "test preview")
				argv := []string{"manage", "create", "--name", "助手", "--description", "验收"}
				argv = append(argv, tc.flags...)
				if dryRun {
					argv = append(argv, "--dry-run")
				} else {
					argv = append(argv, "--yes")
				}
				root.SetArgs(argv)
				err := corecmd.ExecuteForTest(root)
				if err == nil || !strings.Contains(err.Error(), "--type") {
					t.Errorf("create must reject missing or invalid type locally, got %v", err)
				}
				if len(caller.calls) != 0 {
					t.Errorf("invalid create made %d remote calls", len(caller.calls))
				}
			})
		}
	}
}

func TestCrossPlatformCoverageDeapAgentDefaultResponseAndSnapshotArguments(t *testing.T) {
	cases := []struct {
		name string
		argv []string
		tool string
		want map[string]any
	}{
		{"create_open_default", []string{"manage", "create", "--name", "助手", "--description", "验收", "--type", "open_code"}, deapAgentCreateTool,
			map[string]any{"name": "助手", "description": "验收", "digitalTagEmployeeProfile": map[string]any{"type": "open_code", "responseMode": "mention_only"}}},
		{"create_local_default", []string{"manage", "create", "--name", "助手", "--description", "验收", "--type", "local_agent"}, deapAgentCreateTool,
			map[string]any{"name": "助手", "description": "验收", "digitalTagEmployeeProfile": map[string]any{"type": "local_agent", "responseMode": "mention_only"}}},
		{"create_open_empty", []string{"manage", "create", "--name", "助手", "--description", "验收", "--type", "open_code", "--response-mode", " "}, deapAgentCreateTool,
			map[string]any{"name": "助手", "description": "验收", "digitalTagEmployeeProfile": map[string]any{"type": "open_code", "responseMode": "mention_only"}}},
		{"create_explicit_mode", []string{"manage", "create", "--name", "助手", "--description", "验收", "--type", "open_code", "--response-mode", "targeted_proactive"}, deapAgentCreateTool,
			map[string]any{"name": "助手", "description": "验收", "digitalTagEmployeeProfile": map[string]any{"type": "open_code", "responseMode": "targeted_proactive"}}},
		{"save_preserves_mode", []string{"manage", "save-draft", "--agent-uuid", "agent-1", "--name", "新名称"}, deapAgentSaveDraftTool,
			map[string]any{"agentUuid": "agent-1", "name": "新名称"}},
		{"save_open_preserves_mode", []string{"manage", "save-draft", "--agent-uuid", "agent-1", "--type", "open_code"}, deapAgentSaveDraftTool,
			map[string]any{"agentUuid": "agent-1", "digitalTagEmployeeProfile": map[string]any{"type": "open_code"}}},
		{"save_explicit_mode", []string{"manage", "save-draft", "--agent-uuid", "agent-1", "--response-mode", "targeted_proactive"}, deapAgentSaveDraftTool,
			map[string]any{"agentUuid": "agent-1", "digitalTagEmployeeProfile": map[string]any{"responseMode": "targeted_proactive"}}},
		{"detail_default", []string{"manage", "detail", "--agent-uuid", "agent-1"}, deapAgentDetailTool,
			map[string]any{"agentUuid": "agent-1", "snapshot": "draft"}},
		{"detail_published", []string{"manage", "detail", "--agent-uuid", "agent-1", "--snapshot", "published"}, deapAgentDetailTool,
			map[string]any{"agentUuid": "agent-1", "snapshot": "published"}},
		{"detail_legacy_alias", []string{"manage", "detail", "--agent-uuid", "agent-1", "--type", "published"}, deapAgentDetailTool,
			map[string]any{"agentUuid": "agent-1", "snapshot": "published"}},
	}
	for _, tc := range cases {
		for _, dryRun := range []bool{false, true} {
			suffix := "/call"
			if dryRun {
				suffix = "/dry_run"
			}
			t.Run(tc.name+suffix, func(t *testing.T) {
				caller, out := newDeapAgentTestTree(t, dryRun)
				caller.resultText = `{"success":true,"data":{"agentUuid":"agent-1"}}`
				root := deapHandler{}.Command(&captureRunner{})
				root.PersistentFlags().Bool("yes", false, "test confirmation")
				root.PersistentFlags().Bool("dry-run", false, "test preview")
				argv := append([]string{}, tc.argv...)
				if dryRun {
					argv = append(argv, "--dry-run")
				} else {
					argv = append(argv, "--yes")
				}
				root.SetArgs(argv)
				if err := corecmd.ExecuteForTest(root); err != nil {
					t.Fatal(err)
				}
				var got map[string]any
				if dryRun {
					if len(caller.calls) != 0 {
						t.Fatalf("dry-run made %d calls", len(caller.calls))
					}
					var preview struct {
						Tool      string         `json:"tool"`
						Arguments map[string]any `json:"arguments"`
					}
					if err := json.Unmarshal(out.Bytes(), &preview); err != nil {
						t.Fatalf("decode: %v; output=%s", err, out.String())
					}
					if preview.Tool != tc.tool {
						t.Fatalf("tool=%q, want %q", preview.Tool, tc.tool)
					}
					got = preview.Arguments
				} else {
					wantCalls := 1
					if tc.name == "create_local_default" {
						wantCalls = 2
					}
					if len(caller.calls) != wantCalls || caller.calls[0].toolName != tc.tool || caller.calls[0].productID != deapAgentServerID {
						t.Fatalf("unexpected calls: %#v", caller.calls)
					}
					got = caller.calls[0].args
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Fatalf("args=%#v, want %#v", got, tc.want)
				}
			})
		}
	}
}
