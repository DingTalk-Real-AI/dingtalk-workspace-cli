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

func TestCrossPlatformCoverageReportSemanticCatalogExactlyCoversRegisteredSurface(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog_report.json")
	if err != nil {
		t.Fatal(err)
	}
	var source chatSemanticCatalogFixture
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service == "report" {
			registered[item.Command] = item
		}
	}
	if source.Service != "report" || len(registered) != 4 || len(source.Shortcuts) != 4 {
		t.Fatalf("service/registered/catalog=%s/%d/%d, want report/4/4", source.Service, len(registered), len(source.Shortcuts))
	}
	public, unavailable := 0, 0
	var missing, stale []string
	for command, item := range registered {
		record, ok := source.Shortcuts[command]
		if !ok {
			missing = append(missing, command)
			continue
		}
		availability := record.Availability
		if availability == "" {
			availability = source.Availability
		}
		if !record.Reviewed || !item.SemanticReviewed || strings.TrimSpace(item.SemanticDelta) == "" || item.SemanticDelta != record.SemanticDelta {
			t.Errorf("%s semantic review facts drifted", command)
		}
		if item.Risk != record.Risk || item.Availability != availability {
			t.Errorf("%s risk/availability=%q/%q want=%q/%q", command, item.Risk, item.Availability, record.Risk, availability)
		}
		if record.Public {
			public++
			if availability != shortcut.AvailabilityAvailable || item.Hidden || !shortcut.InPublicCatalog("report", command) {
				t.Errorf("%s public availability/visibility drift", command)
			}
			if item.Contract.Empty() || item.Contract.Result == nil || item.Safety.Effect == "" || item.OutputRollout != output.RolloutUnifiedActive {
				t.Errorf("%s lacks public Contract/Safety/Result/unified output", command)
			}
		} else {
			if !item.Hidden || shortcut.InPublicCatalog("report", command) {
				t.Errorf("%s nonpublic shortcut is visible", command)
			}
			if availability == shortcut.AvailabilityUnavailable {
				unavailable++
			}
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
	if public != 4 || unavailable != 0 {
		t.Fatalf("public/unavailable=%d/%d, want 4/0", public, unavailable)
	}
}
