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

//go:build !windows

package transport

import (
	"errors"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	dwsevent "github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event"
)

func shortSecureTempDir(t *testing.T) string {
	t.Helper()
	tempRoot, err := filepath.EvalSymlinks("/tmp")
	if err != nil {
		t.Fatalf("EvalSymlinks: %v", err)
	}
	dir, err := os.MkdirTemp(tempRoot, "dws-et-")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("chmod temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

func TestListen_DialRoundtrip(t *testing.T) {
	path := filepath.Join(shortSecureTempDir(t), "bus.sock")
	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()

	if l.Endpoint() != path {
		t.Errorf("Endpoint = %q, want %q", l.Endpoint(), path)
	}

	// Verify socket file exists with expected mode (0600).
	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat socket: %v", err)
	}
	if mode := st.Mode().Perm(); mode != 0o600 {
		t.Errorf("socket mode = %v, want 0600", mode)
	}

	var serverConn net.Conn
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		c, err := l.Accept()
		if err != nil {
			t.Errorf("Accept: %v", err)
			return
		}
		serverConn = c
	}()

	clientConn, err := Dial(path)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer clientConn.Close()

	wg.Wait()
	if serverConn == nil {
		t.Fatal("server did not accept")
	}
	defer serverConn.Close()

	// Roundtrip a frame
	w := NewWriter(clientConn)
	if err := w.WriteJSON(Hello{Type: FrameTypeHello, ConsumerPID: 123}); err != nil {
		t.Fatalf("WriteJSON: %v", err)
	}
	r := NewReader(serverConn)
	var got Hello
	if err := r.ReadJSON(&got); err != nil {
		t.Fatalf("ReadJSON: %v", err)
	}
	if got.ConsumerPID != 123 {
		t.Errorf("PID = %d", got.ConsumerPID)
	}
}

func TestListen_StaleSocketCleanup(t *testing.T) {
	path := filepath.Join(shortSecureTempDir(t), "bus.sock")
	// Pre-create a stale file at path (not a valid socket).
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatalf("pre-create: %v", err)
	}
	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen should clean stale file, got: %v", err)
	}
	defer l.Close()
}

func TestCrossPlatformCoverageListenCreatesPrivateSocketDirectory(t *testing.T) {
	dir := filepath.Join(shortSecureTempDir(t), "dws-event-test")
	path := filepath.Join(dir, "bus.sock")
	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()

	st, err := os.Stat(dir)
	if err != nil {
		t.Fatalf("stat socket directory: %v", err)
	}
	if mode := st.Mode().Perm(); mode != 0o700 {
		t.Fatalf("socket directory mode = %04o, want 0700", mode)
	}
}

func TestCrossPlatformCoverageListenRejectsWorldAccessibleSocketDirectory(t *testing.T) {
	dir := filepath.Join(shortSecureTempDir(t), "dws-event-test")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if _, err := Listen(filepath.Join(dir, "bus.sock")); err == nil || !strings.Contains(err.Error(), "want 0700") {
		t.Fatalf("Listen error = %v, want 0700 directory rejection", err)
	}
}

func TestCrossPlatformCoverageDialRejectsWorldAccessibleSocketDirectory(t *testing.T) {
	dir := filepath.Join(shortSecureTempDir(t), "dws-event-test")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	if _, err := Dial(filepath.Join(dir, "bus.sock")); err == nil || !strings.Contains(err.Error(), "want 0700") {
		t.Fatalf("Dial error = %v, want 0700 directory rejection", err)
	}
}

func TestCrossPlatformCoverageListenRejectsSymlinkSocketDirectory(t *testing.T) {
	root := shortSecureTempDir(t)
	target := filepath.Join(root, "target")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatalf("mkdir target: %v", err)
	}
	link := filepath.Join(root, "dws-event-test")
	if err := os.Symlink(target, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	if _, err := Listen(filepath.Join(link, "bus.sock")); err == nil || !strings.Contains(err.Error(), "not a directory") {
		t.Fatalf("Listen error = %v, want symlink directory rejection", err)
	}
}

