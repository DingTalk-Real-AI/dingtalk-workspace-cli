// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package builtin_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageHrmregisterSemanticCatalogExactlyCoversRegisteredSurface(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog_hrmregister.json")
	if err != nil {
		t.Fatal(err)
	}
	var source chatSemanticCatalogFixture
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	if source.Service != "hrmregister" || len(source.Shortcuts) != 33 {
		t.Fatalf("service/catalog = %q/%d, want hrmregister/33", source.Service, len(source.Shortcuts))
	}
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service == "hrmregister" {
			registered[item.Command] = item
		}
	}
	if len(registered) != 33 {
		t.Fatalf("registered = %d, want 33", len(registered))
	}
	for command, record := range source.Shortcuts {
		item, ok := registered[command]
		if !ok {
			t.Errorf("catalog contains stale command %s", command)
			continue
		}
		if !record.Public || !record.Reviewed || !item.SemanticReviewed || item.Hidden || !shortcut.InPublicCatalog("hrmregister", command) {
			t.Errorf("%s visibility/review drift", command)
		}
		if item.SemanticDelta != record.SemanticDelta || item.Risk != record.Risk {
			t.Errorf("%s semantic facts drifted", command)
		}
		if item.Contract.Empty() || item.Contract.Result == nil || item.OutputRollout != output.RolloutUnifiedActive || item.Safety.Effect == "" {
			t.Errorf("%s lacks Contract/Result/Safety/unified output", command)
		}
	}
}
