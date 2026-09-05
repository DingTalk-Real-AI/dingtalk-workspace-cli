package launcher

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestVersionBypassesFilesystemExactly(t *testing.T) {
	var stdout bytes.Buffer
	deps := dependencies{args: []string{"dws", "--version"}, environ: []string{"DO_NOT_TRACK=1"}, stdout: &stdout, executable: func() (string, error) {
		t.Fatal("version path touched executable")
		return "", nil
	}}
	if err := run(testOptions(0), deps); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "dws version 1.2.3\n" {
		t.Fatalf("version output = %q", got)
	}
}

func TestVersionPreservesTelemetryUnlessExplicitlyOptedOut(t *testing.T) {
	for _, environment := range [][]string{nil, {"DO_NOT_TRACK="}, {"DO_NOT_TRACK=  "}} {
		deps, options, _ := testCoreSetup(t, []byte("trusted"))
		deps.args = []string{"dws", "--version"}
		deps.environ = environment
		delegated := false
		deps.delegate = func(_ string, args, env []string, _ string, _ io.Reader, _, _ io.Writer) (int, error) {
			delegated = true
			if !reflect.DeepEqual(args, deps.args) {
				t.Fatalf("version argv changed: %v", args)
			}
			return 0, nil
		}
		if err := run(options, deps); err != nil {
			t.Fatal(err)
		}
		if !delegated {
			t.Fatal("version silently bypassed existing identity/clitrack behavior")
		}
	}
}

