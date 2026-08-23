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

func TestCrossPlatformCoverageDINGSemanticCatalogExactlyCoversRegisteredSurface(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog_ding.json")
	if err != nil {
		t.Fatal(err)
	}
	var source chatSemanticCatalogFixture
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service == "ding" {
			registered[item.Command] = item
		}
	}
	if source.Service != "ding" || len(registered) != 5 || len(source.Shortcuts) != 5 {
		t.Fatalf("service/registered/catalog=%s/%d/%d, want ding/5/5", source.Service, len(registered), len(source.Shortcuts))
	}
	wantCompatibilityVisible := map[string]bool{
		"+list":            true,
		"+recall-personal": true,
		"+send-personal":   true,
	}
	wantCompatibilityAvailability := map[string]shortcut.Availability{
		"+list":            shortcut.AvailabilityUnavailable,
		"+recall-personal": shortcut.AvailabilityAvailable,
		"+send-personal":   shortcut.AvailabilityAvailable,
	}
	public, unavailable, compatibilityVisible := 0, 0, 0
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
			if availability != shortcut.AvailabilityAvailable || item.Hidden || !shortcut.InPublicCatalog("ding", command) {
				t.Errorf("%s public availability/visibility drift", command)
			}
			if item.Contract.Empty() || item.Contract.Result == nil || item.Safety.Effect == "" || item.OutputRollout != output.RolloutUnifiedActive {
				t.Errorf("%s lacks public Contract/Safety/Result/unified output", command)
			}
		} else {
			if record.CompatibilityVisible {
				compatibilityVisible++
				if item.Hidden || !item.CompatibilityVisible || availability != wantCompatibilityAvailability[command] || shortcut.InPublicCatalog("ding", command) {
					t.Errorf("%s compatibility-visible boundary drift", command)
				}
			} else if !item.Hidden || item.CompatibilityVisible || shortcut.InPublicCatalog("ding", command) {
				t.Errorf("%s nonpublic shortcut visibility drift", command)
			}
			if availability == shortcut.AvailabilityUnavailable {
				unavailable++
			}
		}
		if record.CompatibilityVisible != wantCompatibilityVisible[command] {
			t.Errorf("%s compatibility-visible=%v, want %v", command, record.CompatibilityVisible, wantCompatibilityVisible[command])
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
	if public != 1 || unavailable != 2 || compatibilityVisible != len(wantCompatibilityVisible) {
		t.Fatalf("public/unavailable/compatibility-visible=%d/%d/%d, want 1/2/%d", public, unavailable, compatibilityVisible, len(wantCompatibilityVisible))
	}
}
