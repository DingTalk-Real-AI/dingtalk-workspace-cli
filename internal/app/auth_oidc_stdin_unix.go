// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build !windows

package app

import (
	"io"
	"os"

	"github.com/muesli/cancelreader"
)

// oidcInputCancelReader wraps cancelreader.NewReader so tests can inject
// cancelreader failures without reaching into the dependency.
var oidcInputCancelReader = cancelreader.NewReader

// oidcCancelableInput wraps a non-regular native file in the cancelreader
// poller. It is shared by every non-Windows newOIDCFileInput implementation.
func oidcCancelableInput(file *os.File) (io.Reader, func(<-chan struct{}), func(), error) {
	reader, err := oidcInputCancelReader(file)
	if err != nil {
		return nil, nil, nil, err
	}
	return reader, func(done <-chan struct{}) {
		if reader.Cancel() {
			<-done
		}
	}, func() { _ = reader.Close() }, nil
}
