package app

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	apperrors "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/errors"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/output"
	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/testseam"
	"github.com/spf13/cobra"
)

func TestOutputSinkAtomicallyReplacesTargetWithMode0600(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "result.txt")
	if err := os.WriteFile(target, []byte("original"), 0o644); err != nil {
		t.Fatal(err)
	}

	var tempMode os.FileMode
	root := newAtomicOutputTestRoot(func(cmd *cobra.Command) error {
		info, err := cmd.OutOrStdout().(*os.File).Stat()
		if err != nil {
			return err
		}
		tempMode = info.Mode().Perm()
		_, err = fmt.Fprint(cmd.OutOrStdout(), "replacement")
		return err
	})
	root.SetArgs([]string{"atomic-output", "--output", target})
	if _, err := root.ExecuteC(); err != nil {
		t.Fatalf("ExecuteC: %v", err)
	}

	assertOutputFile(t, target, "replacement", 0o600)
	if tempMode != 0o600 {
		t.Fatalf("temporary output mode=%#o, want 0600", tempMode)
	}
	assertNoOutputTemps(t, target)
}

func TestOutputSinkHandlerFailurePreservesTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "result.txt")
	if err := os.WriteFile(target, []byte("original"), 0o640); err != nil {
		t.Fatal(err)
	}

	root := newAtomicOutputTestRoot(func(cmd *cobra.Command) error {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), "partial")
		return errors.New("handler failed")
	})
	root.SetArgs([]string{"atomic-output", "--output", target})
	if _, err := root.ExecuteC(); err == nil {
		t.Fatal("ExecuteC succeeded")
	}

	assertOutputFile(t, target, "original", 0o640)
	assertNoOutputTemps(t, target)
}

func TestOutputSinkPanicCleansTempAndPreservesTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "result.txt")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	root := newAtomicOutputTestRoot(func(cmd *cobra.Command) error {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), "partial")
		panic("boom")
	})
	root.SetArgs([]string{"atomic-output", "--output", target})
	if recovered := executeAndRecover(root); recovered == nil {
		t.Fatal("ExecuteC did not panic")
	}

	assertOutputFile(t, target, "original", 0o600)
	assertNoOutputTemps(t, target)
}

func TestOutputSinkRenameFailurePreservesTarget(t *testing.T) {
	testseam.Swap(t, &rootRenameFile, func(string, string) error {
		return errors.New("rename failed")
	})
	dir := t.TempDir()
	target := filepath.Join(dir, "result.txt")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	root := newAtomicOutputTestRoot(func(cmd *cobra.Command) error {
		_, err := fmt.Fprint(cmd.OutOrStdout(), "replacement")
		return err
	})
	root.SetArgs([]string{"atomic-output", "--output", target})
	if _, err := root.ExecuteC(); err == nil || err.Error() == "" {
		t.Fatalf("ExecuteC error=%v, want publication failure", err)
	}

	assertOutputFile(t, target, "original", 0o600)
	assertNoOutputTemps(t, target)
}

func TestOutputSinkSyncAndCloseFailuresPreserveTarget(t *testing.T) {
	tests := []struct {
		name string
		seam func(*testing.T)
	}{
		{
			name: "sync",
			seam: func(t *testing.T) {
				testseam.Swap(t, &rootSyncFile, func(*os.File) error { return errors.New("sync failed") })
			},
		},
		{
			name: "close",
			seam: func(t *testing.T) {
				testseam.Swap(t, &rootCloseFile, func(file *os.File) error {
					_ = file.Close()
					return errors.New("close failed")
				})
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.seam(t)
			dir := t.TempDir()
			target := filepath.Join(dir, "result.txt")
			if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
				t.Fatal(err)
			}

			root := newAtomicOutputTestRoot(func(cmd *cobra.Command) error {
				_, err := fmt.Fprint(cmd.OutOrStdout(), "replacement")
				return err
			})
			root.SetArgs([]string{"atomic-output", "--output", target})
			if _, err := root.ExecuteC(); err == nil {
				t.Fatal("ExecuteC succeeded")
			}

			assertOutputFile(t, target, "original", 0o600)
			assertNoOutputTemps(t, target)
		})
	}
}

func TestOutputSinkUnifiedPublicationFailureFailsAndLeavesNoFinalFile(t *testing.T) {
	testseam.Swap(t, &rootRenameFile, func(string, string) error {
		return errors.New("rename failed")
	})
	dir := t.TempDir()
	target := filepath.Join(dir, "result.json")

	root := NewRootCommand()
	leaf := &cobra.Command{
		Use: "atomic-output-unified",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return output.StoreResult(cmd.Context(), output.Success(map[string]any{"id": "ok"}))
		},
	}
	output.SetCommandRollout(leaf, output.RolloutUnifiedActive)
	root.AddCommand(leaf)
	root.SetArgs([]string{"atomic-output-unified", "--output", target})
	if _, err := root.ExecuteC(); err == nil {
		t.Fatal("unified ExecuteC succeeded without publishing its output")
	} else if code := apperrors.ExitCode(err); code != 5 {
		t.Fatalf("publication exit code=%d, want 5: %v", code, err)
	}
	if _, err := os.Stat(target); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("final output exists after publication failure: %v", err)
	}
	assertNoOutputTemps(t, target)
}

func TestOutputSinkEmissionFailurePreservesTarget(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "result.json")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	leaf := &cobra.Command{
		Use: "atomic-emission-failure",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := output.StoreResult(cmd.Context(), output.Success(map[string]any{"id": "ok"})); err != nil {
				return err
			}
			return cmd.OutOrStdout().(*os.File).Close()
		},
	}
	output.SetCommandRollout(leaf, output.RolloutUnifiedActive)
	root.AddCommand(leaf)
	root.SetArgs([]string{"atomic-emission-failure", "--output", target})
	if _, err := root.ExecuteC(); err == nil {
		t.Fatal("ExecuteC succeeded after emission failure")
	}

	assertOutputFile(t, target, "original", 0o600)
	assertNoOutputTemps(t, target)
}

func TestOutputSinkValidationFailureDoesNotCreateTemp(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "result.txt")
	if err := os.WriteFile(target, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}

	root := NewRootCommand()
	leaf := &cobra.Command{Use: "atomic-validation", RunE: func(*cobra.Command, []string) error { return nil }}
	leaf.Flags().String("required", "", "")
	_ = leaf.MarkFlagRequired("required")
	root.AddCommand(leaf)
	root.SetArgs([]string{"atomic-validation", "--output", target})
	if _, err := root.ExecuteC(); err == nil {
		t.Fatal("ExecuteC succeeded without required flag")
	}

	assertOutputFile(t, target, "original", 0o600)
	assertNoOutputTemps(t, target)
}

func newAtomicOutputTestRoot(run func(*cobra.Command) error) *cobra.Command {
	root := NewRootCommand()
	root.AddCommand(&cobra.Command{
		Use: "atomic-output",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return run(cmd)
		},
	})
	return root
}

func executeAndRecover(cmd *cobra.Command) (recovered any) {
	defer func() { recovered = recover() }()
	_, _ = cmd.ExecuteC()
	return nil
}

func assertOutputFile(t *testing.T, path, want string, wantMode os.FileMode) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if string(data) != want {
		t.Fatalf("output=%q, want %q", data, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat output: %v", err)
	}
	if mode := info.Mode().Perm(); mode != wantMode {
		t.Fatalf("output mode=%#o, want %#o", mode, wantMode)
	}
}

func assertNoOutputTemps(t *testing.T, target string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(target), "."+filepath.Base(target)+".tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary output files remain: %v", matches)
	}
}
