// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

//go:build !windows

package app

import (
	"errors"
	"io"
	"os"
	"testing"

	"github.com/muesli/cancelreader"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
)

func TestCrossPlatformCoverageOIDCStdinCancelReaderFailure(t *testing.T) {
	input, closer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer closer.Close()
	defer input.Close()
	testseam.Swap(t, &oidcInputCancelReader, func(io.Reader) (cancelreader.CancelReader, error) {
		return nil, errors.New("cancelreader unavailable")
	})
	_, cancelRead, cleanup, err := newOIDCFileInput(input)
	if err == nil || cancelRead != nil || cleanup != nil {
		t.Fatalf("cancelreader failure was not surfaced: reader set=%v cleanup set=%v err=%v", cancelRead != nil, cleanup != nil, err)
	}
}
