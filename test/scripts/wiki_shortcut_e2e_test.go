// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package scripts_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestWikiShortcutE2ERequiresInteractiveConfirmation(t *testing.T) {
	script := filepath.Join("..", "..", "scripts", "dev", "wiki-shortcut-e2e.py")
	source, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(source, []byte("--yes")) {
		t.Fatal("real-data Wiki E2E script must not embed confirmation bypass flags")
	}

	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is unavailable")
	}
	cmd := exec.Command(python, script)
	cmd.Stdin = strings.NewReader("")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatal("non-interactive real-data E2E unexpectedly succeeded")
	}
	if !strings.Contains(stderr.String(), "run in an interactive terminal") {
		t.Fatalf("non-interactive failure = %q, want explicit terminal requirement", stderr.String())
	}
	if strings.Contains(stdout.String(), "PASS ") {
		t.Fatalf("non-interactive E2E performed work before refusing: %q", stdout.String())
	}
}
