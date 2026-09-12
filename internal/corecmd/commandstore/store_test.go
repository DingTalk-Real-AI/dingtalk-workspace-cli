// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package commandstore

import (
	"runtime"
	"sync"
	"testing"
	"time"
	"weak"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageCommandMetadataLivesWithCommand(t *testing.T) {
	var store Map
	cmd := &cobra.Command{Use: "live"}
	store.Store(cmd, "first")
	store.Store(cmd, "replacement")
	runtime.GC()
	if got, ok := store.Load(cmd); !ok || got != "replacement" {
		t.Fatalf("live metadata after GC = %v, %v", got, ok)
	}
	count := 0
	store.Range(func(key, value any) bool {
		count++
		if key != cmd || value != "replacement" {
			t.Fatalf("range = %v, %v", key, value)
		}
		return false
	})
	if count != 1 {
		t.Fatalf("live command count = %d", count)
	}
	store.Delete(cmd)
	if _, ok := store.Load(cmd); ok {
		t.Fatal("deleted command is still registered")
	}
	store.Store(nil, "ignored")
	store.Delete(nil)
	if _, ok := store.Load(nil); ok {
		t.Fatal("nil command was registered")
	}
	runtime.KeepAlive(cmd)
}

func TestCrossPlatformCoverageCommandMetadataReclaimsCyclesAndEntries(t *testing.T) {
	var store Map
	discarded := storeDiscardedTree(&store)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		count := 0
		// Inspect the backing map without Range's opportunistic pruning to
		// prove that asynchronous cleanup also releases the metadata itself.
		store.entries.Range(func(_, _ any) bool { count++; return true })
		if discarded.Value() == nil && count == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("discarded command cycle or its metadata was retained")
}

//go:noinline
func storeDiscardedTree(store *Map) weak.Pointer[cobra.Command] {
	root, leaf := &cobra.Command{Use: "root"}, &cobra.Command{Use: "leaf"}
	root.AddCommand(leaf)
	store.Store(leaf, make([]byte, 1024*1024))
	return weak.Make(root)
}

func TestCrossPlatformCoverageCommandMetadataConcurrentGC(t *testing.T) {
	var store Map
	var wg sync.WaitGroup
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 50; i++ {
				cmd := &cobra.Command{Use: "temporary"}
				store.Store(cmd, i)
				if got, ok := store.Load(cmd); !ok || got != i {
					t.Errorf("live concurrent lookup = %v, %v, want %d", got, ok, i)
				}
				store.Range(func(_, _ any) bool { return true })
				store.Delete(cmd)
			}
		}()
	}
	for i := 0; i < 3; i++ {
		runtime.GC()
	}
	wg.Wait()
}

func TestCrossPlatformCoverageCommandMetadataRangePrunesExpired(t *testing.T) {
	var store Map
	discarded := storeDiscardedTree(&store)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		store.Range(func(_, _ any) bool { return true })
		if discarded.Value() == nil {
			count := 0
			store.entries.Range(func(_, _ any) bool { count++; return true })
			if count == 0 {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("Range did not prune expired command metadata")
}

func TestCrossPlatformCoverageCommandMetadataRangePrunesDanglingWeakKeys(t *testing.T) {
	var store Map
	cmd := &cobra.Command{Use: "gone"}
	key := weak.Make(cmd)
	store.entries.Store(key, "stale")
	cmd = nil
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		if key.Value() == nil {
			visited := 0
			store.Range(func(_, _ any) bool {
				visited++
				return true
			})
			if visited != 0 {
				t.Fatalf("pruned range visited %d live commands", visited)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("weak command key never expired")
}
