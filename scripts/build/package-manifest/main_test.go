package main

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/packagemanifest"
)

func TestPackageManifestCommand(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(root, "libexec"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := packagemanifest.Target{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH}
	launcher, core, err := packagemanifest.Paths(target)
	if err != nil {
		t.Fatal(err)
	}
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	if err := os.WriteFile(filepath.Join(root, launcher), []byte("signed launcher"), mode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, core), []byte("signed core"), mode); err != nil {
		t.Fatal(err)
	}
	launcherBefore, _ := os.ReadFile(filepath.Join(root, launcher))
	coreBefore, _ := os.ReadFile(filepath.Join(root, core))

	args := []string{
		"--package-root", root,
		"--version", "v1.2.3",
		"--commit", "0123456789abcdef0123456789abcdef01234567",
		"--edition", "internal",
		"--goos", runtime.GOOS,
		"--goarch", runtime.GOARCH,
	}
	var stdout, stderr bytes.Buffer
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("run code = %d, stderr = %q", code, stderr.String())
	}
	if stdout.String() != packagemanifest.ManifestName+"\n" {
		t.Fatalf("stdout = %q", stdout.String())
	}
	launcherAfter, _ := os.ReadFile(filepath.Join(root, launcher))
	coreAfter, _ := os.ReadFile(filepath.Join(root, core))
	if !bytes.Equal(launcherBefore, launcherAfter) || !bytes.Equal(coreBefore, coreAfter) {
		t.Fatal("command mutated executable bytes")
	}
	args = append([]string{"--verify"}, args...)
	stdout.Reset()
	stderr.Reset()
	if code := run(args, &stdout, &stderr); code != 0 {
		t.Fatalf("verify code = %d, stderr = %q", code, stderr.String())
	}
}

func TestPackageManifestCommandErrors(t *testing.T) {
	cases := [][]string{
		nil,
		{"--unknown"},
		{"--package-root", t.TempDir(), "--version", "v1", "--commit", "c", "--edition", "e", "--goos", runtime.GOOS, "--goarch", runtime.GOARCH},
		{"--package-root", t.TempDir(), "--version", "v1", "--commit", "ABCDEF0123456789ABCDEF0123456789ABCDEF01", "--edition", "e", "--goos", runtime.GOOS, "--goarch", runtime.GOARCH},
	}
	for _, args := range cases {
		var stdout, stderr bytes.Buffer
		if code := run(args, &stdout, &stderr); code == 0 || stderr.Len() == 0 {
			t.Fatalf("run(%q) = %d, stderr = %q", args, code, stderr.String())
		}
	}
}

func TestPackageManifestMain(t *testing.T) {
	previousArgs := os.Args
	previousExit := exitProcess
	t.Cleanup(func() {
		os.Args = previousArgs
		exitProcess = previousExit
	})
	os.Args = []string{"package-manifest"}
	code := -1
	exitProcess = func(value int) { code = value }
	main()
	if code != 2 {
		t.Fatalf("exit code = %d", code)
	}
}
