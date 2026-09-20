// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"errors"

	"golang.org/x/sys/windows"
)

func personalEventConnectionInterrupted(err error) bool {
	// Windows socket I/O returns Winsock codes, not the portable errno values.
	return errors.Is(err, windows.WSAECONNRESET) || errors.Is(err, windows.WSAECONNREFUSED) || errors.Is(err, windows.WSAECONNABORTED) || errors.Is(err, windows.ERROR_NETNAME_DELETED)
}
