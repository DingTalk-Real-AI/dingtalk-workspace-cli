package app

import (
	"path/filepath"
	"testing"
)

func TestDefaultConfigDirUsesHomeDirectoryInOSSMode(t *testing.T) {
	homeDir := filepath.Join(t.TempDir(), "home")
	originalUserHomeDir := userHomeDir
	userHomeDir = func() (string, error) { return homeDir, nil }
	t.Cleanup(func() { userHomeDir = originalUserHomeDir })
	t.Setenv("DWS_CONFIG_DIR", "")

	got := defaultConfigDir()
	want := filepath.Join(homeDir, ".dws")
	if got != want {
		t.Fatalf("defaultConfigDir() = %q, want %q", got, want)
	}
}
