// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestCrossPlatformCoveragePersonalEventClientIDSocketErrors(t *testing.T) {
	for _, err := range []error{windows.WSAECONNRESET, windows.WSAECONNREFUSED, windows.WSAECONNABORTED, windows.ERROR_NETNAME_DELETED} {
		t.Run(err.Error(), func(t *testing.T) { checkPersonalClientIDSocketError(t, err) })
	}
}
