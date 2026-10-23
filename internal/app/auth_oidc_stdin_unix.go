// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build !windows

package app

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/mattn/go-isatty"
	"github.com/muesli/cancelreader"
	"golang.org/x/sys/unix"
)

var (
	oidcInputDuplicateFD  = unix.FcntlInt
	oidcInputSelect       = unix.Select
	oidcInputCancelReader = cancelreader.NewReader
)

func newOIDCFileInput(file *os.File) (io.Reader, func(<-chan struct{}), func(), error) {
	info, err := file.Stat()
	if err != nil {
		return nil, nil, nil, err
	}
	if info.Mode().IsRegular() {
		return file, nil, nil, nil
	}
	fd := file.Fd()
	if runtime.GOOS == "darwin" && isatty.IsTerminal(fd) {
		// cancelreader chooses its BSD terminal workaround by file.Name(),
		// which misses /dev/tty inherited as /dev/stdin. Detect the actual
		// terminal and use select without relying on the file name.
		monitorFD := int(fd)
		var cleanup func()
		if monitorFD >= unix.FD_SETSIZE {
			// select is bounded by FD_SETSIZE on macOS. A duplicate uses the
			// lowest available descriptor without taking ownership of input.
			monitorFD, err = oidcInputDuplicateFD(fd, unix.F_DUPFD_CLOEXEC, 0)
			if err != nil {
				return nil, nil, nil, err
			}
			if monitorFD >= unix.FD_SETSIZE {
				_ = unix.Close(monitorFD)
				return nil, nil, nil, fmt.Errorf("cannot wait for terminal input: no descriptor below %d available", unix.FD_SETSIZE)
			}
			cleanup = func() { _ = unix.Close(monitorFD) }
		}
		canceled := make(chan struct{})
		return oidcTerminalInput{file: file, fd: monitorFD, canceled: canceled}, func(done <-chan struct{}) {
			close(canceled)
			<-done
		}, cleanup, nil
	}
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
