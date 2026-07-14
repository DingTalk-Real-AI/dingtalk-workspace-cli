// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"strings"
	"testing"
)

func TestEmbeddedCatalogKeepsSpecializedAitableViewTextCommandSpecific(t *testing.T) {
	loaded := embeddedSchemaCatalog()
	checked := 0
	for canonical, tool := range loaded.Snapshot.Tools {
		if !strings.HasPrefix(canonical, "aitable.view_get_") &&
			!strings.HasPrefix(canonical, "aitable.view_update_") {
			continue
		}
		checked++
		if title := schemaString(tool["title"]); title == "get_views" || title == "update_view" {
			t.Errorf("%s title leaked generic RPC metadata: %q", canonical, title)
		}
		provenance := schemaMap(tool["field_provenance"])
		for _, field := range []string{"title", "description"} {
			source := schemaString(provenance[field]["source"])
			if source == "mcp_metadata" || source == "pinned_mcp_metadata" {
				t.Errorf("%s %s still comes from generic RPC metadata: %#v", canonical, field, provenance[field])
			}
		}
	}
	if checked < 20 {
		t.Fatalf("checked %d specialized AITable view tools, want the complete reviewed family", checked)
	}
}
