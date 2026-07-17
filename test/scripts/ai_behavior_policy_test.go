// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");

package scripts_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAIBehaviorPolicyProtectsEnforcementInputs(t *testing.T) {
	t.Parallel()

	path, err := filepath.Abs(filepath.Join("..", "..", ".github", "workflows", "ai-behavior-check.yml"))
	if err != nil {
		t.Fatalf("Abs(ai-behavior-check.yml) error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(%s) error = %v", path, err)
	}

	workflow := string(data)
	for _, want := range []string{
		"filename.startsWith('scripts/policy/')",
		"filename === 'test/fixtures/cli-interface-baseline.txt'",
		"previous_filename",
	} {
		if !strings.Contains(workflow, want) {
			t.Errorf("AI behavior policy does not protect %q", want)
		}
	}
}
