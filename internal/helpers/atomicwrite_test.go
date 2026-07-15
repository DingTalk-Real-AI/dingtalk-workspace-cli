package helpers

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestAtomicWrite_Basic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	data := []byte("hello world")

	if err := AtomicWrite(path, data, 0600); err != nil {
		t.Fatalf("AtomicWrite() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("got %q, want %q", got, data)
	}

	// Windows does not expose POSIX permission bits.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Fatalf("permissions = %o, want 0600", perm)
		}
	}
}

func TestAtomicWrite_CreatesDirectory(t *testing.T) {
	t.Parallel()

	base := t.TempDir()
	path := filepath.Join(base, "a", "b", "c", "test.txt")
	data := []byte("nested content")

	if err := AtomicWrite(path, data, 0644); err != nil {
		t.Fatalf("AtomicWrite() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("got %q, want %q", got, data)
	}
}

func TestAtomicWrite_Overwrite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	// Write initial content
	if err := AtomicWrite(path, []byte("initial"), 0600); err != nil {
		t.Fatalf("AtomicWrite() initial error = %v", err)
	}

	// Overwrite
	newData := []byte("overwritten content")
	if err := AtomicWrite(path, newData, 0600); err != nil {
		t.Fatalf("AtomicWrite() overwrite error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, newData) {
		t.Fatalf("got %q, want %q", got, newData)
	}
}

func TestAtomicWrite_NoTempFileOnSuccess(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	if err := AtomicWrite(path, []byte("content"), 0600); err != nil {
		t.Fatalf("AtomicWrite() error = %v", err)
	}

	// Check no .tmp files remain
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".tmp") {
			t.Fatalf("temp file remains: %s", e.Name())
		}
	}
}

func TestAtomicWriteFromReader_Basic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	data := "reader content"
	reader := strings.NewReader(data)

	n, err := AtomicWriteFromReader(path, reader, 0600)
	if err != nil {
		t.Fatalf("AtomicWriteFromReader() error = %v", err)
	}
	if n != int64(len(data)) {
		t.Fatalf("written bytes = %d, want %d", n, len(data))
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if string(got) != data {
		t.Fatalf("got %q, want %q", got, data)
	}
}

func TestAtomicWriteJSON_Basic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := []byte(`{"key": "value"}`)

	if err := AtomicWriteJSON(path, data); err != nil {
		t.Fatalf("AtomicWriteJSON() error = %v", err)
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() error = %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("got %q, want %q", got, data)
	}

	// Windows does not expose POSIX permission bits.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Fatalf("permissions = %o, want 0600", perm)
		}
	}
}
