// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0.
package helpers

import (
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/card/chartgen"
)

func TestCrossPlatformCoverageAicardEveryPresetHasDiscoveryPurpose(t *testing.T) {
	presets, err := chartgen.List()
	if err != nil || len(presets) != 43 {
		t.Fatalf("presets: %d %v", len(presets), err)
	}
	for _, preset := range presets {
		record := chartRecord(preset)
		if purpose, ok := record["useWhen"].(string); !ok || strings.TrimSpace(purpose) == "" {
			t.Errorf("preset %s family %s lacks discovery guidance", preset.ID, preset.Family)
		}
		if record["id"] != preset.ID || record["sourceSha256"] != preset.SourceSHA256 {
			t.Errorf("discovery changed preset provenance: %#v", record)
		}
	}
}
