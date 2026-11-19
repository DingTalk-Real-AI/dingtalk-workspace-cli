// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import "testing"

func TestReviewTemplateDryRunPreservesChartWarnings(t *testing.T) {
	input := aicardTestFile(t, `[{"version":"v1.0","createSurface":{"surfaceId":"review","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json","dataModel":{},"components":[{"id":"root","component":"Chart","data":{"path":"/missing"}}]}}]`)
	result, code, err := executeAicard(t, &aicardCaller{}, "template", "create", "--file", input, "--mcp-id", "fixture-service", "--dry-run")
	if err != nil || code != 0 {
		t.Fatalf("%v %d %#v", err, code, result)
	}
	preflight, ok := result["data"].(map[string]any)["preflight"].(map[string]any)
	if !ok {
		t.Fatal("template validation report was dropped")
	}
	chart := preflight["chartValidation"].(map[string]any)
	if chart["status"] != "unverified" {
		t.Fatalf("unresolved chart lost warning: %#v", preflight)
	}
}
