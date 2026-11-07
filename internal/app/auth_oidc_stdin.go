// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package app

import (
	"context"
	"io"
	"os"
)

// readOIDCCodeInput bounds both the input size and the time spent waiting for
// stdin. Native Unix stdin needs poll-based cancellation: it can be a blocking
// descriptor whose Read cannot be interrupted by os.File.Close.
//
// Custom blocking inputs supplied through SetIn must support concurrent Close
// to interrupt Read. Cancellation closes those inputs; successful reads leave
// them open. Readers without Close must be nonblocking or honor ctx themselves.
func readOIDCCodeInput(ctx context.Context, input io.Reader) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var cancelRead func(<-chan struct{})
	var cleanup func()
	if file, ok := input.(*os.File); ok {
		var err error
		input, cancelRead, cleanup, err = newOIDCFileInput(file)
		if err != nil {
			return nil, err
		}
	}
	if cancelRead == nil {
		if closer, ok := input.(io.Closer); ok {
			cancelRead = func(done <-chan struct{}) {
				if closer.Close() == nil {
					<-done
				}
			}
		}
	}
	read := func() ([]byte, error) {
		data, err := io.ReadAll(io.LimitReader(oidcCodeContextReader{ctx: ctx, input: input}, 8192+3))
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return data, err
	}
	if cancelRead == nil {
		return read()
	}
	type result struct {
		data []byte
		err  error
	}
	if cleanup == nil {
		cleanup = func() {}
	}
	done := make(chan result, 1)
	completed := make(chan struct{})
	released := make(chan struct{})
	defer close(released)
	go func() {
		defer func() {
			// Cancel and cleanup must not race when the read and context
			// finish together. The reader owns cleanup after both are done.
			<-released
			cleanup()
		}()
		data, err := read()
		close(completed)
		done <- result{data: data, err: err}
	}()
	select {
	case <-ctx.Done():
		cancelRead(completed)
		return nil, ctx.Err()
	case got := <-done:
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return got.data, got.err
	}
}

type oidcCodeContextReader struct {
	ctx   context.Context
	input io.Reader
}

func (r oidcCodeContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.input.Read(p)
}
