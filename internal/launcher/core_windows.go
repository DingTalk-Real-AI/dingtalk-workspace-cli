//go:build windows

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package launcher

import "os"

func coreExecutableName() string               { return "dws-core.exe" }
func environmentKeysCaseInsensitive() bool     { return true }
func validateCoreExecutable(os.FileInfo) error { return nil }
