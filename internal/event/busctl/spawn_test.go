// Copyright 2026 Alibaba Group
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package busctl

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Test child mode: when DWS_BUSCTL_TEST_CHILD is set, this test binary
// acts as a fake "dws event _bus" child. It reads ReadyFDFromEnv, writes
// the byte specified by the env var, and either exits or sleeps based on
// the second env var. Used by the Spawn tests below to exercise the
// real fork path without requiring a separate test-helper binary.
//
// We piggy-back on the test binary because building a separate helper
// would require either build tags or a `TestMain` two-phase exec; the
// env-marker pattern is what Go's own os/exec tests use and stays
// confined to this file.
const (
	childEnvMarker = "DWS_BUSCTL_TEST_CHILD"
	// values:
	//   "ready"        — write 'R' then sleep 30s (parent should see ready)
	//   "fail"         — write 'E' then exit (parent should see ErrSpawnFailed)
	//   "silent"       — exit without writing (parent should see ErrSpawnFailed via EOF)
	//   "stall"        — sleep without writing (parent should see ErrSpawnTimeout)
	//   "write-stdout" — write 'R' to ready FD AND to stdout (parent verifies stdout was detached)
)

// TestMain detects the child mode marker and executes the corresponding
// behaviour before delegating to the normal test runner. Production
// invocations never have this env var set so the dispatch is a no-op.
func TestMain(m *testing.M) {
	switch os.Getenv(childEnvMarker) {
	case "ready":
		writeReady('R')
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "fail":
		writeReady('E')
		os.Exit(1)
	case "silent":
		// don't open the ready FD at all — let the pipe close on exec exit
		os.Exit(2)
	case "stall":
		time.Sleep(30 * time.Second)
		os.Exit(0)
	case "write-stdout":
		// Write to stdout BEFORE signalling ready. Parent's Spawn should
		// have detached stdout to /dev/null, so the parent's stdout
		// buffer (captured separately in the test) must NOT see this.
		_, _ = os.Stdout.Write([]byte("POLLUTION-FROM-CHILD\n"))
		writeReady('R')
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func writeReady(b byte) {
	pipe := ReadyFDFromEnv()
	if pipe == nil {
		return
	}
	_, _ = pipe.Write([]byte{b})
	_ = pipe.Close()
}

// spawnWithMarker invokes Spawn against the current test binary with a
// child-mode env var set. Returns the child's PID + the Spawn error.
func spawnWithMarker(t *testing.T, marker string, opts ...func(*SpawnConfig)) (int, error) {
	t.Helper()
	cfg := SpawnConfig{
		ExecPath: os.Args[0],
		ClientID: "ding_spawn_test",
		Env: append(os.Environ(),
			childEnvMarker+"="+marker,
		),
	}
	for _, o := range opts {
		o(&cfg)
	}
	return Spawn(cfg)
}

func TestSpawn_ReadySuccess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Spawn uses Unix Setsid; Windows path covered separately")
	}
	pid, err := spawnWithMarker(t, "ready")
	if err != nil {
		t.Fatalf("Spawn ready: %v", err)
	}
	if pid <= 0 {
		t.Errorf("Spawn returned non-positive pid %d", pid)
	}
	// Reap the child so it doesn't outlive the test.
	if proc, err := os.FindProcess(pid); err == nil {
		_ = proc.Kill()
		_, _ = proc.Wait()
	}
}

func TestSpawn_ReadyFailReportsErrSpawnFailed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Spawn uses Unix Setsid")
	}
	pid, err := spawnWithMarker(t, "fail")
	if !errors.Is(err, ErrSpawnFailed) {
		t.Fatalf("err = %v, want ErrSpawnFailed", err)
	}
	if pid <= 0 {
		t.Errorf("pid should still be reported even on fail, got %d", pid)
	}
}

func TestSpawn_ChildSilentExitReportsErrSpawnFailed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Spawn uses Unix Setsid")
	}
	_, err := spawnWithMarker(t, "silent")
	if !errors.Is(err, ErrSpawnFailed) {
		t.Fatalf("err = %v, want ErrSpawnFailed (EOF on pipe)", err)
	}
}

