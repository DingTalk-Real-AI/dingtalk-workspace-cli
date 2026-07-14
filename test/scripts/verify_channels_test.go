// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0

package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestVerifyAllChannelsScriptContract(t *testing.T) {
	path := filepath.Join("..", "..", "verify", "verify-all-channels.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read verifier: %v", err)
	}

	script := string(data)
	for _, channel := range []string{
		"curl", "powershell", "npm-stable", "npm-beta", "homebrew", "dws-upgrade",
	} {
		if !strings.Contains(script, channel) {
			t.Errorf("verifier does not include channel %q", channel)
		}
	}
	for _, check := range []string{" version", " --help", "npm uninstall", "brew uninstall"} {
		if !strings.Contains(script, check) {
			t.Errorf("verifier does not include lifecycle check %q", check)
		}
	}

	cmd := exec.Command("bash", "-n", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash -n failed: %v\n%s", err, output)
	}
}
