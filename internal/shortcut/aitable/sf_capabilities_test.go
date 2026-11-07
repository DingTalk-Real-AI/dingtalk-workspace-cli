// Copyright 2026 Alibaba Group
// SPDX-License-Identifier: Apache-2.0

package aitable

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
)

func TestCrossPlatformCoverageSFFieldCommandsKeepBuiltinAIWithoutAgentPrompt(t *testing.T) {
	const cfg = `{"type":"builtin","pluginId":"KNOWN_PLUGIN","formData":{"sourceField":"F"},"referFieldIds":["F"],"autoRecompute":false}`
	fields, err := parseBootstrapFields(`[{"fieldName":"AI","type":"object","description":"内置字段","aiConfig":` + cfg + `}]`)
	if err != nil {
		t.Fatal(err)
	}
	var expected map[string]any
	if err := json.Unmarshal([]byte(cfg), &expected); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(fields[0].(map[string]any)["aiConfig"], expected) {
		t.Fatalf("shared field parser changed configuration: %#v", fields)
	}
	if _, err := parseBootstrapFields(`[{"fieldName":"AI","type":"text","description":3,"aiConfig":` + cfg + `}]`); err == nil {
		t.Fatal("builtin bypassed field validation")
	}
	caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"success":true,"data":{"fieldId":"A","verificationStatus":"pending"}}`}}}
	if _, err := runAITableCompositeCLI(t, caller, "+field-update", "--base-id", "B", "--table-id", "T", "--field-id", "A", "--ai-config", cfg, "--yes"); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 1 || caller.calls[0].tool != "update_field" || !reflect.DeepEqual(caller.calls[0].args["aiConfig"], expected) {
		t.Fatal(caller.calls)
	}
	caller = &upsertByKeyCaller{steps: []upsertByKeyStep{
		{text: `{"success":true,"data":{"fields":[]}}`},
		{text: `{"success":true,"data":{"results":[{"success":true,"fieldId":"A","fieldName":"AI"}]}}`},
		{text: `{"success":true,"data":{"fields":[{"fieldId":"A","fieldName":"AI","type":"object","aiConfig":` + cfg + `}]}}`},
	}}
	if _, err := runAITableCompositeCLI(t, caller, "+field-create", "--base-id", "B", "--table-id", "T",
		"--fields", `[{"fieldName":"AI","type":"object","aiConfig":`+cfg+`}]`, "--yes"); err != nil {
		t.Fatal(err)
	}
	if len(caller.calls) != 3 || caller.calls[1].tool != "create_fields" ||
		!reflect.DeepEqual(caller.calls[1].args["fields"].([]any)[0].(map[string]any)["aiConfig"], expected) {
		t.Fatal(caller.calls)
	}
}

// Table creation has a narrower protocol than create_fields. Validate the whole
// declaration before creating even the Base, including fields after batch 15.
func TestCrossPlatformCoverageSFBootstrapRejectsBuiltinBeforeAnyMCP(t *testing.T) {
	for _, raw := range []string{
		`{"fieldName":"裸对象","type":"object"}`,
		`{"fieldName":"物流","type":"object","aiConfig":{"type":"builtin","pluginId":"06FB1281B673284C38B01C81745DAE60","formData":{"expressId":{"id":"tracking"}}}}`,
		`{"fieldName":"摘要","type":"text","aiConfig":{"type":"builtin","pluginId":"ai_summary","formData":{},"prompt":[{"type":"text","value":"不要绕过校验"}],"outputType":"text"}}`,
		`{"fieldName":"摘要","type":"text","config":{"decoratorConfigs":{"type":"builtin","configs":{"pluginId":"ai_summary","formData":{}}}}}`,
	} {
		fields := append(bootstrapFields(15), mustSFField(t, raw))
		encoded := mustJSON(t, fields)
		for _, command := range []string{"+table-bootstrap", "+base-bootstrap"} {
			for _, preview := range []bool{false, true} {
				caller := &upsertByKeyCaller{}
				args := []string{"--name", "配送", "--yes"}
				if command == "+table-bootstrap" {
					args = append(args, "--base-id", "base123", "--fields", encoded)
				} else {
					args = append(args, "--tables", `[{"name":"配送","fields":`+encoded+`}]`)
				}
				if preview {
					args = append(args, "--dry-run")
				}
				_, err := runAITableCompositeCLI(t, caller, command, args...)
				var typed *apperrors.Error
				if !errors.As(err, &typed) || typed.ExitCode() != 3 || !strings.Contains(err.Error(), "field create") || len(caller.calls) != 0 {
					t.Fatalf("command=%s preview=%t field=%s error=%v calls=%#v", command, preview, raw, err, caller.calls)
				}
			}
		}
	}
}

func TestCrossPlatformCoverageSFBuildPlanKeepsBuiltinOnFieldCreationOnly(t *testing.T) {
	field := mustSFField(t, `{"fieldName":"AI","type":"object","aiConfig":{"type":"builtin","pluginId":"KNOWN_PLUGIN","formData":{}}}`)
	for _, tool := range []string{"create_table", "create_fields"} {
		err := preflightPlanStep(buildPlanStep{ID: "step1", Tool: tool, Arguments: map[string]any{
			"tableId": "T", "tableName": "配送", "fields": []any{field},
		}})
		if tool == "create_table" {
			if err == nil || !strings.Contains(err.Error(), "field create") {
				t.Fatalf("unsupported table creation error=%v", err)
			}
		} else if err != nil {
			t.Fatalf("supported field creation error=%v", err)
		}
	}
}

func mustSFField(t *testing.T, raw string) map[string]any {
	t.Helper()
	var field map[string]any
	if err := json.Unmarshal([]byte(raw), &field); err != nil {
		t.Fatal(err)
	}
	return field
}

func TestCrossPlatformCoverageSFBuildPlanAllowsOrdinaryTableFields(t *testing.T) {
	for _, raw := range []string{
		`{"fieldName":"名称","type":"text"}`,
		`{"fieldName":"摘要","type":"text","aiConfig":{"prompt":[{"type":"text","value":"摘要"}],"outputType":"text"}}`,
	} {
		if err := preflightPlanStep(buildPlanStep{ID: "table", Tool: "create_table", Arguments: map[string]any{
			"tableName": "配送", "fields": []any{mustSFField(t, raw)},
		}}); err != nil {
			t.Fatalf("ordinary table field rejected: %s: %v", raw, err)
		}
	}
}

func TestCrossPlatformCoverageSFBuildPlanRejectsObjectBeforeAnyMCP(t *testing.T) {
	plan, checkpoint := setupBuildPlan(t)
	raw := `{"version":1,"baseId":"B","steps":[{"id":"table","tool":"create_table","arguments":{"baseId":"B","tableName":"表","fields":[{"fieldName":"对象","type":"object"}]},"verify":{"tool":"get_tables","arguments":{"baseId":"B"},"expected":{"data":{"tables":[]}}}}]}`
	if err := os.WriteFile(plan, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	for _, preview := range []bool{false, true} {
		caller := &upsertByKeyCaller{}
		args := []string{"+build-plan", "--plan", plan, "--checkpoint", checkpoint, "--yes"}
		if preview {
			args = append(args, "--dry-run")
		}
		_, err := runAITableCompositeCLI(t, caller, args[0], args[1:]...)
		var typed *apperrors.Error
		if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || !strings.Contains(err.Error(), "object") || len(caller.calls) != 0 {
			t.Fatalf("object plan reached MCP: preview=%t err=%v calls=%#v", preview, err, caller.calls)
		}
	}
}

func TestCrossPlatformCoverageSFGenericAIValidationRemainsStrict(t *testing.T) {
	for _, config := range []string{
		`42`, `{}`, `{"prompt":[]}`, `{"prompt":[42]}`,
		`{"prompt":[{"type":"text","value":""}]}`,
		`{"prompt":[{"type":"fieldRef"}]}`,
		`{"prompt":[{"type":"unknown"}]}`,
		`{"prompt":[{"type":"text","value":"摘要"}],"outputType":"number"}`,
	} {
		if _, err := parseBootstrapFields(`[{"fieldName":"AI","type":"text","aiConfig":` + config + `}]`); err == nil {
			t.Fatalf("generic AI validation bypassed: %s", config)
		}
	}
	for _, config := range []string{
		`{"prompt":[{"type":"text","value":"摘要"}],"outputType":"text"}`,
		`{"prompt":[{"type":"fieldRef","fieldId":"F"}],"outputType":"text"}`,
	} {
		if _, err := parseBootstrapFields(`[{"fieldName":"AI","type":"text","aiConfig":` + config + `}]`); err != nil {
			t.Fatalf("valid generic AI rejected: %s: %v", config, err)
		}
	}
}

func TestCrossPlatformCoverageSFTableCopyRejectsUnsupportedBeforeWrites(t *testing.T) {
	for _, raw := range []string{
		`{"fieldName":"对象","type":"object"}`,
		`{"fieldName":"对象别名","fieldType":"object"}`,
		`{"fieldName":"内置AI","type":"text","aiConfig":{"type":"builtin","pluginId":"known","formData":{}}}`,
		`{"fieldName":"旧内置AI","type":"text","config":{"decoratorConfigs":{"type":"builtin","configs":{"pluginId":"known","formData":{}}}}}`,
	} {
		for _, position := range []int{0, 15} {
			for _, preview := range []bool{false, true} {
				for _, strict := range []bool{false, true} {
					t.Run(fmt.Sprintf("%s/position=%d/preview=%t/strict=%t", raw, position, preview, strict), func(t *testing.T) {
						fields := make([]any, 0, position+1)
						for i := 0; i < position; i++ {
							fields = append(fields, map[string]any{"fieldId": fmt.Sprintf("F%d", i), "fieldName": fmt.Sprintf("普通%d", i), "type": "text"})
						}
						field := mustSFField(t, raw)
						field["fieldId"] = "AI"
						fields = append(fields, field)
						caller := &upsertByKeyCaller{steps: []upsertByKeyStep{{text: `{"success":true,"data":{"fields":` + mustJSON(t, fields) + `}}`}}}
						args := []string{"--source-base-id", "SOURCE", "--source-table-id", "SOURCE_TABLE", "--target-base-id", "TARGET", "--new-name", "副本", "--include-records", "--yes"}
						if preview {
							args = append(args, "--dry-run")
						}
						if strict {
							args = append(args, "--strict-fields")
						}
						_, err := runAITableCompositeCLI(t, caller, "+table-copy", args...)
						var typed *apperrors.Error
						if !errors.As(err, &typed) || typed.Category != apperrors.CategoryValidation || typed.ExitCode() != 3 || typed.Reason != "table_copy_unsupported_fields" || !strings.Contains(typed.Message, "field create") {
							t.Fatalf("unsupported copy accepted: err=%v", err)
						}
						if len(caller.calls) != 1 || caller.calls[0].tool != "get_fields" || caller.calls[0].args["baseId"] != "SOURCE" || caller.calls[0].args["tableId"] != "SOURCE_TABLE" {
							t.Fatalf("unsupported copy reached record reads or writes: %#v", caller.calls)
						}
					})
				}
			}
		}
	}
}

func TestCrossPlatformCoverageSFTableCopyKeepsOrdinaryAndGenericAI(t *testing.T) {
	for _, raw := range []string{
		`{"fieldName":"普通","type":"text"}`,
		`{"fieldName":"通用AI","type":"text","aiConfig":{"prompt":[{"type":"text","value":"摘要"}],"outputType":"text"}}`,
	} {
		field := mustSFField(t, raw)
		source := mustSFField(t, raw)
		source["fieldId"] = "SOURCE_FIELD"
		target := mustSFField(t, raw)
		target["fieldId"] = "TARGET_FIELD"
		caller := &upsertByKeyCaller{steps: []upsertByKeyStep{
			{text: `{"success":true,"data":{"fields":[` + mustJSON(t, source) + `]}}`},
			{text: `{"success":true,"data":{"tableId":"TARGET_TABLE"}}`},
			{text: `{"success":true,"data":{"fields":[` + mustJSON(t, target) + `]}}`},
		}}
		_, err := runAITableCompositeCLI(t, caller, "+table-copy", "--source-base-id", "SOURCE", "--source-table-id", "SOURCE_TABLE", "--target-base-id", "TARGET", "--new-name", "副本", "--yes")
		if err != nil || len(caller.calls) != 3 || caller.calls[1].tool != "create_table" || !reflect.DeepEqual(caller.calls[1].args["fields"], []any{field}) {
			t.Fatalf("ordinary copy rejected or configuration changed: err=%v calls=%#v", err, caller.calls)
		}
	}
}
