// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func TestCrossPlatformCoverageAicardDeferredTemplateAndPreviewCompatibility(t *testing.T) {
	const content = `[{"version":"v1.0","createSurface":{"surfaceId":"existing-layout","catalogId":"https://dingtalk.com/card/a2ui/catalogs/public/catalog.json","dataModel":{"answer":"user input"},"components":[{"id":"root","component":"Column","children":["chart","note"]},{"id":"chart","component":"Chart","data":{"path":"/later"},"fallbackMarkdown":"Pending business data"},{"id":"note","component":"Text","text":{"path":"/answer"}}]}}]`
	file := aicardTestFile(t, content)
	// A deferred template is accepted without inventing Chart values.
	t.Setenv(aicardTemplateServiceEnv, "fixture-service")
	for _, operation := range []string{"create", "update"} {
		caller := &aicardTemplateTestCaller{}
		args := []string{operation, "--file", file, "--dry-run"}
		if operation == "update" {
			args = append(args, "--template-id", "existing-template")
		}
		result, err := executeAicardTemplate(t, caller, args...)
		if err != nil || caller.invocations != 0 || result["executed"] != false {
			t.Fatalf("%s: %v %v calls=%d", operation, result, err, caller.invocations)
		}
	}
	for _, dry := range []bool{true, false} {
		// The caller is a fake: even the send branch never sends a real card.
		caller := &aicardCaller{response: `{"success":true,"result":{"bizId":"original-card"}}`}
		args := []string{"preview", "--file", file, "--support-forward=false", "--summary", "Original summary"}
		if dry {
			args = append(args, "--dry-run")
		}
		result, code, err := executeAicard(t, caller, args...)
		if err != nil || code != 0 {
			t.Fatalf("%v %d %v", result, code, err)
		}
		data := result["data"].(map[string]any)
		preflight := data["preflight"].(map[string]any)
		if preflight["valid"] != true || preflight["chartValidation"].(map[string]any)["status"] != "unverified" {
			t.Fatal(preflight)
		}
		if data["supportForward"] != false || data["flowStatus"] != "PROCESSING" || data["summary"] != "Original summary" {
			t.Fatal(data)
		}
		if dry {
			if len(caller.calls) != 0 {
				t.Fatal(caller.calls)
			}
		} else {
			var original []any
			if err := json.Unmarshal([]byte(content), &original); err != nil {
				t.Fatal(err)
			}
			wire := caller.sent["a2uiMessages"].([]string)
			if len(wire) != 1 {
				t.Fatal(wire)
			}
			var sent any
			if err := json.Unmarshal([]byte(wire[0]), &sent); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(original[0], sent) {
				t.Fatal("Preview changed business data or component layout")
			}
		}
	}
	saved, err := os.ReadFile(file)
	if err != nil || string(saved) != content {
		t.Fatal("Template or preview rewrote the source", err)
	}
	// An explicit complete-card lint remains strict.
	caller := &aicardCaller{}
	_, code, err := executeAicard(t, caller, "lint", "--file", file, "--preflight", "new-card")
	if err != nil || code == 0 || len(caller.calls) != 0 {
		t.Fatalf("strict lint: %d %v", code, err)
	}
}
