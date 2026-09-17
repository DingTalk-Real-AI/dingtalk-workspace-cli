// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

// Package commandstore ties framework metadata to the lifetime of a live Cobra
// command without keeping discarded command trees alive. It owns no declaration
// semantics; the caller still owns normalization, cloning and authority.
package commandstore

import (
	"runtime"
	"sync"
	"weak"

	"github.com/spf13/cobra"
)

// Map is a concurrent command-keyed metadata store. The zero value is ready to
// use and must not be copied after first use. Values must never reference the
// command, its tree, or closures capturing either: that would retain the key
// through the value and defeat weak ownership. Framework DTOs satisfy this.
// The command tree's owner must keep it alive while its metadata is in use.
type Map struct {
	entries sync.Map // weak.Pointer[cobra.Command] -> metadata (no command pointers)
}

func (m *Map) Store(cmd *cobra.Command, value any) {
	if cmd == nil {
		return
	}
	key := weak.Make(cmd)
	_, loaded := m.entries.Swap(key, value)
	if !loaded {
		// The cleanup captures only the map and weak key. Cobra trees contain
		// parent/child cycles, so a finalizer would not reclaim them safely.
		runtime.AddCleanup(cmd, func(key weak.Pointer[cobra.Command]) {
			m.entries.Delete(key)
		}, key)
	}
	runtime.KeepAlive(cmd)
}

func (m *Map) Load(cmd *cobra.Command) (any, bool) {
	if cmd == nil {
		return nil, false
	}
	value, ok := m.entries.Load(weak.Make(cmd))
	runtime.KeepAlive(cmd)
	return value, ok
}

func (m *Map) Delete(cmd *cobra.Command) {
	if cmd != nil {
		m.entries.Delete(weak.Make(cmd))
		runtime.KeepAlive(cmd)
	}
}

// Range visits only live commands. Like sync.Map.Range, it is not a consistent
// snapshot in the presence of concurrent registrations. Expired keys are never
// returned, even if their asynchronous cleanup has not yet run.
func (m *Map) Range(visit func(key, value any) bool) {
	m.entries.Range(func(rawKey, value any) bool {
		key := rawKey.(weak.Pointer[cobra.Command])
		cmd := key.Value()
		if cmd == nil {
			m.entries.Delete(key)
			return true
		}
		more := visit(cmd, value)
		runtime.KeepAlive(cmd)
		return more
	})
}
