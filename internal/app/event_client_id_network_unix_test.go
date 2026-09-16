// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build !windows

package app

import (
	"syscall"
	"testing"
)

func TestCrossPlatformCoveragePersonalEventClientIDSocketErrors(t *testing.T) {
	for _, err := range []error{syscall.ECONNRESET, syscall.ECONNREFUSED, syscall.ECONNABORTED} {
		t.Run(err.Error(), func(t *testing.T) { checkPersonalClientIDSocketError(t, err) })
	}
}
