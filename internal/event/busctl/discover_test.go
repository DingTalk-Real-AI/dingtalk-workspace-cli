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
	"net"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/DingTalk-Real-AI/dingtalk-workspace-cli/internal/event/transport"
)

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("uses Unix socket; Windows transport covered by transport_windows_test.go")
	}
}

func shortTempDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("/tmp", "dws-busctl-")
	if err != nil {
		t.Fatalf("mktemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	return dir
}

// startStubBus brings up a transport.Listener at sockPath and accepts
// connections in a goroutine, discarding the data. Returns a closer.
// Used as a stand-in for the real bus daemon in discover unit tests.
func startStubBus(t *testing.T, sockPath string) func() {
	t.Helper()
	l, err := transport.Listen(sockPath)
	if err != nil {
		t.Fatalf("startStubBus listen: %v", err)
	}
	done := make(chan struct{})
	go func() {
		for {
			conn, err := l.Accept()
			if err != nil {
				close(done)
				return
			}
			go func(c net.Conn) {
				buf := make([]byte, 256)
				for {
					if _, err := c.Read(buf); err != nil {
						_ = c.Close()
						return
					}
				}
			}(conn)
		}
	}()
	return func() {
		_ = l.Close()
		<-done
	}
}

func TestDiscover_BusAlreadyRunning_DirectDial(t *testing.T) {
	skipOnWindows(t)
	dir := shortTempDir(t)
	sock := filepath.Join(dir, "bus.sock")
	closer := startStubBus(t, sock)
	defer closer()

	var spawnCalled atomic.Bool
	fakeSpawn := func(SpawnConfig) (int, error) {
		spawnCalled.Store(true)
		return 0, nil
	}

	conn, err := Discover(DiscoverConfig{
		WorkDir:     dir,
		IPCEndpoint: sock,
		ClientID:    "ding_x",
		Spawn:       fakeSpawn,
	})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	defer conn.Close()
	if spawnCalled.Load() {
		t.Fatal("Spawn must not be called when bus is already running")
	}
}

func TestDiscover_NoBus_SpawnSucceeds(t *testing.T) {
	skipOnWindows(t)
	dir := shortTempDir(t)
	sock := filepath.Join(dir, "bus.sock")

	// fakeSpawn launches the stub bus *during* the spawn call to mirror
	// the real flow (bus is up by the time spawn returns).
	var closer func()
	t.Cleanup(func() {
		if closer != nil {
			closer()
		}
	})
	fakeSpawn := func(SpawnConfig) (int, error) {
		closer = startStubBus(t, sock)
		return 12345, nil
	}

	conn, err := Discover(DiscoverConfig{
		WorkDir:     dir,
		IPCEndpoint: sock,
		ClientID:    "ding_x",
		Spawn:       fakeSpawn,
	})
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	defer conn.Close()
}

func TestDiscover_SpawnHardErrorFails(t *testing.T) {
	skipOnWindows(t)
	dir := shortTempDir(t)
	sock := filepath.Join(dir, "bus.sock")

	hardErr := errors.New("exec failed: not found")
	fakeSpawn := func(SpawnConfig) (int, error) { return 0, hardErr }

	_, err := Discover(DiscoverConfig{
		WorkDir:      dir,
		IPCEndpoint:  sock,
		ClientID:     "ding_x",
		Spawn:        fakeSpawn,
		DialDeadline: 100 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("Discover should fail when Spawn returns non-ErrSpawnFailed error")
	}
	if !errors.Is(err, hardErr) {
		t.Fatalf("err = %v, want wrap of %v", err, hardErr)
	}
}

func TestDiscover_SpawnReportsFailButPeerBusComesUp(t *testing.T) {
	skipOnWindows(t)
	dir := shortTempDir(t)
	sock := filepath.Join(dir, "bus.sock")

	// Simulates the race: our Spawn loses (returns ErrSpawnFailed) but
	// during retry-dial a peer bus shows up.
	go func() {
		time.Sleep(80 * time.Millisecond)
		closer := startStubBus(t, sock)
		t.Cleanup(closer)
	}()

	fakeSpawn := func(SpawnConfig) (int, error) { return 0, ErrSpawnFailed }

	conn, err := Discover(DiscoverConfig{
		WorkDir:      dir,
		IPCEndpoint:  sock,
		ClientID:     "ding_x",
		Spawn:        fakeSpawn,
		DialDeadline: 2 * time.Second,
		DialBackoff:  20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Discover should retry-dial after ErrSpawnFailed: %v", err)
	}
	defer conn.Close()
}

func TestDiscover_DeadlineExceeded(t *testing.T) {
	skipOnWindows(t)
	dir := shortTempDir(t)
	sock := filepath.Join(dir, "bus.sock") // never created

	fakeSpawn := func(SpawnConfig) (int, error) { return 0, ErrSpawnFailed }
	start := time.Now()
	_, err := Discover(DiscoverConfig{
		WorkDir:      dir,
		IPCEndpoint:  sock,
		ClientID:     "ding_x",
		Spawn:        fakeSpawn,
		DialDeadline: 150 * time.Millisecond,
		DialBackoff:  20 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("Discover should fail when bus never comes up within deadline")
	}
	if elapsed := time.Since(start); elapsed < 100*time.Millisecond || elapsed > 1*time.Second {
		t.Errorf("deadline-driven exit took %s, expected ~150ms", elapsed)
	}
}

func TestDiscover_ConcurrentCallersOnlyOneSpawn(t *testing.T) {
	skipOnWindows(t)
	dir := shortTempDir(t)
	sock := filepath.Join(dir, "bus.sock")

	// Use a mutex-guarded one-shot Spawn that actually starts the stub bus
	// on first call. Concurrent callers may race: the first wins (returns
	// success), the rest see ErrSpawnFailed but still retry-dial successfully.
	var spawnMu sync.Mutex
	var spawnCount atomic.Int32
	var closer func()
	t.Cleanup(func() {
		if closer != nil {
			closer()
		}
	})
	fakeSpawn := func(SpawnConfig) (int, error) {
		spawnMu.Lock()
		defer spawnMu.Unlock()
		spawnCount.Add(1)
		if closer == nil {
			closer = startStubBus(t, sock)
			return 99, nil
		}
		return 0, ErrSpawnFailed
	}

	const N = 5
	var wg sync.WaitGroup
	errs := make(chan error, N)
	conns := make([]net.Conn, 0, N)
	connsMu := sync.Mutex{}
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			conn, err := Discover(DiscoverConfig{
				WorkDir:      dir,
				IPCEndpoint:  sock,
				ClientID:     "ding_x",
				Spawn:        fakeSpawn,
				DialDeadline: 2 * time.Second,
			})
			if err != nil {
				errs <- err
				return
			}
			connsMu.Lock()
			conns = append(conns, conn)
			connsMu.Unlock()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Errorf("Discover concurrent caller: %v", err)
	}
	for _, c := range conns {
		_ = c.Close()
	}
	// Note: spawnCount can be 1..N because all goroutines fail dial first
	// and call Spawn. The point is they all SUCCESSFULLY connected to the
	// single bus that the first Spawn brought up.
	if len(conns) != N {
		t.Fatalf("only %d/%d callers got a conn", len(conns), N)
	}
}