func TestSpawn_StallReportsTimeout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Spawn uses Unix Setsid")
	}
	// Temporarily shorten ReadyTimeout via a local Spawn variant. The
	// production timeout is 10s — too long for a unit test. We exec
	// manually with a tiny io-wait wrapper to verify the behaviour.
	//
	// We can't change the package-level const, so we re-implement the
	// timeout part directly using the same primitives the production
	// code uses.
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), childEnvMarker+"=stall", ReadyFDEnv+"=3")
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	cmd.ExtraFiles = []*os.File{pw}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	_ = pw.Close()

	// Replicate waitReady with a tiny timeout.
	done := make(chan error, 1)
	go func() {
		b := make([]byte, 1)
		_, err := pr.Read(b)
		done <- err
	}()
	select {
	case <-done:
		t.Fatal("child should have stalled; got data on ready pipe")
	case <-time.After(200 * time.Millisecond):
		// expected — child is stalling, no ready byte arrived
	}
	_ = pr.Close()
}

func TestSpawn_StdioDetached(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Spawn uses Unix Setsid; Windows stdio handling differs")
	}
	// Capture this process's stdout for the duration of the child run.
	// If applyDetach is broken and cmd.Stdout would otherwise inherit,
	// the child's "POLLUTION-FROM-CHILD" line would land in our pipe.
	origStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	defer func() {
		os.Stdout = origStdout
		_ = w.Close()
		_ = r.Close()
	}()

	pid, err := spawnWithMarker(t, "write-stdout")
	if err != nil {
		t.Fatalf("Spawn: %v", err)
	}
	defer func() {
		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Kill()
			_, _ = proc.Wait()
		}
	}()
	// Close the write end on the parent side so reading r will EOF if
	// nothing arrived. Give the child a beat to attempt the write.
	time.Sleep(150 * time.Millisecond)
	_ = w.Close()

	buf := make([]byte, 256)
	r.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	n, _ := r.Read(buf)
	if n > 0 {
		t.Fatalf("child wrote %q to parent's stdout — stdio not detached!", buf[:n])
	}
}

func TestReadyFDFromEnv_NoEnvReturnsNil(t *testing.T) {
	t.Setenv(ReadyFDEnv, "")
	if f := ReadyFDFromEnv(); f != nil {
		t.Errorf("ReadyFDFromEnv with empty env should be nil, got %v", f)
	}
}

func TestReadyFDFromEnv_InvalidIntReturnsNil(t *testing.T) {
	t.Setenv(ReadyFDEnv, "not-an-int")
	if f := ReadyFDFromEnv(); f != nil {
		t.Errorf("ReadyFDFromEnv with bad value should be nil, got %v", f)
	}
}

func TestReadyFDFromEnv_LowFDRejected(t *testing.T) {
	// fd 0/1/2 are stdio — refusing them defends against accidental
	// stdin/stdout/stderr corruption if someone misconfigures.
	t.Setenv(ReadyFDEnv, "1")
	if f := ReadyFDFromEnv(); f != nil {
		t.Errorf("ReadyFDFromEnv should reject stdio fds, got %v", f)
	}
}

// waitReady must surface the child's real startup error (written after the
// 'E' byte) so consume shows WHY the bus failed, not an opaque message.
func TestWaitReady_FailureSurfacesChildError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix pipe")
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	const reason = "newPersonalStreamSource: token expired for corp dinga626"
	go func() {
		_, _ = pw.Write([]byte{'E'})
		_, _ = io.WriteString(pw, reason)
		_ = pw.Close()
	}()
	err = waitReady(pr)
	if !errors.Is(err, ErrSpawnFailed) {
		t.Fatalf("want ErrSpawnFailed, got %v", err)
	}
	if !strings.Contains(err.Error(), reason) {
		t.Errorf("error must surface the child's real reason; got: %v", err)
	}
}

// A bare 'E' (no trailing text) still reports ErrSpawnFailed.
func TestWaitReady_BareFailureByte(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix pipe")
	}
	pr, pw, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer pr.Close()
	go func() { _, _ = pw.Write([]byte{'E'}); _ = pw.Close() }()
	if err := waitReady(pr); !errors.Is(err, ErrSpawnFailed) {
		t.Fatalf("want ErrSpawnFailed, got %v", err)
	}
}
