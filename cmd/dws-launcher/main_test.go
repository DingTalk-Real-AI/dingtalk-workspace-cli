package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCrossPlatformCoveragePartialEmbeddedSchemaIdentityFailsInRealProcess(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "dws-launcher")
	if os.PathSeparator == '\\' {
		binary += ".exe"
	}
	ldflags := strings.Join([]string{
		"-X", "main.version=v1.2.3", "-X", "main.commit=" + strings.Repeat("a", 40),
		"-X", "main.buildTime=2026-09-06T00:00:00Z", "-X", "main.edition=open",
		"-X", "main.coreSHA256=" + strings.Repeat("b", 64), "-X", "main.coreSize=1",
		"-X", "main.schemaCacheEdition=open",
	}, " ")
	build := exec.Command("go", "build", "-trimpath", "-ldflags", ldflags, "-o", binary, "./cmd/dws-launcher")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build partial launcher: %v\n%s", err, output)
	}
	command := exec.Command(binary, "--version")
	command.Env = append(os.Environ(), "DO_NOT_TRACK=1")
	output, err := command.CombinedOutput()
	var exit *exec.ExitError
	if !strings.Contains(string(output), "partial Schema cache identity") ||
		!strings.Contains(string(output), "configuration validate release identity") ||
		!errors.As(err, &exit) || exit.ExitCode() != 125 {
		t.Fatalf("partial launcher result: err=%v output=%q", err, output)
	}
}
