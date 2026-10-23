// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build !windows && !darwin

package app

import (
	"io"
	"os"
)

// newOIDCFileInput builds a cancelable reader for native Unix stdin. Regular
// files need no cancellation machinery; every other input goes through the
// cancelreader-based poller.
func newOIDCFileInput(file *os.File) (io.Reader, func(<-chan struct{}), func(), error) {
	info, err := file.Stat()
	if err != nil {
		return nil, nil, nil, err
	}
	if info.Mode().IsRegular() {
		return file, nil, nil, nil
	}
	return oidcCancelableInput(file)
}
