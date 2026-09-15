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
