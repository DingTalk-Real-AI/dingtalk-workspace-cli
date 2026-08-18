// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package builtin_test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageContactSemanticCatalogExactlyCoversRegisteredSurface(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog_contact.json")
	if err != nil {
		t.Fatal(err)
	}
	var source chatSemanticCatalogFixture
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	if source.Service != "contact" {
		t.Fatalf("service = %q", source.Service)
	}
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service == "contact" {
			registered[item.Command] = item
		}
	}
	if len(registered) != 16 || len(source.Shortcuts) != 16 {
		t.Fatalf("registered/catalog = %d/%d, want 16/16", len(registered), len(source.Shortcuts))
	}
	public, unavailable := 0, 0
	for command, record := range source.Shortcuts {
		item, ok := registered[command]
		if !ok {
			t.Errorf("catalog contains stale %s", command)
			continue
		}
		availability := record.Availability
		if availability == "" {
			availability = source.Availability
		}
		if !record.Reviewed || !item.SemanticReviewed || item.SemanticDelta != record.SemanticDelta || item.Risk != record.Risk {
			t.Errorf("%s semantic facts drifted", command)
		}
		if item.Contract.Empty() || item.Contract.Result == nil || strings.TrimSpace(item.Safety.Effect) == "" || item.OutputRollout != output.RolloutUnifiedActive {
			t.Errorf("%s lacks Contract/Result/Safety/unified output", command)
		}
		if record.Public {
			public++
			if availability != shortcut.AvailabilityAvailable || item.Hidden || !shortcut.InPublicCatalog("contact", command) {
				t.Errorf("%s public availability drift", command)
			}
		} else {
			unavailable++
			if availability != shortcut.AvailabilityUnavailable || !item.Hidden || shortcut.InPublicCatalog("contact", command) {
				t.Errorf("%s unavailable visibility drift", command)
			}
		}
	}
	if public != 13 || unavailable != 3 {
		t.Fatalf("public/unavailable = %d/%d, want 13/3", public, unavailable)
	}
}
