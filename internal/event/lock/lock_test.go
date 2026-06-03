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

package lock

import (
	"errors"
	"io"
	"path/filepath"
	"testing"
)

func TestTryAcquire_FirstCallerWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bus.lock")
	l, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("first TryAcquire: %v", err)
	}
	defer l.Close()
	if l.Path() != path {
		t.Fatalf("Path() = %q, want %q", l.Path(), path)
	}
}

func TestTryAcquire_SecondCallerGetsBusy(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bus.lock")
	l1, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("first TryAcquire: %v", err)
	}
	defer l1.Close()

	l2, err := TryAcquire(path)
	if !errors.Is(err, ErrBusy) {
		t.Fatalf("second TryAcquire: err = %v, want ErrBusy", err)
	}
	if l2 != nil {
		t.Fatal("on ErrBusy the returned lock must be nil")
	}
}

func TestTryAcquire_ReleasedLockIsReacquirable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bus.lock")
	l1, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("first TryAcquire: %v", err)
	}
	if err := l1.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	l2, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("re-acquire after close: %v", err)
	}
	defer l2.Close()
}

func TestTryAcquire_ContentReadWriteWhileHeld(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bus.lock")
	l, err := TryAcquire(path)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	defer l.Close()

	// Write PID-like content through the underlying handle
	const pid = "12345\n"
	if _, err := l.File().WriteString(pid); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Rewind and read back
	if _, err := l.File().Seek(0, io.SeekStart); err != nil {
		t.Fatalf("seek: %v", err)
	}
	buf := make([]byte, len(pid))
	if _, err := io.ReadFull(l.File(), buf); err != nil {
		t.Fatalf("read: %v", err)
	}
	if string(buf) != pid {
		t.Fatalf("read back = %q, want %q", buf, pid)
	}
}

func TestClose_NilSafe(t *testing.T) {
	var l *File
	if err := l.Close(); err != nil {
		t.Fatalf("nil Close should be no-op, got %v", err)
	}
}
