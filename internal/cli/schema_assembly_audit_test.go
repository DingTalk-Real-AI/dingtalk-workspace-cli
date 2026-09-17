// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"errors"
	"strings"
	"testing"
)

func TestCrossPlatformCoverageSchemaAssemblyAudit(t *testing.T) {
	for _, test := range []struct {
		name string
		read func()
	}{
		{"ResolveMeta", func() { _, _ = ResolveMeta("calendar event create") }},
		{"fast path identity", func() { _, _ = SchemaCacheFastPathIdentity() }},
		{"Catalog loader", func() { _ = deliverySchemaCatalog() }},
		{"complete Registry loader", func() { _, _ = deliverySchemaAllPayload() }},
		{"overview loader", func() { _, _ = deliverySchemaOverviewPayload() }},
		{"query loader", func() { _, _ = queryDeliverySchemaPayload([]string{"calendar"}) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			continued := false
			err := AuditSchemaAssembly(func() error {
				test.read()
				continued = true
				return nil
			})
			if continued || !errors.Is(err, ErrSchemaAssemblyConsumedDelivery) || !strings.Contains(err.Error(), test.name) {
				t.Fatalf("delivery access did not abort the build: continued=%v err=%v", continued, err)
			}
			if activeSchemaAssemblyAudit.Load() != nil {
				t.Fatal("failed build left a global audit active")
			}
		})
	}
	if err := AuditSchemaAssembly(func() error { return nil }); err != nil {
		t.Fatalf("clean assembly rejected: %v", err)
	}
	err := AuditSchemaAssembly(func() error {
		func() {
			defer func() { _ = recover() }()
			_, _ = ResolveMeta("calendar event create")
		}()
		return nil
	})
	if !errors.Is(err, ErrSchemaAssemblyConsumedDelivery) {
		t.Fatalf("callback swallowed a forbidden delivery access: %v", err)
	}
	if _, found := ResolveMeta("calendar event create"); !found {
		t.Fatal("normal delivery was not restored after audit failure")
	}
}
