// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.

package helpers

import (
	"errors"
	"runtime"
	"testing"
	"time"
	"weak"

	"github.com/spf13/cobra"
)

func TestCrossPlatformCoverageContractValidationSurvivesGC(t *testing.T) {
	validationError := errors.New("local validation must run")
	cmd := &cobra.Command{Use: "validate", RunE: func(*cobra.Command, []string) error {
		t.Fatal("business execution ran after rejected validation")
		return nil
	}}
	rt := &contractRuntime{validate: func(*cobra.Command, []string) error { return validationError }}
	storeContractRuntime(cmd, rt)
	installContractRunEPipeline(cmd, rt)
	rt = nil // only the installed execution pipeline owns the runtime now
	for i := 0; i < 3; i++ {
		runtime.GC()
	}
	validate := ContractValidate(cmd)
	if validate == nil || validate(cmd, nil) != validationError {
		t.Fatal("outer confirmation guard lost its validation hook after GC")
	}
	if err := cmd.RunE(cmd, nil); err != validationError {
		t.Fatalf("direct RunE validation = %v", err)
	}
	runtime.KeepAlive(cmd)
}

func TestCrossPlatformCoverageContractValidationClosureDoesNotRetainTree(t *testing.T) {
	discarded := discardedContractRuntimeTree()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		runtime.GC()
		if discarded.Value() == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("validation closure retained its discarded command tree")
}

//go:noinline
func discardedContractRuntimeTree() weak.Pointer[cobra.Command] {
	root := &cobra.Command{Use: "root"}
	leaf := &cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(leaf)
	rt := &contractRuntime{validate: func(*cobra.Command, []string) error {
		// Deliberately capture a command, not just DTO data.
		root.SetArgs(nil)
		return nil
	}}
	storeContractRuntime(leaf, rt)
	installContractRunEPipeline(leaf, rt)
	return weak.Make(root)
}
