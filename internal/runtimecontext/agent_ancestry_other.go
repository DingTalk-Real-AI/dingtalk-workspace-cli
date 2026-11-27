// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build !darwin && !linux && !windows

package runtimecontext

func collectProcessAncestry(int) ([]ancestorProcess, bool) {
	return nil, false
}
