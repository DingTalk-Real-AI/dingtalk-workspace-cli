//go:build unix

// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package launcher

import (
	"io"
	"syscall"
)

func platformDelegate(path string, argv, environment []string, _ string, _ io.Reader, _, _ io.Writer) (int, error) {
	return 0, syscall.Exec(path, argv, environment)
}
