// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package builtin_test

import (
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/shortcut/builtin"
)

// BenchmarkShortcutCommands measures the Go-literal storage model: the 376
// shortcut definitions are compiled-in struct literals registered from init(),
// so there is no parse step at all — this times only turning them into the Cobra
// tree. ResolveMeta no longer pays full Catalog decode (~294–360ms / ~175MB);
// it reads assembled SchemaRegistry (BenchmarkResolveMetaFirstHit). Full Catalog
// assemble remains the dws schema / --all path.
func BenchmarkShortcutCommands(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if len(builtin.Commands()) == 0 {
			b.Fatal("no shortcut commands")
		}
	}
}
