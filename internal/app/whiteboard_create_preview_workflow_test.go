package app

import (
	"strings"
	"testing"
)

// Agent guidance is delivered through the final Schema, while the digest stays
// optional for existing CLI scripts. Neither guidance nor a digest is approval.
func TestCrossPlatformCoverageWhiteboardCreatePreviewWorkflowDelivery(t *testing.T) {
	root := NewRootCommand()
	leaf, _, err := root.Find([]string{"whiteboard", "create-with-content"})
	if err != nil {
		t.Fatal(err)
	}
	for _, compact := range []bool{false, true} {
		args := []string{"--cli-path", "whiteboard create-with-content"}
		if compact {
			args = append(args, "--compact")
		}
		payload := executeShortcutSchemaQuery(t, args...)
		summary := schemaContractString(payload["agent_summary"])
		for _, concept := range []string{"必须先执行 whiteboard render", "等待用户明确确认当前版本", "内容回读不能替代预览"} {
			if !strings.Contains(summary, concept) {
				t.Fatalf("compact=%v loses workflow prerequisite %q: %s", compact, concept, summary)
			}
		}
		if payload["confirmation"] != "user_required" {
			t.Fatalf("confirmation changed: %#v", payload["confirmation"])
		}
		params := schemaContractMap(payload["parameters"])
		digest, ok := params["expected-source-digest"]
		if !ok || digest["required"] == true {
			t.Fatalf("digest must remain optional: %#v", digest)
		}
		if !compact && schemaContractString(payload["description"]) != leaf.Long {
			t.Fatal("full Schema description must deliver the same workflow as Cobra Long")
		}
	}
}
