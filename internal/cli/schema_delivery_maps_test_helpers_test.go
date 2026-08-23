// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import "testing"

func mustDeliverySchemaCatalogMaps(t *testing.T) loadedSchemaCatalog {
	t.Helper()
	if !SchemaSourceRootRegistered() {
		t.Fatal("schema source root factory is not registered; package-cli TestMain should install assembly delivery")
	}
	if err := deliverySchemaCatalogError(); err != nil {
		t.Fatalf("load delivery Schema Catalog: %v", err)
	}
	loaded := deliverySchemaCatalog()
	if len(loaded.Snapshot.Tools) == 0 {
		t.Fatal("delivery Schema Catalog tools maps are empty")
	}
	return loaded
}