func TestCrossPlatformCoverageValidatePrivateSocketDirRejectsDifferentOwner(t *testing.T) {
	dir := shortSecureTempDir(t)
	st, err := os.Lstat(dir)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	otherUID := uint32(os.Geteuid() + 1)
	if err := validatePrivateSocketDir(dir, st, otherUID); err == nil || !strings.Contains(err.Error(), "is owned by uid") {
		t.Fatalf("validatePrivateSocketDir error = %v, want owner mismatch", err)
	}
}

func TestCrossPlatformCoverageListenRejectsUntrustedRuntimeRoot(t *testing.T) {
	root := filepath.Join(shortSecureTempDir(t), "untrusted")
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatalf("mkdir root: %v", err)
	}
	if err := os.Chmod(root, 0o777); err != nil {
		t.Fatalf("chmod root: %v", err)
	}
	dir := filepath.Join(root, "dws-event-test")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatalf("mkdir socket dir: %v", err)
	}
	if _, err := Listen(filepath.Join(dir, "bus.sock")); err == nil || !strings.Contains(err.Error(), "neither private nor sticky") {
		t.Fatalf("Listen error = %v, want untrusted runtime root rejection", err)
	}
}

func TestCrossPlatformCoverageListenSharedWorkDirUsesLocalSecureRuntimeEndpoint(t *testing.T) {
	root := shortSecureTempDir(t)
	runtimeDir := filepath.Join(root, "runtime")
	if err := os.Mkdir(runtimeDir, 0o700); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	t.Setenv("XDG_RUNTIME_DIR", runtimeDir)
	sharedWorkDir := filepath.Join(root, "simulated-nfs", "events", "open", "personal_stream", "identity")
	endpoint := dwsevent.IPCEndpoint(sharedWorkDir, "open", dwsevent.SourceKindPersonalStream, "identity")
	if strings.HasPrefix(endpoint, sharedWorkDir) {
		t.Fatalf("endpoint = %q, want socket outside shared WorkDir %q", endpoint, sharedWorkDir)
	}

	l, err := Listen(endpoint)
	if err != nil {
		t.Fatalf("Listen on local runtime endpoint: %v", err)
	}
	defer l.Close()
	if mode, err := os.Stat(filepath.Dir(endpoint)); err != nil {
		t.Fatalf("stat runtime socket directory: %v", err)
	} else if mode.Mode().Perm() != 0o700 {
		t.Fatalf("runtime socket directory mode = %04o, want 0700", mode.Mode().Perm())
	}
}

func TestListen_CloseUnlinksSocket(t *testing.T) {
	path := filepath.Join(shortSecureTempDir(t), "bus.sock")
	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	if err := l.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("socket should be unlinked after Close, stat err = %v", err)
	}
}

func TestDial_NoServerReturnsError(t *testing.T) {
	path := filepath.Join(shortSecureTempDir(t), "nonexistent.sock")
	if _, err := Dial(path); err == nil {
		t.Fatal("Dial to nonexistent socket should error")
	}
}

// TestReader_HandlesPeerCloseEOF asserts that a clean peer close mid-stream
// surfaces as io.EOF to the server's Reader — the EOF signal is what bus
// uses to unregister dead consumers (plan invariant #5).
func TestReader_HandlesPeerCloseEOF(t *testing.T) {
	path := filepath.Join(shortSecureTempDir(t), "bus.sock")
	l, err := Listen(path)
	if err != nil {
		t.Fatalf("Listen: %v", err)
	}
	defer l.Close()

	done := make(chan error, 1)
	go func() {
		c, err := l.Accept()
		if err != nil {
			done <- err
			return
		}
		defer c.Close()
		r := NewReader(c)
		_, err = r.Read()
		done <- err
	}()

	clientConn, err := Dial(path)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	// Close immediately — server should see EOF on first Read.
	_ = clientConn.Close()

	err = <-done
	if !errors.Is(err, io.EOF) {
		t.Fatalf("server Read after peer close = %v, want EOF", err)
	}
}
