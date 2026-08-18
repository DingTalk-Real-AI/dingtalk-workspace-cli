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

func TestCrossPlatformCoverageLiveSemanticCatalogExactlyCoversRegisteredSurface(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog_live.json")
	if err != nil {
		t.Fatal(err)
	}
	var source chatSemanticCatalogFixture
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service == "live" {
			registered[item.Command] = item
		}
	}
	if source.Service != "live" || len(registered) != 1 || len(source.Shortcuts) != 1 {
		t.Fatalf("service/registered/catalog = %q/%d/%d, want live/1/1", source.Service, len(registered), len(source.Shortcuts))
	}
	record := source.Shortcuts["+list-my-lives"]
	item, ok := registered["+list-my-lives"]
	if !ok {
		t.Fatal("registered Live shortcut missing")
	}
	if !record.Reviewed || record.Public || record.Availability != shortcut.AvailabilityUnavailable {
		t.Fatalf("semantic record = %#v", record)
	}
	if !item.SemanticReviewed || item.SemanticDelta != record.SemanticDelta || item.Risk != record.Risk {
		t.Fatal("Live semantic facts drifted")
	}
	if !item.Hidden || item.Availability != shortcut.AvailabilityUnavailable || shortcut.InPublicCatalog("live", item.Command) {
		t.Fatalf("Live visibility drift: hidden=%v availability=%q", item.Hidden, item.Availability)
	}
	if item.Contract.Empty() || item.Contract.Result == nil || strings.TrimSpace(item.Safety.Effect) == "" || item.OutputRollout != output.RolloutUnifiedActive {
		t.Fatal("Live shortcut lacks Contract/Result/Safety/unified output")
	}
}
