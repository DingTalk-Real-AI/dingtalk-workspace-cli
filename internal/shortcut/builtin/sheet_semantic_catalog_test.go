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

func TestCrossPlatformCoverageSheetSemanticCatalogExactlyCoversRegisteredSurface(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog_sheet.json")
	if err != nil {
		t.Fatal(err)
	}
	var source chatSemanticCatalogFixture
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	if source.Service != "sheet" {
		t.Fatalf("service=%q", source.Service)
	}
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service != "sheet" {
			continue
		}
		if _, duplicate := registered[item.Command]; duplicate {
			t.Fatalf("duplicate Sheet Shortcut %s", item.Command)
		}
		registered[item.Command] = item
	}
	if len(registered) != 2 || len(source.Shortcuts) != 2 {
		t.Fatalf("registered/catalog=%d/%d, want 2/2", len(registered), len(source.Shortcuts))
	}
	var missing, stale []string
	for command, item := range registered {
		record, ok := source.Shortcuts[command]
		if !ok {
			missing = append(missing, command)
			continue
		}
		if !record.Reviewed || !item.SemanticReviewed || strings.TrimSpace(item.SemanticDelta) == "" || item.SemanticDelta != record.SemanticDelta {
			t.Errorf("%s semantic review facts drifted", command)
		}
		if !record.Public || item.Hidden || !shortcut.InPublicCatalog("sheet", command) {
			t.Errorf("%s is not delivered as reviewed public", command)
		}
		if item.Availability != shortcut.AvailabilityAvailable || item.Risk != record.Risk {
			t.Errorf("%s availability/risk=%q/%q", command, item.Availability, item.Risk)
		}
		if item.Contract.Empty() || item.Contract.Result == nil || item.Contract.Pagination != nil || strings.TrimSpace(item.Safety.Effect) == "" || item.OutputRollout != output.RolloutUnifiedActive {
			t.Errorf("%s lacks Result/Safety/unified output or publishes false pagination", command)
		}
	}
	for command := range source.Shortcuts {
		if _, ok := registered[command]; !ok {
			stale = append(stale, command)
		}
	}
	sort.Strings(missing)
	sort.Strings(stale)
	if len(missing) != 0 || len(stale) != 0 {
		t.Fatalf("catalog mismatch: missing=%v stale=%v", missing, stale)
	}
}
