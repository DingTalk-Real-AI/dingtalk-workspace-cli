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

func runVerifierSmoke(t *testing.T, reportedVersion, expectedVersion string) (string, error) {
	t.Helper()
	script := readVerifier(t)
	start := strings.Index(script, "smoke() {")
	if start == -1 {
		t.Fatal("verifier does not define smoke()")
	}
	endMarker := "\n}\n\n# Latest stable"
	end := strings.Index(script[start:], endMarker)
	if end == -1 {
		t.Fatal("could not isolate smoke() from verifier")
	}
	smokeFunction := script[start : start+end+2]

	fakeBinary := filepath.Join(t.TempDir(), "dws")
	fakeScript := `#!/bin/sh
case "${1:-}" in
  version) printf '{"version":"v%s"}\n' "$FAKE_DWS_VERSION" ;;
  --help) exit 0 ;;
  *) exit 1 ;;
esac
`
	if err := os.WriteFile(fakeBinary, []byte(fakeScript), 0o755); err != nil {
		t.Fatalf("write fake dws: %v", err)
	}

	cmd := exec.Command("bash", "-c", smokeFunction+"\nsmoke \"$1\" \"$2\"", "smoke-test", fakeBinary, expectedVersion)
	cmd.Env = append(os.Environ(), "FAKE_DWS_VERSION="+reportedVersion)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// TestVerifyAssertsExpectedVersion guards the requirement that a wrong or stale
// version fails the channel instead of silently reporting PASS.
func TestVerifyAssertsExpectedVersion(t *testing.T) {
	tests := []struct {
		name            string
		reportedVersion string
		expectedVersion string
		wantErr         bool
		wantMessage     string
	}{
		{name: "stable exact match", reportedVersion: "1.0.51", expectedVersion: "1.0.51"},
		{name: "beta exact match", reportedVersion: "1.0.52-beta.4", expectedVersion: "v1.0.52-beta.4"},
		{
			name:            "stable rejects beta with shared prefix",
			reportedVersion: "1.0.51-beta.1",
			expectedVersion: "1.0.51",
			wantErr:         true,
			wantMessage:     "version mismatch",
		},
		{
			name:            "missing expected version fails closed",
			reportedVersion: "1.0.51",
			wantErr:         true,
			wantMessage:     "expected version is empty",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := runVerifierSmoke(t, tt.reportedVersion, tt.expectedVersion)
			if tt.wantErr && err == nil {
				t.Fatalf("smoke unexpectedly passed:\n%s", output)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("smoke failed: %v\n%s", err, output)
			}
			if tt.wantMessage != "" && !strings.Contains(output, tt.wantMessage) {
				t.Fatalf("smoke output missing %q:\n%s", tt.wantMessage, output)
			}
		})
	}

	script := readVerifier(t)
	// Homebrew and npm must derive the expected version from the package
	// manager rather than trusting the binary's self-report unconditionally.
	for _, source := range []string{"brew list --versions", "npm view"} {
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
