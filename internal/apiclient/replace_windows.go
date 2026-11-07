// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build windows

package apiclient

import "golang.org/x/sys/windows"

func atomicReplace(source, target string) error {
	return windows.Rename(source, target)
}
