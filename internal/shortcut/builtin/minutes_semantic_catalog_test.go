// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package builtin_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageMinutesSemanticCatalogMatchesRegistryE2E(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog_minutes.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Shortcuts map[string]struct {
			Public   bool   `json:"public"`
			Reviewed bool   `json:"reviewed"`
			Delta    string `json:"semantic_delta"`
		} `json:"shortcuts"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatal(err)
	}
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service == "minutes" {
			registered[item.Command] = item
		}
	}
	if len(registered) != len(catalog.Shortcuts) {
		t.Fatalf("minutes registry/catalog count = %d/%d", len(registered), len(catalog.Shortcuts))
	}
	for command, record := range catalog.Shortcuts {
		item, ok := registered[command]
		if !ok {
			t.Errorf("catalog command %s is not registered", command)
			continue
		}
		if !record.Reviewed || record.Delta == "" || item.Hidden == record.Public || !item.SemanticReviewed || item.Contract.Empty() {
			t.Errorf("minutes %s review/public/contract mismatch: record=%#v item=%#v", command, record, item)
		}
	}
}
