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

func TestCrossPlatformCoverageDevdocSemanticCatalogExactlyCoversRegisteredSurface(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog_devdoc.json")
	if err != nil {
		t.Fatal(err)
	}
	var source chatSemanticCatalogFixture
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	if source.Service != "devdoc" || len(source.Shortcuts) != 1 {
		t.Fatalf("service/count = %q/%d", source.Service, len(source.Shortcuts))
	}
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service == "devdoc" {
			registered[item.Command] = item
		}
	}
	if len(registered) != 1 {
		t.Fatalf("registered Devdoc shortcuts = %d, want 1", len(registered))
	}
	for command, record := range source.Shortcuts {
		item, ok := registered[command]
		if !ok {
			t.Fatalf("catalog entry %s is not registered", command)
		}
		if !record.Reviewed || record.Public || record.Availability != shortcut.AvailabilityUnavailable {
			t.Fatalf("catalog entry is not reviewed private unavailable: %#v", record)
		}
		if !item.Hidden || !item.SemanticReviewed || item.Availability != shortcut.AvailabilityUnavailable || item.SemanticDelta != record.SemanticDelta {
			t.Fatalf("runtime semantic facts drifted: %#v", item)
		}
		if item.Contract.Empty() || item.Contract.Result == nil || item.OutputRollout != output.RolloutUnifiedActive || strings.TrimSpace(item.Safety.Effect) == "" {
			t.Fatal("unavailable Devdoc Shortcut lacks Contract/Result/Safety/unified output")
		}
		if item.Contract.Interface == nil || item.Contract.Interface.Availability != "unavailable" || !strings.Contains(string(item.Contract.Result.DataSchema), "zeroMatchProven") {
			t.Fatal("unavailable Devdoc Shortcut does not publish the semantic zero-match boundary")
		}
	}
}
