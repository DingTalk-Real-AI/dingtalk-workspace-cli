// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"errors"
	"io"
	"os"
	"time"

	"golang.org/x/sys/windows"
)

func newOIDCFileInput(file *os.File) (io.Reader, func(<-chan struct{}), func(), error) {
	handle := windows.Handle(file.Fd())
	var mode uint32
	if windows.GetConsoleMode(handle, &mode) != nil {
		// Redirected pipes, regular files and NUL keep the os.File path.
		return file, nil, nil, nil
	}
	// Keep os.File.Read's Unicode and Ctrl-Z/EOF handling, as well as the
	// console's Ctrl-C mode. cancelreader's Windows backend changes both.
	return file, func(done <-chan struct{}) {
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			default:
			}
			// Cancellation can arrive between checking ctx and entering
			// ReadConsole. Retry ERROR_NOT_FOUND until the read has finished
			// or the pending I/O has actually been interrupted.
			err := windows.CancelIoEx(handle, nil)
			if err != nil && !errors.Is(err, windows.ERROR_NOT_FOUND) {
				return
			}
			select {
			case <-done:
				return
			case <-ticker.C:
			}
		}
	}, nil, nil
}
