// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package app

import (
	"runtime"
	"testing"
	"time"
	"weak"

	"github.com/spf13/cobra"
)

// A leaf points back to its parents. A process-global strong registration of
// even one leaf retains the entire discarded tree, including output buffers.
func TestCrossPlatformCoverageSchemaSourceRootsAreCollectible(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	for _, factory := range []struct {
		name  string
		build func() *cobra.Command
	}{
		{"schema-source", func() *cobra.Command { return NewSchemaSourceRootCommand() }},
		{"public-runtime", func() *cobra.Command { return NewRootCommand() }},
	} {
		t.Run(factory.name, func(t *testing.T) {
			discarded := discardedSchemaSourceRoot(factory.build)
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				runtime.GC()
				if discarded.Value() == nil {
					return
				}
				time.Sleep(10 * time.Millisecond)
			}
			t.Fatal("discarded root remains reachable through framework metadata")
		})
	}
}

// Keep the root's last strong use in a separate stack frame, so this test does
// not accidentally retain it while asking the collector to reclaim the tree.
//
//go:noinline
func discardedSchemaSourceRoot(build func() *cobra.Command) weak.Pointer[cobra.Command] {
	return weak.Make(build())
}
