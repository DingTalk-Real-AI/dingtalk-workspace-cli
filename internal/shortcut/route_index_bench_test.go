// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package shortcut_test

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut"
	_ "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/builtin"
)

func BenchmarkHasServiceIndex(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !shortcut.HasService("calendar") {
			b.Fatal("shortcut service index lost calendar")
		}
	}
}

func BenchmarkHasServiceLinear(b *testing.B) {
	registered := shortcut.All()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		found := false
		for _, item := range registered {
			if item.Service == "calendar" {
				found = true
				break
			}
		}
		if !found {
			b.Fatal("shortcut registry lost calendar")
		}
	}
}
