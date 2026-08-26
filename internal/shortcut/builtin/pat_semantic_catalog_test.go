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

func TestCrossPlatformCoveragePATSemanticCatalogExactlyCoversRegisteredSurface(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog_pat.json")
	if err != nil {
		t.Fatal(err)
	}
	var source chatSemanticCatalogFixture
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service == "pat" {
			registered[item.Command] = item
		}
	}
	if source.Service != "pat" || len(source.Shortcuts) != 2 || len(registered) != 2 {
		t.Fatalf("service/catalog/registered = %q/%d/%d", source.Service, len(source.Shortcuts), len(registered))
	}
	public := 0
	for command, record := range source.Shortcuts {
		item, ok := registered[command]
		if !ok || !record.Reviewed || !item.SemanticReviewed || strings.TrimSpace(item.SemanticDelta) == "" || item.SemanticDelta != record.SemanticDelta {
			t.Fatalf("%s semantic review drift", command)
		}
		if record.Public {
			public++
			if item.Hidden || item.Availability != shortcut.AvailabilityAvailable || item.OutputRollout != output.RolloutUnifiedActive || item.Contract.Result == nil || strings.TrimSpace(item.Safety.Effect) == "" {
				t.Fatalf("%s public contract drift", command)
			}
		} else if !item.Hidden || item.Availability != shortcut.AvailabilityUnavailable {
			t.Fatalf("%s unavailable visibility drift", command)
		}
		if command == "+authorize" {
			if record.Disposition != shortcut.DispositionAliasInternal || record.Primary != "pat chmod" || item.PrimaryCommand != "pat chmod" || !strings.Contains(item.SemanticDelta, "classified=routed") {
				t.Fatalf("%s must route to the reviewed pat chmod execution chain", command)
			}
			if item.Contract.Interface == nil || !strings.Contains(item.Contract.Interface.Reason, "blocked_fixture") || !strings.Contains(item.Contract.Interface.Reason, "classified=routed") {
				t.Fatalf("%s must separate routed implementation from unproved live write", command)
			}
		}
	}
	if public != 1 {
		t.Fatalf("public PAT shortcuts = %d, want 1", public)
	}
}
