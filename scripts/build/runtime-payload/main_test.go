package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/runtimepayload"
)

func TestCrossPlatformCoverageRuntimePayloadCommand(t *testing.T) {
	cache := t.TempDir()
	library, err := runtimepayload.Materialize(runtimepayload.Embedded(), cache, runtime.GOOS, runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Dir(library)
	containerPath := filepath.Join(t.TempDir(), "payload")
	capacity := strconv.Itoa(len(runtimepayload.Embedded()))

	var stdout, stderr bytes.Buffer
	if code := run([]string{"generate", containerPath, root, capacity}, &stdout, &stderr); code != 0 {
		t.Fatalf("generate code = %d, stderr = %q", code, stderr.String())
	}
	generated, err := os.ReadFile(containerPath)
	if err != nil {
		t.Fatal(err)
	}
	binaryPath := filepath.Join(t.TempDir(), "dws")
	if err := os.WriteFile(binaryPath, append([]byte("binary"), generated...), 0o755); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"inject", binaryPath, root}, &stdout, &stderr); code != 0 {
		t.Fatalf("inject code = %d, stderr = %q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := run([]string{"materialize", binaryPath, t.TempDir(), runtime.GOOS, runtime.GOARCH}, &stdout, &stderr); code != 0 || strings.TrimSpace(stdout.String()) == "" {
		t.Fatalf("materialize code = %d, stdout = %q, stderr = %q", code, stdout.String(), stderr.String())
	}

	invalid := [][]string{
		nil,
		{"generate"},
		{"generate", containerPath, root, "invalid"},
		{"inject"},
		{"inject", filepath.Join(t.TempDir(), "missing"), root},
		{"materialize"},
		{"materialize", filepath.Join(t.TempDir(), "missing"), t.TempDir(), runtime.GOOS, runtime.GOARCH},
		{"unknown"},
	}
	for _, args := range invalid {
		stdout.Reset()
		stderr.Reset()
		if code := run(args, &stdout, &stderr); code == 0 || stderr.Len() == 0 {
			t.Fatalf("run(%q) = %d, stderr = %q", args, code, stderr.String())
		}
	}
}

func TestCrossPlatformCoverageRuntimePayloadMain(t *testing.T) {
	previousArgs := os.Args
	previousExit := exitProcess
	t.Cleanup(func() {
		os.Args = previousArgs
		exitProcess = previousExit
	})
	os.Args = []string{"runtime-payload", "unknown"}
	code := -1
	exitProcess = func(value int) { code = value }
	main()
	if code != 2 {
		t.Fatalf("exit code = %d", code)
	}
}
