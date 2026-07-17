package app

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/keychain"
	"github.com/spf13/cobra"
)

func TestAuthMigrateKeychainRemainingBranches(t *testing.T) {
	originalMigrate, originalTarget := migrateKeychainToFileDEK, authMigrateTarget
	t.Cleanup(func() {
		migrateKeychainToFileDEK, authMigrateTarget = originalMigrate, originalTarget
	})
	newRoot := func(format string) (*cobra.Command, *bytes.Buffer) {
		root := &cobra.Command{Use: "dws"}
		root.PersistentFlags().Bool("dry-run", false, "")
		root.PersistentFlags().Bool("yes", false, "")
		root.PersistentFlags().String("format", format, "")
		root.AddCommand(newAuthMigrateKeychainCommand())
		var output bytes.Buffer
		root.SetOut(&output)
		root.SetErr(&output)
		return root, &output
	}
	t.Setenv(keychain.DisableKeychainEnv, "")

	authMigrateTarget = func(*cobra.Command) (string, error) { return "", errors.New("flag") }
	root, _ := newRoot("text")
	root.SetArgs([]string{"migrate-keychain", "--dry-run"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "--to") {
		t.Fatalf("target flag error = %v", err)
	}
	authMigrateTarget = originalTarget
	root, _ = newRoot("text")
	root.SetArgs([]string{"migrate-keychain", "--to", "other", "--dry-run"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "file-dek") {
		t.Fatalf("unsupported target error = %v", err)
	}

	migrateKeychainToFileDEK = func(string, bool) (int, error) { return 0, errors.New("backend") }
	root, _ = newRoot("text")
	root.SetArgs([]string{"migrate-keychain", "--dry-run"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "backend") {
		t.Fatalf("migration backend error = %v", err)
	}
	migrateKeychainToFileDEK = func(string, bool) (int, error) { return 3, nil }
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"migrate-keychain", "--dry-run"}, "预检通过"},
		{[]string{"migrate-keychain", "--yes"}, "迁移完成"},
	} {
		root, output := newRoot("text")
		root.SetArgs(test.args)
		if err := root.Execute(); err != nil || !strings.Contains(output.String(), test.want) {
			t.Fatalf("migrate %v = %v, %q", test.args, err, output.String())
		}
	}
}

// Keep the original test name for the focused macOS auth workflow while also
// opting the coverage fixture into the native platform coverage gate.
func TestCrossPlatformCoverageAuthMigrateKeychainRemainingBranches(t *testing.T) {
	TestAuthMigrateKeychainRemainingBranches(t)
}
