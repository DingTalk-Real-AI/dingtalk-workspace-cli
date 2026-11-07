// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import (
	"errors"
	"testing"
)

func TestCrossPlatformCoverageSchemaAssemblyAuditFailureIsolation(t *testing.T) {
	if err := AuditSchemaAssembly(nil); err == nil {
		t.Fatal("nil callback accepted")
	}
	want := errors.New("assembly failed")
	if err := AuditSchemaAssembly(func() error { return want }); !errors.Is(err, want) {
		t.Fatalf("assembly error lost: %v", err)
	}
	if err := AuditSchemaAssembly(func() error {
		if err := AuditSchemaAssembly(func() error { t.Fatal("nested audit ran"); return nil }); err == nil {
			t.Fatal("nested audit accepted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	func() {
		defer func() {
			if recovered := recover(); recovered != want {
				t.Fatalf("unrelated panic changed: %v", recovered)
			}
		}()
		_ = AuditSchemaAssembly(func() error { panic(want) })
	}()
	if activeSchemaAssemblyAudit.Load() != nil {
		t.Fatal("unrelated panic left the audit active")
	}
}
