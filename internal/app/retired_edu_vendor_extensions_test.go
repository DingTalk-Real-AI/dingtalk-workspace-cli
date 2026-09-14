// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"strings"
	"testing"
)

func TestRetiredEduVendorExtensionsAreAbsentFromRuntimeAndSchema(t *testing.T) {
	products := []string{
		"college-contact",
		"edu-app",
		"edu-contact",
		"edu-familygroup",
		"edu-group",
	}

	root := NewRootCommand()
	for _, product := range products {
		for _, command := range root.Commands() {
			if command.Name() == product {
				t.Fatalf("retired product command %q remains mounted", product)
			}
		}
	}

	retiredProducts := make(map[string]bool, len(products))
	for _, product := range products {
		retiredProducts[product] = true
	}

	snapshot := fullSchemaSnapshotForTest(t)
	for _, product := range snapshot.Catalog["products"].([]map[string]any) {
		productID, _ := product["id"].(string)
		if retiredProducts[productID] {
			t.Errorf("retired product %q remains in the Schema catalog", productID)
		}
	}
	for canonicalPath := range snapshot.Tools {
		for product := range retiredProducts {
			if canonicalPath == product || strings.HasPrefix(canonicalPath, product+".") {
				t.Errorf("retired Schema tool %q remains under product %q", canonicalPath, product)
			}
		}
	}
}
