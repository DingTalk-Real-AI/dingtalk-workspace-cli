package app

import (
	"strings"
	"testing"
)

func TestCrossPlatformCoverageWhiteboardUpdateDiffWorkflowDelivery(t *testing.T) {
	root := NewRootCommand()
	leaf, _, err := root.Find([]string{"whiteboard", "+update"})
	if err != nil {
		t.Fatal(err)
	}
	for _, compact := range []bool{false, true} {
		args := []string{"--cli-path", "whiteboard +update"}
		if compact {
			args = append(args, "--compact")
		}
		payload := executeShortcutSchemaQuery(t, args...)
		summary := schemaContractString(payload["agent_summary"])
		for _, required := range []string{"必须先执行 whiteboard +diff", "等待用户明确确认当前差异", "blocker", "sourceDigest", "target.revision", "内嵌", "兼容性"} {
			if !strings.Contains(summary, required) || !strings.Contains(leaf.Long, required) {
				t.Fatalf("compact=%v missing %q in delivered Schema or Help", compact, required)
			}
		}
		if payload["confirmation"] != "user_required" {
			t.Fatalf("confirmation changed: %v", payload["confirmation"])
		}
		params := schemaContractMap(payload["parameters"])
		digest, ok := params["expected-source-digest"]
		if !ok || digest["required"] == true {
			t.Fatalf("digest must remain optional: %v", digest)
		}
	}
	atomic, _, err := root.Find([]string{"whiteboard", "update"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(atomic.Long, "不得使用本原子入口绕过") {
		t.Fatal("atomic Help loses Agent diff routing")
	}
}
