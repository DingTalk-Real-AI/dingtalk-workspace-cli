// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/transport"
)

// Keep the actual command boundary covered, not only the low-level preflight.
func TestCrossPlatformCoverageAicardTemplateLegacyPlaceholderLifecycle(t *testing.T) {
	const content = `[{"version":"v1.0","createSurface":{"surfaceId":"legacy","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json","dataModel":{"chart":{"type":"bar"}},"components":[{"id":"root","component":"Chart","data":{"path":"/chart"}}]}}]`
	file := aicardTestFile(t, content)
	for _, operation := range []string{"create", "update"} {
		t.Run(operation, func(t *testing.T) {
			caller := &aicardTemplateTestCaller{}
			args := []string{operation, "--file", file, "--mcp-id", "fixture-service", "--dry-run"}
			if operation == "update" {
				args = append(args, "--template-id", "existing-template")
			}
			data, err := executeAicardTemplate(t, caller, args...)
			if err != nil || caller.invocations != 0 {
				t.Fatalf("compatibility dry-run: %v %#v", err, data)
			}
			preflight := data["preflight"].(map[string]any)
			chart := preflight["chartValidation"].(map[string]any)
			if preflight["valid"] != true || chart["status"] != "unverified" {
				t.Fatalf("placeholder must be preserved without certification: %#v", preflight)
			}
		})
	}
	caller := &aicardTemplateTestCaller{result: transport.ToolCallResult{StructuredContent: map[string]any{"success": true, "file": content}}}
	out := filepath.Join(t.TempDir(), "legacy-template.json")
	_, err := executeAicardTemplate(t, caller, "get", "--mcp-id", "fixture-service", "--template-id", "existing-template", "--version", "1", "--out", out)
	if err != nil || caller.invocations != 1 {
		t.Fatalf("legacy get: %v calls=%d", err, caller.invocations)
	}
	saved, err := os.ReadFile(out)
	if err != nil || string(saved) != content {
		t.Fatalf("legacy get changed content: %v %s", err, saved)
	}
	// Ready-to-send lint must continue rejecting this unresolved legacy placeholder.
	result, code, err := executeAicard(t, &aicardCaller{}, "lint", "--file", file, "--preflight", "new-card")
	if err != nil {
		t.Fatal(err)
	}
	if code == 0 || result["ok"] != false || result["error"] == nil {
		t.Fatalf("strict lint unexpectedly certified placeholder: %#v", result)
	}
}
