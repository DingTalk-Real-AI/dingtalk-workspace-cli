// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build darwin

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-isatty"
	"golang.org/x/sys/unix"
)

var (
	oidcInputDuplicateFD = unix.FcntlInt
	oidcInputSelect      = unix.Select
)

// newOIDCFileInput builds a cancelable reader for native Unix stdin. Regular
// files need no cancellation machinery. A terminal input is monitored with
// select (cancelreader chooses its BSD terminal workaround by file.Name(),
// which misses /dev/tty inherited as /dev/stdin); every other non-regular
// input falls through to the cancelreader-based poller.
func newOIDCFileInput(file *os.File) (io.Reader, func(<-chan struct{}), func(), error) {
	info, err := file.Stat()
	if err != nil {
		return nil, nil, nil, err
	}
	if info.Mode().IsRegular() {
		return file, nil, nil, nil
	}
	if reader, cancelRead, cleanup, err, ok := oidcTerminalFileInput(file); ok {
		return reader, cancelRead, cleanup, err
	}
	return oidcCancelableInput(file)
}

// oidcTerminalFileInput builds the macOS terminal-monitored input.
func oidcTerminalFileInput(file *os.File) (io.Reader, func(<-chan struct{}), func(), error, bool) {
	fd := file.Fd()
	if !isatty.IsTerminal(fd) {
		return nil, nil, nil, nil, false
	}
	monitorFD := int(fd)
	var cleanup func()
	if monitorFD >= unix.FD_SETSIZE {
		// select is bounded by FD_SETSIZE on macOS. A duplicate uses the
		// lowest available descriptor without taking ownership of input.
		var err error
		monitorFD, err = oidcInputDuplicateFD(fd, unix.F_DUPFD_CLOEXEC, 0)
		if err != nil {
			return nil, nil, nil, err, true
		}
		if monitorFD >= unix.FD_SETSIZE {
			_ = unix.Close(monitorFD)
			return nil, nil, nil, fmt.Errorf("cannot wait for terminal input: no descriptor below %d available", unix.FD_SETSIZE), true
		}
		cleanup = func() { _ = unix.Close(monitorFD) }
	}
	canceled := make(chan struct{})
	return oidcTerminalInput{file: file, fd: monitorFD, canceled: canceled}, func(done <-chan struct{}) {
		close(canceled)
		<-done
	}, cleanup, nil, true
}

// oidcTerminalInput borrows an exclusively read terminal without changing its
// mode or closing it. Short select waits bound cancellation latency to 50ms while
// avoiding a goroutine per Read. High-numbered inputs borrow a low-numbered
// duplicate only for select; reads still use the original file.
type oidcTerminalInput struct {
	file     *os.File
	fd       int
	canceled <-chan struct{}
}

func (r oidcTerminalInput) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	var set unix.FdSet
	for {
		select {
		case <-r.canceled:
			return 0, context.Canceled
		default:
		}
		set.Set(r.fd)
		timeout := unix.Timeval{Usec: 50000}
		n, err := oidcInputSelect(r.fd+1, &set, nil, nil, &timeout)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if err != nil {
			return 0, err
		}
		select {
		case <-r.canceled:
			return 0, context.Canceled
		default:
		}
		if n > 0 && set.IsSet(r.fd) {
			// Let the original file preserve terminal bytes and EOF/error
			// semantics, including Ctrl-D in canonical mode.
			return r.file.Read(p)
		}
	}
}