func TestDelegatesToVersionedSiblingWithProcessState(t *testing.T) {
	root := t.TempDir()
	launcherPath := filepath.Join(root, "bin", "dws")
	corePath := filepath.Join(root, "libexec", coreExecutableName())
	for _, directory := range []string{filepath.Dir(launcherPath), filepath.Dir(corePath)} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(launcherPath, []byte("launcher"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corePath, []byte("core"), 0o700); err != nil {
		t.Fatal(err)
	}
	deps := systemDependencies()
	deps.args = []string{"dws", "schema", "calendar.event.instances"}
	deps.environ = []string{"KEEP=value", EnvLauncherPath + "=forged", EnvCoreDigest + "=forged", EnvCoreVersion + "=forged"}
	deps.stdin, deps.stdout, deps.stderr = strings.NewReader("input"), &bytes.Buffer{}, &bytes.Buffer{}
	deps.executable = func() (string, error) { return launcherPath, nil }
	deps.evalSymlinks = func(string) (string, error) { return launcherPath, nil }
	deps.getwd = func() (string, error) { return "/working", nil }
	deps.delegate = func(path string, argv, environment []string, cwd string, stdin io.Reader, stdout, stderr io.Writer) (int, error) {
		if path != corePath || !reflect.DeepEqual(argv, deps.args) || cwd != "/working" || stdin != deps.stdin || stdout != deps.stdout || stderr != deps.stderr {
			t.Fatalf("delegation = %q %#v %q", path, argv, cwd)
		}
		wantEnvironment := []string{"KEEP=value", EnvLauncherPath + "=" + launcherPath, EnvCoreDigest + "=" + digestFor([]byte("core")), EnvCoreVersion + "=1.2.3"}
		if !reflect.DeepEqual(environment, wantEnvironment) {
			t.Fatalf("environment = %#v, want %#v", environment, wantEnvironment)
		}
		return 23, nil
	}
	options := testOptions(4)
	options.CoreSHA256 = digestFor([]byte("core"))
	err := run(options, deps)
	var exit *ExitError
	if !errors.As(err, &exit) || exit.Code != 23 {
		t.Fatalf("result = %#v", err)
	}
}

func TestRejectsTamperedSameSizeCore(t *testing.T) {
	deps, options, corePath := testCoreSetup(t, []byte("trusted"))
	if err := os.WriteFile(corePath, []byte("altered"), 0o700); err != nil {
		t.Fatal(err)
	}
	err := run(options, deps)
	if err == nil || !strings.Contains(err.Error(), "SHA-256 mismatch") {
		t.Fatalf("same-size tampered core result = %v", err)
	}
}

func TestRejectsCoreChangedDuringHash(t *testing.T) {
	deps, options, corePath := testCoreSetup(t, []byte("trusted"))
	baseOpen := deps.open
	deps.open = func(path string) (coreFile, error) {
		file, err := baseOpen(path)
		if err != nil {
			return nil, err
		}
		return &testCoreFile{coreFile: file, reader: &changeModeReader{reader: file, path: corePath}}, nil
	}
	err := run(options, deps)
	if err == nil || !strings.Contains(err.Error(), "changed while hashing") {
		t.Fatalf("changed-during-hash result = %v", err)
	}
}

func TestRejectsCoreReadFailure(t *testing.T) {
	deps, options, _ := testCoreSetup(t, []byte("trusted"))
	baseOpen := deps.open
	deps.open = func(path string) (coreFile, error) {
		file, err := baseOpen(path)
		if err != nil {
			return nil, err
		}
		return &testCoreFile{coreFile: file, reader: errorReader{}}, nil
	}
	err := run(options, deps)
	if err == nil || !strings.Contains(err.Error(), "hash core: test read failure") {
		t.Fatalf("read-failure result = %v", err)
	}
}

func BenchmarkLauncherCoreVerification(b *testing.B) {
	content := bytes.Repeat([]byte("dws-core-benchmark"), 1<<16)
	deps, options, _ := testCoreSetup(b, content)
	b.SetBytes(int64(len(content)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := run(options, deps); err != nil {
			b.Fatal(err)
		}
	}
}

func TestRejectsMissingAliasedAndWrongSizeCore(t *testing.T) {
	root := t.TempDir()
	launcherPath := filepath.Join(root, "bin", "dws")
	if err := os.MkdirAll(filepath.Dir(launcherPath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(launcherPath, []byte("launcher"), 0o700); err != nil {
		t.Fatal(err)
	}
	deps := systemDependencies()
	deps.args = []string{"dws", "help"}
	deps.stdin, deps.stdout, deps.stderr = strings.NewReader(""), io.Discard, io.Discard
	deps.executable = func() (string, error) { return launcherPath, nil }
	deps.evalSymlinks = func(string) (string, error) { return launcherPath, nil }
	if err := run(testOptions(4), deps); err == nil {
		t.Fatal("missing core accepted")
	}
	corePath := filepath.Join(root, "libexec", coreExecutableName())
	if err := os.MkdirAll(filepath.Dir(corePath), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corePath, []byte("wrong"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := run(testOptions(4), deps); err == nil {
		t.Fatal("wrong core size accepted")
	}
	if err := os.Remove(corePath); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(launcherPath, corePath); err != nil {
		t.Fatal(err)
	}
	if err := run(testOptions(int64(len("launcher"))), deps); err == nil {
		t.Fatal("core aliasing launcher accepted")
	}
}

const testDigest = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func testOptions(size int64) Options {
	return Options{Version: "1.2.3", Commit: "abcdef", Edition: "open", CoreSHA256: testDigest, CoreSize: size}
}

func testCoreSetup(t testing.TB, content []byte) (dependencies, Options, string) {
	t.Helper()
	root := t.TempDir()
	launcherPath := filepath.Join(root, "bin", "dws")
	corePath := filepath.Join(root, "libexec", coreExecutableName())
	for _, directory := range []string{filepath.Dir(launcherPath), filepath.Dir(corePath)} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(launcherPath, []byte("launcher"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corePath, content, 0o700); err != nil {
		t.Fatal(err)
	}
	deps := systemDependencies()
	deps.args = []string{"dws", "help"}
	deps.stdin, deps.stdout, deps.stderr = strings.NewReader(""), io.Discard, io.Discard
	deps.executable = func() (string, error) { return launcherPath, nil }
	deps.evalSymlinks = func(string) (string, error) { return launcherPath, nil }
	deps.getwd = func() (string, error) { return root, nil }
	deps.delegate = func(string, []string, []string, string, io.Reader, io.Writer, io.Writer) (int, error) {
		return 0, nil
	}
	options := testOptions(int64(len(content)))
	options.CoreSHA256 = digestFor(content)
	return deps, options, corePath
}

func digestFor(content []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(content))
}

type testCoreFile struct {
	coreFile
	reader io.Reader
}

func (f *testCoreFile) Read(buffer []byte) (int, error) { return f.reader.Read(buffer) }

type changeModeReader struct {
	reader  io.Reader
	path    string
	changed bool
}

func (r *changeModeReader) Read(buffer []byte) (int, error) {
	n, err := r.reader.Read(buffer)
	if n > 0 && !r.changed {
		r.changed = true
		if chmodErr := os.Chmod(r.path, 0o600); chmodErr != nil {
			return n, chmodErr
		}
	}
	return n, err
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, errors.New("test read failure") }
