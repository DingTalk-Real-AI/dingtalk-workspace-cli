package contract_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestGeneratedSchemaJSONFixturesAreValidAndToolShapesAreUsable(t *testing.T) {
	t.Parallel()

	files, err := filepath.Glob(filepath.Join(generatedSchemaRoot(), "*.json"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(files) == 0 {
		t.Skip("no generated schema fixtures found; skipping")
	}

	for _, file := range files {
		file := file
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

			if filepath.Base(file) == "catalog.json" {
				products, ok := payload["products"].([]any)
				if !ok || len(products) == 0 {
					t.Fatalf("catalog fixture %s missing products", file)
				}
				return
			}

			if _, ok := payload["path"].(string); !ok {
				t.Fatalf("schema fixture %s missing path", file)
			}
			if _, ok := payload["cli_path"].([]any); !ok {
				t.Fatalf("schema fixture %s missing cli_path", file)
			}

			tool, ok := payload["tool"].(map[string]any)
			if !ok {
				t.Fatalf("schema fixture %s missing tool summary", file)
			}
			if _, ok := tool["rpc_name"].(string); !ok {
				t.Fatalf("schema fixture %s missing tool.rpc_name", file)
			}
			if _, ok := tool["canonical_path"].(string); !ok {
				t.Fatalf("schema fixture %s missing tool.canonical_path", file)
			}
		})
	}
}
