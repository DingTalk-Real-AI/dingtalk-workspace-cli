// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package runtimecontext

import "sync"

// Keep loaded libraries alive for both APIs and any late UMID callbacks.
var nativeLibraries = struct {
	sync.Mutex
	handles map[string]uintptr
}{handles: make(map[string]uintptr)}

func sharedNativeLibrary(path string, open func(string) (uintptr, error)) (uintptr, error) {
	nativeLibraries.Lock()
	defer nativeLibraries.Unlock()
	if handle := nativeLibraries.handles[path]; handle != 0 {
		return handle, nil
	}
	handle, err := open(path)
	if err == nil {
		nativeLibraries.handles[path] = handle
	}
	return handle, err
}
