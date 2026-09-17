// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package app

import "testing"

func TestCrossPlatformCoverageMinutesPaginationFinalSchemaMatchesRuntimeEnvelope(t *testing.T) {
	for _, cliPath := range []string{
		"minutes +list-mine",
		"minutes +list-shared",
		"minutes +list-all",
		"minutes +search",
		"minutes +transcript",
	} {
		t.Run(cliPath, func(t *testing.T) {
			leaf := executeShortcutSchemaQuery(t, "--cli-path", cliPath)
			pagination, ok := leaf["pagination"].(map[string]any)
			if !ok {
				t.Fatalf("%s pagination=%T, want object", cliPath, leaf["pagination"])
			}
			for field, want := range map[string]string{
				"kind":                    "cursor",
				"cursor_parameter":        "cursor",
				"meta_path":               "meta.pagination",
				"endpoint_exhausted_path": "meta.pagination.endpoint_exhausted",
				"next_token_path":         "meta.pagination.next_token",
			} {
				if got := schemaContractString(pagination[field]); got != want {
					t.Errorf("%s pagination.%s=%q, want %q", cliPath, field, got, want)
				}
			}
			result := schemaContractMap(leaf["result"])
			dataSchema := schemaContractMap(result["data_schema"])
			properties := schemaContractMap(dataSchema["properties"])
			for _, field := range []string{"endpointExhausted", "nextToken"} {
				if _, exists := properties[field]; exists {
					t.Errorf("%s Result data_schema leaked pagination field %q", cliPath, field)
				}
			}
		})
	}
}
