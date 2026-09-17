// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/corecmd/contract"
)

func TestResultContractModelWireRoundTripAndCompactPolicy(t *testing.T) {
	result := &contract.ResultSpec{
		Outcomes:       []contract.ResultOutcome{contract.ResultOutcomeFailure, contract.ResultOutcomeSuccess},
		DataSchema:     json.RawMessage(`{ "type":"object", "properties":{"items":{"type":"array","description":"Business result records","items":{"type":"object"}}} }`),
		SensitivePaths: []string{"items.secret", "credential"},
	}
	spec, err := ToolSpecFromRuntime(RuntimeToolSpecInput{
		Identity:   contract.ToolIdentitySpec{ProductID: "dev", Name: "list", CLIName: "list", CLIPath: "dev list"},
		Parameters: []ParameterSpec{{Name: "cursor", Type: "string"}},
		Result:     result,
		Pagination: &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "cursor"},
	})
	if err != nil {
		t.Fatalf("ToolSpecFromRuntime() error = %v", err)
	}
	if got, want := spec.Result.Outcomes, []contract.ResultOutcome{contract.ResultOutcomeSuccess, contract.ResultOutcomeFailure}; !reflect.DeepEqual(got, want) {
		t.Fatalf("outcomes = %#v, want %#v", got, want)
	}
	result.Outcomes[0] = contract.ResultOutcomePending
	result.DataSchema[0] = '['
	if spec.Result.Outcomes[0] != contract.ResultOutcomeSuccess || spec.Result.DataSchema[0] != '{' {
		t.Fatal("ToolSpec result aliases runtime input")
	}

	payload, err := spec.ToPayload()
	if err != nil {
		t.Fatalf("ToPayload() error = %v", err)
	}
	resultPayload, ok := payload["result"].(map[string]any)
	if !ok || schemaString(resultPayload["data_schema"].(map[string]any)["type"]) != "object" {
		t.Fatalf("result payload = %#v", payload["result"])
	}
	if _, exists := specResultSummary(t, spec)["result"]; exists {
		t.Fatal("result must remain full-leaf-only")
	}
	compactResult, exists := stripSchemaPayloadCompact(payload)["result"].(map[string]any)
	if !exists {
		t.Fatal("compact leaf must include the reviewed result contract")
	}
	if outcomes, ok := compactResult["outcomes"].([]any); !ok || len(outcomes) != 2 {
		t.Fatalf("compact result outcomes = %#v", compactResult["outcomes"])
	}
	if dataSchema, ok := compactResult["data_schema"].(map[string]any); !ok || schemaString(dataSchema["type"]) != "object" {
		t.Fatalf("compact result data_schema = %#v", compactResult["data_schema"])
	}
	if !reflect.DeepEqual(compactResult, resultPayload) {
		t.Fatalf("compact result must equal full-leaf result\ncompact: %#v\nfull: %#v", compactResult, resultPayload)
	}
	compactPagination, exists := stripSchemaPayloadCompact(payload)["pagination"].(map[string]any)
	if !exists || schemaString(compactPagination["meta_path"]) != contract.PaginationMetaPath || schemaString(compactPagination["cursor_parameter"]) != "cursor" {
		t.Fatalf("compact pagination = %#v", compactPagination)
	}

	wire, err := schemaToolWireFromPayload(payload)
	if err != nil {
		t.Fatalf("schemaToolWireFromPayload() error = %v", err)
	}
	roundTrip, err := schemaToolSpecFromWire(wire)
	if err != nil {
		t.Fatalf("schemaToolSpecFromWire() error = %v", err)
	}
	roundTripPayload, err := roundTrip.ToPayload()
	if err != nil {
		t.Fatalf("round-trip ToPayload() error = %v", err)
	}
	if !schemaJSONEqual(payload, roundTripPayload) {
		t.Fatalf("result changed across wire round-trip\nfirst: %#v\nround: %#v", payload["result"], roundTripPayload["result"])
	}
}

func specResultSummary(t *testing.T, spec ToolSpec) map[string]any {
	t.Helper()
	payload, err := spec.ToSummaryPayload()
	if err != nil {
		t.Fatalf("ToSummaryPayload() error = %v", err)
	}
	return payload
}

func TestToolWithoutResultKeepsResultAbsent(t *testing.T) {
	spec, err := ToolSpecFromRuntime(RuntimeToolSpecInput{
		Identity: contract.ToolIdentitySpec{ProductID: "dev", Name: "legacy", CLIName: "legacy", CLIPath: "dev legacy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := spec.ToPayload()
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := payload["result"]; exists {
		t.Fatal("tool without Result gained a result key")
	}
}

func TestToolSpecRejectsInvalidResultInsteadOfDroppingIt(t *testing.T) {
	_, err := ToolSpecFromRuntime(RuntimeToolSpecInput{
		Identity: contract.ToolIdentitySpec{ProductID: "dev", Name: "invalid", CLIName: "invalid", CLIPath: "dev invalid"},
		Result: &contract.ResultSpec{
			Outcomes:   []contract.ResultOutcome{contract.ResultOutcomeSuccess},
			DataSchema: json.RawMessage(`[]`),
		},
	})
	if err == nil {
		t.Fatal("invalid result schema was silently dropped")
	}
}

func TestToolSpecRejectsInvalidOrUndeclaredPaginationCursor(t *testing.T) {
	identity := contract.ToolIdentitySpec{ProductID: "dev", Name: "list", CLIName: "list", CLIPath: "dev list"}
	for _, tc := range []struct {
		name       string
		parameters []ParameterSpec
		pagination *contract.PaginationSpec
		want       string
	}{
		{
			name:       "invalid pagination declaration",
			parameters: []ParameterSpec{{Name: "cursor", Type: "string"}},
			pagination: &contract.PaginationSpec{Kind: "offset", CursorParameter: "cursor"},
			want:       "unsupported kind",
		},
		{
			name:       "cursor is not a parameter",
			pagination: &contract.PaginationSpec{Kind: contract.PaginationKindCursor, CursorParameter: "cursor"},
			want:       "is not a declared parameter",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ToolSpecFromRuntime(RuntimeToolSpecInput{
				Identity: identity, Parameters: tc.parameters, Pagination: tc.pagination,
			})
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ToolSpecFromRuntime() error = %v, want %q", err, tc.want)
			}
		})
	}
}
