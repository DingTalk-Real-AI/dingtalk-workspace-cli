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

func readVerifier(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", "verify", "verify-all-channels.sh")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read verifier: %v", err)
	}
	return string(data)
}

func TestVerifyAllChannelsScriptContract(t *testing.T) {
	script := readVerifier(t)
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

	path := filepath.Join("..", "..", "verify", "verify-all-channels.sh")
	cmd := exec.Command("bash", "-n", path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("bash -n failed: %v\n%s", err, output)
	}
}

// TestVerifyAssertsExpectedVersion guards the requirement that a wrong or stale
// version fails the channel instead of silently reporting PASS.
func TestVerifyAssertsExpectedVersion(t *testing.T) {
	script := readVerifier(t)
	for _, marker := range []string{
		"expected=",        // smoke takes an expected version argument
		"version mismatch", // and fails loudly on a mismatch
		"grep -Fq",         // by matching the reported version against it
		"return 1",         // turning a mismatch into a channel failure
	} {
		if !strings.Contains(script, marker) {
			t.Errorf("verifier missing version-assertion marker %q", marker)
		}
	}
	// Homebrew and npm must derive the expected version from the package
	// manager rather than trusting the binary's self-report unconditionally.
	for _, source := range []string{
		"brew list --versions",
		"npm view",
	} {
		if !strings.Contains(script, source) {
			t.Errorf("verifier missing authoritative version source %q", source)
		}
	}
}

// TestVerifyHomebrewCoexistence guards the requirement that beta installs
// alongside stable (keg-only) without disturbing the stable channel.
func TestVerifyHomebrewCoexistence(t *testing.T) {
	script := readVerifier(t)

	stableInstall := `brew install "$TAP/$PACKAGE"`
	betaInstall := `brew install "$TAP/$PACKAGE-beta"`
	stableIdx := strings.Index(script, stableInstall)
	betaIdx := strings.Index(script, betaInstall)
	if stableIdx < 0 || betaIdx < 0 {
		t.Fatalf("expected both stable and beta installs; stableIdx=%d betaIdx=%d", stableIdx, betaIdx)
	}
	if betaIdx <= stableIdx {
		t.Errorf("beta must be installed after stable to prove coexistence")
	}
	// Stable must remain installed when beta lands: no uninstall between them.
	between := script[stableIdx+len(stableInstall) : betaIdx]
	if strings.Contains(between, "brew uninstall") {
		t.Errorf("stable is uninstalled before beta install; cannot prove coexistence")
	}

	for _, marker := range []string{
		"stable version changed after beta install",
		"stable binary changed after beta install",
		"stable link changed after beta install",
	} {
		if !strings.Contains(script, marker) {
			t.Errorf("verifier missing coexistence assertion %q", marker)
		}
	}
}
