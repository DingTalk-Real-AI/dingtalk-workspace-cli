// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"testing"
)

func TestSchemaVisibilityRemainsRegistryNavigationClass(t *testing.T) {
	if SchemaVisibilityPublic != "public" || SchemaVisibilityCompat != "compat" || SchemaVisibilityInternal != "internal" {
		t.Fatalf("SchemaVisibility values changed: %q, %q, %q", SchemaVisibilityPublic, SchemaVisibilityCompat, SchemaVisibilityInternal)
	}
}
