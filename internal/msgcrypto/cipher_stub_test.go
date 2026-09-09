// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build !safechat || !cgo

package msgcrypto

import (
	"context"
	"errors"
	"testing"
)

func TestCrossPlatformCoverageStubBackendFailsClosed(t *testing.T) {
	cipher, err := newBackend(context.Background(), Config{})
	if cipher != nil || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("newBackend() = %#v, %v; want nil, ErrUnavailable", cipher, err)
	}
}
