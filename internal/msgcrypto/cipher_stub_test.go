// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build !(cgo && (darwin || linux || windows) && (amd64 || arm64))

package msgcrypto

import (
	"context"
	"errors"
	"testing"
)

func TestCrossPlatformCoverageStubBackendFailsClosed(t *testing.T) {
	if Available() {
		t.Fatal("Available() = true; want false for unsupported or CGO-disabled builds")
	}
	if BackendVersion != "" {
		t.Fatalf("BackendVersion = %q; want empty stub version", BackendVersion)
	}

	cipher, err := newBackend(context.Background(), Config{})
	if cipher != nil || !errors.Is(err, ErrUnavailable) {
		t.Fatalf("newBackend() = %#v, %v; want nil, ErrUnavailable", cipher, err)
	}
}
