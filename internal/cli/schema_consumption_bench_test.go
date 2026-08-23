// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package cli

import "testing"

func BenchmarkDeliverySchemaCatalog(b *testing.B) {
	if err := deliverySchemaCatalogError(); err != nil {
		b.Fatalf("delivery catalog unavailable: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		resetDeliverySchemaCatalogStateForTest()
		_ = deliverySchemaCatalog()
	}
}

func BenchmarkResolveMetaSteadyState(b *testing.B) {
	if _, ok := ResolveMeta("dev app delete"); !ok {
		b.Fatal("ResolveMeta(dev app delete) missing")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ResolveMeta("dev app delete")
	}
}

func BenchmarkBuildMetaByCLIPath(b *testing.B) {
	loaded := deliverySchemaCatalog()
	if err := deliverySchemaCatalogError(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = buildMetaByCLIPath(loaded)
	}
}
