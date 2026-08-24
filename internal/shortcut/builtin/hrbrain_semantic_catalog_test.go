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

func TestCrossPlatformCoverageHRbrainSemanticCatalogExactlyCoversRegisteredSurface(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog_hrbrain.json")
	if err != nil {
		t.Fatal(err)
	}
	var source chatSemanticCatalogFixture
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service == "hrbrain" {
			registered[item.Command] = item
		}
	}
	if source.Service != "hrbrain" || len(source.Shortcuts) != 11 || len(registered) != 11 {
		t.Fatalf("service/catalog/registered = %q/%d/%d", source.Service, len(source.Shortcuts), len(registered))
	}
	var missing, stale []string
	blockerCounts := map[string]int{}
	for command, item := range registered {
		record, ok := source.Shortcuts[command]
		if !ok {
			missing = append(missing, command)
			continue
		}
		if record.Public || record.Availability != "" || !record.Reviewed || !item.SemanticReviewed || item.Availability != shortcut.AvailabilityUnavailable || !item.Hidden {
			t.Errorf("%s availability/review drift", command)
		}
		if strings.TrimSpace(item.SemanticDelta) == "" || item.SemanticDelta != record.SemanticDelta {
			t.Errorf("%s semantic delta drift", command)
		}
		if item.OutputRollout != output.RolloutUnifiedActive || item.Contract.Empty() || item.Contract.Result == nil || strings.TrimSpace(item.Safety.Effect) == "" {
			t.Errorf("%s lacks Contract/Result/Safety/unified output", command)
		}
		if item.Contract.Interface == nil || strings.Contains(item.Contract.Interface.Reason, "shortcut_defect") {
			t.Errorf("%s lacks a reviewed non-Shortcut blocker", command)
		} else if strings.Contains(item.Contract.Interface.Reason, "classified=adapter_business_service") {
			blockerCounts["adapter_business_service"]++
		} else if strings.Contains(item.Contract.Interface.Reason, "classified=tenant_fixture") {
			blockerCounts["tenant_fixture"]++
		} else {
			t.Errorf("%s has unknown blocker classification", command)
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
	if blockerCounts["adapter_business_service"] != 9 || blockerCounts["tenant_fixture"] != 2 {
		t.Fatalf("blocker counts = %#v", blockerCounts)
	}
}
