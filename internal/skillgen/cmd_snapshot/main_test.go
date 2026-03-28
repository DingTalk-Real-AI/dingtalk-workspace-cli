// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"reflect"
	"testing"
)

func TestFirstSuccessfulInitializeResponsePrefersSuccess(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"initialize": map[string]any{
			"attempts": []any{
				map[string]any{
					"response": map[string]any{
						"jsonrpc": "2.0",
						"id":      1,
						"error": map[string]any{
							"code":    -32000,
							"message": "temporary unavailable",
						},
					},
				},
				map[string]any{
					"response": map[string]any{
						"jsonrpc": "2.0",
						"id":      1,
						"result": map[string]any{
							"protocolVersion": "2025-03-26",
						},
					},
				},
			},
		},
	}

	response := firstSuccessfulInitializeResponse(payload)
	protocolVersion := stringValue(mapValue(mapValue(response)["result"])["protocolVersion"])
	if protocolVersion != "2025-03-26" {
		t.Fatalf("firstSuccessfulInitializeResponse() protocolVersion = %q, want 2025-03-26", protocolVersion)
	}
}

func TestMergedSuccessfulMethodResponseAggregatesToolsAcrossPages(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"methods": map[string]any{
			"tools/list": map[string]any{
				"pages": []any{
					map[string]any{
						"response": map[string]any{
							"jsonrpc": "2.0",
							"id":      2,
							"error": map[string]any{
								"code":    -32603,
								"message": "temporary error",
							},
						},
					},
					map[string]any{
						"response": map[string]any{
							"jsonrpc": "2.0",
							"id":      2,
							"result": map[string]any{
								"tools": []any{
									map[string]any{"name": "tool_a"},
								},
							},
						},
					},
					map[string]any{
						"response": map[string]any{
							"jsonrpc": "2.0",
							"id":      2,
							"result": map[string]any{
								"tools": []any{
									map[string]any{"name": "tool_b"},
								},
							},
						},
					},
				},
			},
		},
	}

	response := mergedSuccessfulMethodResponse(payload, "tools/list")
	tools := mapValue(mapValue(response)["result"])["tools"]
	values, ok := sliceValue(tools)
	if !ok {
		t.Fatalf("mergedSuccessfulMethodResponse() tools type = %T, want []any", tools)
	}

	toolNames := make([]string, 0, len(values))
	for _, raw := range values {
		toolNames = append(toolNames, stringValue(mapValue(raw)["name"]))
	}
	if !reflect.DeepEqual(toolNames, []string{"tool_a", "tool_b"}) {
		t.Fatalf("mergedSuccessfulMethodResponse() tools = %#v, want [tool_a tool_b]", toolNames)
	}
}

func TestMergedSuccessfulMethodResponseFallsBackWhenNoSuccess(t *testing.T) {
	t.Parallel()

	payload := map[string]any{
		"methods": map[string]any{
			"tools/list": map[string]any{
				"pages": []any{
					map[string]any{
						"response": map[string]any{
							"jsonrpc": "2.0",
							"id":      2,
							"error": map[string]any{
								"code":    -32601,
								"message": "method not found",
							},
						},
					},
					map[string]any{
						"response": map[string]any{
							"jsonrpc": "2.0",
							"id":      2,
							"error": map[string]any{
								"code":    -32601,
								"message": "method not found",
							},
						},
					},
				},
			},
		},
	}

	response := mergedSuccessfulMethodResponse(payload, "tools/list")
	if mapValue(response)["error"] == nil {
		t.Fatalf("mergedSuccessfulMethodResponse() expected fallback error response, got %#v", response)
	}
}
