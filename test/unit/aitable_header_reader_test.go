// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package unit

import (
	"os"
	"os/exec"
	"testing"
)

func TestCrossPlatformCoverageAITableHeaderReader(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	cmd := exec.Command(python, "test/scripts/aitable_header_reader_test.py")
	cmd.Dir = "../.."
	cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("AI table header reader tests failed: %v\n%s", err, output)
	}
}
