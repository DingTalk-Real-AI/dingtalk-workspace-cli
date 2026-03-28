package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestMCPJSONFixturesAreValidAndToolShapesAreUsable(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob(filepath.Join("..", "..", "docs", "mcp", "*.json"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(files) == 0 {
		t.Skip("no docs/mcp/*.json fixtures found; skipping")
	}

	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			t.Parallel()

			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("ReadFile(%s) error = %v", file, err)
			}
			var payload map[string]any
			if err := json.Unmarshal(data, &payload); err != nil {
				t.Fatalf("json.Unmarshal(%s) error = %v", file, err)
			}

			if tools := extractTools(payload); tools != nil {
				for idx, tool := range tools {
					if _, ok := tool["name"].(string); ok {
						continue
					}
					if _, ok := tool["toolName"].(string); ok {
						continue
					}
					if _, ok := tool["title"].(string); ok {
						continue
					}
					t.Fatalf("tools[%d] in %s missing name/toolName/title", idx, file)
				}
			}
		})
	}
}

func extractTools(payload map[string]any) []map[string]any {
	if payload == nil {
		return nil
	}
	if result, ok := payload["result"].(map[string]any); ok {
		if tools, ok := toToolList(result["tools"]); ok {
			return tools
		}
	}
	if tools, ok := toToolList(payload["tools"]); ok {
		return tools
	}
	return nil
}

func toToolList(value any) ([]map[string]any, bool) {
	arr, ok := value.([]any)
	if !ok {
		return nil, false
	}
	out := make([]map[string]any, 0, len(arr))
	for _, item := range arr {
		tool, ok := item.(map[string]any)
		if !ok {
			continue
		}
		out = append(out, tool)
	}
	return out, true
}
