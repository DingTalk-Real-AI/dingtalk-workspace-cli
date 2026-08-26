package helpers

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/spf13/cobra"
)

const (
	helpersShellStubEnv        = "DWS_HELPERS_SHELL_STUB"
	helpersShellStubBodySuffix = ".shell-body"
)

var (
	helpersShellStubBaseOnce sync.Once
	helpersShellStubBasePath string
	helpersShellStubBaseDir  string
	helpersShellStubBaseErr  error
)

func testExecutablePath(dir, name string) string {
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return filepath.Join(dir, name)
}

// TestMain turns a copied helpers test binary into a native Windows executable
// fixture. The fixture delegates its saved body to GitHub's bundled sh while
// preserving stdin/stdout/stderr, so stream and app-server tests exercise the
// same protocol on every platform.
func TestMain(m *testing.M) {
	if os.Getenv(helpersShellStubEnv) == "1" {
		os.Exit(runHelpersShellStub())
	}
	// Cobra's Windows pre-exec hook walks the process table on every Execute*
	// call to detect Explorer launches. Helpers tests execute command trees
	// thousands of times, and none of those in-process invocations can be an
	// Explorer launch, so keep that production-only check out of the test
	// process. This also prevents every new exhaustive harness from having to
	// remember a test-local override.
	originalMousetrapHelpText := cobra.MousetrapHelpText
	cobra.MousetrapHelpText = ""
	code := m.Run()
	cobra.MousetrapHelpText = originalMousetrapHelpText
	if helpersShellStubBaseDir != "" {
		_ = os.RemoveAll(helpersShellStubBaseDir)
	}
	os.Exit(code)
}

func runHelpersShellStub() int {
	executable, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	body, err := os.ReadFile(executable + helpersShellStubBodySuffix)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	cmd := exec.Command("sh", "-c", string(body))
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func copyCurrentHelpersTestBinary(destination string) error {
	helpersShellStubBaseOnce.Do(func() {
		helpersShellStubBaseDir, helpersShellStubBaseErr = os.MkdirTemp("", "dws-helpers-exec-stub-")
		if helpersShellStubBaseErr != nil {
			return
		}
		source, err := os.Executable()
		if err != nil {
			helpersShellStubBaseErr = err
			return
		}
		helpersShellStubBasePath = filepath.Join(helpersShellStubBaseDir, "helpers-shell-stub.exe")
		helpersShellStubBaseErr = copyFile(source, helpersShellStubBasePath)
	})
	if helpersShellStubBaseErr != nil {
		return helpersShellStubBaseErr
	}
	if err := os.Link(helpersShellStubBasePath, destination); err == nil {
		return nil
	}
	return copyFile(helpersShellStubBasePath, destination)
}

func copyFile(source, destination string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o700)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(destination)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(destination)
		return closeErr
	}
	return nil
}
