// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package builtin_test

import (
	"encoding/json"
	"os"
	"sort"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
)

func TestCrossPlatformCoverageDocSemanticCatalogExactlyCoversRegisteredSurface(t *testing.T) {
	raw, err := os.ReadFile("../semantic_catalog_doc.json")
	if err != nil {
		t.Fatal(err)
	}
	var source chatSemanticCatalogFixture
	if err := json.Unmarshal(raw, &source); err != nil {
		t.Fatal(err)
	}
	if source.Service != "doc" {
		t.Fatalf("semantic catalog service = %q", source.Service)
	}
	registered := map[string]shortcut.Shortcut{}
	for _, item := range shortcut.All() {
		if item.Service == "doc" {
			registered[item.Command] = item
		}
	}
	if len(registered) != 50 || len(source.Shortcuts) != 50 {
		t.Fatalf("registered/catalog = %d/%d, want 50/50", len(registered), len(source.Shortcuts))
	}
	var missing, stale []string
	public, hidden := 0, 0
	for command, item := range registered {
		record, ok := source.Shortcuts[command]
		if !ok {
			missing = append(missing, command)
			continue
		}
		if !record.Reviewed || !item.SemanticReviewed || record.SemanticDelta != item.SemanticDelta || record.Disposition != item.Disposition {
			t.Errorf("%s: reviewed semantic delivery mismatch", command)
		}
		if got := shortcut.InPublicCatalog("doc", command); got != record.Public || item.Hidden == record.Public {
			t.Errorf("%s: public/hidden mismatch: catalog=%v runtimeHidden=%v", command, record.Public, item.Hidden)
		}
		if record.Public {
			public++
			if item.Contract.Empty() {
				t.Errorf("%s: public Doc shortcut has empty Contract", command)
			}
		} else {
			hidden++
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
	if public != 45 || hidden != 5 {
		t.Fatalf("public/hidden = %d/%d, want 45/5", public, hidden)
	}

	wantPrimaries := map[string]string{
		"+find-doc": "+search", "+doc-append": "+update", "+history-save": "+version-save",
		"+history-list": "+version-list", "+history-revert": "+version-revert", "+share-doc": "+share",
	}
	for command, primary := range wantPrimaries {
		item := registered[command]
		if item.Disposition != shortcut.DispositionAliasInternal || item.PrimaryCommand != primary {
			t.Errorf("%s compatibility routing = %s/%s, want alias_internal/%s", command, item.Disposition, item.PrimaryCommand, primary)
		}
	}
	for _, command := range []string{"+cover-set", "+cover-download", "+cover-clear"} {
		if _, ok := registered[command]; !ok {
			t.Errorf("missing explicit cover shortcut %s", command)
		}
	}
	for _, legacy := range []string{"+resource-update", "+resource-download", "+resource-delete"} {
		if item, ok := registered[legacy]; !ok || !item.Hidden || item.Disposition != shortcut.DispositionAliasInternal {
			t.Errorf("legacy cover name %s is not a hidden compatibility command: %#v", legacy, item)
		}
	}
}
