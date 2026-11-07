// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package shortcut

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestCrossPlatformCoveragePublicCatalogArtifactsStayConsistent(t *testing.T) {
	_, testFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate public catalog test file")
	}
	catalogPath := filepath.Join(filepath.Dir(testFile), "..", "..", "docs", "shortcut-public-catalog.json")
	raw, err := os.ReadFile(catalogPath)
	if err != nil {
		t.Fatalf("read public catalog: %v", err)
	}
	var catalog struct {
		Count   int `json:"count"`
		Results []struct {
			Service       string `json:"service"`
			Command       string `json:"command"`
			SemanticDelta string `json:"semantic_delta"`
		} `json:"results"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("decode public catalog: %v", err)
	}
	if catalog.Count != len(catalog.Results) {
		t.Fatalf("public catalog count = %d, results = %d", catalog.Count, len(catalog.Results))
	}
	if catalog.Count != len(publicShortcutCatalog) {
		t.Fatalf("public catalog count = %d, generated Go keys = %d", catalog.Count, len(publicShortcutCatalog))
	}
	seen := make(map[string]struct{}, len(catalog.Results))
	for i, row := range catalog.Results {
		key := publicCatalogKey(row.Service, row.Command)
		if row.Service == "" || row.Command == "" {
			t.Fatalf("public catalog result %d has an empty identity", i)
		}
		if _, duplicate := seen[key]; duplicate {
			t.Fatalf("public catalog result %d duplicates %q", i, key)
		}
		seen[key] = struct{}{}
		if _, generated := publicShortcutCatalog[key]; !generated {
			t.Fatalf("public catalog result %d is missing from generated Go keys: %q", i, key)
		}
		if row.Service == "contact" && (row.Command == "+search-mobile" || row.Command == "+by-mobile") {
			if !strings.Contains(row.SemanticDelta, "专用手机号精确查询接口") {
				t.Fatalf("public catalog result %d %q has stale mobile lookup semantics", i, key)
			}
			if strings.Contains(row.SemanticDelta, "关键词能力") || strings.Contains(row.SemanticDelta, "关键词数组") {
				t.Fatalf("public catalog result %d %q references the retired keyword-search route", i, key)
			}
		}
	}
	for key := range publicShortcutCatalog {
		if _, documented := seen[key]; !documented {
			t.Fatalf("generated Go key is missing from public catalog JSON: %q", key)
		}
	}
}
