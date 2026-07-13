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
	"sync"
	"testing"
)

func TestListen_DialRoundtrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bus.sock")
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
	path := filepath.Join(t.TempDir(), "bus.sock")
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

func TestListen_CloseUnlinksSocket(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bus.sock")
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
	path := filepath.Join(t.TempDir(), "nonexistent.sock")
	if _, err := Dial(path); err == nil {
		t.Fatal("Dial to nonexistent socket should error")
	}
}

// TestReader_HandlesPeerCloseEOF asserts that a clean peer close mid-stream
// surfaces as io.EOF to the server's Reader — the EOF signal is what bus
// uses to unregister dead consumers (plan invariant #5).
func TestReader_HandlesPeerCloseEOF(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bus.sock")
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
