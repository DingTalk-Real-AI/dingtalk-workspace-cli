// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contractfinal"
)

func TestCrossPlatformCoverageAicardChartSelectionContractsAreExecutableAndDisjoint(t *testing.T) {
	root := newAicardChartCommand()
	cases := []struct {
		name    string
		example string
		fields  []string
	}{
		{name: "list", example: "dws aicard chart list --format json", fields: []string{"registryVersion", "presets", "rendererVerified"}},
		{name: "explain", example: "dws aicard chart explain G01 --format json", fields: []string{"preset", "specVersion", "chartSpecExample", "rendererVerified"}},
		{name: "build", example: "dws aicard chart build --file ./chart-spec.json --out ./chart-card.json --format json", fields: []string{"presetId", "surfaceId", "preflight", "written"}},
	}
	seenUseWhen := map[string]string{}
	for _, tc := range cases {
		cmd, _, err := root.Find([]string{tc.name})
		if err != nil || cmd == nil {
			t.Fatalf("find chart %s: cmd=%v err=%v", tc.name, cmd, err)
		}
		final, ok := contractfinal.RuntimeContractFinal(cmd)
		if !ok || final.Selection == nil {
			t.Fatalf("chart %s missing final selection", tc.name)
		}
		if got := final.Selection.Examples; len(got) != 1 || got[0] != tc.example {
			t.Fatalf("chart %s examples = %#v, want %q", tc.name, got, tc.example)
		}
		if len(final.Selection.UseWhen) != 1 {
			t.Fatalf("chart %s use_when = %#v, want one scenario", tc.name, final.Selection.UseWhen)
		}
		if final.Result == nil {
			t.Fatalf("chart %s missing final result contract", tc.name)
		}
		var schema struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(final.Result.DataSchema, &schema); err != nil {
			t.Fatalf("chart %s result schema: %v", tc.name, err)
		}
		for _, field := range tc.fields {
			if _, ok := schema.Properties[field]; !ok {
				t.Fatalf("chart %s result schema missing %q: %s", tc.name, field, final.Result.DataSchema)
			}
		}
		useWhen := final.Selection.UseWhen[0]
		if tc.name == "build" {
			if _, exists := schema.Properties["artifact"]; exists {
				t.Fatal("build result schema advertises a payload that is only saved to the artifact file")
			}
		}
		if other, exists := seenUseWhen[useWhen]; exists {
			t.Fatalf("chart %s and %s share use_when %q", other, tc.name, useWhen)
		}
		seenUseWhen[useWhen] = tc.name
	}
}

func TestCrossPlatformCoverageAicardChartOfflineBuildAndNoOverwrite(t *testing.T) {
	caller := &aicardCaller{}
	input := aicardTestFile(t, "\ufeff"+`{"specVersion":"1.0","presetId":"G01","title":"Orders","data":{"kind":"category","categories":["A","B"],"series":[{"name":"New","values":[3,4]},{"name":"Returning","values":[1,2]}]}}`)
	outputPath := filepath.Join(t.TempDir(), "chart.json")
	result, code, err := executeAicard(t, caller, "chart", "build", "--file", input, "--out", outputPath, "--dry-run")
	if err != nil || code != 0 || result["data"].(map[string]any)["written"] != false {
		t.Fatalf("dry run: %v %d %+v", err, code, result)
	}
	if _, err := os.Stat(outputPath); !os.IsNotExist(err) {
		t.Fatalf("dry run wrote file: %v", err)
	}
	result, code, err = executeAicard(t, caller, "chart", "build", "--file", input, "--out", outputPath)
	if err != nil || code != 0 {
		t.Fatalf("build: %v %d %+v", err, code, result)
	}
	data := result["data"].(map[string]any)
	if _, exists := data["artifact"]; exists {
		t.Fatal("full business payload leaked to stdout")
	}
	if data["written"] != true || data["sent"] != false || data["preflight"].(map[string]any)["valid"] != true {
		t.Fatalf("incomplete build: %+v", data)
	}
	card, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	var messages []map[string]any
	if err := json.Unmarshal(card, &messages); err != nil || len(messages) != 1 {
		t.Fatalf("card: %v", err)
	}
	artifact, err := os.ReadFile(outputPath + ".artifact.json")
	if err != nil {
		t.Fatal(err)
	}
	var saved map[string]any
	if err := json.Unmarshal(artifact, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["surfaceId"] != data["surfaceId"] || len(saved["finishMessages"].([]any)) != 1 {
		t.Fatalf("lost FINISH artifact: %+v", saved)
	}
	result, code, err = executeAicard(t, caller, "chart", "build", "--file", input, "--out", outputPath)
	if err != nil || code == 0 {
		t.Fatalf("overwrote output: %v %d %+v", err, code, result)
	}
	if len(caller.calls) != 0 {
		t.Fatalf("offline build made network calls: %+v", caller.calls)
	}
}
