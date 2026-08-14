// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package builtin_test

import (
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageWikiSemanticCatalogExactlyCoversRegisteredSurface(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog_wiki.json")
	if err != nil {
		t.Fatal(err)
	}
	var source chatSemanticCatalogFixture
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	if source.Service != "wiki" {
		t.Fatalf("service = %q", source.Service)
	}
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service == "wiki" {
			if _, exists := registered[item.Command]; exists {
				t.Fatalf("duplicate %s", item.Command)
			}
			registered[item.Command] = item
		}
	}
	if len(registered) != 20 || len(source.Shortcuts) != 20 {
		t.Fatalf("registered/catalog = %d/%d, want 20/20", len(registered), len(source.Shortcuts))
	}
	var missing, stale []string
	for command, item := range registered {
		record, ok := source.Shortcuts[command]
		if !ok {
			missing = append(missing, command)
			continue
		}
		if !record.Reviewed || !item.SemanticReviewed || item.Hidden || !record.Public {
			t.Errorf("%s: not publicly reviewed", command)
		}
		if strings.TrimSpace(record.SemanticDelta) == "" || item.SemanticDelta != record.SemanticDelta || item.Disposition != record.Disposition {
			t.Errorf("%s: semantic facts drifted", command)
		}
		if item.Risk != record.Risk {
			t.Errorf("%s: risk=%q want=%q", command, item.Risk, record.Risk)
		}
		if item.Contract.Empty() || item.Contract.Result == nil || strings.TrimSpace(item.Safety.Effect) == "" || item.OutputRollout != output.RolloutUnifiedActive {
			t.Errorf("%s: incomplete contract/safety/result/output", command)
		}
	}
	for command := range source.Shortcuts {
		if _, ok := registered[command]; !ok {
			stale = append(stale, command)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) > 0 || len(stale) > 0 {
		t.Fatalf("catalog mismatch: missing=%v stale=%v", missing, stale)
	}
}
