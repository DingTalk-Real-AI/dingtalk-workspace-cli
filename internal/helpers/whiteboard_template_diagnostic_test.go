package helpers

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	whiteboardcore "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/whiteboard"
)

func TestCrossPlatformCoverageWhiteboardTemplateDiagnostic(t *testing.T) {
	response := map[string]any{"data": map[string]any{"token": "secret-canary"}}
	message := whiteboardTemplateDiagnostic(fmt.Errorf("invalid receipt"), response, response).Error()
	if !strings.Contains(message, "data=map[string]interface {}") || !strings.Contains(message, "executed=<nil>") || strings.Contains(message, "secret-canary") {
		t.Fatalf("unsafe or missing diagnostic: %s", message)
	}
	_ = whiteboardTemplateDiagnostic(fmt.Errorf("empty response"), nil, nil)
}

func TestCrossPlatformCoverageWhiteboardTemplatePlatformReceipt(t *testing.T) {
	var response map[string]any
	decoder := json.NewDecoder(strings.NewReader(`{"success":true,"dryRun":true,"executed":false,"scope":"personal","resourceType":9}`))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		t.Fatal(err)
	}
	result := unwrapWhiteboardResult(response)
	if result["executed"] != false {
		t.Fatal("boolean false lost")
	}
	if err := validateWhiteboardTemplateDryRunResult(result, nil, whiteboardcore.PersonalTemplateSaveTool); err != nil {
		t.Fatal(err)
	}
}

func TestCrossPlatformCoverageWhiteboardTemplateCreatePreviewScope(t *testing.T) {
	for _, tc := range []struct{ tool, scope string }{
		{whiteboardcore.PersonalTemplateCreateTool, "personal"},
		{whiteboardcore.TeamTemplateCreateTool, "team"},
	} {
		args := map[string]any{"templateWorkspaceId": "ws"}
		result := map[string]any{"resourceType": 9, "templateScope": tc.scope, "templateWorkspaceId": "ws"}
		if err := validateWhiteboardTemplateDryRunResult(result, args, tc.tool); err != nil {
			t.Fatal(err)
		}
		delete(result, "templateScope")
		result["scope"] = tc.scope
		if err := validateWhiteboardTemplateDryRunResult(result, args, tc.tool); err == nil {
			t.Fatal("create preview accepted missing templateScope")
		}
		result["templateScope"] = "wrong-scope"
		if err := validateWhiteboardTemplateDryRunResult(result, args, tc.tool); err == nil {
			t.Fatal("create preview accepted mismatched templateScope")
		}
	}
}
