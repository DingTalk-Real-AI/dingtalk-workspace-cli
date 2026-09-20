package app

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/pkg/edition"
)

var _ edition.WhiteboardTemplatePreviewCaller = (*agentExampleFailClosedCaller)(nil)

// Simulate only the reviewed server-preflight capability. Ordinary CallTool
// remains fail-closed; this fixture never reaches a network or write executor.
func (c *agentExampleFailClosedCaller) CallWhiteboardTemplatePreview(_ context.Context, tool string, args map[string]any) (*edition.ToolResult, error) {
	if err := whiteboard.ValidateTemplatePreviewCall(whiteboard.ServerID, tool, args); err != nil {
		return nil, err
	}
	scope := "personal"
	if tool == whiteboard.TeamTemplateSaveTool || tool == whiteboard.TeamTemplateCreateTool {
		scope = "team"
	} else if tool == whiteboard.PublicTemplateCreateTool {
		scope = "public"
	}
	result := map[string]any{
		"success": true, "dryRun": true, "executed": false,
		"resourceType": whiteboard.TemplateType, "requestId": args["requestId"],
	}
	if tool == whiteboard.PersonalTemplateCreateTool || tool == whiteboard.TeamTemplateCreateTool || tool == whiteboard.PublicTemplateCreateTool {
		result["templateScope"] = scope
		result["templateId"] = args["templateId"]
		result["verified"] = true
	} else {
		result["scope"] = scope
		result["sourceNodeId"] = args["sourceNodeId"]
	}
	if scope == "team" {
		workspace, _ := args["templateWorkspaceId"].(string)
		if workspace == "" {
			workspace = "fixture-source-workspace"
		}
		result["templateWorkspaceId"] = workspace
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return &edition.ToolResult{Content: []edition.ContentBlock{{Type: "text", Text: string(raw)}}}, nil
}

func TestCrossPlatformCoverageAgentTemplatePreviewFixture(t *testing.T) {
	caller := &agentExampleFailClosedCaller{}
	for _, tool := range []string{whiteboard.PersonalTemplateSaveTool, whiteboard.TeamTemplateSaveTool, whiteboard.PersonalTemplateCreateTool, whiteboard.TeamTemplateCreateTool, whiteboard.PublicTemplateCreateTool} {
		for _, dry := range []any{nil, false, "true", true} {
			result, err := caller.CallWhiteboardTemplatePreview(context.Background(), tool, map[string]any{"dryRun": dry, "templateWorkspaceId": "ws"})
			if dry != true {
				if err == nil {
					t.Fatalf("accepted unsafe dryRun=%v", dry)
				}
				continue
			}
			if err != nil {
				t.Fatal(err)
			}
			var receipt map[string]any
			if err := json.Unmarshal([]byte(result.Content[0].Text), &receipt); err != nil {
				t.Fatal(err)
			}
			if receipt["executed"] != false || receipt["dryRun"] != true {
				t.Fatalf("invalid preview: %v", receipt)
			}
		}
	}
	if _, err := caller.CallWhiteboardTemplatePreview(context.Background(), whiteboard.PersonalTemplateListTool, map[string]any{"dryRun": true}); err == nil {
		t.Fatal("unreviewed preview accepted")
	}
	if caller.toolCallAttempts.Load() != 0 {
		t.Fatal("preview used ordinary tool caller")
	}
	if _, err := caller.CallTool(context.Background(), whiteboard.ServerID, whiteboard.PersonalTemplateSaveTool, nil); err == nil {
		t.Fatal("ordinary calls must remain blocked")
	}
}
